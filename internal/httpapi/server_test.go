package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// New(Dependencies) 之后，本包的测试需要一个最小装配。Render 必须给真
// 渲染器：server.go 的契约写明它非 nil，GET / 离开占位页后就靠它——给
// nil 等于用 panic 当测试失败信号。装配走 newDeps 这唯一入口。
func testDeps(t *testing.T) Dependencies {
	t.Helper()
	scopes, err := auth.ParseScopes("admin:tok:*")
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(t.TempDir())
	return newDeps(t, s, scopes)
}

func TestHealthzServesOK(t *testing.T) {
	rec := httptest.NewRecorder()
	New(testDeps(t)).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("healthz code = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "ok\n" {
		t.Fatalf("healthz body = %q, want %q", got, "ok\n")
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	New(testDeps(t)).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}
