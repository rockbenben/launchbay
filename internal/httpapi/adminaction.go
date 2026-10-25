package httpapi

import (
	"errors"
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"os"
	"strings"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/render"
)

// 这一页是 §8.8 那两条写动作（**设 PIN** 与**删档**）的零 JavaScript 落点。
//
// spec §8.8 原写"要二次确认，表单发不出，随 ⑤ 的 JS 落地，这一版按钮不渲染"。
// **改判**：二次确认用**两步页面**做——先 GET 这一页读后果，再 POST 提交——
// 于是它在关掉 JavaScript 时照样成立，与 §7.2「关掉 JS 也能看能点」同一条纪律。
// 能零 JS 做的就不该排在 JS 后面，而 ⑤ 还要连带改预算闸（`<script` 计数从 0 变非 0）、
// 建静态资产通路，是一次大得多的工程。
//
// 不用 `confirm()` 的理由不止"那要 JS"：原生对话框样式不可控、移动端上常常
// 直接不出现，而它是**唯一**挡在"手滑删掉一个人"前面的东西——一道在某些设备上
// 自己不出现的闸等于没有闸。

// adminPageGuard 是这两页共有的准入：令牌一律拒，权限类一律 303 回选择页且
// **不带提示**（与 handleAdminCreate / prefs / switch 同纪律）。
//
// 返回 false 表示已经答完了，调用方直接 return。
func (s *Server) adminPageGuard(w http.ResponseWriter, r *http.Request) bool {
	if bearer(r) != "" {
		// 命令行走 API 那条：浏览器表单与机器档案各写一半鉴权，两处迟早差一次。
		redirectPicker(w)
		return false
	}
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		redirectPicker(w)
		return false
	}
	return true
}

// redirectAdmin 是**成功**那一支：回管理页，不带 ?err=。
//
// 成功与失败分成两个函数，而不是让 code 可以为空：成功时地址栏上多挂一个
// 空的 `?err=` 看着像出了错，而 Test提示键只发白名单里有的 那条闸会把 `""`
// 判成"键的来源看不见"——它要的不是"别发空键"，是"键要么是字面量、要么来自
// 一个分发函数"，而成功根本不该有键。
func redirectAdmin(w http.ResponseWriter) {
	w.Header().Set("Location", "/admin")
	w.WriteHeader(http.StatusSeeOther)
}

// redirectAdminErr 是失败那一支。**调用点一律写字面量**（那条 AST 闸按字面量
// 收键；传变量的话键就脱离了这道闸，而未登记的键到这里没人拦）。
func redirectAdminErr(w http.ResponseWriter, code string) {
	w.Header().Set("Location", "/admin?err="+code)
	w.WriteHeader(http.StatusSeeOther)
}

func redirectAdminPinErr(w http.ResponseWriter, id, code string) {
	w.Header().Set("Location", "/admin/pin?profile="+id+"&err="+code)
	w.WriteHeader(http.StatusSeeOther)
}

// redirectAdminDeleteErr 回**删档确认页**并带一把提示键（2026-10-23）。
//
// 为什么回确认页而不是管理页：这一族里"密码不对"是**最容易重来的一个错**
// ——人已经在这一页上、只是钥匙没输对，回到确认页改一格就能再按一次；
// 回管理页的话，那张表与后果说明都得重新点一遍。
//
// **id 必须是合法 slug**：调用点先判过形状（model.ValidSlug），而 slug 里没有
// 能逃出 URL 的字符——它不是一段用户原文，是这一族端点自己的判据。
//
// **键是参数，调用点传的是字面量**（"badkey"）：`Test提示键只发白名单里有的`
// 那道 AST 闸要求"键的来源看得见"——从变量转发（`redirectAdminErr(w, code)`）
// 会让它当场红，而这里只有一把键，本就不该再转一手。
func redirectAdminDeleteErr(w http.ResponseWriter, id, code string) {
	w.Header().Set("Location", "/admin/delete?profile="+id+"&err="+code)
	w.WriteHeader(http.StatusSeeOther)
}

