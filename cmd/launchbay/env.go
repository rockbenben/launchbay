package main

import (
	"fmt"
	"os"

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
	admin, err := auth.ParseAdminToken(os.Getenv("LAUNCHBAY_ADMIN_TOKEN"))
	if err != nil {
		return config{}, fmt.Errorf("LAUNCHBAY_ADMIN_TOKEN: %w", err)
	}
	c.AdminToken = admin
	return c, nil
}

func getenvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
