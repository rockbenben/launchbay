package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

const maxBodyBytes = 1 << 20 // 1 MiB：建档与 PIN 用不到更大

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

// isFormPost 认的是浏览器的默认提交编码：选择页每张卡片都是一个自包含
// 的 <form method=post action="/pin">，关掉 JS 就是 urlencoded。
// Content-Type 没给或给得别的，一律走 JSON 老路，行为逐字不变。
func isFormPost(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/x-www-form-urlencoded"
}

// pinNotices 是表单通路能说到人话的几种失败。?err= 只当键用：不在表里的
// 值一个字都不进页面——query 是用户可控的，原样渲染等于开一个反射点。
// pinNotices 是表单通路能说到人话的几种失败。?err= 只当键用：不在表里的
// 值一个字都不进页面——query 是用户可控的，原样渲染等于开一个反射点。
//
// **值是文案表里的键，不是句子本身**：这句话在两种语言里各有各的说法，
// 写死在这里就等于中文界面成立而英文界面没得翻。i18n 那一层的守恒测试
// 保证每把键在两种语言里都有一条对应的文案。
var pinNotices = map[string]string{
	"pin":     "notice.pin",
	"missing": "notice.missing",
	"profile": "notice.profile",
	// 停用（2026-10-10 第二轮改判）。从前这一支故意复用「PIN 不对」，理由是
	// "单独说已停用等于告诉摸到这台机器的人哪些档案被管理员动过"。那条理由在
	// 本案站不住：选择页本来就把所有档案名摊在屏幕上，§11 也明写不防局域网冒充，
	// 而它的代价落在最该被照顾的人身上——**输对了 PIN 却被叫去再试一次**，
	// 人会一直试下去。说真话且说得出下一步。
	"disabled": "notice.disabled",
	// 引导建档（/setup）的两种没填对。窗口已关**不在这里**：那一支回 `/` 不带键，
	// 因为"这台机器已经建过档了"本身就是枚举线索（设计 §3）。
	"noname": "notice.noname",
	"badpin": "notice.badpin",
	// 名字撞车只有管理页那条路会**发**出这个键：派生 id 在 store 锁外算，两笔并发同名
	// 才会撞进同一个空位（顺序提交永远拿到下一个空位，见 store.NextProfileID）；引导
	// 那条撞车等于"窗口已关"，回 / 不带键。这张表是两页共用的**词汇表**，不是发件人
	// 名单：手打的 `/?err=nametaken` 也会在进门页印出同一句话。那是有意的——那句是常量、
	// 不点名任何档案，而进门页本来就把所有档案名列在屏幕上，拆成两张表只会让词汇分家。
	"nametaken": "notice.nametaken",
	// 管理页那几条零 JS 写动作（设 PIN / 删档 / 停用，见 adminaction.go）的失败。
	// 与上面同一张表，同一条理由：这是**词汇表**，两个页面（/admin 与两个确认页）
	// 都用它把键翻成人话。分开两张表的话，同一个词在两处会有两种说法，
	// 而读日志的人无从知道哪一句才是它。
	"notfound": "notice.notfound",
	"selfpin":  "notice.selfpin",
	// 「不能动自己正在用的这一份」这一句被**删档与停用两处**共用，而两者后果
	// 完全不同（删=不可逆；停用=随手启回来）。从前统一写成"不能删掉"，于是
	// 管理员点「停用」时看到的是"不能删掉你正在用的这一份"。拆成两个键，各说各的事。
	"self":        "notice.self",
	"selfdisable": "notice.selfdisable",
	"failed":      "notice.failed",
	// 回退页（revertpage.go）的三把键。与上面同一张表：这张表是**词汇表**，
	// 谁发键它不关心，只负责把键翻成人话。三把各自对应一个不同的下一步，
	// 所以不合并——"没选"要重选，"已经不在了"要重新看列表，
	// "存不下"要告诉人**书签一个字都没变**（那一支最容易让人以为退了一半）。
	"badsnap": "notice.badsnap",
	"gone":    "notice.gone",
	"nosnap":  "notice.nosnap",
	// 宿主上的数据目录对跑服务的那个用户不可写（群晖/威联通上最常见：
	// ./data 是 root 所属，容器以 uid 65534 跑，落盘一律 EACCES）。这一族
	// 与上面几条不是一类东西——它是**运维装机的第一个故障**，而且用户能在
	// 宿主机上修好，所以要说得出出路。其余落盘失败（磁盘满、只读文件系统、
	// 校验不过）不给键：给不出能照着做的下一步时，句子只会把人引到错的地方。
	// 判据只有 errors.Is(fs.ErrPermission) 这一条，见 createProfile。
	"permission": "notice.permission",
	// 界面语言那两条（lang.go）。**不静默落默认**是它们存在的全部理由：
	// 提交一个不认识的语言却回一句成功，等于"我选了法语，它没告诉我没生效"。
	"nolang":  "notice.nolang",
	"badlang": "notice.badlang",
	// 引导建档那一条（setup.go）：**档案已经建成了**，只是语言没写进去。
	// 它必须说清这一半，否则人会以为什么都没建成，重新建一遍却撞上「窗口已关」。
	"langfail": "notice.langfail",
}

