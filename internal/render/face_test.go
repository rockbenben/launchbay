package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// faceForTest 是"选档案那张卡上画的是哪一张脸"的钉子。
//
// 三个形状都要盖住，因为它们各自对应 faceOf 的一档：
//
//	有缓存 favicon → <img>（那个人自己常用站点的图，最像"服务图标"）；
//	只有索引里的图标名 → 内联 SVG；
//	什么都没有 → 首字母色块（拿档案名算）。
//
// 顺序也是被钉的：**缓存 favicon 优先于图标名**。理由在 faceOf 上——一份刚
// 导进来、图标还没被抓过的档案，若按"第一条有图标的"挑就会落回名字首字母，
// 那等于改之前那张卡，所以它必须先扫一遍 favicon。
func TestFaceOf_三档与优先级(t *testing.T) {
	ix, fc, _ := assets(t)
	p := &model.Profile{ID: "dad", Title: "爸爸"}

	t.Run("缓存里有的 favicon 赢", func(t *testing.T) {
		origin := icons.Origin("http://10.0.0.9:5000/")
		e, err := fc.Put(origin, []byte{0x89, 'P', 'N', 'G'}, "png")
		if err != nil {
			t.Fatal(err)
		}
		b := &model.Bookmarks{Groups: []model.Group{{ID: "g", Layout: model.LayoutList, Items: []model.Item{
			{ID: "a", Name: "有名字的图标", URL: "http://10.0.0.1/", Icon: "tabler:home"},
			{ID: "b", Name: "有 favicon 的", URL: "http://10.0.0.9:5000/", Icon: "auto"},
		}}}}
		got := faceOf(p, b, ix, fc)
		if got.Img != "/icons/"+e.Hash+"."+e.Ext {
			t.Errorf("该用缓存里的 favicon，实际 Img=%q", got.Img)
		}
		if got.SVG != "" {
			t.Error("favicon 绝不内联成 SVG（第三方字节只做 img，见 icon.go）")
		}
	})

	t.Run("没有缓存就用索引里的图标名", func(t *testing.T) {
		b := &model.Bookmarks{Groups: []model.Group{{ID: "g", Layout: model.LayoutList, Items: []model.Item{
			{ID: "a", Name: "有名字的图标", URL: "http://10.0.0.1/", Icon: "tabler:home"},
		}}}}
		got := faceOf(p, b, ix, fc)
		if got.SVG == "" {
			t.Fatalf("该内联那个图标，实际 Letter=%q Img=%q", got.Letter, got.Img)
		}
	})

	t.Run("什么都没有就退回首字母色块", func(t *testing.T) {
		got := faceOf(p, &model.Bookmarks{}, ix, fc)
		if got.Letter != firstChar(p.Title) {
			t.Errorf("该退回档案名的首字母 %q，实际 %q", firstChar(p.Title), got.Letter)
		}
		if got.ChipHue != chipHue(p.Title) {
			t.Error("色相要跟档案页里同名的色块是同一个（chipHue 判据只有一份）")
		}
	})

	t.Run("书读不出来也是首字母", func(t *testing.T) {
		// 坏档/空档：那一句解释在点进去之后，不在这一页重复。
		if got := faceOf(p, nil, ix, fc); got.Letter != firstChar(p.Title) {
			t.Errorf("nil 书签该退回首字母，实际 %q", got.Letter)
		}
	})

	t.Run("tiles 的分组先挑", func(t *testing.T) {
		origin := icons.Origin("http://10.0.0.8/")
		e, err := fc.Put(origin, []byte{0x89, 'P', 'N', 'G'}, "png")
		if err != nil {
			t.Fatal(err)
		}
		b := &model.Bookmarks{Groups: []model.Group{
			{ID: "list", Layout: model.LayoutList, Items: []model.Item{
				{ID: "a", Name: "列表里的", URL: "http://10.0.0.1/", Icon: "tabler:home"},
			}},
			{ID: "bar", Layout: model.LayoutTiles, Items: []model.Item{
				{ID: "b", Name: "书签栏里的", URL: "http://10.0.0.8/", Icon: "auto"},
			}},
		}}
		if got := faceOf(p, b, ix, fc); got.Img != "/icons/"+e.Hash+"."+e.Ext {
			t.Errorf("书签栏（tiles）里的那条该赢，实际 Img=%q", got.Img)
		}
	})
}

