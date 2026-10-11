package render

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 体积预算闸（spec §7.3）。**字节类进 go test，不加 gzip 中间件**：局域网里
// 那点字节换不回一次往返的时间，而中间件会让每个响应都要重新想一遍
// Content-Encoding 与 ETag 怎么相处。这里量的是渲染产物的 gzip 尺寸，
// 与浏览器实际收到的字节同源。
//
// 线是 2026-10-06 在本机实测分布上画的（25 条 = 当前真实规模，图标全内联）。
// 下面这组读数是 ④.5 之后"两处死路对策"那一轮在 HEAD 重新量的（进门页多了一种形状：
// 一份都进不去时要点名坏档）；上一轮的数不往回叠——旧数留在注释里只会让人拿它当今天的余量：
//
//	 25 条  raw  38,049 B / gzip 13,104 B
//	200 条  raw 142,474 B / gzip 26,196 B
//	内联 CSS raw  21,410 B / gzip  9,657 B
//	代管态 25 条 raw 38,355 B / gzip 13,195 B（比无横幅版多 306 raw / 91 gzip，同一操作线）
//	管理页 25 档 raw 33,459 B / gzip 10,765 B（按档案数长的那一页，同一操作线）
//	引导进门页 raw 22,271 B / gzip 10,145 B；单人进门页 22,185 B / 10,101 B
//	进门页带一档坏档 raw 22,875 B / gzip 10,229 B（三档里坏一份那种形状）
//	（进门页这些形状都罩在同一条 16 KB 操作线里，见 TestBudget_进门页其余形状也在同一线内；
//	  进门页比 25 条首屏小得多，但"小得多"是断言不是修辞，也不给它另开一条线）
//
// 新增那一种形状**一行 CSS 都没加**：坏档那一行借的是现成的 `.plain`（与卡片同一张脸
// 的不可点形态，profile 页"需手动打开"那条用的就是它）与 `.badge`。下面那条 1.12 倍
// 的 CSS 线因此一点没动——这不是巧合，是这一轮的设计约束：给进门页补一句解释不该
// 先花掉 2.6 KB 里的份额。
//
// 这一组数在 Task 5 的 M-7 之后没跟着 CSS 那几行改动重量过（收窄 `[type=…]` 与
// 勾选格的内衬各加了一截），到 ④.5 关门重测时已经整体漂了 +129 raw / +60 gzip——
// 线没破，但注释里的"余量"是假的。读数只能整组一起换，别只换红的那一行。
//
// CSS 那条线现在只剩 1.12 倍余量（21,410 / 24,000）。这不是"把线抬一抬"的信号，
// 是下面那三条里第一条该做决定的信号：外壳再加一族之前，得先说清这 2.6 KB 花在
// 哪个特性上、值不值。
//
// 三条操作线**都没有跟着读数往上调**：线是判据，不是读数的函数，一次改动把线
// 也顺手推高 = 这次改动自己给自己发通行证。
//
// 两层线一起钉：契约天花板（§7.3 写死的 120 KB / 40 KB）与**操作线**。只钉天花板
// 的那条在本案规模上差着 5 倍，永远跨不过去，而一条永远绿的闸是假闸——假闸比没闸
// 贵（§7.3 关于 headless 默认值那段就是这个教训）。操作线红了要看注释里的实测数，
// 别顺手把数往上调到刚过。
const (
	budgetHTMLContract   = 120_000 // spec §7.3：首屏 HTML gzip ≤ 120KB
	budget25Operational  = 16_000  // HEAD 实测 gzip 13,104 B 的 1.22 倍
	budget200Operational = 48_000  // HEAD 实测 gzip 26,196 B 的 1.83 倍
	budgetCSSContract    = 40_000  // spec §7.3：CSS 内联 ≤ 40KB
	// HEAD 实测 21,410 B 的 1.12 倍——三条里最薄的一条，而且是有意留薄的：CSS 每加
	// 一族外壳就多几百字节，余量掉到 1 倍以下那天要做的决定是"这 4 KB 花在哪个特性
	// 上、值不值"，不是"把线抬到刚过"。要动这条线必须连上面那段读数一起重写。
	budgetCSSOperational = 24_000
)

func gzipSize(b []byte) int {
	var z bytes.Buffer
	w := gzip.NewWriter(&z)
	if _, err := w.Write(b); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return z.Len()
}

