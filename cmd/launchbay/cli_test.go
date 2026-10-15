package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/httpapi"
	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

// CLI 不许直接写盘：它唯一的通路是 REST。这条用例用假服务当证人——
// 真去写文件的话，这个测试里根本没有 store 可写。
func TestCLIFallsThroughRESTOnly(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		if r.Header.Get("Authorization") == "" {
			t.Errorf("CLI 没带令牌")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":"dad","rev":3,"restoredFrom":1}`))
	}))
	defer srv.Close()

	c := &client{base: srv.URL, token: "tok", hc: srv.Client()}
	raw, code, err := c.do("POST", "/api/v1/revert?profile=dad&to-rev=1", nil, map[string]string{"X-LB-By": "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Fatalf("code %d %s", code, raw)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "POST /api/v1/revert") {
		t.Fatalf("got %+v", got)
	}
}

// 服务没起来时必须报错退出，不许"降级成离线直写"。
func TestCLIFailsLoudlyWhenServiceDown(t *testing.T) {
	c := &client{base: "http://127.0.0.1:1", token: "tok", hc: &http.Client{}}
	_, _, err := c.do("POST", "/api/v1/revert?profile=dad&to-rev=1", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "连不上") {
		t.Fatalf("该给出可行动的失败: %v", err)
	}
}

// 非 2xx 要把服务端的 detail 带出来，不是只给退出码。
func TestCLISurfacesServerDetail(t *testing.T) {
	err := mustOK([]byte(`{"code":404,"detail":"档案或该 rev 的快照不存在"}`), 404, "/api/v1/revert")
	if err == nil || !strings.Contains(err.Error(), "快照不存在") {
		t.Fatalf("got %v", err)
	}
}

// runCLI 的分发表面：**认得出的子命令一律在碰 env 与网络之前返回**，参数不
// 对就是用法错误 2 而不是"带着零值发一次请求"。
//
// 这一格早年是"尚未实现"占位（不许静默当成功），落地后换成了现在这条：
// `profile` 底下有 create 与 rebuild 两个子命令（rebuild 2026-10-10 才落地，
// 在那之前敲它打出的是 create 的用法——那份命令只存在于文档里的日子）。
// `sync-template` 随模板机制整条删掉，所以它**现在不是 CLI 命令**：不认得的
// 第一个参数必须老实落到服务端分支去，而不是被一个已删的子命令名接走。
func TestCLIDispatchAndUsage(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		want    int
		needles []string
	}{
		// profile 底下现在是 create 与 rebuild 两个子命令，用法文本列的是那两行
		// （"rebuild 落地之前这里打的是 create 的用法"正是它曾经的样子）。
		// 完整的两行由 TestCLIProfileUsageListsBothSubcommands 钉。
		{[]string{"profile"}, 2, []string{"用法", "launchbay profile create"}},
		{[]string{"profile", "list"}, 2, []string{"用法", "launchbay profile create"}},
		// **sync-template 不再是子命令**（2026-10-10，模板整条拆掉）：它曾经
		// 是"把一份共有模板推进各个档案"，而每个档案现在完全分离、没有共有的东西。
		// 所以这一格钉的是"**它现在不是 CLI 命令**"——runCLI 对它返回 notCLICommand。
		// 注意这**不等于**"该起服务"：那一步判断在 preConfigDispatch（main 那一格），
		// 由 TestPreConfigDispatch 钉。两层的分工就是这一格存在的原因。
		{[]string{"sync-template"}, notCLICommand, nil},
		{[]string{"revert"}, 2, []string{"用法: launchbay revert --profile <id> --to <rev>"}},
		// --to 0 是端点与 CLI 取法分歧的那一格：CLI 按用法错误拒（revert.go
		// 注释里写了理由），不许把它当"恢复到最初"悄悄放行。
		{[]string{"revert", "--profile", "dad", "--to", "0"}, 2, []string{"用法"}},
		{[]string{"revert", "--profile", "dad", "--to", "-3"}, 2, []string{"用法"}},
	} {
		var buf bytes.Buffer
		if code := runCLI(tc.args, config{Port: "7600"}, &buf); code != tc.want {
			t.Fatalf("%v: code=%d want %d out=%q", tc.args, code, tc.want, buf.String())
		}
		for _, needle := range tc.needles {
			if !strings.Contains(buf.String(), needle) {
				t.Fatalf("%v: 输出该含 %q，实际 %q", tc.args, needle, buf.String())
			}
		}
	}
}

