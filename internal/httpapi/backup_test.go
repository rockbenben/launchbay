package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 这一族是**整份的进出**：导出的是这一份的完整状态（书签 + 台面偏好），
// 导入吃的是自己导出的那种文件。
//
// 钉住的是**边界**而不是形状：备份里该有什么、该没有什么（门闩、档案 id、
// 背景）、两段各自缺省时的行为、以及"目标档案不空"那道闸。

// backupOf 打一次导出并把那份 JSON 读成一张表。
func backupOf(t *testing.T, h http.Handler, path, cookie string) map[string]any {
	t.Helper()
	rec := get(h, path, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("导出该 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	var pkg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &pkg); err != nil {
		t.Fatalf("导出的那份读不出来: %v", err)
	}
	return pkg
}

// getWithToken 打一个带 Bearer 的 GET（现有的 get() 只带 cookie）。
func getWithToken(h http.Handler, path, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Accept-Language", "zh")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// postBackupForm 走浏览器那条路（multipart 表单）。
func postBackupForm(t *testing.T, h http.Handler, cookie, fileBody string) *httptest.ResponseRecorder {
	t.Helper()
	return postBackupFormAuth(t, h, cookie, "", fileBody)
}

// postBackupFormAuth 是同一条路，但能带一枚 Bearer——浏览器那条路**不认令牌**，
// 那一支正是要钉的（令牌来了 → 303 回选择页，且不写盘）。
func postBackupFormAuth(t *testing.T, h http.Handler, cookie, bearer, fileBody string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("backup", "backup.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(fileBody)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/import/backup", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept-Language", "zh")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestBackup_导出的是这一份的完整状态(t *testing.T) {
	_, h := setupBatch(t)
	cookie := loginCookie(t, h, "dad", "1234")
	pkg := backupOf(t, h, "/api/v1/backup?profile=dad", cookie)
	if pkg["kind"] != "launchbay-backup" {
		t.Errorf("kind = %v，要 launchbay-backup", pkg["kind"])
	}
	if pkg["version"] != float64(1) {
		t.Errorf("version = %v，要 1", pkg["version"])
	}
	if pkg["profile"] != "dad" {
		t.Errorf("profile = %v，要 dad（它是这份备份从哪一份导出来的）", pkg["profile"])
	}
	settings, ok := pkg["settings"].(map[string]any)
	if !ok {
		t.Fatalf("settings 那一段不是一个对象: %#v", pkg["settings"])
	}
	// **六格全在**：少了哪一格，恢复到另一台机器上它就留在原样——于是"没选过"
	// 与"选了默认"在盘上分不开。
	for _, k := range []string{"theme", "layout", "view", "addrLevel", "pageTitle", "language"} {
		if _, has := settings[k]; !has {
			t.Errorf("settings 里没有 %s 那一格", k)
		}
	}
	marks, ok := pkg["bookmarks"].(map[string]any)
	if !ok {
		t.Fatalf("bookmarks 那一段不是一个对象: %#v", pkg["bookmarks"])
	}
	// **rev 不在里面**：它是并发判据本身，导入时由服务端分配。
	if _, has := marks["rev"]; has {
		t.Error("书签段里带着 rev——那是并发判据，不该由一份文件带着走")
	}
	groups, ok := marks["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("书签段该有 1 个分组，得到 %#v", marks["groups"])
	}
	g := groups[0].(map[string]any)
	if g["id"] != "nas" {
		t.Errorf("分组 id = %v，要 nas", g["id"])
	}
	items, ok := g["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("分组里该有 1 条书签，得到 %#v", g["items"])
	}
}

func TestBackup_门闩与身份不进备份(t *testing.T) {
	s, h := setup(t)
	cookie := loginCookie(t, h, "dad", "1234")
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.PIN == "" {
		t.Fatal("夹具这一份没有 PIN，这一族测试就没意义")
	}
	rec := get(h, "/api/v1/backup?profile=dad", cookie)
	body := rec.Body.String()
	if strings.Contains(body, p.PIN) {
		t.Error("备份里出现了 PIN 的哈希——门闩不进备份，导出它等于把钥匙印在纸上")
	}
	if strings.Contains(body, "\"pin\"") {
		t.Error("备份里有 pin 那一格")
	}
	// 背景图是这台机器上的一个**文件**，不是一串字符：备份里不该有它。
	if strings.Contains(body, "\"background\"") {
		t.Error("备份里带着 background——那是一个文件，不是一串字符")
	}
}

func TestBackup_只给本人(t *testing.T) {
	_, h := setupBatch(t)
	// 管理员令牌：只管账号，看不见任何人的书签 → 403（不是 401：那把 key 是好的）
	if rec := getWithToken(h, "/api/v1/backup?profile=dad", adminTok); rec.Code != http.StatusForbidden {
		t.Errorf("管理员令牌该 403，得到 %d: %s", rec.Code, rec.Body.String())
	}
	// 档案作用域令牌：作用域不含这一份 → 403
	if rec := getWithToken(h, "/api/v1/backup?profile=dad", kidTok); rec.Code != http.StatusForbidden {
		t.Errorf("kid 的令牌读 dad 该 403，得到 %d: %s", rec.Code, rec.Body.String())
	}
	// 什么凭证都没带 → 401
	if rec := getWithToken(h, "/api/v1/backup?profile=dad", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("没带凭证该 401，得到 %d: %s", rec.Code, rec.Body.String())
	}
	// 缺 ?profile= → 400（"读哪一份"没有默认值）
	if rec := getWithToken(h, "/api/v1/backup", dadTok); rec.Code != http.StatusBadRequest {
		t.Errorf("缺 ?profile= 该 400，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

// 面板上那两格（导出一份 / 导入一份）得真的在页面上。
//
// **判的是"链接里带的是这一份的 id"**：写死一个 id 的导出链接会把别人的东西
// 导出来给人看——而那条链接在模板里是 `{{.Profile.ID}}`，它要是没渲染出来，
// 页面会安静地给出一个空的 `?profile=`。
func TestBackup_面板上有导出链接与导入表单(t *testing.T) {
	_, h := setup(t)
	cookie := loginCookie(t, h, "dad", "1234")
	body := get(h, "/prefs", cookie).Body.String()
	if !strings.Contains(body, `href="/api/v1/backup?profile=dad"`) {
		t.Errorf("面板上该有导出这一份的链接（带这一份的 id）:\n%.1200s", body)
	}
	if !strings.Contains(body, `action="/import/backup?tab=out"`) {
		t.Error("面板上该有导入那张表单（带自己那一节的 ?tab=out，失败时人才不会被人拽走）")
	}
	// **表单必须显式写 enctype**：缺了它浏览器会按 urlencoded 发，服务端读到的是
	// 一个空的文件字段，而页面会说"没有选文件"。
	if !strings.Contains(body, `enctype="multipart/form-data"`) {
		t.Error("导入那张表单该写 enctype=multipart/form-data")
	}
	if !strings.Contains(body, `name="backup"`) {
		t.Error("导入那张表单的文件字段该叫 backup（服务端按这个名字取）")
	}
	// **它得用面板里选文件那一族的脸**（真 input 收起来、露一颗 .fld）——
	// 原生控件在暗色页上是一块浅灰，像另一个产品贴上来的零件。
	if !strings.Contains(body, `class="prefs-file"`) {
		t.Error("导入那张表单没有用 .prefs-file：原生控件会露出来")
	}
}

func TestRestore_只带设置的那份只改台面(t *testing.T) {
	s, h := setup(t)
	cookie := loginCookie(t, h, "dad", "1234")
	revBefore, snapsBefore := snapshotCount(t, s, "dad")
	body := `{"kind":"launchbay-backup","version":1,"profile":"dad","settings":{"theme":"light","pageTitle":"影音室"}}`
	rec := post(h, "/api/v1/restore", body, cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("导入该 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Profile  string   `json:"profile"`
		Rev      int64    `json:"rev"`
		Settings []string `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应读不出来: %v", err)
	}
	if out.Profile != "dad" {
		t.Errorf("profile = %q，要 dad", out.Profile)
	}
	if len(out.Settings) != 2 {
		t.Errorf("settings 该列出改了哪两格，得到 %v", out.Settings)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "light" || p.PageTitle != "影音室" {
		t.Errorf("盘上 theme=%q pageTitle=%q，要 light / 影音室", p.Theme, p.PageTitle)
	}
	// **没给的字段不碰**：只导了主题与页面名，排法不该被顺手归位。
	if p.Layout != "sidebar" || p.View != "" {
		t.Errorf("没给的字段被动了：layout=%q view=%q", p.Layout, p.View)
	}
	// **台面偏好不产生 rev、不做快照**：它不是书签数据（与 /prefs 同一纪律），
	// 所以它进不了「回退」——改错了再按一次键就是。
	revAfter, snapsAfter := snapshotCount(t, s, "dad")
	if revAfter != revBefore || snapsAfter != snapsBefore {
		t.Errorf("改台面偏好不该动书签：rev %d→%d，快照 %d→%d", revBefore, revAfter, snapsBefore, snapsAfter)
	}
}

func TestRestore_带书签的那份写进去并留下快照(t *testing.T) {
	s, h := setupBatch(t)
	// 先把 kid 清空——导入要一个空的档案（下面那条另有一支钉"不空时拒"）。
	if err := s.SaveBookmarks("kid", emptyBookmarks()); err != nil {
		t.Fatal(err)
	}
	dadCookie := loginCookie(t, h, "dad", "1234")
	pkg := backupOf(t, h, "/api/v1/backup?profile=dad", dadCookie)
	raw, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	revBefore, snapsBefore := snapshotCount(t, s, "kid")
	// 导进 kid：目标**从令牌作用域推导**（kid 的令牌只能写 kid），体里那个
	// profile 只是来路说明。
	rec := post(h, "/api/v1/restore", string(raw), "", kidTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("导入该 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	after, err := s.LoadBookmarks("kid")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Groups) != 1 || after.Groups[0].ID != "nas" {
		t.Fatalf("kid 的分组该是备份里那一个，得到 %#v", after.Groups)
	}
	if len(after.Groups[0].Items) != 1 || after.Groups[0].Items[0].ID != "dsm" {
		t.Errorf("条目没跟着过来：%#v", after.Groups[0].Items)
	}
	// **走的是书签写通路**：快照与 rev 一道不少（否则这一份就回不去了）。
	revAfter, snapsAfter := snapshotCount(t, s, "kid")
	if revAfter <= revBefore {
		t.Errorf("导入书签该产生新 rev：%d → %d", revBefore, revAfter)
	}
	if snapsAfter <= snapsBefore {
		t.Errorf("导入书签该留一份快照：%d → %d", snapsBefore, snapsAfter)
	}
}

func TestRestore_目标不空时拒且不写盘(t *testing.T) {
	s, h := setupBatch(t)
	cookie := loginCookie(t, h, "dad", "1234")
	before, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"kind":"launchbay-backup","version":1,"bookmarks":{"groups":[{"id":"x","title":"X","layout":"tiles","items":[]}]}}`
	rec := post(h, "/api/v1/restore", body, cookie, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("目标不空该 422，得到 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "import-target-not-empty") {
		t.Errorf("该说清是「目标档案不空」: %s", rec.Body.String())
	}
	after, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if after.Rev != before.Rev || len(after.Groups) != len(before.Groups) {
		t.Errorf("被拒的那一次不该写盘：rev %d→%d，分组 %d→%d",
			before.Rev, after.Rev, len(before.Groups), len(after.Groups))
	}
}

func TestRestore_不是这份格式的文件被拒(t *testing.T) {
	s, h := setup(t)
	cookie := loginCookie(t, h, "dad", "1234")
	for _, body := range []string{
		`{"kind":"something-else","version":1}`,
		`{"kind":"launchbay-backup","version":99}`,
		`not json at all`,
		`{"kind":"launchbay-backup","version":1}`,
	} {
		rec := post(h, "/api/v1/restore", body, cookie, "")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("这份该被拒（422），得到 %d: %s", rec.Code, rec.Body.String())
		}
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "auto" {
		t.Errorf("被拒的那几次不该写盘，盘上 theme = %q", p.Theme)
	}
}

func TestRestore_设置里有一格不认就在写任何东西之前拒(t *testing.T) {
	s, h := setup(t)
	cookie := loginCookie(t, h, "dad", "1234")
	// 带书签段 + 一格不认的主题：书签那一段**也不许写**——半份成功比全败难收拾。
	body := `{"kind":"launchbay-backup","version":1,` +
		`"settings":{"theme":"不是一档主题"},` +
		`"bookmarks":{"groups":[{"id":"x","title":"X","layout":"tiles","items":[]}]}}`
	rec := post(h, "/api/v1/restore", body, cookie, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("不认的那一格该 422，得到 %d: %s", rec.Code, rec.Body.String())
	}
	after, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Groups) != 0 {
		t.Errorf("设置没过就不该写书签：盘上有 %d 个分组", len(after.Groups))
	}
}

func TestRestore_管理员令牌与别的档案进不来(t *testing.T) {
	_, h := setup(t)
	body := `{"kind":"launchbay-backup","version":1,"settings":{"theme":"light"}}`
	// 管理员那把 key 只管账号：403（它不是坏了，是层级不对）
	rec := post(h, "/api/v1/restore", body, "", adminTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("管理员令牌该 403，得到 %d: %s", rec.Code, rec.Body.String())
	}
	// 什么凭证都没带：401
	rec = post(h, "/api/v1/restore", body, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("没带凭证该 401，得到 %d: %s", rec.Code, rec.Body.String())
	}
	// 认不出来的令牌：401
	rec = post(h, "/api/v1/restore", body, "", "tok-不认识")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("认不出来的令牌该 401，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

// 浏览器那条路的行为证人：令牌一律 303 回选择页，且**不写盘**。
// openapi_test.go 的 tokensRefusedByForm 那一行靠它才立得住。
func TestRestoreForm_令牌一律303且不写盘(t *testing.T) {
	s, h := setup(t)
	rec := postBackupFormAuth(t, h, "", dadTok,
		`{"kind":"launchbay-backup","version":1,"settings":{"theme":"light"}}`)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("令牌该 303 回选择页，得到 %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q，要 /（档案选择页）", loc)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "auto" {
		t.Errorf("被拒的那一次竟然落盘了：theme = %q", p.Theme)
	}
}

func TestRestoreForm_会话能导入(t *testing.T) {
	s, h := setup(t)
	cookie := loginCookie(t, h, "dad", "1234")
	rec := postBackupForm(t, h, cookie, `{"kind":"launchbay-backup","version":1,"settings":{"theme":"light"}}`)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("导入该 303，得到 %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/prefs?tab=look&ok=imported" {
		t.Errorf("Location = %q，要 /prefs?tab=look&ok=imported（导入进来的设置就在那一节看得见，另带一句成功话）", loc)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "light" {
		t.Errorf("盘上 theme = %q，要 light", p.Theme)
	}
	// 跟到落点：这一页顶上要有一句成功话（2026-10-24）。
	// 没有它时，导入一份设置与盘上相同的备份，屏上什么都没变——按了没反应。
	page := get(h, "/prefs?tab=look&ok=imported", cookie)
	if page.Code != http.StatusOK {
		t.Fatalf("跟到 %q 该 200，得到 %d", "/prefs?tab=look&ok=imported", page.Code)
	}
	if !strings.Contains(page.Body.String(), "备份已导入") {
		t.Errorf("导入成功之后面板顶上没有那句成功话:\n%.600s", page.Body.String())
	}
	// 成功不是警告：`.notice.ok` 是蓝色那一档；橙色 `.notice` 留给失败。
	if !strings.Contains(page.Body.String(), `class="notice ok"`) {
		t.Errorf("成功话仍用警告色提醒框，读起来像出了点问题:\n%.600s", page.Body.String())
	}
	// 提示要落在 <main> 里、面板之前，并且与面板同一个宽度基准（2026-10-24）：
	// 从前它插在 body 之后就铺满整页左缘，而面板居中 900px。
	body := page.Body.String()
	mainAt := strings.Index(body, `<main`)
	noticeAt := strings.Index(body, `class="prefs-notice"`)
	panelAt := strings.Index(body, `<section class="prefs">`)
	if mainAt < 0 || noticeAt < mainAt || panelAt < noticeAt {
		t.Errorf("提示不在 main 里、或不在面板之前：main=%d notice=%d panel=%d", mainAt, noticeAt, panelAt)
	}
}

func TestRestoreForm_不是这份格式的文件在页面上说清(t *testing.T) {
	s, h := setup(t)
	cookie := loginCookie(t, h, "dad", "1234")
	rec := postBackupForm(t, h, cookie, `{"kind":"something-else","version":1}`)
	// 浏览器那条路**不吐 JSON 错误**：它把面板画回来，顶部说清这一次为什么没成。
	if rec.Code != http.StatusOK {
		t.Fatalf("没成也要把面板画回来（200），得到 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "不是导出的那种备份") {
		t.Errorf("页面上要说清为什么没成:\n%.600s", rec.Body.String())
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "auto" {
		t.Errorf("没成的那一次不该写盘，盘上 theme = %q", p.Theme)
	}
}
