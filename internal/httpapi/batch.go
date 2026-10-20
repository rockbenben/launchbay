package httpapi

import (
	"errors"
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

type batchRequest struct {
	Profile     string   `json:"profile"`
	ExpectedRev *int64   `json:"expectedRev,omitempty"`
	By          string   `json:"by"`
	Ops         []ops.Op `json:"ops"`
}

type batchResponse struct {
	Profile  string   `json:"profile"`
	Rev      int64    `json:"rev"`
	Applied  int      `json:"applied"`
	Warnings []string `json:"warnings"`
}

// historyOps 把 ops.Brief 换成磁盘上的 store.HistoryOp。ops 是纯函数层，
// 不许 import store，所以这层形状转换只有这一处。
func historyOps(briefs []ops.Brief) []store.HistoryOp {
	out := make([]store.HistoryOp, 0, len(briefs))
	for _, b := range briefs {
		out = append(out, store.HistoryOp{Op: b.Op, Path: b.Path})
	}
	return out
}

// handleBatch 是本案唯一改书签的入口（CLI 也都走它）。
//
// 实际顺序：锁 → 读 → 校验（expectedRev + ops.Run 内存应用）→ 快照 →
// 写（rev+1 落盘）→ 历史。spec §4.4 的"校验 → 快照 → 应用"里，"应用"
// 指内存改动，必须先于快照（失败的批次不该在 snapshots/ 里留一份
// "什么都没改"的副本，那会把 50 份的窗口填满足是无效现场），而快照必须
// 先于落盘——它保的是这次写入之前的状态。
func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	var in batchRequest
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if !model.ValidSlug(in.Profile) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: apierr.FieldInvalid, Field: "profile", Reason: "必须是 ASCII slug"}})
		return
	}
	// **缺 ops 键不是空批**（2026-10-20 补上）：空批给一个空数组（那是一次合法的
	// "什么都没写"），而**整个键都没有**是请求没写对——契约里 ops 是必填的，
	// 于是这里答 422 并点名它。
	if in.Ops == nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: apierr.FieldInvalid, Field: "ops",
			Reason: "不能省掉 ops（要空批就给一个空数组）"}})
		return
	}
	if !store.ValidBy(in.By) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: apierr.FieldInvalid, Field: "by",
			Reason: "只能是 " + strings.Join(store.ByValues, "/")}})
		return
	}
	actor, _, ok := s.require(r, credScoped, in.Profile)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	if s.profileDisabledRefuse(w, in.Profile) {
		return
	}

	out, err := s.applyBatch(in.Profile, actor, in.By, in.ExpectedRev, in.Ops)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writeBatchOutcome(w, in.Profile, out)
}

// batchOutcome 是"一批 ops 打到一个档案"这件事的全部结论。
//
// 分成"结论"与"怎么答出去"两半，是因为**答的方式有两个**：API 按状态码与
// JSON 体答（handleBatch），网页上的浏览器收藏夹导入按页面与 303 答
// （importapi.go）。而"发生了什么"只有一份——两份实现就是两份真相，
// 而快照/历史/rev 这三件事必须在同一个锁内同批发生。
type batchOutcome struct {
	resp     batchResponse
	conflict *int64
	verrs    []ops.Error
	missing  bool
	broken   string
	snapErr  error
}

