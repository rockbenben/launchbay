package httpapi

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// 「PIN 可以不设」这个承诺的**整条通路**（2026-10-12）。
//
// 这一族用例刻意集中在一个文件里，而不是各自散进建档、进门、改 PIN 各自的
// 测试文件：那三段里任何一段单独绿都不代表这件事成立，而它成立与否只取决于
// 三段接起来——建档写下一份空 PIN、选择页不再要输、进门不核对。
//
// 它挡的是那个**最贵的形状**：忘了敲一位 PIN 就回车，静悄悄地建出一份局域网里
// 谁都能进的书签页。所以"忘了"与"不要"必须能分开——这就是 `nopass` 那一格
// 存在的全部理由，也是 `Test建档族_忘了敲PIN仍被拒` 存在的理由。

// profileInputNoPin 造一份**不设防**的建档输入。夹具里 dad/kid 都带 PIN，
// 而这里要的是第三种形状；走 store 而不是 API 是为了不与被测的那条读入路径
// 搅在一起（谁是谁的证人要看得出来）。
func profileInputNoPin(id, title string) store.NewProfileInput {
	return store.NewProfileInput{ID: id, Title: title}
}

// hashOf 是夹具里造一份真哈希的小工具：PinHash 这一格写 `"sha256$a$b"` 也能过
// profile.json 的形状，但那样造出来的档案**谁也进不去**——而这几条用例里有
// "用 PIN 进门"那几步。
func hashOf(t *testing.T, pin string) string {
	t.Helper()
	h, err := auth.HashPIN(pin)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func assertNoPinOnDisk(t *testing.T, s *store.Store, id string) {
	t.Helper()
	p, err := s.LoadProfile(id)
	if err != nil {
		t.Fatalf("%s 读不出来: %v", id, err)
	}
	if p.PIN != "" {
		t.Fatalf("%s 要的是不设防，盘上却是 %q", id, p.PIN)
	}
}

// profileAbsent 钉"没有建出来"。不钉它的话，那些"要 422/要 303 带提示"的用例
// 可能在**建成了**的情况下也绿——而建成了正是它们要拦的事。
func profileAbsent(t *testing.T, s *store.Store, id string) {
	t.Helper()
	if _, err := os.Stat(s.ProfileDir(id)); err == nil {
		t.Errorf("盘上不该有 %s——忘了敲 PIN 竟然建成了", id)
	}
}

// cardOf 从选择页 HTML 里切出**某一份档案那一张卡片**（<li>…</li>）。
//
// 为什么要切：一张卡上同时有名字、PIN 框与按钮，断言"没有 pin 输入框"时如果
// 对整页做 Contains，隔壁那张卡的框会被算进来——那样的用例恒绿，什么都没钉。
func cardOf(t *testing.T, page, title string) string {
	t.Helper()
	i := strings.Index(page, title)
	if i < 0 {
		t.Fatalf("选择页上找不到 %q 这一张:\n%.800s", title, page)
	}
	// 往前找最近一个 <li，往后找配对的 </li>。选择页的卡片是一层 <li>，
	// 卡片内部不再有 <li>（分组列表在档案页那边），所以第一个 </li> 就是收尾。
	start := strings.LastIndex(page[:i], "<li")
	end := strings.Index(page[i:], "</li>")
	if start < 0 || end < 0 {
		t.Fatalf("页面结构对不上，找不到 %q 那张卡的边界", title)
	}
	return page[start : i+end+len("</li>")]
}

// 建档那三条壳（API、/setup 表单、/admin 表单）都要能建出**不设防**的一份。
// 三条分别建三份，因为它们是三条独立的读入路径，共用同一个落点却各有各的
// 读法（JSON 里的 `nopass` 布尔 vs 表单里的 `nopass=1`）——漏掉任何一条，
// 那一族上这个特性就等于没有。
func Test建档族_三张壳都能建出不设防的档案(t *testing.T) {
	t.Run("API", func(t *testing.T) {
		s, h := setup(t)
		rec := post(h, "/api/v1/profiles", `{"id":"tv","title":"客厅","nopass":true}`, "", testAdminKey)
		if rec.Code != http.StatusCreated {
			t.Fatalf("want 201, got %d %s", rec.Code, rec.Body)
		}
		assertNoPinOnDisk(t, s, "tv")
	})

	// /setup 是**零档案**时的一次性通路（判据是盘上 profiles/ 的目录数），
	// 所以这一格必须用 setupZero 而不是通用夹具——夹具里已经有 dad/kid 了，
	// 在那份盘上打 /setup 只会撞上"窗口已关"，验证不到 nopass 有没有被读。
	t.Run("/setup 表单", func(t *testing.T) {
		s, h := setupZero(t)
		rec := postForm(h, "/setup", "title=客厅&nopass=1", "")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("want 303, got %d %s", rec.Code, rec.Body)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Fatalf("成功要 303 回 /，得到 %q", loc)
		}
		assertNoPinOnDisk(t, s, "home") // 中文名 → home 基名
	})

	t.Run("/admin 表单", func(t *testing.T) {
		s, h := setup(t)
		rec := postForm(h, "/admin", "title=客厅&nopass=1", adminOnlyCookie(t))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("want 303, got %d %s", rec.Code, rec.Body)
		}
		if loc := rec.Header().Get("Location"); strings.Contains(loc, "err=") {
			t.Fatalf("成功不该带 ?err=，得到 %q", loc)
		}
		assertNoPinOnDisk(t, s, "home")
	})
}

