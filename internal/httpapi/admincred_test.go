package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
)

// 2026-10-20 审计之后补的两条**行为证人**。
//
// 1. `credTokenOnly` 那一档说自己是「档案作用域令牌，**且不认会话**」——
//    而从前的 require 里没有这一支：带会话的请求会一路落到最后一行
//    `return sess.Active, …, true`。那一档的"不认会话"只由两个 import
//    handler 自己在调用 require **之前**多写了一遍 Authorize 撑着，而端点层
//    那一遍是纵深防御、不是判据：下一个人加一条 credTokenOnly 端点而没重复
//    那个前置检查，会话就静默通过——而这一族的全部意义是「档案从令牌作用域
//    推导、不接受申报」这条更强的性质。
//
// 2. 管理员那把 key 打 import/apply 时，从前答 401「令牌认不出来」——
//    而同一个两步流程的第一步答的是 403「这把钥匙只管账号」。两句话的出路
//    正好相反，调用方会去换令牌，而它没有令牌问题。

// credTokenOnly 那一档**自己**就不认会话。
func TestCredTokenOnly_那一档自己不认会话(t *testing.T) {
	srv, h := setupServer(t)
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))

	// 判据本身：require 问的就是这一档。
	req := httptest.NewRequest("GET", "/api/v1/import", nil)
	req.Header.Set("Cookie", cookie)
	if _, _, ok := srv.require(req, credTokenOnly, "dad"); ok {
		t.Error("require 对 credTokenOnly + 有会话放行了：那一档说它不认会话")
	}
	// 端点那一头：带会话 cookie 打那两条只认令牌的端点，一律 401。
	for _, p := range []string{"/api/v1/import", "/api/v1/import/apply"} {
		rec := post(h, p, "{}", cookie, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("会话 cookie 打 %s 要 401，实际 %d：%s", p, rec.Code, rec.Body)
		}
	}
	// 令牌那一半仍然通（别把这一档整个堵死）。
	req2 := httptest.NewRequest("GET", "/api/v1/import", nil)
	req2.Header.Set("Authorization", "Bearer "+dadTok)
	if _, _, ok := srv.require(req2, credTokenOnly, "dad"); !ok {
		t.Error("档案作用域令牌过不了 credTokenOnly：那一档只认令牌，不是只认会话")
	}
}

// 管理员那把 key 打 import 那两条，一律 403 credential-admin-token，
// 与第一步同形——**不是** 401「令牌认不出来」。
func Test管理员那把key打导入一律403而不是401(t *testing.T) {
	_, h := setupBatch(t)
	for _, p := range []string{"/api/v1/import", "/api/v1/import/apply"} {
		rec := post(h, p, "{}", "", testAdminKey)
		if rec.Code != http.StatusForbidden {
			t.Errorf("管理员 key 打 %s 要 403，实际 %d：%s", p, rec.Code, rec.Body)
			continue
		}
		if !strings.Contains(rec.Body.String(), "credential-admin-token") {
			t.Errorf("%s 的 403 要用 credential-admin-token（那句话的出路才对得上事实）：%s", p, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "credential-scope") {
			t.Errorf("%s 的 403 不该用 credential-scope：那句话说的是'你管的是别的档案'，与事实相反", p)
		}
	}
}

// `GET /admin` 认**两条管理员通路**：管理员那把 key 与 `lb_admin` cookie。
// 而**档案会话不算管理权**（登进 dad 也不等于能管这台机器）。
//
// 从前那一页只问 `hasAdminCookie`（只读 cookie），于是拿管理员 key 的人拿到的是
// **登录页**（200 + 一个表单）——形状上"成功"、内容却是错的，脚本会以为自己没
// 登录。而契约（AGENTS.md 端点表、openapi 的 security、代码注释）都说它认。
func Test管理页认两条管理员通路而不认档案会话(t *testing.T) {
	_, h := setupBatch(t)
	adminCookie := loginAdmin(t, h)

	// 管理 cookie → 管理页。
	body := adminPageGet(t, h, "/admin", adminCookie)
	if !strings.Contains(*body, "爸爸") {
		t.Errorf("带管理 cookie 该拿到管理页：\n%.300s", *body)
	}
	// 管理员那把 key → **同一张管理页**（不是登录页）。
	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员那把 key 打管理页要 200，实际 %d", rec.Code)
	}
	withKey := rec.Body.String()
	if strings.Contains(withKey, "进入管理") {
		t.Errorf("管理员那把 key 拿到的是登录页：\n%.300s", withKey)
	}
	if !strings.Contains(withKey, "爸爸") {
		t.Errorf("管理员那把 key 该拿到管理页：\n%.300s", withKey)
	}
	// **档案会话不算管理权**（这一条不能松）。
	body = adminPageGet(t, h, "/admin", auth.CookieProfile+"=dad")
	if !strings.Contains(*body, "进入管理") {
		t.Errorf("只有档案会话该拿到登录页：\n%.300s", *body)
	}
}

// 管理 cookie 与管理员令牌是**两件独立的事**（2026-10-12）：管理员可能同时登着
// 自己的档案在用书签。这一条钉住 lb_admin 永远进不了 credScoped。
func Test管理员cookie进不了档案作用域(t *testing.T) {
	srv, h := setupServer(t)
	adminCookie := loginAdmin(t, h)
	req := httptest.NewRequest("GET", "/api/v1/data?profile=dad", nil)
	req.Header.Set("Cookie", adminCookie)
	if _, _, ok := srv.require(req, credScoped, "dad"); ok {
		t.Error("管理员 cookie 过了 credScoped：管理员读不到任何人的书签")
	}
	// 而管理员那一族仍然是通的。
	req2 := httptest.NewRequest("GET", "/api/v1/status", nil)
	req2.Header.Set("Cookie", adminCookie)
	if _, _, ok := srv.require(req2, credAdmin, ""); !ok {
		t.Error("管理员 cookie 过不了 credAdmin")
	}
	_ = auth.CookieAdmin
}