// noticeOf 把 ?err= 的键翻成这一页该说的话。**不在表里的键回空串**——
// query 是用户可控的，原样渲染等于开一个反射点（§6.1）。
//
// lang 显式传进来而不是自己去读 store，是为了让"用哪种语言翻"这件事
// 在调用点上看得见：生产代码传这台机器当前的语言，测试传 i18n.Default
// 去对照中文那句话。翻字只有这一份。
func noticeOf(lang i18n.Lang, key string) string {
	k, ok := pinNotices[key]
	if !ok {
		return ""
	}
	return i18n.Load(lang).T(k)
}

// notice 是生产用的那一层：按这台机器当前的语言翻。
func (s *Server) notice(key string) string {
	return noticeOf(s.deps.Store.Language(), key)
}

// redirectPinFail 把无 JS 的表单失败送回选择页。成功分支早就按 fromForm
// 分 303/JSON（Task 11），失败分支以前一律吐裸 JSON，等于把关掉脚本的家人
// 关在门外（部署验收 Important #2）。
func redirectPinFail(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/?err="+code, http.StatusSeeOther)
}

// redirectPicker 是浏览器通路的"拒"（spec §6.1）：303 回档案选择页、不报错、
// 不带 ?err=。它与 API 通路的 writeAuthFailure 是**两种长相、一套判据**——
// 判据永远在 require 那一份里，这里只改外壳。
//
// 为什么不复用 redirectPinFail：那条是给"输错了"用的（带 ?err= 才说得出人话），
// 权限类拒绝带提示反而是在告诉调用方"你认得的这个人不是管理员"。
func redirectPicker(w http.ResponseWriter) {
	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusSeeOther)
}

