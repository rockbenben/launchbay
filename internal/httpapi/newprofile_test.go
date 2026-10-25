package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
)

// newProfileID 从 201 的响应体里取档案 id。
//
// **解 JSON 而不是找字面量**：响应体是缩进过的（store.WriteJSON），找
// `"id":"home"` 这种串会永远找不到——那不是行为坏了，是断言的取法不对。
func newProfileID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("201 的响应体解不出来: %v（%s）", err, rec.Body.String())
	}
	return out.ID
}

// 自建档（2026-10-23，用户拍板：「非管理员用户有个 new 接口，可以自己建档案」）。
//
// 这一族的判据分三块，各有各的证人：
//   - **免凭证**：不带任何凭证也建得出来（§11 不防局域网内冒充，而建出来的那
//     一份是空的、只属于它自己的 PIN）；
//   - **id 由名字派生、不接受申报**：不许自己起 id——否则谁都能建一份 id 叫
//     `dad` 的档案顶替别人的那一格；
//   - **不发会话**：免凭证建档不等于免凭证进入，建完照样要输 PIN 才进门。
func Test自建档_任何人都不需要凭证(t *testing.T) {
	s, h := setup(t)

	rec := post(h, "/api/v1/profiles/new", `{"title":"客厅电视","pin":"4321"}`, "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("自建档该 201，实际 %d：%s", rec.Code, rec.Body.String())
	}
	// 中文名 slug 不出来 → 落到 home 基名序列（store.NextProfileID）。
	if got := newProfileID(t, rec); got != "home" {
		t.Errorf("中文名的 id 该由基名序列派生，得到 %q", got)
	}
	// 档案真的在盘上，而且**是空的**（没有任何预置内容）。
	if _, err := s.LoadProfile("home"); err != nil {
		t.Fatalf("档案没建出来：%v", err)
	}
	snap, err := s.Current("home")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(snap.B.Groups); n != 0 {
		t.Errorf("新档案该是空的，得到 %d 个分组", n)
	}
	// **不发会话**：建完照样要输 PIN 才进门（与 /setup 同一条纪律）。
	if c := rec.Header().Get("Set-Cookie"); strings.Contains(c, auth.CookieProfile) {
		t.Errorf("自建档不该发会话 cookie，实际 %q", c)
	}
}

