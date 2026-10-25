package render

import (
	"os"
	"strings"
	"testing"
)

// 「打开一条书签、什么都不改、按保存」不该产生一次改动（2026-10-24 实测）。
//
// 实拍到的形状：编辑模态把名字、说明、地址三格**永远发出去**，于是打开一条
// 按保存会写一版快照、rev +1，并在历史里留下"改名 / 改说明 / 改地址"三条
// 假动作；其中"说明"那一格还会把卡片上当主机名显示的 example.com 写进空说明。
// 分组那一族早就是"没改就不算一次改名"，这里补上同一份判据。
func TestEditSheet_没改的字段不发(t *testing.T) {
	js, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	for _, want := range []string{
		// 说明读盘上那份，不读卡片上可能只是主机名的 <i>。
		"const desc = li.dataset.desc || '';",
		// 三格各比一次：没改的不进 fields。
		"if (nextName !== name) fields.name = nextName;",
		"if (d.value !== desc) fields.desc = d.value;",
		"if (u.value.trim() !== url) fields.url = u.value.trim();",
		// 一条 op 都没有（也没挪组）就关掉，不发空批。
		"if (!ops.length) { closeEdit(); return; }",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("app.js 里少了 %q：编辑保存又把没改的字段发出去了", want)
		}
	}
}
