package icons

import (
	"net/url"
	"strings"
)

// 这个文件是「从一个网址里猜图标名」的判据，**只有一份**。
//
// 它原来长在 internal/httpapi 里（resolve 的 iconCandidates）。导入器
// （tools/import-flare）要做同一件事——浏览器导出的书签里没有图标名，只有
// favicon，而 favicon 是二进制、不是索引里的名字——于是两处都会写一遍"主机段
// → 路径段 → 标题词"这套顺序与过滤。那套顺序不是随手排的，它决定候选的优先级，
// 两处各排一次迟早分家，而分家之后没人知道哪一处才是当初想给谁看的。
//
// 所以搬到这儿：图标的"怎么找到它"和图标索引是同一件事，比放在 HTTP 层更靠得住。

// Candidates 按优先级猜图标候选，**品牌层（simple-icons）先于通用层（tabler）**。
// 上限 limit：给界面选的时候 5 条够用（再多要滚动，反而让人闭眼选第一个）；
// 导入时只要 1 条。
func Candidates(x *Index, rawURL, title string, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, tok := range URLTokens(rawURL, title) {
		for _, prefix := range []string{"simple-icons", "tabler"} {
			res := x.Search(tok, prefix, 2)
			for _, k := range res.Icons {
				if seen[k] {
					continue
				}
				seen[k] = true
				out = append(out, k)
			}
		}
		if limit > 0 && len(out) >= limit {
			return out[:limit]
		}
	}
	return out
}

// URLTokens 取主机各段、路径各段、标题里的词。
//
// **顺序是有意义的**：主机名比标题更接近"这到底是什么服务"（`github.com` 里的
// github 说的是这个站点是什么，而标题"我的代码托管"什么都不说）。排序就靠 append
// 的调用次序——加一行 sort.SliceStable(…, return false) 是"什么都不做还长得像在做"。
func URLTokens(rawURL, title string) []string {
	var toks []string
	seen := map[string]bool{}
	push := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		// 全十进制数字的段（IP 八位组、纯数字路径段/标题词）不配占 token 名额：
		// Search("127") 大概率零命中，却白吃 4 个 token 里的一格、候选里的一席。
		if len(s) < 2 || len(s) > 32 || seen[s] || allDigits(s) {
			return
		}
		seen[s] = true
		toks = append(toks, s)
	}
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		for _, seg := range strings.Split(u.Hostname(), ".") {
			if !reservedLabels[strings.ToLower(seg)] {
				push(seg)
			}
		}
		for _, seg := range strings.Split(strings.Trim(u.Path, "/"), "/") {
			push(seg)
		}
	}
	for _, w := range strings.FieldsFunc(title, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r >= 0x4e00)
	}) {
		push(w)
	}
	if len(toks) > 4 {
		toks = toks[:4]
	}
	return toks // 顺序即优先级：主机各段 → 路径段 → 标题词
}

// reservedLabels 滤掉那些出现在每个内网域名里、却说明不了任何事的段。
var reservedLabels = map[string]bool{
	"www": true, "lan": true, "local": true, "localdomain": true,
	"home": true, "internal": true, "intranet": true, "localhost": true,
}

// allDigits 判"整段都是十进制数字"。
//
// **刻意没有导出**：internal/httpapi 里另有一处同名判据，用途是"数字首标签是 IP
// 八位组不是服务名"（分组建议）。那是一个关于**分组**的问题，与图标无关；把这个
// 五行谓词导出去让 HTTP 层调用，等于让索引包知道分组建议的存在。两处各留一份的
// 重复代价极小——它没有可分家的余地（"全是数字吗"只有一个答案）——而耦合的代价
// 是真的：以后有人改分组建议的口径，会顺手改到图标这一侧。
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
