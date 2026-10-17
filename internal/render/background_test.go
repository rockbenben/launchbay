package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 这一页自己的背景图（2026-10-14）在渲染侧的三个判据。前两个的共同形状是
// **坏值不许变成一行输出**：盘上的 profile.json 能被人手改，而它的 Background
// 会进 `<html style=…>`——那里多一个引号就是一次属性注入。第三个是"坏背景不挡
// 页面"：那张图没了，这一页其余部分照常出。

// setBG 直接改盘上的 profile.json，绕开写侧（写侧只写自己算出来的名字）。
func setBG(t *testing.T, s *store.Store, id, name string, off bool) {
	t.Helper()
	if err := s.WithProfileLock(id, func() error {
		p, err := s.LoadProfile(id)
		if err != nil {
			return err
		}
		p.Background, p.BackgroundOff = name, off
		return s.SaveProfile(p)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundURL_坏值一律归成没有背景(t *testing.T) {
	cases := map[string]*model.Profile{
		"没设":        {ID: "dad"},
		"暂时不显示":     {ID: "dad", Background: "0123456789abcdef.png", BackgroundOff: true},
		"路径穿越":      {ID: "dad", Background: "../../data/profiles/dad/profile.json"},
		"形状不对":      {ID: "dad", Background: "0123456789ABCDEF.png"},
		"带引号（属性注入）": {ID: "dad", Background: `abc.png";background:url(http://evil/);--x:"`},
		"是缓存索引不是图":  {ID: "dad", Background: "index.json"},
	}
	for name, p := range cases {
		if got := paintedBackground(p); got != "" {
			t.Errorf("%s：backgroundURL = %q，要空串（模板那边只有「有/没有」两种形状）", name, got)
		}
	}
	got := paintedBackground(&model.Profile{ID: "dad", Background: "0123456789abcdef.jpg"})
	if got != "/backgrounds/0123456789abcdef.jpg" {
		t.Errorf("合法值算出的 URL = %q", got)
	}
	if paintedBackground(nil) != "" || backgroundFile(nil) != "" {
		t.Error("nil 档案也要归成空串，而不是崩在这一个函数上")
	}
}

func TestProfilePage_背景落在html上而不是body上(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的书签", "yt")
	setBG(t, s, "dad", "0123456789abcdef.jpg", false)

	body := string(mustPage(t, s, "dad"))
	if !strings.Contains(body, `data-bg="1"`) {
		t.Error("有背景时 <html> 上没有 data-bg：极光那层就还压在人挑的那张图上")
	}
	if !strings.Contains(body, "url(/backgrounds/0123456789abcdef.jpg)") {
		t.Errorf("背景 URL 没进那一段 style：%.300s", body)
	}
	// **必须落在 <html> 而不是 <body>**：base.css 里 html 与 body 各有一层背景，
	// 而按 CSS 的背景传播规则，html 有背景时 body 的背景不再铺满画布——写错一层，
	// 症状是「背景只在内容那一块出现，两侧露着纯色」。
	iHTML := strings.Index(body, "<html")
	iBody := strings.Index(body, "<body")
	if iHTML < 0 || iBody < 0 || iHTML > iBody {
		t.Errorf("style 落在 <html> 之后 <body> 之前才是对的：html@%d body@%d", iHTML, iBody)
	}
	// 暂时不显示：一段 style 都不该有（它是"不画"，不是"画一张透明的"）
	setBG(t, s, "dad", "0123456789abcdef.jpg", true)
	body = string(mustPage(t, s, "dad"))
	if strings.Contains(body, "data-bg=\"1\"") || strings.Contains(body, "/backgrounds/") {
		t.Errorf("暂时不显示时不该有任何背景声明：%.300s", body)
	}
	// 坏值：页面照常出（这一页其余部分一个字不少），只是没有背景
	setBG(t, s, "dad", `x.png";--:url(//evil)`, false)
	body = string(mustPage(t, s, "dad"))
	if strings.Contains(body, "/backgrounds/") || strings.Contains(body, "evil") {
		t.Errorf("坏引用被写进页面了：%.300s", body)
	}
	if !strings.Contains(body, "爸爸的书签") {
		t.Error("坏背景不许挡页面：这一页其余部分照常出")
	}
}

func TestPrefsPanel_背景那一节画得出来且三张表单各自只交自己那件事(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的书签", "yt")
	setBG(t, s, "dad", "0123456789abcdef.jpg", false)

	body := string(mustPrefs(t, s, "dad"))
	// 预览走**同一个端点**，于是它同时是"这张图还在盘上"的证人
	if !strings.Contains(body, `src="/backgrounds/0123456789abcdef.jpg"`) {
		t.Error("面板里没有背景预览")
	}
	for _, want := range []string{
		`action="/prefs/background" enctype="multipart/form-data"`, // 上传那一张
		`name="file"`,             // 字段名
		`name="hidden" value="1"`, // 显示着 → 给的是「暂时不显示」
		`name="remove" value="1"`, // 移除
	} {
		if !strings.Contains(body, want) {
			t.Errorf("面板里少了 %q", want)
		}
	}
	if strings.Count(body, `action="/prefs/background"`) != 3 {
		t.Errorf("背景这一节该有三张独立表单（一次按下只改一件事），实到 %d 张",
			strings.Count(body, `action="/prefs/background"`))
	}
	// 「暂时不显示」与「恢复显示」**只画一颗**：都画出来的话，页面上同时有两颗，
	// 而其中一颗按下去是个空动作（§8.1：点了没反应的东西不进界面）。
	if strings.Contains(body, `name="hidden" value="0"`) {
		t.Error("现在是显示着的，却同时画了「恢复显示」那一颗")
	}
	setBG(t, s, "dad", "0123456789abcdef.jpg", true)
	body = string(mustPrefs(t, s, "dad"))
	if strings.Contains(body, `name="hidden" value="1"`) || !strings.Contains(body, `name="hidden" value="0"`) {
		t.Error("暂时不显示时该给的是「恢复显示」那一颗，而且只这一颗")
	}
	// 坏引用：这一节退回"还没有背景图"的说法，而面板其余部分照常
	setBG(t, s, "dad", "index.json", false)
	body = string(mustPrefs(t, s, "dad"))
	if strings.Contains(body, "/backgrounds/index.json") {
		t.Error("坏引用被写进面板了")
	}
	if !strings.Contains(body, "页面名字") {
		t.Error("坏背景不许挡台面")
	}
}

// 面板里那一节的**次序**也是判据：这一屏读作「这一页长什么样」的几行（改名/怎么看/
// 分组导航/主题/背景）连成一片，然后才是两个出口（回退与历史），最后才是落在书签
// 数据上的「分组」。
//
// 背景那一节 2026-10-14 插进去时落在出口**下面**，于是这屏读成「四行设置 → 两个
// 出口 → 又一行设置」——设置与出口在一屏里各断一次。屏上看不出毛病，于是没人报，
// 只有这一条会红。
func TestPrefsPanel_设置连成一片而出口在它们下面(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的书签", "yt")
	body := string(mustPrefs(t, s, "dad"))
	// 锚点用**结构形状**而不是 i18n 键或译文：页面里出的是译文（而「分组」又是
	// 「分组导航」的前缀，按字找会先找到错的那一节）。
	at := map[string]int{}
	for name, anchor := range map[string]string{
		"改名":    `class="prefs-label" for="lb-page-title"`,
		"怎么看":   `name="view" value="groups"`,
		"分组导航":  `name="layout" value="sidebar"`,
		"主题":    `name="theme" value="auto"`,
		"背景":    `class="prefs-bg"`,
		"回退与历史": `href="/history"`,
		// 「分组」这一节的锚点从 `prefs-groups-head` 换成 `prefs-groups`（2026-10-14）：
		// 那个 div 只画了个并排的小标签，已被 `h3.prefs-sec` 取代——三个分节头现在
		// 共用一个类，所以**不能拿分节头当锚点**（三处都匹配，取到的是第一处）。
		// 节的**正文容器**仍然是唯一的那一个 div。
		"分组": `class="prefs-groups"`,
	} {
		i := strings.Index(body, anchor)
		if i < 0 {
			t.Fatalf("面板里没有「%s」那一节（锚点 %q 没匹配上）", name, anchor)
		}
		at[name] = i
	}
	for _, pair := range [][2]string{
		{"改名", "怎么看"}, {"怎么看", "分组导航"}, {"分组导航", "主题"},
		{"主题", "背景"}, {"背景", "分组"}, {"分组", "回退与历史"},
	} {
		if at[pair[0]] > at[pair[1]] {
			t.Errorf("「%s」该排在「%s」前面（屏上是「这一页长什么样」的几行与「分组」设置连成一片，最后才是出口）",
				pair[0], pair[1])
		}
	}
}

// 背景图上面那层压暗（--scrim）不许薄到让正文在**纯白照片**上读不出来。
//
// 为什么拿"纯白照片"当最坏情况：`background-image:linear-gradient(var(--scrim),…),url(图)`
// 的第一层是压在图上的，最亮的图把压暗层顶到最淡——而人的照片里最亮的那张常常
// 真的是白底（证件照、白板照、截图）。§7.5 的 4.5:1 是**硬数字**，而这一层的
// 厚薄是个人审美与可读性之间的取舍：把同一个数钉在 4.5:1 上，"我想让图更显眼"
// 就必须由人显式说，而不能靠调薄压暗层悄悄把可读性赔进去。
//
// 顺带钉住一件事：**亮色那两条通路必须同数**（`.prefs-bg` 那条不比，因为压暗层
// 在亮档是提亮而不是压暗，值本来就不同；亮档的两条通路才是同一个事实的两处表述）。
func TestCSS_压暗层不许薄到白照片上的正文读不出来(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := stripCSSComments(string(raw))
	white, ok := cssColor(t, "#ffffff")
	if !ok {
		t.Fatal("读不出纯白这个夹具")
	}
	for _, path := range []struct {
		selector string
		name     string
	}{
		{":root{", "dark :root"},
		{"[data-theme=light]{", "light [data-theme=light]"},
	} {
		vars := cssVars(t, css, path.selector)
		ink, ok1 := cssColor(t, vars["--ink"])
		scrim, ok2 := cssColor(t, vars["--scrim"])
		if !ok1 || !ok2 {
			t.Errorf("%s：这个通路上没有 --ink 或 --scrim（--ink=%q --scrim=%q）", path.name, vars["--ink"], vars["--scrim"])
			continue
		}
		got := wcagContrast(ink, compositeOver(scrim, white))
		t.Logf("%s：正文压在「纯白照片 + 这一档的压暗层」上 = %.2f:1（门槛 ≥4.5:1，--scrim=%s）", path.name, got, vars["--scrim"])
		if got < 4.5 {
			t.Errorf("%s 的 --scrim（%s）太薄：正文压在纯白照片上只有 %.2f:1，"+
				"而 §7.5 要 4.5。想让背景更显眼就换别的手段（收窄色板/把主色提亮），别从这里扣可读性",
				path.name, vars["--scrim"], got)
		}
	}
	// 亮色两条通路同数（与色板那一族同一条判据）
	lightA := cssVars(t, css, "[data-theme=light]{")["--scrim"]
	lightB := cssVars(t, css, ":root:not([data-theme]){")["--scrim"]
	if lightA != lightB {
		t.Errorf("亮档两条通路的 --scrim 分家了：%q vs %q——选 light 的人与跟着系统走的人会拿到不同的可读性", lightA, lightB)
	}
}
func mustPage(t *testing.T, s *store.Store, id string) []byte {
	t.Helper()
	p, err := renderer(t, s).ProfilePage(PageRequest{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return p.Body
}
