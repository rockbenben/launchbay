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

func Test设置面板显示镜像版本(t *testing.T) {
	previous := dockerVersion
	dockerVersion = "1.2.3"
	t.Cleanup(func() { dockerVersion = previous })

	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	r := renderer(t, s)

	page, err := r.PrefsPage("dad", "", i18n.Zh, "")
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)

	// 在**面板的最下面那一行**里（`.prefs-foot`）：不在顶栏（主视觉区），也不在
	// 档案页的页脚（那儿现在只剩用户自己那段文字了）。
	//
	// **那一行是 `<div>` 不是 `<p>`**：全局那条 `main p{…max-width:42em}`（说明句
	// 一行不许超过 42 字）会把 `<p>` 截到 630px——这一行要占满面板的宽度。
	i := strings.Index(body, `class="prefs-foot"`)
	if i < 0 {
		t.Fatalf("设置面板里没有最下面那一行（镜像版本所在的地方）")
	}
	foot := body[i:]
	if j := strings.Index(foot, "</div>"); j >= 0 {
		foot = foot[:j]
	}
	if !strings.Contains(foot, `href="`+RepoURL+`"`) {
		t.Errorf("那一行里没有那个仓库链接（要找 href=%q）", RepoURL)
	}
	// **新标签打开**：它是站外的链接，而这一页开着面板，离开就丢状态了；
	// 而**外链**不认 rel=noopener 的话，新开的标签能反过来操纵这一页（window.opener）。
	if !strings.Contains(foot, `rel="noopener noreferrer"`) {
		t.Errorf("那个链接没有 rel=\"noopener noreferrer\"：外链不认它就能反过来操纵这一页")
	}
	// **图标得有话**：图形按钮不带可及名，读屏软件只会念「链接」。而这一枚是
	// **图标 + 文字**，所以名字就是那句可见文字。
	if !strings.Contains(foot, `Docker <bdi>1.2.3</bdi>`) {
		t.Errorf("设置面板没有显示当前镜像版本：%s", foot)
	}
	if strings.Contains(foot, "aria-label=") {
		t.Errorf("那个链接带 aria-label：它的可见文字就是名字，再写一遍是同一句话说两遍")
	}
	// **「回我的页面」与它在同一行**（2026-10-21，用户要的"平行"）：两样东西
	// 都是这一页底部的——一个是出口，一个是引用。分两行就散了。
	if !strings.Contains(foot, `class="prefs-back"`) {
		t.Errorf("那一行里没有「回我的页面」那枚出口：它与源码入口该是同一行（用户要的「平行」）")
	}

	// **搬走不是复制**：档案页不再带它。
	prof, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(prof.Body), RepoURL) {
		t.Errorf("档案页上还有那个仓库链接：它已经搬进设置面板了，两处都有就成了两处都像二手的")
	}
}
