package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
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
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
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
			{Kind: apierr.FieldInvalid, Field: "theme", Reason: "要带 theme、layout、view 或 title 至少一个"},
			{Kind: apierr.FieldInvalid, Field: "layout", Reason: "要带 theme、layout、view 或 title 至少一个"},
			{Kind: apierr.FieldInvalid, Field: "title", Reason: "要带 theme、layout、view 或 title 至少一个"},
			{Kind: apierr.FieldInvalid, Field: "view", Reason: "要带 theme、layout、view 或 title 至少一个"},
		})
		return
	}
	// 同名多值（?theme=dark&theme=light）不是"取第一个"，是没说清要改什么。
	if hasTheme && (len(theme) != 1 || !model.ValidTheme(theme[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "theme", Reason: "只能是 " + strings.Join(model.ThemeValues, "|")}})
		return
	}
	if hasLayout && (len(layout) != 1 || !model.ValidLayout(layout[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "layout", Reason: "只能是 " + strings.Join(model.LayoutValues, "|")}})
		return
	}
	// view 的枚举里**没有空串**：留空是"没选过"，那是读侧的形状（Profile.View
	// 的注释），表单要清回默认就该送 "groups"，不是送空。
	if hasView && (len(view) != 1 || !model.ValidView(view[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "view", Reason: "只能是 " + strings.Join(model.ViewValues, "|")}})
		return
	}
	// title 是**这一页叫什么**（profile.PageTitle），不是"这是谁的那一份"。
	// 上限必须在服务端判：表单那个 maxlength 只有浏览器认，而 curl 与扩展不认——
	// 这一格会进每一张档案页的 <h1> 与 <title>，一条几 KB 的名字等于把首屏撑大。
	// 空串是**合法值**，意思是"没起过名字"：渲染侧按界面语言给通用默认，
	// 不往盘里编一个字（清名字与改名走同一格表单）。
	if hasTitle && (len(title) != 1 || len([]rune(title[0])) > maxPageTitleRunes) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "title", Reason: fmt.Sprintf("最多 %d 个字，空着 = 用默认名", maxPageTitleRunes)}})
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
// 令牌一律 303 回选择页，**不是 403**（2026-10-14 实测后改判）：`selfProfileOnly`
// 对 bearer 的第一句就是 redirectPicker，而这一族四条（/prefs、/prefs/group、
// /prefs/tabs、/prefs/background）都走它。浏览器通路的权限拒绝一律 303（spec §6.1）——
// 令牌与无会话答同一句，因为这一页只由链接与表单走进来，走到这里没有 JSON 错误可消费。
// 文档曾在这三处写"令牌一律 403"，实测四种长相全是 303；`TestPrefs_面板这一族对令牌没有403`
// 与 `TestPrefs_面板带令牌一律送回选择页不是403` 是那次实测留下的钉子。
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
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	gid := r.FormValue("group")
	// 分组 id 是寻址键（§4.4），而 ops 的路径文法本身就只收 ASCII slug——
	// 这里先判一次，是为了让"分组名写错了"这句话直接点名 group 字段，
	// 而不是变成一句"路径解析失败"。
	if gid == "" || !model.ValidSlug(gid) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "group", Reason: "要带 group，且必须是 ASCII slug"}})
		return
	}
	// **一次按下 = 一个意图，判据是"名字"而不是"只提交被按下的那个控件"。**
	//
	// 这条通路上线时写的理由是"一张表单里两组键，靠 HTML 只提交被按下的那个控件
	// 分开"——那句话漏了**不是控件的那一格**：命名过的输入框每一次提交都跟着走。
	// 于是真浏览器按下一颗色块交出的是 `accent=#22b8a0&accent=#123456`（色块 +
	// 那颗常驻的取色器），按「磁贴」交出的是 `layout=tiles&accent=#123456`：
	// 前者被下面那道"恰好一个"判成形状错（人看到的是整屏裸 JSON），后者顺手把
	// 颜色也改了。实测过这两条才改的判据。
	//
	// 现在的形状：取色器改名 `custom`，那颗动作键带 `name=apply_custom`。
	// 于是四种按下各交得出什么，是**结构上定死的**，不是靠约定：
	//   按色块          → accent=<hex>            （custom 也在，但没有 apply_custom，读不到）
	//   按「跟随主色」   → accent=                 （同上）
	//   按磁贴/单列      → layout=<值>              （同上）
	//   按「用这个颜色」 → apply_custom=1 + custom=<hex>
	// `accent` 出现两次仍然按形状错拒：那已经不是浏览器的形状，是有人在拼 body。
	accent, hasAccent := r.Form["accent"]
	layout, hasLayout := r.Form["layout"]
	custom := r.Form["custom"]
	_, wantsCustom := r.Form["apply_custom"]
	// **改名**（2026-10-15）：分组名此前在界面上一个出口都没有——ops 一直认
	// `set path=groups/<gid>/title`，而面板只画得出颜色与排法。
	//
	// 这一格**常驻**在表单里（输入框每一次提交都跟着走，与取色器同一性质），
	// 所以「只在真的改了时才算一次改名」这个判据必须落在**值**上而不是名字上：
	// 按一颗色块也会带上 `rename=<原名>`，若照单全收就变成"改个颜色顺手把名字
	// 重写一遍"。于是下面按当前值把它压掉。
	rename, hasRename := r.Form["rename"]
	// onlyRename：**这一次按下说的就是改名**（没夹带颜色或排法）。它同时管住
	// 422 那一支与"要不要发这条 op"，两处判据必须同一个，否则会出现"报错说名字
	// 那一格不对、而屏上按的是色块"。
	onlyRename := hasRename && !hasAccent && !hasLayout && !wantsCustom
	if len(rename) > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "rename", Reason: "rename 只能有一个值"}})
		return
	}
	if onlyRename {
		if !hasRename {
			writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
				{Kind: apierr.FieldInvalid, Field: "accent", Reason: "要带 accent、layout，或改分组名"},
				{Kind: apierr.FieldInvalid, Field: "layout", Reason: "要带 accent、layout，或改分组名"},
			})
			return
		}
		// 空着就是"想改成一个空名字"，那是形状错，要点名 rename —— 而**不**是继续
		// 往下走让 ops 报一句「title 为空」，那句话说的是文档校验，说不出
		// "是这一格没填"。
		if strings.TrimSpace(rename[0]) == "" {
			writeFieldErrors(w, http.StatusUnprocessableEntity,
				[]fieldError{{Kind: apierr.FieldInvalid, Field: "rename", Reason: "分组名不能空着"}})
			return
		}
	} else if !hasAccent && !hasLayout && !wantsCustom {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
			{Kind: apierr.FieldInvalid, Field: "accent", Reason: "要带 accent、layout，或改分组名"},
			{Kind: apierr.FieldInvalid, Field: "layout", Reason: "要带 accent、layout，或改分组名"},
		})
		return
	}
	// 颜色那一格：谁说话由名字定。apply_custom 在场 = 取色器说话；否则 accent 说话。
	// 两个都不在 = 这一次只改排法，颜色一个字节不碰（从前它会被顺手写成取色器
	// 里的默认值，而那一格屏上从来没说过它会被提交）。
	var accentVal string
	handlesAccent := wantsCustom || hasAccent
	if wantsCustom {
		if len(custom) != 1 || !model.ValidAccent(custom[0]) || custom[0] == "" {
			writeFieldErrors(w, http.StatusUnprocessableEntity,
				[]fieldError{{Kind: apierr.FieldInvalid, Field: "custom", Reason: "自定义那一格只能是 #rgb 或 #rrggbb"}})
			return
		}
		accentVal = custom[0]
	} else if hasAccent {
		if len(accent) != 1 || !model.ValidAccent(accent[0]) {
			writeFieldErrors(w, http.StatusUnprocessableEntity,
				[]fieldError{{Kind: apierr.FieldInvalid, Field: "accent", Reason: "空着 = 跟随页面主色，否则只能是 #rgb 或 #rrggbb"}})
			return
		}
		accentVal = accent[0]
	}
	if hasLayout && (len(layout) != 1 || !model.ValidGroupLayout(layout[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "layout", Reason: "只能是 " + strings.Join(model.GroupLayoutValues, "/")}})
		return
	}
	// 当前值读一次（groupNow 那一支的解释在它自己头上）。
	curTitle, curAccent, curLayout, haveCur := s.groupNow(id, gid)
	var opList []ops.Op
	if handlesAccent && (!haveCur || accentVal != curAccent) {
		opList = append(opList, ops.Op{Op: "set", Path: "groups/" + gid + "/accent",
			Value: json.RawMessage(strconv.Quote(accentVal))})
	}
	if hasLayout && (!haveCur || layout[0] != curLayout) {
		opList = append(opList, ops.Op{Op: "set", Path: "groups/" + gid + "/layout",
			Value: json.RawMessage(strconv.Quote(layout[0]))})
	}
	// 改名那一格的值上限在这里判，理由与 maxPageTitleRunes 同源：表单上的
	// maxlength 只有浏览器认，curl 与扩展不认。
	//
	// 改名那一格的值上限在这里判，理由与 maxPageTitleRunes 同源：表单上的
	// maxlength 只有浏览器认，curl 与扩展不认。
	//
	// **校验只在「它就是那一次的全部意图」时生效**（onlyRename）：输入框里留着
	// 一次敲完没提交的内容时，按一颗色块会把那半句一起带上——那是"顺手被另一格
	// 拦下"，说清是哪一格该比说清它超了几十个字有用，而颜色那一格必须照改。
	// 反过来，只交了名字那一格（回车）时它就是全部意图，空着与超长都要点名 rename。
	if hasRename {
		t := strings.TrimSpace(rename[0])
		if onlyRename {
			if t == "" {
				writeFieldErrors(w, http.StatusUnprocessableEntity,
					[]fieldError{{Kind: apierr.FieldInvalid, Field: "rename", Reason: "分组名不能空着"}})
				return
			}
			if len([]rune(t)) > model.MaxGroupTitleRunes {
				writeFieldErrors(w, http.StatusUnprocessableEntity,
					[]fieldError{{Kind: apierr.FieldInvalid, Field: "rename",
						Reason: fmt.Sprintf("最多 %d 个字", model.MaxGroupTitleRunes)}})
				return
			}
		}
		// 一个字节都没变的**不发**：发出去 ops 会回一句「设的值与当前值相同」，
		// 而 applyBatch 只看 opList 非空，仍会**存一版快照并把 rev +1**——于是
		// "按一下已经在用的那颗色块"会在「回退」里留下一条什么都没改的版本。
		// 这一节自己写着「每按一次会存一版」，那句话得是真的。
		// 混着别的意图（onlyRename 为假）时也不发：一次按下只改一件事。
		if t != "" && (!haveCur || t != curTitle) {
			opList = append(opList, ops.Op{Op: "set", Path: "groups/" + gid + "/title",
				Value: json.RawMessage(strconv.Quote(t))})
		}
	}
	// expectedRev 传 nil：面板是**服务端渲染**的，它画出来的那一刻的 rev 未必
	// 是人按下那一刻的 rev（另一个人刚改过、或这一页开着很久）。冲突在这里不是
	// 危险——两件事改的是不同字段，ops 在锁内重读当前值再改。
	//
	// opList 可能**是空的**（按了一下已经在用的那颗色块）：applyBatch 那一支
	// 直接早退，不写快照也不加 rev，于是「什么都没改」就是真的什么都没发生。
	s.applyPrefsOps(w, r, id, opList)
}

