package httpapi

import (
	"errors"
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// resolveRequest 没有 by 字段：resolve 不写历史行（它只往 data/icons/ 落
// 缓存），给它一个 by 就是把一个没人消费的字段摆进契约。
type resolveRequest struct {
	URL     string `json:"url"`
	Profile string `json:"profile"`
}

type resolveGroup struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// resolveResponse 的键序 = 字段序（§4.3 对响应体成立）。
// suggestedGroup 用指针、favicon 用指针：这两个的"没有"是有效信息，
// 显式 null 比留空字段更容易让 AI 判出"没建议"而不是"字段丢了"。
// IconCandidates 恒非 nil（iconCandidates 从 []string{} 起步），零命中写 []。
type resolveResponse struct {
	URL            string        `json:"url"`
	Title          string        `json:"title"`
	Desc           string        `json:"desc"`
	SuggestedGroup *resolveGroup `json:"suggestedGroup"`
	IconCandidates []string      `json:"iconCandidates"`
	Favicon        *string       `json:"favicon"`
}

// handleResolve 把一条 URL 解析成添加书签时的候选。同一个实现供页面浮层
// 与 AI 共用（spec §6.1），所以它是 credScoped：要读该档案的现有分组才能
// 给"建议分组"，申报档案不是形式。
//
// 分流：形状与协议 → 422；凭证 → 401/403；档案 → 404/503/500；
// 目标抓不到 → 502。抓图失败不在此列——它降级成 favicon:null，因为没人
// 依赖 favicon 才肯存一条书签。
//
// profile 的**形状**只在申报了的时候才判：空申报不是"形状错"，而是
// "没说要动哪个档案"，那是凭证类答案（require 的 credScoped 分支拒 → 403）。
// 把空串也拿去过 ValidSlug 会把一个合法的 credScoped 语义问题伪装成 422。
func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	var in resolveRequest
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if in.Profile != "" && !model.ValidSlug(in.Profile) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "profile", Reason: "必须是 ASCII slug"}})
		return
	}
	if _, _, ok := s.require(r, credScoped, in.Profile); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	// 地址只在这里验一次形状（抓的时候 Fetcher 还会按 §5 再判一轮）：
	// 这里要的是"422 还是 502"这个分工——file:// 不是抓不下来，是不能拿去抓。
	// suggestGroup 与 urlTokens 各自再 parse 一遍是有意为之：那两个函数要能
	// 脱离 handler 单测，签名里只有原始字符串，不该为了少一次解析而把
	// *url.URL 塞进参数让测试去构造它。
	if _, uerr := parseResolveURL(in.URL); uerr != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: kindOf(uerr, apierr.URLIncomplete), Field: "url", Reason: uerr.Error()}})
		return
	}

	snap, err := s.deps.Store.Current(in.Profile)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.ProfileNotFound, Detail: "档案不存在"})
		return
	case err != nil:
		// 503 只由本次读返回的错误派生（最终整枝评审 #3 的口径）：
		// 真 I/O 错不是 ErrCorrupt，不许被登记表里的陈旧记录答成 503。
		if errors.Is(err, store.ErrCorrupt) {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Code: 503, Kind: apierr.ProfileBroken, Detail: corruptDetail(s.deps.Store, in.Profile, err)})
			return
		}
		writeInternalError(w, err)
		return
	}

	body, finalURL, err := s.deps.Fetch.Page(in.URL)
	if err != nil {
		// 502 而不是 500：失败在目标站点那边。detail 用 sentinel 自带的人话，
		// 一次都不拼 Go 内部错误——里面会有 URL、实现细节与换行。
		writeJSON(w, http.StatusBadGateway, errorResponse{Code: 502, Kind: apierr.TargetUnreachable, Detail: resolveDetail(err)})
		return
	}
	page := icons.Extract(body, finalURL)
	resp := resolveResponse{
		URL:            in.URL,
		Title:          page.Title,
		Desc:           page.Description,
		SuggestedGroup: suggestGroup(snap.B, in.URL, page.Title),
		IconCandidates: iconCandidates(s.deps.Icons, in.URL, page.Title),
	}
	if fav := s.resolveFavicon(in.URL, page, finalURL); fav != "" {
		f := fav
		resp.Favicon = &f
	}
	writeJSON(w, http.StatusOK, resp)
}

