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
	// 四：resolve 与保存的失败出口都要接上落点（错误体那一支、抛异常那一支、
	// 以及 submit 那一发）。少了 submit 这一支，同一句红字会同时出现在地址格
	// 与表单最底端（2026-10-24 实测）。
	for _, at := range []string{"say(msg, 'bad', isURLProblem(r) ? urlErr : null)",
		"isURLProblem(e) ? urlErr : null",
		"e.kind === 'url-incomplete' ? urlErr : null"} {
		if !strings.Contains(js, at) {
			t.Errorf("失败出口没有把地址类的错归位，缺这一句：%s", at)
		}
	}
	// 五：空地址也归那一格（它是唯一可能空的那一格），且要把焦点送回去。
	//
	// **顺序是「先送焦点，再写那句话」**（2026-10-22 改）：焦点那一格有个
	// 「一敲就撤」的规矩（clearUrlErr），而 `focus()` 会把焦点从「保存」搬回来——
	// 那一次 focus 事件会把**刚刚写上去的那句**当场擦掉，浮层里一个字不剩。
	// 反过来就对了：focus 事件先跑（撤掉上一轮的话），这一句才写上去。
	if !strings.Contains(js, "fUrl.focus(); say(T('app.err.url'), 'bad', urlErr)") {
		t.Error("空地址的错误没有归位到地址那一格（也没把焦点送回那一格），" +
			"或者写句子与送焦点的顺序反了——focus 事件会把刚写的那句擦掉")
	}
	// 五之二：**上面那一句要真能走到**（2026-10-22 补）。
	//
	// 原生表单校验会**抢在产品自己的校验前面**：地址那一格带着 `required` 时，
	// 浏览器拦下提交，浮层里一个字都没有，出来的是**浏览器自己的气泡**——而那句
	// **跟着浏览器界面语言走，不跟页面的语言走**。
	//
	// 实测（2026-10-22）：页面中文（`lang="zh-CN"`，顶栏「我的书签」）+ 浏览器
	// 界面英文（`navigator.language = en-US`）→ 气泡是 "Please fill out this
	// field."；而「先填地址。」那句（中英各一份）在浏览器里**永远到不了**。
	//
	// 判据是**那一格不许再带 required**：带了，上面第五句就白写。
	// 可及性不丢——required 顺带告诉读屏「这一格必填」，所以 aria-required 要在。
	urlDecl := `id: 'lb-url'`
	if strings.Contains(js, urlDecl) && strings.Contains(js, `required: 'required'`) {
		t.Error("地址那一格还带着 required：原生校验会抢在产品自己的校验前面，" +
			"浮层里一个字都没有，出来的是浏览器自己的气泡（跟着浏览器界面语言走，不跟页面走）")
	}
	if !strings.Contains(js, `'aria-required': 'true'`) {
		t.Error("地址那一格没有 aria-required：去掉 required 之后读屏就不知道这一格必填了")
	}
	// 五之三：**type="url" 自己也会拦提交**（2026-10-24 实测）。
	//
	// 去掉 required 还不够：敲 "abc" 按保存，浏览器照样先冒出「请输入网址。」
	// （跟着浏览器界面语言走），submit() 与产品自己的那句都到不了。表单必须
	// 带 novalidate——留着 type=url 的键盘与 autofill，把判据交回 submit()。
	if !strings.Contains(js, "novalidate: ''") {
		t.Error("添加浮层的表单没有 novalidate：type=url 会抢在 submit() 前面弹浏览器自己的气泡")
	}
	// 五之四：**一眼不是地址的输入在客户端就停住**（2026-10-24）。
	//
	// 去掉原生校验之后，敲 "abc" 会先跑一次 resolve、按保存时再跑一次 batch，
	// 两次都必然失败；这里提前拦，正式判据仍在服务端的 CheckURL。
	if !strings.Contains(js, "const parseableAddress = (s) => {") ||
		!strings.Contains(js, "if (!parseableAddress(url)) {") {
		t.Error("没有客户端地址形状闸：坏地址会照发 resolve 与 batch")
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
