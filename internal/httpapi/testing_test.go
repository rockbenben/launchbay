package httpapi

import (
	"sync"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

// 解析那两套 gz 索引不便宜（约 2MB 解压 + 解码），整个测试包共用一份：
// Index 一旦装配就只读，所以 sync.Once 之后不需要任何锁。
var (
	loadOnce sync.Once
	loadedIX *icons.Index
)

func testIndex() *icons.Index {
	loadOnce.Do(func() { loadedIX = icons.MustLoad() })
	return loadedIX
}

// newDeps 是 httpapi 测试的唯一装配点。评审教训：装配散落时"新加一个依赖
// 字段"会变成逐个测试红的追查，而这里只需要改一行。
//
// Cache 必须与 srv.deps.Store 指向同一个目录、且就是服务端手里那面镜像：
// /status 的缺图统计读的是 deps.Cache，测试若再 s.LoadIconCache() 拿第二个
// 对象往里 Put，服务器那面永远不变（裁定 R3）。Fetch 同样在这里装好，
// 任何测试都不会看到 nil 的一个。
func newDeps(t *testing.T, s *store.Store, sc auth.Scopes) Dependencies {
	t.Helper()
	fc, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	// Render 与 Cache 必须是同一个 IconCache 对象：resolve Put 之后
	// Generation 一变，ProfilePage 的缓存键才跟得上（裁定 R3 同源）。
	return Dependencies{Store: s, Tokens: sc, Render: render.New(s, testIndex(), fc), Icons: testIndex(),
		Fetch: icons.NewFetcher(), Cache: fc}
}

// seedItem 往某档案的分组里追加一条条目，经 Store.Current + SaveBookmarks，
// **不经 REST**：测试要的是"盘上有这个数据"，绕一圈网络等于用被测通路准备夹具
// （batch 的图标闸门还会替测试决定能不能种，那不是这里想验的东西）。
func seedItem(t *testing.T, s *store.Store, profile, group, itemID, rawURL string) {
	t.Helper()
	snap, err := s.Current(profile)
	if err != nil {
		t.Fatal(err)
	}
	for i := range snap.B.Groups {
		if snap.B.Groups[i].ID != group {
			continue
		}
		snap.B.Groups[i].Items = append(snap.B.Groups[i].Items, model.Item{
			ID: itemID, Name: itemID, URL: rawURL, Alias: []string{}, Tags: []string{},
		})
		if err := s.SaveBookmarks(profile, snap.B); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("档案 %q 里没有分组 %q", profile, group)
}

func revOf(t *testing.T, s *store.Store, id string) int64 {
	t.Helper()
	rev, err := s.CurrentRev(id)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

// snapshotCount 报当前 rev 与快照份数：resolve 在整条链上只许写 data/icons/，
// 这两个数一个都不许变（spec §5/§6.1）。
func snapshotCount(t *testing.T, s *store.Store, id string) (int64, int) {
	t.Helper()
	rev := revOf(t, s, id)
	revs, err := s.ListSnapshots(id)
	if err != nil {
		t.Fatal(err)
	}
	return rev, len(revs)
}

func historyLines(t *testing.T, s *store.Store, profile string) int {
	t.Helper()
	entries, err := s.ReadHistory(profile, 0)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