// 反面那一半：**忘了敲**仍然被拒。
//
// 这一格是整个特性的支点。如果"空 pin"与"不设防"用同一个形状表达（也就是让
// `pin:""` 表示不设防），它必然是绿的——而产品就回到了"手滑即公开"。它必须
// 红着，直到有人**故意**删掉它并同时写下新的设计。
func Test建档族_忘了敲PIN仍被拒(t *testing.T) {
	t.Run("API 点名字段", func(t *testing.T) {
		s, h := setup(t)
		rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈"}`, "", testAdminKey)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("忘了敲 PIN 要 422，得到 %d %s", rec.Code, rec.Body)
		}
		if !fieldErrorsName(rec, "pin") {
			t.Errorf("422 要点名 pin 字段（脚本作者据此知道该补哪一格），实际 %s", rec.Body)
		}
		profileAbsent(t, s, "mom")
	})

	t.Run("/admin 表单回提示", func(t *testing.T) {
		s, h := setup(t)
		rec := postForm(h, "/admin", "title=妈妈", adminOnlyCookie(t))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("want 303, got %d %s", rec.Code, rec.Body)
		}
		if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=") {
			t.Fatalf("空 PIN 提交要带提示回去，得到 %q", loc)
		}
		profileAbsent(t, s, "home")
	})
}

// 选择页上，不设防的那一份**不给 PIN 输入框，也不多说一句**。
//
// 2026-10-12 改判：这一格原先是"不给框，但写一句「不设 PIN，点一下就进」"，
// 理由是门口的空框等于骗人。现在**名字回到卡上了**（区分档案靠它），卡上已经
// 有名字与服务图标，再加一行说明就成了说明书——而且那句话的代价是每张不设防的
// 卡都比旁边高一截，一排卡片底边参差。代价已经在**建档那一步**说清楚了
// （勾选框下面那句与 README），进门时不必说第二遍。
//
// 卡片仍然是**自包含的**：那张卡上除了名字与图标就只有「进入」这一枚按钮，
// 按下去就进来了。
func Test选择页_不设防的那份不给输入框(t *testing.T) {
	s, h := setup(t)
	if _, err := s.CreateProfile(profileInputNoPin("tv", "客厅那台")); err != nil {
		t.Fatal(err)
	}
	body := get(h, "/", "").Body.String()
	card := cardOf(t, body, "客厅那台")
	if strings.Contains(card, `name="pin"`) {
		t.Errorf("不设防的那份不该有 PIN 输入框:\n%.600s", card)
	}
	// 不多解释：卡上不许出现「不设 PIN」那句。
	if strings.Contains(card, "不设 PIN") {
		t.Errorf("卡上不该再解释一遍（建档那一步已经说过）:\n%.600s", card)
	}
	// 名字与「进入」还在——它仍然是能认出来、能按下去的一张卡。
	if !strings.Contains(card, "客厅那台") {
		t.Errorf("卡上要看得见档案名:\n%.600s", card)
	}
	if !strings.Contains(card, "进入") {
		t.Errorf("不设防的那份也要能按「进入」:\n%.600s", card)
	}
	// 隔壁那份有 PIN 的仍然要有框——判据必须是"这一份有没有 PIN"，不是"这台
	// 机器上有没有 PIN"。这一格挡的是判据写错一层（NeedsPIN 算成全局的）。
	if c := cardOf(t, body, "爸爸"); !strings.Contains(c, `name="pin"`) {
		t.Errorf("有 PIN 的那份不该被连坐:\n%.600s", c)
	}
}

