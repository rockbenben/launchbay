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
// 下面这组读数是 **2026-10-15** 在同一台机器上重量的。上一组写在 2026-10-13，
// 两天里已经全面失准——它写着「内联 CSS raw 17,360 / 余 1.39 倍」，而实测那时的
// 余量是 **3 字节**。读数只能整组一起换，这条纪律不变：
//
//	 25 条  raw  45,984 B / gzip 10,274（线 16,000，1.56 倍）
//	200 条  raw 179,714 B / gzip 28,348（线 48,000，1.69 倍）
//	内联 CSS raw 23,982 B / gzip  5,882（线  8,000，1.36 倍）
//	管理页 25 档    raw  41,176 / gzip  8,145（同 16 KB 那条，1.96 倍）
//	回退页 50 份    raw  36,414 / gzip  7,214（同上，2.22 倍）
//	引导进门页      raw  25,927 / gzip  7,007（同上，2.28 倍）
//	单人进门页      raw  25,303 / gzip  6,486（同上，2.47 倍）
//	进门页带一档坏档 raw 26,197 / gzip 6,731（同上，2.38 倍）
//	app.js raw 124,635 / gzip 47,737（线 61,440，1.29 倍）
//
// 「代管态 25 条」那一行随那个形状一起在 2026-10-10 删了：一条描述不存在
// 形状的读数，比少一行更容易骗人。
//
// ── CSS 那条线：24,000 raw → 8,000 gzip（2026-10-15）────────────────────────
//
// **不是抬线，是把单位改回规格写明的那个。** 三件事同时成立：
//   1. §7.3 原文是「CSS 内联 ≤ 40KB（**压缩后**）」（specs/2026-10-05 那个设计
//      文档第 410 行），而从前那条闸拿 len(raw) 去比 40,000——判据与规格写的
//      不是同一个单位，**连它自己的报错都写着"§7.3 按压缩后算"**。
//   2. 本文件里其余每一条闸量的都是 gzip，**只有 CSS 这一条量 raw**。
//   3. 后果不是"闸太严"，是**闸在说假话**：今天同一份字节 raw 23,982、
//      gzip 5,882，差 4 倍；而动手之前 base.css 的内联字节是 23,997，线是
//      24,000——**余量 3 字节**。它冻结了两天，任何一条 CSS 规则都会把它弄红，
//      而它红的时候告诉人的是"raw 满了"，可 raw 满不满与用户在局域网里多等
//      多久毫无关系。**假闸比没闸贵**（§7.3 关于 headless 默认值那段就是
//      这个教训）。
//
// **8,000 这个数不是画出来的**，它挂在同一文件里已有的另一条线上：进门页与
// 引导页的 HTML 极少（单人页 raw 25,303 / gzip 6,486），而其中 **5,882 就是
// 这份 CSS**——那些小页面几乎就是 CSS 本身。CSS 一旦长到逼近进门页自己那条
// 16,000，进门页就先顶破它自己的线。**取一半 = 8,000**，今天 1.36 倍，与
// app.js 那条 1.29 倍同一量级。
//
// 换单位之后这条闸**仍然是三条里最紧的一条**，而且它量的东西与另两条不同：
// HTML 随档案规模长，而内联 CSS 是**每一页都要付的固定成本**，没有第二条闸
// 在管它（不量它，它就永远长）。
//
// HTML 那两条线仍是 2026-10-06 画的、**没有跟着读数往上调过**：线是判据不是
// 读数的函数，一次改动把线顺手推高 = 这次改动自己给自己发通行证。

const (
	budgetHTMLContract   = 120_000 // spec §7.3：首屏 HTML gzip ≤ 120KB
	budget25Operational  = 16_000  // 2026-10-06 画线；今天 25 条实测 gzip 8,681 B
	budget200Operational = 48_000  // 同上；今天 200 条实测 gzip 23,537 B
	budgetCSSContract    = 40_000  // spec §7.3 原文：CSS 内联 ≤ 40KB（**压缩后**）
	// 2026-10-15 改判：24,000 **raw** → 8,000 **gzip**。理由与推导在上面那段——
	// 一句话是"规格写的是压缩后，而这条闸量的是没压的"。
	// **要动这条线仍然得连上面那组读数一起重写**，而且得说清新数挂在哪条线上。
	budgetCSSOperational = 8_000
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
	//
	// **判的是 gzip，不是 raw**（2026-10-15 改）。三件事同时成立：
	//   1. §7.3 原文写的是「CSS 内联 ≤ 40KB（**压缩后**）」
	//      （specs/2026-10-05-launchbay-design.md），而这一条从前拿 `len(raw)`
	//      去比 40,000 —— **判据与规格写的不是同一个单位**，连报错那句都写着
	//      "§7.3 的 40KB 按压缩后算"，而它比的是没压的。
	//   2. 本文件里其余每一条闸量的都是 gzip（首屏 HTML、管理页、回退页、
	//      历史页，以及正下方那条 app.js），**只有 CSS 这一条是 raw**。
	//   3. CSS 与整页一起压之后，单独压它得到的 gzip 就是它**并进页面时的
	//      边际成本**（CSS 内部的重复 token 是它自己的字典，与 HTML 几乎不共享）。
	//
	// raw 仍然打出来，只是不再当判据：它是"这一轮长了多少"的信号，
	// 而不是"用户在局域网里多等了多久"的成本。**这两个数在今天的实测里差 4 倍**
	// （23,982 / 5,882）——用 raw 判的闸会在这两条线之间挑错的那条。
	raw := InlineCSS()
	gz := gzipSize(raw)
	t.Logf("内联 CSS: raw=%d gzip=%d（线 %d，按 gzip 判）", len(raw), gz, budgetCSSOperational)
	if gz > budgetCSSOperational {
		pk := gzipSize(pickerShape(t))
		t.Errorf("内联 CSS gzip = %d B，超了操作线 %d B（这条线挂在进门页自己那条 16,000 上："+
			"最瘦的那一页 gzip 只有 %d B，而其中 %d B 就是这份 CSS——CSS 再涨下去，"+
			"先顶破的是进门页自己那条线）", gz, budgetCSSOperational, pk, gz)
	}
	if gz > budgetCSSContract {
		t.Errorf("内联 CSS gzip = %d B，破了 §7.3 的 40KB 契约天花板（规格写的是压缩后）", gz)
	}
}