// 卡上不印名字了，但**不能没有名字**：它在 title 与按钮的 aria-label 上。
//
// 这一条钉的是那个"无障碍名"：一张只有图标和「进入」两个字的卡，对读屏的人
// 来说三张卡完全一样——而它是全家共用的进门页，那正是最需要能分清谁是谁的地方。
func TestPickerPage_卡上不印名字但仍有无障碍名(t *testing.T) {
	s := pickerStore(t, "dad", "mom")
	save(t, s, "dad", "爸爸的台", "dsm")
	body := pickerBodyOf(t, s)

	// 可见的名字不许出现：档案名只进 title 与 aria-label。
	for _, gone := range []string{`<b>档案dad</b>`, `<b>档案mom</b>`, `class="pick-name"`} {
		if strings.Contains(body, gone) {
			t.Errorf("卡上不该再印 %q", gone)
		}
	}
	// 脸在那儿。
	if !strings.Contains(body, `class="pick-face"`) {
		t.Errorf("卡上该有一张脸:\n%.800s", body)
	}
	// 名字仍在两处：鼠标停上去（title）与读屏（aria-label）。
	if n := strings.Count(body, "title=\"档案"); n != 2 {
		t.Errorf("每张卡的 title 都要带档案名，实际 %d 处", n)
	}
	if n := strings.Count(body, `aria-label="进入 档案`); n != 2 {
		t.Errorf("按钮的 aria-label 要念出是谁，实际 %d 处:\n%.800s", n, body)
	}
}

// 挑选这件事不许拿档案的内容当泄露：选择页是**全家都能看到**的进门页，而它现在
// 为了画那张脸去读了每一份档案的书签。带进模板的只有一张脸（图标或首字母），
// 书签名、URL、分组名一个都不许出现在这一页。
func TestPickerPage_不泄露别人收藏里的名字与地址(t *testing.T) {
	s := pickerStore(t, "dad", "mom")
	secretName, secretURL := "只有爸爸知道的那个站", "http://10.0.0.66/secret"
	saveBookmarks(t, s, "dad", model.Bookmarks{Rev: 2, Groups: []model.Group{{
		ID: "g", Title: "只有爸爸知道的那个分组", Layout: model.LayoutList,
		Items: []model.Item{{ID: "a", Name: secretName, URL: secretURL, Alias: []string{}, Tags: []string{}}},
	}}})
	body := pickerBodyOf(t, s)
	for _, leak := range []string{secretName, secretURL, "只有爸爸知道的那个分组"} {
		if strings.Contains(body, leak) {
			t.Errorf("进门页泄露了 %q:\n%.800s", leak, body)
		}
	}
}

// faceOf 只在**选择页**读书签，别处不许多读一遍：这一页每来一次就要为每份档案
// 读一次 bookmarks.json，速度是这个产品排在第一位的卖点。读不出来时退首字母
// 而不是报错——坏档的解释页在点进去之后。
func TestPickerPage_坏书签不挡别人那张卡(t *testing.T) {
	s := pickerStore(t, "dad", "mom")
	save(t, s, "dad", "爸爸的台", "dsm")
	if err := s.WriteFileAtomic(s.BookmarksPath("mom"), []byte("{坏得读不了")); err != nil {
		t.Fatal(err)
	}
	body := pickerBodyOf(t, s)
	if n := strings.Count(body, `action="/pin"`); n != 2 {
		t.Errorf("书签坏掉不该少一张卡（坏的是 mom，dad 照旧进得去），实际 %d 张", n)
	}
	if n := strings.Count(body, `class="pick-face"`); n != 2 {
		t.Errorf("两份都该有脸（mom 那张退首字母），实际 %d 张", n)
	}
}

var _ = store.New // 保持 store 的显式引用（夹具类型）