// applyBatch 是**书签写入的唯一落点**：锁 → 读档案 → 判 expectedRev →
// ops.Run 内存应用 → 写快照 → 落盘（rev+1）→ 记审计线。
//
// 2026-10-10 起它不再只服务于 POST /api/v1/batch：浏览器收藏夹导入
// （importapi.go）也走它，那条路把 bookimport 转出来的 ops 递进来。
// **不许在导入那条路上另写一份**——快照要先于落盘、失败批次不该留快照、
// 历史失败要降级成 warning 而不是 5xx，这三条各自都有过血的判据，
// 抄一遍就要抄三遍。
func (s *Server) applyBatch(profile, actor, by string, expectedRev *int64, opList []ops.Op) (batchOutcome, error) {
	var (
		out      batchOutcome
		conflict *int64
		verrs    []ops.Error
		missing  bool
		broken   string
		snapErr  error
	)
	err := s.deps.Store.WithProfileLock(profile, func() error {
		snap, err := s.deps.Store.Current(profile)
		switch {
		case errors.Is(err, store.ErrNotFound):
			missing = true
			return nil
		case err != nil:
			// 503 只由**本次**读返回的错误派生（最终整枝评审 #3）：
			// 真 I/O 错不是 ErrCorrupt，不许被登记表里的陈旧记录答成
			// "503 + 坏档原因"——那会把调用方支去修一个没坏的文件。
			if errors.Is(err, store.ErrCorrupt) {
				broken = corruptDetail(s.deps.Store, profile, err)
				return nil
			}
			return err
		}
		if expectedRev != nil && *expectedRev != snap.Rev {
			c := snap.Rev
			conflict = &c
			return nil
		}
		if len(opList) == 0 {
			out.resp = batchResponse{Profile: profile, Rev: snap.Rev, Applied: 0, Warnings: []string{}}
			return nil
		}
		next, errs, warns := ops.Run(snap.B, opList, s.deps.Icons)
		if len(errs) > 0 {
			verrs = errs
			return nil
		}
		// 快照错误要分诊，不能一律当 507：rev>0 说明这份快照已经落地，
		// 只是裁剪失败——按"快照成功"继续写，把裁剪错误降级成告警；
		// rev==0 且报错才是真写不下去 → 507 取消整个写入。store 侧的
		// 契约本来就是这么写的（snapshot.go：裁剪失败照报但 rev 仍然有效，
		// 调用方按"快照成功"处理），这里不照做就会把"磁盘还剩一点空间"
		// 报成"写不进去"。注意必须给外层 snapErr 赋值（普通 =）：闭包里
		// `snapRev, snapErr :=` 会新建同名变量遮蔽外层，507 分支就死了。
		var snapRev int64
		snapRev, snapErr = s.deps.Store.WriteSnapshot(profile)
		if snapErr != nil && snapRev == 0 {
			return nil // 真写不下去：snapErr 已置位，收尾按 507 处理
		}
		if snapErr != nil {
			// 降级成告警继续写（rev 仍然有效），但**必须同时打进 stderr**——与下面
			// 那句 `history append failed` 同一处置：这条告警唯一的去处是响应体里的
			// `warnings`，而浏览器那条路压根不读它（app.js 一个 warnings 都没接），
			// 于是"裁剪一直在失败、快照目录一直涨"就只剩容器日志这一处痕迹。
			// 症状本身没有屏上出口：写照写、页照看，磁盘悄悄变胖。
			warns = append(warns, "旧快照裁剪失败: "+snapErr.Error())
			fmt.Fprintf(os.Stderr, "snapshot prune failed profile=%s: %v\n", profile, snapErr)
			snapErr = nil
		}
		next.Rev = snap.Rev + 1
		if err := s.deps.Store.SaveBookmarks(profile, next); err != nil {
			// 快照成功而落盘失败会留下一份与盘上现状一致的快照：接受，
			// 代价只是恢复窗口里多一个"什么都没改"的现场；不为此建孤儿
			// 快照清理机制（controller 裁定）。
			return err
		}
		out.resp = batchResponse{Profile: profile, Rev: next.Rev, Applied: len(opList), Warnings: warns}
		if out.resp.Warnings == nil {
			out.resp.Warnings = []string{}
		}
		histErr := s.deps.Store.AppendHistory(store.HistoryEntry{
			At:      time.Now().Format(time.RFC3339),
			Profile: profile,
			Actor:   actor,
			By:      by,
			Rev:     next.Rev,
			Ops:     historyOps(ops.Describe(opList)),
		})
		if histErr != nil {
			// 书签已经写成功了。这时候报 5xx 会诱导调用方重试、把同一批
			// op 再应用一次——那才是真正的数据事故。所以：照实返回成功，
			// 历史缺失进 warnings 并打到 stderr。审计线少一行不该让
			// "没人敢用这个接口"。
			out.resp.Warnings = append(out.resp.Warnings, "历史没写进去（书签已写入）")
			fmt.Fprintf(os.Stderr, "history append failed profile=%s: %v\n", profile, histErr)
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	out.conflict, out.verrs, out.missing, out.broken, out.snapErr = conflict, verrs, missing, broken, snapErr
	return out, nil
}

// writeBatchOutcome 把落点的结论按 API 形状答出去。
func (s *Server) writeBatchOutcome(w http.ResponseWriter, profile string, out batchOutcome) {
	switch {
	case out.missing:
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.ProfileNotFound, Detail: "档案不存在"})
	case out.broken != "":
		// 坏档案不是 500（不是服务的错）也不是 422（不是这次请求的错）。
		// 503 + store 给的文件名与行号：AI 拿着它能直接去修那个文件。
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Code: 503, Kind: apierr.ProfileBroken, Detail: out.broken})
	case out.conflict != nil:
		writeJSON(w, http.StatusConflict, errorResponse{Code: 409, Kind: apierr.RevConflict, Detail: "rev 已变，请重读后重试", CurrentRev: out.conflict})
	case len(out.verrs) > 0:
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Code: 422, Kind: apierr.OpsInvalid, Detail: "ops 校验未过，整批未写入", Errors: opFieldErrors(out.verrs)})
	case out.snapErr != nil:
		writeJSON(w, http.StatusInsufficientStorage, errorResponse{Code: 507, Kind: apierr.SnapshotFailed, Detail: "快照写不下去，本次写入已取消"})
	default:
		writeJSON(w, http.StatusOK, out.resp)
	}
}

