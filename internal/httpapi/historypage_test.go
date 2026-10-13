package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// 改动历史那一屏（2026-10-13）。
//
// 审计线一直在写，`GET /api/v1/history` 一直读得出来，缺的是屏。这一族要钉的
// 不是"页面出来了"，而是那几条**只有这一页才负责**的事：
//   1. 只认会话、目标不接受申报（管理员登着这一页也只能看自己那一份）；
//   2. 路径里的 id 要换成**认得出的名字**，被删掉的从快照里找回——
//      一句"删了 3 条"而不说删了哪 3 条，等于把这一页最该说的话咽回去；
//   3. 每一种 op/path 形状都说得出一句话，包括这台机器前身留下的怪形状；
//   4. 截断要说出来，并且给的是**没列出的条数**；
//   5. 坏档不挡（事故发生时这一页正是人最该看的那一页）。

const historyDadCookie = "lb_profile=dad"

func historyAt(offset time.Duration) string {
	return time.Now().Add(offset).UTC().Format(time.RFC3339)
}

// getTok 是"GET 一条页面通路同时带一把令牌"——`get()` 不带 Authorization，
// 而这一族最要紧的那一发正是它。
func getTok(h http.Handler, path, cookie, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
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

// adminCookieOf 走一次真进门拿 lb_admin（派生值不是 key 本身，见 adminpage_test）。
func adminCookieOf(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := postForm(h, "/admin/login", "key="+testAdminKey, "")
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieAdmin {
			return auth.CookieAdmin + "=" + c.Value
		}
	}
	t.Fatal("登录后没拿到 lb_admin cookie")
	return ""
}

// 这一族不许写任何东西：303 出去之后磁盘上的审计线一行都不许多。
func TestHistory_令牌与无会话都303且不写盘(t *testing.T) {
	s, h := setupBatch(t)
	before := auditLines(t, s)
	if rec := get(h, "/history", ""); rec.Code != http.StatusSeeOther {
		t.Errorf("无会话该 303, got %d", rec.Code)
	}
	for _, tok := range []string{dadTok, adminTok} {
		rec := getTok(h, "/history", historyDadCookie, tok)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("带令牌（%s）该 303 而不是渲染历史, got %d %s", tok, rec.Code, rec.Body)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("303 该回选择页，得到 %q", loc)
		}
	}
	// **只带管理权、没有档案会话**：这一页没有"哪一份"可答，回选择页。
	if rec := get(h, "/history", adminCookieOf(t, h)); rec.Code != http.StatusSeeOther {
		t.Errorf("只带管理权该 303, got %d", rec.Code)
	}
	if n := auditLines(t, s); n != before {
		t.Errorf("这一族什么都不该写，审计线却从 %d 行涨到 %d 行", before, n)
	}
}

