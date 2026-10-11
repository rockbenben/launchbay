package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
)

// seedBroken 建一份档案、写书签、落一份快照、追一行历史，然后把 profile.json 写成读不动
// 的字节。返回改前的「现场」——设计 §7.1 点名的三样（bookmarks.json、snapshots/ 下每个
// 文件、history.jsonl 已有内容）只能比字节，不能比印象。
func seedBroken(t *testing.T, id string) (*Store, map[string][]byte) {
	t.Helper()
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: id, Title: "旧名", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	b := model.Bookmarks{Rev: 3, Title: id, Groups: []model.Group{{
		ID: "g", Title: "组", Layout: model.LayoutList,
		Items: []model.Item{{ID: "i", Name: "条目", URL: "http://10.0.0.1/", Icon: "tabler:home"}},
	}}}
	if err := s.SaveBookmarks(id, &b); err != nil {
		t.Fatal(err)
	}
	before := seedScene(t, s, id)
	breakProfileJSON(t, s, id)
	if _, err := s.LoadProfile(id); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("夹具没把 profile.json 写成内容错: %v", err)
	}
	return s, before
}

// seedScene 造出「重建不许碰」的那份现场（书签快照 + 一行全家庭历史）并收下这些字节。
// 半件事那两条测试也要同一份现场——「现场该有哪几样」的判据因此只写一处，不在每条测试里
// 各猜一遍（rev 是 0 还是 3 这种细节也不用来挑键名）。
func seedScene(t *testing.T, s *Store, id string) map[string][]byte {
	t.Helper()
	if _, err := s.WriteSnapshot(id); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(HistoryEntry{
		At: time.Now().Format(time.RFC3339), Profile: id, Actor: "ai", By: "cli", Rev: 3,
		Ops: []HistoryOp{{Op: "set", Path: "groups/g/items/i/name"}},
	}); err != nil {
		t.Fatal(err)
	}
	roster := diskRoster(t, s, id)
	// 正对照：书签、快照、历史三样都要真的在，否则「逐字节不变」是在对空集合成立。
	for _, must := range []string{"bookmarks.json", "history.jsonl"} {
		if len(roster[must]) == 0 {
			t.Fatalf("夹具没造出 %s，现场：%v", must, sortedKeys(roster))
		}
	}
	if !hasSnapshot(roster) {
		t.Fatalf("夹具没造出快照，现场：%v", sortedKeys(roster))
	}
	return roster
}

func hasSnapshot(roster map[string][]byte) bool {
	for k := range roster {
		if strings.HasPrefix(k, "snapshots/") {
			return true
		}
	}
	return false
}

// diskRoster 收下「重建不许碰」的全部字节：书签、snapshots/ 整棵树、全家庭历史线。
// 键是稳定标签，值是从盘上读回来的原字节。
func diskRoster(t *testing.T, s *Store, id string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	add := func(label, path string) {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return // 不在也是一种现场：改后若冒出文件，键的数目就会不一样
		}
		if err != nil {
			t.Fatalf("读现场 %s: %v", label, err)
		}
		out[label] = data
	}
	add("bookmarks.json", s.BookmarksPath(id))
	add("history.jsonl", s.HistoryPath())
	root := s.SnapshotsDir(id)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil // 没快照目录也是一种现场：改后冒出文件，键数就会不一样
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out["snapshots/"+filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("收 snapshots: %v", err)
	}
	return out
}

