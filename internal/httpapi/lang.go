package httpapi

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/rockbenben/launchbay/internal/apierr"
	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
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
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	// 同名多值不是"取第一个"，是没说清要哪个（/prefs、/setup 同纪律）。
	if n := len(r.Form["language"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "language", Reason: "同一个字段只能出现一次"}})
		return
	}
	// **字段没给**与**给了一个空串**是两件事（2026-10-20 改）：空串是"跟随浏览器"
	// 那一档（管理员要把站级默认取消掉，回到"没人设过"），而字段没给是"这个请求
	// 没说语言"（脚本漏了它）。从前两者同形，于是"取消默认"这条路根本走不通——
	// 下拉里那一档按下去，服务端说的是"你没选"。
	vals, given := r.Form["language"]
	if !given {
		redirectAdminFail(w, "nolang")
		return
	}
	if len(vals) > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "language", Reason: "同一个字段只能出现一次"}})
		return
	}
	raw := strings.TrimSpace(vals[0])
	if raw == "" {
		// 取消站级默认 → 回到"没人设过"：各人按浏览器说的那一门显示。
		if err := s.deps.Store.ClearLanguage(); err != nil {
			// 落盘失败绝不说成功：语言没存下去，界面还是原来那一种。
			writeInternalError(w, err)
			return
		}
		w.Header().Set("Location", "/admin")
		w.WriteHeader(http.StatusSeeOther)
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
		// **权限类拒绝不带提示**（2026-10-20 修）：从前这里回
		// `redirectSetup(w, "permission")`，而 permission 那一句是
		// 「数据目录写不进去——请把数据目录交给跑它的那个用户（容器里是
		// uid 65534）」——它是**宿主数据目录不可写**的出路，与"你带令牌来了
		// 这条路不认令牌"毫无关系。给一个带 Bearer 的请求指去改宿主目录
		// 权限，比不说更坏。同一族端点全部用 redirectPicker（303 回选择页、
		// 不带键）。
		redirectPicker(w)
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
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	if k := len(r.Form["language"]); k > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "language", Reason: "同一个字段只能出现一次"}})
		return
	}
	// 字段没给 = 这个请求没说语言（脚本漏了）；给了空串 = **取消站级默认**
	// （回到"没人设过"，各人按浏览器说的那一门显示）。两个入口（这里与
	// /admin/language）同一口径。
	vals, given := r.Form["language"]
	if !given {
		redirectSetup(w, "nolang")
		return
	}
	if len(vals) > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "language", Reason: "同一个字段只能出现一次"}})
		return
	}
	raw := strings.TrimSpace(vals[0])
	if raw == "" {
		if err := s.deps.Store.ClearLanguage(); err != nil {
			writeInternalError(w, err)
			return
		}
		// 回 `/` —— 零档案时那就是引导页本身，所以"回到刚才那一页"与
		// "换成新语言重画"是同一个跳转。
		http.Redirect(w, r, "/", http.StatusSeeOther)
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

// machineLang 是"**没有个人选择时**"这一种语言：这一次请求该用哪一门。
//
// 链条只有这一处（2026-10-20），三格：
//
//  1. **站级的默认**——管理员在管理页设过的那一个（`store.DefaultLanguage`）。
//     他设过就是他说过了，后面的格子轮不到。
//  2. **浏览器语言**（`Accept-Language` 里认得的那一门）——"第一次使用、什么都
//     还没设过"时，人**正在读**的那一门语言就是最像他的那一门。机器刚装好、
//     还没人建过档，这一格才轮得到。
//  3. **en**——上面都答不出来时给英文（国际通用那一门），而不是中文。
//
// 为什么第 3 格是 en 而不是中文：中文是**这个项目原本的默认**（作者一家人的
// 语言），而这一格问的是"读者要哪一门"——**没有读者信息时**，国际通用那一门
// 比"作者家的那一门"更接近陌生人。而中文仍然是**管理员设的默认**与
// **没设过任何语言时新建的那一份**的落点（见下）。
//
// 判据与渲染层共用一处：这一格是 i18n.ProfileLang 的 fallback 那一半。
func (s *Server) machineLang(r *http.Request) i18n.Lang {
	if l, ok := s.deps.Store.DefaultLanguage(); ok {
		return l
	}
	// 请求可以是 nil（单元测试里直接问"这台机器现在是什么语言"）：那就没有
	// 浏览器说的那一门，于是落 en。
	if r != nil {
		if l, ok := browserLangOr(r.Header.Get("Accept-Language")); ok {
			return l
		}
	}
	return i18n.En
}

// langOf 拿"这一次请求、这一份档案"该用哪种语言。
//
// 链条最上面那一格是**这一份自己记的**（profile.json 的 language）——它最具体，
// 别人替它选的那一份不算数。没记（空串）就落到 machineLang 那三格。
func (s *Server) langOf(r *http.Request, profileID string) i18n.Lang {
	p, err := s.deps.Store.LoadProfile(profileID)
	if err != nil {
		return s.machineLang(r)
	}
	return s.langOfProfile(r, p)
}

// langOfProfile 同 langOf，但**档案已经读出来了**（省一次读盘）。
//
// 档案页那一头本来就要读它（判停用），顺手把语言一起定了。
//
// **判据是 i18n.ProfileLang 一处**（2026-10-20 修）：从前这里把
// "这一份自己记了没有语言"与"记了一个不认识的值"当成同一件事，于是
// 手改/拷贝来的 `"language":"zh-CN"`（或 "fr"）会把这一份**静默钉死在中文**——
// 既不跟站级默认，也不跟浏览器语言，链的后两格被完全跳过。而 i18n.ProfileLang
// 的新口径是"不认识的 = 没选过"（读侧宽、写侧严），空串与非法值同形。
func (s *Server) langOfProfile(r *http.Request, p *model.Profile) i18n.Lang {
	if p == nil {
		return s.machineLang(r)
	}
	return i18n.ProfileLang(p.Language, s.machineLang(r))
}

// t 是**站级（默认语言）的通用查表**：按这台机器当前的默认语言翻一句话，
// 可带占位符。
//
// 为什么由 httpapi 侧翻好再塞进 view，而不是让模板查表（模板里那六个页面已经是
// `{{.T .Explain}}` 的形状）：**有些句子要带运行时的值**——
// 「删掉这一份的 %d 条书签与 %d 份快照」里的数字是这一页刚算出来的。
// 模板查表拿不到这些值，而在这里组装就不用给 AdminAction 再加一列参数。
//
// 所以两处形状并存，各有理由：模板里的 chrome（语言无关的那些）走 view.T，
// 这一族（要带值）在 Go 侧查表。翻字只有 i18n.Load 这一份。
//
// **它说的是"站级那一种"**（2026-10-20 改判）：管理页、选择页、登录页与三张
// 确认页都是**机器层面**的页面，它们用默认语言。**档案自己那一份**的页面
// （档案页、台面面板、改动历史、回退、导入）用 trFor(r, id) 翻。
func (s *Server) t(r *http.Request, key string, args ...any) string {
	return i18n.Load(s.machineLang(r)).T(key, args...)
}

// tr 是一种语言下的翻译器。
//
// 它存在的理由（2026-10-20）：一台机器上现在**可能同时有好几种语言**——每一份
// 档案自己记一份，而站级那一份是默认。一个页面上有些句子来自模板的 chrome
// （渲染层按档案的语言查），有些句子来自 httpapi 侧组装的（带运行时值的那
// 些）。这两半必须是**同一种语言**，否则一页上会出现中英混排。
//
// 所以档案页那一族不用 s.t（那是站级的），改用 trFor(r, id) 拿一个按档案语言
// 翻话的翻译器；机器层面的页面继续用 s.t。
type tr struct{ cat *i18n.Catalog }

func (t tr) t(key string, args ...any) string { return t.cat.T(key, args...) }

// trFor 返回一个**按某一份档案**翻话的翻译器（这一次请求里）。
//
// 判据只有一处：i18n.ProfileLang——这一份自己记了语言就用它，没记就用
// machineLang 那三格（站级默认 → 浏览器语言 → en）。档案读不出来（坏档、
// 刚被删）→ 同 machineLang。
func (s *Server) trFor(r *http.Request, id string) tr {
	return tr{cat: i18n.Load(s.langOf(r, id))}
}

// notice 是 `?err=` 那个键翻成**站级那一种**语言的一句人话。
//
// 站级的那些页面用这一条（默认语言）；档案自己那几页用 noticeLang（那一份的
// 语言）。白名单与查表还是 noticeOf 那一份——这里只是把"用哪种语言"显式化。
func (s *Server) notice(r *http.Request, key string) string {
	return noticeOf(s.machineLang(r), key)
}

// noticeLang 把 `?err=` 那个键翻成**指定语言**的一句人话。
func (s *Server) noticeLang(lang i18n.Lang, key string) string {
	return noticeOf(lang, key)
}

// langValues 是"能选哪几种语言"的那份名单，给 422 的那句话用。
//
// 名单只有 i18n.All 一处（新增一种语言只改它），所以这句话不会与下拉框里的
// 那几档分家。
func langValues() string {
	out := make([]string, 0, len(i18n.All))
	for _, l := range i18n.All {
		out = append(out, string(l))
	}
	return strings.Join(out, "|")
}

// browserLang 读 `Accept-Language` 里**第一个**能认出来的语言。
//
// 为什么用这个头而不是给接口加一个 `?lang=` 字段：浏览器发请求时**一定**带着它，
// 而脚本（curl、Go 的 http.Client、这台机器自己的 CLI）默认不发——"没有这个头"
// 就是"调用方不是浏览器"。于是那份逐字不变的中文报告继续是脚本的锚点，一个字都
// 不用改，也不需要谁去申报什么、申报错了再没人管。
//
// 解析只有一份（browserLangOr 那一处，**q 值也看**了——2026-10-20 修：
// 从前只取第一个标签且不剥 `;q=`，于是 `zh;q=0.9` 整条认不出）。
//
// **注意这一份的兜底是中文**（那是导入报告的契约：脚本与 CLI 拿到的那一份
// 必须逐字不变）。"还没设过语言时给浏览器那一门、认不出给 en"是**另一条**
// 判据（browserLangOr 的调用方 machineLang），别把两份混成一份。
func browserLang(h string) i18n.Lang {
	l, _ := browserLangOr(h)
	if l == "" {
		return i18n.Default
	}
	return l
}

// browserLangOr 读 `Accept-Language` 里**认得的那一门**，认不出就说认不出
// （第二个返回值 false）。
//
// 它是"什么都还没设过"时那一条链的第三格（见 machineLang）：机器刚装好、
// 还没人建过档，**浏览器说的那一门**就是最像读者的一门；浏览器说的那一门我们
// 没有，就交给调用方（那一格落 en，而不是中文）。
//
// **权重（q 值）要看**（2026-10-20 修）。从前只取**第一个**标签且不剥
// `;q=…`，于是 `Accept-Language: zh;q=0.9` 整条认不出（切出来的是
// "zh;q=0.9"）——一个**说中文的客户端拿到英文首页**。而导入报告那一族
// （browserLang）落的是中文，于是说英文的扩展拿到中文报告。
//
// 现在的口径：
//   - 按 q 值从高到低找（q 相同按原文顺序，这是 RFC 的稳定排序）；
//   - `q=0` = 客户端**不接受**这一门，跳过（`en;q=0,zh;q=0.5` → zh）；
//   - `*` 是"其余都行"，不是"他要哪一门"，跳过；
//   - 区域码取主标签（`zh-CN` → `zh`）。
func browserLangOr(h string) (i18n.Lang, bool) {
	for _, pref := range parseAcceptLanguage(h) {
		if pref.tag == "*" || pref.q <= 0 {
			continue
		}
		primary := strings.ToLower(strings.TrimSpace(strings.SplitN(pref.tag, "-", 2)[0]))
		for _, l := range i18n.All {
			if string(l) == primary {
				return l, true
			}
		}
	}
	return "", false
}

// langPref 是 Accept-Language 里的一个偏好：标签与权重。
type langPref struct {
	tag string
	q   float64
}

// parseAcceptLanguage 把 Accept-Language 拆成偏好列表，**按 q 值从高到低**排。
//
// 排序用稳定排序（SliceStable）：q 相同按原文顺序——那是客户端自己写的
// 顺序，而"平手时按原文"是 RFC 的口径，也是人读那份头时的读法。
//
// q 解析不出来时按 1.0 算（客户端的首选），q>1 钳到 1。
func parseAcceptLanguage(h string) []langPref {
	var out []langPref
	for _, raw := range strings.Split(h, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, ";")
		tag := strings.TrimSpace(parts[0])
		if tag == "" {
			continue
		}
		q := 1.0
		for _, p := range parts[1:] {
			p = strings.TrimSpace(p)
			if !strings.HasPrefix(strings.ToLower(p), "q=") {
				continue
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(p[2:]), 64)
			if err != nil {
				continue // 权重写坏了：按 1.0 算（它是客户端的首选）
			}
			q = v
			break
		}
		if q > 1 {
			q = 1
		}
		out = append(out, langPref{tag: tag, q: q})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].q > out[j].q })
	return out
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

// wantsHTML 答的是"这一条是不是**人在浏览器里导航**过来的"。判据只有 Accept 里
// 有没有 text/html：浏览器发导航请求时一定带上它，而 curl、Go 的 http.Client
// 与扩展都不带（它们要的是机器可读的那一份，`*/*` 不算点名要 HTML）。
//
// 它只用来**选答法**，从来不用来选判据：状态码、kind 与权限判定两边完全同一份，
// 变的只是那一屏给人看的是花括号还是一句人话。
func wantsHTML(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}