func (s *Server) handlePin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Profile string `json:"profile"`
		PIN     string `json:"pin"`
	}
	fromForm := isFormPost(r)
	if fromForm {
		// 表单分支也必须吃 maxBodyBytes：ParseForm 会整段读 body，
		// 不套 MaxBytesReader 的话注释里承诺的 1 MiB 上限对无 JS
		// 通路是假话（JSON 通路一直有 io.LimitReader 罩着）。
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := r.ParseForm(); err != nil {
			writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "body", Reason: "表单读不出来"}})
			return
		}
		in.Profile = r.FormValue("profile")
		in.PIN = r.FormValue("pin")
	} else if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if !model.ValidSlug(in.Profile) {
		if fromForm {
			redirectPinFail(w, r, "profile")
			return
		}
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "profile", Reason: "必须是 ASCII slug"}})
		return
	}
	target, err := s.deps.Store.LoadProfile(in.Profile)
	if errors.Is(err, store.ErrNotFound) {
		if fromForm {
			redirectPinFail(w, r, "profile")
			return
		}
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "档案不存在"})
		return
	}
	if errors.Is(err, store.ErrCorrupt) || errors.Is(err, model.ErrValidation) {
		// profile.json 坏了不是 500（评审 Important #4）：§12 要求它与
		// bookmarks.json 同等待遇——给家属看的解释页。真实原因早已由
		// LoadProfile 登记进 BrokenReason（文件名+位置），响应不带磁盘路径。
		reason, _ := s.deps.Store.BrokenReason(in.Profile)
		s.writeBrokenProfile(w, r, in.Profile, reason)
		return
	}
	if err != nil {
		// 其余读失败（I/O 等真内部错误）：500 保持，响应不回显内部错误
		// ——真实原因进 stderr。
		writeInternalError(w, err)
		return
	}

	if in.PIN == "" {
		// 空 PIN 一律拒（2026-10-10 改判）。原来这一支是"免 PIN 切档"：带着会话
		// 交一张空表单时，拿 home/active 那一套去放行——那正是代管态，而代管态
		// 已经不存在了。`/switch to=<id>` 与"只认合法的会话形状"两条一起把这条路
		// 走死了，于是**会话合法与否已经不影响这一支的答案**，再判它合法，只是让
		// 一个永远走不到的分支看起来像一条路。
		if fromForm {
			redirectPinFail(w, r, "missing")
			return
		}
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Detail: "缺 PIN"})
		return
	}

	// 停用的档案：PIN 输对也进不去（2026-10-10）。
	//
	// 从前这一支的答案与"PIN 不对"**逐字相同**（防枚举），2026-10-10 第二轮改判为
	// 说真话：见 pinNotices["disabled"] 那段理由。停用档案不进选择页（判据在
	// handleRoot 造那张卡片表的地方），所以家里不会看到一张点不进去的卡——
	// 会走到这一支的人，要么是管理员刚把它停用而他还站在门里，要么是拿着旧链接。
	if target.Disabled {
		if fromForm {
			redirectPinFail(w, r, "disabled")
			return
		}
		// 这条 detail 走 noticeOf(i18n.Default, …) 而不是 s.notice：**JSON 错误固定中文**。
		// 页面上的提示随这台机器的语言翻，API 的 detail 不翻——它是给脚本与 AI 读的，
		// 混进第二种语言会让"这句话在两种语言里各有一份"这件事对不上号，而调用方要的
		// 是稳定可对照的文本，不是随界面变的散文。
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Detail: noticeOf(i18n.Default, "disabled")})
		return
	}

	if !auth.VerifyPIN(target.PIN, in.PIN) {
		if fromForm {
			redirectPinFail(w, r, "pin")
			return
		}
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Detail: "PIN 不对"})
		return
	}
	auth.WriteSession(w, in.Profile)
	writePinOK(w, r, fromForm, auth.Session{Active: in.Profile})
}

// pinSessionResponse 是 /pin JSON 通路的成功体。用 struct 不用 map：
// 键序由字段序固定（§4.3；这个键今天恰好是字母序，map 蒙对了，
// 下一个人再加一个键就会把顺序交出去——admin.go 开头同一句话）。
//
// **只有一个键**：原来还有 `managing`，它由 `Session.Managing()` 推出，而代管
// 2026-10-10 整条下线之后那个值恒为 false。留一个恒假的键是那种"看起来还能用"
// 的假选项——真有人读它，会把"这台机器可能正在代管"这件事读成真的。
// 删键是契约变更，openapi.json 同一处已同步。
type pinSessionResponse struct {
	Active string `json:"active"`
}

