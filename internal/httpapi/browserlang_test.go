package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 语言的完整链条（2026-10-20 改判后的形状）：
//
//	这一份自己记的 → 站级的默认（**管理员设过**的）→ **浏览器语言** → en
//
// 第三格只在"**什么都还没设过**"时才会被问到——那时"默认中文"那句并不成立
// （谁都没说过要中文），而**浏览器说的那一门**是人正在读的那一门，最像他。
//
// 这一族钉的是第三格与第四格（第一格与第二格各有自己的用例）。
func Test语言_什么都没设过时按浏览器说的那一门(t *testing.T) {
	_, h := setupZero(t) // 一台还没建过档、还没人设过语言的机器

	for _, tc := range []struct {
		name     string
		header   string
		wantLang string
	}{
		{"浏览器说中文", "zh-CN,zh;q=0.9", "zh-CN"}, // 中文那一门的 <html lang> 是 zh-CN（RFC 5646）
		{"浏览器说英文", "en-US,en;q=0.9", "en"},
		// 认不出来的浏览器语言 → **en**（国际通用那一门），不是中文。
		{"浏览器说我们没有的那一门", "fr-FR,fr;q=0.9", "en"},
		// 浏览器什么都没说（脚本、CLI）→ 同样是 en。
		{"浏览器什么都没说", "", "en"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tc.header != "" {
				req.Header.Set("Accept-Language", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("首屏该 200，得到 %d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, `<html lang="`+tc.wantLang+`"`) {
				t.Errorf("这一页该是 %s 的（<html lang>），实际：\n%.300s", tc.wantLang, body)
			}
		})
	}
}

// 管理员**设过**默认之后，浏览器说的那一门就不算数了。
//
// 这一格是"管理员的语言是默认"那句话的落点：默认一旦有人说过，就是他说过了，
// 后面的格子（浏览器、en）轮不到。
func Test语言_管理员设过默认之后浏览器说的不算数(t *testing.T) {
	s, h := setupZero(t)
	if err := s.SetLanguage(i18n.Zh); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `<html lang="zh-CN"`) {
		t.Errorf("管理员把默认设成了中文，浏览器的英文就不该接管这一页：\n%.300s", body)
	}
}

// 这一份自己记了语言，它压过站级的默认，也压过浏览器说的那一门。
func Test语言_自己记的那一份压过默认与浏览器(t *testing.T) {
	s, h := setupBatch(t)
	// 管理员把默认设成中文
	if err := s.SetLanguage(i18n.Zh); err != nil {
		t.Fatal(err)
	}
	// 而 kid 自己记了英文——走**本人那一条真实通路**（台面面板那一格）。
	kid := sessionCookie(t, loginAs(t, h, "kid", "5678"))
	if rec := postForm(h, "/prefs", "language=en", kid); rec.Code != http.StatusSeeOther {
		t.Fatalf("改自己那一份的语言该 303，得到 %d", rec.Code)
	}
	// 浏览器的头说中文，而 kid 自己说的是英文
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "zh")
	req.Header.Set("Cookie", kid)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `<html lang="en"`) {
		t.Errorf("kid 自己记了英文，页面就该是英文的：\n%.300s", body)
	}
}
