package render

import (
	"strings"
	"testing"
)

// 「分组」那一节的**分节头**必须与另两节同一种形状。
//
// 原来这一节自己画头：`<div class="prefs-groups-head">` 是 `display:flex` +
// `align-items:baseline`，于是「分组」与它那句长说明**并排挤在同一行**上
// （1440 实拍：标签 13px、说明 12px、中间 10px）；另两节走 `h3.prefs-sec`——
// 13px 大写字、带分隔线、独占一行。同一屏里三个分节头长得像三种东西。
//
// 判据是"它是分节头"，所以它该与另两个分节头共用同一个类；分隔线也因此只有
// 一份（原先那一节自己又画了一条 `border-top`）。
func Test分组那节的分节头与另两节同形(t *testing.T) {
	body := string(mustPrefs(t, seeded(t), "dad"))

	if strings.Contains(body, "prefs-groups-head") {
		t.Error("分组那一节还在自己画分节头：它与另两节长得不一样，而分节线也因此有两份")
	}
	// 四个分节头都是 h3.prefs-sec——数出来正好四个（第四颗是 2026-10-20 加的「我的 PIN」）。
	var heads []string
	for _, chunk := range strings.Split(body, `<h3 class="prefs-sec">`)[1:] {
		heads = append(heads, chunk[:strings.Index(chunk, "</h3>")])
	}
	if len(heads) != 4 {
		t.Fatalf("面板里有 %d 个分节头，期望 4 个：%q", len(heads), heads)
	}
	if !strings.Contains(body, `<h3 class="prefs-sec">分组</h3>`) {
		t.Error("分组那一节没有用 h3.prefs-sec 当分节头")
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
		t.Error("prefs-sec 那一条被删了：三个分节头就都没有分隔线了")
	}
}
