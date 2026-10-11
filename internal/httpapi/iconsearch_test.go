package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func getJSON(t *testing.T, h http.Handler, url string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s 的响应不是 JSON: %v / %s", url, err, rec.Body)
		}
	}
	return rec.Code, body
}

func TestIconsSearch_公开可查(t *testing.T) {
	_, h := setup(t) // 既有的夹具装配
	code, body := getJSON(t, h, "/api/v1/icons?q=docker")
	if code != http.StatusOK {
		t.Fatalf("匿名查图标要 200（spec §6.1 把它列在公开三件里），得到 %d", code)
	}
	if body["query"] != "docker" {
		t.Errorf("query 不回显原词: %v", body["query"])
	}
	total, _ := body["total"].(float64)
	returned, _ := body["returned"].(float64)
	if total < 1 {
		t.Fatal("simple-icons:docker 必须命中")
	}
	if int(returned) != len(body["icons"].([]any)) {
		t.Errorf("returned=%v 与实际条数不符：这两个数分开报才有意义", body["returned"])
	}
}

func TestIconsSearch_参数分流(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct{ url, want string }{
		{"/api/v1/icons", "缺 q"},
		{"/api/v1/icons?q=docker&prefix=mdi", "prefix 非法"},
		{"/api/v1/icons?q=docker&limit=abc", "limit 非数字"},
		{"/api/v1/icons?q=docker&limit=-1", "limit 负数"},
	} {
		code, body := getJSON(t, h, tc.url)
		if code != http.StatusBadRequest {
			t.Errorf("%s（%s）要 400，得到 %d", tc.url, tc.want, code)
		}
		if body["detail"] == nil || body["detail"] == "" {
			t.Errorf("%s 的 400 要说人话: %v", tc.url, body)
		}
	}
}

func TestIconsSearch_limit上限掐得住(t *testing.T) {
	_, h := setup(t)
	code, body := getJSON(t, h, "/api/v1/icons?q=a&limit=99999")
	if code != http.StatusOK {
		t.Fatalf("得到 %d", code)
	}
	got := len(body["icons"].([]any))
	if got > iconsSearchMaxLimit {
		t.Errorf("返回 %d 条，超过上限 %d", got, iconsSearchMaxLimit)
	}
	// 夹具本身要成立：q=a 必须匹配到超过上限，否则这条测的根本不是"截断"
	// （真实产物上 total=6161，2026-10 实测）。公开可查那条用 q=docker，
	// 真实 total=2 ≤ limit，returned 写成 total 它也看不出来——被截断的
	// 现场里 returned 与 total 才分得开。
	total, _ := body["total"].(float64)
	if int(total) <= iconsSearchMaxLimit {
		t.Fatalf("夹具失效：q=a 的 total=%v 该超过上限 %d，没东西可截", body["total"], iconsSearchMaxLimit)
	}
	returned, _ := body["returned"].(float64)
	if int(returned) != got {
		t.Errorf("returned=%v 与实际条数 %d 不符：截断现场里这两个数更不许混", returned, got)
	}
}

func TestIconsSearch_零命中不写null(t *testing.T) {
	_, h := setup(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/icons?q=zzzzz-zzz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("零命中是 200 不是 404：它说的是「索引里没有」，不是「端点坏了」")
	}
	if !strings.Contains(rec.Body.String(), `"icons": []`) {
		t.Errorf("§4.3 空数组必须写 []，得到 %s", rec.Body)
	}
}

// 整键回查的三条已裁定行为由 icons.Search 定死，端点必须原样透出、不许在
// handler 里再判一遍：这三条线各钉一种"端点自己改写了搜索语义"的走样。
func TestIconsSearch_透出搜索层的三条裁定(t *testing.T) {
	_, h := setup(t)

	// ① 整键命中只回那一条（tabler:box 命中介键，不退子串的 111 条）。
	code, body := getJSON(t, h, "/api/v1/icons?q=tabler:box")
	if code != http.StatusOK {
		t.Fatalf("得到 %d", code)
	}
	if total, _ := body["total"].(float64); total != 1 {
		t.Errorf("整键命中 total 要是 1（只回那一条），得到 %v", body["total"])
	}

	// ② 整键被 prefix 排除时返回空——不退回去搜别的层。
	code, body = getJSON(t, h, "/api/v1/icons?q=tabler:box&prefix=simple-icons")
	if code != http.StatusOK {
		t.Fatalf("得到 %d", code)
	}
	if total, _ := body["total"].(float64); total != 0 {
		t.Errorf("被 prefix 排除的整键要 0 条（不退搜别的层），得到 %v / %s", body["total"], body["icons"])
	}

	// ③ 整键命不中才退子串：tabler:docker 在真实产物上是 2 条。
	code, body = getJSON(t, h, "/api/v1/icons?q=tabler:docker")
	if code != http.StatusOK {
		t.Fatalf("得到 %d", code)
	}
	if total, _ := body["total"].(float64); total != 2 {
		t.Errorf("tabler:docker 整键命不中，退子串要 2 条，得到 %v", body["total"])
	}
}

// 写路径闸门必须真的通电：陌生图标名在 HTTP 通路上同样被 422 拒掉、
// 整批不落盘；而真实索引里的名字必须放行——正/反两个方向都测，才分得开
// "闸门通了"与"闸门把一切都拒了"（比如误把 typed-nil 索引当白名单传进去）。
func TestBatch图标闸门已接真索引(t *testing.T) {
	s, h := setupBatch(t)
	badBody := `{"profile":"dad","by":"ai","ops":[
	  {"op":"add","path":"groups/nas/items","value":{"id":"weird","name":"陌生图标条目","url":"http://10.0.0.4/","icon":"tabler:whatever-not-in-index"}}]}`
	// 用作用域为 dad 的令牌：`*` 过不了 credScoped（2026-10-10）。
	rec := post(h, "/api/v1/batch", badBody, "", dadTok)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("陌生图标名该被通电的闸门 422 拒掉，得到 %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Errors []struct {
			OpsIndex *int   `json:"opsIndex"`
			Field    string `json:"field"`
			Reason   string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) != 1 || out.Errors[0].Field != "icon" {
		t.Fatalf("要点名 icon 字段: %s", rec.Body)
	}
	if out.Errors[0].OpsIndex == nil || *out.Errors[0].OpsIndex != 0 {
		t.Fatalf("opsIndex 要指到出错的那条 op: %s", rec.Body)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 1 || len(b.Groups[0].Items) != 1 {
		t.Fatalf("被拒的批次什么都不许留下: rev=%d items=%d", b.Rev, len(b.Groups[0].Items))
	}
	if list, err := s.ListSnapshots("dad"); err != nil || len(list) != 0 {
		t.Fatalf("被拒的批次不许做快照: err=%v list=%+v", err, list)
	}

	// 正向对照：真实索引里的名字走同一条通路必须放行。
	goodBody := `{"profile":"dad","by":"ai","ops":[
	  {"op":"add","path":"groups/nas/items","value":{"id":"box","name":"箱子","url":"http://10.0.0.5/","icon":"tabler:box"}}]}`
	rec = post(h, "/api/v1/batch", goodBody, "", dadTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("真索引里的名字必须放行（闸门不许一刀切）: %d %s", rec.Code, rec.Body)
	}
	if b, err := s.LoadBookmarks("dad"); err != nil || b.Rev != 2 {
		t.Fatalf("对照批次要真的写入: rev=%+v err=%v", b, err)
	}
}
