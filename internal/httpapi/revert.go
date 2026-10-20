package httpapi

import (
	"errors"
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

// revertResponse 用 struct 不用 map：键序由字段序决定（§4.3 的写出契约
// 对 API 响应同样成立，batch.go 的 errorResponse 是同一条裁定的先例）。
type revertResponse struct {
	Profile      string `json:"profile"`
	Rev          int64  `json:"rev"`
	RestoredFrom int64  `json:"restoredFrom"`
}

// isUnreadable 说的是"这份数据文件现在读不了"：ErrCorrupt/ErrInvalid 是
// store 解码失败的归口，ErrNotExist 是文件整个没了。只有 revert 用它
// （batch 对坏档案报 503，不是"忽略并继续写"），所以不放进 store。
func isUnreadable(err error) bool {
	return errors.Is(err, store.ErrCorrupt) || errors.Is(err, store.ErrInvalid) || errors.Is(err, os.ErrNotExist)
}

// byOfRequest 读 X-LB-By 自报渠道。它只喂历史的 by 字段（spec §4.6：
// 自报口径，不参与任何权限判定），所以非法值不值得一个 422——退回 web
// 并把原值打进 stderr，界面上少一个筛选标签比拒掉一次恢复便宜。
func byOfRequest(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("X-LB-By"))
	if raw == "" {
		return "web"
	}
	if !store.ValidBy(raw) {
		fmt.Fprintf(os.Stderr, "忽略非法的 X-LB-By %q，按 web 记\n", raw)
		return "web"
	}
	return raw
}

// revertOutcome 是 revert 那次写入的结果。**落点层不认 HTTP**：同一次写入有两个
// 消费者（API 说 JSON、浏览器表单说 303），而"能不能退、退成了没有"这些判据
// 必须在同一个地方算一次——复制一份到表单那条路上，两份迟早只在一处修。
//
// 这条纪律与管理页那三张两步确认页同源：/admin/pin 的落点 adminPin 返回结构化
// 结果，API 与表单各答各的。
type revertOutcome struct {
	NewRev       int64
	RestoredFrom int64
	// NotFound 是"档案不在，或那一版的快照不在"。**只有这一道形状闸**：
	// 快照目录名就是 rev，所以一个不是快照的 rev 自然查不到，而"页面上给过
	// 的那个号"到 POST 时可能已经被裁剪掉（保留 50 份）——两种都归它，
	// 页面上分别说"没选"与"已经不在了"。
	NotFound bool
	// SnapFailed 是"当前这一版存不下来，本次恢复已取消"（API 那边的 507）。
	// 与 NotFound 分开：书签一个字都没变，值得说清是哪一种。
	SnapFailed bool
	Done       bool // 真的写下去了。false 且无错是"不该发生的形状"，API 那条据此报 500
}

