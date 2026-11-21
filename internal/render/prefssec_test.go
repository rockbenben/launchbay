package render

import (
	"slices"
	"strings"
	"testing"
)

// 各节共用标题标记，让标题与下一行说明各自独占一行。
func Test设置面板的分节标题(t *testing.T) {
	body := string(mustPrefs(t, seeded(t), "dad"))

	if strings.Contains(body, "prefs-groups-head") {
		t.Error("分组那一节还在自己画分节头：它与另两节长得不一样，而分节线也因此有两份")
	}
	// 「这一页」分成三节，其余分类各保留一节。
	var heads []string
	for _, chunk := range strings.Split(body, `<h3 class="prefs-sec">`)[1:] {
		heads = append(heads, chunk[:strings.Index(chunk, "</h3>")])
	}
	want := []string{"通用", "外观", "打开书签", "分组", "备份与历史", "我的 PIN"}
	if !slices.Equal(heads, want) {
		t.Fatalf("面板分节标题 = %q，期望 %q", heads, want)
	}
	// 它那句说明从 `.hint` 换成 `.prefs-hint`：前者在 `.prefs-field` 里那条规则
	// 管的是"标签与控件之间"的那一句，这一节没有控件，它是一节的引子。
	if !strings.Contains(body, `class="prefs-hint"`) {
		t.Error("分组那节的说明不是 .prefs-hint")
	}
}

// 反向证人：分节线只有一份。base.css 里不许再有第二处给分节画 border-top。
func Test分节线只有一份(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	if n := strings.Count(css, ".prefs-groups-head"); n != 0 {
		t.Errorf("base.css 里还有 %d 处 .prefs-groups-head：那个 div 已经不存在了", n)
	}
	if !strings.Contains(css, ".prefs h3.prefs-sec{") {
		t.Error("缺少共用的 prefs-sec 标题样式")
	}
}
