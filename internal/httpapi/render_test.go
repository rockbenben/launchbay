package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

func setupRender(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	s := store.New(t.TempDir())
	hash, err := auth.HashPIN("1234")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "dad", Title: "爸爸", PinHash: hash}); err != nil {
		t.Fatal(err)
	}
	b := store.EmptyBookmarks()
	b.Title = "爸爸的台"
	b.Groups = []model.Group{{ID: "nas", Title: "NAS 应用", Layout: model.LayoutList, Items: []model.Item{
		{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
	}}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	sc, err := auth.ParseScopes("ai-dad:tok-dad:dad")
	if err != nil {
		t.Fatal(err)
	}
	return New(newDeps(t, s, sc)).Handler(), s
}

func TestProfilePageHeaders(t *testing.T) {
	h, _ := setupRender(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("code %d body %.200s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("X-LB-Profile"); got != "dad" {
		t.Fatalf("X-LB-Profile = %q", got)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") || !strings.Contains(cc, "private") {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("缺 ETag")
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("CSP = %q", csp)
	}
	if !strings.Contains(rec.Body.String(), "爸爸") {
		t.Fatalf("要渲染出档案页: %.200s", rec.Body)
	}
}

func TestPickerHasItsOwnProfileHeader(t *testing.T) {
	h, _ := setupRender(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := rec.Header().Get("X-LB-Profile"); got != "picker" {
		t.Fatalf("选择页的 X-LB-Profile = %q", got)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("选择页 Cache-Control = %q（跨档案命中就是泄露）", cc)
	}
}

// bookmarklet 带进来的 ?add= 必须在 PIN 步骤活着：丢了它，未登录用户
// 点收藏按钮就是静默无事发生。
func TestKeepAddSurvivesPinStep(t *testing.T) {
	h, _ := setupRender(t)
	req := httptest.NewRequest("GET", "/?add=http%3A%2F%2F10.0.0.3%2Fnew", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `name="keep_add"`) || !strings.Contains(body, "http://10.0.0.3/new") {
		t.Fatalf("?add= 没进选择页表单:\n%.600s", body)
	}
}

func TestIfNoneMatchReturns304(t *testing.T) {
	h, _ := setupRender(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	first := httptest.NewRecorder()
	h.ServeHTTP(first, req)
	etag := first.Header().Get("ETag")

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req2.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	req2.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	h.ServeHTTP(second, req2)

	if second.Code != http.StatusNotModified {
		t.Fatalf("want 304, got %d", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Fatal("304 不许带 body")
	}
}

// 坏档案：只坏那一个，页面要给出可读原因而不是白屏或全站 500。
func TestBrokenProfileShowsReason(t *testing.T) {
	h, s := setupRender(t)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{\n \"rev\": 1,\n \"groups\": [\n")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("坏档案要给解释页而不是 5xx, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "读不了") || !strings.Contains(body, "第 4 行") {
		t.Fatalf("要给文件名行号与人话解释:\n%.400s", body)
	}
	if strings.Contains(body, "admin:tok-admin") || strings.Contains(body, "sha256$") {
		t.Fatal("解释页不许把令牌或 PIN 哈希抖出来")
	}
}

func TestUnknownProfileInCookieFallsBackToPicker(t *testing.T) {
	h, _ := setupRender(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "ghost"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "ghost"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-LB-Profile") != "picker" {
		t.Fatalf("指向不存在的档案要回选择页, got %q", rec.Header().Get("X-LB-Profile"))
	}
}

// 进管理页那颗口子必须**一直画着**（2026-10-15）。它原先挂在 HasAdmin 上，
// 于是刚装完机器、手里只有 compose 那把 key 的人在全站找不到第二处通向 /admin
// 的地方——账号层面那一整页在界面上不可达。这一条钉的就是"没有 cookie 时它也在"，
// 而档案页顶栏那枚「管理」徽标仍按 HasAdmin 画（它是捷径不是门）。
func Test选择页永远给得出管理入口而档案页的徽标仍看会话(t *testing.T) {
	h, _ := setupRender(t)
	const link = `<a class="badge" href="/admin">管理这台机器</a>`

	// ① 什么 cookie 都没有：入口必须在。
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), link) {
		t.Fatalf("无 cookie 的选择页没有管理入口，装完机器的人只能手打 /admin：%.300s", rec.Body)
	}

	// ② 带着管理权：也必须在（这一条不许因为"已经进得去"就把它撤掉）。
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", adminOnlyCookie(t))
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), link) {
		t.Fatal("带 lb_admin 的选择页反而没了管理入口")
	}

	// ③ 登着档案、没有管理权：档案页顶栏那枚「管理」不画。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "爸爸") {
		t.Fatalf("这一条要量的是档案页，结果落到了别处：%.200s", body)
	}
	if strings.Contains(body, `<a class="badge" href="/admin">管理</a>`) {
		t.Fatal("没有管理权的会话不该看见档案页顶栏的「管理」徽标")
	}
}

// 404 的两种答法（2026-10-15）。这一条钉的是**分界**而不是页面本身：
// 人在地址栏里打错一个字不该看见裸 JSON，而脚本要的 `route-not-found` 一枚都不许少。
// 状态码两边必须相同——把浏览器那一支答成 200 就是给一个不存在的页发缓存。
func Test404_浏览器拿页面脚本拿kind(t *testing.T) {
	h, _ := setupRender(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/prefs/nope", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("浏览器那一支的状态码必须是 404，拿到 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<html lang="zh-CN">`) {
		t.Fatal("这一页也要带 lang：它是一整屏人话，不是错误串")
	}
	if !strings.Contains(body, "这一页不在这里") {
		t.Fatalf("页面没说出这是怎么回事：%.300s", body)
	}
	if strings.Contains(body, "route-not-found") {
		t.Fatal("人看的这一屏不许出现机器码")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("Cache-Control = %q", cc)
	}
	// 空头 X-LB-Profile = SW 不存这一页（它不属于任何档案）。
	if v := rec.Header().Get("X-LB-Profile"); v != "" {
		t.Fatalf("X-LB-Profile = %q，这一页不属于任何档案", v)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/prefs/nope", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"kind": "route-not-found"`) {
		t.Fatalf("不带 Accept: text/html 的调用方拿到的仍是 kind 那一份：%.200s", rec.Body)
	}
}
