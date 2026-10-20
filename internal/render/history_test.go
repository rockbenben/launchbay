package render

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 改动历史那一页的**类契约**：模板挂出来的每一个类，内联下发的那份 CSS 里
// 都要有规则接住它，而 `.ops` 必须挂在 `.plain` **里面**——那条规则是后代
// 选择器，类挂到别的元素上就匹配不上（`.tech` 那次把类挂到 `<pre>` 自己身上，
// 报告就被默认的 pre 不换行横向滚出视口，是浏览器实拍才发现的）。
//
// 这一族第一次落地踩的是反向的那一枪：`.rep-list` 住在 app.css，而导入页这类
// 零脚本页面只内联 base.css、不引 app.css（§7.2）。类名对得上、规则在另一个
// 文件里，页面上就是没样式——扫 CSS 的那条闸只看 base.css，看不见这个错位。
// 所以这里把"渲染出来的类"与"内联下发的规则"**两头一起扫**。
func Test历史页的类都在内联CSS里有规则(t *testing.T) {
	ix, fc, s := assets(t)
	page, err := New(s, ix, fc).HistoryPage(HistoryView{
		Profile: "dad", Heading: "dad 的改动历史", Explain: "这一页记的是动作。",
		Rows: []HistoryRow{{
			At: "2026-10-13T06:00:00Z", Channel: "网页上", Rev: 2, Count: 9,
			Ops:  []string{"改名 · NAS › 群晖", "删掉 · NAS › 旧的一条"},
			More: "…另外还有 7 处没列出来",
		}},
	}, i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	css := string(InlineCSS())

	// 每一样都是页面上真实存在的**钩子**：`class="section"` 曾经在这里被列过
	// 一次，而 base.css 里根本没有那一条规则——那一页用的是裸 `<section>`
	// 加全局那条 `section{margin:…}`。列一个不存在的类，等于把这一条闸
	// 变成"页面上有没有这个类"的空转检查。
	for _, cls := range []string{`class="hist"`, `class="plain"`, `class="ops"`,
		`class="badge"`, `class="hint"`, `<section>`} {
		if !strings.Contains(body, cls) {
			t.Errorf("页面上没有 %s——这一页的形状变了，下面那半条闸要跟着看", cls)
		}
	}
	for _, rule := range []string{`.hist{`, `.hist .ops{`, `.plain{`, `.badge{`,
		`.hint{margin:`, `section{margin:`, `h2::before{content:"`} {
		if !strings.Contains(css, rule) {
			t.Errorf("内联 CSS 里没有 %s 这条规则，页面上那个类就是没样式的", rule)
		}
	}

	// 嵌套关系：`.ops` 那一簇要在 `.plain` 那张卡片**里面**。
	li := strings.Index(body, `<li class="plain">`)
	ops := strings.Index(body, `<ul class="ops">`)
	close := strings.Index(body[li:], "</li>")
	if li < 0 || ops < 0 || close < 0 || li+close < ops {
		t.Errorf(".ops 没有挂在 .plain 里面（那条规则是后代选择器，匹配不上等于没写）")
	}
}

// noon 是"本机今天的中午十二点"再加减几天。
//
// **不用 `now - 25h`**：凌晨跑的时候那会跨**两**个日历日，于是这一条每天
// 有一段时间是假绿——而它要钉的正是"按日历日切节"。
func noon(days int) time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d, 12, 0, 0, 0, time.Local).AddDate(0, 0, days)
}

var dayH2 = regexp.MustCompile(`<h2>(.*?)</h2>`)

