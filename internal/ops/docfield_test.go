package ops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 根级标量字段（2026-10-14）：原来那道闸是「路径必须以 groups 开头」，它是
// "set 能碰的字段必须在白名单内"这条原则的粗粒度实现——连**页面画得出来的**
// `openInNewTab` 一起挡在外面。这一族把新判据逐条钉住，顺带钉住**仍然拒的**那几条
// （rev / groups / search / weather）：改判据最常见的失败是"闸松了以后什么都
// 能 set 了"。

// docRun 跑一条 op 并把它的失败原因原样交回（空串 = 改得动）。Run 的错误是逐条
// 收集的 []Error，那是给 HTTP 层的形状；这里只要第一条的 Reason。
func docRun(t *testing.T, b *model.Bookmarks, op Op) string {
	t.Helper()
	_, errs, _ := Run(b, []Op{op}, nil)
	if len(errs) == 0 {
		return ""
	}
	return errs[0].Reason
}

// docRunOn 与 docRun 相同，但把 **Run 产出的那份新文档**交回——Run 是纯函数，
// 它克隆一份再改，原件一个字不动（所以"改完看原件"什么也看不到）。
func docRunOn(t *testing.T, b *model.Bookmarks, op Op) (*model.Bookmarks, string) {
	t.Helper()
	out, errs, _ := Run(b, []Op{op}, nil)
	if len(errs) != 0 {
		return out, errs[0].Reason
	}
	return out, ""
}

func docEmpty() *model.Bookmarks { return &model.Bookmarks{Rev: 1} }

func TestDocField_白名单里的四格改得动(t *testing.T) {
	for _, tc := range []struct {
		field string
		value string
		check func(b *model.Bookmarks) bool
	}{
		{"openInNewTab", "true", func(b *model.Bookmarks) bool { return b.OpenInNewTab }},
		{"openInNewTab", "false", func(b *model.Bookmarks) bool { return !b.OpenInNewTab }},
		{"clock", "true", func(b *model.Bookmarks) bool { return b.Clock }},
		{"footer", `"night"`, func(b *model.Bookmarks) bool { return b.Footer == "night" }},
		{"title", `"deck"`, func(b *model.Bookmarks) bool { return b.Title == "deck" }},
	} {
		b, why := docRunOn(t, docEmpty(), Op{Op: "set", Path: tc.field, Value: json.RawMessage(tc.value)})
		if why != "" {
			t.Errorf("set path=%s value=%s 被拒了：%s", tc.field, tc.value, why)
			continue
		}
		if !tc.check(b) {
			t.Errorf("set path=%s value=%s 之后字段没变成那个值：openInNewTab=%v footer=%q title=%q",
				tc.field, tc.value, b.OpenInNewTab, b.Footer, b.Title)
		}
	}
}

func TestDocField_仍然拒的那几条各有各的理由(t *testing.T) {
	for _, tc := range []struct{ path, wantIn string }{
		{"rev", "并发判据"},
		{"groups", "整表替换"},
		{"search", "对象"},
		{"weather", "对象"},
		{"nosuchfield", "没有"},
		// 根不是 groups 的多段路径：仍然拒（别让它变成"任意路径"的后门）
		{"openInNewTab/x", ""},
	} {
		why := docRun(t, docEmpty(), Op{Op: "set", Path: tc.path, Value: json.RawMessage("true")})
		if why == "" {
			t.Errorf("set path=%s 不该被放行", tc.path)
			continue
		}
		if tc.wantIn != "" && !strings.Contains(why, tc.wantIn) {
			t.Errorf("set path=%s 的拒绝语里没有「%s」：%s（笼统一句话会让调用方以为整条 ops 坏了）",
				tc.path, tc.wantIn, why)
		}
	}
}

func TestDocField_布尔那两格不接受字符串(t *testing.T) {
	// 症状是"按了新标签页，没反应"：页面上的 `{{if .OpenInNewTab}}` 只认真布尔，
	// 而 decode 对 "true" 是放行的（那是合法 JSON 字符串）。所以这一条单独卡。
	for _, field := range []string{"openInNewTab", "clock"} {
		if why := docRun(t, docEmpty(), Op{Op: "set", Path: field, Value: json.RawMessage(`"true"`)}); why == "" {
			t.Errorf("set path=%s value=\"true\" 要被拒（字符串会被页面静默当假）", field)
		}
	}
}

func TestDocField_add_remove_move都不认标量(t *testing.T) {
	for _, op := range []string{"add", "remove", "move"} {
		why := docRun(t, docEmpty(), Op{Op: op, Path: "openInNewTab", Value: json.RawMessage("true"), To: "apps"})
		if why == "" {
			t.Errorf("%s path=openInNewTab 不该被放行（标量只有 set 有形状）", op)
			continue
		}
		// 那句话得指着出路，而不是只说"不许"
		if !strings.Contains(why, "set path=openInNewTab") {
			t.Errorf("%s 的拒绝语没有指出出路（set path=openInNewTab）：%s", op, why)
		}
	}
}
