package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// getWithMethod 是任意方法 + 一条凭证的通路。第四参数按前缀分流：
// "Bearer ..." 进 Authorization，其余进 Cookie。Task 7/8 共用这一份实现。
func getWithMethod(h http.Handler, method, path, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	switch {
	case strings.HasPrefix(header, "Bearer "):
		req.Header.Set("Authorization", header)
	case header != "":
		req.Header.Set("Cookie", header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// dadTokHeader 让坏档那条用例读起来像人话：那条走的是令牌通路。
//
// **它不是管理员令牌**（名字里没有 admin 是有意的）：2026-10-10 之后 `*` 只管
// 账号层面，revert 改的是书签，必须用作用域正好是目标档案的那枚。
func dadTokHeader() string { return "Bearer " + dadTok }

func TestRevertRestoresOlderStateAsANewRev(t *testing.T) {
	s, h := setupBatch(t)
	// rev1 已在 setupBatch 里；先做一次写入造出 rev2，快照保住 rev1。
	body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"改过的"}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
	}
	b, _ := s.LoadBookmarks("dad")
	if b.Rev != 2 || b.Groups[0].Items[0].Name != "改过的" {
		t.Fatalf("前置状态不对: rev=%d", b.Rev)
	}
	rec := post(h, "/api/v1/revert?profile=dad&to-rev=1", "", "", dadTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Profile      string `json:"profile"`
		Rev          int64  `json:"rev"`
		RestoredFrom int64  `json:"restoredFrom"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Rev != 3 || out.RestoredFrom != 1 {
		t.Fatalf("revert 该是一次新写入（rev 前进）: %+v", out)
	}
	b, _ = s.LoadBookmarks("dad")
	if b.Rev != 3 || b.Groups[0].Items[0].Name != "群晖" {
		t.Fatalf("内容没回到 rev1: rev=%d name=%q", b.Rev, b.Groups[0].Items[0].Name)
	}
	// 不抹历史：现在该有两条（batch 一条、revert 一条）。
	hist, _ := s.ReadHistory("dad", 0)
	if len(hist) != 2 {
		t.Fatalf("历史该保留并追加: %+v", hist)
	}
	if hist[0].Ops[0].Op != "revert" || hist[0].Ops[0].Path != "snapshots/1" {
		t.Fatalf("revert 该在历史里自成一式: %+v", hist[0].Ops)
	}
	// revert 之前的状态本身也要保住：rev2 的快照该存在。
	if _, err := s.LoadSnapshot("dad", 2); err != nil {
		t.Fatalf("revert 前没给当前状态做快照: %v", err)
	}
}

func TestRevertArgumentAndScope(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, qs, bearer, cookie string
		want                     int
	}{
		{"缺 profile", "?to-rev=1", adminTok, "", 400},
		{"缺 to-rev", "?profile=dad", adminTok, "", 400},
		{"to-rev 不是整数", "?profile=dad&to-rev=abc", adminTok, "", 400},
		{"to-rev 是负数", "?profile=dad&to-rev=-1", adminTok, "", 400},
		{"快照不存在", "?profile=dad&to-rev=99", dadTok, "", 404},
		{"档案不存在", "?profile=ghost&to-rev=1", ghostTok, "", 404},
		{"作用域外令牌", "?profile=kid&to-rev=1", dadTok, "", 403},
		{"无凭证", "?profile=dad&to-rev=1", "", "", 401},
		// **管理员会话碰不到别人的书签**（2026-10-10 的承重证人）：原来这一格是
		// "管理员代管 kid，kid 没有快照所以 404"——代管态不存在了，同一发请求现在
		// 止步在权限层。删掉这一格之前，"管理员能改任何人的书签"可以一路绿过去。
		{"管理员会话读别人的书签", "?profile=kid&to-rev=1", "", "lb_profile=dad", 403},
	} {
		// brief 里这一格走的是 getBearer（GET 通路）；/api/v1/revert 只注册
		// POST，GET 会被 mux 判成 404，除两格 404 外全红。改走 getWithMethod
		// 的 POST 通路，Bearer 前缀进 Authorization——语义与 getBearer 相同。
		header := tc.cookie
		if tc.bearer != "" {
			header = "Bearer " + tc.bearer
		}
		if rec := getWithMethod(h, "POST", "/api/v1/revert"+tc.qs, header); rec.Code != tc.want {
			t.Fatalf("%s want %d got %d %s", tc.name, tc.want, rec.Code, rec.Body)
		}
	}
}

// revert 只回书签：profile.json 的偏好不在恢复范围内（spec §4.6）。
func TestRevertDoesNotTouchProfileJSON(t *testing.T) {
	s, h := setupBatch(t)
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	p.Title = "改过的名字"
	p.Theme = "light"
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	// 再改一次偏好，然后 revert 书签。
	p2, _ := s.LoadProfile("dad")
	p2.Title = "又改了一次"
	if err := s.SaveProfile(p2); err != nil {
		t.Fatal(err)
	}
	// 本用例自己写了 snapshots/1，200 必然可达：非 200 就是端点坏了，
	// Fatalf 而不是 Logf（旧版容忍任何非 200，端点整条断掉也能绿）。
	if rec := getWithMethod(h, "POST", "/api/v1/revert?profile=dad&to-rev=1", dadTokHeader()); rec.Code != 200 {
		t.Fatalf("revert 该成功（snapshots/1 是本用例亲手写的）, got %d %s", rec.Code, rec.Body)
	}
	// 正向证人：恢复确实发生了——rev 从 1 前进到 2，历史多一行 revert。
	// 只验 Title 不变的话，"revert 根本没跑"也满足断言。
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("revert 后读不了书签: %v", err)
	}
	if b.Rev != 2 {
		t.Fatalf("revert 该把 rev 1 恢复成新一次写入（rev 2）: %d", b.Rev)
	}
	hist, _ := s.ReadHistory("dad", 0)
	if len(hist) != 1 || hist[0].Ops[0].Op != "revert" {
		t.Fatalf("历史该有这一条 revert: %+v", hist)
	}
	now, _ := s.LoadProfile("dad")
	if now.Title != "又改了一次" {
		t.Fatalf("revert 动了 profile.json: %q", now.Title)
	}
}

// 坏档案（bookmarks.json 读不了）恰恰是最想 revert 的时刻：
// 这条通路不许被 Current() 的坏档判断挡死。
func TestRevertWorksOnBrokenProfile(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.WriteSnapshot("dad"); err != nil { // rev1 的快照
		t.Fatal(err)
	}
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte("{坏掉"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := getWithMethod(h, "POST", "/api/v1/revert?profile=dad&to-rev=1", dadTokHeader())
	if rec.Code != 200 {
		t.Fatalf("坏档案该能 revert, got %d %s", rec.Code, rec.Body)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("revert 后仍读不了: %v", err)
	}
	if b.Rev != 2 {
		t.Fatalf("revert 该产生新 rev: %d", b.Rev)
	}
}

// revert 的新 rev 必须大于 max(盘上当前 rev, 被恢复内容的 rev, 最高 retained 快照)。
// 只保前两者会留一个静默 clobber：坏档下 CurrentRev 报 0，revert 到 rev1 产出
// rev2——与 snapshots/2 同名。下一次写入的 WriteSnapshot 按内容 rev 命名目录，
// 直接覆盖 snapshots/2，磁盘上留下两份不同内容都叫 rev 2。
func TestRevertRevBoundsByHighestRetainedSnapshot(t *testing.T) {
	s, h := setupBatch(t)
	// 造出 snapshots/1..3 与 rev3 的 bookmarks.json。
	if _, err := s.WriteSnapshot("dad"); err != nil { // snapshots/1
		t.Fatal(err)
	}
	for _, name := range []string{"二次改名", "三次改名"} {
		body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"` + name + `"}]}`
		if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
			t.Fatalf("前置写入 %s: %d %s", name, rec.Code, rec.Body)
		}
		if _, err := s.WriteSnapshot("dad"); err != nil { // snapshots/2、snapshots/3
			t.Fatal(err)
		}
	}
	original2, err := s.LoadSnapshot("dad", 2)
	if err != nil {
		t.Fatal(err)
	}
	// 撕坏 bookmarks.json：CurrentRev 只能报 (0, ErrCorrupt)。
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte("{坏掉"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := getWithMethod(h, "POST", "/api/v1/revert?profile=dad&to-rev=1", dadTokHeader())
	if rec.Code != 200 {
		t.Fatalf("坏档该能 revert, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Rev int64 `json:"rev"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Rev <= 3 {
		t.Fatalf("新 rev 必须高于最高 retained 快照 3，否则会与 snapshots/2 撞名: rev=%d", out.Rev)
	}
	// 下一次写入是 clobber 的真正现场：WriteSnapshot 按当前内容的 rev 命名目录。
	// revert 若产出 rev≤3，这一步就把 snapshots/2 的原始字节覆盖掉。
	body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"revert 之后"}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("revert 后写入失败: %d %s", rec.Code, rec.Body)
	}
	now2, err := s.LoadSnapshot("dad", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original2, now2) {
		t.Fatalf("snapshots/2 被 revert 后的撞名写入覆盖了：rev 不能复用快照已经指着的号")
	}
}

