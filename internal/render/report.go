package render

import (
	"html/template"
	"regexp"
	"strings"
)

// reportHTML 把导入报告那份 markdown 文本渲染成真的标记。
//
// 为什么要有这么一层：报告的文本是 CLI、扩展与页面**三个读者共用的一份**（判据只有
// bookimport.Report 那一份，不许为了页面再抄一遍句子）。可 markdown 在浏览器里不会
// 自己变成排版——实测页面上直接印着 `# 浏览器书签导入报告`、`## 结论`、`**没能进来**`、
// 以及成对的反引号，家里人读到的是标记而不是内容。
//
// 只认报告真正用到的那几种构造（`#`/`##` 标题、`-` 与 `1.` 列表、`**粗**`、“ `码` “），
// 别的字符一律当文字。**先转义再放标记**：转义只处理 < > &，所以文本里的引号不会被
// 换成 &#34;（那是旧写法在 <pre> 里由 html/template 顺手换的，页面上就露出实体码了）。
// ReportHTML is the exported door to the same renderer the import page uses.
// The browser extension posts to the API and gets back a report string; without
// this it would paste markdown into a <pre> and show the markup characters.
func ReportHTML(md string) template.HTML { return reportHTML(md) }

func reportHTML(md string) template.HTML {
	var out strings.Builder
	var list string // "" | "ul" | "ol"
	closeList := func() {
		if list != "" {
			out.WriteString("</" + list + ">")
			list = ""
		}
	}
	for _, raw := range strings.Split(md, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			closeList()
		case strings.HasPrefix(trimmed, "## "):
			closeList()
			out.WriteString("<h3 class=\"rep-h\">" + inline(trimmed[3:]) + "</h3>")
		case strings.HasPrefix(trimmed, "# "):
			closeList() // 文档级标题：这一页的 h1 已经说了"从浏览器导入收藏夹"，不再重复一遍
		case strings.HasPrefix(trimmed, "- "):
			if list != "ul" {
				closeList()
				out.WriteString("<ul class=\"rep-list\">")
				list = "ul"
			}
			out.WriteString("<li>" + inline(trimmed[2:]) + "</li>")
		case orderedItem.MatchString(trimmed):
			if list != "ol" {
				closeList()
				out.WriteString("<ol class=\"rep-list\">")
				list = "ol"
			}
			out.WriteString("<li>" + inline(orderedItem.FindStringSubmatch(trimmed)[2]) + "</li>")
		default:
			closeList()
			out.WriteString("<p>" + inline(trimmed) + "</p>")
		}
	}
	closeList()
	return template.HTML(out.String()) // 每个片段都各自转义过
}

// 有序列表项：`1. ` 开头。报告里只有对账那一节用它。
var orderedItem = regexp.MustCompile(`^(\d+)\. (.*)$`)

// inline 转义文本并放两种行内标记。顺序是判据：先转义，标记就只能由**我们**引入，
// 报告里出现的 `<script>` 之类只会以文字的面目显示出来。
func inline(s string) string {
	s = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
	s = codeSpan.ReplaceAllString(s, "<code>$1</code>")
	s = boldSpan.ReplaceAllString(s, "<b>$1</b>")
	return s
}

// 行内标记。反引号不能写进 Go 的反引号串，所以 codeSpan 用 \x60；它不许跨到第二个
// 反引号之外（报告里有 `{"host": "..."}` 这种带引号的对象字面量，贪婪匹配会把整句吞掉）。
var (
	codeSpan = regexp.MustCompile("\x60([^\x60]+)\x60")
	boldSpan = regexp.MustCompile(`\*\*(.+?)\*\*`)
)
