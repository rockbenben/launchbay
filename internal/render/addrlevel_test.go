package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 卡片第二行印什么：`host`（默认）/ `full` / `none`（2026-10-19 加）。
//
// **这条钉的是行为不是键的存在**：键的存在由 `httpapi/prefs_test.go` 那份
// 名单管（十三颗键、状态走 aria-pressed），这里管的是"按下去之后卡片上那格
// 真的变了"。两条分开是因为坏法不同——键在但值没传下去，名单照样绿。
//
// 夹具一律走磁盘（SaveProfile）而不是直接喂 executor：那三格要经过
// load → addrLevelOf → tmpl 三段，绕开任何一段就等于没测那一段（与
// TestProfilePage_theme属性只表显式选择 同一处置）。
func addrLevelPage(t *testing.T, s *store.Store, level string) string {
	t.Helper()
	if level != "" {
		p, err := s.LoadProfile("dad")
		if err != nil {
			t.Fatal(err)
		}
		p.AddrLevel = level
		if err := s.SaveProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	return string(mustRender(t, s, "dad"))
}

func addrLevelSeed(t *testing.T, s *store.Store) {
	t.Helper()
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Groups = []model.Group{{
		ID: "g1", Title: "工具", Layout: model.LayoutTiles,
		Items: []model.Item{
			// 一条有说明、一条没说明——三档要分得开，这两条才够
			{ID: "a", Name: "有说明的那条", URL: "https://docs.google.com/x", Desc: "一句话说明"},
			{ID: "b", Name: "没说明的那条", URL: "https://nas.example.com/y"},
		},
	}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
}

// **默认（盘上没这个键）**：没说明的那条印主机名，不印完整地址。
//
// "没选过"与"选了 host"在屏上**本来就该分不开**——它们是同一件事，
// 而分得开才说明归一漏了（store/load.go 那条"停止编造默认值"的判据）。
func TestAddrLevel_默认档印主机名不印完整地址(t *testing.T) {
	s := seeded(t)
	addrLevelSeed(t, s)
	body := addrLevelPage(t, s, "")
	if !strings.Contains(body, "nas.example.com") {
		t.Error("默认档下没说明的那条该印主机名 nas.example.com")
	}
	if strings.Contains(body, `<i class="addr">`) {
		t.Error("默认档不该印完整地址（那是 full 那一档）")
	}
	if !strings.Contains(body, "一句话说明") {
		t.Error("默认档下有说明的那条该印说明")
	}
}

// **full**：两处完整地址都在，主机名与说明让位；并且落到 body 的属性上，
// 因为 CSS 靠它让这一格压过视图隐藏。
//
// 断言只认**人眼看到的那一格**（`<i>` / `<span class="host">`），不拿整页搜：
// 说明还会印进 `data-desc`（给 app.js 的搜索与编辑读，不是给人看的），
// 地址还会进 `title=` 与外部地址那一支——那些**不归这一格管**，全都不动。
func TestAddrLevel_full档印完整地址(t *testing.T) {
	s := seeded(t)
	addrLevelSeed(t, s)
	body := addrLevelPage(t, s, "full")
	for _, want := range []string{
		`<i class="addr">https://docs.google.com/x</i>`,
		`<i class="addr">https://nas.example.com/y</i>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("full 档该印 %s", want)
		}
	}
	if strings.Contains(body, `<i class="host">`) {
		t.Error("full 档不该还印主机名那一格")
	}
	if strings.Contains(body, `<i>一句话说明</i>`) {
		t.Error("full 档该印完整地址而不是说明——两件事在这一档里分得开")
	}
	if !strings.Contains(body, `data-addr-level="full"`) {
		t.Error("full 档没落到 body 的 data-addr-level 上：CSS 靠它让这一格压过视图隐藏")
	}
}

// **none**：那一格整个不存在（连空标签都不留）。
//
// 注意判据只查 `<i>` 与 `<span class="host">`：`data-desc` 还在——那是 app.js
// 搜索用的数据，**`none` 不该把数据也删掉**（搜到一条却什么都不显示，才是坏的）。
func TestAddrLevel_none档什么都不印(t *testing.T) {
	s := seeded(t)
	addrLevelSeed(t, s)
	body := addrLevelPage(t, s, "none")
	if strings.Contains(body, `<i class="addr">`) || strings.Contains(body, `<i class="host">`) {
		t.Error("none 档不该还留着第二行")
	}
	if strings.Contains(body, `<i>一句话说明</i>`) {
		t.Error("none 档不该还留着说明那一格")
	}
	if !strings.Contains(body, "data-desc=") {
		t.Error("none 档把 data-desc 也删了：那是 app.js 搜索用的数据，删掉它搜到一条却什么都不显示")
	}
	if !strings.Contains(body, `data-addr-level="none"`) {
		t.Error("none 档没落到 body 的 data-addr-level 上")
	}
}

// **大卡带里点不动的外部条目也认这一格**（2026-10-19 补）。
//
// 大卡带里那一条印的是 `host` 那一格，与它旁边那张能点的卡**同形**——所以
// 「第二行印什么」这一格说了算：`none` 档不许在它身上留一行。
//
// 从前它无条件印 `hostOf .URL`，于是 `none` 档在大卡带里**漏网**（能点的那张
// 收掉了、点不动的这张还在）——那一格于是说谎：用户显式选了「不显示」，屏上
// 还有一整行字。
//
// **正文卡里点不动的外部地址那一行不归这一格管**：那是"点不动的地址"那一行
// （`<code>` 加徽标与复制键），给人照着敲的，与"第二行"那一格是两件事。
func TestAddrLevel_大卡带里点不动的外部条目也认这一格(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Groups = []model.Group{{
		ID: "hot", Title: "每天必点", Layout: model.LayoutFeature,
		Items: []model.Item{
			// 没说明的：那一格印地址本身（`hostOf` 对非 http/https 原样返回，
			// 那类地址没有主机名可印，编一个出来更坏）
			{ID: "x", Name: "点不动的扩展页", URL: "chrome://extensions/"},
			// 有说明的：与旁边那张能点的卡同一句判据——有说明印说明
			{ID: "w", Name: "带说明的扩展页", URL: "chrome://apps/", Desc: "自建的应用"},
			{ID: "y", Name: "能点的那条", URL: "https://nas.example.com/z"},
		},
	}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}

	host := addrLevelPage(t, s, "host")
	for _, want := range []string{
		`<span class="host" title="chrome://extensions/">chrome://extensions/</span>`,
		`<span class="host" title="chrome://apps/">自建的应用</span>`,
		`<span class="host" title="https://nas.example.com/z">nas.example.com</span>`,
	} {
		if !strings.Contains(host, want) {
			t.Errorf("host 档下大卡带该印 %s", want)
		}
	}

	full := addrLevelPage(t, s, "full")
	for _, want := range []string{
		`<span class="host" title="chrome://apps/">chrome://apps/</span>`,
		`<span class="host" title="https://nas.example.com/z">https://nas.example.com/z</span>`,
	} {
		if !strings.Contains(full, want) {
			t.Errorf("full 档下大卡带该印 %s", want)
		}
	}

	// none 档：大卡带里**一条第二行都不留**——那两支都归这一格管。
	none := addrLevelPage(t, s, "none")
	if strings.Contains(none, `<span class="host"`) {
		t.Error("none 档下大卡带里还留着第二行：点不动的外部条目漏网了——那一格于是说谎")
	}
}

// **空串归一成 host，不留空档**（2026-10-19）。
//
// 判据只有 addrLevelOf 一份，而它在渲染时算、不往盘里编一个字（与 themeAttr
// 那个"auto 归一成空串"同一族处置）。**脏值也兜一层**：模板与 CSS 都在按这个
// 值做决定，而一个没被挡住的��会变成"屏上什么都不印"这种安静的错误。
func TestAddrLevel_空串与脏值都归一成host(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", "host"}, // 没选过
		{"host", "host"},
		{"full", "full"},
		{"none", "none"},
		{"nonsense", "host"},
	} {
		if got := addrLevelOf(tc.in); got != tc.want {
			t.Errorf("addrLevelOf(%q) = %q，要 %q", tc.in, got, tc.want)
		}
	}
}

// `full` 必须压过视图自己的隐藏规则（2026-10-19）。
//
// dense 视图与 ≤700px 磁贴都收 `.card i`（base.css 那两条），而 full 印的是
// `i.addr`——**同名不同类**。少了下面这条 CSS，用户显式选了 full 在那两档里
// 却拿不到，与"没选"在屏上分不开：那是 §8.1「点了没反应的东西不进界面」
// 禁掉的那种东西，而这一格是**用户自己点的**。
func TestFull档压过视图隐藏(t *testing.T) {
	css := readWeb(t, "web/base.css")
	for _, want := range []string{
		`[data-addr-level=full] .card i.addr`,
		`[data-addr-level=full] .tiles .card i.addr`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("base.css 里没有 %q：显式选了 full 的人在 dense 与窄屏磁贴下拿不到地址", want)
		}
	}
	// 反向证人：那两条隐藏规则**不许**改成收 `i.addr`（那会让显式选择也一起消失）
	if strings.Contains(css, `.card i.addr{display:none}`) {
		t.Error("有人把 full 那一档也一起收掉了：显式选择压不过视图，§8.1 说的正是这个")
	}
}

