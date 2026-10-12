package model

import (
	"net/url"
	"testing"
)

// CheckURL 的判据逐条钉住（2026-10-12 补 `file:` 那一档之前，这个函数**一条测试
// 都没有**——它从"没有主机名就打不开"一路收到把本地文件也挡在门外，中间没有人
// 喊过疼，因为没有一条用例会因为这件事变红）。
//
// 两条线各管一格，别混：
//
//   - **能不能存**（CheckURL）：有主机名，或者 `file:` 且路径非空。
//   - **浏览器点不点得开**（ExternalScheme）：只有 http/https 能点，别的标
//     「需手动打开」。所以放行 `file:` 不是"放进来就能点"——它进来之后那枚
//     徽标仍然在，因为浏览器确实禁止从 http 页面跳 file:。
func TestCheckURL_哪些能存(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
		why  string
	}{
		{"https://example.com/a", true, "普通网页"},
		{"http://10.0.0.3:5000/", true, "内网带端口"},
		{"chrome://extensions/", true, "§6.2/§9 要求原样保留"},
		// 本地文件（2026-10-12）：地址在路径里，Host 恒为空，因为那台"机器"就是本机。
		{"file:///C:/Users/Administrator/Downloads/x.html", true, "Windows 本地路径"},
		{"file:///home/me/archive.html", true, "Linux 本地路径"},
		{"file:///D:/a b/x.html", true, "路径里有空格——空格不是拒绝的理由"},
		{"file://server/share/x.html", true, "UNC 共享：既有主机名也有路径"},
		{"file:///C:/x%20y.html", true, "百分号编码的空格"},
		// 仍然拒的：脚本与数据。点一下就在这一页里执行，不是"限制多了一点"。
		{"javascript:void(0)", false, "带脚本"},
		{"data:text/html,<script>x</script>", false, "带数据"},
		// 仍然拒的：动作不是目的地。
		{"mailto:a@b.c", false, "动作"},
		{"tel:13800000000", false, "动作"},
		// 缺关键部件的形状。
		{"", false, "空"},
		{"file://", false, "file: 但没有路径"},
		{"file://host-with-no-path", false, "file: 有主机名却没有路径"},
		{"http://", false, "只有协议"},
		{"/relative/path", false, "相对路径"},
		{"not a url", false, "根本不是 URL"},
	} {
		err := CheckURL(tc.url)
		if got := err == nil; got != tc.want {
			t.Errorf("CheckURL(%q) 能存=%v，要 %v（%s）%v", tc.url, got, tc.want, tc.why, err)
		}
	}
}

// 放行 `file:` 换来的必须是**能看见代价**，不是"以为点得开"：渲染层那条
// ExternalScheme 管的是浏览器点不点得开，file: 落在它眼里是"点不开"。
//
// 这一格是两道判据之间的分工证人：存储说"收"，呈现说"得自己复制到地址栏"。
// 只钉其中一半的话，改动另一半时没人会发现两块判据打架。
func TestCheckURL与ExternalScheme_各管一半(t *testing.T) {
	const local = "file:///C:/Users/Administrator/Downloads/x.html"
	if err := CheckURL(local); err != nil {
		t.Fatalf("本地文件该能存：%v", err)
	}
	if !ExternalScheme(local) {
		t.Error("本地文件从网页里点不开——徽标要留着，别因为它能存就以为能点")
	}
	// 反过来：带主机名的 chrome:// 能存，但同样点不开（§9 的特例）。
	if err := CheckURL("chrome://extensions/"); err != nil {
		t.Errorf("chrome://extensions/ 该能存：%v", err)
	}
	if !ExternalScheme("chrome://extensions/") {
		t.Error("chrome:// 也点不开，徽标要留着")
	}
}

// `file:` 的路径不许被悄悄改写：它是别人（用户的浏览器）给的原文，导进来又被
// 动过手脚的地址，在一堆本地文件里最难认出来。
func TestFileURL_原样保留(t *testing.T) {
	const local = "file:///D:/Downloads/トリセツ（取扱説明書）アーカイブス.html"
	b := &Bookmarks{Groups: []Group{{ID: "g", Title: "本地", Layout: LayoutList,
		Items: []Item{{ID: "i", Name: "本地网页", URL: local, Alias: []string{}, Tags: []string{}}}}}}
	if err := b.Validate(); err != nil {
		t.Fatalf("含本地文件的档案该合法：%v", err)
	}
	if got := b.Groups[0].Items[0].URL; got != local {
		t.Errorf("URL 被改写了：%q", got)
	}
	if _, err := url.Parse(local); err != nil {
		t.Errorf("这条 file: 地址连解析都不该失败：%v", err)
	}
}

// `file:` 不许被拿去抓图标：那台"机器"是**用户自己的电脑**，服务跑在 NAS 上。
// 这条判据住在 internal/icons 的 `Origin`（对非 http 返回空串），那边另有一条
// 钉它的用例——这里刻意不抄一份：判据只有一份，证人写在它自己那儿。
//
// 为什么特意点出来：放行 `file:` 之后，"能不能存"与"能不能抓到图标"分成了两件事，
// 而两件事的分界**不在** CheckURL 里。写在这里是为了让下一个人知道分界在哪，
// 而不是让他在这里再判一次。
