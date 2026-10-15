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

// adminScene 用夹具里现成的 dad / kid 两个档案，返回**管理员 + 登在 dad 那一份**的
// cookie 串。
//
// 两条 cookie 都要（2026-10-12）：管理员与档案会话是两件独立的事，而这一族测试钉的
// 恰好是**两者同时在场**时的行为——selfpin / deleteBlocked 那道"别动你正在用的那一份"
// 只有在管理员**自己也在用某一份**时才拦得住。只给 lb_admin 的话，那两道闸在所有
// 用例里都会形同虚设，而它们正是这一族最要紧的两条保护。
//
// **不复用 setupServer 建一遍档案**：那份夹具已经建好了，自己再 POST 一次建档
// 既要令牌（401）又要担心多出一份档案，而"夹具只有 dad 与 kid"正是
// setupServer 注释里说的那句意思。
func adminScene(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	srv, h := setupServer(t)
	sess := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	return srv, h, sess + "; " + adminOnlyCookie(t)
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

// 「这台机器再没有人能建档」这个缺口**整条不存在了**（2026-10-12）。
//
// 早先这里钉的是一个不可达：管理员是某个档案，所以"最后一个管理员"要靠 self
// 那道闸才够不着，而 API 那条机器通路是故意放行的——于是"删光所有人之后这台机器
// 没人能管"是一个真实存在的状态，只是被设计成"人的手滑碰不到"。
//
// 现在管理员是 docker-compose.yml 里那把 key，与档案无关，于是**删多少份档案都
// 不影响这台机器还能不能管**。这一条钉的就是新形状：把 dad 与 kid 都删掉之后，
// 管理员 cookie 仍然打得开管理页。
func TestAdmin_删光档案也不影响这台机器还能管(t *testing.T) {
	srv, h, admin := adminScene(t)
	for _, id := range []string{"kid"} {
		if rec := postForm(h, "/admin/delete", "profile="+id, admin); rec.Code != http.StatusSeeOther {
			t.Fatalf("删 %s 该 303，实际 %d %s", id, rec.Code, rec.Body)
		}
	}
	// dad 是"我正在用的那一份"，删它要先把那份会话放下——这正是 self 那道闸。
	if rec := postForm(h, "/admin/delete", "profile=dad", admin); !strings.Contains(rec.Header().Get("Location"), "err=self") {
		t.Fatalf("删自己该 self，实际 %q", rec.Header().Get("Location"))
	}
	// 放下会话之后再删：这一次没有任何档案了。
	noProfile := adminOnlyCookie(t)
	if rec := postForm(h, "/admin/delete", "profile=dad", noProfile); rec.Code != http.StatusSeeOther {
		t.Fatalf("没登任何档案时删 dad 该 303，实际 %d %s", rec.Code, rec.Body)
	}
	if n, _ := srv.deps.Store.CountProfiles(); n != 0 {
		t.Fatalf("磁盘上还剩 %d 份档案，这一族的前提是「全删光」", n)
	}
	// **零档案，管理员仍然进得去**——这就是 2026-10-12 那次改判的整个理由。
	body := adminPageGet(t, h, "/admin", noProfile)
	if !strings.Contains(*body, "zero") && !strings.Contains(*body, "管理") {
		t.Errorf("零档案时管理页该照常渲染:\n%.300s", *body)
	}
}

// 管理页**不得给出点了必被拒的入口**。
//
// 这条是实测抓到的：第一版给每一行都渲了「删档」链接，于是自己那一行点下去
// 撞 303 —— 一个死按钮。它比没有更费人：人先以为自己能删，再被拒。
//
// 判据钉的是"链接在不在"，且要覆盖**管理员自己登在别人那一份**：那时他不是
// 任何人正在用的那一份，给自己家设 PIN 是合法的，链接必须回来——正反两面都写死。
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

	t.Run("管理员登在别人那一份时，自己家的入口要回来", func(t *testing.T) {
		// 早先这里写的是"管理态下 /admin 不可达，所以 selfpin 永不误伤"，而它的
		// **前提就是错的**：credAdmin 当年判的是"active 那个档案是不是管理员"，
		// 于是切进 kid 之后管理页当场 303。
		//
		// 2026-10-12 之后管理员与档案会话分开，于是这一格**变得可达**了：
		// 管理员可以正登着 kid 的书签，同时手里还有那把 key。回到管理页给
		// 自己家（dad）设 PIN 是合法的——他不是在改自己的密码。
		postForm(h, "/switch", "to=kid", dad)
		// 切档只动 lb_profile，lb_admin 原样留着——这正是"两件独立的事"。
		asKid := auth.CookieProfile + "=kid; " + authOnly(t)
		body := adminPageGet(t, h, "/admin", asKid)
		if !strings.Contains(*body, "admin/pin?profile=dad") {
			t.Error("管理员登在 kid 那一份时，自己家 dad 那行该给得出「设 PIN」")
		}
		if !strings.Contains(*body, "admin/delete?profile=dad") {
			t.Error("同样该给得出「删档」——他此刻没在用 dad")
		}
		// 而他正在用的 kid 那一行，两个入口都必须不给（selfpin / self）。
		if strings.Contains(*body, "admin/pin?profile=kid") {
			t.Error("kid 那行给了「设 PIN」——点下去会被 selfpin 拦，是死按钮")
		}
		if strings.Contains(*body, "admin/delete?profile=kid") {
			t.Error("kid 那行给了「删档」——点下去会被 self 拦，是死按钮")
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
		for _, want := range []string{"小满", "new", "/admin/pin", "4 到 6"} {
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
