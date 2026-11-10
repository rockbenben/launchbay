package model

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strings"
)

type LinkMode string

const (
	LinkAuto LinkMode = "auto"
	LinkLAN  LinkMode = "lan"
	LinkWAN  LinkMode = "wan"
)

var LinkModeValues = []string{string(LinkAuto), string(LinkLAN), string(LinkWAN)}

func ValidLinkMode(s string) bool {
	return s == "" || slices.Contains(LinkModeValues, s)
}

// PreferredLinkMode 计算当前浏览器生效的地址模式（2026-11-08）：
// 各浏览器的偏好保存在本地 cookie（lb_link_<id>）中，优先级最高；
// 若该浏览器未曾选择或已清除，则继承档案级默认（Profile.LinkMode）；
// 若档案亦未配置，则回退为 auto（随访问入口自动判定）。
// 显式选 auto 同样会压过档案默认。
func (p *Profile) PreferredLinkMode(browser LinkMode) LinkMode {
	if browser != "" && ValidLinkMode(string(browser)) {
		return browser
	}
	if p.LinkMode == LinkLAN || p.LinkMode == LinkWAN {
		return p.LinkMode
	}
	return LinkAuto
}

// CheckWanURL 校验条目的外网覆盖地址（urls/wan）：
// 必须是含合法主机名的 http/https 地址，主机名必须是 ASCII（国际域名使用 punycode），
// 且必须符合 HostOnly 格式与全局 CheckURL 安全检查。
func CheckWanURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("urls/wan 必须是含主机名的 http/https 地址")
	}
	for _, r := range u.Host {
		if r > 127 {
			return fmt.Errorf("urls/wan 主机名必须是 ASCII（国际域名请用 punycode）")
		}
	}
	if HostOnly(u.Host) == "" {
		return fmt.Errorf("urls/wan 主机名或端口格式不对")
	}
	if err := CheckURL(raw); err != nil {
		return fmt.Errorf("urls/wan: %w", err)
	}
	return nil
}

// EffectiveURL 获取某条书签在当前链接模式下的生效地址：
// 仅在明确处于 WAN 模式且配置了有效 urls.wan 时返回外网地址，其余情况一律使用主地址 it.URL。
func EffectiveURL(it Item, mode LinkMode) string {
	if mode == LinkWAN && it.URLs != nil && CheckWanURL(it.URLs.WAN) == nil {
		return it.URLs.WAN
	}
	return it.URL
}

// HostOnly 提取并规范化纯主机名（保留 IPv6 的括号格式 [::1]，剥离端口号）：
// 严格校验端口纯数字与字符合法性，防止畸形 host/port 组合注入 bookmarklet 或破坏页面渲染。
func HostOnly(raw string) string {
	host := raw
	if strings.HasPrefix(raw, "[") || strings.Contains(raw, ":") {
		if strings.HasSuffix(raw, "]") {
			host = raw
		} else {
			h, port, err := net.SplitHostPort(raw)
			if err != nil {
				return ""
			}
			for _, r := range port {
				if r < '0' || r > '9' {
					return ""
				}
			}
			host = h
			if strings.Contains(h, ":") {
				host = "[" + h + "]"
			}
		}
	}
	if strings.HasPrefix(host, "[") {
		// netip accepts arbitrary zone identifiers; Host also enters bookmarklet JavaScript.
		for _, r := range host {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("[]:.%_-", r)) {
				return ""
			}
		}
		if _, err := netip.ParseAddr(host[1 : len(host)-1]); err != nil {
			return ""
		}
		return host
	}
	if host == "" {
		return ""
	}
	for _, r := range host {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_') {
			return ""
		}
	}
	return host
}

// LANShape 描述当前请求地址解析得到的网络形态及其判定规则。
type LANShape struct {
	Mode LinkMode
	Rule string
}

// LANShapeOf 依据主机名或 IP 与局域网规则计算归属模式（LAN 或 WAN）：
// 依次匹配：
//  1. 空主机名回退 LAN（fallback）
//  2. 显式配置的站级内网主机名白名单（lanHosts）
//  3. IP 地址形态（环回 loopback、链路本地 link-local、私有 IPv4 private-ipv4、IPv6 ULA ula；其余为 wan-domain）
//  4. 单标签纯主机名（bare-host，如 nas、router）
//  5. 本地私有保留域名（local-domain，如 .home.arpa、.local、.localhost）
//
// 其余有效公网域名归为 wan-domain。
func LANShapeOf(raw string, lanHosts []string) LANShape {
	host := strings.ToLower(strings.TrimSuffix(HostOnly(raw), "."))
	if host == "" {
		return LANShape{LinkLAN, "fallback"}
	}
	for _, h := range lanHosts {
		if strings.EqualFold(host, strings.TrimSuffix(HostOnly(strings.TrimSpace(h)), ".")) {
			return LANShape{LinkLAN, "lan-host"}
		}
	}
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		ip = ip.Unmap()
		switch {
		case ip.IsLoopback():
			return LANShape{LinkLAN, "loopback"}
		case ip.IsLinkLocalUnicast():
			return LANShape{LinkLAN, "link-local"}
		case ip.IsPrivate() && ip.Is4():
			return LANShape{LinkLAN, "private-ipv4"}
		case ip.IsPrivate():
			return LANShape{LinkLAN, "ula"}
		default:
			return LANShape{LinkWAN, "wan-domain"}
		}
	}
	if !strings.Contains(host, ".") {
		return LANShape{LinkLAN, "bare-host"}
	}
	if host == "home.arpa" || strings.HasSuffix(host, ".home.arpa") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") {
		return LANShape{LinkLAN, "local-domain"}
	}
	return LANShape{LinkWAN, "wan-domain"}
}
