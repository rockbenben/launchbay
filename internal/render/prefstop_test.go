package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 独立台面页要有一个顶部出口（2026-10-24）。
//
// 这一页在手机上能长到近 1800px，而底部那颗「回我的页面」要滚到底才够得着；
// 零 JS / PWA 独立窗口又没有浏览器返回键。顶部这条只属于独立页——浮层只取
// `section.prefs`，如果把它放进面板 define，浮层里会与 × 重复。
func TestPrefsPage_独立页有顶部出口且不进浮层面板(t *testing.T) {
	s := seeded(t)
	page, err := renderer(t, s).PrefsPage("dad", "", i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	if !strings.Contains(body, `class="prefs-back prefs-top"`) {
		t.Error("独立设置页没有顶部出口：手机上只能滚到底部才能回去")
	}
	// 反面：这条必须在 `section.prefs` **之前**。浮层只取那个 section，
	// 放进去就会在浮层里多出一颗与 × 重复的出口。
	at := strings.Index(body, `<section class="prefs">`)
	if at < 0 {
		t.Fatal("页面里没有 section.prefs")
	}
	if strings.Contains(body[at:], "prefs-top") {
		t.Error("顶部出口被放进面板 define 了：浮层里会与关闭键重复")
	}
}
