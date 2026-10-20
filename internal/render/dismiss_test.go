package render

import (
	"strings"
	"testing"
)

// 「点层外关掉那一层」（2026-10-14）。
//
// 实拍到的症状：唤出 ⌘K 之后点了页面别处，面板纹丝不动——而背后整页还被
// `inert` 摘掉了可聚焦性，于是点不动、Tab 也出不去。诊断：`document` 上**只有**
// 一个 keydown 监听，从前没有任何 pointerdown/mousedown/click 的「点外面」路径，
// Esc 与 × 是仅有的两个出口。
//
// 这一族钉的是**一份判据两处共用**，而不是「多了一个监听器」。
func Test点层外能关掉最上面那一层(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	// 一：监听器在，且用 pointerdown（不是 click——按下后拖一点再松开时 click
	// 压根不发生，而"甩一下关掉"是最自然的那个动作）。
	if !strings.Contains(js, "document.addEventListener('pointerdown'") {
		t.Error("没有 pointerdown 的层外监听：点外面关不掉那一层")
	}
	// 二：判据是 contains（"这一下点在谁身上"，DOM 结构说了算），
	// 而不是量 rect（层内重排时 rect 会变）。
	if !strings.Contains(js, "if (top.contains(e.target)) return;") {
		t.Error("层外判据不是 contains：改用坐标会在层内重排时判错")
	}

	// 三：**一份判据两处共用**。Esc 与「点外面」各写一份顺序，早晚会漂，
	// 而漂的症状是"某一层按 Esc 关了、点外面却关不掉"或反过来。
	if !strings.Contains(js, "function closeTopLayer()") {
		t.Error("没有 closeTopLayer：Esc 与「点外面」各写一份必然漂")
	}
	if !strings.Contains(js, "if (e.key === 'Escape') {\n      e.preventDefault();\n      closeTopLayer();") {
		t.Error("Esc 那一支没有走 closeTopLayer")
	}
	if strings.Count(js, "closeTopLayer();") < 2 {
		t.Error("「点外面」那一支没有走 closeTopLayer")
	}

	// 四：「哪些算一层」也只有一份（topLayer），syncTrap 与层外关共用。
	// 两份各自写一遍时：syncTrap 少认一层 → 那一层开着时背后仍可聚焦；
	// 层外关少认一层 → 点了外面关不掉它。两种症状都不响。
	if !strings.Contains(js, "const top = topLayer();") {
		t.Error("syncTrap 没有复用 topLayer：「哪些算一层」出现了第二份判据")
	}
	if strings.Contains(js, "document.querySelectorAll('#lb-sheet, .cmdk, .sheet')].filter((n) => !n.hidden).pop()") &&
		strings.Count(js, "#lb-sheet, .cmdk, .sheet") > 1 {
		t.Error("那一串选择器在两处各写了一遍")
	}

	// 五：**多选不在「关掉」的路上**。它是页面的一部分（顶栏一条多选带），
	// 点外面去不叫"关掉弹层"；Esc 要管它，所以那一支必须 return false
	// 让「点外面」继续往下一层找，而不是把整条多选带撤掉。
	if !strings.Contains(js, "if (multi.on) { return false; }") {
		t.Error("多选那支不是 return false：点外面会把整条多选带撤掉")
	}

	// 六：摘掉 DOM 的小层（挑组/挑标签/确认）**必须同步 syncTrap**——不调的话
	// 页面会整片留在 inert 里（面板没了，一个字都点不动）。
	//
	// 2026-10-20 起这一支还是三件事：摘掉、同步 inert、**把焦点还给打开它的那
	// 一位**（焦点原本停在被摘掉的节点上，键盘用户会被扔回页面顶端）。判据按
	// **三件事各在不在**来钉，不按整行字面量——那一行每次改个缩进就红一次，
	// 而这条闸要守的是行为。
	if !strings.Contains(js, "dlg.remove();") ||
		!strings.Contains(js, "syncTrap();") ||
		!strings.Contains(js, "opener.focus()") {
		t.Error("摘掉小层那一支少了动作：摘掉 / 同步 inert / 焦点归还，三件都要在")
	}
}
