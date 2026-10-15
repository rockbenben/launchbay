package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 顶栏那排分组**不许横向滚动**（2026-10-14 实拍后改判）。
//
// 原来是 `overflow-x:auto`。实测 714 那一档（8 个分组）放不下就开始滚，
// 而滚动的后果有三条，最后一条是致命的：
//  1. 「后端」被截成「后」字，而它下面就是那一节本身；
//  2. 冒出一条灰色系统滚动条横贯整页，压在第一条磁贴上面；
//  3. **没有任何"右边还有"的提示** —— 没有渐隐、没有箭头，而那条滚动条
//     在 Windows 上只在悬停/滚动时现形，**触屏与平板上根本没有**。
//
// 隐藏掉一块主导航而没人知道它在那儿，等于让「某个分组找不到了」。
//
// WCAG 1.4.10（Reflow）要的是同一个方向：窄宽度下不该靠二维滚动才看得全。
func Test顶栏分组折行而不是横向滚(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))

	ul := extractRule(css, ".gnav.top ul{")
	if ul == "" {
		t.Fatal("base.css 里找不到 .gnav.top ul")
	}
	if !strings.Contains(ul, "flex-wrap:wrap") {
		t.Errorf(".gnav.top ul 没有折行：放不下时会横向滚动\n%s", ul)
	}
	// **overflow-x 必须不在场**：留着它（哪怕当前不触发）也会把这一层变成
	// 滚动容器 —— `overflow` 非 visible 会建立新的 BFC，内层一旦更长，
	// 父级静默裁掉。"它现在没触发"不是一条判据。
	if strings.Contains(ul, "overflow") {
		t.Errorf(".gnav.top ul 仍有 overflow：横滚被静默保留了\n%s", ul)
	}
	// 每颗键**不许缩**（`li` 仍 flex:0 0 auto）：它们是 44px 的触摸目标（§7.5），
	// 缩了就踩不准；折行的是容器排布，不是键本身。
	li := extractRule(css, ".gnav.top li{")
	if !strings.Contains(li, "flex:0 0 auto") {
		t.Errorf(".gnav.top li 不再是 flex:0 0 auto：折行时键会被压窄，44px 的触摸地板就没了\n%s", li)
	}

	// 反向证人曾经钉的是"**侧栏抽屉那条仍然横滚**"，理由是"它有把手提示自己是抽屉"。
	// 2026-10-15 全站取景把这条判据推翻，而推翻它的正是上面那三条本身：
	// 390 档抽屉**默认是开的**（`<details class="drawer" open>`），把手那一刻已经
	// 尽完职责——它说的是"这里有一列分组"，不是"你看到的这半列后面还有半列"。
	// 现场量到 8 个分组里 4 个（工具/播客/娱乐/后端）整颗在右缘之外，一颗都点不到，
	// 而触屏上那条滚动条不现形。WCAG 1.4.10 那一句话对这一支同样成立，
	// 甚至更成立：顶栏那一行至少是整幅宽，这一支只有 245px。
	// 抽屉与顶栏的**差别**留在排法上（一个带把手可收，一个常驻），不在"能不能看全"上。
	drawer := extractRule(css, ".gnav:not(.top) ul{")
	if drawer == "" {
		t.Fatal("base.css 里找不到 .gnav:not(.top) ul")
	}
	if !strings.Contains(drawer, "flex-wrap:wrap") {
		t.Errorf("≤700 的抽屉分组带没有折行：一半的分组在右缘之外\n%s", drawer)
	}
	if strings.Contains(drawer, "overflow") {
		t.Errorf("≤700 的抽屉分组带仍有 overflow：横滚被静默保留\n%s", drawer)
	}
	if li := extractRule(css, ".gnav:not(.top) li{"); !strings.Contains(li, "flex:0 0 auto") {
		t.Errorf("≤700 的抽屉分组键被允许缩窄：44px 的触摸地板要留在\n%s", li)
	}

	// 文案不许再教一个**已经不可能发生**的动作。
	for _, lang := range []i18n.Lang{i18n.Zh, i18n.En} {
		got := i18n.Load(lang).T("profile.layout.toTopbar")
		for _, gone := range []string{"左右滑", "scroll sideways", "scroll"} {
			if strings.Contains(strings.ToLower(got), strings.ToLower(gone)) {
				t.Errorf("%s 的 toTopbar 仍教人「%s」：那一层已经不会横向滚了", lang, gone)
			}
		}
	}
}
