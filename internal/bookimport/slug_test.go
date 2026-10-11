package bookimport

import (
	"strconv"
	"strings"
	"testing"
)

// slug 的头号性质是**稳定**：id 一旦写进 bookmarks.json 就成了别人的寻址依据
// （§4.4 不用下标），改它等于把人书签里的链接指空。所以"同输入永远同输出"
// 要是一条断言，不是一个愿望。
func TestSlugify_同输入永远同输出(t *testing.T) {
	for _, title := range []string{"NAS", "影音", "工具", "Media Server", "播客", "a/b:c", "  ", "ＡＢＣ"} {
		t.Run(title, func(t *testing.T) {
			first := slugify(title)
			for i := 0; i < 5; i++ {
				if got := slugify(title); got != first {
					t.Fatalf("第 %d 次算出 %q，第一次是 %q——同一份输入必须永远同一个结果", i, got, first)
				}
			}
		})
	}
}

// slug 只收 ASCII，且不许以分隔符开头结尾。id 要进 URL 与文件系统路径，
// 中文与符号在这两处都要额外的编码约定，产出了就是给自己埋雷。
func TestSlugify_只收ASCII且无首尾分隔符(t *testing.T) {
	for _, tc := range []struct{ title, want string }{
		{"NAS", "nas"},
		{"NAS 应用", "nas"},
		{"Media Server", "media-server"},
		{"a---b", "a-b"},
		{"  padded  ", "padded"},
		{"UPPER", "upper"},
		{"v1.2", "v1-2"},
		{"", ""},
		{"播客", ""}, // 全中文取不到 ASCII：由调用方退化，不许产出半角噪音
		{"!!!", ""},
	} {
		t.Run(tc.title, func(t *testing.T) {
			got := slugify(tc.title)
			if got != tc.want {
				t.Fatalf("slugify(%q) = %q，要 %q", tc.title, got, tc.want)
			}
			if got != "" {
				if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
					t.Errorf("不该有首尾分隔符：%q", got)
				}
				for _, r := range got {
					if r > 127 {
						t.Errorf("含非 ASCII 字符 %q：%q", r, got)
					}
				}
			}
		})
	}
}

// **want="" 那几支是防假绿的对照**：把 slugify 改成"什么都返回原串"，上面
// ASCII 的用例照样全绿，只有中文/空标点这几支会红。中文标题在 flare 的七个
// 分类里占多数，所以"取不到 ASCII"不是边角情况，是常态。
func TestSlugify_取不到ASCII必须为空而不是编一个(t *testing.T) {
	for _, title := range []string{"播客", "影音", "工具", "常用", "🔥"} {
		if got := slugify(title); got != "" {
			t.Errorf("slugify(%q) = %q，取不到 ASCII 时必须返回空让调用方退化，不能硬凑一个（拼音之类）——那会产出骗人的 id", title, got)
		}
	}
}

// 冲突必须**显式去重并记下顶替了谁**：两个分类撞成同一个 id 时，不去重就是
// 后者被寻址到前者身上——书书签进了错误的组，而报告里一个字都没有。
func TestSlugifyAll_冲突显式去重且记下被顶替者(t *testing.T) {
	titles := []string{"NAS", "nas", "NAS ", "影音"}
	got := slugifyAll(titles)
	if len(got) != len(titles) {
		t.Fatalf("结果条数 = %d，要 %d（下标必须与输入一一对应）", len(got), len(titles))
	}
	// 第 2 个与第 1 个撞：应被去重，且 Displaced 说的是**标题**不是下标
	// （报告里写"顶替了 NAS"，读的人才知道是谁；写"下标 0"没人看得懂）。
	if got[1].Slug == got[0].Slug {
		t.Errorf("第 2 个 slug %q 与第 1 个撞了，没去重", got[1].Slug)
	}
	if got[1].Displaced != "NAS" {
		t.Errorf("去重要记下被顶替者，要 \"NAS\"，实际 %q", got[1].Displaced)
	}
	if got[0].Displaced != "" {
		t.Errorf("第 1 个不该有 Displaced，实际 %q", got[0].Displaced)
	}
	// 全产出去重之后，id 必须互不相同——这是"书书签进错误的组"那道防线本身。
	seen := map[string]int{}
	for i, r := range got {
		if prev, dup := seen[r.Slug]; dup {
			t.Errorf("第 %d 个与第 %d 个 id 相同：%q", i, prev, r.Slug)
		}
		seen[r.Slug] = i
	}
}

// 全中文标题退化时，给的是 group-N 而不是空——空 id 会让整个分组无法寻址，
// 那是导入直接失败的原因。
func TestSlugifyAll_全中文退化但仍可用(t *testing.T) {
	got := slugifyAll([]string{"播客", "影音", "工具"})
	for i, r := range got {
		if r.Slug == "" {
			t.Fatalf("第 %d 个退化成空 id——那个分组就没法寻址了", i)
		}
		if !r.FellBack {
			t.Errorf("第 %d 个退化了却没记 FellBack，报告里会看不出它是被迫的", i)
		}
		if want := "group-" + strconv.Itoa(i+1); r.Slug != want {
			t.Errorf("第 %d 个 slug = %q，要 %q", i, r.Slug, want)
		}
	}
}
