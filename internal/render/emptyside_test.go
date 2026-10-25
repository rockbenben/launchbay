package render

import (
	"strings"
	"testing"
)

// **一个分组都没有时，不留侧栏那条空带**（2026-10-22）。
//
// 侧栏那一列是定宽 206px，而「分组」那一格空的时候那 206px **什么都不画**
// （实测 `.side` 宽 206、**高 0**，`summary` 是 `display:none`）。
// 于是新建的第一份档案的第一屏就是：左边一条 206px 的空白，正文从 x=266 开始，
// 而右边只有 36px——**内容偏在右边**，看着像排版没对。
//
// **判据由服务端给**（`.shell` 上那个 `no-side`）：它是"这一份有没有分组"这件事的
// 唯一判据（`.Bookmarks.Nav` 为空 = 没有分组）。
//
// **不许用 CSS 的 `:empty` 去判**：那个 `ul` 里留着换行的空白，而 `:empty` 在浏览器里
// 不算它空——第一版就是这么写的，规则**根本没生效**（实测侧栏仍占 206px）。
//
// **它仍是纯 CSS 的隐藏，标记照旧在 DOM 里**（与紧凑档那一族同一条纪律）：服务端给
// 的是**一个类名**，不是"不渲染那一列"——零 JS 时页面与开着 JS 时长得一样。
func Test没有分组时侧栏那一列不占(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	tmpl := readWeb(t, "web/profile.tmpl")

	// 一：判据在服务端（`.Bookmarks.Nav` 为空 = 没有分组），不是一个抄的字面量。
	if !strings.Contains(tmpl, `<div class="shell{{if not .Bookmarks.Nav}} no-side{{end}}">`) {
		t.Error("`.shell` 上没有那个 no-side 类：没有分组时侧栏那一列还占着 206px（实测正文左 266 / 右 36）")
	}
	// 二：那一条 CSS 认这个类名。
	if !strings.Contains(css, ".shell.no-side{grid-template-columns:minmax(0,1fr)}") {
		t.Error("`.shell.no-side` 没有把栅格收成单列")
	}
	// 三：侧栏那一列**服务端就不画**（不是"画了再藏"）：那里空的时候本来就是
	// 206px 的空气（summary 是 display:none、.side 量出来高 0），画不画看得见的东西
	// 一样，而不画就不占掉栅格的第一列——**栅格首列是写死的 206px**，内容没了列还在。
	// （紧凑档那一族藏侧栏是**因为视图档位**，标记要留在 DOM 里，判据不一样。）
	if !strings.Contains(tmpl, "{{if .Bookmarks.Nav}}<aside class=\"side\">") {
		t.Error("侧栏那一列没有按分组数条件化：空的时候它还占着栅格第一列")
	}
	// 四：顶栏那一档（分组列表在 `.shell` 之外，空着就是一条空带）——服务端不画它。
	if !strings.Contains(tmpl, `{{if .Bookmarks.Nav}}<nav class="gnav top"`) {
		t.Error("顶栏布局那一档，没有分组时那一行还画着（一条空带）")
	}
	// 反向证人：顶栏那一档本来就是单列——上面那条判据要跟它同形。
	if !strings.Contains(css, "[data-profile-layout=topbar] .shell{grid-template-columns:minmax(0,1fr)}") {
		t.Error("顶栏那一档的参照物变了：上面那条判据要跟它同形")
	}
}
