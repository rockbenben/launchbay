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
	// **不带 Admin**（2026-10-12）：档案不再有"是不是管理员"这个属性，`NewProfileInput`
	// 上那一格连同 `model.Profile.Admin` 一起删掉了——建出来的每一份都只是一个人的书签页。
	p, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"})
	if err != nil {
		t.Fatal(err)
	}
	// 原来的 `!p.Admin` 那一断言随字段一起删掉（钉的字段已不存在，见上）：
	// 换成钉**还在的**那些字段，免得这条用例只剩"建了两个文件"。
	if p.ID != "dad" || p.Title != "爸爸" || p.PIN != "sha256$a$b" || p.CreatedAt == "" {
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
	// profile.json 的写出形状：**不含 admin 键**（2026-10-12）。这一条替代了旧版
	// "写出的 JSON 里含 `"admin": true`" 的断言——档案不再有管理员属性，
	// 而 §4.3 的写出契约由 struct 字段序固定，所以键的**有无**本身就是判据。
	profRaw, err := os.ReadFile(s.ProfilePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(profRaw), "admin") {
		t.Fatalf("profile.json 不该再有 admin 键:\n%s", profRaw)
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
		// 「pin 空」这一格 2026-10-12 从这里挪走了：建档**可以**不设 PIN，
		// 所以它不再是错误。它翻成了下面 TestCreateProfile_不设PIN也建得出来。
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

// 建档**可以**不带 PIN 哈希（2026-10-12）：这是"可以不设"这个承诺的落盘那一端。
// 只钉"不报错"不够——还得钉盘上真的写出空 PIN，且书签页与目录一样建出来，
// 否则会留下一份结构齐全、只是进不去的空壳（那正是旧规则要挡的东西，而它没挡对）。
func TestCreateProfile_不设PIN也建得出来(t *testing.T) {
	s := New(t.TempDir())
	p, err := s.CreateProfile(NewProfileInput{ID: "tv", Title: "客厅那台"})
	if err != nil {
		t.Fatalf("不设 PIN 的档案该建得出来: %v", err)
	}
	if p.PIN != "" {
		t.Errorf("要的是不设防，建出来却是 %q", p.PIN)
	}
	got, err := s.LoadProfile("tv")
	if err != nil {
		t.Fatalf("读不回来: %v", err)
	}
	if got.PIN != "" {
		t.Errorf("重读之后 PIN 变成 %q——写盘把它编出来了吗", got.PIN)
	}
	b, err := s.LoadBookmarks("tv")
	if err != nil {
		t.Fatalf("书签页要一样建出来: %v", err)
	}
	if b.Rev != 0 {
		t.Errorf("新档案的 rev 该是 0，实际 %d", b.Rev)
	}
}

// 建档第二次写失败时必须把刚建的档案目录回收掉——留下半个档案比报错更难查。
//
// **这个触发形状已经不存在了**（2026-10-19），记下来免得后来的人照它改：
// 从前这里先 `MkdirAll` 整个档案目录，再让 CreateProfile 建一次，依赖
// 「存在性只看 profile.json」那道旧闸放行，于是能走到第二次写。现在判据按
// 目录判（`idTaken`，为了挡住「目录在、profile.json 缺」时静默覆盖既有书签），
// 预建档案目录会**先被那道闸挡成 ErrExists**，回滚路径压根走不到。
//
// 而闸挡住时它**不该**删目录：那个目录里可能装着**别人的书签**（那正是这道闸
// 要救的形状）。rollbackDir 是 RemoveAll(整目录)，它只在"目录里只有刚建的东西"
// 时成立——所以这里钉的是"闸挡下不许删盘上的东西"。
func TestCreateProfile_目录被占时不删盘上已有的东西(t *testing.T) {
	s := New(t.TempDir())
	dir := s.ProfileDir("kid")
	if err := os.MkdirAll(filepath.Join(dir, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshots", "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.CreateProfile(NewProfileInput{ID: "kid", Title: "娃", PinHash: "h"})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("目录已存在时该返回 ErrExists，实际 %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "snapshots", "keep")); statErr != nil {
		t.Fatalf("被存在性闸挡下时不该删掉盘上已有的东西：%v", statErr)
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
