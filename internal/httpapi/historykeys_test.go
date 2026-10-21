package httpapi

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// 历史那一族的 key 是**map 的值**（渠道与字段的名字归那些 map 管），
// 所以"扫调用点字面量"那一遍**看不见它们**——
//
//	itemFieldKeys["name"] = "history.op.item.nmae"
//
// 写错一个，屏幕上就印出 `⟨history.op.item.nmae⟩`，而没有任何测试会红。
//
// 渲染层那一侧扫不到它们（它们不在 .go 的调用点里），所以这一族在这里查：
// 判据是**每一枚都在语言包里**（两种语言都要有）。
func Test历史那一族的key都在语言包里(t *testing.T) {
	for name, m := range map[string]map[string]string{
		"itemFieldKeys":  itemFieldKeys,
		"groupFieldKeys": groupFieldKeys,
		"byChannels":     byChannels,
	} {
		for field, key := range m {
			if key == "" {
				t.Errorf("%s[%q] 是空串：页面上那一格会印出尖括号", name, field)
				continue
			}
			for _, lang := range i18n.All {
				if got := i18n.Load(lang).T(key); strings.HasPrefix(got, "⟨") {
					t.Errorf("%s[%q] = %q：%s 的语言包里没有它（页面上会印出尖括号）",
						name, field, key, lang)
				}
			}
		}
	}
}

// 渠道那一族**要全**：`by` 是调用方自报的口径，AGENTS.md 写着七个值
// （ai / bookmarklet / extension / web / import / cli）。而语言包里少了哪一档，
// 那一档在历史页上就印出尖括号——而历史页是"谁动过这台机器"的唯一证据。
func Test历史那一族的渠道都有一句话(t *testing.T) {
	want := []string{"ai", "bookmarklet", "extension", "web", "import", "cli"}
	for _, c := range want {
		if _, ok := byChannels[c]; !ok {
			t.Errorf("byChannels 里少了 %q：那一档在历史页上会印出尖括号", c)
		}
	}
}
