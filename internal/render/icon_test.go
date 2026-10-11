package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
)

func TestPiece_有名就内联SVG(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "gh", Name: "GitHub", URL: "https://github.com/", Icon: "simple-icons:github"}, ix, fc)
	svg := string(p.SVG)
	if svg == "" {
		t.Fatal("品牌层命中却没给出 SVG")
	}
	if !strings.Contains(svg, `viewBox="0 0 24 24"`) || !strings.Contains(svg, "<path ") {
		t.Errorf("SVG 形状不对: %s", svg)
	}
	if !strings.Contains(svg, `aria-hidden="true"`) {
		t.Error("图标必须 aria-hidden：可及名由相邻的名字承担（spec §7.5）")
	}
	if !strings.Contains(svg, `class="ic-svg ic-fill"`) {
		t.Errorf("层标记要能让 CSS 选到呈现: %s", svg)
	}
	if p.Missing || p.Img != "" {
		t.Errorf("命中时不该带缺图标记或 img: %+v", p)
	}
}

func TestPiece_线条层带stroke标记(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "h", Name: "Home", URL: "http://10.0.0.1/", Icon: "tabler:home"}, ix, fc)
	if !strings.Contains(string(p.SVG), `ic-stroke`) {
		t.Errorf("tabler 必须是 stroke 层: %s", p.SVG)
	}
}

func TestPiece_陌生名字走首字母并标缺(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "x", Name: "青龙面板", URL: "http://10.0.0.2/", Icon: "tabler:nothere"}, ix, fc)
	if p.SVG != "" || p.Img != "" {
		t.Error("查不到就不该有图")
	}
	if !p.Missing {
		t.Error("名存实亡的图标要能被界面标出来（spec §12 的“不假装成功”）")
	}
	if p.Letter != "青" {
		t.Errorf("首字母要取第一个 rune，得到 %q", p.Letter)
	}
	if p.ChipHue < 0 || p.ChipHue > 359 {
		t.Errorf("色相越界: %d", p.ChipHue)
	}
}

func TestPiece_空icon是没设不是缺(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "x", Name: "X", URL: "http://10.0.0.2/"}, ix, fc)
	if p.Missing {
		t.Error("空 icon 不该打缺图标（口径见 spec §5）")
	}
	if p.Letter != "X" {
		t.Errorf("要首字母兜底，得到 %q", p.Letter)
	}
}

func TestPiece_auto有缓存就出图(t *testing.T) {
	ix, fc, _ := assets(t)
	origin := icons.Origin("http://10.0.0.3:8096/")
	if _, err := fc.Put(origin, []byte{0x89, 'P', 'N', 'G'}, "png"); err != nil {
		t.Fatal(err)
	}
	p := pieceFor(model.Item{ID: "j", Name: "Jellyfin", URL: "http://10.0.0.3:8096/web/", Icon: "auto"}, ix, fc)
	if e, ok := fc.Lookup(origin); !ok || p.Img != "/icons/"+e.Hash+"."+e.Ext {
		t.Errorf("img 路径不对: %q", p.Img)
	}
	if p.SVG != "" {
		t.Error("favicon 绝不内联成 SVG：第三方字节只做 img（见 icon.go 注释）")
	}
	if p.Missing {
		t.Error("有图就不算缺")
	}
}

func TestPiece_auto无缓存算缺但不画坏(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "j", Name: "Jellyfin", URL: "http://10.0.0.3:8096/", Icon: "auto"}, ix, fc)
	if p.Img != "" || p.SVG != "" {
		t.Error("没缓存不该有图")
	}
	if !p.Missing {
		t.Error("auto 且无缓存要进缺图口径（spec §5）")
	}
	if p.Letter == "" {
		t.Error("界面仍然只能走首字母，不能空着")
	}
}

func TestValidatedAccent_只认hex(t *testing.T) {
	// accent 会进 style 属性，而它是用户可写的字符串：判据只有这一条正则。
	// 非法值一律当"没写"，色块退回名称哈希——不报错、不猜、不洗。
	for accent, want := range map[string]string{
		"#4c8dff": "#4c8dff", "#abc": "#abc", "red": "", "#4c8dgff": "", "#4c8dff1": "", "": "",
	} {
		if got := validatedAccent(accent); got != want {
			t.Errorf("validatedAccent(%q) = %q，要 %q", accent, got, want)
		}
	}
}

