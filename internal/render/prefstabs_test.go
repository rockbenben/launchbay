package render

import (
	"github.com/rockbenben/launchbay/internal/i18n"
	"strings"
	"testing"
)

// 左侧分类 tab（2026-10-15）。钉的是三件事，缺一件都还不了原：
//
//  1. **三颗 radio 与三块 pane 一一对应**，而且**默认那颗 checked**——不 checked
//     就谁都不显示（pane 默认 `display:none`），整块面板是空的。
//  2. **CSS 里有那六条显隐规则**：切换靠 `:checked ~ .prefs-panes` 这一族兄弟
//     选择器，写不成 `JS 切 class` 也不用切——`/prefs` 那张独立页**不引 app.js**，
//     而同一份 define 也渲染进档案页的浮层，所以只有这两条路都成立的写法才
//     不会让两条路分家（症状是"浮层里有 tab、独立页里没有"）。
//  3. **窄屏那一档翻成顶部一条**：248px 的内容宽里 150px 的竖列会把色板那一行
//     （512）彻底挤没。
//
// 第四件不在这里钉而在那里：44px 归导航族那条联合规则管（族数闸数的是规则条数）。
func TestPrefsTabs_三颗radio与三块pane一一对应且默认选中第一颗(t *testing.T) {
	html := prefsPanelHTML(t, "look")
	if n := strings.Count(html, `class="prefs-tab" type="radio"`); n != 4 {
		t.Fatalf("面板里 %d 颗 tab radio，要 4 颗（第四颗是 2026-10-20 加的「我的 PIN」）", n)
	}
	if !strings.Contains(html, `id="prefs-tab-look" checked`) {
		t.Error("第一颗（外观）没有 checked：不 checked 就谁都不显示，整块面板是空的")
	}
	for _, id := range []string{"prefs-tab-look", "prefs-tab-out", "prefs-tab-groups", "prefs-tab-pin"} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Errorf("少了一颗 tab radio：%s", id)
		}
		// 标签的 for 必须指着那颗 radio，否则点标签按不动。
		if !strings.Contains(html, `for="`+id+`"`) {
			t.Errorf("tab 标签没有 for=%q：点它按不动那颗 radio", id)
		}
	}
	// **三块 pane 都要有 id**：显隐规则是按 id 指的（pane 是 fieldset 的兄弟，
	// `:has()` 从共同祖先上选，落到 pane 上只能按 id）。少了第一块那个 id，
	// 于是"按第二、三颗时关掉外观"那两条**匹配不到任何东西**——而默认态看起来
	// 一切正常（默认那颗恰好就是外观），症状是"切到分组还是显示外观"。
	for _, id := range []string{"prefs-pane-look", "prefs-pane-out", "prefs-pane-groups", "prefs-pane-pin"} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Errorf("少了一块 pane 的 id：%s（显隐规则按 id 指，少一个就有一处切不干净）", id)
		}
	}
}

func TestPrefsTabs_切换靠兄弟选择器而不是JS(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	// 六条：每颗 radio 关掉其余两块。三颗 radio 各两块，缺一条就有一种组合
	// 露出**两节**（或全空）。
	for _, want := range []string{
		".prefs-tabs:has(#prefs-tab-look:checked) #prefs-pane-out",
		".prefs-tabs:has(#prefs-tab-look:checked) #prefs-pane-groups",
		".prefs-tabs:has(#prefs-tab-out:checked) #prefs-pane-look",
		".prefs-tabs:has(#prefs-tab-out:checked) #prefs-pane-groups",
		".prefs-tabs:has(#prefs-tab-groups:checked) #prefs-pane-look",
		".prefs-tabs:has(#prefs-tab-groups:checked) #prefs-pane-out",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("base.css 少了显隐规则 %q：少一条就有一种组合露出两节", want)
		}
	}
	// 退化的方向必须是"看得全"：pane 默认可见，只有被 checked 的那颗把它关掉。
	// 写成 `.prefs-pane{display:none}` + 三条打开规则的话，CSS 没加载时整块
	// 面板是空的——那是"看不见"的退化方向。
	if strings.Contains(css, ".prefs-pane{display:none}") {
		t.Error("pane 默认被藏起来了：CSS 没加载时整块面板会是空的（退化方向应该是看得全）")
	}
	if !strings.Contains(css, "#prefs-pane-look") {
		t.Error("没有按 id 指定 pane：靠位置（第几块）显隐的话，中间插一块就全错位")
	}
	// 窄屏翻成顶部一条——见本文件顶上那段第 3 条。
	if !strings.Contains(css, ".prefs-tabs{flex-direction:column") {
		t.Error("窄屏那一档没有把左侧那列翻成顶部一条：248px 的内容宽里 150px 的竖列会把色板那一行挤没")
	}
}

