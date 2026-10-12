package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

// handleRebuildProfile 修一份**读不动**的 profile.json（spec §6.1、§12）。
//
// 这条端点存在的唯一理由是：坏档**原本没有出路**。`bookmarks.json` 坏了能用
// revert 或快照救回来，而 `profile.json` 坏了——profiles/pin 与 profiles/disable
// 都要先读它（503）、POST /api/v1/profiles 又会 409（且 CreateProfile 会把那份
// **还能读的书签**一起覆盖掉）。没有这一条，界面那句"要在这台机器上修好那个文件"
// 指的就是一条不存在的路，只剩手工改磁盘——而手工路线明令禁止。
//
// 三条判据都在 store 的同一次按 id 的临界区里（`RebuildProfile`），handler 一律
// 不许自己先 LoadProfile 看一眼再决定写不写：两次读之间档案被人修好，这里就会
// 盖掉一份读得动的文件，而这正是本端点唯一的危险。与 CreateFirstProfile 把"磁盘
// 为空"搬进锁内是同一条纪律（§4.5）。
//
// **不许再包一层 WithProfileLock**：RebuildProfile 自己拿锁，sync.Mutex 不可重入，
// 包一层就是当场死锁（store/rebuild.go 的【重入陷阱】）。历史行在锁外追——
// AppendHistory 是 O_APPEND 的只增追加，本来就不与任何读共用状态。
func (s *Server) handleRebuildProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Profile string `json:"profile"`
		Title   string `json:"title"`
		PIN     string `json:"pin"`
		// **没有 admin 那一格**（2026-10-12）。它是三格必填的最后一格，删掉之后
		// 这条端点的必填集就是 profile/title/pin 三格——不需要指针，因为再没有
		// "给了 false 与没给"要区分的那一格。
		//
		// 旧脚本送来 admin 会撞 decodeBody 的未知字段闸 → 422 且点名 admin：
		// 那正是它该收到的答案（"这一格没有了"），而不是静默忽略。
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	// 访问判定交回 require 这一份唯一判据，顺序与 batch / handleCreateProfile 同口径：
	// 先解码（形状错 422），再申报进 require（凭证错 401/403）。
	actor, _, ok := s.require(r, credAdmin, in.Profile)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	var errs []fieldError
	if !model.ValidSlug(in.Profile) {
		errs = append(errs, fieldError{Field: "profile", Reason: "必须是 ASCII slug（小写字母、数字、连字符）"})
	}
	if strings.TrimSpace(in.Title) == "" {
		errs = append(errs, fieldError{Field: "title", Reason: "不能为空"})
	}
	hash, pinErrs := hashFormPIN(in.PIN) // PIN 判据只有一份：建档族共用（createrule_test.go 第三档钉着）
	errs = append(errs, pinErrs...)
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, errs)
		return
	}
	p, err := s.deps.Store.RebuildProfile(store.RebuildInput{
		ID: in.Profile, Title: in.Title, PinHash: hash,
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		// 这条端点**不建目录**：建档案仍然只有 POST /api/v1/profiles 与 POST /setup。
		// 说清这一点是为了让人别把它当"重建"的第二条路——目录不在就是不在。
		writeJSON(w, http.StatusNotFound, errorResponse{
			Code: 404, Detail: "档案不存在。这条通路只重写 profile.json，建不出新档案（建档走 POST /api/v1/profiles）"})
	case errors.Is(err, store.ErrReadable):
		// 409 而不是 403/404：档案在、也读得动，只是**不需要**重建。改 PIN 请走
		// profiles/pin——它要的是新 PIN，不是整份重写（这正是两条端点不许合并的
		// 那条理由，profile-repair 设计 §0）。
		writeJSON(w, http.StatusConflict, errorResponse{
			Code: 409, Detail: "这份档案读得动，不需要重建；要改 PIN 请走 POST /api/v1/profiles/pin"})
	case errors.Is(err, store.ErrInvalid):
		// store 的错误文本含磁盘路径，与 500 同一理由不回显。
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "profile", Reason: "字段不合法，重建没成"}})
	case err != nil:
		// 落盘失败与"写完回读仍读不动"都在这里（回读失败是 500，且坏档登记留着
		// ——那正是"别把一个已经修好的文件说成没好"的要求）。
		writeInternalError(w, err)
	default:
		// 审计行：重建动的是 profile.json 而不是书签树，所以 Rev: 0、path 用
		// profile/rebuild（§4.6 的记账口径，与 profiles/pin 的 profile/pin 同一族）。
		//
		// **审计行写不下去时照报 200**，与 pinapi 的 500 刻意不同，理由写在这里免得
		// 下一个人"顺手改成一致"：pinapi 那条的理由是"设 PIN 幂等，重试收敛"，
		// 而重建**不幂等**——重试会拿到 409（档案已经读得动了）。报 500 等于对
		// 一个已经修好的档案说"没修好"，把人重新引回手工改磁盘那条死路，而真实
		// 缺口只是少了一行审计。缺口进 stderr 与容器日志。
		if histErr := s.deps.Store.AppendHistory(store.HistoryEntry{
			At: time.Now().Format(time.RFC3339), Profile: p.ID, Actor: actor,
			By: byOfRequest(r), Rev: 0,
			Ops: historyOps([]ops.Brief{{Op: "set", Path: "profile/rebuild"}}),
		}); histErr != nil {
			fmt.Fprintf(os.Stderr, "rebuild history append failed profile=%s: %v\n", p.ID, histErr)
		}
		// 成功体与建档同一形状：键序 id, title, admin 由字段序固定（§4.3）。
		// 200 的含义与 spec §6.1 一致——"那张卡真的回来了"，因为 RebuildProfile
		// 在同一把锁内回读过，返回的就是盘上那份。
		writeJSON(w, http.StatusOK, createProfileResponse{ID: p.ID, Title: p.Title})
	}
}
