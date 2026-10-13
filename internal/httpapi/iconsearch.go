package httpapi

import (
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"strconv"
	"strings"
)

// 上限是给 AI 的：一次拿回 6 千个名字既没意义又会淹没上下文。真实用法
// 都是"搜一个词、挑一个"，200 条足够覆盖 prefix 过滤后的任何合理查询。
const (
	iconsSearchDefaultLimit = 20
	iconsSearchMaxLimit     = 200
)

// iconsResponse 用 struct：键序固定（§4.3），returned 与 total 是两个数——
// "零命中"与"命中很多被截断"必须能分开判，否则 AI 会把截断当成没有。
type iconsResponse struct {
	Query    string   `json:"query"`
	Total    int      `json:"total"`
	Returned int      `json:"returned"`
	Icons    []string `json:"icons"`
}

// handleIconsSearch 在服务端内嵌的索引里找图标名。公开（credNone）：
// 它只能说"某个图标名有没有"，说不出任何人有什么书签（spec §5/§6.1）。
//
// 搜索层的三条已裁定行为（空 q 空结果、Total 与 Icons 分账、整键命中只回
// 那一条且被 prefix 排除时返回空）由 icons.Search 定死，这里原样透出、
// 不再判一遍——端点替搜索层想事就是第二份真相。
func (s *Server) handleIconsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.QueryInvalid, Detail: "要带 ?q=<关键词>；这个端点是查名字，不吐全量索引"})
		return
	}
	// prefix 绑一次就地用：校验用它是它、喂给 Search 的也得是它——读两遍
	// Query() 等于允许这两处悄悄不是同一个值（B14）。
	p := r.URL.Query().Get("prefix")
	if p != "" && p != "tabler" && p != "simple-icons" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.QueryInvalid, Detail: "prefix 只能是 tabler 或 simple-icons"})
		return
	}
	limit := iconsSearchDefaultLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.QueryInvalid, Detail: "limit 要是 1 以上的整数"})
			return
		}
		if n > iconsSearchMaxLimit {
			n = iconsSearchMaxLimit
		}
		limit = n
	}
	res := s.deps.Icons.Search(q, p, limit)
	writeJSON(w, http.StatusOK, iconsResponse{
		Query: res.Query, Total: res.Total, Returned: len(res.Icons), Icons: res.Icons,
	})
}
