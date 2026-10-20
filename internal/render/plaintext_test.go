package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 语言包里除导入报告那一族以外都是**纯文本**：那些句子直接印在页面上，
// 于是 markdown 记号（`**` 与反引号）落到屏幕上就是两个星号与一对反引号——
// 不是加粗、不是代码。
//
// 2026-10-20 实测踩到两处，都是**印在给人看的一屏上**：面板「我的 PIN」那一句
// 顺手写了 `**没有门**`；导入页那句「javascript:」带着一对反引号。符号出现在
// 最不该出现它的地方，看起来就像机器坏了。
//
// **只有 `import.rep.*` 这一族走 markdown**（internal/bookimport 的 report.go 拼出
// 报告，`render.ReportHTML` 把它渲染成 HTML），所以豁免只有这一族——而它有
// 自己的测试守着（bookimport 那一族与 render.ReportHTML）。
func Test语言包里纯文本那几句不许带markdown记号(t *testing.T) {
	marks := []string{"**", "`"}
	for _, lang := range []i18n.Lang{i18n.Zh, i18n.En} {
		c := i18n.Load(lang)
		for _, k := range c.Keys() {
			if strings.HasPrefix(k, "import.rep.") {
				continue // 这一族是 markdown（见上）
			}
			// T 不带参数就是**原样那一句**（带占位符的会印成 %!s(MISSING)，
			// 而记号检查看的是有没有那两个符号，与参数无关）。
			s := c.T(k)
			for _, m := range marks {
				if strings.Contains(s, m) {
					t.Errorf("%s 的 %s 是纯文本，却带着 markdown 记号 %q——页面上会原样印出来:\n%s",
						lang, k, m, s)
				}
			}
		}
	}
}
