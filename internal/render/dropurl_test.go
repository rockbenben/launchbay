package render

import (
	"strings"
	"testing"
)

// 把书签栏/地址栏上的一个链接拖到档案页上（2026-10-16）。
//
// 判据钉的是**三件最容易写错、且症状都是"功能测试全绿、屏上没反应"的事**：
//
//  1. **三种引擎给的键不一样**。从书签栏/地址栏拖到页面上时，Chrome 与 Safari
//     给 `text/uri-list`，**Firefox 不给**，只给它的私有 `text/x-moz-url`
//     （外加 text/plain/text/html）。Mozilla 把它记成缺陷：bugzilla 1912164
//     （P3，2024-08-07 开；报告人实测 Chrome@127 与 Safari@17.5 都有 uri-list，
//     "The issue only exists when dragging from the Firefox address bar /
//     bookmarks into Firefox"，未修）。只认第一支 = 在 Firefox 上整条不成立。
//  2. **dragover 不写 preventDefault，drop 根本不会派发**——而那不报错、不抛
//     异常，只是松手之后什么也没发生。
//  3. **站内拖动要挡掉**：这一页自己的卡片拖动**同时**带 text/uri-list
//     （app.js 那条 dragstart，为的是能拖到书签栏去），而重排书签带的是
//     `application/x-lb-item`。不挡就会把自己的卡片再拖一遍进添加浮层。
//
// 另外两条是产品的形状，不是实现细节：**拖进来不自动保存**（与书签栏按钮
// 同一条路，预填 + 打开浮层 + 还要人按一次保存，§6.3），以及**只收 http/https**
// （javascript:/data: 服务端本来就拒，前端不重复发明）。
func Test拖进来的链接直接落库且不被站内排序吃掉(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	// 一、三种键一个都不能少（顺序从可靠到兜底）。
	for _, key := range []string{"text/uri-list", "text/x-moz-url", "text/plain"} {
		if !strings.Contains(js, "'"+key+"'") {
			t.Errorf("没有读 %s：Chrome/Safari 给 uri-list、Firefox 只给 x-moz-url，"+
				"少哪一支就在那个浏览器上整条不成立", key)
		}
	}

	// 二、**必须在捕获阶段**并 stopPropagation。这是本条最容易写错、且症状完全
	// 不像 bug 的那一处：卡片的 dragover/drop 为了站内排序调了 stopPropagation，
	// 于是挂在 document 上的**冒泡**监听永远收不到拖到卡片上的那一次——
	// 屏幕上看不出任何异常，只有"拖进来没反应"。冒泡监听挂在哪一层会被这一条
	// 连着判（`}, true)`），站内拖动原样放行由 dragIsURL 那句前缀守着。
	if n := strings.Count(js, "}, true);"); n < 2 {
		t.Errorf("捕获阶段的 dragover/drop 只有 %d 处，要 2 处；"+
			"冒泡那一层收不到拖到卡片上的 drop（卡片的处理器 stopPropagation 了）", n)
	}
	for _, want := range []string{"e.stopPropagation();"} {
		if !strings.Contains(between(js, "document.addEventListener('drop'", "}, true);"), want) {
			t.Errorf("drop 那一段缺 %s：外来链接要在这里按停，否则还会走一遍站内排序", want)
		}
	}
	if !strings.Contains(between(js, "document.addEventListener('dragover'", "}, true);"), "e.preventDefault()") {
		t.Error("dragover 没 preventDefault：浏览器不会派发 drop，松手之后什么也不发生")
	}

	// 三、站内拖动按**自家那套 MIME**挡掉，不靠"猜是不是 uri-list"。
	guard := between(js, "const dragIsURL = (dt) =>", "};")
	if !strings.Contains(guard, "application/x-lb") {
		t.Errorf("dragIsURL 没有排除站内拖动：这一页的卡片拖动也带 text/uri-list，"+
			"会被当外来链接再接一次\n%.300s", guard)
	}

	// 四、**直接存，不问**（2026-10-16 改判）：落点是写盘 + 重载，
	// 而不是打开添加浮层让人再按一次保存。
	save := between(js, "async function saveDropped", "\n  }")
	for _, want := range []string{"applyOps(ops,", "spliceCardIn(gid, value.id, url, dropAt)", "showBanner("} {
		if !strings.Contains(save, want) {
			t.Errorf("saveDropped 缺 %q：拖进来的链接是直接存下去，存没存成要有一句话说\n%.400s", want, save)
		}
	}
	if strings.Contains(save, "openSheet()") {
		t.Error("saveDropped 里打开了添加浮层：这次改判就是不要那一步确认——" +
			"拖放的落点已经说了「存进哪一组」")
	}
	if strings.Contains(save, "reloadKeepingPlace(") {
		t.Error("saveDropped 里还在重载：新卡片要**从松手的位置滑进它自己的格子**（2026-10-16），" +
			"重载会把它换成一次整页重画")
	}

	// 接卡片那一段：抓服务端的片段、只接一行、给它补挂监听、从松手处滑过来。
	sp := between(js, "async function spliceCardIn(", "\n  }")
	for _, want := range []string{"fetch('/', ", `li[data-item="' + item + '"]`, `a[href="`, "bindItem(", "flip(new Map([[li, from]]))"} {
		if !strings.Contains(sp, want) {
			t.Errorf("spliceCardIn 缺 %q：新节点的把手/拖动监听一个都没有，不补挂就是一条死卡片\n%.500s", want, sp)
		}
	}
	// **在目标组那一个 `<ul>` 里找**（判据是 listOf，见 sortorder_test.go）：大卡带的
	// 组容器是 `ul.feat-row[data-gid]` 自己，写死 section 就永远找不到它——接不上，
	// 于是每一次拖进大卡组都静悄悄退回整页重载。
	if !strings.Contains(sp, "const list = listOf(gid, doc);") {
		t.Error("spliceCardIn 找那一行时没有用 listOf：大卡带里同一组的容器是 ul 自己")
	}
	// 空组那句「这一组还没有书签」要跟着走：拖进去的就是**第一条**。
	if !strings.Contains(sp, "syncEmpty(ul)") {
		t.Error("spliceCardIn 没有同步空组那一句：拖进空组之后卡片在那儿、那一句还在说这一组是空的")
	}
	// **先按 id、再按地址**（2026-10-16 复查）：id 是我们发过去的那个、服务端原样落盘，
	// 它罩得住三种卡片形态（可点的 / 点不动的 / 大卡的），而 `a[href=…]` 只罩得住
	// 带 href 的那两种；地址留作兜底。
	if strings.Index(sp, `li[data-item="' + item + '"]`) > strings.Index(sp, `a[href="`) {
		t.Error("spliceCardIn 先按地址找了：那只罩得住可点的那一种卡片形态")
	}
	// 那一发**必须带 Accept: text/html**：sw.js 只放行「Accept 里带 text/html」的
	// HTML 请求（给 `/prefs` 那一路留的规矩）。不带就落到资产缓存那条 cache-first，
	// 第一次执行时被存进去、之后恒读到那一份——症状是新卡片接不上、静悄悄退回
	// 重载，看上去"有时滑、有时闪"，像随机（与 sw.js 里 `/prefs` 那段同一个形状）。
	if !strings.Contains(sp, "Accept: 'text/html'") {
		t.Error("spliceCardIn 那一发 fetch 没带 Accept: text/html：它会命中资产缓存里第一次存进去的旧 HTML")
	}
	// 卡片标记**只能由服务端画**：脚本再画一份就是"同一份 define 在两处落地"。
	if strings.Contains(sp, "svgMarkup") || strings.Contains(sp, "innerHTML =") {
		t.Error("spliceCardIn 在自己拼卡片的标记：图标那段是全案唯一 svgMarkup 的出口")
	}
	// 接不上就退回重载：宁可旧样子，不能屏上没有而盘上有。
	if !strings.Contains(sp, "fallback(") {
		t.Error("spliceCardIn 没有退回重载的分支：抓不到那一行时，屏上会少一条而盘上多一条")
	}

	// 存成之后那两样东西都得**活过这一下**：横幅是页面里画出来的节点，而新卡片是
	// 上一条那条断言说的"接进来"——两者都必须在**不重载**的前提下出现，
	// 所以 gid 与条目 id 一路带进 spliceCardIn，由它按那一刻的 DOM 取组名。
	if !strings.Contains(js, "T('app.added.saved', groupTitle(gid))") {
		t.Error("成功那句没有从接进来的 DOM 里取组名：组名是服务端渲染的，重载之前不在页面上")
	}
	if !strings.Contains(js, "classList.add('is-new')") {
		t.Error("没有 is-new 那个落位：它是这一条唯一的入场反馈（FLIP 走的是位移，不是落位）")
	}

	// 五、两处不写就等于默默丢东西的分支，各有一句要说清的话。
	for _, want := range []string{"app.drop.dup", "app.drop.noGroup"} {
		if !strings.Contains(js, want) {
			t.Errorf("没有 app.%s 那一句：%s", want, "同一个地址拖两次不该多存一条" /* dup */)
		}
	}
	// 只收 http/https 的整段地址：从地址栏选一段文字拖进来时负载常是
	// 「https://x.com 搜索词」，多带一个词就不该当成地址。
	parse := between(js, "const urlFromDrag = (dt) =>", "\n  };")
	if !strings.Contains(parse, "^https?:\\/\\/") {
		t.Errorf("没有按协议与整段形状过滤：\n%.400s", parse)
	}
	if !strings.Contains(parse, "startsWith('#')") {
		t.Errorf("没有跳过 uri-list 的注释行：那是行格式规范的一部分\n%.400s", parse)
	}
}

// between 取 from 之后、to 之前的那一段（找不到就退回空串，让断言去说"缺哪一句"）。
func between(s, from, to string) string {
	i := strings.Index(s, from)
	if i < 0 {
		return ""
	}
	rest := s[i+len(from):]
	j := strings.Index(rest, to)
	if j < 0 {
		return rest
	}
	return rest[:j]
}