// writePinOK 是验证通过后的出路：表单提交是一次浏览器导航，拿 JSON 当
// 页面渲染就是白屏，所以 303 回首页（303 保证后续是 GET）；fetch/JSON
// 调用方照旧拿 JSON 体。
func writePinOK(w http.ResponseWriter, r *http.Request, fromForm bool, sess auth.Session) {
	if fromForm {
		// **keep_add 必须在这里接回来**：选择页逐张卡片把它带进了 PIN 表单，
		// 而 P1-④ 是零 JS 的、没有"前端接住"这件事——④ 的注释里那句"由前端或
		// 重定向接住"当时两端都没落地，于是 ?add= 在进门这一步就静默丢了，
		// bookmarklet 用户输完 PIN 回到一个普通首页，浮层里空的。
		// 现在有 ⑤ 的浮层了，这条线才接得起来。
		if a := strings.TrimSpace(r.FormValue("keep_add")); a != "" {
			loc := "/?add=" + url.QueryEscape(a)
			if t := strings.TrimSpace(r.FormValue("keep_title")); t != "" {
				loc += "&t=" + url.QueryEscape(t)
			}
			http.Redirect(w, r, loc, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusOK, pinSessionResponse{Active: sess.Active})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		// "GET /" 是兜底 pattern，未注册的路径要维持 404（Task 1 钉过的行为）。
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "not found"})
		return
	}
	// ?add= 是 bookmarklet 带进来的。PIN 步骤不许丢它：选择页里
	// 用 hidden 字段带上，验证成功后由前端（P1-④）或重定向接住。
	keepAdd := r.URL.Query().Get("add")
	// ?err= 只查表，不解释：未登记的键渲染成空字符串，页面回到普通形状。
	notice := s.notice(r.URL.Query().Get("err"))

	sess := auth.ReadSession(r)
	// 会话合法性这一次请求只查一遍（与 Task 3 的"一次分诊就够"同一口径）：
	// 查完的结果交给下面那个清 cookie 的分支用，判据本身是 Session.Valid
	// 那唯一的一份。
	legal := sess.Valid()
	if !sess.IsEmpty() && !legal {
		// 非法形状（升级前的代管 cookie 就在这里被认出）：清 cookie 回选择页。
		auth.ClearSession(w)
		sess = auth.Session{}
	}
	if !sess.IsEmpty() {
		p, err := s.deps.Store.LoadProfile(sess.Active)
		switch {
		case errors.Is(err, store.ErrNotFound):
			// 档案已删除：这是"没这个档案"，清 cookie 回选择页。
			// 其它读失败（profile.json 坏了）不清、不静默（评审 #4）：
			// 继续往下走，ProfilePage 会把 ErrCorrupt 递出来渲染解释页
			// ——§12"该档案页面显示读不了"，坏数据文件不是登出理由。
			auth.ClearSession(w)
			sess = auth.Session{}
		case err == nil && p.Disabled:
			// 停用是管理员**刚刚**做的事，而这个人的会话是停用之前发出去的（30 天）。
			// 从前停用只挡 PIN 那道门：已经站在门里的人可以一直用到 cookie 过期，
			// 而界面一个字都不说，而他正是被停用的那一个。清会话，并把原因说清楚。
			auth.ClearSession(w)
			sess = auth.Session{}
			notice = s.notice("disabled")
		}
	}

	if sess.IsEmpty() {
		// 三形状的判据在这里算一次，模板只印（设计 §2）。
		// 用 CountProfiles（目录数）而不是 ListProfiles 的长度：坏档案也占位，
		// 否则"装坏一个"就把引导窗口重新打开（Task 1 的同一条纪律，两份判据
		// 必须是同一个数）。
		n, err := s.deps.Store.CountProfiles()
		if err != nil {
			s.handleError(w, err)
			return
		}
		opts := render.PickerOptions{KeepAdd: keepAdd, KeepAddTitle: r.URL.Query().Get("t"), Notice: notice}
		switch {
		case n == 0:
			opts.Setup, opts.Heading = true, s.t("picker.heading.none")
		case n == 1:
			// 单人安装不再先问"谁"：标题直接是那个人的名字。名字读不出来
			// （档案坏了）时不编造，用一句中性话——这一页本来就要输 PIN。
			opts.Heading = s.onlyProfileTitle()
		default:
			// ≥2 不传标题：那三个字只写一处（render.PickerPage 的默认值）。这里再写一遍
			// 就是两份真相，改文案的人只会改一处，另一处变成没人看见的旧话。
			// 判据本身（"现在是多人形状"）仍然在这里算。
		}
		page, err := s.deps.Render.PickerPage(opts)
		if err != nil {
			s.handleError(w, err)
			return
		}
		s.writePage(w, r, page)
		return
	}

	// ?q= 是纯读侧参数：不参与权限判定、不写任何东西，过滤发生在渲染侧
	// （render/filter.go 的判据只有一份）。无会话时它根本不会被读到——
	// 选择页没有书签可筛，也不许把它回声进表单（spec §8）。
	//
	// 上面那一段已经把非法会话清成空会话了（Session.Valid 现在一并认得出
	// 升级前的代管 cookie）。到这里 `lb_profile` 就是全部，渲染侧不再需要
	// "是不是代管"与"别人是谁"这两个输入——2026-10-10 改判把 `/switch to=<id>`
	// 删了之后，它们永远不会有人传。
	req := render.PageRequest{
		ID: sess.Active, Query: r.URL.Query().Get("q"),
		Add: keepAdd, AddTitle: r.URL.Query().Get("t")}
	page, err := s.deps.Render.ProfilePage(req)
	if errors.Is(err, store.ErrCorrupt) {
		reason, _ := s.deps.Store.BrokenReason(sess.Active)
		s.writeBrokenProfile(w, r, sess.Active, reason)
		return
	}
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// onlyProfileTitle 取唯一那个档案的显示名；读不出来就退回一句中性的话，
// 不编造名字也不空着。判据（有几个档案）在 handleRoot 已经算过，这里只是取名字——
// 所以它不改判据，也就不违反"判据只有一份"。
func (s *Server) onlyProfileTitle() string {
	ps, err := s.deps.Store.ListProfiles()
	if err != nil || len(ps) != 1 || strings.TrimSpace(ps[0].Title) == "" {
		return "你的书签"
	}
	return ps[0].Title
}

