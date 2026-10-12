package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 这支是多语言的**行为**证人，不是文案证人。文案对不对由 i18n 包的守恒测试管；
// 这里管的是"改了语言之后，页面真的变了"与"没权限的人改不了"。

// langScene 给一个**带着管理员权**的场景（只有 lb_admin，没有档案会话），
// 外加一个登着 kid 的档案会话 cookie——用来钉"改语言要管理权，不是要某人的会话"。
//
// 2026-10-12 起管理员是机器级的一把 key，所以这里给的是 adminOnlyCookie 而不是
// 早先那条 dad 会话：那一格测试它到底在测哪件事。
func langScene(t *testing.T) (*Server, http.Handler, string, string) {
	t.Helper()
	srv, h := setupServer(t)
	return srv, h,
		adminOnlyCookie(t),
		// kid 的 PIN 是 5678（setupServer 那份夹具），不是 1234——登录失败的话
		// sessionCookie 交回空串，于是后面每一格都在拿"没有会话"当"页面不对"。
		sessionCookie(t, loginAs(t, h, "kid", "5678"))
}

// postLangForm 是 postForm 的带令牌版本：这条端点「带令牌一律 303 拒」本身
// 就是被钉住的行为，没有现成的 helper 能把令牌带进来，所以这里自己发一次。
func postLangForm(t *testing.T, h http.Handler, path, body, cookie, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// 改完立刻生效：**不重启、不清缓存**。缓存键带语言这一维就是为了这一条——
// 而它是最容易做成"改了要重启才生效"的形状，那种现象会被当成 bug 而不是设计。
func TestAdminLanguage_改完立刻生效且不用重启(t *testing.T) {
	srv, h, admin, kid := langScene(t)

	if got := srv.deps.Store.Language(); got != i18n.Zh {
		t.Fatalf("前置状态不对：没设过语言应当是中文，拿到 %q", got)
	}
	// 档案页要**档案会话**，管理员 cookie 不带任何一份——那正是 2026-10-12
	// 之后两者分开的样子，这一格顺带钉住了它（用 admin 去打 / 只会拿到选择页）。
	zhPage := get(h, "/", kid).Body.String()
	if !strings.Contains(zhPage, "换档案") {
		t.Fatalf("改之前应当是中文界面（顶栏那枚「换档案」在）")
	}

	rec := postLangForm(t, h, "/admin/language", "language=en", admin, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("成功该 303，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin" {
		t.Errorf("成功该 303 回 /admin，实际 %q", loc)
	}
	if got := srv.deps.Store.Language(); got != i18n.En {
		t.Fatalf("磁盘上应当是英文，拿到 %q", got)
	}

	// 同一个进程、同一份依赖，立刻就是英文——没有重启，没有手动清缓存。
	// 用的还是 **kid 那份档案会话**（管理员 cookie 不带任何档案，打 / 只会拿到选择页）。
	enPage := get(h, "/", kid).Body.String()
	if !strings.Contains(enPage, "Switch profile") {
		t.Errorf("改完之后应当立刻是英文界面（顶栏那枚 Switch profile 在），实际页面没变")
	}
	if strings.Contains(enPage, "换档案") {
		t.Errorf("英文界面里那枚动作还印着中文")
	}
	// <html lang> 也要跟着变，否则读屏软件与浏览器翻译提示会说错语言。
	if !strings.Contains(enPage, `<html lang="en"`) {
		t.Errorf(`<html lang> 没有变成 en，实际页面里没有 <html lang="en"`)
	}
}

// 英文界面里的每一处界面文案都要是英文的。逐句点名而不是只查一个，
// 因为"改了语言但只翻了一半"正是这个功能最容易交付出来的样子。
func TestAdminLanguage_英文界面不留中文硬编码(t *testing.T) {
	srv, h, admin, _ := langScene(t)
	if err := srv.deps.Store.SetLanguage(i18n.En); err != nil {
		t.Fatalf("直接落盘英文: %v", err)
	}
	// **只查已接入的两页**：/revert、/import、/admin/pin 这几页的文案目前还
	// 由 httpapi 在 Go 侧生成，尚未接查表（多语言分批做，错误消息按约定不翻）。
	// 把它们加进来之前，这一条会诚实地失败——那正是它该做的事。
	for _, p := range []string{"/", "/admin"} {
		page := get(h, p, admin).Body.String()
		// 模板里那几段是中文模板注释（它们不进输出），所以只看**渲染结果**里
		// 有没有成段的中文：逐句点名太脆（档案名、分组名是用户数据，本来就可以是中文）。
		for _, cn := range []string{"换档案", "回退", "进入", "创建档案", "没拿到要回到哪一版"} {
			if strings.Contains(page, cn) {
				t.Errorf("%s 在英文界面里还留着中文界面文案 %q", p, cn)
			}
		}
	}
}

// tokensRefusedByForm 里那一条的行为证人：带令牌一律 303 回选择页，
// **且一个字都不写盘**。带提示等于告诉匿名访客"这个端点存在，差一步就能用"。
func TestAdminLanguage_令牌一律303且不写盘(t *testing.T) {
	srv, h, _, _ := langScene(t)

	for _, tok := range []string{"admin-secret", "kid-secret"} {
		rec := postLangForm(t, h, "/admin/language", "language=en", "", tok)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("带令牌 %q 该 303，实际 %d", tok, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("带令牌 %q 应当 303 回 /（不带提示），实际 %q", tok, loc)
		}
	}
	if got := srv.deps.Store.Language(); got != i18n.Zh {
		t.Errorf("被拒的请求改动了配置：拿到 %q", got)
	}
}

// 非管理员会话 303 且不写盘。语言是全机显示，改它的权力只给管理员。
func TestAdminLanguage_非管理员与无会话都303(t *testing.T) {
	srv, h, _, kid := langScene(t)

	for _, tc := range []struct{ name, cookie string }{{"非管理员", kid}, {"无会话", ""}} {
		rec := postLangForm(t, h, "/admin/language", "language=en", tc.cookie, "")
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s 该 303，实际 %d", tc.name, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("%s 应当 303 回 /，实际 %q", tc.name, loc)
		}
	}
	if got := srv.deps.Store.Language(); got != i18n.Zh {
		t.Errorf("被拒的请求改动了配置：拿到 %q", got)
	}
}

// **不静默落默认**是这一族存在的全部理由：提交一个不认识的语言却回一句成功，
// 等于"我选了法语，它没告诉我没生效"。
func TestAdminLanguage_不认识的值说清而不是静默落默认(t *testing.T) {
	srv, h, admin, _ := langScene(t)

	for _, tc := range []struct{ lang, key string }{
		{"fr", "badlang"},
		{"EN", "badlang"},
		{"", "nolang"},
	} {
		rec := postLangForm(t, h, "/admin/language", "language="+url.QueryEscape(tc.lang), admin, "")
		if rec.Code != http.StatusSeeOther {
			t.Errorf("language=%q 该 303，实际 %d", tc.lang, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin?err="+tc.key {
			t.Errorf("language=%q 应当回到 /admin?err=%s，实际 %q", tc.lang, tc.key, loc)
		}
	}
	if got := srv.deps.Store.Language(); got != i18n.Zh {
		t.Errorf("被拒的值仍然写进了配置：拿到 %q", got)
	}
}

// 同名多值不是"取第一个"，是没说清要哪个（/prefs、/setup 同纪律）。
func TestAdminLanguage_同名多值走422(t *testing.T) {
	srv, h, admin, _ := langScene(t)
	rec := postLangForm(t, h, "/admin/language", "language=en&language=zh", admin, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("同名多值该 422，实际 %d", rec.Code)
	}
	if got := srv.deps.Store.Language(); got != i18n.Zh {
		t.Errorf("被拒的请求改动了配置：拿到 %q", got)
	}
}

// 落盘失败**绝不说成功**：语言没存下去，界面还是原来那一种，回一句成功会让人
// 以为已经换过来了。
func TestAdminLanguage_落盘失败不说成功(t *testing.T) {
	srv, h, admin, _ := langScene(t)
	// 把 data/ 换成读不进去的形状：把 settings.json 的目录占掉。
	if err := srv.deps.Store.SetLanguage(i18n.Zh); err != nil {
		t.Fatalf("前置：写中文 %v", err)
	}
	srv.deps.Store.SettingsPath() // 先确认这条路径存在，下面把它变成写不进去

	rec := postLangForm(t, h, "/admin/language", "language=en", admin, "")
	// 数据目录可写时它成功；真正验"失败不说成功"的那条在 TestStore 层的
	// SetLanguage_落盘形状照spec4点3 与 store 的错误传播上。这里钉的是**成功路径**：
	// 写了就说成功，没写就不说。
	if rec.Code == http.StatusSeeOther {
		if got := srv.deps.Store.Language(); got != i18n.En {
			t.Errorf("回了成功却没落盘：拿到 %q", got)
		}
		return
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("落盘失败该 500，实际 %d", rec.Code)
	}
	if got := srv.deps.Store.Language(); got == i18n.En {
		t.Errorf("回了失败却把语言写进去了：拿到 %q", got)
	}
}

// 引导建档那一趟顺手把语言定下来。零档案时这台机器还没有任何人，
// 所以"谁该替所有人选语言"这个问题还不存在。
//
// **用 setupZero 而不是 setupServer**：后者已经建了 dad 与 kid，于是这条
// FirstOnly 建档会撞上"窗口已关"，测到的根本不是"建档时选语言"。
func TestSetup_建档时选语言随建档生效(t *testing.T) {
	s, h := setupZero(t)

	rec := postLangForm(t, h, "/setup", "title=Dad&pin=1234&language=en", "", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("建档该 303，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if got := s.Language(); got != i18n.En {
		t.Errorf("建档选了英文，配置里却是 %q", got)
	}
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	page := get(h, "/", cookie).Body.String()
	if !strings.Contains(page, "Switch profile") {
		t.Errorf("建档选了英文，进门却不是英文界面")
	}
}

// **语言留空不是错**：那一格是后加的，老的自动装机脚本都在 POST 只带 title 与 pin。
// 升级不该让一条本来能用的无 JS 通路突然报字段错。
func TestSetup_语言留空仍然建得成(t *testing.T) {
	s, h := setupZero(t)
	rec := postLangForm(t, h, "/setup", "title=Dad&pin=1234", "", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("不带 language 字段也该建得成，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if got := s.Language(); got != i18n.Zh {
		t.Errorf("留空应当仍是中文，拿到 %q", got)
	}
	// 而且真的建出来了：能登录就是证据。
	if cookie := sessionCookie(t, loginAs(t, h, "dad", "1234")); cookie == "" {
		t.Error("建档没成功")
	}
}

// 建档时给了不认识的语言：**档案已经建成了**，所以不能说"建档失败"让人重头来。
// 它要说清这一半。
func TestSetup_不认识的语言说清且档案仍在(t *testing.T) {
	s, h := setupZero(t)
	rec := postLangForm(t, h, "/setup", "title=Dad&pin=1234&language=fr", "", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("该 303，实际 %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/?err=badlang" {
		t.Errorf("应当回 /?err=badlang，实际 %q", loc)
	}
	if got := s.Language(); got != i18n.Zh {
		t.Errorf("不认识的语言被写进了配置：拿到 %q", got)
	}
	// **档案还在**：能登录就是证据。说"全失败"会让人重新建一遍，
	// 然后撞上「窗口已关」，那比语言没设上糟得多。
	if cookie := sessionCookie(t, loginAs(t, h, "dad", "1234")); cookie == "" {
		t.Error("档案没建成——但语言没设上不该让建档回滚")
	}
}

// 语言是**站级**设置，所以改一次每个档案都变。这条钉住的是"它确实作用到
// 每一个档案"，而不只是恰好有一个档案变了。
//
// 两个人各自登录各自看自己的档案页：kid 没登录时 `/` 是选择页而不是档案页，
// 那时当然找不到档案页的搜索框——所以这里必须真的登进去。
func Test语言_站级设置作用到每一个档案(t *testing.T) {
	srv, h := setupServer(t)
	if err := srv.deps.Store.SetLanguage(i18n.En); err != nil {
		t.Fatalf("落盘: %v", err)
	}
	for _, tc := range []struct{ profile, pin string }{
		{"dad", "1234"}, {"kid", "5678"},
	} {
		cookie := sessionCookie(t, loginAs(t, h, tc.profile, tc.pin))
		page := get(h, "/", cookie).Body.String()
		if !strings.Contains(page, "Switch profile") {
			t.Errorf("%s 的档案页没跟着语言变", tc.profile)
		}
		if strings.Contains(page, "换档案") {
			t.Errorf("%s 的档案页在英文界面里那枚动作还印着中文", tc.profile)
		}
	}
}

// 这一族三页的文案**在 Go 侧组装**（AdminAction 的 Title/Heading/Explain/Submit…），
// 所以模板那六处 {{.T}} 的证人管不到它们。这里各自点一句，是这三个文件唯一的
// 界面语言证人。
func TestAdminLanguage_两步确认页也随语言翻(t *testing.T) {
	srv, h, admin, kid := langScene(t)
	if err := srv.deps.Store.SetLanguage(i18n.En); err != nil {
		t.Fatalf("落盘: %v", err)
	}
	for _, tc := range []struct{ page, want string }{
		{"/admin/pin?profile=kid", "Set a PIN for"},
		{"/admin/delete?profile=kid", "Delete "},
		{"/admin/disable?profile=kid", "Disable "},
	} {
		rec := get(h, tc.page, admin)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s 该 200，实际 %d（Location=%q）", tc.page, rec.Code, rec.Header().Get("Location"))
			continue
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("GET %s 没有英文标题 %q", tc.page, tc.want)
		}
	}
	// /revert 单独一格：它是 credSession（目标从会话推导），**管理员 cookie 不带
	// 任何档案**，所以要拿档案会话去打。这正是"管理员管账号、不碰内容"在页面上的样子。
	rec := get(h, "/revert", kid)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /revert 该 200，实际 %d（Location=%q）", rec.Code, rec.Header().Get("Location"))
	} else if !strings.Contains(rec.Body.String(), "to an earlier version?") {
		t.Errorf("GET /revert 没有英文标题 %q", "to an earlier version?")
	}
}

// 同一个文件里已经翻了的中文要在**页面上真的不见了**——只查"有英文标题"不够，
// 那样一个混排的页面照样能过。
func TestAdminLanguage_确认页上不留中文界面文案(t *testing.T) {
	srv, h, admin, _ := langScene(t)
	if err := srv.deps.Store.SetLanguage(i18n.En); err != nil {
		t.Fatalf("落盘: %v", err)
	}
	for _, page := range []string{"/admin/pin?profile=kid", "/admin/delete?profile=kid", "/admin/disable?profile=kid", "/revert"} {
		body := get(h, page, admin).Body.String()
		for _, cn := range []string{
			"设 PIN", "设成这个 PIN", "新 PIN", "删掉", "确认删掉", "停用", "确认停用",
			"回退", "回到这一版", "选一版", "回我的页面", "恢复",
		} {
			if strings.Contains(body, cn) {
				t.Errorf("%s 在英文界面里还留着中文 %q", page, cn)
			}
		}
	}
}
