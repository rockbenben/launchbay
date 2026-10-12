package main

import (
	"strings"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("LAUNCHBAY_PORT", "")
	t.Setenv("LAUNCHBAY_DATA_DIR", "")
	// 2026-10-12：LAUNCHBAY_WRITE_TOKENS 现在**只装档案作用域令牌**，档案段写 *
	// 是配置错误（管理员改用 LAUNCHBAY_ADMIN_TOKEN）；而缺管理员 key 是启动错误，
	// 所以这里两格都得给。
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "ai-dad:tok-dad:dad")
	t.Setenv("LAUNCHBAY_ADMIN_TOKEN", "tok-admin")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "7600" {
		t.Fatalf("port = %q", cfg.Port)
	}
	if cfg.DataDir != "data" {
		t.Fatalf("dataDir = %q", cfg.DataDir)
	}
	if len(cfg.Tokens) != 1 || cfg.Tokens[0].Profile != "dad" {
		t.Fatalf("tokens = %+v", cfg.Tokens)
	}
	if cfg.AdminToken.Secret() != "tok-admin" {
		t.Fatalf("adminToken 没落进 cfg: %q", cfg.AdminToken.Secret())
	}
}

// 令牌配置写错要拒绝启动，而不是跑起来之后每个写请求都 401——
// 后者会让人以为是 PIN 的问题，白查半天。
func TestLoadConfigRejectsBadTokens(t *testing.T) {
	t.Setenv("LAUNCHBAY_ADMIN_TOKEN", "tok-admin")
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:zzz")
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for malformed tokens")
	}
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for empty tokens")
	}
	// 2026-10-12：还多一种形状——档案段写 * 的老配置。它是明确报错而不是静默忽略：
	// 静默的话升级后管理员令牌无声失效，症状是"建档接口永远 403"。
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:zzz:*")
	_, err := loadConfig()
	if err == nil {
		t.Fatal("want error for a * profile scope in LAUNCHBAY_WRITE_TOKENS")
	}
	if !strings.Contains(err.Error(), "LAUNCHBAY_ADMIN_TOKEN") {
		t.Fatalf("报错该指出去哪儿改: %v", err)
	}
	// 管理员 key 缺失同样是**启动错误**：症状是所有人卡在建档页而日志一个字不打。
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "ai-dad:tok-dad:dad")
	t.Setenv("LAUNCHBAY_ADMIN_TOKEN", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for missing LAUNCHBAY_ADMIN_TOKEN")
	}
}