func sortedKeys(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// assertRosterUntouched 把改前那份现场逐字节比回来（书签、快照、历史一个字节都不许动）。
func assertRosterUntouched(t *testing.T, s *Store, id string, before map[string][]byte) {
	t.Helper()
	after := diskRoster(t, s, id)
	if len(before) != len(after) {
		t.Errorf("现场的文件数变了：改前 %v，改后 %v", sortedKeys(before), sortedKeys(after))
	}
	for label, data := range before {
		if string(after[label]) != string(data) {
			t.Errorf("%s 被动了：\n改前 %q\n改后 %q", label, data, after[label])
		}
	}
	if _, err := s.LoadBookmarks(id); err != nil {
		t.Errorf("重建之后书签反而读不动了: %v", err)
	}
}

// breakProfileJSON 把 profile.json 写成半截 JSON——夹具需要「坏」，只能手工造。
func breakProfileJSON(t *testing.T, s *Store, id string) {
	t.Helper()
	if err := s.WriteFileAtomic(s.ProfilePath(id), []byte("{\n  \"id\": \""+id+"\",\n  \"title\":\n")); err != nil {
		t.Fatal(err)
	}
}

// authHash 造一个真哈希：夹具不许自己拼 sha256$... 假装哈希过——那会让「rebuild 写进去
// 的哈希能用」这件事永远没被证明（本仓库为这类假夹具改过口径）。
func authHash(t *testing.T) string {
	t.Helper()
	h, err := auth.HashPIN("4321")
	if err != nil {
		t.Fatalf("真哈希造不出来: %v", err)
	}
	return h
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil // 「什么都没多出来」这类断言要的正是这个形状：零档案时 profiles/ 本就不在
	}
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestRebuild_坏档重建后书签快照历史一字不动(t *testing.T) {
	s, before := seedBroken(t, "mom")
	h := authHash(t)
	p, err := s.RebuildProfile(RebuildInput{ID: "mom", Title: "妈妈", PinHash: h, Admin: true})
	if err != nil {
		t.Fatalf("坏档 rebuild 该成功: %v", err)
	}
	if p.Title != "妈妈" || !p.Admin || p.PIN != h {
		t.Errorf("回显与递进来的不一致: %+v", p)
	}
	// 这一条必须排在**任何其它读之前**检查：清登记走的也是 loadProfile 里那条
	// clearBroken，而下面为了断言默认值还要再读一次——先读后查等于测试自己把登记清掉，
	// 「实现里写完不回读」那把枪就永远打不红（S3 实测抓到过这件事）。
	if got := s.Broken(); len(got) != 0 {
		t.Errorf("重建后坏档登记还在: %v——写完那次锁内回读没生效（或 clearBroken 挂错了层）", got)
	}
	assertRosterUntouched(t, s, "mom", before)
	got, err := s.LoadProfile("mom")
	if err != nil {
		t.Fatalf("重建后读不动: %v", err)
	}
	if *got != *p {
		t.Errorf("返回的档案与盘上那份不是同一份: %+v vs %+v", p, got)
	}
	// theme/layout/createdAt 按默认重建是能力边界，但必须**是**默认值而不是空串：
	// 空串会让 §4.3 的校验与进门页的 themeAttr 各说各话。
	if got.Theme != "auto" || got.Layout != "sidebar" {
		t.Errorf("默认值没落地: theme=%q layout=%q", got.Theme, got.Layout)
	}
	if _, err := time.Parse(time.RFC3339, got.CreatedAt); err != nil {
		t.Errorf("createdAt 该是 RFC3339（不是空串、不是别的形状）: %q", got.CreatedAt)
	}
}

func TestRebuild_读得动的档案不许被盖(t *testing.T) {
	s, _ := seedBroken(t, "mom") // mom 坏着，dad 健康：一份盘上同时有两种结局
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	path := s.ProfilePath("dad")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RebuildProfile(RebuildInput{ID: "dad", Title: "改名", PinHash: authHash(t)}); !errors.Is(err, ErrReadable) {
		t.Fatalf("读得动该 ErrReadable，得到 %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("ErrReadable 那一支还是把文件写了：%q -> %q", before, after)
	}
	// 反向诱饵：证明上面那条**看得见写**——写一旦发生，这份夹具一定看得见。
	//
	// 判据只用**内容**，不碰文件元数据。这不是图省事，是两种元数据在容器上都被
	// 实测证伪了（同一份代码在 NTFS 上绿、在 `docker build` 的镜像门禁里红，而红
	// 的从来不是生产代码，是这两条判据本身）：
	//   · mtime：overlayfs 粒度是整秒，同一瞬间连写两次得到的时间戳逐位相同；
	//   · inode：号码会从小池子里回收（实测 139504 → 139505 → 139504），而
	//     WriteFileAtomic 的临时文件每次 rename 走都释放一个号，重建那次恰好
	//     领回基准那一个，SameFile 于是把一次真写报成"没动"。
	//
	// 内容判据在这里是**完备**的：ErrReadable 那一支若误写，写下去的是递进来的
	// 那份（Title "改名"），字节必变；真想写出与原来一模一样的字节，就得把刚读
	// 出来的那份原样存回去，而 store 的每一次写都是 WriteFileAtomic 的
	// 「同目录临时文件 + rename」，没有原地重写的路可走。
	breakProfileJSON(t, s, "dad")
	if _, err := s.RebuildProfile(RebuildInput{ID: "dad", Title: "爸爸", PinHash: authHash(t)}); err != nil {
		t.Fatalf("坏掉之后该能重建: %v", err)
	}
	rebuilt, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("重建后读不动: %v", err)
	}
	if string(rebuilt) == string(before) {
		t.Error("重建之后内容还是原来那份——这份夹具证明不了写会发生，上面那条断言是恒真")
	}
}

func TestRebuild_读得动时字段非法也先说读得动(t *testing.T) {
	// 设计 §2.1 的锁内步序：第 1 步是「读得动吗」，第 2 步才是校验。健康档案 + 空 title
	// 必须答 ErrReadable（handler 答 409），不是 ErrInvalid（422）——「想拿这条通路改档」
	// 才是调用方真正的错处。枪 S8：把 Validate 挪回判据之前 → 这条红。
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.ProfilePath("dad"))
	if _, err := s.RebuildProfile(RebuildInput{ID: "dad", Title: "   ", PinHash: authHash(t)}); !errors.Is(err, ErrReadable) {
		t.Fatalf("读得动 + 字段非法该先答 ErrReadable，得到 %v", err)
	}
	after, _ := os.ReadFile(s.ProfilePath("dad"))
	if string(before) != string(after) {
		t.Errorf("拒绝重建的那一支把文件写了: %q -> %q", before, after)
	}
}

