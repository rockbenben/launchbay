package render

import (
	"fmt"
	"sync"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 解析那两套 gz 产物不便宜（约 2MB 解压 + 全量校验），整个测试包共用一份：
// Index 一旦装配就只读，sync.Once 之后不需要任何锁（与 httpapi 的 testIndex 同形）。
var (
	iconIndexOnce sync.Once
	iconIndex     *icons.Index
)

// assets 返回测试用的索引、缓存与档案目录。索引走真产物解析一次，缓存建在
// t.TempDir() 上——每个用例一份干净的，互不影响；Put 只写进这个临时目录。
// 目录里预先建好 dad 档案：ProfilePage 的读通路要 profile.json 真的存在。
func assets(t *testing.T) (*icons.Index, *store.IconCache, *store.Store) {
	t.Helper()
	iconIndexOnce.Do(func() { iconIndex = icons.MustLoad() })
	s := store.New(t.TempDir())
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "dad", Title: "dad", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	fc, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	return iconIndex, fc, s
}

// saveBookmarks 用 store.SaveBookmarks 落一份给定的 Bookmarks（写路径校验照跑
// ——夹具不许绕过校验塞坏数据进去）。
func saveBookmarks(t *testing.T, s *store.Store, id string, b model.Bookmarks) {
	t.Helper()
	if err := s.SaveBookmarks(id, &b); err != nil {
		t.Fatal(err)
	}
}

// withIcons 造 n 条 icon 均为 tabler:home 的书签；n == 0 时给一个空分组的壳
// （auto 夹具自己往 Groups[0].Items 里塞条目）。URL 全用 10. 段示例地址。
func withIcons(n int) model.Bookmarks {
	b := *store.EmptyBookmarks()
	b.Rev = 1
	g := model.Group{ID: "g", Title: "组", Layout: model.LayoutList}
	for i := 0; i < n; i++ {
		g.Items = append(g.Items, model.Item{
			ID:   fmt.Sprintf("i%d", i),
			Name: fmt.Sprintf("条目%d", i),
			URL:  fmt.Sprintf("http://10.0.0.%d/", i+1),
			Icon: "tabler:home",
		})
	}
	b.Groups = []model.Group{g}
	return b
}

// iconFixture 直接构造一个 icons.Icon（跳过 Decode），用来验渲染层的第二道闸：
// 万一有人绕过加载白名单亲手造了 Icon，markup 拼接仍须兜得住。
func iconFixture(prefix, name, layer string, els []icons.Element) icons.Icon {
	return icons.Icon{Prefix: prefix, Name: name, ViewBox: "0 0 24 24", Layer: layer, Elements: els}
}

// renderer 是既有用例的统一装配缝：New 因图标依赖改成三参数后，只关心
// "有一个渲染器"的旧测试一律从这里走——索引共享、缓存是 assets 新建的空缓存，
// 两处各自临时装配一遍正是本次要收掉的病根。
//
// 注意接缝的边界：这里的 fc 挂在 assets(t) 自己的临时目录上，**不跟着传进来的
// s 走**——拿它往 s 的图标缓存里 Put，页面的缓存代次不会动。要测代次失效，
// 别用这个缝，自己 New(s, ix, fc) 拿同一对 (s, fc)。
func renderer(t *testing.T, s *store.Store) *Renderer {
	t.Helper()
	ix, fc, _ := assets(t)
	return New(s, ix, fc)
}
