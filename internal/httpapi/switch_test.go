package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
)

// 夹具复用本包既有的三件：setupBatch(t)（dad=爸爸 管理员 + kid=小满 非管理员，
// 各带一份书签）、postForm(h, path, body, cookie)、cookieMap(t, rec)。
// 不要在这里再定义第二套——同包重名是编译错误，换个名字是第二份夹具。

func TestSwitch_非管理员伪造to跳别人被拒(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=dad", "lb_profile=kid")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d，要 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q，要回选择页 /（不带 ?err=：那是权限拒绝，不是输错 PIN）", loc)
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] == "dad" {
		t.Error("被拒却把会话切过去了：那是越权写入 cookie")
	}
}

// to 是表单里的自由字符串。形状就不是档案 id 的（这里是 URL 编码的 ../dad，
// 解码成 "../dad"）拿到的回答与"没这个档案"一模一样，而且**不写出任何会话
// cookie**：handler 先按 model.ValidSlug 挡掉，不去拼磁盘路径——/pin 的 profile
// 字段同一口径。这条钉的是长相（303 + 无 cookie）；挡在拼路径之前那一层是
// 防御性冗余，它的"少一次 filepath.Join"没有可观测的差，所以不假装它有证人。
func TestSwitch_非slug的to与没这个档案同一句话(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=%2E%2E%2Fdad", dadFormCookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("得到 %d %q，要 303 回选择页", rec.Code, rec.Header().Get("Location"))
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] != "" {
		t.Errorf("非 slug 的 to 不该写出会话 cookie: %v", c)
	}
}

func TestSwitch_回选择页清两条cookie(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=picker", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	c := cookieMap(t, rec)
	if c[auth.CookieProfile] != "" || c[auth.CookieHome] != "" {
		t.Errorf("回选择页要清掉两条 cookie: %v", c)
	}
}

// 目标档案不存在：也是 303 回选择页，不给"这个 id 有没有"这种枚举线索
// ——与"无权"同一句话（spec §6.1：少一种枚举面）。
func TestSwitch_目标不存在不报错也不切(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=nobody", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] == "nobody" {
		t.Error("切到了不存在的档案")
	}
}

// 目标档案存在但 profile.json 读不了：不切过去（切过去就是一页读不了的台），
// 也不把它说成服务坏了。拒的长相仍然是 303，不是 503。
func TestSwitch_目标档案读不了不切(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.ProfilePath("kid"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postForm(h, "/switch", "to=kid", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] == "kid" {
		t.Error("切进了一个 profile.json 读不了的档案")
	}
}

func TestSwitch_没有会话一律回选择页(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=home", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d，要 303（浏览器通道的拒不是 401 页面）", rec.Code)
	}
}

// 三种拒绝都不许带令牌分支：/switch 只认会话，与 /prefs 同族。
func TestSwitch_令牌走不通(t *testing.T) {
	_, h := setupBatch(t)
	req := httptest.NewRequest(http.MethodPost, "/switch", strings.NewReader("to=kid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("令牌打 /switch：得到 %d %q，要 303 回选择页", rec.Code, rec.Header().Get("Location"))
	}
}

// §13 的第三条 DOM 断言：**没有人会看到管理横幅**——代管态 2026-10-10 整条下线，
// 那条横幅随之不存在。本人看自己本来就不该画；升级前那种两枚 cookie 不相等的
// 会话是非法形状，会被清掉回选择页，页面里当然也没有横幅。
//
// 断言留在这里而不是删掉：删掉之后"横幅还会不会出现在某一页上"就没有证人了，
// 而那个疑问的答案必须是"永远不会"。
func TestSwitch_没有人的会话出横幅(t *testing.T) {
	_, h := setupBatch(t)
	own := get(h, "/", "lb_profile=kid")
	if own.Code != http.StatusOK {
		t.Fatalf("本人会话得到 %d", own.Code)
	}
	if strings.Contains(own.Body.String(), "正在管理") {
		t.Errorf("非管理员看自己的台却出了横幅:\n%.400s", own.Body)
	}
	forge := get(h, "/", "lb_home=kid; lb_profile=dad")
	if forge.Code != http.StatusOK {
		t.Fatalf("非法会话得到 %d，要 200 的选择页", forge.Code)
	}
	if strings.Contains(forge.Body.String(), "正在管理") {
		t.Errorf("home 指着非管理员的会话竟画出了横幅:\n%.400s", forge.Body)
	}
	if c := cookieMap(t, forge); c[auth.CookieProfile] != "" || c[auth.CookieHome] != "" {
		t.Errorf("非法会话该被清掉两条 cookie: %v", c)
	}
}

// 输错 PIN 不是权限拒绝，它照旧回选择页并带 ?err= 提示（spec §6.1 里那句
// "不许被一起统一成裸 303"的证人）。这条测的是既有通路，本期把它钉成回归项：
// 有人会把 /pin 的失败分支"顺手"改成裸 303。
func TestSwitch_相邻那条_PIN输错仍带err提示(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/pin", "profile=kid&pin=0000", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/?err=") {
		t.Errorf("PIN 输错要带 ?err= 提示，得到 %q——它不是权限拒绝，不许统一成裸 303", loc)
	}
	if noticeOf(i18n.Default, strings.TrimPrefix(loc, "/?err=")) == "" {
		t.Errorf("err 键必须在白名单表里: %q", loc)
	}
	if _, ok := rec.Header()["Set-Cookie"]; ok {
		t.Error("输错 PIN 不该发会话 cookie")
	}
}
