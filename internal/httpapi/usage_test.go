package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

// spec §13：这三条要**故意做一次刷点击再断言 rev 与文件数没变**——
// 只看代码"没调 store 的书签写通路"不算验证，最容易出的错恰恰是
// 顺手复用了写书签的那条通路。
func TestUsageDoesNotTouchRevSnapshotsOrHistory(t *testing.T) {
	s, h := setupBatch(t)
	before, err := s.Current("dad")
	if err != nil {
		t.Fatal(err)
	}
	snapN := s.SnapshotCount("dad")
	histN := len(mustReadHistory(t, s, "dad"))
	dirN := countDirEntries(t, s.SnapshotsDir("dad"))

	for i := 0; i < 3; i++ {
		rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "", dadTok)
		if rec.Code != 200 {
			t.Fatalf("第 %d 次上报失败: %d %s", i+1, rec.Code, rec.Body)
		}
	}
	after, _ := s.Current("dad")
	if after.Rev != before.Rev {
		t.Fatalf("刷点击把 rev 推到了 %d", after.Rev)
	}
	if s.SnapshotCount("dad") != snapN || countDirEntries(t, s.SnapshotsDir("dad")) != dirN {
		t.Fatalf("刷点击做出了快照")
	}
	if len(mustReadHistory(t, s, "dad")) != histN {
		t.Fatalf("刷点击写了历史")
	}
	// 唯一该出现的新文件是 usage.json。
	raw, err := os.ReadFile(s.UsagePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	// §4.3 缩进形状：标量键后带一个空格（同 store 侧 usage_test 的证人）。
	if !strings.Contains(string(raw), `"clicks": 3`) {
		t.Fatalf("三次点击该累计成 3: %s", raw)
	}
}

