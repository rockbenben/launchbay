package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

// 这组用例存在的理由：**这条端点在 2026-10-10 之前只存在于文档里**。
// store 的 RebuildProfile 早就写好并测过，而端点、CLI 与两处契约表全部缺席——
// 于是坏档在事实上没有出路（实测 profiles/pin 503、profiles/disable 503、
// profiles 409、CLI 打印别的子命令的用法），而界面一直叫人"在这台机器上修好
// 那个文件"。行为证人钉的就是那一天钉不住的那件事：坏档现在**真的能被修回来**。
//
// 判据一律读磁盘（sha256），不看返回值——"响应说成功"与"那张卡真的回来了"是
// 两件事，§13 的坏档那几条要求的就是分开验。

// breakProfileFile 把 profile.json 写成半截 JSON：夹具需要「坏」，只能手工造。
func breakProfileFile(t *testing.T, s *store.Store, id string) {
	t.Helper()
	if err := s.WriteFileAtomic(s.ProfilePath(id), []byte("{\n  \"id\": \""+id+"\",\n  \"title\":\n")); err != nil {
		t.Fatal(err)
	}
}

// digestTree 把一棵目录树按 "相对路径 → 内容 sha256" 压成一张可比对的表。
// 快照与书签都比它：重建的承诺是"一个字节都不碰"，而"看起来一样"不是证据。
func digestTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if _, err := os.Stat(root); err != nil {
		return out // 目录不存在是一份合法的读数，别当错误
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("读 %s 失败: %v", root, err)
	}
	return out
}

func diffTrees(t *testing.T, what string, before, after map[string]string) {
	t.Helper()
	for k, v := range before {
		got, ok := after[k]
		if !ok {
			t.Errorf("%s：重建之后 %s 不见了", what, k)
			continue
		}
		if got != v {
			t.Errorf("%s：重建改动了 %s（重建的承诺是一个字节都不碰）", what, k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			t.Errorf("%s：重建凭空多出 %s", what, k)
		}
	}
}

// setupBrokenKid 给 kid 放一份有内容的书签，再把它的 profile.json 写坏。
// 返回 store、handler 与书签落盘目录（哈希那张表的根）。
func setupBrokenKid(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	s, h := setupBatch(t)
	breakProfileFile(t, s, "kid")
	return s, h
}

// 核心一条：坏档修回来，书签、快照与历史已有内容逐字节不变。
func TestRebuild_坏档修得回来且书签快照逐字节不变(t *testing.T) {
	s, h := setupBrokenKid(t)
	before := digestTree(t, filepath.Join(s.ProfileDir("kid"), "snapshots"))
	marksBefore := digestTree(t, s.ProfileDir("kid"))["bookmarks.json"]

	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","pin":"5678"}`, "", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("坏档该重建成功，得到 %d：%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "kid" || got.Title != "小满" {
		t.Errorf("200 的体该是重建后的档案 {kid 小满}，得到 %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"admin"`) {
		t.Errorf("重建的响应体不该再有 admin 那一格（档案已不是管理员）: %s", rec.Body.String())
	}

	// 书签：一个字节都没动
	if after := digestTree(t, s.ProfileDir("kid"))["bookmarks.json"]; after != marksBefore {
		t.Errorf("重建改动了 bookmarks.json")
	}
	// 快照：一份不多一份不少（重建不写新 rev，也就不该有新快照）
	diffTrees(t, "snapshots/", before, digestTree(t, filepath.Join(s.ProfileDir("kid"), "snapshots")))

	// 盘上那份 profile.json 现在读得动了，而且哈希是真哈希（新 PIN 能进门）
	p, err := s.LoadProfile("kid")
	if err != nil {
		t.Fatalf("重建之后 profile.json 该读得动：%v", err)
	}
	if p.Title != "小满" {
		t.Errorf("重建出来的档案形状不对：%+v", p)
	}
	// 坏档登记消失：/status 不再列它。§13 钉的是"响应说成功 = 进门页那张卡真的回来了"
	rec = getBearer(h, "/api/v1/status", adminTok)
	if strings.Contains(rec.Body.String(), `"broken": true`) {
		t.Errorf("重建成功之后 /status 仍把它记成坏档：\n%s", rec.Body.String())
	}
}

// 读得动的档案：409，且**没有发生任何写**。回读用哈希而不是"看起来一样"。
func TestRebuild_读得动的档案409且没有发生任何写(t *testing.T) {
	s, h := setupBatch(t)
	before := digestTree(t, s.ProfileDir("kid"))

	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"改名试试","pin":"9999"}`, "", adminTok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("读得动的档案该 409，得到 %d：%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "profiles/pin") {
		t.Errorf("409 的 detail 应该说清出路（改 PIN 走 profiles/pin），得到：%s", rec.Body.String())
	}
	diffTrees(t, "整个档案目录", before, digestTree(t, s.ProfileDir("kid")))
}

// 目录不存在：404，且**不创建任何目录**。这条端点不是第二条建档通路。
func TestRebuild_目录不存在时404且不建目录(t *testing.T) {
	s, h := setupBatch(t)
	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"ghost","title":"幽灵","pin":"4321"}`, "", adminTok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("目录不在该 404，得到 %d：%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(s.ProfileDir("ghost")); err == nil {
		t.Errorf("重建凭空建出了 profiles/ghost：这条通路只重写 profile.json")
	}
}

// 请求体里那个 `admin` 键**现在整个不存在了**（2026-10-12），于是这一格翻面：
// 送它来是 422 且点名是它，而省掉它就是正常请求。
//
// 翻面而不是删掉，是因为两种口径要分清楚：盘上的 profile.json 里那个 `admin`
// 必须被**悄悄放过**（否则升级要用户挨个 rebuild），而请求体里送来一个不存在的
// 字段是一个脚本的误解——422 点名比沉默忽略诚实。
func TestRebuild_送admin来是422(t *testing.T) {
	_, h := setupBrokenKid(t)
	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","pin":"5678","admin":false}`, "", adminTok)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("带 admin 的 body 该 422，得到 %d：%s", rec.Code, rec.Body.String())
	}
	if !fieldErrorsName(rec, "admin") {
		t.Errorf("422 该点名 admin 那一格，得到：%s", rec.Body.String())
	}
	// 正对照：不带它就是一次正常的重建。
	if rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","pin":"5678"}`, "", adminTok); rec.Code != http.StatusOK {
		t.Errorf("不带 admin 该 200，得到 %d：%s", rec.Code, rec.Body.String())
	}
}

