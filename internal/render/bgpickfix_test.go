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
//
// 2026-10-16 改判一次：面板打开时**不再是空网格**（实拍"点图标库什么都没出来"），
// 改成先摆这一份已经在用的图标。**原判据没有被推翻，被推翻的是它当时选的那句话**——
// 它守的是"状态行答的是此刻缺的那条指令，不是图库的来源科普"，这一条仍然成立：
// 有起始格时那一行说的是"这些是这一份用过的、其余要搜"，仍然是一条指令。
// 下面几条断言按新形状改写，守的仍是同一件事。
//
// 2026-10-20 再改判一次：起始格那一族又添了**服务端那份常用清单**
// （`GET /api/v1/icons/starter`）——"这一份已经在用的"在实测里几乎永远是空的
// （书签默认 auto，auto 与"没设"都被排除），面板于是永远空着。于是状态行变三态，
// 而"空的那一态仍要说「先搜」"这条原判据一字未动。

func Test图标库空态说的是先搜而不是图库的来源(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	// 清空搜索框那一支要**回到浏览视图**而不是回到一张空网格（2026-10-16 定、
	// 2026-10-20 第三次改判）：那一下是"我不搜了，看刚才那批"，把它变成空白
	// 等于逼人重敲一遍刚看过的词。
	const emptyBranch = "if (!q) {"
	j := strings.Index(js, emptyBranch)
	if j < 0 {
		t.Fatal("app.js 里找不到图标库的空查询那一支")
	}
	// 窗口取 400：那一支前面有几句注释（说清"清空 = 回到浏览视图"），窗口太窄
	// 会把要钉的那一句挤出去——那就不是钉判据，是钉排版了。
	seg := js[j : j+400]
	if !strings.Contains(seg, "paintIconStarter()") {
		t.Errorf("空查询时没有回到浏览视图：\n%.400s", seg)
	}
	if strings.Contains(seg, "app.icon.libHint") {
		t.Errorf("空查询时又拿来源说明顶上了（那句答的不是此刻缺的东西）：\n%.400s", seg)
	}

	// 浏览视图那几段：**每一段都要有一句自己的话**（2026-10-20 第三次改判）。
	//
	// 这一族改过三回：两态（自己用过的 / 空）→ 三态（加上"只有常用那批"）→
	// 现在的**浏览视图**：这一份已经在用的 + 常用的 + **图库里的全部**（可翻页）。
	// 每一段都要看得见自己是谁——三段摆在一起而没有标题，人会以为后面那些也是
	// 搜出来的。
	//
	// 钉的是**各段的话都在**（行为），不是"它们在源码里挨着"（写法）：
	// 2026-10-20 之前那条断言钉的是一句三元表达式的原文，而把它拆成分支之后
	// 行为一模一样、断言却红了——那说明它守的是写法而不是判据。
	ps := strings.Index(js, "function paintIconStarter()")
	if ps < 0 {
		t.Fatal("app.js 里找不到 paintIconStarter")
	}
	rest := js[ps:]
	end := strings.Index(rest, "\n  }\n")
	if end < 0 {
		t.Fatal("paintIconStarter 的函数体找不着结尾")
	}
	body := rest[:end]
	// 浏览视图那几段：**每一段都要有一句自己的话**（2026-10-20 第三次改判）。
	//
	// 这一族改过三回：两态（自己用过的 / 空）→ 三态（加上"只有常用那批"）→
	// 现在的**浏览视图**：这一份已经在用的 + 常用的 + **图库里的全部**（可翻页）。
	// 每一段都要看得见自己是谁——三段摆在一起而没有标题，人会以为后面那些也是
	// 搜出来的。
	//
	// 钉的是**各段的话都在**（行为），不是"它们在源码里挨着"（写法）：
	// 2026-10-20 之前那条断言钉的是一句三元表达式的原文，而把它拆成分支之后
	// 行为一模一样、断言却红了——那说明它守的是写法而不是判据。
	for _, k := range []string{"app.icon.libStarter", "app.icon.libCommon", "app.icon.libStart"} {
		if !strings.Contains(body, "T('"+k+"'") {
			t.Errorf("浏览视图少了 %q 这一句：这一份用过的 / 常用的 / 怎么找到，各要有一句", k)
		}
	}
	// **图库里的全部那一段**：标题（带着总数）与「再加载一批」那枚键。
	// 标题在**翻页那一处**建——第一页还没到时那个"共 N 个"是 0，而"0 个"
	// 是一句假话，所以那一段在第一页到达之前根本不出现。
	if !strings.Contains(js, "T('app.icon.libAll'") {
		t.Error("浏览视图没有「图库里的全部」那一段的标题")
	}
	if !strings.Contains(js, "T('app.icon.libLoad'") {
		t.Error("浏览视图没有「再加载一批」那枚键：图库里的全部要能翻页")
	}
	if !strings.Contains(js, "/api/v1/icons/browse") {
		t.Error("app.js 没有去翻图库那条端点")
	}

	// 起始格只能取这一份**显式写过名字**的图标：`auto` 是"去抓 favicon"，
	// 不是一枚可挑的图标，混进来会让"这一份在用的"这句话变成假话。
	if !strings.Contains(js, "name === 'auto'") {
		t.Error("起始格没有把 auto 排除掉：它是去抓 favicon，不是可挑的图标名")
	}
	// 它必须**克隆卡片上已经画好的那一枚**（`li[data-icon-name]`）而不是自己画——
	// "图库不进 app.js"（AGENTS.md）这条纪律，靠这两行守着。
	if !strings.Contains(js, "li[data-icon-name]") || !strings.Contains(js, "face.cloneNode(true)") {
		t.Error("起始格没有从卡片上克隆现成的那一枚：自己画就等于把图标库塞进脚本")
	}

	// 来源说明降级为网格下面那行常驻小字，且**必须与状态行是两个元素**。
	if !strings.Contains(js, `class: 'hint icpick-hint'`) {
		t.Error("来源说明没有单独一行（它得从状态行的位子上退下来）")
	}

	// 三种语言都要有那几句——只加中文时英文界面上会退回 key 本身。
	for _, lang := range []i18n.Lang{i18n.Zh, i18n.En} {
		cat := i18n.Load(lang)
		for _, k := range []string{"app.icon.libStart", "app.icon.libHint", "app.icon.libStarter", "app.icon.libCommon"} {
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
