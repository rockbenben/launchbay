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

// 一条 hex 写进 `style="--sec-accent:…"` 时被 html/template 替换成 `ZgotmplZ`
// （2026-10-14 实拍）。症状是**改了颜色、标题不变**：数据对了、`validatedAccent`
// 也放行了，而屏上那一格色点退回 `--accent` 兜底的蓝。仓库里已经栽过一次同族
// 的（图标名走 `data-icon`），所以这一条钉的是"别再栽"。
func Test分组色必须作为CSS写进style而不是普通字符串(t *testing.T) {
	tmpl := readWeb(t, "web/profile.tmpl")

	// 一：两个挂载点都用**包过的那一份**（AccentCSS），不用裸字符串。
	// 裸字符串在 style 属性位置会被判为不可信——这是 html/template 的上下文转义，
	// 与值本身是不是颜色无关。
	for _, bad := range []string{
		`style="--sec-accent:{{.}};`,
		`style="--sec-accent:{{.Accent}}`,
	} {
		if strings.Contains(tmpl, bad) {
			t.Errorf("模板里还有 %q：它会被 html/template 替换成 ZgotmplZ（实测颜色到不了屏上）", bad)
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
