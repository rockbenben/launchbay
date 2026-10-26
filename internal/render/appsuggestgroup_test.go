package render

import (
	"os"
	"strings"
	"testing"
)

// `suggestedGroup` 是**对象 `{id,title}`**，不是分组 id 字符串（2026-10-19 修）。
//
// 服务端 `resolveResponse.SuggestedGroup` 是 `*resolveGroup{ID,Title}`，JSON 出来
// 是 `{"id":"...","title":"..."}`。两处消费方都把它当字符串用了：
//
//   - `internal/render/web/app.js` 浮层那格：把整个对象跟 `o.value`（字符串）比，
//     恒不相等 —— **建议分组一次都没被选上**，浮层永远停在默认的第一组。
//   - `extension/bg.js` 右键保存：整个对象塞进 `picked.group`，下面拿它跟
//     `g.id` 比，同样恒 false —— **每条都落进第一个分组**，而那段注释还写着
//     "已修好"。
//
// 判据是**形状只有一处**：消费方一律取 `.id`。所以这条钉两处都用 `.id`，
// 且都不许把整个对象直接跟字符串比。
func TestResolve的建议分组是对象消费方要取id(t *testing.T) {
	app := readAppJS(t)
	ext := readExtensionJS(t)

	// 1) 浮层那格：取 want = r.suggestedGroup.id，再拿 want 比 o.value
	if !strings.Contains(app, "const want = r.suggestedGroup.id;") {
		t.Error("app.js 浮层那格没有取 `r.suggestedGroup.id`：整个对象跟字符串比恒不相等，建议分组一次都没被选上")
	}
	// 2) 扩展那处：picked.group.id
	if !strings.Contains(ext, "g.id === (picked.group && picked.group.id)") {
		t.Error("extension/bg.js 没有取 `picked.group.id`：整个对象跟字符串 id 比恒为 false，每条都落进第一个分组")
	}

	// 反向证人：整个对象跟字符串比的那两种写法都不许回来。
	// （它们看着像对的——服务端确实给的是这个形状——所以要显式禁掉。）
	for _, bad := range []string{
		"o.value === r.suggestedGroup}",
		"fGroup.value = r.suggestedGroup;",
		"g.id === picked.group)",
		"g.id === picked.group;",
	} {
		if strings.Contains(app, bad) || strings.Contains(ext, bad) {
			t.Errorf("又出现把整个 suggestedGroup 对象跟字符串比的写法 %q", bad)
		}
	}
}

// readExtensionJS 读扩展里的一个文件。扩展不在本包的作用域里，但**它与 app.js
// 消费的是同一份服务端契约**（`/api/v1/resolve` 的响应形状），所以两处一起钉。
func readExtensionJS(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../extension/bg.js")
	if err != nil {
		t.Fatalf("extension/bg.js 读不到: %v", err)
	}
	return string(raw)
}

// 扩展派生的条目 id 要能**区分只差 query 的两个地址**（2026-10-19）。
//
// 从前 `idFor` 只取「主机名首段 + 路径末段」，于是两个 YouTube 视频派生出同一个
// id（`youtube-watch`）。扩展先按 URL 判重（两个 URL 不同 → 不是重复），再按 id
// 写入 —— 两次判据不一致，于是第二个**根本没存进去**，服务端答 422
// `duplicate-id`，而扩展把这句话翻成「这一条已经在这份里了」，把人支去改另一条。
//
// 判据是**地址不同就该是不同的一条**，所以 query 与 hash 都要进 id；超长还要
// 加哈希后缀（光截断的话，两个 query 很长但前 32 字相同的地址仍然撞）。
//
// **上限是 32**（2026-10-20 修）：服务端收的是 model.ValidSlug 的 32
// （`^[a-z0-9][a-z0-9-]{0,31}$`），而扩展从前截到 40——于是凡是派生 id 超过
// 32 的地址，**扩展一条书签都存不进去**（整批 422），而地址本身一点问题都没有。
// app.js 那边 2026-10-16 就改好了（它的 `ascii()` 用 SLUG_MAX=32）。
func Test扩展的条目id分得开只差query的地址(t *testing.T) {
	ext := readExtensionJS(t)
	if !strings.Contains(ext, "(u.search || '') + (u.hash || '')") {
		t.Error("extension/bg.js 的 idFor 没把 query 与 hash 算进去：只差 query 的两个地址派生出同一个 id，第二个存不进去却被说成「已经存过」")
	}
	if !strings.Contains(ext, "base.slice(0, 32 - h.length - 1) + '-' + h") {
		t.Error("extension/bg.js 的 idFor 超长时没加哈希后缀（或上限不是 32）：两个 query 很长但前 32 字相同的地址仍然撞 id")
	}
	if strings.Contains(ext, "base.length > 40") || strings.Contains(ext, "slice(0, 40") {
		t.Error("extension/bg.js 的 id 上限还是 40，而服务端只收 32：凡是派生 id 超 32 的地址整批 422，一条都存不进去")
	}
}

