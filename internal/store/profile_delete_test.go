package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteProfileRemovesDirIncludingSnapshots(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.SnapshotsDir("dad"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.SnapshotsDir("dad")+"/1-bookmarks.json", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProfile("dad"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ProfileDir("dad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("档案目录还在: %v", err)
	}
	list, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("列表里还有: %+v", list)
	}
}

// 删档不动 history.jsonl：全家庭审计线要留着（spec §6.1 的删档说明）。
func TestDeleteProfileKeepsHistory(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(HistoryEntry{At: "t", Profile: "dad", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProfile("dad"); err != nil {
		t.Fatal(err)
	}
	hist, err := s.ReadHistory("", 0)
	if err != nil || len(hist) != 1 {
		t.Fatalf("历史该保留: %v %+v", err, hist)
	}
}

// 删不存在的档案要**同时**被 errors.Is(err, os.ErrNotExist)（端点据此 404）
// 与 errors.Is(err, ErrNotFound)（调用方的存在性判据）认出：只满足其一，
// 另一头的判据就会假阴性。原来的 !os.IsNotExist(err) && err == nil 是
// "任何错都算过"的弱判据，这里换成项目门规的 errors.Is 并双检两个哨兵。
func TestDeleteMissingProfileIsNotFound(t *testing.T) {
	s := New(t.TempDir())
	err := s.DeleteProfile("ghost")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("该满足 os.ErrNotExist, got %v", err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("该满足 ErrNotFound, got %v", err)
	}
}

// 越界的 id 不许变成"把整个 profiles 目录删了"。
func TestDeleteProfileRefusesPathTraversal(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	// 哨兵放 data root：若某道闸失守把 profilesDir 甚至 root 端掉，它先没。
	sentinel := filepath.Join(s.Root(), "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"..", "../profiles", "/", ""} {
		if err := s.DeleteProfile(id); err == nil {
			t.Fatalf("DeleteProfile(%q) 该拒", id)
		}
	}
	// 拒绝不等于"半个删除已经发生"：profilesDir、哨兵、dad 目录都该原样还在。
	if _, err := os.Stat(s.profilesDir()); err != nil {
		t.Fatalf("profiles 目录被误删了: %v", err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("data root 被端了，哨兵该还在: %v", err)
	}
	if _, err := os.Stat(s.ProfileDir("dad")); err != nil {
		t.Fatalf("dad 目录被顺手带走: %v", err)
	}
}

// profileDirWithin 是包含闸本身。DeleteProfile 前面的 slug 闸挡掉了所有非法
// slug，所以没有一条非法 id 能透过 DeleteProfile 走到这道闸——不直接喂 helper
// 的话，修正后的闸永远测不到（评审 Finding 1）。这里绕过 DeleteProfile 直调。
func TestProfileDirWithin(t *testing.T) {
	s := New(t.TempDir())
	// 正常 slug：确是 profilesDir 的同名直接子目录。
	if dir, ok := s.profileDirWithin("dad"); !ok || dir != s.ProfileDir("dad") {
		t.Fatalf("profileDirWithin(dad) 该放行且指向 %s, got (%q, %v)", s.ProfileDir("dad"), dir, ok)
	}
	// 越界/畸形 id 一律拒——包括旧的 Rel 闸会误放行的 "..".
	bad := []string{"..", ".", "", "/", s.Root(), filepath.Join(s.Root(), "nested")}
	for _, id := range bad {
		if dir, ok := s.profileDirWithin(id); ok {
			t.Fatalf("profileDirWithin(%q) 不该放行，返回 %q", id, dir)
		}
	}
}

// clearBroken 的两行不是装饰：删档后坏档登记必须一起清掉，否则 /status
// union 坏档表时会挂出一条指向不存在档案的幽灵记录（与 Finding 2 的并表互锁）。
func TestDeleteProfileClearsBrokenRegistry(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	// 制造坏档：bookmarks.json 写坏，强制读一次把它登记进 Broken()。
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadBookmarks("dad"); err == nil {
		t.Fatal("坏书签该读失败")
	}
	if _, ok := s.BrokenReason("dad"); !ok {
		t.Fatal("dad 该被登记为坏档")
	}
	if err := s.DeleteProfile("dad"); err != nil {
		t.Fatal(err)
	}
	for _, id := range s.Broken() {
		if id == "dad" {
			t.Fatalf("删档后 Broken() 还挂着 dad: %+v", s.Broken())
		}
	}
	if _, ok := s.BrokenReason("dad"); ok {
		t.Fatal("删档后 BrokenReason(dad) 该为 false")
	}
}