// pickerShape 拿"最瘦的那种页面"当尺子：内联 CSS 在那一页里占多少，只有和
// 那一页自己的字节对着量才知道。它是 CSS 那条线的推导里那个分母。
func pickerShape(t *testing.T) []byte {
	t.Helper()
	ix, fc, s := assets(t)
	p, err := New(s, ix, fc).PickerPage(PickerOptions{Heading: "谁来用？"})
	if err != nil {
		t.Fatal(err)
	}
	return p.Body
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
	// 改动历史那一屏（2026-10-13）：它按**动作数**长，一页最多 100 行、
	// 一行最多 6 条短句，于是最坏那一支是 100×6。不在名单里的话，
	// "除档案页外都零脚本"就少看一种形状而没人发现。
	hv := HistoryView{Profile: "dad", Heading: "dad 的改动历史",
		Explain: "这一页记的是动作，不是内容。"}
	for i := 0; i < 100; i++ {
		row := HistoryRow{At: "2026-10-13T06:00:00Z", Channel: "AI 助手",
			Count: 9, Rev: int64(i + 2), More: "…另外还有 3 处没列出来"}
		for j := 0; j < 6; j++ {
			row.Ops = append(row.Ops, fmt.Sprintf("改名 · 自建服务 › 群晖 DSM %d", j))
		}
		hv.Rows = append(hv.Rows, row)
	}
	hp, err := r.HistoryPage(hv)
	if err != nil {
		t.Fatal(err)
	}
	pages["history"] = hp.Body
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
		// **不带 Admin**（2026-10-12）：`AdminRow.Admin` 删掉了，表格里也就没有
		// 那一列徽标——"谁能管这台机器"是这一页的访问权，不是每一行档案的属性。
		rows = append(rows, AdminRow{
			ID: "dad", Title: fmt.Sprintf("档案 %02d", i), Rev: int64(i),
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

// 改动历史满页那一支（2026-10-13）：100 次动作 × 每次 6 条短句。
//
// 沿用 25 条首屏那条操作线、不另开一条——**这一页的字节全是一行一行的句子**，
// 没有内联图标、没有样式表，同一份 gzip 字典能吃掉整页重复的那些
// 「改名 · 自建服务 › 」，所以它天然比首屏小。真到超线那天要问的是
// "要不要把每页 100 次收成一页 50 次"，不是"把线抬到刚过"。
func TestBudget_改动历史满页也在同一线内(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	v := HistoryView{Profile: "dad", Heading: "dad 的改动历史",
		Explain: "这一页记的是动作，不是内容。"}
	for i := 0; i < 100; i++ {
		row := HistoryRow{At: "2026-10-13T06:00:00Z", Channel: "AI 助手",
			Count: 9, Rev: int64(i + 2), More: "…另外还有 3 处没列出来"}
		for j := 0; j < 6; j++ {
			row.Ops = append(row.Ops, fmt.Sprintf("改名 · 自建服务 › 群晖 DSM %d", j))
		}
		v.Rows = append(v.Rows, row)
	}
	p, err := r.HistoryPage(v)
	if err != nil {
		t.Fatal(err)
	}
	raw, gz := len(p.Body), gzipSize(p.Body)
	t.Logf("改动历史 100 次×6 条: raw=%d gzip=%d", raw, gz)
	if gz > budget25Operational {
		t.Errorf("改动历史满页 gzip = %d B，超了那条操作线 %d B（线不抬）", gz, budget25Operational)
	}
	// 读数不许来自空壳页面：满页那 100 张卡片真在页上。
	if n := strings.Count(string(p.Body), `<li class="plain">`); n != 100 {
		t.Errorf("该画 100 张卡片，画了 %d 张——读数就不是满页那一支", n)
	}
	if !strings.Contains(string(p.Body), "另外还有 3 处没列出来") {
		t.Error("截断那一句话没在场，说明满页那一支被悄悄截短了")
	}
}
