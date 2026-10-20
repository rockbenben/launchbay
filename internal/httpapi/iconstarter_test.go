package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
)

// 图标选择器的**默认那一批**（2026-10-20）：`GET /api/v1/icons/starter`。
//
// 这条端点是为了治一个具体的症状：**面板打开是一张空网格**。搜索那条按契约
// 空查询回一张空图（库里一万多条，倒出来是一屏噪声），而"这一份已经在用的
// 那些图标"在实测里几乎永远是空的（书签默认 auto，auto 与"没设"都被排除），
// 于是点开「图标库…」什么也没有。
//
// 这几条钉的是它**与界面那一半的关系**：同一份索引、同一个画格子的出口，
// 而它给的是一整批、不截断。

// 公开可查：它是一份公共名单，说不出任何人有什么书签。
func TestIconStarter_公开可查且画出方格(t *testing.T) {
	_, h := setup(t)
	rec := getTiles(t, h, "/api/v1/icons/starter") // 无凭证

	if rec.Code != http.StatusOK {
		t.Fatalf("匿名要 200，得到 %d：%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("这一条吐的是 HTML 方格，Content-Type 要是 text/html，得到 %q", ct)
	}
	// 这段要被插进页面里，缺 nosniff 时浏览器照样 sniff——所以它是承重的。
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("缺 X-Content-Type-Options: nosniff：这一段是要被插进页面里的文本")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Errorf("方格里没有内联 SVG（要与卡片上那枚同源）：%s", body)
	}
	// **不能是空的**：这一条端点存在的全部理由就是"打开就有东西可挑"。
	if n := strings.Count(body, "<button"); n < 12 {
		t.Errorf("只画了 %d 格，太少了：面板打开要有一屏可挑的东西", n)
	}
}

// 画出来的必须是**名单上那几条**，而且一条不多一条不少。
func TestIconStarter_画的就是名单上那几条(t *testing.T) {
	_, h := setup(t)
	rec := getTiles(t, h, "/api/v1/icons/starter")
	body := rec.Body.String()

	want := icons.MustLoad().StarterNames()
	for _, key := range want {
		if !strings.Contains(body, `data-icon="`+key+`"`) {
			t.Errorf("方格里少了 %q", key)
		}
	}
	if n := strings.Count(body, "<button"); n != len(want) {
		t.Errorf("名单上 %d 条，却画了 %d 格", len(want), n)
	}
}

// **不带 X-LB-Icons-Total**：那一格在搜索那条上是为了说"命中很多被截断"，
// 而这一批是固定的一整批。报一个总数反而像在说"还有别的没给你"。
func TestIconStarter_不带总数那一格(t *testing.T) {
	_, h := setup(t)
	rec := getTiles(t, h, "/api/v1/icons/starter")
	if got := rec.Header().Get(iconTilesTotalHeader); got != "" {
		t.Errorf("这一批不截断，不该报总数（那是搜索那条的事）：%q", got)
	}
}

// 与搜索那条**同族**：同一份索引、同一个画格子的出口。于是"默认那一批"与
// "搜出来的那些"画出来的是同一种格子（同一个 svgMarkup）。
func TestIconStarter_与搜索那条画的是同一种格子(t *testing.T) {
	_, h := setup(t)
	starter := getTiles(t, h, "/api/v1/icons/starter").Body.String()
	searched := getTiles(t, h, "/api/v1/icons/tiles?q=docker").Body.String()

	// 同一格的外壳：类型、class、data-icon 与 aria-label 都在。
	for _, frag := range []string{`<button type="button" class="fld icpick-tile" data-icon=`, `<span class="icpick-face">`, `<span class="icpick-name">`} {
		if !strings.Contains(starter, frag) {
			t.Errorf("默认那一批里少了 %q", frag)
		}
		if !strings.Contains(searched, frag) {
			t.Errorf("搜索那条里少了 %q", frag)
		}
	}
}
