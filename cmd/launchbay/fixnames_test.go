package main

import "testing"

// hostNameFor 是 `profile fix-names` 全部的判断（2026-10-17）。
//
// 这一族的错分两类，而第二类比"少改几条"贵得多：
//
//	**少改**：该改的没改，人手动改那几条——代价是几条书签的名字。
//	**多改**：一个真实标题里带 URL 的（「AI 提示词库 | ChatGPT、DeepSeek 提示词…
//	  https://…」，那是这份收藏最值钱的一条），把它换成 `aishort.xyz` 等于**毁数据**。
//
// 所以这里钉的是"宁可少改"：只有名字本身就是地址（两种形状）才动，其余一个字都不碰。
func TestHostNameFor_只改名字本身就是地址的那些(t *testing.T) {
	for _, tc := range []struct {
		name, url, want string
		ok              bool
		why             string
	}{
		{"https://www.rfc-editor.org/info/rfc833391", "https://www.rfc-editor.org/info/rfc833391",
			"rfc-editor.org", true, "导入抓不到标题时最常见的形状"},
		{"http://10.0.0.5:8096/", "http://10.0.0.5:8096/",
			"10.0.0.5", true, "人把地址粘进名字格，且带尾斜杠"},

		{"AI 提示词库 | ChatGPT、DeepSeek 提示词… https://aishort.xyz/", "https://aishort.xyz/",
			"", false, "真实标题里带 URL：改掉是毁数据，不是整理"},
		{"群晖 DSM", "http://10.0.0.3:5000",
			"", false, "正常名字一律不动"},
		{"Hacker News", "https://news.ycombinator.com/",
			"", false, "正常名字一律不动"},
		{"example.com", "https://example.com/x",
			"", false, "名字已经是主机名，而那本来就是合理的短名字"},
		{"www.example.com", "https://www.example.com/x",
			"", false, "同上（带 www 也是主机名）"},
		{"", "https://example.com/", "", false, "没有名字就没有可改的"},
		{"x", "", "", false, "没有地址就判不了"},
		{"https://example.com/", "ftp://10.0.0.9/x",
			"", false, "名字自己是个 URL、但不是这一条的地址：按它的主机名改会写成别处的名字"},
		{"http://10.0.0.5:8096", "http://10.0.0.5:8096/",
			"10.0.0.5", true, "一边有尾斜杠一边没有，仍然是同一条"},
	} {
		got, ok := hostNameFor(tc.name, tc.url)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s / %s → (%q,%v)，要 (%q,%v)　%s",
				tc.name, tc.url, got, ok, tc.want, tc.ok, tc.why)
		}
	}
}

// 已经就是主机名的那些不改：命令对它们是**幂等**的，跑第二遍不再产生 op。
func TestHostNameFor_已经是主机名的算没得改(t *testing.T) {
	if got, ok := hostNameFor("example.com", "https://example.com/x"); ok {
		t.Errorf("已经是主机名的被判成可改（→%q）：跑第二遍还会再发一批 op", got)
	}
}