func (s *Server) writePage(w http.ResponseWriter, r *http.Request, p render.Page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-LB-Profile", p.Profile)
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'")
	if p.NoStore {
		w.Header().Set("Cache-Control", "private, no-store")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("ETag", p.ETag)
		if r.Header.Get("If-None-Match") == p.ETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	_, _ = w.Write(p.Body)
}

// writeBrokenProfile 给坏档案一个 200 的解释页而不是 500：坏的是
// 数据文件，不是服务。页面只含档案 id 与 store 的文件名+行号原因，
// 不含书签内容、PIN 哈希或令牌；no-store，免得旧解释被缓存住。
// id 显式传入：GET / 报的是会话里的 active，POST /pin 报的是目标档案，
// 两者不是同一个来源，借用 sess.Active 会串。
func (s *Server) writeBrokenProfile(w http.ResponseWriter, r *http.Request, id, reason string) {
	if reason == "" {
		reason = "原因未知（见服务端 stderr）"
	}
	page, err := s.deps.Render.BrokenPage(id, reason)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// handleError 是渲染路径的 5xx 出口：与 writeInternalError 同一契约，
// 内部错误文本（磁盘路径等）只进 stderr，响应保持 generic。
func (s *Server) handleError(w http.ResponseWriter, err error) {
	writeInternalError(w, err)
}

// createProfileResponse 是建档 201 的成功体。键序 id, title, admin 是
// 契约（openapi.json 与 AGENTS.md 都这么写）——旧写法用 map，Go 按字母
// 序吐出 admin, id, title，线上与文档从那天起就是两句话（最终整枝
// 评审 #2；§4.3 的"响应体一律 struct"对这条同样成立）。
type createProfileResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Admin bool   `json:"admin"`
}

// createInput 是建档的全部输入。两条表单通路（引导页、管理页）不问 id，但它们
// 也不靠"ID 是空串"来暗示要派生：`POST /api/v1/profiles` 的契约把 id 写成必填
// （openapi 的 required），空 id 在那里是 422，不是"随便你起个名"。所以派生是
// **显式**的一格。FirstOnly 把落点换成 CreateFirstProfile——引导通路的"磁盘为空"
// 判据与写入必须在 store 的同一次串行化里，handler 不数档案。
type createInput struct {
	ID        string
	Title     string
	PIN       string
	Admin     bool
	DeriveID  bool
	FirstOnly bool
}

// createResult 是建档的结果外壳无关部分：Status == 0 是成功；FieldErrs 非空
// 是 422 级；Status 是 JSON 壳要写的状态码，Err 是它要写进 stderr 的那个原始错误
// （不是从字符串重建的——writeInternalError 打的必须是真正失败的那条链）。
// 表单壳不写这些码：它把 FieldErrs 折成 ?err= 白名单键、把 ErrBootstrapClosed
// 折成不带 err 的 303。
type createResult struct {
	Profile   *model.Profile
	FieldErrs []fieldError
	Status    int
	Detail    string
	Err       error
	// Permission 只标一件事：落盘失败是"宿主的数据目录对这个用户不可写"
	// 这一族（errors.Is fs.ErrPermission）。它不是状态码——API 那条壳照样
	// 回 500（调用方自己看 stderr 与容器日志），只有两张表单把它折成
	// ?err=permission。折成键的不是"落盘失败"而是"这一族"：磁盘满与只读
	// 文件系统给不出照着做的下一步，塞进同一个键等于把人引到错的地方。
	Permission bool
}

// permDenied 判一件事：这次落盘失败是不是"宿主上的数据目录对跑服务的那个用户
// 不可写"这一族。它是**唯一的判据**，写在函数里而不是散在两处 if 上——
// 引导那张表单与管理页那张共用一句话，判据一散开就会有一处先长出第二种口径。
//
// 判的是哨兵 fs.ErrPermission（OS 层的 EACCES/EPERM），不是错误文本：store
// 从 MkdirAll 到 WriteFileAtomic 一路包着 %w，哨兵逐层 unwrap 得出来，文本则要
// 逐条跟着平台措辞变（Windows 说 "Access is denied."、Linux 说 "permission
// denied"）——按文本判等于第二天换个平台就失效。
//
// 刻意**不**把 EROFS（只读文件系统）算进来：那句话教的是 chown，而 chown 治不了
// 一个被挂成 ro 的卷，教错出路比不说更坏。它留在 500 里，与"磁盘满"同一族。
func permDenied(err error) bool { return errors.Is(err, fs.ErrPermission) }

// hashFormPIN 是建档族（createProfile / handleRebuildProfile）共用的 PIN 判据：
// 格式 + 哈希一次做完。"判据只有一份"由 Test建档判据只有一处 钉着（Task 4 把
// handleRebuildProfile 加进那条名单）——第二条建档壳落地那天它若绕过这里自己哈希，闸就红。
//
// 两条失败各回各的文本：格式错说"要 4-6 位数字"（给人看的那句），auth 内部的
// 哈希错说它自己的原因（这类错不该被粉饰成格式问题，人读了也修不了）。
func hashFormPIN(pin string) (string, []fieldError) {
	if !auth.ValidPinFormat(pin) {
		return "", []fieldError{{Field: "pin", Reason: "要 4-6 位数字"}}
	}
	hash, err := auth.HashPIN(pin)
	if err != nil {
		return "", []fieldError{{Field: "pin", Reason: err.Error()}}
	}
	return hash, nil
}

func (s *Server) createProfile(in createInput) createResult {
	// ID 不 trim：带空格的 id 在今天的契约里就是 422（`{"id":" mom "}` 不会悄悄
	// 变成 mom），本次是重构，不改这条。派生那一路由 NextProfileID 自己折 slug。
	id := in.ID
	if in.DeriveID {
		id = s.deps.Store.NextProfileID(in.Title)
	}
	var errs []fieldError
	if !in.DeriveID && !model.ValidSlug(id) { // 只有 API 那条壳会走到这格：表单不问 id
		errs = append(errs, fieldError{Field: "id", Reason: "必须是 ASCII slug（小写字母、数字、连字符）"})
	}
	if strings.TrimSpace(in.Title) == "" {
		errs = append(errs, fieldError{Field: "title", Reason: "不能为空"})
	}
	hash, pinErrs := hashFormPIN(in.PIN) // PIN 判据只有一份：建档族共用（见函数上的注释）
	errs = append(errs, pinErrs...)
	if len(errs) > 0 {
		return createResult{FieldErrs: errs}
	}
	// 建档的串行化（存在性检查 + 落盘）在 store 内部完成，调用方不需要、
	// 也不应该再包一层 WithProfileLock。
	// 建档不带任何预置内容：每个档案从零开始，形状一模一样（2026-10-10，
	// 模板机制整条拆掉之后）。
	pi := store.NewProfileInput{ID: id, Title: in.Title, PinHash: hash, Admin: in.Admin}
	var p *model.Profile
	var err error
	if in.FirstOnly {
		p, err = s.deps.Store.CreateFirstProfile(pi)
	} else {
		p, err = s.deps.Store.CreateProfile(pi)
	}
	switch {
	case errors.Is(err, store.ErrBootstrapClosed):
		return createResult{Status: http.StatusConflict, Detail: "引导期已过"}
	case errors.Is(err, store.ErrExists):
		return createResult{Status: http.StatusConflict, Detail: "档案已存在"}
	case errors.Is(err, store.ErrInvalid), errors.Is(err, store.ErrCorrupt):
		// store 的错误文本含磁盘路径，与 500 同一理由不回显；给一句人话。
		// 键是 profile 而不是 fromTemplate：模板机制 2026-10-10 整条拆掉之后，
		// 这两个哨兵唯一还可能的来源就是档案自己写不下去。
		return createResult{FieldErrs: []fieldError{{Field: "profile", Reason: "建档没成：数据文件读不了或写不下去"}}}
	case permDenied(err):
		// 这一族排在最后一条之前、且独立于 500：它是"宿主上 ./data 不是
		// 跑服务的那个用户写的"——群晖/威联通装机的头一个故障，用户在宿主机
		// 上 chown 一下就能继续，所以两张表单要能说得出这句话（Status 留给
		// API 那条壳回 500）。判据是哨兵而不是错误文本：store 各处包了
		// %w，一层层 unwrap 下去 EACCES 还在，而文本一个字都不许进页面。
		return createResult{Status: http.StatusInternalServerError, Err: err, Permission: true}
	case err != nil:
		return createResult{Status: http.StatusInternalServerError, Err: err}
	}
	return createResult{Profile: p}
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		PIN   string `json:"pin"`
		Admin bool   `json:"admin"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	// 访问判定交回 require 这一份唯一判据（最终整枝评审 #1）。以前这里
	// 自决（s.authorize 只认 Bearer），是全案唯一绕开 require 的端点，
	// 与路由表声明的 credAdmin、与兄弟端点 GET/DELETE /profiles 和
	// /profiles/pin（都接受管理员会话）都不齐。spec §6.1 说的是
	// "仅管理员"，不是"仅管理员令牌"——这确实放宽了该端点：管理员会话
	// 现在能建档（P1-④ 的管理页要用），档案作用域的令牌照旧 403。
	// 顺序与 batch/adminPin 同口径：先解码（形状错 422），再申报进 require。
	if _, _, ok := s.require(r, credAdmin, in.ID); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	res := s.createProfile(createInput{ID: in.ID, Title: in.Title, PIN: in.PIN,
		Admin: in.Admin})
	switch {
	case len(res.FieldErrs) > 0:
		writeFieldErrors(w, http.StatusUnprocessableEntity, res.FieldErrs)
	case res.Status == http.StatusConflict:
		writeJSON(w, res.Status, errorResponse{Code: res.Status, Detail: res.Detail})
	case res.Status != 0:
		writeInternalError(w, res.Err)
	default:
		// 201 只在成功这一支由这一次 WriteHeader 决定（writeJSON 就是
		// 它的唯一出路），"删掉状态码"这种回归仍会当场红。
		writeJSON(w, http.StatusCreated, createProfileResponse{
			ID: res.Profile.ID, Title: res.Profile.Title, Admin: res.Profile.Admin})
	}
}

// fieldError 是 422/字段级错误的唯一条目形状。OpsIndex 只有 ops 校验
// 错会带（batch.go 的 opFieldErrors 负责填），请求级字段错没有这个键；
// 用指针是因为 opsIndex:0 说的是"第 0 条"，裸 int + omitempty 会把 0 吞掉。
//
// 注：建档端点曾有自己的一份 s.authorize 自决（只认 Bearer），最终整枝
// 评审 #1 把它并回 require 唯一判据后 authorize 再无调用者，已删——
// "绕开 require 自己判访问"这条路不该在文件里留着诱人照抄。Bearer
// 解析的唯一实现是 routes.go 的 bearer(r)。
type fieldError struct {
	OpsIndex *int   `json:"opsIndex,omitempty"`
	Field    string `json:"field"`
	Reason   string `json:"reason"`
}

var unknownFieldRE = regexp.MustCompile(`unknown field "([^"]+)"`)

