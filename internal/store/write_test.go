package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
)

func bookmarksFixture(rev int64, title string) *model.Bookmarks {
	b := EmptyBookmarks()
	b.Rev = rev
	b.Title = title
	b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList, Items: []model.Item{
		{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
	}}}
	return b
}

func TestSaveAndReload(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SaveBookmarks("dad", bookmarksFixture(1, "爸爸&<b>台")); err != nil {
		t.Fatal(err)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 1 || b.Title != "爸爸&<b>台" {
		t.Fatalf("got %+v", b)
	}
	raw, _ := os.ReadFile(s.BookmarksPath("dad"))
	if strings.Contains(string(raw), `\u003c`) {
		t.Fatalf("写出仍被 HTML 转义:\n%s", raw)
	}
}

func TestNoTempFilesLeftBehind(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SaveBookmarks("dad", bookmarksFixture(1, "x")); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(s.ProfileDir("dad"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("临时文件残留: %s", e.Name())
		}
	}
}

// 编码失败时原文件必须一个字节都不动，也不留临时文件。
// 顺序写反（先开目标文件再编码）就会把已有书签截成半个 JSON。
func TestEncodeFailureLeavesOriginalIntact(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SaveBookmarks("dad", bookmarksFixture(7, "原样")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.BookmarksPath("dad"))

	type unencodable struct {
		Rev int64    `json:"rev"`
		Ch  chan int `json:"ch"`
	}
	path := s.BookmarksPath("dad")
	if err := s.WriteFileJSON(path, unencodable{Rev: 8, Ch: make(chan int)}); err == nil {
		t.Fatal("want encode error")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatalf("编码失败却动了原文件:\n%s", after)
	}
	for _, e := range mustReadDir(t, filepath.Dir(path)) {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("失败路径残留临时文件: %s", e.Name())
		}
	}
}

func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ents
}

