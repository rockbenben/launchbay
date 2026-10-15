package httpapi

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 台面面板「分组」那一节的三条钉。每一条各挡一种谎，而三条都犯过的形状正是
// 这格之前的样子：**accent 零 UI**（ops 一直认 `groups/<id>/accent`，界面一个
// 入口都没有，于是每个类目的色块都是 CSS 的 `--sec-accent,var(--accent)` 兜底值，
// 一整页同一个蓝），而磁贴/单列只在分组标题那颗 JS 按钮上，零 JS 用户看得见改不了。

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
	// 自定义那一格必须真的有原生取色器，而且**它不许与色板那颗键同名**：
	// 命名过的输入框每一次提交都跟着走，同名就等于每一次按色块都交两个 accent。
	// 这一格改名 custom 之后，颜色由 apply_custom 那颗键说话（判据在 prefs.go）。
	if !strings.Contains(html, `type="color"`) {
		t.Error("面板里没有取色器：色板之外的颜色一个都选不了")
	}
	if !strings.Contains(html, `name="custom"`) {
		t.Error("取色器那一格没有改名成 custom：它会随每一次提交跟着走")
	}
	if !strings.Contains(html, `name="apply_custom" value="1"`) {
		t.Error("「用这个颜色」那颗键没带 name=apply_custom：后端分不出人按的是哪一颗")
	}
	if got, want := pressedAccents(html, "nas"), model.GroupAccentPalette[3]; len(got) != 1 || got[0] != want {
		t.Errorf("nas 这一行按下的色是 %v，要恰好是盘上那个 %q", got, want)
	}
}

// 每一颗键都要说得出"这是哪一组的哪一颗"，而那颗动作键不许装成开关。
//
// 两条都是从取景实测来的：一块六组的面板上 13 个可及名各重复六遍
// （磁贴/单列/用这个颜色/跟随主色/八色/自定义颜色 ×6），读屏听得到按下与否，
// 听不到这是谁的；而「用这个颜色」带着 aria-pressed，于是它一边说"按下我"
// 一边表示"自定义生效中"——那是第三种形状，这一族里没有它。
//
// 组名从**同一张表单里**读，不写死：写死了以后改档案标题就假红，而那才是这条
// 判据真正要防的漂移（名字与键分家）。
func TestPrefsGroup_每颗键带组名而动作键不装开关(t *testing.T) {
	_, h := setupBatch(t)
	// 分组 id 从**渲染出来的面板**里取，不写死：写死了夹具一改分组名单这条就假红，
	// 而假红的闸护不住任何一边。每一组都过一遍——这一族的形状在各组是同一份标记，
	// 而"同一份标记"这件事正是被测的对象。
	first := get(h, "/prefs", dadFormCookie).Body.String()
	ids := regexp.MustCompile(`name="group" value="([^"]+)"`).FindAllStringSubmatch(first, -1)
	if len(ids) < 1 {
		t.Fatalf("夹具里 dad 一个分组都没有，这一条没有可量的形状")
	}
	for _, m := range ids {
		gid := m[1]
		setGroupAccent(t, h, "dad", gid, "#123456") // 盘上一个不在色板里的颜色
		html := get(h, "/prefs", dadFormCookie).Body.String()
		form := groupForm(html, gid)
		if form == "" {
			t.Fatalf("面板里没有 %s 那张 form", gid)
		}
		// 组名从**那个输入框的 value** 读（2026-10-15 起名字那一格就是它自己，
		// 不再是一段印死的字 + 一个 title）。判据的意图没变：组名与键读的是同一份，
		// 写死一个名字就会在改档案标题那天假红。
		nm := regexp.MustCompile(`name="rename" class="prefs-group-rename" value="([^"]*)"`).FindStringSubmatch(form)
		if len(nm) != 2 {
			t.Fatalf("%s 那一行没读到组名", gid)
		}
		labels := regexp.MustCompile(`aria-label="([^"]+)"`).FindAllStringSubmatch(form, -1)
		// 磁贴/单列/用这个颜色/跟随主色/八色/取色器 = 12 颗，一颗都不许漏前缀。
		if len(labels) < 12 {
			t.Errorf("%s 那一行只有 %d 个 aria-label，要 12 颗键都有可及名", gid, len(labels))
		}
		for _, l := range labels {
			if !strings.HasPrefix(l[1], nm[1]+" · ") {
				t.Errorf("%s 里这颗键的可及名 %q 没带组名 %q——读屏只听得见重复的那半句", gid, l[1], nm[1])
			}
		}
		// 那颗动作键：有 name=apply_custom，但**没有** aria-pressed。
		apply := regexp.MustCompile(`<button[^>]*prefs-swatch-apply[^>]*>`).FindString(form)
		if apply == "" {
			t.Fatalf("%s 那一行没找到「用这个颜色」那颗键", gid)
		}
		if strings.Contains(apply, "aria-pressed") {
			t.Errorf("动作键带了 aria-pressed（%s）：它没有按下/没按下可言，状态该由取色器那一格说", apply)
		}
		// 而"现在生效的是自定义"这一维落在取色器身上：#123456 不在色板里 → 有环。
		well := regexp.MustCompile(`<input[^>]*type="color"[^>]*>`).FindString(form)
		if !strings.Contains(well, `data-active="1"`) {
			t.Errorf("盘上是自定义色而取色器那一格没带 data-active：%s", well)
		}
		// 换回色板里的颜色，环要跟着撤——留着就是两颗键同时说"是我"。
		setGroupAccent(t, h, "dad", gid, model.GroupAccentPalette[0])
		well = regexp.MustCompile(`<input[^>]*type="color"[^>]*>`).FindString(groupForm(get(h, "/prefs", dadFormCookie).Body.String(), gid))
		if strings.Contains(well, "data-active") {
			t.Errorf("颜色在色板上了，取色器还挂着环：%s", well)
		}
	}
}