// **full 那一档不缩进**（2026-10-19 实拍后改）。
//
// 那条 `margin-left:32px` 是给**说明**留的：说明短，缩进一个图标位仍读得成一行。
// 而完整地址本身三四十个字符，在 150px 的窄磁贴里那 32px 占了内容区的 21%。
//
// **实拍（380px / 两列，每格 150px、内容区 130px）**：缩进版下名字那格被压到
// 98px，「有说明短地址」截成「有 说明短 地」，而下面 `https://x.com/a` 完整——
// **第一行是每天要认的名字，第二行只是补充，而补充把名字挤没了**。去掉缩进之后
// 两者齐左，地址少一截而名字完整。
//
// 这一条**只有实拍能发现**：它不违反任何契约，不撞任何测试，两档的输出都"对"，
// 只是读起来的优先级反了。
func TestFull档第二行不缩进(t *testing.T) {
	css := readWeb(t, "web/base.css")
	if !strings.Contains(css, "[data-addr-level=full] .tiles .card i.addr{margin-left:0}") {
		t.Error("full 档的第二行还在缩进一个图标位：150px 的窄磁贴里那 32px 占了内容区 21%，" +
			"实测把名字挤到 98px（截断），而地址却完整——必读的那一半被补充那半挤没了")
	}
}

