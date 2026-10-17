package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 选择页那张卡上的脸（2026-10-17 用户决定）：**永远是档案名的首字母色块**。
//
// 从前是「这个人最常用站点的图标」三取一（favicon → 图标名 → 首字母）。改成文字
// 头像的四条理由记在 icon.go 的 faceOf 上：会认错人、两份档案可能常用同一个服务、
// 它当着全家人的面说出了收藏内容、而它要读整份 bookmarks.json 才画得出来——
// 后两条在类型上也堵死了：faceOf 现在只收一份 profile，而 render.go 那个调用点
// 要是还想传书签，连编译都过不去（这一页每来一次就是每份档案读一次）。
//
// 这里钉的是**单数**：不再有第二档。将来谁想聪明一点（比如名字太短时改用图标），
// 红的是这一条，而不是等到有人把「爸爸的台」看成别人的账号照片。
func TestFaceOf_永远是档案名的首字母色块(t *testing.T) {
	p := &model.Profile{ID: "dad", Title: "爸爸的台"}
	got := faceOf(p)
	if got.Letter != firstChar(p.Title) {
		t.Errorf("脸该是档案名的首字母 %q，实际 %q", firstChar(p.Title), got.Letter)
	}
	if got.SVG != "" || got.Img != "" {
		t.Errorf("脸上不该再有图标（SVG=%q Img=%q）：这一页每个人都看得见，而「这份收藏里最常点哪个站」不该印在这儿", got.SVG, got.Img)
	}
	if got.ChipHue != chipHue(p.Title) {
		t.Error("色相要跟档案页里同名的色块是同一个（chipHue 判据只有一份）")
	}
}

// 卡上**名字与服务图标都在**（2026-10-12）。
//
// 这一条钉的是"图标不许顶掉名字"：两个档案完全可能常用同一个服务（都用 GitHub），
// 那时两张卡的图标一模一样——名字是区分档案的那一维，图标负责好看与"认得出来"。
// 曾经有一版只留图标、把名字藏进 title 与 aria-label，那是把"好看"做成了"认不出
// 谁是谁"，已撤。
//
// aria-label 仍然要：读屏停在「进入」上时，念出来的是"进入爸爸的书签页"而不是
// 光一个"进入"——页面上看得见的名字补不了按钮自己的可访问名。
func TestPickerPage_名字与图标都在卡上(t *testing.T) {
	s := pickerStore(t, "dad", "mom")
	save(t, s, "dad", "爸爸的台", "dsm")
	body := pickerBodyOf(t, s)

	// 两样都在：名字（区分档案）与脸（那个服务的图标）。
	if !strings.Contains(body, `<b class="pick-name">档案dad</b>`) {
		t.Errorf("卡上要看得见档案名:\n%.800s", body)
	}
	if !strings.Contains(body, `class="pick-face"`) {
		t.Errorf("卡上要有服务图标:\n%.800s", body)
	}
	if n := strings.Count(body, `aria-label="进入档案`); n != 2 {
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
