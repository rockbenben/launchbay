package httpapi

import (
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
)

// 英文界面里**两句话之间要有空格**（2026-10-19 补）。
//
// 模板把两条文案拼在一起（profile.tmpl 的删除提示、import.tmpl 的报告头），
// 中文不需要空格，英文需要。而中文那一侧修过——那半句原来带一个空格，上屏成了
// 「…一起没了。 删掉之后…」那种半角接缝（profile.tmpl 自己的注释记着这件事），
// 英文那一侧没跟上，于是读作 "…deleting it takes them too.You can bring this
// version back…"。
//
// **判据是整页扫**：英文界面任何一页出现「小写字母 + 句号/问号/叹号 + 大写字母」
// 就当场红。写成"检查那两处"的断言只能钉住已想到的那两处，而这一族的毛病正是
// **拼串**——下一个被拼在一起的两条文案会从别处冒出来。
var spliceRE = regexp.MustCompile(`[a-z][.!?][A-Z]`)

func Test英文界面没有粘在一起的两句话(t *testing.T) {
	s, h := setup(t)
	// 语言是机器级的，而**渲染缓存的键里没有语言**——所以要先把语言定下来，
	// 再让第一屏按它渲染（否则可能拿到另一种语言缓存下来的那一页）。
	if err := s.SetLanguage(i18n.En); err != nil {
		t.Fatal(err)
	}
	// 夹具里要**有分组**：删除提示那一行是分组里的东西，没有分组就没有那行字，
	// 于是这条闸会在最需要它的时候空转（这正是它要防的那种坏法）。
	if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 1, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList,
		Items: []model.Item{{ID: "a", Name: "好", URL: "http://10.0.0.1/", Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/prefs", "/history", "/revert"} {
		body := get(h, path, dadFormCookie).Body.String()
		text := stripTags(body)
		if m := spliceRE.FindString(text); m != "" {
			idx := strings.Index(text, m)
			t.Errorf("%s 的英文文案里两句话粘在一起了：…%s…（两段文案是拼起来的，"+
				"英文要一个空格，中文不要——中文那侧修过，英文这侧漏了）",
				path, text[max(0, idx-40):min(len(text), idx+40)])
		}
	}
}

// stripTags 去掉标签与脚本样式，只留人读得到的那层文字。
func stripTags(body string) string {
	body = regexp.MustCompile(`(?s)<script.*?</script>`).ReplaceAllString(body, " ")
	body = regexp.MustCompile(`(?s)<style.*?</style>`).ReplaceAllString(body, " ")
	body = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(body, " ")
	return regexp.MustCompile(`<[^>]+>`).ReplaceAllString(body, " ")
}
