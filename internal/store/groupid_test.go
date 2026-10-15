package store

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 台面面板「＋ 新分组」派生 id（2026-10-15）。与 NextProfileID 同一副骨架、
// 同一份 slug 判据，差别只有两处：基名叫 group（档案那族的基名是 home），
// 而"被占了没有"由调用方给——分组住在某个人的 bookmarks.json 里，store
// 这一层不读它。
func TestNextGroupID(t *testing.T) {
	taken := map[string]bool{}
	used := func(id string) bool { return taken[id] }
	for _, c := range []struct{ title, want, why string }{
		{"NAS", "nas", "ASCII 名原样小写化"},
		{"Media Server", "media-server", "词间一个连字符"},
		// slug 得动但**被占了** → 基名序列第一格。这一格与档案那条同一处置：
		// 硬塞成 nas-2 会让那个 id 与"另一个叫 NAS 的组"长得一样而语义不同。
		{"NAS 应用", "group", "slug nas 已被占，退回基名"},
		{"播客", "group-2", "中文 slug 不出东西（§11.2 写死在明处的代价）"},
		{"影音", "group-3", "基名序列接着数"},
	} {
		got := NextGroupID(c.title, used)
		if got != c.want {
			t.Errorf("NextGroupID(%q) = %q，要 %q（%s）", c.title, got, c.want, c.why)
		}
		taken[got] = true
	}
	// 判据是"这个 id 在不在"而不是"现在有几组"：删掉一组之后那个 id 空出来，
	// 下一个同名组就该重新拿到它（与 NextProfileID 那条同形）。
	delete(taken, "group-2")
	if got := NextGroupID("影音", used); got != "group-2" {
		t.Errorf("group-2 空出来之后该被重新用上，得到 %q", got)
	}
	// 派出来的 id 必须是 model.ValidSlug 认的形状：它要进 bookmarks.json 当
	// 寻址键，还要进 op 的路径（§4.4 不认下标）。
	for _, title := range []string{"NAS 与存储", "  ", "日本語", strings.Repeat("a", 50)} {
		got := NextGroupID(title, used)
		if !model.ValidSlug(got) {
			t.Errorf("NextGroupID(%q) = %q，model.ValidSlug 不认", title, got)
		}
	}
}