// realistic 造 n 条书签，摊在 4 个分组里，每条一个**互不相同的真实图标名**。
// 为什么必须不同：同名的内联 SVG 在 gzip 眼里就是一份字典，n=200 也只算
// 一条的字节——那会让这条闸测不到真东西（首屏的真实形状就是几百个不同的图标
// markup 堆在一起）。名字从索引产物里取，不手写枚举。
func realistic(t *testing.T, n int) *model.Bookmarks {
	t.Helper()
	ix, _, _ := assets(t)
	names := ix.Names("tabler")
	if len(names) == 0 {
		t.Fatal("索引里没有 tabler 名字，夹具造不出真形状")
	}
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Title = "发射台"
	titles := []string{"内网", "影音", "工具", "读与收藏"}
	for i := 0; i < 4; i++ {
		b.Groups = append(b.Groups, model.Group{
			ID: fmt.Sprintf("g%d", i), Title: titles[i], Layout: model.LayoutTiles,
			Items: []model.Item{},
		})
	}
	for i := 0; i < n; i++ {
		g := &b.Groups[i%4]
		g.Items = append(g.Items, model.Item{
			ID:   fmt.Sprintf("i%d", i),
			Name: fmt.Sprintf("条目 %d 号·内网服务", i),
			URL:  fmt.Sprintf("http://10.0.0.%d:%d/some/path", i%250+1, 8000+i),
			Desc: fmt.Sprintf("这是第 %d 条的说明文字，用来量正文占比", i),
			Icon: names[i%len(names)],
		})
	}
	return b
}

func TestBudget_首屏字节在两条线内(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	for _, tc := range []struct {
		n, operational int
	}{
		{25, budget25Operational},
		{200, budget200Operational},
	} {
		if err := s.SaveBookmarks("dad", realistic(t, tc.n)); err != nil {
			t.Fatal(err)
		}
		// 每个规模一个新渲染器：缓存键含 bookmarks.json 的 mtime，
		// 同一台渲染器连着量两个规模会把上一个的条目当成本次的读数。
		r = New(s, ix, fc)
		p, err := r.ProfilePage(PageRequest{ID: "dad"})
		if err != nil {
			t.Fatal(err)
		}
		raw, gz := len(p.Body), gzipSize(p.Body)
		t.Logf("%d 条: raw=%d gzip=%d", tc.n, raw, gz)
		if gz > tc.operational {
			t.Errorf("%d 条的首屏 gzip = %d B，超了操作线 %d B"+
				"（实测基线见本文件注释；超线先查是谁把字节堆进了首屏）", tc.n, gz, tc.operational)
		}
		if gz > budgetHTMLContract {
			t.Errorf("%d 条的首屏 gzip = %d B，破了 §7.3 的 120KB 契约天花板", tc.n, gz)
		}
		// 首屏必须真的带内容（否则"字节达标"是用空页面换来的假绿）。
		if !bytes.Contains(p.Body, []byte(`<a class="card"`)) {
			t.Errorf("%d 条夹具没渲染出磁贴", tc.n)
		}
	}
}

func TestBudget_进门页其余形状也在同一线内(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	for _, tc := range []struct {
		name string
		opts PickerOptions
		want string
	}{
		{"引导", PickerOptions{Heading: "还没有档案", Setup: true}, `action="/setup"`},
		{"单人", PickerOptions{Heading: "爸爸的台"}, "<h1>爸爸的台</h1>"},
	} {
		p, err := r.PickerPage(tc.opts)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		raw, gz := len(p.Body), gzipSize(p.Body)
		t.Logf("%s 进门页: raw=%d gzip=%d", tc.name, raw, gz)
		if gz > budget25Operational {
			t.Errorf("%s 进门页 gzip = %d B，超了 25 条首屏那条操作线 %d B（线不抬）",
				tc.name, gz, budget25Operational)
		}
		if !bytes.Contains(p.Body, []byte(tc.want)) {
			t.Errorf("%s 那支没渲染出 %q——读数来自空壳页面", tc.name, tc.want)
		}
	}
	// 第四支：三档里有一档读不动（死路①的对策新增的形状）。它同样不另开线。
	bs := pickerStore(t, "dad", "mom", "kid")
	breakProfile(t, bs, "mom")
	bp, err := pickerRenderer(t, bs).PickerPage(PickerOptions{Heading: "谁来用？"})
	if err != nil {
		t.Fatal(err)
	}
	raw, gz := len(bp.Body), gzipSize(bp.Body)
	t.Logf("进门页 带一档坏档: raw=%d gzip=%d", raw, gz)
	if gz > budget25Operational {
		t.Errorf("带坏档那一支 gzip = %d B，超了那条操作线 %d B（线不抬）", gz, budget25Operational)
	}
	if !bytes.Contains(bp.Body, []byte("数据文件读不了")) {
		t.Error("读数来自没渲染出点名段的空壳页面")
	}
}

