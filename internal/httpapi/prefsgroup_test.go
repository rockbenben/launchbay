package httpapi

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 台面面板「分组」那一节的三条钉。每一条各挡一种谎，而三条都犯过的形状正是
// 这格之前的样子：**accent 零 UI**（ops 一直认 `groups/<id>/accent`，界面一个
// 入口都没有，于是每个类目的色块都是 CSS 的 `--sec-accent,var(--accent)` 兜底值，
// 一整页同一个蓝），而磁贴/单列只在分组标题那颗 JS 按���上，零 JS 用户看得见改不了。

// 色板上的每一颗都要在面板里画出来，且**按下的是盘上那个值**。
//
// 两条方向都要钉：少一颗是那一色选不了（"能配却没有出口"的另一种形状），
// 按下的不是当前值是"屏幕说的和盘上存的不是一件事"。
func TestPrefsGroup_色板画全且按下的就是盘上那个(t *testing.T) {
	_, h := setupBatch(t) // dad 有分组
	setGroupAccent(t, h, "dad", "nas", model.GroupAccentPalette[3])
	page := get(h, "/prefs", dadFormCookie)
	if page.Code != http.StatusOK {
		t.Fatalf("面板该 200，得到 %d", page.Code)
	}
	html := page.Body.String()
	for _, c := range model.GroupAccentPalette {
		if !strings.Contains(html, `name="accent" value="`+c+`"`) {
			t.Errorf("色板上 %s 那一颗没画出来：选不了这一色", c)
		}
	}
	// 「跟随页面主色」也是一颗能按的键，不是装饰：没有它，从自定义色回不到默认。
	if !strings.Contains(html, `class="prefs-swatch prefs-swatch-none"`) {
		t.Error("面板里没有「跟随页面主色」那一颗：从自定义色回不到默认，色板就成了单向门")
	}
	// 自定义那一格必须真的有原生取色器（accent 收任意 hex 的唯一入口）。
	if !strings.Contains(html, `type="color"`) {
		t.Error("面板里没有取色器：色板之外的颜色一个都选不了")
	}
	if got, want := pressedAccents(html, "nas"), model.GroupAccentPalette[3]; len(got) != 1 || got[0] != want {
		t.Errorf("nas 这一行按下的色是 %v，要恰好是盘上那个 %q", got, want)
	}
}

