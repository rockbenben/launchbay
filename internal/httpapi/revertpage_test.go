package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
)

// 回退页的零 JS 两步确认（2026-10-11）。
//
// 这一族最要紧的三条判据各有一条证人：
//   1. **目标不接受申报**——管理员进得了这一页也只能退自己那一份（2026-10-10 的
//      承重边界在浏览器这一面上同样成立）；
//   2. **坏档不挡**——revert 正是数据文件坏掉时的出路，这一页要是把它挡在门外，
//      出路就只剩 API 与命令行；
//   3. **令牌与无会话一律 303 且不写盘**——它是 openapi 那张 tokensRefusedByForm
//      豁免表里两行的合法性来源（那张表说"文档有没有跟着行为走"，行为由这里钉）。

const revertDadCookie = "lb_profile=dad"

// revertPost 走浏览器无 JS 时的编码：application/x-www-form-urlencoded。
// postForm 那一份不带令牌，而"带令牌也要拒"正是这里要验的一格。
func revertPost(h http.Handler, form, cookie, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/revert", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// twoRevisions 让 dad 走到 rev2，于是快照里躺着 rev1——一个"刚才那次改动不对"
// 的现场，正是这一页要解决的那一种。
func twoRevisions(t *testing.T, h http.Handler) {
	t.Helper()
	body := `{"profile":"dad","by":"ai","expectedRev":1,"ops":[
	  {"op":"set","path":"groups/nas/items/dsm/name","value":"改过的"}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
	}
}

// 这一页存在的理由是"让人认出要退哪一版"。所以判据不是"页面出来了"，而是
// **每格带着认得出它的信息**：版本号 + 条目数 + 分组名。一个只印 "rev 12" 的
// 列表回答不了任何问题。
func index_or_zero(s, sub string) int {
	if i := strings.Index(s, sub); i >= 0 {
		return i
	}
	return 0
}

func TestRevertPage_把版本列成人认得出的样子(t *testing.T) {
	_, h := setupBatch(t)
	twoRevisions(t, h)

	rec := get(h, "/revert", revertDadCookie)
	if rec.Code != 200 {
		t.Fatalf("该 200, got %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`action="/revert"`,     // 表单指向第二步
		`name="to-rev"`,        // 那一族的字段名
		"<legend>选一版</legend>", // 组标题（读屏软件靠它知道这一组 radio 是什么）
		"第 1 版",                // 版本号
		"1 条书签",                // 条目数
		"NAS",                  // 分组名
		"现在这份会先存成一份快照",         // 后果写死
		"回我的页面",                // 返回徽标回档案页，不是回管理页
	} {
		if !strings.Contains(body, want) {
			t.Errorf("页面上少了 %q——一个只有版本号的列表回答不了「退哪一版」", want)
		}
	}
	// 零 JS：这一族是「关掉脚本照样成立」的那一半，预算闸也照它量。
	if n := strings.Count(body, "<script"); n != 0 {
		t.Errorf("页里有 %d 处 <script——两步确认的判据是零 JavaScript", n)
	}
	// 表单里不许有 profile：目标从会话推导，一个由客户端说了算的字段等于把
	// "退哪一份"从服务端判据降成客户端声明。
	if strings.Contains(body, `name="profile"`) {
		t.Error("表单里出现了 profile 字段——目标必须只从会话推导")
	}
	// 预选**次新**的那一版：最新一份快照就是"刚才存下来的当前状态"，
	// 默认选它等于让最常见的那个意图（退掉刚才那次改动）需要多动一次手。
	if !strings.Contains(body, `value="1" checked`) {
		t.Error("没有预选次新版那一格")
	}
	// 页面上不许残留 markdown。`{{.Explain}}` 走 html/template 的转义，
	// **强调**在页面上就是两个星号——初稿真这么写过，被浏览器实拍逮到，
	// 而全部单测都绿着（它们断言的子串里没有那对星号）。与导入报告那条同一纪律。
	if strings.Contains(body, "**") {
		t.Error("页面上出现了 **——Explain 是纯文本，强调靠句子不靠符号")
	}
	if strings.Contains(body, "`") {
		t.Error("页面上出现了反引号——磁盘文件名与代码词不进页面（删档页那条同款理由）")
	}
	// 后果说明里那两处空行必须真的断成段：`.explain` 是 pre-line，
	// 没有它"现在是这样"与"回退会怎样"会挤成一段（这一页的全部意义就在读清它）。
	if !strings.Contains(body, "<p class=\"explain\">") {
		t.Error("读数来自没有后果说明段的页面")
	}
}