// 不许凭一个凭证给别人档案刷点击：只写"当前档案"。
func TestUsageCannotFloodAnotherProfile(t *testing.T) {
	s, h := setupBatch(t)
	// dadTok 的作用域是 dad，请求里没有 profile 参数——它只能落在 dad 上。
	if rec := post(h, "/api/v1/usage", `{"itemIds":["cartoon"]}`, "", dadTok); rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(s.UsagePath("kid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("拿 dad 的凭证刷了 kid 的 usage: %v", err)
	}
	// 想指定别人的档案？带 ?profile=kid 来 → 403，不是"写到 kid"。
	if rec := post(h, "/api/v1/usage?profile=kid", `{"itemIds":["cartoon"]}`, "", dadTok); rec.Code != 403 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	// 403 不是"写了才拒"：kid 的 usage.json 仍然不存在。
	if _, err := os.Stat(s.UsagePath("kid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("403 之前已经把 kid 的 usage 写了: %v", err)
	}
}

// 会话通路落在 lb_profile 上；管理员代管时**不**顺手写别人的计数。
func TestUsageSessionWritesActiveOnly(t *testing.T) {
	s, h := setupBatch(t)
	rec := loginAs(t, h, "dad", "1234")
	cookie := sessionCookie(t, rec)
	if got := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, cookie, ""); got.Code != 200 {
		t.Fatalf("got %d %s", got.Code, got.Body)
	}
	if _, err := os.Stat(s.UsagePath("dad")); err != nil {
		t.Fatalf("dad 的 usage 没写: %v", err)
	}
	if _, err := os.Stat(s.UsagePath("kid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("写了别人的")
	}
}

func TestUsageUnknownIdsAreReportedNotRejected(t *testing.T) {
	_, h := setupBatch(t)
	rec := post(h, "/api/v1/usage", `{"itemIds":["nope","dsm"]}`, "", dadTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	// 响应体解出来比值（batch_test.go 的既定口径：wire 上只有
	// warnings:[] 那一条字节级断言的先例，其余不做子串匹配——
	// writeJSON 是 2 空格缩进，`"unknown":["nope"]` 紧凑形状在
	// wire 上根本不存在，简报那条断言永远命不中）。
	var out struct {
		Updated int      `json:"updated"`
		Unknown []string `json:"unknown"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Unknown) != 1 || out.Unknown[0] != "nope" {
		t.Fatalf("未知 id 该报出来: %s", rec.Body)
	}
	// nope 记到了、dsm 也记到了：items 共 2 条。
	if out.Updated != 2 {
		t.Fatalf("updated 该是 2: %s", rec.Body)
	}
	// 同一份清单里重复的未知 id：每一次点击都算数，unknown 只报一条
	// （评审裁定的 Minor——它是"这些 id 我不认识了"的清单，不是计数器）。
	rec = post(h, "/api/v1/usage", `{"itemIds":["nope","nope","dsm"]}`, "", dadTok)
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Unknown) != 1 || out.Unknown[0] != "nope" {
		t.Fatalf("重复的未知 id 该去重: %s", rec.Body)
	}
	if out.Updated != 2 {
		t.Fatalf("去重不许少记条目: %s", rec.Body)
	}
}

// spec §12：`?profile=` 指向不存在的档案 → 404，与"无权限"分开返回
// （与 batch.go 里那句 `404 "档案不存在"` 同口径，read.go/admin.go 各有证人）。修复前这一条是
// 200 + 盘上多出 profiles/ghost/：里面只有 usage.json，ListProfiles 列不出、
// DELETE /profiles 删不掉，下一个用同一 id 建档的人继承死人的点击史
// （评审 Critical）。
func TestUsageGhostProfileIs404AndCreatesNoDirectory(t *testing.T) {
	s, h := setupBatch(t)
	// ghostTok：作用域正好是 ghost、盘上没有 ghost（`*` 过不了 credScoped）。
	rec := post(h, "/api/v1/usage?profile=ghost", `{"itemIds":["dsm"]}`, "", ghostTok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("档案不存在该 404, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != 404 || out.Detail != "档案不存在" {
		t.Fatalf("404 体该与 batch 同形状: %s", rec.Body)
	}
	// 读在锁内、写在读之后，所以 404 这一条连目录都不该存在。
	if _, err := os.Stat(s.ProfileDir("ghost")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("404 却造出了幽灵目录: %v", err)
	}
}

// 幽灵档案不是"管理员手滑"这一个口味（评审 Critical 的可达性）：作用域
// 令牌对应的档案被删掉、浏览器里留着死档案的 lb_profile cookie，都走同一条
// 通路——require 只看令牌作用域/会话形状，不回查磁盘，所以只有 store 的
// ErrNotFound 与端点的 404 分支挡得住。两种都要 404 且不许把目录复活。
func TestUsageWithDeletedProfileCredentialIs404(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.DeleteProfile("dad"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.UsagePath("dad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("夹具不干净: %v", err)
	}
	if rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "", dadTok); rec.Code != http.StatusNotFound {
		t.Fatalf("档案已删的作用域令牌该 404, got %d %s", rec.Code, rec.Body)
	}
	if rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "lb_profile=dad", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("留着死档案 cookie 的会话该 404, got %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(s.ProfileDir("dad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("一次上报把删掉的档案复活成了幽灵目录: %v", err)
	}
}

// handleUsage 自作决定的三条分支此前一条都没测过（评审 Important 3）：
// 管理员带申报放行 / 管理员缺申报 400 / 申报不是 slug 400。形状照
// read_test.go 的判据表。没有这张表，将来一次重构把 slug 闸摘掉，
// `?profile=..` 的数据根目录写入就重新开闸而测试全绿。
func TestUsageAdminClaimBranches(t *testing.T) {
	s, h := setupBatch(t)
	for _, tc := range []struct {
		name   string
		qs     string
		bearer string
		want   int
	}{
		// **管理员令牌不再放行任何内容端点**（2026-10-10 的承重证人）。这一族原本
		// 有五格全用管理员令牌，现在只有三格换成了档案作用域令牌，另两格变成
		// "管理员令牌一律 403"——写别人的点击计数同样不行。
		{"档案作用域令牌带申报放行", "?profile=dad", dadTok, 200},
		{"管理员令牌带申报", "?profile=dad", adminTok, 403},
		{"管理员令牌申报别人", "?profile=kid", adminTok, 403},
		{"档案作用域令牌缺申报（只写自己）", "", dadTok, 200},
		{"申报不是 ASCII slug", "?profile=%2B%2B", dadTok, 400},
		{"申报越界", "?profile=..", dadTok, 400},
		{"作用域令牌越界申报", "?profile=kid", dadTok, 403},
		{"作用域令牌不申报（只写自己）", "", dadTok, 200},
	} {
		rec := post(h, "/api/v1/usage"+tc.qs, `{"itemIds":["dsm"]}`, "", tc.bearer)
		if rec.Code != tc.want {
			t.Fatalf("%s want %d got %d %s", tc.name, tc.want, rec.Code, rec.Body)
		}
	}
	// 被 400/403 拒掉的申报一个字都不许落盘（拒写必须早于写）。
	for _, p := range []string{
		filepath.Join(s.Root(), "usage.json"),
		filepath.Join(s.Root(), "profiles", "usage.json"),
	} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("被拒的请求写出了 %s: %v", p, err)
		}
	}
}

// 坏档案（bookmarks.json 读不了）在 usage 上不该是裸 500：spec §12 要
// "报文件与位置"，AI 拿 BrokenReason 那句才能直接去修文件；batch/revert/data
// 一律 503，这里同口径（评审 Important 5）。
// 注：usage.json **自己**坏了不进 brokenProfiles[]（LoadUsage 不 markBroken），
// 本轮裁定接受，AddUsage 仍是"拒绝覆盖坏文件"。
func TestUsageBrokenProfileIs503WithReason(t *testing.T) {
	s, h := setupBatch(t)
	brokenDoc := "{\n  \"rev\": 1,\n"
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte(brokenDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("坏档案该 503 而不是裸 500, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != 503 || !strings.Contains(out.Detail, "bookmarks.json") {
		t.Fatalf("detail 要点名坏的是哪个文件: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), s.Root()) {
		t.Fatalf("不许回显磁盘绝对路径: %s", rec.Body)
	}
	// 503 之前没有偷偷写计数。
	if _, err := os.Stat(s.UsagePath("dad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("503 却写了 usage.json: %v", err)
	}
}

// 最终整枝评审 #3（其一）：503 分诊一律由**本次请求返回的错误**派生，
// 坏档登记表不能再当"要不要 503"的判据。usage.json 自己坏了从不
// markBroken（LoadUsage 不进登记表），旧实现对任何错误都查登记表——
// 干净夹具里它查不到东西，把真数据问题答成 500；本条钉的正是这条路：
// 必须是 503，且 detail 点名 usage.json。
func TestUsageCorruptUsageFileIs503CitingUsageJSON(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.UsagePath("dad"), []byte("{\n  \"items\": [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("坏 usage.json 是数据文件坏了，该 503 不该 500, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != 503 || !strings.Contains(out.Detail, "usage.json") || strings.Contains(out.Detail, "profile.json") {
		t.Fatalf("detail 必须点名真正坏掉的 usage.json: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), s.Root()) {
		t.Fatalf("不许回显磁盘绝对路径: %s", rec.Body)
	}
}

// 最终整枝评审 #3（其二，指错文件的那副面孔）：登记表里留着 profile.json
// 的陈旧记录时，坏 usage.json 不许被答成"503 citing profile.json"。
// 夹具把"陈旧"造真：先弄坏 profile.json 让 /pin 的 LoadProfile 登记原因，
// 再把 profile.json 原样修回去——登记要等下一次成功 LoadProfile 才清，
// 而 usage 通路根本不读 profile.json，等不到那一刻。旧实现拿登记表的
// 优先项作答，detail 里说的是一个此刻根本没坏的文件。
func TestUsageCorruptUsageWithStaleProfileMarkerCitesUsageJSON(t *testing.T) {
	s, h := setupBatch(t)
	profileRaw, err := os.ReadFile(s.ProfilePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ProfilePath("dad"), []byte("{\n  \"id\": \"dad\",\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := post(h, "/pin", `{"profile":"dad","pin":"1234"}`, "", ""); rec.Code != http.StatusOK {
		t.Fatalf("坏 profile.json 进门该给 200 解释页（登记坏档的同一步）, got %d %s", rec.Code, rec.Body)
	}
	if err := os.WriteFile(s.ProfilePath("dad"), profileRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.UsagePath("dad"), []byte("[oops\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); strings.Contains(body, "profile.json") || !strings.Contains(body, "usage.json") {
		t.Fatalf("本次请求坏的是 usage.json，detail 不许引用 profile.json: %s", body)
	}
}

func TestUsageNeedsCredentialsAndRejectsOversizedList(t *testing.T) {
	_, h := setupBatch(t)
	if rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "", ""); rec.Code != 401 {
		t.Fatalf("got %d", rec.Code)
	}
	// 一次报 5000 个 id：前端节流坏了或有人在刷。拒掉，不写盘。
	ids := make([]string, 5000)
	for i := range ids {
		ids[i] = `"x"`
	}
	body := `{"itemIds":[` + strings.Join(ids, ",") + `]}`
	if rec := post(h, "/api/v1/usage", body, "", dadTok); rec.Code != 422 {
		t.Fatalf("超量上报该 422, got %d", rec.Code)
	}
}

func mustReadHistory(t *testing.T, s *store.Store, id string) []store.HistoryEntry {
	t.Helper()
	h, err := s.ReadHistory(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func countDirEntries(t *testing.T, dir string) int {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		t.Fatal(err)
	}
	return len(ents)
}
