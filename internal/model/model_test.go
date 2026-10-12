package model

import (
	"strings"
	"testing"
)

func goodBookmarks() *Bookmarks {
	return &Bookmarks{
		Title: "发射台",
		Groups: []Group{{ID: "nas", Title: "NAS 应用", Layout: LayoutList, Items: []Item{
			{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000",
				Icon: "simple-icons:synology", Alias: []string{"nas"}, Tags: []string{}},
		}}},
		Search:  Search{Enabled: true, Default: "g", Engines: []SearchEngine{{Prefix: "g", Name: "Google", URL: "https://www.google.com/search?q=%s"}}},
		Weather: Weather{Enabled: false, Location: "上海市"},
	}
}

func TestGoodDataPasses(t *testing.T) {
	if err := goodBookmarks().Validate(); err != nil {
		t.Fatalf("合法数据被拒: %v", err)
	}
}

// spec §6.2：图标名的存在性只卡写路径。形状合法但索引里没有的名字，
// model.Validate 必须放过——否则一次图标库刷新就能把家里人的档案打成坏档。
//
// 名字取 ASCII slug 形状（`^[a-z0-9]+(-[a-z0-9]+)*$`），不是随手编的中文：
// checkIcon 本就要卡形状非法，拿一个形状就违例的名字当证人，会被以"形状非法"
// 拒掉（红错原因），证不到"model 不认索引"。必须是**形状合法、只是索引里没有**。
func TestValidate_索引外的图标名照样合法(t *testing.T) {
	b := goodBookmarks()
	b.Groups[0].Items[0].Icon = "tabler:not-a-real-icon-name"
	if err := b.Validate(); err != nil {
		t.Errorf("model 层不该认图标索引，它没有这个信息: %v", err)
	}
}

func TestRejects(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*Bookmarks)
		wantInErr string
	}{
		{"非 slug 分组 id", func(b *Bookmarks) { b.Groups[0].ID = "NAS 应用" }, "NAS 应用"},
		{"重复分组 id", func(b *Bookmarks) { g := b.Groups[0]; b.Groups = append(b.Groups, g) }, "重复"},
		{"非法 layout", func(b *Bookmarks) { b.Groups[0].Layout = "grid" }, "layout"},
		{"同组 item id 重复", func(b *Bookmarks) {
			it := b.Groups[0].Items[0]
			b.Groups[0].Items = append(b.Groups[0].Items, it)
		}, "重复"},
		{"跨分组 item id 重复", func(b *Bookmarks) {
			b.Groups = append(b.Groups, Group{ID: "dev", Title: "开发", Layout: LayoutList,
				Items: []Item{b.Groups[0].Items[0]}})
		}, "重复"},
		{"非 http/https 协议不再拒（spec §6.2 允许存但标警告，标由渲染层做）",
			func(b *Bookmarks) { b.Groups[0].Items[0].URL = "chrome://extensions/" }, ""},
		{"ftp 有主机名同样允许存", func(b *Bookmarks) { b.Groups[0].Items[0].URL = "ftp://10.0.0.3/" }, ""},
		{"空 URL", func(b *Bookmarks) { b.Groups[0].Items[0].URL = "" }, "url"},
		{"无主机名的 URL（chrome:// 后面空了）", func(b *Bookmarks) { b.Groups[0].Items[0].URL = "chrome://" }, "主机名"},
		{"mailto/tel 这类没有 host", func(b *Bookmarks) { b.Groups[0].Items[0].URL = "mailto:home@launchbay.lan" }, "主机名"},
		{"缺协议的相对路径", func(b *Bookmarks) { b.Groups[0].Items[0].URL = "/relative/path" }, "主机名"},
		{"url 解析失败", func(b *Bookmarks) { b.Groups[0].Items[0].URL = "://10.0.0.3" }, "解析失败"},
		{"缺 name", func(b *Bookmarks) { b.Groups[0].Items[0].Name = "" }, "name"},
		{"缺 group title", func(b *Bookmarks) { b.Groups[0].Title = " " }, "title"},
		{"运行时不许 MDI 图标名", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "mdi:download" }, "icon"},
		{"未知图标前缀", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "fa6-brands:github" }, "icon"},
		{"图标缺前缀", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "github" }, "icon"},
		// 这两条从"该报错"翻成"**不许**报错"，形状留着当反向证人：
		// search/weather/clock 没有任何渲染消费者，它们坏着也不能让整份档案读不出来
		// （校验失败的代价是坏档，而收益是零）。哪天 search 长出界面，这两条
		// 要连同界面一起翻回来——那时它们是缺陷，现在它们是越权。
		{"搜索引擎 url 缺 %s 不许毁掉整份档案", func(b *Bookmarks) { b.Search.Engines[0].URL = "https://x" }, ""},
		{"search default 悬空不许毁掉整份档案", func(b *Bookmarks) { b.Search.Default = "dd" }, ""},
		{"nil items 合法", func(b *Bookmarks) { b.Groups[0].Items = nil }, ""},
		{"icon 为空合法（走首字母）", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "" }, ""},
		{"icon=auto 合法", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "auto" }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := goodBookmarks()
			tc.mutate(b)
			err := b.Validate()
			if tc.wantInErr == "" {
				if err != nil {
					t.Fatalf("want accept, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantInErr)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Fatalf("error %q 不含 %q", err, tc.wantInErr)
			}
		})
	}
}

