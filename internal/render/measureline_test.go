package render

import (
	"regexp"
	"strconv"
	"testing"
)

// 说明句的行长上限（2026-10-16 量的）。这一条值得证人，因为它的失效方式很安静：
// 选择器名单被改、或者有人把 max-width 删掉，页面**照样好看**，只是那一行又长回
// 100 多个字——而读的人不会报告这件事，他只会以为自己看漏了一行。
//
// 量出来的原值（1440 档，拿同字号现算字宽，不猜）：/history 112 字、/admin 112、
// /admin 104、书签栏那两段 101、/import 91、/prefs 75。CJK 舒服区间 35–45 字。
func Test说明句有一行长度上限(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	rule := extractRule(css, "main p,main .notice,main .hint,main .explain{")
	if rule == "" {
		t.Fatal("base.css 里找不到那条行长上限（选择器名单被改了？）")
	}
	m := regexp.MustCompile(`max-width:\s*(\d+(?:\.\d+)?)em`).FindStringSubmatch(rule)
	if m == nil {
		// 单位必须是 em：px 的话 12px 的说明与 15px 的横幅落在**不同字数**上，
		// 上限就成了"这一族多宽"而不是"这一行读几个字"。
		t.Fatalf("行长上限缺失或不是 em 单位：%s", rule)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	// 20em 会把句子切成竖条，80em 等于没写。
	if n < 30 || n > 60 {
		t.Errorf("行长上限 %dem 不在 30–60 那一档（CJK 舒服区间 35–45 字）", n)
	}
	// 表格那一格**不许被这条管**：td 实测 30–31 字，它要的是用满宽度放数据。
	if regexp.MustCompile(`(^|[,\s])td([,\s{]|$)`).MatchString(rule) {
		t.Errorf("行长上限伸进了 td：表格那一列该用满宽度\n%s", rule)
	}
}
