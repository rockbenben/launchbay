package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesTableIsComplete(t *testing.T) {
	s := New(Dependencies{})
	seen := map[string]bool{}
	for _, rt := range s.routes() {
		key := rt.Method + " " + rt.Pattern
		if seen[key] {
			t.Fatalf("路由表里 %s 出现了两次", key)
		}
		seen[key] = true
		if rt.Summary == "" {
			t.Fatalf("%s 没有 Summary：openapi.json 与 AGENTS.md 都从这里取一句话说明", key)
		}
		if rt.H == nil {
			t.Fatalf("%s 没有 handler", key)
		}
	}
	// P1-① 就有的四条必须在表里，一条都不能被"顺手重构"弄丢。
	for _, want := range []string{"GET /healthz", "POST /pin", "GET /", "POST /api/v1/profiles"} {
		if !seen[want] {
			t.Fatalf("路由表少了 %s", want)
		}
	}
}

// 注册方式换了，行为一个字不许变：/healthz 仍是 text/plain "ok\n"，
// 未知路径仍 404，/api/v1 前缀仍带 no-store。
func TestExistingBehaviorSurvivesTheRefactor(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/healthz", "")
	if rec.Code != 200 || rec.Body.String() != "ok\n" {
		t.Fatalf("/healthz 变了: %d %q", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
	rec = get(h, "/api/v1/nothing-here", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未知 API 路径该 404, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("no-store 中间件掉了: %q", cc)
	}
}

// require 的判据表：这是"AI 误写别人档案"那条防线唯一的实现处。
// 夹具事实：setup() 建 dad(管理员)/kid 两档案，令牌 ai-dad:tok-dad:dad 与
// admin:tok-admin:*。所以 tok-dad 的作用域只有 dad——拿它请求 kid 必须拒，
// 且不许"回落"到 dad。
func TestRequireMatrix(t *testing.T) {
	// 夹具同 setup()：brief 里写的 s, _ := setup(t) 拿到的是 *store.Store
	// （既有测试按此签名用它取 store），这里改用共用 fixture 的 setupServer。
	s, _ := setupServer(t)
	req := func(tok, cookie string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/data", nil)
		if tok != "" {
			r.Header.Set("Authorization", "Bearer "+tok)
		}
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		return r
	}
	for _, tc := range []struct {
		name        string
		tok, cookie string
		profile     string
		want        bool
		wantActor   string
	}{
		// **管理员令牌动不了任何人的书签**（2026-10-10）：`*` 只管账号层面，
		// credScoped 一律拒——哪怕申报的就是"自己"那个档案也不放行。
		// 下面那几行"管理员会话代管"是同一条规则的会话侧，两边一起构成证人。
		{"管理员令牌动任何档案", "tok-admin", "", "kid", false, ""},
		{"管理员令牌动自己范围内的也不行", "tok-admin", "", "dad", false, ""},
		{"档案作用域令牌动别人", "tok-dad", "", "kid", false, ""},
		{"档案作用域令牌动自己", "tok-dad", "", "dad", true, "ai-dad"},
		{"未知令牌", "nope", "", "dad", false, ""},
		// Task 5 评审 Important 的证人：形状像真令牌的死令牌（tok-bogus）在
		// require 这里就该拒绝——handler 侧 writeAuthFailure 拿 Authorize
		// 把它分诊成 401（而不是 403），前提正是"认不出来"在这一层成立。
		{"死令牌动任何档案都拒绝", "tok-bogus", "", "dad", false, ""},
		{"管理员会话动别人的书签", "", "lb_profile=dad", "kid", false, ""},
		{"管理员会话用管理员令牌位", "", "lb_profile=dad", "dad", true, "dad"},
		{"非管理员会话跨档案", "", "lb_profile=kid", "dad", false, ""},
		{"非管理员会话动自己", "", "lb_profile=kid", "kid", true, "kid"},
		// 下面几行是**升级前那种 cookie**（lb_profile 与 lb_home 不相等）的证人。
		// cookie 不签名，所以"kid 伪造 lb_home=dad"与"dad 真的在代管 kid"是同一种
		// 形状，代码分不出来（spec §11 明写不防冒充）——但代管 2026-10-10 已下线，
		// 于是这一整个形状**不再合法**：Session.Valid 一并拒掉，user 看到的是
		// "重新登录"，不是半截权限。
		{"旧代管 cookie 动 dad 该拒", "", "lb_profile=kid; lb_home=dad", "dad", false, ""},
		{"旧代管 cookie 连自己的台也进不去", "", "lb_profile=kid; lb_home=dad", "kid", false, ""},
		// home 悬挂（ghost 是合法 slug 但盘上没有）与上面同一种判据：整枚会话作废。
		// 曾经它的结论是"写权限照成立、只是 actor 不许记那个不存在的名字"，那条
		// 口径随代管一起没了——今天没有"代管中的本人"这个形状。
		{"home 悬挂时整枚会话作废", "", "lb_profile=dad; lb_home=ghost", "kid", false, ""},
		// Task 6 评审的 Critical：空申报不是"不受限"。跨档案聚合读必须在
		// 判据处显式要 credAdmin，所以 credScoped + profileID=="" 一律拒——
		// 钉在 require 这一层，而不是钉在 /history 里（否则下一个用 require
		// 的端点会重新继承同一个洞）。管理员令牌同样不适用：类别不对就是不对。
		{"kid 会话空申报不算越权放行", "", "lb_profile=kid", "", false, ""},
		{"管理员令牌空申报也不算 scoped 放行", "tok-admin", "", "", false, ""},
		{"无凭证", "", "", "dad", false, ""},
	} {
		actor, _, ok := s.require(req(tc.tok, tc.cookie), credScoped, tc.profile)
		if ok != tc.want {
			t.Fatalf("%s: ok=%v want %v", tc.name, ok, tc.want)
		}
		if ok && actor != tc.wantActor {
			t.Fatalf("%s: actor=%q want %q（actor 要进历史）", tc.name, actor, tc.wantActor)
		}
	}
}

// credAdmin 与 credSession 各挡一半：档案作用域令牌不许走管理员端点，
// 令牌也不许走"改自己 PIN"。
func TestRequireAdminAndSessionClasses(t *testing.T) {
	s, _ := setupServer(t)
	r := httptest.NewRequest("POST", "/api/v1/status", nil)
	r.Header.Set("Authorization", "Bearer tok-dad")
	if _, _, ok := s.require(r, credAdmin, "dad"); ok {
		t.Fatal("档案作用域令牌不该过 credAdmin，哪怕 profile 就是它自己的")
	}
	r2 := httptest.NewRequest("POST", "/api/v1/profiles/self/pin", nil)
	r2.Header.Set("Authorization", "Bearer tok-admin")
	if _, _, ok := s.require(r2, credSession, "dad"); ok {
		t.Fatal("令牌不该过 credSession：改自己 PIN 要的是那个人的会话")
	}
}
