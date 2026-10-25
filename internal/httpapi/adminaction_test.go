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
//
// **密码是 2026-10-23 新加的那一道**（用户拍板「删除需要有确认警示，并要求密码」）：
// POST 必须带那把机器级密钥，于是下面每一个 POST 都带 `key=`。
func TestAdmin_删档的形状闸在POST上也拦(t *testing.T) {
	s, h, dad := adminScene(t)

	t.Run("不能删掉正在用的那一份", func(t *testing.T) {
		rec := postForm(h, "/admin/delete", "profile=dad&key="+testAdminKey, dad)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=self") {
			t.Fatalf("删自己该 303 ?err=self，实际 %d %q", rec.Code, rec.Header().Get("Location"))
		}
		if _, err := s.deps.Store.LoadProfile("dad"); err != nil {
			t.Fatalf("dad 不该被删掉：%v", err)
		}
	})

	t.Run("删掉之后就找不回来了", func(t *testing.T) {
		if rec := postForm(h, "/admin/delete", "profile=kid&key="+testAdminKey, dad); rec.Code != http.StatusSeeOther {
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
		rec := postForm(h, "/admin/delete", "profile=根本没有&key="+testAdminKey, dad)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=notfound") {
			t.Fatalf("该 303 ?err=notfound，实际 %d %q", rec.Code, rec.Header().Get("Location"))
		}
	})
}

