package render

import (
	"strings"
	"testing"
)

// 排序这一族的复查（2026-10-16）。触发它的是一句话：「排序逻辑和相关逻辑都检查下」。
//
// 服务端那份契约早是对的（`internal/ops` 的 `move`：先摘除、pos 按摘除后算、index
// ∈ [0, bound]、缺席即追加末尾），客户端的落点 index 也与它逐条对齐。问题全在
// **"那一组在屏上是哪一个节点"** 这一个判据上：大卡带那一组被整组搬出了正文，它的
// 容器是 `ul.feat-row[data-gid]` 自己，而 app.js 里三处写着 `section[data-gid]`。
// 三个后果，三种形状：
//
//  1. 排序/搬动**落进或搬出大卡带**时算不出落点 → 走「算不出就退回重载」那一支：
//     服务端搬对了、屏上整页闪一次、每张卡重播入场。功能对，体感是"这一块坏了"。
//  2. **拖网址到大卡上**会一路找到 null，再由「拖到空白处退到第一组」兜底——
//     **存进了完全不相干的那一组**，而横幅说的是「已存进你指的那一组」。这一族里
//     最坏的一种：写进去了、报成功了、位置是错的。
//  3. **就地搬动之后空组那一句不跟着变**：把第一条搬进空组，卡片在那儿、那一句还在
//     说这一组还没有书签（从一组搬空则反过来）。那是 2026-10-16 那一轮把排序改成
//     "就地完成"带出来的——从前重载，模板重画，句自然对。
//
// 另外一条与容器无关：**多选搬组每一条都写 index:0**，一批下来落成的顺序是选中
// 顺序整个反过来（服务端那份形状由 internal/ops 的 TestBatchMoveKeepsOrderOnly… 钉住）。
func Test组容器判据认两种形状(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	// 判据本身：先按 [data-gid] 找组容器，再看它是 ul 自己还是 section 底下那个 ul。
	lo := between(js, "const listOf = (gid, root) =>", "\n  };")
	for _, want := range []string{`querySelector('[data-gid="' + gid + '"]')`, "host.tagName === 'UL' ? host : host.querySelector(':scope > ul')"} {
		if !strings.Contains(lo, want) {
			t.Errorf("listOf 缺 %q\n%.400s", want, lo)
		}
	}
	// 第二参数是根：接卡片要在**抓回来的那份 document** 里找同一组。
	if !strings.Contains(js, "const list = listOf(gid, doc);") {
		t.Error("spliceCardIn 没有在抓回来的那份 document 里找组：它找的是当前屏上的那份")
	}

	// **一处判据，三处用**。下面三条是这个坑的三个受害者，缺一条就退回旧写法。
	for _, want := range []string{
		"const srcList = dragSrc && listOf(dragSrc.gid);",
		"const ul = listOf(dstGid);",
		"const list = listOf(gid, doc);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("没有 %q：大卡带的组容器是 ul[data-gid] 自己，写死 section 就找不到它", want)
		}
	}
	// 剩下的写死 section 的地方要是还有别的用途，那得是"节"而不是"组"——
	// 排版切换那颗键只对正文组有意义（大卡组的 layout 存的就是 feature），那里保留。
	if strings.Contains(js, `document.querySelector('section[data-gid="' + dstGid + '"] ul')`) ||
		strings.Contains(js, `doc.querySelector('section[data-gid="' + gid + '"]')`) {
		t.Error("还有地方按 section[data-gid] 取组的列表：那是大卡带永远取不到的那一种")
	}

	// **拖进来的网址落进哪一组**：行 → 它那一组；组标题 → 标题那一组。从前是
	// `closest('section[data-gid]')`，大卡那张卡的祖先里没有一个 section 带 data-gid，
	// 于是 null → 兜底到第一组。判据与站内排序的 `dropTargetOf` 同源。
	gd := between(js, "const dropGroupGid = (node) =>", "\n  };")
	for _, want := range []string{"node.closest('li[data-item]')", "groupOf(li)", "node.closest('h2[data-group]')"} {
		if !strings.Contains(gd, want) {
			t.Errorf("dropGroupGid 缺 %q\n%.400s", want, gd)
		}
	}
	// 兜底那一支要认得出**两种**容器里的第一种组（判据在 groupsOnPage 一处）。
	if !strings.Contains(js, "groupsOnPage()[0]?.dataset.gid") {
		t.Error("拖到空白处的兜底还没走 groupsOnPage：页面上第一组可能是大卡带那一组")
	}

	// 组名（横幅里那句）按 `h2[data-group]` 找：大卡带的标题挂在带子里、不是 section 底下，
	// 写死 section 时存进大卡的那一条会在横幅上显示组 id 而不是组名。
	if strings.Contains(js, `document.querySelector('section[data-gid="' + gid + '"] h2')`) {
		t.Error("取组名还在 section[data-gid] h2 上：大卡带的标题在 ul 前面，不在那条路径里")
	}
}