func Test自建档_id由名字派生且不接受申报(t *testing.T) {
	s, h := setup(t)

	// 拉丁名折成 slug（"Tv" → "tv"），而 dad 已占着 "dad"。
	rec := post(h, "/api/v1/profiles/new", `{"title":"Tv","pin":"4321"}`, "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("自建档该 201，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if got := newProfileID(t, rec); got != "tv" {
		t.Errorf("拉丁名的 id 该折成 slug，得到 %q", got)
	}

	// **不接受 id 申报**：送来是 422 且点名该字段（decodeBody 拒未知键）。
	// 这一格是这一族与管理员那条最大的差别——不许顶替别人的 id。
	rec = post(h, "/api/v1/profiles/new", `{"id":"dad","title":"冒充","pin":"4321"}`, "", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("送来 id 该 422，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "id") {
		t.Errorf("422 该点名 id 那一格，得到：%s", rec.Body.String())
	}
	// dad 还在（那一次请求没写成任何东西）。
	if _, err := s.LoadProfile("dad"); err != nil {
		t.Fatalf("dad 不该被动：%v", err)
	}
}

func Test自建档_字段错点名字那一格(t *testing.T) {
	_, h := setup(t)

	for _, tc := range []struct {
		name, body, field string
	}{
		{"名字空", `{"title":"","pin":"4321"}`, "title"},
		{"PIN 格式不对", `{"title":"小满","pin":"12"}`, "pin"},
		{"PIN 与不设 PIN 同给", `{"title":"小满","pin":"4321","nopass":true}`, "nopass"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(h, "/api/v1/profiles/new", tc.body, "", "")
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("该 422，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.field) {
				t.Errorf("422 该点名 %s，得到：%s", tc.field, rec.Body.String())
			}
		})
	}
}

// **档案名的长度上限**（2026-10-23 加）：上限是 `model.MaxProfileTitleRunes`，
// 与三张建档表单上那个 maxlength 同一个数。
//
// 为什么写侧要判：那个 maxlength 只有浏览器认，curl 与扩展不认——而这一条
// 是**免凭证**的，于是局域网里任何设备都能写进一个几 KB 的名字，而选单页
// （**全家人每次打开都要拉的那一页**）从此每次都带着它。实测：一个 5000 字的
// 标题让那一页从 4.8 万字节涨到 5.3 万。
func Test自建档_名字太长要拒(t *testing.T) {
	s, h := setup(t)

	long := strings.Repeat("长", model.MaxProfileTitleRunes+1)
	rec := post(h, "/api/v1/profiles/new", `{"title":"`+long+`","pin":"4321"}`, "", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("超长名字要 422，实际 %d：%s", rec.Code, rec.Body)
	}
	if !fieldErrorsName(rec, "title") {
		t.Errorf("422 要点名 title：%s", rec.Body)
	}
	// **盘上多不出任何一份档案**（被拒的请求不许留半成品）。
	if n, _ := s.CountProfiles(); n != 2 {
		t.Errorf("盘上的档案数该仍是夹具那 2 份，得到 %d", n)
	}
	// 刚好到上限的收得下（判据是"超过"，不是"达到"）。
	ok := strings.Repeat("长", model.MaxProfileTitleRunes)
	rec = post(h, "/api/v1/profiles/new", `{"title":"`+ok+`","pin":"4321"}`, "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("刚好到上限该收下，实际 %d：%s", rec.Code, rec.Body)
	}
}

// 浏览器那条：选单页上那张表单的落点。
//
// 与 /setup 同一张表单、同一套判据，差别只有"随时可建"这一条——所以它**不是**
// 零档案时的一次性通路：机器上已经有 dad 与 kid 了，它照样建得出来。
func Test自建档_浏览器那条(t *testing.T) {
	s, h := setup(t)

	rec := postForm(h, "/new", "title=客厅电视&pin=4321", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("建档该 303，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("成功该回选单页 /，实际 %q", loc)
	}
	if _, err := s.LoadProfile("home"); err != nil {
		t.Fatalf("档案没建出来：%v", err)
	}
	// 选单页上要**看得见**那张新卡（它是自己刚建的那一份）。
	page := get(h, "/", "")
	if !strings.Contains(page.Body.String(), "客厅电视") {
		t.Errorf("选单页上该有那张新卡:\n%.400s", page.Body.String())
	}
	// 字段没填对 → 303 带 ?err= 白名单键（与 /setup 同一张词表）；名字带回、
	// 并且把折着的「新建自己的一份」展开（2026-10-24）。
	rec = postForm(h, "/new", "title=客厅电视&pin=12", "")
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=badpin") {
		t.Fatalf("PIN 格式不对该 303 ?err=badpin，实际 %d %q", rec.Code, loc)
	}
	if !strings.Contains(loc, "new=1") || !strings.Contains(loc, "title=") {
		t.Errorf("/new 的错误回跳要带 new=1 与 title=：%q", loc)
	}
	page = get(h, loc, "")
	if !strings.Contains(page.Body.String(), `class="pick-new" open`) {
		t.Errorf("/new 的错误回跳没有展开折叠的表单:\n%.500s", page.Body.String())
	}
	if !strings.Contains(page.Body.String(), "客厅电视") {
		t.Errorf("/new 的错误回跳没有把名字填回去:\n%.500s", page.Body.String())
	}
}

// **删档要那把 key 本身**（2026-10-23，用户拍板「删除需要有确认警示，并要求密码」）。
//
// 这一条是这一族最容易被当成"多此一举"的地方，所以钉三件事：
//   - 只带管理 cookie → 403（凭证认出来了，但这一档要的不是它）；
//   - 什么都不带 → 401（没带凭证）；
//   - 带上那把 key → 200（真的删掉了）。
//
// 401 与 403 不许合并：合并成 403 会让"忘带 key"的人以为自己没登录。
func Test删档_只带管理cookie不够(t *testing.T) {
	s, h := setup(t)

	rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=kid", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("什么都不带该 401，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatalf("kid 不该被删掉：%v", err)
	}

	// 只带管理 cookie：它是 key 的派生值，而删档要 key 本身。
	rec = getWithMethod(h, "DELETE", "/api/v1/profiles?profile=kid", adminOnlyCookie(t))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("只带管理 cookie 该 403，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatalf("kid 不该被删掉：%v", err)
	}

	// 带上那把 key 才成。
	rec = getWithMethod(h, "DELETE", "/api/v1/profiles?profile=kid", "Bearer "+adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("带那把 key 该 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if _, err := s.LoadProfile("kid"); err == nil {
		t.Error("kid 还在——带 key 的那一次该真删掉")
	}
}