// 两道闸的第二道：白名单在加载时拒（Task 1 的 TestDecode_未知tag未知attr与越界值都拒），
// 这里假设"万一有人绕过加载直接构造了 Icon"，markup 拼接仍然逃不出属性值。
func TestSvgMarkup_值一律转义(t *testing.T) {
	hostile := iconFixture("tabler", "x", "stroke", []icons.Element{{
		Tag:   "path",
		Attrs: []icons.Attr{{Name: "d", Value: `"/><script>alert(1)</script>`}},
	}})
	out := string(svgMarkup(hostile))
	if strings.Contains(out, "<script>") || strings.Contains(out, `"><script`) {
		t.Errorf("属性值逃出了转义: %s", out)
	}
	if !strings.Contains(out, "&#34;") && !strings.Contains(out, "&quot;") {
		t.Errorf("引号应当被转义，得到 %s", out)
	}
}

// 第二道闸要落在代码上，不能只活在注释里：层、元素名、属性名这三样同样要过
// 白名单谓词（icons.IsDrawable / icons.IsKeptAttr，与加载侧同一把尺子、各量一次）。
// 三种逃逸形状逐一验死，且用精确等值断言——"输出里一个 <svg 属性位都不多出引号"
// 这种话用 Contains 是量不出来的，只有整串相等才兜得住。
func TestSvgMarkup_第二道闸查层元素名与属性名(t *testing.T) {
	shell := `<svg class="ic-svg ic-stroke" viewBox="0 0 24 24" aria-hidden="true" focusable="false">`

	// ① 层带引号与空格 → 整个 SVG 被拒：返回空串，pieceFor 拿到空 SVG 后
	// 模板按 {{if .SVG}} 自然退到首字母色块（不编默认层）。
	badLayer := iconFixture("tabler", "x", `stroke" onclick="x`, []icons.Element{
		{Tag: "path", Attrs: []icons.Attr{{Name: "d", Value: "M0 0"}}},
	})
	if out := svgMarkup(badLayer); out != "" {
		t.Errorf("非白名单层应当整幅拒掉（空 SVG = 退首字母），得到: %s", out)
	}

	// ② 非可绘制元素名 → 少画一个元素，外壳一个字节都不许多。
	scriptTag := iconFixture("tabler", "x", "stroke", []icons.Element{
		{Tag: "script", Attrs: []icons.Attr{{Name: "onerror", Value: "x"}}},
	})
	if out, want := string(svgMarkup(scriptTag)), shell+"</svg>"; out != want {
		t.Errorf("白名单外元素逃进了 markup:\ngot:  %s\nwant: %s", out, want)
	}

	// ③ 白名单外属性名 → 丢掉这条属性、元素与合法属性照留。
	badAttr := iconFixture("tabler", "x", "stroke", []icons.Element{
		{Tag: "path", Attrs: []icons.Attr{{Name: "onmouseover", Value: "x"}, {Name: "d", Value: "M0 0"}}},
	})
	if out, want := string(svgMarkup(badAttr)), shell+`<path d="M0 0"/></svg>`; out != want {
		t.Errorf("白名单外属性名逃进了 markup:\ngot:  %s\nwant: %s", out, want)
	}

	// 退化口径的另一半：SVG 为空时界面不至于空着——pieceFor 恒填 Letter。
	// （这条改前就是绿的：Letter 的恒填不依赖第二道闸，它证明的是"退到首字母"
	// 这个退化目标真的存在，而非闸本身。）
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "h", Name: "Home", URL: "http://10.0.0.1/", Icon: "tabler:home"}, ix, fc)
	if p.Letter == "" {
		t.Error("Letter 必须恒填，否则空 SVG 退到的是一张空脸而不是首字母色块")
	}
}

