package auth

import "testing"

func TestParseScopes(t *testing.T) {
	scopes, err := ParseScopes("ai-dad:xxx:dad,ext-mom:yyy:mom,admin:zzz:*")
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 3 {
		t.Fatalf("got %d", len(scopes))
	}
	if scopes[2].Profile != ScopeAny || !scopes[2].IsAdmin() {
		t.Fatalf("admin scope 解析错: %+v", scopes[2])
	}
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
	scopes, _ := ParseScopes("ai-dad:xxx:dad,admin:zzz:*")
	if s, ok := scopes.Authorize("xxx"); !ok || s.Profile != "dad" || s.IsAdmin() {
		t.Fatalf("got %+v %v", s, ok)
	}
	if s, ok := scopes.Authorize("zzz"); !ok || !s.IsAdmin() {
		t.Fatalf("admin: %+v %v", s, ok)
	}
	for _, secret := range []string{"nope", "", "XXX", "xxx ", "x"} {
		if _, ok := scopes.Authorize(secret); ok {
			t.Fatalf("secret %q 不该通过", secret)
		}
	}
}

func TestScopeAllows(t *testing.T) {
	scopes, _ := ParseScopes("ai-dad:xxx:dad,admin:zzz:*")
	dad, _ := scopes.Authorize("xxx")
	admin, _ := scopes.Authorize("zzz")
	if !dad.Allows("dad") || dad.Allows("mom") {
		t.Fatalf("dad scope 判定错: %+v", dad)
	}
	if !admin.Allows("mom") || !admin.Allows("dad") {
		t.Fatalf("admin scope 应放行任何档案: %+v", admin)
	}
}
