package httpapi

import (
	"net/http"
	"net/netip"

	"github.com/rockbenben/launchbay/internal/model"
)

func (s *Server) linkShapeOf(r *http.Request, p *model.Profile, hosts, cidrs []string) model.LANShape {
	if p != nil && (p.LinkMode == model.LinkLAN || p.LinkMode == model.LinkWAN) {
		return model.LANShape{Mode: p.LinkMode, Rule: "manual"}
	}
	shape := model.LANShapeOf(r.Host, hosts)
	if shape.Rule != "fallback" {
		return shape
	}
	if addr, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		for _, raw := range cidrs {
			if prefix, err := netip.ParsePrefix(raw); err == nil && prefix.Contains(addr.Addr()) {
				return model.LANShape{Mode: model.LinkLAN, Rule: "lan-cidr"}
			}
		}
	}
	return shape
}
