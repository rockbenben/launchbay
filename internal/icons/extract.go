package icons

import (
	"html"
	"net/url"
	"strings"
	"unicode"
)

// maxTitleRunes / maxDescRunes：resolve 的结果是给人填表单用的候选，
// 不是存档。抓一个 800 字的 og:description 存进书签只会毁掉卡片布局。
const (
	maxTitleRunes = 160
	maxDescRunes  = 300
)

type Page struct {
	Title       string `json:"title"`
	Description string `json:"desc"`
	Favicon     string `json:"-"` // 只在内部用于抓图，不进响应
}

// Extract 从 HTML 里只取三样：<title>、og:description、favicon link。
// spec §6.1 写死了"不存正文、不留抓到的 HTML"，所以这个函数没有任何
// 返回整份文档的路径，也没有把 body 存进结构体的字段。
//
// 候选状态（best）是函数局部的：写成包级变量就等于让两次 Extract 互相污染，
// 而并发下它是真的会坏——resolve 会同时在几个会话里被调用。
func Extract(body []byte, base string) Page {
	src := string(body)
	lower := strings.ToLower(src)
	var p Page
	bestRank, bestHref := 9, ""

	if i := strings.Index(lower, "<title"); i >= 0 {
		if gt := tagEnd(lower, i); gt > 0 {
			rest := lower[gt+1:]
			if j := strings.Index(rest, "</title"); j >= 0 {
				p.Title = truncateRunes(cleanText(src[gt+1:gt+1+j]), maxTitleRunes)
			}
		}
	}

	// meta 与 link 共用一次标签扫描：属性解析要认引号，所以不能按 '>' 切。
	for _, tg := range scanTags(src, lower, "<meta", "<link") {
		attrs := parseAttrs(tg.body)
		switch tg.name {
		case "meta":
			// spec §6.1 写死了"只取 <title> 与 og:description"，描述只认
			// property="og:description"：朴素的 name="description" 不收。曾按
			// "站内页大量没配 og" 放宽同时认两种、并按文档序取——方向反了，brief
			// 与 spec 一致时 spec 是权威。只认 og 之后"og 优先于朴素"自动成立，
			// 顺序证人钉在 TestExtract_描述只认og且og优先，防止再被"兼容"放宽退化。
			if strings.EqualFold(attrs["property"], "og:description") && p.Description == "" {
				p.Description = truncateRunes(cleanText(attrs["content"]), maxDescRunes)
			}
		case "link":
			rel := strings.ToLower(attrs["rel"])
			if !strings.Contains(rel, "icon") {
				break // "stylesheet"、"preload" 之类
			}
			href := attrs["href"]
			if href == "" {
				break
			}
			// 优先级：apple-touch-icon > icon > shortcut icon。
			// 第一个通常 180px，缩到 24px 也还是一张清楚的小图；
			// 16px 的 favicon.ico 在高 DPI 屏上会糊成灰点。
			rank := 1
			switch {
			case strings.Contains(rel, "apple-touch-icon"):
				rank = 0
			case strings.Contains(rel, "shortcut"):
				rank = 2
			}
			if rank < bestRank {
				bestRank = rank
				bestHref = resolveHref(href, base)
			}
		}
	}
	p.Favicon = bestHref
	return p
}

type tag struct {
	name  string
	start int
	body  string // 含 '<' 与 '>' 的原样子串
}

// scanTags 按 "<name" 找标签，用 tagEnd 找到真正的 '>'。
func scanTags(src, lower string, opens ...string) []tag {
	var out []tag
	for _, open := range opens {
		for i := 0; ; {
			j := strings.Index(lower[i:], open)
			if j < 0 {
				break
			}
			k := i + j
			end := tagEnd(lower, k)
			if end < 0 {
				break
			}
			out = append(out, tag{name: open[1:], start: k, body: src[k : end+1]})
			i = end + 1
		}
	}
	return out
}

// tagEnd 返回从 i 处标签开始的那个 '>' 的下标，跳过引号内的 '>'。
// 这是 extract 唯一一处"自己扫字符串"的地方，也是 TestExtract_属性值里有
// 尖括号也不截断 存在的理由：`content="a>b"` 用正则按 '>' 切会当场截断正文。
func tagEnd(lower string, i int) int {
	var quote byte
	for j := i; j < len(lower); j++ {
		c := lower[j]
		switch {
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && c == '>':
			return j
		}
	}
	return -1
}

// parseAttrs 解一个标签里的属性。只认 name="value" / name='value' /
// name=value 三种写法，键统一小写——HTML 里 PROPERTY 与 property 是同一个东西。
//
// 标签名与属性的分界按**任意空白**（换行/制表都算）切，不按第一个 ' ' 切：
// 真实网页常用 `<meta\nproperty="og:description" …>` 的形状，按单空格切会把
// 换行前那段当标签名丢掉、整条属性的解析静默消失（B3，证人
// TestExtract_属性前是换行仍能解析）。
func parseAttrs(t string) map[string]string {
	out := map[string]string{}
	i := strings.IndexFunc(t[1:], unicode.IsSpace)
	if i < 0 {
		return out
	}
	src := t[i+1:]
	for src != "" {
		src = strings.TrimLeft(src, " \t\r\n")
		eq := strings.IndexByte(src, '=')
		if eq < 0 {
			break
		}
		key := strings.ToLower(strings.TrimRight(src[:eq], " \t\r\n"))
		src = src[eq+1:]
		var val string
		if len(src) > 0 && (src[0] == '"' || src[0] == '\'') {
			q := src[0]
			end := strings.IndexByte(src[1:], q)
			if end < 0 {
				break
			}
			val = src[1 : 1+end]
			src = src[end+2:]
		} else {
			sp := strings.IndexAny(src, " \t\r\n>")
			if sp < 0 {
				sp = len(src)
			}
			val = src[:sp]
			src = src[sp:]
		}
		if key != "" {
			out[key] = val
		}
	}
	return out
}

func resolveHref(href, base string) string {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return b.ResolveReference(ref).String()
}

// cleanText 只做去实体与折叠空白；限长由调用方按自己的上限截
// （标题 maxTitleRunes、描述 maxDescRunes，两个数不同，不能在这混为一谈）。
func cleanText(s string) string {
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ") // 折叠所有空白
	return strings.TrimSpace(s)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
