package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/store"
)

func Test档案与管理使用同一语言和主题(t *testing.T) {
	_, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	admin := adminOnlyCookie(t)
	if rec := postForm(h, "/prefs", "language=pt_BR&theme=light", dad); rec.Code != http.StatusSeeOther {
		t.Fatalf("保存档案偏好得到 %d：%s", rec.Code, rec.Body)
	}
	pages := []struct {
		path, cookie, key string
	}{
		{"/", dad, "prefs.open.label"},
		{"/prefs", dad, "prefs.heading"},
		{"/admin", dad, "admin.login.key"},
		{"/admin", dad + "; " + admin, "admin.lang.h2"},
		{"/admin/delete?profile=kid&err=badkey", dad + "; " + admin, "notice.badkey"},
	}
	cat := i18n.Load(i18n.Lang("pt_BR"))
	for _, page := range pages {
		rec := get(h, page.path, page.cookie)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<html lang="pt-BR" data-theme="light">`) ||
			!strings.Contains(rec.Body.String(), cat.T(page.key)) {
			t.Errorf("%s 没有同时使用当前档案的语言和主题（%d，文案 %s）", page.path, rec.Code, page.key)
		}
	}
	rec := get(h, "/admin", admin)
	if n := strings.Count(rec.Body.String(), "<option "); n != len(i18n.All)+1 {
		t.Errorf("管理页默认语言下拉有 %d 项，要 %d 项", n, len(i18n.All)+1)
	}
	if !regexp.MustCompile(`<html lang="[^"]+">`).MatchString(rec.Body.String()) {
		t.Error("无档案会话时管理页应该随系统主题")
	}
	// The rendered module URL identifies a public, immutable, single-language asset.
	script := regexp.MustCompile(`<script type="module" src="([^"]+)">`).FindStringSubmatch(get(h, "/", dad).Body.String())
	if len(script) != 2 || !strings.Contains(script[1], "?locale=pt_BR.") {
		t.Fatalf("主界面本地化脚本 URL 缺失：%v", script)
	}
	asset := get(h, script[1], "")
	if asset.Code != 200 || !strings.Contains(asset.Header().Get("Cache-Control"), "immutable") ||
		!strings.Contains(asset.Body.String(), cat.T("app.sheetTitle")) {
		t.Errorf("公开脚本没有返回葡萄牙语文案（%d）", asset.Code)
	}
	invalid := get(h, strings.Split(script[1], "?")[0]+"?locale=pt_BR.unknown", "")
	if invalid.Code != http.StatusNotFound || strings.Contains(invalid.Header().Get("Cache-Control"), "immutable") {
		t.Error("未知脚本版本应该返回不可长期缓存的 404")
	}
}

// 2026-10-20 改判：**语言是每一份档案自己的**，站级那一份只是**默认**。
//
// 从前语言整个住在站级，理由是"一家人共用一台导航台，谁也不该被别人改掉
// 阅读语言"。管理员变成一把机器级的 key 之后那条理由不成立了：管理员不属于
// 任何档案，而"这一份用哪种语言"恰恰是这一份自己的事。改完之后那条边界
// 反而更干净——**每一份只能改自己那一份**，管理员改的是默认。
//
// 这一族钉的是行为（哪一页说哪种语言），文案对不对由 i18n 包的守恒测试管。

// profileLang 读一份档案自己记的语言（空串 = 跟着默认）。
func profileLang(t *testing.T, s *store.Store, id string) string {
	t.Helper()
	p, err := s.LoadProfile(id)
	if err != nil {
		t.Fatalf("读 %s 的 profile.json: %v", id, err)
	}
	return p.Language
}

// 每一份档案念自己的语言；没自己选的那一份跟默认。
func Test语言_每一份档案念自己的语言(t *testing.T) {
	srv, h := setupServer(t)
	// 默认是中文（夹具里没设过语言）。
	if got := srv.deps.Store.Language(); got != i18n.Zh {
		t.Fatalf("前置状态不对：默认应当是中文，拿到 %q", got)
	}
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	kid := sessionCookie(t, loginAs(t, h, "kid", "5678"))

	// 给 dad 记一份英文：走**面板那一格**（POST /prefs 的 language），不是改盘。
	rec := postForm(h, "/prefs", "language=en", dad)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("改自己那一份的语言该 303，实际 %d %s", rec.Code, rec.Body)
	}
	if got := profileLang(t, srv.deps.Store, "dad"); got != "en" {
		t.Fatalf("dad 的语言没落盘：%q", got)
	}

	// dad 的档案页是英文，kid 的仍是中文（默认）。
	dadPage := get(h, "/", dad).Body.String()
	if !strings.Contains(dadPage, "Switch profile") {
		t.Errorf("dad 自己选了英文，他的档案页却不是英文的：\n%.300s", dadPage)
	}
	if strings.Contains(dadPage, "换档案") {
		t.Errorf("dad 的档案页上还有中文界面文案")
	}
	kidPage := get(h, "/", kid).Body.String()
	if !strings.Contains(kidPage, "换档案") {
		t.Errorf("kid 没自己选语言，应当跟默认（中文）：\n%.300s", kidPage)
	}
}

// 改自己那一份，**只改自己那一份**。别人的档案语言谁也动不了。
func Test语言_改自己那一份不动别人(t *testing.T) {
	srv, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	postForm(h, "/prefs", "language=en", dad)
	if got := profileLang(t, srv.deps.Store, "kid"); got != "" {
		t.Errorf("dad 改了自己的语言，kid 的却变成了 %q", got)
	}
}

