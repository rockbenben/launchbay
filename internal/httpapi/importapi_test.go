package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/bookimport"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
)

// nbFile 是一份最小的、能被 parseNetbook 读懂的浏览器导出。
//
// 它照抄真实导出的骨架（DOCTYPE / TITLE / <DL><p> 嵌套），而不是一个
// "差不多"的简写——导入这条路的判据全在解析器里，用一个形状不同的夹具
// 测通，等于什么都没测。
const nbFile = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3>工具</H3>
    <DL><p>
        <DT><A HREF="http://10.0.0.5/" ADD_DATE="1">一个工具</A>
        <DT><A HREF="http://10.0.0.6/">另一个工具</A>
    </DL><p>
</DL><p>`

// postImport 造一次 multipart 上传。
func postImport(t *testing.T, h http.Handler, cookie, contentType string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/import", body)
	req.Header.Set("Content-Type", contentType)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func uploadOf(name string, body []byte) (string, io.Reader) {
	return uploadGo(name, body, "")
}

// uploadGo 在上传里**带上那颗按钮**（`go`）。选文件那张表上有两个 submit，
// 各带一个 value，服务端读的就是这个字段——所以"按了哪一颗"在测试里必须
// 真的以字段形式出现，否则这条分叉的测试会一直停在"什么按钮都没按"。
func uploadGo(name string, body []byte, goVal string) (string, io.Reader) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	// filename 必须带 .html：浏览器导出就是这个名，而 accept 也只放它。
	part, _ := mw.CreateFormFile("bookmarks", name)
	_, _ = part.Write(body)
	if goVal != "" {
		_ = mw.WriteField("go", goVal)
	}
	_ = mw.Close()
	return mw.FormDataContentType(), &buf
}

func loginCookie(t *testing.T, h http.Handler, id, pin string) string {
	t.Helper()
	return sessionCookie(t, loginAs(t, h, id, pin))
}

func TestImport_档案页空的时候出现入口(t *testing.T) {
	s, h := setupBatch(t)
	// setupBatch 给 dad 放了内容；先清空，让它回到"一条书签都没有"。
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/", loginCookie(t, h, "dad", "1234"))
	if !strings.Contains(rec.Body.String(), `href="/import"`) {
		t.Errorf("空档案的档案页该给一个导入入口:\n%.600s", rec.Body.String())
	}
}

func TestImport_有书签的档案页不给入口(t *testing.T) {
	_, h := setupBatch(t)
	body := get(h, "/", loginCookie(t, h, "dad", "1234")).Body.String()
	if strings.Contains(body, `href="/import"`) {
		t.Errorf("已经有书签的档案不该再提示导入:\n%.600s", body)
	}
}

func TestImport_没有会话去导入页只回选择页(t *testing.T) {
	_, h := setupBatch(t)
	rec := get(h, "/import", "")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("没会话该 303 回选择页，得到 %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestImport_上传只给报告不写盘(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	before, _ := s.LoadBookmarks("dad")

	ct, body := uploadOf("bookmarks.html", []byte(nbFile))
	rec := postImport(t, h, cookie, ct, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("上传该 200，得到 %d %s", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	if !strings.Contains(out, "确认导入") || !strings.Contains(out, `name="token"`) {
		t.Errorf("该给一张报告加确认表单:\n%.900s", out)
	}
	after, _ := s.LoadBookmarks("dad")
	if after.Rev != before.Rev {
		t.Errorf("第二步把 rev 推进了：%d → %d——看报告不许写盘", before.Rev, after.Rev)
	}
}

func TestImport_不是书签文件要说清而不是报成功(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	// Convert **不拒绝**非 Netscape 输入：喂一段普通 HTML 它解析出零条并照常返回。
	// 不判这一条，用户就会看到"导入成功 0 条"，像真的一样。
	ct, body := uploadOf("page.html", []byte(`<!doctype html><html><body>hi</body></html>`))
	rec := postImport(t, h, cookie, ct, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("该 200 带回选文件的一页，得到 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "没读出书签") {
		t.Errorf("该说清这个文件里没读出书签:\n%.600s", rec.Body.String())
	}
}

func TestImport_确认写入走的是batch那个落点(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	ct, body := uploadOf("bookmarks.html", []byte(nbFile))
	up := postImport(t, h, cookie, ct, body)
	token := hiddenValue(t, up.Body.String(), "token")

	form := strings.NewReader("token=" + token)
	req := httptest.NewRequest("POST", "/import/apply", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("确认写入该 303 回首页，得到 %d %s", rec.Code, rec.Body.String())
	}

	got, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, g := range got.Groups {
		n += len(g.Items)
	}
	if n != 2 {
		t.Errorf("导入后该有 2 条书签，实际 %d（分组 %d 个）", n, len(got.Groups))
	}
	if got.Rev == 0 {
		t.Error("rev 没有前进——落盘没走成")
	}
	// 审计线要留着：它是"谁动过这台机器"的唯一证据。
	hist, _ := s.ReadHistory("dad", 0)
	if len(hist) == 0 || hist[0].By != "import" {
		t.Errorf("审计线该记一条 by=import 的行，实际 %+v", hist)
	}
}

func TestImport_令牌只能用一次(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	ct, body := uploadOf("bookmarks.html", []byte(nbFile))
	token := hiddenValue(t, postImport(t, h, cookie, ct, body).Body.String(), "token")

	apply := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/import/apply", strings.NewReader("token="+token))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", cookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	apply()
	first, _ := s.LoadBookmarks("dad")
	apply() // 重放：令牌已删
	second, _ := s.LoadBookmarks("dad")
	if first.Rev != second.Rev {
		t.Errorf("重放同一枚令牌又写了一次：rev %d → %d", first.Rev, second.Rev)
	}
}

func TestImport_非空档案不许导(t *testing.T) {
	s, h := setupBatch(t) // dad 有两条书签
	cookie := loginCookie(t, h, "dad", "1234")
	before, _ := s.LoadBookmarks("dad")

	rec := get(h, "/import", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("非空档案访问导入页该 200（把话说清），得到 %d", rec.Code)
	}
	out := rec.Body.String()
	if strings.Contains(out, `type="file"`) {
		t.Errorf("非空档案不该还给上传表单:\n%.600s", out)
	}
	if !strings.Contains(out, "已经有") {
		t.Errorf("该说清为什么导不了:\n%.600s", out)
	}
	after, _ := s.LoadBookmarks("dad")
	if after.Rev != before.Rev {
		t.Errorf("非空档案那趟把 rev 推进了：%d → %d", before.Rev, after.Rev)
	}
}

func TestImport_超限的上传被拒(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	big := make([]byte, maxImportBytes+1024)
	copy(big, nbFile)
	ct, body := uploadOf("bookmarks.html", big)
	rec := postImport(t, h, cookie, ct, body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "8 MB") {
		t.Errorf("超限上传该被拒并说清上限，得到 %d:\n%.600s", rec.Code, rec.Body.String())
	}
}

func TestImport_算书签名与CLI是同一份(t *testing.T) {
	// bookimport.Ops 是从 package main 搬过来的：搬对了的证据是它生成的
	// ops 仍然全过 ops 的形状校验（decodeImportOps 里 DisallowUnknownFields
	// 就会兜住少写的字段）。这条钉的是"两边真的共用一份实现"。
	raw := []byte(nbFile)
	res, err := bookimport.Convert(raw, testIndex(), bookimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n := bookimport.CountItems(res.Bookmarks); n != 2 {
		t.Fatalf("夹具本身该读出 2 条，实际 %d", n)
	}
	if ops := bookimport.Ops(res.Bookmarks); len(ops) != 3 {
		t.Errorf("2 条书签 + 1 个分组该是 3 个 ops，实际 %d", len(ops))
	}
}

// nbMixedFile 是"真实导出掺了几条进不来的"那种形状：两条好书签 + 一条
// javascript: + 一条 data:。用户报的那次故障就是它——整份文件因为一条坏
// 地址进不去，于是收藏夹里另外两条也跟着丢了。
const nbMixedFile = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3>收藏夹</H3>
    <DL><p>
        <DT><A HREF="https://example.com/a" ADD_DATE="1">一条正常的书签</A>
        <DT><A HREF="javascript:void(0)">一个书签小工具</A>
        <DT><A HREF="https://example.com/b">另一条正常的书签</A>
        <DT><A HREF="data:text/html,hello">data 链接</A>
    </DL><p>
</DL><p>`

