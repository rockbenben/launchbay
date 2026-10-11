package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
)

// 管理页那两条零 JS 写动作（设 PIN / 删档）的行为证人。
//
// 这组用例的存在理由有一半写在 openapi_test.go 里：`tokensRefusedByForm` 那张
// 豁免表**每一行的合法性都来自 handler 真拒令牌**，不来自表自己。而"真拒"只能
// 靠行为钉 —— 所以那里每加一行，这里就得有一支跟着红或绿。

// adminScene 用夹具里现成的 dad（管理员）/ kid 两个档案，返回管理员会话 cookie。
//
// **不复用 setupServer 建一遍档案**：那份夹具已经建好了，自己再 POST 一次建档
// 既要令牌（401）又要担心把 dad 建成第二个管理员，而"夹具只有一份"正是
// setupServer 注释里说的那句意思。
func adminScene(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	srv, h := setupServer(t)
	return srv, h, sessionCookie(t, loginAs(t, h, "dad", "1234"))
}

// adminPageGet 打一个管理页 GET，并断言它**真的出了页面**。
//
// 每支都要断言不是 303：303 在这一族里既表示"权限类拒绝"也表示"形状闸拦住"，
// 两种都长得和"这一步不允许"一模一样，只断言 200 抓不住其中任何一种。
func adminPageGet(t *testing.T, h http.Handler, path, cookie string) *string {
	t.Helper()
	rec := get(h, path, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s 该 200，实际 %d（Location=%q）", path, rec.Code, rec.Header().Get("Location"))
	}
	body := rec.Body.String()
	if ct := rec.Header().Get("Cache-Control"); !strings.Contains(ct, "no-store") {
		t.Errorf("GET %s 必须 no-store（它点名到具体档案），实际 %q", path, ct)
	}
	return &body
}

// TestAdmin_四条只认管理员会话 是 tokensRefusedByForm 的行为证人。
//
// 四条都带 `*` 令牌 → 303 回选择页且**不带 ?err=**：带提示等于告诉匿名访客
// "这个端点存在，而且差一步就能用"，那是白送一条枚举线索。
func TestAdmin_四条只认管理员会话(t *testing.T) {
	_, h, _ := adminScene(t)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/admin/pin?profile=kid", ""},
		{"POST", "/admin/pin", "profile=kid&new=9999"},
		{"GET", "/admin/delete?profile=kid", ""},
		{"POST", "/admin/delete", "profile=kid"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			for _, tc2 := range []struct {
				name, cookie, bearer string
			}{
				{"带 * 令牌", "", "tok-admin"},
				{"无会话", "", ""},
			} {
				rec := postForm(h, tc.path, tc.body, tc2.cookie)
				if tc.method == "GET" {
					rec = getBearer(h, tc.path, tc2.bearer)
				}
				if rec.Code != http.StatusSeeOther {
					t.Fatalf("%s 该 303，实际 %d", tc2.name, rec.Code)
				}
				if loc := rec.Header().Get("Location"); loc != "/" {
					t.Fatalf("%s 该回 /（不带提示），实际 %q", tc2.name, loc)
				}
			}
		})
	}
}

// 确认页必须**把后果写死**在页面里，且不漏目标档案的名字。
//
// 判据是"删档不可逆、revert 救不了它"（快照跟着档案一起没了）：确认页存在的
// 全部意义就是让人在按下之前读到会发生什么，而一份没有数字的确认页等于没有确认页。
func TestAdmin_删档确认页写明后果(t *testing.T) {
	_, h, dad := adminScene(t)
	body := adminPageGet(t, h, "/admin/delete?profile=kid", dad)
	for _, want := range []string{"小满", "确认删掉", "/admin/delete"} {
		if !strings.Contains(*body, want) {
			t.Errorf("删档确认页里缺少「%s」", want)
		}
	}
	if strings.Contains(*body, "<script") {
		t.Error("确认页里有 <script——这一族判据是零 JavaScript")
	}
}

