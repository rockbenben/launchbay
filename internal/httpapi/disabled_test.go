package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

// 停用这一族动作的三条证人。它们此前**一条都没有**：/api/v1/profiles/disable
// 的响应值写错了（成功停用之后回 `disabled:false`）而 CI 全绿，正是缺这三条。

func disableViaStore(t *testing.T, s *store.Store, id string, disabled bool) {
	t.Helper()
	p, err := s.LoadProfile(id)
	if err != nil {
		t.Fatal(err)
	}
	p.Disabled = disabled
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
}

// 停用的档案：PIN **输对了**也进不去。这一支从前复用「PIN 不对」那句话，
// 于是人会一遍遍重敲一个正确的 PIN——谎话的代价落在被停用的那个人身上。
func Test停用档案要说得出为什么进不去(t *testing.T) {
	s, h := setupBatch(t)
	disableViaStore(t, s, "kid", true)

	rec := postForm(h, "/pin", "profile=kid&pin=5678", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("停用该 303，得到 %d: %s", rec.Code, rec.Body)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "err=disabled") {
		t.Errorf("停用档案输对 PIN 之后 Location = %q，要带 err=disabled（说清真原因，别让人重敲对的 PIN）", loc)
	}
	// 停用这一支排在 PIN 校验**之前**：档案是不是停用与 PIN 敲对没敲对无关，
	// 而"你被停用了"对敲错的人也是更有用的那句话（他重敲一百遍也进不去）。
	// 对照另一头：没停用的档案敲错 PIN 仍然只说 err=pin，两条不许并成一句。
	if l2 := postForm(h, "/pin", "profile=kid&pin=0000", "").Header().Get("Location"); !strings.Contains(l2, "err=disabled") {
		t.Errorf("停用档案敲错 PIN 的 Location = %q，也该说停用", l2)
	}
	if l3 := postForm(h, "/pin", "profile=dad&pin=0000", "").Header().Get("Location"); !strings.Contains(l3, "err=pin") || strings.Contains(l3, "disabled") {
		t.Errorf("没停用的档案敲错 PIN 的 Location = %q，要 err=pin 且不说停用", l3)
	}
	// 选择页要把这句话印出来（白名单里的键才进页面）。
	page := get(h, "/?err=disabled", "").Body.String()
	if !strings.Contains(page, "已经被停用") {
		t.Errorf("选择页没印出停用的那句话:\n%.300s", page)
	}
}

// 停用是管理员**刚刚**做的事，而被停用的人手里的会话是停用之前发的（30 天）。
// 从前停用只挡 PIN 那道门：已经站在门里的人可以一直用到 cookie 过期，界面
// 一个字都不说，而他正是被停用的那一个。
func Test停用把已经在用的人请出去(t *testing.T) {
	s, h := setupBatch(t)
	before := get(h, "/", kidFormCookie)
	if !strings.Contains(before.Body.String(), "动画") {
		t.Fatalf("前置不成立：kid 的会话本来就该进得去自己的台:\n%.300s", before.Body)
	}
	disableViaStore(t, s, "kid", true)

	rec := get(h, "/", kidFormCookie)
	// 两条 cookie 都要清（Get 只回第一条，这里必须用 Values）。
	sc := strings.Join(rec.Header().Values("Set-Cookie"), " | ")
	if !strings.Contains(sc, "lb_profile=;") || !strings.Contains(sc, "Max-Age=0") {
		t.Errorf("停用之后旧会话该被清掉，Set-Cookie = %q", sc)
	}
	if strings.Contains(rec.Body.String(), "动画") {
		t.Errorf("停用之后仍然端出 kid 的书签页——停用只挡住了没进来的人:\n%.300s", rec.Body)
	}
	// 反向对照：没被停用的人照旧进得去（这条闸不许顺手把别人也踢了）。
	if dad := get(h, "/", dadFormCookie); !strings.Contains(dad.Body.String(), "群晖") {
		t.Errorf("dad 没被停用却被挡在外面:\n%.300s", dad.Body)
	}
}

// 响应体必须说**这一次真的设成了什么**。旧代码只在"同状态早退"那一支给
// res.Disabled，落盘那一支从没赋值——于是成功停用回答 `disabled:false`，
// 与磁盘上的 `true` 相反（实测：CLI 与 AI 都会照这个假答案继续往下走）。
func Test停用接口的响应值跟着磁盘走(t *testing.T) {
	s, h := setupBatch(t)
	rec := post(h, "/api/v1/profiles/disable", `{"profile":"kid","disabled":true}`, "", "tok-admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Profile  string `json:"profile"`
		Disabled bool   `json:"disabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	p, err := s.LoadProfile("kid")
	if err != nil {
		t.Fatal(err)
	}
	if out.Disabled != p.Disabled {
		t.Errorf("响应 disabled=%v 而磁盘 disabled=%v：响应与磁盘矛盾，调用方读到的是假结论", out.Disabled, p.Disabled)
	}
	if !p.Disabled {
		t.Error("前置不成立：这一步就该真的停用")
	}
	// 幂等再设一次，答案不许翻转。
	rec2 := post(h, "/api/v1/profiles/disable", `{"profile":"kid","disabled":true}`, "", "tok-admin")
	if !strings.Contains(rec2.Body.String(), `"disabled": true`) { // §4.3：缩进 2 空格，冒号后有空格
		t.Errorf("同状态再设一次之后响应 = %s", rec2.Body)
	}
}
