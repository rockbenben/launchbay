package httpapi

import (
	"github.com/rockbenben/launchbay/internal/i18n"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 「改名」那条通路（2026-10-15）。分组名此前在界面上一个出口都没有——ops 一直认
// `set path=groups/<gid>/title`，而面板只画得出颜色与排法。
//
// 这一族钉的是**「值没变就不算一次改名」**：改名那一格常驻在表单里（输入框每次
// 提交都跟着走），所以按一颗色块也会带上 `rename=<原名>`。若照单全收，"改个颜色"
// 就变成"改个颜色顺手把名字重写一遍"——而 applyBatch 只看 opList 非空，于是**按
// 一下已经在用的那颗色块也会存一版快照并把 rev +1**，在「回退」里留下一条什么都没
// 改的版本。
func TestPrefsGroup_改名只在真变了的时候发生(t *testing.T) {
	s, h := setupBatch(t)

	// 夹具的组名从**渲染出来的面板**里取，不写死。
	page := get(h, "/prefs", dadFormCookie).Body.String()
	gid, title := firstRenameBox(t, page)
	if gid == "" {
		t.Fatal("面板里没有改名那一格")
	}
	revBefore := loadBookmarks(t, s, "dad").Rev

	// 一、原样交回去：一条 op 都不该发。
	rec := postGroupForm(t, h, url.Values{"group": {gid}, "rename": {title}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("原样提交该 303，got %d（%s）", rec.Code, rec.Body.String())
	}
	if got := loadBookmarks(t, s, "dad").Rev; got != revBefore {
		t.Errorf("什么都没改却把 rev 从 %d 推到了 %d——「按一下已经在用的那颗色块」会存一版快照", revBefore, got)
	}

	// 二、真改：落库，且只有这一组的标题变了。
	const want = "NAS 与存储（改过）"
	rec = postGroupForm(t, h, url.Values{"group": {gid}, "rename": {want}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("改名该 303，got %d（%s）", rec.Code, rec.Body.String())
	}
	g := findGroup(t, s, "dad", gid)
	if g.Title != want {
		t.Errorf("落库的组名 = %q，要 %q", g.Title, want)
	}
	if got := loadBookmarks(t, s, "dad").Rev; got != revBefore+1 {
		t.Errorf("改名后 rev = %d，要 %d（一次改名存一版）", got, revBefore+1)
	}
	// **id 不许跟着变**：分组 id 是寻址键，而分组导航、别人的链接与导出文件都指着它。
	if gid2, _ := firstRenameBox(t, get(h, "/prefs", dadFormCookie).Body.String()); gid2 != gid {
		t.Errorf("改名之后这一组的 id 从 %q 变成了 %q", gid, gid2)
	}

}

// 名字那一格的三种坏形状。**每一条都要点名 rename**——它说的是"名字那一格没填/
// 太长"，而 ops 那句「title 为空」说的是文档校验，说不出是哪一格。
func TestPrefsGroup_改名的坏形状点名rename(t *testing.T) {
	s, h := setupBatch(t)
	gid, _ := firstRenameBox(t, get(h, "/prefs", dadFormCookie).Body.String())
	before := findGroup(t, s, "dad", gid).Title

	cases := []struct {
		name   string
		vals   url.Values
		notice bool
	}{
		// 名字空着是人按得到的一发 → 面板重画 + 横幅点名那一格（batch11 T2，
		// 与建组那一支同一判据）。另两种浏览器的形状给不出：名字框带 maxlength，
		// 同名多值要手拼 body —— 那仍是 422 + 中文原话，给拼请求的人看。
		{"空着（想改成空名字）", url.Values{"group": {gid}, "rename": {"   "}}, true},
		{"太长了", url.Values{"group": {gid}, "rename": {strings.Repeat("字", 61)}}, false},
		{"两个 rename", url.Values{"group": {gid}, "rename": {"甲", "乙"}}, false},
	}
	for _, tc := range cases {
		rec := postGroupForm(t, h, tc.vals)
		if tc.notice {
			want := i18n.Load(i18n.Zh).T("prefs.err.renameEmpty")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
				t.Errorf("%s 要把面板重画回来并说出那一格（要含 %q），得到 %d：%s",
					tc.name, want, rec.Code, trimBody(rec.Body.String()))
			}
			if got := findGroup(t, s, "dad", gid).Title; got != before {
				t.Errorf("%s 把名字改成了 %q——形状错的那一条不许写进去", tc.name, got)
			}
			continue
		}
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s 该 422，got %d（%s）", tc.name, rec.Code, trimBody(rec.Body.String()))
			continue
		}
		// 422 的正文是缩进过的 JSON，而这一族不判空白之外的任何东西：
		// 把所有空白去掉再找字段名，免得哪天缩进一改这条闸就假红。
		if flat := strings.ReplaceAll(rec.Body.String(), " ", ""); !strings.Contains(flat, `"field":"rename"`) {
			t.Errorf("%s 的报错没有点名 rename：%.200s", tc.name, rec.Body.String())
		}
		if got := findGroup(t, s, "dad", gid).Title; got != before {
			t.Errorf("%s 把名字改成了 %q——形状错的那一条不许写进去", tc.name, got)
		}
	}
}

// **空着 + 顺手按了色块**不许被连坐：那是"输入框里留着上一次敲完没提交的内容"，
// 说清"是名字那一格超长"不如说清"按的是哪一颗"。而颜色那一格**必须照改**。
func TestPrefsGroup_空着的名字不连坐颜色(t *testing.T) {
	s, h := setupBatch(t)
	gid, title := firstRenameBox(t, get(h, "/prefs", dadFormCookie).Body.String())
	const hex = "#22b8a0"

	rec := postGroupForm(t, h, url.Values{"group": {gid}, "rename": {""}, "accent": {hex}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("按色块该 303，got %d（%s）", rec.Code, trimBody(rec.Body.String()))
	}
	g := findGroup(t, s, "dad", gid)
	if g.Title != title {
		t.Errorf("组名被一个空输入框改成了 %q", g.Title)
	}
	if g.Accent != hex {
		t.Errorf("颜色 = %q，要 %q——「按的是哪颗」必须说了算", g.Accent, hex)
	}
}

// 「按一下**已经在用**的那颗」不许写盘（2026-10-15）。
//
// applyBatch 只看 opList 非空：只要有一条 op 落进去就**一定**存一版快照并把
// rev +1。于是不改判据的话，"手滑再按一次那颗已经在用的色块"会在「回退」里
// 留下一条什么都没改的版本——而这一节自己写着「每**改**一次会存一版」。
//
// 三个格子各按一次：颜色、排法、名字。**判据是值而不是按下的那颗键**，所以
// 这一族要三样都压掉；漏一样就只在那一样上复发。
func TestPrefsGroup_按一下已经在用的那一格不写盘(t *testing.T) {
	s, h := setupBatch(t)
	gid, title := firstRenameBox(t, get(h, "/prefs", dadFormCookie).Body.String())
	g := findGroup(t, s, "dad", gid)
	curAccent, curLayout := g.Accent, g.Layout
	before := loadBookmarks(t, s, "dad").Rev

	// 夹具里这一组可能本来就没设色；先按一颗确定不同于当前的，再按它一次。
	if curAccent == "" {
		setGroupAccent(t, h, "dad", gid, "#4488ff")
		curAccent = "#4488ff"
	}
	before = loadBookmarks(t, s, "dad").Rev

	for _, tc := range []struct {
		name string
		vals url.Values
	}{
		{"颜色（当前值）", url.Values{"group": {gid}, "accent": {curAccent}}},
		{"排法（当前值）", url.Values{"group": {gid}, "layout": {curLayout}}},
		{"名字（原样）", url.Values{"group": {gid}, "rename": {title}}},
	} {
		rec := postGroupForm(t, h, tc.vals)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s：原样提交该 303，got %d", tc.name, rec.Code)
		}
		if got := loadBookmarks(t, s, "dad").Rev; got != before {
			t.Errorf("%s：什么都没改却把 rev 从 %d 推到了 %d——「按一下已经在用的那颗」在回退里留下一条空版本",
				tc.name, before, got)
		}
	}
}

// 面板上那一格必须**是输入框**，而且带着服务端那个上限。
//
// 判据钉的是三样：名字不再是一段印死的字（`value` 就是当前标题）、
// 上限与 model 那个数同源（服务端判的与浏览器拦的是同一个数）、
// 它常驻在**每一条** form 里（漏一组就等于那一组永远改不了名）。
func TestPrefsPanel_每一组都有一个带上限的名字输入框(t *testing.T) {
	_, h := setupBatch(t)
	page := get(h, "/prefs", dadFormCookie).Body.String()

	// 分组表单按**它带着 group id** 数（`name="group"`），不按 `action` 数：
	// 「＋ 新分组」那张（2026-10-15）走同一个 action，只是它没有 id 那一格——
	// 新组的 id 是服务端派生的（`store.NextGroupID`）。按 action 数会把那张
	// 也算进来，于是"每一组都有一个改名框"这条就变成假红。
	forms := strings.Count(page, `name="group" value=`)
	boxes := strings.Count(page, `name="rename"`)
	if forms == 0 {
		t.Fatal("面板里没有分组表单")
	}
	if boxes != forms {
		t.Errorf("面板上 %d 张分组表单却有 %d 个改名输入框：每一组都要有一个", forms, boxes)
	}
	// 建组那张：有一个 `add` 输入框，而**没有** `group` 那一格（id 由服务端派生，
	// 界面上不问——与 `/setup`、`/new` 与管理页那三张建档表单同一族决定）。
	if got := strings.Count(page, `name="add"`); got != 1 {
		t.Errorf("面板里 %d 个建组输入框，要 1 个（清空的那一份没有它）", got)
	}
	// 上限：模板上印的那个数必须与 model 那个数一致（改一处要改另一处）。
	if !strings.Contains(page, `maxlength="`+strconv.Itoa(model.MaxGroupTitleRunes)+`"`) {
		t.Errorf("改名输入框的 maxlength 不是 model.MaxGroupTitleRunes（%d）", model.MaxGroupTitleRunes)
	}
	// 输入框里就是当前标题（`value` 而不是一段印死的字）。
	if _, title := firstRenameBox(t, page); title == "" {
		t.Error("改名输入框的 value 是空的：名字得在框里")
	}
}

// postGroupForm 交一张 /prefs/group 表单。**用 url.Values 而不是手拼字符串**：
// 这一族的坏形状（空名字、超长名字）都含要转义的字，手拼的那些错误会变成
// "测试自己错了"而不是"服务端答错了"。
func postGroupForm(t *testing.T, h http.Handler, v url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return postForm(h, "/prefs/group", v.Encode(), dadFormCookie)
}

// firstRenameBox 从渲染出来的面板里取**第一张表单的那一格**：(分组 id, 当前名字)。
// 两者都从面板读，不写死——写死了夹具一改就假红，而假红的闸护不住任何一边。
func firstRenameBox(t *testing.T, html string) (gid, title string) {
	t.Helper()
	f := groupForm(html, firstGroupID(t, html))
	if f == "" {
		return "", ""
	}
	m := regexp.MustCompile(`name="rename" class="prefs-group-rename" value="([^"]*)"`).FindStringSubmatch(f)
	if len(m) != 2 {
		t.Fatal("分组表单里读不到改名那一格")
	}
	return firstGroupID(t, html), m[1]
}

func firstGroupID(t *testing.T, html string) string {
	t.Helper()
	m := regexp.MustCompile(`name="group" value="([^"]+)"`).FindStringSubmatch(html)
	if len(m) != 2 {
		t.Fatal("面板里没有分组表单")
	}
	return m[1]
}

func trimBody(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}
