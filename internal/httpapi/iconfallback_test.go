package httpapi

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

type fallbackFetchStub struct {
	hosts []string
}

func (*fallbackFetchStub) Page(string) ([]byte, string, error)  { panic("unexpected page request") }
func (*fallbackFetchStub) Image(string) ([]byte, string, error) { panic("unexpected image request") }
func (f *fallbackFetchStub) FallbackImage(_ context.Context, host string) ([]byte, string, error) {
	f.hosts = append(f.hosts, host)
	if host == "unavailable.example.org" {
		return nil, "", nil
	}
	return []byte("from provider"), "png", nil
}

func TestIconFallbackRequiresSessionAndUsesOnlyPublicHostname(t *testing.T) {
	srv, h := setupServer(t)
	fetch := &fallbackFetchStub{}
	srv.deps.Fetch = fetch
	s := srv.deps.Store
	cookie := loginCookie(t, h, "dad", "1234")
	rev := revOf(t, s, "dad")
	if err := os.WriteFile(s.SettingsPath(), []byte(`{"lanHosts":["private.example.org"]}`), 0600); err != nil {
		t.Fatal(err)
	}

	const path = "/api/v1/icons/fallback"
	for _, tc := range []struct {
		body, cookie, token string
		code                int
	}{
		{`{"host":"example.org"}`, "", "", http.StatusUnauthorized},
		{`{"host":"example.org"}`, "", dadTok, http.StatusForbidden},
		{`{"host":"example.org"}`, "", adminTok, http.StatusForbidden},
		{`{"host":"https://example.org/private"}`, cookie, "", http.StatusUnprocessableEntity},
		{`{"host":"example.org:8443"}`, cookie, "", http.StatusUnprocessableEntity},
		{`{"host":"127.0.0.1"}`, cookie, "", http.StatusUnprocessableEntity},
		{`{"host":"nas"}`, cookie, "", http.StatusUnprocessableEntity},
		{`{"host":"nas.home.arpa"}`, cookie, "", http.StatusUnprocessableEntity},
		{`{"host":"private.example.org"}`, cookie, "", http.StatusUnprocessableEntity},
		{`{"host":"example.org","url":"https://example.org"}`, cookie, "", http.StatusUnprocessableEntity},
		{`{"host":"EXAMPLE.ORG."}`, cookie, "", http.StatusOK},
		{`{"host":"unavailable.example.org"}`, cookie, "", http.StatusBadGateway},
	} {
		rec := post(h, path, tc.body, tc.cookie, tc.token)
		if rec.Code != tc.code {
			t.Fatalf("%s: got %d, want %d: %s", tc.body, rec.Code, tc.code, rec.Body)
		}
		if tc.code == http.StatusOK && (!strings.Contains(rec.Body.String(), "data:image/png;base64,ZnJvbSBwcm92aWRlcg==") ||
			rec.Header().Get("Cache-Control") != "private, no-store") {
			t.Fatalf("invalid preview response: %s", rec.Body)
		}
	}
	if !reflect.DeepEqual(fetch.hosts, []string{"example.org", "unavailable.example.org"}) {
		t.Fatalf("unexpected provider queries: %v", fetch.hosts)
	}
	if after := revOf(t, s, "dad"); after != rev {
		t.Fatalf("preview changed rev from %d to %d", rev, after)
	}
}
