package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 拖拽的落点（2026-10-15 一次修三个洞）。三个洞都是**代码能编译、单测全绿、
// 只有真拖一次才看得见**的那种，所以这一族用两条证人钉：
//
//  1) 渲染出来的 HTML：每一条 `li[data-item]` 都必须能向上走到一个 `data-gid`
//     的容器——那是 op 里 path 的来源，走不到就等于把条目写进一个不存在的组。
//  2) app.js 逐字：落点的两个坐标（源组/目标组）与队尾那一档。
//
// 三条 bug 各自的样子：
//   · **队尾够不着**：从前只有 li 一种落点，index 就是被指着那条的下标，于是
//     「插到它前面」永远成立、「插到它后面」永远不成立。
//   · **跨组从来没通过**：服务端的 move 本来就支持（path 指源组、to 指目标组），
//     而前端把**目标组**的 gid 同时填进了两处——同组时两者相等，bug 完全隐形。
//   · **大卡带里连 gid 都取不到**：带子的组容器是 `ul[data-gid]` 而不是 section。

// 一、渲染层：每一条都要能走到自己那一组。
func Test每一条书签都能向上走到自己的组(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 4
	b.Groups = []model.Group{
		{ID: "hot", Title: "每天必点", Layout: model.LayoutFeature,
			Items: []model.Item{{ID: "h1", Name: "一", URL: "http://10.0.2.3:5000"}}},
		{ID: "hot2", Title: "也常点", Layout: model.LayoutFeature,
			Items: []model.Item{{ID: "h2", Name: "二", URL: "http://10.0.2.4:5000"}}},
		{ID: "cold", Title: "一年一次", Layout: model.LayoutTiles,
			Items: []model.Item{{ID: "c1", Name: "三", URL: "http://10.0.2.5:9000"}}},
		{ID: "list", Title: "单列", Layout: model.LayoutList,
			Items: []model.Item{{ID: "l1", Name: "四", URL: "http://10.0.2.6:9000"}}},
	}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	p, err := renderer(t, s).ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)

	// 每一条的 data-item 后面紧跟的是它自己的容器往上找到的第一个 data-gid。
	// 用「从这一条往回找最近的那个 data-gid」来验，而不是数标签——数标签数不出
	// 「这一条到底属于谁」。
	for _, id := range []string{"h1", "h2", "c1", "l1"} {
		if !strings.Contains(body, `data-item="`+id+`"`) {
			t.Fatalf("%s 根本没渲染出来：\n%s", id, bandOnly(body))
		}
	}
	// 带子里两段各自是独立容器：**带子的外层 section 没有 data-gid**。这正是从前
	// `closest('section')` 取到 undefined 的那一步，所以反过来钉：它不许有。
	bandStart := strings.Index(body, `<section class="feat"`)
	if bandStart < 0 {
		t.Fatalf("没有大卡带：\n%s", bandOnly(body))
	}
	bandOpen := body[bandStart : bandStart+90]
	if strings.Contains(bandOpen, "data-gid") {
		t.Errorf("大卡带的外层 section 带着 data-gid：\n%s", bandOpen)
	}
	// 两个大卡组各自一个 data-gid 容器（ul），两个正文组各自一个（section）。
	if n := strings.Count(body, `data-gid="`); n != 4 {
		t.Errorf("整页有 %d 个组容器，要 4 个（四组各一个）：\n%s", n, bandOnly(body))
	}
	// **每一组都要有组标题**：那是「拖到队尾」的落点，空组尤其只有它。
	if n := strings.Count(body, "<h2"); n != 4 {
		t.Errorf("组标题只有 %d 个，四个组该各有 1 个（带子外层 section 不带 h2）：\n%s", n, bandOnly(body))
	}
	for _, gid := range []string{"hot", "hot2", "cold", "list"} {
		if !strings.Contains(body, `data-group="`+gid+`"`) {
			t.Errorf("组 %s 没有 h2[data-group]：拖到它标题上排不到队尾", gid)
		}
	}
}

