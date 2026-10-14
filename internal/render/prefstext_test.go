package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 设置页「分组导航」那一格：它下面那句话**必须与它解释的那段 CSS 同源**。
//
// 从前那句话的判据是 `.Compact`（只看 view 是 wall/dense），而**真正隐藏侧栏的
// 条件是 `layout=sidebar` ∧ `view∈{wall,dense}`**（base.css 那两条
// `[data-profile-layout=sidebar][data-profile-view=…]`）。于是 `topbar + dense`
// 那一档——顶栏已经在导航、侧栏压根没画——面板还在说「选『侧栏』时不画那一列」。
// **一句为另一档写的解释落在这一档，读起来像"我按了侧栏它就不见了"。**
//
// 解释一个条件却在另一个条件成立时出现，比不解释更坏：不解释时人自己会去发现，
// 而这句是权威地告诉他一件此刻不成立的事。
func Test分组导航那句解释与隐藏侧栏的CSS同源(t *testing.T) {
	raw, err := webFS.ReadFile("web/profile.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	tmpl := string(raw)

	// 判据必须**同时**带 layout=sidebar 与 .Compact——少一个就是这一档说错话。
	if !strings.Contains(tmpl, "{{if and (eq .Profile.Layout \"sidebar\") .Compact}}") {
		t.Error("那句解释的判据不是「layout=sidebar 且紧凑档」：" +
			"它现在解释的是一个它并不描述的条件（topbar+dense 时侧栏压根没画）")
	}
	// 反向证人：光有 .Compact 不够——那正是原 bug。
	if strings.Contains(tmpl, "{{if .Compact}}<p class=\"hint\">{{.T \"prefs.layout.sidebarNote\"}}") {
		t.Error("判据退回只看 .Compact 了：topbar+dense 那一档会印出一句不成立的话")
	}

	// 隐藏侧栏的那两条 CSS 还在场吗——**这段话解释的就是它们**。
	cssRaw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := stripCSSComments(string(cssRaw))
	for _, sel := range []string{
		"[data-profile-layout=sidebar][data-profile-view=wall] .side",
		"[data-profile-layout=sidebar][data-profile-view=dense] .side",
	} {
		if !strings.Contains(css, sel+",") && !strings.Contains(css, sel+"{") {
			t.Errorf("base.css 里少了 %s：面板那句话解释的那段样式没了", sel)
		}
	}
}

// 「横向抽屉」这四个字是不成立的：顶栏那一行是 `<nav>` + flex，**没有 details、
// 没有把手、没有展开收起**（实测 base.css 只有三条：一条 margin、两条 flex）。
// 说了"抽屉"，人会去找一个不存在的把手——这是 §8.1「点了没反应的东西不进界面」
// 在文案上的同一个形状。
func Test顶栏那句不叫抽屉(t *testing.T) {
	for _, lang := range i18n.All {
		// 走 i18n 包而不是去读 ../i18n/messages/*.json：语言包是 embed 进那个包的，
		// 从 render 这里用相对路径读会穿透包边界（go:embed 不允许）。
		got := i18n.Load(lang).T("profile.layout.toTopbar")
		for _, wrong := range []string{"抽屉", "drawer"} {
			if strings.Contains(got, wrong) {
				t.Errorf("%s 的 toTopbar 仍写着 %q：那一行是 flex 横排，没有折叠也没有把手（实际是 %q）",
					lang, wrong, got)
			}
		}
	}
	// 正向证人：它得说清**排不下时会发生什么**——那是这一行唯一该对人说的话。
	// 2026-10-14 从「能左右滑」改成「折成两行」：顶栏改 `flex-wrap`（base.css
	// `.gnav.top ul`），`.gnav:not(.top)` 那一档侧栏抽屉仍是 `overflow-x:auto`。
	// 所以这里比的是**换行**那条，不是整个 `overflow-x` 是否还存在于 CSS 里——
	// 侧栏那一档没改，它留着是应该的。
	cssRaw, _ := webFS.ReadFile("web/base.css")
	if !strings.Contains(stripCSSComments(string(cssRaw)), ".gnav.top ul{display:flex;flex-wrap:wrap;gap:6px 8px}") {
		t.Error("base.css 里那一行改了：文案要跟着改，别留着一句描述旧行为的话")
	}
}
