package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

// Scope 是一枚**档案作用域**的写令牌，能读能写且只限一个档案。
//
// 管理员**不在这个列表里**（2026-10-12）：它是一把机器级的裸 key，见 AdminToken。
// 早先管理员是"档案段写 `*` 的那一条"，于是 Scope 得为一个跟档案毫无关系的值
// 开特例——`IsAdmin`、`Allows` 各一个星号分支、解析时给 `*` 跳过 slug 校验。
// 管理员本来就不属于任何档案，把那条特例从结构里拿掉比留着更省事。
type Scope struct {
	Name    string
	Secret  string
	Profile string
}

// Allows 只回答"这一枚能不能动这个档案"。跨档案是 403，没有第二个含义——
// 管理员令牌根本走不到这里（它在 require() 里先被分走，且只放行 credAdmin）。
func (s Scope) Allows(profileID string) bool { return s.Profile == profileID }

type Scopes []Scope

// ParseScopes 吃 LAUNCHBAY_WRITE_TOKENS 的值：name:secret:profile 逗号分隔。
// 段数不对、任一段为空、条目间多一个逗号、profile 不是合法 slug、或两个条目
// 共用同一个 secret，都算配置错误——最后一种会让人没法单独吊销某一端。
//
// **错误消息一律不回显密钥段**（2026-10-19）：这些错误经 `log.Fatalf` 进 stderr，
// 而 stderr 在容器里就是 `docker logs`。写错一个字符是常事，而从前 `%q` 打整条
// 令牌，于是那串密钥跟着进了日志——配置里写错能改，日志里出现就改不回来了。
// 形状由 `redacted` 那一处掌握，名字与档案段留着（那是操作员定位"哪一条写错了"
// 唯一要用的东西）。
//
// 档案段写 `*` 是**明确报错**而不是静默忽略：那是从 2026-10-12 之前的写法沿用下来
// 的配置，报错里直接指出去哪儿（LAUNCHBAY_ADMIN_TOKEN），省得升级后管理员令牌
// 静默失效、表现为"建档接口永远 403"。
//
// **空值不是错误，返回一个空列表**（2026-10-12）。这台机器的大多数用法根本用不到
// 档案作用域令牌——书签页、管理页、回退页都走会话 cookie，要令牌的是浏览器扩展、
// CLI 与 AI 脚本，而那三样本来就要人主动去装。硬性要求一个环境变量等于让每个
// 装机的人都得理解它、填它、然后把它留成一个公开的默认值——那份默认值恰好能写
// 那个最常见的档案 id（`dad`），是白送的一把钥匙。
//
// 空列表不是"配置坏了"：拿令牌打端点仍然是 401（没有可认的凭证），扩展与 CLI
// 则各自报"配置里没有作用域为 X 的令牌"并说清要补什么。
// redacted 把一条令牌回显成**不含密钥**的形状：名字与档案段原样，密钥段写成 `***`。
//
// **为什么必须有它**（2026-10-19）：这些解析错误会经 `log.Fatalf` 进 stderr，
// 而 stderr 在容器里就是 `docker logs`。操作员把环境变量写错一个字符是常事，
// 而从前的 `%q` 把**整串密钥**打进了日志——从此它躺在日志系统里、被复制进
// 工单、被备份走。密钥在配置里是"写错就修"，进了日志就是"已经在别处了"。
//
// 段数不对时（不是三段）更要小心：那时候第二段**可能就是密钥**，也可能是
// 档案段——分不清，所以**除第一段外全部抹掉**。名字是第一段，它不是秘密，
// 而它是操作员定位"哪一条写错了"唯一要用的东西。
func redacted(parts []string) string {
	if len(parts) == 0 {
		return "***"
	}
	out := []string{parts[0]}
	for i := 1; i < len(parts); i++ {
		out = append(out, "***")
	}
	return strings.Join(out, ":")
}

