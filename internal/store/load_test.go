package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

const dadBookmarks = `{"rev":3,"title":"爸爸","groups":[{"id":"nas","title":"NAS 应用","layout":"list","items":[{"id":"dsm","name":"群晖 DSM","url":"http://10.0.0.3:5000","alias":[],"tags":[]}]}],"search":{"enabled":false,"default":"","engines":[]},"weather":{"enabled":false,"location":"上海市"},"openInNewTab":false,"clock":false,"footer":""}`

const dadProfile = `{"id":"dad","title":"爸爸","admin":true,"pin":"sha256$a$b","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar"}`

const kidProfile = `{"id":"kid","title":"小满","admin":false,"pin":"sha256$c$d","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar"}`

func put(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, "profiles/dad/profile.json", dadProfile)
	put(t, root, "profiles/dad/bookmarks.json", dadBookmarks)
	put(t, root, "profiles/kid/profile.json", kidProfile) // 故意没有 bookmarks.json
	return root
}

func TestListAndLoad(t *testing.T) {
	s := New(fixtureRoot(t))
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("want 2, got %d", len(profiles))
	}
	if profiles[0].ID != "dad" || profiles[1].ID != "kid" {
		t.Fatalf("ListProfiles 要按 id 排序: %+v", profiles)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("LoadBookmarks: %v", err)
	}
	if b.Rev != 3 || b.Groups[0].Items[0].ID != "dsm" {
		t.Fatalf("got %+v", b)
	}
	if len(s.Broken()) != 0 {
		t.Fatalf("干净夹具不该有坏档: %v", s.Broken())
	}
}

func TestMissingProfileIsErrNotFound(t *testing.T) {
	_, err := New(fixtureRoot(t)).LoadBookmarks("ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// 直接往磁盘写一份带陌生图标名的 bookmarks.json，读回来必须成功、
// 且名字一字不改地留着（渲染层会给它首字母，Task 9）。
// 这是"绕过写入通路种数据"的合法用法：测的是读路径的宽容，不是第二条写通路。
func TestLoadBookmarks_陌生图标名不进坏档(t *testing.T) {
	root := fixtureRoot(t) // 既有 helper：建 dad（含 bookmarks.json）与 kid
	// 用 put() 覆写而不是 SaveBookmarks：后者会跑校验与归一化，
	// 用写通路种数据会让这条测试变成自证循环。
	// 文件写成形似磁盘的形状（2 空格缩进、一个尾换行、alias/tags 是 []）：
	// 它验的正是那个解码器，不是随便一段能解析的 JSON。图标名取形状合法
	// （ASCII slug）、只是本仓库索引里没有的名字——否则会被 checkIcon 以"形状
	// 非法"拒掉、把这条证人读成坏档（红错原因），证不到"陌生名字不该进坏档"。
	put(t, root, "profiles/dad/bookmarks.json", `{
  "rev": 3,
  "title": "t",
  "groups": [{"id": "g", "title": "G", "layout": "list", "items": [
    {"id": "a", "name": "A", "url": "http://10.0.0.1/", "icon": "simple-icons:not-a-brand-yet", "alias": [], "tags": []}
  ]}],
  "search": {"enabled": false, "default": "", "engines": []},
  "weather": {"enabled": false, "location": ""},
  "openInNewTab": false,
  "clock": false,
  "footer": ""
}
`)

	b, err := New(root).LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("陌生图标名不该让档案读不了: %v", err)
	}
	if got := b.Groups[0].Items[0].Icon; got != "simple-icons:not-a-brand-yet" {
		t.Errorf("名字被改了: %q", got)
	}
}

// 刚建好、还没写过书签 = 空档案，不是坏档案。
func TestProfileWithoutBookmarksIsEmptyNotBroken(t *testing.T) {
	s := New(fixtureRoot(t))
	b, err := s.LoadBookmarks("kid")
	if err != nil {
		t.Fatalf("want empty, got %v", err)
	}
	if b.Rev != 0 || len(b.Groups) != 0 {
		t.Fatalf("want zero, got %+v", b)
	}
	if b.Groups == nil {
		t.Fatal("空档案的 groups 要能安全 range 且后续写出为 []")
	}
	if len(s.Broken()) != 0 {
		t.Fatalf("want no broken, got %v", s.Broken())
	}
}

// 一个人的手误不该让全家开不了导航页。
func TestCorruptBookmarksBreakOnlyThatProfile(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json", `{"rev":3,"groups":`)
	s := New(root)

	if _, err := s.LoadBookmarks("dad"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("坏档案不该带走别人: %d", len(profiles))
	}
	if got := s.Broken(); len(got) != 1 || got[0] != "dad" {
		t.Fatalf("Broken() = %v want [dad]", got)
	}
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatalf("kid 应照常可用: %v", err)
	}
}