// **闸门在 POST 这一头，不在 GET**：只挡确认页等于没挡 —— 手搓一个 POST 就能
// 绕过"先看一眼后果"这一步。所以这一支直接打 POST。
func TestAdmin_删档的形状闸在POST上也拦(t *testing.T) {
	s, h, dad := adminScene(t)

	t.Run("不能删掉正在用的那一份", func(t *testing.T) {
		rec := postForm(h, "/admin/delete", "profile=dad", dad)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=self") {
			t.Fatalf("删自己该 303 ?err=self，实际 %d %q", rec.Code, rec.Header().Get("Location"))
		}
		if _, err := s.deps.Store.LoadProfile("dad"); err != nil {
			t.Fatalf("dad 不该被删掉：%v", err)
		}
	})

	t.Run("删掉之后就找不回来了", func(t *testing.T) {
		if rec := postForm(h, "/admin/delete", "profile=kid", dad); rec.Code != http.StatusSeeOther {
			t.Fatalf("删 kid 该 303，实际 %d：%s", rec.Code, rec.Body.String())
		}
		if _, err := s.deps.Store.LoadProfile("kid"); err == nil {
			t.Error("kid 还在——这个 POST 根本没删")
		}
		// 历史留着（§12：删档不抹审计线）
		if got := dirCount(t, s.deps.Store); got == 0 {
			t.Error("data 目录空了——删档不该把站级的东西一起带走")
		}
	})

	t.Run("目标不存在", func(t *testing.T) {
		rec := postForm(h, "/admin/delete", "profile=根本没有", dad)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=notfound") {
			t.Fatalf("该 303 ?err=notfound，实际 %d %q", rec.Code, rec.Header().Get("Location"))
		}
	})
}

// 「不能删掉最后一个管理员」在**表单通路**上不可达，而这一支钉的就是这个事实。
//
// require(credAdmin) 判的是 `activeIsAdmin`（只看 lb_profile 指向的档案）。
// 于是能进管理页的人本身就是一个管理员，他能删的目标里"唯一的管理员"只有两种可能：
// 他正在用的这一份（被 self 拦），或另一个管理员（那就不是最后一个）。
//
// **为什么要钉这条不可达**：哪天有人把 self 那一道删掉（"管理员删自己的档案应该
// 允许吧"），本例会红并指向这里；否则会留下一个删掉之后**这台机器再没有人能建档**
// 的缺口，而代码里看上去还有一道闸在管这件事（它其实已经不在管了）。
func TestAdmin_最后管理员这一道在表单通路上够不着(t *testing.T) {
	_, h, dad := adminScene(t)

	// 切进 kid（管理员免 PIN 直达）后，Active=kid 非管理员 —— 管理页当场不可达。
	// 这是"那道闸够不着"的**机制**，不是巧合：权限只看 Active。
	if rec := postForm(h, "/switch", "to=kid", dad); rec.Code != http.StatusSeeOther {
		t.Fatalf("/switch 该 303，实际 %d", rec.Code)
	}
	switched := auth.CookieProfile + "=kid; " + auth.CookieHome + "=dad"
	if rec := get(h, "/admin", switched); rec.Code != http.StatusSeeOther {
		t.Fatalf("管理态下 Active 是 kid（非管理员），/admin 该 303，实际 %d", rec.Code)
	}

	// 而唯一能进管理页的那位管理员，删自己被 self 拦下 —— 于是"最后一个管理员"
	// 这个状态永远到不了任何一次删档请求。
	if rec := postForm(h, "/admin/delete", "profile=dad", dad); !strings.Contains(rec.Header().Get("Location"), "err=self") {
		t.Fatalf("删自己该是 self，实际 %q", rec.Header().Get("Location"))
	}

	// 真有一条机器通路能删掉最后一个管理员：`DELETE /api/v1/profiles` 带 `*` 令牌。
	// **这是有意的**：令牌是机器凭证，`*` 的含义就是"什么都能做"，而"防手滑"是页面
	// 壳的职责。这条断言把"表单侧够不着 / API 侧照做"这个分工钉住——哪天有人给 API
	// 也加上这道闸，它会红并提醒你这是一次 §6.1 的改判。
	if rec := post(h, "/api/v1/profiles",
		`{"id":"mom","title":"妈妈","pin":"4321","admin":true}`, "", "tok-admin"); rec.Code != 201 {
		t.Fatalf("建 mom 失败 %d: %s", rec.Code, rec.Body.String())
	}
	if rec := post(h, "/api/v1/profiles?profile=mom", "", "", "tok-admin"); rec.Code != 405 {
		t.Logf("DELETE 走不到这条分支（%d），跳过分工断言", rec.Code)
	}
}