// groupNow 读这一组当前那三个可改的标量（标题 / 颜色 / 排法）与"读得到吗"。
//
// 唯一的用处是**把"值没变"的那些 op 压掉**（2026-10-15）。applyBatch 只看
// opList 非空：只要有一条 op 落进去，它就**一定**写一版快照并把 rev +1，于是
// "按一下已经在用的那颗色块"会在「回退」里留下一条什么都没改的版本——而这一节
// 自己写着「每**改**一次会存一版」，那句话得是真的。
//
// 读不到时 ok=false：那三个判据全部跳过，于是照单发出去，让 ops 在锁内用权威值
// 说话（那里报出来的错才是对的），而不是猜一个值把别人的改动压掉。
func (s *Server) groupNow(profile, gid string) (title, accent, layout string, ok bool) {
	b, err := s.deps.Store.LoadBookmarks(profile)
	if err != nil {
		return "", "", "", false
	}
	for i := range b.Groups {
		if b.Groups[i].ID == gid {
			return b.Groups[i].Title, b.Groups[i].Accent, b.Groups[i].Layout, true
		}
	}
	return "", "", "", false
}

// applyPrefsOps 是台面面板里**改书签数据**的那几节共用的落点与收尾
// （2026-10-14：`/prefs/group` 与 `/prefs/tabs` 共用一份）。
//
// **为什么不并进 `POST /prefs` 的字段名单**：那一条是 urlencoded 的台面偏好表单，
// 落点是 profile.json（改完不产生 rev、不做快照）；而 `openInNewTab` 住在**书签文档**
// 里——它跟着导出走，改它要过 rev 与快照，能在「回退」里退回来。两族落点不同，
// 混在一处会让那句「四个字段至少给一个」开始管到不属于它的东西。
func (s *Server) applyPrefsOps(w http.ResponseWriter, r *http.Request, id string, opList []ops.Op) {
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
