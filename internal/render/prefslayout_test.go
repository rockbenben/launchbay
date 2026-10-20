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
	// 改台面偏好的**六行**必须各自独立（改名 / 怎么看 / 第二行 / 分组导航 / 主题 /
	// 界面语言）。
	// 「点开书签」不在这一数里——它改的是书签文档，走 `/prefs/tabs`。
	//
	// 2026-10-19 从 4 变 5：多了「第二行印什么」那一格（`addrLevel`）。它与
	// view 一样是 `POST /prefs` 的一个字段、同样按下即改，所以**是这一族的一员**。
	//
	// 2026-10-20 从 5 变 6：多了「界面语言」那一格（`language`）——每一份档案
	// 自己记一份语言，管理员在管理页设的那一个只是**默认**。它同样是
	// `POST /prefs` 的一个字段、同样按下即改。
	if n := strings.Count(tmpl, `action="/prefs" class="prefs-row"`); n != 6 {
		t.Errorf("改台面偏好的独立表单是 %d 个，要恰好 6（改名/怎么看/第二行/导航/主题/语言）", n)
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

// 设置面板每一行都要有说明，**但说明说的是"后果"，不是"怎么用"**（2026-10-17 改判）。
//
// 从前的判据是「六行一个都不能少」，理由是"少一行的症状是这一格在屏上不解释自己，
// 只能靠猜"。那句话在两种情况下不成立，而那两种当时就有：
//
//	· 两颗键的字**就是**那句话的答案：「侧栏 / 顶栏」不需要再解释成「放在左边一列，
//	  还是顶上一行」；「这一页 / 新一页」同理。屏上多一行是在把按钮上的字重打一遍。
//	· 那句话与**另一处一字不差**（`prefs.explain` 的第三句 vs `prefs.groups.hint`）——
//	  同一句话印两遍，删掉的那一遍正是短的。
//
// 用户 2026-10-17 定的口径：**用法显而易见就不要说明**。所以这里钉的不再是"六行都有
// hint"，而是"**有后果要说的行，一行都不许少**"——而后果那几句正是最容易在改版时被
// 顺手删掉的那几句（它们不在按钮上，删掉之后没人会补）。
func Test设置面板的说明只说后果(t *testing.T) {
	tmpl := readWeb(t, "web/profile.tmpl")
	// 有后果的几行：改名（显示在哪、留空用默认）、视图（换一换整页怎么摆）、主题、
	// 分组这一节（改的是书签本身、能退回去）、回退与历史这一节（查看审计日志与恢复快照）。
	for _, k := range []string{
		"prefs.rename.hint", "prefs.view.hint", "prefs.theme.hint", "prefs.groups.hint", "prefs.sec.out.hint",
	} {
		if !strings.Contains(tmpl, `{{.T "`+k+`"}}`) {
			t.Errorf("模板里没有引用 %s：那一行有后果要说，屏上却不说", k)
		}
		if i18n.Load(i18n.Zh).T(k) == "" || i18n.Load(i18n.En).T(k) == "" {
			t.Errorf("语言包里少了 %s", k)
		}
	}
	// 「改完按回车」那一格也留：它**没有保存键**，回车是唯一的出手势——那不是显而易见的
	// 用法，而是这一格唯一的出路。（它在 input 的 title 上，所以是 `$.T` 不是 `.T`。）
	if !strings.Contains(tmpl, `{{$.T "prefs.groups.rename.hint"}}`) {
		t.Error("分组改名那一格没有说明：它没有保存键，「按回车」是唯一的出手势")
	}
	// **钉住那两处是有意删的**：将来谁看着"六行变四行"想补回去，红的是这一条。
	//
	// 判语言包要查 **key 集合**而不是 `T()`：缺 key 时 `T` 返回的是包着尖括号的 key
	// 自己（"⟨key⟩" 那条纪律，屏上看得见），非空——拿它当"文案还在"的证据，
	// 删干净了反而会报这一条红。
	for _, k := range []string{"prefs.layout.hint", "prefs.tabs.hint"} {
		if strings.Contains(tmpl, `{{.T "`+k+`"}}`) {
			t.Errorf("%s 又被引用了：两颗键的字已经是那句话的答案", k)
		}
		if containsString(i18n.Load(i18n.Zh).Keys(), k) {
			t.Errorf("中文语言包里 %s 还在：删了引用却留着文案，下一次有人抄回来就用上了", k)
		}
		if containsString(i18n.Load(i18n.En).Keys(), k) {
			t.Errorf("英文语言包里 %s 还在：同上", k)
		}
	}
	// 而「紧凑档下这一列不画」**必须留**：它说的是按下去之后屏上会发生什么，
	// 不是那两个键怎么用。
	if strings.Contains(tmpl, `{{if and (eq .Profile.Layout "sidebar") .Compact}}<p class="hint">`) {
		t.Error("sidebarNote 又退回另起一行的 hint 了：它与那两颗键视觉脱节")
	}
	if !strings.Contains(tmpl, `{{.T "prefs.layout.sidebarNote"}}`) {
		t.Error("sidebarNote 没有了：紧凑档下这一列不画这件事必须在按之前说清")
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
