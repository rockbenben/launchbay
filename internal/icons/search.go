package icons

import (
	"sort"
	"strings"
)

// SearchResult 的字段序就是 JSON 键序（§4.3 对响应体同样成立）。
// Icons 一律非 nil：零命中序列化出 "icons":[]，绝不写 null——Task 5 把它直接
// 嵌进响应体，null 会让 AI 判出"端点坏了"。
type SearchResult struct {
	Query string   `json:"query"`
	Total int      `json:"total"`
	Icons []string `json:"icons"`
}

// rank 小者优先。别名命中按"父名的精确命中"计，因为它本来就指向同一条图标。
const (
	rankExact  = 0 // 名字等于查询词
	rankPrefix = 1 // 名字以查询词加 '-' 开头（docker → docker-alt）
	rankAlias  = 2 // 别名等于查询词（等价于父名精确）
	rankInfix  = 3 // 名字包含查询词
)

// prefixRank 决定同 rank 内的层序：品牌层在前（spec §5 的双层用途表）。
// 键序与 load.go 的解析顺序无关——那是"哪套先报错"，这里是"哪套先出现"。
//
// 它是不可变查找而不是包级 map（M-5）：本包的立论是"Index 装配后只读、不带
// 锁"，而 `var map` 摆在那里就是并发写的潜在面（首轮实现的破坏用例就是用例改
// 它）。函数没有可写的中间状态，改层序只能改代码、不能改运行时状态。
func prefixRank(prefix string) int {
	switch prefix {
	case "simple-icons":
		return 0
	case "tabler":
		return 1
	}
	return 9 // 白名单外的前缀进不了索引，这里只是不留 panic 路径
}

type hit struct {
	key   string
	rank  int
	layer int
}

// Search 在本地索引里按关键词找候选图标名。零命中与被截断是两个状态，
// 所以 Total 报匹配总数、Icons 只带 limit 条（spec §6.1）。
//
// limit 只管 Icons 的条数，管不着 Total（I-1）：limit<=0 时匹配照做、Total 报
// 真数、Icons 是非 nil 空切片。把 limit==0 做成提前返回连带 Total 归零，就等于
// 对调用方撒谎说"这个查询零命中"，而 spec §6.1 立的恰恰是这两个状态必须可分。
// Task 5 的端点会钳 limit>=1，但那是端点的事：Search 是本包导出的方法，Go 侧
// 调用方（Task 5 的 resolve 候选、Task 8 的 iconCandidates）从 struct 零值拿到
// limit==0 就会中招，不 panic、不报错，只是静默错。
//
// 只做子串匹配：这是给人和 AI 用的名字查找，不是全文检索。上倒排索引或
// 拼音是另一件事，而一万条量级的名字做子串扫描本来就是一微秒级的事。
//
// 匹配只看 ':' 之后的名字段，不看集合前缀（控制器裁定 1）：拿全键做子串
// 匹配的话，q=tabler 会命中 6265 条、q=simple 会命中整个品牌层——集合名
// 不是图标名，那种"全都能搜到"是搜索的假象。TestSearch_集合名不是图标名
// 钉住这条。
func (x *Index) Search(q, prefix string, limit int) SearchResult {
	q = strings.ToLower(strings.TrimSpace(q))
	// prefix 与 q 同口径归一（M-2）：不归一的话 prefix=Tabler 静默 0 命中，和
	// "这个前缀下真的没有"同形。这里只做归一，**不校验白名单**：白名单外的前缀
	// 走到下面自然匹配不到任何 byPrefix 键、返回空；把非法值判成 422 是 Task 5
	// handler 的判据，别在这里替它兜。
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	res := SearchResult{Query: q, Icons: []string{}}
	if q == "" {
		return res
	}

	var hits []hit
	seen := map[string]bool{}
	add := func(key string, rank int) {
		if seen[key] {
			return // 别名与父名指向同一条：只留一条
		}
		seen[key] = true
		hits = append(hits, hit{key: key, rank: rank, layer: layerOf(key)})
	}

	// 带前缀的查询词（"tabler:box"）按整键回查：AI 常拿上一步的结果原样回查。
	// 整键命中就只回那一条、不再跑子串（I-3）：拿全键回来要的是"确认这一条"，
	// 把它混进 111 条同名子串里、还让另一层的同名条目（simple-icons:box，层序
	// 更靠前）排第一，就是"搜到了写不进"的近亲——拿 top-1 当答案的调用方会写错
	// 图标。命中仍要过 prefix 过滤：prefix 说了只要 simple-icons，就别把 tabler
	// 端回来；那一条被 prefix 排除时结果就是空，不退回去搜同名别的层。
	wholeKeyHit := false
	if i := strings.IndexByte(q, ':'); i > 0 {
		if key, ok := x.Canonical(q); ok {
			wholeKeyHit = true
			if p := key[:strings.IndexByte(key, ':')]; prefix == "" || prefix == p {
				add(key, rankExact)
			}
		}
	}

	// 整键命不中才退回按名字段搜索（"AI 抄来的名字带错前缀"这条路要留着）。
	if !wholeKeyHit {
		name := q
		if i := strings.IndexByte(name, ':'); i > 0 {
			name = name[i+1:]
		}
		// 名字段为空（"tabler:" 这类查询）：整键查已在上面试过并落空，剩下的
		// 只有 Contains(x, "") 恒真的全量放行——按"空查询一律返回空"收口。
		if name != "" {
			x.scanName(name, prefix, add)
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		if hits[i].layer != hits[j].layer {
			return hits[i].layer < hits[j].layer
		}
		return hits[i].key < hits[j].key
	})

	res.Total = len(hits)
	for i, h := range hits {
		if i >= limit {
			break // limit<=0 在这里天然收成空 Icons，不动 Total
		}
		res.Icons = append(res.Icons, h.key)
	}
	return res
}

// scanName 按名字段子串匹配收集候选：精确 > 前缀（带 '-' 边界）> 包含，
// 别名表单独扫一遍并归一到父名（byPrefix 循环看不见别名）。add 自带去重，
// 所以别名与父名指向同一条时只留一条。
func (x *Index) scanName(name, prefix string, add func(key string, rank int)) {
	for p, list := range x.byPrefix {
		if prefix != "" && p != prefix {
			continue
		}
		for _, key := range list {
			seg := key[strings.IndexByte(key, ':')+1:]
			switch {
			case strings.HasSuffix(key, ":"+name):
				add(key, rankExact)
			case strings.Contains(key, ":"+name+"-"):
				add(key, rankPrefix)
			case strings.Contains(seg, name):
				add(key, rankInfix)
			}
		}
	}
	for alias, target := range x.alias {
		if prefix != "" && !strings.HasPrefix(alias, prefix+":") {
			continue
		}
		aliasName := alias[strings.IndexByte(alias, ':')+1:]
		if aliasName == name {
			add(target, rankAlias)
		}
	}
}

// layerOf 把全键拆成集合前缀再问层序。
func layerOf(key string) int {
	p, _, _ := strings.Cut(key, ":")
	return prefixRank(p)
}