func TestProfilePage_内联数量与缓存代次(t *testing.T) {
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", withIcons(12)) // 12 条都有合法 icon（tabler:home）
	r := New(s, ix, fc)

	page, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	// 数被检单位而不是数标签：将来页面里出现任何装饰性 <svg> 都不该混进这个 12。
	if n := strings.Count(string(page.Body), `class="ic-svg ic-stroke"`); n != 12 {
		t.Errorf("内联了 %d 个 stroke 图标，要 12 个（每个用到的条目一个，不是整个索引）", n)
	}
	if strings.Contains(string(page.Body), `<img class="ic-img"`) {
		t.Error("这批条目没有 auto，不该出现 img")
	}

	again, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if again.ETag != page.ETag {
		t.Error("同 (档案,home+指纹,rev,bookmarks.mtime,profile 指纹,iconGen) 必须命中缓存")
	}

	// 这一段只测一个变量：图标缓存。书签在上面那次 save 之后一个字都不许再动
	// ——否则 rev/mtime 与 gen 两个失效源混在一个断言里，ETag 变了也可能只是
	// 因为"写了一次书签"，缓存键存在的理由（什么都没写、只有图标缓存变了）
	// 就永远没被量过（spec §5 第 5 条点名的谎话）。
	// rev 必须与上面那次 12 条的 save 不同：两次内容不同而 rev 相同的话，
	// 缓存键里唯一能分开它们的就只剩 bookmarks.json 的 mtime，而同刻两次写
	// 在 Linux 上会拿到完全相同的时间戳（P1-④ 评审 I-2 量到的同一个病），
	// 于是下面这条"缺图标角标"就在 Linux 上时红时绿。测试不许靠时钟运气。
	auto := withIcons(0)
	auto.Rev = 2
	auto.Groups[0].Items = []model.Item{{ID: "fav", Name: "F", URL: "http://10.0.0.9/", Icon: "auto"}}
	saveBookmarks(t, s, "dad", auto) // 此刻缓存里还没有这张图
	before, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(before.Body), `<img class="ic-img"`) {
		t.Fatal("缓存里还没有图，不该出现 img")
	}
	if !strings.Contains(string(before.Body), "图标未匹配") {
		t.Error("auto 无缓存要端出缺图标角标（spec §12 的“不假装成功”）")
	}

	// 只动缓存、不动书签：刚抓到的图必须不等下次写书签就显示。
	if _, err := fc.Put(icons.Origin("http://10.0.0.9/"), []byte("img"), "png"); err != nil {
		t.Fatal(err)
	}
	fresh, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ETag == before.ETag {
		t.Fatal("图标缓存变了而页面 ETag 没变：缓存键里少了代次")
	}
	if !strings.Contains(string(fresh.Body), `<img class="ic-img"`) {
		t.Errorf("新页面应当端出 favicon: %.200s", fresh.Body)
	}
}

// 裁定 U2 的证人：条目身上只带 --chip-h 这一个数，分组色不进 itemView；
// 分组色的判据只有 validatedAccent 一份，挂载点两个——section 的 --sec-accent
// 与它那一条侧栏行的色点（设计 §7 的三件套），条目身上一个都不许有。
// 非法 accent（磁盘读路径不校验它，见 model/check.go）要整个当没写——
// 一个字节都不许进页面，而不是被转义后留在 style 里。
func TestProfilePage_分组色只从判据进(t *testing.T) {
	ix, fc, s := assets(t)
	b := withIcons(0)
	// 夹具必须带条目：证人防的是"分组色从条目身上二次进场"（裁定 V3 的破坏
	// 剧本就是经 exec.go 往 itemView 里塞 style="--sec-accent:…"）。空分组
	// 让那种注入渲染不出来，证人就只剩一半硬度。
	b.Groups = []model.Group{
		{ID: "g1", Title: "甲组", Layout: model.LayoutTiles, Accent: "#4C8DFF", Items: []model.Item{
			{ID: "a1", Name: "甲一", URL: "http://10.0.0.1/", Icon: "tabler:home"},
		}},
		{ID: "g2", Title: "乙组", Layout: model.LayoutList, Accent: "red; --x:expr(url(/#\"))", Items: []model.Item{
			{ID: "b1", Name: "乙一", URL: "http://10.0.0.2/", Icon: "tabler:home"},
		}},
	}
	saveBookmarks(t, s, "dad", b)
	page, err := New(s, ix, fc).ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	// 数的是**属性形状** `style="--sec-accent:`，不是裸 token：base.css 内联进
	// 每页，h2::before 与 .gnav .dot 都写着 var(--sec-accent,var(--accent))，裸 token
	// 因此恒多出几处——按 Task 9 的老口径数 token 会在 CSS 重写这天改数而假红。
	// 别把它"简化"回去：属性形状数的才是"有人给元素设了分组色"这件事。
	// 现在的事实是两处挂载点、一份判据：每个有合法 accent 的分组恰好 2 次
	// （section 的 --sec-accent 与设计 §7 要求的那枚侧栏色点，同一个属性名），
	// 条目身上照旧 0 次——破坏剧本（exec.go 往 itemView 塞 style）任一处都让它变 3。
	if n := strings.Count(body, `style="--sec-accent:`); n != 2 {
		t.Errorf("--sec-accent 属性出现 %d 次，要恰好 2 处（合法那个分组的 section 与它的侧栏行；条目身上一个都不许有）:\n%.300s", n, body)
	}
	if !strings.Contains(body, `style="--sec-accent:#4c8dff"`) {
		t.Errorf("合法 accent 要小写后从 section 的 --sec-accent 进 CSS:\n%.300s", body)
	}
	for _, leak := range []string{"expr(", "red;", "--x:"} {
		if strings.Contains(body, leak) {
			t.Errorf("非法 accent 的碎片 %q 逃进了页面——判色发生在渲染侧，非法即当没写", leak)
		}
	}
	if strings.Contains(body, "--chip-bg") {
		t.Error("条目身上不许带分组色：那是第二个真相（见 exec.go 注释）")
	}
}
