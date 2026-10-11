package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/render"
)

const kidFormCookie = "lb_profile=kid"

func TestAdmin_管理员拿到页面且不含条目内容(t *testing.T) {
	s, h := setupBatch(t) // dad=爸爸(admin)、kid=小满；书签里有"群晖/动画"与 10. 段地址
	if err := s.SaveBookmarks("kid", &model.Bookmarks{Rev: 1, Title: "小满的", Groups: []model.Group{{
		ID: "cartoon", Title: "动画片", Layout: model.LayoutList,
		Items: []model.Item{{ID: "ani", Name: "动画站", URL: "http://10.0.0.9:8888", Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/admin", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员会话该拿 200，得到 %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	// 名单与徽标仍然都在；**`action="/switch"` 不在其中**——2026-10-10 起
	// 管理员进不去别人的台，管理页那一格只剩账号层面的动作（设 PIN/停用/删档）。
	for _, want := range []string{"爸爸", "小满"} {
		if !strings.Contains(body, want) {
			t.Errorf("管理页缺 %q:\n%.400s", want, body)
		}
	}
	// 一行一个档案：条目内容一个都不许出现（spec §8.8）。这份名单与
	// /api/v1/profiles 那条用的是同一口径，两处都钉。
	for _, banned := range []string{"动画站", "动画片", "http://10.0.0.9:8888", "群晖", `"url"`} {
		if strings.Contains(body, banned) {
			t.Errorf("管理页漏出了条目内容 %q——它只该报数量", banned)
		}
	}
	// 跨档案页面必须 no-store：缓存住 = 把全家庭的名字与条目数留给下一个
	// 共用这台电脑的人（§7.1 的串号）。
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q，管理页要 no-store", cc)
	}
}

func TestAdmin_非管理员与无会话都303回选择页(t *testing.T) {
	_, h := setupBatch(t)
	for name, ck := range map[string]string{"非管理员会话": kidFormCookie, "没有会话": ""} {
		rec := get(h, "/admin", ck)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s：得到 %d，要 303（浏览器通道的拒不吐 403 页面）", name, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("%s：Location = %q，要 /", name, loc)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("%s：拒的一路不该带页面体: %s", name, rec.Body)
		}
	}
}

// 坏档案那一行：占一行、标出来、**不给「进入编辑」**——/switch 对读不了的
// 档案是不切的（Task 4），渲染一颗点了没反应的按钮进界面，违反本期的规则。
func TestAdmin_坏档案占一行且没有死按钮(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.BookmarksPath("kid"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/admin", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "kid") {
		t.Errorf("坏档案整行消失了（spec §12：消失比报 0 更坏）:\n%s", body)
	}
	if !strings.Contains(body, "读不了") {
		t.Errorf("坏档案那行要说清读不了:\n%.400s", body)
	}
	if strings.Contains(body, `value="kid"`) {
		t.Error("坏档案不该有「进入编辑」——切过去就是一页读不了的台")
	}
}

// profile.json 坏到列不出来（ListProfiles 直接 continue 掉它）的档案，管理页
// 这一行只存在于聚合的 Broken() 并集里。这条是"聚合只有一份"的页面侧证人：
// 并集若只在 JSON 那一侧修，管理页会整行消失——正是 §12 点名的形状。
// 与 TestAdmin_坏档案占一行且没有死按钮（bookmarks.json 坏，档案仍在列表里、
// 带着 title）是两种坏法，各钉一头。
func TestAdmin_坏profile档案也占一行(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.ProfilePath("kid"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/admin", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	// title 读不出来，这一行只剩 id：模板在 title 为空时回退渲染 id。
	if !strings.Contains(body, "kid") {
		t.Errorf("profile.json 坏的档案没在管理页占一行（spec §12）:\n%s", body)
	}
	if !strings.Contains(body, "读不了") {
		t.Errorf("坏档案那行要说清读不了:\n%.400s", body)
	}
	if strings.Contains(body, `value="kid"`) {
		t.Error("坏档案不该有「进入编辑」")
	}
}

// 聚合只有一份：管理页报的 items/rev 必须与 /api/v1/status 逐字一致。
// 两处各写一遍遍历的话，坏档并集这类细节迟早只在一处修。
func TestAdmin_与status同一份聚合(t *testing.T) {
	s, h := setupBatch(t)
	// 夹具必须**分得出**这两格：dad/kid 原本都是 rev=1、items=1，把「条目」和
	// 「rev」两格对调，逐格比对也照样绿（实测过：列对调之后这条闸原样通过）。
	// 所以这里先给 kid 加到 2 条，让 items(2) != rev(1)，两格才有各自的值。
	if err := s.SaveBookmarks("kid", &model.Bookmarks{Rev: 1, Title: "小满的", Groups: []model.Group{{
		ID: "cartoon", Title: "动画片", Layout: model.LayoutList,
		Items: []model.Item{
			{ID: "ani", Name: "动画站", URL: "http://10.0.0.9:8888", Alias: []string{}, Tags: []string{}},
			{ID: "ani2", Name: "动画站二", URL: "http://10.0.0.9:8889", Alias: []string{}, Tags: []string{}},
		},
	}}}); err != nil {
		t.Fatal(err)
	}
	status := getBearer(h, "/api/v1/status", adminTok)
	var out struct {
		Profiles []struct {
			ID       string `json:"id"`
			Items    int    `json:"items"`
			Rev      int64  `json:"rev"`
			Modified string `json:"modified"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Profiles) < 2 {
		t.Fatalf("夹具只给出 %d 个档案，这一条要至少两个才分得出行", len(out.Profiles))
	}
	// 分辨力的前提要钉住：上面那段"给 kid 加到 2 条"哪天被改回 items==rev，
	// 列对调就再次隐形，而逐格比对会悄无声息地退回看不见。
	distinguishable := false
	for _, p := range out.Profiles {
		if p.Items != int(p.Rev) {
			distinguishable = true
		}
	}
	if !distinguishable {
		t.Fatal("每个档案的 items 都等于 rev：这一条分不出「条目」格和「rev」格")
	}
	page := get(h, "/admin", dadFormCookie).Body.String()
	for _, p := range out.Profiles {
		if p.Modified == "" {
			t.Errorf("%s 的 status 行没有 modified：页面就没法报最近改动时间", p.ID)
		}
		// 逐格比，不是整页 Contains：整页 Contains 只要数字在页面任何角落出现过
		// 就算过，"条目"与"rev"两格对调、"最近改动"整格不渲染，它都看不见（评审 I-1）。
		cells := adminRowCells(t, page, p.ID)
		for i, want := range []struct {
			col  string
			want string
		}{{"条目", strconv.Itoa(p.Items)}, {"rev", strconv.FormatInt(p.Rev, 10)}, {"最近改动", p.Modified}} {
			got := cells[1+i]
			if want.col == "最近改动" {
				// 这一格现在印的是本地「月-日 时:分」，不是 status 的 RFC3339 原文。
				// 断言的是**同一个时刻**而不是同一个字符串：拿同一份格式化函数去比
				// 同一份格式化函数是套套逻辑，量不出错；这里自己解析再对一次表。
				if !sameMinuteLocal(got, want.want) {
					t.Errorf("%s 的「最近改动」格 = %q，与 status 报的 %q 不是同一个时刻:\n%s", p.ID, got, want.want, page)
				}
				continue
			}
			if got != want.want {
				t.Errorf("%s 的「%s」格 = %q，status 报 %q:\n%s", p.ID, want.col, got, want.want, page)
			}
		}
	}
}

// 管理页不属于任何档案，它在 X-LB-Profile 与 ETag 前缀里用的名字必须是
// `model.ValidSlug` **拒收**的串：`"admin"` 本身是个能建出来的档案 id，谁真建了
// 它，⑥ 的 SW 就会把这张跨档案的名单页当成那个档案的页面存进 `html-admin` 桶
// （本期这几页 no-store，今天不可达，但形状要先定，见 render.AdminSentinel）。
func TestAdmin_档案头不是任何可建的档案id(t *testing.T) {
	_, h := setupBatch(t)
	rec := get(h, "/admin", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员该拿 200，得到 %d", rec.Code)
	}
	got := rec.Header().Get("X-LB-Profile")
	if model.ValidSlug(got) {
		t.Errorf("X-LB-Profile = %q，它同时是一个合法的档案 id：SW 会把跨档案名单页存进那个档案的桶", got)
	}
	if got != render.AdminSentinel {
		t.Errorf("X-LB-Profile = %q，要 render.AdminSentinel（%q）", got, render.AdminSentinel)
	}
	// ETag 前缀与它同源：NoStore 这一支本来不发 ETag，所以这里核的是渲染器
	// 自己算的那一份（handler 不发，值仍要与头一致，否则下一个人会以为前缀随便写）。
	if et := rec.Header().Get("ETag"); et != "" {
		t.Errorf("管理页是 NoStore，不该发 ETag，得到 %q", et)
	}
}

// adminRowCells 取出管理页里属于某个档案那一行的各格文本（标签剥掉、空白压平），
// 按列序返回：档案、条目、rev、最近改动、快照、动作。
// 定位靠 id：正常行把它渲染在 <code> 里，profile.json 坏到读不出 title 的行
// 只剩 <b>id</b>（模板的回退）。两种都要认，否则这条 helper 会在坏档那行
// 直接判不出行来。
// sameMinuteLocal 判"页面上那个月-日 时:分"与 status 的 RFC3339 UTC 原文是不是
// 同一个时刻（按本地时区）。分钟级：这一格的用途是"今天上午改的"，不是秒表。
func sameMinuteLocal(cell, rfc3339 string) bool {
	want, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return false
	}
	got, err := time.ParseInLocation("01-02 15:04", cell, time.Local)
	if err != nil {
		return false
	}
	// 两边都只取「月-日 时:分」这一段比：页面上那一格没有年份，而
	// ParseInLocation 给 got 补的是 0000 年——连年份一起比永远不等。
	return got.Format("01-02 15:04") == want.Local().Format("01-02 15:04")
}

func adminRowCells(t *testing.T, page, id string) []string {
	t.Helper()
	for _, chunk := range strings.Split(page, "</tr>") {
		i := strings.LastIndex(chunk, "<tr")
		if i < 0 {
			continue
		}
		row := chunk[i:]
		if !strings.Contains(row, "<code>"+id+"</code>") && !strings.Contains(row, "<b>"+id+"</b>") {
			continue
		}
		var cells []string
		for _, c := range strings.Split(row, "</td>") {
			j := strings.Index(c, "<td")
			if j < 0 {
				continue
			}
			cells = append(cells, flattenTags(c[j:]))
		}
		// 列数是这张表的结构：少一格意味着渲染侧删了列或 <td> 漂了，
		// 上面的逐格比对会全部错位——先在这里拦住。
		if len(cells) != 6 {
			t.Fatalf("%s 那一行有 %d 格，要 6 格（档案/条目/rev/最近改动/快照/动作）:\n%s", id, len(cells), row)
		}
		return cells
	}
	t.Fatalf("管理页里没有 %s 那一行:\n%s", id, page)
	return nil
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// flattenTags 去掉标签并把空白压成单个空格：格子里裹着 <b>/<code>/<span> 与
// 换行缩进，逐字比之前先把它们归一。
func flattenTags(s string) string {
	return strings.Join(strings.Fields(tagRE.ReplaceAllString(s, " ")), " ")
}

// 管理页的建档表单（设计 §4）。行为侧证人：判据全走 createProfile，这里只验外壳接线。
func TestAdmin_表单建出普通档案(t *testing.T) {
	s, h := setupBatch(t) // dad=admin 会话可用，kid 是普通档案
	before := dirCount(t, s)
	rec := postForm(h, "/admin", "title=%E5%A6%88%E5%A6%88&pin=5678", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("建档成功要 303，得到 %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin" {
		t.Errorf("成功该回管理页，Location = %q", loc)
	}
	if got := dirCount(t, s); got != before+1 {
		t.Fatalf("磁盘上该多出一个档案：%d → %d", before, got)
	}
	// 中文名字 → 基名序列里第一个空位（Task 1 的派生规则在真实通路上生效）。
	p, err := s.LoadProfile("home")
	if err != nil {
		t.Fatalf("新档案没落在预期位置: %v", err)
	}
	if p.Admin {
		t.Error("不勾「能管理全家」就建成管理员了——勾和没勾得是两个答案")
	}
	if !strings.HasPrefix(p.PIN, "sha256$") {
		t.Errorf("PIN 要经 HashPIN 存成三段式，得到 %q", p.PIN)
	}
}

// 勾选的那一格是模板与 handler 之间的**接件**：模板写 value="1"，handler 比 == "1"。
// 测试从模板里取那个值再原样发出去，两个方向都被钉住：模板改值（浏览器就会发别的值）
// 或 handler 改比较，这一条都会红——写死 "admin=1" 的测试只对一半漂移敏感（复扫实测）。
func TestAdmin_勾选才建管理员(t *testing.T) {
	s, h := setupBatch(t)
	v := adminCheckboxValue(t)
	body := "title=nas&pin=5678&admin=" + url.QueryEscape(v)
	if rec := postForm(h, "/admin", body, dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body.String())
	}
	p, err := s.LoadProfile("nas")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Admin {
		t.Errorf("模板勾上发的值是 %q，handler 却没建成管理员——接件断了一边", v)
	}
	// 反向：不发那个值就不是管理员（"勾与不勾是两个答案"的另一半）。
	if rec := postForm(h, "/admin", "title=noswitch&pin=5678", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("不勾的提交要 303，得到 %d", rec.Code)
	}
	if p2, err := s.LoadProfile("noswitch"); err != nil {
		t.Fatal(err)
	} else if p2.Admin {
		t.Error("不勾也建成了管理员")
	}
}

// adminCheckboxValue 从 admin.tmpl 里取建档勾选的 value（模板与 handler 的接件值）。
func adminCheckboxValue(t *testing.T) string {
	t.Helper()
	raw := readGoFile(t, "../render/web/admin.tmpl")
	m := checkboxValueRE.FindStringSubmatch(raw)
	if m == nil {
		t.Fatal("admin.tmpl 里找不到建档勾选的 value——接件的形式变了，这两半得同时改")
	}
	return m[1]
}

var checkboxValueRE = regexp.MustCompile(`name="admin"\s+value="([^"]+)"`)

// 浏览器通路不认令牌（/prefs、/switch 同族纪律）：机器建档有 API 那条路。
func TestAdmin_表单通路不认令牌(t *testing.T) {
	s, h := setupBatch(t)
	before := dirCount(t, s)
	// 逐条列出 openapi_test.go 的 tokensRefusedByForm：那张表每一行的合法性
	// 都来自"这里收到 Bearer 真的 303 且不写盘"，所以这张表与这条测试必须一起长。
	// 2026-10-10 加进 /admin/disable 的两条（停用/启用那两步）。
	rec := post(h, "/admin", `{"title":"x","pin":"1234"}`, "", adminTok)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("令牌打 POST /admin 要 303，得到 %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/" {
		t.Errorf("要回选择页，Location = %q", rec.Header().Get("Location"))
	}
	for _, path := range []string{"/admin/pin", "/admin/delete", "/admin/disable"} {
		if rec := postForm(h, path, "profile=kid", ""); rec.Code != http.StatusSeeOther {
			t.Errorf("令牌打 POST %s 要 303，得到 %d %s", path, rec.Code, rec.Body)
		}
		if rec := getBearer(h, path+"?profile=kid", adminTok); rec.Code != http.StatusSeeOther {
			t.Errorf("令牌打 GET %s 要 303，得到 %d %s", path, rec.Code, rec.Body)
		}
	}
	if got := dirCount(t, s); got != before {
		t.Errorf("令牌那一路在磁盘上留下了东西：%d → %d", before, got)
	}
}

func TestAdmin_非管理员与无会话都不写盘(t *testing.T) {
	s, h := setupBatch(t)
	before := dirCount(t, s)
	for _, ck := range []string{"", kidFormCookie} {
		rec := postForm(h, "/admin", "title=x&pin=1234", ck)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
			t.Errorf("cookie=%q：要 303 回 /，得到 %d Location=%q",
				ck, rec.Code, rec.Header().Get("Location"))
		}
	}
	if got := dirCount(t, s); got != before {
		t.Errorf("两种拒都在磁盘上留下了东西：%d → %d（判据是磁盘，不是状态码）", before, got)
	}
}

// 字段错与形状错是两种长相（spec §6.1）：没填对 → 303 带键；读不出形状 → 422 JSON。
func TestAdmin_字段错回带键的303(t *testing.T) {
	_, h := setupBatch(t)
	for body, want := range map[string]string{
		"title=&pin=1234": "noname",
		"title=dad&pin=1": "badpin",
		"title=&pin=":     "noname", // 两格都没填对：先说名字那一格
		"title=%E7%88%B8": "badpin", // 没给 pin
	} {
		rec := postForm(h, "/admin", body, dadFormCookie)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%q 要 303，得到 %d", body, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin?err="+want {
			t.Errorf("%q 的 Location = %q，要 /admin?err=%s", body, loc, want)
		}
	}
}

func TestAdmin_形状错走JSON外壳(t *testing.T) {
	s, h := setupBatch(t)
	before := dirCount(t, s)
	for _, tc := range []struct{ form, field string }{
		{"title=x&pin=" + strings.Repeat("9", maxBodyBytes+4096), "body"},
		{"title=a&title=b&pin=1234", "title"},
		{"title=%E7%88%B8&pin=1234&pin=5678", "pin"},
	} {
		rec := postForm(h, "/admin", tc.form, dadFormCookie)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%.40q 要 422 JSON 外壳，得到 %d", tc.form, rec.Code)
		}
		f, reason, _ := onlyFieldError(t, rec.Body.Bytes(), tc.field)
		if f != tc.field {
			t.Errorf("%.40q 要恰好点名 %q，得到 %q", tc.form, tc.field, f)
		}
		if reason == "" {
			t.Errorf("%.40q 的字段错要带理由", tc.form)
		}
	}
	// 正对照：合格提交走 303，不是 422。
	if rec := postForm(h, "/admin", "title=%E5%A6%88&pin=5678", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Errorf("合格提交要 303，得到 %d", rec.Code)
	}
	if got := dirCount(t, s); got != before+1 {
		t.Errorf("三条形状错都不许落盘，只有那条合格提交该落：before=%d now=%d", before, got)
	}
}

// notice 必须真的印出来：管理员看不见"刚才那条没建上"就等于表单静默失败。
// 主证在**页面**这一头（逐个键 GET /admin?err=… 看渲染出来的句子）；循环里那句
// "map 里这个键不许为空"只是让 `Contains` 有得可比的先决条件——少了它，键被删掉时
// 断言就变成和空串比，恒真。
func TestAdmin_提示在场(t *testing.T) {
	_, h := setupBatch(t)
	for _, key := range []string{"badpin", "noname", "nametaken", "permission"} {
		// 这句先要有：`strings.Contains(body, "")` 永远为真，白名单里的键被删掉时
		// 页面只会"什么都不印"，光看页面看不出来——实测删掉 nametaken 那一行，
		// 只剩页面断言的话全绿（Contains 打空串就是这个形状）。
		if noticeOf(i18n.Default, key) == "" {
			t.Errorf("pinNotices 缺键 %q：管理页会说不出那句失败", key)
			continue
		}
		rec := get(h, "/admin?err="+key, dadFormCookie)
		if !strings.Contains(rec.Body.String(), noticeOf(i18n.Default, key)) {
			t.Errorf("管理页说不出登记的失败 %q:\n%.400s", key, rec.Body.String())
		}
	}
	// 未登记的键一个字都不进页面（与进门页同一套白名单纪律）。
	rec2 := get(h, "/admin?err=%E4%B8%8D%E6%98%AF%E9%94%AE", dadFormCookie)
	if strings.Contains(rec2.Body.String(), "不是键") {
		t.Error("未登记的 ?err= 被复读进管理页了（反射点）")
	}
	if strings.Contains(rec2.Body.String(), `class="notice"`) {
		t.Error("没有可说的提示时不许画空的提示元素")
	}
}

// 撞名那一种失败要说人话，不许吐 500。派生 id 只算一次（在锁外），所以两个同名并发
// 提交会算出同一个空位——后到的那个由 store 判成 ErrExists（409、不带 Err）。少了
// 壳里那支 StatusConflict 分支，409 就掉进 writeInternalError(res.Err)，吐一个
// "内部错误: <nil>" 的 500（评审 I-1 的实测形状）。
//
// 这条断两件事：任何一次提交都不许 500，以及这一轮**确实**撞上过（撞不上就是
// 全串行的调度，分支没被执行，本条在空转——由 taken 那个正对照说出口）。
// 要确认它带电，看 task-5-report.md 的 X1 破坏：删掉壳里那支，它当场红。
func TestAdmin_并发同名不吐500(t *testing.T) {
	s, h := setupBatch(t)
	const n = 40
	var wg sync.WaitGroup
	codes := make([]int, n)
	locs := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := postForm(h, "/admin", "title=%E5%90%8C%E5%90%8D&pin=5678", dadFormCookie)
			codes[i] = rec.Code
			locs[i] = rec.Header().Get("Location")
		}(i)
	}
	wg.Wait()
	taken := 0
	for i := range codes {
		if codes[i] != http.StatusSeeOther {
			t.Fatalf("第 %d 次提交得到 %d（要 303；409 落成 500 就是这一条要抓的）", i, codes[i])
		}
		switch locs[i] {
		case "/admin":
		case "/admin?err=nametaken":
			taken++
		default:
			t.Errorf("第 %d 次的 Location = %q，只能是成功或 nametaken 两种", i, locs[i])
		}
	}
	// 读数打出来给 `-v` 复核用：没有这一行，"撞名分支确实被执行过"就只剩一句
	// 无法复验的转述（复扫点的正是这个）。
	t.Logf("taken=%d/%d", taken, n)
	// 正对照：这条证人不能靠"一次都没撞"通过。派生 id 在锁外算，40 个并发提交里
	// 一定有两笔算到同一个空位（taken 的具体数字随调度漂，本机两跑见过 26 与 38，
	// 所以这里不写死——写的是"不许为 0"这件事，数字自己看 Logf）。
	// taken == 0 意味着这一轮 40 笔全串行跑完——StatusConflict 那一支没被执行，
	// 本条在空转，就该由这里说出口。删掉壳里那支红的是上面那句"得到 500"（X1 实测），
	// 不是这条正对照：两条分工不同，正对照防的是"分支还在但一次都没走"。
	// "页面能把这句话画出来"不在这里验：那由 TestAdmin_提示在场 从页面那头钉，
	// 它不靠调度，所以这条不必为了证明键说得出口而去撞名。
	if taken == 0 {
		t.Error("40 次并发提交一次都没撞名：这一轮没走到 StatusConflict 分支，本条在空转")
	}
	if got := dirCount(t, s); got != 2+n-taken {
		t.Errorf("磁盘上档案数 %d，要 %d（起点 2 + 建成数；撞掉的那 %d 次不许留目录）",
			got, 2+n-taken, taken)
	}
}

// 死路②（P1-④.5 验收结转的那处）：零档案时 `/admin` 注册着但永远 303。这不是故障，
// 是单门纪律——建档只有一道门（/setup 表单或 POST /api/v1/profiles），而 /admin 的前提
// 是管理员会话，零档案时不可能有。此前它只有验收当天一次手测，这里钉成证人。
func TestAdmin_零档案时这一面不可达(t *testing.T) {
	s, h := setupZero(t)
	// 无会话的 GET：裸 303 回选择页。Location 精确等于 "/"——带 ?err= 就是假话，
	// 权限类拒绝不解释（§6.1）。
	rec := get(h, "/admin", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("零档案时 GET /admin 要 303，得到 %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("GET /admin 的出路要恰好 \"/\"（不带键），得到 %q", loc)
	}
	// 带 `*` 令牌的 POST：浏览器通路不认令牌，同一条裸 303，磁盘一个档案都不多。
	rec = post(h, "/admin", `{"title":"x","pin":"1234"}`, "", adminTok)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("带令牌的 POST /admin 要 303 回 \"/\"，得到 %d %q",
			rec.Code, rec.Header().Get("Location"))
	}
	if n := dirCount(t, s); n != 0 {
		t.Errorf("磁盘上多了 %d 个档案目录，要 0——这条通路在零档案时什么都做不成", n)
	}
	// 正对照 ①：同一把令牌打 API 那条是活的。拒它的是"这是浏览器表单通路"，不是鉴权
	// 失败——少了这条，上面两断言会把"令牌本身不可用"也一起钉成应有行为。
	rec = getBearer(h, "/api/v1/profiles", adminTok)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"profiles": []`) {
		t.Errorf("管理员令牌打 GET /api/v1/profiles 要 200 空表，得到 %d %.200s", rec.Code, rec.Body)
	}
	// 正对照 ②：建成第一个档案之后这一面真的可达——否则"永远 303"连它自己一起钉死了。
	if rec := postForm(h, "/setup", "title=%E7%88%B8&pin=1234", ""); rec.Code != http.StatusSeeOther {
		t.Fatalf("引导建档该 303，得到 %d", rec.Code)
	}
	cookie := sessionCookie(t, postForm(h, "/pin", "profile=home&pin=1234", ""))
	rec = postForm(h, "/admin", "title=%E5%A6%88&pin=5678", cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin" {
		t.Fatalf("有管理员会话之后 POST /admin 要 303 回 /admin，得到 %d %q",
			rec.Code, rec.Header().Get("Location"))
	}
	if n := dirCount(t, s); n != 2 {
		t.Errorf("管理页建档之后磁盘上要有 2 个档案，得到 %d", n)
	}
}

// 坏档那一格从前写「先进那个档案修数据文件」——profile.json 读不动的人根本进不去，
// 那是一句做不到的承诺（§12：坏数据不许换来假出口）。换成两种坏法都成立的一句。
func TestAdmin_坏档那一格不给做不到的承诺(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.WriteFileAtomic(s.ProfilePath("kid"), []byte("坏得读不了")); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/admin", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员会话该拿 200，得到 %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "先进那个档案") {
		t.Errorf("坏档那一格承诺了一个做不到的动作（读不动的档案进不去）:\n%.500s", body)
	}
	if !strings.Contains(body, "要在这台机器上修好那个文件") {
		t.Errorf("坏档那一格没给出诚实出路:\n%.500s", body)
	}
	if !strings.Contains(body, "数据文件读不了") {
		t.Errorf("坏档那一行丢了那句状态（管理页与进门页同一词汇）:\n%.500s", body)
	}
	// 正对照：两行档案仍然都在（爸爸是健康的、kid 是坏的那份）。没有这条，上面三条
	// 会因为"整页没渲染出档案行"而一起成立——那是假绿。
	if !strings.Contains(body, "爸爸") {
		t.Errorf("管理页没渲染出爸爸那一行:\n%.500s", body)
	}
	// **整页一个 `action="/switch"` 都不许有**：那是免 PIN 直达别人的台留下的
	// 唯一入口，删掉之后它必须消失（误留着就是一条后门）。
	if strings.Contains(body, `action="/switch"`) {
		t.Errorf("管理页还留着切档表单——那是管理员进别人台的后门:\n%.500s", body)
	}
}
