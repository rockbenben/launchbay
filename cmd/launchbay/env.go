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
