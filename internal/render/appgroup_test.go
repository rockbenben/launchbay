package render

import (
	"strings"
	"testing"
)

// 添加浮层里「分组」那一格。钉的是**建组入口一直都在**这件事。
//
// 2026-10-13 之前：新分组只在"一个分组都还没有"的时候问——
// `fNewGroup.hidden = fGroup.options.length > 0`。于是有了书签之后，界面上
// **再也建不了新分组**，而 ops 一直认 `add path=groups`。那不是「选了不生效」，
// 是"这一条根本不在界面上"：CLI 与 AI 能建，一家人自己在网页上不能。
//
// 形状钉三样：末尾那颗哨兵选项**无条件追加**、下拉**不再被藏起来**、建组那一批
// ops 用的是人挑的那一档而不是写死的 tiles。三样缺一，症状各不相同，而都
// 不会让任何一条功能测试变红——所以逐字行钉，与 `TestCSS_硬规格在场` 同一族。
func TestAppJS_建组的入口一直在而不是只在零分组时(t *testing.T) {
	src := readAppJS(t)

	wants := []string{
		// 哨兵选项无条件追加：它是**每一份档案**的建组入口。
		"fGroup.append(el('option', { value: NEW_GROUP, text: T('app.group.new') }));",
		// 写侧：建组那一条 ops 带上人挑的 layout，而不是恒定的 tiles。
		"layout: shape",
		// 读侧：submit 认那颗哨兵，而不是认"下拉被藏起来了"。
		"if (fGroup.value !== NEW_GROUP) {",
	}
	for _, w := range wants {
		if !strings.Contains(src, w) {
			t.Errorf("app.js 少了逐字行 %q：建组入口又不常在了（症状是「有了书签之后网页上建不了新分组」，而任何功能测试都不会红）", w)
		}
	}

	// **不许再出现藏下拉或按选项数藏名字框的那两种写法**。它们不是"也可以"的
	// 实现，是这一格变回死路的那条路：只要有一行还在，零分组以外的档案仍然建不了组。
	//
	// 这里逐字钉的是"这两句不在"，而不是"某个函数不在"——函数可以被搬走重写，
	// 而这两句只要还在就足以把洞重新打开。
	//
	// 扫的是**剥掉注释之后**的可执行代码（`stripJSLine`，与
	// `TestAppI18N_可执行代码里没有漏翻的中文` 同一份）：上面那段解释"从前怎么
	// 坏的"必须把旧写法逐字引出来，而直接扫原文会把那段注释也算成违规——于是
	// 这条闸逼着人不写解释，而解释正是下一个人改回去的理由。
	code := stripJSLine(src)
	for _, banned := range []string{
		"fNewGroup.hidden = fGroup.options.length > 0",
		"fGroup.hidden = true",
	} {
		if strings.Contains(code, banned) {
			t.Errorf("app.js 的可执行代码里还有 %q：那一行是建组入口只在零分组时才在场的写法，正是这一条要拆掉的东西", banned)
		}
	}
}

