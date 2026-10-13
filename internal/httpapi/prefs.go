package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

// handlePrefs 是浏览器表单通路：改的是"这个人对自己台面的偏好"（主题 +
// 档案形态 + 整页排法 + 这一页叫什么）。它**不是** API 端点，也不建 JSON 版——表单发不出 JSON，而 AI
// 替你换主题没有真实需求；每多一条 API 面就要 openapi 与 AGENTS.md 各同步一次
// （spec §6.1）。
//
// 为什么只认会话不认令牌：与 self/pin 同族同判据——"谁能改谁"。管理员令牌
// 替别人改偏好是另一回事，本期没有这个需求，也不预留。
//
// 拒的长相是 303 回选择页，不是 401/403（spec §6.1 的浏览器通路映射）：
// 这条路只由页面表单走进来，走到这里就没人能消费一段 JSON 错误。
// 422（形状错）与 503（文件读不了）照旧走 JSON 外壳——它们不是权限拒绝。
//
// 为什么不写历史行：它不动书签、不产生 rev，与"改自己 PIN"同类。四条不写
// 审计行的动作一起记在 spec §15，防止下一个人在这里"顺手补一行"。
// maxPageTitleRunes 是"这一页叫什么"那一格的长度上限。按**字**（rune）数而不是
// 按字节：这一格写中文是常态，按字节算会让中文比英文少 3 倍额度而没人说得清为什么。
const maxPageTitleRunes = 60

func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		redirectPicker(w)
		return
	}
	// 上限必须在这里设：ParseForm 会整段读 body，不套 MaxBytesReader 的话
	// 承诺的 1 MiB 对这条无 JS 通路是假话（handlePin 的表单分支同一条教训）。
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			// 字段名跟着 handlePin 的同一失败走（body）：同一个病在两个端点上叫两个名，
			// AI 学到的词汇表就分家了。
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	theme, hasTheme := r.Form["theme"]
	layout, hasLayout := r.Form["layout"]
	title, hasTitle := r.Form["title"]
	view, hasView := r.Form["view"]
	if !hasTheme && !hasLayout && !hasTitle && !hasView {
		// "没说要改什么"不是"什么都不做然后成功"：那会让一个拼错的字段名
		// 静默返回 303，而调用方以为改成了。
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
			{Field: "theme", Reason: "要带 theme、layout、view 或 title 至少一个"},
			{Field: "layout", Reason: "要带 theme、layout、view 或 title 至少一个"},
			{Field: "title", Reason: "要带 theme、layout、view 或 title 至少一个"},
			{Field: "view", Reason: "要带 theme、layout、view 或 title 至少一个"},
		})
		return
	}
	// 同名多值（?theme=dark&theme=light）不是"取第一个"，是没说清要改什么。
	if hasTheme && (len(theme) != 1 || !model.ValidTheme(theme[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "theme", Reason: "只能是 " + strings.Join(model.ThemeValues, "|")}})
		return
	}
	if hasLayout && (len(layout) != 1 || !model.ValidLayout(layout[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "layout", Reason: "只能是 " + strings.Join(model.LayoutValues, "|")}})
		return
	}
	// view 的枚举里**没有空串**：留空是"没选过"，那是读侧的形状（Profile.View
	// 的注释），表单要清回默认就该送 "groups"，不是送空。
	if hasView && (len(view) != 1 || !model.ValidView(view[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "view", Reason: "只能是 " + strings.Join(model.ViewValues, "|")}})
		return
	}
	// title 是**这一页叫什么**（profile.PageTitle），不是"这是谁的那一份"。
	// 上限必须在服务端判：表单那个 maxlength 只有浏览器认，而 curl 与扩展不认——
	// 这一格会进每一张档案页的 <h1> 与 <title>，一条几 KB 的名字等于把首屏撑大。
	// 空串是**合法值**，意思是"没起过名字"：渲染侧按界面语言给通用默认，
	// 不往盘里编一个字（清名字与改名走同一格表单）。
	if hasTitle && (len(title) != 1 || len([]rune(title[0])) > maxPageTitleRunes) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "title", Reason: fmt.Sprintf("最多 %d 个字，空着 = 用默认名", maxPageTitleRunes)}})
		return
	}
	var saveErr error
	id := sess.Active
	// 读→改→验→写整段进锁，与 handleSelfPin 同形（评审 Important 1 的教训：
	// SaveProfile 不自带锁，裸 RMW 与并发的管理员设 PIN 撞车会互相盖掉）。
	// 没给的字段不碰：只改 layout 不许把 theme 顺手归位。
	err := s.deps.Store.WithProfileLock(id, func() error {
		p, err := s.deps.Store.LoadProfile(id)
		if err != nil {
			return err // 分诊在锁外做：404 / 503 / 500
		}
		if hasTheme {
			p.Theme = theme[0]
		}
		if hasLayout {
			p.Layout = layout[0]
		}
		if hasView {
			p.View = view[0]
		}
		if hasTitle {
			// 空串照旧写下去（= 清除），不是"没给"：没给的是 !hasTitle 那一支。
			p.PageTitle = strings.TrimSpace(title[0])
		}
		saveErr = s.deps.Store.SaveProfile(p)
		return nil
	})
	if err != nil {
		// 与 handleSelfPin 同一条分诊，不新造第四种口径。
		s.writeProfileReadFailure(w, id, err)
		return
	}
	if saveErr != nil {
		writeInternalError(w, saveErr)
		return
	}
	// 成功与拒都回 `/`：字节相同，语义不同（拒是"你没资格动这条路"，成功是
	// "改完了，回首页看新主题"）。判据在 require 那一份里，外壳在这一句里。
	redirectPicker(w)
}

