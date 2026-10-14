package render

import (
	"strings"
	"testing"
)

// 「地址这一格自己带一条错误」（2026-10-14 实拍后加）。
//
// 实拍到的症状：地址是表单**第一格**，而错误挂在**最底端**（在「保存」按钮下面），
// 中间隔着名字/描述/分组/排法/图标五格；而 resolve 是 **blur** 触发的，那时焦点
// 已经跳到「名字」——人正看着的那一格下面什么都没有，最远的一端亮着红字。
//
// 这一族钉的是**归位判据**：错误属于哪一格，就贴在那一格下面。
func Test地址类的错落在地址那一格而不是表单最底端(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)

	// 一：表单里要有「标签 + 控件 + 那一行错误」这一层结构。
	if !strings.Contains(js, `class: 'fld-group'`) {
		t.Error("表单里没有 .fld-group 那一层：错误贴着控件下面，需要一个把两者包起来并排成列的容器")
	}
	if !strings.Contains(js, `class: 'sheet-status bad fieldmsg'`) {
		t.Error("没有那一行字段错误元素（class 里要同时是状态行与字段消息）")
	}
	// 二：role=alert 而不是 status —— 字段错误是**断言**（要打断），
	// 底部那条是**进度**（不打断）。role 错了读屏的优先级就反了。
	if !strings.Contains(js, `class: 'sheet-status bad fieldmsg', role: 'alert'`) {
		t.Error("字段错误那一行不是 role=alert：它是断言，读屏应当立刻念；status 是留给进度行的")
	}

	// 三：**地址类的失败要显式选那一格**。判据是 kind 而不是状态码——
	// 422 同时是 batch 的「那一组已经不在了」，按码判会把一次保存失败也贴到地址。
	if !strings.Contains(js, "const URL_KINDS = new Set(['url-incomplete', 'field-invalid'])") {
		t.Error("没有 URL_KINDS 这份判据：按状态码判会把 batch 的 422 也贴到地址那一格")
	}
	if !strings.Contains(js, "'url-incomplete', 'field-invalid'") ||
		strings.Contains(js, "URL_KINDS = new Set(['target-unreachable'") {
		t.Error("URL_KINDS 的名单不对：target-unreachable 是「那台机器没应答」，地址本身是对的")
	}
	// 四：resolve 的两处失败出口都要接上落点（错误体那一支与抛异常那一支）。
	for _, at := range []string{"say(msg, 'bad', isURLProblem(r) ? urlErr : null)",
		"isURLProblem(e) ? urlErr : null"} {
		if !strings.Contains(js, at) {
			t.Errorf("resolve 的失败出口没有把地址类的错归位，缺这一句：%s", at)
		}
	}
	// 五：空地址也归那一格（它是唯一可能空的那一格），且要把焦点送回去。
	if !strings.Contains(js, "say(T('app.err.url'), 'bad', urlErr); fUrl.focus()") {
		t.Error("空地址的错误没有归位到地址那一格（也没把焦点送回那一格）")
	}

	// 六：**一敲就撤**。对着一个已经改对的地址继续被告知它不对，比不报错更坏。
	if !strings.Contains(js, "fUrl.addEventListener('input', clearUrlErr)") ||
		!strings.Contains(js, "fUrl.addEventListener('focus', clearUrlErr)") {
		t.Error("地址那一行错误不会在改动或回到那一格时撤掉：上一句会一直挂着")
	}

	// 七：空的时候不占位（`.fieldmsg:empty{display:none}`）——否则每张表单
	// 都多出一段 1.2em 的空白。
	cssRaw, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stripCSSComments(string(cssRaw)), ".sheet .fieldmsg:empty{display:none}") {
		t.Error("空的那一行会占位：没报错时表单也多出一段空白")
	}
}
