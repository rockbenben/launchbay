package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// RepoURL 指的是**这个仓库**——改了模块名而忘了改它，那道闸要红。
//
// 判据是 **go.mod 的 module 路径**：它与仓库地址是同一件事的两种写法
// （`github.com/<owner>/<repo>`），而 module 路径是机器读得出来的那一个。
//
// **为什么要有这条**：那个链接是给家里人点的（提需求、报问题），而它指向
// 哪里是**页面之外**的事——仓库改名或搬家之后，页面照旧画、链接照旧点得开，
// 只是去到了一个**别的仓库**（404，或者更糟：另一个项目）。
// 这种错平时完全看不出来：链接是绿的、页面是正常的。
func Test仓库地址与模块路径同源(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^module\s+(\S+)`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatalf("go.mod 里没有 module 行：%s", string(raw))
	}
	want := "https://" + m[1]
	if RepoURL != want {
		t.Errorf("RepoURL = %q，而 go.mod 的 module 路径是 %q ——那个链接指向的不是这个仓库", RepoURL, m[1])
	}
}

// **一处**：档案页**页脚**那一枚 GitHub 猫头。
//
// **为什么是页脚**（2026-10-20 的两轮改判）：网页上「源码/仓库」历来就在页脚，
// 而那是这一页**最不抢眼**的位置——顶栏那一排是「这台机器/这一份」的动作，源码
// 链接放那儿是抢主视觉区；而设置面板最下面那一处紧挨着「回我的页面」，挨着的
// 读起来像个导航项。
//
// **它是一枚图标，不带文字**：猫头是「代码/仓库」最通用的记号，而落点正好就是
// GitHub。所以判据不是"有一行字"，而是"有一个图形 + 一个可及名"——**图标换成
// 图标不等于丢掉名字**：读屏与悬停都得有话说。
//
// **可及名与语言包同源**：界面上那句与语言包里那把键逐字相同（`common.repoTitle`），
// 改文案只改一处。
func Test页脚带着仓库链接(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	r := renderer(t, s)

	page, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)

	// 在**页脚里**：不在顶栏（主视觉区），也不在别处。
	i := strings.Index(body, "<footer>")
	j := strings.Index(body, "</footer>")
	if i < 0 || j < 0 {
		t.Fatalf("这一页没有页脚（页脚是那一枚链接所在的地方）")
	}
	foot := body[i : j+len("</footer>")]
	if !strings.Contains(foot, `href="`+RepoURL+`"`) {
		t.Errorf("页脚里没有那个仓库链接（要找 href=%q）", RepoURL)
	}
	// 新标签打开：它是站外的链接，而这一页开着面板，离开就丢状态了；
	// 而**外链**不认 rel=noopener 的话，新开的标签能反过来操纵这一页（window.opener）。
	if !strings.Contains(foot, `rel="noopener noreferrer"`) {
		t.Errorf("页脚里那个链接没有 rel=\"noopener noreferrer\"：外链不认它就能反过来操纵这一页")
	}
	// **图标得有话**：图形按钮不带可及名，读屏软件只会念「链接」。
	m := regexp.MustCompile(`aria-label="([^"]+)"`).FindStringSubmatch(foot)
	if m == nil {
		t.Fatal("页脚里那个图标链接没有 aria-label：读屏软件只会念「链接」")
	}
	if want := i18n.Load(i18n.Zh).T("common.repoTitle"); m[1] != want {
		t.Errorf("页脚那个可及名是 %q，而语言包里那把键是 %q：两处不是同一句", m[1], want)
	}
}
