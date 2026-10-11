package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
)

// fieldErrorsName 判 422 体里是否逐条点名了某个字段（且给了理由）——
// 本包对错误体的标准读法：解出来比值，不抓字节。
func fieldErrorsName(rec *httptest.ResponseRecorder, field string) bool {
	var out struct {
		Errors []struct {
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		return false
	}
	for _, e := range out.Errors {
		if e.Field == field && e.Reason != "" {
			return true
		}
	}
	return false
}

// loginAs 用 PIN 进门并返回落会话 cookie 的响应（self/pin 只认会话，
// 测试拿它的 Set-Cookie 组后续请求的 Cookie 头）。
func loginAs(t *testing.T, h http.Handler, profile, pin string) *httptest.ResponseRecorder {
	t.Helper()
	return post(h, "/pin", `{"profile":"`+profile+`","pin":"`+pin+`"}`, "", "")
}

// sessionCookie 把登录响应里的两条会话 cookie 拼成请求头原文。
func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	cm := cookieMap(t, rec)
	return auth.CookieProfile + "=" + cm[auth.CookieProfile] + "; " +
		auth.CookieHome + "=" + cm[auth.CookieHome]
}

func TestSelfPinNeedsOldPinAndSession(t *testing.T) {
	s, h := setupBatch(t)
	// setup() 里 dad 的 PIN 是 1234。
	rec := loginAs(t, h, "dad", "1234")
	cookie := sessionCookie(t, rec)
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"1234","new":"4321"}`, cookie, ""); got.Code != 200 {
		t.Fatalf("改自己 PIN 该 200, got %d %s", got.Code, got.Body)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPIN(p.PIN, "4321") || auth.VerifyPIN(p.PIN, "1234") {
		t.Fatal("PIN 没换过来，或旧 PIN 还能用")
	}
	// 被拒的改动一页都不许动盘：按 profile.json 的**原始字节**比（评审
	// Minor）。重读模型比哈希只能证明"PIN 没变"，证不出"文件没被重写过"
	// ——而整档重写恰恰是这条路径的失败模式（陈旧副本把别人的改动盖回去）。
	// 前提：先把盘上形状换成"合法但紧凑单行"。本服务写出的是 2 空格缩进，
	// 规范形状重抄一遍的字节与原件相同，那这条断言就等于没测；夹具压紧凑
	// 用的是 map（只为摆磁盘形状，与被裁定的"响应体不许用 map"无关）。
	canonical, err := os.ReadFile(s.ProfilePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	var shaped map[string]any
	if err := json.Unmarshal(canonical, &shaped); err != nil {
		t.Fatal(err)
	}
	rawBefore, err := json.Marshal(shaped)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ProfilePath("dad"), rawBefore, 0o644); err != nil {
		t.Fatal(err)
	}
	profileUntouched := func(what string) {
		t.Helper()
		raw, err := os.ReadFile(s.ProfilePath("dad"))
		if err != nil {
			t.Fatalf("读 profile.json: %v", err)
		}
		if !bytes.Equal(raw, rawBefore) {
			t.Fatalf("%s 之后 profile.json 的字节变了:\nwant %q\ngot  %q", what, rawBefore, raw)
		}
	}
	// 旧 PIN 不对 → 403，且不改盘。
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"9999","new":"1111"}`, cookie, ""); got.Code != 403 {
		t.Fatalf("旧 PIN 不对该 403, got %d %s", got.Code, got.Body)
	}
	profileUntouched("旧 PIN 不对（403）")
	// 新 PIN 形状不合法 → 422 点名 new。（解出来比值，不做字节子串断言：
	// §4.3 的写出契约是 2 空格缩进，`"field":"new"` 这种紧凑子串永远匹配
	// 不上自家 wire 格式——brief 原文的这条断言是计划缺陷，Task 5 起
	// batch_test.go 就立了"除唯一一处空数组契约外一律解码比值"的规矩。）
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"4321","new":"12"}`, cookie, ""); got.Code != 422 ||
		!fieldErrorsName(got, "new") {
		t.Fatalf("got %d %s", got.Code, got.Body)
	}
	profileUntouched("新 PIN 形状不合法（422）")
	// 令牌不许走 self/pin（那是"谁有令牌谁能改谁"）。
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"4321","new":"2222"}`, "", adminTok); got.Code != 403 {
		t.Fatalf("令牌该被拒, got %d %s", got.Code, got.Body)
	}
	// 带着合法会话混令牌也拒——这才是 credSession 与 credScoped 的分界：
	// 上一格在 credScoped 下靠"空申报即拒"（Task 6 的 Critical 修）碰巧同
	// 码挡住；这一格 credScoped+`*` 令牌会真放行"凭令牌改别人 PIN"，改坏
	// 类别必须红（brief 的破坏① 只有这一格咬得动）。
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"4321","new":"2222"}`, cookie, adminTok); got.Code != 403 {
		t.Fatalf("会话+令牌混着来该被拒, got %d %s", got.Code, got.Body)
	}
	// 匿名 401。
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"4321","new":"2222"}`, "", ""); got.Code != 401 {
		t.Fatalf("匿名该 401, got %d", got.Code)
	}
}

