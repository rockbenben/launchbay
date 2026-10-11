package icons

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestExtract_标题描述与favicon(t *testing.T) {
	body := []byte(`<html><head>
		<title>  Jellyfin &amp; Co  </title>
		<meta property="og:description" content="  家庭媒体服务器 ">
		<link rel="icon" href="/favicon.ico">
		<link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png">
	</head><body><h1>正文不进任何东西</h1></body></html>`)
	got := Extract(body, "http://10.0.0.3:8096/web/index.html")

	if got.Title != "Jellyfin & Co" {
		t.Errorf("标题要去实体与首尾空格，得到 %q", got.Title)
	}
	if got.Description != "家庭媒体服务器" {
		t.Errorf("描述 = %q", got.Description)
	}
	if got.Favicon != "http://10.0.0.3:8096/apple-touch-icon.png" {
		t.Errorf("有多个候选时要 apple-touch-icon（通常 180px，比 16px favicon 清楚），得到 %q", got.Favicon)
	}
	if strings.Contains(got.Title, "正文") || strings.Contains(got.Description, "正文") {
		t.Error("正文漏进来了")
	}
}

func TestExtract_属性值里有尖括号也不截断(t *testing.T) {
	// 这是正则版会栽的那个形状：content="a>b" 里>在引号内，不是标签结束。
	// 夹具用 og:description（property=），不用朴素的 name="description"——
	// spec §6.1 只取 og，见 TestExtract_描述只认og且og优先。tagEnd 关心的是
	// 引号里的 >，与 key 是 property 还是 name 无关，换成 og 后这条证人照旧。
	body := []byte(`<meta property="og:description" content="差 6 > 4 的比分">`)
	got := Extract(body, "http://10.0.0.1/")
	if got.Description != "差 6 > 4 的比分" {
		t.Errorf("被引号里的 > 截断了: %q", got.Description)
	}
}

// TestExtract_描述只认og且og优先 钉死 spec §6.1 的"只取 <title> 与 og:description"：
// 描述只认 property="og:description"，朴素的 name="description" 一律不收。
// 三个夹具的期望串互不相同、也不会与 Title/Favicon 偶然重合（本仓库栽过永真证人）：
//   - 只有 og → 取 og 串；
//   - 只有朴素 → 必须留空（spec：没有 og 就是没有，宁缺勿编）；
//   - 朴素在前 + og 在后 → og 赢，与文档顺序无关。
//
// 第三条是顺序证人：只要有人为了"兼容"再把 name= 加回文档序取用，"朴素在前"这个
// 夹具会拿到"朴素在前"而非"OG在后"，这条当场红。
func TestExtract_描述只认og且og优先(t *testing.T) {
	onlyOG := Extract([]byte(`<meta property="og:description" content="只有OG的串">`), "http://10.0.0.1/")
	if onlyOG.Description != "只有OG的串" {
		t.Errorf("只有 og 时要取 og，得到 %q", onlyOG.Description)
	}

	onlyPlain := Extract([]byte(`<meta name="description" content="只有朴素的串">`), "http://10.0.0.1/")
	if onlyPlain.Description != "" {
		t.Errorf("只有朴素 name=description 按 spec 必须留空，得到 %q", onlyPlain.Description)
	}

	plainFirst := Extract([]byte(
		`<meta name="description" content="朴素在前的串"><meta property="og:description" content="OG在后的串">`),
		"http://10.0.0.1/")
	if plainFirst.Description != "OG在后的串" {
		t.Errorf("朴素在前也不许赢，og 必须与文档顺序无关地胜出，得到 %q", plainFirst.Description)
	}
}

func TestExtract_缺项就空着不编造(t *testing.T) {
	got := Extract([]byte(`<html><head><meta charset="utf-8"></head></html>`), "http://10.0.0.1/")
	if got.Title != "" || got.Description != "" || got.Favicon != "" {
		t.Errorf("没有就是没有: %+v", got)
	}
}

func TestExtract_标题要限长(t *testing.T) {
	long := "<title>" + strings.Repeat("标", 400) + "</title>"
	got := Extract([]byte(long), "http://10.0.0.1/")
	if n := len([]rune(got.Title)); n > maxTitleRunes {
		t.Errorf("标题 %d 字，上限 %d", n, maxTitleRunes)
	}
}

func TestExtract_相对与绝对href都能落地(t *testing.T) {
	got := Extract([]byte(`<link rel="icon" href="../fav.ico">`), "http://10.0.0.1/a/b/index.html")
	if got.Favicon != "http://10.0.0.1/a/fav.ico" {
		t.Errorf("相对路径解析错: %q", got.Favicon)
	}
	got = Extract([]byte(`<link rel="icon" href="http://cdn.10.0.0.9.fakedomain.test/f.ico">`), "http://10.0.0.1/")
	if got.Favicon == "" {
		t.Error("绝对 href 应当原样保留")
	}
}

