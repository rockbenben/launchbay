package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/rockbenben/launchbay/internal/apierr"
	"github.com/rockbenben/launchbay/internal/ops"
)

// handlePrefsTabs 是台面面板里「点开书签」那一行：**点一条书签是在当前标签页，
// 还是新开一个**（`Bookmarks.OpenInNewTab`）。
//
// **为什么它不住在 `POST /prefs` 的字段名单里**：那一格改的是台面偏好，落点是
// profile.json（不产生 rev、不做快照）；而这一个住在**书签文档**里——它跟着导出/
// 导入走，改它要过 rev 与快照，于是能在「回退」里退回来。所以它走 applyBatch 那条
// 书签写通路（与 `/prefs/group` 同一份收尾，applyPrefsOps）。
//
// 顺带解掉 2026-10-10 记下的那笔账：那一轮实测"档案级字段走 batch 改不了，六条
// 全拒"，于是界面上一格都没画出来——其中 `openInNewTab` 恰好是**页面上真的用着**
// 的一格（`<a target="_blank">`）。2026-10-14 ops 把那道粗闸换成根级标量白名单
// （见 internal/ops/path.go 的 docFields），这一行才有落点。
//
// **只认 `tabs=0|1`**：这一格是二值开关，把 "1"/"yes"/"true" 都当 1 收下，等于
// 让人以为按了而其实没按（枚举写侧严，AGENTS.md 那条枚举闸就是为这个）。
func (s *Server) handlePrefsTabs(w http.ResponseWriter, r *http.Request) {
	id, ok := s.selfProfileOnly(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "这份表单读不出来"}})
		return
	}
	raw, present := r.Form["tabs"]
	if !present || len(raw) != 1 || (raw[0] != "0" && raw[0] != "1") {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
			{Kind: apierr.FieldInvalid, Field: "tabs", Reason: "要带 tabs，且只能是 0（当前标签页）或 1（新标签页）"},
		})
		return
	}
	v := raw[0] == "1"
	s.applyPrefsOps(w, r, id, []ops.Op{{
		Op:    "set",
		Path:  "openInNewTab",
		Value: json.RawMessage(strconv.FormatBool(v)),
	}})
}

// 这一格与「外部协议」那一行的分工（2026-10-14 实测）：`chrome://` 与 `file://`
// 的条目**点不动**是浏览器的边界而不是我们的取舍——从 http 页面点这两个协议的链接，
// Chrome 一律回 `Not allowed to load local resource`（控制台实测，headless 同款
// Chromium 复现两次），所以它们继续是"需手动打开 + 复制键"那一行，而这一格对它们
// 本来就不生效（那一行不是链接）。能被这一格影响的是 http/https/mailto/tel/sms。
