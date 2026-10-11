package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/render"
)

// ⑥ 的服务端一半：/sw.js 与 `X-LB-Profile`。
//
// **哨兵值这一组是整套离线机制的安全地基**：sw.js 读不到 cookie，它能知道"这一页
// 属于谁"只因为服务端在每个 HTML 响应上带了 `X-LB-Profile`。而它据此决定
// **存不存进设备的缓存**——管理页那一份含全家庭的档案名 + 条目数 + rev + 快照数，
// 一旦哨兵改名或漏发，SW 会开始把那份东西留在别人的设备上，而**没有任何一环会
// 报错**：页面照常渲染、缓存照常工作、离线照常回退，只是从某天起那台机器的缓存里
// 多了一份全家概况。
func TestSW_哨兵值决定什么能被缓存(t *testing.T) {
	_, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))

	t.Run("档案页的哨兵就是那个 id", func(t *testing.T) {
		rec := get(h, "/", dad)
		if got := rec.Header().Get("X-LB-Profile"); got != "dad" {
			t.Errorf("档案页 X-LB-Profile = %q，要 \"dad\"（sw.js 据此存进 html-dad）", got)
		}
	})

	t.Run("选择页是 picker", func(t *testing.T) {
		rec := get(h, "/", "")
		if got := rec.Header().Get("X-LB-Profile"); got != "picker" {
			t.Errorf("选择页 X-LB-Profile = %q，要 \"picker\"", got)
		}
	})

	t.Run("管理页的哨兵是 ValidSlug 拒收的形状", func(t *testing.T) {
		rec := get(h, "/admin", dad)
		got := rec.Header().Get("X-LB-Profile")
		if got != render.AdminSentinel {
			t.Errorf("管理页 X-LB-Profile = %q，要 AdminSentinel(%q)", got, render.AdminSentinel)
		}
		// 形状本身也要钉：AdminSentinel 之所以长成 "-admin"，是因为
		// "admin" 是一个真能建出来的档案 id（§6.2）。哪天它变回合法 slug，
		// 这一条会红——而 sw.js 的 `-admin 一律不存` 判据也就跟着失效了。
		if got == "admin" || !strings.HasPrefix(got, "-") {
			t.Errorf("AdminSentinel 必须是一个合法 slug 拒收的形状（开头一个 '-'），实际 %q", got)
		}
	})
}

// /sw.js 自身的三个头。**no-cache 与 app.js 的 immutable 相反，是刻意的**：
// SW 的更新机制正是"每次导航比对这份文件的字节"，immutable 会让那次比对形同虚设。
func TestSW_静态端点的头(t *testing.T) {
	_, h := setupServer(t)

	rec := get(h, "/sw.js", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/sw.js 该 200，实际 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q，要 javascript——浏览器拒绝用别的类型注册 SW", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control = %q，要含 no-cache（immutable 会让更新检查形同虚设）", cc)
	}
	if etag := rec.Header().Get("ETag"); etag != "" {
		t.Errorf("不该有 ETag（no-cache 已经让它每次都被校验）：%q", etag)
	}
	if got := len(rec.Body.Bytes()); got != len(render.SWJS()) {
		t.Errorf("吐出 %d 字节，内嵌的是 %d 字节", got, len(render.SWJS()))
	}
}

// sw.js 里那几条判据是**它自己的**行为，而 JS 不在 Go 的测试面上。
// 于是这里读字节断言那几行字符串——它钉的不是"逻辑对"，而是
// "这几条约定与 spec §7.1 / 与文档还一致"：缓存名格式、超时值、不 skipWaiting、
// 不缓存 -admin。改了这几条而没改文档/没想清楚后果，这里会红。
func TestSW_关键约定在字节里(t *testing.T) {
	src := string(render.SWJS())
	for _, want := range []struct{ what, needle string }{
		{"缓存名按档案分桶", "'html-' + id"},
		{"回退的终点是选择页", "'picker'"},
		{"管理页一律不存", "id === '-admin'"},
		{"HTML 超时 1.5 秒（spec §7.1 的数）", "HTML_TIMEOUT_MS = 1500"},
		{"不静默抢占", "刻意不调 skipWaiting"},
		{"导航判据是 mode 而不是 URL", "req.mode === 'navigate'"},
	} {
		if !strings.Contains(src, want.needle) {
			t.Errorf("sw.js 里找不到%s（找的是 %q）", want.what, want.needle)
		}
	}
	if strings.Contains(src, "skipWaiting()") {
		t.Error("sw.js 不许调 skipWaiting()——§7.1 要求更新由用户决定，不静默抢占")
	}
}