// 色板那八颗的名字必须**跟着界面语言走**（C1）。
//
// 触发的实测：英文站上把 /prefs 打开，aria-label 那八个位置印的是「蓝 青 绿 黄
// 橙 红 紫 粉」——名字当时住在 model 的一张 Go 表里，绕过了"文案只有一份，
// 在 messages/{zh,en}.json"那条纪律。英文界面里一个英语读者看见中文单字，
// 比看见英文更没用（他念不出来，而那本来就是给念不出颜色的人准备的）。
func TestPrefsGroup_色名跟着界面语言走(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.SetLanguage(i18n.En); err != nil {
		t.Fatal(err)
	}
	page := get(h, "/prefs", dadFormCookie).Body.String()
	form := groupForm(page, "nas")
	if form == "" {
		t.Fatal("英文界面下没有分组那一行的 form")
	}
	labels := regexp.MustCompile(`class="prefs-swatch"[^>]*aria-label="([^"]+)"`).FindAllStringSubmatch(form, -1)
	if len(labels) == 0 {
		// 属性顺序由模板排版决定，反过来写也要认得到（否则这条闸会在排一次属性后静默空转）。
		labels = regexp.MustCompile(`aria-label="([^"]+)"[^>]*class="prefs-swatch"`).FindAllStringSubmatch(form, -1)
	}
	if len(labels) < len(model.GroupAccentPalette) {
		t.Fatalf("只数到 %d 颗色块的可及名，色板有 %d 颗", len(labels), len(model.GroupAccentPalette))
	}
	for _, l := range labels {
		name := l[1]
		if cjk(name) {
			t.Errorf("英文界面上这颗色块的可及名是中文：%q——名字不许住在 Go 表里", name)
			break
		}
	}
	if !strings.Contains(form, "Blue") || !strings.Contains(form, "Pink") {
		t.Errorf("英文界面上没有 Blue/Pink 这两个色名：%s", firstBytes(form))
	}
	if err := s.SetLanguage(i18n.Zh); err != nil {
		t.Fatal(err)
	}
	zhPage := groupForm(get(h, "/prefs", dadFormCookie).Body.String(), "nas")
	if !strings.Contains(zhPage, "蓝") {
		t.Errorf("切回中文之后那颗键的可及名里没有「蓝」：%s", firstBytes(zhPage))
	}
}