// 空组那一句要跟着就地移动一起变（2026-10-16）。
//
// 那一句由模板**永远画在 DOM 里**（有内容时 `hidden`），脚本只翻开关：文字只有模板
// 那一处来源——脚本里另抄一份就是两处会漂，而 `profile.group.empty` 连语言包的
// `app.*` 切片都进不去（实测：屏上会是 `⟨profile.group.empty⟩`）。
func Test空组那一句跟着就地移动变(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	rawTmpl, err := webFS.ReadFile("web/profile.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	tmpl := string(rawTmpl)

	// 模板那一支：有内容时 hidden，不是"空才画"。少这一条，脚本翻开关就没有对象，
	// 而"自己造一句"那条路要的就是第二份文字。
	if !strings.Contains(tmpl, `<p class="hint grp-empty"{{if .Items}} hidden{{end}}>`) {
		t.Errorf("profile.tmpl 里空组那一句不是「永远画、有内容时 hidden」\n%.400s", between(tmpl, "grp-empty", "\n"))
	}

	se := between(js, "function syncEmpty(list) {", "\n  }")
	for _, want := range []string{
		"list.parentElement.tagName !== 'SECTION'", // 大卡带模板没画这一句
		":scope > p.grp-empty",                     // 只认模板画的那一个
		"!list.querySelector('li[data-item]')",     // 空 = 那个 ul 里一条都没有
	} {
		if !strings.Contains(se, want) {
			t.Errorf("syncEmpty 缺 %q\n%.400s", want, se)
		}
	}
	if !strings.Contains(se, "hint.hidden = !!list.querySelector(") {
		t.Errorf("syncEmpty 不是翻那一句的 hidden：\n%.400s", se)
	}
	// **不许自己造那一句**：文字只有模板一处来源，脚本再抄一份就是两处会漂。
	for _, banned := range []string{"T('profile.group.empty')", "createElement", "insertAdjacentHTML"} {
		if strings.Contains(se, banned) {
			t.Errorf("syncEmpty 里还有 %q：那一句的文字只有模板一处来源", banned)
		}
	}
	// 两处就地移动都要调它：排序搬动（跨组时源与目标各一次）与拖网址进来
	// （后者常常就是**第一条**）。数的是调用点：定义 1 + 排序 2 + 接卡片 1。
	if n := strings.Count(js, "syncEmpty("); n != 4 {
		t.Errorf("syncEmpty 一共出现 %d 处，要 4 处（定义 + 排序的源/目标 + 接卡片）："+
			"少一处就有一处搬完之后那句话留在那儿说反了", n)
	}
	if !strings.Contains(js, "syncEmpty(srcList)") {
		t.Error("排序那一处没有同步**源组**：从一组搬空之后那句「这一组还没有书签」要回来")
	}
}

// 多选搬一组：**不给 index**。
//
// ops 在锁内按数组次序应用，缺席即追加到末尾，于是批内先后原样落下去；每一条都写
// index:0 则是第 2 条把第 1 条顶到后面——一批整个倒过来。服务端那份形状在
// internal/ops 的 TestBatchMoveKeepsOrderOnlyWhenIndexIsAbsent（钉的是"倒过来"确实是
// 那个形状），这里是客户端那一半。
func Test多选搬组不给index(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	mv := between(js, "const ops = picked.map((li) => ({", "\n            }));")
	if mv == "" {
		t.Fatal("app.js 里找不到多选搬组那一条 op 的构造")
	}
	if strings.Contains(mv, "index") {
		t.Errorf("多选搬组又给了 index：一批 move 按次序应用，每条都插到最前就是"+
			"把选中顺序整个反过来\n%.400s", mv)
	}
	for _, want := range []string{"op: 'move'", "to,", "groups/${groupOf(li).dataset.gid}/items/"} {
		if !strings.Contains(mv, want) {
			t.Errorf("多选搬组那一条 op 缺 %q\n%.400s", want, mv)
		}
	}
	// 批内先后不是偶然：取的是 `pickedItems()`，那就是屏上那一列从上到下。
	if !strings.Contains(js, "const pickedItems = () =>") {
		t.Error("pickedItems 没了：多选搬组的批内先后全靠它给出的次序")
	}
}
