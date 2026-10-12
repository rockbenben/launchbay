package main

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
)

// mustAdminToken 是那把机器级管理员 key 的夹具（2026-10-12：管理员不再是
// "档案段写 * 的那一条令牌"，而是独立的 LAUNCHBAY_ADMIN_TOKEN）。
//
// 原来的 mustScopes（LAUNCHBAY_WRITE_TOKENS 的夹具）随这两条测试一起删掉了：
// newAdminClient 不再吃 auth.Scopes，那个列表里从此只有档案作用域令牌，
// 留着它就是一个没人用的 helper。档案作用域那条路（newScopedClient）由
// cli_test.go 的 revert 端到端覆盖。
func mustAdminToken(t *testing.T, secret string) auth.AdminToken {
	t.Helper()
	tok, err := auth.ParseAdminToken(secret)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// newAdminClient 拿的就是那把 key 本身：端口与 key 由调用方（loadConfig 的 cfg）
// 传入，这一步不读 env——选错凭证，后面每一次写都会打到错误的门。
//
// 原来这里断言的是"从令牌列表里挑出作用域为 * 的那一条"，而那个列表现在
// **只装档案作用域令牌**（ParseScopes 见到 * 直接报错），所以判据翻面成
// "配置里那把 key 原样变成 Authorization 头里的那串 secret"。
func TestNewAdminClientUsesTheAdminKey(t *testing.T) {
	c, err := newAdminClient("8600", mustAdminToken(t, "tok-admin"))
	if err != nil {
		t.Fatal(err)
	}
	if c.base != "http://127.0.0.1:8600" || c.token != "tok-admin" {
		t.Fatalf("该用那把管理员 key 并指向给定端口: %+v", c)
	}
	// 档案作用域令牌**不许**被误当成管理员凭证：新签名里它压根进不来（类型不同），
	// 这条钉的是"列表里只有 name:secret:<档案 id> 时不会有人手滑把它喂进来"。
	if _, err := auth.ParseScopes("ai-dad:tok-dad:dad,admin:tok-admin:*"); err == nil {
		t.Fatal("LAUNCHBAY_WRITE_TOKENS 里不该再有档案段为 * 的条目")
	}
}

// 配置里没有管理员 key 时必须给出可行动的失败，不许拿别的东西悄悄发出去
// （那会收到一个费解的 403）。零值 AdminToken（没配 LAUNCHBAY_ADMIN_TOKEN）
// 正是那条路。
func TestNewAdminClientNeedsAdminKey(t *testing.T) {
	_, err := newAdminClient("7600", auth.AdminToken{})
	if err == nil {
		t.Fatal("没配管理员 key 该报错退出")
	}
	if !strings.Contains(err.Error(), "LAUNCHBAY_ADMIN_TOKEN") {
		t.Fatalf("失败要说清缺哪个环境变量: %v", err)
	}
}
