package httpapi

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// targetProfile 从 query 取 profile 并过作用域。缺 profile 是 400：
// "读哪个档案"没有默认值（spec §6.1 明写不回落）。
func (s *Server) targetProfile(w http.ResponseWriter, r *http.Request, want cred) (string, bool) {
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Detail: "要带 ?profile=<id>"})
		return "", false
	}
	if !model.ValidSlug(id) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Detail: "profile 不是 ASCII slug"})
		return "", false
	}
	if _, _, ok := s.require(r, want, id); !ok {
		s.writeAuthFailure(w, r)
		return "", false
	}
	return id, true
}

// dataResponse 用 struct 不用 map：键序由字段序决定（§4.3 的写出契约
// 对 API 响应同样成立，AI 会 diff 它）。
type dataResponse struct {
	Profile  string           `json:"profile"`
	Rev      int64            `json:"rev"`
	Modified string           `json:"modified"`
	Data     *model.Bookmarks `json:"data"`
}

func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	id, ok := s.targetProfile(w, r, credScoped)
	if !ok {
		return
	}
	snap, err := s.deps.Store.Current(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "档案不存在"})
	case err != nil:
		// 503 由本次错误派生（最终整枝评审 #3）：登记表里有记录不等于
		// "这次读失败是数据坏了"——真 I/O 错必须走 500，不能被陈旧
		// 记录答成指错文件的 503。
		if errors.Is(err, store.ErrCorrupt) {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{
				Code: 503, Detail: corruptDetail(s.deps.Store, id, err)})
			return
		}
		writeInternalError(w, err)
	default:
		writeJSON(w, http.StatusOK, dataResponse{
			Profile:  id,
			Rev:      snap.Rev,
			Modified: snap.Modified.UTC().Format(time.RFC3339),
			Data:     snap.B,
		})
	}
}
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	id, ok := s.targetProfile(w, r, credScoped)
	if !ok {
		return
	}
	if _, err := s.deps.Store.LoadProfile(id); errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "档案不存在"})
		return
	}
	raw, err := os.ReadFile(s.deps.Store.BookmarksPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// 空档案没有 bookmarks.json。给一份"合法的空"而不是 404：
			// 调用方要的是能喂回 batch 的东西。
			raw = []byte("{}\n")
		} else {
			writeInternalError(w, err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`-bookmarks.json"`)
	_, _ = w.Write(raw)
}

// historyResponse 同样用 struct：{entries} 只有一个键，但 map 一旦用上
// 就等着被加第二个键然后把顺序交出去（§4.3）。
type historyResponse struct {
	Entries []store.HistoryEntry `json:"entries"`
}

func (s *Server) handleHistoryAPI(w http.ResponseWriter, r *http.Request) {
	// ?profile= 可缺，但缺了不等于"什么都能看到"：见下面 want 的取值。
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	if id != "" && !model.ValidSlug(id) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Detail: "profile 不是 ASCII slug"})
		return
	}
	limit := 0
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Detail: "limit 要是非负整数"})
			return
		}
		limit = n
	}
	// 不填 profile = 要全家庭审计线，那是**跨档案聚合信息**，只给管理员
	// （spec §6.1 对 /status 的定位同样管这里）。留空当"不申报"从而跳过
	// 作用域检查，等于任何 kid 会话都能读到别人的动作线（Task 6 评审的
	// Critical）。申报了就按申报的那个档案走常规作用域判定。
	want := credScoped
	if id == "" {
		want = credAdmin
	}
	if _, _, ok := s.require(r, want, id); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	entries, err := s.deps.Store.ReadHistory(id, limit)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if entries == nil {
		entries = []store.HistoryEntry{}
	}
	writeJSON(w, http.StatusOK, historyResponse{Entries: entries})
}
