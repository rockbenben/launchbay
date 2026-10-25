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

// 「确认导入」不许压在几十行技术报告之后（2026-10-25）：摘要已经给了会进哪些组、
// 少了哪几条；报告是排查材料，页面自己说“看不懂可以跳过”，就不该让唯一的出口
// 留在报告末尾。表单只许出现一次（令牌一次性，不要两个提交口）。
func TestImportPage_确认导入在完整报告之前(t *testing.T) {
	s := seeded(t)
	p, err := renderer(t, s).ImportPage(ImportView{
		Mode: "report", Count: 2, Groups: 2, Report: "report body", Token: "tok",
		PreviewGroups: []PreviewGroup{{Title: "Work", Count: 2}},
	}, i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)
	apply := strings.Index(body, `action="/import/apply"`)
	report := strings.Index(body, `class="rep"`)
	if apply < 0 || report < 0 {
		t.Fatalf("确认表单或报告不见了：apply=%d report=%d\n%.900s", apply, report, body)
	}
	if apply > report {
		t.Error("「确认导入」排在完整报告之后——摘要看完还要滚过整份报告才能确认")
	}
	if n := strings.Count(body, `action="/import/apply"`); n != 1 {
		t.Errorf("确认表单出现了 %d 次，要 1 次（令牌一次性，不要两个提交口）", n)
	}
}
