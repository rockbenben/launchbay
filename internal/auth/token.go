package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

const ScopeAny = "*"

type Scope struct {
	Name    string
	Secret  string
	Profile string
}

func (s Scope) IsAdmin() bool { return s.Profile == ScopeAny }

func (s Scope) Allows(profileID string) bool {
	return s.Profile == ScopeAny || s.Profile == profileID
}

type Scopes []Scope

// ParseScopes 吃 LAUNCHBAY_WRITE_TOKENS 的值：name:secret:profile 逗号分隔。
// 段数不对、任一段为空、条目间多一个逗号、profile 不是合法 slug、或两个条目
// 共用同一个 secret，都算配置错误——最后一种会让人没法单独吊销某一端。
func ParseScopes(envValue string) (Scopes, error) {
	if strings.TrimSpace(envValue) == "" {
		return nil, errors.New("未配置任何令牌")
	}
	out := Scopes{}
	seen := map[string]string{}
	for _, raw := range strings.Split(envValue, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, fmt.Errorf("令牌列表 %q 有多余的逗号", envValue)
		}
		parts := strings.Split(raw, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("令牌 %q 应为 name:secret:profile 三段", raw)
		}
		for i, p := range parts {
			if strings.TrimSpace(p) == "" {
				return nil, fmt.Errorf("令牌 %q 的第 %d 段为空", raw, i+1)
			}
		}
		// 修剪判空就必须修剪后存入：存 " xxx" 的话，操作员在 env 里
		// 多打一个空格，这条令牌永远匹配不上——正是 §6.3 拒绝对坏
		// 条目"静默跳过"要防的同一种"永远 401"。
		name, secret, profile := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
		if profile != ScopeAny && !model.ValidSlug(profile) {
			return nil, fmt.Errorf("令牌 %q 的档案段 %q 不是 ASCII slug", raw, profile)
		}
		if prev, ok := seen[secret]; ok {
			return nil, fmt.Errorf("secret 同时用于 %q 和 %q，无法分别吊销", prev, name)
		}
		seen[secret] = name
		out = append(out, Scope{Name: name, Secret: secret, Profile: profile})
	}
	return out, nil
}

// Authorize 遍历全部条目做常量时间比较，不提前返回。
// 这在局域网里谈不上防护，但成本几乎为零。
func (l Scopes) Authorize(secret string) (Scope, bool) {
	if secret == "" {
		return Scope{}, false
	}
	var hit Scope
	found := 0
	for _, s := range l {
		if subtle.ConstantTimeCompare([]byte(s.Secret), []byte(secret)) == 1 {
			hit = s
			found++
		}
	}
	if found != 1 {
		return Scope{}, false
	}
	return hit, true
}