// cjk 报这段文字里有没有汉字——语言闸的"英文态不许残留中文"用的是同一把尺。
func cjk(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
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
	// 落回 `/prefs`（改颜色要连着改好几组，回首页就断了），**并带着 `?tab=`**
	// （2026-10-15）：这一族的每一次按色块都是一次提交，而分组那一节有二十行时
	// "改完跳回第一类"等于那一节没法用。请求没带 tab 时落回第一颗。
	if loc := rec.Header().Get("Location"); loc != "/prefs?tab=look" {
		t.Errorf("Location = %q，要 /prefs?tab=look（回面板、且停在原来那一节）", loc)
	}
	// **排法要挑一个真的不等于当前值的**（2026-10-15）：夹具里 nas 本来就是 list，
	// 而服务端现在会把"值没变"的 op 压掉（applyBatch 只看 opList 非空，不压的话
	// 按一下已经在用的那颗色块也会存一版快照并 +1 rev）。这条断言要守的是
	// "真改动会推进 rev"，用一个没改的排法去测它，测到的其实是压掉那件事。
	postForm(h, "/prefs/group", "group=nas&layout="+model.LayoutFeature, dadFormCookie)
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
	if nas.Layout != model.LayoutFeature {
		t.Errorf("盘上 layout = %q，要 %s", nas.Layout, model.LayoutFeature)
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

// 逐颗键复放**浏览器真的会交出去的那份 body**。
//
// 这条测试是本格从实测里抱回来的：独立页 /prefs 一个 <script> 都不引（那条通路
// 的成立前提就是裸 HTML），而同一张 form 里除被按下的那颗键之外还站着命名过的
// 取色器——它每一次提交都跟着走。于是"按色块 = 只交 accent"、"按磁贴 = 只交
// layout"这两句话从前是注释里的约定而不是结构：屏上按色块拿到的是整屏裸 JSON
// （两个 accent 被判形状错），按磁贴则顺手把颜色改掉（一次按下写两条 op）。
//
// 判据只有一句：**一次按下改得出它说的那一件事，改不出别的**。所以每一段都断
// 两头——要变的那一格变了，不该动的那一格一个字节没动，而 rev 只推进一格
// （推进两格就说明这一次按下写了两条 op）。
func TestPrefsGroup_逐颗键复放浏览器的载荷(t *testing.T) {
	s, h := setupBatch(t)

	// 起点：nas 有颜色 #dd6aa6，排法 tiles。
	setGroupAccent(t, h, "dad", "nas", "#dd6aa6")
	postForm(h, "/prefs/group", "group=nas&layout=tiles", dadFormCookie)

	type presses struct {
		name, body string
		wantAccent *string
		wantLayout string
	}
	empty := ""
	teal := "#22b8a0"
	custom := "#123456"
	cases := []presses{
		// 按色板「青」：色块交 accent，取色器同时交 custom——后者必须被无视。
		{"色板那一颗", "group=nas&accent=%2322b8a0&custom=%23dd6aa6", &teal, model.LayoutTiles},
		// 按「跟随页面主色」：交的是 accent=（空串合法），取色器那个值同样必须被无视。
		{"回到默认那一颗", "group=nas&accent=&custom=%23dd6aa6", &empty, model.LayoutTiles},
		// 按「单列」：只许改排法。这一条从前会连 accent 一起写（屏上亮两颗 pressed）。
		{"排法那一颗", "group=nas&layout=list&custom=%23dd6aa6", &empty, model.LayoutList},
		// 按「用这个颜色」：这一颗带 name=apply_custom，于是取色器那格才说话。
		{"自定义那颗键", "group=nas&apply_custom=1&custom=%23123456", &custom, model.LayoutList},
	}
	for _, c := range cases {
		before := currentRev(t, s, "dad")
		if rec := postForm(h, "/prefs/group", c.body, dadFormCookie); rec.Code != http.StatusSeeOther {
			t.Fatalf("%s：%q 要 303，得到 %d: %s", c.name, c.body, rec.Code, rec.Body)
		}
		if after := currentRev(t, s, "dad"); after != before+1 {
			t.Errorf("%s：rev 从 %d 变成 %d，要只推进一格——多出来的那一格就是这一次按下顺手改的第二件事", c.name, before, after)
		}
		g := findGroup(t, s, "dad", "nas")
		if g.Accent != *c.wantAccent {
			t.Errorf("%s：盘上 accent = %q，要 %q", c.name, g.Accent, *c.wantAccent)
		}
		if g.Layout != c.wantLayout {
			t.Errorf("%s：盘上 layout = %q，要 %q", c.name, g.Layout, c.wantLayout)
		}
	}

	// 只交得出取色器、没按下那颗键 = 有人在拼 body，或表单被改了形状：仍然要说清缺什么。
	rec := postForm(h, "/prefs/group", "group=nas&custom=%23123456", dadFormCookie)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("只给 custom 要 422，得到 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"field": "accent"`) {
		t.Errorf("那句 422 要点名 accent：%s", rec.Body)
	}
}

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

// ── 删掉一个分组（2026-10-15）────────────────────────────────────────────
// 空组在页面上不画（render 那一侧判据见 exec.go 的 profile），而清掉它是设置页
// 唯一的出口——所以这一族钉的是"这一格真的能删一组，且删得干净"。

// 删组走的是**书签写通路**：rev 推进、审计有一行、快照存下来了。三道闸少任何
// 一道，"删错了能在「回退」里找回来"这句话对这一格就是假的——而它恰恰是这一格
// 唯一的安全网（界面上那颗红键前面那一句"还能用回退找回"说的是这个事实）。
func TestPrefsGroup_删掉一组推进rev并留审计行(t *testing.T) {
	s, h := setupBatch(t)
	seedEmptyGroup(t, s, "dad", "empty", "清空的组")
	before := currentRev(t, s, "dad")
	rec := postForm(h, "/prefs/group", "group=empty&remove=1", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("删组要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	for _, g := range loadBookmarks(t, s, "dad").Groups {
		if g.ID == "empty" {
			t.Fatalf("empty 这一组还在盘上：%+v", g)
		}
	}
	if after := currentRev(t, s, "dad"); after != before+1 {
		t.Errorf("rev 从 %d 变成 %d，要正好推进一格：它走的是书签写通路", before, after)
	}
	if !hasHistoryBy(t, s, "dad", "web") {
		t.Error("审计线里没有 by=web 的行：删一组是网页动作，不记就分不清是谁删的")
	}
	// 快照：删之前那一份必须已经落下来，否则「回退」里根本没有可回来的版本。
	if snaps, err := s.ListSnapshots("dad"); err != nil || len(snaps) == 0 {
		t.Errorf("没有快照留下来，删错了退不回去：%v %v", snaps, err)
	}
	// 别的那一组一个字都不许动。
	if g := findGroup(t, s, "dad", "nas"); g.Title != "NAS" || len(g.Items) != 1 {
		t.Errorf("nas 这一组被顺手改了：%+v", g)
	}
}

// **一次按下只说一件事**：那张表单里名字输入框与取色器是**常驻**的（命名过的
// 输入框每一次提交都跟着走），所以按「删掉」时它们照样跟着提交——而它们一个
// 都不该生效：照单发出去就是"顺手把名字也改了"，而删组那一刻人根本没想过要改
// 名字。若顺序再写反一点（先 remove 再 set title），ops 会报目标不存在而**整批
// 不写**——于是"这一格删不掉"，症状与这一族要防的完全不同。
//
// 唯一的证据在审计行里：一次按下 = 一条 op。所以这里断的是**那一条 op 是什么**，
// 而不是"盘上有没有变"（被删掉的那组本来就没法再看）。
func TestPrefsGroup_按删键时审计线上只有那一条op(t *testing.T) {
	s, h := setupBatch(t)
	seedEmptyGroup(t, s, "dad", "empty", "清空的组")
	// 载荷照**浏览器真的会交出来的那份**写：名字框里留着半句没提交的内容，
	// 取色器带着它的当前值，而 accent/layout 一个都没有（那两颗键不是 submitter）。
	rec := postForm(h, "/prefs/group",
		"group=empty&remove=1&rename="+url.QueryEscape("半句没提交的名字")+"&custom=%23dd6aa6",
		dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("删组要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	hist, err := s.ReadHistory("dad", 1)
	if err != nil || len(hist) == 0 {
		t.Fatalf("审计线读不出来：%v", err)
	}
	ops := hist[0].Ops
	if len(ops) != 1 {
		t.Fatalf("这一次按下写了 %d 条 op %+v，要恰好 1 条：一次按下只改一件事", len(ops), ops)
	}
	if ops[0].Op != "remove" || ops[0].Path != "groups/empty" {
		t.Errorf("那一条 op = %+v，要 remove groups/empty", ops[0])
	}
}

// 已经没有的组：把面板重画回来并说清为什么，而不是静默成功（与 accent 那条同一
// 处置）。这一格尤其要这样：人是从面板上按下去的那一颗，而面板刚画出来。
func TestPrefsGroup_删一个已经没了的组要把面板重画回来(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/prefs/group", "group=ghost&remove=1", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("组没了要把面板重画回来（200），得到 %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `class="notice"`) {
		t.Errorf("页面上没有那句为什么：\n%.600s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `class="prefs"`) {
		t.Error("没有把面板本身重画出来：人到了一个只有一句话的页面上")
	}
}

// 分组 id 那一格在删之前先判：ops 的路径文法只收 ASCII slug，而它报出来的是
// 一句"路径解析失败"——对表单作者说的是路径，而这一格收的其实是分组 id。
func TestPrefsGroup_删组也要收group那一格(t *testing.T) {
	s, h := setupBatch(t)
	before := currentRev(t, s, "dad")
	rec := postForm(h, "/prefs/group", "remove=1", dadFormCookie)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("没带 group 要 422，得到 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"field": "group"`) {
		t.Errorf("那句 422 要点名 group：%s", rec.Body.String())
	}
	if after := currentRev(t, s, "dad"); after != before {
		t.Errorf("被拒的那次不许写盘，rev 从 %d 变成了 %d", before, after)
	}
}

// 色板那一行的**容器**必须在。
//
// 这一条是被一次真事故逼出来的（2026-10-15）：把「删掉这一组」从色板行移上来时，
// 一段以 `{{/*` 起头而**忘了收 `*/}}`** 的注释留在了原地，于是它一路吞到下一处
// `*/}}` 为止——把 `<div class="prefs-keys prefs-group-colors">` 那个开标签
// 一起吞成了注释文字。渲染不报错、十颗色块与取色器照旧在（所以上面所有形状
// 断言都绿），可它们从那以后是 `.prefs-group` 的**直接** flex 子项，于是与
// 名字、排法挤到同一行，色板在 512px 那行折成两段。
//
// 症状是"看着眼熟但哪儿不对"，而**功能测试一条都不会红**——所以逐字数容器。
func TestPrefsGroup_色板那一行的容器没被吞掉(t *testing.T) {
	_, h := setupBatch(t)
	html := get(h, "/prefs", dadFormCookie).Body.String()
	forms := strings.Count(html, `name="group" value=`)
	if rows := strings.Count(html, `class="prefs-keys prefs-group-colors"`); rows != forms {
		t.Errorf("面板上 %d 张分组表单却有 %d 个色板行容器：容器被吞掉时色块会变成 form 的直接子项，与名字和排法挤到同一行", forms, rows)
	}
	// 同一族的另一半：**开标签必须在第一颗色块之前**。闭标签数与开标签数相等是废话
	// 判据（它只会在两处同时丢时红），所以断的是"从排法那一行到第一颗色块之间，
	// 色板那个容器确实在那儿"——吞掉的正是这个开标签。
	from := strings.Index(html, `prefs-group-shape">`)
	to := strings.Index(html, `name="accent"`)
	if from < 0 || to < 0 {
		t.Fatalf("面板里找不到排法那一行或第一颗色块：shape=%d accent=%d", from, to)
	}
	if !strings.Contains(html[from:to], `class="prefs-keys prefs-group-colors"`) {
		t.Errorf("排法那一行与第一颗色块之间没有色板行容器：\n%.400s", html[from:to])
	}
}

// ── 建一个新分组（2026-10-15）────────────────────────────────────────────
// 建组此前只有两个出口：顶栏「＋ 添加」浮层里那个「＋ 新分组…」下拉项，
// 与 `POST /api/v1/batch` 的 `add path=groups`。于是**这一屏改不了组表本身**。

// 建组走的是书签写通路：快照、审计、rev 一道闸不少，所以建错了能退回来。
// 顺带钉住**id 是服务端派生的**：面板那一格只问名字，而中文名派生不出 ASCII
// （§11.2 写死在明处的代价），所以盘上那个 id 多半是 group / group-2。
func TestPrefsGroup_建组推进rev并派生id(t *testing.T) {
	s, h := setupBatch(t)
	before := currentRev(t, s, "dad")
	rec := postForm(h, "/prefs/group", "add="+url.QueryEscape("待整理"), dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("建组要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	var got *model.Group
	for i, g := range loadBookmarks(t, s, "dad").Groups {
		if g.Title == "待整理" {
			got = &loadBookmarks(t, s, "dad").Groups[i]
		}
	}
	if got == nil {
		t.Fatalf("盘上没有「待整理」那一组：%v", loadBookmarks(t, s, "dad").Groups)
	}
	if !model.ValidSlug(got.ID) {
		t.Errorf("派生出的 id %q 不是合法 slug：它要进 bookmarks.json 当寻址键", got.ID)
	}
	if got.Layout != model.LayoutTiles {
		t.Errorf("没带 layout 时该用默认 tiles，得到 %q", got.Layout)
	}
	if after := currentRev(t, s, "dad"); after != before+1 {
		t.Errorf("rev 从 %d 变成 %d，要正好推进一格", before, after)
	}
	if !hasHistoryBy(t, s, "dad", "web") {
		t.Error("审计线里没有 by=web 的行：建组也是网页动作")
	}
	// 别的那一组一个字都不许动。
	if g := findGroup(t, s, "dad", "nas"); g.Title != "NAS" || len(g.Items) != 1 {
		t.Errorf("nas 这一组被顺手改了：%+v", g)
	}
}

// 按下排法那颗键要**建出那一档**——那张表单里名字是常驻的，所以按磁贴交出的是
// `add=待整理&layout=tiles`，而"只建组"与"建一个磁贴的组"是同一个意图。
func TestPrefsGroup_建组那一档排法按了哪颗就是哪档(t *testing.T) {
	s, h := setupBatch(t)
	rec := postForm(h, "/prefs/group", "add="+url.QueryEscape("常看的")+"&layout="+model.LayoutFeature, dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("建组要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	var got *model.Group
	b := loadBookmarks(t, s, "dad")
	for i := range b.Groups {
		if b.Groups[i].Title == "常看的" {
			got = &b.Groups[i]
		}
	}
	if got == nil {
		t.Fatalf("盘上没有「常看的」那一组")
	}
	if got.Layout != model.LayoutFeature {
		t.Errorf("建出来是 %q，要 feature（按了哪颗就是哪档）", got.Layout)
	}
}

// 形状错要 422 且**点名 add**，并且不许写盘。
//
// `group` 那一格在这一支**不参与**：新组还没有 id，id 是这里派生的——所以
// "必带 group"是对改已有那一组说的，把这条判据挪到建组前面会让建组永远 422。
func TestPrefsGroup_建组的形状错点名add且不写盘(t *testing.T) {
	s, h := setupBatch(t)
	before := currentRev(t, s, "dad")
	for _, c := range []struct{ body, field string }{
		{"add=", "add"},
		{"add=%20%20", "add"}, // 只敲了空格
		{"add=" + url.QueryEscape(strings.Repeat("字", 61)), "add"},     // 超长（上限 60）
		{"add=" + url.QueryEscape("影音") + "&layout=columns", "layout"}, // 排法不在三个里
		{"add=%E5%BD%B1%E8%A7%86&add=%E5%A5%BD%E5%8F%8B", "add"},       // 同名多值
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

// 建出来的组**当场能改也能删**：这一族一次按下改一件事，而"建完发现改不了"
// 的症状与"没有建这个出口"一模一样。
func TestPrefsGroup_建出来的组立刻能改名与删除(t *testing.T) {
	s, h := setupBatch(t)
	postForm(h, "/prefs/group", "add=work", dadFormCookie)
	findGroup(t, s, "dad", "work") // 建出来了（下面两格都以它为目标；找不到会自己 t.Fatal）
	if rec := postForm(h, "/prefs/group", "group=work&rename="+url.QueryEscape("工作"), dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("给刚建的组改名要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	if got := findGroup(t, s, "dad", "work").Title; got != "工作" {
		t.Errorf("改名后盘上是 %q，要「工作」", got)
	}
	// **id 不随改名而变**：它是寻址键（§4.4），别人书签里的链接指着它。
	if findGroup(t, s, "dad", "work").ID != "work" {
		t.Error("改名把 id 一起改了：那是寻址键")
	}
	if rec := postForm(h, "/prefs/group", "group=work&remove=1", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("删掉刚建的组要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	for _, gg := range loadBookmarks(t, s, "dad").Groups {
		if gg.ID == "work" {
			t.Errorf("work 那一组还在盘上：%+v", gg)
		}
	}
}

// seedEmptyGroup 往某档案里加一个**一条书签都没有**的分组（页面照画的那种）。
//
// 走 Store 而不是 REST：测试要的是"盘上有这个数据"，而它没有可调用的通路
// ——建组走 batch，可那正是被测的另一族（与 seedItem 同一条理由）。
func seedEmptyGroup(t *testing.T, s *store.Store, profile, id, title string) {
	t.Helper()
	snap, err := s.Current(profile)
	if err != nil {
		t.Fatal(err)
	}
	snap.B.Groups = append(snap.B.Groups, model.Group{
		ID: id, Title: title, Layout: model.LayoutList, Items: []model.Item{},
	})
	if err := s.SaveBookmarks(profile, snap.B); err != nil {
		t.Fatal(err)
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

// findGroup 取一个分组的**整份**盘上形状，好让"这一次按下只改了它说的那件事"
// 能两头都断（颜色与排法各查一次，别只查变了的那一头）。
func findGroup(t *testing.T, s *store.Store, profile, id string) *model.Group {
	t.Helper()
	b := loadBookmarks(t, s, profile)
	for i := range b.Groups {
		if b.Groups[i].ID == id {
			return &b.Groups[i]
		}
	}
	t.Fatalf("%s 里没有分组 %q", profile, id)
	return nil
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
var groupFormRE = regexp.MustCompile(`(?s)<form method="post" action="/prefs/group\?tab=[a-z]+".*?</form>`)

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
