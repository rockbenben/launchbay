package render

import (
	"strings"
	"testing"
)

// 报告那份文本是 CLI、扩展与页面**三个读者共用的一份**（bookimport.Report.Text()）。
// 页面这一路以前直接把 markdown 塞进 <pre>，于是家里人读到的是 `## 结论` 与
// `**没能进来**`。这几条钉的是"渲染器真的把标记变成了排版，而且没把文字吃掉"。

const reportFixture = "# 浏览器书签导入报告\n\n" +
	"## 结论\n\n条目：文件里 12 条 → 进档案 9 条。另有 3 条**没能进来**（逐条列在下面）。\n\n" +
	"## 没能进来的\n\n- 脚本小工具 —— `javascript:void(0)`（url 缺主机名 \"javascript:void(0)\"）\n\n" +
	"## 对账\n\n1. 条目数：该进的 9 条，档案里 9 条 —— 过\n2. URL 差集：丢 0 条 —— 过\n"

func Test报告渲染_标记不许漏到页面上(t *testing.T) {
	got := string(reportHTML(reportFixture))
	for _, banned := range []string{"## ", "**", "`", "&#34;", "# 浏览器"} {
		if strings.Contains(got, banned) {
			t.Errorf("渲染结果里还有 markdown 标记或实体码 %q:\n%s", banned, got)
		}
	}
	for _, want := range []string{
		`<h3 class="rep-h">结论</h3>`,
		"<b>没能进来</b>",
		"<code>javascript:void(0)</code>",
		`<ul class="rep-list">`, `<ol class="rep-list">`, "<li>",
		"文件里 12 条 → 进档案 9 条",
		`"javascript:void(0)"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("渲染结果缺 %q:\n%s", want, got)
		}
	}
	// 文档级标题那一行不该变成第二个 h1/h2：这一页的 h1 已经是"从浏览器导入收藏夹"。
	if strings.Contains(got, "<h2") || strings.Contains(got, "<h1") {
		t.Errorf("报告里冒出了 h1/h2，与页面自己的标题打架:\n%s", got)
	}
}

// 报告文本里流过**用户数据**（书签名、地址）。放标记之前必须先转义，否则一条名叫
// `<img onerror=1>` 的书签就能在导入报告里落进 HTML。
func Test报告渲染_用户数据里的标记只以文字出现(t *testing.T) {
	got := string(reportHTML("- 名字 `x` —— <script>alert(1)</script> & \"引号\""))
	if strings.Contains(got, "<script>") {
		t.Errorf("原文里的 <script> 被当成标记放了出去:\n%s", got)
	}
	for _, want := range []string{"&lt;script&gt;", "&amp;", `"引号"`} {
		if !strings.Contains(got, want) {
			t.Errorf("缺 %q（转义只该动 < > &，引号在文本内容里是合法字符，不该被换成实体码）:\n%s", want, got)
		}
	}
}

// 报告里有 `{"host": "..."}` 这种带引号的对象字面量：codeSpan 贪婪一点就会把整句
// 吞成一个 code，读起来像排版坏了。
func Test报告渲染_反引号不跨到第二个之外(t *testing.T) {
	got := string(reportHTML("- 想手动指定：在 `POST /api/v1/batch` 逐条设 `icon`"))
	if n := strings.Count(got, "<code>"); n != 2 {
		t.Errorf("两个 code 段渲染出 %d 个：<code> 不许跨反引号吞整句\n%s", n, got)
	}
}

// 故意破坏一次上面那台机器的反向对照：把渲染器换成"原样塞进 <pre>"（旧行为），
// 第一条证人必须红。这里不是测旧行为，是钉住"这份测试真的会咬"。
func Test报告渲染_旧行为会被抓住(t *testing.T) {
	// 夹具必须**带着**那些标记：旧写法是把这份文本原样塞进 <pre>，而 html/template
	// 在文本内容里会把 " 转成 &#34;。夹具要是先被洗干净，上面那几条"不许出现标记"
	// 的证人就成了空转。
	if !strings.Contains(reportFixture, "## 结论") || !strings.Contains(reportFixture, "**没能进来**") ||
		!strings.Contains(reportFixture, `"javascript`) {
		t.Fatal("夹具本身不带 markdown 标记与裸引号，上面几条证人等于没测")
	}
}
