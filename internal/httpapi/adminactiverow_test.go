package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
)

// 管理页那一行是**当前会话正在用的那一份**时，**删档与停用**两颗键不给
// （按下去必然被拒：/admin/delete 对自己 303 `?err=self`）。改之前那一格渲染
// 成一个空 `<span class="acts">`——1440 实拍里那一行读起来像"这一份没有动作"。
//
// **空格子与"没这回事"在屏上是同一种样子**，而这一格本来就有答案：它正被你自己
// 用着。所以把它说成它本来的样子，而不是留一个洞。
//
// 2026-10-20：**加/改 PIN 那一颗对每一份档案都在**（含自己正在用的那一份）——
// 管理员是机器级的一把 key，不属于任何档案，那道闸在 PIN 这一族上没有依据
// （本人自己那边 self/pin 照样要旧 PIN）。于是这一行现在有：一颗改 PIN 的键 +
// 一句说明为什么少了两颗。
func TestAdmin_当前在用的那一行说清为什么没有动作(t *testing.T) {
	s, h := setupBatch(t) // dad 与 kid 两份档案
	// 正登着 kid，管理员会话另拿——这两件事互相独立（2026-10-12 起）。
	page := adminPageGet(t, h, "/admin", kidFormCookie+"; "+authOnly(t))

	if !strings.Contains(*page, i18n.Load(i18n.Zh).T("admin.act.active")) {
		t.Errorf("当前在用的那一行没有说清它为什么没有动作：\n%.600s", *page)
	}
	// 反向证人：**只对那一行说**。kid 之外的行仍然是三颗动作键，一个都不能少。
	if got := strings.Count(*page, "/admin/pin?profile=dad"); got != 1 {
		t.Errorf("dad 那一行的「改 PIN」出现了 %d 次，期望 1 次", got)
	}
	if got := strings.Count(*page, "/admin/delete?profile=dad"); got != 1 {
		t.Errorf("dad 那一行的「删档」出现了 %d 次，期望 1 次", got)
	}
	// 关键：删档与停用这两个**点了必然被拒**的入口不许长在自己那一行上——
	// 这一族是"不给做不到的承诺"。
	for _, dead := range []string{"/admin/delete?profile=kid", "/admin/disable?profile=kid"} {
		if strings.Contains(*page, dead) {
			t.Errorf("当前在用的那一份仍然给出了 %s：按下去必然被拒（死按钮）", dead)
		}
	}
	// 而「改 PIN」是给的：管理员改自己那一份的 PIN 合法（2026-10-20 改判）。
	if !strings.Contains(*page, "/admin/pin?profile=kid") {
		t.Error("当前在用的那一份没有「改 PIN」入口：管理员是机器级的一把 key，改自己那一份合法")
	}
	// 说明让「回选择页」，那一页就得真有一个能离开这一份的入口（2026-10-24）：
	// /switch 只清档案会话，管理员 cookie 留着，回来还能进管理页。
	if got := strings.Count(*page, `action="/switch"`); got != 1 {
		t.Errorf("/switch 离开入口出现了 %d 次，期望只在当前在用的那一行出现 1 次", got)
	}
	if !strings.Contains(*page, `name="to" value="picker"`) {
		t.Error("离开入口没带 to=picker：点了不会回选择页")
	}
	_ = s
}

// 没有档案会话时（只有管理员那把 key），**每一行都该有三颗动作键**——那一格空着
// 的判据是"是不是正在用的那一份"，不是"有没有带 lb_profile"。
func TestAdmin_没有档案会话时每一行都有动作(t *testing.T) {
	_, h := setupBatch(t)
	page := adminPageGet(t, h, "/admin", authOnly(t))
	if strings.Contains(*page, i18n.Load(i18n.Zh).T("admin.act.active")) {
		t.Errorf("没有任何档案会话，却有一行自称「%s」：\n%.600s",
			i18n.Load(i18n.Zh).T("admin.act.active"), *page)
	}
	for _, p := range []string{"dad", "kid"} {
		if !strings.Contains(*page, "/admin/pin?profile="+p) {
			t.Errorf("%s 那一行没有「改 PIN」：\n%.600s", p, *page)
		}
	}
}

var _ = auth.CookieProfile // 保持 import：CookieProfile 是会话 cookie 的名字
var _ = http.StatusOK
