package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// 图标选择器那一格（2026-10-14）：`GET /api/v1/icons` 的界面那一半。
//
// 这几条钉的是**它与 JSON 那条的关系**：同一个 q、同一份索引、同一套裁定，而吐的是
// 画好的 HTML。少任何一条的症状都不响——脚本那条照常能用，而界面那一格空白、
// 或悄悄吐出六千格。

func getTiles(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

func TestIconTiles_公开可查且画出方格(t *testing.T) {
	_, h := setup(t)
	rec := getTiles(t, h, "/api/v1/icons/tiles?q=docker") // 无凭证：与 JSON 那条同族

	if rec.Code != http.StatusOK {
		t.Fatalf("匿名查图标要 200，得到 %d：%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("这一条吐的是 HTML 方格，Content-Type 要是 text/html，得到 %q", ct)
	}
	// 这段要被插进页面里，缺 nosniff 时浏览器照样 sniff——所以它是承重的。
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("缺 X-Content-Type-Options: nosniff：这一段是要被插进页面里的文本")
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-icon="simple-icons:docker"`) {
		t.Errorf("方格里没有图标名：%s", body)
	}
	if !strings.Contains(body, "<svg") {
		t.Errorf("方格里没有内联 SVG（要与卡片上那枚同源）：%s", body)
	}
	// 与 JSON 那条**逐条相等**：同一个 q 不许在两条端点上给出两批图标。
	code, jsonBody := getJSON(t, h, "/api/v1/icons?q=docker&limit=24")
	if code != http.StatusOK {
		t.Fatalf("JSON 那条要 200，得到 %d", code)
	}
	want := jsonBody["icons"].([]any)
	if n := strings.Count(body, "<button"); n != len(want) {
		t.Errorf("同一个 q=%s：JSON 那条给 %d 个名字，方格那一端画了 %d 格", "docker", len(want), n)
	}
}

func TestIconTiles_总数与画出来的条数分开报(t *testing.T) {
	_, h := setup(t)
	// 搜一个宽词：命中远多于画出来的。截断**必须说出来**（§4.3 那条"零命中"与
	// "命中很多被截断"要能分开判），否则界面会说"就这几个"。
	rec := getTiles(t, h, "/api/v1/icons/tiles?q=a&limit=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d", rec.Code)
	}
	total, err := strconv.Atoi(rec.Header().Get("X-LB-Icons-Total"))
	if err != nil {
		t.Fatalf("缺 X-LB-Icons-Total（或不是整数）：界面据此说「共 N 个，先看前 M 个」：%q", rec.Header().Get("X-LB-Icons-Total"))
	}
	if total <= 5 {
		t.Errorf("夹具不成立：q=a 的命中要超过 5，实际 %d（这条测的就是截断现场）", total)
	}
	if n := strings.Count(rec.Body.String(), "<button"); n != 5 {
		t.Errorf("limit=5 却画了 %d 格", n)
	}
}

func TestIconTiles_零命中200空体_缺q与坏limit400(t *testing.T) {
	_, h := setup(t)
	if rec := getTiles(t, h, "/api/v1/icons/tiles?q=zzzzz-zzz"); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("零命中是 200 且正文为空（界面自己说「没有叫这个名字的」），得到 %d / %q", rec.Code, rec.Body.String())
	}
	for _, tc := range []struct{ url, why string }{
		{"/api/v1/icons/tiles", "缺 q"},
		{"/api/v1/icons/tiles?q=docker&limit=abc", "limit 非数字"},
		{"/api/v1/icons/tiles?q=docker&limit=0", "limit 为 0"},
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
	// 上限要真的钳住（它给界面看的那一档比给 AI 的那条小）
	rec := getTiles(t, h, "/api/v1/icons/tiles?q=a&limit=99999")
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d", rec.Code)
	}
	if n := strings.Count(rec.Body.String(), "<button"); n > iconTilesMaxLimit {
		t.Errorf("画了 %d 格，超过上限 %d", n, iconTilesMaxLimit)
	}
}
