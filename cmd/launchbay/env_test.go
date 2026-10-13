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
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:zzz:*")
	_, err := loadConfig()
	if err == nil {
		t.Fatal("want error for a * profile scope in LAUNCHBAY_WRITE_TOKENS")
	}
	if !strings.Contains(err.Error(), "LAUNCHBAY_ADMIN_TOKEN") {
		t.Fatalf("报错该指出去哪儿改: %v", err)
	}
}

// 管理员 key 缺失**只拦服务端，不拦 CLI 子命令**（2026-10-14）。
//
// 以前 loadConfig 无条件要它，于是"我只想退一档"的人拿着 dad 的档案令牌，
// 被告知的是"这台机器就没有人能建档、删档或给别人设 PIN"——说的是另外三件事，
// 而那三件事他一件都没在做。判据挪进 serveNeedsAdmin 之后，这一族要钉两件事：
// CLI 那一边真的能走通，serve 那一边仍然起不来。
func Test管理员key只拦serve不拦CLI(t *testing.T) {
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "ai-dad:tok-dad:dad")
	for _, empty := range []string{"", "   "} {
		t.Setenv("LAUNCHBAY_ADMIN_TOKEN", empty)
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("缺管理员 key 不该让 CLI 子命令连配置都读不动（%q）：%v", empty, err)
		}
		if err := serveNeedsAdmin(cfg); err == nil {
			t.Fatal("服务端那一支该仍然把它当启动错误：缺了这台机器没人能管")
		} else if !strings.Contains(err.Error(), "LAUNCHBAY_ADMIN_TOKEN") {
			t.Errorf("报错该指出去哪儿改：%v", err)
		}
	}
	t.Setenv("LAUNCHBAY_ADMIN_TOKEN", "tok-admin")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveNeedsAdmin(cfg); err != nil {
		t.Errorf("配了 key 之后 serve 不该再拦：%v", err)
	}
}

// **空令牌列表是合法配置**（2026-10-12）。书签页、管理页、回退、导入全都走会话
// cookie，一个档案作用域令牌都不需要；硬性要求那个环境变量等于让每个装机的人都得
// 填它，然后把它留成一个公开的默认值——那份默认值恰好能写最常见的那个档案 id。
//
// 这一格钉的是"它真的能起来"，而上面那条钉的是"管理员缺了起不来"：两者一起，
// 必填与可选的边界才不会被下一次改动悄悄挪动。
func TestLoadConfig令牌列表可以为空(t *testing.T) {
	t.Setenv("LAUNCHBAY_ADMIN_TOKEN", "tok-admin")
	for _, empty := range []string{"", "   "} {
		t.Setenv("LAUNCHBAY_WRITE_TOKENS", empty)
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("空令牌列表不该是错误（%q）：%v", empty, err)
		}
		if len(cfg.Tokens) != 0 {
			t.Errorf("空值应给出空列表，得到 %+v", cfg.Tokens)
		}
		if cfg.AdminToken.Secret() != "tok-admin" {
			t.Errorf("管理员那把不受影响：%q", cfg.AdminToken.Secret())
		}
	}
}
