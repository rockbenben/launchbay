package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 一份真实的浏览器导出把嵌套文件夹拍平成"工具 / 同步 / 脚本"这样的标题，
// 于是导入完的档案页是**三十个分组、三十行长标题**，看不出哪几个是一家的。
// 这一族用例钉的是"层级被读回来"这件事：
//
//   - **侧栏**按深度缩进，路径的中间段是**合成父级**（不可点、没有计数）；
//   - **节标题**拆成叶名 + 弱化的父路径，眼睛落在叶名上；
//   - 磁盘上的标题**一个字都没改**（id、寻址、已有档案全不受影响）——
//     层级只存在于标题里，渲染层把它读回来给人看，不去改数据。
//
// 为什么值得单独立一族用例：这份改动是纯呈现的，没有判据搬走，所以将来有人
// "顺手"把标题改成叶名、或者把分隔符换个字，这几条都会红——那正是它们要防的。

// navFixture 造出一份"导完之后"的分组列表：三个顶层、若干深路径，外加两个
// 路径段同时也是真实分组的情形（"工具" 与 "工具 / 同步" 都在）。
func navFixture() []groupView {
	mk := func(id, title string, n int, accent string) groupView {
		g := groupView{ID: id, Title: title, Layout: model.LayoutList, Accent: accent}
		g.Parents, g.Leaf = pathSplit(title)
		for i := 0; i < n; i++ {
			g.Items = append(g.Items, itemView{ID: id + "-" + string(rune('a'+i)), Name: "x"})
		}
		return g
	}
	return []groupView{
		mk("g1", "书签栏", 2, ""),
		mk("g2", "工具", 2, "#ff0000"),
		mk("g3", "工具 / 同步", 0, ""), // 自己一条书签都没有，但仍是真实分组
		mk("g4", "工具 / 同步 / 脚本", 2, ""),
		mk("g5", "阅读", 1, ""),
	}
}

func TestGroupNav_路径被读回层级(t *testing.T) {
	nav := groupNav(navFixture())
	var labels []string
	for _, n := range nav {
		labels = append(labels, n.Label)
	}
	want := []string{"书签栏", "工具", "工具 / 同步", "工具 / 同步 / 脚本", "阅读"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("树的形状不对：\n得到 %v\n要   %v", labels, want)
	}
	// 深度是那条缩进的唯一判据：0/0/1/2/0。
	for i, want := range []int{0, 0, 1, 2, 0} {
		if nav[i].Depth != want {
			t.Errorf("%q 的 Depth=%d，要 %d", nav[i].Label, nav[i].Depth, want)
		}
	}
}

// 合成父级与真实分组**必须分得清**：只有真分组能链到锚点、才有计数与色点。
// 弄混的后果是双向的——把父级画成可点，点过去是个不存在的锚点；把真分组画成
// 父级，那个分组在侧栏里就没有入口了，而它明明是可进去的。
//
// "工具 / 同步" 这一格就是后一种情形：它自己一条书签都没有，可它是一个真实的
// 分组（导入器为空文件夹不建组，但这一份是有内容的）。
func TestGroupNav_只有真分组可点(t *testing.T) {
	nav := groupNav(navFixture())
	type row struct {
		Label   string
		Href    string
		Count   int
		IsGroup bool
	}
	var got []row
	for _, n := range nav {
		got = append(got, row{n.Label, n.Href, n.Count, n.IsGroup})
	}
	want := []row{
		{"书签栏", "#g-g1", 2, true},
		{"工具", "#g-g2", 2, true},
		{"工具 / 同步", "#g-g3", 0, true}, // 真实分组：空也要能进去（它有条目数为 0 的那一份）
		{"工具 / 同步 / 脚本", "#g-g4", 2, true},
		{"阅读", "#g-g5", 1, true},
	}
	if len(got) != len(want) {
		t.Fatalf("行数不对：得到 %d，要 %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 行 = %+v，要 %+v", i, got[i], want[i])
		}
	}
}

// 一个**中间段**被折出来的格子（既不是完整路径、也不是任何分组的标题）才是不
// 可点的：它只是路标。这一格用的是"工具 / 同步 / 脚本"存在而"工具 / 同步"
// **不存在**的那种形状——真实导出里极常见（父文件夹自己没有书签）。
func TestGroupNav_折出来的中间段不可点(t *testing.T) {
	g := groupView{ID: "g", Title: "工具 / 同步 / 脚本", Layout: model.LayoutList}
	g.Parents, g.Leaf = pathSplit(g.Title)
	g.Items = []itemView{{ID: "a", Name: "x"}}
	nav := groupNav([]groupView{g})
	if len(nav) != 3 {
		t.Fatalf("该折出三行（工具 / 同步 / 脚本），实际 %d 行", len(nav))
	}
	// 前两格是折出来的路标：不可点、没有锚点与计数。
	for i := 0; i < 2; i++ {
		if nav[i].IsGroup {
			t.Errorf("第 %d 行 %q 不该是可点的真分组", i, nav[i].Label)
		}
		if nav[i].Href != "" || nav[i].Count != 0 {
			t.Errorf("不可点的那格不该带锚点或计数：%+v", nav[i])
		}
	}
	if nav[2].Href != "#g-g" || nav[2].Count != 1 || !nav[2].IsGroup {
		t.Errorf("最后一行才是真分组，该带锚点与计数：%+v", nav[2])
	}
	if nav[1].Label != "同步" || nav[0].Label != "工具" {
		t.Errorf("父级格该只显示自己那一段：%q / %q", nav[0].Label, nav[1].Label)
	}
}