// decodeFieldErrors 把"未知字段"点名到具体字段上——422 要逐条给字段，
// 只报 body 等于没报；其余解码错误（语法、类型）归到 body。
func decodeFieldErrors(err error) []fieldError {
	if m := unknownFieldRE.FindStringSubmatch(err.Error()); m != nil {
		return []fieldError{{Field: m[1], Reason: "未知字段"}}
	}
	return []fieldError{{Field: "body", Reason: err.Error()}}
}

// decodeBody 先读满（带上限）、再验字节、再解；拒未知键。
// 读满本身就是抽干：同一连接上的下一个请求不会读到上一段的尾巴。
func decodeBody(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return fmt.Errorf("读请求体: %w", err)
	}
	if len(body) > maxBodyBytes {
		return fmt.Errorf("请求体超过 %d 字节上限", maxBodyBytes)
	}
	// encoding/json 会把非法 UTF-8 静默替换成 U+FFFD 再交给我们存盘：
	// 一次编码不端的调用就永久改掉一个书签的名字。宁可拒掉。
	if !utf8.Valid(body) {
		return fmt.Errorf("请求体不是合法 UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = store.WriteJSON(w, v)
}

// writeInternalError 只给 500 用：内部错误文本常含磁盘路径与文件内容，
// 不该出现在 HTTP 响应里；真实原因写进 stderr 给运维看。
// 401/403/404/409/422 的响应体不受影响，照旧点名原因。
// 体走 errorResponse（最终整枝评审 #2：本案错误响应只有一个外壳
// {code, detail, currentRev?, errors?}，map 产生的字母序不算形状）。
func writeInternalError(w http.ResponseWriter, err error) {
	fmt.Fprintf(os.Stderr, "launchbay: 内部错误: %v\n", err)
	writeJSON(w, http.StatusInternalServerError, errorResponse{Code: 500, Detail: "内部错误"})
}

func writeFieldErrors(w http.ResponseWriter, code int, errs []fieldError) {
	writeJSON(w, code, errorResponse{Code: code, Errors: errs})
}

// corruptDetail 给 503 取文本（最终整枝评审 #3）：503 的**判定**必须来自
// 本次请求返回的错误（调用方用 errors.Is(err, store.ErrCorrupt) 把关），
// 文本说的也必须刚刚真正失败的那个文件。store 的 ErrCorrupt 文案按契约
// 点名文件与位置（load.go 刻意用 filepath.Base，不留磁盘路径）；只有
// 少数走 errors.Join 的校验错文本里不带文件名时才退回坏档登记表取文本。
// 登记表不能再当判据：它优先 profile 来源，usage.json 的损坏更是不进表
// ——拿"要不要 503"去查它，会被陈旧记录答成指错文件、指错原因。
func corruptDetail(st *store.Store, id string, err error) string {
	msg := err.Error()
	if strings.Contains(msg, ".json") {
		return msg
	}
	if reason, isBroken := st.BrokenReason(id); isBroken {
		return reason
	}
	return msg
}