// 预选哪一格是一条**判断**，不是随手挑的：WriteSnapshot 在写之前拍，所以最新
// 那份快照就是"上一次改动之前的样子"——而人点进这一页的绝大多数理由是
// "刚才那次改动不对"。三档版本才看得出选错没（两档时最新那份同时也是唯一一份）。
func TestRevertPage_预选的是上一次改动之前那一版(t *testing.T) {
	s, h := setupBatch(t)
	twoRevisions(t, h) // rev2，快照 = [1]
	if rec := post(h, "/api/v1/batch",
		`{"profile":"dad","by":"ai","expectedRev":2,"ops":[
		  {"op":"set","path":"groups/nas/items/dsm/name","value":"又改了一次"}]}`,
		"", dadTok); rec.Code != 200 {
		t.Fatalf("第二次写入失败: %d %s", rec.Code, rec.Body)
	}
	revs, err := s.ListSnapshots("dad")
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 2 || revs[0] != 2 {
		t.Fatalf("夹具前提不对：快照 = %v（想要 [2 1]）", revs)
	}
	body := get(h, "/revert", revertDadCookie).Body.String()
	if !strings.Contains(body, `value="2" checked`) {
		t.Errorf("没有预选 rev2（= 上一次改动之前那一份）；快照降序是 %v", revs)
	}
	// 另一格不许同时被选中：两个 radio 共一个 name，checked 只能有一个。
	// **先剪掉 <style> 再数**：base.css 整份内联进每一张页面，而选中态的样式里
	// 就有 `input:checked` 这个选择器——整篇数子串会被 CSS 喂饱（base.css 里
	// "注释别写尖括号形状串"那条警告说的是同一件事，只是那次是 <details>）。
	// 数的是**属性形式** ` checked`，不是裸 token。
	body = body[:index_or_zero(body, "<style>")] + body[index_or_zero(body, "</style>")+8:]
	if n := strings.Count(body, ` checked`); n != 1 {
		t.Errorf("页面上有 %d 处 checked——单选组里只能有一个", n)
	}
}

// 一份快照都没有时**不画表单**，而是说清为什么。画一个只有按钮的空表单，
// 等于让人点了之后吃一个没有出路的表单提交。
func TestRevertPage_一份快照都没有时不画表单(t *testing.T) {
	_, h := setupBatch(t) // 只 SaveBookmarks，没有走过写入 ⇒ 零快照
	rec := get(h, "/revert", revertDadCookie)
	if rec.Code != 200 {
		t.Fatalf("该 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "还没有可回退的版本") {
		t.Error("这一页没把「为什么不能退」说清楚")
	}
	if strings.Contains(body, `<form method="post"`) {
		t.Error("零份快照时仍画了表单——点了只有一条没有出路的路")
	}
}

// 令牌与无会话一律 303 回选择页、**不带提示**，且一个字节都不写盘。
// 这是 tokensRefusedByForm 那两行的行为来源。
func TestRevertPage_令牌与无会话都拒且不写盘(t *testing.T) {
	s, h := setupBatch(t)
	twoRevisions(t, h)
	before, _ := s.LoadBookmarks("dad")

	for _, tc := range []struct {
		name, cookie, bearer string
	}{
		{"GET 带作用域令牌", "", dadTok},
		{"GET 带管理员令牌", "", adminTok},
		{"GET 无会话", "", ""},
		{"POST 带令牌", revertDadCookie, dadTok},
		{"POST 无会话", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if strings.HasPrefix(tc.name, "GET") {
				req := httptest.NewRequest("GET", "/revert", nil)
				if tc.cookie != "" {
					req.Header.Set("Cookie", tc.cookie)
				}
				if tc.bearer != "" {
					req.Header.Set("Authorization", "Bearer "+tc.bearer)
				}
				rec = httptest.NewRecorder()
				h.ServeHTTP(rec, req)
			} else {
				rec = revertPost(h, "to-rev=1", tc.cookie, tc.bearer)
			}
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("该 303, got %d %s", rec.Code, rec.Body)
			}
			if loc := rec.Header().Get("Location"); loc != "/" {
				t.Errorf("该回 /（不带提示）, got %q", loc)
			}
		})
	}
	after, _ := s.LoadBookmarks("dad")
	if before.Rev != after.Rev || len(before.Groups) != len(after.Groups) {
		t.Error("被拒的请求动了盘")
	}
}

