package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 条目/分组 id 的长度上限（2026-10-16 排序复查时撞出来的）。
//
// 症状：从地址派生的 id 一旦**超过上限**，整批被服务端 422 拒掉，而屏上只有一句
// 「这一批没有写进去：这台机器不收刚才那些内容」（真话在控制台里）。地址一点问题
// 都没有——实测 `https://example.com/sortcheck-body-1789612345678` 派生出 36 字符的
// id，被整批拒掉。
//
// 根因是两侧各写了一个数：`ascii()` 从前只切到 **40**，而 `model.ValidSlug` 的上限
// 是 **32**（`^[a-z0-9][a-z0-9-]{0,31}$`）。所以这条钉的是"两个数必须相等"，
// 而且**上限那个数从 model 现算**，不在这里再抄一份——抄一份就是下一次漂移的起点。
//
// 光切到 32 还不够：两个长地址共用同一个 32 字前缀 → duplicate-id，也是 422。
// 所以切完要补一段**由同一串文字算出来的稳定后缀**（同一段判据见下面那条断言）。
func Test派生的id不超过model的上限(t *testing.T) {
	// 从 model 现算上限：32 这个数只住在正则里。
	max := 0
	for n := 1; n < 80; n++ {
		if model.ValidSlug(strings.Repeat("a", n)) {
			max = n
		}
	}
	if max != 32 {
		t.Fatalf("model.ValidSlug 的上限算出来是 %d，钉住这个测试之前先看一眼是不是判据变了", max)
	}

	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	if !strings.Contains(js, "const SLUG_MAX = "+itoa(max)+";") {
		t.Errorf("app.js 里的 SLUG_MAX 与 model.ValidSlug 的上限（%d）对不上：\n"+
			"切短了会撞 duplicate-id，切长了整批被 422 拒掉，而屏上只说「没存进去」", max)
	}
	// 切完补稳定后缀：可读的那一半留住，唯一性交给原文。
	asc := between(js, "function ascii(text) {", "\n  }")
	for _, want := range []string{"hash36(s)", "s.slice(0, SLUG_MAX - h.length - 1) + '-' + h"} {
		if !strings.Contains(asc, want) {
			t.Errorf("ascii 缺 %q：只切到上限会让两个长地址共用同一个前缀（duplicate-id）\n%.400s", want, asc)
		}
	}
	// 从前那个 40 必须没了：它就是这条 bug 的来路。
	if strings.Contains(js, ".slice(0, 40)") {
		t.Error("ascii 里还留着 slice(0, 40)：那是超过 model 上限的那一刀")
	}
}
