package render

import (
	"strings"
	"testing"
)

// 管理页「这台机器」那一节的卡片要**上下两条边都齐**（2026-10-20 实拍后改）。
//
// 实拍量到的：右那张「书签栏小按钮」卡的上沿比左边低 18px、下沿又差着几十
// 像素——两条边都不平。三个原因各钉一处：
//   - `.adm.bm` 从前带 `margin:18px 0 0`：那 18px 是它 2026-10-16 之前**独占
//     一块**时与上一块之间的距离，进栅格之后它把右卡整体推低了；
//   - 全局那条 `section{margin:13px 0}`（原本给档案页分组用的）落在管理页
//     这两张 `<section>` 卡上，又推了 13px；
//   - `.adm-two` 用的是 `align-items:start`：两列各按内容高，下沿永远参差。
//
// 这条钉的是**样式里那几行**而不是像素——像素要浏览器，而这一层没有浏览器。
// 少任何一条，卡与卡的上沿/下沿就会差十几像素。
func TestAdminPage_机器那节的卡片上下沿都齐(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))

	// ① 右卡不再自带顶边距（间距归栅格的 gap 管）。
	if !strings.Contains(css, ".adm.bm{margin:0;") {
		t.Error("base.css 里 `.adm.bm` 又带上了自己的 margin：右卡会被推离左卡的上沿")
	}
	// ② 左栏那张卡不吃全局 `section{margin:13px 0}`。
	if !strings.Contains(css, ".adm-two .keycard{margin:0}") {
		t.Error("base.css 里 `.adm-two .keycard` 没有归零 margin：全局 section 的 13px 会落在它身上")
	}
	// ③ 两列一样高 → 下沿齐。这一格要**在 `.adm-two` 那条规则里**判，
	//    别在整个文件里搜 `align-items:stretch`（那一串在别处有七八处）。
	two := ruleBody(css, ".adm-two{")
	if two == "" {
		t.Fatal("base.css 里找不到 `.adm-two` 那条规则")
	}
	if !strings.Contains(two, "align-items:stretch") {
		t.Errorf("`.adm-two` 那一格该是 align-items:stretch（两列一样高，下沿才齐），实际：%s", two)
	}
	// 反向证人：它不许再是 start——那正是"两卡不平"的那一格。
	if strings.Contains(two, "align-items:start") {
		t.Errorf("`.adm-two` 又是 align-items:start：两列按内容高，下沿必然参差")
	}
}

// ruleBody 取一条 CSS 规则的正文（选择器之后、第一个 `}` 之前）。
//
// 只取**第一条**同名规则——样式表里后面的同名规则会覆盖前面的，而这一层要判的
// 是"生效的那一条"。找不到就回空串。
func ruleBody(css, selector string) string {
	i := strings.Index(css, selector)
	if i < 0 {
		return ""
	}
	rest := css[i+len(selector):]
	j := strings.Index(rest, "}")
	if j < 0 {
		return ""
	}
	return rest[:j]
}
