package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 导入预览顶上那句「会导入到这些分组」过去把 `、` 与 `（）` 直接写在模板里，
// 英文页于是印成 `Work（3）、Personal（2）`（2026-10-24 审查抓到的）。
// 标点是文案的一部分：两种语言各用自己那一份，模板里不许再出现语言分支或
// 手写标点。长文件夹名也要能折，不能把这一页撑宽。
func TestImportPage_预览摘要的标点跟着页面语言(t *testing.T) {
	s := seeded(t)
	p, err := renderer(t, s).ImportPage(ImportView{
		Mode: "report", Count: 2, Groups: 2, Report: "report body", Token: "tok",
		PreviewGroups: []PreviewGroup{{Title: "Work", Count: 1}, {Title: "Personal", Count: 2}},
	}, i18n.En)
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)
	if !strings.Contains(body, "Work (1), Personal (2)") {
		t.Errorf("英文预览摘要没有用英文标点：\n%.500s", body)
	}
	if strings.Contains(body, "Work（1）") || strings.Contains(body, "Personal（2）") {
		t.Errorf("英文预览摘要仍带着中文括号：\n%.500s", body)
	}
	if !strings.Contains(body, `.preview-sum{overflow-wrap:anywhere}`) {
		t.Error("预览摘要没有折行判据：没有空格的长分组名会把整页撑宽")
	}
}