// 一次真写入之后，页面上要能看到那一条的**人话**：渠道、版本、每条短句里
// 那条书签的名字。"改了 2 处"加一串路径不算——那是把 API 换了个壳。
func TestHistory_把人话渲染出来(t *testing.T) {
	_, h := setupBatch(t)
	body := `{"profile":"dad","by":"ai","expectedRev":1,"ops":[
	  {"op":"set","path":"groups/nas/items/dsm/name","value":"群晖 DSM"},
	  {"op":"add","path":"groups/nas/items","value":{"id":"qb","name":"下载","url":"http://10.0.0.3:8080"}}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
	}
	rec := get(h, "/history", historyDadCookie)
	if rec.Code != 200 {
		t.Fatalf("该 200, got %d %s", rec.Code, rec.Body)
	}
	page := rec.Body.String()
	for _, want := range []string{
		"AI 助手",         // 渠道那一格
		"改名 · NAS › 群晖", // 名字是**现在的**名字（这一条刚被改成"群晖 DSM"）
		"新增一条 · 放进 NAS", // add 只到分组那一层：历史里没有 value，新条目的名字无从得知
		"2 处改动",
		"第 2 版",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("页面上没有 %q", want)
		}
	}
	if strings.Contains(page, "⟨") {
		t.Errorf("有键没查到（页面上出现了 ⟨…⟩）：%s", page)
	}
	// 这一页点名到"谁动过这个人的书签"，缓存住就留给下一个共用这台电脑的人。
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("该 no-store，得到 %q", cc)
	}
	if p := rec.Header().Get("X-LB-Profile"); p != "dad" {
		t.Errorf("X-LB-Profile 该是 dad（⑥ 的离线桶靠它分家），得到 %q", p)
	}
}

// 反向那一半：同一个 history.jsonl 是全机器共用的，kid 那一页不许出现 dad 的
// 任何一行。**没有这一条证人，"按档案过滤"这句话是空的**。
func TestHistory_看不到别人的行(t *testing.T) {
	_, h := setupBatch(t)
	body := `{"profile":"dad","by":"ai","expectedRev":1,"ops":[
	  {"op":"set","path":"groups/nas/items/dsm/name","value":"爸爸改的名字"}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
	}
	kid := get(h, "/history", "lb_profile=kid").Body.String()
	for _, leak := range []string{"爸爸改的名字", "AI 助手", "NAS"} {
		if strings.Contains(kid, leak) {
			t.Errorf("kid 那一页漏进了 dad 的东西（%q）：%s", leak, kid)
		}
	}
	if !strings.Contains(kid, "还没有改动记录") {
		t.Errorf("kid 那一页该只说一句「没有」：%s", kid)
	}
}

// 删掉的条目**不在盘上了**，它的名字只存在于那一改动之前的快照里。
// 这一条是这一页最容易被省掉的判断：省掉之后，"谁删了我的东西"回答成
// "删了 groups/nas/items/dsm"——而被删的那一条恰恰最需要被念出名字。
func TestHistory_删掉的那条从快照里找回名字(t *testing.T) {
	_, h := setupBatch(t) // dad 的夹具里那条叫「群晖」
	body := `{"profile":"dad","by":"cli","expectedRev":1,"ops":[
	  {"op":"remove","path":"groups/nas/items/dsm"}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("前置删除失败: %d %s", rec.Code, rec.Body)
	}
	page := get(h, "/history", historyDadCookie).Body.String()
	if !strings.Contains(page, "删掉 · NAS › 群晖") {
		t.Errorf("删掉的那一条该被念出名字（那一改动之前的快照认得它）：%s", page)
	}
	if strings.Contains(page, "删掉 · NAS › dsm") {
		t.Error("还在印 id，说明没去开那一改动之前的快照")
	}
	if !strings.Contains(page, "命令行") {
		t.Errorf("渠道没说成命令行：%s", page)
	}
}

// 形状全覆盖：ops 文法里每一种 `{op, path}` 都要有一句人话，外加账号类那几条
// 不归书签文法管的，和一条**这台机器前身留下的怪形状**。
//
// 怪形状那一发是重点：界面不许对它沉默。历史里真有过的一行被这一页跳过，
// 这一页就成了假账——而它存在的意义正是"一行都没漏"。
func TestHistory_每种形状都有一句话(t *testing.T) {
	s, h := setupBatch(t)
	list := []string{
		`{"op":"add","path":"groups","index":0,"value":{"id":"media","title":"影音","layout":"tiles","items":[]}}`,
		`{"op":"add","path":"groups/media/items","value":{"id":"jf","name":"影视库","url":"http://10.0.0.3:8096"}}`,
		`{"op":"set","path":"groups/media/items/jf/name","value":"影音库"}`,
		`{"op":"set","path":"groups/media/items/jf/url","value":"http://10.0.0.3:8097/"}`,
		`{"op":"set","path":"groups/media/items/jf/icon","value":"simple-icons:jellyfin"}`,
		`{"op":"set","path":"groups/media/items/jf/desc","value":"客厅那台"}`,
		`{"op":"set","path":"groups/media/items/jf/alias","value":["jf"]}`,
		`{"op":"set","path":"groups/media/items/jf/tags","value":["电视"]}`,
		`{"op":"set","path":"groups/media/items/jf/aiNote","value":"按遥控器第二个键"}`,
		`{"op":"set","path":"groups/media/title","value":"影音与电视"}`,
		`{"op":"set","path":"groups/media/layout","value":"list"}`,
		`{"op":"set","path":"groups/media/accent","value":"#66ccff"}`,
		`{"op":"move","path":"groups/media/items/jf","to":"nas"}`,
		`{"op":"remove","path":"groups/nas/items/jf"}`, // 上一步刚把它挪进 nas
		`{"op":"remove","path":"groups/media"}`,
	}
	want := []string{
		"新增了一个分组", "新增一条 · 放进 影音", "改名 · ", "改地址 · ", "换图标 · ",
		"改说明 · ", "改别名 · ", "改标签 · ", "改 AI 备注 · ",
		"改分组名 · ", "改这一组的排法 · ", "改这一组的颜色 · ",
		"挪动 · ", "删掉 · ", "删掉整个分组 · ",
	}
	rev := int64(1)
	for i, o := range list {
		body := fmt.Sprintf(`{"profile":"dad","by":"web","expectedRev":%d,"ops":[%s]}`, rev, o)
		rec := post(h, "/api/v1/batch", body, historyDadCookie, "")
		if rec.Code != 200 {
			t.Fatalf("第 %d 发该 200, got %d %s（op=%s）", i+2, rec.Code, rec.Body, o)
		}
		rev++
	}
	// 账号类三发，顺序有讲究：**回退要赶在停用之前**——一份被停用的档案
	// 在界面上已经进不去，那时再发 POST /revert 拿到的是"停用"那一族的 303，
	// 就不是这一发要验的结论了。
	if rec := postForm(h, "/revert", "to-rev=1", historyDadCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("回退该 303, got %d %s", rec.Code, rec.Body)
	}
	if rec := post(h, "/api/v1/profiles/pin", `{"profile":"dad","new":"4321"}`, "", adminTok); rec.Code != 200 {
		t.Fatalf("设 PIN 该 200, got %d %s", rec.Code, rec.Body)
	}
	if rec := post(h, "/api/v1/profiles/disable", `{"profile":"dad","disabled":true}`, "", adminTok); rec.Code != 200 {
		t.Fatalf("停用该 200, got %d %s", rec.Code, rec.Body)
	}
	// 前身留下的怪形状：路径不在文法里，直接落进审计线，绕开写侧校验。
	if err := s.AppendHistory(store.HistoryEntry{
		At: historyAt(0), Profile: "dad", Actor: "dad", By: "sync-template", Rev: 0,
		Ops: []store.HistoryOp{{Op: "set", Path: "somewhere/else"}},
	}); err != nil {
		t.Fatal(err)
	}

	page := get(h, "/history", historyDadCookie).Body.String()
	// 管理员那一族的两句话会打架：机器那一面不带 X-LB-By 时 `by` 缺省记成 `web`，
	// 于是并排读出来是「网页上 · 由管理员经手」——自相矛盾的一句话。
	// 判据：**经手人是管理员时他说"谁"，渠道那一格让位**（见 historyRow）。
	if strings.Contains(page, "网页上 · 由管理员经手") {
		t.Errorf("管理员那一行同时说了两个经手方：%s", head(page))
	}
	if !strings.Contains(page, " · 由管理员经手") {
		t.Errorf("管理员那一行没说清是谁动的：%s", head(page))
	}
	var missing []string
	for _, w := range append(want,
		"改了这道门的 PIN", "由管理员经手", "停用或启用了这一份", "不动书签",
		"回退到第 1 版", "动了 somewhere/else", "sync-template") {
		// 认不出的渠道**原样印**，不说"未知"：历史里真有过的一行不许被这一页否认。
		if !strings.Contains(page, w) {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		t.Errorf("有 %d 种形状在这一页上没被说出来：\n  %s\n页面：%s",
			len(missing), strings.Join(missing, "\n  "), page)
	}
}

// 一次改 30 条是常见形状（AI 与导入都这么写）。一行列 6 条短句，剩下的
// **必须说出没列出的条数**——只列 6 条不说，"就这些"与"我只给你看 6 条"
// 长得一模一样（⌘K 面板那一处已经被这一枪打过一次）。
func TestHistory_截断要说出没列出的条数(t *testing.T) {
	_, h := setupBatch(t)
	var many []string
	for i := 0; i < 30; i++ {
		many = append(many, fmt.Sprintf(
			`{"op":"set","path":"groups/nas/items/dsm/name","value":"第 %d 次改的名字"}`, i))
	}
	body := fmt.Sprintf(`{"profile":"dad","by":"ai","expectedRev":1,"ops":[%s]}`, strings.Join(many, ","))
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("批量写入失败: %d %s", rec.Code, rec.Body)
	}
	page := get(h, "/history", historyDadCookie).Body.String()
	if !strings.Contains(page, "30 处改动") {
		t.Errorf("结论那一格该说 30 处：%s", page)
	}
	if !strings.Contains(page, "另外还有 24 处没列出来") {
		t.Errorf("截断没说清（要的是**没列出**的 24，不是总数 30）：%s", page)
	}
	if strings.Contains(page, "另外还有 30") {
		t.Error("把总数当成了没列出的数")
	}
}

// 一页 100 行封顶，并且**报出这一份一共记了多少次**。
// 那句"一共"说的是这一份、不是这台机器：审计线是全机器共用的一个文件，
// 说成机器会被读成"别人家的改动也混在这一页里"。
func TestHistory_一页最多一百次(t *testing.T) {
	s, h := setupBatch(t)
	const total = 130
	for i := 0; i < total; i++ {
		if err := s.AppendHistory(store.HistoryEntry{
			At: historyAt(-time.Duration(i) * time.Hour), Profile: "dad",
			Actor: "dad", By: "web", Rev: int64(i + 2),
			Ops: []store.HistoryOp{{Op: "set", Path: "groups/nas/items/dsm/name"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	page := get(h, "/history", historyDadCookie).Body.String()
	if !strings.Contains(page, fmt.Sprintf("只列最近 100 次 · 这一份一共记了 %d 次", total)) {
		t.Errorf("截断结论没说出来：%s", page)
	}
	if n := strings.Count(page, "一处改动"); n != 100 {
		t.Errorf("说了只列 100 次，页面上那一格却有 %d 行（应当正好 100）", n)
	}
	if other := get(h, "/history", "lb_profile=kid").Body.String(); strings.Contains(pageText(other), fmt.Sprint(total)) {
		t.Errorf("dad 的次数漏进了 kid 那一页：%s", pageText(other))
	}
}

// pageText 剥掉 `<style>` 与 `<script>` 两块，只留人眼看得见的字。
// 这一格原先直接对整份 HTML 找数字，而首屏骨架是**内联**进每一页的
// （§7.2）——CSS 里出现 `column-width:130px` 的那一天，"130 漏进了别人那一页"
// 就成了假话。判据说的是"页面上说出来的数"，那就得先看页面上有的字。
func pageText(html string) string {
	out := html
	for _, tag := range [][2]string{{"<style>", "</style>"}, {"<script>", "</script>"}} {
		for {
			i := strings.Index(out, tag[0])
			if i < 0 {
				break
			}
			j := strings.Index(out[i:], tag[1])
			if j < 0 {
				out = out[:i]
				break
			}
			out = out[:i] + out[i+j+len(tag[1]):]
		}
	}
	return out
}

// 坏档不挡：这一页一个字都不必读 bookmarks.json，而事故发生时它正是
// 人最该看的那一页（"最后一次是谁动的"）。名字借不到就印 id——那是
// 屏幕上本来就看得见的东西，比编一个名字诚实。
func TestHistory_坏档不挡(t *testing.T) {
	s, h := setupBatch(t)
	body := `{"profile":"dad","by":"extension","expectedRev":1,"ops":[
	  {"op":"remove","path":"groups/nas/items/dsm"}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
	}
	broken := filepath.Join(s.Root(), "profiles", "dad", "bookmarks.json")
	if err := os.WriteFile(broken, []byte("{ 坏掉的字节"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/history", historyDadCookie)
	if rec.Code != 200 {
		t.Fatalf("坏档不该挡这一页, got %d %s", rec.Code, rec.Body)
	}
	page := rec.Body.String()
	if !strings.Contains(page, "浏览器扩展") || !strings.Contains(page, "删掉 · ") {
		t.Errorf("坏档那一支页面上没把这一行说出来：%s", page)
	}
	// 现在这一版读不出来，但那一改动之前的快照还认得它——快照不在这次损坏里。
	if !strings.Contains(page, "群晖") {
		t.Errorf("删掉的那一条该由快照报出名字：%s", page)
	}
}

// 台面面板里那一行与命令面板那一格都指向 /history：入口不在页面上，
// 这一屏就等于不存在（"人点不到的东西不算承诺"那条纪律）。
//
// 2026-10-13：这一枚从**顶栏**搬进了台面面板（`GET /prefs`，档案页里那颗
// 「台面」弹出的浮层渲染的是同一份标记）。判据没变——仍然是"从档案页出发，
// 两步之内能点到 /history"——但"画得出来"要在**面板那份 HTML** 里查，
// 而"档案页上有没有路通向面板"要在档案页那份 HTML 里查。两条都得钉：
// 只钉后者，那一天面板渲染坏了会一路绿灯。
func TestHistory_入口在台面面板上画得出来(t *testing.T) {
	_, h := setupBatch(t)
	// 第一步：档案页上有一颗带可见文字的入口。
	//
	// 2026-10-13 那一屏把这颗键的话从「台面」换成了「外观」——"台面"是这个项目
	// 内部对那一组台面偏好的称呼，新用户第一次看见它并不知道那能改什么，
	// 而判据是"没读过文档的人一眼懂不懂"。钉的字跟着换成屏上真的印着的那两个字，
	// id 仍是 lb-prefs-open（id 是内部形状，不跟着界面词漂）。
	home := get(h, "/", historyDadCookie).Body.String()
	if !strings.Contains(home, `id="lb-prefs-open"`) || !strings.Contains(home, "外观") {
		t.Errorf("档案页顶栏少了进台面面板的那一枚：%s", firstBytes(home))
	}
	// 第二步：面板里画着通往 /history 的那一枚。
	page := get(h, "/prefs", historyDadCookie).Body.String()
	if !strings.Contains(page, `href="/history"`) {
		t.Errorf("台面面板少了进改动历史的那一枚：%s", firstBytes(page))
	}
	if !strings.Contains(page, "改动历史") {
		t.Error("那一枚没有可见文字（读屏与语音输入的人都点不到它）")
	}
	// 零改动的新档案也画：与回退那枚不同，"还没有人动过"本身就是这一页要答的，
	// 不是一条死路。
	_, h2 := setup(t)
	if rec := post(h2, "/api/v1/profiles", `{"id":"open","title":"公屏","nopass":true}`, "", testAdminKey); rec.Code != 201 {
		t.Fatalf("建档该 201, got %d %s", rec.Code, rec.Body)
	}
	if rec := get(h2, "/prefs", "lb_profile=open"); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `href="/history"`) {
		t.Errorf("零改动的档案的面板少了这一枚（那一支页面答的是「还没有」，不是死路）：%d %s",
			rec.Code, firstBytes(rec.Body.String()))
	}
	if hp := get(h2, "/history", "lb_profile=open"); hp.Code != 200 ||
		!strings.Contains(hp.Body.String(), "还没有改动记录") {
		t.Errorf("零改动那一支该 200 并说清为什么是空的：%d %s",
			hp.Code, firstBytes(hp.Body.String()))
	}
}

// 档案被删掉之后会话还指着它 → 303 回选择页，而不是画一张空历史。
func TestHistory_目标没了回选择页(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.DeleteProfile("dad"); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/history", historyDadCookie)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("该 303, got %d %s", rec.Code, rec.Body)
	}
}

// 标题里要带这一份的名字：共用一台电脑的人先确认"这是谁的"再看内容。
// 坏档时 profile.json 读不出来，那一句不许编一个名字，落 id。
func TestHistory_标题说的是这一份(t *testing.T) {
	s, h := setupBatch(t)
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(p.Title) == "" {
		t.Fatal("夹具前提：dad 有档案名")
	}
	head := get(h, "/history", historyDadCookie).Body.String()
	if !strings.Contains(head, "<title>"+p.Title+"的改动历史 · launchbay</title>") {
		t.Errorf("<title> 没带上这一份的名字：%s", firstBytes(head))
	}
	// 坏档那一支：profile.json 读不动 → 标题落 id，不编名字。
	brokenProfile := filepath.Join(s.Root(), "profiles", "dad", "profile.json")
	if err := os.WriteFile(brokenProfile, []byte("{ 坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	page := get(h, "/history", historyDadCookie).Body.String()
	if !strings.Contains(page, "<title>dad的改动历史 · launchbay</title>") {
		t.Errorf("坏档时标题该落 id 而不是编一个名字：%s", firstBytes(page))
	}
}

func head(s string) string {
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

func firstBytes(s string) string {
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

// auditLines 数审计线里有几行（判"这一族有没有写东西"用）。
// 按行数而不是按档案数：这一条要抓的是"一个字都没多写"。
func auditLines(t *testing.T, s *store.Store) int {
	t.Helper()
	raw, err := os.ReadFile(s.HistoryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
