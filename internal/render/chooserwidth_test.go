package render

import (
	"strings"
	"testing"
)

// 选择页那一族（选择页 / 引导建档 / 管理登录 / 坏档）在宽屏上**自己收窄**。
//
// 实拍量到的（1418 视口，改之前）：`ul.profiles` 排出**八栏各 161px** 而实际只有
// 两份档案 —— 卡片挤在左上角，右边 76% 空着；引导建档那张表单 1346px 宽、
// 输入框 1321px。而这两页是装机与进门的第一块屏幕，"第一眼看上去像坏了"的
// 代价最高。`main` 的 1600 是给档案页与管理页那种**内容本来就铺满**的页面定的。
//
// 这一族收成 760，于是同一视口下排三栏（194×3 + 14×2 ≤ 728，而 194×4 > 728）。
func Test选择页那一族自己收窄(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))

	rule := extractRule(css, ".chooser{")
	if rule == "" {
		t.Fatal("base.css 里找不到 .chooser")
	}
	if !strings.Contains(rule, "max-width:760px") {
		t.Errorf(".chooser 不是 760px：\n%s", rule)
	}
	// 建档表单再收一档——两格输入撑满 760 仍是两倍宽的框。
	setup := extractRule(css, ".chooser form.setup{")
	if !strings.Contains(setup, "max-width:460px") {
		t.Errorf("建档表单没收窄到 460px：\n%s", setup)
	}
	// 栏宽下限**只在这一族里抬**：≤700 的平板档在 150px 下排 2 栏 × 299px，
	// 那一档本来就宽、也好点，不该被顺手改掉。
	if !strings.Contains(css, ".chooser .profiles{grid-template-columns:repeat(auto-fill,minmax(180px,1fr))}") {
		t.Error("这一族里的卡片栏宽没抬到 180px：两份档案会缩在左上角")
	}
	// 反向证人：**全站那条不许被动**。档案页的磁贴密度是量过的（见 .tiles 那条
	// 190px 的注释），跟着收窄会把 41 条那一份的整页高度再拉长。
	if !strings.Contains(css, "main{max-width:1600px") {
		t.Error("main 的 1600 被改了：档案页与管理页那一族是照着它排的")
	}
}

// 四个页面模板都要挂上那个类——**类只写在 CSS 里而模板没挂**，是这类改动最常见
// 的失败形状：样式看着写好了，一页都没变。
func Test收窄那一族的四个模板都挂了chooser(t *testing.T) {
	for _, tc := range []struct {
		file string
		want string
	}{
		{"web/picker.tmpl", `<main role="main" class="chooser">`},
		{"web/adminlogin.tmpl", `<main role="main" class="chooser">`},
		{"web/broken.tmpl", `<main role="main" class="chooser">`},
	} {
		if got := readWeb(t, tc.file); !strings.Contains(got, tc.want) {
			t.Errorf("%s 的 main 没有挂 chooser（这一族里它要跟着收窄）", tc.file)
		}
	}
}

// 建档表单里「不设 PIN」那一格是**勾选框**，它的文字本来就不在控件下面。
//
// `.setup label` 把每一格都排成竖的（两格输入各带一行标签），于是勾选框与它的
// 文字也成了上下两截：1440 实拍方块在 y=418、文字在 y=457，中间 39px 什么都没有。
// 判据是"这一格是不是一个输入框与它的标签"——勾选框不是。
func Test建档的勾选框那格退回行向(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	rule := extractRule(css, ".setup label.chk{")
	if rule == "" {
		t.Fatal("base.css 里找不到 .setup label.chk")
	}
	for _, want := range []string{"flex-direction:row", "align-items:center"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".setup label.chk 少了 %q：\n%s", want, rule)
		}
	}
	// 正向证人：模板里那一格带着 .chk，两处对得上。
	if got := readWeb(t, "web/picker.tmpl"); !strings.Contains(got, `<label class="chk">`) {
		t.Error("picker.tmpl 里「不设 PIN」那一格没有 .chk")
	}
}