func TestRebuild_半件事目录可以重建且不碰书签(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "kid", Title: "小满", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	before := seedScene(t, s, "kid") // 书签快照 + 一行历史，与 mom 那条同一份现场形状
	if err := os.Remove(s.ProfilePath("kid")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RebuildProfile(RebuildInput{ID: "kid", Title: "小满", PinHash: authHash(t)}); err != nil {
		t.Fatalf("目录在、profile.json 不在该允许: %v", err)
	}
	assertRosterUntouched(t, s, "kid", before)
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatalf("重建后读不动: %v", err)
	}
}

func TestRebuild_半件事目录不顺手建书签(t *testing.T) {
	// 设计 §7 第 3 条的后半句是「只多一个 profile.json」。上一条里书签本来就在，比对字节
	// 抓不到「书签不在时被顺手建了一份」——那正是「复用 CreateProfile 来实现 rebuild」的
	// 形状（它无条件写一份 bookmarks.json）。枪 S6 打的也是这一条。
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "kid", Title: "小满", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{s.ProfilePath("kid"), s.BookmarksPath("kid")} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if got := dirNames(t, s.ProfileDir("kid")); len(got) != 0 {
		t.Fatalf("夹具该把两份文件都摘掉，实际还剩 %v", got)
	}
	if _, err := s.RebuildProfile(RebuildInput{ID: "kid", Title: "小满", PinHash: authHash(t)}); err != nil {
		t.Fatalf("重建该成功: %v", err)
	}
	if got := dirNames(t, s.ProfileDir("kid")); len(got) != 1 || got[0] != "profile.json" {
		t.Errorf("目录里应该是恰好一个 profile.json，实际 %v", got)
	}
}

func TestRebuild_目录不存在时不建目录(t *testing.T) {
	s := New(t.TempDir())
	_, err := s.RebuildProfile(RebuildInput{ID: "ghost", Title: "幽灵", PinHash: authHash(t)})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("没这份档案该 ErrNotFound，得到 %v", err)
	}
	// ErrNotFound 只说「ghost 不是目录」，它挡住不了「把 ghost 写成一份文件」那种实现，
	// 所以要单独断盘上什么都没多出来（评审 Important 1 补的缺口）。
	if got := dirNames(t, filepath.Join(s.root, "profiles")); len(got) != 0 {
		t.Errorf("profiles/ 里多出了 %v——这条通路变成第二条建档通路了", got)
	}
	if raw, statErr := os.ReadFile(filepath.Join(s.root, "profiles", "ghost")); statErr == nil {
		t.Errorf("rebuild 写出了 profiles/ghost 这个文件: %q", raw)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("profiles/ghost 的 stat 得到意外错误: %v", statErr)
	}
}

