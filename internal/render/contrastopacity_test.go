package render

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 元素**透明度**那一层的对比度（2026-10-21 加）。
//
// 为什么单开一条：上面那条闸（TestCSS_对比度算得过）量的是**色变量**——
// 「--ink2 压在 --card 上」——而 `.pencil` / `.copybtn` 静息时还有一层
// **opacity**。两层叠起来的**有效**颜色才是眼睛真看到的那个，
// 而色变量那条闸**看不见 opacity**（它是元素样式，不是变量）。
//
// 那颗键在**触摸/粗指针**那一档是**常驻**的（手指没有 hover），而它是手机上
// **唯一**能改一条书签的入口——它够不够得着，全看它静息时有多清楚。
// 2026-10-21 实测（字色 --ink2，底是卡片合成后的颜色）：
//
//	.34 → 暗档 2.08:1 / 亮档 1.70:1 —— 两套主题都不到 WCAG 1.4.11 的 3:1
//	.70 → 暗档 4.64:1 / 亮档 3.43:1 —— 两套主题都过
//
// 这条闸钉的就是那个下限。**桌面那一档一个字都不许动**：精确指针仍然是
// 静息 0、指到或键盘走进这一行才 1（"降噪"由不画框不画底承担，不由透明度承担）。
func TestCSS_元素透明度的对比度算得过(t *testing.T) {
	appRaw, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	baseRaw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	app := stripCSSComments(string(appRaw))
	base := stripCSSComments(string(baseRaw))

	// ① 静息透明度：从 `.pencil,.copybtn` 那条基础规则里读。
	rule := extractRule(app, ".pencil,.copybtn{")
	if rule == "" {
		t.Fatal("app.css 里找不到 .pencil,.copybtn 那条规则")
	}
	opacity := opacityOf(t, rule)
	const minOpacity = 0.70
	if opacity < minOpacity {
		t.Errorf("把手静息透明度 %.2f 低于 %.2f：触摸档它是常驻的（手指没有 hover），"+
			"而它是手机上唯一能改一条书签的入口（实测 .34 → 暗档 2.08:1、亮档 1.70:1，两套主题都不到 3:1）",
			opacity, minOpacity)
	}

	// ② 桌面那一档仍然是"静息不画"：精确指针下它 0，指到或键盘走进这一行才 1。
	if !strings.Contains(app, "opacity:0}") && !strings.Contains(app, "opacity:0 ") {
		t.Error("app.css 里找不到精确指针那一档的 opacity:0——桌面静息不画的判据不见了")
	}

	// ③ 算两套主题：字色 --ink2 乘上那层透明度，压在卡片合成后的底上。
	dark := cssVars(t, base, ":root{")
	lightExplicit := cssOverlay(dark, cssVars(t, base, "[data-theme=light]{"))
	lightMedia := cssOverlay(dark, cssVars(t, base, ":root:not([data-theme]){"))
	for _, p := range []struct {
		name string
		vars map[string]string
	}{
		{"暗档 :root", dark},
		{"亮档 [data-theme=light]", lightExplicit},
		{"亮档 :root:not([data-theme]) 媒体查询", lightMedia},
	} {
		bg, ok := cssColor(t, p.vars["--bg"])
		if !ok {
			t.Errorf("%s：读不到 --bg", p.name)
			continue
		}
		card, ok := cssColor(t, p.vars["--card"])
		if !ok {
			t.Errorf("%s：读不到 --card", p.name)
			continue
		}
		card = compositeOver(card, bg)
		ink2, ok := cssColor(t, p.vars["--ink2"])
		if !ok {
			t.Errorf("%s：读不到 --ink2", p.name)
			continue
		}
		// 有效颜色 = 字色按 opacity 压在卡片上（**合成发生在 sRGB 空间**，
		// 与浏览器一致——先在线性光里混会算错）。
		mix := func(f, b float64) float64 { return f*opacity + b*(1-opacity) }
		eff := cssRGBA{mix(ink2.r, card.r), mix(ink2.g, card.g), mix(ink2.b, card.b), 1}
		r := wcagContrast(eff, card)
		// 3:1 是 WCAG 1.4.11（非文本控件）那一档：它是一枚图标型按钮，
		// 它的**图形**就是全部的可及名（旁边没有文字替它说）。
		const min = 3.0
		if r < min {
			t.Errorf("%s：把手静息（opacity %.2f）压在卡片上只有 %.2f:1（要 ≥%.1f:1）",
				p.name, opacity, r, min)
		}
		t.Logf("%s：把手静息 opacity %.2f → %.2f:1（门槛 ≥%.1f:1）", p.name, opacity, r, min)
	}
}

// opacityOf 从一条规则文本里读 opacity 的值。
func opacityOf(t *testing.T, rule string) float64 {
	t.Helper()
	m := regexp.MustCompile(`opacity:\s*([0-9.]+)`).FindStringSubmatch(rule)
	if m == nil {
		t.Fatalf("规则里没有 opacity: %s", rule)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("opacity %q 读不成数: %v", m[1], err)
	}
	return v
}
