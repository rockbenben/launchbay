package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/rockbenben/launchbay/internal/apierr"
	"github.com/rockbenben/launchbay/internal/render"
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

// 图标选择器那一格的上限比给 AI 的那条小：它画的是**给人看**的方格，一屏放六十来颗
// 就满了，再多只是让人往下滚。给 AI 的那条要 200，因为它一次拿回的是上下文。
const (
	iconTilesDefaultLimit = 24
	iconTilesMaxLimit     = 120
)

// iconTilesTotalHeader 把**匹配总数**送给界面。截断必须说出来（§4.3 那条
// "零命中"与"命中很多被截断"要能分开判）：搜 "a" 有六千多条，而这一格只画前几十，
// 界面不说一句"先看前 N 个"，人就会以为库里只有那几十个。
const iconTilesTotalHeader = "X-LB-Icons-Total"

// handleIconTiles 是上面那条搜索的**界面那一半**：同样的 q、同样出自那份内嵌索引，
// 吐的却是一段**已经画好的 HTML 方格**（render.IconTiles）。
//
// 为什么不叫客户端去 `/api/v1/icons` 拿名字自己画：索引是内嵌产物（六千多个名字），
// 让只看首屏的每一页都背一份图库，正是 §7.3 那条操作线要挡的事；而服务端这一段
// 用的是 svgMarkup——与页面上画卡片图标**同一个出口**，于是挑的时候看见的形状与
// 存下去之后卡片上那一枚逐像素相同。
//
// 公开（credNone），与 JSON 那条同族：它只能说"某个图标名画得出来"，说不出任何人
// 有什么书签（§5/§6.1）。落在 `/api/v1/` 前缀下，于是 no-store 由前缀中间件给。
//
// **响应不是 JSON**，所以 Content-Type 要写明、nosniff 要带上：浏览器拿到一个
// text/html 却没有 nosniff 时会照样 sniff，而这一段是要被插进页面里的。
func (s *Server) handleIconTiles(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Code: 400, Kind: apierr.QueryInvalid,
			Detail: "要带 ?q=<关键词>：这一条是给人挑图标的，空查询回一张空图而不是全量",
		})
		return
	}
	limit := iconTilesDefaultLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.QueryInvalid, Detail: "limit 要是 1 以上的整数"})
			return
		}
		if n > iconTilesMaxLimit {
			n = iconTilesMaxLimit
		}
		limit = n
	}
	// limit 是给界面看的那一档，而 total 是搜索层的全量答案——两者分开报，
	// 界面才说得出"共 N 个，先看前 M 个"。一次搜索同时给这两样（render.IconTiles）。
	markup, total := render.IconTiles(s.deps.Icons, q, limit)
	w.Header().Set(iconTilesTotalHeader, strconv.Itoa(total))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(markup))
}