// 回退是一次**新写入**：rev 前进、历史追加、`by` 记 web。
func TestRevertForm_退回指定版本并产生新rev(t *testing.T) {
	s, h := setupBatch(t)
	twoRevisions(t, h)

	rec := revertPost(h, "to-rev=1", revertDadCookie, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("该 303, got %d %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("成功该回 /, got %q", loc)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 3 || b.Groups[0].Items[0].Name != "群晖" {
		t.Fatalf("内容没回到 rev1：rev=%d name=%q", b.Rev, b.Groups[0].Items[0].Name)
	}
	hist, _ := s.ReadHistory("dad", 0)
	if len(hist) != 2 {
		t.Fatalf("历史该追加不抹：%+v", hist)
	}
	if hist[0].By != "web" {
		t.Errorf("浏览器这条该记 by=web, got %q", hist[0].By)
	}
	if hist[0].Ops[0].Op != "revert" || hist[0].Ops[0].Path != "snapshots/1" {
		t.Errorf("历史里该自成一式：%+v", hist[0].Ops)
	}
}

// 目标不接受申报：**表单里塞 profile=kid 也只能退 dad 自己**。
// 管理员管账号、不碰内容（2026-10-10 的边界）——即使 kid 的 PIN 就是他设的。
func TestRevertForm_目标不接受申报(t *testing.T) {
	s, h := setupBatch(t)
	twoRevisions(t, h)
	if rec := post(h, "/api/v1/batch",
		`{"profile":"kid","by":"ai","expectedRev":1,"ops":[
		  {"op":"set","path":"groups/g1/items/cartoon/name","value":"改坏的"}]}`,
		"", kidTok); rec.Code != 200 {
		t.Fatalf("kid 的前置写入失败: %d %s", rec.Code, rec.Body)
	}

	rec := revertPost(h, "profile=kid&to-rev=1", revertDadCookie, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("该 303, got %d %s", rec.Code, rec.Body)
	}
	dad, _ := s.LoadBookmarks("dad")
	if dad.Groups[0].Items[0].Name != "群晖" {
		t.Errorf("退的该是 dad 自己：name=%q", dad.Groups[0].Items[0].Name)
	}
	kid, _ := s.LoadBookmarks("kid")
	if kid.Groups[0].Items[0].Name != "改坏的" || kid.Rev != 2 {
		t.Errorf("kid 的书签被动过了：rev=%d name=%q", kid.Rev, kid.Groups[0].Items[0].Name)
	}
}

// 三种失败各说各的事，且**都不能说成功**：
// 没填对 → 重选；那一版不在了 → 重新看列表；存不下 → 明确"一个字都没变"。
func TestRevertForm_字段没填对说人话(t *testing.T) {
	s, h := setupBatch(t)
	twoRevisions(t, h)

	for _, tc := range []struct{ form, want string }{
		{"", "/revert?err=badsnap"},
		{"to-rev=", "/revert?err=badsnap"},
		{"to-rev=abc", "/revert?err=badsnap"},
		{"to-rev=-1", "/revert?err=badsnap"},
		{"to-rev=9999", "/revert?err=gone"},
	} {
		t.Run(tc.form, func(t *testing.T) {
			rec := revertPost(h, tc.form, revertDadCookie, "")
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("该 303, got %d %s", rec.Code, rec.Body)
			}
			if loc := rec.Header().Get("Location"); loc != tc.want {
				t.Errorf("Location = %q, want %q", loc, tc.want)
			}
		})
	}
	// 页面上真的印了那句话（那张白名单闸只保证键在表里，印不印是另一回事）。
	for _, key := range []string{"badsnap", "gone", "nosnap"} {
		p := get(h, "/revert?err="+key, revertDadCookie)
		if !strings.Contains(p.Body.String(), noticeOf(i18n.Default, key)) {
			t.Errorf("键 %q 在页面上没印出人话", key)
		}
	}
	b, _ := s.LoadBookmarks("dad")
	if b.Rev != 2 || b.Groups[0].Items[0].Name != "改过的" {
		t.Errorf("失败的提交动了盘：rev=%d name=%q", b.Rev, b.Groups[0].Items[0].Name)
	}
}