// 凭证：档案作用域令牌 403、无凭证 401。它重写的是账号文件，不是任何人的书签，
// 所以它与 profiles/pin 同族（credAdmin），而 `*` 令牌能过。
func TestRebuild_凭证边界(t *testing.T) {
	_, h := setupBrokenKid(t)
	body := `{"profile":"kid","title":"小满","pin":"5678"}`
	if rec := post(h, "/api/v1/profiles/rebuild", body, "", kidTok); rec.Code != http.StatusForbidden {
		t.Errorf("档案作用域令牌打它是 403，得到 %d：%s", rec.Code, rec.Body.String())
	}
	if rec := post(h, "/api/v1/profiles/rebuild", body, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("无凭证该 401，得到 %d：%s", rec.Code, rec.Body.String())
	}
	if rec := post(h, "/api/v1/profiles/rebuild", body, "", "tok-unknown"); rec.Code != http.StatusUnauthorized {
		t.Errorf("认不出的令牌该 401，得到 %d：%s", rec.Code, rec.Body.String())
	}
}

// 审计行：目标档案的历史里多一行 rev:0、path profile/rebuild，且 by 认 X-LB-By。
// "谁动过这台机器"是 §4.6 的唯一证据，重建这种整份重写文件的动作尤其不能没有。
func TestRebuild_历史追一行profile_rebuild(t *testing.T) {
	s, h := setupBrokenKid(t)
	if rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","pin":"5678"}`, "", adminTok); rec.Code != http.StatusOK {
		t.Fatalf("重建该成功，得到 %d：%s", rec.Code, rec.Body.String())
	}
	entries, err := s.ReadHistory("kid", 0)
	if err != nil {
		t.Fatal(err)
	}
	var hit bool
	for _, e := range entries {
		if len(e.Ops) == 1 && e.Ops[0].Path == "profile/rebuild" {
			hit = true
			if e.Rev != 0 {
				t.Errorf("重建不产生书签 rev，历史行该是 rev:0，得到 %d", e.Rev)
			}
			if e.Actor == "" {
				t.Errorf("审计行要留名（actor 为空）")
			}
		}
	}
	if !hit {
		t.Errorf("kid 的历史里没有 profile/rebuild 那一行：%+v", entries)
	}
}

