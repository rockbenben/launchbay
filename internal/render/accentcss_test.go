package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// readWeb 读 embed 进来的静态资产。**测试文件里没有它**——这一族几处都要读
// base.css / profile.tmpl / app.js，写一份在这里而不是每处各抄一遍 err 处理。
func readWeb(t *testing.T, name string) string {
	t.Helper()
	raw, err := webFS.ReadFile(name)
	if err != nil {
		t.Fatalf("读 %s: %v", name, err)
	}
	return string(raw)
}

// 一条 hex 到不了标题色点上（2026-10-14 实拍）。症状是**改了颜色、标题不变**：
// 数据对了、`validatedAccent` 也放行了，而屏上那一格退回 `--accent` 兜底的蓝。
//
// **真因是模板里指错了东西**，不是"颜色值被判为不可信"：改之前写的是
// `style="--sec-accent:{{.}}"`——而在 `{{range .Groups}}` 里 `{{.}}` 是**整个
// groupView 结构体**，html/template 把结构体塞进 CSS 上下文就判为不可信，整个
// style 值变成 `ZgotmplZ`（连字面写的 `--d:0` 一起没了）。实测：字面部分由模板
// 自己写、插值只占一个值位置的形状，一个 `#e8823c` 是**过得去**的。
//
// 所以下面这条钉的不是"CSS 值要包一层"，而是**别再用 `{{.}}` 拿字段**——
// 那个写法在任何一行都可能是循环项或根，换一行就静默指错。仓库里栽过一次同族
// 的（图标名走 `data-icon`，那里的判据是 URL 属性）。
func Test分组色必须作为CSS写进style而不是普通字符串(t *testing.T) {
	tmpl := readWeb(t, "web/profile.tmpl")

	// 一：两个挂载点都用**包过的那一份**（AccentCSS）。它既是正确的读法，
	// 也是双保险：字段名写着 AccentCSS 而不是 Accent，指错的那一种错不可能再发生；
	// 而类型（template.CSS）会在将来判据改动时顶住——把裸 string 写进 CSS 上下文
	// 是编译器不管、运行时才炸的形状。
	for _, bad := range []string{
		`style="--sec-accent:{{.}};`,
		`style="--sec-accent:{{.Accent}}`,
	} {
		if strings.Contains(tmpl, bad) {
			t.Errorf("模板里还有 %q：那是改之前的写法（`{{.}}` 拿的是整个结构体，颜色到不了屏上）", bad)
		}
	}
	if !strings.Contains(tmpl, `style="--sec-accent:{{.AccentCSS}};--d:`) {
		t.Error("section 的 --sec-accent 没有用 AccentCSS")
	}

	// 二：**`{{with}}` 里点号已经换了**。那里面写 `.AccentCSS` 编译期就报
	// `can't evaluate field AccentCSS in type string`（实测过），所以必须先绑别名。
	if strings.Contains(tmpl, `{{with .Accent}}<i class="dot" style="--sec-accent:{{.AccentCSS}}"`) {
		t.Error("色点那一处在 {{with}} 里直接用点号：with 会把 . 换成那个字符串")
	}
	if !strings.Contains(tmpl, "{{$n := .}}") || !strings.Contains(tmpl, "--sec-accent:{{$n.AccentCSS}}") {
		t.Error("色点那一处没有先绑循环项的别名")
	}
}

// CSSAccent 包 template.CSS 是**跳过全部转义**——安全性完全来自上游那一份判据。
// 所以它必须先过 validatedAccent（只放行 #rgb/#rrggbb），而不是直接包原始值。
func TestCSSAccent先过判据再包(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"#e8823c", "#e8823c"},
		{"#ABC", "#abc"}, // 大小写归一（沿用 validatedAccent 那一份）
		{"", ""},
		{"red", ""},           // 不是 hex：model.ValidAccent 拒
		{"#gggggg", ""},       // 形状像而内容不是
		{"expression(1)", ""}, // 明确危险：绝不能原样进 style
		{"#fff;}body{display:none", ""},
	} {
		if got := string(CSSAccent(tc.in)); got != tc.want {
			t.Errorf("CSSAccent(%q) = %q，要 %q", tc.in, got, tc.want)
		}
	}
	// 判据只有一份：CSSAccent 自己走 validatedAccent，不另立一个正则。
	if !model.ValidAccent("") {
		t.Error("ValidAccent(\"\") 应当为真（空串 = 没设色）")
	}
}