// statusRowOf 拿一行聚合（§8.8 那张表的一行）。
//
// 复用 statusRows 而不是自己再遍历一遍 profiles/：两份遍历迟早只在一处修，
// 而漏掉的那一处表现为**某个档案在管理页上一行都不出现**。
func (s *Server) statusRowOf(id string) (statusProfile, bool) {
	rows, _, _, err := s.statusRows()
	if err != nil {
		return statusProfile{}, false
	}
	for _, r := range rows {
		if r.ID == id {
			return r, true
		}
	}
	return statusProfile{}, false
}

// handleAdminPinPage 是加/改 PIN 的第一步：显示目标档案与后果，让人输入 PIN。
//
// **加**与**改**说的是两件事，页面要分开说：这一份现在有没有 PIN
// （auth.HasPin，与 statusRows 同一份判据），决定了这一页是"给一份没上锁的
// 书签页加一道锁"还是"换一把锁"。同一句模板套两种情况，读的人得自己猜
// 哪一个是哪一个——而"加锁"与"换锁"的代价根本不同。
func (s *Server) handleAdminPinPage(w http.ResponseWriter, r *http.Request) {
	if !s.adminPageGuard(w, r) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	row, ok := s.statusRowOf(id)
	switch {
	case !ok:
		redirectAdminErr(w, "notfound")
		return
	case row.Broken:
		// 坏档改不了 PIN：LoadProfile 读不出 profile.json，adminPin 的锁内读
		// 必然报错。与其让它落到一句"读不出来"，不如在这里说清要干什么
		//（④.6 的 rebuild 那条路）。
		redirectAdminErr(w, "notfound")
		return
	}
	// **目标是不是当前会话正在用的那一份，这里不再拦**（2026-10-20 改判）。
	//
	// 从前这道闸的理由是"改自己的 PIN 要旧 PIN（家人共用设备，改自己的那一份
	// 属于身份变更）"。可管理员是**机器级**的一把 key，不属于任何档案——
	// 它不在任何一个"自己"里，于是那条理由在管理页上从来没有依据：本人自己
	// 那边（`POST /api/v1/profiles/self/pin`）照样要旧 PIN，而管理员这一边
	// 是"家人忘了 PIN 的出路"，它必须能管**每一份**档案。
	//
	// 同一件事的**机器通路**（`POST /api/v1/profiles/pin`）本来就只要求管理员
	// 身份、从来不拦"正在用的那一份"；管理页比它更严，于是两处对同一件事
	// 给出不同答案——那是真的坏（管理员在 API 上能改、在页面上不能）。
	//
	// 仍然拦着的是**删档与停用**：那两件是会话层面的事（删掉自己正在用的那
	// 一份会把会话悬空），与权限无关，见 deleteBlocked。
	title, heading, explain := s.t(r, "admin.pin.title.add"), s.t(r, "admin.pin.heading.add", rowLabel(row)), s.t(r, "admin.pin.explain.add", rowLabel(row))
	submit, field, hint := s.t(r, "admin.pin.submit.add"), s.t(r, "admin.pin.field.add"), s.t(r, "admin.pin.field.hint.add")
	if row.PinSet {
		title, heading, explain = s.t(r, "admin.pin.title.change"), s.t(r, "admin.pin.heading.change", rowLabel(row)), s.t(r, "admin.pin.explain.change", rowLabel(row))
		submit, field, hint = s.t(r, "admin.pin.submit.change"), s.t(r, "admin.pin.field.change"), s.t(r, "admin.pin.field.hint.change")
	}
	page, err := s.deps.Render.AdminActionPage(render.AdminAction{
		ID: id, Title: title, Heading: heading,
		// 档案名两侧要有引号：从前写成"进 妈妈的台 这一份"，名字裸粘在句中读起来
		// 像漏了字。人称也换掉——"他本人""你得"假定了对方的性别与身份，
		// 而管理员未必是家长。
		Explain: explain,
		// 改的是**自己正在用的那一份**时有一句额外的话要说：现在这个页面
		// 还在（会话不受影响），但下次进门要输新的那个 PIN。
		Note:   s.pinNoteFor(r, row),
		Notice: s.notice(r, r.URL.Query().Get("err")),
		Action: "/admin/pin",
		Submit: submit,
		// 长度范围**取自 auth 那两个常量**（2026-10-20）：这一格是这一族页面
		// 上唯一有长度范围的一格，而那一格原先连 minlength/maxlength 都没有——
		// 浏览器于是不拦超长，拦人的只有服务端，于是"页面上打进去能过"与
		// "这一格只收 4 到 6 位"在界面上是两句矛盾的话。数字不在这里写死：
		// 写死就是与服务端的判据两处各一份。
		Fields: []render.AdminField{{Name: "new", Label: field, Type: "password", Autofocus: true, Hint: hint,
			// PIN 那一格要数字键盘（它是 4 到 6 位数字）；inputmode 归字段自己，
			// 不归模板——管理密钥那一格要的是相反的那一种（见下面那处）。
			InputMode: "numeric",
			MinLen:    auth.PinMinLen, MaxLen: auth.PinMaxLen}},
	}, s.machineLang(r))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// pinNoteFor 是"目标就是当前会话正在用的那一份"时那句额外说明（别的情形空串）。
//
// 它是**后果**：改 PIN 不会把当前会话踢掉（会话挂在档案 id 上，不挂在 PIN
// 上），但下一次进门要输新的那个——不说的话，人会以为"改完就进不来了"，
// 于是不敢改自己那一份，于是永远用旧 PIN。
func (s *Server) pinNoteFor(r *http.Request, row statusProfile) string {
	if row.ID == "" || row.ID != sessionProfileID(r) {
		return ""
	}
	return s.t(r, "admin.pin.note.self")
}

// handleAdminPinForm 是第二步：走 API 同一个落点（adminPin），**不自写判据**。
func (s *Server) handleAdminPinForm(w http.ResponseWriter, r *http.Request) {
	if !s.adminPageGuard(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	if n := len(r.Form["new"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "new", Reason: "同一个字段只能出现一次"}})
		return
	}
	// **profile 同名多值也要拒**（2026-10-20 补）。`profile` 是"改哪一份"的身份
	// 字段，而同族的 /admin/delete 早就查了它（那边是 adminpage.go 抄的同一条
	// 纪律）。从前这一条只查 new，于是
	// `POST /admin/pin profile=kid&profile=dad&new=1357` 取第一个就改了 kid——
	// 与 /prefs、/setup 那条"同名多值不是取第一个，是没说清"同一族纪律。
	if n := len(r.Form["profile"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "profile", Reason: "同一个字段只能出现一次"}})
		return
	}
	id := strings.TrimSpace(r.FormValue("profile"))
	// 表单这一路的 new **总是给的**（表单里那一格就在那儿），空串 = 明确取消。
	// 所以这里传指针、不是传值：指针的 nil（"没给"）与空串（"取消"）在落点
	// 里分得开，而 API 那条要能说"你没给 new"。
	newPin := r.FormValue("new")
	res := s.adminPin(r, id, &newPin)
	if len(res.FieldErrs) > 0 && res.FieldErrs[0].Field == "cred" {
		redirectPicker(w)
		return
	}
	if len(res.FieldErrs) > 0 {
		// 只有"格式不对"回这一页重填；别的（读不出来/落盘失败）回管理页，
		// 因为重填并不能解决"那个档案读不出来"。
		if !model.ValidSlug(id) {
			redirectAdminErr(w, "notfound")
			return
		}
		redirectAdminPinErr(w, id, "badpin")
		return
	}
	switch {
	case res.ReadErr != nil:
		if errors.Is(res.ReadErr, os.ErrNotExist) {
			redirectAdminErr(w, "notfound")
			return
		}
		writeInternalError(w, res.ReadErr)
	case res.SaveErr != nil, res.HistErr != nil:
		// 500 的那两种：页面上只说"没成功"，细节在日志与 stderr。
		// **绝不能说成功**——PIN 已经改了而审计行没落，说成功会让下一个人
		// 以为历史是全的。
		fmt.Fprintf(os.Stderr, "admin pin form failed profile=%s save=%v hist=%v\n", id, res.SaveErr, res.HistErr)
		redirectAdminPinErr(w, id, "failed")
	default:
		redirectAdmin(w)
	}
}

