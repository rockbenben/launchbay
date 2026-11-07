package httpapi

import (
	"net/http"

	"github.com/rockbenben/launchbay/internal/model"
)

func browserLinkMode(r *http.Request, id string) model.LinkMode {
	c, err := r.Cookie("lb_link_" + id)
	if err != nil || !model.ValidLinkMode(c.Value) {
		return ""
	}
	return model.LinkMode(c.Value)
}

func setBrowserLinkMode(w http.ResponseWriter, r *http.Request, id, mode string) {
	http.SetCookie(w, &http.Cookie{
		Name: "lb_link_" + id, Value: mode, Path: "/", MaxAge: 365 * 24 * 60 * 60,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil,
	})
}

func linkShapeOf(r *http.Request, p *model.Profile, hosts []string) model.LANShape {
	if p == nil {
		return model.LANShapeOf(r.Host, hosts)
	}
	browser := browserLinkMode(r, p.ID)
	if mode := p.PreferredLinkMode(browser); mode != model.LinkAuto {
		if browser == "" {
			return model.LANShape{Mode: mode, Rule: "profile-default"}
		}
		return model.LANShape{Mode: mode, Rule: "manual"}
	}
	return model.LANShapeOf(r.Host, hosts)
}
