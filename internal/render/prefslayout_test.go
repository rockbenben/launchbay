package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 台面面板的排版（2026-10-14 参考 Flare 那一屏重排）：从「标签在左、控件在右」
// 改成「**说明在上、控件在下**」。
//
// 改的理由是实测出来的那一处，不是"Flare 那样好看"：从前标签列固定 92px，
// **92px 放不下任何解释**，于是每一行的解释只能在悬停 title 里，而屏上真放不下
// 的那句（分组导航"紧凑档不画这一列"）只能另起一行——它与那两颗键**视觉脱节**，
// 读起来像面板底部多了一段不相干的话。
func Test面板每行是说明在上控件在下(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	tmpl := readWeb(t, "web/profile.tmpl")

	// 一：`.prefs-field` 是**列**（flex-direction:column）且 `min-width:0`。
	// 后者是承重的：没有它，这一格会被自己那行说明的 min-content 撑破，
	// 而窄屏上的 flex-wrap 就再也换不了行。
	f := extractRule(css, ".prefs .prefs-field{")
	if f == "" {
		t.Fatal("base.css 里没有 .prefs-field 规则")
	}
	if !strings.Contains(f, "flex-direction:column") {
		t.Errorf(".prefs-field 不是列排布：说明与控件会并排，那正是这次要改掉的\n%s", f)
	}
	if !strings.Contains(f, "min-width:0") {
		t.Errorf(".prefs-field 少了 min-width:0：窄屏上这一格会被说明的 min-content 撑破")
	}

	// 二：说明位 `.prefs-hint` 在场，且它与键自己的 title **分工不同**——
	// title 答"这一颗"，说明答"这一格"。两处都要在。
	if !strings.Contains(css, ".prefs .prefs-hint{") {
		t.Error("没有 .prefs-hint：那一句常驻说明无处安放")
	}
	if !strings.Contains(tmpl, `<p class="prefs-hint">`) {
		t.Error("模板里没有 .prefs-hint 元素")
	}

	// 三：标签**不再固定 92px 那一列**。说明在上时标签不必占一条竖线，
	// 而留着 `flex:0 0 92px` 会让它在列排布里仍占 92px 高的一条空行。
	l := extractRule(css, ".prefs .prefs-label{")
	if strings.Contains(l, "flex:0 0 92px") {
		t.Errorf("标签还占着 92px 的固定列：列排布下那是一条空行\n%s", l)
	}

	// 四：**每行仍是一张独立表单、按下即改**。Flare 那个整页一个「保存修改」
	// 在这里赢不了：改错了要撤销就得先存一版错的，而"按一下只改一件事"是
	// 本面板从一开始就有的那条。所以别把它改成一个总表单。
	if !strings.Contains(tmpl, `action="/prefs" class="prefs-row"`) {
		t.Error("改设置的那几行不再是独立的表单")
	}
	// 只查**标记**：那两个字可能出现在注释里（"Flare 那个整页一个「保存修改」…"），
	// 而注释里提到它是对的——那正是在解释为什么我们**不**这么做。
	if strings.Contains(tmpl, `class="prefs-saveall"`) ||
		strings.Contains(tmpl, `name="prefs-all"`) {
		t.Error("面板加了一个整页保存：那会让改错了一项也存进去")
	}
	// 改台面偏好的**四行**必须各自独立（改名 / 怎么看 / 分组导航 / 主题）。
	// 「点开书签」不在这一数里——它改的是书签文档，走 `/prefs/tabs`。
	if n := strings.Count(tmpl, `action="/prefs" class="prefs-row"`); n != 4 {
		t.Errorf("改台面偏好的独立表单是 %d 个，要恰好 4（改名/怎么看/导航/主题）", n)
	}
	if n := strings.Count(tmpl, `action="/prefs/tabs" class="prefs-row"`); n != 1 {
		t.Errorf("「点开书签」那一行是 %d 个，要恰好 1", n)
	}
}