func TestRebuild_档案位置上是个文件时报没这份档案(t *testing.T) {
	// `idTaken` 验的是"是个目录"。裸 os.Stat 会在 profiles/ghost 这份**文件**上放行，
	// 而 ErrNotFound 那句判据说的是"没这份档案"——枪 S9 把 idTaken 换成裸 Stat 就红在这里。
	s := New(t.TempDir())
	ghost := filepath.Join(s.root, "profiles", "ghost")
	if err := os.MkdirAll(filepath.Dir(ghost), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ghost, []byte("我不是目录"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.RebuildProfile(RebuildInput{ID: "ghost", Title: "幽灵", PinHash: authHash(t)})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("位置上是文件该 ErrNotFound，得到 %v", err)
	}
	raw, statErr := os.ReadFile(ghost)
	if statErr != nil {
		t.Fatalf("那份文件不见了: %v", statErr)
	}
	if string(raw) != "我不是目录" {
		t.Errorf("rebuild 改了位置上的文件: %q", raw)
	}
}

func TestRebuild_非内容错的读失败不许覆盖(t *testing.T) {
	s, _ := seedBroken(t, "mom")
	// 把 profile.json 换成一个「读不动但不是坏 JSON」的形状：一个名叫 profile.json 的
	// 目录。os.ReadFile 对目录返回 I/O 错，既不是 ErrCorrupt 也不是 ErrNotFound——
	// 这一支必须与那两种分开。
	if err := os.Remove(s.ProfilePath("mom")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.ProfilePath("mom"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 先拿到「读不动」的那句原文。这条测试断的是**原样上报**（设计 §2.1 第 5 步），
	// 不是「最后反正也失败了」：这一份夹具里覆盖也会失败（往目录里写），所以只断
	// 「有没有报错」的版本，删掉守卫那把枪（S1）照样绿——比对错误原文才抓得到它。
	_, readErr := s.LoadProfile("mom")
	if readErr == nil {
		t.Fatal("夹具没让 profile.json 变坏——这条测试什么都没测")
	}
	if errors.Is(readErr, ErrCorrupt) {
		t.Fatalf("夹具造出来的是内容错(%v)，不是读失败——这条测试在测另一件事", readErr)
	}
	if errors.Is(readErr, ErrNotFound) {
		t.Fatalf("夹具造出来的是文件不在(%v)，不是读失败——这条测试在测另一件事", readErr)
	}
	_, err := s.RebuildProfile(RebuildInput{ID: "mom", Title: "妈妈", PinHash: authHash(t)})
	if err == nil || errors.Is(err, ErrReadable) {
		t.Fatalf("读失败该原样上报，得到 %v", err)
	}
	if errors.Is(err, ErrCorrupt) {
		t.Errorf("把读失败说成内容错：%v", err)
	}
	if err.Error() != readErr.Error() {
		t.Errorf("没原样上报：得到 %v，而读失败的原文是 %v", err, readErr)
	}
	if fi, statErr := os.Stat(s.ProfilePath("mom")); statErr != nil || !fi.IsDir() {
		t.Error("读失败那一支把目录/文件改了——覆盖就是二次事故")
	}
}

func TestRebuild_校验排在落盘之前(t *testing.T) {
	s, before := seedBroken(t, "mom")
	h := authHash(t)
	for _, tc := range []struct{ name, id, title string }{
		{"id 不是 slug", "Bad ID", "x"},
		{"id 越界", "../escape", "x"},
		{"title 全空", "mom", "   "},
		{"缺 PIN 哈希", "mom", "妈妈"},
	} {
		in := RebuildInput{ID: tc.id, Title: tc.title, PinHash: h}
		if tc.name == "缺 PIN 哈希" {
			in.PinHash = ""
		}
		if _, err := s.RebuildProfile(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s 该 ErrInvalid，得到 %v", tc.name, err)
		}
	}
	assertRosterUntouched(t, s, "mom", before)
	if got := dirNames(t, filepath.Join(s.root, "profiles")); len(got) != 1 || got[0] != "mom" {
		t.Errorf("越界与非法 id 在盘上留下了东西: %v", got)
	}
}

func TestRebuild_并发恰好一路写完(t *testing.T) {
	s, before := seedBroken(t, "mom")
	h := authHash(t)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.RebuildProfile(RebuildInput{ID: "mom", Title: "妈妈", PinHash: h, Admin: i%2 == 0})
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil && !errors.Is(e, ErrReadable) {
			t.Errorf("第 %d 路得到 %v", i, e)
		}
	}
	// 持锁时**恰好一路**能写完，其余三路都在它之后看见「读得动」。这一条是本机唯一能钉住
	// 「判据与写入同属一次串行化」（权威源 §4.5）的证人：去掉 WithProfileLock 会有两路
	// 同时读到坏档、同时覆盖（枪 S7 红在这里）。`-race` 归容器那一路（Task 6）。
	n := 0
	for _, e := range errs {
		if e == nil {
			n++
		}
	}
	if n != 1 {
		t.Errorf("四路里 %d 路成功，要恰好 1 路: %v", n, errs)
	}
	if _, err := s.LoadProfile("mom"); err != nil {
		t.Fatalf("并发之后档案仍读不动: %v", err)
	}
	raw, err := os.ReadFile(s.ProfilePath("mom"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "}\n") {
		t.Errorf("profile.json 末尾形状不对（§4.3 要恰好一个换行）: %q", raw[max(0, len(raw)-8):])
	}
	assertRosterUntouched(t, s, "mom", before)
}
