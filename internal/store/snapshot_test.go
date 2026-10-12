package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func seededWithContent(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	// PinHash 留着（2026-10-12 起建档**不要求**它，简报的测试代码不写也不会再报
	// `invalid: 档案 "dad" 缺 PIN 哈希`；这里仍照抄，是为了让这份夹具与它的源头
	// 逐字一致——将来改夹具时少一处需要一起改的地方）。
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	b := &model.Bookmarks{Rev: 3, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutTiles,
		Items: []model.Item{{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}}},
	}}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWriteSnapshotCopiesCurrentRev(t *testing.T) {
	s := seededWithContent(t)
	rev, err := s.WriteSnapshot("dad")
	if err != nil {
		t.Fatal(err)
	}
	if rev != 3 {
		t.Fatalf("快照目录名该是被保住的 rev, got %d", rev)
	}
	raw, err := s.LoadSnapshot("dad", 3)
	if err != nil {
		t.Fatal(err)
	}
	// 快照必须与磁盘原件逐字节相同——revert 靠的就是这份字节。
	want, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(want) {
		t.Fatalf("快照与原件不一致")
	}
}

func TestSnapshotOfMissingBookmarksIsNotAnError(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "kid", Title: "小满", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	// CreateProfile 会写出 rev 0 的 bookmarks.json（P1-①/8 的契约，简报假设
	// 建档后没有这个文件，实测 count=1），所以"还没有 bookmarks.json"的
	// 状态（外部程序删过文件）要手工删掉它来构造。
	if err := os.Remove(s.BookmarksPath("kid")); err != nil {
		t.Fatal(err)
	}
	// 空档案（还没有 bookmarks.json）第一次写入前无东西可保：0 份快照、不是错误。
	rev, err := s.WriteSnapshot("kid")
	if err != nil {
		t.Fatalf("空档案的快照该静默跳过: %v", err)
	}
	if rev != 0 || s.SnapshotCount("kid") != 0 {
		t.Fatalf("空档案不该造出快照: rev=%d count=%d", rev, s.SnapshotCount("kid"))
	}
}

func TestSnapshotsPrunedToNewest50(t *testing.T) {
	s := seededWithContent(t)
	for i := 1; i <= 55; i++ {
		b, err := s.LoadBookmarks("dad")
		if err != nil {
			t.Fatal(err)
		}
		b.Rev = int64(i)
		if err := s.SaveBookmarks("dad", b); err != nil {
			t.Fatal(err)
		}
		if _, err := s.WriteSnapshot("dad"); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListSnapshots("dad")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 50 {
		t.Fatalf("只该留 50 份, got %d", len(list))
	}
	if list[0] != 55 || list[49] != 6 {
		t.Fatalf("该留最新 50 份（55..6）, got %d..%d", list[0], list[49])
	}
	if _, err := os.Stat(filepath.Join(s.SnapshotsDir("dad"), "5")); !os.IsNotExist(err) {
		t.Fatalf("第 5 份该被裁掉, stat err=%v", err)
	}
}

// 非数字目录名（有人手工往里塞了 notes）不许让快照列表整个崩掉，
// 也不许被当成一份可 revert 的快照。
func TestNonNumericSnapshotDirsAreIgnored(t *testing.T) {
	s := seededWithContent(t)
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(s.SnapshotsDir("dad"), "notes")
	if err := os.MkdirAll(stray, 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListSnapshots("dad")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0] != 3 {
		t.Fatalf("got %+v", list)
	}
}
