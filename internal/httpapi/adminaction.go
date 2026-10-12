package httpapi

import (
	"errors"
	"fmt"
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

// handleAdminPinPage 是设 PIN 的第一步：显示目标档案与后果，让人输入新 PIN。
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
	case row.ID != "" && row.ID == sessionProfileID(r):
		// 改自己的 PIN 要**旧 PIN**（家人共用设备，改自己的那一份属于身份变更）。
		// 管理页这条路不要旧 PIN，所以它必须让开——否则这里就成了绕过旧 PIN 的后门。
		//
		// 判据 2026-10-12 变了：管理员不再是某个档案，于是"这是不是我自己"变成
		// "我此刻是不是正登在某一份里"。没登任何档案的管理员（sessionProfileID
		// 为空）不受这条限制——他不是在改自己的密码，是在改别人的。
		redirectAdminErr(w, "selfpin")
		return
	}
	page, err := s.deps.Render.AdminActionPage(render.AdminAction{
		ID: id, Title: s.t("admin.pin.title"), Heading: s.t("admin.pin.heading", rowLabel(row)),
		// 档案名两侧要有引号：从前写成"进 妈妈的台 这一份"，名字裸粘在句中读起来
		// 像漏了字。人称也换掉——"他本人""你得"假定了对方的性别与身份，
		// 而管理员未必是家长。
		Explain: s.t("admin.pin.explain", rowLabel(row)),
		Notice:  s.notice(r.URL.Query().Get("err")),
		Action:  "/admin/pin",
		Submit:  s.t("admin.pin.submit"),
		Fields:  []render.AdminField{{Name: "new", Label: s.t("admin.pin.field"), Type: "password", Autofocus: true}},
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// handleAdminPinForm 是第二步：走 API 同一个落点（adminPin），**不自写判据**。
func (s *Server) handleAdminPinForm(w http.ResponseWriter, r *http.Request) {
	if !s.adminPageGuard(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	if n := len(r.Form["new"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "new", Reason: "同一个字段只能出现一次"}})
		return
	}
	id := strings.TrimSpace(r.FormValue("profile"))
	res := s.adminPin(r, id, r.FormValue("new"))
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
		ID: id, Title: s.t("admin.delete.title"), Heading: s.t("admin.delete.heading", rowLabel(row)),
		// 页面上不该出现 markdown 的反引号与磁盘文件名：`revert` 与 history.jsonl
		// 是给读代码的人看的词，家里人在浏览器里看到只会愣一下。
		Explain: s.t("admin.delete.explain", row.Items, row.Snapshots),
		Notice:  s.notice(r.URL.Query().Get("err")),
		Action:  "/admin/delete",
		Submit:  s.t("admin.delete.submit"),
		Danger:  true,
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// deleteBlocked 是删档唯一的形状闸。
//
// **只有一道，没有"最后一个管理员"那一道**——这不是漏写，是它在本案的权限模型下
// **不可达**：require(credAdmin) 判的是 `activeIsAdmin`（只看 lb_profile 指向的
// 档案，routes.go 的 activeIsAdmin 注释里写了为什么只看它）。于是能进管理页的人
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
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	if n := len(r.Form["profile"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "profile", Reason: "同一个字段只能出现一次"}})
		return
	}
	id := strings.TrimSpace(r.FormValue("profile"))
	if !model.ValidSlug(id) {
		redirectAdminErr(w, "notfound")
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