// **单列的断点收回 430**（2026-10-19 实拍后改判）。
//
// 这条断点曾从 430 收到 340，理由是「430 那一档把 390 整个收成一列，一屏只见
// 10 条」——那个理由今天仍然成立。但收窄时那条算式是**估的**（注释写「两列时
// 每个格子约 180px」），实测最常见的手机宽度上只有 150px，于是 341–425px 这段
// 每天要认的名字被三行 clamp 截断，而 340px 以下反而拿到 268px。
//
// 实测分界（`full` 档，一条 16 字长名压边界）：**430px 时每格 175px 不截断，
// 425px 时 173px 开始截**。430 是量出来的分界，不是凑的整数。
//
// 钉的是**那个数**：它一旦又被收窄回去，被截断的正是「人认一个站点靠的恰恰是
// 它在哪个域上」的那一半。
func Test单列断点在430(t *testing.T) {
	css := readWeb(t, "web/base.css")
	if !strings.Contains(css, "@media (max-width:430px){.tiles,.rows,.profiles{grid-template-columns:minmax(0,1fr)}}") {
		t.Error("单列断点不在 430：341–425px 这段每格只有 133–170px，名字被三行 clamp 截断，" +
			"而 340px 以下反而拿到 268px——实测 430px 每格 175px 不截断、425px 时 173px 开始截")
	}
	if strings.Contains(css, "@media (max-width:340px){.tiles") {
		t.Error("单列断点又被收回 340 了：那段宽度下两列够不到 175px，名字被截断")
	}
}
