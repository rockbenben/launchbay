package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

// 导入报告那一屏端到端的证人：报告文本是 markdown（CLI 与扩展也读它），
// 页面这一路以前**原样塞进 <pre>**，于是家里人读到 `## 结论`、`**没能进来**`、
// 成对的反引号，以及 html/template 顺手换出来的 &#34;。
// 这一条不测渲染器本身（那是 render/report_test.go），测的是"真上传一份导出文件
// 之后，页面上一个标记字符都不该剩"——它正是能抓住"明细行里那对空反引号"
// 那种形状的证据（源文本里一个不起眼的 ` “ ` 会让整行排版烂掉，单元测试的夹具
// 里没有那一条）。
func Test导入报告页面上不许残留markdown标记(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", store.EmptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	file := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
  <DT><H3>常用</H3>
  <DL><p>
    <DT><A HREF="https://www.google.com/">Google</A>
    <DT><A HREF="https://github.com/x">GitHub</A>
    <DT><A HREF="javascript:void(0)">一段脚本</A>
    <DT><A HREF="file:///D:/x.txt">本地文件</A>
  </DL><p>
</DL><p>
`)
	ct, body := uploadOf("bookmarks.html", file)
	rec := postImport(t, h, loginCookie(t, h, "dad", "1234"), ct, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("报告页该 200，得到 %d: %.300s", rec.Code, rec.Body)
	}
	page := rec.Body.String()
	if !strings.Contains(page, `<div class="rep">`) {
		t.Fatalf("报告没有走结构化渲染（还是 <pre> 那份 markdown？）:\n%.400s", page)
	}
	for _, banned := range []string{"## ", "**", "`", "&#34;"} {
		if strings.Contains(page, banned) {
			t.Errorf("页面上残留 %q——报告文本里的标记必须被渲染掉，不是被转义露出来", banned)
		}
	}
	for _, want := range []string{`<h3 class="rep-h">`, "<code>", "<li>", "没能进来"} {
		if !strings.Contains(page, want) {
			t.Errorf("报告里缺 %q（该有排版后的结构）", want)
		}
	}
	// 对账那两个数不许再打架：结论说"文件里 N 条"，对账说"该进的 M 条"。
	if strings.Contains(page, "条目数：源") {
		t.Error("对账第 1 条又写成「源 N」：那个数是过滤后的，与结论里的文件总数不是同一批")
	}
}