// 空串 = **跟随默认**：管理员改了默认，这一份就跟着改。
//
// 这一格是"每一份自己记"与"管理员那一份是默认"两句话的交点——没有它，
// "默认"就只是管理员自己看得见的一个设置。
func Test语言_跟随默认的那一份跟着管理员走(t *testing.T) {
	srv, h := setupServer(t)
	admin := adminOnlyCookie(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	kid := sessionCookie(t, loginAs(t, h, "kid", "5678"))

	// kid 跟着默认：管理员把默认改成英文，kid 的档案页就变英文。
	if rec := postLangForm(t, h, "/admin/language", "language=en", admin, ""); rec.Code != http.StatusSeeOther {
		t.Fatalf("改默认该 303，实际 %d %s", rec.Code, rec.Body)
	}
	kidPage := get(h, "/", kid).Body.String()
	if !strings.Contains(kidPage, "Switch profile") {
		t.Errorf("默认改成英文后，跟随默认的那一份没跟着变：\n%.300s", kidPage)
	}
	// dad 自己选了中文（他改的是自己那一份），默认变英文与他无关。
	postForm(h, "/prefs", "language=zh", dad)
	dadPage := get(h, "/", dad).Body.String()
	if !strings.Contains(dadPage, "换档案") {
		t.Errorf("自己选了中文的那一份被管理员的默认带走了：\n%.300s", dadPage)
	}
	// 空串 = 回到跟随默认：把 dad 的那一份清掉，他重新跟着默认走。
	postForm(h, "/prefs", "language=", dad)
	if got := profileLang(t, srv.deps.Store, "dad"); got != "" {
		t.Errorf("清掉之后 dad 应当是「跟随默认」（空串），实际 %q", got)
	}
	dadPage = get(h, "/", dad).Body.String()
	if !strings.Contains(dadPage, "Switch profile") {
		t.Errorf("回到跟随默认之后，dad 的页面没有跟着默认走：\n%.300s", dadPage)
	}
}

// 写侧严：不认识的值 422 点名 language，不把垃圾写进盘上。
//
// 读侧宽是另一条纪律（i18n.ProfileLang 把不认识的值落默认），两处合起来是
// 那个形状：**手改 profile.json 写坏这一格，最多让那个人看到默认语言，而不会
// 把整份档案判成坏档**（那是让人再也进不去的代价）。
func Test语言_不认识的值被拒(t *testing.T) {
	srv, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	rec := postForm(h, "/prefs", "language=xx", dad)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("不认识的语言该 422，实际 %d %s", rec.Code, rec.Body)
	}
	if !fieldErrorsName(rec, "language") {
		t.Errorf("422 要点名 language 字段，实际 %s", rec.Body)
	}
	if got := profileLang(t, srv.deps.Store, "dad"); got != "" {
		t.Errorf("被拒的值不该落盘：%q", got)
	}
}

func Test语言_面板下拉列出启用的语言并预选当前档(t *testing.T) {
	_, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	for _, current := range []string{"fr", "pt_BR", ""} {
		rec := postForm(h, "/prefs", "language="+current, dad)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("保存语言 %q 得到 %d", current, rec.Code)
		}
		page := get(h, "/prefs", dad).Body.String()
		_, menu, found := strings.Cut(page, `<select class="fld" id="lb-prefs-language" name="language"`)
		menu, _, closed := strings.Cut(menu, "</select>")
		if !found || !closed {
			t.Fatal("设置面板没有界面语言下拉")
		}
		if count := strings.Count(menu, "<option "); count != len(i18n.All)+1 {
			t.Errorf("语言下拉有 %d 项，要 %d 项（含跟随默认）", count, len(i18n.All)+1)
		}
		if !strings.Contains(menu, `<option value=""`) {
			t.Error("语言下拉缺跟随默认")
		}
		for _, lang := range i18n.All {
			option := `<option value="` + string(lang) + `"`
			if string(lang) == current {
				option += " selected"
			}
			if !strings.Contains(menu, option+">"+lang.DisplayName()+"</option>") {
				t.Errorf("语言下拉缺少正确的 %s 选项", lang)
			}
		}
		if !strings.Contains(menu, `<option value="`+current+`" selected>`) || strings.Count(menu, " selected>") != 1 {
			t.Errorf("语言下拉应仅预选 %q", current)
		}
	}
}

// 改动历史与回退这两页也是**那一份**的语言：它们的话是 httpapi 侧组装的
// （带运行时值的那几句），而模板的 chrome 是渲染层按档案语言查的——
// 两半必须是同一种语言，否则一页上会混排多种语言。
func Test语言_历史页与回退页也是那一份的语言(t *testing.T) {
	_, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	postForm(h, "/prefs", "language=en", dad)

	hist := get(h, "/history", dad).Body.String()
	if !strings.Contains(hist, "Revert") && !strings.Contains(hist, "revert") {
		t.Errorf("改动历史页没有用这一份的语言：\n%.300s", hist)
	}
	if strings.Contains(hist, "改动历史") {
		t.Errorf("改动历史页上留着中文界面文案")
	}

	rev := get(h, "/revert", dad).Body.String()
	if strings.Contains(rev, "回退") {
		t.Errorf("回退页没有用这一份的语言：\n%.300s", rev)
	}
}
