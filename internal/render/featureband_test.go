package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 大卡那一档（2026-10-14）：设成 `feature` 的分组**从正文搬进顶部那条带子**，
// 而不是再画一遍。
//
// 这条判据钉的是「搬走」：同一组在一页里出现两次，那才是真的坏——它会让
// "点哪个"变成一个问题，而这一页的全部意义是让人不用想。
func Test大卡那一组从正文搬进顶部而不是复制(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 3
	b.Groups = []model.Group{
		{ID: "hot", Title: "每天必点", Layout: model.LayoutFeature,
			Items: []model.Item{{ID: "h1", Name: "NAS", URL: "http://192.168.2.3:5000"}}},
		{ID: "cold", Title: "一年一次", Layout: model.LayoutTiles,
			Items: []model.Item{{ID: "c1", Name: "旧的", URL: "http://192.168.2.3:9000"}}},
	}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	p, err := renderer(t, s).ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)

	if n := strings.Count(body, `data-gid="hot"`); n != 1 {
		t.Errorf("「每天必点」在页面上出现了 %d 次，要 1 次（搬走不是复制）：\n%s", n, bandOnly(body))
	}
	if !strings.Contains(body, `<section class="feat"`) {
		t.Fatalf("没有大卡带：\n%s", bandOnly(body))
	}
	// 正文的分组序列里不该再有它：判据是「它在 .feat 里，而不在普通 section 里」。
	if strings.Contains(body, `<section aria-labelledby="g-hot"`) {
		t.Error("大卡那一组同时还在正文里：同一组一页两处")
	}
	// 正向证人：另一个分组照旧在正文里。
	if !strings.Contains(body, `<section aria-labelledby="g-cold"`) {
		t.Error("没设大卡的那一组被一起搬走了")
	}
}

// 大卡那一组**不许从分组导航里消失**——它只是换了位置。
func Test大卡那一组仍然在分组导航里(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 3
	b.Groups = []model.Group{
		{ID: "hot", Title: "每天必点", Layout: model.LayoutFeature,
			Items: []model.Item{{ID: "h1", Name: "NAS", URL: "http://192.168.2.3:5000"}}},
	}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	p, err := renderer(t, s).ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.Body), `#g-hot`) {
		t.Error("大卡那一组从分组导航里消失了：换位置不等于换存在")
	}
}

// 大卡那一行灰色小字给的是**主机名**，而 `chrome://` 那一类给的是原地址。
//
// 2026-10-14 实拍踩过：`url.Parse("chrome://extensions/")` 把 `extensions`
// 当成 Host，于是那一行显示成「extensions」——看上去像一个域名，而它是
// 这条协议的路径段。
func Test大卡的主机名只认http与https(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"http://192.168.2.3:5000", "192.168.2.3:5000"},
		{"https://github.com/a/b", "github.com"},
		{"chrome://extensions/", "chrome://extensions/"},
		{"chrome://apps", "chrome://apps"},
		{"file:///D:/x.html", "file:///D:/x.html"},
		{"不是地址", "不是地址"},
	} {
		if got := hostOf(tc.raw); got != tc.want {
			t.Errorf("hostOf(%q) = %q，要 %q", tc.raw, got, tc.want)
		}
	}
}

// 多个分组都设成大卡时，它们**并进同一块**，而不是各占一块。
//
// 2026-10-15 改判：原来一个分组一块，于是想把两组都提上来就得到两块一模一样的
// 带子、中间隔一条线——那不是"一个放大的区域"，是两块小区域。现在所有大卡分组
// 共处一块，跨分组一起扫过去不再断开；分组名从整块的 h2 降成块内的 `.feat-g`，
// **合并是为了扫得更顺，不是为了把分组信息丢掉**——所以段数仍然等于分组数。
func Test多个大卡分组合进同一块(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 3
	b.Groups = []model.Group{
		{ID: "a", Title: "甲", Layout: model.LayoutFeature,
			Items: []model.Item{{ID: "a1", Name: "一", URL: "http://10.0.0.1/"},
				{ID: "a2", Name: "二", URL: "http://10.0.0.2/"}}},
		{ID: "b", Title: "乙", Layout: model.LayoutFeature,
			Items: []model.Item{{ID: "b1", Name: "三", URL: "http://10.0.0.3/"}}},
		{ID: "c", Title: "丙", Layout: model.LayoutTiles,
			Items: []model.Item{{ID: "c1", Name: "四", URL: "http://10.0.0.4/"}}},
	}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	p, err := renderer(t, s).ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)

	if n := strings.Count(body, `<section class="feat"`); n != 1 {
		t.Errorf("大卡那块有 %d 段外壳，要 1 段（多个分组共用一块）：\n%s", n, bandOnly(body))
	}
	// 分组信息不许被合并掉：块内每一段有自己的标题。
	if n := strings.Count(body, `class="feat-g"`); n != 2 {
		t.Errorf("块内只有 %d 个分段标题，要 2 个（每组一个）", n)
	}
	if n := strings.Count(body, `class="feat-row"`); n != 2 {
		t.Errorf("块内只有 %d 个条目行，要 2 个", n)
	}
	// 三张卡一张都不许少（两段共 3 条）。数 class 名而不是数标签：带内的
	// `<li>` 与正文的同构，用 li 会把正文的也算进来。
	if n := strings.Count(body, "feat-card"); n != 3 {
		t.Errorf("大卡张数不对：feat-card 出现 %d 次，要 3 次", n)
	}
	// 没设大档的那一组仍然在正文里。
	if !strings.Contains(body, `<section aria-labelledby="g-c"`) {
		t.Error("没设大卡的那一组被一起搬走了")
	}
	if strings.Contains(body, `<section aria-labelledby="g-a"`) || strings.Contains(body, `<section aria-labelledby="g-b"`) {
		t.Error("大卡分组同时还在正文里")
	}
}

// bandOnly 在失败时只把带子附近那一段打出来：整页 HTML 太长，读不到点上。
func bandOnly(body string) string {
	i := strings.Index(body, `<main`)
	j := strings.Index(body, `</main>`)
	if i < 0 || j < 0 {
		return body
	}
	s := body[i:j]
	if len(s) > 1400 {
		return s[:1400] + "…"
	}
	return s
}
