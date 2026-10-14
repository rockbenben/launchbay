package render

import (
	"strings"
	"testing"
)

// 密集档按**宽度**排栏，而不是按内容高度（2026-10-14 实拍后改判）。
//
// 实拍症状（用户真实数据：8 组 41 条）：密集档只排 2 栏，右边空着一大片，
// 读作「没占满全宽」。根因是原来那行 `[data-profile-view=dense] main{column-width:224px}`
// ——**multi-column 的语义是"内容顺序灌进若干栏、每栏高度平分"**，于是**栏数由
// 内容高度封顶**：内容灌满两栏就到底了，第三栏压根不存在，哪怕右边还空着 700px。
//
// 这一档的名字承诺的是"尽可能多栏"，而它给的是"每栏 224px、至于几栏由内容够不够
// 决定"。判据换成 grid 的 auto-fill：**按容器宽度**算栏数。
func Test密集档按宽度排栏而不是由内容高度封顶(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := stripCSSComments(string(raw))

	// 一：grid auto-fill，且**每栏有 1fr** ——1fr 兜住余量，所以宽屏上
	// 「最后一行不满」不会留下半栏空白（那与"没占满"是同一种不满）。
	if !strings.Contains(css, "[data-profile-view=dense] main{display:grid;") {
		t.Error("密集档的 main 不是 grid 容器")
	}
	if !strings.Contains(css, "grid-template-columns:repeat(auto-fill,minmax(200px,1fr))") {
		t.Error("密集档没有 `repeat(auto-fill,minmax(…,1fr))`：按宽度排栏的那份判据不在场")
	}
	// 二：反向证人——**不许退回 multi-column**。它就是"没占满全宽"的病根。
	if strings.Contains(css, "[data-profile-view=dense] main{column-width") {
		t.Error("密集档又写回 column-width 了：multi-column 的栏数由内容高度封顶，右边会空着")
	}
	// 三：auto-fill 而不是 auto-fit。后者在有剩余时**把最后几栏拉宽**
	//（一行里两栏各占一半），而密集档要的是栏宽稳定——224 一栏一眼能扫过去。
	if strings.Contains(css, "[data-profile-view=dense] main{display:grid;\n  grid-template-columns:repeat(auto-fit") {
		t.Error("用了 auto-fit：它会拉宽最后几栏，栏宽就不再稳定")
	}

	// 四：间距**已经由 main 的 gap 管**，section 那个 margin-bottom 留着会叠成两倍。
	// 它是分栏时代的写法（分栏时 section 之间没有 gap，全靠它）。
	if !strings.Contains(css, "[data-profile-view=dense] section{margin-bottom:0;") {
		t.Error("section 的 margin-bottom 没收回：grid 的 gap 之外又多出一截，纵向间距翻倍")
	}
	// 五：break-inside 是**分页/分栏**属性，grid 下无效。留着它不致命，
	// 但它会让人以为"一节不许被劈开"这条在密集档还受保护——而 grid 下
	// section 本来就是一个整块，不需要它。
	if strings.Contains(css, "[data-profile-view=dense] section{break-inside") {
		t.Error("密集档还留着 break-inside：那是分栏属性，grid 下不生效，留着会误导下一个人")
	}

	// 六：**wall 档一个字节都不改**。它要的是"一节不许被劈成两栏"
	//（劈开的分组标题读起来像"少了几个"），那是 multi-column 才有的需求——
	// 密集档是"条目尽量多"，两者不是同一件事，判据不同。
	if !strings.Contains(css, "[data-profile-view=wall] main{column-width:300px") {
		t.Error("wall 档的 column-width 不见了：那一档要的是整块不许被劈开，不该跟着改")
	}
}
