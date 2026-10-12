package httpapi

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// setupServer 是夹具的唯一实现处：setup 与需要 *Server 的测试（require
// 判据表）共用同一份档案/令牌，不复制第二套 fixture。
func setupServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	s := store.New(t.TempDir())
	hash := func(pin string) string {
		h, err := auth.HashPIN(pin)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "dad", Title: "爸爸", PinHash: hash("1234")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "kid", Title: "小满", PinHash: hash("5678")}); err != nil {
		t.Fatal(err)
	}
	scopes, err := auth.ParseScopes("ai-dad:tok-dad:dad,ai-kid:tok-kid:kid,ai-ghost:tok-ghost:ghost,ai-nobody:tok-nobody:nobody")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(newDeps(t, s, scopes))
	return srv, srv.Handler()
}

func setup(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	srv, h := setupServer(t)
	return srv.deps.Store, h
}

func post(h http.Handler, path, body, cookie, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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

func get(h http.Handler, path, cookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// postForm 走浏览器无 JS 时选择页的编码：application/x-www-form-urlencoded。
func postForm(h http.Handler, path, body, cookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cookieMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, c := range rec.Result().Cookies() {
		out[c.Name] = c.Value
	}
	return out
}

// 登录只发**一条** cookie（2026-10-10 第三轮：代管态下线，lb_home 不再发出去），
// 并顺手把升级前那一条作废。
//
// 这条断的是"不再发"而不只是"发了什么"：一条服务端不再使用、却还赖在浏览器里的
// cookie 是那种没人记得为什么会存在的残留，而它会让人以为"会话有两条"。
func TestPinGrantsOneSessionCookie(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/pin", `{"profile":"dad","pin":"1234"}`, "", "")
	if rec.Code != 200 {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	cm := cookieMap(t, rec)
	if cm[auth.CookieProfile] != "dad" {
		t.Fatalf("会话 cookie 不对: %v", cm)
	}
	if v, ok := cm[auth.CookieHome]; !ok || v != "" {
		t.Fatalf("lb_home 不该再被发出来（值要为空）: %v", cm)
	}
	// 响应里 managing 这个键也删了（它恒为 false，读它的人会被骗）。
	if strings.Contains(rec.Body.String(), "managing") {
		t.Errorf("/pin 的 JSON 响应里不该再有 managing: %s", rec.Body.String())
	}
}

// 关掉 JS 的进门通路：选择页的卡片提交就是 urlencoded 表单，必须落
// 会话 cookie 并把浏览器带回首页（303），而不是 422 一段 JSON。
func TestPinFormPostSetsCookiesAndRedirectsToRoot(t *testing.T) {
	_, h := setup(t)
	rec := postForm(h, "/pin", "profile=dad&pin=1234", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("表单提交 want 303, got %d %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location = %q, want /（表单提交要落在页面上）", loc)
	}
	cm := cookieMap(t, rec)
	if cm[auth.CookieProfile] != "dad" {
		t.Fatalf("会话 cookie 不对: %v", cm)
	}
}

// 表单支持不许把 JSON 通路挤坏：Task 9 的测试用的是同一个入口，这里
// 再钉一次响应形状——200 + JSON 体、没有 Location，未知字段照旧 422。
func TestPinJSONPostStillGetsJSONBodyNotRedirect(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/pin", `{"profile":"dad","pin":"1234"}`, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("JSON want 200, got %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("JSON 调用方不该被重定向: %q", loc)
	}
	var body struct {
		Active   string `json:"active"`
		Managing bool   `json:"managing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体不是 JSON: %v %s", err, rec.Body)
	}
	if body.Active != "dad" || body.Managing {
		t.Fatalf("JSON 体不合规: %s", rec.Body)
	}
	// DisallowUnknownFields 只属于 JSON 通路。
	if rec := post(h, "/pin", `{"profile":"dad","pin":"1234","extra":1}`, "", ""); rec.Code != 422 {
		t.Fatalf("未知字段 want 422, got %d %s", rec.Code, rec.Body)
	}
}

// 表单进门的失败必须是"回到选择页并说清楚哪里错了"，不是浏览器里一段
// 裸 JSON：无 JS 的家人没有脚本可跑，401 + JSON 体等于把他关在门外
// （部署验收 Important #2）。一条 cookie 都不许落下——进门失败却发会话，
// 比回到选择页坏得多。
//
// 2026-10-12：空表单不再有专属的一把键。夹具里两份档案都有 PIN，所以"忘了输"
// 与"输错了"是同一件事的两种写法，都答 `pin`；一份**故意不设 PIN** 的档案
// 交空表单是**进得来**的，那条走 TestPinForm_不设防的档案空表单就能进。
func TestPinFormFailuresRedirectBackToPicker(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct{ name, body, cookie, wantErr string }{
		{"缺 pin 就是没输对", "profile=dad", "", "pin"},
		{"空 pin 就是没输对", "profile=dad&pin=", "", "pin"},
		{"PIN 错", "profile=dad&pin=0000", "", "pin"},
		{"档案不存在", "profile=ghost&pin=1234", "", "profile"},
		{"slug 非法", "profile=%2B%2B&pin=1234", "", "profile"},
		// 下面两格是 2026-10-10 改判的证人：会话成不合法**不影响**这一支的
		// 答案。原来这里有"会话形态非法"与"非管理员免 PIN 切档"两种分诊——那是
		// 代管态的两条出路，而免 PIN 直达（`/switch to=<id>`）已经整条删掉了。
		// 于是两种会话（合法的本人会话、被伪造成代管的会话）拿到同一个答案；
		// 2026-10-12 允许不设 PIN 之后，这个答案是 `pin` 而不是 `missing`——
		// 判据只有 auth.VerifyEntry 一处，会话根本不参与。
		{"带会话的空 pin 也只是没输对", "profile=kid&pin=", "lb_profile=dad; lb_home=kid", "pin"},
		{"伪代管会话不再有出路", "profile=dad&pin=", "lb_profile=kid; lb_home=dad", "pin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postForm(h, "/pin", tc.body, tc.cookie)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("表单失败 want 303, got %d %s", rec.Code, rec.Body)
			}
			if loc := rec.Header().Get("Location"); loc != "/?err="+tc.wantErr {
				t.Fatalf("Location = %q, want /?err=%s", loc, tc.wantErr)
			}
			if cs := rec.Result().Cookies(); len(cs) != 0 {
				t.Fatalf("失败却落了 cookie: %v", cs)
			}
		})
	}
}

// 不设 PIN 的档案，空表单就该进得来（2026-10-12）。这是"可以不设"这个承诺的
// 行为证人：只钉"有 PIN 的档案仍然要输对"是不够的——那条在改动之前就成立，
// 而这一条是**新**能力，只在盘上那份档案确实没有 PIN 时才成立。
//
// 夹具另外建一份 `open`：PinHash 留空。建档走 store 而不是 API/表单，所以这条
// 只钉"进门"这一段，不掺建档那一段的读法（那条另有证人）。
func TestPinForm_不设防的档案空表单就能进(t *testing.T) {
	s, h := setup(t)
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "open", Title: "客厅那台"}); err != nil {
		t.Fatal(err)
	}
	// 两条都验：字段整格缺席（模板 `{{if .NeedsPIN}}` 那一支）与字段在但留空。
	// 后者才是**手滑**的样子——选择页该画框就画框，只是这份档案压根不核对。
	for _, body := range []string{"profile=open", "profile=open&pin="} {
		rec := postForm(h, "/pin", body, "")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s 想要 303 进档案页，实际 %d %s", body, rec.Code, rec.Body)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Fatalf("%s 该 303 回 /（首页按会话渲染成档案页），实际 %q", body, loc)
		}
		// 进门要落一条**指向这一份**的会话 cookie。注意它不是唯一一条：同一处
		// 还会把升级前那条 `lb_home` 清掉（Max-Age=0），所以这里找的是"有
		// 没有把 lb_profile 设成 open"，不是"一共有几条"。
		var got []string
		for _, c := range rec.Result().Cookies() {
			got = append(got, c.Name+"="+c.Value)
		}
		if !slices.Contains(got, "lb_profile=open") {
			t.Fatalf("进门要落 lb_profile=open，实际 %v", got)
		}
	}
	// 停用优先于"没有 PIN"：一份被管理员停用的不设防档案也不能进——
	// 「谁都能进」说的是这台机器上的局域网，不是任何人。
	disableViaStore(t, s, "open", true)
	rec := postForm(h, "/pin", "profile=open", "")
	if loc := rec.Header().Get("Location"); loc != "/?err=disabled" {
		t.Fatalf("停用优先于不设防，实际 %q", loc)
	}
}

// ?err= 只当白名单的键用：不在表里的值一个字都不许进页面——query 是
// 用户可控的，把它原样渲染等于开一个反射点。
func TestRootNoticeComesFromWhitelistNotQuery(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/?err=pin", "")
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "PIN 不对") {
		t.Fatalf("白名单文案没进页面: %s", body)
	}
	if !strings.Contains(body, `role="alert"`) {
		t.Fatalf("提示要能被读屏认出来（role=alert）")
	}
	// 未登记的键：页面回到普通形状，既不报错也不回显。
	rec = get(h, "/?err=%3Cscript%3Ealert(1)%3C%2Fscript%3E", "")
	body = rec.Body.String()
	if strings.Contains(body, "<script") || strings.Contains(body, "alert(1)") || strings.Contains(body, "err=") {
		t.Fatalf("未登记的 err 码被回显了: %s", body)
	}
	// 已经进门的人不该看到提示：提示只属于选择页。
	rec = post(h, "/pin", `{"profile":"dad","pin":"1234"}`, "", "")
	cm := cookieMap(t, rec)
	own := auth.CookieProfile + "=" + cm[auth.CookieProfile] + "; " + auth.CookieHome + "=" + cm[auth.CookieHome]
	rec = get(h, "/?err=pin", own)
	if got := rec.Header().Get("X-LB-Profile"); got != "dad" {
		t.Fatalf("带着会话却被当选择页: X-LB-Profile=%q", got)
	}
	if strings.Contains(rec.Body.String(), "PIN 不对") {
		t.Fatalf("档案页面不该出现选择页提示")
	}
}

// 最终整枝评审 Important #8：maxBodyBytes 对表单通路也得是真的。
// ParseForm 会整段读 body，超限必须被拒——超限的 PIN 永远验证不过，
// 但 1 MiB 的钉是"承诺的上限成立"，不是"PIN 校验兜底"。
func TestOversizedFormBodyIsRejected(t *testing.T) {
	_, h := setup(t)
	body := "profile=dad&pin=" + strings.Repeat("9", maxBodyBytes+4096)
	rec := postForm(h, "/pin", body, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("超过 maxBodyBytes 的表单 want 422, got %d %.200s", rec.Code, rec.Body)
	}
	if cs := rec.Result().Cookies(); len(cs) != 0 {
		t.Fatalf("被拒的表单提交不该落 cookie: %v", cs)
	}
}

func TestPinWrongIs401UnknownProfileIs404(t *testing.T) {
	_, h := setup(t)
	if rec := post(h, "/pin", `{"profile":"dad","pin":"0000"}`, "", ""); rec.Code != 401 {
		t.Fatalf("错 PIN want 401, got %d", rec.Code)
	}
	if rec := post(h, "/pin", `{"profile":"ghost","pin":"1234"}`, "", ""); rec.Code != 404 {
		t.Fatalf("不存在 want 404, got %d", rec.Code)
	}
	// 401 与 404 必须可区分：前者是"这档案不是你的"，后者是"没这档案"。
}

func TestNonAdminCannotSwitchWithoutPin(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct{ name, cookie string }{
		{"本人会话", "lb_profile=kid"},
		{"无会话", ""},
		{"非法形态（缺 home）", "lb_profile=kid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(h, "/pin", `{"profile":"dad","pin":""}`, tc.cookie, "")
			if rec.Code != 401 {
				t.Fatalf("cookie %q want 401, got %d %s", tc.cookie, rec.Code, rec.Body)
			}
		})
	}
}

// 历史说明（这条注释曾经指向两个已经不在的测试，留着记录它们为什么消失）：
// 本处原有 TestForgedHomeDoesNotGrantAdmin（断言 (kid, dad-home) 免 PIN 切回
// dad 要 401），最终整枝评审 Group 3.2 把它改判成
// TestExitManagingBackToHomeNeedsNoAdminProof——当时 spec §6.1 明写"退出管理"
// 这条免 PIN 写回，于是判据翻了个个儿。
//
// **2026-10-10 代管态整条下线，那条测试也跟着没了**：要它守的"退出管理"端点
// （`POST /switch to=home`）已经不存在，守一个不存在的端点没有意义。
// 「伪造 home 拿不到额外权限」这件事本身**仍然有人钉**，证人换成下面两条：
// TestLegacyManagingCookieIsClearedNotRendered（升级前那种代管 cookie 一律判
// 非法并清掉）与 TestAdminLookupHomeGrantsNothing（伪造 home 换不来任何权限）。

// 升级前发出去的那枚**代管 cookie**（`lb_profile=kid; lb_home=dad`）必须被清掉、
// 落在选择页，而不是把 kid 的台渲染出来。
//
// 这条是本轮改动最要紧的证人：服务端不再发 lb_home 了，读侧于是"只写一条、读两条
// 只为拒旧形状"。**读侧要是干脆不看 lb_home，这枚 cookie 就重新变成后门**——它
// 自包含，读的人只看到 lb_profile=kid，于是被当成"kid 自己登录"，而当年它真的
// 是管理员代管 kid 时用的。代管 2026-10-10 已下线，这种 cookie 不再合法。
//
// 判页看 X-LB-Profile 而不是内容里有没有名字：选择页真的会列档案名（"小满"是合法
// 选项），而渲染错页时标题是"发射台"。
func TestLegacyManagingCookieIsClearedNotRendered(t *testing.T) {
	_, h := setup(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", "lb_profile=kid; lb_home=dad")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200 选择页, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-LB-Profile"); got != "picker" {
		t.Fatalf("旧代管 cookie 给了 %q 页, want picker", got)
	}
	if strings.Contains(rec.Body.String(), "发射台") {
		t.Fatalf("旧代管 cookie 渲染出了档案页:\n%s", rec.Body)
	}
	var cleared int
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			cleared++
		}
	}
	if cleared != 2 {
		t.Fatalf("want 两条 cookie 被清（会话 + 升级前那条）, got %d", cleared)
	}
}

// 升级**不许把人踢下线**：升级前那对相等的 cookie（`lb_profile=dad`）
// 照旧能进门、照旧出档案页。
//
// 它与上一条是同一个改判的两面：两条不等 = 旧代管 = 后门，要拒；两条相等 = 普通
// 本人会话，没理由让人重新输一次 PIN。把"读侧不再发 lb_home"做成"读侧完全不认
// lb_home"的话，这条就会红——而那正是把升级变成一次全员登出的最省事写法。
func TestLegacyEqualPairStillWorksAfterUpgrade(t *testing.T) {
	_, h := setup(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", "lb_profile=dad")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-LB-Profile"); got != "dad" {
		t.Fatalf("升级前的本人会话被踢下线了（落到了 %q 页）——升级不该要求重新输 PIN", got)
	}
}

// §7.1：/api/v1 前缀下所有响应一律 `private, no-store`，状态码不论。
// 中间件挂在前缀上而不是逐个 handler 里设——P1-② 还要加七八个端点，
// 靠"每个人都记得"迟早漏一个。前缀外的响应不受影响（/healthz 佐证）。
func TestAPIPrefixAlwaysNoStore(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", "tok-admin")
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("201 响应 Cache-Control = %q", got)
	}
	rec = post(h, "/api/v1/profiles", `{}`, "", "")
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("401 响应 Cache-Control = %q", got)
	}
	// 未注册样例路径：/api/v1/data 在 Task 6 已注册成读端点，留着它这条
	// 用例测的就不再是"未注册"（400 而非 404）。换 ghost 并写明意图。
	req := httptest.NewRequest("GET", "/api/v1/ghost-endpoint", nil)
	r2 := httptest.NewRecorder()
	h.ServeHTTP(r2, req)
	if r2.Code != http.StatusNotFound {
		t.Fatalf("前缀下未注册路径 want 404, got %d", r2.Code)
	}
	if got := r2.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("404 响应 Cache-Control = %q", got)
	}
	req = httptest.NewRequest("GET", "/healthz", nil)
	r3 := httptest.NewRecorder()
	h.ServeHTTP(r3, req)
	if got := r3.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("/healthz 不该被 API 前缀中间件管到: %q", got)
	}
}

func TestCreateProfileNeedsAdminToken(t *testing.T) {
	s, h := setup(t)
	if rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", "tok-dad"); rec.Code != 403 {
		t.Fatalf("非管理员令牌 want 403, got %d", rec.Code)
	}
	if rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", "nope"); rec.Code != 401 {
		t.Fatalf("坏令牌 want 401, got %d", rec.Code)
	}
	rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", "tok-admin")
	if rec.Code != 201 {
		t.Fatalf("want 201, got %d %s", rec.Code, rec.Body)
	}
	if _, err := s.LoadProfile("mom"); err != nil {
		t.Fatalf("档案没落盘: %v", err)
	}
	if strings.Contains(rec.Body.String(), "9999") {
		t.Fatalf("响应回显了 PIN: %s", rec.Body)
	}
}

// 最终整枝评审 #1：建档的访问判定交回 require（唯一判据），与
// GET/DELETE /profiles、/profiles/pin 同族——spec §6.1 的"仅管理员"。
// 四种凭证形态各一个结局：管理员 key 201、**档案会话 403**（2026-10-12 起
// "管理员"是机器级的一把 key，登进 dad 不等于有管理权）、档案作用域令牌 403、
// 未知令牌 401（writeAuthFailure 的 401/403 分流）。
// 末尾的按字节断言同时是评审 #2 的证人：201 体键序必须是文档说的 id,title——
// 换回 map 会被 Go 排成字母序，先红这里。
func TestCreateProfileAccessGoesThroughRequire(t *testing.T) {
	s, h := setup(t)
	// (a) 管理员 key（LAUNCHBAY_ADMIN_TOKEN 那把）。
	got := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", testAdminKey)
	if got.Code != 201 {
		t.Fatalf("管理员 key 建档 want 201, got %d %s", got.Code, got.Body)
	}
	if want := "{\n  \"id\": \"mom\",\n  \"title\": \"妈妈\"\n}\n"; got.Body.String() != want {
		t.Fatalf("201 体必须按文档键序逐字节为 %q, got %q", want, got.Body.String())
	}
	if _, err := s.LoadProfile("mom"); err != nil {
		t.Fatalf("建出来的档没落盘: %v", err)
	}
	// (a2) **登进 dad 不等于有管理权**：那只是"某人正在用的那一份"。
	// 少这一格的话，"管理员会话也能建档"那条旧承诺会一直躺在注释里。
	dadCookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	if rec := post(h, "/api/v1/profiles", `{"id":"mom9","title":"九妈","pin":"9999"}`, dadCookie, ""); rec.Code != 403 {
		t.Fatalf("只有档案会话 want 403, got %d %s", rec.Code, rec.Body)
	}
	// (b) 别人的档案会话：同样 403。
	kidCookie := sessionCookie(t, loginAs(t, h, "kid", "5678"))
	if rec := post(h, "/api/v1/profiles", `{"id":"mom2","title":"二妈","pin":"9999"}`, kidCookie, ""); rec.Code != 403 {
		t.Fatalf("非管理员会话 want 403, got %d %s", rec.Code, rec.Body)
	}
	// (c) 档案作用域令牌：与旧行为一致，403。
	if rec := post(h, "/api/v1/profiles", `{"id":"mom3","title":"三妈","pin":"9999"}`, "", "tok-dad"); rec.Code != 403 {
		t.Fatalf("作用域令牌 want 403, got %d %s", rec.Code, rec.Body)
	}
	// (d) 未知令牌：401，不许并成 403（死令牌重试没意义）。
	if rec := post(h, "/api/v1/profiles", `{"id":"mom4","title":"四妈","pin":"9999"}`, "", "nope"); rec.Code != 401 {
		t.Fatalf("未知令牌 want 401, got %d %s", rec.Code, rec.Body)
	}
	// 四条里只有 (a) 该落盘。
	if _, err := s.LoadProfile("mom2"); err == nil {
		t.Fatal("403 却建出了档案")
	}
}

func TestCreateProfileRejectsBadBody(t *testing.T) {
	_, h := setup(t)
	cases := []struct {
		body  string
		field string
	}{
		{`{"id":"mom"}`, "title"},
		{`{"id":"mom","title":"妈","pin":"12"}`, "pin"},
		{`{"id":"Mom","title":"妈","pin":"9999"}`, "id"},
		{`{"id":"mom","title":" ","pin":"9999"}`, "title"},
		{`{"id":"mom","title":"妈","pin":"9999","extra":1}`, "extra"},
	}
	for _, tc := range cases {
		rec := post(h, "/api/v1/profiles", tc.body, "", "tok-admin")
		if rec.Code != 422 {
			t.Fatalf("body %s want 422, got %d %s", tc.body, rec.Code, rec.Body)
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
		if len(out.Errors) == 0 {
			t.Fatalf("422 必须逐条给字段: %s", rec.Body)
		}
		var hit bool
		for _, e := range out.Errors {
			if e.Field == tc.field && e.Reason != "" {
				hit = true
			}
		}
		if !hit {
			t.Fatalf("body %s 要报出字段 %q: %s", tc.body, tc.field, rec.Body)
		}
	}
}

// —— 权限只看当前在用的档案（lb_profile）。下面两条是**升级前那种 cookie**
// （lb_profile 与 lb_home 不相等）的证人：代管 2026-10-10 下线之后，那种形状
// 不再合法，Session.Valid 一并拒掉。

// 改写说明（最终整枝评审 Group 3.1，本条原名
// TestAdminLookupFromActiveProfileIgnoresHome）：旧断言基于 2ff6a1c 裁定
// 的"home 写成谁都无所谓"，但同一份 spec §6.1 明写第三种形态（home 指向
// 非管理员档案且 ≠ lb_profile）非法——(dad, kid-home) 恰好是它。磁盘背书
// 的合法性判据（Valid 且 home==active 或 home 在盘上真是管理员）现在由
// handleRoot 与 handlePin 共用，非管理员/不存在 home 的会话免 PIN 切档
// 一律 401。"管理员判定看 Active 不看 Home"的证人现在是下面 (b)：
// (kid, dad-home) 切第三方 mom 必须 401——旧的 Home-回查实现在这里放行。
func TestThirdShapeRejectedAtPin(t *testing.T) {
	s, h := setup(t)
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "mom", Title: "妈妈", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range []string{"lb_profile=dad; lb_home=ghost", "lb_profile=dad; lb_home=kid"} {
		rec := post(h, "/pin", `{"profile":"mom","pin":""}`, cookie, "")
		if rec.Code != 401 {
			t.Fatalf("cookie %q 是第三种形态，want 401, got %d %s", cookie, rec.Code, rec.Body)
		}
	}
}

// 第三种形态在 GET / 的行为（评审 Important #2）：§6.1 要求清两条 cookie
// 回选择页，不渲染任何书签。旧实现只查 cookie 形状不查盘，
// (lb_profile=dad, lb_home=ghost) 拿着 dad 的管理员身份照常出书签页。
func TestThirdShapeClearedAtRoot(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct{ name, cookie string }{
		{"home 指向已删除的档案", "lb_profile=dad; lb_home=ghost"},
		{"home 不是管理员", "lb_profile=dad; lb_home=kid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Cookie", tc.cookie)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("want 200 选择页, got %d", rec.Code)
			}
			if got := rec.Header().Get("X-LB-Profile"); got != "picker" {
				t.Fatalf("非法形态给了 %q 页, want picker", got)
			}
			if strings.Contains(rec.Body.String(), "发射台") {
				t.Fatalf("非法形态渲染出了档案页:\n%s", rec.Body)
			}
			var cleared int
			for _, c := range rec.Result().Cookies() {
				if c.MaxAge < 0 {
					cleared++
				}
			}
			if cleared != 2 {
				t.Fatalf("want 两条 cookie 被清, got %d", cleared)
			}
		})
	}
}

func TestCorruptProfileAtRootShowsExplainerNotClear(t *testing.T) {
	s, h := setup(t)
	if err := os.WriteFile(s.ProfilePath("dad"), []byte("{\n  \"id\": \"dad\",\n  \"title\":\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("坏 profile.json 要给解释页, got %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "读不了") {
		t.Fatalf("解释页必须说清档案读不了:\n%.400s", body)
	}
	if got := rec.Header().Get("X-LB-Profile"); got != "dad" {
		t.Fatalf("解释页要落在坏的那个档案上, got %q", got)
	}
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			t.Fatalf("坏数据文件不是登出理由，不许清 cookie: %+v", c)
		}
	}
	if strings.Contains(body, s.Root()) {
		t.Fatalf("解释页泄露了磁盘路径: %s", body)
	}
}

func TestAdminLookupHomeGrantsNothing(t *testing.T) {
	s, h := setup(t)
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "mom", Title: "妈妈", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	rec := post(h, "/pin", `{"profile":"mom","pin":""}`, "lb_profile=kid; lb_home=dad", "")
	if rec.Code != 401 {
		t.Fatalf("非管理员在用 + 管理员 home 竟可切档: %d %s", rec.Code, rec.Body)
	}
}

// 旧的 body 里那个 admin 键：**422，且点名是它**。
//
// 这与磁盘上的 profile.json 走的是相反的口径，那是有意的：落盘的 `admin` 必须被
// 悄悄放过（否则升级就要求用户挨个 rebuild），而**请求体**里送来一个已经不存在的
// 字段，是一个脚本对这台机器的误解——沉默地忽略会让它以为"建成管理员了"，
// 而实际什么也没发生。422 把字段名点出来，脚本作者一眼知道该删哪一行。
func TestCreateProfile用管理员key建出普通档案(t *testing.T) {
	s, h := setup(t)
	rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", testAdminKey)
	if rec.Code != 201 {
		t.Fatalf("管理员 key 该被接受, got %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), `"admin"`) {
		t.Errorf("201 的响应体不该再有 admin 那一格: %s", rec.Body)
	}
	p, err := s.LoadProfile("mom")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.PIN, "sha256$") {
		t.Errorf("PIN 要经 HashPIN 存成三段式，得到 %q", p.PIN)
	}
	rec = post(h, "/api/v1/profiles", `{"id":"kid2","title":"二小","pin":"8888","admin":true}`, "", testAdminKey)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("带 admin 的 body 该 422，得到 %d %s", rec.Code, rec.Body)
	}
	if !fieldErrorsName(rec, "admin") {
		t.Errorf("422 要点名 admin 那一格：脚本作者需要知道该删哪一行，实际 %s", rec.Body)
	}
}

// 409 路径：管理员令牌建一个已存在的 id。
func TestCreateExistingProfileIsConflict(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/api/v1/profiles", `{"id":"kid","title":"再建一个","pin":"9999"}`, "", "tok-admin")
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复建档 want 409, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Code != 409 || out.Detail == "" {
		t.Fatalf("409 体不合规: %s", rec.Body)
	}
}

// 改写说明（最终整枝评审 Group 3.3，本条原名 TestInternalErrorBodyIsGeneric）：
// id 与目录名不一致是 profile.json 坏了（ErrCorrupt），§12 要求它与
// bookmarks.json 同等待遇——给家属看的解释页，而不是一个笼统的 500。
// 原断言"500 + generic body"被 spec 推翻；泄露检查逐字保留（响应不许
// 出现磁盘根路径），并新增 PIN 哈希不外泄的断言。
func TestCorruptProfilePinShowsExplainerNot500(t *testing.T) {
	s, h := setup(t)
	raw, err := os.ReadFile(s.ProfilePath("kid"))
	if err != nil {
		t.Fatal(err)
	}
	swapped := strings.Replace(string(raw), `"id": "kid"`, `"id": "other"`, 1)
	if swapped == string(raw) {
		t.Fatalf("夹具没改掉 id:\n%s", raw)
	}
	if err := os.WriteFile(s.ProfilePath("kid"), []byte(swapped), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := post(h, "/pin", `{"profile":"kid","pin":"5678"}`, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("读不了的档案 want 200 解释页（§12），got %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "读不了") {
		t.Fatalf("要给「这个档案读不了」的解释页:\n%.400s", body)
	}
	if strings.Contains(body, s.Root()) {
		t.Fatalf("响应泄露了磁盘路径: %s", body)
	}
	if strings.Contains(body, "sha256$") {
		t.Fatalf("解释页不许抖出 PIN 哈希: %s", body)
	}
}

// ErrNotFound 与 ErrCorrupt 必须分流（评审 Important #4）：档案不存在
// 照旧 404 JSON——"没这个档案"与"档案坏了"是两句话。
func TestPinUnknownProfileStill404NotExplainer(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/pin", `{"profile":"ghost","pin":"1234"}`, "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("404 要维持 JSON 形状: %q", ct)
	}
}

// P1-① 遗留 Minor 2 的证人：writeInternalError 的"真 I/O 错误 → 500 且
// 响应不回显内部错误"此前没有覆盖。夹具按 Windows 可行的路子制造
// ReadFile 失败（os.Chmod(dir, 0o000) 在 Windows 不稳）：把 dad 的
// bookmarks.json 换成一个**目录**——os.ReadFile 打开目录句柄后读必然
// 报错（实测 Windows 是 "Incorrect function."，POSIX 是 EISDIR），而这不是
// ErrNotExist、也不会被登记成坏档（markBroken 只记解码/校验失败），
// 于是 /api/v1/data 走到 handleData 的 writeInternalError 分支。
// 断言两头：500 的形状是 generic 的 {code, detail}；响应里既没有磁盘
// 路径也没有 Go 错误串——内部原因只进 stderr 给运维看。
func TestRealIOErrorGivesGeneric500(t *testing.T) {
	s, h := setupBatch(t)
	path := s.BookmarksPath("dad")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/data?profile=dad", dadTok)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("真 I/O 错误该 500, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != 500 || out.Detail != "内部错误" {
		t.Fatalf("500 体必须是 generic 形状: %s", rec.Body)
	}
	body := rec.Body.String()
	// Go 的错误串（"read <路径>: …"）必然带着完整路径，路径不在即串不在；
	// 再单独钉一次文件名与根目录，防的是"只漏文件名"这种半截回显。
	if strings.Contains(body, s.Root()) || strings.Contains(body, "bookmarks.json") {
		t.Fatalf("500 响应泄露了磁盘路径/内部错误: %s", body)
	}
}

// dadFormCookie 是"爸爸本人看自己"的合法会话形状（两条都要有，缺一服务端
// 就当非法会话清掉——见 handlers_test.go 里既有的 PIN 成功用例）。
// 名字带 Form：handlers_test.go#TestCreateProfileAccessGoesThroughRequire 与
// pinapi_test.go#TestPinChangeDoesNotTouchRevOrSnapshots 各有一个**函数内局部变量**
// 叫 kidCookie/dadCookie（那是 sessionCookie() 算出来的），同名包级常量合法但会
// 遮蔽出两种含义——一个文件里两个同名不同源的东西正是下一个人读错的地方。
// Task 3 与 Task 4 的测试复用同一条串。
const dadFormCookie = "lb_profile=dad"

// 管理员 cookie 的夹具。2026-10-12 起管理员与档案会话是**两件独立的事**，于是
// 测试里必须能分开摆出三种形状：
//
//	adminOnlyCookie  拿着管理权，没登任何档案（最常见的形状）
//	dadFormCookie    登着 dad，**没有**管理权
//	adminAndDad      两者都有 —— 用来钉 selfpin / deleteBlocked 那两条保护
//
// 早先这三者是同一个东西（dad 既是管理员又是在用的人），所以只有 dadFormCookie
// 一个常量。合成一个之后，"管理员"与"某个人"就再也分不开了——而那正是这次要拆的。
func adminOnlyCookie(t *testing.T) string {
	t.Helper()
	return auth.CookieAdmin + "=" + testAdminToken(t).DeriveCookie()
}

// authOnly 只取管理员 cookie 那一段（不带任何档案会话）。切档那类测试要自己
// 拼 `lb_profile=<id>; <这个>`，用它就不会把 `lb_profile` 也带进去。
func authOnly(t *testing.T) string {
	t.Helper()
	return adminOnlyCookie(t)
}

func adminAndDadCookie(t *testing.T) string {
	t.Helper()
	return dadFormCookie + "; " + adminOnlyCookie(t)
}

// q 是用户可控的 query。没有会话时它必须**一个字都不进页面**：选择页没有
// 搜索框，把 q 回声进去就等于开一个反射点（与 ?err= 只查表、不解释同一口径）。
func TestRoot_无会话时q被忽略且不回声(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/?q=%3Cimg%20src%3Dx%20onerror%3Dalert(1)%3E", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "onerror") || strings.Contains(body, "&lt;img") ||
		strings.Contains(body, `name="q"`) {
		t.Errorf("选择页把 q 回声进去了:\n%.400s", body)
	}
}

// 有会话时 q 进搜索框的 value——html/template 在那里转义，两句话都要钉：
// 一条钉它被回填（搜索态），一条钉它没变成可执行上下文。
func TestRoot_搜索框里的q是转义过的(t *testing.T) {
	_, h := setupBatch(t)
	rec := get(h, `/?q=<b>"x`, dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `<b>"x`) {
		t.Fatalf("q 没转义:\n%.400s", body)
	}
	// 钉**转义后的形状**，不只是"没原样出现"：把回填的字段换成 template.HTML
	// 时 html/template 会在属性位置把可疑片段吃掉（实测得到 value="&#34;x"），
	// 只查"没原样出现 + 有 value=\"" 的那半截证人会漏掉这种"q 被截掉一截"的
	// 实现——搜索框回填本身就是功能，缺字节也是坏。
	if !strings.Contains(body, `value="&lt;b&gt;&#34;x"`) {
		t.Errorf("q 没按 html/template 的属性转义回填进搜索框:\n%.400s", body)
	}
	if !strings.Contains(body, "搜索书签") {
		t.Errorf("搜索框没回填:\n%.400s", body)
	}
}

// 同一份原文两处消费：家属看的解释页与 AI 看的 503 detail。为了一张页面
// 把详情改成第二套，两处就会各说各话——这条钉住它们逐字相同。
func Test坏档原文页面与503同源(t *testing.T) {
	s, h := setup(t)
	// 截断的 JSON：错误文本是"…第 N 行附近: JSON 在值结束前就断了"，
	// 不含引号与尖括号，页面渲染与 API detail 可以直接逐字比。
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{\n \"rev\": 2,\n")); err != nil {
		t.Fatal(err)
	}
	page := get(h, "/", dadFormCookie)
	if page.Code != http.StatusOK {
		t.Fatalf("坏档案给的是解释页而不是 500，得到 %d", page.Code)
	}
	body := page.Body.String()

	rec := getBearer(h, "/api/v1/data?profile=dad", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("坏书签档案的读通路该 503，得到 %d: %s", rec.Code, rec.Body)
	}
	var errBody struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Detail == "" {
		t.Fatal("503 的 detail 是空的：§12 要求报得出文件与位置")
	}
	// 页面里那份是 HTML 转义之后的字节，比较前先按同一套转义走一遍
	// （直接比会在原文含引号/尖括号时假红；用 HTMLEscapeString 才是在比
	// 同一件事的两个副本）。
	if want := string(template.HTMLEscapeString(errBody.Detail)); !strings.Contains(body, want) {
		t.Errorf("解释页里的原文与 503 的 detail 不是同一句\n页面:\n%.400s\ndetail: %q", body, errBody.Detail)
	}
}

// 503 文本还有第二个来源：corruptDetail 只在"本次错误自己的文本点不出文件"时
// 才回退到坏档登记表。校验类坏档正是这一支——load.go 那边返回的是 errors.Join
// 出来的串（ErrCorrupt 的文案里没有 .json），登记表那份才是包了文件名的 wrapped。
// 这一支今天零覆盖（评审 I-3）：把它改成 return msg，AI 侧的 503 立刻丢掉文件名
// （§12"报得出文件与位置"对 API 那一半失效），而解释页与 detail 各说各话——
// 同源不变量在这条支路上完全没有保护。夹具挑的是"合法 JSON、非法内容"那一类：
// 条目地址没有主机名（search/weather 那族已经不再参与校验，见 model/check.go——
// 用它们做夹具会让这条证人测一个已经不存在的形状）。
func Test坏档原文同源_校验类走登记表回退(t *testing.T) {
	s, h := setup(t)
	doc := `{"rev":1,"title":"爸爸的台","groups":[{"id":"nas","title":"NAS","layout":"tiles",` +
		`"items":[{"id":"i1","name":"没主机的地址","url":"mailto:home@launchbay.lan","alias":[],"tags":[]}]}]}`
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte(doc)); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/data?profile=dad", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("内容非法的书签档案读通路该 503，得到 %d: %s", rec.Code, rec.Body)
	}
	// 登记表是在这次读的时候才被写上的（markBroken 在 LoadBookmarks 里），所以取它
	// 要在请求之后——先钉住它点得出文件名：Join 那份点不出，这正是回退分支存在的
	// 理由，也是"以后有人统一成 return msg"会悄悄丢掉的东西。
	reason, ok := s.BrokenReason("dad")
	if !ok || reason == "" {
		t.Fatalf("校验失败要进坏档登记表（回退分支的取文处）: %v %q", ok, reason)
	}
	if !strings.Contains(reason, "bookmarks.json") {
		t.Fatalf("登记表的原因要点名文件（fixture 前提）: %q", reason)
	}
	// 前提也要钉（评审 R-5）：这条测试跑的是**回退**分支，而回退的判据是"本次错误
	// 自己的文本点不出文件"。上面只钉了登记表那份点得出，没钉 Join 那份点不出——
	// 万一哪天有人把 load.go 的校验分支改成返回带文件名的 wrapped（技术上更"对"），
	// corruptDetail 就会走直通分支，detail 照样等于 reason、这整条测试照样绿，
	// 而它声称在测的那条支路已经没人守了。这里直接问一次 store 给的是什么。
	_, err := s.LoadBookmarks("dad")
	if err == nil {
		t.Fatal("夹具坏了还读得出来，这条测试的整个前提没了")
	}
	if strings.Contains(err.Error(), ".json") {
		t.Fatalf("本次错误的文本如今点得出文件（%q），corruptDetail 走的是直通分支，这条测试不再测回退", err.Error())
	}
	var out struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Detail != reason {
		t.Errorf("回退分支要给的是登记表那句原文：detail=%q reason=%q", out.Detail, reason)
	}
	page := get(h, "/", dadFormCookie)
	if page.Code != http.StatusOK {
		t.Fatalf("同一份坏数据下，家属侧该是解释页而不是 500，得到 %d", page.Code)
	}
	if want := string(template.HTMLEscapeString(out.Detail)); !strings.Contains(page.Body.String(), want) {
		t.Errorf("校验类坏档的解释页与 503 不同源\ndetail: %q\n页面:\n%.400s", out.Detail, page.Body.String())
	}
}

// 代管态与它护着的那一维缓存键都在 2026-10-10 改判里删掉了（见
// `2026-10-10-launchbay-admin-boundary-design.md`）。新边界的证人在
// `boundary_test.go`——那里钉的是"管理员碰不到别人的书签"这件事本身，
// 比"某个函数该返回空串"这种形状断言耐改。