// writeAuthFailure 把"没认出来"与"认出来但没资格"分开：前者 401，后者 403。
// 合并成 403 会让调用方拿着一个死令牌反复重试——它收到的那句"作用域不含
// 该档案"在骗它（Task 5 评审的 Important）。判据就是 require 用过的同一份
// 认凭证逻辑：没认出来才轮到会话。
//
// **管理员那把 key 也要算"认出来了"**（2026-10-12）。它不在 Tokens 列表里了，
// 于是只查 Tokens 的话，一把完全有效的管理员 key 打只认会话的端点会拿到 401
// "令牌无效或已吊销"——那是在说一把好钥匙是废的。调用方会照着这句话去换令牌，
// 而真相是"这把钥匙不带这个门"。
func (s *Server) writeAuthFailure(w http.ResponseWriter, r *http.Request) {
	if tok := bearer(r); tok != "" {
		if _, ok := s.deps.Tokens.Authorize(tok); !ok && !s.deps.AdminToken.Matches(tok) {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Kind: apierr.CredentialUnknown, Detail: "令牌无效或已吊销"})
			return
		}
		writeJSON(w, http.StatusForbidden, errorResponse{Code: 403, Kind: apierr.CredentialScope, Detail: "凭证作用域不含该档案"})
		return
	}
	// 只拿得出管理员 cookie、拿不出档案会话时，那也**不是**"没有凭证"：
	// 他确实有凭证，只是不带这个端点。说成 401 会让他以为自己没登录。
	if !auth.ReadSession(r).Valid() && !s.hasAdminCookie(r) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Kind: apierr.CredentialMissing, Detail: "需要令牌或会话"})
		return
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Code: 403, Kind: apierr.CredentialScope, Detail: "凭证作用域不含该档案"})
}

// profileDisabledRefuse 判"这一份被停用了"，并把那条拒绝写出去。
//
// **为什么令牌通路也要它**（2026-10-20 补上）：浏览器那条 PIN 门早就在拒停用的档案
// （handlers.go 的 disabled 分支），而档案作用域令牌却照读照写——于是"停用"对拿着
// 令牌的人（浏览器扩展、脚本）是**空的**：扩展里那句「这一份被停用了，写不进去」
// 一次都不会出现。
//
// **管理员不受这条约束**（它走 credAdmin）：停用要能被启回来，而管理员管的正是
// 账号层面这一族。
//
// 读不出来的那一份**不在这里分诊**（各有各的 503/坏档理由）——这里只管"读得动、
// 而它被停用了"这一种。
func (s *Server) profileDisabledRefuse(w http.ResponseWriter, id string) bool {
	p, err := s.deps.Store.LoadProfile(id)
	if err != nil || !p.Disabled {
		return false
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Code: 403, Kind: apierr.ProfileDisabled, Detail: "这一份被停用了"})
	return true
}

// errorResponse 是全 API 错误响应的唯一外壳：struct 字段序 = 输出键序
// （§4.3 那条"禁止用 map 产生输出键序"对 API 响应同样成立——map 会被 Go
// 按字母序排，{code, detail, currentRev} 发出去就成了 code, currentRev,
// detail），也让一个端点的多种错误不会漂成两种形状（最终整枝评审 #2：
// writeFieldErrors/writeInternalError 已折进这里，422 不再有"带不带
// detail"两种外壳）。
type errorResponse struct {
	Code int `json:"code"`
	// Kind 是**机器可读**的错误分类（internal/apierr 那一族常量）。状态码不够分：
	// 422 既可能是"那一组没了"也可能是"这一条已经存过了"，两句出路完全不同。
	// detail 仍是给人排查的中文原话，两边各管各的读者。
	Kind       string       `json:"kind"`
	Detail     string       `json:"detail"`
	CurrentRev *int64       `json:"currentRev,omitempty"`
	Errors     []fieldError `json:"errors,omitempty"`
}

// opFieldErrors 把 ops.Error 换成响应条目。OpsIndex 用指针透传：
// opsIndex:0 说的是"第 0 条 op"，裸值加 omitempty 会把它吞成"没指认"。
func opFieldErrors(errs []ops.Error) []fieldError {
	out := make([]fieldError, 0, len(errs))
	for _, e := range errs {
		idx := e.OpsIndex
		out = append(out, fieldError{OpsIndex: &idx, Kind: e.Kind, Field: e.Field, Reason: e.Reason})
	}
	return out
}