// 进不来的那条要被**跳过**，不是让整份文件进不来。
//
// 这条钉的是端到端那一步：报过"有 2 条过不了校验，整批没有写入"之后，
// 用户要看到的是"两条进来了，两条没进来（原因写着）"。写路径的整批拒绝
// 本身没动——那是给 AI 与 CLI 的纪律（§6.2），改的是导入这条路**先把坏条目
// 挑出去**。
func TestImport_进不来的条目被跳过而不是整批失败(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	ct, body := uploadOf("bookmarks.html", []byte(nbMixedFile))
	up := postImport(t, h, cookie, ct, body)
	if up.Code != http.StatusOK {
		t.Fatalf("上传该给报告页，得到 %d:\n%.600s", up.Code, up.Body.String())
	}
	page := up.Body.String()
	// 报告页要说清两件事：会进几条、没进几条。只说"会加 N 条"会让人以为
	// 文件里就这么多书签。
	// 2026-10-12：这两句从前是碎片拼的，钉的字面量里有 `<b>`；现在一句话一个键，
	// 数字走参数进句子内部。**整句**钉而不是"包含 2"——从前那条「、 3」的多余空格
	// 正是碎片接缝造出来的，只钉数字就抓不到它回来。
	if !strings.Contains(page, "会加 2 条书签、1 个分组。") {
		t.Errorf("该说清会加 2 条书签、1 个分组（并且顿号后不许有多余空格）：\n%.600s", page)
	}
	if !strings.Contains(page, "另有 2 条没能进来") {
		t.Errorf("该说清有 2 条没能进来：\n%.600s", page)
	}
	if !strings.Contains(page, "javascript:void(0)") {
		t.Errorf("报告里要逐条点名哪几条没进来：\n%.600s", page)
	}

	// 真正写入：成功回首页（不是"没写进去"那一页），盘上留下两条。
	token := hiddenValue(t, page, "token")
	rec := postImportApply(t, h, cookie, "token="+token)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("该 303 回首页，实际 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	got, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if n := countItemsOf(got); n != 2 {
		t.Errorf("盘上该有 2 条书签，实际 %d", n)
	}
}

