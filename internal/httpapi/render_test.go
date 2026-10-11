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
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "dad", Title: "爸爸", PinHash: hash, Admin: true}); err != nil {
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
	sc, err := auth.ParseScopes("admin:tok-admin:*")
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