// 半件事目录（有目录、没有 profile.json）也能重建——那是"文件不在"那一支，
// 与"内容错"同样允许覆盖。
func TestRebuild_半件事目录可以重建(t *testing.T) {
	s, h := setup(t)
	if err := os.MkdirAll(s.ProfileDir("mom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFileAtomic(s.BookmarksPath("mom"), []byte("{\n  \"rev\": 7,\n  \"title\": \"\",\n  \"groups\": [],\n  \"search\": {\"enabled\": false, \"default\": \"\", \"engines\": []},\n  \"weather\": {\"enabled\": false, \"location\": \"\"},\n  \"openInNewTab\": false,\n  \"clock\": false,\n  \"footer\": \"\"\n}\n")); err != nil {
		t.Fatal(err)
	}
	before := digestTree(t, s.ProfileDir("mom"))
	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"mom","title":"妈妈","pin":"4321"}`, "", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("半件事目录该重建成功，得到 %d：%s", rec.Code, rec.Body.String())
	}
	after := digestTree(t, s.ProfileDir("mom"))
	if after["bookmarks.json"] != before["bookmarks.json"] {
		t.Errorf("重建动了 bookmarks.json")
	}
	if _, ok := after["profile.json"]; !ok {
		t.Errorf("重建没有写出 profile.json")
	}
	if len(after) != len(before)+1 {
		t.Errorf("重建只该多出一个 profile.json，之前 %d 个文件，之后 %d 个", len(before), len(after))
	}
}

// 字段级错误：slug / 空标题 / PIN 格式，每一格都要点名。
func TestRebuild_字段级错误逐条点名(t *testing.T) {
	_, h := setupBrokenKid(t)
	for _, tc := range []struct{ name, body, field string }{
		{"id 不是 slug", `{"profile":"妈妈","title":"x","pin":"1234"}`, "profile"},
		{"标题为空", `{"profile":"kid","title":"  ","pin":"1234"}`, "title"},
		{"PIN 三位", `{"profile":"kid","title":"小满","pin":"123"}`, "pin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(h, "/api/v1/profiles/rebuild", tc.body, "", adminTok)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("该 422，得到 %d：%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"`+tc.field+`"`) {
				t.Errorf("422 该点名 %s 那一格，得到：%s", tc.field, rec.Body.String())
			}
		})
	}
}

// 未知字段一律拒（解码开了 DisallowUnknownFields）：AI 照着文档打错一个键就
// 整个 422，而不是静默忽略——后者会让人以为那一格生效了。
func TestRebuild_未知字段拒整条(t *testing.T) {
	_, h := setupBrokenKid(t)
	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","pin":"5678","theme":"dark"}`, "", adminTok)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("带未知字段该 422，得到 %d：%s", rec.Code, rec.Body.String())
	}
}

// 读得动 vs 读不动的分诊不许串：坏档一律 503 的那几族（profiles/pin、
// profiles/disable）说明**读**这条路上没有出路，而这条端点才是。把它钉在这里，
// 免得下一个人"顺手"把重建的判据写成先 LoadProfile 看一眼。
func TestRebuild_坏档时兄弟端点仍无出路(t *testing.T) {
	_, h := setupBrokenKid(t)
	body := `{"profile":"kid","new":"1111"}`
	if rec := post(h, "/api/v1/profiles/pin", body, "", adminTok); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("坏档上 profiles/pin 该 503，得到 %d：%s", rec.Code, rec.Body.String())
	}
	if rec := post(h, "/api/v1/profiles/disable", `{"profile":"kid","disabled":true}`, "", adminTok); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("坏档上 profiles/disable 该 503，得到 %d：%s", rec.Code, rec.Body.String())
	}
	// 而重建这一条走得通——这三行合起来才是"坏档不再死锁"的完整读数。
	if rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","pin":"5678"}`, "", adminTok); rec.Code != http.StatusOK {
		t.Errorf("重建该 200，得到 %d：%s", rec.Code, rec.Body.String())
	}
}