// 面板改得出颜色也改得出磁贴/单列，**且走的是书签写通路**。
//
// 后一半比前半更值钱：这两个字段落在 bookmarks.json 上，所以 rev 必须推进、
// 历史必须有一行、快照必须先于落盘——那三道闸是"改坏了能退回去"那条承诺的全部。
// 走成"只写 profile.json"的话，界面显示改了而书签文件一个字节没动。
func TestPrefsGroup_颜色与排法都改得动且推进rev(t *testing.T) {
	s, h := setupBatch(t)
	before := currentRev(t, s, "dad")
	rec := postForm(h, "/prefs/group", "group=nas&accent=%23e0574f", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("改颜色要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/prefs" {
		t.Errorf("Location = %q，要 /prefs（改颜色是要连着改好几组的事，回首页就断了）", loc)
	}
	postForm(h, "/prefs/group", "group=nas&layout=list", dadFormCookie)
	b := loadBookmarks(t, s, "dad")
	var nas *model.Group
	for i := range b.Groups {
		if b.Groups[i].ID == "nas" {
			nas = &b.Groups[i]
		}
	}
	if nas == nil {
		t.Fatalf("nas 这一组不见了")
	}
	if nas.Accent != "#e0574f" {
		t.Errorf("盘上 accent = %q，要 #e0574f", nas.Accent)
	}
	if nas.Layout != model.LayoutList {
		t.Errorf("盘上 layout = %q，要 list", nas.Layout)
	}
	if after := currentRev(t, s, "dad"); after != before+2 {
		t.Errorf("rev = %d，要 %d：两次改动各推进一格，不推进就等于没写进书签文件", after, before+2)
	}
	if !hasHistoryBy(t, s, "dad", "web") {
		t.Error("审计线里没有 by=web 的行：这一格是网页改的，不记就分不清是谁动的")
	}
}

// 空 accent 是**合法值**（= 没设色），而不是"没说要改什么"。
//
// 这条钉的是那个容易搞混的方向：把空串当非法，界面就没有"回到默认"那颗键；
// 把空串当"没给"，按那颗键就会静默什么都不做——两者都会让人以为回不去了。
func TestPrefsGroup_空accent是回到默认不是没改(t *testing.T) {
	s, h := setupBatch(t)
	setGroupAccent(t, h, "dad", "nas", "#dd6aa6")
	if rec := postForm(h, "/prefs/group", "group=nas&accent=", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("清回默认要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	if a := groupAccent(t, s, "dad", "nas"); a != "" {
		t.Errorf("盘上 accent = %q，要空串（没设色，色块退回页面主色）", a)
	}
	html := get(h, "/prefs", dadFormCookie).Body.String()
	if got := pressedAccents(html, "nas"); len(got) != 1 || got[0] != "" {
		t.Errorf("清空之后按下的应该是「跟随页面主色」那一颗，实际 %v", got)
	}
}

// 形状错要 **422 且点名**，而且**不许写盘**。
//
// 分组 id 这一格先判的理由：ops 的路径文法本来就只收 ASCII slug，可它报出来
// 的是一句"路径解析失败"——那对表单作者说的是路径，而这一格收的其实是分组 id。
func TestPrefsGroup_形状错422点名且不写盘(t *testing.T) {
	s, h := setupBatch(t)
	before := currentRev(t, s, "dad")
	for _, c := range []struct{ body, field string }{
		{"accent=%23e0574f", "group"},
		{"group=nas", "accent"},
		{"group=nas&accent=not-a-colour&layout=list", "accent"},
		{"group=nas&layout=columns", "layout"},
		{"group=nas&accent=%23e0574f&accent=%2300ff00", "accent"},
	} {
		rec := postForm(h, "/prefs/group", c.body, dadFormCookie)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%q 要 422，得到 %d", c.body, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), `"field": "`+c.field+`"`) {
			t.Errorf("%q 的 422 要点名 %s：%s", c.body, c.field, rec.Body.String())
		}
	}
	if after := currentRev(t, s, "dad"); after != before {
		t.Errorf("被拒的那几次不许写盘，rev 从 %d 变成了 %d", before, after)
	}
}

// 分组已经被删了 → **面板重画回来并说清为什么**，不是 303 去一个看起来像
// 成功的页面。书签是别人也在改的东西，面板开着的时候那一组完全可能没了。
func TestPrefsGroup_分组没了要说清而不是静默成功(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/prefs/group", "group=ghost&accent=%23e0574f", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("分组没了要把面板重画回来（200），得到 %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `class="notice"`) {
		t.Errorf("页面上没有那句为什么：\n%.600s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `class="prefs"`) {
		t.Error("没有把面板本身重画出来：人到了一个只有一句话的页面上")
	}
}

// 坏档**只撤掉那一节**，其余四行照常可用。
//
// 这条钉的是"坏档不挡"的**范围**：改台面偏好只写 profile.json，而书签文件读坏
// 了正是最需要改排版看看的时候——所以不能整页退化成一句话。可反过来，面板里
// 那一节画的是盘上分组的样子，读不出来时画一屏猜的同样是谎。
func TestPrefsGroup_坏档只撤掉分组那一节(t *testing.T) {
	s, h := setupBatch(t)
	writeBrokenBookmarks(t, s)
	rec := get(h, "/prefs", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("坏档不该挡这一页，得到 %d", rec.Code)
	}
	html := rec.Body.String()
	for _, want := range []string{`name="theme"`, `name="view"`, `name="layout"`, `name="title"`} {
		if !strings.Contains(html, want) {
			t.Errorf("坏档时 %s 那一行不见了：改台面只写 profile.json，它照旧该能用", want)
		}
	}
	if strings.Contains(html, `class="prefs-groups"`) {
		t.Error("分组清单画出来了：书签文件读不动，这一节的每一行都是猜的")
	}
	if !strings.Contains(html, `data-prefs-groups-broken`) {
		t.Error("没有说清这一节为什么不在")
	}
}

// ── 本文件的夹具 ────────────────────────────────────────────────────────────
// 上面六条各自只测一件事，这一堆 helper 是它们共用的读法。**不放进别的测试
// 文件**：同包同名的第二份定义是编译错误，而"换个名字就是第二套夹具"。

// setGroupAccent 走与界面**完全同一条**通路（POST /prefs/group），不用 batch
// 绕过去：夹具走一条路、断言查另一条路，那两条路迟早分家，而这里分家的症状是
// "测试全绿而界面改不动"。
func setGroupAccent(t *testing.T, h http.Handler, profile, group, accent string) {
	t.Helper()
	rec := postForm(h, "/prefs/group", "group="+group+"&accent="+url.QueryEscape(accent), dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("夹具：给 %s/%s 设色 %q 要 303，得到 %d: %s", profile, group, accent, rec.Code, rec.Body)
	}
}

func loadBookmarks(t *testing.T, s *store.Store, id string) *model.Bookmarks {
	t.Helper()
	b, err := s.LoadBookmarks(id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func currentRev(t *testing.T, s *store.Store, id string) int64 {
	t.Helper()
	r, err := s.CurrentRev(id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func groupAccent(t *testing.T, s *store.Store, profile, id string) string {
	t.Helper()
	for _, g := range loadBookmarks(t, s, profile).Groups {
		if g.ID == id {
			return g.Accent
		}
	}
	t.Fatalf("没有 %s 这一组", id)
	return ""
}

// hasHistoryBy 查审计线里有没有某条 by 的行。`by=web` 是这一格的身份：它与
// 扩展、CLI、导入写的是同一份数据，分不开就答不出"这一格是人点的还是机器改的"。
func hasHistoryBy(t *testing.T, s *store.Store, profile, by string) bool {
	t.Helper()
	hist, err := s.ReadHistory(profile, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range hist {
		if e.By == by {
			return true
		}
	}
	return false
}

// groupFormRE 抓面板里**某一个分组那一行**：从它的 hidden group 值起，到下一个
// 分组（或这一节末尾）止。用它切块而不是整页找，是为了让"这一行"这件事在
// 测试里有定义——同一个 group 值可能在别处也出现（比如这一格 id 被印在别处）。
var groupFormRE = regexp.MustCompile(`(?s)<form method="post" action="/prefs/group".*?</form>`)

func groupForm(html, id string) string {
	for _, m := range groupFormRE.FindAllString(html, -1) {
		if strings.Contains(m, `name="group" value="`+id+`"`) {
			return m
		}
	}
	return ""
}

// pressedAccents 返回那一行里 aria-pressed="true" 的颜色值列表。
//
// 空串那一颗（「跟随页面主色」）提交的值就是空串，所以它以 `value=""` 出现，
// 列表里也就有一个空元素——**它必须能被数到**，否则"清回默认之后按下的那颗"
// 这件事根本没法断言。
// 中间允许别的属性：模板里 `value` 后面还跟着 `class` 与 `style`，
// 逐字相邻的那种正则会在**排一次属性顺序**时静默失配——症状是"按下的那几颗
// 数出来是空的"，看起来像 pressed 压根没画。
var pressedAccentRE = regexp.MustCompile(`name="accent" value="([^"]*)"[^>]*aria-pressed="true"`)

func pressedAccents(html, id string) []string {
	form := groupForm(html, id)
	if form == "" {
		return nil
	}
	var out []string
	for _, m := range pressedAccentRE.FindAllStringSubmatch(form, -1) {
		out = append(out, m[1])
	}
	return out
}

// writeBrokenBookmarks 把 dad 的书签文件写成读不动的那种坏字节。判据用**真的
// 读不动**，而不是"删掉文件"——后者是档案没了，走的是另一条分诊。
func writeBrokenBookmarks(t *testing.T, s *store.Store) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.Root(), "profiles", "dad", "bookmarks.json"),
		[]byte("{ 坏掉的字节"), 0o644); err != nil {
		t.Fatal(err)
	}
}
