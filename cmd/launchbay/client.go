package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
)

// client 是 CLI 子命令唯一的写盘通路：一律经本机 REST。
//
// 为什么不在 CLI 里直接 import store 写文件：进程内互斥锁罩不住另一个
// 进程，CLI 绕过服务写盘 = 与正在跑的容器抢同一个文件（spec §4.5）。
// 服务没起来时这里就报错退出，**不留"离线直写"的兜底分支**。
type client struct {
	base  string
	token string
	hc    *http.Client
}

// newAdminClient 用那把机器级管理员 key 建客户端，base 指向给定端口。
// 它自己不读 env：端口与令牌的唯一来源是 loadConfig（env.go），main.go 把
// 同一份 cfg 交给 serve 和 CLI——再读一次 env 就是第二份真相。
//
// 它不再"从令牌列表里挑一枚作用域为 * 的"：管理员根本不在那个列表里，
// 是一把独立的 LAUNCHBAY_ADMIN_TOKEN（2026-10-12）。
func newAdminClient(port string, admin auth.AdminToken) (*client, error) {
	if admin.Secret() == "" {
		return nil, fmt.Errorf("配置里没有 LAUNCHBAY_ADMIN_TOKEN，CLI 子命令要它（spec §10）")
	}
	return &client{
		base:  "http://127.0.0.1:" + port,
		token: admin.Secret(),
		hc:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// newScopedClient 挑一枚**作用域正好是某个档案**的令牌（2026-10-10 改判）。
//
// 为什么管理员 key 不够用：它只管账号层面（建档/删档/改 PIN/status/审计聚合），
// 读写书签一律要档案作用域令牌。所以 CLI 里凡是碰别人书签的子命令（revert、
// import-bookmarks 的写入那一半）都必须逐档案换令牌——这就是"管理员写不了任何人的
// 书签"在机器那一面的样子。
//
// 报错的措辞要把出路说清楚：补一枚 `name:secret:<id>` 的令牌就行，
// 而不是让人对着"作用域不含该档案"去猜哪个令牌不对。
func newScopedClient(port string, scopes auth.Scopes, profile string) (*client, error) {
	for _, sc := range scopes {
		if sc.Profile == profile {
			return &client{
				base:  "http://127.0.0.1:" + port,
				token: sc.Secret,
				hc:    &http.Client{Timeout: 10 * time.Second},
			}, nil
		}
	}
	return nil, fmt.Errorf(
		"配置里没有作用域为 %s 的令牌。管理员 key 只管账号层面，"+
			"改这一份的书签要用一枚 name:secret:%s 的令牌（LAUNCHBAY_WRITE_TOKENS）",
		profile, profile)
}

func (c *client) do(method, path string, body []byte, hdr map[string]string) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("连不上 %s%s（服务起了吗）: %w", c.base, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("读响应: %w", err)
	}
	return raw, resp.StatusCode, nil
}

// mustOK 把非 2xx 变成带 detail 的错误：CLI 的失败要能说清是哪一格拒绝的，
// 而不是"退出码 1"。
func mustOK(raw []byte, code int, path string) error {
	if code >= 200 && code <= 299 {
		return nil
	}
	var e struct {
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(raw, &e)
	if e.Detail == "" {
		e.Detail = strings.TrimSpace(string(raw))
	}
	return fmt.Errorf("%s 返回 %d：%s", path, code, e.Detail)
}
