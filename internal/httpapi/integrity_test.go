package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func TestJSONRequestsRejectTrailingContent(t *testing.T) {
	for _, tail := range []string{"{}", "null", "garbage", string([]byte{0xff})} {
		t.Run(fmt.Sprintf("%x", tail), func(t *testing.T) {
			s, h := setupBatch(t)
			before, err := os.ReadFile(s.BookmarksPath("dad"))
			if err != nil {
				t.Fatal(err)
			}
			body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"footer","value":"changed"}]}` + tail
			rec := post(h, "/api/v1/batch", body, "", dadTok)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("trailing content: want 422, got %d: %s", rec.Code, rec.Body)
			}
			after, err := os.ReadFile(s.BookmarksPath("dad"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Error("invalid request changed bookmarks")
			}
		})
	}
}

func TestRestoreRejectsTrailingContentAndInvalidUTF8(t *testing.T) {
	for _, body := range []string{
		`{"kind":"launchbay-backup","version":1,"settings":{"theme":"light"}}{}`,
		`{"kind":"launchbay-backup","version":1,"settings":{"pageTitle":"` + string([]byte{0xff}) + `"}}`,
	} {
		s, h := setup(t)
		rec := post(h, "/api/v1/restore", body, "", dadTok)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("invalid backup: want 422, got %d: %s", rec.Code, rec.Body)
		}
		p, err := s.LoadProfile("dad")
		if err != nil {
			t.Fatal(err)
		}
		if p.Theme != "auto" || p.PageTitle != "" {
			t.Errorf("invalid backup changed settings: %+v", p)
		}
	}
}

func TestRestoreAdminCookieAloneIsForbidden(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/api/v1/restore",
		`{"kind":"launchbay-backup","version":1,"settings":{"theme":"light"}}`, adminCookieOf(t, h), "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("recognized admin credential: want 403, got %d: %s", rec.Code, rec.Body)
	}
}

func TestRestoreEmptyResultsAreArrays(t *testing.T) {
	for _, section := range []string{`"settings":{}`, `"bookmarks":{"groups":[]}`} {
		_, h := setup(t)
		rec := post(h, "/api/v1/restore", `{"kind":"launchbay-backup","version":1,`+section+`}`, "", dadTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
		}
		for _, field := range []string{"settings", "warnings"} {
			if !bytes.Contains(rec.Body.Bytes(), []byte(`"`+field+`": []`)) {
				t.Errorf("%s must be an empty array: %s", field, rec.Body)
			}
		}
	}
}

func TestBackupMarksRejectStaleEmptySnapshot(t *testing.T) {
	s, _ := setup(t)
	cur, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	cur.Groups = []model.Group{{ID: "empty", Title: "Empty", Layout: model.LayoutTiles, Items: []model.Item{}}}
	if err := s.SaveBookmarks("dad", cur); err != nil {
		t.Fatal(err)
	}
	// Another tab writes into the previously empty group after restore reads it.
	next := *cur
	next.Rev++
	next.Groups = []model.Group{{ID: "empty", Title: "Empty", Layout: model.LayoutTiles,
		Items: []model.Item{{ID: "new", Name: "New bookmark", URL: "https://example.com/new"}}}}
	if err := s.SaveBookmarks("dad", &next); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	srv := New(newDeps(t, s, nil))
	out, err := srv.applyBackupMarks("dad", "dad", "import", cur, &backupMarks{})
	if err != nil {
		t.Fatal(err)
	}
	if out.conflict == nil || *out.conflict != next.Rev {
		t.Errorf("stale restore must conflict, got %+v", out)
	}
	after, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || historyLines(t, s, "dad") != 0 || s.SnapshotCount("dad") != 0 {
		t.Error("stale restore changed bookmarks, snapshots or history")
	}
}

func TestConcurrentImportsNeverMergeIntoOccupiedProfile(t *testing.T) {
	s, _ := setup(t)
	srv := New(newDeps(t, s, nil))
	const count = 12
	tokens := make([]string, count)
	for i := range tokens {
		raw := []byte(fmt.Sprintf(`<DL><DT><H3>Group%d</H3><DL><DT><A HREF="https://example.com/%d">Link%d</A></DL></DL>`, i, i, i))
		tokens[i] = srv.stashImport("dad", raw, 1, nil)
		if tokens[i] == "" {
			t.Fatal("could not stash fixture")
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, token := range tokens {
		wg.Go(func() {
			<-start
			_, _, _ = srv.applyImport(token, "dad", "dad", "import")
		})
	}
	close(start)
	wg.Wait()
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	items := 0
	for _, g := range b.Groups {
		items += len(g.Items)
	}
	if items != 1 || b.Rev != 1 || historyLines(t, s, "dad") != 1 {
		t.Fatalf("only one import may commit: items=%d rev=%d", items, b.Rev)
	}
}

func TestConcurrentRestoresKeepOneWinner(t *testing.T) {
	s, h := setup(t)
	start := make(chan struct{})
	type result struct {
		name string
		code int
		body []byte
	}
	results := make(chan result, 12)
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			name := fmt.Sprintf("import-%d", i)
			body := fmt.Sprintf(`{"kind":"launchbay-backup","version":1,"settings":{"pageTitle":%q},"bookmarks":{"groups":[{"id":%q,"title":"Imported","layout":"tiles","items":[{"id":%q,"name":"Imported","url":"https://example.com"}]}]}}`, name, name, name)
			<-start
			rec := post(h, "/api/v1/restore", body, "", dadTok)
			results <- result{name: name, code: rec.Code, body: rec.Body.Bytes()}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	winner := ""
	for result := range results {
		switch result.code {
		case http.StatusOK:
			if winner != "" {
				t.Errorf("multiple restores succeeded: %s and %s", winner, result.name)
			}
			winner = result.name
		case http.StatusConflict:
			var out errorResponse
			if err := json.Unmarshal(result.body, &out); err != nil || out.CurrentRev == nil || *out.CurrentRev != 1 || out.Kind != "rev-conflict" {
				t.Errorf("conflict must include currentRev: %s", result.body)
			}
		case http.StatusUnprocessableEntity:
			var out errorResponse
			if err := json.Unmarshal(result.body, &out); err != nil || out.Kind != "import-target-not-empty" {
				t.Errorf("unexpected restore rejection: %s", result.body)
			}
		default:
			t.Errorf("unexpected status %d: %s", result.code, result.body)
		}
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if winner == "" || p.PageTitle != winner || b.Rev != 1 || len(b.Groups) != 1 || b.Groups[0].ID != winner {
		t.Fatalf("only the winning restore may change bookmarks and settings: winner=%q pageTitle=%q bookmarks=%+v", winner, p.PageTitle, b)
	}
}
