package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 两个 2026-10-14 实拍出来的病，各带一条钉。
//
// 它们都属于"**功能测试全绿、屏上却不对**"那一类，所以钉的是**形状本身**：
// 少任何一样，症状都不会在别处响起来。

func Test图标库空态说的是先搜而不是图库的来源(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	// 空查询那一支必须说"先搜"，不能退回来源说明。
	const emptyBranch = "if (!q) {"
	j := strings.Index(js, emptyBranch)
	if j < 0 {
		t.Fatal("app.js 里找不到图标库的空查询那一支")
	}
	seg := js[j : j+220]
	if !strings.Contains(seg, "app.icon.libStart") {
		t.Errorf("空查询时说的话不是「先搜一个名字」：\n%.220s", seg)
	}
	if strings.Contains(seg, "app.icon.libHint") {
		t.Errorf("空查询时又拿来源说明顶上了（那句答的不是此刻缺的东西）：\n%.220s", seg)
	}

	// 面板打开的那一初始始态走同一句话——否则人看到的第一眼仍是那一句科普。
	if !strings.Contains(js, "text: T('app.icon.libStart') })") {
		t.Error("面板打开时的初始状态行没有说「先搜一个名字」")
	}

	// 来源说明降级为网格下面那行常驻小字，且**必须与状态行是两个元素**。
	if !strings.Contains(js, `class: 'hint icpick-hint'`) {
		t.Error("来源说明没有单独一行（它得从状态行的位子上退下来）")
	}

	// 两种语言都要有那两句——只加中文时英文界面上会退回 key 本身。
	for _, lang := range []i18n.Lang{i18n.Zh, i18n.En} {
		cat := i18n.Load(lang)
		for _, k := range []string{"app.icon.libStart", "app.icon.libHint"} {
			if got := cat.T(k); got == "" || strings.HasPrefix(got, "⟨") {
				t.Errorf("%s 的语言包里 %q 没落上（T 给的是 %q）", lang, k, got)
			}
		}
	}
}

// 背景那一格的「选择文件」**不能是原生控件**（实拍：暗色页上是一块浅灰底 +
// 系统字体的东西，压在暗色面板里像另一个产品贴上来的零件）。
//
// **为什么不是"改一改原生控件的样式"**：那颗按钮由浏览器自己画，
// `color-scheme` 管不到它，能改外框的 `::file-selector-button` 一族要按引擎各写一份，
// 而第三方浏览器会落空。所以判据是**反过来的那一半**：真 input 收起来，
// 露出来的是一颗 `.fld`（全站按钮同一张脸），input 透明铺在上面仍然可点。
func Test背景选文件按钮是全站那张脸而不是原生控件(t *testing.T) {
	cssRaw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := stripCSSComments(string(cssRaw))
	tmplRaw, err := webFS.ReadFile("web/profile.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	tmpl := string(tmplRaw)

	// 一：原生 input 必须透明铺在按钮上（这是"收起来"的机制本身）。
	if !strings.Contains(css, ".prefs-bg-file input[type=file]{position:absolute;inset:0") {
		t.Error("真 input 没有透明铺在那颗按钮上：不铺它就还是原生控件，铺了它点击热区才是整颗键")
	}
	if !strings.Contains(css, "opacity:0") {
		t.Error("真 input 没有 opacity:0：它会盖在按钮上把按钮画没")
	}
	// 二：可见的那颗必须是 .fld，而不是别的形状。
	if !strings.Contains(tmpl, `<span class="fld" aria-hidden="true">`) {
		t.Error("屏上那一颗不是 .fld：全站按钮是同一张脸，这一格自己画一个就穿了")
	}
	// 三：那颗可见按钮**不许自己吃掉点击**（pointer-events:none）——点击要落到
	// 下面那个 input 上，而 input 铺满整颗键，任何一点都该打开文件对话框。
	if !strings.Contains(css, ".prefs-bg-file .fld{pointer-events:none}") {
		t.Error("可见按钮没有 pointer-events:none：点击会落在 span 上而不是下面的 input 上")
	}
	// 四：反向证人——**不许退回"给原生控件写样式"那条路**。
	for _, dead := range []string{"::-webkit-file-upload-button", "::file-selector-button"} {
		if strings.Contains(css, dead) {
			t.Errorf("css 里出现了 %s：那一族要按引擎各写一份外观，而第三方浏览器会落空", dead)
		}
	}
	// 五：选了文件之后要有地方显示文件名（原生控件收起之后那一段就没了）。
	if !strings.Contains(tmpl, `class="prefs-bg-picked"`) {
		t.Error("没有那一行文件名回显：控件收起之后「选了什么」就无处可说")
	}
	if !strings.Contains(css, ".prefs-bg-picked:empty{display:none}") {
		t.Error("文件名那一行在没选文件时也占位，面板会多出一段空白")
	}
}