func ParseScopes(envValue string) (Scopes, error) {
	if strings.TrimSpace(envValue) == "" {
		return nil, nil
	}
	out := Scopes{}
	seen := map[string]string{}
	for i, raw := range strings.Split(envValue, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			// **不回显整条列表，也指出是哪一条**（2026-10-19 修）：它整条都是密钥，
			// 而"第几条"是操作员定位唯一要用的信息。名字那一段也留着。
			return nil, fmt.Errorf("令牌列表的第 %d 条是空的（前面多了一个逗号）", i+1)
		}
		parts := strings.Split(raw, ":")
		// 从这一条起，错误消息里**一律不回显密钥段**（2026-10-19 修）。
		//
		// 这条纪律不是洁癖：这些错误经 `log.Fatalf` 进 **stderr**，而 stderr
		// 在容器里就是 `docker logs` 能读到的内容。操作员把 `LAUNCHBAY_WRITE_TOKENS`
		// 写错一个字符是常事，而那条错消息会把**整串密钥**打进日志——从此它躺在
		// 日志系统里、被人复制粘贴进工单、被备份走。密钥在配置里是"写错就修"，
		// 进了日志就是"已经在别处了"。
		//
		// 形状是"名字 + 档案段"（这两段都不是秘密），密钥段一律写作 `***`。
		// 名字与档案段已经足够定位是哪一条写错了。
		if len(parts) != 3 {
			return nil, fmt.Errorf("令牌 %s 应为 name:secret:profile 三段（密钥段不回显）", redacted(parts))
		}
		for i, p := range parts {
			if strings.TrimSpace(p) == "" {
				return nil, fmt.Errorf("令牌 %s 的第 %d 段为空（密钥段不回显）", redacted(parts), i+1)
			}
		}
		// 修剪判空就必须修剪后存入：存 " xxx" 的话，操作员在 env 里
		// 多打一个空格，这条令牌永远匹配不上——正是要防的同一种"永远 401"。
		name, secret, profile := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
		if profile == "*" {
			return nil, fmt.Errorf(
				"令牌 %s 的档案段是 * —— 管理员改用独立的一把 key（LAUNCHBAY_ADMIN_TOKEN），"+
					"这里是档案作用域令牌的列表，只放 name:secret:<档案 id>", redacted(parts))
		}
		if !model.ValidSlug(profile) {
			return nil, fmt.Errorf("令牌 %s 的档案段 %q 不是 ASCII slug", redacted(parts), profile)
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

// adminCookieDomain 是从 key 派生 cookie 值的域分隔串。写死一个版本号是为了以后
// 真要轮换时旧 cookie 会整体失效，而不是两种派生值互相认。
const adminCookieDomain = "lb-admin-v1"

// DefaultAdminToken 是 docker-compose.yml 里 LAUNCHBAY_ADMIN_TOKEN 的**默认值**。
//
// 它是**公开的**（仓库里写着，README 里也写着），所以它只当两件事用：
//   - 登录页与管理页把这句话写出来，好让装好机器的人**知道自己该输什么**、
//     也知道**这把钥匙人人都猜得到**（那正是要换掉它的理由）；
//   - IsDefault 判"这台机器现在用的还是不是它"——还在的话，管理页给一句
//     "建议换掉"的提醒。
//
// **当前配置的那一把一个字都不进页面**：那把 key 会进 devtools、进导出、
// 进浏览器同步，而"默认那把"是公开知识，印出来不泄露任何东西。
const DefaultAdminToken = "launchbay123"

// IsDefault 答"这台机器现在用的还是不是那个公开的默认值"。
//
// 它只用来给管理页一句提醒，**不参与任何鉴权**：鉴权只有 Matches 一份。
func (t AdminToken) IsDefault() bool {
	return t.secret != "" && t.secret == DefaultAdminToken
}

// AdminToken 是**机器级**管理员凭证：一把裸 key，来自 LAUNCHBAY_ADMIN_TOKEN。
//
// 它是这台机器唯一的"能建档、删档、给别人设 PIN、停用、看跨档案状态"的身份，
// 且**读不到任何人的书签**（2026-10-10 那条边界在这里继续成立，见 routes.go 的
// require）。它不再依附于任何档案，所以也不需要"档案 id"、不需要 `*`。
//
// 为什么浏览器那条路不是裸 key 而是派生值：cookie 会进 devtools、进导出、
// 进浏览器同步。存派生值的话，**原始 key 从不离开那一次 POST**。
// 派生是无状态的（HMAC，无服务端会话表），所以判据仍然只有一份。
type AdminToken struct {
	secret string
}

// ParseAdminToken 读 LAUNCHBAY_ADMIN_TOKEN。空值是**启动错误**而不是"这台机器
// 没有管理员"：后者的症状是所有人卡在建档页上、而日志一个字都不打。

// ErrNoAdminToken 是"这台机器没有管理员 key"这一件事的**唯一一份文本**：
// 服务端启动要报它（main.go 的 serve 分支），CLI 那一支却**不该**被它拦住
// （revert 与 import-bookmarks 用的是档案作用域令牌）。
var ErrNoAdminToken = errors.New("未配置 LAUNCHBAY_ADMIN_TOKEN：这台机器就没有人能建档、删档或给别人设 PIN")

func ParseAdminToken(envValue string) (AdminToken, error) {
	v := strings.TrimSpace(envValue)
	if v == "" {
		return AdminToken{}, ErrNoAdminToken
	}
	return AdminToken{secret: v}, nil
}

// Matches 比对**令牌本身**（Authorization: Bearer）。
func (t AdminToken) Matches(secret string) bool {
	if t.secret == "" || secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(t.secret), []byte(secret)) == 1
}

// DeriveCookie 是放进 lb_admin cookie 的值：HMAC(key, 域分隔串) 的十六进制。
func (t AdminToken) DeriveCookie() string {
	mac := hmac.New(sha256.New, []byte(t.secret))
	mac.Write([]byte(adminCookieDomain))
	return hex.EncodeToString(mac.Sum(nil))
}

// MatchesCookie 比对 cookie 里那个派生值。长度不等先挡掉——ConstantTimeCompare
// 遇到长度不同直接返回 0，不构成问题，但先判一次省得每次都进 HMAC 的分支。
func (t AdminToken) MatchesCookie(value string) bool {
	want := t.DeriveCookie()
	if len(want) != len(value) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(value)) == 1
}

// Secret 交出原始 key。只有 CLI 的 newAdminClient 要它——那条路是把 key 放进
// Authorization 头发给本机服务（spec §4.5），不是把它写进浏览器 cookie。
func (t AdminToken) Secret() string { return t.secret }
