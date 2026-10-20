package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 「点开书签」那一格（2026-10-14）。这一族钉的是**它落在哪条通路上**：它改的是
// 书签文档，所以每一次按下都要有 rev 与快照（能在「回退」里退回来）——而上面那几行
// 台面偏好改完不产生任何版本。钉这条的原因是两族混在一屏里时，**只有落点不同**这一件
// 事屏上看不出来。

func TestPrefsTabs_改的是书签文档所以有版本也能退回来(t *testing.T) {
	s, h := setup(t)
	putGroup(t, s, "dad", "g1")
	seedItem(t, s, "dad", "g1", "yt", "https://youtube.com/")
	rev0 := revOf(t, s, "dad")
	if sn := snapshotCountOf(t, s, "dad"); sn != 0 {
		t.Fatalf("夹具一开始就有 %d 份快照", sn)
	}
	before := getBody(t, h, "/", dadFormCookie)
	if !strings.Contains(before, `<a class="card" href="https://youtube.com/"`) {
		t.Fatalf("夹具的条目没渲染成链接：%.200s", before)
	}

	if rec := postForm(h, "/prefs/tabs", "tabs=1", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("切到新标签页要 303，得到 %d：%s", rec.Code, rec.Body)
	}
	if rev := revOf(t, s, "dad"); rev != rev0+1 {
		t.Errorf("rev 没推进（%d → %d）：这一格改的是书签文档，不是台面偏好", rev0, rev)
	}
	if sn := snapshotCountOf(t, s, "dad"); sn != 1 {
		t.Errorf("快照份数 = %d，要 1（能退回来是这一族与上面那几行的分别）", sn)
	}
	after := getBody(t, h, "/", dadFormCookie)
	if !strings.Contains(after, `target="_blank"`) {
		t.Errorf("页面没带上 target=_blank：%s", headLines(after, 4))
	}

	// 切回去：再推进一版，于是这一格能一路退回去
	if rec := postForm(h, "/prefs/tabs", "tabs=0", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("切回当前标签页要 303，得到 %d", rec.Code)
	}
	if rev := revOf(t, s, "dad"); rev != rev0+2 {
		t.Errorf("rev = %d，要 %d", rev, rev0+2)
	}
	// 切回来之后，**书签卡**上不许再带 target=_blank。
	//
	// 收窄到那一张卡（2026-10-20）：整页扫 `target="_blank"` 会撞上**外链**——
	// 顶栏那枚 GitHub 猫头永远新标签打开（它是站外的链接，离开这一页就丢了
	// 面板状态，而这一格管的是**书签**在不在新标签页打开）。这一条要问的是
	// 「书签怎么开」，不是「页面上有没有哪个链接新标签打开」。
	if p := getBody(t, h, "/", dadFormCookie); strings.Contains(cardTagOf(p, `class="card" href="https://youtube.com/"`), `target="_blank"`) {
		t.Error("切回来之后书签卡还在发 target=_blank")
	}
}

// cardTagOf 从页面里取出**那一张书签卡**的 <a> 标签（从 class 到 > 为止）。
//
// 为什么不再扫整页：页面上有别的 target=_blank（站外的链接），而这一格管的是
// 书签怎么开——整页扫会把「站外链接新标签打开」也算成「书签卡新标签打开」。
func cardTagOf(page, marker string) string {
	i := strings.Index(page, marker)
	if i < 0 {
		return ""
	}
	j := strings.Index(page[i:], ">")
	if j < 0 {
		return ""
	}
	return page[i : i+j+1]
}

func TestPrefsTabs_只认0或1且拒了不写盘(t *testing.T) {
	s, h := setup(t)
	putGroup(t, s, "dad", "g1")
	seedItem(t, s, "dad", "g1", "yt", "https://youtube.com/")
	rev0 := revOf(t, s, "dad")

	for _, body := range []string{"", "tabs=", "tabs=true", "tabs=yes", "tabs=2", "tabs=0&tabs=1"} {
		rec := postForm(h, "/prefs/tabs", body, dadFormCookie)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("body=%q 要 422（枚举写侧严），得到 %d", body, rec.Code)
			continue
		}
		var fe struct {
			Errors []struct{ Field, Reason string } `json:"errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &fe); err != nil || len(fe.Errors) == 0 || fe.Errors[0].Field != "tabs" {
			t.Errorf("body=%q 的 422 要点名 tabs 字段：%s", body, rec.Body)
		}
		if rev := revOf(t, s, "dad"); rev != rev0 {
			t.Errorf("body=%q 被拒之后 rev 却动了（%d → %d）", body, rev0, rev)
		}
	}
	if sn := snapshotCountOf(t, s, "dad"); sn != 0 {
		t.Errorf("被拒的那些不该留下快照（%d 份）", sn)
	}
}

// 令牌一律 **303 回选择页**：这一族走 `selfProfileOnly`，而它对 bearer 的答法是
// redirectPicker（不是 403）——`/prefs` 与 `/prefs/group` 同一族，这里跟着它们。
func TestPrefsTabs_令牌被送回选择页(t *testing.T) {
	_, h := setup(t)
	if rec := post(h, "/prefs/tabs", "tabs=1", "", dadTok); rec.Code != http.StatusSeeOther {
		t.Errorf("带档案令牌要 303 回选择页（这一族只认会话），得到 %d：%s", rec.Code, rec.Body)
	}
}

// 面板里那一格画得出来，且两颗键的 pressed 跟着盘上那个值走。
func TestPrefsTabs_面板上那一格跟着盘上走(t *testing.T) {
	_, h := setup(t)
	page := getBody(t, h, "/prefs", dadFormCookie)
	if !strings.Contains(page, `name="tabs" value="0" aria-pressed="true"`) {
		t.Errorf("夹具那一档该是「当前标签页」按下：%.300s", page)
	}
	if !strings.Contains(page, `name="tabs" value="1" aria-pressed="false"`) {
		t.Errorf("另一颗不该同时按下：%.300s", page)
	}
	postForm(h, "/prefs/tabs", "tabs=1", dadFormCookie)
	page = getBody(t, h, "/prefs", dadFormCookie)
	if !strings.Contains(page, `name="tabs" value="1" aria-pressed="true"`) ||
		!strings.Contains(page, `name="tabs" value="0" aria-pressed="false"`) {
		t.Errorf("切过去之后 pressed 没跟着走：%.300s", page)
	}
}

// getBody 顺带把状态码钉上：这一族的几条断言都在读页面，而"页面 200"这件事
// 不该由每条断言各自确认。
func getBody(t *testing.T, h http.Handler, path, cookie string) string {
	t.Helper()
	rec := get(h, path, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s 要 200，得到 %d：%s", path, rec.Code, firstBytes(rec.Body.String()))
	}
	return rec.Body.String()
}

// putGroup 在某个档案里造一个空分组（seedItem 只往**已存在**的分组里追加条目）。
func putGroup(t *testing.T, s *store.Store, id, gid string) {
	t.Helper()
	if err := s.WithProfileLock(id, func() error {
		b, err := s.LoadBookmarks(id)
		if err != nil {
			return err
		}
		b.Groups = append(b.Groups, model.Group{ID: gid, Title: gid, Layout: model.LayoutTiles, Items: []model.Item{}})
		return s.SaveBookmarks(id, b)
	}); err != nil {
		t.Fatal(err)
	}
}