// handlePrefsPage 渲染**台面面板的独立页**（GET /prefs）。
//
// 这一页是顶栏那颗「台面」的**不经 JS 的那条路**：档案页里页内嵌的那一份浮层
// （app.js 拦下点击打开）与这一页渲染的是同一个 define，于是两处永远一致。
// 改设置的动作本身始终是普通的 form POST /prefs——面板是 JS 画的，改设置不是。
//
// 准入与 /revert、/history 同一族（selfProfileOnly）：档案从会话推导、不接受申报，
// 所以管理员带 cookie 进得了这一页也只能改自己那一份——管理员管账号、不碰内容。
// 带令牌一律 403：令牌发给 AI/扩展/CLI，而这一页是给人看的浏览器通路。
func (s *Server) handlePrefsPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.selfProfileOnly(w, r)
	if !ok {
		return
	}
	// 档案整个没了（会话指着一个已删的档案）→ 回选择页，那里会重新列一遍。
	// **坏档不挡**：改台面偏好只写 profile.json 的 theme/view/layout/pageTitle，
	// 而坏档正是最需要改排版看看的时候。
	if _, err := s.deps.Store.LoadProfile(id); errors.Is(err, store.ErrNotFound) {
		redirectPicker(w)
		return
	} else if err != nil && !errors.Is(err, store.ErrCorrupt) && !errors.Is(err, store.ErrInvalid) {
		writeInternalError(w, err)
		return
	}
	page, err := s.deps.Render.PrefsPage(id)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// handlePrefsGroup 是台面面板「分组」那一节的表单通路：改一个分组的**颜色**与