// 报错要给行号：改文件的人是打开文件改的，不是打开字节偏移量改的。
func TestCorruptErrorNamesFileAndLine(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json", "{\n  \"rev\": 3,\n  \"groups\": [\n    { \"id\": \"nas\" }\n  ],\n")
	_, err := New(root).LoadBookmarks("dad")
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bookmarks.json") {
		t.Fatalf("要点出文件名: %s", msg)
	}
	if !strings.Contains(msg, "第 6 行") {
		t.Fatalf("要点出正确行号（截断发生在第 6 行后）: %s", msg)
	}
}

// 拼错的键必须报错：静默忽略会让人以为改动生效了。
func TestUnknownKeyIsRejected(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json", strings.Replace(dadBookmarks, `"groups"`, `"groupz"`, 1))
	_, err := New(root).LoadBookmarks("dad")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	if !strings.Contains(err.Error(), "groupz") {
		t.Fatalf("要点出未知键名: %v", err)
	}
}

// profile.json 里的 id 与目录名不一致是真实事故（手复制目录留下的）。
func TestProfileIDMustMatchDir(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/kid/profile.json", kidProfile) // id=kid 一致，先确认正常
	put(t, root, "profiles/mom/profile.json", kidProfile) // 内容还是 id=kid
	_, err := New(root).LoadProfile("mom")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt for id mismatch, got %v", err)
	}
	if !strings.Contains(err.Error(), "目录名") {
		t.Fatalf("报错要说明是目录名不一致: %v", err)
	}
}

func TestValidateRunsOnLoad(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json",
		strings.Replace(dadBookmarks, `"layout":"list"`, `"layout":"grid"`, 1))
	_, err := New(root).LoadBookmarks("dad")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt wrapping validation, got %v", err)
	}
	if !errors.Is(err, model.ErrValidation) {
		t.Fatalf("要同时保住 model.ErrValidation 供上层分流, got %v", err)
	}
}

// 评审 Minor（错误文本口径）：会原样上家属解释页的原因——校验错、
// rev 负数、id 与目录名不一致——一律 filepath.Base，与语法错误分支
// 同一口径。全路径只配进 stderr/运维日志，不该出现在 HTTP 响应里。
func TestFamilyFacingCorruptReasonsUseBaseName(t *testing.T) {
	root := fixtureRoot(t)
	s := New(root)

	put(t, root, "profiles/dad/bookmarks.json",
		strings.Replace(dadBookmarks, `"layout":"list"`, `"layout":"grid"`, 1))
	_, err := s.LoadBookmarks("dad")
	if err == nil {
		t.Fatal("want validation error")
	}
	reason, ok := s.BrokenReason("dad")
	if !ok || !strings.Contains(reason, "bookmarks.json") {
		t.Fatalf("原因要点名文件: %q", reason)
	}
	if strings.Contains(reason, root) || strings.Contains(reason, filepath.Join("profiles", "dad")) {
		t.Fatalf("校验错的解释页文案不该带磁盘路径: %q", reason)
	}

	put(t, root, "profiles/kid/profile.json",
		strings.Replace(kidProfile, `"theme":"auto"`, `"theme":"neon"`, 1))
	_, err = s.LoadProfile("kid")
	if err == nil {
		t.Fatal("want profile validation error")
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("decodeProfile 的报错不该带磁盘路径: %v", err)
	}
	if !strings.Contains(err.Error(), "profile.json") {
		t.Fatalf("decodeProfile 仍要点名文件: %v", err)
	}

	put(t, root, "profiles/dad/bookmarks.json",
		strings.Replace(dadBookmarks, `"rev":3`, `"rev":-1`, 1))
	_, err = s.LoadBookmarks("dad")
	if err == nil {
		t.Fatal("want negative rev error")
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("rev 报错不该带磁盘路径: %v", err)
	}
}

// Task 3 review：error message 要点出位置——name 缺失与 icon 形状非法
// 都要同时带 group id 与 item id，不能只有 url 分支被钉住。
func TestValidationErrorsNameLocation(t *testing.T) {
	// (a) name 为空：报错要同时含 group id(nas) 与 item id(dsm)
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json",
		strings.Replace(dadBookmarks, `"name":"群晖 DSM"`, `"name":""`, 1))
	_, err := New(root).LoadBookmarks("dad")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("name 缺失要归到 ErrCorrupt, got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "nas") || !strings.Contains(msg, "dsm") {
		t.Fatalf("name 缺失的报错要同时点出 group id 与 item id: %s", msg)
	}

	// (b) icon 形状非法：同样要含 group id 与 item id
	root2 := fixtureRoot(t)
	put(t, root2, "profiles/dad/bookmarks.json", strings.Replace(dadBookmarks,
		`"url":"http://10.0.0.3:5000"`,
		`"icon":"tabler:Bad Name","url":"http://10.0.0.3:5000"`, 1))
	_, err = New(root2).LoadBookmarks("dad")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("icon 非法要归到 ErrCorrupt, got %v", err)
	}
	msg = err.Error()
	if !strings.Contains(msg, "nas") || !strings.Contains(msg, "dsm") {
		t.Fatalf("icon 非法的报错要同时点出 group id 与 item id: %s", msg)
	}
}