func TestAdminPinGoesIntoTargetHistory(t *testing.T) {
	s, h := setupBatch(t)
	rec := loginAs(t, h, "dad", "1234")
	cookie := sessionCookie(t, rec)
	// dad 是管理员：给 kid 设 PIN，不带旧 PIN。
	if got := post(h, "/api/v1/profiles/pin", `{"profile":"kid","new":"8765"}`, cookie, ""); got.Code != 200 {
		t.Fatalf("管理员设 PIN 该 200, got %d %s", got.Code, got.Body)
	}
	p, err := s.LoadProfile("kid")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPIN(p.PIN, "8765") {
		t.Fatal("kid 的 PIN 没被设上")
	}
	// 目标档案的历史里该有一条，actor 是 dad。
	hist, _ := s.ReadHistory("kid", 0)
	if len(hist) != 1 || hist[0].Actor != "dad" || hist[0].Profile != "kid" {
		t.Fatalf("管理员设 PIN 要在目标档案留名: %+v", hist)
	}
	// 行的形状也是契约：rev:0 标记"这不是书签改动"（界面按这条渲染），
	// ops 指向 profile/pin 而不是 groups/...，by 走 X-LB-By 缺省 web。
	if hist[0].Rev != 0 || hist[0].By != "web" || len(hist[0].Ops) != 1 ||
		hist[0].Ops[0].Op != "set" || hist[0].Ops[0].Path != "profile/pin" {
		t.Fatalf("设 PIN 的历史行该是 rev:0 + set profile/pin: %+v", hist[0])
	}
	// 非管理员会话不许设别人。
	rec2 := loginAs(t, h, "kid", "8765")
	if got := post(h, "/api/v1/profiles/pin", `{"profile":"dad","new":"0000"}`, sessionCookie(t, rec2), ""); got.Code != 403 {
		t.Fatalf("非管理员设别人 PIN 该 403, got %d %s", got.Code, got.Body)
	}
	// 档案作用域令牌也不行（管理员端点要 *）。
	if got := post(h, "/api/v1/profiles/pin", `{"profile":"kid","new":"1234"}`, "", dadTok); got.Code != 403 {
		t.Fatalf("got %d", got.Code)
	}
}

