package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

// 分组导航在「全局」「密集」这两档下不占那一列（2026-10-14）。
//
// 这一族钉的是三件事，而它们的症状都是"功能测试全绿、人看不出来"：
//  1. **样式**：那一列真的不画了（纯 CSS，`layout=topbar` 那一档一个字不动）；
//  2. **文案**：面板里那颗键在紧凑档下印的是「不显示」——印着「侧栏」而屏上什么
//     都没变，就是 §8.1 说的「点了没反应的东西不进界面」；
//  3. **判据只有一份**：紧凑 = view ∈ {wall, dense}，空串（没选过）不算。

func setView(t *testing.T, s *store.Store, id, view string) {
	t.Helper()
	if err := s.WithProfileLock(id, func() error {
		p, err := s.LoadProfile(id)
		if err != nil {
			return err
		}
		p.View, p.Layout = view, "sidebar"
		return s.SaveProfile(p)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCompactView_紧凑只有全局与密集两档(t *testing.T) {
	// **空串是分组视图**，而它也是升级前每一份 profile.json 的形状——把它算成
	// 紧凑档等于让所有老档案一升级就丢掉导航。
	for _, tc := range []struct {
		view string
		want bool
	}{
		{"", false}, {"groups", false}, {"wall", true}, {"dense", true},
		{"WALL", false}, {"wallx", false}, // 判据是**精确相等**，不是包含
	} {
		if got := compactView(tc.view); got != tc.want {
			t.Errorf("compactView(%q) = %v，要 %v", tc.view, got, tc.want)
		}
	}
}

func TestCSS_紧凑两档不画侧栏那一列(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	// 两条都要写全：**只写 .side{display:none} 会留下 206px 的空列**（栅格首列是
	// 写死的 206px，内容没了列还在），而那一列正是这一轮要收回来的东西。
	for _, want := range []string{
		"[data-profile-layout=sidebar][data-profile-view=wall] .side,",
		"[data-profile-layout=sidebar][data-profile-view=dense] .side{display:none}",
		"[data-profile-layout=sidebar][data-profile-view=wall] .shell,",
		"[data-profile-layout=sidebar][data-profile-view=dense] .shell{grid-template-columns:minmax(0,1fr)}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("base.css 里少了 %q", want)
		}
	}
	// **topbar 那一档一个字都不许动**：它是"想留着目录的人"选的出路，而它本来
	// 就是顶部横排一行。把它一起藏掉，这一轮就把那条出路也掐了。
	if strings.Contains(css, "[data-profile-layout=topbar][data-profile-view=wall] .side") ||
		strings.Contains(css, "[data-profile-layout=topbar][data-profile-view=dense] .side") {
		t.Error("topbar 那一档也被藏掉了：那正是紧凑档下留着目录的那条出路")
	}
	// 判据里必须带着 layout=sidebar。少了它，**选了顶栏的人也会丢掉那一行**——
	// 而他选顶栏的原因很可能正是"我要目录"。
	//
	// 逐行扫，而不是拿一个子串去 Index：`[data-profile-layout=sidebar][data-profile-view=dense] .side{display:none}`
	// 这一行**本身就包含** `[data-profile-view=dense] .side{display:none}`，用 Index
	// 判会把正确的那一条也当成漏判——这条断言自己先犯过一次（第一版就是这么写的，
	// 绿着一条错的话）。判据必须是"这一行里有没有 layout=sidebar"，而不是
	// "文件里有没有那个串"。
	for _, line := range strings.Split(css, "\n") {
		if !strings.Contains(line, ".side{display:none}") {
			continue
		}
		if !strings.Contains(line, "layout=sidebar") {
			t.Errorf("藏侧栏的这条规则没有带上 layout=sidebar，会连顶栏那一档一起藏掉：%s", strings.TrimSpace(line))
		}
	}
}

func TestPrefsPanel_紧凑档下说清那一列为什么不画(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的书签", "yt")

	// 分组视图（默认）：标签照旧，且**不解释**——那一列画得好好的
	setView(t, s, "dad", "")
	body := string(mustPrefs(t, s, "dad"))
	if !strings.Contains(body, `name="layout" value="sidebar" aria-pressed="true" title="把分组列表放回左边。">侧栏</button>`) {
		t.Errorf("分组视图下那一颗该印「侧栏」：%.300s", body)
	}
	if strings.Contains(body, "sidebarNote") || strings.Contains(body, "没有需要跳过去的节") {
		t.Error("分组视图下不该解释「为什么不画」——那一列画得好好的")
	}

	// 两档紧凑：**标签一个字都不许变**（那条不变量由
	// TestPrefs_开关的标签稳定而状态走aria 钉着），解释进下面那一句
	for _, view := range []string{"wall", "dense"} {
		setView(t, s, "dad", view)
		body = string(mustPrefs(t, s, "dad"))
		if !strings.Contains(body, `name="layout" value="sidebar" aria-pressed="true" title="把分组列表放回左边。">侧栏</button>`) {
			t.Errorf("%s 档下「侧栏」那颗变了样：可见文字必须稳定（标签随另一格的值变正是那条要挡的走样）", view)
		}
		if !strings.Contains(body, "左边那列用不上") {
			t.Errorf("%s 档下没有解释「那一列为什么不画」：人按了侧栏而屏上什么都没有", view)
		}
		// 「目录」在这产品里已经有了一个确定含义（数据目录），而这一列是**分组列表**
		// ——英文一直写的是 Group list position。所以中文也照着说「分组列表」
		// （2026-10-20 改：原来写「想留着目录」，读的人会当成文件目录）。
		if !strings.Contains(body, "想留着分组列表就选「顶栏」") {
			t.Errorf("%s 档下没有给出路：把分组列表整个拿掉而不说怎么留回来，人只会以为它没了", view)
		}
		if !strings.Contains(body, `name="layout" value="topbar"`) {
			t.Errorf("%s 档下「顶栏」那颗不见了", view)
		}
	}
}

// 档案页**照旧渲染那一列**——它是纯 CSS 的隐藏，而 DOM 里那份标记还在。这条钉的
// 正是"为什么用 CSS 而不是服务端不渲染"：服务端不渲染会让 JS 关掉时与开着时
// 长得不一样，而这里两档完全一致（CSS 是同一份内联 base.css）。
func TestProfilePage_侧栏的标记两档都还在只是被CSS藏了(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的书签", "yt")
	for _, view := range []string{"", "wall", "dense"} {
		setView(t, s, "dad", view)
		body := string(mustPage(t, s, "dad"))
		if !strings.Contains(body, `<aside class="side">`) {
			t.Errorf("view=%q：侧栏的标记不在 DOM 里（隐藏是 CSS 的事，服务端不渲染会让两档形状分家）", view)
		}
		if want := `data-profile-view="` + view + `"`; view != "" && !strings.Contains(body, want) {
			t.Errorf("view=%q：body 上没有 %s", view, want)
		}
	}
}
