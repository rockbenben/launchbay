package httpapi

import (
	"encoding/json"
	"github.com/rockbenben/launchbay/internal/model"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
)

func TestLinkShapeManualAndHostPriority(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "http://nas.example.com/", nil)
	r.RemoteAddr = "10.0.0.7:54321"
	p := &model.Profile{LinkMode: model.LinkLAN}
	if got := s.linkShapeOf(r, p, nil, nil); got.Rule != "manual" || got.Mode != model.LinkLAN {
		t.Fatal(got)
	}
	p.LinkMode = model.LinkAuto
	if got := s.linkShapeOf(r, p, nil, []string{"10.0.0.0/8"}); got.Mode != model.LinkWAN {
		t.Fatal("CIDR overrode Host", got)
	}
	r.Host = ""
	if got := s.linkShapeOf(r, p, nil, nil); got.Rule != "fallback" {
		t.Fatal(got)
	}
	if got := s.linkShapeOf(r, p, nil, []string{"broken", "10.0.0.0/8"}); got.Rule != "lan-cidr" {
		t.Fatal(got)
	}
}

func TestDualURLRequestRenderingSearchCacheAndBackup(t *testing.T) {
	s, h := setupBatch(t)
	body := `{"profile":"dad","by":"ai","expectedRev":1,"ops":[{"op":"set","path":"groups/nas/items/dsm/urls/wan","value":"https://nas.example.com/a?x=1&y=2"}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	history := get(h, "/history", cookie).Body.String()
	if !strings.Contains(history, "改外网地址 ·") || strings.Contains(history, "%!(EXTRA") || strings.Contains(history, "groups/nas/items/dsm/urls/wan") {
		t.Fatal("WAN history action did not name the item")
	}
	read := func(host, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Host = host
		r.Header.Set("Cookie", cookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	lan := read("10.0.0.5:7600", "/")
	wan := read("nas.example.com", "/")
	if lan.Header().Get("X-LB-Link-Mode") != "lan" || wan.Header().Get("X-LB-Link-Mode") != "wan" {
		t.Fatal(lan.Header(), wan.Header())
	}
	if !strings.Contains(wan.Body.String(), `href="https://nas.example.com/a?x=1&amp;y=2"`) {
		t.Fatal("WAN href absent")
	}
	if lan.Header().Get("ETag") == wan.Header().Get("ETag") {
		t.Fatal("LAN/WAN cache collided")
	}
	for _, rec := range []*httptest.ResponseRecorder{lan, wan} {
		if !strings.Contains(rec.Body.String(), `data-addr-wan="https://nas.example.com/a?x=1&amp;y=2"`) {
			t.Fatal("editing lost original override")
		}
	}
	if rec := read("nas.example.com", "/?q=10.0.0.3"); strings.Contains(rec.Body.String(), `data-item="dsm"`) {
		t.Fatal("search matched inactive URL")
	}
	if rec := read("nas.example.com", "/?q=nas.example.com"); !strings.Contains(rec.Body.String(), `data-item="dsm"`) {
		t.Fatal("search missed active URL")
	}
	before, _ := s.LoadBookmarks("dad")
	if rec := postForm(h, "/prefs", "linkMode=wan", cookie); rec.Code != 303 {
		t.Fatal(rec.Code, rec.Body)
	}
	if rec := read("10.0.0.5:7600", "/"); rec.Header().Get("X-LB-Link-Mode") != "wan" {
		t.Fatal(rec.Header())
	}
	after, _ := s.LoadBookmarks("dad")
	if before.Rev != after.Rev {
		t.Fatal("preference wrote bookmark rev")
	}
	pkg := backupOf(t, h, "/api/v1/backup?profile=dad", cookie)
	if pkg["settings"].(map[string]any)["linkMode"] != "wan" {
		t.Fatal(pkg["settings"])
	}
	if rec := post(h, "/api/v1/restore", `{"kind":"launchbay-backup","version":1,"settings":{"linkMode":"lan"}}`, cookie, ""); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body)
	}
	p, _ := s.LoadProfile("dad")
	if p.LinkMode != model.LinkLAN {
		t.Fatal(p.LinkMode)
	}
	if rec := post(h, "/api/v1/restore", `{"kind":"launchbay-backup","version":1,"settings":{"linkMode":"proxy"}}`, cookie, ""); rec.Code != 422 {
		t.Fatal(rec.Code, rec.Body)
	}
	for _, value := range []string{"linkMode=", "linkMode=proxy", "linkMode=lan&linkMode=wan"} {
		if rec := postForm(h, "/prefs", value, cookie); rec.Code != 422 {
			t.Fatal(value, rec.Code)
		}
	}
	raw, _ := json.Marshal(pkg)
	if strings.Contains(string(raw), "lanHosts") || strings.Contains(string(raw), "lanCIDRs") {
		t.Fatal("machine rules leaked into backup")
	}
}

func TestLinkDiagnosticsAndAuxiliaryHeaders(t *testing.T) {
	_, h := setupBatch(t)
	cookie := adminCookieOf(t, h)
	for _, host := range []string{"10.0.0.5:7600", "nas", "nas.example.com", netip.AddrFrom4([4]byte{203, 0, 113, 9}).String() + ":7600", "[fd00::5]:7600"} {
		r := httptest.NewRequest("GET", "/admin", nil)
		r.Host, r.RemoteAddr = host, "10.0.0.7:54321"
		r.Header.Set("Cookie", cookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		body := rec.Body.String()
		if rec.Code != 200 || !strings.Contains(body, host) || !strings.Contains(body, r.RemoteAddr) || !strings.Contains(body, "lanCIDRs") || strings.Contains(body, testAdminKey) {
			t.Fatalf("diagnostics missing facts or exposed key for %s", host)
		}
		if rec.Header().Get("X-LB-Link-Mode") != "" {
			t.Fatal("admin got profile mode header")
		}
	}
	for _, target := range []string{"/", "/prefs", "/history", "/import"} {
		cookie := ""
		if target != "/" {
			cookie = dadFormCookie
		}
		if rec := get(h, target, cookie); rec.Header().Get("X-LB-Link-Mode") != "" {
			t.Fatal("auxiliary response got mode header", target)
		}
	}
}

func TestRestoreBadLinkModeDoesNotWriteBookmarks(t *testing.T) {
	s, h := setup(t)
	before, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"kind":"launchbay-backup","version":1,"settings":{"linkMode":"proxy"},"bookmarks":{"groups":[{"id":"new","title":"new","items":[{"id":"new","name":"new","url":"https://example.com"}]}]}}`
	if rec := post(h, "/api/v1/restore", body, "", dadTok); rec.Code != 422 {
		t.Fatal(rec.Code, rec.Body)
	}
	after, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil || string(before) != string(after) {
		t.Fatal("invalid settings wrote bookmarks", err)
	}
}

func TestBackupDualURLRestoresBothAddressesIntoAnotherProfile(t *testing.T) {
	s, h := setup(t)
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: "list", Items: []model.Item{{ID: "dsm", Name: "DSM", URL: "http://10.0.0.5:5000", URLs: &model.ItemURLs{WAN: "https://nas.example.com"}}}}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, h, "dad", "1234")
	if rec := postForm(h, "/prefs", "linkMode=wan", cookie); rec.Code != 303 {
		t.Fatal(rec.Code)
	}
	pkg := backupOf(t, h, "/api/v1/backup?profile=dad", cookie)
	raw, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if rec := post(h, "/api/v1/restore", string(raw), "", "tok-kid"); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body)
	}
	restored, err := s.LoadBookmarks("kid")
	if err != nil {
		t.Fatal(err)
	}
	it := restored.Groups[0].Items[0]
	if it.URL != "http://10.0.0.5:5000" || it.URLs == nil || it.URLs.WAN != "https://nas.example.com" {
		t.Fatal(it)
	}
	p, err := s.LoadProfile("kid")
	if err != nil || p.LinkMode != model.LinkWAN {
		t.Fatal(p, err)
	}
}
