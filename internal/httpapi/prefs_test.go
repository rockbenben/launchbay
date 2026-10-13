package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 发请求的 helper 与常量本包都有了：`postForm(h, path, body, cookie)`、
// `get(h, path, cookie)`、`cookieMap(t, rec)`、`dadFormCookie`（前三件在
// handlers_test.go，`dadFormCookie` 由 Task 2 加在同一文件）。
// **不要再定义第二遍**——同包重名是编译错误，换个名字就是第二套夹具。

func TestPrefs_主题改得动且立刻看得见(t *testing.T) {
	s, h := setup(t)
	// 先把首页烤进缓存：这一条测的是"改了立刻看得见"，而看不见有两种原因——
	// 端点没写进去，或写进去了而缓存端出来还是旧的。不先请求一次首页，
	// 第二次请求是冷缓存、必然重算，端点那半就永远证不到 profile.json 指纹那一维。
	warm := get(h, "/", dadFormCookie)
	if strings.Contains(warm.Body.String(), `data-theme="light"`) {
		t.Fatal("夹具一开始就是 light，换成 light 就测不出变化了")
	}
	rec := postForm(h, "/prefs", "theme=light", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("改成功要 303 回首页，得到 %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q，要 /", loc)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "light" {
		t.Errorf("盘上 theme = %q，要 light", p.Theme)
	}
	// 关键一步：紧接着读首页，必须已经带上新主题。这一条同时是
	// Task 1 那个 profile.json 内容指纹缓存键维度的端到端证人。
	page := get(h, "/", dadFormCookie)
	if !strings.Contains(page.Body.String(), `data-theme="light"`) {
		t.Error("改了主题而页面没变：缓存键里少了 profile.json 的指纹")
	}
}

func TestPrefs_形态与主题可以各改各的(t *testing.T) {
	s, h := setup(t)
	if rec := postForm(h, "/prefs", "layout=topbar", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("只改 layout 要 303，得到 %d", rec.Code)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Layout != "topbar" || p.Theme != "auto" {
		t.Errorf("只给 layout 却动了别的字段：layout=%q theme=%q", p.Layout, p.Theme)
	}
	// 写出去的文件仍守 §4.3：缩进 2 空格、末尾一个换行、中文不转义。
	raw, err := os.ReadFile(s.ProfilePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(raw, []byte("}\n")) || bytes.HasSuffix(raw, []byte("\n\n")) {
		t.Errorf("profile.json 的写出形状破了 §4.3: %q", raw[len(raw)-8:])
	}
}

// 整页排法（全局视图）改得动、只动它自己，而且**改完立刻看得见**：
// 判据是页面 `<body>` 上那个 data-profile-view——CSS 那一档全靠它，
// 属性没发出去的话这一档在界面上根本不存在（功能通 ≠ 界面可选到）。
func TestPrefs_排法改得动且页面带上属性(t *testing.T) {
	s, h := setup(t)
	// 全部走 pageText：内联 CSS 里全是 `[data-profile-view=wall]` 那种选择器，
	// 直接对整份 HTML 找这个串，"属性发没发"永远是真的（同一份内联 CSS 也刚把
	// historypage_test 里那句"130 漏进别人那一页"变成假话）。
	if rec := get(h, "/", dadFormCookie); strings.Contains(pageText(rec.Body.String()), "data-profile-view") {
		t.Errorf("没改过就不该发这个属性（空 = 没选过，默认那一档由 CSS 的缺省形状兜）")
	}
	if rec := postForm(h, "/prefs", "view=wall", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("只改 view 要 303，得到 %d", rec.Code)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.View != "wall" || p.Theme != "auto" || p.Layout != "sidebar" {
		t.Errorf("只给 view 却动了别的字段：view=%q theme=%q layout=%q", p.View, p.Theme, p.Layout)
	}
	page := get(h, "/", dadFormCookie).Body.String()
	if !strings.Contains(pageText(page), `data-profile-view="wall"`) {
		t.Errorf("页面没带上 data-profile-view，那一档的 CSS 就找不到落点：%s", headLines(page, 6))
	}
	// 切回分组：属性发的是 groups，而**没选过**的那一份什么都不发（模板那个
	// `{{with}}`）。两种形状渲染出来同一张脸——CSS 只认 `=wall`，剩下的一律走
	// 缺省形状；区别只在盘上：一份是"这个人选过又选回来了"，一份是"没动过"。
	if rec := postForm(h, "/prefs", "view=groups", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("切回 groups 要 303，得到 %d", rec.Code)
	}
	if page = get(h, "/", dadFormCookie).Body.String(); !strings.Contains(pageText(page), `data-profile-view="groups"`) {
		t.Errorf("切回之后页面该发 data-profile-view=\"groups\"：%s", headLines(page, 6))
	}
}

// 两颗二态开关：标签**不随状态换**，状态走 aria-pressed。
//
// 这条钉的是"形状"而不是"改得动"（改得动由上面那两条覆盖）：从前这两颗键把状态
// 印在文字里（「分组」/「全局」），读屏拿到的是一个每次按下都改名字的可及名，
// 而"这是不是一个开关"没有任何地方说。稳定标签 + aria-pressed 才是那个标准形状，
// 顺带把顶栏的宽度也钉住了（文字不换 = 不跳）。
//
// 「顶栏导航」这一颗补的是一个一直缺着的入口：Profile.Layout 此前只有 CSS 与
// 模板属性，表单没有键、扩展与 CLI 都不收，只有手改 profile.json（禁止事项）。
func TestPrefs_两颗开关的标签稳定而状态走aria(t *testing.T) {
	s, h := setup(t)
	// 起点：dad 是 sidebar + 没选过 view（盘上没有那个键）
	two := func(html string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`<button type="submit" aria-pressed="(\w+)" title="[^"]*">([^<]+)</button>`).FindAllStringSubmatch(html, -1) {
			out = append(out, m[2]+"="+m[1])
		}
		return out
	}
	page := get(h, "/", dadFormCookie).Body.String()
	got := two(page)
	want := []string{"全局视图=false", "顶栏导航=false"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("静息态的两颗开关不对：%v，要 %v", got, want)
	}
	// 开全局视图：标签一个字不动，只有 pressed 翻
	if rec := postForm(h, "/prefs", "view=wall", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("view=wall 要 303，得到 %d", rec.Code)
	}
	if got = two(get(h, "/", dadFormCookie).Body.String()); strings.Join(got, ",") != "全局视图=true,顶栏导航=false" {
		t.Errorf("开完全局视图：%v", got)
	}
	// 开顶栏导航：两维互不干涉（view 还开着）
	if rec := postForm(h, "/prefs", "layout=topbar", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("layout=topbar 要 303，得到 %d", rec.Code)
	}
	page = get(h, "/", dadFormCookie).Body.String()
	if got = two(page); strings.Join(got, ",") != "全局视图=true,顶栏导航=true" {
		t.Errorf("两维都开：%v", got)
	}
	if !strings.Contains(pageText(page), `data-profile-layout="topbar"`) || !strings.Contains(pageText(page), `data-profile-view="wall"`) {
		t.Errorf("两个属性没同时发出来：%s", headLines(page, 6))
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Layout != "topbar" || p.View != "wall" {
		t.Errorf("盘上不是那两个值：layout=%q view=%q", p.Layout, p.View)
	}
	// 那颗键送的是**另一档**（hidden 由服务端算），所以再按一次就关得掉
	if rec := postForm(h, "/prefs", "layout=sidebar", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("切回 sidebar 要 303，得到 %d", rec.Code)
	}
	if got = two(get(h, "/", dadFormCookie).Body.String()); strings.Join(got, ",") != "全局视图=true,顶栏导航=false" {
		t.Errorf("关掉顶栏导航：%v", got)
	}
}

// headLines 取**看得见的字**的前 n 行：整份 HTML 的头几十行全是内联 CSS，
// 失败信息贴出来没法读（pageText 那一格的同一件事）。
func headLines(html string, n int) string {
	ls := strings.Split(pageText(html), "\n")
	if len(ls) > n {
		ls = ls[:n]
	}
	return strings.Join(ls, "\n")
}

// 非法值与"没说改什么"都是 422，而且要**点名是哪个字段**——422 不指字段，
// 调用方只能猜；而"两个字段都不给"静默 303 会让一个拼错的字段名假装成功。
func TestPrefs_非法与缺字段都422且点名(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct {
		name, body, wantField string
	}{
		{"非法主题", "theme=pink", `"field": "theme"`},
		{"非法形态", "layout=list", `"field": "layout"`},
		{"非法排法", "view=tiled", `"field": "view"`},
		// 空串**不是**"清回默认"的写法：那会与服务端"没这个键 = 没选过"那一格撞车
		// （Profile.View 的注释）。表单要清就送 groups。
		{"排法不吃空值", "view=", `"field": "view"`},
		{"大小写不放过", "theme=Light", `"field": "theme"`},
		{"两个都不给", "", `"field": "theme"`},
		{"只有无关字段", "color=red", `"field": "layout"`},
		{"同名多值", "theme=dark&theme=light", `"field": "theme"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postForm(h, "/prefs", tc.body, dadFormCookie)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"code": 422`) {
				t.Errorf("要走统一错误外壳: %s", rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tc.wantField) {
				t.Errorf("422 要点名字段 %q，得到: %s", tc.wantField, rec.Body)
			}
		})
	}
}

// 浏览器通路的**权限类**拒绝 = 303 回选择页、不写盘（spec §6.1）。
// 令牌走 /prefs 与没带会话走 /prefs 是同一种长相：这条路本来就不给 AI 用，
// 而"你是令牌所以我拒"这种区分只在 API 通路有意义（那里 401/403 永远分开）。
// 断言的核心不是状态码，是**盘上没变**——把 credSession 换成 credScoped，
// 管理员令牌就能替所有人改偏好，那时状态码仍是 303 而这条会红。
func TestPrefs_令牌与无会话都不写盘(t *testing.T) {
	// 三种长相都要试，因为它们过 require 的路径不同：
	//   - 令牌 + 会话 cookie 是唯一能区分 credSession 与 credScoped 的那一种：
	//     只带令牌时 sess.Active 是空串，credScoped 开头那条"空申报一律拒"会先
	//     把它挡下来，换成 credScoped 也照样拒——那一条证不到东西（夹具形状取自
	//     pinapi_test.go 的"cookie + 管理员令牌 → 403"）。
	//   - 只带令牌、无凭证：两种凭证类别都该拒，留着是因为它们各自是真实长相。
	// 每个子用例各起一套 setup：共用一个 store 时第一个用例若真写脏了盘，后面两个
	// 会替它红——红错地方就找不到凶手，而"到底哪一支漏了"正是这条测试要回答的。
	for _, tc := range []struct {
		name, cookie string
		tok          bool
	}{
		{"管理员令牌带着 dad 的会话", dadFormCookie, true},
		{"管理员令牌不带会话", "", true},
		{"无凭证", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, h := setup(t)
			req := httptest.NewRequest(http.MethodPost, "/prefs", strings.NewReader("theme=dark"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.tok {
				req.Header.Set("Authorization", "Bearer "+adminTok)
			}
			if tc.cookie != "" {
				req.Header.Set("Cookie", tc.cookie)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
				t.Errorf("得到 %d %q，要 303 回选择页（浏览器通道的拒不该吐 401 页面）",
					rec.Code, rec.Header().Get("Location"))
			}
			p, err := s.LoadProfile("dad")
			if err != nil {
				t.Fatal(err)
			}
			// 夹具的建档默认值是 auto：盘上仍是 auto 才算"被拒的那一次什么都没写"。
			if p.Theme != "auto" {
				t.Fatalf("被拒的一支真的落盘了：theme=%q", p.Theme)
			}
		})
	}
}

func TestPrefs_超限表单被拒且不落盘(t *testing.T) {
	s, h := setup(t)
	body := "theme=dark&junk=" + strings.Repeat("x", maxBodyBytes+1)
	rec := postForm(h, "/prefs", body, dadFormCookie)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("超过 1 MiB 的表单要 422，得到 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"field": "body"`) {
		t.Errorf("要说的是表单读不出来: %s", rec.Body)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "auto" {
		t.Errorf("超限表单里带的 theme=dark 竟然落盘了：theme=%q", p.Theme)
	}
}

// profile.json 坏了要答 503（与 self/pin 同口径），不许把"文件坏了"说成"服务坏了"。
// 注意这**不是**权限拒绝：读失败是第三种东西，422/503 都照旧走 JSON 外壳，
// 只有"你没资格"这一类才映射成 303。
func TestPrefs_坏profile答503(t *testing.T) {
	s, h := setup(t)
	if err := os.WriteFile(s.ProfilePath("dad"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postForm(h, "/prefs", "theme=dark", dadFormCookie)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "profile.json") {
		t.Errorf("503 的 detail 要点名文件（spec §12）: %s", rec.Body)
	}
}

// 字节层面读不出表单（转义写坏）走 422，不走上面那条"两个字段都不给"的分支：
// 两者是两种病——一个是"没说要改什么"，一个是"说了但没听清"。这条同时是
// handlePrefs 里 ParseForm 那支的唯一证人：没有它，那支改了也没人红。
func TestPrefs_表单字节坏422点名body(t *testing.T) {
	s, h := setup(t)
	rec := postForm(h, "/prefs", "theme=%zz", dadFormCookie)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"field": "body"`) {
		t.Errorf("表单读不出来要点名 body（与 /pin 同一词汇），得到: %s", rec.Body)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "auto" {
		t.Errorf("422 的一次调用不该落盘，盘上 theme = %q", p.Theme)
	}
}

// 422 那句"只能是 …"必须从 model 的枚举表生成，不是端点里再抄一份：加一档主题
// 只改 model 一处，判据与文案一起变；文案是副本的话，下一次加档就会让拒绝语
// 报出一句不含新档的话，而调用方照它写请求就永远差一档（P1-④ 评审 I-6）。
func TestPrefs_拒绝文案就是model的枚举表(t *testing.T) {
	_, h := setup(t)
	for _, c := range []struct {
		field  string
		values []string
	}{
		{"theme", model.ThemeValues},
		{"layout", model.LayoutValues},
	} {
		rec := postForm(h, "/prefs", c.field+"=不是这里面的值", dadFormCookie)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s 非法值要 422，得到 %d: %s", c.field, rec.Code, rec.Body)
		}
		var out struct {
			Errors []struct {
				Field  string `json:"field"`
				Reason string `json:"reason"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		want := "只能是 " + strings.Join(c.values, "|")
		var got string
		for _, e := range out.Errors {
			if e.Field == c.field {
				got = e.Reason
			}
		}
		if got != want {
			t.Errorf("%s 的拒绝语 = %q，要 %q（逐字等于 model 那张表）", c.field, got, want)
		}
		// 表里每一档都要真的出现在文案里：join 写错分隔符时上面那条也会红，
		// 但这一条说清红的原因是"哪一档不见了"。
		for _, v := range c.values {
			if !strings.Contains(got, v) {
				t.Errorf("%s 的拒绝语 %q 没列出 model 里的 %q", c.field, got, v)
			}
		}
	}
}

// 「这一页叫什么」那一格：改名立刻看得见、空着清除、超长点名拒。
//
// 它住在 profile.json（与 theme/layout 同族）而不是 bookmarks.json 的 Title 字段里，
// 这条测试就是那一步的证人：改完之后**书签的 rev 一个字都不许动**——
// 起个名字不是改数据，不该产生一版可回退的快照。
func TestPrefs_页面名改得动清空回默认且不碰rev(t *testing.T) {
	s, h := setup(t)
	before, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	page := get(h, "/", dadFormCookie)
	if !strings.Contains(page.Body.String(), "<h1 title=\"我的书签\">我的书签</h1>") {
		t.Fatalf("没起名字时顶栏要用通用默认「我的书签」:\n%.400s", page.Body.String())
	}
	if rec := postForm(h, "/prefs", "title=影音室", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("只改 title 要 303（它也是「给了一个字段」），得到 %d: %s", rec.Code, rec.Body)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.PageTitle != "影音室" {
		t.Errorf("盘上 pageTitle = %q，要 影音室", p.PageTitle)
	}
	after, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if after.Rev != before.Rev {
		t.Errorf("改个页面名字不该产生新 rev：%d → %d", before.Rev, after.Rev)
	}
	body := get(h, "/", dadFormCookie).Body.String()
	if !strings.Contains(body, "<h1 title=\"影音室\">影音室</h1>") {
		t.Error("改了名而顶栏没变：缓存键少了 profile.json 的指纹")
	}
	// 标签页两件都要说：页面名在前，档案名在后（切标签页认的是"谁的台"）。
	// 夹具里那份档案叫「爸爸」（handlers_test 的 setup），不是渲染侧的字面量。
	if m := regexp.MustCompile(`<title>([^<]*)</title>`).FindStringSubmatch(body); m == nil || m[1] != "影音室 · 爸爸" {
		t.Errorf("标签页要写「页面名 · 档案名」，得到 %#v", m)
	}
	// 空 = 清除，回到默认；不是"保持原样"。
	if rec := postForm(h, "/prefs", "title=", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("清空要 303，得到 %d", rec.Code)
	}
	if p, _ := s.LoadProfile("dad"); p.PageTitle != "" {
		t.Errorf("清空后盘上还剩 %q", p.PageTitle)
	}
	if body := get(h, "/", dadFormCookie).Body.String(); !strings.Contains(body, "我的书签") {
		t.Error("清空后顶栏没回到通用默认")
	}
	// 超长点名拒，且不落盘。
	long := "title=" + strings.Repeat("长", 61)
	rec := postForm(h, "/prefs", long, dadFormCookie)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("61 个字要 422，得到 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"field": "title"`) {
		t.Errorf("422 要点名 title:\n%s", rec.Body.String())
	}
	if p, _ := s.LoadProfile("dad"); p.PageTitle != "" {
		t.Errorf("被拒的那次不许写盘，盘上成了 %q", p.PageTitle)
	}
}