// cliCfg 从 t.Setenv 过的 env 走真实的 loadConfig——e2e 测的就是
// "main.go 把同一份 cfg 交给 runCLI"这条生产接线。
func cliCfg(t *testing.T, srv *httptest.Server) config {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAUNCHBAY_PORT", u.Port())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// adminKeyEnv 摆好 CLI 侧那两格凭证（2026-10-12）：档案作用域令牌留在
// LAUNCHBAY_WRITE_TOKENS，管理员是**独立的一把** LAUNCHBAY_ADMIN_TOKEN。
// 少了第二格 loadConfig 直接拒绝启动（"这台机器就没有人能建档"），
// 留着 * 那一格则是明确报错——所以每个端到端用例都要走这个 helper。
func adminKeyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "ai-dad:tok-dad:dad")
	t.Setenv("LAUNCHBAY_ADMIN_TOKEN", testAdminKey)
}

// testAdminKey 是这些端到端用例里那把管理员 key。newRealServer 用同一个值装配
// httpapi 的 AdminToken，否则 CLI 发出的 Bearer 与服务端认的那把对不上。
const testAdminKey = "tok-admin"

// 端到端跑真 runCLI → cmdRevert → newAdminClient → c.do，假服务当证人。
// 重点是 X-LB-By: cli：丢了它，每一次 CLI revert 都会被历史悄悄记成 web——
// 这个头就是 spec 缺口裁定为"人在终端跑 CLI"专门引入的渠道。
// 顺带钉 query 转义：--profile 值带空格不许拼出多一个参数。
func TestCLIRevertEndToEnd(t *testing.T) {
	adminKeyEnv(t)
	var methods, paths, queries, bys, auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		queries = append(queries, r.URL.RawQuery)
		bys = append(bys, r.Header.Get("X-LB-By"))
		auths = append(auths, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":"dad","rev":3,"restoredFrom":1}`))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	if code := runCLI([]string{"revert", "--profile", "dad", "--to", "1"}, cliCfg(t, srv), &buf); code != 0 {
		t.Fatalf("成功该退出码 0，实际 %d out=%q", code, buf.String())
	}
	if len(methods) != 1 || methods[0] != "POST" || paths[0] != "/api/v1/revert" {
		t.Fatalf("该打 POST /api/v1/revert，实际 method=%+v path=%+v", methods, paths)
	}
	if queries[0] != "profile=dad&to-rev=1" {
		t.Fatalf("query 不对: %q", queries[0])
	}
	if bys[0] != "cli" {
		t.Fatalf("X-LB-By: cli 丢了，CLI 渠道会被记成 web: %q", bys[0])
	}
	// **该用的是作用域为 dad 的那枚令牌**（2026-10-10 改判）：管理员 key 只管
	// 账号层面，写书签一律要档案作用域令牌。这条断言是那条边界在机器那一面的
	// 证人——把它改回那把管理员 key，等于把 CLI 改回用管理员凭证写任何人的书签。
	// （2026-10-12 起管理员不再是"列表里作用域为 * 的那一条"，而是独立的一把 key，
	// 但这条边界一个字没变。）
	if auths[0] != "Bearer tok-dad" {
		t.Fatalf("该用作用域为 dad 的令牌而不是管理员令牌: %q", auths[0])
	}
	if !strings.Contains(buf.String(), "已恢复到 rev 1") {
		t.Fatalf("成功话术该带上 rev: %q", buf.String())
	}
	// 转义这一条改到 revertPath 上单独验（2026-10-10）：经 runCLI 那条路已经
	// 走不到含空格的 profile——CLI 先挑令牌，而令牌作用域必须是合法 slug，
	// 配置里不可能有 "dad x" 的令牌。"自己转义、不信任 flag"本身不变，
	// 钉在下面那条上。
	if got := revertPath("dad x", 2); got != "/api/v1/revert?profile=dad+x&to-rev=2" {
		t.Fatalf("--profile 值该被转义: %q", got)
	}
}