// 分节小标题：八行平铺时，读者要自己判断哪几行是一类。
// 一个 13px 小标题 + 一条线就把"这一页长什么样 / 出口 / 分组"说清了。
func Test面板有分节且出口不在设置节里(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	tmpl := readWeb(t, "web/profile.tmpl")

	if !strings.Contains(css, ".prefs h3.prefs-sec{") {
		t.Error("没有分节小标题的样式")
	}
	// 第一个分节紧跟 h2，**不留顶线**（否则标题下面立刻多一条线）。
	if !strings.Contains(css, ".prefs h3.prefs-sec:first-of-type{border-top:0;padding-top:0}") {
		t.Error("第一个分节没有收回顶线：h2 底下会立刻多一条线")
	}
	for _, k := range []string{"prefs.sec.look", "prefs.sec.out"} {
		if i18n.Load(i18n.Zh).T(k) == "" || i18n.Load(i18n.En).T(k) == "" {
			t.Errorf("语言包里少了 %q（两种语言都要有）", k)
		}
	}

	// 「出口」那一节必须**排在「点开书签」之后**：点开书签改的是书签文档，
	// 它是设置；回退与历史不是设置。出口在前会把设置挤到最后一行设置之下。
	//
	// 比的是 **h3 那一格**而不是 `prefs.sec.out` 这个 key（2026-10-15 加左侧
	// 分类 tab 之后）：同一句 key 现在在左侧那列的 tab 标签上也出现一次，
	// 按 key 找位置量到的是"标签在导航里"，不是"这一节在第几屏"。
	outAt := strings.Index(tmpl, `<h3 class="prefs-sec">{{.T "prefs.sec.out"}}`)
	tabAt := strings.Index(tmpl, `action="/prefs/tabs"`)
	if outAt < 0 || tabAt < 0 {
		t.Fatal("出口节或点开书签那一行不见了")
	}
	if outAt < tabAt {
		t.Error("「出口」节排在「点开书签」之前：它是设置，而回退/历史不是")
	}
}

// 每一行都要有那句常驻说明——**六行一个都不能少**。
// 少一行的症状是"这一格在屏上不解释自己，只能靠猜"，而它不会让任何测试变红。
func Test六行都有常驻说明(t *testing.T) {
	tmpl := readWeb(t, "web/profile.tmpl")
	for _, k := range []string{
		"prefs.rename.hint", "prefs.view.hint", "prefs.layout.hint",
		"prefs.theme.hint", "prefs.tabs.hint",
	} {
		if !strings.Contains(tmpl, `{{.T "`+k+`"}}`) {
			t.Errorf("模板里没有引用 %s：那一行在屏上没有解释", k)
		}
		if i18n.Load(i18n.Zh).T(k) == "" || i18n.Load(i18n.En).T(k) == "" {
			t.Errorf("语言包里少了 %s", k)
		}
	}
	// 「分组导航」那一格的那句解释**接在它的说明后面**，而不是从前那样
	// 另起一行（另起一行时它与那两颗键视觉脱节）。
	if strings.Contains(tmpl, `{{if and (eq .Profile.Layout "sidebar") .Compact}}<p class="hint">`) {
		t.Error("sidebarNote 又退回另起一行的 hint 了：它与那两颗键视觉脱节")
	}
	// 两句之间**不留半角空格**（batch11 T6）：中文句号后面直接接下一句。
	// 这条证人从前钉的是带空格的那一份——那正是它该挡而没挡住的东西：
	// 拼串时模板里的空格会原样上屏，成了「…放在左边一列。 并排与紧凑…」这种接缝。
	if !strings.Contains(tmpl, `{{if and (eq .Profile.Layout "sidebar") .Compact}}{{.T "prefs.layout.sidebarNote"}}{{end}}`) {
		t.Error("sidebarNote 没有接在那一格的说明后面")
	}
}

// extractRule 取出 `sel{…}` 那一条规则的完整文本（含换行）。
func extractRule(css, sel string) string {
	i := strings.Index(css, sel)
	if i < 0 {
		return ""
	}
	rest := css[i:]
	if j := strings.Index(rest, "}"); j >= 0 {
		return rest[:j+1]
	}
	return rest
}