// to-rev=0 的端点取法（与 CLI 对 --to 0 的用法错误拒收**相反**，是刻意的）：
// CreateProfile 会落一份 rev-0 的 bookmarks.json，snapshots/0 因此可以是
// 真实快照——有就恢复它、没有才 404。端点不特殊拒绝 0，把判断交给查表。
func TestRevertToRevZeroRestoresRealSnapshot(t *testing.T) {
	s, h := setup(t)                                  // dad 建档即 rev 0
	if _, err := s.WriteSnapshot("dad"); err != nil { // 保的就是 rev 0 那份
		t.Fatal(err)
	}
	rec := getWithMethod(h, "POST", "/api/v1/revert?profile=dad&to-rev=0", dadTokHeader())
	if rec.Code != 200 {
		t.Fatalf("rev-0 快照是真实的，to-rev=0 该恢复它, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Rev          int64 `json:"rev"`
		RestoredFrom int64 `json:"restoredFrom"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Rev != 1 || out.RestoredFrom != 0 {
		t.Fatalf("恢复 rev0 该产生新 rev1: %+v", out)
	}
}

// byOfRequest 的三档：缺省 web；合法值原样（含本案补进枚举的 "cli"）；
// 非法值退回 web（自报口径不参与判定，不值得一次 422）。
func TestRevertByOfRequestChannel(t *testing.T) {
	for _, tc := range []struct{ hdr, want string }{
		{"", "web"},
		{"cli", "cli"},
		{"ai", "ai"},
		{"  ", "web"},
		{"cron", "web"},
	} {
		r := httptest.NewRequest("POST", "/api/v1/revert", nil)
		if tc.hdr != "" {
			r.Header.Set("X-LB-By", tc.hdr)
		}
		if got := byOfRequest(r); got != tc.want {
			t.Fatalf("X-LB-By=%q 该记 %q，实际 %q", tc.hdr, tc.want, got)
		}
	}
}
