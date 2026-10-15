package render

import (
	"strings"
	"testing"
)

// 面板里那颗「删掉这一组」在**浮层里**走 app.js 那条 applyOps，而 app.js 必须
// **先问 remove** 再去读颜色/排法。
//
// 为什么不问形状而是钉这一行字：那张表单里名字输入框与取色器都常驻，删键是
// submitter 时它们照样跟着进 FormData，于是 `groupOpFromForm` 的第一句就决定了
// 这一族改的是哪一件事。把 remove 放到后面（或者干脆漏掉）的症状是**按了没反应**：
// layout/accent 都没在按下的那颗键上，rename 又没变，于是它返回 null，委托直接
// `if (!op) return` ——屏上什么也没发生，而**零 JS 的独立页照样删得掉**，两条路
// 于是分家（同一族的病：从前删一条书签只改了服务端那条路，浮层里删不掉）。
//
// 位置也要钉：它在 `layout`/`accent` 之前——读了它们就等于把一次"只删"变成
// "删 + 顺手改颜色"。
func TestAppJS_面板里删掉一组认的是remove而不是别的几格(t *testing.T) {
	src := readAppJS(t)
	const want = "if (data.has('remove')) return { op: 'remove', path: `groups/${gid}` };"
	at := strings.Index(src, want)
	if at < 0 {
		t.Fatalf("app.js 的 groupOpFromForm 里少了逐字行 %q：浮层里按「删掉这一组」会静默没反应（而零 JS 那条路照样删得掉，两边分家）", want)
	}
	for _, after := range []string{
		"const layout = data.get('layout');",
		"const accent = data.has('apply_custom')",
	} {
		i := strings.Index(src, after)
		if i < 0 {
			t.Fatalf("app.js 里找不到 %q，这一族测试的判据本身失效了", after)
		}
		if i < at {
			t.Errorf("%q 排在删组那一支之前：读它就等于把「只删一组」变成「删 + 顺手改那一格」", after)
		}
	}
	// 反向证人：判据是**名字**，不是"读没读到值"。写成 `data.get('remove') === '1'`
	// 也能过上面那行逐字检查之外的形状，但这一行字已经把形状钉死了——所以这里只
	// 补一句：它不许被写成一个与 layout/accent 同族的分支（那会是"删掉这一组"
	// 排在最后"，症状与漏掉一样）。
	if strings.Index(src, want) > strings.Index(src, "const box = form.querySelector('input[name=rename]');") {
		t.Error("删组那一支排到了改名那一支之后：前面的几格会先被读掉")
	}
}

// 「＋ 新分组」那一张**必须早退**，让它原生提交。
//
// 症状是"点「建这一组」按了没反应"：那份委托先 `preventDefault()` 再问
// `groupOpFromForm`，而那张表单没有 `input[name=group]`（新组的 id 由服务端
// 派生，界面上不问）→ 返回 null → 直接 return。于是**浮层里按了没反应，
// 零 JS 的独立页照样建得出来**，两条路分家——同一族的病（从前删一条书签只改了
// 服务端那条路）。
//
// 判据是**表单自己有没有 group 那一格**，不是"它是不是建组那张"：后者会把
// 这条判据写成一张名单，而名单下一个新表单就漏一次。
func TestAppJS_建组那张表单不归面板委托管(t *testing.T) {
	src := readAppJS(t)
	const want = "if (!form.querySelector('input[name=group]')) return;"
	at := strings.Index(src, want)
	if at < 0 {
		t.Fatalf("app.js 的面板委托里少了逐字行 %q：浮层里点「建这一组」会静默没反应（而零 JS 那条路照样建得出来，两边分家）", want)
	}
	// 它必须**早于**这一次委托自己的 preventDefault：放在后面就晚了，提交已经
	// 被吞掉了，return 也救不回来。（范围要**从委托那一行往后**找——整份 app.js
	// 里 preventDefault 有十几处，拿全文件第一个来比是比错了对象。）
	//
	// 委托那一行本身也钉住 `^=`：分组那一族的 action 带着 `?tab=<当前那一节>`
	// （不带着它，保存完跳回第一类），而 `form[action="/prefs/group"]` 匹配不到
	// 带查询串的——于是浮层里改颜色会静默变成整页提交（读起来像"设置变单独页面"）。
	from := strings.Index(src, "const form = e.target.closest('form[action^=\"/prefs/group\"]');")
	if from < 0 || from > at {
		t.Fatalf("找不到面板那份委托（要认 action^= 那个带查询串的形状），或它排在别处：从 %d，那一行 early-return 在 %d", from, at)
	}
	if p := strings.Index(src[from:], "e.preventDefault();"); p < 0 || at > from+p {
		t.Errorf("那一行 early-return 排在这一次委托的 preventDefault 之后：提交已经被吞掉，return 也救不回来")
	}
}

// 「删掉这一组」在**名字那一行右端**，展开后**独占一行**。
//
// 位置是量出来的，不是推出来的（1280 实拍）：名字那一行的右端那一列是
// 「这一组的键」（磁贴/单列/大卡/用这个，246px），而删是同一族的动作——挂在
// 它后面，那一行是 246+87=333，名字仍有约 550px（最长的组名要 99px）。
// **从色板那一行移上来**（2026-10-15 实拍后改判）：那一行右端空着 379px，
// 把一颗红字孤零零挂在十颗色点后面，它读起来像色板的一部分而不是另一个意图。
//
// 展开后独占一行是同顶栏「改名」那条 `.rename[open]`：确认要占的地方不该挤在
// 别的东西旁边。
func TestCSS_删掉这一组在名字那行右端且展开后独占一行(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	for _, want := range []string{
		".prefs-group-del{flex:0 0 auto;margin-left:auto}",
		".prefs-group-del[open]{flex:1 0 100%",
		// 三角要去掉**两态都去掉**：全局 `details[open]>summary::before` 的权重
		// (0,1,3) 高过 `.prefs-group-del>summary::before` 的 (0,1,2)，只写后者
		// 的话就是"收起时没三角、一展开 ▾ 又冒出来"（实拍逮到）。
		".prefs-group-del>summary::before{content:none}",
		".prefs-group-del[open]>summary::before{content:none}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("base.css 少了 %q", want)
		}
	}
	// 44px 不许在这里另开一族：summary 已经在导航族那条联合规则里
	// （`.gnav a,summary,…{min-height:44px}`），再写一条就是第七族。
	for _, sel := range []string{
		".prefs-group-del{flex", ".prefs-group-del[open]{flex",
		".prefs-group-del>summary{", ".prefs-group-del[open]>summary{",
		".prefs-group-del-box{",
	} {
		if r := extractRule(css, sel); strings.Contains(r, "min-height") {
			t.Errorf("%s 里又写了 min-height：44px 一族只写一遍\n%s", sel, r)
		}
	}
}
