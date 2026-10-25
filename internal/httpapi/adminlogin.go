package httpapi

import (
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/auth"
)

// 管理页的登录/登出（2026-10-12）。
//
// 管理员不再是某个档案，于是浏览器那条路也需要**自己的**凭证：以前是"登进那个
// 管理员档案"，现在是"输一次这台机器的 key"。服务端不存 key——cookie 里放的是
// 它的 HMAC 派生值（见 auth.AdminToken），原始 key 只在那一次 POST 里出现过。
//
// 这两条是 credNone 而不是 credAdmin：**没管理权不是权限类拒绝**，那正是这一页
// 要处理的形状。加了 require(credAdmin) 的话，登录页自己就永远进不去。

// handleAdminLogin 是 `POST /admin/login`：表单通路，`{key}`。
//
// 判据只有 auth.AdminToken.Matches 一份，**不回落到任何档案**：key 错了就是
// 错了，不试别人的 PIN、不查磁盘上的 admin 标记（那个属性已经不存在了）。
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		// 表单读不出来与 key 不对是**两个形状**：前者是 422（字段级），后者是
		// 303 + ?err=badkey。把它们合成一个 303 会让人以为 key 写错了，
		// 而真正的原因是请求体坏了。
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	// **key 只从请求体里读**（2026-10-23 修）：`r.FormValue` 是**查询串与表单体
	// 的合并值**，所以 `POST /admin/login?key=<那把 key>`（body 空）也算数——
	// 而那把 key 于是进了 URL：浏览器历史、访问日志、Referer 头都留着它。
	// 这一条存在的全部理由就是"原始 key 从不离开那一次 POST"（见 auth.AdminToken），
	// 于是它只能走请求体。
	//
	// 同名多值照旧拒（"没说要哪一个"不是"取第一个"），而它查的是**请求体**那一份
	// ——查询串里那个 key 现在压根不算数。
	if n := len(r.PostForm["key"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "key", Reason: "同一个字段只能出现一次"}})
		return
	}
	if !s.deps.AdminToken.Matches(strings.TrimSpace(r.PostFormValue("key"))) {
		// 不说"这把 key 是管理员的那把"——那等于把 key 的**身份**漏出去。
		// 只说不对，与 PIN 步骤对错都答同一句是同一条纪律。
		redirectAdminFail(w, "badkey")
		return
	}
	auth.WriteAdminCookie(w, s.deps.AdminToken.DeriveCookie())
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// handleAdminLogout 只清 lb_admin。**不动 lb_profile**：管理员完全可能同时登着
// 自己的档案在用书签，"退出管理"不该把他从自己的台子里踢出去——那两件事
// 2026-10-12 之后就再也分不开了。
func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearAdminCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleAdminLoginPage 是 `GET /admin` 在**没有管理权**时的那一支。
//
// 它与 handleAdminPage 的分界只有一条判据：hasAdminCookie。早先这里是
// `redirectPicker(w)`（303 回选择页），那是"权限类拒绝一律 303"那条纪律；
// 2026-10-12 之后那条形状**不再适用**——升级完没有任何人进得去管理页（档案
// 不再是管理员，而 key 还没输过），整个账号层面会锁死在一个没有人能开的页面上。
//
// 这不是一条枚举线索：管理页存不存在，README 与 compose 里都写着。
func (s *Server) handleAdminLoginPage(w http.ResponseWriter, r *http.Request) {
	page, err := s.deps.Render.AdminLoginPage(s.notice(r, r.URL.Query().Get("err")), s.adminOptions(), s.machineLang(r))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}
