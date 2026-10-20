package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// loginAdmin 登一次管理权，返回那条 lb_admin cookie 的值（管理页那几条
// 零 JS 表单通路只认它）。
func loginAdmin(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := postForm(h, "/admin/login", "key="+testAdminKey, "")
	var v string
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieAdmin {
			v = c.Value
		}
	}
	if v == "" {
		t.Fatal("没拿到管理 cookie")
	}
	return auth.CookieAdmin + "=" + v
}

// 这一族是 2026-10-20 审计之后补的**行为证人**。
//
// 审计（三个方向各一遍）交回的最重一条：`POST /api/v1/profiles/self/pin` 与
// `POST /api/v1/profiles/pin` 都收一个**普通 string** 的 new，于是**"没给这个
// 字段"与"给了空串"分不开**——而这两件事的后果正好相反：前者是"脚本漏了字段"，
// 后者是"明确要求取消这道门"。从前它们同形，于是
//
//	POST /api/v1/profiles/self/pin {"old":"1234"}
//
// 把这一份的门拆了、答 200 + pinChanged:true。契约写着 new 必填，而
// AGENTS.md 那条「不要这道门必须是一个被说出来的决定」管的就是这件事。
//
// 下面几条钉的是**行为**，不是源码形状：实现怎么写都行，这几条不许变。

// 缺 new = 没填对，**不是**取消这道门。两条端点同一件事。
func Test改PIN_缺new不是取消这道门(t *testing.T) {
	s, h := setupBatch(t)
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))

	// 本人：只给 old。
	rec := post(h, "/api/v1/profiles/self/pin", `{"old":"1234"}`, cookie, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("self/pin 缺 new 要 422，实际 %d：%s", rec.Code, rec.Body)
	}
	if !fieldErrorsName(rec, "new") {
		t.Errorf("422 要点名 new：%s", rec.Body)
	}
	// 管理员：只给 profile。
	rec = post(h, "/api/v1/profiles/pin", `{"profile":"kid"}`, "", testAdminKey)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("profiles/pin 缺 new 要 422，实际 %d：%s", rec.Code, rec.Body)
	}
	if !fieldErrorsName(rec, "new") {
		t.Errorf("422 要点名 new：%s", rec.Body)
	}
	// **盘上那两道门都还在**——这是这一族存在的全部理由。
	for _, id := range []string{"dad", "kid"} {
		p, err := s.LoadProfile(id)
		if err != nil {
			t.Fatal(err)
		}
		if !auth.HasPin(p.PIN) {
			t.Errorf("%s 的门被拆了：缺一个字段不该等于取消 PIN", id)
		}
	}
	// 显式给空串 = 明确取消，那条路仍然是通的（两个方向都要有）。
	rec = post(h, "/api/v1/profiles/self/pin", `{"old":"1234","new":""}`, cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("显式给空串要 200（取消这道门），实际 %d：%s", rec.Code, rec.Body)
	}
	if p, _ := s.LoadProfile("dad"); auth.HasPin(p.PIN) {
		t.Error("显式给空串没有取消这道门")
	}
}

// **鉴权排在参数校验之前**：同一个"你没这个权限"的事实，答案不许随载荷变。
func Test改PIN_鉴权排在参数校验之前(t *testing.T) {
	_, h := setupBatch(t)
	cases := []struct {
		name string
		body string
		tok  string
		want int
	}{
		// 从前这两条答 422（"要 4-6 位数字" / "必须是 ASCII slug"）——
		// 等于给没资格问的人开了一个小 oracle：id 是不是 slug、PIN 格式对不对。
		{"无凭证 + 格式错的 PIN", `{"profile":"dad","new":"1"}`, "", http.StatusUnauthorized},
		{"无凭证 + 档案 id 不合法", `{"profile":"../etc","new":"5678"}`, "", http.StatusUnauthorized},
		// 档案作用域令牌：认出来了，但作用域不含"给档案设 PIN"这一档 → 403。
		{"档案令牌 + 格式错的 PIN", `{"profile":"dad","new":"1"}`, dadTok, http.StatusForbidden},
		{"档案令牌 + 档案 id 不合法", `{"profile":"../etc","new":"5678"}`, dadTok, http.StatusForbidden},
		// 有管理权的那把才轮到参数校验说话。
		{"管理员 + 格式错的 PIN", `{"profile":"dad","new":"1"}`, testAdminKey, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(h, "/api/v1/profiles/pin", tc.body, "", tc.tok)
			if rec.Code != tc.want {
				t.Errorf("要 %d，实际 %d：%s", tc.want, rec.Code, rec.Body)
			}
		})
	}
}

