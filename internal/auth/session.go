package auth

import (
	"net/http"

	"github.com/rockbenben/launchbay/internal/model"
)

const (
	CookieProfile = "lb_profile"
	// CookieAdmin 装的是**派生值**（HMAC，见 AdminToken.DeriveCookie），不是
	// LAUNCHBAY_ADMIN_TOKEN 本身。浏览器那边的管理员身份与档案会话是**两件事**：
	// 管理员可能同时登着自己的档案在用书签，也可能只拿着管理权没登任何档案。
	CookieAdmin = "lb_admin"
	// CookieHome 是**升级前**那条 cookie 的名字。代管态 2026-10-10 整体下线，
	// 服务端从此不再发它；它留在代码里只有一个用途——认出"升级前发出去的代管
	// cookie"并判它非法（见 Session.Valid）。
	CookieHome          = "lb_home"
	SessionCookieMaxAge = 30 * 24 * 3600
)

// Session 只描述一件事：**当前在用哪个档案**。权限只看 Active，一条 cookie 就够。
//
// LegacyHome 是升级前那条 `lb_home` cookie 的值。它**不参与任何权限判定**，
// 只用来认出旧形状：代管那条 cookie 是自包含的（没有服务端会话表），读的人
// 只看到 `lb_profile=kid`，于是会被当成"kid 自己登录"——而"管理员代管别人的台"
// 这件事 2026-10-10 已经整条下线，那种 cookie 也不再合法。
//
// **读侧为什么不能干脆不看它**：不看，那枚 cookie 就重新变成后门（10-10 那次
// 改判恰恰是为了堵它；routes.go 里当时那条 `Home != Active` 的分支就是它）。
// 于是形状是"**只写一条、读两条只为拒旧形状**"：新登录的人不必被登出一次
// （旧的本人 cookie 两条相等，照样有效），旧代管 cookie 照旧进不来。
//
// 两条 cookie 都不签名——spec §11 明写不防局域网内冒充。
type Session struct {
	Active     string
	LegacyHome string
}

func (s Session) IsEmpty() bool { return s.Active == "" && s.LegacyHome == "" }

// Valid 是会话合法性的**唯一判据**（原来叫 sessionLegal，现在并回这里）。
//
// 三种形状：只有 lb_profile（今天的形状，合法）；两条相等（升级前的本人会话，
// **合法——升级不许把人踢下线**）；两条不等（升级前的代管会话，非法）。
//
// slug 判据只有 model.ValidSlug 一份（账本 T7→T9 承诺）：cookie 校验与档案 id
// 校验若各留一份字面量，漂移会静默不对称——磁盘上合法的 id 在 cookie 里被判
// 非法，或反之。
func (s Session) Valid() bool {
	if !model.ValidSlug(s.Active) {
		return false
	}
	return s.LegacyHome == "" || s.LegacyHome == s.Active
}

func ReadSession(r *http.Request) Session {
	return Session{Active: cookieOf(r, CookieProfile), LegacyHome: cookieOf(r, CookieHome)}
}

func cookieOf(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil || !model.ValidSlug(c.Value) {
		return ""
	}
	return c.Value
}

// ReadAdminCookie 取管理员 cookie 的**原始值**（派生值），合法性由
// AdminToken.MatchesCookie 判——判据只有那一份，这里不重复"看起来像不像"。
func ReadAdminCookie(r *http.Request) string {
	c, err := r.Cookie(CookieAdmin)
	if err != nil {
		return ""
	}
	return c.Value
}

// WriteAdminCookie 存派生值。**绝不发原始 key**：cookie 会进 devtools、
// 进浏览器导出、进同步，而派生值即使泄露也只等于一把管理员 key——与原始
// key 泄露的后果一样，但至少原始那句不会被复制出去。
func WriteAdminCookie(w http.ResponseWriter, derived string) {
	setCookie(w, CookieAdmin, derived, SessionCookieMaxAge)
}

// ClearAdminCookie 登出管理身份。
func ClearAdminCookie(w http.ResponseWriter) {
	setCookie(w, CookieAdmin, "", -1)
}

// WriteSession 发会话 cookie：**只有一条**。
//
// 顺带把升级前那条 `lb_home` 作废掉（maxAge=-1）：于是老浏览器登录一次就干净，
// 而"一条服务端不再使用的 cookie 赖在浏览器里"正是那种没人记得为什么会存在的
// 残留。它与 Valid() 里那道旧形状闸不冲突——被作废的这条下次请求就不存在了。
func WriteSession(w http.ResponseWriter, active string) {
	setCookie(w, CookieHome, "", -1)
	setCookie(w, CookieProfile, active, SessionCookieMaxAge)
}

// ClearSession 登出：两条都清。`lb_home` 那条今天已经不再被发出去，清它是为了
// 让还留着旧 cookie 的浏览器在登出时顺手干净。
func ClearSession(w http.ResponseWriter) {
	setCookie(w, CookieHome, "", -1)
	setCookie(w, CookieProfile, "", -1)
}

func setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
