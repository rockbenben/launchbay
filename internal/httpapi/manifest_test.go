package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestManifest(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/manifest.webmanifest", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /manifest.webmanifest 期望 200，实际 %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/manifest+json") {
		t.Errorf("Content-Type = %q，期望 application/manifest+json", ct)
	}
	cc := rec.Header().Get("Cache-Control")
	if !strings.Contains(cc, "public") {
		t.Errorf("Cache-Control = %q，期望带 public", cc)
	}

	var m struct {
		Name            string `json:"name"`
		ShortName       string `json:"short_name"`
		StartURL        string `json:"start_url"`
		Display         string `json:"display"`
		BackgroundColor string `json:"background_color"`
		ThemeColor      string `json:"theme_color"`
		Icons           []struct {
			Src   string `json:"src"`
			Sizes string `json:"sizes"`
			Type  string `json:"type"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("解析 manifest JSON 失败: %v", err)
	}
	if m.Name != "Launchbay" || m.ShortName != "Launchbay" {
		t.Errorf("name/short_name 不对: name=%q, short_name=%q", m.Name, m.ShortName)
	}
	if m.Display != "standalone" {
		t.Errorf("display = %q，期望 standalone", m.Display)
	}
	if m.StartURL != "/" {
		t.Errorf("start_url = %q，期望 /", m.StartURL)
	}
	if len(m.Icons) == 0 {
		t.Errorf("manifest 缺少 icons")
	}
}
