package render

import (
	"strings"
	"testing"
)

// 原生取色器的边框**必须自己给**（2026-10-14 实拍后补）。
//
// 实拍症状：暗色设置面板上每个分组的最右端凭空多出一个**亮蓝方框**，8 个分组
// 8 个框。根因：`.prefs-color` 从前只写了 width/height/padding/cursor，而
// `<input type=color>` 的那一圈框是**浏览器画的**（UA 默认样式）——全站其它控件
// 的边框都是 `--line`（半透明白），唯独它由 UA 代劳，于是它是整行唯一亮蓝的东西。
//
// **这不是设计，是疏漏**：同一族里另外十颗色块（`.prefs-swatch`）都是
// 半透明底 + 1px `--line`，而它不是，一眼看过去不像同一族。
//
// **`background:none` 是承重的那一半**：原生那层是 `background-color` 画在色块
// **之外**的，只改 border 不清它，就会在 `--line` 之外再多一圈——把一个疏漏
// 修成两个。
func Test取色器的边框与背景都由我们自己给而不是浏览器给(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := stripCSSComments(string(raw))

	i := strings.Index(css, ".prefs .prefs-color{")
	if i < 0 {
		t.Fatal("base.css 里找不到 .prefs-color 规则")
	}
	rule := css[i:]
	if j := strings.Index(rule, "}"); j >= 0 {
		rule = rule[:j]
	}

	// 一：边框必须被显式接管，且**用 --line**（全站同一份），不是 UA 那个亮蓝。
	if !strings.Contains(rule, "border:1px solid var(--line)") {
		t.Errorf("取色器没有显式给边框：UA 那一圈会在暗色面板上亮成一个蓝方框\n实际那条：%s", rule)
	}
	// 二：**背景必须清掉**。UA 的那层 background-color 画在色块之外，
	// 只改 border 会变成两圈。
	if !strings.Contains(rule, "background:none") {
		t.Errorf("取色器没有清掉 UA 的背景色：那层画在色块之外，会多出一圈\n实际那条：%s", rule)
	}
	// 三：圆角与 .fld 同一档，而 44−2×(1+2)=40 正好是色块本身——
	// 尺寸与内衬这套算术是"与旁边十颗同尺寸同外框"这句话的依据，别让它悄悄变。
	if !strings.Contains(rule, "border-radius:9px") {
		t.Errorf("圆角不是 9px（与 .fld 同一档）：它会与同行的色块不像一族\n实际那条：%s", rule)
	}
	if !strings.Contains(rule, "width:44px") || !strings.Contains(rule, "height:44px") {
		t.Errorf("尺寸不是 44 见方：§7.5 的触摸地板与同行节奏都靠它\n实际那条：%s", rule)
	}
	if !strings.Contains(rule, "padding:2px") {
		t.Errorf("内衬不是 2px：44−2×(1+2)=40 正好是色块，改了就不是那个数了\n实际那条：%s", rule)
	}

	// 四：**状态那一维仍走 data-active 的环**。边框归全站、状态归这一格，
	// 两件事各归各处——把那一维也塞进 border-color 会让"生效中"与"这是控件"
	// 混成同一句话，而 `data-active` 是**例外标记**（盘上颜色不在色板表里
	// 才出现），满屏常亮就不是标记了。
	if !strings.Contains(css, ".prefs .prefs-color[data-active]{outline:2px solid var(--accent)") {
		t.Error("data-active 的环不见了：那一格「现在生效的是自定义色」就没有载体了")
	}
}