// **两份 id 派生的基座也必须同形**（2026-10-20 补上）。这一族出过的两次事都是
// "两边各派一份"造成的：
//   - 第一次是 query 漏掉（duplicate-id 说了假话）；
//   - 这一次是扩展的基座与页面**分家**了（扩展多算了 query 与 hash，页面没算），
//     于是同一个地址在两边得到两个 id——同一条书签被**存成两条**（地址完全相同）。
//
// 判据钉**形状**：两边都要用「整条路径 + query + hash」，而上限都取服务端那 32。
// 只取末段时，`github.com/userA/project` 与 `github.com/userB/project` 会派生出
// 同一个 id，撞 id 时那句「这一条已经在这份里了」是**假话**。
func Test两份id派生的基座同形(t *testing.T) {
	for _, src := range []struct {
		what string
		js   string
	}{
		{"app.js", readAppJS(t)},
		{"extension/bg.js", readExtensionJS(t)},
	} {
		if !strings.Contains(src.js, `(u.pathname.split('/').filter(Boolean).join('-') || '')`) {
			t.Errorf("%s 的 id 基座只取路径末段：两个不同的页面会派生出同一个 id，"+
				"撞 id 时那句「这一条已经在这份里了」是假话", src.what)
		}
		if !strings.Contains(src.js, `(u.search || '') + (u.hash || '')`) {
			t.Errorf("%s 的 id 基座没算 query 与 hash：只差 query 的两个地址会撞 id", src.what)
		}
	}
	// 上限：两边都得是 32（服务端 model.ValidSlug 的上限）——扩展从前是 40，于是
	// 凡是派生 id 超 32 的地址，扩展整批 422，一条都存不进去。
	if !strings.Contains(readAppJS(t), "SLUG_MAX = 32") {
		t.Error("app.js 的 ascii() 不再是 SLUG_MAX = 32：与服务端 model.ValidSlug 的上限对不上了")
	}
	if strings.Contains(readExtensionJS(t), "base.length > 40") {
		t.Error("extension/bg.js 的 id 上限还是 40，而服务端只收 32：凡是派生 id 超 32 的地址整批 422")
	}
}

// **两份 id 派生必须同形**（2026-10-19）。这一族已经因此出过两次事：
// 第一次是 query 漏掉（duplicate-id 说了假话），第二次更重——
//
//	app.js 的 `slug(text, true)` 对**裸 IP 走哈希**（`ip-<hash>`），而
//	`extension/bg.js` 的 `idFor` 一律取 `hostname` 的第一段，于是
//	`10.0.2.3:6789` 在扩展里变成 `10-…`：
//	  - 同一主机的第二个服务**撞 id**，症状是「明明填对了地址却存不进去」；
//	  - 而更早一版是整串 ascii，得到 `192-168-2-36789` 那种——它把**内网地址与
//	    端口烘进了 id**，而 `GET /api/v1/export` 发的是磁盘原字节，于是导出文件
//	    发给别人就等于把这段拓扑一起发出去。**app.js 那边早就是哈希了，只有扩展
//	    漏了。**
//
// 判据钉**形状**而不是逐字实现：两边都必须有「四段全是数字」那一支，而且都必须
// 把它变成 `ip-` 开头的哈希。逐字钉会让一次无害的重构变成红线，而这里要防的是
// 「一边的保护没了」。
//
// **2026-10-26 补路径**：只看主机+端口仍然会把同一 NAS 端口下的 `/photo` 与
// `/music` 撞成一条；两边都要把 pathname / query / hash 一起算进哈希。
func Test两份id派生对裸IP都走哈希(t *testing.T) {
	const ipv4Guard = `/^\d{1,3}(\.\d{1,3}){3}$/`

	for _, src := range []struct {
		what string
		js   string
	}{
		{"app.js", readAppJS(t)},
		{"extension/bg.js", readExtensionJS(t)},
	} {
		if !strings.Contains(src.js, ipv4Guard) {
			t.Errorf("%s 没有「四段全是数字」那一支：裸 IP 会进可读那条，于是 id 里烘进内网地址与端口，"+
				"而 GET /api/v1/export 发的是磁盘原字节——导出文件发给别人就等于把这段拓扑一起发出去", src.what)
		}
		if !strings.Contains(src.js, `'ip-' + (hash36(host + (u.port || '') + u.pathname + (u.search || '') + (u.hash || '')) || 'item')`) {
			t.Errorf("%s 没有把裸 IP 变成 ip-<主机+端口+路径>：同一主机的第二个服务（换端口或换路径）会撞 id，症状是「明明填对了地址却存不进去」", src.what)
		}
	}
	// 两边**逐字同形**：不是"都有这一支"就够了——host+port+path+query+hash 一起哈希
	// 才既不泄漏拓扑、又分得开同一端口下的不同页面；漏掉路径就会让 `/photo` 与
	// `/music` 撞回同一个 id。
	if !strings.Contains(readExtensionJS(t), `'ip-' + (hash36(host + (u.port || '') + u.pathname + (u.search || '') + (u.hash || '')) || 'item')`) {
		t.Error("extension/bg.js 的哈希没带路径：同一 IP:端口的不同页面会撞回同一个 id")
	}
}