// **删档要密码**（2026-10-23，用户拍板「删除需要有确认警示，并要求密码」）。
//
// 这一条是"删档不可逆"的对策：管理 cookie 是那把机器级 key 的派生值（它进
// devtools、进导出、进浏览器同步），而删档是这台机器上唯一找不回来的动作。
// 于是页面这一条要**再输一次那把 key**。
//
// 两件事要分清：**没填**与**填错**是同一件事（"你得把那把钥匙拿出来"），
// 所以一把键 badkey 就够；而**密码不对时档案一个字都不动**——这是这条闸
// 的全部意义（密码是唯一挡在"手滑"前面的东西，它要真的挡得住）。
func TestAdmin_删档要密码(t *testing.T) {
	s, h, dad := adminScene(t)

	t.Run("确认页上确实有那一格", func(t *testing.T) {
		// 密码这件事要**看得见**：确认页上得有那一格（type=password），
		// 否则人永远不知道要填什么。
		//
		// 这一支排**最前**：后面的三支会真把 kid 删掉，而确认页要的是一个
		// 还在的档案（不在的话答 303 notfound，那不是这一支要验的东西）。
		body := adminPageGet(t, h, "/admin/delete?profile=kid", dad)
		if !strings.Contains(*body, `name="key"`) || !strings.Contains(*body, `type="password"`) {
			t.Errorf("确认页上该有「管理密钥」那一格（name=key、type=password）:\n%.400s", *body)
		}
		// **软键盘要是文字的**：那把 key 是一段任意文本，而 PIN 那一格要的是
		// 数字键盘。模板原先把 inputmode 写死成 numeric，于是这一格在手机上
		// 弹出来的是数字键盘——用户自己换的那把 key 只要有字母就输不进去。
		if !strings.Contains(*body, `inputmode="text"`) {
			t.Errorf("管理密钥那一格的 inputmode 该是 text:\n%.400s", *body)
		}
		if strings.Contains(*body, `inputmode="numeric"`) {
			t.Errorf("管理密钥那一格不该要数字键盘（那把 key 可以有字母）:\n%.400s", *body)
		}
	})

	t.Run("没填密码", func(t *testing.T) {
		rec := postForm(h, "/admin/delete", "profile=kid", dad)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=badkey") {
			t.Fatalf("没填密码该 303 ?err=badkey，实际 %d %q", rec.Code, rec.Header().Get("Location"))
		}
		if _, err := s.deps.Store.LoadProfile("kid"); err != nil {
			t.Fatalf("kid 不该被删掉：%v", err)
		}
	})

	t.Run("密码不对", func(t *testing.T) {
		rec := postForm(h, "/admin/delete", "profile=kid&key=不对的那把", dad)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=badkey") {
			t.Fatalf("密码不对该 303 ?err=badkey，实际 %d %q", rec.Code, rec.Header().Get("Location"))
		}
		if _, err := s.deps.Store.LoadProfile("kid"); err != nil {
			t.Fatalf("kid 不该被删掉：%v", err)
		}
	})

	t.Run("密码对了才删", func(t *testing.T) {
		rec := postForm(h, "/admin/delete", "profile=kid&key="+testAdminKey, dad)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("密码对了该 303，实际 %d %s", rec.Code, rec.Body.String())
		}
		if _, err := s.deps.Store.LoadProfile("kid"); err == nil {
			t.Error("kid 还在——密码对了却没删")
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
		if rec := postForm(h, "/admin/delete", "profile="+id+"&key="+testAdminKey, admin); rec.Code != http.StatusSeeOther {
			t.Fatalf("删 %s 该 303，实际 %d %s", id, rec.Code, rec.Body)
		}
	}
	// dad 是"我正在用的那一份"，删它要先把那份会话放下——这正是 self 那道闸。
	if rec := postForm(h, "/admin/delete", "profile=dad&key="+testAdminKey, admin); !strings.Contains(rec.Header().Get("Location"), "err=self") {
		t.Fatalf("删自己该 self，实际 %q", rec.Header().Get("Location"))
	}
	// 放下会话之后再删：这一次没有任何档案了。
	noProfile := adminOnlyCookie(t)
	if rec := postForm(h, "/admin/delete", "profile=dad&key="+testAdminKey, noProfile); rec.Code != http.StatusSeeOther {
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
//
// 2026-10-20 改判：**加/改 PIN 这一枚对每一份档案都在**（含自己正在用的那一
// 份）——管理员是机器级的一把 key，不属于任何档案，那道"别动你正在用的那一
// 份"的闸在 PIN 这一族上没有依据（本人自己那边 self/pin 照样要旧 PIN）。
// 仍然不给的是**删档与停用**：那两件是会话层面的事，点了必然被 self 拦。
func TestAdmin_管理页不给死按钮(t *testing.T) {
	_, h, dad := adminScene(t)

	t.Run("自己那一行：改 PIN 给得出，删档与停用不给", func(t *testing.T) {
		body := adminPageGet(t, h, "/admin", dad)
		if !strings.Contains(*body, "admin/pin?profile=dad") {
			t.Error("自己那行没有「改 PIN」——管理员是机器级的一把 key，改自己那一份的 PIN 是合法的")
		}
		if strings.Contains(*body, "admin/delete?profile=dad") {
			t.Error("自己那行给了「删档」——点下去会被 self 拦，是一个死按钮")
		}
		if strings.Contains(*body, "admin/disable?profile=dad") {
			t.Error("自己那行给了「停用」——点下去会被 selfdisable 拦，是一个死按钮")
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
			t.Error("管理员登在 kid 那一份时，自己家 dad 那行该给得出「改 PIN」")
		}
		if !strings.Contains(*body, "admin/delete?profile=dad") {
			t.Error("同样该给得出「删档」——他此刻没在用 dad")
		}
		// 而他正在用的 kid 那一行：改 PIN 仍然给得出（2026-10-20 改判），
		// 但删档与停用必须不给（self / selfdisable 会拦）。
		if !strings.Contains(*body, "admin/pin?profile=kid") {
			t.Error("kid 那行该给得出「改 PIN」——管理员是机器级的 key，与档不挂钩")
		}
		if strings.Contains(*body, "admin/delete?profile=kid") {
			t.Error("kid 那行给了「删档」——点下去会被 self 拦，是死按钮")
		}
	})
}

// 加 PIN 与改 PIN 是**两件事**，页面要分开说。
//
// 判据是这一份现在有没有 PIN（auth.HasPin，与 statusRows 同一份）：没锁上的
// 那一份，"加"的后果是"从谁都能进变成要输数字"；锁着的那一份，"改"的后果是
// "旧 PIN 当场作废、留空 = 取消"。同一句模板套两种情形，读的人得自己猜。
func TestAdmin_设PIN页分清加与改(t *testing.T) {
	_, h, dad := adminScene(t)

	t.Run("没设过 PIN 的那一份：说「加」", func(t *testing.T) {
		// 先用管理员那条路把 kid 的 PIN 撤掉（留空 = 取消），于是它变成
		// "没锁上"的那一种——夹具里 dad 与 kid 一开始都有 PIN。
		if rec := postForm(h, "/admin/pin", "profile=kid&new=", dad); rec.Code != http.StatusSeeOther {
			t.Fatalf("撤掉 kid 的 PIN 该 303，实际 %d %s", rec.Code, rec.Body)
		}
		body := adminPageGet(t, h, "/admin/pin?profile=kid", dad)
		for _, want := range []string{"加 PIN", "加一道 PIN"} {
			if !strings.Contains(*body, want) {
				t.Errorf("加 PIN 的确认页里缺少「%s」", want)
			}
		}
		if strings.Contains(*body, "改 PIN") {
			t.Error("kid 已经没设 PIN 了，页面上却说的是「改」")
		}
	})

	t.Run("设过 PIN 的那一份：说「改」，并说清旧 PIN 当场作废", func(t *testing.T) {
		body := adminPageGet(t, h, "/admin/pin?profile=dad", dad)
		for _, want := range []string{"改 PIN", "当场作废"} {
			if !strings.Contains(*body, want) {
				t.Errorf("改 PIN 的确认页里缺少「%s」", want)
			}
		}
		if strings.Contains(*body, "加一道 PIN") {
			t.Error("dad 已经设过 PIN，页面上却说的是「加」")
		}
	})
}

// 改自己正在用的那一份的 PIN，**不再让开**（2026-10-20 改判）。
//
// 从前这一支 303 回 `?err=selfpin`，理由是"改自己的 PIN 要旧 PIN"。可管理员是
// 机器级的一把 key，不属于任何档案——它不在任何一个"自己"里，于是那条理由
// 在管理页上从来没有依据：本人自己那边（self/pin）照样要旧 PIN，而管理员这
// 一边是"家人忘了 PIN 的出路"，它必须能管**每一份**档案。
//
// 同一件事的机器通路（POST /api/v1/profiles/pin）本来就只要求管理员身份、
// 从来不拦"正在用的那一份"；管理页比它更严，两处对同一件事给出不同答案。
func TestAdmin_改当前在用的那一份的PIN不再被拦(t *testing.T) {
	_, h, dad := adminScene(t)
	rec := get(h, "/admin/pin?profile=dad", dad)
	if rec.Code != http.StatusOK {
		t.Fatalf("改自己那一份的 PIN 该 200（页面出来了），实际 %d %q",
			rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), "admin/pin") {
		t.Error("这一页该有提交到 /admin/pin 的表单")
	}
}

// 设 PIN 的形状闸：目标不存在。（"目标是自己"那一支 2026-10-20 改判成
// **不再拦**，单独一支用例钉着，见 TestAdmin_改当前在用的那一份的PIN不再被拦。）
func TestAdmin_设PIN的形状闸(t *testing.T) {
	_, h, dad := adminScene(t)

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