// handleAdminDeletePage 是删档的第一步，**这一页必须写出后果**——它存在的
// 全部意义就是让"会发生什么"先被读到，而后果的数字（条目数、快照数）只有
// 这里才拿得到（statusRows 一趟就有，不用为了排版再遍历一次）。
func (s *Server) handleAdminDeletePage(w http.ResponseWriter, r *http.Request) {
	if !s.adminPageGuard(w, r) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	row, ok := s.statusRowOf(id)
	if !ok {
		redirectAdminErr(w, "notfound")
		return
	}
	if s.deleteBlocked(r, row) {
		redirectAdminErr(w, "self")
		return
	}
	page, err := s.deps.Render.AdminActionPage(render.AdminAction{
		ID: id, Title: s.t(r, "admin.delete.title"), Heading: s.t(r, "admin.delete.heading", rowLabel(row)),
		// 页面上不该出现 markdown 的反引号与磁盘文件名：`revert` 与 history.jsonl
		// 是给读代码的人看的词，家里人在浏览器里看到只会愣一下。
		//
		// **id 也要说出来**（2026-10-23）：标题是给人看的，而两份档案可以同名
		// （派生 id 那时是 home / home-2 两份不同的档案，标题一模一样）——只写
		// 名字的那一页分不清要删的是哪一份，而删掉找不回来。
		Explain: s.t(r, "admin.delete.explain", row.ID, row.Items, row.Snapshots),
		Notice:  s.notice(r, r.URL.Query().Get("err")),
		Action:  "/admin/delete",
		Submit:  s.t(r, "admin.delete.submit"),
		Danger:  true,
		// **要密码**（2026-10-23，用户拍板「删除需要有确认警示，并要求密码」）：
		// 这一格填那把机器级密钥（LAUNCHBAY_ADMIN_TOKEN）。它是这台机器上唯一
		// 不可逆的动作，而管理 cookie 是它的派生值——它进 devtools、进导出、进
		// 浏览器同步。于是"登着管理页"不再等于"有权删档"。
		//
		// 标签复用登录页那一格（admin.login.key）：同一把钥匙，同一套词汇。
		//
		// **inputmode 是 text，不是 numeric**（2026-10-23）：那把 key 是一段任意
		// 文本（默认那把是 launchbay123，用户自己换的那把可以有字母），数字键盘
		// 会让它在手机上根本没法输。PIN 那一格要的正是相反的那一种。
		Fields: []render.AdminField{{Name: "key", Label: s.t(r, "admin.login.key"), Type: "password",
			Autofocus: true, Hint: s.t(r, "admin.delete.keyHint"), InputMode: "text"}},
	}, s.machineLang(r))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// deleteBlocked 是删档唯一的形状闸。
//
// **只有一道，没有"最后一个管理员"那一道**——这不是漏写，是它在本案的权限模型下
// **不可达**：require(credAdmin) 判的是"这次请求有没有带一把真的管理权"（routes.go
// 的 AdminToken.Matches 与 hasAdminCookie），而**管理员是机器级的一把 key，不属于
// 任何档案**（档案上的 admin 属性 2026-10-12 就删了）。于是能进管理页的人
// 本身就是一个管理员，他能删的目标里"唯一的管理员"只有两个可能：他正在用的这一份
// （被 self 拦）或另一个管理员（那就不是最后一个）。
//
// 一条永远不触发的闸比没有更糟：它看起来像在那里守着。留着它，下一个人删掉 self
// 那一道时不会想到还有这道在（而它当时也确实不在管事）。
//
// 真要让"删掉最后一个管理员"成为一条约束，得改的是权限模型（让 Home 也参与判定
// 或给出别的判据），那是 §6.1 的改判，不该藏在一个页面的 if 里。
func (s *Server) deleteBlocked(r *http.Request, row statusProfile) bool {
	// 不能删自己：正在用这一份的人（会话就挂在它上面）把家删了。
	// 空串那格是"没登任何档案"——那是 2026-10-12 之后管理员最常见的样子，
	// 他没在用谁的台，删谁都合理，所以要显式排除掉。
	return row.ID != "" && row.ID == sessionProfileID(r)
}

// handleAdminDeleteForm 是第二步：同一个 Store.DeleteProfile 落点，与 API 那条
// （handleDeleteProfile）共用。**端点层不重复"规范化 id"**——那三道闸
// （ASCII slug / 目录须是 profilesDir 下的直接子目录 / 档案写锁）全在 store 里。
func (s *Server) handleAdminDeleteForm(w http.ResponseWriter, r *http.Request) {
	if !s.adminPageGuard(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	if n := len(r.Form["profile"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "profile", Reason: "同一个字段只能出现一次"}})
		return
	}
	id := strings.TrimSpace(r.FormValue("profile"))
	// 形状先判（它不泄露任何东西：问的是"这个 id 合不合法"，不是"它在不在"）。
	if !model.ValidSlug(id) {
		redirectAdminErr(w, "notfound")
		return
	}
	// **密码**（2026-10-23，用户拍板「删除需要有确认警示，并要求密码」）：
	// 这一条要那把机器级密钥（LAUNCHBAY_ADMIN_TOKEN）。判据只有
	// auth.AdminToken.Matches 一处——常量时间比较，key 的长度与内容都不进日志。
	//
	// 顺序在**存在性**之前：**先验密码，再谈删哪一份**——于是"key 不对"这一支
	// 的答案与目标无关（不问"这个 id 在不在"），而那正是这一族该有的形状。
	//
	// 没填与填错是**同一件事**（"你得把那把钥匙拿出来"），所以一把键 badkey
	// 就够，不分成两把——多一把只会让其中一半的话是假的。
	// **密码只从请求体里读**（2026-10-23 修）：`r.FormValue` 是**查询串与表单体
	// 的合并值**，所以 `POST /admin/delete?profile=kid&key=<那把 key>`（body 空）
	// 也算数——而那把 key 于是进了 URL：浏览器历史、访问日志、Referer 头都留着它。
	// 管理 cookie 存在的全部理由正是"原始 key 从不离开那一次 POST"，删档这一条
	// 更不能反着来。
	//
	// `r.PostFormValue` 只看请求体（ParseForm 填的那一份），查询串里的 key 一律
	// 不认——于是密码格必须真的填在那张表单里。
	//
	// 同名多值与 profile 同一族纪律（`/admin/login` 对 key 也是这么拒的）：
	// "没说要哪一个"不是"取第一个"。
	if n := len(r.PostForm["key"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "key", Reason: "同一个字段只能出现一次"}})
		return
	}
	key := strings.TrimSpace(r.PostFormValue("key"))
	if !s.deps.AdminToken.Matches(key) {
		redirectAdminDeleteErr(w, id, "badkey")
		return
	}
	row, ok := s.statusRowOf(id)
	if !ok {
		redirectAdminErr(w, "notfound")
		return
	}
	if s.deleteBlocked(r, row) {
		redirectAdminErr(w, "self")
		return
	}
	err := s.deps.Store.DeleteProfile(id)
	switch {
	case err == nil:
		redirectAdmin(w)
	case errors.Is(err, os.ErrNotExist):
		redirectAdminErr(w, "notfound")
	default:
		writeInternalError(w, err)
	}
}

// sessionProfileID 拿当前会话**正在用**的那一份（空串 = 没会话）。
//
// 2026-10-12 之前这段注释讲的是"管理员代管"：那时管理员进管理页时 Active 是自己，
// 切进别人档案管理时 Active 是别人，所以删档要挡的是 Active。代管态 2026-10-10 就
// 下线了，管理员也不再是某个档案，于是这里的形状变成**最朴素的那种**：登了哪一份就
// 是哪一份，没登就是空串，而空串那格谁也不挡。
func sessionProfileID(r *http.Request) string {
	return auth.ReadSession(r).Active
}

// rowLabel 是这一行在页面上叫什么：有显示名用显示名，没有就用 id。
// 两处（两个确认页 + 后果说明）都走它，免得一个地方写"爸爸"、另一个写 dad。
func rowLabel(row statusProfile) string {
	if strings.TrimSpace(row.Title) != "" {
		return row.Title
	}
	return row.ID
}