// 批量整理时，报错没有位置就等于让 AI 猜哪条错了。
//
// 规则变更（评审 Finding 2）：这条原本用 "telnet://x"（有 host）来触发
// 协议白名单拒绝；协议白名单已按 spec §6.2 取消，所以换成同样会真被拒
// 的"缺主机名"值——断言的是报错点名分组与条目，一点没减。
func TestErrorNamesLocation(t *testing.T) {
	b := goodBookmarks()
	b.Groups[0].Items[0].URL = "telnet://"
	err := b.Validate()
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"nas", "dsm"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("报错必须点出分组与条目，缺 %q: %v", want, err)
		}
	}
}

// 渲染层的判据：非 http/https 一律"需手动打开"，chrome:// 与大小写、
// 带端口的内网地址都要判对。
func TestExternalScheme(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"http://10.0.0.3:5000", false},
		{"https://example.com/a?b=1", false},
		{"chrome://extensions/", true},
		{"chrome://apps", true},
		{"ftp://10.0.0.3/", true},
		{"mailto:home@launchbay.lan", true},
		{"", true},
		{"://10.0.0.3", true}, // 解析失败也别当内部链接
	}
	for _, tc := range cases {
		if got := ExternalScheme(tc.url); got != tc.want {
			t.Fatalf("ExternalScheme(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestProfileValidate(t *testing.T) {
	base := Profile{ID: "dad", Title: "爸爸", PIN: "sha256$a$b", Theme: "auto", Layout: "sidebar"}
	if err := base.Validate(); err != nil {
		t.Fatalf("合法 profile 被拒: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*Profile)
	}{
		{"大写 id", func(p *Profile) { p.ID = "Dad" }},
		{"空 title", func(p *Profile) { p.Title = "" }},
		{"非法 theme", func(p *Profile) { p.Theme = "pink" }},
		{"非法 layout", func(p *Profile) { p.Layout = "topbar2" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mut(&p)
			if err := p.Validate(); err == nil {
				t.Fatalf("want reject: %+v", p)
			}
		})
	}
	// PIN **可以为空**（2026-10-12）：一份档案可以不设 PIN，此后进它不需要口令。
	// 这一格原先混在上面的拒绝表里——它挡的是"建档写了一半留下空哈希"，顺手把
	// "故意不设防"也挡了，而那正是这个产品要给的那种档案（公共电视屏、只读平板）。
	// 所以它翻成了正面用例，判据仍在 auth（见 auth 的 HasPin/VerifyEntry）。
	noPin := base
	noPin.PIN = ""
	if err := noPin.Validate(); err != nil {
		t.Fatalf("不设 PIN 的档案是合法的: %v", err)
	}
}

// theme/layout 两组枚举的判据必须只有一份：Profile.Validate 走 map，表单端点
// （httpapi 的 /prefs）走下面这两个谓词。这条钉的是"枚举只有一份"的消费者
// 形状——空串与大写在 map 与谓词里必须是同一个答案，否则表单端点会放行一个
// Validate 随后拒绝的值，写进去就是把整个档案打成坏档。
func TestValidThemeLayout_枚举只有一份(t *testing.T) {
	for th, want := range map[string]bool{"auto": true, "dark": true, "light": true,
		"": false, "Auto": false, "pink": false, "system": false} {
		if got := ValidTheme(th); got != want {
			t.Errorf("ValidTheme(%q) = %v，要 %v", th, got, want)
		}
	}
	for ly, want := range map[string]bool{"sidebar": true, "topbar": true,
		"": false, "Sidebar": false, "list": false} {
		if got := ValidLayout(ly); got != want {
			t.Errorf("ValidLayout(%q) = %v，要 %v", ly, got, want)
		}
	}
}
