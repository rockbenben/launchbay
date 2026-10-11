package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCreateProfileWritesTwoFiles(t *testing.T) {
	s := New(t.TempDir())
	p, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b", Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "dad" || !p.Admin || p.CreatedAt == "" {
		t.Fatalf("got %+v", p)
	}
	for _, path := range []string{s.ProfilePath("dad"), s.BookmarksPath("dad")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("缺文件 %s", path)
		}
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 0 || len(b.Groups) != 0 {
		t.Fatalf("新档案要空且 rev 0: %+v", b)
	}
	raw, _ := os.ReadFile(s.BookmarksPath("dad"))
	if !strings.Contains(string(raw), `"groups": []`) {
		t.Fatalf("groups 要写成空数组而不是 null:\n%s", raw)
	}
}

func TestCreateProfileRejectsAndCleansUp(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   NewProfileInput
		want error
	}{
		{"id 已存在", NewProfileInput{ID: "dad", Title: "又", PinHash: "h"}, ErrExists},
		{"id 非法", NewProfileInput{ID: "Dad", Title: "爸", PinHash: "h"}, ErrInvalid},
		{"title 空", NewProfileInput{ID: "kid", Title: " ", PinHash: "h"}, ErrInvalid},
		{"pin 空", NewProfileInput{ID: "kid", Title: "娃", PinHash: ""}, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.CreateProfile(tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
	// 失败的建档不许留下半个目录：否则选单里会出现永远进不去的档案。
	for _, id := range []string{"kid"} {
		if _, err := os.Stat(s.ProfileDir(id)); err == nil {
			t.Fatalf("失败建档残留目录 %s", id)
		}
	}
	if entries, err := os.ReadDir(s.profilesDir()); err == nil {
		if len(entries) != 1 {
			t.Fatalf("profiles 目录该只剩 dad, got %d", len(entries))
		}
	}
}

func TestCreateProfileRollsBackHalfWrittenProfile(t *testing.T) {
	s := New(t.TempDir())
	dir := s.ProfileDir("kid")
	if err := os.MkdirAll(filepath.Join(dir, "bookmarks.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(NewProfileInput{ID: "kid", Title: "娃", PinHash: "h"}); err == nil {
		t.Fatal("SaveBookmarks 撞目录该失败")
	}
	if _, statErr := os.Stat(dir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("写入半途失败必须回收整个档案目录 %s，stat=%v", dir, statErr)
	}
}

// 建档的串行化必须在 CreateProfile 内部：存在性检查、建目录、两次写
// 是一个动作。放在调用方（handler 包 WithProfileLock）意味着每个新调用
// 点都要记得——忘了就双双通过 exists 检查再互相覆盖。
// 两个 goroutine 共用起跑信号，同一 id 并发建档：恰好一个成功、
// 一个 ErrExists。这是概率性见证（窗口很窄，单轮未必命中交错），
// 所以门禁配 -count=25；确定性的证明需要测试缝（注入延迟），本期不做。
func TestCreateProfileConcurrentSameID(t *testing.T) {
	s := New(t.TempDir())
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = s.CreateProfile(NewProfileInput{ID: "twins", Title: "双胞胎", PinHash: "sha256$a$b"})
		}(i)
	}
	close(start)
	wg.Wait()
	var ok, exists int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrExists):
			exists++
		default:
			t.Fatalf("意外错误: %v", err)
		}
	}
	if ok != 1 || exists != 1 {
		t.Fatalf("want 恰好 1 成功 + 1 ErrExists, got ok=%d exists=%d errs=%v", ok, exists, errs)
	}
}