// 建档那两格**互斥**：勾了「不设 PIN」就把 PIN 那一格留空。两个都给不是
// "以勾选框为准"，是**没说清**——从前敲进去的 PIN 被静默丢掉，建出一份
// 不设防的档案，而那个人以为门锁上了。CLI 那边早就退 2（用法错误）。
func Test建档_PIN与不设PIN同给不是静默丢(t *testing.T) {
	s, h := setupBatch(t)

	// 管理页那张建档表单。**表单的形状是 303 + 一个键**（不是 422）——
	// 那一族的失败一律回带键的 303，键是 "pinboth"（见 setupFieldKey）。
	rec := postForm(h, "/admin", "title=客厅&pin=1234&nopass=1", loginAdmin(t, h))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("两个要给 303（带键），实际 %d：%s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin?err=pinboth" {
		t.Errorf("Location = %q，要 /admin?err=pinboth", loc)
	}
	// **没有建出任何一份档案**——一个静悄悄建出来的档案比一个错误更坏。
	if _, err := s.LoadProfile("home"); err == nil {
		t.Error("两个都给却把档案建出来了")
	}

	// API 那条同一件事。
	rec = post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"1234","nopass":true}`, "", testAdminKey)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("API 两个都给要 422，实际 %d：%s", rec.Code, rec.Body)
	}
	if !fieldErrorsName(rec, "nopass") {
		t.Errorf("422 要点名 nopass：%s", rec.Body)
	}
	if _, err := s.LoadProfile("mom"); err == nil {
		t.Error("两个都给却把档案建出来了")
	}
}

// pinChanged 这个字段名承诺"变了没有"，恒写 true 就是**这一格没有信息**。
func Test改PIN_pinChanged真算(t *testing.T) {
	s, h := setupBatch(t)
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))

	// 设上一个：变了。
	rec := post(h, "/api/v1/profiles/self/pin", `{"old":"1234","new":"4321"}`, cookie, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pinChanged": true`) {
		t.Fatalf("改了 PIN 要 200 + pinChanged:true，实际 %d：%s", rec.Code, rec.Body)
	}
	// 设成**同一个** PIN：有效的 PIN 没变。
	rec = post(h, "/api/v1/profiles/self/pin", `{"old":"4321","new":"4321"}`, cookie, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pinChanged": false`) {
		t.Fatalf("设成同一个 PIN 要 pinChanged:false，实际 %d：%s", rec.Code, rec.Body)
	}
	// 对一份**本来就没有 PIN**的档案取消：什么都没变。
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "tv", Title: "客厅"}); err != nil {
		t.Fatal(err)
	}
	rec = post(h, "/api/v1/profiles/pin", `{"profile":"tv","new":""}`, "", testAdminKey)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pinChanged": false`) {
		t.Fatalf("取消一道本来就没有的门要 pinChanged:false，实际 %d：%s", rec.Code, rec.Body)
	}
}

// 管理页那张表单：profile 是"改哪一份"的身份字段，同名多值要拒。
// 与 /admin/delete、/prefs、/setup 同一族纪律。
func Test管理页设PIN_profile同名多值要拒(t *testing.T) {
	s, h := setupBatch(t)
	adminCookie := loginAdmin(t, h)

	rec := postForm(h, "/admin/pin", "profile=kid&profile=dad&new=1357", adminCookie)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("同名多值要 422，实际 %d：%s", rec.Code, rec.Body)
	}
	if !fieldErrorsName(rec, "profile") {
		t.Errorf("422 要点名 profile：%s", rec.Body)
	}
	// 盘上两份的门都还在（没有"取第一个就改了"）。
	for _, id := range []string{"dad", "kid"} {
		p, err := s.LoadProfile(id)
		if err != nil {
			t.Fatal(err)
		}
		if !auth.HasPin(p.PIN) {
			t.Errorf("%s 的门被改了：同名多值不该取第一个", id)
		}
	}
}
