package render

import (
	"os"
	"strings"
	"testing"
)

// 备份导入成功之后，浮层不能只换面板（2026-10-24）。
//
// 实拍：导入一份 theme=light 的设置备份，面板里「亮色」已经按下、顶上还说
// 「备份已导入」，可页面本身仍是暗的——theme/layout/view/页面名/语言要重载
// 才会画到这一页上。这里钉两条：成功（`.notice.ok`）要重载并带一次性横幅；
// 失败（普通 `.notice`）仍留在面板里说清。
func Test备份导入成功_浮层重载并带成功横幅(t *testing.T) {
	js, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	for _, want := range []string{
		"if (notice && notice.classList.contains('ok')) {",
		"sessionStorage.setItem(RESTORE_OK, '1')",
		"location.reload();",
		"if (sessionStorage.getItem(RESTORE_OK)) {",
		"T('app.backup.imported')",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("app.js 里少了 %q：备份导入成功的重载/成功横幅被拆掉了", want)
		}
	}
	// 服务端把那句话画在 `.prefs-notice` 里面（独立页靠这层与居中面板对齐）。
	// 浮层两处取的都是里面那个 `.notice`；取外面的包装层会让 `ok` 判据失效，
	// 取 `main > .notice` 则一个都取不到——两处都是"错话说过了而屏上没这句话"。
	if got := strings.Count(src, "doc.querySelector('.prefs-notice .notice')"); got != 2 {
		t.Errorf("浮层取提示的那两处有 %d 处用了正确的选择器，要 2 处", got)
	}
}