// 一条都进不来是**另一个形状**："文件里没书签"会把人引去换一个同样导不进来
// 的文件（那不是文件选错了）。
func TestImport_全都进不来时说的是另一句话(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	const allBad = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>Bookmarks</TITLE>
<DL><p><DT><H3>小工具</H3><DL><p>
<DT><A HREF="javascript:void(0)">甲</A>
<DT><A HREF="javascript:void(1)">乙</A>
</DL><p></DL><p>`
	ct, body := uploadOf("bookmarks.html", []byte(allBad))
	rec := postImport(t, h, cookie, ct, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("该回到选文件那一步并说清原因，得到 %d", rec.Code)
	}
	out := rec.Body.String()
	if !strings.Contains(out, "没有一条能进来") {
		t.Errorf("该说清是「都没能进来」而不是「没读出书签」:\n%.600s", out)
	}
	if strings.Contains(out, "没读出书签") {
		t.Errorf("这句话会把人引去换文件：\n%.600s", out)
	}
}

// 令牌通路（扩展走的那条）也要说清跳过数：扩展只显示一行结论，
// 没有逐条明细，所以那个数就是它唯一能说的话。
func TestImportAPI_跳过数在响应里(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/v1/import",
		strings.NewReader(nbMixedFile))
	req.Header.Set("Content-Type", "text/html")
	req.Header.Set("Authorization", "Bearer "+dadTok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("令牌通路该给计划，得到 %d %.300s", rec.Code, rec.Body.String())
	}
	var plan struct {
		Count   int    `json:"count"`
		Skipped int    `json:"skipped"`
		Report  string `json:"report"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Count != 2 || plan.Skipped != 2 {
		t.Errorf("该是 2 条进、2 条跳过，实际 count=%d skipped=%d", plan.Count, plan.Skipped)
	}
}

// hiddenValue 从渲染好的 HTML 里取出某个 hidden input 的值。
func hiddenValue(t *testing.T, body, name string) string {
	t.Helper()
	marker := `name="` + name + `" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("页面里没有 hidden 的 %q:\n%.900s", name, body)
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, `"`)
	return rest[:j]
}

func emptyBookmarks() *model.Bookmarks {
	return &model.Bookmarks{Rev: 1, Title: "发射台", Groups: nil}
}

// postImportApply 走第三步那枚表单（零 JS 的 urlencoded）。
func postImportApply(t *testing.T, h http.Handler, cookie, form string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/import/apply", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func countItemsOf(b *model.Bookmarks) int {
	n := 0
	for _, g := range b.Groups {
		n += len(g.Items)
	}
	return n
}

// 长文件夹名（2026-10-12）：slug 化之后正好占满 31/32 字节的那一档。
//
// 这一档曾经让**整个导入失败**：条目 id 是"分组id-书签名"，预算算出来是 0 或
// 负，于是每一条都拼成 33/34 字节，而写路径按 model.ValidSlug 拒它——症状是
// 报告说"会加 N 条"，点下去却回一句"有 N 条过不了校验，整批没有写入"。
// 夹具必须照抄这个形状：一个 40 多字符的 ASCII 文件夹名会被 clipSlug 截到 32。
const nbLongFolderFile = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<DL><p>
    <DT><H3>My Very Long Development Tools Folder Name</H3>
    <DL><p>
        <DT><A HREF="http://10.0.0.5/">一个工具</A>
        <DT><A HREF="http://10.0.0.6/">另一个工具</A>
    </DL><p>