// TestExtract_并发不串号 是"候选状态（bestRank/bestHref）必须函数局部"这条约束
// 唯一的证人。CGO_ENABLED=0（本项目 Docker 目标硬约束）下 -race 不可用，所以这里
// 用行为断言：同一个 Extract 起 N 个 goroutine 并发跑【两个不同的 base】，每个
// goroutine 核对自己的 base 与拿到的 Favicon 是否一致。
//
//   - 函数局部（当前实现）：每次 Extract 有独立的 bestHref，永远各拿各的 → 全绿。
//   - 包级候选 + 入口重置：A 把 bestHref 写成 base1/apple.png，B 在同一时刻把它
//     重置/覆写成 base2/apple.png 或空串，A 读到的就是别人的地址（甚至空串）→ 红。
//
// 两个 base 是关键：只有让它们产出【不同】的绝对地址，跨调用污染才看得见。评审
// 那版探针用"一半带 link 一半不带"，靠的是读到空串；这里两 base 连"读到对但错档"
// 的串号都能抓到，且是常驻测试而非跑完即删的探针。
func TestExtract_并发不串号(t *testing.T) {
	body := []byte(`<link rel="apple-touch-icon" href="/apple.png">`)
	bases := []string{"http://10.0.0.1/", "http://10.0.0.2/"}
	const goroutines = 64
	const iters = 1000

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			base := bases[g%len(bases)]
			want := base + "apple.png" // resolveHref("/apple.png", base) 的确定结果
			for k := 0; k < iters; k++ {
				if got := Extract(body, base); got.Favicon != want {
					t.Errorf("并发串号：base=%s 第 %d 次拿到 %q，要 %q", base, k, got.Favicon, want)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestExtract_属性前是换行仍能解析 钉 B3：真实网页常用换行切分标签名与属性
// （`<meta\nproperty="og:description" …>`），而 parseAttrs 曾只按第一个 ' '
// 切——属性前是换行/制表时整条属性的解析静默丢掉，desc 变空、resolve 端出
// null，没有任何报错。修前红：描述取不出来。
func TestExtract_属性前是换行仍能解析(t *testing.T) {
	body := []byte("<html><head>\n<meta\nproperty=\"og:description\"\ncontent=\"换行切分的描述\">\n" +
		"<link\nrel=\"apple-touch-icon\"\nhref=\"/apple.png\">\n</head></html>")
	got := Extract(body, "http://10.0.0.1/")
	if got.Description != "换行切分的描述" {
		t.Errorf("属性前是换行时 og 描述静默丢了: %q", got.Description)
	}
	// link 同夹具顺带钉住：favicon 候选也走 parseAttrs，标签名与属性之间
	// 是换行时 href/rel 都得照常解出来。
	if got.Favicon != "http://10.0.0.1/apple.png" {
		t.Errorf("属性前是换行时 link href 也丢了: %q", got.Favicon)
	}
}

// TestPage_结构不留抓到的HTML 钉死 spec §6.1 的结构性约束"不存正文、不留抓到的
// HTML"：这条只活在注释里时，"顺手加一个字段装正文"或"把 Favicon 的 json:"-"
// 去掉让它进响应"都能悄无声息通过全部行为测试。
//
//   - Page 字段集合恰是 Title/Description/Favicon 三个，多一个都红（加 Body 装 HTML）。
//   - Page 序列化后对外键集合恰是 {title, desc}，且绝不含 Favicon（Favicon 是 json:"-"，
//     只在内部用于抓图）。去掉 json:"-" → Favicon 进序列化、键数变 3 → 红。
func TestPage_结构不留抓到的HTML(t *testing.T) {
	typ := reflect.TypeOf(Page{})
	if typ.NumField() != 3 {
		t.Fatalf("Page 只能有三个字段（不留装正文的字段），实际 %d 个", typ.NumField())
	}
	wantFields := []string{"Title", "Description", "Favicon"}
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Name != wantFields[i] {
			t.Errorf("字段 %d 要 %q，实际 %q", i, wantFields[i], typ.Field(i).Name)
		}
	}

	raw, err := json.Marshal(Page{Title: "标题候选", Description: "描述候选", Favicon: "http://10.0.0.1/apple.png"})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	// 对外两键，恰是 title/desc。
	if len(keys) != 2 {
		t.Errorf("Page 序列化对外只能两键 title/desc，实际 %d 键: %s", len(keys), raw)
	}
	for _, k := range []string{"title", "desc"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("序列化缺 %q 键: %s", k, raw)
		}
	}
	// Favicon（json:"-"）以任何名义都不许出现在响应里，抓到的 HTML 更不许。
	for _, forbidden := range []string{"Favicon", "favicon", "body", "Body", "html", "HTML"} {
		if _, ok := keys[forbidden]; ok {
			t.Errorf("响应不许含 %q 键（Favicon 只内部用、绝不装正文）: %s", forbidden, raw)
		}
	}
}
