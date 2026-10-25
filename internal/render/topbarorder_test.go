package render

import (
	"strings"
	"testing"
)

// 搜索结果页的页头**不该长高一倍**（2026-10-22 改）。
//
// `.hits`（「找到 N 条…」那条读数）是 `flex:0 0 100%`——它要**独占一行**（那是它自己
// 的形状：一条读数，不是顶栏上的一颗键）。而它从前排在 nav **前面**，于是那一行把
// nav 顶到了**第三行**：搜索结果页的页头从 46px 涨到 130px，右上角那两枚徽标掉到
// 第三行，而第二行右边空着 460px（实测 1280 档）。正文也被推低了 84px。
//
// 换个顺序就对了：第一行照旧是「标题 + 搜索 + ＋添加 + 徽标」，读数那一行仍在它
// 原来的位置（第二行），页头 76px。**Tab 顺序也不变**（读数那一行不是可聚焦的）。
//
// 判据是**读数那一行排在 nav 之后**——判据只看**顺序**，不看 CSS 那一行的宽度：
// 宽度是「读数要独占一行」那条形状，它与「谁在前面」是两件事。
func Test搜索态的读数那一行排在徽标之后(t *testing.T) {
	tmpl := readWeb(t, "web/profile.tmpl")
	navAt := strings.Index(tmpl, `<nav aria-label="{{.T "profile.nav.aria"}}">`)
	hitsAt := strings.Index(tmpl, `<p class="hits">`)
	if navAt < 0 {
		t.Fatal("模板里找不到顶栏那个 nav")
	}
	if hitsAt < 0 {
		t.Fatal("模板里找不到那条读数（.hits）")
	}
	if hitsAt < navAt {
		t.Error("读数那一行排在 nav 前面：它 flex:0 0 100% 独占一行，于是把 nav 顶到第三行" +
			"（页头从 46px 涨到 130px，右上角那两枚徽标掉到第三行）")
	}
	// 反向证人：读数那一行仍然**独占一行**（那是它的形状，与顺序是两件事）。
	css := stripCSSComments(readWeb(t, "web/base.css"))
	if !strings.Contains(css, ".hits{flex:0 0 100%;") {
		t.Error("读数那一行不独占一行了：它会挤在顶栏那一行里，与徽标抢位置")
	}
}
