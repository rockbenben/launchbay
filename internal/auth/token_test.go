package auth

import "testing"

func TestParseScopes(t *testing.T) {
	scopes, err := ParseScopes("ai-dad:xxx:dad,ext-mom:yyy:mom")
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 2 {
		t.Fatalf("got %d", len(scopes))
	}
	if scopes[1].Name != "ext-mom" || scopes[1].Secret != "yyy" || scopes[1].Profile != "mom" {
		t.Fatalf("scope 解析错: %+v", scopes[1])
	}
}

// 档案段写 `*` 从 2026-10-12 起是**明确报错**，不是"当成管理员"也不是静默忽略。
// 那两种做法都留过害：当成管理员 = 升级后管理员令牌继续能用而形状已经变了；
// 静默忽略 = 它变成一枚永远匹配不上的死令牌，症状是"建档接口永远 403"。
// 报错里必须指出去了哪儿，否则人要自己去猜。
func TestParseScopes拒绝星号并指出出路(t *testing.T) {
	_, err := ParseScopes("ai-dad:xxx:dad,admin:zzz:*")
	if err == nil {
		t.Fatal("档案段是 * 应当报错")
	}
	if !contains(err.Error(), "LAUNCHBAY_ADMIN_TOKEN") {
		t.Fatalf("报错要说出去哪儿（LAUNCHBAY_ADMIN_TOKEN），实际：%v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestParseScopesRejects(t *testing.T) {
	for _, bad := range []string{"a:b", "a:b:c:d", "a::c", ":b:c", "a:b:", "a:b:c,,d:e:f",
		"a:b:Mom", "a:b:../etc", "   "} {
		if _, err := ParseScopes(bad); err == nil {
			t.Fatalf("应拒: %q", bad)
		}
	}
	if _, err := ParseScopes("a:b:c,x:b:y"); err == nil {
		t.Fatal("同一 secret 两个条目要视为配置错误：无法分别吊销")
	}
}

func TestParseScopesStoresTrimmedSegments(t *testing.T) {
	// 修剪判空却存未修剪分段（账本 T6 minor）：env 里 "ai-dad: xxx:dad"
	// 多打一个空格，secret 存成 " xxx"，此后 Authorize("xxx") 永远 401——
	// 正是 §6.3 反对的"操作员以为存在却永远匹配不上"的失败形状。
	scopes, err := ParseScopes("ai-dad: xxx:dad")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := scopes.Authorize("xxx")
	if !ok || s.Secret != "xxx" || s.Name != "ai-dad" || s.Profile != "dad" {
		t.Fatalf("分段必须存修剪后的值: %+v ok=%v", s, ok)
	}
	if _, ok := scopes.Authorize(" xxx"); ok {
		t.Fatal("带空格的输入不该匹配：比较对象是修剪后存的那份")
	}
}

func TestAuthorize(t *testing.T) {
	scopes, _ := ParseScopes("ai-dad:xxx:dad,ai-mom:zzz:mom")
	if s, ok := scopes.Authorize("xxx"); !ok || s.Profile != "dad" {
		t.Fatalf("got %+v %v", s, ok)
	}
	if s, ok := scopes.Authorize("zzz"); !ok || s.Profile != "mom" {
		t.Fatalf("mom: %+v %v", s, ok)
	}
	for _, secret := range []string{"nope", "", "XXX", "xxx ", "x"} {
		if _, ok := scopes.Authorize(secret); ok {
			t.Fatalf("secret %q 不该通过", secret)
		}
	}
}

// Allows 现在只有一种答案：这一枚能不能动这个档案。跨档案永远是 403——
// 管理员走的是另一条路（require 里先被分走），根本到不了这里。
func TestScopeAllows(t *testing.T) {
	scopes, _ := ParseScopes("ai-dad:xxx:dad,ai-mom:zzz:mom")
	dad, _ := scopes.Authorize("xxx")
	mom, _ := scopes.Authorize("zzz")
	if !dad.Allows("dad") || dad.Allows("mom") {
		t.Fatalf("dad scope 判定错: %+v", dad)
	}
	if !mom.Allows("mom") || mom.Allows("dad") {
		t.Fatalf("mom scope 判定错: %+v", mom)
	}
	if dad.Allows("") || dad.Allows("*") {
		t.Fatal("没有任何档案作用域令牌该放行空目标或星号")
	}
}

func TestParseAdminToken(t *testing.T) {
	if _, err := ParseAdminToken("   "); err == nil {
		t.Fatal("空的管理 key 要报错：那台机器就没有人能建档/删档/改 PIN")
	}
	at, err := ParseAdminToken("  topsecret  ")
	if err != nil {
		t.Fatal(err)
	}
	if at.Secret() != "topsecret" {
		t.Fatalf("修剪后存: %q", at.Secret())
	}
}

func TestAdminTokenMatches(t *testing.T) {
	at, _ := ParseAdminToken("topsecret")
	if !at.Matches("topsecret") {
		t.Fatal("本该匹配")
	}
	for _, bad := range []string{"", "TOPsecret", "topsecre", "topsecret ", "x"} {
		if at.Matches(bad) {
			t.Fatalf("%q 不该匹配", bad)
		}
	}
	// 未配置的那一把（零值）绝不能"匹配任何东西"——那是把没配管理员变成
	// 全员管理员。
	var empty AdminToken
	if empty.Matches("") || empty.Matches("anything") {
		t.Fatal("零值 AdminToken 什么都不该放行")
	}
}

// cookie 里装的是**派生值**，不是 key 本身。这条是「原始密钥不进浏览器」的
// 唯一证人：把 DeriveCookie 的返回值拿去当 Authorization 的 Bearer，必须匹配不上。
func TestAdminTokenCookie装的是派生值(t *testing.T) {
	at, _ := ParseAdminToken("topsecret")
	derived := at.DeriveCookie()
	if derived == "" || derived == "topsecret" {
		t.Fatalf("派生值不该等于原始 key: %q", derived)
	}
	if at.Matches(derived) {
		t.Fatal("派生值被当成令牌放行了——那等于把 cookie 直接变成 Bearer")
	}
	if !at.MatchesCookie(derived) {
		t.Fatal("本该认自己的派生值")
	}
	for _, bad := range []string{"", "topsecret", derived + "x", derived[:len(derived)-1], "ADMIN"} {
		if at.MatchesCookie(bad) {
			t.Fatalf("%q 不该通过 cookie 判据", bad)
		}
	}
}

func TestAdminToken派生值随key而变(t *testing.T) {
	a, _ := ParseAdminToken("one")
	b, _ := ParseAdminToken("two")
	if a.DeriveCookie() == b.DeriveCookie() {
		t.Fatal("两把不同的 key 派生出同一个 cookie 值：换 key 不会把旧 cookie 作废")
	}
	if a.MatchesCookie(b.DeriveCookie()) {
		t.Fatal("一把 key 该认不出另一把的派生值")
	}
}
