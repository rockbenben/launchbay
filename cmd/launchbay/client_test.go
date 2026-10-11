package main

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
)

func mustScopes(t *testing.T, env string) auth.Scopes {
	t.Helper()
	scopes, err := auth.ParseScopes(env)
	if err != nil {
		t.Fatal(err)
	}
	return scopes
}

// newAdminClient 是 CLI 唯一挑凭证的地方：只认作用域为 * 的令牌。
// 端口与令牌由调用方（loadConfig 的 cfg）传入，这一步不读 env——
// 选错凭证，后面每一次写都会打到错误的人/错误的门。
func TestNewAdminClientPicksAdminScope(t *testing.T) {
	c, err := newAdminClient("8600", mustScopes(t, "ai-dad:tok-dad:dad,admin:tok-admin:*"))
	if err != nil {
		t.Fatal(err)
	}
	if c.base != "http://127.0.0.1:8600" || c.token != "tok-admin" {
		t.Fatalf("该挑管理员令牌并指向给定端口: %+v", c)
	}
}

// 配置里没有管理员令牌（没配令牌、或只配了档案作用域令牌）时必须给出
// 可行动的失败，不许拿档案作用域令牌悄悄发出去（那会收到一个费解的 403）。
func TestNewAdminClientNeedsAdminScope(t *testing.T) {
	for _, scopes := range []auth.Scopes{nil, mustScopes(t, "ai-dad:tok-dad:dad")} {
		_, err := newAdminClient("7600", scopes)
		if err == nil {
			t.Fatalf("scopes=%+v 该报错退出", scopes)
		}
		if !strings.Contains(err.Error(), "管理员令牌") {
			t.Fatalf("失败要说清缺什么: %v", err)
		}
	}
}
