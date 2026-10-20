package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 2026-10-20 审计之后补的一族：**整条语言链在每一页上都成立**。
//
// 审计交回的三条（这一族把它们逐条钉住）：
//
//  1. 404 页与坏档页在**渲染层**自己问站级默认——于是全新机器 + 英文浏览器上，
//     它们是全站唯一两页中文的，而 404 页那颗按钮又是英文的（混排）。
//  2. Accept-Language 只取**第一个**标签且不剥 ";q="——"zh;q=0.9" 整条认不出，
//     一个说中文的客户端拿到英文首页。
//  3. 档案里写坏的语言值（"zh-CN" / "fr"）把这一份**静默钉死在中文**，
//     面板上三颗键还全都不亮。
//
// 文案对不对由 i18n 包的守恒测试管；这里钉的是**行为**。

// getLang 走浏览器那一支，但**语言由调用方给**（夹具里的 get 固定发 zh）。
func getLang(h http.Handler, path, cookie, lang string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", lang)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// 404 页与坏档页**跟其余每一页一样**走整条链：站级默认 → 浏览器 → en。
// 从前它们在渲染层自己问站级默认，于是全新机器上它们是全站唯一的中文页。
func Test语言_404页与坏档页也走整条链(t *testing.T) {
	srv, h := setupServer(t)
	// **没人设过默认语言**（把 settings.json 删掉）：于是链落到浏览器那一门。
	if err := os.Remove(srv.deps.Store.SettingsPath()); err != nil {
		t.Fatal(err)
	}
	// 404 页：英文浏览器 → 英文页；中文浏览器 → 中文页。
	// 断言里那两处 `&#39;` 是**转义后的撇号**（html/template 转的，屏上是 '）。
	rec := getLang(h, "/nope-not-here", "", "en")
	body := rec.Body.String()
	if !strings.Contains(body, "That page isn&#39;t here") {
		t.Errorf("英文浏览器该拿到英文的 404 页：\n%.300s", body)
	}
	if strings.Contains(body, "这一页不在这里") {
		t.Errorf("404 页上还留着中文：\n%.300s", body)
	}
	// 混排那一半：那颗"回我的页面"按钮必须与页面同语言。
	if strings.Contains(body, "回我的页面") {
		t.Errorf("404 页上那颗按钮与页面不同语言（混排）：\n%.300s", body)
	}
	rec = getLang(h, "/nope-not-here", "", "zh")
	if !strings.Contains(rec.Body.String(), "这一页不在这里") {
		t.Errorf("中文浏览器该拿到中文的 404 页：\n%.300s", rec.Body)
	}

	// 坏档页：同一件事。
	if err := os.WriteFile(srv.deps.Store.BookmarksPath("dad"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	rec = getLang(h, "/", cookie, "en")
	body = rec.Body.String()
	if !strings.Contains(body, "This profile&#39;s data file cannot be read.") {
		t.Errorf("英文浏览器该拿到英文的坏档页：\n%.300s", body)
	}
	if strings.Contains(body, "这个档案的数据文件读不了。") {
		t.Errorf("坏档页上还留着中文：\n%.300s", body)
	}
}

// 档案里写坏的语言值 = **没选过**：跟着链走，面板上亮「跟随默认」。
//
// 从前非法值被当成"显式选了中文"——既不跟站级默认，也不跟浏览器语言，
// 链的后两格被跳过；而面板上三颗键全都不亮（那一格的职责恰恰是显示当前档）。
func Test语言_档案里写坏的语言值当没选过(t *testing.T) {
	srv, h := setupServer(t)
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	// 盘上写坏这一格（手改/拷贝来的），绕过写侧的 422。
	p, err := srv.deps.Store.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	p.Language = "fr"
	if err := srv.deps.Store.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	// 站级默认是中文（夹具设过），所以这一份跟着默认 = 中文。
	if got := get(h, "/", cookie).Body.String(); !strings.Contains(got, "换档案") {
		t.Errorf("写坏的语言值不该把这一份钉死在别处，它应当跟默认：\n%.300s", got)
	}
	// **面板上要亮一颗键**：不认识的 = 没选过 = 亮「跟随默认」。
	panel := get(h, "/prefs", cookie).Body.String()
	if !strings.Contains(panel, `aria-pressed="true"`) {
		t.Errorf("台面面板上「界面语言」那一行一颗键都不亮：这一格的职责是显示当前档\n%.600s", panel)
	}
	if !strings.Contains(panel, "跟随默认") {
		t.Errorf("面板上没有「跟随默认」那一档：\n%.600s", panel)
	}
}

// Accept-Language 的**权重**要剥掉：`zh;q=0.9` 认得出 zh（从前整条认不出）。
func Test语言_AcceptLanguage带权重也认得出(t *testing.T) {
	srv, h := setupServer(t)
	if err := os.Remove(srv.deps.Store.SettingsPath()); err != nil {
		t.Fatal(err)
	}
	// 主链：说中文的客户端拿到中文首页（从前拿到英文）。
	rec := getLang(h, "/", "", "zh;q=0.9")
	if !strings.Contains(rec.Body.String(), "谁来用？") && !strings.Contains(rec.Body.String(), "爸爸") {
		t.Errorf("zh;q=0.9 的客户端该拿到中文页：\n%.300s", rec.Body)
	}
	// 站级默认一旦有人设过，浏览器那一格轮不到（链条的顺序不许反）。
	if err := srv.deps.Store.SetLanguage(i18n.En); err != nil {
		t.Fatal(err)
	}
	rec = getLang(h, "/", "", "zh;q=0.9")
	if !strings.Contains(rec.Body.String(), "Who is this for?") {
		t.Errorf("管理员设过默认之后，浏览器那一格不该抢在前面：\n%.300s", rec.Body)
	}
}

// `POST /lang` 是**浏览器表单通路**：带令牌来的是权限类拒绝，答 303 回选择页、
// **不带提示**。从前它回 `?err=permission`——而 permission 那一句是
// 「去把宿主数据目录交给跑它的那个用户」，那是**数据目录不可写**的出路，
// 与"这条路不认令牌"毫无关系。给错出路比不说更坏。
func Test语言_lang端点带令牌不指去改宿主目录(t *testing.T) {
	_, h := setupServer(t)
	rec := post(h, "/lang", "language=en", "", dadTok)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("带令牌要 303，实际 %d：%s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q，要 /（权限类拒绝不带提示）", loc)
	}
}
