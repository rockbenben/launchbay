package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 引导判据要的是"这台机器有没有被人用过"，那是目录级的事实。
// 用 ListProfiles() 会漏：它读不动 profile.json 时静默跳过，
// 于是"把一个档案装坏了"等于把引导窗口重新打开（spec §12 与 §11 的交叉点）。
func TestCountProfiles_坏档案照样占位(t *testing.T) {
	s := New(t.TempDir())
	if n, err := s.CountProfiles(); err != nil || n != 0 {
		t.Fatalf("空安装该是 (0, nil)，得到 (%d, %v)", n, err)
	}
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountProfiles(); n != 1 {
		t.Errorf("建了一个档案后 CountProfiles = %d，要 1", n)
	}
	// 把它装坏：profile.json 写成读不出的字节。
	raw, err := os.ReadFile(s.ProfilePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ProfilePath("dad"), append([]byte("{坏"), raw...), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountProfiles(); n != 1 {
		t.Errorf("档案坏了以后 %d，要仍是 1（坏档案占位，窗口不许重开）", n)
	}
	if _, err := s.ListProfiles(); err != nil {
		t.Fatalf("ListProfiles 对坏档案的容错是本测试的前提: %v", err)
	}
}

// 前提证人：上面那句"ListProfiles 会跳过坏档案"必须是真的，
// 否则这条测试测的是一个不存在的差别（它红了就说明 store 的行为变了，
// 本任务的判据选择要重新讨论，而不是改测试）。
func TestCountProfiles_前提没变(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ProfilePath("dad"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	ps, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 0 {
		t.Fatalf("坏档案仍被 ListProfiles 列出（%d 个）：本任务改用目录数判据的理由不再成立", len(ps))
	}
}

func TestNextProfileID(t *testing.T) {
	s := New(t.TempDir())
	cases := []struct{ title, want string }{
		{"Dad", "dad"},
		{"nas box", "nas-box"},
		// 这两格钉的是归一化取向，不是合法性：ValidSlug 其实放得过 dad-，
		// 剥掉它是为了 id 稳定好看（它要写进目录名与快照路径）。
		{"Dad ", "dad"},
		{"nas  box  ", "nas-box"},
		{"MiXeD--Case!!", "mixed-case"},
		{"  爸爸  ", "home"}, // 中文一律 slug 不出东西 → 基名序列
		{"---", "home"},    // 只有分隔符 → 同上
		{"日本語", "home"},
	}
	for _, c := range cases {
		if got := s.NextProfileID(c.title); got != c.want {
			t.Errorf("NextProfileID(%q) = %q，要 %q", c.title, got, c.want)
		}
	}
	// 长度上界：model.ValidSlug 只收到 32 个字符。
	long := s.NextProfileID(strings.Repeat("a", 50))
	if len(long) > 32 || !model.ValidSlug(long) {
		t.Errorf("超长名字派生出 %q（%d 字符），要合法 slug 且 ≤32", long, len(long))
	}
}

// 撞名走基名序列，而且判据是"这个目录在不在"——不是"现在有几个人"：
// 删过档的计数会撒谎。
func TestNextProfileID_撞名与删除后回到基名(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("dad"); got != "home" {
		t.Errorf("slug 撞了已有 id 要退回基名序列，得到 %q", got)
	}
	if err := os.RemoveAll(s.ProfileDir("dad")); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("dad"); got != "dad" {
		t.Errorf("目录删掉以后该能重新用 dad，得到 %q（判据像在数东西而不是看在不在）", got)
	}
	if got := s.NextProfileID("爸爸"); got != "home" {
		t.Errorf("基名序列起点要是 home，得到 %q", got)
	}
	if err := os.MkdirAll(s.ProfileDir("home"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("爸爸"); got != "home-2" {
		t.Errorf("home 被占要递增到 home-2，得到 %q", got)
	}
	if err := os.MkdirAll(s.ProfileDir("home-2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("妈妈"); got != "home-3" {
		t.Errorf("要 home-3，得到 %q", got)
	}
	// 最锐的一型：**中间那个**空出来。现在盘上有 home 与 home-2 两个目录，删掉 home
	// 之后新建必须回到 home——按"现在有几个档案"算的实现会给出 home-3（数到 3 才空），
	// 于是名字里带一个没人用的 -3，而那个空位就永远悬着。
	if err := os.RemoveAll(s.ProfileDir("home")); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("奶奶"); got != "home" {
		t.Errorf("home 空出来后新建该拿回 home，得到 %q（判据像在数有几个档案，而不是查目录在不在）", got)
	}
}

// 判据数的是**目录**：profiles/ 下一个普通文件不是档案。删掉实现里那句
// d.IsDir() 之后，整包原本全绿（评审复核抓到的证人缺口）——一个残留文件
// 就能把免令牌窗口永久焊死，这后果比"多算一个"更难受。
func TestCountProfiles_普通文件不算档案(t *testing.T) {
	s := New(t.TempDir())
	if err := os.MkdirAll(s.profilesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root(), "profiles", "leftover.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountProfiles(); err != nil || n != 0 {
		t.Errorf("profiles/ 下只有一个残留文件时该是 (0, nil)，得到 (%d, %v)", n, err)
	}
	// 对照：同一个位置换成目录就必须算上，否则上面那句可以靠"永远返回 0"蒙过。
	if err := os.MkdirAll(filepath.Join(s.Root(), "profiles", "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountProfiles(); err != nil || n != 1 {
		t.Errorf("真档案目录必须算进去，得到 (%d, %v)", n, err)
	}
}

// 主证人：判据必须在 store 内部、写入之前。把判据搬到 handler（"先查后写"）
// 就是这条打红——它比任何并发测试都可靠，因为它不靠时钟运气。
func TestCreateFirstProfile_窗口只开得下一个人(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateFirstProfile(NewProfileInput{ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatalf("空安装引导建档失败: %v", err)
	}
	_, err := s.CreateFirstProfile(NewProfileInput{ID: "home-2", Title: "妈妈", PinHash: "sha256$a$b"})
	if !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("第二次引导要 ErrBootstrapClosed（errors.Is 链要在），得到 %v", err)
	}
	if n, _ := s.CountProfiles(); n != 1 {
		t.Errorf("拒的那一次磁盘上多出了东西：%d 个目录，要 1", n)
	}
}

// 空目录（不是合法档案）也占位：判据是"有没有人用过这台机器"，
// 不是"有没有一个进得去的档案"。
func TestCreateFirstProfile_半个目录也占位(t *testing.T) {
	s := New(t.TempDir())
	if err := os.MkdirAll(filepath.Join(s.Root(), "profiles", "ghost"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFirstProfile(NewProfileInput{ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("ghost 占位时要拒，得到 %v", err)
	}
}

// 承重的并发证人：把 CountProfiles 挪到锁外（先查后写）实测 20/20 红，
// 因为它测的是主证人测不到的那个组合——**两个不同名字各派生一个 id**，
// 各自过自己的 exists 检查，于是安静地建出两个档案。本机没有 C 编译器，
// -race 跑不了，所以这条靠 barrier + -count=20 复现（评审复核过同样的结果）。
func TestCreateFirstProfile_并发提交只有一个成(t *testing.T) {
	s := New(t.TempDir())
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, title := range []string{"alpha", "intruder"} {
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			<-start
			_, err := s.CreateFirstProfile(NewProfileInput{
				ID: s.NextProfileID(title), Title: title, PinHash: "sha256$a$b"})
			results <- err
		}(title)
	}
	close(start)
	wg.Wait()
	close(results)
	var ok, closed int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrBootstrapClosed):
			closed++
		default:
			t.Fatalf("意外的错: %v", err)
		}
	}
	if ok != 1 || closed != 1 {
		t.Errorf("建成 %d、拒 %d，要 1/1", ok, closed)
	}
	if n, _ := s.CountProfiles(); n != 1 {
		t.Errorf("磁盘上有 %d 个档案目录，要 1 个（两个都建成管理员是最贵的形状）", n)
	}
}
