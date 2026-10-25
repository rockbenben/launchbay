package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// **那把机器级 key 只从请求体里读**（2026-10-23 修）。
//
// 理由：管理 cookie 存在的全部理由是"原始 key 从不离开那一次 POST"，而
// `r.FormValue` 是**查询串与表单体的合并值**——于是
// `POST /admin/delete?profile=kid&key=<那把 key>`（body 空）也算数，
// 而那把 key 于是进了 URL：浏览器历史、访问日志、Referer 头都留着它。
//
// 这一族两个入口都要守：**登录**那条与**删档**那条（删档是这台机器上唯一
// 不可逆的动作，最不该让它的钥匙走 URL）。
func Test管理密钥只从请求体读(t *testing.T) {
	s, h := setup(t)

	// 登录那条：key 走 URL → 不认（凭据认不出来 → 303 + ?err=badkey）。
	rec := postForm(h, "/admin/login?key="+testAdminKey, "", "")
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=badkey") {
		t.Fatalf("key 走 URL 不该登录成功，实际 %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// body 里给 → 认（回管理页，不带提示）。
	rec = postForm(h, "/admin/login", "key="+testAdminKey, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("body 里给 key 该 303 进管理页，实际 %d：%s", rec.Code, rec.Body)
	}

	// 删档那条：key 走 URL → 不删（档案还在）。
	// 请求体里**只带 profile**（那一格是从表单来的）；key 只在查询串里。
	admin := adminOnlyCookie(t)
	rec = postForm(h, "/admin/delete?key="+testAdminKey, "profile=kid", admin)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=badkey") {
		t.Fatalf("key 走 URL 不该删得掉，实际 %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatalf("kid 不该被删掉：%v", err)
	}
	// body 里给 → 删得掉。
	rec = postForm(h, "/admin/delete", "profile=kid&key="+testAdminKey, admin)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("body 里给 key 该 303，实际 %d %s", rec.Code, rec.Body)
	}
	if _, err := s.LoadProfile("kid"); err == nil {
		t.Error("kid 还在——body 里给了 key 却没删")
	}
}

// 同一族纪律：**同名多值要拒**——"没说要哪一个"不是"取第一个"。
// 与 profile 那一格、与 `POST /admin/login` 对 key 的那一格同一纪律。
func Test删档的key同名多值要拒(t *testing.T) {
	s, h := setup(t)
	admin := adminOnlyCookie(t)

	rec := postForm(h, "/admin/delete", "profile=kid&key="+testAdminKey+"&key=另一个", admin)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("同名多值要 422，实际 %d：%s", rec.Code, rec.Body)
	}
	if !fieldErrorsName(rec, "key") {
		t.Errorf("422 要点名 key：%s", rec.Body)
	}
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatalf("kid 不该被删掉：%v", err)
	}
}
