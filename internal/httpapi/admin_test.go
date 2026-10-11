package httpapi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
)

func TestStatusIsAdminOnlyAndAggregates(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/status", adminTok)
	if rec.Code != 200 {
		t.Fatalf("管理员令牌该放行, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Profiles []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			Admin         bool   `json:"admin"`
			Rev           int64  `json:"rev"`
			Items         int    `json:"items"`
			IconUnmatched int    `json:"iconUnmatched"`
			Snapshots     int    `json:"snapshots"`
			Broken        bool   `json:"broken"`
		} `json:"profiles"`
		BrokenProfiles []string `json:"brokenProfiles"`
		Totals         struct {
			Profiles      int `json:"profiles"`
			Items         int `json:"items"`
			IconUnmatched int `json:"iconUnmatched"`
			Snapshots     int `json:"snapshots"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Profiles) != 2 || out.Totals.Profiles != 2 {
		t.Fatalf("got %+v", out.Profiles)
	}
	if out.Totals.Items != 2 {
		t.Fatalf("条目合计不对: %+v", out.Totals)
	}
	if out.Profiles[0].ID != "dad" || out.Profiles[0].Snapshots != 1 || !out.Profiles[0].Admin {
		t.Fatalf("dad 的汇总不对: %+v", out.Profiles[0])
	}
	// 档案作用域令牌与匿名都要拒：这是跨档案聚合信息（spec §6.1）。
	if rec := getBearer(h, "/api/v1/status", dadTok); rec.Code != 403 {
		t.Fatalf("档案作用域令牌该 403, got %d", rec.Code)
	}
	if rec := get(h, "/api/v1/status", ""); rec.Code != 401 {
		t.Fatalf("匿名该 401, got %d", rec.Code)
	}
	// 管理员会话也放行（它是磁盘上确认过的管理员）。
	if rec := get(h, "/api/v1/status", "lb_profile=dad"); rec.Code != 200 {
		t.Fatalf("管理员会话该 200, got %d", rec.Code)
	}
}

// 坏档案不许从 /status 上消失：spec §12 要它汇总 brokenProfiles[]，
// 一个人的手误要看得见。
func TestStatusReportsBrokenProfiles(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.BookmarksPath("kid"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/status", adminTok)
	var out struct {
		Profiles       []map[string]any `json:"profiles"`
		BrokenProfiles []string         `json:"brokenProfiles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.BrokenProfiles) != 1 || out.BrokenProfiles[0] != "kid" {
		t.Fatalf("brokenProfiles 不对: %+v", out.BrokenProfiles)
	}
	for _, p := range out.Profiles {
		if p["id"] == "kid" && p["broken"] != true {
			t.Fatalf("坏档案那条该标 broken=true: %+v", p)
		}
	}
	// 坏档案仍然在列表里（不是被剔除），rev 报 0。
	if len(out.Profiles) != 2 {
		t.Fatalf("坏档案被剔了: %+v", out.Profiles)
	}
}

// profile.json 本身坏掉的档案，ListProfiles 会直接 continue 掉它——只遍历
// ListProfiles 的 /status 会让它整行消失，违反 spec §12「坏档案仍要在列表里
// 占一行，消失比报 0 更坏」。行集必须并上 Store.Broken()。
func TestStatusKeepsRowForCorruptProfileJSON(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.ProfilePath("kid"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/status", adminTok)
	var out struct {
		Profiles       []map[string]any `json:"profiles"`
		BrokenProfiles []string         `json:"brokenProfiles"`
		Totals         struct {
			Profiles int `json:"profiles"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	// kid 没有从聚合里消失。
	var kid map[string]any
	for _, p := range out.Profiles {
		if p["id"] == "kid" {
			kid = p
		}
	}
	if kid == nil {
		t.Fatalf("profile.json 坏的档案没在 /status 占一行: %+v", out.Profiles)
	}
	if kid["broken"] != true {
		t.Fatalf("坏 profile.json 那条该 broken=true: %+v", kid)
	}
	// 不编造 title：读不出来就是空串。
	if title, ok := kid["title"].(string); !ok || title != "" {
		t.Fatalf("坏档案的 title 该是空串而非编造: %#v", kid["title"])
	}
	// brokenProfiles[] 与行集一致。
	found := false
	for _, id := range out.BrokenProfiles {
		if id == "kid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("kid 该进 brokenProfiles: %+v", out.BrokenProfiles)
	}
	if out.Totals.Profiles != 2 {
		t.Fatalf("totals.profiles 该把坏档案算进去: %d", out.Totals.Profiles)
	}
}

// iconUnmatched 是读时算的差集（spec §5）：只数"看着像图标名而索引里没有"
// 与"auto 且缓存无图"，空 icon 不算缺。这一条同时钉两件事——新字段真的
// 被解得出来（旧用例的 struct 若不补字段就证不到它），以及数字当场跟着
// 缓存变：记成事件的实现在最后一步仍会端出 2。
func TestStatus_缺图数读时算(t *testing.T) {
	srv, h := setupServer(t)
	s := srv.deps.Store
	fc := srv.deps.Cache // 服务器手里那面镜像：Put 必须走它，另 Load 一个是第二个对象（裁定 R3）
	// 四条：好名、陌生名、auto 无缓存、空 icon。
	if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 1, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList, Accent: "#4c8dff",
		Items: []model.Item{
			{ID: "ok", Name: "好", URL: "http://10.0.0.1/", Icon: "tabler:home", Alias: []string{}, Tags: []string{}},
			{ID: "ghost", Name: "怪", URL: "http://10.0.0.2/", Icon: "tabler:nothere", Alias: []string{}, Tags: []string{}},
			{ID: "fav", Name: "图", URL: "http://10.0.0.3:8096/", Icon: "auto", Alias: []string{}, Tags: []string{}},
			{ID: "bare", Name: "空", URL: "http://10.0.0.4/", Alias: []string{}, Tags: []string{}},
		},
	}}}); err != nil {
		t.Fatal(err)
	}

	dadRow := func(t *testing.T) (int, int) {
		t.Helper()
		rec := getBearer(h, "/api/v1/status", adminTok)
		if rec.Code != 200 {
			t.Fatalf("status 得到 %d: %s", rec.Code, rec.Body)
		}
		var out struct {
			Profiles []struct {
				ID            string `json:"id"`
				IconUnmatched int    `json:"iconUnmatched"`
			} `json:"profiles"`
			Totals struct {
				IconUnmatched int `json:"iconUnmatched"`
			} `json:"totals"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		for _, pr := range out.Profiles {
			if pr.ID == "dad" {
				return pr.IconUnmatched, out.Totals.IconUnmatched
			}
		}
		t.Fatalf("dad 没在响应里: %+v", out.Profiles)
		return 0, 0
	}

	row, tot := dadRow(t)
	// 陌生名 + auto 无缓存 = 2；空 icon 不算缺（spec §5 的口径）。
	if row != 2 || tot != 2 {
		t.Fatalf("iconUnmatched = 行 %d 合计 %d，要 2/2", row, tot)
	}

	// 关键一步：给那条 auto 的 origin 放进缓存，数字必须当场跟着降。
	// 这就是"读时算"与"记成事件"的分水岭——记成事件的话这里还是 2。
	if _, err := fc.Put(icons.Origin("http://10.0.0.3:8096/"), []byte("图"), "png"); err != nil {
		t.Fatal(err)
	}
	if row, tot := dadRow(t); row != 1 || tot != 1 {
		t.Errorf("缓存里已经有图了，iconUnmatched 还是 行 %d 合计 %d：数字没跟着缓存变", row, tot)
	}
}

func TestListProfilesHasMetaOnly(t *testing.T) {
	_, h := setupBatch(t)
	rec := getBearer(h, "/api/v1/profiles", adminTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	// 零条目、零 URL、零分组名：列表端点不是书签的第二个读口。
	body := rec.Body.String()
	for _, banned := range []string{"10.0.0.3", "群晖", "NAS", "dsm"} {
		if strings.Contains(body, banned) {
			t.Fatalf("档案列表漏出了书签内容 %q: %s", banned, body)
		}
	}
}

func TestDeleteProfileEndpoint(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatal(err)
	}
	rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=kid", "Bearer "+adminTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if _, err := s.LoadProfile("kid"); err == nil {
		t.Fatal("档案还在")
	}
	// 再删一次该 404，不是"幂等成功"。
	if rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=kid", "Bearer "+adminTok); rec.Code != 404 {
		t.Fatalf("重复删档该 404, got %d", rec.Code)
	}
	// 缺 profile / 不是 slug。
	if rec := getWithMethod(h, "DELETE", "/api/v1/profiles", "Bearer "+adminTok); rec.Code != 400 {
		t.Fatalf("缺 profile 该 400, got %d", rec.Code)
	}
	if rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=..", "Bearer "+adminTok); rec.Code != 400 {
		t.Fatalf("非 slug 该 400, got %d", rec.Code)
	}
	// 档案作用域令牌删档该 403（不是 404：先鉴权后查存在）。
	if rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=dad", "Bearer "+dadTok); rec.Code != 403 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}
