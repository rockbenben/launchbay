package httpapi

import (
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

// profileDisabledResponse 是停用/启用两条端点共用的成功体。
// 用 struct 不用 map：键序由字段序固定（§4.3，pinChangedResponse 是同一条先例）。
type profileDisabledResponse struct {
	Profile  string `json:"profile"`
	Disabled bool   `json:"disabled"`
}

// disableResult 是"停用/启用"这个动作的结论。四个出口与 adminPinResult 同一族，
// 同一条理由：并成一个 error 会丢掉"落盘成了但审计行没落"这一个只有它自己
// 能说的话（§4.6 的审计线是"谁动过这台机器"的唯一证据）。
type disableResult struct {
	FieldErrs []fieldError
	ReadErr   error
	SaveErr   error
	HistErr   error
	Disabled  bool
}

// adminDisable 是停用/启用的**唯一落点**：校验 → 锁内读写 → 历史。
//
// 为什么它要有 API 端点而不只是管理页那一枚按钮：CLI 与 AI 也需要它
// （"这台机器上那个账号先停掉"是一条运维动作）。但**两个壳不许各写一遍判据**
// ——spec §8.8 对设 PIN 与删档都是这么裁定的，adminPin 已经把那坑踩过一次。
//
// 判据只有"当前档案在磁盘上是不是已经停用"这一条，目标由调用方决定停还是启用：
// 幂等（同状态再设一次不进锁、不写盘、不记审计），于是重试是安全的。
func (s *Server) adminDisable(r *http.Request, profile string, disabled bool) disableResult {
	var res disableResult
	if !model.ValidSlug(profile) {
		res.FieldErrs = []fieldError{{Kind: apierr.FieldInvalid, Field: "profile", Reason: "必须是 ASCII slug"}}
		return res
	}
	actor, _, ok := s.require(r, credAdmin, profile)
	if !ok {
		// 与 adminPin 同口径：权限类拒绝不能自成一个 FieldErr——那会让页面显示
		// "profile 不是 slug"，而真相是"你不该看见这一页"。由调用方直接答。
		res.FieldErrs = []fieldError{{Kind: apierr.CredentialAdminOnly, Field: "cred", Reason: "需要管理员会话"}}
		return res
	}
	// 锁的返回值在这一族是**有意忽略**的：结论全在 res 里（ReadErr/SaveErr/
	// HistErr 各自一位），WithProfileLock 自己只在回调返回非 nil 时把那个错
	// 传上来，而每个非 nil 都已经在 res 里有位置了。赋给一个没人读的局部变量
	// 只会诱使后来的人以为它参与了分诊。
	s.deps.Store.WithProfileLock(profile, func() error {
		p, err := s.deps.Store.LoadProfile(profile)
		if err != nil {
			res.ReadErr = err // 分诊在锁外做：404 / 503 / 500
			return err
		}
		if p.Disabled == disabled {
			res.Disabled = p.Disabled // 同状态：什么都不做，答案照给
			return nil
		}
		p.Disabled = disabled
		res.Disabled = disabled // 结论要跟着**这一次真的设成了什么**走，见下面那条注释
		res.SaveErr = s.deps.Store.SaveProfile(p)
		if res.SaveErr != nil {
			return nil
		}
		// Rev: 0 与设 PIN 同一条理由——rev 是书签计数器，停用不动书签。
		// path 用 profile/disabled：动的不是书签树。
		res.HistErr = s.deps.Store.AppendHistory(store.HistoryEntry{
			At: time.Now().Format(time.RFC3339), Profile: profile, Actor: actor,
			By: byOfRequest(r), Rev: 0,
			Ops: historyOps([]ops.Brief{{Op: "set", Path: "profile/disabled"}}),
		})
		return nil
	})
	return res
}

// writeAdminDisable 把落点的结论按 API 形状答出去。
//
// 历史追加失败的处理与 adminPin 一致（报 500，不报 200 + warning）：停用是幂等的，
// 重试写回同一个标志、把缺的那行补上，收敛而不是叠加；而"已经停了、审计线里没有"
// 必须让调用方知道。别把它改成 batch 那套"5xx 会诱使重试"的形状——那条理由
// 在这里不成立。
func (s *Server) writeAdminDisable(w http.ResponseWriter, profile string, res disableResult) {
	switch {
	case len(res.FieldErrs) > 0:
		writeFieldErrors(w, http.StatusUnprocessableEntity, res.FieldErrs)
	case res.ReadErr != nil:
		s.writeProfileReadFailure(w, profile, res.ReadErr)
	case res.SaveErr != nil:
		writeInternalError(w, res.SaveErr)
	case res.HistErr != nil:
		fmt.Fprintf(os.Stderr, "disable history append failed profile=%s: %v\n", profile, res.HistErr)
		writeInternalError(w, res.HistErr)
	default:
		writeJSON(w, http.StatusOK, profileDisabledResponse{Profile: profile, Disabled: res.Disabled})
	}
}

// handleAdminDisableAPI 是机器那一面（CLI / AI）：JSON 体，要求管理员。
//
// 为什么它带 `disabled` 字段而不是分 enable/disable 两条端点：停用与启用是
// **同一个动作的两头**，一个"改成这个状态"的端点对两边都好写，而两条端点会
// 让 `/api/v1/profiles/enable` 变成一条只在某个状态下才有意义的路。
func (s *Server) handleAdminDisableAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Profile  string `json:"profile"`
		Disabled bool   `json:"disabled"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	res := s.adminDisable(r, in.Profile, in.Disabled)
	if len(res.FieldErrs) > 0 && res.FieldErrs[0].Field == "cred" {
		s.writeAuthFailure(w, r)
		return
	}
	s.writeAdminDisable(w, in.Profile, res)
}

// disableSelfBlocked 是停用的**唯一形状闸**：不能停用自己正在用的那一份。
//
// 与 deleteBlocked 同一句判据（sessionProfileID），但**理由不同**：删档的理由是
// "把家删了"；停用的理由更轻——停用自己在用的那一份会立刻把自己关在门外，
// 而管理员随时能启回来，所以它只是防手滑，不是安全闸。
//
// 停用**管理员自己**是允许的（他可以停用自己那份、去看另一个管理员的台）——
// 那是账号层面的正常操作，不涉及任何人的书签。
func (s *Server) disableSelfBlocked(r *http.Request, row statusProfile) bool {
	return row.ID == sessionProfileID(r)
}

// handleAdminDisablePage 是第一步（GET）：读 ?profile=<id>，回一张写明后果的页面。
//
// **两步二次确认、零 JavaScript**，与设 PIN/删档同一形状（spec §8.8）。停用不可逆
// 吗？不是——数据一个字节不动，随时能启回来。但它同样要一道确认：人点错一下就把
// 家里人关在门外，而这一下点下去页面会自己跳走，事后才发现"我家怎么进不去了"。
func (s *Server) handleAdminDisablePage(w http.ResponseWriter, r *http.Request) {
	if !s.adminPageGuard(w, r) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	row, ok := s.statusRowOf(id)
	if !ok {
		redirectAdminErr(w, "notfound")
		return
	}
	if s.disableSelfBlocked(r, row) {
		redirectAdminErr(w, "selfdisable")
		return
	}
	label := rowLabel(row)
	var heading, explain, submit string
	disabled := !row.Disabled
	if disabled {
		heading = s.t(r, "admin.disable.heading.on", label)
		explain = s.t(r, "admin.disable.explain.on", label)
		submit = s.t(r, "admin.disable.submit.on")
	} else {
		heading = s.t(r, "admin.disable.heading.off", label)
		explain = s.t(r, "admin.disable.explain.off", label)
		submit = s.t(r, "admin.disable.submit.off")
	}
	page, err := s.deps.Render.AdminActionPage(render.AdminAction{
		ID: id, Title: s.t(r, map[bool]string{true: "admin.disable.title.on", false: "admin.disable.title.off"}[disabled]),
		Heading: heading, Explain: explain,
		Notice: s.notice(r, r.URL.Query().Get("err")),
		Action: "/admin/disable",
		Submit: submit,
	}, s.machineLang(r))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// handleAdminDisableForm 是第二步（POST）。判据与第一步同一份（adminDisable），
// 这一层只管怎么读入、怎么答出。
func (s *Server) handleAdminDisableForm(w http.ResponseWriter, r *http.Request) {
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
	if !model.ValidSlug(id) {
		redirectAdminErr(w, "notfound")
		return
	}
	row, ok := s.statusRowOf(id)
	if !ok {
		redirectAdminErr(w, "notfound")
		return
	}
	if s.disableSelfBlocked(r, row) {
		redirectAdminErr(w, "selfdisable")
		return
	}
	res := s.adminDisable(r, id, !row.Disabled)
	switch {
	case len(res.FieldErrs) > 0 && res.FieldErrs[0].Field == "cred":
		redirectPicker(w)
	case res.ReadErr != nil:
		s.writeProfileReadFailure(w, id, res.ReadErr)
	case res.SaveErr != nil, res.HistErr != nil:
		// 落盘或审计失败走 500 JSON 外壳——**绝不说成功**：停用可能已经生效而
		// 审计行没落，下一个人看到的是一份"没人动过"的审计线。
		if res.SaveErr != nil {
			writeInternalError(w, res.SaveErr)
		} else {
			fmt.Fprintf(os.Stderr, "disable history append failed profile=%s: %v\n", id, res.HistErr)
			writeInternalError(w, res.HistErr)
		}
	default:
		redirectAdmin(w)
	}
}
