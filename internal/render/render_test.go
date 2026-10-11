package render

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

func seeded(t *testing.T) *store.Store {
	t.Helper()
	s := store.New(t.TempDir())
	for _, id := range []string{"dad", "mom"} {
		if _, err := s.CreateProfile(store.NewProfileInput{ID: id, Title: id, PinHash: "sha256$a$b"}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func save(t *testing.T, s *store.Store, id, title, itemID string) {
	t.Helper()
	b := store.EmptyBookmarks()
	b.Rev = 5
	// 标记串放进**分组标题**而不是书签文档的 Title：顶栏那一格现在印的是
	// 档案名（Profile.Title），书签 Title 已经不上屏了，拿它当探针等于测一个
	// 渲染不出来的字段。
	// 标记串放进**条目的 desc**而不是书签文档的 Title：顶栏那一格现在印的是档案名
	// （Profile.Title），书签 Title 已经不上屏了，拿它当探针等于测一个渲染不出来的字段。
	// 分组标题与条目名字各自保持原样——另有证人钉着它们。
	b.Title = "发射台"
	b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList, Items: []model.Item{
		{ID: itemID, Name: itemID + " 台", Desc: title, URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
	}}}
	if err := s.SaveBookmarks(id, b); err != nil {
		t.Fatal(err)
	}
	// 让两个档案的 mtime 一定不同，否则缓存键会真的撞。
	time.Sleep(30 * time.Millisecond)
}

// 本计划最关键的一条：两档案 rev 相同。只按 rev 做键的话，
// 第二个档案会命中第一个的页面——这就是"串号"。
func TestTwoProfilesSameRevDoNotCrossContaminate(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	save(t, s, "mom", "妈妈的台", "emby")
	r := renderer(t, s)

	dad, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	mom, err := r.ProfilePage(PageRequest{ID: "mom"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dad.Body), "爸爸的台") || strings.Contains(string(dad.Body), "妈妈的台") {
		t.Fatalf("dad 页面串了:\n%s", dad.Body)
	}
	if !strings.Contains(string(mom.Body), "妈妈的台") || strings.Contains(string(mom.Body), "爸爸的台") {
		t.Fatalf("mom 页面串了:\n%s", mom.Body)
	}
	if dad.ETag == mom.ETag {
		t.Fatalf("两档案 ETag 相同，浏览器缓存会跨档案命中: %s", dad.ETag)
	}
	if dad.Profile != "dad" || mom.Profile != "mom" {
		t.Fatalf("Page.Profile 要给 handler 写 X-LB-Profile 用: %q %q", dad.Profile, mom.Profile)
	}
}

// 内容字节完全相同的两个档案，ETag 也必须不同。
func TestIdenticalContentStillDistinctETag(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "同一份", "dsm")
	src, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	clone := *src
	if err := s.SaveBookmarks("mom", &clone); err != nil {
		t.Fatal(err)
	}
	// Task 11 的模板会把 Profile.Title 排版进页面：要让两页字节相同，
	// 档案显示名也必须一致，光书签内容一致已经不够。
	dp, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	mp, err := s.LoadProfile("mom")
	if err != nil {
		t.Fatal(err)
	}
	mp.Title = dp.Title
	if err := s.SaveProfile(mp); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	dad, _ := r.ProfilePage(PageRequest{ID: "dad"})
	mom, _ := r.ProfilePage(PageRequest{ID: "mom"})
	// **"两页字节相同"这个前提在 ⑤ 之后不成立了**：body 上的 data-profile 装的是
	// 档案 id（添加浮层提交时要用它带 profile），两个档案因此必然不同。
	//
	// 少掉的那份覆盖要去别处补回来：ETag 的档案名前缀从"内容可能撞车时的救命绳"
	// 变成"与页面里那个 id 对齐的标记"，而**跨档案同页缓存**这件事改由 data-profile
	// 本身挡着——它在字节里，不在 ETag 里。下面这两条就是在钉它。
	for name, p := range map[string]Page{"dad": dad, "mom": mom} {
		if !strings.Contains(string(p.Body), `data-profile="`+name+`"`) {
			t.Errorf("%s 那页的 body 里没有 data-profile=\"%s\"——它正是把两个档案的首屏挡在\n"+
				"各自缓存里的那一维；没有它，串号就不只依赖 ETag 了", name, name)
		}
	}
	if dad.ETag == mom.ETag {
		t.Fatal("ETag 要带档案名，内容相同也不许跨档案相同")
	}
}

func TestCacheHitAndExternalEditInvalidates(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "第一版", "dsm")
	r := renderer(t, s)

	first, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := r.ProfilePage(PageRequest{ID: "dad"})
	if first.ETag != second.ETag {
		t.Fatal("同内容不该产生不同 ETag")
	}
	if r.CacheSize() == 0 {
		t.Fatal("没进缓存")
	}

	// 外部手改基线文件（rev 不变）：mtime 变了就必须失效重算。
	raw := strings.Replace(string(mustRaw(t, s, "dad")), "第一版", "第二版", 1)
	time.Sleep(30 * time.Millisecond)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte(raw)); err != nil {
		t.Fatal(err)
	}
	third, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(third.Body), "第二版") {
		t.Fatalf("手改基线后仍在给旧内容:\n%s", third.Body)
	}
	if third.ETag == first.ETag {
		t.Fatal("内容变了 ETag 却没变")
	}
}

