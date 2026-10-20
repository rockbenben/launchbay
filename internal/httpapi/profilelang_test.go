package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/store"
)

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
	rec := postForm(h, "/prefs", "language=fr", dad)
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

// 面板上那一格**在场**，且按亮的是"现在这一档"。
//
// 形状由 render 那条测试钉（面板里每张 POST /prefs 表单恰好一个名字），
// 这一支钉的是"这一格画出来了，而且画的是对的那一档"。
func Test语言_面板上有那一格且按亮当前档(t *testing.T) {
	_, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	postForm(h, "/prefs", "language=en", dad)
	page := get(h, "/prefs", dad).Body.String()
	for _, want := range []string{`name="language"`, "Follow the default", "English"} {
		if !strings.Contains(page, want) {
			t.Errorf("面板上缺 %q", want)
		}
	}
	// 按亮的是"English"那一颗（这一份现在就是英文）——aria-pressed 说的是状态。
	if !strings.Contains(page, `name="language" value="en" aria-pressed="true"`) {
		t.Errorf("按亮的不是当前那一档（English）：\n%.400s", page)
	}
}

// 改动历史与回退这两页也是**那一份**的语言：它们的话是 httpapi 侧组装的
// （带运行时值的那几句），而模板的 chrome 是渲染层按档案语言查的——
// 两半必须是同一种语言，否则一页上中英混排。
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