// model.Validate 只在 search.Enabled 为真时检查 default。Enabled:false 配一个
// 悬空 default 是合法数据（关掉搜索后 default 无人消费），不能当坏档处理。
// 加这条钉住形状，避免以后有人把它"修"成坏档案。
func TestDisabledSearchWithDanglingDefaultIsValid(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json",
		strings.Replace(dadBookmarks, `"enabled":false,"default":""`,
			`"enabled":false,"default":"google"`, 1))
	s := New(root)
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("关闭搜索时的悬空 default 要当合法数据: %v", err)
	}
	if b.Search.Enabled || b.Search.Default != "google" {
		t.Fatalf("没读到期望的 search 形状: %+v", b.Search)
	}
	// 同一个 Store 实例上查：坏档标记是在这次 Load 过程中登记的。
	if got := s.Broken(); len(got) != 0 {
		t.Fatalf("不该进 Broken(): %v", got)
	}
}

func TestCurrentCarriesMtimeForCacheKey(t *testing.T) {
	s := New(fixtureRoot(t))
	snap, err := s.Current("dad")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Rev != 3 || snap.Modified.IsZero() {
		t.Fatalf("Snapshot 要带 rev 与 mtime: %+v", snap)
	}
}

// 手工 mkdir 出来的空目录不该被报成"某某档案读不了"。
func TestStrayEmptyDirIsSkippedNotBroken(t *testing.T) {
	root := fixtureRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "profiles", "stray"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New(root)
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("want 2, got %d", len(profiles))
	}
	if got := s.Broken(); len(got) != 0 {
		t.Fatalf("空目录不该进 Broken(): %v", got)
	}
}

func TestEmptyDataDirListsNothing(t *testing.T) {
	s := New(t.TempDir())
	p, err := s.ListProfiles()
	if err != nil || len(p) != 0 {
		t.Fatalf("want empty ok, got %v %v", p, err)
	}
}

// Task 4 review 补 1：坏档案必须能 heal。常驻 Store（main.go 只建一个）
// 里 markBroken 只增不减的话，用户修好 bookmarks.json 之后该档案会
// 永远报坏，污染坏档案页与 /api/v1/status。
func TestBrokenClearsAfterBookmarksFixed(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json", `{"rev":3,"groups":`)
	s := New(root)
	if _, err := s.LoadBookmarks("dad"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	if got := s.Broken(); len(got) != 1 || got[0] != "dad" {
		t.Fatalf("Broken() = %v want [dad]", got)
	}
	// 用户修好了文件：写回合法内容，重新读成功。
	put(t, root, "profiles/dad/bookmarks.json", dadBookmarks)
	if _, err := s.LoadBookmarks("dad"); err != nil {
		t.Fatalf("修好后重读应成功: %v", err)
	}
	if got := s.Broken(); len(got) != 0 {
		t.Fatalf("修好后坏档标记必须清掉: %v", got)
	}
}

// profile.json 坏了再修好同理。但清理必须按来源：
// LoadProfile 的成功不许顺手抹掉 bookmarks.json 的坏记录
// （TestCorruptBookmarksBreakOnlyThatProfile 钉住了坏书签要挺过档案列表）。
func TestBrokenClearsAfterProfileFixed(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/profile.json", `{"id":"dad","title":`)
	put(t, root, "profiles/dad/bookmarks.json", `{"rev":3,"groups":`)
	s := New(root)
	// 两个来源都要先被读出来登记，否则"还剩哪个坏"无从谈起。
	if _, err := s.LoadBookmarks("dad"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	if _, err := s.LoadProfile("dad"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	if got := s.Broken(); len(got) != 1 || got[0] != "dad" {
		t.Fatalf("Broken() = %v want [dad]", got)
	}
	put(t, root, "profiles/dad/profile.json", dadProfile)
	if _, err := s.LoadProfile("dad"); err != nil {
		t.Fatalf("修好后重读应成功: %v", err)
	}
	// profile 来源清了，但 bookmarks 来源还坏着——整体仍应报坏。
	if got := s.Broken(); len(got) != 1 || got[0] != "dad" {
		t.Fatalf("bookmarks 仍坏着，不该整体清空: %v", got)
	}
	put(t, root, "profiles/dad/bookmarks.json", dadBookmarks)
	if _, err := s.LoadBookmarks("dad"); err != nil {
		t.Fatalf("修好后重读应成功: %v", err)
	}
	if got := s.Broken(); len(got) != 0 {
		t.Fatalf("两个来源都修好后必须清空: %v", got)
	}
}

// Task 4 review 补 2：profile 的校验错误必须和 bookmarks 路径对称地
// 保住 model.ErrValidation——上层要能 errors.Is 分流"内容不合法"与
// "文件读不了"，两条路径只能二选一不行。
func TestProfileValidationKeepsErrValidation(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/kid/profile.json",
		strings.Replace(kidProfile, `"theme":"auto"`, `"theme":"neon"`, 1))
	_, err := New(root).LoadProfile("kid")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	if !errors.Is(err, model.ErrValidation) {
		t.Fatalf("profile 路径要对称暴露 model.ErrValidation, got %v", err)
	}
}
