package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

const (
	adminTok  = "tok-admin"  // setup(): admin:tok-admin:*（只管账号，改判后过不了 credScoped）
	nobodyTok = "tok-nobody" // setup(): ai-nobody:tok-nobody:nobody —— 同上，给 resolve 那条 404 用
	ghostTok  = "tok-ghost"  // setup(): ai-ghost:tok-ghost:ghost —— 作用域对但盘上没有，用来打 404
	dadTok    = "tok-dad"    // setup(): ai-dad:tok-dad:dad
	kidTok    = "tok-kid"    // setup(): ai-kid:tok-kid:kid —— 新模型下"每个档案一枚令牌"
)

// setupBatch 在 setup() 之上给 dad 与 kid 各放一份有内容的书签。
func setupBatch(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	s, h := setup(t)
	if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 1, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList,
		Items: []model.Item{{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBookmarks("kid", &model.Bookmarks{Rev: 1, Title: "小满的", Groups: []model.Group{{
		ID: "g1", Title: "看", Layout: model.LayoutTiles,
		Items: []model.Item{{ID: "cartoon", Name: "动画", URL: "http://10.0.0.9", Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	return s, h
}

func TestBatchHappyPathBumpsRevAndSnapshotsAndLogs(t *testing.T) {
	s, h := setupBatch(t)
	body := `{"profile":"dad","by":"ai","expectedRev":1,"ops":[
	  {"op":"add","path":"groups/nas/items","value":{"id":"jellyfin","name":"影视库","url":"http://10.0.0.3:8096","icon":"simple-icons:jellyfin"}}]}`
	rec := post(h, "/api/v1/batch", body, "", dadTok)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body)
	}
	// wire 上 warnings 必须是 [] 而不是 null（§4.3 的空数组契约）。
	// 这是本包响应形状唯一保留的字节级断言——其余一律解出来比值。
	if !strings.Contains(rec.Body.String(), `"warnings": []`) {
		t.Fatalf(`warnings 该以 "warnings": [] 上 wire: %s`, rec.Body)
	}
	var out struct {
		Profile  string   `json:"profile"`
		Rev      int64    `json:"rev"`
		Applied  int      `json:"applied"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Profile != "dad" || out.Rev != 2 || out.Applied != 1 {
		t.Fatalf("响应不对: %+v", out)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 2 || b.Groups[0].Items[1].ID != "jellyfin" {
		t.Fatalf("磁盘没写对: %+v", b.Groups[0].Items)
	}
	// 写前快照保的是 rev 1 那一份（解出来比值，不做字节子串断言）。
	raw, err := s.LoadSnapshot("dad", 1)
	if err != nil {
		t.Fatalf("该有 rev1 的快照: %v", err)
	}
	var snap model.Bookmarks
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("快照不是合法书签 JSON: %v", err)
	}
	if snap.Rev != 1 {
		t.Fatalf("快照里是写入之后的状态，顺序错了: rev=%d", snap.Rev)
	}
	hist, err := s.ReadHistory("dad", 0)
	if err != nil || len(hist) != 1 {
		t.Fatalf("历史该有一条: %v %+v", err, hist)
	}
	// actor 是**令牌的名字**：这条用的是作用域为 dad 的那枚（ai-dad），
	// 不再是管理员令牌——`*` 在 2026-10-10 之后只管账号层面。
	if hist[0].By != "ai" || hist[0].Actor != "ai-dad" || hist[0].Rev != 2 {
		t.Fatalf("历史内容不对: %+v", hist[0])
	}
	if len(hist[0].Ops) != 1 || hist[0].Ops[0].Op != "add" || hist[0].Ops[0].Path != "groups/nas/items" {
		t.Fatalf("历史该记下动作与路径: %+v", hist[0].Ops)
	}
	// 白名单的原始行保险：历史里不许出现 op 的 value（URL 含 8096），
	// 只该有 op+path——类型系统挡不住手滑，这一行挡。
	rawHist, err := os.ReadFile(s.HistoryPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawHist), "8096") {
		t.Fatalf("value 泄进历史了: %s", rawHist)
	}
}

// 全成或全败：第 2 条错，第 1 条也不许留在磁盘上；失败的批次
// 既不留快照也不写历史——什么都没发生才对。
func TestBatchIsAllOrNothing(t *testing.T) {
	s, h := setupBatch(t)
	body := `{"profile":"dad","by":"ai","ops":[
	  {"op":"set","path":"groups/nas/items/dsm/name","value":"改名成功但白做"},
	  {"op":"set","path":"groups/nas/items/ghost/name","value":"x"}]}`
	rec := post(h, "/api/v1/batch", body, "", dadTok)
	if rec.Code != 422 {
		t.Fatalf("want 422, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Errors []struct {
			OpsIndex int    `json:"opsIndex"`
			Field    string `json:"field"`
			Reason   string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) != 1 || out.Errors[0].OpsIndex != 1 {
		t.Fatalf("要逐条指到出错的那条: %+v", out.Errors)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 1 || b.Groups[0].Items[0].Name != "群晖" {
		t.Fatalf("整批失败却把第一条写进去了: rev=%d name=%q", b.Rev, b.Groups[0].Items[0].Name)
	}
	if _, err := s.LoadSnapshot("dad", 1); err == nil {
		t.Fatal("失败的批次不该做快照")
	}
	if hist, _ := s.ReadHistory("dad", 0); len(hist) != 0 {
		t.Fatalf("失败的批次不该写历史: %+v", hist)
	}
}

func TestBatchConflictScopeAndMissing(t *testing.T) {
	_, h := setupBatch(t)
	rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"ai","expectedRev":9,"ops":[]}`, "", dadTok)
	if rec.Code != 409 {
		t.Fatalf("rev 冲突该 409, got %d %s", rec.Code, rec.Body)
	}
	var conf struct {
		Code       int    `json:"code"`
		Detail     string `json:"detail"`
		CurrentRev *int64 `json:"currentRev"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &conf); err != nil {
		t.Fatal(err)
	}
	if conf.Code != 409 || conf.Detail == "" || conf.CurrentRev == nil || *conf.CurrentRev != 1 {
		t.Fatalf("409 要带上现 rev 让调用方重读: %s", rec.Body)
	}
	// ai-dad 的作用域是 dad：写 kid 必须 403，且不能"回落"到 dad。
	rec = post(h, "/api/v1/batch", `{"profile":"kid","by":"ai","ops":[]}`, "", dadTok)
	if rec.Code != 403 {
		t.Fatalf("越作用域该 403, got %d %s", rec.Code, rec.Body)
	}
	// 死令牌（不认识）该 401，不许报成 403：那句"作用域不含该档案"
	// 会骗着调用方拿同一个死令牌反复重试（Task 5 评审的 Important）。
	if rec := post(h, "/api/v1/batch", `{"profile":"kid","by":"ai","ops":[]}`, "", "tok-bogus"); rec.Code != 401 {
		t.Fatalf("未知令牌该 401 而不是 403, got %d %s", rec.Code, rec.Body)
	}
	// 无凭证 401。
	if rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"ai","ops":[]}`, "", ""); rec.Code != 401 {
		t.Fatalf("无凭证该 401, got %d", rec.Code)
	}
	// 档案不存在 404，与"无权限"分开返回（spec §12）。
	if rec := post(h, "/api/v1/batch", `{"profile":"ghost","by":"ai","ops":[]}`, "", ghostTok); rec.Code != 404 {
		t.Fatalf("档案不存在该 404, got %d %s", rec.Code, rec.Body)
	}
}

func TestBatchRejectsUnknownBy(t *testing.T) {
	_, h := setupBatch(t)
	rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"cron","ops":[]}`, "", dadTok)
	if rec.Code != 422 {
		t.Fatalf("未知 by 该 422, got %d %s", rec.Code, rec.Body)
	}
	// 解出来比值而不是抓字节：要点名是哪个字段。
	var out struct {
		Code   int `json:"code"`
		Errors []struct {
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) == 0 || out.Errors[0].Field != "by" {
		t.Fatalf("要点名是哪个字段: %s", rec.Body)
	}
}

// 部署验收遗留 #5：请求体里的非法 UTF-8 不许被 encoding/json 静默
// 替换成 U+FFFD 存进磁盘（实测过一次，盘上落的是 d0 a1 + 两个 U+FFFD）。
func TestBatchRejectsInvalidUTF8Body(t *testing.T) {
	s, h := setupBatch(t)
	raw := "{\"profile\":\"dad\",\"by\":\"ai\",\"ops\":[{\"op\":\"set\"," +
		// \xd0\xa1 单独是合法 UTF-8（U+0421），后面再跟两个各自非法的
		// 字节：不拦的话 encoding/json 会把它们各替换成一个 U+FFFD，
		// 盘上落的就是注释里说的 d0 a1 + 两个 U+FFFD。
		"\"path\":\"groups/nas/items/dsm/name\",\"value\":\"\xd0\xa1\xb5\xd6\"}]}"
	rec := post(h, "/api/v1/batch", raw, "", dadTok)
	if rec.Code != 422 {
		t.Fatalf("非法 UTF-8 该 422, got %d %s", rec.Code, rec.Body)
	}
	// 不仅要拒，还要说清是被什么闸拒的：422 的理由里该点出 UTF-8，
	// 调用方才不至于对着一个合法 JSON 猜自己哪里错了。
	if !strings.Contains(rec.Body.String(), "UTF-8") {
		t.Fatalf("422 该点名 UTF-8: %s", rec.Body)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	// U+FFFD 用转义写：裸字符放进源文件会被工具链吃掉。
	if strings.ContainsRune(b.Groups[0].Items[0].Name, '\uFFFD') {
		t.Fatalf("U+FFFD 被写进磁盘了: %q", b.Groups[0].Items[0].Name)
	}
}

// 空 ops 不是错误，但不该产生新 rev / 快照 / 历史：
// 一次什么都没做的写入留在审计线里是噪声。
func TestBatchEmptyOpsIsNoop(t *testing.T) {
	s, h := setupBatch(t)
	rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"ai","ops":[]}`, "", dadTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	b, _ := s.LoadBookmarks("dad")
	if b.Rev != 1 {
		t.Fatalf("空批不该 bump rev: %d", b.Rev)
	}
	if hist, _ := s.ReadHistory("dad", 0); len(hist) != 0 {
		t.Fatalf("空批不该写历史: %+v", hist)
	}
	// 用 ListSnapshots 判"没有快照"而不是 SnapshotCount：后者出错也返回 0，
	// 它自己的注释就警告"别把 0 当没有快照"——真要判断得看 error。
	if list, err := s.ListSnapshots("dad"); err != nil || len(list) != 0 {
		t.Fatalf("空批不该做快照: err=%v list=%+v", err, list)
	}
}

// 快照写不下去 = 这次写入整个取消（spec §12：不许"跳过快照继续写"）。
// 等价夹具：把 snapshots 路径做成一个普通文件，MkdirAll 必然失败。
// 真实场景是磁盘满；这里不装盘，只验"失败即取消"。
func TestBatchSnapshotFailureRefusesWrite(t *testing.T) {
	s, h := setupBatch(t)
	dir := s.SnapshotsDir("dad") // 父目录 profiles/dad 已由建档建好
	if err := os.WriteFile(dir, []byte("挡路"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"改不动"}]}`
	rec := post(h, "/api/v1/batch", body, "", dadTok)
	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("快照失败该 507, got %d %s", rec.Code, rec.Body)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 1 || b.Groups[0].Items[0].Name == "改不动" {
		t.Fatalf("507 却把书签写了: rev=%d", b.Rev)
	}
}

func TestBatchHistoryFailureStillWritesAndWarns(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.Mkdir(s.HistoryPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"历史写不进但书签要落盘"}]}`
	rec := post(h, "/api/v1/batch", body, "", dadTok)
	if rec.Code != 200 {
		t.Fatalf("历史失败不许 5xx（会诱导重复应用）, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Rev      int64    `json:"rev"`
		Applied  int      `json:"applied"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(out.Warnings, "|"), "历史没写进去") {
		t.Fatalf("warnings 该有历史缺失那条: %+v", out.Warnings)
	}
	if out.Rev != 2 || out.Applied != 1 {
		t.Fatalf("成功形状不许变形: %+v", out)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 2 || b.Groups[0].Items[0].Name != "历史写不进但书签要落盘" {
		t.Fatalf("书签必须真的写入: rev=%d name=%q", b.Rev, b.Groups[0].Items[0].Name)
	}
}

// 告警从不挡写入：ops.Run 报的 warning（值与当前相同）照样落盘、照样
// bump rev——"没改成东西"是信息，不是失败。
func TestBatchWarningDoesNotBlockWrite(t *testing.T) {
	s, h := setupBatch(t)
	body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"群晖"}]}`
	rec := post(h, "/api/v1/batch", body, "", dadTok)
	if rec.Code != 200 {
		t.Fatalf("告警不是错误, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Rev      int64    `json:"rev"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Warnings) == 0 {
		t.Fatalf("设了与当前相同的值该有告警: %s", rec.Body)
	}
	if out.Rev != 2 {
		t.Fatalf("告警不许拦 rev: %+v", out)
	}
	if b, err := s.LoadBookmarks("dad"); err != nil || b.Rev != 2 {
		t.Fatalf("磁盘该写到 rev 2: %v %+v", err, b)
	}
}

// 坏档案该 503 而不是 500/422：不是服务的错，也不是这次请求的错。
// 机制：LoadBookmarks 在 Current 返回之前就把档案标坏（markBroken），
// 同一个加锁读里 BrokenReason 有值——"能用但没测"的分支，不钉住的话
// 下一次重构会把它悄悄变成 500。
func TestBatchBrokenProfileIs503(t *testing.T) {
	s, h := setupBatch(t)
	brokenDoc := `{"rev":1,"groups":`
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte(brokenDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"动不了"}]}`
	rec := post(h, "/api/v1/batch", body, "", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("坏档案该 503, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != 503 || !strings.Contains(out.Detail, "bookmarks.json") {
		t.Fatalf("detail 要点名坏的是哪个文件（AI 拿着它能直接去修）: %+v", out)
	}
	// 写入没碰这个文件：盘上还是那半截 JSON（这一条只能按字节比——
	// 文件本来就解不开，这正是 503 的题眼）。
	if raw, err := os.ReadFile(s.BookmarksPath("dad")); err != nil || string(raw) != brokenDoc {
		t.Fatalf("503 不许改档案: %v %s", err, raw)
	}
	// 也没顺手做快照（校验都没过）。
	if list, err := s.ListSnapshots("dad"); err != nil || len(list) != 0 {
		t.Fatalf("坏档案不许留快照: err=%v list=%+v", err, list)
	}
}