func TestBudget_内联CSS有上限(t *testing.T) {
	// 量**真正内联进页面**的那份字节（注释已剥），不是磁盘上的源文件：
	// 源文件的注释不下发，拿它判"页面里真占的那部分"量的是一个不存在的数。
	raw := InlineCSS()
	if err := error(nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("内联 CSS: raw=%d gzip=%d", len(raw), gzipSize(raw))
	if len(raw) > budgetCSSOperational {
		t.Errorf("内联 CSS = %d B，超了操作线 %d B（§7.3 的 40KB 按压缩后算，"+
			"这里钉的是原始字节：CSS 与整页一起压，单独量 gzip 没意义，而原始字节"+
			"才是页面里真占的那部分）", len(raw), budgetCSSOperational)
	}
	if len(raw) > budgetCSSContract {
		t.Errorf("内联 CSS = %d B，破了 §7.3 的 40KB 契约天花板", len(raw))
	}
}

// ⑤ 之后的首屏脚本预算（spec §7.3 的「JS ≤ 60KB gzip」）。
//
// **它与下面那条"除 profile 外零脚本"是一件事的两半**：一个说档案页最多能有多重，
// 另一个说别的页面根本不许有。两半都松了，§7.2「关掉 JS 时全部书签仍可见可点」就
// 从一条判据退化成一句介绍。
//
// 量的是内嵌脚本的 gzip 尺寸，与浏览器实际下载到的同源（§7.1 那一行
// `immutable` 意味着它只在指纹变时被重新下载一次，所以它的预算量级与 HTML 不同：
// HTML 每次翻页都要传，JS 常常整个会话只传一次）。
func TestBudget_appJS在预算内(t *testing.T) {
	const limit = 60 << 10 // 60KB gzip，spec §7.3
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(AppJS()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	got := buf.Len()
	t.Logf("app.js：raw %d B / gzip %d B（线 %d B）", len(AppJS()), got, limit)
	if got > limit {
		t.Errorf("app.js gzip %d B，超出 60 KB 线 %d B", got, limit)
	}
}

// 档案页引一个 script，**其余每一种页面形状仍然是零**。
// spec §7.2 的话仍然成立：关掉 JS 时首屏该有的东西一样不少。
//
// 下面那张 pages 表是**手抄名单**（没有一处代码登记"一共有几种形状"，推导不出来）：
// 新增一种页面形状而忘了在这里加一行，这条闸就少看一种而没人发现。名字里因此不写
// 数字——写死"五种"的下场是数字先腐烂，名单后腐烂。
func TestBudget_除档案页外都零脚本(t *testing.T) {
	ix, fc, s := assets(t)
	if err := s.SaveBookmarks("dad", realistic(t, 25)); err != nil {
		t.Fatal(err)
	}
	r := New(s, ix, fc)
	pages := map[string][]byte{}
	// 档案页**不放进这张表**：它引一个 script（⑤ 的真字节），形状的判据在
	// TestProfileStructureAndNoJS 那条里——恰好 1 个 type=module 外部脚本。
	// 先跑一遍，形状不对的话由那条抓。
	if _, err := r.ProfilePage(PageRequest{ID: "dad"}); err != nil {
		t.Fatal(err)
	}
	pk, err := r.PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pages["picker"] = pk.Body
	// 引导形状：同一张模板的另一支，字节与文案都不同，也要过同一条零脚本闸。
	su, err := r.PickerPage(PickerOptions{Heading: "还没有档案", Setup: true})
	if err != nil {
		t.Fatal(err)
	}
	pages["setup"] = su.Body
	// 带坏档的那一支（④.5 死路对策新增的形状）：同一张模板的第三种长相，也要过这条闸。
	bbs := pickerStore(t, "dad", "mom")
	breakProfile(t, bbs, "mom")
	bd, err := pickerRenderer(t, bbs).PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pages["pickerBlocked"] = bd.Body
	br, err := r.BrokenPage("dad", "bookmarks.json 第 3 行附近: 测试夹具")
	if err != nil {
		t.Fatal(err)
	}
	pages["broken"] = br.Body
	ad, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸", Items: 25, Rev: 3}}, "")
	if err != nil {
		t.Fatal(err)
	}
	pages["admin"] = ad.Body
	// 第四张确认页（回退，2026-10-11 加的）。它按**快照份数**长——最多 50 份，
	// 每份一格（版本号 + 条目数 + 分组名），所以这里用满额那一支量。
	// 这一格不在名单里的话，"除档案页外都零脚本"就少看一种形状而没人发现。
	rv, err := r.AdminActionPage(AdminAction{
		Title: "回退到某一版", Heading: "回退「爸爸」的某一版？",
		Explain: "现在：第 3 版，41 条书签。", Action: "/revert",
		Submit: "回到这一版", NavHref: "/", NavLabel: "回我的页面",
		ChoiceName: "to-rev", ChoiceLegend: "选一版",
		Choices: func() []AdminChoice {
			cs := make([]AdminChoice, 0, 50)
			for i := 0; i < 50; i++ {
				cs = append(cs, AdminChoice{
					Value: fmt.Sprintf("%d", i), Label: fmt.Sprintf("第 %d 版 · 41 条书签 · 分组：NAS、影音、常用", i),
				})
			}
			return cs
		}(),
	})
	if err != nil {
		t.Fatal(err)
	}
	pages["revert"] = rv.Body
	for name, body := range pages {
		if n := strings.Count(string(body), "<script"); n != 0 {
			t.Errorf("%s 页面里有 %d 处 <script——④ 的判据是零 JavaScript", name, n)
		}
	}
}

// 管理页是这一族里唯一按**档案数**长的一页，它今天没有任何字节读数（复扫 M-7）。
// 沿用 25 条首屏那条操作线，不另开一条：档案数到 25 时这页仍要小得多，"小得多"
// 由断言说，不由修辞说。notice 与建档表单都必须在场（否则读数来自空壳页面）。
func TestBudget_管理页25档也在同一线内(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	rows := make([]AdminRow, 0, 25)
	for i := 0; i < 25; i++ {
		rows = append(rows, AdminRow{
			ID: "dad", Title: fmt.Sprintf("档案 %02d", i), Admin: i == 0, Rev: int64(i),
			Modified: "2026-10-08T00:00:00Z", Items: 25, Snapshots: 3,
		})
	}
	p, err := r.AdminPage(rows, "PIN 要 4-6 位数字。")
	if err != nil {
		t.Fatal(err)
	}
	raw, gz := len(p.Body), gzipSize(p.Body)
	t.Logf("管理页 25 档: raw=%d gzip=%d", raw, gz)
	if gz > budget25Operational {
		t.Errorf("管理页 25 档 gzip = %d B，超了那条操作线 %d B（线不抬）", gz, budget25Operational)
	}
	for _, want := range []string{`<table class="adm"`, `action="/admin"`, `PIN 要 4-6 位数字。`} {
		if !strings.Contains(string(p.Body), want) {
			t.Errorf("管理页读数少了 %q——读数来自空壳页面不算过", want)
		}
	}
}

// 回退页（2026-10-11）按**快照份数**长：保留 50 份，于是这一页最坏有 50 格。
// 沿用 25 条首屏那条操作线、不另开一条——线是判据，不是读数的函数
// （见本文件上面那段"三条操作线都没有跟着读数往上调"）。
//
// 顺带钉住零快照那一支：它**整张表单都不画**，而这件事只有渲染结果看得见。
func TestBudget_回退页满额快照也在同一线内(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	choices := make([]AdminChoice, 0, 50)
	for i := 0; i < 50; i++ {
		choices = append(choices, AdminChoice{
			Value: fmt.Sprintf("%d", i),
			Label: fmt.Sprintf("第 %d 版 · 41 条书签 · 分组：自建服务、影音、常用、办公、学习", i),
		})
	}
	p, err := r.AdminActionPage(AdminAction{
		Title: "回退到某一版", Heading: "回退「爸爸」的某一版？",
		Explain: "现在：第 3 版，41 条书签。", Action: "/revert",
		Submit: "回到这一版", NavHref: "/", NavLabel: "回我的页面",
		ChoiceName: "to-rev", ChoiceLegend: "选一版", Choices: choices,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, gz := len(p.Body), gzipSize(p.Body)
	t.Logf("回退页 50 份快照: raw=%d gzip=%d", raw, gz)
	if gz > budget25Operational {
		t.Errorf("回退页 50 份 gzip = %d B，超了那条操作线 %d B（线不抬）", gz, budget25Operational)
	}
	for _, want := range []string{`action="/revert"`, `name="to-rev"`, "回我的页面"} {
		if !strings.Contains(string(p.Body), want) {
			t.Errorf("回退页读数少了 %q——读数来自空壳页面不算过", want)
		}
	}
	// 零份快照那一支：说清为什么，且一个 form 都不画。
	empty, err := r.AdminActionPage(AdminAction{
		Title: "回退到某一版", Heading: "回退「爸爸」的某一版？",
		Explain: "现在：第 0 版，0 条书签。\n\n这个档案还没有可回退的版本。",
		Action:  "/revert", Submit: "回到这一版", NavHref: "/", NavLabel: "回我的页面",
		NoForm: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(empty.Body), `<form`) {
		t.Error("零份快照时仍画了表单——点了只有一条没有出路的路")
	}
	if strings.Contains(string(p.Body), `<form`) == false {
		t.Error("有得选时反而没有表单")
	}
}
