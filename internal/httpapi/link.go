package httpapi

import (
	"net/http"

	"github.com/rockbenben/launchbay/internal/model"
)

// browserLinkMode 读取当前浏览器对该档案选择的地址模式（存放在本地 cookie 中，2026-11-08）。
// 仅作用于当前浏览器的本档案，不影响其他设备或他人。
func browserLinkMode(r *http.Request, id string) model.LinkMode {
	c, err := r.Cookie("lb_link_" + id)
	if err != nil || !model.ValidLinkMode(c.Value) {
		return ""
	}
	return model.LinkMode(c.Value)
}

// setBrowserLinkMode 将地址模式偏好写入当前浏览器的 lb_link_<id> cookie。
// 设定 1 年有效期，HttpOnly 且跟随同源策略。
func setBrowserLinkMode(w http.ResponseWriter, r *http.Request, id, mode string) {
	http.SetCookie(w, &http.Cookie{
		Name: "lb_link_" + id, Value: mode, Path: "/", MaxAge: 365 * 24 * 60 * 60,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil,
	})
}

// linkShapeOf 计算当前请求渲染档案页时生效的链接模式与规则：
// 优先按浏览器 cookie 与档案默认偏好判定；若为 auto 则按 Host 请求头与局域网规则计算。
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