// 停在哪一节要**活过一次保存**——它是 URL 上的 `?tab=`，而分组那一节里每一次
// 按色块都是一次提交。
//
// 不做这一条的症状是"按了没反应"变成"按了跳到别处"：二十个分组时改完第二十组
// 被弹回第一组，人要重新滚回去。判据两处：渲染时**只有**那颗 checked（不是三颗
// 都按、也不是一颗都不按），以及分组那一族的表单 action 带着它。
func TestPrefsTabs_停在哪一节由tab说了算且表单带着它(t *testing.T) {
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", withIcons(2))
	r := New(s, ix, fc)

	for _, tab := range []string{"look", "out", "groups"} {
		page, err := r.PrefsPage("dad", tab, i18n.Zh)
		if err != nil {
			t.Fatal(err)
		}
		html := string(page.Body)
		// **只数这一列的三颗**（batch11 T1 之后）：从前这条证人数字面上所有
		// ` checked>`，那时全页只有 tab 那一族用 radio。现在「＋ 新分组」那一行
		// 的三颗排法也是 radio（它们选而不发，提交的是那颗「建这一组」），
		// 数面上所有 checked 就成了 2 —— 而它要钉的那件事（这一列一次只停在一节）
		// 一个字都没变。改的是**证人的口径**，不是把断言删掉。
		n := 0
		for _, one := range []string{"look", "out", "groups"} {
			if strings.Contains(html, `id="prefs-tab-`+one+`" checked>`) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("tab=%s 时三颗里有 %d 颗带着 checked，要恰好 1 颗（这一列一次只停在一节）", tab, n)
		}
		if !strings.Contains(html, `id="prefs-tab-`+tab+`" checked>`) {
			t.Errorf("tab=%s 时按下的不是它自己", tab)
		}
		// 分组那一族的表单**必须**带着 `?tab=groups`——**它带的是自己那一节，
		// 不是「当前那一节」**（2026-10-20 改判）。
		//
		// **旧理由是什么**：从前它带的是 `{{$.PrefTab}}`（页面上停在哪一节），
		// 理由是「停在哪要活过一次保存」。
		//
		// **为什么现在不成立**：**独立页上那列 radio 根本不改 URL**（那张页不引
		// app.js，radio 只切本地）——于是「当前那一节」在人按下去的那一刻**已经
		// 变了**，而表单上带的是**页面加载时**的那一节。实测（2026-10-20）：独立页
		// 打开在「外观」，点「分组」，按一个颜色 → 落回 `/prefs?tab=look`，**人
		// 刚待的那一节没了**。带自己那一节就落在 `/prefs?tab=groups`，人还在原地。
		//
		// 浮层那条路不受影响：那里的分组表单被 app.js 拦下走 applyOps（不导航），
		// 带的那个 `?tab=` 压根没人读。
		if !strings.Contains(html, `action="/prefs/group?tab=groups"`) {
			t.Errorf("tab=%s 时分组表单的 action 没带自己那一节：改完会跳回别的节", tab)
		}
		// 而它**不许**带「当前那一节」——那正是上面那条实测里的错法。
		if tab != "groups" && strings.Contains(html, `action="/prefs/group?tab=`+tab+`"`) {
			t.Errorf("tab=%s 时分组表单带了当前那一节：独立页上 radio 不改 URL，带它是带了个错的", tab)
		}
	}
	// 不认识的值与空串都落回第一颗——那是"三颗都没按"的反面（空屏）。
	for _, bad := range []string{"", "nope", "look&x"} {
		page, err := r.PrefsPage("dad", bad, i18n.Zh)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(page.Body), `id="prefs-tab-look" checked>`) {
			t.Errorf("tab=%q 没有落回第一颗", bad)
		}
	}
}

// prefsPanelHTML 取渲染出来的面板。走真正的渲染路径（不是读模板），因为这一族
// 的病多数在"模板与 CSS 说的一致、而实际渲染出来不是那样"。
func prefsPanelHTML(t *testing.T, tab string) string {
	t.Helper()
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", withIcons(2))
	page, err := New(s, ix, fc).PrefsPage("dad", tab, i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	return string(page.Body)
}