// **重名的分组每一组都要有一行**（2026-10-22 修）：台面面板「＋ 新分组」不查
// 重名（id 由服务端派生，名字原样留下），「改名」那一格也不查——于是磁盘上躺着
// 两个叫「常用」的组，是**人正常用这台机器用得出来的形状**。
//
// 从前 groupNav 按**标题**去重，第二个**整行不见了**：它从侧栏的导航里消失，
// 而页面上明明还画着它（同一个屏上，「＋ 添加」那格的下拉里两份都在）。实测：
// 一份里有三个叫「常用」的组，侧栏只剩一行「常用 1」，另外两组（4 条与 0 条）
// 连入口都没有——**一组从导航里消失了**，而它明明是可进去的。
//
// 这一族钉的是"**没有一组被藏起来**"：行数 = 组数，每一行的锚点指向**自己**
// 那一组（不是同名的另一组），计数也各是各的。
func TestGroupNav_重名的组每一组都有一行(t *testing.T) {
	mk := func(id, title string, n int) groupView {
		g := groupView{ID: id, Title: title, Layout: model.LayoutList}
		g.Parents, g.Leaf = pathSplit(title)
		for i := 0; i < n; i++ {
			g.Items = append(g.Items, itemView{ID: id + "-" + string(rune('a'+i)), Name: "x"})
		}
		return g
	}
	groups := []groupView{
		mk("g1", "常用", 1),
		mk("g2", "常用", 4),
		mk("g3", "常用", 0),
		mk("g4", "工作", 3),
	}
	nav := groupNav(groups)
	if len(nav) != len(groups) {
		t.Fatalf("每一组都要有一行：得到 %d 行，要 %d 行（%+v）", len(nav), len(groups), nav)
	}
	want := []struct {
		href  string
		count int
	}{
		{"#g-g1", 1},
		{"#g-g2", 4},
		{"#g-g3", 0},
		{"#g-g4", 3},
	}
	for i, w := range want {
		if nav[i].Href != w.href || nav[i].Count != w.count {
			t.Errorf("第 %d 行 = %+v，要 href=%s count=%d", i, nav[i], w.href, w.count)
		}
	}
}

// 中途段那一行要归**真正的那个组**："工具 / 同步" 存在时，"工具" 那一段是
// "工具" 那一组的入口，锚点与计数都得取自**它**。
//
// 从前取的是当前这一组（正在拆 "工具 / 同步 / 脚本" 的那一组），于是顺序一换
// （深的那组排在前面），"工具" 那一行会链到**"工具 / 同步"** 那一组去——点过去
// 落点不对，而那一组自己的入口反而不存在了。
func TestGroupNav_中途段归真正的那个组(t *testing.T) {
	mk := func(id, title string, n int) groupView {
		g := groupView{ID: id, Title: title, Layout: model.LayoutList}
		g.Parents, g.Leaf = pathSplit(title)
		for i := 0; i < n; i++ {
			g.Items = append(g.Items, itemView{ID: id + "-" + string(rune('a'+i)), Name: "x"})
		}
		return g
	}
	// 深的那组排在前面——这正是"导入完的档案"里常见的顺序。
	nav := groupNav([]groupView{
		mk("g3", "工具 / 同步 / 脚本", 2),
		mk("g2", "工具", 2),
		mk("g1", "工具 / 同步", 0),
	})
	if len(nav) != 3 {
		t.Fatalf("该有三行，实际 %d 行（%+v）", len(nav), nav)
	}
	// "工具" 那一行链到 g2（真正叫「工具」的那一组），不是 g3。
	if nav[0].Label != "工具" || nav[0].Href != "#g-g2" || nav[0].Count != 2 {
		t.Errorf("「工具」那一行该归 g2：%+v", nav[0])
	}
	// "工具 / 同步" 那一行链到 g1。
	if nav[1].Label != "工具 / 同步" || nav[1].Href != "#g-g1" {
		t.Errorf("「工具 / 同步」那一行该归 g1：%+v", nav[1])
	}
	if nav[2].Label != "工具 / 同步 / 脚本" || nav[2].Href != "#g-g3" {
		t.Errorf("最后一行该是 g3：%+v", nav[2])
	}
}

// 没有分隔符的标题原样进树（深度 0），而**数据一个字都不许被改**：groupNav
// 之后重读磁盘，标题必须与标题一致。
func TestGroupNav_不动数据(t *testing.T) {
	groups := navFixture()
	titles := make([]string, len(groups))
	for i, g := range groups {
		titles[i] = g.Title
	}
	_ = groupNav(groups)
	for i, g := range groups {
		if g.Title != titles[i] {
			t.Errorf("第 %d 个标题被改了：%q → %q", i, titles[i], g.Title)
		}
	}
	// 同一份数据渲两次，出来要一模一样（树上没有 map 迭代顺序这类不确定性）。
	first, second := groupNav(groups), groupNav(groups)
	if len(first) != len(second) {
		t.Fatalf("两次调用行数不同：%d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("第 %d 格两次不一样：%+v vs %+v", i, first[i], second[i])
		}
	}
}
