package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 空分组在页面上**照画**，而且要说清它是空的。
//
// 这条判据 2026-10-15 翻过一次：先判「空的自动隐藏」，当天改回照画。改判的
// 理由是量出来的——页面上那几处「这一条放进哪一组」都是从**渲染出来的标题**
// 读的（app.js 的 `main h2[data-group]`、`[data-gid]`），于是藏起空组等于
// 让添加浮层的下拉、拖拽落点、「移动到」名单**一起**失效：「这一组清空了，
// 怎么才能再放东西进去」在界面上就没有答案了。
//
// 所以现在钉的是三件事，缺一不可：
//   - 空组**在**（节容器 + 锚点 + 侧栏那一格都在）；
//   - 它**说清自己是空的**（一个只有标题、正文空着的节读起来像渲染坏了）；
//   - 有内容的组照旧在（这条挡的是"为了给空组让位把别的也藏了"）。
func emptyGroupFixture() model.Bookmarks {
	b := *store.EmptyBookmarks()
	b.Rev = 3
	b.Groups = []model.Group{
		{ID: "nas", Title: "NAS", Layout: model.LayoutList, Items: []model.Item{
			{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
		}},
		// 一条都没有的那一组：真实形状（条目被搬走之后剩下的），不是构造的怪东西。
		{ID: "gone", Title: "清空的组", Layout: model.LayoutList, Items: []model.Item{}},
		// 大卡那一档的空组：它进的是顶部那块带子，与正文那一份是两条路，
		// 所以要各断一次——只断正文的话，顶部会留一个没有卡片的标题。
		{ID: "feat", Title: "空的大卡", Layout: model.LayoutFeature, Items: []model.Item{}},
	}
	return b
}

func TestProfilePage_空组照画并说清它是空的(t *testing.T) {
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", emptyGroupFixture())
	r := New(s, ix, fc)

	page, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	html := string(page.Body)
	// 三个判据缺一不可：节容器、锚点、导航里的那一格。少任何一处，它就成了
	// 一个"页面上有、却进不去"的分组。
	for _, want := range []string{`data-gid="gone"`, `id="g-gone"`, `#g-gone`,
		`data-gid="feat"`, `id="f-feat"`} {
		if !strings.Contains(html, want) {
			t.Errorf("页面里没有 %q：空组被藏起来了（那会让添加浮层的下拉与拖拽落点一起失效）", want)
		}
	}
	// 而它得说清自己是空的：一个只有标题、正文空着的节读起来像渲染坏了。
	// **只数正文那一处**：大卡带子里的空组由标题上那枚条目数徽标说（它印「0」），
	// 那是那一块本来就有的载体，而往带子里塞一句 hint 会掉进它的横向排布里
	// （它不是一列一列的正文，是一个 section 里连着若干段 h2+ul）。
	if n := strings.Count(html, "这一组还没有书签"); n != 1 {
		t.Errorf("正文那个空组该有一句「这一组还没有书签」，实际 %d 句", n)
	}
	if !strings.Contains(html, `<span class="feat-n">0</span>`) {
		t.Error("大卡那个空组没有条目数徽标：带子里没有正文 hint，它靠那枚「0」说自己是空的")
	}
	// 有内容的组不许被牵连。
	if !strings.Contains(html, `id="g-nas"`) {
		t.Errorf("有书签的那一组不见了：\n%.600s", html)
	}
}

// 搜不到的时候不画（filter 那一份）与空组照画是两件事，各说各话。
func TestProfilePage_搜到东西时不把空组算成命中(t *testing.T) {
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", emptyGroupFixture())
	r := New(s, ix, fc)

	hit, err := r.ProfilePage(PageRequest{ID: "dad", Query: "群晖"})
	if err != nil {
		t.Fatal(err)
	}
	// 搜的时候空组**不在结果里**（它没有条目可命中）——但那是 filter 的判据，
	// 不是"空组该被藏起来"。这一条钉的是两者不混：searchparity 与 filter 的
	// 那几支已经各自钉过命中，这里只确认没把空组捞进来。
	if strings.Contains(string(hit.Body), `data-gid="gone"`) {
		t.Error("搜到群晖时那个空组也出现在结果里：空组没有条目，不该算命中")
	}
}

// 设置面板里那一列**不过滤**——它读的是另一次（`prefsGroups`）。
//
// 面板那一列与页面那一列现在**都**画空组（改判之后两边一致了），但仍是另一次
// 读：它多带 `Count`（删组确认句要用），而页面那份由 `profile()` 自己算。
func TestPrefsPage_空组照画并带着那颗删键(t *testing.T) {
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", emptyGroupFixture())
	r := New(s, ix, fc)

	page, err := r.PrefsPage("dad", "")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page.Body)
	if !strings.Contains(html, `name="group" value="gone"`) {
		t.Errorf("面板里没有那个空组：它清掉了就再也建不回来了\n%.600s", html)
	}
	// 每一行都得有那颗删键：ops 一直认 `remove path=groups/<gid>`，面板画不出
	// 它就等于"删一组"没有出口（与 accent/rename 当初那两格同一个洞）。
	rows := strings.Count(html, `name="group" value=`)
	if keys := strings.Count(html, `name="remove" value="1"`); keys != rows {
		t.Errorf("面板里 %d 颗删键，%d 行分组：每一行都要有出口", keys, rows)
	}
	// 空组的那一句与有内容的那一句**必须不同**：这一格的作用就是说出代价，
	// 而「还有 0 条」那句话回答不了"删它会不会少书签"。
	if !strings.Contains(html, "这一组是空的") {
		t.Errorf("空组那一行没有说清它是空的：\n%.600s", html)
	}
	if !strings.Contains(html, "这一组有 1 条书签") {
		t.Errorf("有内容的组那一行没有说出条数：\n%.600s", html)
	}
}