// 改 PIN 的两半：给不设防的一份**不需要旧 PIN**，给有 PIN 的一份仍然要；
// 两者都能撤。
func Test改PIN_旧PIN只在这一份有PIN时才要(t *testing.T) {
	s, h := setup(t)
	if _, err := s.CreateProfile(profileInputNoPin("tv", "客厅")); err != nil {
		t.Fatal(err)
	}

	t.Run("不设防的那份可以直接设上", func(t *testing.T) {
		cookie := sessionCookie(t, loginAs(t, h, "tv", ""))
		rec := post(h, "/api/v1/profiles/self/pin", `{"new":"1234"}`, cookie, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d %s", rec.Code, rec.Body)
		}
		p, err := s.LoadProfile("tv")
		if err != nil {
			t.Fatal(err)
		}
		if p.PIN == "" {
			t.Error("设完之后盘上仍是空的")
		}
	})

	t.Run("有 PIN 的那份不给旧 PIN 仍然拒", func(t *testing.T) {
		// 这一格在改动之前就是 403，而它**不能**因为上面那半条顺手改成 200：
		// 家人共用设备，改自己的那一份属于身份变更。
		cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
		rec := post(h, "/api/v1/profiles/self/pin", `{"new":"5678"}`, cookie, "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d %s", rec.Code, rec.Body)
		}
	})

	t.Run("本人可以把 PIN 撤掉", func(t *testing.T) {
		// 能设就能撤，否则"不设防"一旦选了就再也回不来——而那对一份公共电视屏
		// 是常见用法：先用着，后来想起来了。
		//
		// 注意 `old` 仍然要给：撤的是**一道已经锁上的门**，所以"我知道门现在是
		// 锁着的"要被证明一次。"不设防的那份不需要旧 PIN"与"锁着的门撤锁要旧
		// PIN"是同一枚硬币的两面——省掉后者等于让任何坐在别人设备前的人
		// 把那道门拆了。
		cookie := sessionCookie(t, loginAs(t, h, "tv", "1234"))
		if rec := post(h, "/api/v1/profiles/self/pin", `{"old":"1234","new":""}`, cookie, ""); rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d %s", rec.Code, rec.Body)
		}
		if p, err := s.LoadProfile("tv"); err != nil || p.PIN != "" {
			t.Errorf("撤掉之后盘上该是空的，得到 %q %v", p.PIN, err)
		}
	})

	t.Run("撤自己的 PIN 仍要旧 PIN", func(t *testing.T) {
		// 上面那条注释里那句话的行为证人：知道"撤"这个动作本身也是身份变更。
		// 这一格在 2026-10-12 之前就成立，而它**不能**被"空 new 一律放行"顺手
		// 改成 200——那是一条绕过旧 PIN 的后门。
		if _, err := s.CreateProfile(store.NewProfileInput{
			ID: "tv2", Title: "另一个", PinHash: hashOf(t, "4321")}); err != nil {
			t.Fatal(err)
		}
		cookie := sessionCookie(t, loginAs(t, h, "tv2", "4321"))
		if rec := post(h, "/api/v1/profiles/self/pin", `{"new":""}`, cookie, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("不给旧 PIN 就要 403，得到 %d %s", rec.Code, rec.Body)
		}
	})

	t.Run("管理员能把 PIN 撤掉", func(t *testing.T) {
		// 与本人那条同一个 `isNoPass` 判据、同一个落点（adminPin）。管理员是唯一
		// 能给别人去/留这道门的人：一个都不锁的家族里，他得能把门重新锁上。
		rec := post(h, "/api/v1/profiles/pin", `{"profile":"kid","new":""}`, "", testAdminKey)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d %s", rec.Code, rec.Body)
		}
		if p, err := s.LoadProfile("kid"); err != nil || p.PIN != "" {
			t.Errorf("撤掉之后盘上该是空的，得到 %q %v", p.PIN, err)
		}
	})
}
