package main

import (
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("LAUNCHBAY_PORT", "")
	t.Setenv("LAUNCHBAY_DATA_DIR", "")
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:zzz:*")
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
}

// 令牌配置写错要拒绝启动，而不是跑起来之后每个写请求都 401——
// 后者会让人以为是 PIN 的问题，白查半天。
func TestLoadConfigRejectsBadTokens(t *testing.T) {
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:zzz")
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for malformed tokens")
	}
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for empty tokens")
	}
}
