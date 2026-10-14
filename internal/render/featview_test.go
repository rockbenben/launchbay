package render

import (
	"strings"
	"testing"
)

// 大卡那一块在**三档 view** 下的形状（2026-10-15 实拍后补）。
//
// 它此前只在「分组」那一档是对的，因为那一条 `main` 本来就是普通块级——带子
// 只是 `main` 的一个普通 `<section>`。而另外两档 `main` 各自是别的机制，带子
// 于是成了**并列的一栏**：
//
//	全局（wall）：`main` 是 CSS 多列（column-width:300px）。带子 1440 实测
//	  **317px 宽、七张 112px 大卡竖着叠了 911px**，而隔壁两列是 44px 的紧凑行——
//	  它换来的"一眼扫得到"正好丢光，成了墙里最窄的一列。
//	密集（dense）：`main` 是 grid（`repeat(auto-fill,minmax(200px,1fr))`，1440 下
//	  每列 208px），而带子内部 `.feat-row` 是 `minmax(300px,1fr)`——**轨道比列宽
//	  还宽，那几张卡直接压在隔壁「影音」那一栏上**。这一条比"不好看"严重：它是
//	  两处内容重叠（实拍：NAS/Portainer 盖住了 Emby/小雅影视…）。
//
// 修法是**按那一档的机制各写一句**：多列用 column-span、grid 用 grid-column，
// 而「分组」那一档什么都不用写。
func Test大卡带在两档多列视图里横跨全部列(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/app.css"))
	for _, want := range []struct{ what, needle string }{
		{"全局（多列）", "[data-profile-view=wall] .feat{column-span:all}"},
		{"密集（grid）", "[data-profile-view=dense] .feat{grid-column:1/-1}"},
	} {
		if !strings.Contains(css, want.needle) {
			t.Errorf("app.css 里没有%s那一档「横跨全部列」的那一句（%q）：带子会退化成并列的一栏", want.what, want.needle)
		}
	}
}

// 密集档那几条收窄规则**不适用于大卡**（2026-10-15 实拍后补）。
//
// 症状：密集档下带子里那两张点不动的卡（`span class="feat-card plain"`）**没有
// 卡片底**——名字与图标浮在背景上，而同一排里可点的那几张有底有边。同一个带子
// 里两种卡长得不一样，而这正是 2026-10-14 那一轮刚消掉的形状。
//
// 根因是**特异性**不是顺序：`[data-profile-view=dense] .plain` 是 (0,2,0)，
// 而带子自己的 `.feat-card` 是 (0,1,0)，于是密集档那句赢——而它要的
// `background:none;border:1px solid transparent` 正是把卡片那张脸收掉的那一档。
// 于是"密集"顺带把常用入口也缩了，而 app.css 那条注释写的恰是相反的判断：
// 「那一档的『紧凑』说的是分组之间不留大块空白，而不是把常用入口也缩小」。
//
// 判据是**选择器被收窄了**，不是"带子那边写得更重"——后者只是把特异性差
// 换成顺序差，谁在前面就又变回谁赢。
func Test密集档的收窄规则不适用于大卡(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	for _, want := range []string{
		"[data-profile-view=dense] .card:not(.feat-card)",
		"[data-profile-view=dense] .plain:not(.feat-card)",
		"[data-profile-view=dense] .card:not(.feat-card) b{padding-right:0}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("base.css 里少了 %q：密集档会把带子里那张卡的脸也收掉", want)
		}
	}
	// 反向证人：不收窄的那三句留着的话，带子里两种卡在密集档下长得不一样。
	for _, bad := range []string{
		"[data-profile-view=dense] .card,[data-profile-view=dense] .plain{",
		"[data-profile-view=dense] .card b{padding-right:0}",
	} {
		if strings.Contains(css, bad) {
			t.Errorf("base.css 里还留着没收窄的那一句：%s", bad)
		}
	}
}
