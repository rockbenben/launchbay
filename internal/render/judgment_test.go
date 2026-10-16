package render

import (
	"strings"
	"testing"
)

// 「一个判据一个出口」这条纪律的机械守卫（2026-10-16）。
//
// 这个项目的 bug 反复是同一句：**同一个判断在两处各写一份，于是其中一份覆盖不到
// 某种形状**，症状永远是"屏上与盘上分家"或"某一种卡片形态行为不一样"，而没有任何
// 报错。2026-10-15/16 两轮里被抓到的六处都是这个形状：
//
//	组容器有两种（section[data-gid] > ul / ul[data-gid] 自己）→ 排序退回重载、拖网址进大卡落进第一组
//	一行该有的监听绑了两轮（bindItem 一轮、长按多选一轮）→ 拖进来的书签长按不进多选
//	地址的载体三样（href / <code> / data-addr）→ 大卡带里的外部地址读成空串、保存 422
//	组标题两份查找 → 带子里任何一条都拿到第一段的组名
//	计数用 href 前缀匹配 → news 与 news2 连坐
//	多选搬组每条都给 index → 选中顺序整个反过来
//
// 下面每一条钉的是「出口只有一处」，不是某一句写法的字面——写法可以换，出口不能多。
func Test一个判据一个出口(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	body := stripJSComments(js)

	t.Run("一行该有的监听只从bindItem出去", func(t *testing.T) {
		bi := between(js, "function bindItem(li) {", "\n  }")
		if !strings.Contains(bi, "bindMulti(li);") {
			t.Errorf("bindItem 没有绑长按多选那一族：拖进来的新行只补了把手/复制/拖动，\n"+
				"于是它长按不进多选、在多选里点它也不切换（实测：横扫那一步红）\n%.400s", bi)
		}
		// 幂等：它会 append 一颗把手，绑两遍就是两颗。
		if !strings.Contains(bi, "lbItemBound") {
			t.Error("bindItem 不是幂等的：同一个 li 绑两遍就有两颗把手")
		}
		if !strings.Contains(js, "function bindMulti(li) {") {
			t.Fatal("app.js 里没有 bindMulti")
		}
		// **行上**的 pointerdown 只许出现在 bindMulti 里（document 上那一条是"点外面
		// 关掉最上面那层"，与行无关，不算进来——所以这里比的是带 li. 的那一句）。
		if n := strings.Count(body, "li.addEventListener('pointerdown'"); n != 1 {
			t.Errorf("行上的 pointerdown 监听有 %d 处，要 1 处（bindMulti）："+
				"再有一处就是新行又少一族监听", n)
		}
	})

	t.Run("读地址只从itemURL出去", func(t *testing.T) {
		if !strings.Contains(js, "const itemURL = (li) =>") {
			t.Fatal("app.js 里没有 itemURL")
		}
		iu := between(js, "const itemURL = (li) =>", "\n  };")
		for _, want := range []string{"li.querySelector('a[href]')", "li.querySelector('code')", "li.querySelector('[data-addr]')"} {
			if !strings.Contains(iu, want) {
				t.Errorf("itemURL 缺 %q：三种卡片形态各有一个地址载体\n%.400s", want, iu)
			}
		}
		// 读 <code> 的地方只许有 itemURL 那一处（拖动那份读的是 href，不是 code）。
		if n := strings.Count(body, "querySelector('code')"); n != 1 {
			t.Errorf("读 <code> 的地方有 %d 处，要 1 处（itemURL）："+
				"各处自己读就会漏掉没有 <code> 的那种形态", n)
		}
		// 至少两个调用点（⌘K 与编辑模态）。
		if n := strings.Count(js, "itemURL(li)"); n < 2 {
			t.Errorf("itemURL 只有 %d 个调用点，要 ≥2", n)
		}
	})

	t.Run("组标题只从groupTitle出去", func(t *testing.T) {
		gn := between(js, "function groupName(group) {", "\n  }")
		if !strings.Contains(gn, "groupTitle(group.dataset.gid)") {
			t.Errorf("groupName 没有走 groupTitle：两份查找只覆盖了正文那一段，\n"+
				"带子里一段段共用一个 section，按「最近的那个 h2」会拿到第一段的组名\n%.400s", gn)
		}
		// 读叶名的只许 groupTitle 一处（groupName 处理 H2 本身时是同一句的另一形态）。
		if n := strings.Count(js, "querySelector('.sec-leaf')"); n > 2 {
			t.Errorf("读叶名的写法有 %d 处，要 ≤2（groupTitle 与 groupName 的 H2 分支）", n)
		}
		gt := between(js, "function groupTitle(gid) {", "\n  }")
		if !strings.Contains(gt, `h2[data-group="' + gid + '"]`) {
			t.Errorf("groupTitle 不按 h2[data-group] 找：大卡带的标题挂在那儿，不在 section 底下\n%.300s", gt)
		}
		if strings.Contains(js, `section[data-gid="' + gid + '"] h2`) {
			t.Error("还有地方按 section[data-gid] h2 取组名：大卡组会显示成组 id")
		}
	})

	t.Run("计数按整段href比而不是前缀", func(t *testing.T) {
		bc := between(js, "function bumpCount(gid, by) {", "\n  }")
		if strings.Contains(bc, `a[href^="#g-' + gid + '"]`) {
			t.Errorf("bumpCount 还在用 href 前缀匹配：`#g-news` 会连坐 `#g-news2`，"+
				"搬一条进 news 时屏上 news2 的计数也 +1（而盘上没动它）\n%.400s", bc)
		}
		if !strings.Contains(bc, "a[href^=\"#\"]") {
			t.Errorf("bumpCount 不再遍历导航锚点了：组计数会停在一个都不改\n%.300s", bc)
		}
	})

	t.Run("分组集合只从groupsOnPage出去", func(t *testing.T) {
		// 三个功能各要一次：拖网址进来的兜底（页面上第一组）、编辑模态的分组下拉、
		// 多选的「移动到」名单。加添加浮层那一格共四处（前两轮合并过一次）。
		if !strings.Contains(js, "const groupsOnPage = () =>") {
			t.Fatal("app.js 里没有 groupsOnPage")
		}
		if n := strings.Count(js, "groupsOnPage()"); n < 3 {
			t.Errorf("groupsOnPage 只有 %d 处调用，要 ≥3："+
				"「哪些组参与这个功能」在几处各写一遍选择器，改一处不会带着另外几处走", n)
		}
		// 除了它自己那一份，不许再有地方遍历这个集合。
		rest := strings.Replace(body, between(body, "const groupsOnPage = () =>", "\n"), "", 1)
		for _, sel := range []string{"querySelectorAll('section[data-gid]')", "querySelectorAll('[data-gid]')"} {
			if strings.Contains(rest, sel) {
				t.Errorf("还有地方自己遍历分组集合（%s）：认两种容器的判据要只有一处", sel)
			}
		}
	})
}
