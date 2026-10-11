package render

import (
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

// matches 按 q 过滤一份书签，返回**新的** Bookmarks、命中数与总条数。
//
// 判据只有这一份（spec §8.1）：条目看 name/alias/url/tags/desc，分组标题命中
// 就把整组捞出来。大小写折叠一次、CJK 按子串（不分词——局域网书签量级不需要
// 分词，而分词会带来一套没人维护的词典）。
//
// 不原地改入参：调用方给的是 store 里那份书签对象，改它就是改磁盘真相的内存副本。
// 说清楚边界：空 query 时返回的分组与入参**共享** Items 底层数组（不复制是故意的，
// 空 query 等于不过滤，复制一整台书签只为得到同一份内容是白花一次）；有 query 时
// 每个分组拿到的是新建切片。两种情况下调用方都只读——这与 Page.Body 那条"缓存切片
// 不许原地改"是同一条纪律。
func matches(q string, b *model.Bookmarks) (*model.Bookmarks, int, int) {
	needle := strings.ToLower(strings.TrimSpace(q))
	// 整体复制再换掉 Groups：逐字段抄一份的话，model.Bookmarks 以后加的字段
	// （Clock 就是这么差点被吃掉）会在搜索结果里静默消失——那不是过滤，是丢功能。
	out := *b
	out.Groups = nil
	total, hits := 0, 0
	for _, g := range b.Groups {
		total += len(g.Items)
		if needle == "" {
			out.Groups = append(out.Groups, g)
			hits += len(g.Items)
			continue
		}
		groupHit := containsFold(needle, g.Title)
		ng := g
		ng.Items = nil
		for _, it := range g.Items {
			if groupHit || matchItem(needle, it) {
				ng.Items = append(ng.Items, it)
				hits++
			}
		}
		if len(ng.Items) > 0 {
			out.Groups = append(out.Groups, ng)
		}
	}
	if needle == "" {
		return &out, total, total
	}
	return &out, hits, total
}

// matchItem 是条目侧唯一的命中判据。needle 必须已经折叠过小写——折叠只发生在
// matches 入口一次，散在各处再 ToLower 会有漏掉一处的那一半实现。
func matchItem(needle string, it model.Item) bool {
	if containsFold(needle, it.Name) || containsFold(needle, it.URL) ||
		containsFold(needle, it.Desc) {
		return true
	}
	for _, s := range it.Alias {
		if containsFold(needle, s) {
			return true
		}
	}
	for _, s := range it.Tags {
		if containsFold(needle, s) {
			return true
		}
	}
	return false
}

// containsFold 是"needle（已折叠过）出现在 hay 里"。参数顺序刻意与
// iconCandidates 那套分开：这里 hay 是被搜的字段，needle 是查询词。
func containsFold(needle, hay string) bool {
	return needle != "" && strings.Contains(strings.ToLower(hay), needle)
}