// revertLocked 是 revert 的**唯一落点**：档案锁内完成快照→写盘→审计，
// 调用方拿到的是"发生了什么"而不是 HTTP 状态。
//
// actor 与 by 都由调用方给：API 那条 actor 是令牌名或会话档案（require 的
// 返回值），表单那条是会话自己。判据在端点层，落点不猜。
//
// 它**是一次新写入**：rev 前进、不抹历史（spec §4.6）。回滚成"把 rev 改回旧值"
// 会让 expectedRev 语义失效，也会让两次 revert 互相覆盖。
//
// 坏档案（bookmarks.json 读不了）恰恰是最需要 revert 的时刻，所以这里
// 不走 Current()：直接读快照，当前状态只在"能做快照时"做快照。
func (s *Server) revertLocked(id, actor string, rev int64, by string) (revertOutcome, error) {
	var out revertOutcome
	err := s.deps.Store.WithProfileLock(id, func() error {
		// 档案不存在才 404；profile.json 坏了**继续恢复**——revert 只回
		// 书签（§4.6），而坏档正是最需要这条通路的时候。
		if _, err := s.deps.Store.LoadProfile(id); errors.Is(err, store.ErrNotFound) {
			out.NotFound = true
			return nil
		}
		raw, err := s.deps.Store.LoadSnapshot(id, rev)
		if errors.Is(err, store.ErrNotFound) {
			out.NotFound = true
			return nil
		}
		if err != nil {
			// 快照在但读不出字节（I/O 错）不是"不存在"：别把它粉饰成 404。
			return err
		}
		b, err := store.DecodeBookmarks(s.deps.Store.BookmarksPath(id), raw)
		if err != nil {
			// 快照本身坏了：报 500（这是真的内部错误），不假装恢复成功。
			// 错误里点名是哪份快照——裸解码错误只说 bookmarks.json，
			// 而坏的是 snapshots/<rev>/bookmarks.json。
			return fmt.Errorf("profile %s 的快照 rev %d 读不了: %w", id, rev, err)
		}
		// 当前状态先保住——revert 也是一次写入，写坏了还得能回来。
		// 分诊与 Task 5 的 batch 同一口径：裁剪失败返回 rev>0 并带错，
		// 快照本身已落地，按成功继续、错误降级进 stderr（不对称处：
		// batch 把它放进响应 warnings，revertResponse 没有那个字段，
		// 只进 stderr——Task 11 的文档要提到这一差）；rev==0 且报错
		// 才是真写不下去 → 取消本次恢复。档案是坏档案时 WriteSnapshot
		// 必然失败（读不了就保不了），那种情况下**继续恢复**：坏档没有
		// 值得保的东西，而 revert 正是出路。内层错误用另一个名字：
		// 闭包里 `snapRev, snapErr :=` 会遮蔽外层同名变量，507 分支
		// 就死了（batch.go 钉过的同一课）。
		snapRev, snapWriteErr := s.deps.Store.WriteSnapshot(id)
		switch {
		case snapWriteErr != nil && snapRev > 0:
			fmt.Fprintf(os.Stderr, "launchbay: revert 前旧快照裁剪失败 profile=%s: %v\n", id, snapWriteErr)
		case snapWriteErr != nil && !isUnreadable(snapWriteErr):
			out.SnapFailed = true
			return nil // 真写不下去：Out 已置位，收尾按"已取消"处理
		}
		cur, curErr := s.deps.Store.CurrentRev(id)
		if curErr != nil && !isUnreadable(curErr) {
			return curErr
		}
		// 新 rev 必须大于三者：盘上当前 rev、被恢复内容的 rev、
		// 最高 retained 快照的 rev。坏档下 CurrentRev 只能报 0，只保
		// 前两者会产出快照已经用过的号——下一次写入的 WriteSnapshot
		// 按内容 rev 命名目录，会**静默覆盖**同号快照，磁盘上两份不同
		// 内容共用一个 rev、历史里两行引用同一个号。ListSnapshots 按
		// 降序排，snaps[0] 就是最高 retained；prune 只删最旧的，所以
		// 这个界只会偏高不会偏低，作下限是可靠的。
		if b.Rev > cur {
			cur = b.Rev
		}
		snaps, err := s.deps.Store.ListSnapshots(id)
		if err != nil {
			return err
		}
		if len(snaps) > 0 && snaps[0] > cur {
			cur = snaps[0]
		}
		b.Rev = cur + 1
		if err := s.deps.Store.SaveBookmarks(id, b); err != nil {
			return err
		}
		out.NewRev, out.RestoredFrom = b.Rev, rev
		// 历史追加在档案锁**内**（spec §4.4 锁定写流程的一步；batch.go
		// 同款）：先放锁再追加，两个同档案写可能按 rev 顺序落盘、按逆序
		// 追加，审计线最新一行就对不上盘上最新 rev。取舍不变：书签已写
		// 成功 ⇒ 照旧 200，追加失败进 stderr（报 5xx 会诱导重复 revert）。
		histEntry := store.HistoryEntry{
			At: time.Now().Format(time.RFC3339), Profile: id, Actor: id,
			By: by, Rev: out.NewRev,
			// revert 不是一条书签 op，历史里自成一式（brief 的 spec 缺口取法）。
			Ops: historyOps([]ops.Brief{{Op: "revert", Path: "snapshots/" + strconv.FormatInt(rev, 10)}}),
		}
		if err := s.deps.Store.AppendHistory(histEntry); err != nil {
			fmt.Fprintf(os.Stderr, "history append failed profile=%s: %v\n", id, err)
		}
		out.Done = true
		return nil
	})
	return out, err
}

// parseRevertRev 读 to-rev。**表单与 API 共用**：两边对"这个号是什么形状"的
// 说法只有一份，页面上给的和机器发的是同一种数。
//
// 空串与非整数都是"形状不对"，负数单独一条：它不是"查不到"，是参数就错了，
// 不该靠"目录恰好不存在"兜成 404。
func parseRevertRev(raw string) (int64, bool) {
	rev, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || rev < 0 {
		return 0, false
	}
	return rev, true
}

// handleRevert 从快照恢复（机器那条通路）。答法见 revertResponse 的形状。
//
// to-rev=0 的取法与 CLI 不同，这是刻意的：CreateProfile 会写 rev 0 的
// bookmarks.json，snapshots/0 因此可以是**真实快照**；端点不特殊拒绝 0，
// 有快照 0 就恢复它、没有就 404（LoadSnapshot 是诚实的查表）。WriteSnapshot
// 的 (0,nil)"无东西可保"哨兵只在 err != nil 的分诊里被读，与恢复哪一份无关。
// "--to 0 是用法错误"只属于 CLI（人敲 0 多半是手误，机器调用交给 404 说话）。
func (s *Server) handleRevert(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.QueryInvalid, Detail: "要带 ?profile=<id>"})
		return
	}
	if !model.ValidSlug(id) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.FieldInvalid, Detail: "profile 不是 ASCII slug"})
		return
	}
	rev, ok := parseRevertRev(r.URL.Query().Get("to-rev"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.QueryInvalid, Detail: "to-rev 要是非负整数"})
		return
	}
	actor, _, ok := s.require(r, credScoped, id)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	if s.profileDisabledRefuse(w, id) {
		return
	}
	out, err := s.revertLocked(id, actor, rev, byOfRequest(r))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	switch {
	case out.NotFound:
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.MissingTarget, Detail: "档案或该 rev 的快照不存在"})
	case out.SnapFailed:
		writeJSON(w, http.StatusInsufficientStorage, errorResponse{Code: 507, Kind: apierr.SnapshotFailed, Detail: "快照写不下去，本次恢复已取消"})
	case out.Done:
		writeJSON(w, http.StatusOK, revertResponse{Profile: id, Rev: out.NewRev, RestoredFrom: out.RestoredFrom})
	default:
		writeInternalError(w, errors.New("revert 没有产出任何结果"))
	}
}