// **这一组自己的排法**（磁贴 / 单列）。
//
// **为什么不在 /prefs 里一起办**：那两个字段落在 bookmarks.json 的分组上，
// 属于书签数据——快照、审计线、rev 三道闸一道都不能少，所以它走
// `applyBatch`（与 POST /api/v1/batch 同一个落点，`/import/apply` 也是这么走的）。
// `/prefs` 只写 profile.json 的台面偏好，混进来的话它那句"四个字段至少给一个"
// 就会开始管到不属于它的东西，而 422 的字段名也就再也不能当"这一格写错"读。
//
// **档案不接受申报**：与 /revert、/history、/prefs 同一族，档案从会话推导。
// 带令牌一律 403——这一页是给人看的。
//
// 成功回 `/prefs` 而不是 `/`：改颜色本来就是一件要连着改好几组的事，
// 回到面板才能接着按下一颗；而面板上有「回我的页面」，出去的路是现成的。
func (s *Server) handlePrefsGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := s.selfProfileOnly(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	gid := r.FormValue("group")
	// 分组 id 是寻址键（§4.4），而 ops 的路径文法本身就只收 ASCII slug——
	// 这里先判一次，是为了让"分组名写错了"这句话直接点名 group 字段，
	// 而不是变成一句"路径解析失败"。
	if gid == "" || !model.ValidSlug(gid) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "group", Reason: "要带 group，且必须是 ASCII slug"}})
		return
	}
	accent, hasAccent := r.Form["accent"]
	layout, hasLayout := r.Form["layout"]
	// 一张表单里摆着颜色与排法两组键，靠 HTML"只提交被按下的那个控件"分开。
	// 所以"两样都没交"是真有人按了提交键却没有键被按下（curl 拼错字段名、
	// 或按了没有 name 的那颗"用这个颜色"）——那不是"什么都不改然后成功"。
	if !hasAccent && !hasLayout {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
			{Field: "accent", Reason: "要带 accent 或 layout 至少一个"},
			{Field: "layout", Reason: "要带 accent 或 layout 至少一个"},
		})
		return
	}
	if hasAccent && (len(accent) != 1 || !model.ValidAccent(accent[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "accent", Reason: "空着 = 跟随页面主色，否则只能是 #rgb 或 #rrggbb"}})
		return
	}
	if hasLayout && (len(layout) != 1 || !model.ValidGroupLayout(layout[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "layout", Reason: "只能是 " + strings.Join(model.GroupLayoutValues, "/")}})
		return
	}
	var opList []ops.Op
	if hasAccent {
		opList = append(opList, ops.Op{Op: "set", Path: "groups/" + gid + "/accent",
			Value: json.RawMessage(strconv.Quote(accent[0]))})
	}
	if hasLayout {
		opList = append(opList, ops.Op{Op: "set", Path: "groups/" + gid + "/layout",
			Value: json.RawMessage(strconv.Quote(layout[0]))})
	}
	// expectedRev 传 nil：面板是**服务端渲染**的，它画出来的那一刻的 rev 未必
	// 是人按下那一刻的 rev（另一个人刚改过、或这一页开着很久）。冲突在这里不是
	// 危险——两件事改的是不同字段，ops 在锁内重读当前值再改。
	out, err := s.applyBatch(id, id, "web", nil, opList)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	switch {
	case out.missing:
		// 档案整个没了：会话指着它，而它已经被删了。与 GET /prefs 同一个去处。
		redirectPicker(w)
	case out.broken != "":
		// 坏档在这一条路上是**常态而不是意外**：GET /prefs 那一节不画，
		// 于是人按不到这里的键；能走到这儿只可能是书页开着、文件随后坏了。
		// 说清是什么坏了，别把人支去修一个没坏的文件。
		s.renderPrefsNotice(w, r, id, out.broken)
	case len(out.verrs) > 0:
		// 整批没写：最常见的是"这个分组已经没了"（被人删了），把 ops 的原话带出去。
		s.renderPrefsNotice(w, r, id, out.verrs[0].Reason)
	case out.snapErr != nil:
		s.renderPrefsNotice(w, r, id, "这一版存不下，改动已取消，书签一个字都没变")
	default:
		http.Redirect(w, r, "/prefs", http.StatusSeeOther)
	}
}

// renderPrefsNotice 把"这次没改成，因为…"画回面板顶部，而不是把人丢到一个
// 没有出错的首页上（那与"成功了"长得一模一样）。
//
// 落点复用的是 handlePrefsPage 的那道分诊（读不动 profile.json → 503），
// 所以这里只多一句 notice，页面本身由同一条渲染路径出。
func (s *Server) renderPrefsNotice(w http.ResponseWriter, r *http.Request, id, notice string) {
	page, err := s.deps.Render.PrefsPage(id)
	if err != nil {
		s.handleError(w, err)
		return
	}
	body := page.Body
	// 插在 <body...> 之后、面板之前。位置判据用面板自己的根 class 而不是
	// "<body>"：body 标签带一串 data-* 属性，逐字找标签名会在下一次有人
	// 加属性时静默失配。
	at := bytes.Index(body, []byte(`<section class="prefs">`))
	if at < 0 {
		// 面板的根 class 变了：宁可把这句话原样写进页头，也不要悄悄丢掉一次失败。
		s.writePage(w, r, page)
		return
	}
	warn := `<p class="notice">` + html.EscapeString(notice) + `</p>` + "\n"
	page.Body = append(body[:at], append([]byte(warn), body[at:]...)...)
	s.writePage(w, r, page)
}
