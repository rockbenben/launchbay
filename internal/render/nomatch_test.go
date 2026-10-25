package render

import (
	"strings"
	"testing"
)

// 搜索无结果那一句会把查询词原样印出来，而"贴一整条地址进来搜"是常事：
// 一个没有空格的长串不折行时，窄屏整页横向可滚（实测视口 486、scrollWidth
// 1593；390 档由第二道横扫钉住）。判据落在 `.nomatch` 那一格上——只在放不下时断行。
func TestProfilePage_超长搜索词不撑破版式(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	long := strings.Repeat("a", 200)
	p, err := renderer(t, s).ProfilePage(PageRequest{ID: "dad", Query: long})
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)
	if !strings.Contains(body, `<p class="nomatch">`) {
		t.Errorf("搜索无结果那一句没有 nomatch 类，长查询词会把整页撑宽:\n%.300s", body)
	}
	if !strings.Contains(body, `.nomatch{overflow-wrap:anywhere}`) {
		t.Error("页面内联的 CSS 里没有 .nomatch 的折行判据")
	}
}
