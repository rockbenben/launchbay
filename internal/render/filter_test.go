package render

import (
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func book() *model.Bookmarks {
	return &model.Bookmarks{Title: "T", Groups: []model.Group{
		{ID: "nas", Title: "NAS 与内网", Items: []model.Item{
			{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{"nas"}, Tags: []string{"存储管理"}},
			{ID: "qb", Name: "qBittorrent", URL: "http://10.0.0.3:8181", Alias: []string{"bt"}, Tags: []string{}},
		}},
		{ID: "read", Title: "读与收藏", Items: []model.Item{
			{ID: "weread", Name: "微信读书", URL: "https://weread.qq.com", Desc: "看书用这个", Alias: []string{}, Tags: []string{"书"}},
		}},
	}}
}

func TestMatches_六个字段都算命中(t *testing.T) {
	for _, tc := range []struct {
		name, q string
		want    []string
	}{
		{"名字", "群晖", []string{"nas/dsm"}},
		{"别名", "bt", []string{"nas/qb"}},
		{"地址", "8181", []string{"nas/qb"}},
		{"标签", "存储", []string{"nas/dsm"}},
		{"描述", "看书", []string{"read/weread"}},
		// 分组标题命中 → 该组全部条目都算（"搜 NAS 把内网那组捞出来"是真实用法）。
		{"分组标题", "读与", []string{"read/weread"}},
		{"大小写不敏感", "QB", []string{"nas/qb"}},
		{"无命中", "不存在的词", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb, hits, total := matches(tc.q, book())
			var got []string
			for _, g := range fb.Groups {
				for _, it := range g.Items {
					got = append(got, g.ID+"/"+it.ID)
				}
			}
			if total != 3 {
				t.Errorf("总数 = %d，要 3（总数与过滤无关）", total)
			}
			if hits != len(tc.want) {
				t.Fatalf("命中 %d 条 %v，要 %d 条 %v", hits, got, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if got[i] != w {
					t.Errorf("第 %d 条 = %q，要 %q", i, got[i], w)
				}
			}
		})
	}
}

// 空 query 与"没命中"必须是两个不同答案：模板要靠它说两句不同的话。
func TestMatches_空query与无命中是两个状态(t *testing.T) {
	if fb, hits, total := matches("", book()); hits != total || len(fb.Groups) != 2 {
		t.Errorf("空 query 该原样全给：hits=%d total=%d groups=%d", hits, total, len(fb.Groups))
	}
	if _, hits, _ := matches("zzz", book()); hits != 0 {
		t.Errorf("无命中 hits=%d，要 0", hits)
	}
}

// 过滤不许改到调用方传进来的那份数据：它是 store 缓存里的书签对象。
// 顺手也钉住"非 Groups 字段一个都不丢"——逐字段抄的实现会在这里红。
func TestMatches_不改入参且不吃掉别的字段(t *testing.T) {
	b := book()
	b.Clock = true
	b.OpenInNewTab = true
	b.Rev = 7
	fb, _, _ := matches("群晖", b)
	if len(b.Groups[0].Items) != 2 {
		t.Errorf("入参被改了：nas 组剩 %d 条", len(b.Groups[0].Items))
	}
	if len(fb.Groups[0].Items) != 1 {
		t.Errorf("结果条数不对：%d", len(fb.Groups[0].Items))
	}
	if fb.Clock != b.Clock || fb.OpenInNewTab != b.OpenInNewTab || fb.Title != b.Title {
		t.Errorf("过滤把非 Groups 的字段吃了：clock=%v newtab=%v title=%q",
			fb.Clock, fb.OpenInNewTab, fb.Title)
	}
}