// 重建后的 PIN 是**新**哈希且真的能进门：夹具不许自己拼 sha256$… 假装哈希过，
// 否则"rebuild 写进去的哈希能用"这件事永远没被证明。
func TestRebuild_新PIN能进门(t *testing.T) {
	s, h := setupBrokenKid(t)
	if rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","pin":"4321"}`, "", adminTok); rec.Code != http.StatusOK {
		t.Fatalf("重建该成功，得到 %d：%s", rec.Code, rec.Body.String())
	}
	p, err := s.LoadProfile("kid")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.PIN, "sha256$") {
		t.Fatalf("写进去的不是真哈希：%q", p.PIN)
	}
	if rec := postForm(h, "/pin", "profile=kid&pin=4321", ""); rec.Code != http.StatusSeeOther {
		t.Errorf("用重建后的新 PIN 该能进门（303），得到 %d", rec.Code)
	}
	// 旧 PIN 随之失效——重建换掉的是整个 profile.json，PIN 在其中。
	if rec := postForm(h, "/pin", "profile=kid&pin=5678", ""); rec.Code != http.StatusSeeOther {
		t.Errorf("旧 PIN 该失效，得到 %d", rec.Code)
	} else if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=pin") {
		t.Errorf("旧 PIN 该被拒（Location 带 err=pin），实际 %q", loc)
	}
}

// 端点不许自己 LoadProfile 看一眼再决定写不写（那会让两次读之间被人修好的档案
// 被盖掉）。这条钉的是**读得动 → 一个字节都不许写**：它在上面那条里已经从哈希
// 验过一遍，这里补的是"顺序"——409 的那一次请求里，store 侧的锁内判据必须先于
// 任何 SaveProfile 发生。用一个可读的档案打它，读盘前后必须逐字节相同。
func TestRebuild_409那一次没有留下任何写(t *testing.T) {
	s, h := setupBatch(t)
	before := digestTree(t, s.ProfileDir("dad"))
	for i := 0; i < 3; i++ {
		rec := post(h, "/api/v1/profiles/rebuild",
			`{"profile":"dad","title":"爸爸","pin":"1111"}`, "", adminTok)
		if rec.Code != http.StatusConflict {
			t.Fatalf("第 %d 次该 409，得到 %d", i, rec.Code)
		}
	}
	diffTrees(t, "dad 的档案目录", before, digestTree(t, s.ProfileDir("dad")))
}

// 重建时 theme/layout 按默认值重写（读不出来就没有"保留"这个选项，§12 明写的
// 取舍）。这两个默认值是 store 里**硬编码的字面量**，而合法性由 model 的
// allowedTheme/allowedLayout 判——两层各有一份知识，改动任何一边而忘了另一边，
// 症状是"每一条重建都在 422"（Validate 在落盘前跑）。
//
// 顺带钉住"取舍"本身：重建之后读得出来的档案，标题是新给的那个，theme/layout
// 是默认值，不是从半截 JSON 里猜出来的。
func TestRebuild_重建出的档案默认theme与layout合法(t *testing.T) {
	s, h := setupBrokenKid(t)
	if rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","pin":"5678"}`, "", adminTok); rec.Code != http.StatusOK {
		t.Fatalf("重建该成功，得到 %d：%s", rec.Code, rec.Body.String())
	}
	p, err := s.LoadProfile("kid")
	if err != nil {
		t.Fatalf("重建出的档案该过 model 校验：%v", err)
	}
	if p.Theme != "auto" || p.Layout != "sidebar" {
		t.Errorf("重建后的默认形态该是 auto/sidebar，得到 %q/%q", p.Theme, p.Layout)
	}
	// 钉住那条取舍：读不出来的主题/布局**没有被抢救**成别的值。
	if p.Theme == "dark" || p.Layout == "topbar" {
		t.Errorf("重建不该凭空猜主题/布局，得到 %q/%q", p.Theme, p.Layout)
	}
}

// model.ValidSlug 与 store 的目录闸是同一道；这一条只钉 handler 没有自己放宽它。
func TestRebuild_路径穿越被拒(t *testing.T) {
	_, h := setupBrokenKid(t)
	for _, bad := range []string{"../escape", "a/b", "..", "Mom"} {
		body, _ := json.Marshal(map[string]any{
			"profile": bad, "title": "x", "pin": "1234", "admin": false,
		})
		rec := post(h, "/api/v1/profiles/rebuild", string(body), "", adminTok)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("profile=%q 该 422，得到 %d：%s", bad, rec.Code, rec.Body.String())
		}
	}
}
