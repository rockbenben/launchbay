package render

import (
	"strings"
	"testing"
)

// 面板里的表单**留在浮层里**（2026-10-14 实拍后补）。
//
// 实拍症状：在设置浮层里把「点开书签」改成「新标签页」，**整页跳到 /prefs 独立页**
// ——于是那屏从浮层变成了单独页面。根因：面板里只有 `form[action="/prefs/group"]`
// 被 app.js 拦下走 applyOps，其余四条一律**原生提交**，而服务端答 303 回 `/prefs`。
// 零 JS 时那正是对的（面板本来就没有），**浮层里就成了一次整页导航**。
//
// `/prefs/group` 那条早就在走 applyOps，而落点与那张表单**完全相同**
// （applyPrefsOps → applyBatch）——所以这不是新通路，是漏了同一份委托。
func Test面板里的表单留在浮层里而不是整页跳走(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	// 一：四条表单都要有委托。少一条就是"某一格按下去整页跳走"。
	for _, act := range []string{"/prefs/group", "/prefs/tabs", "/prefs", "/prefs/background"} {
		if !strings.Contains(js, `form[action="`+act+`"]`) {
			t.Errorf("浮层里没有 %s 那张表单的委托：在浮层里按一下它会整页跳到 /prefs", act)
		}
	}

	// 二：**每一条都 preventDefault**。少一条就是原生提交照旧发生。
	if strings.Count(js, "e.preventDefault();") < 4 {
		t.Error("preventDefault 少于四处：至少有一条表单没被拦下来")
	}

	// 三：「点开书签」走 **applyOps**（书签文档 → rev + 快照 + 审计），
	// 而不是自己发明一条 POST——服务端那条 /prefs/tabs 与这张表单落点相同。
	if !strings.Contains(js, "{ op: 'set', path: 'openInNewTab', value: v === '1' }") {
		t.Error("「点开书签」没有走 applyOps 的 set openInNewTab")
	}
	// 四：那一格**不整页重载**：它只改一个布尔，整页重载会把面板整个重建
	//（prefsLoaded 那一闩只在首次 fetch 后为真），"按一下 → 面板消失"与
	// "整页跳走"是同一种丢失。状态就地写在 aria-pressed 上。
	if !strings.Contains(js, "markPressed(form, v);") {
		t.Error("「点开书签」改完没有就地按 aria-pressed：那一格唯一的可见状态没人更新")
	}
	if !strings.Contains(js, "function markPressed(form, value)") {
		t.Error("markPressed 不在场")
	}

	// 五：`/prefs` 那四格**不能走 applyOps**——它们改的是 `profile.json`
	//（台面偏好，不产生 rev），而 applyOps 打的是 `/api/v1/batch`（书签文档），
	// 那是**另一份文件**。把两族混成一条就是"改了 A 却在 B 上生效"。
	if !strings.Contains(js, `form[action="/prefs"]`) {
		t.Error("改名/视图/导航/主题那四格没有自己的委托")
	}
	// 主题要**立刻见效**：这一格屏上原话是"按一下就生效，不产生快照"，
	// 而不整页重载的话只有 html 上的 data-theme 得自己换。
	if !strings.Contains(js, "document.documentElement.dataset.theme =") {
		t.Error("主题没有就地换 data-theme：不重载的话按了亮色页面还是暗的")
	}

	// 六：背景那一族是**唯一必须整页重载**的——三个动作各自要面板重画
	//（预览图、按钮组、"正在用它"那句话都会变），而这一格画不出自己。
	// 但**必须是显式的一处**：每族各写一句 reload 与"一律不重载"都读不出来。
	if !strings.Contains(js, `form[action="/prefs/background"]`) ||
		strings.Count(js, "location.reload();") < 1 {
		t.Error("背景那一族的整页重载不在场")
	}
	// 反向证人：**只有背景那一族**该重载。别族重载 = 面板消失。
	if strings.Contains(js, "if (ok) reloadKeepingPlace();\n      // 「点开书签」") {
		t.Error("「点开书签」又跟着整页重载了")
	}
}

// **浮层那条"字段标签占一行"不许波及面板左侧那三颗 tab**（2026-10-15 实拍）。
//
// `.sheet label{display:block}` 与 base.css 的 `.prefs-nav label{display:flex;
// align-items:center}` 权重相同（0,1,1），而 app.css 在 base.css 之后 → 前者赢。
// 于是同一份 define 在两个表面上长得不一样：独立页那三颗的文字在 44px 盒子里
// 居中（实测上留白 12 / 下留白 14），浮层里贴顶（1 / 27），那颗"当前在哪一节"的
// 主色竖条也从 flex 项掉回内联盒。浏览器里才看得出来，静态读 CSS 读不出来——
// 所以这条闸钉的是**形状**：那条规则必须带 `:has(...)`，裸的 `.sheet label` 一出现就红。
func TestCSS_浮层字段标签规则不波及面板tab(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/app.css"))
	if !strings.Contains(css, ".sheet label:has(") {
		t.Error("浮层的字段标签规则没有用 :has() 收窄（它会把面板左侧那三颗 tab 的 label 一起收成 block）")
	}
	// 扫每一处 `.sheet label`：下一个非空白字符必须是冒号（即 `:has(...)`）。
	// 不是冒号就是裸选择器，而它带 display 时面板那三颗会跟着遭殃。
	// （Go 的 RE2 没有先行断言，所以逐处看下一个字符。）
	for at := 0; ; {
		j := strings.Index(css[at:], ".sheet label")
		if j < 0 {
			break
		}
		at += j
		k := at + len(".sheet label")
		for k < len(css) && (css[k] == 32 || css[k] == 9 || css[k] == 10) {
			k++
		}
		if k >= len(css) || css[k] == ':' {
			at = k
			continue
		}
		body := css[at:]
		if end := strings.Index(css[at:], "}"); end > 0 {
			body = css[at : at+end+1]
		}
		if strings.Contains(body, "display") {
			t.Errorf("出现裸的 .sheet label 且它带 display——面板那三颗 tab 会被收成 block：%s", body)
		}
		at++
	}
	base := stripCSSComments(readWeb(t, "web/base.css"))
	if !strings.Contains(base, ".prefs-nav label{display:flex;align-items:center") {
		t.Error("base.css 里那条 tab label 的 flex 形状不在了——居中靠的就是它")
	}
}