</DL><p>`

// 端到端地钉"长文件夹名不再整批被拒"：走真的上传 + 真的写入，不只是看 op。
func TestImport_长文件夹名不再整批被拒(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	ct, body := uploadGo("bookmarks.html", []byte(nbLongFolderFile), "direct")
	rec := postImport(t, h, cookie, ct, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("直接导入该给完成页，得到 %d:\n%.600s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "没写进去") {
		t.Errorf("长文件夹名不该再整批被拒:\n%.600s", rec.Body.String())
	}
	got, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if n := countItemsOf(got); n != 2 {
		t.Fatalf("两条都该进来，实际 %d 条:\n%+v", n, got.Groups)
	}
	for _, g := range got.Groups {
		for _, it := range g.Items {
			if !model.ValidSlug(it.ID) {
				t.Errorf("落盘的 id 不合法：%q", it.ID)
			}
		}
	}
}

// 「直接导入」那枚按钮：收下文件就写，并说清少了哪几条。
func TestImport_直接导入一步写进去并点名跳过的那几条(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	ct, body := uploadGo("bookmarks.html", []byte(nbMixedFile), "direct")
	rec := postImport(t, h, cookie, ct, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d:\n%.600s", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	// **不许是报告页**：这一路的意义就是没有报告，只有结果。
	if strings.Contains(out, `action="/import/apply"`) {
		t.Errorf("直接导入不该再停在报告页上等一次点头:\n%.600s", out)
	}
	if !strings.Contains(out, "导入完成") {
		t.Errorf("该说清写进去几条:\n%.600s", out)
	}
	// 少的是哪几条：逐条点名。只有一个数字的话，那几条就再也没人知道了。
	if !strings.Contains(out, "没能进来") {
		t.Errorf("有跳过的条目时要点名:\n%.600s", out)
	}
	got, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if n := countItemsOf(got); n != 2 {
		t.Fatalf("两条该真的写进盘上，实际 %d 条", n)
	}
}

// 存档前的那一屏要给两颗按钮：少了「直接导入」，想快点导完的人就只能去读报告。
func TestImport_选文件那一步给直接导入与预览两颗按钮(t *testing.T) {
	s, h := setupBatch(t)
	// 档案非空时 /import 渲染的是「不用导了」那一屏，压根没有这两颗按钮。
	if err := s.SaveBookmarks("dad", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	body := get(h, "/import", loginCookie(t, h, "dad", "1234")).Body.String()
	for _, want := range []string{`value="direct"`, `value="preview"`, "直接导入"} {
		if !strings.Contains(body, want) {
			t.Errorf("选文件页缺 %q:\n%.600s", want, body)
		}
	}
}

// 真写不进去时，那句话要**点名是哪一条**（2026-10-12）。
//
// 只给"N 条过不了校验"的话，用户手上只有一份他改不动的导出文件，而这个数字
// 告诉他"文件有问题"却没告诉他"哪一条"——于是下一次导入还是同一句话，他仍然
// 不知道该怎么办。判据是 ops.Run 报的 OpsIndex，翻译在 bookimport 那边与 Ops
// 同住一处。
//
// 这条用例**故意用一份能写进去的文件**：它钉的是那句话的形状（点名 + 理由），
// 而"整批被拒"那一支由 TestImport_长文件夹名不再整批被拒 的反面保证——真的整批
// 被拒时，这半条会自然失效（那一句压根印不出来）。所以这里走的是
// importOutcomeText 的单测口径：直接把一批 verrs 喂进去。
func TestImport_写入失败时点名是哪一条(t *testing.T) {
	srv, _ := setupServer(t)
	b := &model.Bookmarks{Groups: []model.Group{{
		ID: "g", Title: "工具", Layout: model.LayoutList,
		Items: []model.Item{{ID: "a", Name: "第一个", URL: "http://10.0.0.5/"}, {ID: "b", Name: "出问题的那个", URL: "http://10.0.0.6/"}},
	}}}
	// ops 的顺序：先全部建组（1 条），再按组加条目。所以 index 2 是第二个条目。
	out := batchOutcome{verrs: []ops.Error{{OpsIndex: 2, Field: "url", Reason: "id 不是 ASCII slug"}}}
	got := srv.importOutcomeText(out, b)
	for _, want := range []string{"出问题的那个", "http://10.0.0.6/", "id 不是 ASCII slug"} {
		if !strings.Contains(got, want) {
			t.Errorf("点名那句话里缺 %q：%s", want, got)
		}
	}
	// 没有文档可对照时退回旧句式——宁可少一句点名，也不要指错一条。
	if got := srv.importOutcomeText(out, nil); !strings.Contains(got, "过不了校验") ||
		strings.Contains(got, "出问题的那个") {
		t.Errorf("拿不到文档就该退回计数那句，实际：%s", got)
	}
}
