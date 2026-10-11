package bookimport

import "github.com/rockbenben/launchbay/internal/model"

// Ops 把一次导入产出的书签转成 ops（**不写盘**，交给调用方提交）。
//
// 2026-10-10 从 cmd/launchbay 搬到这里：网页上的浏览器收藏夹导入
// （internal/httpapi/importapi.go）与 CLI 的 import-bookmarks 是同一件事的
// 两条入口，而"转成 ops"这一步判据不少（建组顺序、空字段不带）。它留在
// package main 的时候 HTTP 侧根本够不着，于是只能抄一份——抄的那份不会跟
// 着改，而它决定的是**写进别人台里的每一条**。
//
// 顺序是"先全部建组、再按组加条目"，而不是建一个组加一个组的条目——
// **一批 ops 全成或全败**，所以顺序只影响失败时的报错指向，不影响正确性；
// 但按组聚在一起时，"这个 id 重复了"这类错误会在同一组附近连着报出来。
//
// 空字段不带：icon 为空就**不写 icon**（而不是写 ""），因为空 icon 与"没这个
// 字段"在渲染与校验上是两回事，前者是"没设走首字母兜底"，后者会让 ops 校验
// 以为你想把它清空。
//
// 键序用 map 而不是 struct：ops 的 value 是 `any`，交给 JSON 编码器定序；
// 这批 op 不进磁盘上的任何文件（写盘的是 ops.Run 之后的 bookmarks.json，
// 它的键序由 model.Bookmarks 的字段序固定，见 §4.3）。
func Ops(b *model.Bookmarks) []map[string]any {
	var ops []map[string]any
	for _, g := range b.Groups {
		ops = append(ops, map[string]any{
			"op": "add", "path": "groups",
			"value": map[string]any{"id": g.ID, "title": g.Title, "layout": g.Layout},
		})
	}
	for _, g := range b.Groups {
		for _, it := range g.Items {
			v := map[string]any{"id": it.ID, "name": it.Name, "url": it.URL}
			if it.Icon != "" {
				v["icon"] = it.Icon
			}
			if it.Desc != "" {
				v["desc"] = it.Desc
			}
			ops = append(ops, map[string]any{
				"op": "add", "path": "groups/" + g.ID + "/items", "value": v,
			})
		}
	}
	return ops
}

// CountItems 数一个书签文档里的条目总数。导入那条路上要用它判"这个档案是不是
// 空的"——CLI 与网页必须用同一个判据，两处各数一遍就会出现"网页说能导、
// 落点说档案非空"这种当场打架的形状。
func CountItems(b *model.Bookmarks) int {
	if b == nil {
		return 0
	}
	n := 0
	for _, g := range b.Groups {
		n += len(g.Items)
	}
	return n
}
