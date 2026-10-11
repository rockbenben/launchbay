package httpapi

import (
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// handleAdminLanguage 是管理页那个「界面语言」表单的落点：POST /admin/language。
//
// **为什么是站级而不是档案级**：语言是一家人共用的阅读偏好。放进 profile.json
// 等于把"改全机显示"的权力交给任何一份档案的所有者——那既是权限模型的破口，
// 也让"语言"从一个显示设置变成了需要仲裁的状态。落点是 data/settings.json
// （store.Language），删档不动它，/prefs 也管不到它。
//
// **为什么只收会话、带令牌一律 303**：与 handleAdminCreate 同一句。浏览器表单
// 通路不接受令牌做身份，`*` 令牌能过的那一族里不包含"改全机显示"这件事。
//
// **为什么不用两步确认**：/admin/pin 与 /admin/delete 那套是给"改坏了撤不回来"的
// 动作准备的。改语言随时改回来，且一个字节的档案数据都不动——为它加一道
// 确认页只会让人以为后果比实际重。
//
// **为什么不静默落默认**：提交一个不认识的语言却回一句成功，等于"我选了法语，
// 它没告诉我没生效"。那是本地语言包被改坏或手改配置抄错了的形状，得说清。
func (s *Server) handleAdminLanguage(w http.ResponseWriter, r *http.Request) {
	if bearer(r) != "" {
		redirectPicker(w)
		return
	}
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		redirectPicker(w)
		return
	}
	// 上限与同族端点一致：ParseForm 整段读 body，不套上限的话 1 MiB 是假话。
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	// 同名多值不是"取第一个"，是没说清要哪个（/prefs、/setup 同纪律）。
	if n := len(r.Form["language"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "language", Reason: "同一个字段只能出现一次"}})
		return
	}
	raw := strings.TrimSpace(r.FormValue("language"))
	if raw == "" {
		redirectAdminFail(w, "nolang")
		return
	}
	l := i18n.Lang(raw)
	if !l.Valid() {
		redirectAdminFail(w, "badlang")
		return
	}
	if err := s.deps.Store.SetLanguage(l); err != nil {
		// 落盘失败绝不说成功：语言没存下去，界面还是原来那一种。
		writeInternalError(w, err)
		return
	}
	// 渲染缓存带语言这一维（render 的 cacheKey.lang），所以不需要在这里清缓存：
	// 下一次渲染的键就变了，自动重画。
	w.Header().Set("Location", "/admin")
	w.WriteHeader(http.StatusSeeOther)
}

// t 是**生产用的通用查表**：按这台机器当前的语言翻一句话，可带占位符。
//
// 为什么由 httpapi 侧翻好再塞进 view，而不是让模板查表（模板里那六个页面已经是
// `{{.T .Explain}}` 的形状）：**有些句子要带运行时的值**——
// 「删掉这一份的 %d 条书签与 %d 份快照」里的数字是这一页刚算出来的。
// 模板查表拿不到这些值，而在这里组装就不用给 AdminAction 再加一列参数。
//
// 所以两处形状并存，各有理由：模板里的 chrome（语言无关的那些）走 view.T，
// 这一族（要带值）在 Go 侧查表。翻字只有 i18n.Load 这一份。
func (s *Server) t(key string, args ...any) string {
	return i18n.Load(s.deps.Store.Language()).T(key, args...)
}
