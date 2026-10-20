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

// handleIconBrowse 是图标选择器的**浏览**那一档：不搜什么，直接翻图库。
//
// **为什么要有它**（2026-10-20）：面板从前只有"搜一个词"与"这一份已经在用的"两批，
// 于是"我想看看这台机器里到底有哪些图标、随便挑一个"这条路是断的——而那一万个
// 图标是**内嵌在二进制里**的（spec §5），它不是"别人的东西"，是这台机器自带的。
// 手工挑的常用清单只是其中一小撮（41 个），其余九千多个仍然够不着。
//
// **翻页而不是一次倒出来**：一万个方格的 markup 是十几 MB，一次性吐出来会把
// 面板卡死、也会把这一台家用机器的内存吃穿。所以按 offset 翻页（每页默认 60，
// 上限 120），界面滚到底再要下一页。
//
// **顺序是稳定的**（icons.Index.AllNames）：品牌层在前、通用层在后、层内按名字
// 排序。翻页的位置就是顺序里的位置，顺序一变就等于翻乱了。
//
// 公开（credNone）：它是一份公共名单，说不出任何人有什么书签（§5/§6.1）。
// 落在 `/api/v1/` 前缀下，于是 no-store 由前缀中间件给。
//
// **响应不是 JSON**，与 /api/v1/icons/tiles 同族：Content-Type 写明、nosniff
// 带上（这一段要被插进页面里）。
const (
	iconBrowseDefaultLimit = 60
	iconBrowseMaxLimit     = 120
)

// iconBrowseTotalHeader 报**全库**的条数。它与搜索那条的 X-LB-Icons-Total
// （那是"命中多少"）不是一件事，所以另起一个名字：浏览这一档没有"命中"这回事，
// 界面要说的是「图库里有 N 个」。
const iconBrowseTotalHeader = "X-LB-Icons-Library"

func (s *Server) handleIconBrowse(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Code: 400, Kind: apierr.QueryInvalid, Detail: "offset 要是非负整数",
			})
			return
		}
		offset = n
	}
	limit := iconBrowseDefaultLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Code: 400, Kind: apierr.QueryInvalid, Detail: "limit 要是 1 以上的整数",
			})
			return
		}
		if n > iconBrowseMaxLimit {
			n = iconBrowseMaxLimit
		}
		limit = n
	}
	all := s.deps.Icons.AllNames()
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	markup := render.IconTilesFor(s.deps.Icons, all[offset:end])
	w.Header().Set(iconBrowseTotalHeader, strconv.Itoa(total))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(markup))
}

// handleIconTiles 是上面那条搜索的**界面那一半**：同样的 q、同样出自那份内嵌索引，
// 吐的却是一段**已经画好的 HTML 方格**（render.IconTiles）。
//
// 为什么不叫客户端去 `/api/v1/icons` 拿名字自己画：索引是内嵌产物（一万多个名字，品牌层与通用层两层加起来），
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

// handleIconStarter 是图标选择器的**默认那一批**：不搜什么，直接给一份常用清单。
//
// **为什么要有这一条**（2026-10-20）：选择器面板从前打开是一张空网格 + 一句
// 「先搜个名字」。起始格（这一份已经在用的图标，app.js 那一族）救不了它——
// 书签默认 icon=auto（去抓 favicon），而 auto 与"没设"都被那一格排除掉，
// 于是实测里这一格几乎永远是空的，面板永远空着。
// 换个**一定有货**的来源：一份人工挑的常用清单（icons.Starter），与服务端画
// 卡片图标走同一个出口（render.svgMarkup），于是挑的时候看见的形状与存下去
// 之后卡片上那一枚逐像素相同。
//
// 公开（credNone）：它是一份公共名单，说不出任何人有什么书签（§5/§6.1）。
// 落在 `/api/v1/` 前缀下，于是 no-store 由前缀中间件给。
//
// **响应不是 JSON**，与 /api/v1/icons/tiles 同族：Content-Type 要写明、nosniff
// 要带上（浏览器拿到一个 text/html 却没有 nosniff 时会照样 sniff，而这一段
// 是要被插进页面里的）。
//
// **不带 X-LB-Icons-Total**：那一格在搜索那条上是为了说"命中很多被截断"，
// 而这一批是**固定的一整批**（没有截断这回事），报一个总数反而像在说"还有
// 别的没给你"。画出来的条数就是全部。
func (s *Server) handleIconStarter(w http.ResponseWriter, r *http.Request) {
	names := s.deps.Icons.StarterNames()
	markup := render.IconTilesFor(s.deps.Icons, names)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(markup))
}