// 一天一节：切分与那两个相对词（今天/昨天）都在渲染侧算，因为它们要读
// **本机时区**与**界面语言**——httpapi 那头算得出行，算不出"这一台机器上的
// 人把哪一天叫今天"。
func Test历史页按天切节(t *testing.T) {
	ix, fc, s := assets(t)
	far := noon(-9)
	page, err := New(s, ix, fc).HistoryPage(HistoryView{
		Profile: "dad", Heading: "dad 的改动历史",
		Rows: []HistoryRow{
			{At: noon(0).Format(time.RFC3339), Channel: "网页上", Count: 1, Rev: 3},
			{At: noon(0).Format(time.RFC3339), Channel: "AI 助手", Count: 2, Rev: 4},
			{At: noon(-1).Format(time.RFC3339), Channel: "命令行", Count: 1, Rev: 5},
			{At: far.Format(time.RFC3339), Channel: "浏览器扩展", Count: 1, Rev: 6},
			{At: "不是时间", Channel: "网页上", Count: 1, Rev: 7},
		},
	}, i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	labels := dayH2.FindAllStringSubmatch(body, -1)
	var got []string
	for _, l := range labels {
		got = append(got, l[1])
	}
	// 跨年那一档独立说一遍（不借 dayDate 自己算期望值，那是拿实现当规格）：
	// 同一年的日期**不带**年份，跨年必须带——只写「4 Oct」的人读到去年那一节，
	// 会以为那是今年十月。
	wantDate := far.Format("1月2日")
	if far.Year() != time.Now().Year() {
		wantDate = far.Format("2006年1月2日")
	}
	want := []string{"今天", "昨天", wantDate, "时间不明"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("节的切分与顺序不对：\n  要 %v\n  得 %v\n页面：%s", want, got, body)
	}
	// 解不出时间的行**不许静默丢掉**：这一页存在的全部意义是"一行都没漏"
	// （与导入报告"## 没能进来"逐条点名、回退页"另有 N 份快照读不出来"同纪律）。
	if n := strings.Count(body, `<li class="plain">`); n != 5 {
		t.Errorf("五行动作该画五张卡片，画了 %d 张", n)
	}
	// 英文那一支：相对词与日期形状都要跟着语言换（`01-02` 那种数字日期在
	// 英语读者那里分不清是月在前还是日在前）。
	en, err := New(s, ix, fc).tmpl.history(HistoryView{Profile: "dad", Rows: []HistoryRow{
		{At: noon(-1).Format(time.RFC3339), Channel: "web", Count: 1},
		{At: noon(-9).Format(time.RFC3339), Channel: "cli", Count: 1},
	}}, i18n.Load(i18n.En))
	if err != nil {
		t.Fatal(err)
	}
	enBody := string(en)
	if !strings.Contains(enBody, "<h2>Yesterday</h2>") {
		t.Errorf("英文那一支没把昨天说成 Yesterday：%s", head(enBody))
	}
	wantEn := far.Format("2 Jan")
	if far.Year() != time.Now().Year() {
		wantEn = far.Format("2 Jan 2006")
	}
	if !strings.Contains(enBody, "<h2>"+wantEn+"</h2>") {
		t.Errorf("英文那一支的日期形状不对（要 %q）：%s", wantEn, head(enBody))
	}
}

// 一行一次动作，而**没有内容**可列。这一页不许把书签的本体抄进来（§4.6：
// 审计行只有 `{op, path}`，一记 value 就变成第二份书签数据，还会带出地址
// 与备注）。这一条钉的是"这一页没有第二条数据通路"——将来谁想给历史行加
// "改成了什么"，红的是这一条，逼他先回去读 §4.6。
func Test历史页不抄内容(t *testing.T) {
	ix, fc, s := assets(t)
	page, err := New(s, ix, fc).HistoryPage(HistoryView{
		Profile: "dad", Heading: "dad 的改动历史",
		Rows: []HistoryRow{{
			At: "2026-10-13T06:00:00Z", Channel: "AI 助手", Count: 1, Rev: 2,
			Ops: []string{"改地址 · NAS › 群晖"},
		}},
	}, i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	for _, banned := range []string{"http://10.0.0.", "sha256$", `"pin"`, "bookmarks.json"} {
		if strings.Contains(body, banned) {
			t.Errorf("历史页上出现了 %q：那一页只许说动作，不许把内容与磁盘抄进来", banned)
		}
	}
	if strings.Count(body, `<li class="plain">`) != 1 {
		t.Error("一行一次动作：这一发只该有一张卡片")
	}
}

// 一条都没有：那一支要说清"为什么是空的"，不许留一块空白。
// 空白看着像设计如此，而这一页的空白尤其容易被读成"它坏了"。
func Test历史页零行说清为什么(t *testing.T) {
	ix, fc, s := assets(t)
	page, err := New(s, ix, fc).HistoryPage(HistoryView{
		Profile: "dad", Heading: "dad 的改动历史", Explain: "这一页记的是动作。",
	}, i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	if !strings.Contains(body, "还没有改动记录") {
		t.Errorf("零行那一支没把话说出来：%s", head(body))
	}
	if strings.Contains(body, `<li class="plain">`) || strings.Contains(body, `<ul class="hist">`) {
		t.Error("零行那一支还画了列表壳")
	}
	if strings.Contains(body, "⟨") {
		t.Errorf("有键没查到：%s", head(body))
	}
}

func head(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