// 二、app.js：三个洞各自的形状。
func Test拖拽落点的三个坐标(t *testing.T) {
	// **剥掉注释再扫**：这一族自己那几段注释里就写着从前那句 `closest('section')`
	//（那是留给下一个人的路标），照着原文数会把注释算成代码。
	js := stripJSComments(readAppJS(t))

	// 组容器认 [data-gid]。**判据是这一句本身**，不是"哪里用对了"——从前是
	// `closest('section')`，而大卡带里那一段没有自己的 section。
	if !strings.Contains(js, "const groupOf = (node) => node.closest('[data-gid]');") {
		t.Error("app.js 里没有 groupOf：[data-gid] 才是组容器的判据")
	}
	if n := strings.Count(js, "closest('section')"); n != 0 {
		t.Errorf("app.js 里还有 %d 处 closest('section')：大卡带的组容器不是 section", n)
	}

	// op：path 指**源组**、to 指**目标组**。
	if !strings.Contains(js, "path: `groups/${src.gid}/items/${src.id}`, to: dst, index }") {
		t.Error("li 落点那条 op 的 path 不是源组：跨组时它会指向一个不含这条目的组")
	}
	// 反向证人：从前那一句是 `path: groups/<目标组>/items/<被拖的>，to: <同一个 gid>`。
	for _, bad := range []string{
		"path: `groups/${gid}/items/${moved}`, to: gid",
		"path: `groups/${dst}/items/",
	} {
		if strings.Contains(js, bad) {
			t.Errorf("app.js 里还留着把目标组当源组的写法：%s", bad)
		}
	}

	// 队尾：插前/插后由指针位置决定。（2026-10-16 委托化之后，落点由
	// `dropTargetOf` 算成 `{node, after}`，于是这一句的落点变量名从 `li`/`after`
	// 变成 `t.node`/`t.after`——判据没变，只是换了个拿法。）
	for _, want := range []string{
		"return e.clientY > r.top + r.height / 2;",
		"let index = sibs.indexOf(t.node) + (t.after ? 1 : 0);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js 里少了 %q：队尾永远够不着", want)
		}
	}

	// 组级落点存在，且**刻意不带 index**（服务端 index 缺席即追加到末尾）。
	head := js[strings.Index(js, "h2[data-group]"):]
	if !strings.Contains(head, "path: `groups/${src.gid}/items/${src.id}`, to: dst }") {
		t.Error("拖到组标题那条 op 没有省略 index：自己再算一遍下标正是队尾那个 bug 的来路")
	}
}

// 三、批量动作那份「移动到」名单要含**每一组**。
func Test批量移动的名单里有大卡组(t *testing.T) {
	js := readAppJS(t)
	if !strings.Contains(js, "document.querySelectorAll('[data-gid]')") {
		t.Error("批量移动的目标名单不是 [data-gid]：大卡组不在里面")
	}
	if strings.Contains(js, "document.querySelectorAll('section[data-gid]')") {
		t.Error("批量移动的目标名单还写着 section[data-gid]")
	}
}

// 四、把手与拖拽认的那两种形态之外还有第三种：**大卡**。
//
// 带子里可点的那张卡是 `<a class="feat-card">` —— 既不带 `card` 也不带 `plain`，
// 而下面那句 `li.querySelector('.card') || li.querySelector('.plain')` 是**两种形态
// 的兜底**，于是 `if (!card) continue` 把带子里每一条可点的书签整条跳过：
// 没有 ✎、没有 draggable、没有落点。
//
// 症状特别容易读成别的：带子里点不动的那几条（`span class="feat-card plain"` 带
// `plain`）**是好的**，于是同一个组里"有的能拖有的不能"，而分组与索引都无辜。
// 钉的是**三种形态一个都不能少**。
func Test大卡那一组的条目也要有把手与拖拽(t *testing.T) {
	js := stripJSComments(readAppJS(t))
	if !strings.Contains(js, "li.querySelector('.card, .feat-card, .plain')") {
		t.Errorf("把手/拖拽那一句没认大卡形态：\n%s", firstLines(js, "const card = li.querySelector", 3))
	}
	// ⌘K 读地址与编辑模态读地址**从 2026-10-16 起不再是两处各自的查找**，而是同一个
	// `itemURL`（卡片三种形态各有一个地址载体：大卡带里点不动的外部地址那一行屏上不印
	// 地址，模板写在 data-addr 上）。所以这里钉的是"两处都走它"与"它认全三种形态"，
	// 而不是某一处选择器的字面。
	if n := strings.Count(js, "itemURL(li)"); n < 2 {
		t.Errorf("读地址只有 %d 处走 itemURL，要 ≥2 处（⌘K 与编辑模态）："+
			"各写一遍查找的形状就是「其中一种卡片形态读成空串」", n)
	}
	if !strings.Contains(js, "const itemURL = (li) =>") {
		t.Fatal("app.js 里没有 itemURL：读地址的判据散回各处了")
	}
	for _, want := range []string{"li.querySelector('a[href]')", "li.querySelector('code')", "li.querySelector('[data-addr]')"} {
		if !strings.Contains(js, want) {
			t.Errorf("itemURL 缺 %q：三种卡片形态各有一个地址载体，少一种就有一类读成空串"+
				"（编辑模态那一处会整批 422，屏上只说「这一条没存上」）", want)
		}
	}
	// 反向证人：从前那两句还在。
	for _, old := range []string{"li.querySelector('.card');", "li.querySelector('.card, .feat-card')"} {
		if strings.Contains(js, old) {
			t.Errorf("app.js 里还留着 %q：大卡里读不到地址", old)
		}
	}
	if strings.Contains(js, "li.querySelector('.card') || li.querySelector('.plain')") {
		t.Error("app.js 里还留着「两种形态」的兜底写法")
	}
	// 模板那一侧：载体得真的印出来，否则 itemURL 的第三支永远是空。
	tmplRaw, err := webFS.ReadFile("web/profile.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tmplRaw), `<span class="feat-card plain" data-addr="{{.URL}}">`) {
		t.Error("profile.tmpl 的大卡带没有在外部地址那张卡上写 data-addr：" +
			"那是全案唯一一处屏上不印地址的形态")
	}
}

func firstLines(src, from string, n int) string {
	i := strings.Index(src, from)
	if i < 0 {
		return "(找不到 " + from + ")"
	}
	s := src[i:]
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
