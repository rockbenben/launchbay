package httpapi

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func TestUploadedBookmarkIconRoundTrip(t *testing.T) {
	s, h := setupBatch(t)
	cookie := loginCookie(t, h, "dad", "1234")
	pngIcon := func(width int) string {
		t.Helper()
		var buf bytes.Buffer
		if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, width, 64))); err != nil {
			t.Fatal(err)
		}
		return model.UploadedIconPrefix + base64.StdEncoding.EncodeToString(buf.Bytes())
	}
	setIcon := func(icon string) *httptest.ResponseRecorder {
		t.Helper()
		body := fmt.Sprintf(`{"profile":"dad","by":"web","expectedRev":1,"ops":[{"op":"set","path":"groups/nas/items/dsm/icon","value":%q}]}`, icon)
		return post(h, "/api/v1/batch", body, cookie, "")
	}

	for _, bad := range []string{
		"data:image/svg+xml;base64,PHN2Zz4=",
		model.UploadedIconPrefix + "bm90IGEgcG5n",
		pngIcon(65),
	} {
		if rec := setIcon(bad); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field": "icon"`) {
			t.Fatalf("expected invalid icon field, got %d: %s", rec.Code, rec.Body)
		}
	}
	icon := pngIcon(64)
	if rec := setIcon(icon); rec.Code != http.StatusOK {
		t.Fatalf("could not save icon: %d: %s", rec.Code, rec.Body)
	}

	page := get(h, "/", cookie)
	if !strings.Contains(page.Body.String(), `src="`+icon+`"`) || strings.Contains(page.Body.String(), "data-icon-missing") {
		t.Fatal("saved icon did not render as a PNG")
	}
	if page := get(h, "/", loginCookie(t, h, "kid", "5678")); strings.Contains(page.Body.String(), icon) {
		t.Fatal("uploaded icon appeared outside its profile")
	}
	status := getWithToken(h, "/api/v1/status", adminTok)
	if status.Code != http.StatusOK || strings.Contains(status.Body.String(), icon) ||
		!strings.Contains(status.Body.String(), `"iconUnmatched": 0`) {
		t.Fatalf("unexpected admin status: %d: %s", status.Code, status.Body)
	}

	backup := getWithToken(h, "/api/v1/backup?profile=dad", dadTok)
	if err := s.SaveBookmarks("kid", &model.Bookmarks{Rev: 1, Groups: []model.Group{}}); err != nil {
		t.Fatal(err)
	}
	if rec := post(h, "/api/v1/restore", backup.Body.String(), "", kidTok); rec.Code != http.StatusOK {
		t.Fatalf("could not restore backup: %d: %s", rec.Code, rec.Body)
	}
	restored, err := s.LoadBookmarks("kid")
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.Groups[0].Items[0].Icon; got != icon {
		t.Fatal("restored bookmark lost its uploaded icon")
	}
}
