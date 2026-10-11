package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// getBearer 是给只带令牌不带 cookie 的用例用的（Task 7+ 的读/删端点复用）。
func getBearer(h http.Handler, path, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestReadDataNeedsCredentials(t *testing.T) {
	_, h := setupBatch(t)
	for _, tc := range []struct {
		name   string
		qs     string
		bearer string
		cookie string
		want   int
	}{
		{"匿名", "?profile=dad", "", "", 401},
		{"作用域外令牌", "?profile=kid", dadTok, "", 403},
		{"档案作用域令牌", "?profile=dad", dadTok, "", 200},
		// **`*` 令牌读不到别人的书签**（2026-10-10 改判的承重证人）：管理员令牌
		// 只管账号，读 dad 的书签要用作用域正好是 dad 的那枚。删掉这一格之前，
		// 读与写的越界是可以一路绿着过去的。
		{"管理员令牌读别人的书签", "?profile=dad", adminTok, "", 403},
		{"档案不存在", "?profile=ghost", ghostTok, "", 404},
		{"缺 profile", "", dadTok, "", 400},
		{"profile 不是 slug", "?profile=%2B%2B", dadTok, "", 400},
		{"会话本人", "?profile=dad", "", "lb_profile=dad", 200},
		// 管理员会话同样只管账号。
		{"管理员会话读别人的书签", "?profile=kid", "", "lb_profile=dad", 403},
	} {
		var rec *httptest.ResponseRecorder
		if tc.bearer != "" {
			rec = getBearer(h, "/api/v1/data"+tc.qs, tc.bearer)
		} else {
			rec = get(h, "/api/v1/data"+tc.qs, tc.cookie)
		}
		if rec.Code != tc.want {
			t.Fatalf("%s want %d got %d %s", tc.name, tc.want, rec.Code, rec.Body)
		}
	}
}

func TestReadDataShape(t *testing.T) {
	_, h := setupBatch(t)
	rec := getBearer(h, "/api/v1/data?profile=dad", dadTok)
	var out struct {
		Profile  string          `json:"profile"`
		Rev      int64           `json:"rev"`
		Modified string          `json:"modified"`
		Data     model.Bookmarks `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("不是合法 JSON: %v %s", err, rec.Body)
	}
	if out.Profile != "dad" || out.Rev != 1 || len(out.Data.Groups) != 1 {
		t.Fatalf("got %+v", out)
	}
	if out.Modified == "" {
		t.Fatal("modified 要给 RFC3339，AI 用它判断数据新旧")
	}
	// 顶层键序 profile, rev, modified, data 写死一次：防有人改成 map 输出
	// （§4.3 的"禁止用 map 产生输出键序"对 API 响应同样成立）。
	// wire 形状按本包 writeJSON 的实际输出比：2 空格缩进、冒号后带空格
	// （batch_test 里 `"warnings": []` 同一口径），不是紧凑形。
	if raw := rec.Body.String(); !strings.HasPrefix(raw, "{\n  \"profile\": \"dad\",\n  \"rev\": 1,\n") {
		t.Fatalf("键序变了: %.60s", raw)
	}
}

// export 的"与磁盘内容一致"要能逐字节比：AI 拿它做改前/改后 diff。
func TestExportIsByteIdenticalToDisk(t *testing.T) {
	s, h := setupBatch(t)
	rec := getBearer(h, "/api/v1/export?profile=dad", dadTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	want, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Body.String() != string(want) {
		t.Fatalf("export 与磁盘不一致")
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="dad-bookmarks.json"` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	// 加固：夹具里磁盘文件是 SaveBookmarks 写的规范形，"从内存重编码"
	// 与它逐字节相同——body 断言对这个破坏是睁眼瞎（Task 6 自审发现）。
	// 真实场景是手改过的文件，所以这里把它改成非规范缩进再比一轮：
	// 走一遍内存模型必按 §4.3 归一，字节就对不上了。
	alt := strings.ReplaceAll(string(want), "\n  \"", "\n    \"")
	if alt == string(want) {
		t.Fatal("夹具文件里没有可缩进改写的键？夹具变了要复核本用例")
	}
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte(alt), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = getBearer(h, "/api/v1/export?profile=dad", dadTok)
	if rec.Body.String() != alt {
		t.Fatalf("export 必须回原字节，不许过内存模型归一: got %s", rec.Body)
	}
}

// export 另有两条分支在 Task 6 评审时无人证过：档案不存在该 404；档案存在
// 但还没写过 bookmarks.json 该给一份"合法的空"（调用方要的是能喂回 batch
// 的东西），而不是 404 也不是空响应体。
func TestExportMissingProfileAndEmptyProfile(t *testing.T) {
	s, h := setupBatch(t)
	// ghostTok：作用域正好是 ghost、而盘上没有 ghost——2026-10-10 之后
	// "`*` 令牌读任意档案"那条路没了，404 只剩这一种走法。
	if rec := getBearer(h, "/api/v1/export?profile=ghost", ghostTok); rec.Code != http.StatusNotFound {
		t.Fatalf("档案不存在该 404, got %d %s", rec.Code, rec.Body)
	}
	if err := os.Remove(s.BookmarksPath("kid")); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/export?profile=kid", kidTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("空档案该 200 而不是 404, got %d %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); body != "{}\n" {
		t.Fatalf("空档案的 body = %q，要 \"{}\\n\"", body)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="kid-bookmarks.json"` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.AppendHistory(store.HistoryEntry{At: "t", Profile: "dad", Actor: "admin", By: "ai", Rev: 2}); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/history?profile=dad&limit=5", dadTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Entries []store.HistoryEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Entries) != 1 || out.Entries[0].Rev != 2 {
		t.Fatalf("got %+v", out.Entries)
	}
	// 空历史该是 []，不是 null（§4.3 对 API 同样成立）。wire 上是缩进形
	// `"entries": []`（writeJSON 的口径，见上）。
	rec = getBearer(h, "/api/v1/history?profile=kid", kidTok)
	if !strings.Contains(rec.Body.String(), `"entries": []`) {
		t.Fatalf("空集写成 null 了: %s", rec.Body)
	}
	// limit 不是数字 → 400。"0 = 全部"与"填错了"是两件事。
	if rec := getBearer(h, "/api/v1/history?limit=abc", dadTok); rec.Code != 400 {
		t.Fatalf("坏 limit 该 400, got %d", rec.Code)
	}
	// 档案作用域的令牌只能看自己的历史——**dad 的令牌看不了 kid**。
	if rec := getBearer(h, "/api/v1/history?profile=kid", dadTok); rec.Code != 403 {
		t.Fatalf("越作用域该 403, got %d", rec.Code)
	}
}

// 审计线的可见性按"谁在用"判，不是按"有没有申报"判。本用例是 Task 6 评审
// 那条 Critical 的证人：上面 /history 的用例全走 bearer，于是"kid 会话不带
// ?profile= 就能读到全家庭审计线"这个洞在绿测试下活了一整轮。cookie 通路
// 必须自己有一条，形状才齐。
func TestHistorySessionVisibility(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.AppendHistory(store.HistoryEntry{At: "t", Profile: "kid", Actor: "kid", By: "web", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(store.HistoryEntry{At: "t", Profile: "dad", Actor: "dad", By: "web", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	kid := "lb_profile=kid"
	dad := "lb_profile=dad"
	for _, tc := range []struct {
		name, qs, cookie string
		want             int
	}{
		{"kid 不申报（要全家庭）该拒", "", kid, 403},
		{"kid 报自己", "?profile=kid", kid, 200},
		{"kid 报别人", "?profile=dad", kid, 403},
		{"dad 不申报（管理员聚合）放行", "", dad, 200},
		{"匿名不申报该 401", "", "", 401},
	} {
		rec := get(h, "/api/v1/history"+tc.qs, tc.cookie)
		if rec.Code != tc.want {
			t.Fatalf("%s want %d got %d %s", tc.name, tc.want, rec.Code, rec.Body)
		}
	}
	// 拒了还得真的什么都没给：403 的响应体里不许混进别人的审计行。
	rec := get(h, "/api/v1/history", kid)
	if body := rec.Body.String(); strings.Contains(body, `"entries"`) {
		t.Fatalf("403 还在审计线上漏了内容: %s", body)
	}
}

// 坏档案的读端点该给 503 + 文件名与行号：不是 500（不是我们的错）、
// 不是 404（档案在，只是读不了）、更不回显磁盘绝对路径。
func TestReadBrokenProfileIs503WithReason(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte("{\n  \"rev\": 1,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/data?profile=dad", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "bookmarks.json") {
		t.Fatalf("要报出是哪个文件: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), s.Root()) {
		t.Fatalf("不许把磁盘绝对路径回显出去: %s", rec.Body)
	}
}
