package render

import (
	"strings"
	"testing"
)

// ⌘K 面板**不许无条件跳 `location.href`**（2026-10-19）。
//
// 症状形状是 §8.1 那条「点了没反应的东西不进界面」，而且是最隐蔽的一档：
// 面板扫的是**全部** `li[data-item]`，模板把点不动的外部地址（chrome:// /
// file:// …）渲染成 `<code>` 纯文本，于是 `itemURL` 能把真实地址原样读回来。
// 到这一步都还是对的——原先那一行 `location.href = hit.url` 于是把
// `file:///…` 交给浏览器，Chrome 拒绝、页面原地不动、**一声不吭**。
//
// 而模板那一支早就处理对了：`.plain .badge.warn.ext` 那颗徽标被 app.js 就地
// 换成能按的复制键（app.js 里 `badge.replaceWith(btn)` 那一段），出路就是
// 复制到地址栏或拖到书签栏。于是同一个地址**在正文里能复制、在 ⌘K 里点了
// 没反应**——两条路分家，跟这一族里先前那几个是同一个病。
//
// 判据钉的是「跳转前必须先问 external」，不是一份协议名单：名单每加一个
// 协议就要回来改一次，而"这一行能不能点"在 DOM 上是现成的（有没有
// `<a href>`），模板与脚本读的是同一件事。
func TestAppJS_面板里点不动的地址不许拿去跳转(t *testing.T) {
	src := readAppJS(t)

	// 1) 那一行必须还在，且前面先判 external —— 顺序钉死：先跳再判等于没判。
	const run = "function cmdkRun(hit) {"
	at := strings.Index(src, run)
	if at < 0 {
		t.Fatalf("app.js 里找不到 cmdkRun：⌘K 按回车那一族没有落点了")
	}
	end := strings.Index(src[at:], "\n  }")
	if end < 0 {
		t.Fatal("cmdkRun 的函数体读不出来（这一族测试的判据本身失效了）")
	}
	body := src[at : at+end]
	// **只看代码行**：注释里写着为什么这么判（那段话里也出现 hit.external 与
	// location.href），拿整段去 Index 会命中注释而不是代码——那种测试第一次
	// 跑就红，改注释就绿，于是它其实什么都没钉。
	var code []string
	for _, line := range strings.Split(body, "\n") {
		if s := strings.TrimSpace(line); !strings.HasPrefix(s, "//") && s != "" {
			code = append(code, s)
		}
	}
	joined := strings.Join(code, "\n")

	judge := strings.Index(joined, "hit.external")
	jump := strings.Index(joined, "location.href = hit.url")
	if judge < 0 {
		t.Fatalf("cmdkRun 里没有判 hit.external：点不动的地址（chrome:// / file:// …）被无条件拿去跳转，按下去浏览器拒绝、页面原地不动\n%s", joined)
	}
	if jump < 0 {
		t.Fatalf("cmdkRun 里连正常跳转那一支都没了——修 external 判据时把跳转整个删掉的话，可点书签也点不动了\n%s", joined)
	}
	if judge > jump {
		t.Errorf("external 那一判排在跳转之后：先跳再判等于没判\n%s", joined)
	}

	// 2) external 那一支要落到复制上，而不是落空。
	if !strings.Contains(joined, "copyText(hit.url)") {
		t.Errorf("external 那一支没有复制地址：按下去什么都发生了，屏上却什么都没变——那只是把「没反应」换了个更隐蔽的形状\n%s", joined)
	}
}

// ⌘/Ctrl ↵ 是 `cmdkRun` 的**第二个入口**：同一份 `cmdkHits`、同一个
// `cmdkSel`，只是开成后台标签。它原先不判 external，于是同一个条目
// **按回车复制地址、按 ⌘↵ 照样跳**——两个键差一个字，行为却相反，
// 而"浏览器打不开这种地址"那个问题原样还在。
func TestAppJS_面板后台打开键与回车同判(t *testing.T) {
	src := readAppJS(t)
	at := strings.Index(src, "window.open(hit.url")
	if at < 0 {
		t.Fatal("app.js 里找不到 ⌘↵ 那一支的 window.open")
	}
	// 取那一行所在的一小段：判据必须与 hit 同一个条件式里，不能在别处
	lineStart := strings.LastIndex(src[:at], "\n")
	lineEnd := strings.Index(src[at:], "\n")
	if lineStart < 0 || lineEnd < 0 {
		t.Fatal("读不出那一行")
	}
	line := src[lineStart : at+lineEnd]
	if !strings.Contains(line, "!hit.external") {
		t.Errorf("⌘↵ 那一支没有排除 external：同一个条目按回车复制、按它却跳转\n%s", strings.TrimSpace(line))
	}
}

// external 的判据只有一处，且就是「这一行有没有 <a href>」——
// 模板把点不动的地址渲染成纯文本，所以链接形态的有无是现成的信号。
// 另写一份协议名单（`^chrome:` 之类）就是第二份真相，加协议时必漏一次。
func TestAppJS_面板判点不动的依据是卡片形态不是协议名单(t *testing.T) {
	src := readAppJS(t)
	if !strings.Contains(src, "external: !li.querySelector('a[href]')") {
		t.Error("面板没有把「external」绑到卡片形态上（没有 <a href> 就是点不动）：改成自己抄一份协议名单的话，名单外那个协议就会漏一次")
	}
	for _, bad := range []string{`hit.url.startsWith('chrome:')`, `hit.url.indexOf('file:') === 0`} {
		if strings.Contains(src, bad) {
			t.Errorf("面板里出现了按协议判外部地址的写法 %q：判据散成第二份真相", bad)
		}
	}
}

// 面板那一行得**先说清按下去会发生什么**。只复制不提示的话，地址确实进了
// 剪贴板、屏上却一点变化都没有，人只会当成没生效再按一次。
//
// 判据钉的是 **`app.cmdk.*` 而不是 `profile.external.badge`**：两份文案字面
// 一样，但 app.js 的 `APP_I18N` 只收语言包的 `app.*` 切片，用了 profile.* 会
// 让 `T` 返回 `⟨profile.external.badge⟩` —— 尖括号直接印在屏上。所以这一格
// 需要自己那条 app 文案，而不是"反正模板里有同一句"。
func TestAppJS_面板里点不动的条目标出需手动打开(t *testing.T) {
	src := readAppJS(t)
	if !strings.Contains(src, "T('app.cmdk.external')") {
		t.Error("⌘K 命中列表没给点不动的条目标「需手动打开」：按回车只是把地址复制走，屏上没有任何反馈")
	}
	// 反向证人：别图省事去用模板那一条。字面相同，但键族不同——APP_I18N 只收
	// app.*，profile.* 会让 T 返回 ⟨key⟩，那个尖括号会印在屏上。
	if strings.Contains(src, "T('profile.external.badge')") {
		t.Error("⌘K 用了 profile.* 的键：APP_I18N 是语言包的 app.* 切片，T 会返回 ⟨profile.external.badge⟩ 印在屏上")
	}
}