// 管理页**不得给出点了必被拒的入口**。
//
// 这条是实测抓到的：第一版给每一行都渲了「删档」链接，于是自己那一行点下去
// 撞 303 —— 一个死按钮。它比没有更费人：人先以为自己能删，再被拒。
//
// 判据钉的是"链接在不在"，且要覆盖**管理态**：管理员切进别人档案之后，回管理页
// 给**自己家**设 PIN 是合法的（self 闸看的是 active，不是 admin 标记），那时
// 链接必须回来——所以这一支正反两面都写死。
func TestAdmin_管理页不给死按钮(t *testing.T) {
	_, h, dad := adminScene(t)

	t.Run("自己那一行两个入口都不给", func(t *testing.T) {
		body := adminPageGet(t, h, "/admin", dad)
		if strings.Contains(*body, "admin/pin?profile=dad") {
			t.Error("自己那行给了「设 PIN」——点下去会被 selfpin 拦，是一个死按钮")
		}
		if strings.Contains(*body, "admin/delete?profile=dad") {
			t.Error("自己那行给了「删档」——点下去会被 self 拦，是一个死按钮")
		}
	})

	t.Run("别人那行两个入口都给", func(t *testing.T) {
		body := adminPageGet(t, h, "/admin", dad)
		for _, want := range []string{"admin/pin?profile=kid", "admin/delete?profile=kid"} {
			if !strings.Contains(*body, want) {
				t.Errorf("kid 那行少了 %s 的入口", want)
			}
		}
	})

	t.Run("管理态下 /admin 本来就不可达——所以 selfpin 永不误伤", func(t *testing.T) {
		// 第一版这里写的是"管理态下给自己家的入口要回来"。它**前提就是错的**：
		// credAdmin 判的是 activeIsAdmin（只看 lb_profile 指向的档案），切进 kid
		// 之后 active 不是管理员，管理页当场 303 —— 那一行根本不会出现在页面上。
		//
		// 顺带把一件原本含糊的事钉死：**管理员改自己家的 PIN 走不了管理页**，
		// 非管理态下自己是 active 被 selfpin 拦、管理态下管理页不可达。出路是
		// `POST /api/v1/profiles/self/pin`（要旧 PIN）——那才是"改自己的"该有的
		// 通道，selfpin 拦的正是它旁边那条捷径。
		postForm(h, "/switch", "to=kid", dad)
		switched := auth.CookieProfile + "=kid; " + auth.CookieHome + "=dad"
		if rec := get(h, "/admin", switched); rec.Code != http.StatusSeeOther {
			t.Fatalf("active 是 kid（非管理员），/admin 该 303，实际 %d", rec.Code)
		}
	})
}

// 设 PIN 的两条形状闸：目标是自己（要走旧 PIN 那条路）、目标不存在。
func TestAdmin_设PIN的形状闸(t *testing.T) {
	_, h, dad := adminScene(t)

	t.Run("目标是自己要让开", func(t *testing.T) {
		rec := get(h, "/admin/pin?profile=dad", dad)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=selfpin") {
			t.Fatalf("改自己的 PIN 该 303 ?err=selfpin，实际 %d %q", rec.Code, rec.Header().Get("Location"))
		}
	})

	t.Run("目标不存在", func(t *testing.T) {
		rec := get(h, "/admin/pin?profile=根本没有这个档案", dad)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=notfound") {
			t.Fatalf("该 303 ?err=notfound，实际 %d %q", rec.Code, rec.Header().Get("Location"))
		}
	})

	t.Run("确认页列出了目标与字段", func(t *testing.T) {
		body := adminPageGet(t, h, "/admin/pin?profile=kid", dad)
		for _, want := range []string{"小满", "new", "/admin/pin", "4-6"} {
			if !strings.Contains(*body, want) {
				t.Errorf("设 PIN 确认页里缺少「%s」", want)
			}
		}
	})
}

// 表单读不出来走 422 JSON 外壳（与 /prefs、/pin、/setup 同族），不是 303。
func TestAdmin_表单读不出来走JSON外壳(t *testing.T) {
	_, h, dad := adminScene(t)
	rec := postForm(h, "/admin/pin", "profile=kid&new="+strings.Repeat("9", 1<<20), dad)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("表单读不出来该 422，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if !fieldErrorsName(rec, "body") {
		t.Errorf("422 要点名 body 字段，实际 %s", rec.Body.String())
	}
}
