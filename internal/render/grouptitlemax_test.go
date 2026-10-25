package render

import (
	"strconv"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
)

// 「分组名最多 60 个字」这条规矩，从前**只写在设置面板那两个输入框上**：
// 建组那一格与每一组的改名框都是服务端渲染的，maxlength 顺手就印上了，
// 服务端那条表单通路（/prefs/group）也判它。
//
// 而「＋ 添加」浮层里那个「新分组名」输入框是**脚本造的**，忘了这一件事——
// 于是同一个动作在两个入口给出两个答案：面板里 60 字以上打不进去，浮层里
// 打多少都收。实测（2026-10-22）：浮层里存进去一个 200 字的分组名；
// 回到设置面板，那一格（maxlength=60，框宽 416px）**只显示得下前 46 个字**，
// 而那个名字还印在每个节标题、分组导航与导出文件里。
//
// 症状是「同一个动作两个答案」加上「名字看不全」：人分不清自己那份到底叫什么。
//
// 这一族钉两件事：
//   - **页面把那个数递给脚本**（app.js 是静态资产，读不到服务端的 catalog）；
//   - **脚本拿它给那一格**（数从 <body> 上读，不在脚本里写死一个字面量——
//     写死一份就多一处会漂的地方，而 model 那一份才是唯一来源）。
func Test浮层里新分组名那一格也有上限(t *testing.T) {
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", withIcons(1))
	page, err := New(s, ix, fc).ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	want := `data-max-group-title="` + strconv.Itoa(model.MaxGroupTitleRunes) + `"`
	if !strings.Contains(string(page.Body), want) {
		t.Errorf("档案页没有把分组名上限印在 <body> 上（要 %s）", want)
	}

	js := readWeb(t, "web/app.js")
	// 数从 body 上读，不在脚本里写死。
	if !strings.Contains(js, "body.dataset.maxGroupTitle") {
		t.Error("app.js 没有从 <body> 上读那个数：写死一份就多一处会漂的地方")
	}
	// 那一格拿它当 maxlength。
	i := strings.Index(js, "id: 'lb-newgroup'")
	if i < 0 {
		t.Fatal("app.js 里找不到「lb-newgroup」那一格")
	}
	line := js[i:min(i+300, len(js))]
	if !strings.Contains(line, "maxlength: maxGroupTitle") {
		t.Errorf("「新分组名」那一格没有 maxlength：\n%s", line)
	}
}

// 面板那两格的 maxlength 与这个数**必须是同一个**：三处各写一个字面量的话，
// 改一处要记得改另外两处，而漏掉的那一处症状是「界面上限与实际判的对不上」。
//
// 判据是 model 那个数，而不是抄一份 60——抄的那一份会漂。
func Test面板那两格与浮层那一个上限是同一个数(t *testing.T) {
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", withIcons(1))
	page, err := New(s, ix, fc).PrefsPage("dad", "", i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	if !strings.Contains(body, `maxlength="`+strconv.Itoa(model.MaxGroupTitleRunes)+`"`) {
		t.Errorf("设置面板上的 maxlength 不是 model.MaxGroupTitleRunes（%d）", model.MaxGroupTitleRunes)
	}
	// 浮层那一格走的是页面 <body> 上那个数——两个数得是同一个。
	prof, err := New(s, ix, fc).ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prof.Body), `data-max-group-title="`+strconv.Itoa(model.MaxGroupTitleRunes)+`"`) {
		t.Error("浮层那一格拿到的数与面板那两格不是一个")
	}
}
