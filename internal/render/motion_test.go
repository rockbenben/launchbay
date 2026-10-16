package render

import (
	"strings"
	"testing"
)

// 动效这一族（2026-10-16）。三条判据，都与"看起来好不好"无关：
//
//  1. **只碰 transform / opacity**（spec §8.1：不碰会触发重排的属性）。一条
//     `width` 或 `left` 会在动画期间把整页重排几十次——在 LAN 机器上顺带把
//     那一帧的滚动条抖一下。这一条钉的是**属性名**，不是写法。
//  2. **减弱动效必须显式点名**。base.css 那条 `@media (prefers-reduced-motion:reduce)`
//     里有 `*{transition:none!important}`，但**没有** `animation:none` 的通配：
//     通配会掀掉别处刻意保留的东西（极光漂移是逐条点的）。于是新加的动画必须
//     自己写进那个块——而 `li.is-new` 的权重高于通配，**不点名就关不掉**。
//  3. **排序不再整页重载**。此前每一次排序/移动都以 reload 收尾，于是"松手"
//     这一下看到的是整页闪一次、每张卡重播一遍入场；而排序恰恰是全案唯一一个
//     "人一直盯着那一行"的动作。钉的是落点处理器调 `moveInPlace` 而不是
//     `reloadKeepingPlace`，且 `moveInPlace` 算不出落点时**仍要退回重载**——
//     宁可旧样子，不能错样子。
func Test动效只碰transform与opacity(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	allow := map[string]bool{"transform": true, "opacity": true, "box-shadow": true, "filter": true}
	seen := 0
	for _, block := range keyframeBlocks(css) {
		seen++
		// 关键帧里每一拍是 `from{…}` / `0%,50%{…}` —— 先按 `}` 切成一拍一拍，
		// 再把选择器（`{` 之前那截）扔掉，剩下的才是声明。
		for _, step := range strings.Split(block, "}") {
			decls := step
			if k := strings.LastIndex(step, "{"); k >= 0 {
				decls = step[k+1:]
			}
			for _, decl := range strings.Split(decls, ";") {
				prop, _, ok := strings.Cut(strings.TrimSpace(decl), ":")
				if !ok {
					continue
				}
				prop = strings.TrimSpace(prop)
				if prop == "" {
					continue
				}
				if !allow[prop] {
					t.Errorf("@keyframes 里写了 %q：动效只碰 transform/opacity，"+
						"其余属性每一帧都触发重排（spec §8.1）", prop)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("app.css 里一个 @keyframes 都没有：本该有新增条目的落位与光环")
	}
	// 顺带把"这族动效真的存在"钉住：只有一条空壳的 keyframes 也算上面那条绿。
	for _, want := range []string{"@keyframes lb-pop", "@keyframes lb-ring", "li.is-new", "li.is-new::after"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css 里少了 %q", want)
		}
	}
	for _, bannedUse := range []string{"li.is-settle", "lb-settle"} {
		if strings.Contains(css, bannedUse) {
			t.Errorf("还留着 %q：给同一条同时挂 scale 关键帧与 translate 内联变换是互斥的——"+
				"animation 在跑时内联 transform 不生效，那一行会先跳到新位置再滑一次", bannedUse)
		}
	}
}

func Test减弱动效显式点名到新增的那几条(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	i := strings.Index(css, "@media (prefers-reduced-motion:reduce)")
	if i < 0 {
		t.Fatal("app.css 里没有 prefers-reduced-motion 那一块")
	}
	// 这一块到**下一个 @media 或文件尾**为止（后面还跟着别的查询，不属于它）。
	block := css[i:]
	if j := strings.Index(block[1:], "@media"); j >= 0 {
		block = block[:j+1]
	}
	for _, want := range []string{"li.is-new{animation:none}", "li.is-new::after{display:none}"} {
		if !strings.Contains(block, want) {
			t.Errorf("减弱动效那一块没有 %q：`*{transition:none}` 只管过渡、管不到 animation，"+
				"而 li.is-new 的权重高于通配，不点名就关不掉", want)
		}
	}
	// 「改完回来的那一页不重播整页入场」也属于减弱动效的同类需求：它靠
	// data-restore 这个标记掀，不是靠媒体查询——所以钉的是那条规则本身在。
	if !strings.Contains(css, "html[data-restore] .card") {
		t.Error("没有 data-restore 那一条：改完之后的重载会重播整页入场，" +
			"真正变的那一条淹没在十二张一起上浮里")
	}
}

func Test排序就地完成而不是整页重载(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	// 两处落点（拖到某一行的上/下半、拖到组标题）都在**同一个委托的 drop** 里
	// （2026-10-16 从"每行一个 + 每组标题一个"合过来），所以这里钉的是那一个
	// 处理器：两种落点都要就地移动、都不许重载、都要在 await 之前取 FLIP 快照。
	drop := between(js, "dragRoot.addEventListener('drop', async (e) =>", "\n  });")
	if drop == "" {
		t.Fatal("app.js 里找不到那个委托的 drop：排序的落点处理没了")
	}
	if n := strings.Count(drop, "moveInPlace("); n != 2 {
		t.Errorf("委托的 drop 里 moveInPlace 出现 %d 次，要 2 次（拖到某一行 + 拖到组标题）："+
			"少一支就是那种落点松手之后整页闪一次", n)
	}
	if strings.Contains(drop, "reloadKeepingPlace(") {
		t.Error("委托的 drop 里还有重载：松手之后整页闪一次，那一下里没有「这一行挪了」的读法")
	}
	// 两种 op 的形状也一并钉住：拖到行上给 index（插前/插后由指针那一半决定），
	// 拖到组标题上**刻意不给** index（服务端缺席即追加到末尾）。
	for _, want := range []string{
		"to: dst, index }], 'app.ops.move'",
		"to: dst }], 'app.ops.move'",
	} {
		if !strings.Contains(drop, want) {
			t.Errorf("委托的 drop 里少了 %q：队尾那条路自己再算一遍下标正是那个 bug 的来路", want)
		}
	}
	// FLIP 的"旧位置"必须在 await **之前**取：dragend 在 drop 之后立刻触发，
	// 等 applyOps 回来时 dragBefore 已经被清成 null（位置对、动画没有）。
	if strings.Index(drop, "const before = dragBefore;") < 0 {
		t.Error("委托的 drop 没有在 await 之前取 FLIP 快照")
	} else if strings.Index(drop, "const before = dragBefore;") > strings.Index(drop, "await applyOps") {
		t.Error("委托的 drop 在 await 之后才取快照：那时 dragend 已经清掉了它")
	}

	// 跨组时要改两处计数，否则屏上说的数与盘上存的不一样。
	mi := between(js, "function moveInPlace(", "\n  }")
	for _, want := range []string{"bumpCount(dragSrc.gid, -1)", "bumpCount(dstGid, 1)", "flip(before)"} {
		if !strings.Contains(mi, want) {
			t.Errorf("moveInPlace 缺 %q\n%.400s", want, mi)
		}
	}
	// **要挪的是被拖的那一条**：查 li 时走 `listOf(源组)` 那一支。落点那行只负责算
	// index；传错它的话，屏上"把落点那行摘下来再按同一下标插回去"= 什么都没发生，
	// 而服务端挪的是被拖的那条——症状是**屏上与盘上分家**，且屏上看不出任何异常。
	if !strings.Contains(mi, "const li = srcList && srcList.querySelector(") {
		t.Errorf("moveInPlace 没有把查 li 的范围限定在源组那一个列表里：\n%.400s", mi)
	}
	// 插进去的位置由 `itemsOf` 说了算，与落点那一份**同一判据**（`li[data-item]`）：
	// 一处用 `li[data-item]` 算下标、另一处用 `ul.children` 插，就是差一位的错，
	// 而屏上看不出来。
	if !strings.Contains(mi, "const sibs = itemsOf(ul);") || !strings.Contains(mi, "sibs[index] || null") {
		t.Errorf("moveInPlace 插入位置没有与落点那处共用 itemsOf 判据：\n%.500s", mi)
	}
	// 组容器**有两种形状**（正文是 section[data-gid] > ul，大卡带是 ul[data-gid] 自己）：
	// 写死 section 时大卡带永远算不出落点，于是每次都退回重载——服务端搬对了、屏上整页
	// 闪一次，而那恰好是页面最显眼的那一块。
	if !strings.Contains(mi, "listOf(dstGid)") {
		t.Errorf("moveInPlace 没有用 listOf 取目标列表：\n%.400s", mi)
	}
	// **算不出落点就退回重载**：宁可旧样子，不能错样子。
	if !strings.Contains(mi, "reloadKeepingPlace();") {
		t.Error("moveInPlace 没有退回重载的分支：找不到那一组/那一行时，" +
			"留在原地不写会让屏上与盘上对不上")
	}
}

// 提示的三种待遇（2026-10-16）。判据不在"这条消息重不重要"，在**它是不是一次结果**：
// 成了（'ok'）自己走，没成（'bad'）留住，中性的不动。
//
// 两条最容易被写反的：
//
//	· **失败不许自动消失**。§8.7「写失败一律显式报错，不静默吞」——一条自己走掉的
//	  失败提示，等于把"刚才没存进去"变成"什么都没发生"。
//	· **正在读的句子不能被收走**。所以悬停与聚焦都要暂停倒计时：键盘用户没法
//	  "把指针移上去"，只挂 pointerenter 等于对这一半人无效。
func Test提示只在成功时自己走且会被读的人按住(t *testing.T) {
	jsRaw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsRaw)
	cssRaw, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssRaw)

	sb := between(js, "function showBanner(text, kind) {", "\n  }")
	if !strings.Contains(sb, "if (kind === 'ok') armBanner(p);") {
		t.Errorf("不是成功态才挂倒计时：\n%.400s", sb)
	}
	if !strings.Contains(sb, "kind === 'bad' ? ' bad'") || !strings.Contains(sb, "kind === 'ok' ? ' ok'") {
		t.Errorf("横幅没有按 kind 分成三张脸：\n%.400s", sb)
	}
	// 上一条还没走完的倒计时不能替这一条续命。
	if !strings.Contains(sb, "clearTimeout(bannerTimer)") {
		t.Error("新横幅没有清掉上一条的倒计时：连着两次操作时第一条的计时器还在跑")
	}

	arm := between(js, "function armBanner(p) {", "\n  }")
	for _, want := range []string{"pointerenter", "focusin", "pointerleave", "focusout"} {
		if !strings.Contains(arm, want) {
			t.Errorf("暂停/续上没有 %q：键盘用户没法把指针移上去，只挂 pointerenter 等于对一半人无效", want)
		}
	}
	// 它是浮着的：出现与消失都不许推正文。
	if !strings.Contains(css, "#lb-banner{position:fixed;") {
		t.Error("横幅还在文档流里：出现与消失会把正文整体推一下再拉回来（实测 main 的 top 变了 80px）")
	}
	// 那条会缩的细线是"几秒后消失"这句话本身——没有它，自动消失就是一次突袭。
	if !strings.Contains(css, "#lb-banner.ok::after") || !strings.Contains(css, "scaleX(var(--wait") {
		t.Error("成功态没有那条会缩的细线：倒计时看得见，才不叫突袭")
	}
	// 减弱动效：进场动效与细线关掉，**倒计时不关**。
	block := css[strings.Index(css, "@media (prefers-reduced-motion:reduce)"):]
	for _, want := range []string{"#lb-banner{animation:none}", "#lb-banner.ok::after{display:none}"} {
		if !strings.Contains(block, want) {
			t.Errorf("减弱动效那一块没有 %q", want)
		}
	}
}

// keyframeBlocks 取出 app.css 里每一个 @keyframes 的花括号内容。
func keyframeBlocks(css string) []string {
	var out []string
	rest := css
	for {
		i := strings.Index(rest, "@keyframes")
		if i < 0 {
			return out
		}
		rest = rest[i:]
		open := strings.Index(rest, "{")
		if open < 0 {
			return out
		}
		depth, close := 0, -1
		for j := open; j < len(rest); j++ {
			switch rest[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					close = j
				}
			}
			if close > 0 {
				break
			}
		}
		if close < 0 {
			return out
		}
		out = append(out, rest[open+1:close])
		rest = rest[close:]
	}
}
