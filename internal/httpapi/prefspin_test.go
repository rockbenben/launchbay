package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
)

// 台面面板里那张「我的 PIN」表单（2026-10-20）。
//
// 它是**本人**改自己那道门的那条路——从前只有 API 与管理页，于是"能设就能撤"
// 那句话对本人是空的：他能改自己那份的界面、名字、背景，唯独改不了自己那道门
// （除非去求管理员拿管理密钥）。
//
// 判据与 API 那条**共用同一个落点**（selfPin），所以这里钉的是**行为**：
// 改了 / 旧 PIN 不对不改 / 留空 = 取消。
func Test面板里那张我的PIN表单(t *testing.T) {
	s, h := setupBatch(t)
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))

	// 面板上有这一格（第四颗 tab）。
	page := get(h, "/prefs?tab=pin", cookie).Body.String()
	// **action 带着 `?tab=pin`**：这一格只出现在「我的 PIN」那一节，而服务端答错时
	// 把面板连同那句提示一起重画、读的是请求上的 `?tab=`——不带它，错话就落回第一
	// 颗 tab（屏上是「外观」那一节顶上一句「现在的 PIN 不对」，而人刚待的地方没了）。
	for _, must := range []string{`action="/prefs/pin?tab=pin"`, `name="new"`, `name="old"`} {
		if !strings.Contains(page, must) {
			t.Errorf("面板上少了 %q", must)
		}
	}

	// 改：对的那把旧 PIN + 新的 → 盘上换掉了。
	rec := postForm(h, "/prefs/pin?tab=pin", "old=1234&new=4321", cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("改 PIN 该 303，实际 %d：%s", rec.Code, rec.Body)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPIN(p.PIN, "4321") {
		t.Error("新的 PIN 没生效")
	}

	// 旧 PIN 不对 → **不改盘上**，而且说得清是哪一样不对。
	rec = postForm(h, "/prefs/pin?tab=pin", "old=1234&new=5678", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("旧 PIN 不对该回面板（200），实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "现在的 PIN 不对") {
		t.Errorf("旧 PIN 不对时页面没有说清：%.300s", rec.Body)
	}
	// **而且要落回「我的 PIN」那一节**：服务端答错时把面板连同那句提示一起重画，
	// 而那一重画读的是请求上的 `?tab=`。不带它的话，错话会落回第一颗 tab——屏上
	// 是「外观」那一节顶上一句「现在的 PIN 不对」，而人刚待的地方没了。
	if !strings.Contains(rec.Body.String(), `id="prefs-tab-pin" checked`) {
		t.Errorf("旧 PIN 不对时页面没有落回「我的 PIN」那一节：%.300s", rec.Body)
	}
	p, _ = s.LoadProfile("dad")
	if !auth.VerifyPIN(p.PIN, "4321") {
		t.Error("旧 PIN 不对却把门改了")
	}

	// 新的格式错 → 也不改。
	rec = postForm(h, "/prefs/pin?tab=pin", "old=4321&new=1", cookie)
	if !strings.Contains(rec.Body.String(), "4 到 6 位数字") {
		t.Errorf("新 PIN 格式错时页面没有说清：%.300s", rec.Body)
	}
	p, _ = s.LoadProfile("dad")
	if !auth.VerifyPIN(p.PIN, "4321") {
		t.Error("新 PIN 格式错却把门改了")
	}

	// **留空 = 取消这道门**（"能设就能撤"的那一半）。
	rec = postForm(h, "/prefs/pin?tab=pin", "old=4321&new=", cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("取消这道门该 303，实际 %d：%s", rec.Code, rec.Body)
	}
	p, _ = s.LoadProfile("dad")
	if auth.HasPin(p.PIN) {
		t.Error("留空没有取消这道门")
	}
}

// 原本**不设防**的那一份，能不能在面板里给自己设上第一个 PIN。
//
// **能，而且那一格会自己变形**：这一份没有 PIN 时，面板上**只画「新的」那一格**
// ——「现在的」那一格不存在，因为没有什么可证明的（没有的东西不需要证明）。
// 于是本人第一次锁上自己那道门**不需要任何旧 PIN**，而这恰恰是「能设就能撤」
// 那一半的前提：不设防一旦选了还能关回去。
//
// 这一条钉的是**表单那条路**（API 那条在 nopass_test.go 里已经钉过）：两个壳
// 共用 selfPin 落点，判据只有一份，但**画出来的那一屏**是各画各的——那一屏少
// 画一格，人就会以为自己没资格改，而事实是他能改。
func Test面板里那张我的PIN表单_没设防的那份可以直接设上(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.CreateProfile(profileInputNoPin("tv", "客厅")); err != nil {
		t.Fatal(err)
	}
	cookie := sessionCookie(t, loginAs(t, h, "tv", ""))

	// 面板上**只有「新的」那一格**。
	page := get(h, "/prefs?tab=pin", cookie).Body.String()
	if strings.Contains(page, `name="old"`) {
		t.Errorf("这一份没有 PIN，面板上却问「现在的 PIN」——没有什么可证明的:\n%.400s", page)
	}
	if !strings.Contains(page, `name="new"`) {
		t.Errorf("面板上少了「新的 PIN」那一格:\n%.400s", page)
	}
	// **文案也跟着形状走**：这一份没有门，于是「改这道门」「取消这道门」那两句
	// 都是对着不存在的门说的。按钮该说「锁上」，代价该说「不锁上会怎样」。
	if !strings.Contains(page, "锁上这道门") {
		t.Errorf("这一份没有门，按钮该说「锁上这道门」:\n%.400s", page)
	}
	for _, wrong := range []string{"改这道门", "取消这道门"} {
		if strings.Contains(page, wrong) {
			t.Errorf("这一份没有门，页面上却还有 %q:\n%.400s", wrong, page)
		}
	}

	// **不带 old**，只给 new → 设上了。
	rec := postForm(h, "/prefs/pin?tab=pin", "new=4321", cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("没设防的那份该直接设上，实际 %d：%s", rec.Code, rec.Body)
	}
	p, err := s.LoadProfile("tv")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPIN(p.PIN, "4321") {
		t.Fatal("新的 PIN 没生效")
	}

	// 设上之后那一格变回**两格**：现在要证明「现在的」了。
	page = get(h, "/prefs?tab=pin", cookie).Body.String()
	if !strings.Contains(page, `name="old"`) {
		t.Errorf("这一份已经有 PIN 了，面板上却还只有一格:\n%.400s", page)
	}
	if !strings.Contains(page, "改这道门") {
		t.Errorf("这一份已经有 PIN 了，按钮该说「改这道门」:\n%.400s", page)
	}

	// 而它现在真的锁上了：随便一个 PIN 进不去（**拿不到会话 cookie** 才算数——
	// 状态码与提示都是长相，那张 cookie 才是门）。
	rec = postForm(h, "/pin", "profile=tv&pin=9999", "")
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=pin") {
		t.Errorf("设上之后随便一个 PIN 该进不去，实际 %d → %q", rec.Code, rec.Header().Get("Location"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieProfile {
			t.Errorf("设上之后随便一个 PIN 竟然拿到了会话 cookie（%s）——门没锁上", c.Value)
		}
	}
	// 对的那把进得去。
	rec = postForm(h, "/pin", "profile=tv&pin=4321", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("新的 PIN 该进得去，实际 %d → %q", rec.Code, rec.Header().Get("Location"))
	}
}

// 这一格**只认会话**：带令牌一律 303 回选择页（浏览器表单通路的纪律）。
func Test面板里那张我的PIN表单只认会话(t *testing.T) {
	s, h := setupBatch(t)
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	req := httptest.NewRequest("POST", "/prefs/pin", strings.NewReader("old=1234&new=4321"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "zh")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Authorization", "Bearer "+dadTok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("带令牌该 303，实际 %d", rec.Code)
	}
	p, _ := s.LoadProfile("dad")
	if !auth.VerifyPIN(p.PIN, "1234") {
		t.Error("带令牌的请求改了盘上的门")
	}
}