// 建新分组时的排法与名字框**跟着下拉当前值显隐**，不是永远摆着。
//
// 钉的是"只在建组时出现"：往已有分组里加一条时它没有作用，而一个永远在、
// 永远不动的输入框是界面上最吵的一种东西（§8.1）。这一条量的是**出现与消失
// 两侧**——只钉出现的话，写成 `fNewGroup.hidden = false` 的静态显示也会全绿。
func TestAppJS_建组那几格跟着下拉显隐(t *testing.T) {
	src := readAppJS(t)
	for _, w := range []string{
		"const creating = fGroup.value === NEW_GROUP;",
		"fNewGroup.hidden = !creating;",
		"fGroupShapeWrap.hidden = !creating;",
		"fGroup.addEventListener('change', syncGroupFields);",
	} {
		if !strings.Contains(src, w) {
			t.Errorf("app.js 少了逐字行 %q：建组那几格不会跟着下拉显隐了（要么永远占着位置，要么选了新分组也不问名字）", w)
		}
	}
	// 连着往同一个新分组里加第二条时，刚建出来的那一组必须**就地进下拉并选中**。
	// 不然下一次还停在「＋ 新分组…」且名字框留着上一次的内容，于是拿同一个
	// group id 再 add 一遍——ops 拒掉，而屏幕上那句话说的是"保存失败"。
	//
	// 钉的是**三片同时出现**（条件、追加、选中）：少了"追加"是新分组下不拉里
	// 没有它，少了"选中"是它在里面但下一次仍停在建组那一档。逐字行而不是一条
	// 完整语句——这条语句里混着转义引号，逐字抄它等于把排版也钉住。
	for _, w := range []string{
		"if (g.ops.length && !fGroup.querySelector(",
		"fGroup.insertBefore(opt, fGroup.lastElementChild);",
		"fGroup.value = g.gid;",
	} {
		if !strings.Contains(src, w) {
			t.Errorf("app.js 少了逐字行 %q：连着往同一个新分组里加第二条会拿同一个 id 再建一遍，然后报一句保存失败", w)
		}
	}
}

// `hidden` 属性在浮层里**真的藏得住**——这一条是上面那条的前提，而它自己是一个
// 早就坏了的东西：浏览器默认的 `[hidden]{display:none}` 权重 0，而 `.fld` 与
// `.sheet label` 都是作者规则（0,1,0 / 0,1,1），作者规则赢。于是
// `el.hidden = true` 静默失效，屏幕上照旧摆着。
//
// 症状从前就在骗人：旧代码按选项数把「新分组叫什么」藏起来，那一句从来没生效
// ——输入框一直摆着，填了也不起作用（groupOpsAndTarget 只在零分组那一支读它）。
// **不是新写的代码发现的，是旧的一直没被发现的**，所以钉的是那条盖得住的规则
// 本身，而不是"某个元素被藏了"。
func TestCSS_浮层里的hidden盖得住作者规则的display(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		".fld{display:block",
		".sheet label{display:block",
		".sheet [hidden]{display:none}",
	} {
		if !strings.Contains(string(raw), w) {
			t.Errorf("app.css 里少了 %q：`.fld` 与 `.sheet label` 都是作者规则，权重压过浏览器默认的 `[hidden]{display:none}`——少了那条盖得住的，代码里每一个 el.hidden = true 都是空话", w)
		}
	}
}

// `el()` 写属性时**空串也要写上去**——这一条是两个真实 bug 的共同根因。
//
// 少写一个空 `value` 属性，对 `<option>` 来说不是"少几个字节"而是**换语义**：
// 按规范，没有 value 属性的 option 其 `.value` 回退到**文字**。于是「不选图标
// （用名字第一个字）」那一颗读回来的是这句中文，submit 把它当图标名发出去。
// 症状是**每一条从浮层存进去的书签都存不进去**，而那一格看上去完全正常。
//
// 同一个开关还害着 `hidden`：`.fld` 与 `.sheet label` 都是作者规则，压过浏览器
// 默认的 `[hidden]{display:none}`（见上面那条）。
//
// 钉的是"**空串不跳过**"这件事本身，而不是某一个 option——只钉那一颗的话，
// 下一个写 `class: ”` 的人会把同一个坑再踩一遍，而那次的症状更不明显。
func TestAppJS_el写属性时空串不跳过(t *testing.T) {
	src := readAppJS(t)
	if !strings.Contains(src, "else if (v != null) n.setAttribute(k, v);") {
		t.Error("app.js 里 el() 又开始跳过空串了：`<option value=\"\">` 少写一个 value 属性就是换语义（.value 回退成文字），而 `hidden`/`class` 少写一个是静默失效")
	}
	if strings.Contains(stripJSLine(src), "v !== '' && v != null") {
		t.Error("app.js 里还有 `v !== ''` 那一种写法：它就是这条要拆掉的东西")
	}
}