// 坏档不挡这一页，而且**退得回去**——这是它作为"坏档出路"的一部分。
func TestRevertPage_坏档也能进来退(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	// 把当前这份写坏（模拟手滑截断成半个 JSON）。
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte(`{"rev":2,"groups":[`), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := get(h, "/revert", revertDadCookie)
	if rec.Code != 200 {
		t.Fatalf("坏档不该被挡在门外：%d %s", rec.Code, rec.Body)
	}
	// 照实说"现在这一份的**书签**读不出来"：坏的是书签文档（profile.json 读得动，
	// 否则根本走不到回退页）。说成"这一份读不出来"会让人以为整个档案没了。
	if !strings.Contains(rec.Body.String(), "这一份的书签读不出来") {
		t.Error("没照实说「现在这一份的书签读不出来」")
	}
	if !strings.Contains(rec.Body.String(), `name="to-rev"`) {
		t.Error("坏档时连可回退的版本都不列——这一页正是这种时候的出路")
	}
	if rec := revertPost(h, "to-rev=1", revertDadCookie, ""); rec.Code != http.StatusSeeOther {
		t.Fatalf("坏档该退得回去: %d %s", rec.Code, rec.Body)
	}
	if b, err := s.LoadBookmarks("dad"); err != nil || b.Groups[0].Items[0].Name != "群晖" {
		t.Fatalf("没退回去: err=%v", err)
	}
}

