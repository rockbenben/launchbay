package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newCache(t *testing.T) (*IconCache, *Store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	s := New(t.TempDir())
	c, err := s.LoadIconCache()
	if err != nil {
		t.Fatalf("空目录也要能起一个空缓存: %v", err)
	}
	c.now = func() time.Time { return now }
	return c, s, &now
}

// assertOriginsSorted 解出 index.json 的 entries 并验 origin 严格升序。
// 这是 sort.Slice 的证人：删掉排序，三次键的 map 遍历恰好全序的概率约
// 1/6、四键约 1/24——所以这个检查在同一份测试里跑两次（第二次排在
// 第 4 个 origin 的 Put 之后），两次独立遍历都碰巧有序的概率降到约
// 1/144，排序才真的有人盯着。
func assertOriginsSorted(t *testing.T, raw []byte) {
	t.Helper()
	var f struct {
		Entries []struct {
			Origin string `json:"origin"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("index.json 解不出来: %v", err)
	}
	for i := 1; i < len(f.Entries); i++ {
		if f.Entries[i-1].Origin >= f.Entries[i].Origin {
			t.Fatalf("entries 不是按 origin 升序: [%d]=%q 在 [%d]=%q 前",
				i-1, f.Entries[i-1].Origin, i, f.Entries[i].Origin)
		}
	}
}

func TestIconCache_字节一致只存一份(t *testing.T) {
	c, s, _ := newCache(t)
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}

	e1, err := c.Put("http://10.0.0.3:8096", png, "png")
	if err != nil {
		t.Fatal(err)
	}
	e2, err := c.Put("http://10.0.0.9:8080", png, "png") // 同一张图，另一个服务
	if err != nil {
		t.Fatal(err)
	}
	if e1.Hash != e2.Hash {
		t.Errorf("同字节必须同哈希: %s vs %s", e1.Hash, e2.Hash)
	}
	ents, _ := os.ReadDir(s.IconsDir())
	var blobs int
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), "index.json") {
			blobs++
		}
	}
	if blobs != 1 {
		t.Errorf("内容寻址的意义就是不去重就白占盘，得到 %d 个 blob", blobs)
	}
	got, err := os.ReadFile(s.IconBlobPath(e1.Hash, e1.Ext))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(png) {
		t.Error("blob 字节被改过：内容哈希就不是原字节了")
	}
}

func TestIconCache_哈希与扩展名就是文件名形状(t *testing.T) {
	c, s, _ := newCache(t)
	e, err := c.Put("http://10.0.0.3:8096", []byte("x"), "ico")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Hash) != 16 || strings.TrimRight(e.Hash, "0123456789abcdef") != "" {
		t.Errorf("hash %q 要 16 位小写十六进制——Task 11 的路径校验靠这个形状", e.Hash)
	}
	if filepath.Base(s.IconBlobPath(e.Hash, e.Ext)) != e.Hash+".ico" {
		t.Error("blob 文件名要能被 /icons/{name} 原样认出来")
	}
}

func TestIconCache_代次只在真变化时递增(t *testing.T) {
	c, _, _ := newCache(t)
	g0 := c.Generation()
	if _, err := c.Put("http://10.0.0.3:8096", []byte("a"), "png"); err != nil {
		t.Fatal(err)
	}
	g1 := c.Generation()
	if g1 == g0 {
		t.Fatal("缓存变了而代次没动，首屏会一直端出旧页（spec §5 第 5 条）")
	}
	if _, err := c.Put("http://10.0.0.3:8096", []byte("a"), "png"); err != nil { // 同一 origin 同一字节
		t.Fatal(err)
	}
	if c.Generation() != g1 {
		t.Error("同一张图重复上报不该让所有页面重算")
	}
}

func TestIconCache_失败冷却24小时(t *testing.T) {
	c, _, now := newCache(t)
	if err := c.MarkFail("http://10.0.0.7/"); err != nil {
		t.Fatal(err)
	}
	if !c.InCooldown("http://10.0.0.7/") {
		t.Error("刚失败就该在冷却里")
	}
	*now = now.Add(23 * time.Hour)
	if !c.InCooldown("http://10.0.0.7/") {
		t.Error("23 小时还在冷却")
	}
	*now = now.Add(2 * time.Hour)
	if c.InCooldown("http://10.0.0.7/") {
		t.Error("25 小时后应当允许再试一次")
	}
	// 成功过一次的 origin 不受失败记录影响
	if _, err := c.Put("http://10.0.0.7/", []byte("z"), "png"); err != nil {
		t.Fatal(err)
	}
	if c.InCooldown("http://10.0.0.7/") {
		t.Error("已经拿到图了还冷却，等于把自己锁死")
	}
}

func TestIconCache_落盘可复读且键序稳定(t *testing.T) {
	c, s, _ := newCache(t)
	for _, o := range []string{"http://10.0.0.9/", "http://10.0.0.3/", "http://10.0.0.7/"} {
		if _, err := c.Put(o, []byte(o), "png"); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(s.IconIndexPath())
	if err != nil {
		t.Fatal(err)
	}
	// 复读不 bump 代次（裁定 R2：读盘不算变化）。
	again, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	if again.Generation() != 0 {
		t.Errorf("复读出来的缓存代次 = %d，要 0（读盘不算变化）", again.Generation())
	}
	if !strings.Contains(string(raw), `"origin": "http://10.0.0.3/"`) {
		t.Errorf("index.json 形状不对: %s", raw)
	}
	second, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	// 排序证人第一次：升序。
	assertOriginsSorted(t, raw)
	// 第 4 个 origin 触发第二次真实落盘——独立的第二次 map 遍历，
	// 排序证人第二次。
	if _, err := second.Put("http://10.0.0.5/", []byte("4"), "png"); err != nil {
		t.Fatal(err)
	}
	third, err := os.ReadFile(s.IconIndexPath())
	if err != nil {
		t.Fatal(err)
	}
	assertOriginsSorted(t, third)
}

// 原名坏 index.json 里的点在 Go 标识符里非法，落地时改成下划线。
func TestIconCache_坏index_json不许静默清空(t *testing.T) {
	s := New(t.TempDir())
	if err := os.MkdirAll(s.IconsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.IconIndexPath(), []byte("{半"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadIconCache(); err == nil {
		t.Fatal("读坏了还返回一个空缓存，等于把整站图标缓存清零——宁可启动失败")
	}
}

func TestIconCache_Lookup只认成功项(t *testing.T) {
	c, _, _ := newCache(t)
	if _, ok := c.Lookup("http://10.0.0.3:8096"); ok {
		t.Error("没存过不该有")
	}
	if err := c.MarkFail("http://10.0.0.3:8096"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup("http://10.0.0.3:8096"); ok {
		t.Error("失败记录不是图标")
	}
}

// TestIconCache_带failAt的可用项不锁人 是 InCooldown 里 `e.Hash != ""` 判据
// 的唯一证人（裁定 Step 8 第 2 条的落地修正）。常规写通路造不出"同条目既有
// Hash 又有 FailAt"的状态——Put 换整个条目（FailAt 被清），MarkFail 见有图
// 早退；这种冲突条目只可能来自手改/旧版写的 index.json。裁决口径：有图优先，
// 否则一个多余的 failAt 会把可用的缓存白白锁 24 小时。
// 没有这条测试，破坏第 2 条（删 e.Hash != "" 判据）打不中任何东西。
func TestIconCache_带failAt的可用项不锁人(t *testing.T) {
	s := New(t.TempDir())
	if err := os.MkdirAll(s.IconsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	e := IconEntry{Origin: "http://10.0.0.7/", Hash: "0123456789abcdef", Ext: "png",
		At:     now.Format(time.RFC3339),
		FailAt: now.Add(-time.Hour).Format(time.RFC3339)} // 一小时前的失败，24 小时内
	if err := s.WriteFileJSON(s.IconIndexPath(), iconIndexFile{Entries: []IconEntry{e}}); err != nil {
		t.Fatal(err)
	}
	c, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	if c.InCooldown("http://10.0.0.7/") {
		t.Error("有图条目被 failAt 判进冷却：已经拿到图的 origin 等于把自己锁死")
	}
	if _, ok := c.Lookup("http://10.0.0.7/"); !ok {
		t.Error("冲突条目里图那半边丢了")
	}
}

// TestIconCache_重复同一字节要端回盘上那个时间戳 钉 B5：Put 命中"完全没变"
// 分支时返回的必须是**旧条目**——修前返回的是带着新 At 的新条目（那个时间戳
// 盘上根本没有），等于对调用方端出一个假时间戳。夹具把假时钟推进 48 小时再
// 重复同一次 Put，两个 At 才分得开。
func TestIconCache_重复同一字节要端回盘上那个时间戳(t *testing.T) {
	c, _, now := newCache(t)
	png := []byte{0x89, 'P', 'N', 'G', 1}
	first, err := c.Put("http://10.0.0.3:8096", png, "png")
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(48 * time.Hour)
	again, err := c.Put("http://10.0.0.3:8096", png, "png")
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Errorf("完全没变的 Put 端出了盘上没有的条目：At %q vs %q", again.At, first.At)
	}
}

// TestIconCache_saveLocked失败必须回滚内存镜像 钉 B6：Put 里 blob 落盘成功、
// index.json 落不下去时，内存镜像必须退回写入前的样子——否则 byID 声称有、
// 盘上没有、gen 又不动，Lookup 会把这个谎继续端给后面的 resolve，而 resolve
// 的 MarkFail 因 `Hash != ""` 变成静默 no-op。夹具：把 index.json 的位置占
// 成目录，WriteFileAtomic 的 rename 撞目录必败（store/profile_test.go 同款
// 手法，不去改 store 代码做注入点）。证人两半：新 origin 回滚到"不存在"；
// 已有 origin 改图失败后回滚到**旧条目**而不是丢也不是新条目。
// MarkFail 与 Put 是**同一条不变式**：byID 绝不能声称一条盘上的 index.json
// 里不存在的记录。Put 一直守着，MarkFail 漏了（2026-10-19 修）。
//
// 漏了的后果不是"脏内存"这么轻：那条记录是**冷却**，于是这个 origin 在本次
// 进程里被记 24 小时不重抓（InCooldown 为真），重启即失效——冷却本该跨重启
// 生效。它悄悄退化成了进程内状态，症状是"图标偶尔抓不到，重启一次又好"。
//
// 夹具与上面那条同款：把 index.json 的位置占成目录，rename 撞目录必败。
func TestIconCache_MarkFail落盘失败回滚冷却(t *testing.T) {
	c, s, _ := newCache(t)
	// 先让一个 origin 有图：它那条 MarkFail 是静默 no-op（失败不该抹掉可用的缓存），
	// 所以另取一个**没有图**的 origin 来触发真正写盘的那一支
	if _, err := c.Put("http://10.0.0.3:8096", []byte("a"), "png"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.IconIndexPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.IconIndexPath(), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := c.MarkFail("http://10.0.0.9:8080"); err == nil {
		t.Fatal("index.json 落不下去而 MarkFail 说成功：成功是假的")
	}
	if _, ok := c.Lookup("http://10.0.0.9:8080"); ok {
		t.Error("saveLocked 失败后内存镜像仍记着冷却：盘上没有，重启就失效，冷却悄悄退化成了进程内状态")
	}
	if c.InCooldown("http://10.0.0.9:8080") {
		t.Error("回滚之后这个 origin 还在冷却里")
	}
}

func TestIconCache_saveLocked失败必须回滚内存镜像(t *testing.T) {
	c, s, _ := newCache(t)
	old, err := c.Put("http://10.0.0.3:8096", []byte("a"), "png")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.IconIndexPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.IconIndexPath(), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Put("http://10.0.0.9:8080", []byte("b"), "png"); err == nil {
		t.Fatal("index.json 落不下去而 Put 说成功：成功是假的")
	}
	if _, ok := c.Lookup("http://10.0.0.9:8080"); ok {
		t.Error("saveLocked 失败后内存镜像声称有这个 origin——盘上没有，Lookup 在端谎")
	}

	if _, err := c.Put("http://10.0.0.3:8096", []byte("c"), "png"); err == nil {
		t.Fatal("同上，已有 origin 的失败也必须报出来")
	}
	got, ok := c.Lookup("http://10.0.0.3:8096")
	if !ok {
		t.Error("回滚把原本健康的旧条目也抹了：失败前它是在盘上的")
	} else if got.Hash != old.Hash {
		t.Errorf("失败后镜像里留着没落盘的新条目：hash %q，要旧的 %q", got.Hash, old.Hash)
	}
}