// parseResolveURL 只放行 http/https 且必须有主机名。与 model.checkURL 的
// 区别是这里"要拿去抓"，file:// 与 gopher:// 不是可接受的抓取目标，
// 而它们作为书签值本身是合法的（spec §6.2 不设协议白名单）。
func parseResolveURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("resolve 只接受完整的 http/https 地址")
	}
	return u, nil
}

// resolveFavicon 只在缓存没有且不在冷却时抓图（spec §5 第 1 条：
// resolve 是唯一的即时抓取口，且它自己也不该每次重抓）。
//
// finalURL（跟完重定向后的地址）在这里刻意不用：缓存键必须是**人填的那个**
// 地址的 origin，否则一条 302 去别家主机的书签会把别人的图标登记在这条书签
// 的 origin 上，下次同一 origin 就命中错图。参数保留是和 Extract(body, base)
// 的调用形状对齐，不是给实现读的。
func (s *Server) resolveFavicon(rawURL string, page icons.Page, finalURL string) string {
	origin := icons.Origin(rawURL)
	if origin == "" {
		return ""
	}
	if e, ok := s.deps.Cache.Lookup(origin); ok {
		return "/icons/" + e.Hash + "." + e.Ext
	}
	if s.deps.Cache.InCooldown(origin) {
		return ""
	}
	candidate := page.Favicon
	if candidate == "" {
		// 没写 link 标签就按约定路径试一次：内网服务十有八九挂着
		// /favicon.ico 而不在 HTML 里声明它（spec §5 那句"内网服务普遍没
		// favicon"说的正是这批，试一次的成本比让家里人自己填低）。
		candidate = origin + "/favicon.ico"
	}
	data, ext, err := s.deps.Fetch.Image(candidate)
	if err != nil {
		if markErr := s.deps.Cache.MarkFail(origin); markErr != nil {
			// 冷却记不下去，下次 resolve 还会重抓一次：可接受的退化，
			// 但要说出来——否则"失败 24 小时后再试"这句会变成假话。
			// origin 是调用方自己给的地址，进 stderr 可接受；令牌绝不进。
			// err 由 store 侧包装，**原文里带着它要写的那个盘上路径**
			// （Task 8 修复轮在同类行实测到 `…\icons\<hash>.png: Access is
			// denied`——旧注释那句"stderr 不含磁盘路径"当时就是假话，这里改
			// 成真话：与下面 Put 失败那行同一口径，泄漏面没有新增）。
			fmt.Fprintf(os.Stderr, "resolve icon cooldown failed origin=%s: %v\n", origin, markErr)
		}
		return ""
	}
	e, err := s.deps.Cache.Put(origin, data, ext)
	if err != nil {
		// 存盘失败与抓图失败同一口径说出来（形状照 batch.go 的历史失败那行：
		// origin=错误原文）。origin 是调用方自报的地址，进 stderr 可接受；
		// 令牌绝不进。错误由 store 包装，原文里可能带它自己要写的文件名——这与
		// batch 那行看到的是同一件事，不是这一行新增的泄漏面。
		fmt.Fprintf(os.Stderr, "resolve icon cache put failed origin=%s: %v\n", origin, err)
		// 冷却是 best-effort，别把它说成硬的：Put 写不下去的盘故障（ENOSPC/
		// 权限）下，MarkFail 写的也是同一块盘，大概率同样记不住——我们不为此
		// 加"只在内存里记失败"的第二次真相（§12 明面反对的东西）。这里不存在
		// 失控抓取循环：resolve 只由人或 AI 的动作触发，重抓次数天然被人手限速，
		// 与 §11 那条"没有后台探测回路"是两个东西，别混为一谈。
		if markErr := s.deps.Cache.MarkFail(origin); markErr != nil {
			fmt.Fprintf(os.Stderr, "resolve icon cooldown failed origin=%s: %v\n", origin, markErr)
		}
		return "" // 存不下去就回空：抓到了但没落地，与没抓到在响应上等价
	}
	return "/icons/" + e.Hash + "." + e.Ext
}

