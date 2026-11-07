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

// PreferredLinkMode keeps existing profile and backup defaults for browsers
// which have never chosen a mode. An explicit Auto also overrides that default.
func (p *Profile) PreferredLinkMode(browser LinkMode) LinkMode {
	if browser != "" && ValidLinkMode(string(browser)) {
		return browser
	}
	if p.LinkMode == LinkLAN || p.LinkMode == LinkWAN {
		return p.LinkMode
	}
	return LinkAuto
}

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

func EffectiveURL(it Item, mode LinkMode) string {
	if mode == LinkWAN && it.URLs != nil && CheckWanURL(it.URLs.WAN) == nil {
		return it.URLs.WAN
	}
	return it.URL
}

// HostOnly preserves IPv6 brackets and rejects malformed host/port combinations.
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

type LANShape struct {
	Mode LinkMode
	Rule string
}

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