// 服务返回 503 → 退出 1、detail 原样带出；且失败当场证明 CLI 没在本地
// 写过任何东西：工作目录仍然空着（CLI 唯一的写通路是 REST）。
func TestCLIRevertServerErrorExitsOneNoLocalWrite(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	adminKeyEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":503,"detail":"服务正在维护，恢复已取消"}`))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	if code := runCLI([]string{"revert", "--profile", "dad", "--to", "1"}, cliCfg(t, srv), &buf); code != 1 {
		t.Fatalf("服务侧失败该退出 1，实际 %d out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "服务正在维护") {
		t.Fatalf("detail 该原样带出来: %q", buf.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("CLI 在本地写了东西（它唯一的写通路是 REST）: %+v", entries)
	}
}

// 建档**从零开始**：不带任何预置内容（模板机制 2026-10-10 整条拆掉了）。
//
// 这条现在钉的是一个原则而不只是一个参数：每个档案完全分离，建出来时形状
// 一模一样——没有"谁的档案起步就比别人多几条"。
// 以前这里断言的是"模板克隆过来了"，那时新档案的初始内容取决于点名了哪个模板。
func TestProfileCreateEndToEnd(t *testing.T) {
	s, srv := newRealServer(t)
	defer srv.Close()
	adminKeyEnv(t)
	var buf bytes.Buffer
	// **不带 --admin**（2026-10-12）：那个旗标随档案的 admin 属性一起删掉了，
	// 留着它 flag 会当场报错退出 2。它现在仍是"用法错误"，由下面那半条钉。
	code := runCLI([]string{"profile", "create", "--id", "mom", "--title", "妈妈",
		"--pin", "1234"}, cliCfg(t, srv), &buf)
	if code != 0 {
		t.Fatalf("建档该成功, code=%d out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "已建档 mom") {
		t.Fatalf("成功话术不对: %q", buf.String())
	}
	p, err := s.LoadProfile("mom")
	if err != nil {
		t.Fatal(err)
	}
	// 原来这里是 `if !p.Admin`——`model.Profile.Admin` 2026-10-12 已删（钉的字段不存在），
	// 换成钉**还在的**那些，免得这条用例只剩下"建出来了"这一句。
	if p.ID != "mom" || p.Title != "妈妈" || p.PIN == "" {
		t.Fatalf("建出来的档案形状不对: %+v", p)
	}
	raw, err := os.ReadFile(s.ProfilePath("mom"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "admin") {
		t.Fatalf("建出来的 profile.json 不该含 admin 键（2026-10-12）:\n%s", raw)
	}
	b, err := s.LoadBookmarks("mom")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Groups) != 0 {
		t.Fatalf("建出来的档案不该带任何预置分组，实际 %+v", b.Groups)
	}
	// 被删掉的旗标必须**真的是用法错误**：flag 会拒绝它，于是退出 2 而不是建出一个档案。
	var badBuf bytes.Buffer
	if code := runCLI([]string{"profile", "create", "--id", "kid", "--title", "娃",
		"--pin", "1234", "--admin"}, cliCfg(t, srv), &badBuf); code != 2 {
		t.Fatalf("--admin 已删，该是用法错误 2，实际 %d out=%q", code, badBuf.String())
	}
}

// `--nopass` 是「明确不设 PIN」（2026-10-12）。命令行上这一格更要紧：漏敲
// `--pin` 与故意不设，如果用同一个形状表达，脚本里那行就会静悄悄地建出一份
// 局域网里谁都能进的书签页——而脚本往往是在 CI 或者半夜跑的，没人会看见。
func TestProfileCreateNopass(t *testing.T) {
	s, srv := newRealServer(t)
	defer srv.Close()
	adminKeyEnv(t)

	var buf bytes.Buffer
	code := runCLI([]string{"profile", "create", "--id", "tv", "--title", "客厅",
		"--nopass"}, cliCfg(t, srv), &buf)
	if code != 0 {
		t.Fatalf("--nopass 该建得出来, code=%d out=%q", code, buf.String())
	}
	p, err := s.LoadProfile("tv")
	if err != nil {
		t.Fatal(err)
	}
	if p.PIN != "" {
		t.Fatalf("--nopass 建出来的却带 PIN: %q", p.PIN)
	}
	// 成功话术必须把这件事说出来：CLI 的输出多半是被人看或者被脚本读的，
	// 而"已建档 tv"与"已建档 mom"长得一模一样。
	if !strings.Contains(buf.String(), "不设 PIN") {
		t.Errorf("建出不设防的那一份，话术里要写明:\n%s", buf.String())
	}

	// 两种给错都必须是**用法错误 2**，而不是发出去让服务端猜：
	//   - 两个都不给 → 服务端回 422 说「pin 要 4-6 位数字」，而我压根没打 PIN。
	//   - 两个都给 → 服务端按 nopass 走，--pin 被静默丢掉，下一个人会以为门锁上了。
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"两个都不给", []string{"profile", "create", "--id", "tv2", "--title", "另一个"}},
		{"两个都给", []string{"profile", "create", "--id", "tv2", "--title", "另一个",
			"--pin", "1234", "--nopass"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if code := runCLI(tc.args, cliCfg(t, srv), &b); code != 2 {
				t.Fatalf("该是用法错误 2，实际 %d out=%q", code, b.String())
			}
			if _, err := s.LoadProfile("tv2"); err == nil {
				t.Error("用法错误之后却建出了档案")
			}
		})
	}
}

