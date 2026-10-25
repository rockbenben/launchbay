package render

import (
	"os"
	"strings"
	"testing"
)

// 编辑一条书签时改得了图标（batch9 U7）。
//
// 起点是实测：编辑模态只有 名称/描述/地址 三格，而「＋ 添加」那一层有图标格——
// 于是"给一条已经在库里的书签换个图标"在界面上没有任何出口，能选图标的那一层
// 只管新增。导入报告里那句"想补图标，导完之后用 POST /api/v1/batch 逐条设 icon"
// 因此是真的，而那一屏的读者是一家人里的某一个人。
//
// 这条闸比的是形状：三样少一样就红。少任何一样，症状都一样"看起来有图标格，
// 但要么读不到当前值、要么按保存什么都不写"。
func TestEditSheet_hasIconFieldAndOnlyWritesItWhenTouched(t *testing.T) {
	js, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	for _, want := range []string{
		// 那一格本身，与它当前的值从 li[data-icon] 来。
		`id: 'lb-edit-icon'`,
		"fillIconChoices(ic, [], curIcon)",
		"li.dataset.iconName",
		// 没动过就不发：这是这一格唯一安全的写法——"不选图标"是空串，
		// 与"这一条本来没有图标"同值，靠非空判断会把图标清掉。2026-10-24
		// 起它并进"没改的字段不发"那一组（同一个保存动作，同一套判据）。
		"if (ic.value !== curIcon) fields.icon = ic.value;",
		// 候选图标仍要能现查（resolve 那一条通路复用添加那层的同一份判据）。
		"'/api/v1/resolve', { url: u.value.trim() || url, profile }",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("app.js 里少了 %q：编辑模态的图标那一格又变成装饰", want)
		}
	}
	tmpl, err := os.ReadFile("web/profile.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tmpl), `data-icon-name="{{.Icon}}"`) {
		t.Error(`profile.tmpl 的 li[data-item] 不带 data-icon-name：编辑那一格不知道"现在用的是哪一个"，打开就等于清空`)
	}
	// 反向对照：写成 data-icon 会被 Go 的模板当 URL 属性换掉（#ZgotmplZ），
	// 症状是"图标格打开了，里面写着一个谁都不认识的串"。这条禁的是那个名字。
	if strings.Contains(string(tmpl), `data-icon="`) {
		t.Error("profile.tmpl 用了 data-icon：html/template 认它是 URL 属性，值会变成 #ZgotmplZ")
	}
}
