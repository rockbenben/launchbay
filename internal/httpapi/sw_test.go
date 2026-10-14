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

// 面板那一路 fetch 不许进资产缓存（2026-10-15）。
//
// **这个 bug 比它拦住的那件事老得多**：app.js 的 prefsOpen 用
// `fetch('/prefs', {Accept:'text/html'})` 取面板再注入，而那是**同源 fetch、
// mode 不是 navigate**，于是从前落到 cacheFirst —— 被 `assets-lb-sw-1` 钉住。
// 而那一族是**指纹资源**的缓存（immutable），钉住就是永久。
//
// 实测症状：改完一个颜色再打开设置，屏上还是**第一次打开时**的颜色，
// 而同一刻 `GET /api/v1/data` 与盘上都是新值。于是"设置改了没生效"与
// "按了没反应"同时成立，而两者指向的都不是原因。
//
// 钉三样，缺一样就还会复发：
//  1. 判据是 **Accept**（不是 URL 形状）——`/prefs` 只是眼下唯一一处；
//  2. 这一句排在 **navigate 那一支之后**——排前面的话导航就再也走不到
//     htmlFirst，离线回退会整个丢掉；
//  3. 代次**不是 lb-sw-1**——已被钉住的那份就躺在那个桶里，而 activate 只清
//     assets-<别的代次>；不换代次它会一直留着，将来谁调一下判断顺序就复发。
func TestSW_面板那一份不进资产缓存(t *testing.T) {
	src := string(render.SWJS())
	const guard = "includes('text/html')) return;"
	iGuard := strings.Index(src, guard)
	if iGuard < 0 {
		t.Fatal("sw.js 里没有「Accept 含 text/html 就不接管」那一支")
	}
	iNav := strings.Index(src, "req.mode === 'navigate'")
	if iNav < 0 {
		t.Fatal("sw.js 里找不到导航那一支")
	}
	if iGuard < iNav {
		t.Error("那一支排在导航之前：导航会绕过 htmlFirst，离线回退整个丢掉")
	}
	// 资产缓存那一族只放指纹资源。面板那份 HTML 进去过一次就要被删干净。
	// 判据带引号：**这一族读的是字节（含注释）**，而 sw.js 那段解释"为什么换代次"
	// 的注释里就写着 lb-sw-1 —— 按裸串判的话，那条注释会把这个闸自己弄红。
	if strings.Contains(src, "'lb-sw-1'") {
		t.Error("代次还是 lb-sw-1：那份被钉住的面板 HTML 还在桶里")
	}
	if !strings.Contains(src, "const ASSET_CACHE = 'assets-' + VERSION;") {
		t.Error("资产缓存的名字不该与代次脱钩——activate 那一支靠前缀清旧代次")
	}
}
