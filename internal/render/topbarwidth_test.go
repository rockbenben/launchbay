package render

import (
	"strings"
	"testing"
)

// 两个「屏上不对、测试不红」的形状，都出在**同一族**：一个元素该跟别的元素
// 对齐，却没有跟它对齐的那份约束。

// 顶栏分组导航那一行必须与下面的内容**同一条左边线**（2026-10-14 实拍）。
//
// 那一处写在 `.shell` **之外**（模板的 `{{if $top}}`），而 `header` 与 `.shell`
// 都带 `max-width:1600px;margin:0 auto;padding:0 16px`——唯独它什么都没写。
// 于是视口宽于 1600 时它顶到屏幕 0、下面那屏是居中的：左边线差出
// (100vw−1600)/2 + 16。没有人会报这个错，它只是看着没对齐。
func Test顶栏那一行跟下面的内容同一条左边线(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))

	i := strings.Index(css, ".gnav.top{")
	if i < 0 {
		t.Fatal("base.css 里找不到 .gnav.top")
	}
	rule := css[i:]
	if j := strings.Index(rule, "}"); j >= 0 {
		rule = rule[:j]
	}

	// 三个数与 header / .shell **同一份**：max-width / margin / padding。
	// 抄错一个就重新错开，而错开不会有人报。
	if !strings.Contains(rule, "max-width:1600px") {
		t.Errorf("顶栏那一行没有 max-width：宽屏上它与下面那屏不是同一条内容宽\n实际那条：%s", rule)
	}
	if !strings.Contains(rule, "padding:0 16px") {
		t.Errorf("顶栏那一行没有那 16px 内衬：左边界会比下面那屏多出/少出 16\n实际那条：%s", rule)
	}
	// margin 必须是 `0 auto`——`0 0 14px` 会让它靠左，而 auto 才与下面那屏同一条线。
	if !strings.Contains(rule, "margin:0 auto 14px") {
		t.Errorf("顶栏那一行的 margin 不是 `0 auto 14px`：不居中就没有同一条左边线\n实际那条：%s", rule)
	}
	// 反向证人：从前的写法是 `margin:0 0 14px`（靠左）。
	if strings.Contains(rule, "margin:0 0 14px") {
		t.Error("margin 退回靠左了：那一行顶到屏幕 0，而下面那屏是居中的")
	}

	// 参照物本身要在场（这一条解释的是"与谁对齐"）
	for _, ref := range []string{
		"header{display:flex;flex-wrap:wrap;align-items:center;gap:10px 12px;\n  max-width:1600px;margin:0 auto 18px;padding:0 16px}",
		".shell{display:grid;grid-template-columns:206px minmax(0,1fr);gap:24px;align-items:start;\n  max-width:1600px;margin:0 auto;padding:0 16px}",
	} {
		if !strings.Contains(css, ref) {
			t.Errorf("对齐的参照物变了：%s\n它一变，上面那三个数就成了抄着一个旧参照物", ref[:40]+"…")
		}
	}
}

// 台面面板那四格（改名 / 怎么看 / 分组导航 / 主题）里，**只有主题能就地生效**。
//
// 从前四格一律不重渲染，于是 view/layout/title 按下去屏上一动不动——症状是
// 「设置改了没生效」，而服务端明明答了 303、数据也写进去了。**留在浮层在这里
// 赢不了立刻见效**：屏上没变化时，人不会想"要不要刷新"，他会以为没保存成功，
// 于是按第二遍——第二遍同样没变化。
//
// 判据是**这一格的效果画在哪里**：data-theme 是 <html> 上的一个属性，换掉它
// 整页立刻变色；view/layout/title 决定的是整页怎么画，没有一个属性可换。
func Test台面那四格里只有主题能就地其余必须重渲染(t *testing.T) {
	js := readWeb(t, "web/app.js")

	i := strings.Index(js, `form[action="/prefs"]`)
	if i < 0 {
		t.Fatal("app.js 里找不到 /prefs 那张表单的委托")
	}
	seg := js[i:]
	if j := strings.Index(seg, "// 背景那一节"); j > 0 {
		seg = seg[:j]
	}

	// 一：主题就地——换 <html> 上那一个属性，并就地按那颗键。
	if !strings.Contains(seg, "document.documentElement.dataset.theme =") {
		t.Error("主题没有就地换 data-theme：它本可以不重渲染就生效")
	}
	// 二：其余三格重渲染。
	if !strings.Contains(seg, "location.reload();") {
		t.Error("view/layout/title 没有重渲染：按下去屏上一动不动，而数据写进去了" +
			"（症状是「设置改了没生效」，人会以为没保存成功然后再按一遍）")
	}
	// 三：**主题那条不许落到 reload**。它能就地，就地更即时（浮层也不必重建）。
	themeAt := strings.Index(seg, "btn.name === 'theme'")
	loadAt := strings.Index(seg, "location.reload();")
	if themeAt < 0 || loadAt < 0 || loadAt < themeAt {
		t.Error("主题那一支没有在 reload 之前 return：主题会被无谓地整页重载")
	}
	if !strings.Contains(seg, "if (btn.value != null) markPressed(form, btn.value);\n          return;") {
		t.Error("主题就地之后没有 return（也没就地按那颗键的 aria-pressed）")
	}
}