// 设 PIN 不是书签写入：不许动 rev、不许做快照。
func TestPinChangeDoesNotTouchRevOrSnapshots(t *testing.T) {
	s, h := setupBatch(t)
	before, err := s.Current("kid")
	if err != nil {
		t.Fatal(err)
	}
	n := s.SnapshotCount("kid")
	rec := loginAs(t, h, "kid", "5678")
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"5678","new":"1357"}`, sessionCookie(t, rec), ""); got.Code != 200 {
		t.Fatalf("got %d %s", got.Code, got.Body)
	}
	after, _ := s.Current("kid")
	if after.Rev != before.Rev {
		t.Fatalf("改 PIN 不该 bump rev: %d → %d", before.Rev, after.Rev)
	}
	if s.SnapshotCount("kid") != n {
		t.Fatalf("改 PIN 不该做快照")
	}
	// 管理员给别人设 PIN 同样不是书签写入（此前只有 self 路径有证人；
	// 管理员那条历史行的 Rev:0 只是"暗示"不动 rev，钉不住实际写入）。
	dadCookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	if got := post(h, "/api/v1/profiles/pin", `{"profile":"kid","new":"2468"}`, dadCookie, ""); got.Code != 200 {
		t.Fatalf("管理员设 PIN 该 200, got %d %s", got.Code, got.Body)
	}
	if admin, _ := s.Current("kid"); admin.Rev != after.Rev {
		t.Fatalf("管理员设 PIN 不该 bump rev: %d → %d", after.Rev, admin.Rev)
	}
	if got := s.SnapshotCount("kid"); got != n {
		t.Fatalf("管理员设 PIN 不该做快照: %d → %d", n, got)
	}
	if p, err := s.LoadProfile("kid"); err != nil || !auth.VerifyPIN(p.PIN, "2468") {
		t.Fatalf("管理员那次设 PIN 要真的落盘: %v", err)
	}
}

// 评审 Important 2：profile.json 坏了，两条 PIN 端点必须与 /data、/batch、
// /history 同口径——503 + store 给的"文件名 + 第几行"（spec §12 要求报出
// 文件与错误位置），不是通用 500。500 把"家属把文件改坏了"说成"服务坏了"，
// 调用方拿到 500 只会重试，拿到 503+行号能直接去修那个文件。
func TestPinEndpointsBrokenProfileIs503(t *testing.T) {
	s, h := setupBatch(t)
	brokenDoc := "{\n  \"id\": \"kid\",\n  \"title\":\n"
	// 会话在文件坏掉之前拿到：坏档登不了门，但已经登过的门不许因此变成 500。
	cookie := sessionCookie(t, loginAs(t, h, "kid", "5678"))
	adminCookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	if err := os.WriteFile(s.ProfilePath("kid"), []byte(brokenDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path, body, cookie string }{
		{"self/pin", "/api/v1/profiles/self/pin", `{"old":"5678","new":"1357"}`, cookie},
		{"profiles/pin", "/api/v1/profiles/pin", `{"profile":"kid","new":"8765"}`, adminCookie},
	} {
		rec := post(h, tc.path, tc.body, tc.cookie, "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s：坏 profile.json 该 503, got %d %s", tc.name, rec.Code, rec.Body)
		}
		var out errorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s：503 体不是合法 JSON: %v %s", tc.name, err, rec.Body)
		}
		if out.Code != 503 || !strings.Contains(out.Detail, "profile.json") {
			t.Fatalf("%s：detail 要点名坏的是哪个文件（调用方拿着它能直接去修）: %+v", tc.name, out)
		}
		if strings.Contains(rec.Body.String(), s.Root()) {
			t.Fatalf("%s：不许把磁盘绝对路径回显出去: %s", tc.name, rec.Body)
		}
		// 503 不落盘：盘上还是那半截 JSON（这一条只能按字节比——文件本来
		// 就读不出来，走模型反而是自证）。
		if raw, err := os.ReadFile(s.ProfilePath("kid")); err != nil || string(raw) != brokenDoc {
			t.Fatalf("%s：503 不许改档案: %v %s", tc.name, err, raw)
		}
	}
}

// 评审 Important 2 的另一半：会话还活着、档案刚被删掉 = store.ErrNotFound。
// 管理员路径本来就 404，self 路径却报 500——同一个条件不许有两个答案。
// 两条都钉：分诊现在住在共用的 writeProfileReadFailure 里，只测一边等于
// 另一边随时可以被悄悄改回去。
func TestPinEndpointsMissingProfileIs404(t *testing.T) {
	s, h := setupBatch(t)
	cookie := sessionCookie(t, loginAs(t, h, "kid", "5678"))
	adminCookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	if err := s.DeleteProfile("kid"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path, body, cookie string }{
		// self：Active 指着刚删掉的档案。
		{"self/pin", "/api/v1/profiles/self/pin", `{"old":"5678","new":"1357"}`, cookie},
		// admin：目标档案不存在（管理员端点本来就分得出 404）。
		{"profiles/pin", "/api/v1/profiles/pin", `{"profile":"kid","new":"8765"}`, adminCookie},
	} {
		rec := post(h, tc.path, tc.body, tc.cookie, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s：档案已删该 404, got %d %s", tc.name, rec.Code, rec.Body)
		}
		var out errorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s：404 体不是合法 JSON: %v %s", tc.name, err, rec.Body)
		}
		if out.Code != 404 || out.Detail != "档案不存在" {
			t.Fatalf("%s：404 体要与另一条端点同形: %+v", tc.name, out)
		}
	}
}

// 评审 Important 1 的管理员半边：LoadProfile 必须在 WithProfileLock **之内**。
// 证人（与 self 那条同一手法）：测试占住 kid 的档案锁，在锁内把 profile.json
// 换成读不出来的半截 JSON。读在锁内 → 释放锁后管理员拿到的是这份新状态 →
// 503，盘上仍是那半截；读在锁外（回归）→ 拿到加锁前的可读副本，释放锁后
// 整档盖回去 → 200，盘上的损坏被抹掉——盘上一个 PIN、历史里另一句的幽灵
// 形状就是这么来的。锁内那 200ms 只是给回归分支走到它自己的读：修复版被
// mutex 保证读在释放之后，所以这一格不会因为机器慢而假红，最坏是漏检。
func TestAdminPinReadsInsideProfileLock(t *testing.T) {
	s, h := setupBatch(t)
	adminCookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	brokenDoc := "{\n  \"id\": \"kid\",\n  \"title\":\n"
	done := make(chan *httptest.ResponseRecorder, 1)
	err := s.WithProfileLock("kid", func() error {
		go func() {
			done <- post(h, "/api/v1/profiles/pin", `{"profile":"kid","new":"8765"}`, adminCookie, "")
		}()
		time.Sleep(200 * time.Millisecond)
		return os.WriteFile(s.ProfilePath("kid"), []byte(brokenDoc), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := <-done
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("锁内读该看见锁内改的损坏并 503, got %d %s", rec.Code, rec.Body)
	}
	if raw, err := os.ReadFile(s.ProfilePath("kid")); err != nil || string(raw) != brokenDoc {
		t.Fatalf("陈旧副本把 profile.json 盖回去了: %v %s", err, raw)
	}
}

// 评审 Important 1：self/pin 的读→验→写整段在档案锁内（SaveProfile 不自带
// 锁；裸 RMW 与并发的管理员设 PIN 撞车时，陈旧副本会把新哈希连同
// "dad 给 kid 设了 PIN"那行审计一起覆盖掉——盘上一个 PIN，历史里另一句）。
// 证人要确定，不靠两个 goroutine 抢时序：测试自己占住这条档案锁，占不到
// 锁就一步都不许走；改回锁外 RMW 时它照样完成（并当场落盘），立刻红。
func TestSelfPinWaitsForProfileLock(t *testing.T) {
	s, h := setupBatch(t)
	cookie := sessionCookie(t, loginAs(t, h, "kid", "5678"))
	done := make(chan int, 1)
	err := s.WithProfileLock("kid", func() error {
		go func() {
			done <- post(h, "/api/v1/profiles/self/pin", `{"old":"5678","new":"1357"}`, cookie, "").Code
		}()
		select {
		case code := <-done:
			return fmt.Errorf("档案锁还被占着，self/pin 就完成并落盘了（code %d）：RMW 不在锁内", code)
		case <-time.After(500 * time.Millisecond):
			return nil // 真在等锁：这是要的行为
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != 200 {
		t.Fatalf("等锁不是不干了：锁释放后该照常成功, got %d", code)
	}
	p, err := s.LoadProfile("kid")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPIN(p.PIN, "1357") {
		t.Fatal("锁释放后 PIN 要真的换上")
	}
}