// resolveDetail 把抓取层的 sentinel 翻成 502 的 detail。
//
// 用 errors.Is 逐条判而不是 `switch err`：sentinel 现在确实是裸返回，但
// 等哪天有人在 Fetcher 里 %w 包一层，按等值比较的 switch 会静默掉到
// default——那等于把"目标连不上"报成"目标抓不下来"，调用方看到的分诊就
// 是假的。本仓库的 triage 早已统一走 errors.Is（batch.go、revert.go 同）。
func resolveDetail(err error) string {
	switch {
	case errors.Is(err, icons.ErrBadURL):
		return "地址形状不合法"
	case errors.Is(err, icons.ErrUnreachable):
		return icons.ErrUnreachable.Error()
	case errors.Is(err, icons.ErrTimeout):
		return icons.ErrTimeout.Error()
	case errors.Is(err, icons.ErrStatus):
		return icons.ErrStatus.Error()
	case errors.Is(err, icons.ErrTooBig):
		return icons.ErrTooBig.Error()
	case errors.Is(err, icons.ErrWrongType):
		return icons.ErrWrongType.Error()
	case errors.Is(err, icons.ErrTooManyRedirects):
		return icons.ErrTooManyRedirects.Error()
	default:
		// 今天 fetch 只回 sentinel，这分支够不到；等有人加了新 sentinel 却忘了
		// 在这里登记，这行日志就是唯一的证人——不把它打进 stderr，注释里那句
		// "原始 err 进日志"就是假话。给调用方的仍只有一句话，err 不进响应。
		fmt.Fprintf(os.Stderr, "resolve unknown fetch error: %v\n", err)
		return "目标抓不下来"
	}
}

// suggestGroup 只从**这个档案现有的分组**里挑，规则全是本地打分：
// 同 host 的兄弟条目 > 同首标签 > 分组 id/标题出现在地址或标题里。
// 挑不出就返回 nil——凭空造一个分组等于替用户决定他的书架长什么样。
func suggestGroup(b *model.Bookmarks, rawURL, title string) *resolveGroup {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	first := firstLabel(host)
	hay := strings.ToLower(rawURL + " " + title)

	best, bestScore := -1, 0
	for i, g := range b.Groups { // 档案顺序即优先级：同分时靠前的赢
		score := 0
		for _, it := range g.Items {
			iu, err := url.Parse(it.URL)
			if err != nil {
				continue
			}
			ih := strings.ToLower(iu.Hostname())
			switch {
			case host != "" && ih == host:
				score = max(score, 3)
			case len(first) > 2 && !allDigits(first) && firstLabel(ih) == first:
				// 数字首标签是 IP 八位组，不是服务名：两个毫不相干的
				// 192.x/127.x 主机不配靠它拿 2 分。整主机相等（score-3）
				// 才是 IP 字面量的正确形态，内网同主机的建议能力不受影响。
				score = max(score, 2)
			}
		}
		if score < 1 && (containsFold(hay, g.ID) || containsFold(hay, g.Title)) {
			score = 1
		}
		if score > bestScore {
			bestScore, best = score, i
		}
	}
	if best < 0 {
		return nil
	}
	return &resolveGroup{ID: b.Groups[best].ID, Title: b.Groups[best].Title}
}

// iconCandidates 把地址与标题折成几个词，逐个在索引里找，品牌层优先。
// 上限 5 条：添加浮层里给 5 个候选已经要滚动了，10 条只会让人闭眼选第一个。
//
// 判据本体在 internal/icons（Candidates）——导入器要做同一件事（浏览器导出的书签
// 只有 favicon，没有图标名），两份顺序逻辑迟早分家。这层留作薄包装是因为本包
// 的测试直接调它，而"搬走实现"不该顺手把证人一起搬没。
func iconCandidates(ix *icons.Index, rawURL, title string) []string {
	return icons.Candidates(ix, rawURL, title, 5)
}

// urlTokens 是 icons.URLTokens 的薄包装，同上：顺序判据只有一份，测试照旧调它。
func urlTokens(rawURL, title string) []string { return icons.URLTokens(rawURL, title) }

func firstLabel(host string) string {
	if i := strings.IndexByte(host, '.'); i > 0 {
		return host[:i]
	}
	return host
}

// allDigits 判"整段都是十进制数字"。空串返回 false：它进不了任何通道，
// 该由调用方的长度闸门处理，而不是让"数字过滤"顺手兼职当非空校验。
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func containsFold(hay, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	return len(needle) >= 3 && strings.Contains(hay, needle)
}