// 读不出来的快照**不列但报数**。静默少列几份会让人以为"只有这几个版本"，
// 而一份躺在那里不吭声的坏快照才是真正该被说出口的事。
func TestRevertPage_读不出来的快照要点名(t *testing.T) {
	s, h := setupBatch(t)
	twoRevisions(t, h)
	bad := filepath.Join(s.SnapshotsDir("dad"), "77", "bookmarks.json")
	if err := os.MkdirAll(filepath.Dir(bad), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(`{ 这不是书签文件`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/revert", revertDadCookie)
	body := rec.Body.String()
	if strings.Contains(body, "第 77 版") {
		t.Error("读不出来的快照被列出来了——点了只会吃一个 404")
	}
	if !strings.Contains(body, "另有 1 份快照读不出来") {
		t.Errorf("没报数：\n%s", body)
	}
}

// 「等 N 组」里的 N 是**那一版真实的分组数**，不是"页面上收下了几个名字"。
//
// 截断只是排版手段；一旦截组名的顺手也把总数截了，这一格报的就是一件没发生过的
// 事——有八个分组的人读到"等 3 组"，会据此判定"这版不是我要的那版"，而他依据的
// 数字是假的。这一页存在的唯一理由就是让人认出版本，报错的数比不报更坏。
func TestRevertPage_组名截断不许把总组数一起截掉(t *testing.T) {
	s, h := setupBatch(t)
	it := func(id, name string) model.Item {
		return model.Item{ID: id, Name: name, URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}}
	}
	long := "这个分组的名字特别长用来测试侧栏里的截断行为"
	four := &model.Bookmarks{Rev: 2, Title: "爸爸的台", Groups: []model.Group{
		{ID: "nas", Title: "NAS 与存储", Layout: model.LayoutList, Items: []model.Item{it("dsm", "群晖")}},
		{ID: "long", Title: long, Layout: model.LayoutList, Items: []model.Item{it("qbit", "下载")}},
		{ID: "media", Title: "影音", Layout: model.LayoutList, Items: []model.Item{it("jf", "影视库")}},
		{ID: "net", Title: "网络", Layout: model.LayoutList, Items: []model.Item{it("cf", "控制台")}},
	}}
	if err := s.SaveBookmarks("dad", four); err != nil {
		t.Fatal(err)
	}
	// 只有真写入才会给"现在这一版"拍下快照，那份快照才是这里的四分组形状。
	body := `{"profile":"dad","by":"ai","expectedRev":2,"ops":[
	  {"op":"add","path":"groups/nas/items","value":{"id":"pt","name":"Portainer","url":"http://10.0.0.9:9443/"}}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
	}

	page := get(h, "/revert", revertDadCookie).Body.String()
	if !strings.Contains(page, "（共 4 组）") {
		t.Errorf("四分组那一版没报出真实组数（那一格报的是截断后收下几个名字）:\n%s", page)
	}
	// 名字只印前面几个、每个按字截：整串长名上了页面就等于把列表撑成书签页。
	if strings.Contains(page, long) {
		t.Error("长分组名整串上了页面——它比这一格要回答的问题长得多")
	}
	// 截多少由 clipRunes 那个参数决定，这里跟着它算而不是抄一遍字面量：
	// 真要改那一格得改代码，改代码就要动这条证人，而这条证人会当场指出对不上。
	clipped := string([]rune(long)[:12]) + "…"
	if !strings.Contains(page, clipped) {
		t.Errorf("长分组名没截成 %q:\n%s", clipped, page)
	}
	// 版本号前面要有落盘时间：这一页回答的是"上周三那版"，裸版本号答不了时间。
	// 只验形状不比表，因为快照就是这一刻写的，比秒表会输在跨分钟的运气上。
	if !regexp.MustCompile(`\d{2}-\d{2} \d{2}:\d{2} · 第 2 版`).MatchString(page) {
		t.Errorf("那一格没有时间前缀:\n%s", page)
	}
}

// **每一行要说清它与上面那一版差在哪**（2026-10-15 batch11 U2）。
//
// 实拍（dad 满 50 版）：前六行「31 条书签」一字不差、整页 3.25 屏。那一格答的是
// "这一版有什么"，而人要挑的是"哪一版是我想要回去的"——差的那一维是**与隔壁那版的差**。
// 所以：条数变了就说变了多少，没变就直说没变（没变也是信息，它把"改的是内容不是数量"
// 说出口，而不是让人以为列表坏了）。
func TestRevertPage_每行说出它与上面那一版的差(t *testing.T) {
	s, h := setupBatch(t)
	// 快照是**写之前**拍的，所以"加一条"那次只让**下一版**的快照多一条。
	// 三次写入：改名（快照 = N）→ 加一条（快照 = N）→ 改名（快照 = N+1）。
	// 于是列表从新到旧是 N+1 / N / N：中间那一行比上面少 1 条，
	// 最旧那一行与它上面那行数量与分组都一样。
	for _, body := range []string{
		`{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"改一"}]}`,
		`{"profile":"dad","by":"ai","ops":[{"op":"add","path":"groups/nas/items","value":{"id":"dp-u2","name":"U2 加的一条","url":"http://10.0.0.5:9/"}}]}`,
		`{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"改二"}]}`,
	} {
		if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
			t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
		}
	}
	rev := currentRev(t, s, "dad")
	body := get(h, "/revert", revertDadCookie).Body.String()
	// 列表从新到旧：最新那一行（rev 本身）没有可比对象，所以它什么都不加；
	// 往下第一行是"加那一条之前"，它比上面少 1 条；再往下那次只改了名字。
	if !strings.Contains(body, "比上面那一版少 1 条") {
		t.Errorf("第 2 行没说清它比上面那一版少一条（rev=%d）：\n%s", rev, firstBytes(body))
	}
	if !strings.Contains(body, "与上面那一版的条数与分组都一样") {
		t.Error("只改了名字的那一版没有说「与上面那一版的条数与分组都一样」——那正是它没变的那件事")
	}
	// 增量不许盖掉版本号与条目数：它是**加**在后面的判据，不是替换。
	if !strings.Contains(body, "条书签") {
		t.Error("行里没有了条目数——增量是补充，不是替代")
	}
}
