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

// handleSetupLanguage 是**引导页上那个语言下拉自己的通路**：POST /lang。
//
// 它存在是因为一个具体的失败形状（2026-10-12 实测）：下拉原本在
// `POST /setup` 那张表单里，于是"改语言"这个动作的生效时机是**按另一个按钮**。
// 人拨了下拉、页面纹丝不动、连下拉自己的标签都还是原来那种语言——
// 看上去就是坏的。而它恰恰是**新装机的人看到的第一块屏幕**。
//
// 所以这里给它一张自己的表单与自己的按钮：按下去，页面用新语言重画，
// 看得见。零 JavaScript——这一族页面的判据是关掉脚本一样好用。
//
// **只在零档案时开放**（判据与 /setup 同一份 `CountProfiles`）：语言是站级设置，
// 装完之后只有管理员能改（`POST /admin/language`）。零档案是装机窗口，那一刻
// 这台机器还没有任何档案，也就没有"谁该替所有人选语言"这个问题——与
// /setup 能顺手选语言是同一条理由。
//
// `/setup` 的 `language?` 字段**继续收**：它是"一次填完"的形状，脚本与
// curl 仍然可以一趟发完。两个入口写的是同一个 `store.SetLanguage`，判据只有一份。
func (s *Server) handleSetupLanguage(w http.ResponseWriter, r *http.Request) {
	if bearer(r) != "" {
		redirectSetup(w, "permission")
		return
	}
	n, err := s.deps.Store.CountProfiles()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if n != 0 {
		// 装机窗口已关：与 /setup 回 `/` 不带键同一形状——"这台机器已经建过档了"
		// 本身就是那条枚举线索（同 /switch 的判据口径）。走 redirectPicker 而不是
		// redirectSetup(w, "")：后者那个空键不在白名单里，而「不带键」这句话
		// 是 redirectPicker 本身给的，不该再经 ?err= 走一圈。
		redirectPicker(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	if k := len(r.Form["language"]); k > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "language", Reason: "同一个字段只能出现一次"}})
		return
	}
	raw := strings.TrimSpace(r.FormValue("language"))
	if raw == "" {
		redirectSetup(w, "nolang")
		return
	}
	l := i18n.Lang(raw)
	if !l.Valid() {
		redirectSetup(w, "badlang")
		return
	}
	if err := s.deps.Store.SetLanguage(l); err != nil {
		// 落盘失败绝不说成功：界面还是原来那一种语言，而用户以为自己改了。
		writeInternalError(w, err)
		return
	}
	// 回 `/` —— 零档案时那就是引导页本身，所以"回到刚才那一页"与
	// "换成新语言重画"是同一个跳转。
	http.Redirect(w, r, "/", http.StatusSeeOther)
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

// browserLang 读 `Accept-Language` 里**第一个**能认出来的语言。
//
// 为什么用这个头而不是给接口加一个 `?lang=` 字段：浏览器发请求时**一定**带着它，
// 而脚本（curl、Go 的 http.Client、这台机器自己的 CLI）默认不发——"没有这个头"
// 就是"调用方不是浏览器"。于是那份逐字不变的中文报告继续是脚本的锚点，一个字都
// 不用改，也不需要谁去申报什么、申报错了再没人管。
//
// 只看第一个标签，不解析 q 值：`de,en;q=0.9` 落默认（中文）而不是猜"他也会英文"。
// 认不出来的一律落默认，**不 406**——一份显示层的兜底不该让一次导入停下来。
func browserLang(h string) i18n.Lang {
	first := strings.TrimSpace(strings.SplitN(h, ",", 2)[0])
	tag := strings.ToLower(strings.TrimSpace(strings.SplitN(first, "-", 2)[0]))
	for _, l := range i18n.All {
		if string(l) == tag {
			return l
		}
	}
	return i18n.Default
}

// apiReportWords：`POST /api/v1/import` 那份报告的句子从哪一本语言里查。
//
// 中文那一份是**默认**而不是"翻了一遍得到中文"：不注入 Words 时 Report 自己查
// zh 那本 catalog，所以脚本那一路的字节与今天完全一致——CLI 与 curl 都不发
// Accept-Language，这个判据对它们就是"没有"。
//
// 换语言时换的是**调用方浏览器的界面语言**，不是这台机器的站级语言：扩展读不到
// 站级设置（那要一次带凭证的请求），而它后面坐着的正是那台浏览器的主人。
// 与 `extension/_locales` 同一判据（AGENTS.md「界面语言」那一节末段）。
func (s *Server) apiReportWords(r *http.Request) func(string, ...any) string {
	lang := browserLang(r.Header.Get("Accept-Language"))
	if lang == i18n.Default {
		return nil
	}
	return i18n.Load(lang).T
}
