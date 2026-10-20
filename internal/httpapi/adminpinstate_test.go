package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
)

// 管理页要能一眼看出**哪几份还没上锁**。
//
// 判据是 auth.HasPin（空哈希 = 没设 PIN），值由 statusRows 算好传下来
// （statusProfile.PinSet → render.AdminRow.PinSet）。两个后果：
//   - 动作键说的是「加 PIN」还是「改 PIN」——同一枚键在两种情形下说两件事，
//     人不用点进去才知道这一份是锁着的还是开着的；
//   - 没上锁的那一份带一枚告警徽标：不设防的档案在局域网里谁都能进，
//     那是管理员最该先看见的一件事。
func TestAdmin_表里分得出加与改(t *testing.T) {
	_, h, dad := adminScene(t)
	// 先把 kid 的 PIN 撤掉（留空 = 取消），它于是变成"没上锁"的那一份。
	if rec := postForm(h, "/admin/pin", "profile=kid&new=", dad); rec.Code != http.StatusSeeOther {
		t.Fatalf("撤掉 kid 的 PIN 该 303，实际 %d %s", rec.Code, rec.Body)
	}
	body := adminPageGet(t, h, "/admin", dad)
	zh := i18n.Load(i18n.Zh)
	if !strings.Contains(*body, zh.T("admin.act.addpin")) {
		t.Errorf("没上锁的那一份该说「%s」", zh.T("admin.act.addpin"))
	}
	if !strings.Contains(*body, zh.T("admin.act.changepin")) {
		t.Errorf("上了锁的那一份该说「%s」", zh.T("admin.act.changepin"))
	}
	if !strings.Contains(*body, zh.T("admin.badge.nopin")) {
		t.Errorf("没上锁的那一份该带一枚「%s」徽标", zh.T("admin.badge.nopin"))
	}
}

// 装好机器的人第一次站在登录页上，要知道的只有一件事：**我该输什么**。
//
// 答案就是 compose 里那个**公开的**默认值——它写在仓库与 README 里，
// 认得它的人本来就管得了这台机器。而**当前配置的那一把一个字都不许进页面**：
// 它会进 devtools、进导出、进浏览器同步。
func TestAdmin_登录页写出默认那把密钥(t *testing.T) {
	_, h := setupServer(t)
	rec := get(h, "/admin", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("没有管理权时该渲染登录页（200），实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, auth.DefaultAdminToken) {
		t.Errorf("登录页没有写出默认那把密钥 %q：装机器的人不知道该输什么", auth.DefaultAdminToken)
	}
	// 反向证人：**当前配置的那把不许出现在页面上**。
	if strings.Contains(body, testAdminKey) {
		t.Errorf("登录页把当前配置的管理密钥印出来了（%q）", testAdminKey)
	}
}

// 管理页那一块「管理密钥」说两件事：默认那把是什么、**这台机器是不是还在用它**。
//
// 第二件只在**已经进了管理页**的人眼前才有意义——未登录的那一页不说
// "这台机器还在用默认那把"，那等于给摸到这台机器的人一条现成的钥匙。
func TestAdmin_管理页说清默认那把与是否还在用它(t *testing.T) {
	srv, h := setupServer(t)
	zh := i18n.Load(i18n.Zh)
	body := adminPageGet(t, h, "/admin", authOnly(t))
	if !strings.Contains(*body, auth.DefaultAdminToken) {
		t.Errorf("管理页没有写出默认那把密钥 %q", auth.DefaultAdminToken)
	}
	// 测试里那把不是默认值，于是不该有"还在用默认那把"的提醒。
	if strings.Contains(*body, zh.T("admin.key.warn")) {
		t.Errorf("这台机器用的不是默认那把，页面上却说了「%s」", zh.T("admin.key.warn"))
	}
	// 换成默认那把：提醒要出现。
	at, err := auth.ParseAdminToken(auth.DefaultAdminToken)
	if err != nil {
		t.Fatal(err)
	}
	srv.deps.AdminToken = at
	body = adminPageGet(t, h, "/admin", auth.CookieAdmin+"="+at.DeriveCookie())
	if !strings.Contains(*body, zh.T("admin.key.warn")) {
		t.Errorf("这台机器用的就是默认那把，页面上却没有那句「%s」", zh.T("admin.key.warn"))
	}
}

// 登录页**不**说"这台机器还在用默认那把"：那一页是给还没登录的人看的，
// 而那句话等于把钥匙递给每一个摸得到这台机器的人。
func TestAdmin_登录页不说这台机器还在用默认那把(t *testing.T) {
	_, h := setupServer(t)
	rec := get(h, "/admin", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("没有管理权时该渲染登录页（200），实际 %d", rec.Code)
	}
	warn := i18n.Load(i18n.Zh).T("admin.key.warn")
	if strings.Contains(rec.Body.String(), warn) {
		t.Errorf("登录页说了「%s」：那句话等于给匿名访客递钥匙", warn)
	}
}
