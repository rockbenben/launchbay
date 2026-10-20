package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// 图标选择器的**浏览**那一档（2026-10-20）。
//
// 这一族存在的理由是"这一台机器里到底有哪些图标、随便挑一个"这条路从前是断的：
// 面板只有"搜一个词"与"这一份已经在用的"两批，而那一万个图标是**内嵌在二进制里**
// 的（spec §5）——它不是"别人的东西"，是这台机器自带的。
//
// 下面钉的是**三件事**：翻页的位置对不对、总数报得对不对、翻不动的时候老实说。

// 公开可查，且画得出方格。
func TestIconBrowse_公开可查且画出方格(t *testing.T) {
	_, h := setup(t)
	rec := getTiles(t, h, "/api/v1/icons/browse") // 无凭证

	if rec.Code != http.StatusOK {
		t.Fatalf("匿名翻图库要 200，得到 %d：%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("这一条吐的是 HTML 方格，Content-Type 要是 text/html，得到 %q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("缺 X-Content-Type-Options: nosniff：这一段是要被插进页面里的文本")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Errorf("方格里没有内联 SVG（要与卡片上那枚同源）：%s", body)
	}
	if n := strings.Count(body, "<button"); n < 12 {
		t.Errorf("只画了 %d 格，太少了：翻图库这一档要一次给一屏", n)
	}
}

// **总数报的是全库的条数**（不是这一页的），界面要靠它说"图库里有 N 个"。
func TestIconBrowse_总数报的是全库条数(t *testing.T) {
	_, h := setup(t)
	rec := getTiles(t, h, "/api/v1/icons/browse?limit=10")
	total, err := strconv.Atoi(rec.Header().Get("X-LB-Icons-Library"))
	if err != nil {
		t.Fatalf("缺 X-LB-Icons-Library（或不是整数）：界面要靠它说「图库里有 N 个」：%q",
			rec.Header().Get("X-LB-Icons-Library"))
	}
	if total < 1000 {
		t.Errorf("全库只有 %d 条？内嵌的是两套索引（品牌层 + 通用层），条数是万的量级", total)
	}
	if n := strings.Count(rec.Body.String(), "<button"); n != 10 {
		t.Errorf("limit=10 却画了 %d 格", n)
	}
}

// 翻页的位置就是顺序里的位置：offset=5 的第一格，必须是 offset=0 那一页的**第 6 格**
// （下标 5）——也就是两页**接得上**，而不是各翻各的。
func TestIconBrowse_翻页的位置是顺序里的位置(t *testing.T) {
	_, h := setup(t)
	names := func(rec *httptest.ResponseRecorder) []string {
		var out []string
		// 切完第一段是 data-icon 之前的那些 HTML，不是名字——从第二段起算。
		for _, part := range strings.Split(rec.Body.String(), `data-icon="`)[1:] {
			if i := strings.Index(part, `"`); i > 0 {
				out = append(out, part[:i])
			}
		}
		return out
	}

	first := names(getTiles(t, h, "/api/v1/icons/browse?limit=6"))
	second := names(getTiles(t, h, "/api/v1/icons/browse?offset=5&limit=1"))
	if len(first) < 6 || len(second) < 1 {
		t.Fatalf("两页都要有东西：%d / %d", len(first), len(second))
	}
	if first[5] != second[0] {
		t.Errorf("翻页错位了：offset=0 的第 6 格是 %q，offset=5 的第 1 格是 %q",
			first[5], second[0])
	}
	// 同一枚图标不该在两页里各出现一次。
	a := names(getTiles(t, h, "/api/v1/icons/browse?limit=5"))
	b := names(getTiles(t, h, "/api/v1/icons/browse?offset=5&limit=5"))
	seen := map[string]bool{}
	for _, n := range append(append([]string{}, a...), b...) {
		if seen[n] {
			t.Errorf("%q 在两页里各出现了一次", n)
		}
		seen[n] = true
	}
}

// 形状不对要老实说 400，不许悄悄当成"从 0 开始"。
func TestIconBrowse_形状不对回400(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct{ url, why string }{
		{"/api/v1/icons/browse?offset=-1", "offset 是负数"},
		{"/api/v1/icons/browse?offset=abc", "offset 不是数字"},
		{"/api/v1/icons/browse?limit=0", "limit 是 0"},
		{"/api/v1/icons/browse?limit=abc", "limit 不是数字"},
	} {
		rec := getTiles(t, h, tc.url)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s 要 400，得到 %d", tc.why, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), `"kind"`) {
			t.Errorf("%s 的 400 要带机器可读的 kind：%s", tc.why, rec.Body)
		}
	}
	// 超上限要钳住，不该报错（那是界面给的一格，不是脚本的一次越界）。
	rec := getTiles(t, h, "/api/v1/icons/browse?limit=99999")
	if rec.Code != http.StatusOK {
		t.Fatalf("limit 超上限要钳住而不是 400，得到 %d", rec.Code)
	}
	if n := strings.Count(rec.Body.String(), "<button"); n > 120 {
		t.Errorf("画了 %d 格，超过上限 120", n)
	}
}