// —— Step 5：端到端（假服务 = 真 handler + 真 store）——
// newRealServer 用真的 httpapi handler 当假服务：CLI 的每一次调用都会真的
// 落到临时目录里的档案上。这样"只经 REST"与"干跑不写盘"都是实测，不是断言愿望。
func newRealServer(t *testing.T) (*store.Store, *httptest.Server) {
	t.Helper()
	s := store.New(t.TempDir())
	hash := func(pin string) string {
		h, err := auth.HashPIN(pin)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	// dad：夹具档案，带一个非空的清单——建档/导入门槛那几条用例要的就是"这个档案不是空的"。
	// 不带 Admin（2026-10-12）：`NewProfileInput.Admin` 已删，档案不再有管理员属性。
	if _, err := s.CreateProfile(store.NewProfileInput{
		ID: "dad", Title: "爸爸", PinHash: hash("1234")}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 1, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList,
		Items: []model.Item{{ID: "dsm", Name: "我家的群晖", URL: "http://10.0.0.3:5000",
			Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	// 档案作用域令牌 + **那把管理员 key**（2026-10-12：管理员不在列表里了，
	// 它是独立的 AdminToken，装配点按同一个 httpapi.Dependencies 接进去）。
	scopes, err := auth.ParseScopes("ai-dad:tok-dad:dad")
	if err != nil {
		t.Fatal(err)
	}
	adminKey, err := auth.ParseAdminToken(testAdminKey)
	if err != nil {
		t.Fatal(err)
	}
	// 缓存建在同一个临时 store 上并与 Render/Cache 共用一个对象
	// （与 httpapi 的 newDeps 同形，裁定 R3：两处必须是同一面镜像）。
	fc, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	return s, httptest.NewServer(httpapi.New(httpapi.Dependencies{
		Store: s, Tokens: scopes, AdminToken: adminKey,
		Render: render.New(s, e2eIconIndex(), fc), Icons: e2eIconIndex(),
		Fetch: icons.NewFetcher(), Cache: fc}).Handler())
}

// —— 导入：0 条的两个形状 ——
//
// `cmdImportBookmarks` 在"一条都进不来"时**碰不到 REST**：Convert 完就返回 1。
// 所以这两条不需要假服务，夹具落到临时文件即可——而这恰好让它们成为"这句话说对了吗"
// 的干净证人。
//
// 两种"0 条"必须说两句：文件里除了回收站什么都没有（老情形），或者读出了书签但一条
// 都没能进来（全是 javascript: 这类没有主机名的地址）。混成一句会把人引去换一个同样
// 导不进来的文件。
func TestCLIImport_全都没能进来时说另一句(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "bookmarks.html")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	const allSkipped = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>Bookmarks</TITLE>
<DL><p><DT><H3>小工具</H3><DL><p>
<DT><A HREF="javascript:void(0)">甲</A>
<DT><A HREF="javascript:void(1)">乙</A>
</DL><p></DL><p>`
	const trashedOnly = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>Bookmarks</TITLE>
<DL><p><DT><H3>回收站</H3><DL><p>
<DT><A HREF="https://example.com/a">早就删了的一条</A>
</DL><p></DL><p>`

	for _, tc := range []struct {
		name   string
		file   string
		needle string
		absent string
	}{
		{"全是进不来的", allSkipped, "全都没能进来", "回收站里的那些有意的没算在内"},
		{"只有回收站", trashedOnly, "回收站里的那些有意的没算在内", "全都没能进来"},
	} {
		var buf bytes.Buffer
		code := runCLI([]string{"import-bookmarks", "--profile", "dad",
			write(t, tc.file)}, config{Port: "7600"}, &buf)
		if code != 1 {
			t.Errorf("%s：该退出 1，实际 %d out=%q", tc.name, code, buf.String())
		}
		if !strings.Contains(buf.String(), tc.needle) {
			t.Errorf("%s：输出该含 %q，实际 %q", tc.name, tc.needle, buf.String())
		}
		if strings.Contains(buf.String(), tc.absent) {
			t.Errorf("%s：输出不该含 %q（那是另一个形状的话）: %q", tc.name, tc.absent, buf.String())
		}
	}
}

// mustPlan 那套薄封装是 sync-template 时代留下的（2026-10-10 随模板整条删掉）；
// 这里只留下真装配服务用的那份共享图标索引——batch 的活闸门要真索引。
//
// 真装配服务必须带真索引：导入的 add 带着图标名走 batch，typed-nil 的
// *icons.Index 进了闸门会在 Has 上解引用空指针——这里不降级、不传 nil，整个测试包
// 共用一份真索引，让 CLI 端到端也过活闸门。
var (
	e2eIconsOnce sync.Once
	e2eIcons     *icons.Index
)

func e2eIconIndex() *icons.Index {
	e2eIconsOnce.Do(func() { e2eIcons = icons.MustLoad() })
	return e2eIcons
}

// preConfigDispatch 那三条判据各有一位证人（2026-10-16）。
//
// 这一格从前不存在：`--help` 与一个打错的子命令都直接落进 loadConfig 之后。
// 后果实测过两条——help 的回答是一句"未配置 LAUNCHBAY_ADMIN_TOKEN"（唯一那份
// 说明在唯一需要它的场合拿不到），而 `launchbay revrt …` 的回答是一句
// "listening on :17999"，端口还是它自己上一行占的。
func TestPreConfigDispatch(t *testing.T) {
	var so, se bytes.Buffer
	if act, code := preConfigDispatch(nil, &so, &se); act != dispatchServe || code != 0 {
		t.Fatalf("什么都没给必须起服务，拿到 %v/%d", act, code)
	}

	for _, a := range []string{"-h", "--help", "help"} {
		so.Reset()
		se.Reset()
		act, code := preConfigDispatch([]string{a}, &so, &se)
		if act != dispatchExit || code != 0 {
			t.Fatalf("%s: 要退 0，拿到 %v/%d", a, act, code)
		}
		// 印的必须是那份名单本身，而不是一句"见 README"。
		for _, want := range []string{"revert", "profile", "import-bookmarks", "healthcheck"} {
			if !strings.Contains(so.String(), want) {
				t.Fatalf("%s 的输出缺 %q：%s", a, want, so.String())
			}
		}
		if se.Len() != 0 {
			t.Fatal("--help 走 stdout，不许往 stderr 印东西")
		}
	}

	so.Reset()
	se.Reset()
	act, code := preConfigDispatch([]string{"revrt"}, &so, &se)
	if act != dispatchExit || code != 2 {
		t.Fatalf("未知子命令要退 2，拿到 %v/%d", act, code)
	}
	if !strings.Contains(se.String(), `未知子命令 "revrt"`) {
		t.Fatalf("要点名那个打错的词：%s", se.String())
	}
	if so.Len() != 0 {
		t.Fatal("错误走 stderr，不走 stdout")
	}

	// healthcheck 与 -flag 开头的两个形状**不许被这一格接走**：前者是容器探活
	// （它要在 loadConfig 之后读同一份 env），后者留给子命令自己的 flag 解析。
	for _, a := range []string{"healthcheck", "-port=1", "--profile"} {
		if act, code := preConfigDispatch([]string{a}, io.Discard, io.Discard); act != dispatchServe {
			t.Fatalf("%q 不该被这一格接走，拿到 %v/%d", a, act, code)
		}
	}
	// 已知子命令同样放行——真正的分发在 runCLI，那里有别的证人。
	for _, c := range cliCommands {
		if act, _ := preConfigDispatch([]string{c.name}, io.Discard, io.Discard); act != dispatchServe {
			t.Fatalf("已知子命令 %q 不该被这一格接走", c.name)
		}
	}
}

// 端点回了 200 但**响应读不出来**时，CLI 不许替它报数（2026-10-16）。
// 从前这两条路都是 `_ = json.Unmarshal(...)` 然后照常打印，于是形状不对的
// 200 会印成"已恢复到 rev 0 的内容，新的 rev 是 0"——一个编出来的成功，
// 退出码还是 0，脚本作者据此认为回退做完了。
func TestCLIDoesNotInventSuccessFromAnUnreadableBody(t *testing.T) {
	cases := []struct {
		name string
		body string
		args []string
	}{
		{"revert 回的不是 JSON", `not json at all`, []string{"revert", "--profile", "dad", "--to", "3"}},
		// rebuild 回的是**缺 title** 的半份：从前那行印成"已重建 dad（）"。
		{"rebuild 缺字段", `{"id":"dad"}`, []string{"profile", "rebuild", "--profile", "dad", "--title", "爸爸", "--nopass"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			u, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("LAUNCHBAY_PORT", u.Port())
			t.Setenv("LAUNCHBAY_WRITE_TOKENS", "ai-dad:tok-dad:dad")
			t.Setenv("LAUNCHBAY_ADMIN_TOKEN", testAdminKey)
			cfg, err := loadConfig()
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			code := runCLI(tc.args, cfg, &buf)
			out := buf.String()
			if code == 0 {
				t.Fatalf("读不出来的响应被当成了成功（退 0）：%q", out)
			}
			for _, fabricated := range []string{"已恢复到 rev 0", "已重建", "已建档"} {
				if strings.Contains(out, fabricated) {
					t.Fatalf("输出里出现了编出来的结论 %q：%q", fabricated, out)
				}
			}
		})
	}
}