// 并发写之后任何一次读都必须拿到完整文件。
func TestConcurrentSavesKeepFileParseable(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SaveBookmarks("dad", bookmarksFixture(0, "seed")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := s.WithProfileLock("dad", func() error {
				return s.SaveBookmarks("dad", bookmarksFixture(int64(i), "并发"))
			})
			if err != nil {
				t.Errorf("save %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("并发写后读失败: %v", err)
	}
	if len(b.Groups) != 1 || len(b.Groups[0].Items) != 1 {
		t.Fatalf("内容不完整: %+v", b)
	}
}

// dad 被锁住时 mom 不该跟着排队。
func TestProfileLocksAreIndependent(t *testing.T) {
	s := New(t.TempDir())
	dadIn := make(chan struct{})
	finish := make(chan struct{})
	go func() {
		_ = s.WithProfileLock("dad", func() error {
			close(dadIn)
			<-finish
			return nil
		})
	}()
	<-dadIn
	done := make(chan struct{})
	go func() {
		_ = s.WithProfileLock("mom", func() error { return nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("mom 被 dad 的锁挡住了：锁没按档案分")
	}
	close(finish)
}

func TestSaveRejectsInvalidBeforeTouchingDisk(t *testing.T) {
	s := New(t.TempDir())
	bad := bookmarksFixture(1, "坏")
	bad.Groups[0].Layout = "grid"
	err := s.SaveBookmarks("dad", bad)
	if !strings.Contains(err.Error(), "layout") {
		t.Fatalf("want layout error, got %v", err)
	}
	if _, err := os.Stat(s.BookmarksPath("dad")); err == nil {
		t.Fatal("校验没过就不该落盘")
	}
}

func TestSaveProfileCreatesDir(t *testing.T) {
	s := New(t.TempDir())
	p := model.Profile{ID: "dad", Title: "爸爸", Admin: true, PIN: "sha256$a$b",
		CreatedAt: "2026-10-05T12:00:00+08:00", Theme: "auto", Layout: "sidebar"}
	if err := s.SaveProfile(&p); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Admin {
		t.Fatalf("got %+v", got)
	}
}

// 最终整枝评审 Important #7（账本 T5→T8/9 承诺）：写路径的校验错必须
// 与读路径对称地暴露 model.ErrValidation——P1-② 要靠它分流 422。
// %w: %v 会把 sentinel 断成纯文本，errors.Is 分不出来。
func TestSavePathsExposeErrValidationSentinel(t *testing.T) {
	s := New(t.TempDir())

	bad := bookmarksFixture(1, "坏")
	bad.Groups[0].Layout = "grid"
	err := s.SaveBookmarks("dad", bad)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("SaveBookmarks 要保住 ErrInvalid: %v", err)
	}
	if !errors.Is(err, model.ErrValidation) {
		t.Fatalf("SaveBookmarks 要暴露 model.ErrValidation 供 422 分流: %v", err)
	}

	bp := model.Profile{ID: "dad", Title: "爸爸", PIN: "sha256$a$b",
		CreatedAt: "2026-10-05T12:00:00+08:00", Theme: "neon", Layout: "sidebar"}
	err = s.SaveProfile(&bp)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("SaveProfile 要保住 ErrInvalid: %v", err)
	}
	if !errors.Is(err, model.ErrValidation) {
		t.Fatalf("SaveProfile 要暴露 model.ErrValidation: %v", err)
	}
}

// 最终整枝评审 Critical #1：null 必须在一次读→写往返后被抹平成 []。
// §4.3 写出契约的全部意义是人和 AI 都能拿到稳定 diff；磁盘上的文件
// 允许出现 "alias": null（人或半路中断的 AI 写出来的），解码成 nil、
// 原样写回就把 null 续了一命。四个位置（group.items、item.alias、
// item.tags、search.engines）各喂一个 null。
func TestNullSurvivesNoRoundTrip(t *testing.T) {
	root := fixtureRoot(t)
	withNull := `{"rev":3,"title":"爸爸","groups":[
	  {"id":"empty","title":"空组","layout":"list","items":null},
	  {"id":"nas","title":"NAS","layout":"list","items":[
	    {"id":"dsm","name":"群晖 DSM","url":"http://10.0.0.3:5000","alias":null,"tags":null}]}],
	  "search":{"enabled":false,"default":"","engines":null},
	  "weather":{"enabled":false,"location":"上海市"},"openInNewTab":false,"clock":false,"footer":""}`
	put(t, root, "profiles/dad/bookmarks.json", withNull)
	s := New(root)

	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("null 形状要读得进来: %v", err)
	}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if strings.Contains(got, "null") {
		t.Fatalf("读→写往返后仍把 null 写回磁盘（违反 §4.3 写出契约）:\n%s", got)
	}
	for _, want := range []string{`"items": []`, `"alias": []`, `"tags": []`, `"engines": []`} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺 %q（null 必须写成空数组）:\n%s", want, got)
		}
	}
	// 再读回来必须仍是合法数据（normalize 不能把内容读坏）。
	again, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("往返后重读失败: %v", err)
	}
	if len(again.Groups) != 2 || again.Groups[1].Items[0].ID != "dsm" ||
		len(again.Groups[0].Items) != 0 || again.Groups[1].Items[0].Alias == nil || again.Search.Engines == nil {
		t.Fatalf("往返后形状不对: %+v", again)
	}
}

// 写出侧的同一道保险：不经过磁盘、内存里直接构造的 Bookmarks（P1-②
// add ops 的示例值就没有 alias/tags 字段）也必须写不出 null。
func TestSaveBookmarksNormalizesNilSlices(t *testing.T) {
	s := New(t.TempDir())
	b := &model.Bookmarks{Rev: 1, Title: "t",
		Groups: []model.Group{{ID: "g", Title: "组", Layout: model.LayoutList,
			Items: []model.Item{{ID: "new", Name: "新条目", URL: "http://10.0.0.3/"}}}}}
	// Groups[0].Items 非 nil，但 Item 的 Alias/Tags 是 nil，Search.Engines 也是 nil。
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("内存构造的 nil 切片被写成 null:\n%s", raw)
	}
}
