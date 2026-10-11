package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 用 Cookie 请求头原文喂值：r.AddCookie 会把非法字节（例如 \）先删掉再发出，
// 被测代码根本收不到 "a\b"，那条断言就成了假的绿。浏览器/脚本能往头里写什么，
// 服务端就得按什么判。
func requestWith(pairs ...[2]string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	parts := make([]string, 0, len(pairs))
	for _, kv := range pairs {
		parts = append(parts, kv[0]+"="+kv[1])
	}
	r.Header.Set("Cookie", strings.Join(parts, "; "))
	return r
}

func TestEmptyWhenNoCookies(t *testing.T) {
	if s := ReadSession(httptest.NewRequest("GET", "/", nil)); !s.IsEmpty() {
		t.Fatalf("want empty, got %+v", s)
	}
	rec := httptest.NewRecorder()
	WriteSession(rec, "dad")
	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("want 2 cookies（会话 + 把旧那条作废）, got %d", len(cookies))
	}
	for _, c := range cookies {
		if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
			t.Fatalf("cookie 属性不全: %+v", c)
		}
		if c.Secure {
			t.Fatal("本站跑在局域网 http 上，设 Secure 浏览器会直接丢掉 cookie")
		}
	}
	// 会话那条带值、带有效期；**lb_home 那条必须是被作废的**（空值 + MaxAge<0），
	// 于是老浏览器登录一次就干净了。
	var profile, legacy *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case CookieProfile:
			profile = c
		case CookieHome:
			legacy = c
		}
	}
	if profile == nil || profile.Value != "dad" || profile.MaxAge != SessionCookieMaxAge {
		t.Fatalf("会话 cookie 不对: %+v", profile)
	}
	if legacy == nil || legacy.Value != "" || legacy.MaxAge >= 0 {
		t.Fatalf("升级前那条 lb_home 必须被作废（空值 + MaxAge<0）: %+v", legacy)
	}
}

// 写进去的只有一条，读回来的也只有一条。
func TestRoundTrip(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteSession(rec, "dad")
	r := httptest.NewRequest("GET", "/", nil)
	// 只把会话那一条喂回去——lb_home 已被作废，浏览器下一次就不会再发它。
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieProfile {
			r.AddCookie(c)
		}
	}
	got := ReadSession(r)
	if got.Active != "dad" || got.LegacyHome != "" || !got.Valid() {
		t.Fatalf("round trip 坏: %+v", got)
	}
}

// 三种 cookie 形状的判据（代管态 2026-10-10 下线之后的全部合法形状）。
//
// 旧会话**不被登出**是这里最要紧的一条：升级不该让人重新输一次 PIN，所以升级前
// 那对相等的 cookie 照旧有效。而两条不等的那枚必须进不来——cookie 自包含，读的人
// 只看到 lb_profile=kid，会当成"kid 自己登录"，那正是 10-10 要堵的后门。
func TestValidShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		sess Session
		want bool
		why  string
	}{
		{"今天的形状：只有 lb_profile", Session{Active: "mom"}, true, "唯一被发出的形状"},
		{"升级前的本人会话：两条相等", Session{Active: "mom", LegacyHome: "mom"}, true, "升级不许把人踢下线"},
		{"升级前的代管会话：两条不等", Session{Active: "mom", LegacyHome: "dad"}, false, "代管已下线，那种 cookie 是后门"},
		{"没有档案 id", Session{}, false, "空"},
		{"档案 id 不合法", Session{Active: ".."}, false, "slug 判据"},
	} {
		if got := tc.sess.Valid(); got != tc.want {
			t.Errorf("%s：Valid = %v，要 %v（%s）", tc.name, got, tc.want, tc.why)
		}
	}
}

// 上面那条判据要真的作用在**读出来的** cookie 上，而不是只对手构的 struct 成立：
// 这一条走完整的 Cookie 头。
func TestLegacyPairReadFromHeader(t *testing.T) {
	if s := ReadSession(requestWith([2]string{CookieProfile, "mom"})); !s.Valid() {
		t.Fatalf("只有 lb_profile 该有效: %+v", s)
	}
	if s := ReadSession(requestWith([2]string{CookieProfile, "mom"}, [2]string{CookieHome, "mom"})); !s.Valid() {
		t.Fatalf("升级前的本人会话不该被登出: %+v", s)
	}
	if s := ReadSession(requestWith([2]string{CookieProfile, "kid"}, [2]string{CookieHome, "dad"})); s.Valid() {
		t.Fatalf("升级前的代管 cookie 必须判非法: %+v", s)
	}
}

// lb_home 不参与权限，但它**参与形状判定**：一个非法值被 cookieOf 读成空串，
// 于是 LegacyHome 变成 ""（"没有旧 cookie"），而不是"有一条非法的旧 cookie"。
// 后果是 `lb_profile=dad; lb_home=..` 会被当成合法会话——这没问题（lb_home 从来
// 不给权限），但形状判据也不能因为"有一条非法的"而误伤本人会话。
func TestPathTraversalishCookieValuesRejected(t *testing.T) {
	for _, v := range []string{"", ".", "..", "a/b", "a\\b", "Mom", "nope!", "a%2Fb"} {
		if ReadSession(requestWith([2]string{CookieProfile, v})).Valid() {
			t.Fatalf("非法档案名 %q 被判有效", v)
		}
		// 两条都非法时同样无效；其中一条非法时以 lb_profile 为准（见上）。
		if ReadSession(requestWith([2]string{CookieProfile, v}, [2]string{CookieHome, v})).Valid() {
			t.Fatalf("非法档案名 %q（两条）被判有效", v)
		}
	}
}

func TestClearExpiresBothWithoutValue(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearSession(rec)
	n := 0
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge >= 0 || c.Value != "" {
			t.Fatalf("清除要用过期而非删值: %+v", c)
		}
		n++
	}
	// 两条都清：lb_home 今天已经不再被发出去，清它是为了让还留着旧 cookie 的
	// 浏览器在登出时顺手干净。
	if n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
}