func TestInvalidateDropsOnlyThatProfile(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "甲", "dsm")
	save(t, s, "mom", "乙", "emby")
	r := renderer(t, s)
	if _, err := r.ProfilePage(PageRequest{ID: "dad"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ProfilePage(PageRequest{ID: "mom"}); err != nil {
		t.Fatal(err)
	}
	if r.CacheSize() != 2 {
		t.Fatalf("cache = %d, want 2", r.CacheSize())
	}
	r.Invalidate("dad")
	if r.CacheSize() != 1 {
		t.Fatalf("Invalidate 该只清一个档案, cache = %d", r.CacheSize())
	}
}

func TestProfilePage_改profile会重算(t *testing.T) {
	s := seeded(t)
	r := renderer(t, s)
	first, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(first.Body, []byte(`data-theme="light"`)) {
		t.Fatal("夹具一开始就是 light，换成 light 就测不出变化了")
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	p.Theme = "light"
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	second, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(second.Body, []byte(`data-theme="light"`)) {
		t.Errorf("改了 profile.json 而页面还是旧的：缓存键里没有 profile.json 的指纹")
	}
}

// 上面那条只证明"这一维在"，证明不了它**扛得住时间戳粒度**。P1-④ 验收在 Linux
// 容器里实测：同一时钟刻内两次原子替换，profile.json 的 mtime 纳秒级完全相同
// （CIFS/9p 挂载更常只到秒），于是"靠 mtime 当指纹"的键在真部署上会让家属
// 按了主题按钮而页面不动。这里把 mtime 钉回改动前那个值，就把这件事变成任何
// 文件系统上都能确定性复现的形状：内容变了、mtime 没变，页面**必须**还是重算过。
func TestProfilePage_mtime没变而内容变了要重算(t *testing.T) {
	s := seeded(t)
	r := renderer(t, s)
	first, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(first.Body, []byte(`data-theme="light"`)) {
		t.Fatal("夹具一开始就是 light，换成 light 就测不出变化了")
	}
	path := s.ProfilePath("dad")
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	before := st.ModTime()
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	p.Theme = "light"
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	// 把时间戳按回去：模拟"改了内容但文件系统给不出新时间戳"。
	if err := os.Chtimes(path, before, before); err != nil {
		t.Fatal(err)
	}
	st2, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	after := st2.ModTime()
	if !after.Equal(before) {
		t.Fatalf("夹具没造出目标形状：mtime 没按回去（%v → %v）", before, after)
	}
	second, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(second.Body, []byte(`data-theme="light"`)) {
		t.Errorf("内容变了而 mtime 没变，页面还是旧的：profile.json 那一维不能只靠时间戳")
	}
	if first.ETag == second.ETag {
		t.Errorf("两次的 ETag 相同（%s）——家属的浏览器会拿 304 继续显示旧页", first.ETag)
	}
}

func TestProfilePage_带q不进缓存但照发ETag(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	save(t, s, "mom", "妈妈的台", "emby")
	r := renderer(t, s)

	a1, err := r.ProfilePage(PageRequest{ID: "dad", Query: "dsm"})
	if err != nil {
		t.Fatal(err)
	}
	if n := r.CacheSize(); n != 0 {
		t.Errorf("带 q 的渲染进了内存缓存（%d 条）：键空间会随击键膨胀", n)
	}
	if a1.ETag == "" {
		t.Error("旁路不等于不发 ETag：浏览器 304 那一路照样要走")
	}
	a2, _ := r.ProfilePage(PageRequest{ID: "dad", Query: "dsm"})
	if a2.ETag != a1.ETag {
		t.Errorf("同一个 q 两次渲染 ETag 不同（%q vs %q）：304 永远命不中", a1.ETag, a2.ETag)
	}
	b, err := r.ProfilePage(PageRequest{ID: "dad", Query: "emby"})
	if err != nil {
		t.Fatal(err)
	}
	// "emby" 在 dad 的台里一个都不命中，它**同样不许进缓存**——旁路是每条带 q 的
	// 请求都走，不是只有第一条。"不同 q 不许互相命中"这一维不在这里：ETag 是内容哈希
	// （spec §7.1），把查询词折进哈希会误杀"两个 URL 同一份表示"这种合法情形，
	// 那条判据由 views_test.go 的 TestProfilePage_不同q不互相命中 守（Task 2 落地了过滤）。
	if b.ETag == "" {
		t.Error("第二条带 q 的请求没发 ETag：旁路不等于不发")
	}
	if n := r.CacheSize(); n != 0 {
		t.Errorf("第二条带 q 的请求也占进了缓存（%d 条）：旁路只对第一条生效", n)
	}
	// 不带 q 的那一条照旧进缓存，且与带 q 的互不干扰。
	plain, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if n := r.CacheSize(); n != 1 {
		t.Errorf("不带 q 的渲染该占一条缓存，got %d", n)
	}
	// 不带 q 的照旧端出全量页面：旁路只关带 q 的请求，不许把缓存写坏成"谁都拿空页"。
	if plain.ETag == "" || !bytes.Contains(plain.Body, []byte("dsm 台")) {
		t.Errorf("全量页缺 ETag 或没端出条目（etag=%q）", plain.ETag)
	}
}

func TestBrokenProfileSurfacesReason(t *testing.T) {
	s := seeded(t)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{\n \"rev\": 2,\n")); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	_, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err == nil {
		t.Fatal("坏档案要报错，让 handler 走坏档案页")
	}
	if !strings.Contains(err.Error(), "第 ") {
		t.Fatalf("错误要带行号: %v", err)
	}
}

func TestPickerPageListsNamesOnly(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	r := renderer(t, s)
	p, err := r.PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)
	if !strings.Contains(body, "dad") {
		t.Fatalf("选择页要列出档案: %s", body)
	}
	// 关键隐私断言：选择页一个条目、一个 URL、一个分组名都不许有。
	for _, forbidden := range []string{"10.0.0.3", "爸爸的台", "NAS", "dsm 台"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("选择页泄露了 %q:\n%s", forbidden, body)
		}
	}
	if p.Profile != "picker" {
		t.Fatalf("Profile = %q", p.Profile)
	}
	if !p.NoStore {
		t.Fatal("选择页也要 NoStore 级隐私处理")
	}
}

func mustRaw(t *testing.T, s *store.Store, id string) []byte {
	t.Helper()
	b, err := os.ReadFile(s.BookmarksPath(id))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
