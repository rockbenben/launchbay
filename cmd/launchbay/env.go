package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rockbenben/launchbay/internal/auth"
)

type config struct {
	Port       string
	DataDir    string
	Tokens     auth.Scopes
	AdminToken auth.AdminToken
}

func loadConfig() (config, error) {
	c := config{Port: getenvOr("LAUNCHBAY_PORT", "7600"), DataDir: getenvOr("LAUNCHBAY_DATA_DIR", "data")}
	// 档案作用域令牌是**可选**的（2026-10-12）：空值 = 这台机器没有给扩展/AI/CLI
	// 发写权限，服务照常起。管理员那把才是必填的——缺了它，建档与删档就没有人
	// 能做，而那是这台机器唯一一处"没有它就装不起来"的地方。
	scopes, err := auth.ParseScopes(os.Getenv("LAUNCHBAY_WRITE_TOKENS"))
	if err != nil {
		return config{}, fmt.Errorf("LAUNCHBAY_WRITE_TOKENS: %w", err)
	}
	c.Tokens = scopes
	// **空值是合法的**，但只对 CLI 子命令合法：`revert` 与 `import-bookmarks` 用的是
	// 档案作用域令牌（管理员那把在内容端点上恰恰是 403，AGENTS.md「令牌与鉴权」）。
	// 以前这里无条件要管理员 key，于是"我只想退一档"的人被告知的是
	// "这台机器就没有人能建档、删档或给别人设 PIN"——说的是另外三件事。
	// 服务端那一支仍然硬要：判据挪回 main.go 的 serve 分支，那儿才是"这台机器要能管"的地方。
	if raw := strings.TrimSpace(os.Getenv("LAUNCHBAY_ADMIN_TOKEN")); raw != "" {
		admin, err := auth.ParseAdminToken(raw)
		if err != nil {
			return config{}, fmt.Errorf("LAUNCHBAY_ADMIN_TOKEN: %w", err)
		}
		c.AdminToken = admin
	}
	return c, nil
}

func getenvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
