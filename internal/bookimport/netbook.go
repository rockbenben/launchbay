package bookimport

import (
	"fmt"
	"html"
	"strings"
)

// 本文件是 **Netscape 书签文件**的解析器：Chrome / Edge / Firefox / Safari
// 都能导出的那个 .html（`<!DOCTYPE NETSCAPE-Bookmark-file-1>` 开头）。
//
// 它长得像 HTML，但**不是合法的 HTML**：`<DT>` 没有闭合标签，`<DL><p>` 与
// `</DL><p>` 后面跟着一个多余的 `<p>`，`<DD>` 后面也没有闭合。所以标准库的
// HTML 解析器（以及任何正经的解析器）在它面前都会当成畸形文档处理，而我们需要的
// 恰恰是**按 Netscape 当年那套约定**去读它。
//
// 因此这里手写一个只认四个标签的扫描器：`<DL>` / `<DT>` / `<H3>` / `<A>`，
// 外加一个 `<DD>`。别的标签（TITLE、H1、META、HR、注释）一律跳过——不是宽容，
// 是它们**确实不含书签数据**，跳过的部分仍然进 nbNotes 供报告点账。
//
// **唯一的外部依赖是标准库的 html.UnescapeString**：URL 里的 `&amp;` 不解码，
// 一半带查询参数的网址会存成错的，而它要等到哪天被人点开才发现。

// nbBookmark 是一条书签。
type nbBookmark struct {
	Name string
	URL  string
	Desc string
}

// nbFolder 是一个文件夹，Items 与 Subs 各自保持文件里的出现顺序。
type nbFolder struct {
	Name  string
	Items []nbBookmark
	Subs  []*nbFolder
	// trashed 标出浏览器的回收站。**用字段标而不是"名字叫回收站"或"全局
	// 单例"**：前者是判断，后者会让文件里两个同名回收站共享同一个对象、
	// 彼此的内容混在一起——而浏览器导出里确实可能同时出现"书签栏"和"其他书签"
	// 两棵树，各自带一个回收站。
	trashed bool
	// toolbar 标出浏览器的**书签栏**那一层，来自 Netscape 导出里标准的
	// `PERSONAL_TOOLBAR_FOLDER="true"` 属性（Chrome / Edge / Firefox 都打）。
	//
	// **用属性判而不是用名字判**：从前是 `strings.HasSuffix(title, "书签栏")`
	// ——那是一条中文硬编码，于是同一个产品里，中文 Chrome 导出的书签栏拿到
	// tiles，英文 Chrome（文件夹叫 "Bookmarks bar"）拿到的却是 list；反过来
	// 用户自己建一个叫"我的书签栏"的普通文件夹也会被误判成书签栏。属性是
	// 浏览器自己声明的，与界面语言无关，也不会撞上用户起的名字。
	toolbar bool
}

// nbNote 是解析过程中"没照单全收"的每一件事。
//
// **跳过必须记账**：解析器认不出的东西一旦被静默丢掉，那份导入报告会显示
// "迁了 42 条"而实际少了三条，而丢在哪一行没有任何记录。所以这个类型存在的意义
// 就是让"没导入"这件事永远有一张清单。
type nbNote struct {
	Line int
	What string
}

type nbNotes []nbNote

func (n *nbNotes) add(line int, format string, a ...any) {
	*n = append(*n, nbNote{Line: line, What: fmt.Sprintf(format, a...)})
}

// nbTree 是解析结果：根节点（合成出来的，不对应文件里任何一层）。
type nbTree struct {
	Root *nbFolder
	// Trashed 是浏览器"书签栏/其他书签"之外那个回收站。
	// **单独拆出来而不是混在树里**：它是用户已经删掉的东西，混进去等于
	// 把它复活——而导入报告里"凭空多出来"的恰恰是这类事故。
	Trashed *nbFolder
}

// parseNetbook 解析整个文件。
func parseNetbook(raw []byte) (*nbTree, nbNotes) {
	var notes nbNotes
	root := &nbFolder{Name: ""}

	// stack 是当前的 DL 层级；stack[0] 恒为 root。
	stack := []*nbFolder{root}
	// pending 是"刚遇到 <H3>、正等着它的 <DL> 到来"的那个文件夹。
	var pending *nbFolder
	// last 是"刚遇到 <A>、正等着 <DD> 补描述"的那条书签。
	var last *nbBookmark

	line := 1
	i := 0
	for i < len(raw) {
		c := raw[i]
		if c == '\n' {
			line++
			i++
			continue
		}
		if c != '<' {
			i++
			continue
		}
		// 注释 <!-- ... -->：整段跳过。里面不会有书签，但可能藏着
		// < 字符而不成标签——那才是需要跳过而不是误当标签的原因。
		if strings.HasPrefix(string(raw[i:min(i+4, len(raw))]), "<!--") {
			end := indexFrom(raw, i, "-->")
			if end < 0 {
				break
			}
			line += strings.Count(string(raw[i:end+3]), "\n")
			i = end + 3
			continue
		}
		name, attrs, next := scanTag(raw, i)
		if next <= i {
			// 畸形标签：一个字符一个字符往前走，而不是退出整个解析。
			// 退出的话，文件里一个坏标签会让后面所有书签都不被导入，
			// 而报告只会说"迁了 0 条"——看不出是被一个字符卡住的。
			notes.add(line, "第 %d 列有个读不出名字的标签，跳过", i)
			i++
			continue
		}
		i = next

		switch strings.ToUpper(name) {
		case "DL":
			// 三种 DL，各有各的处置：
			//  1. 紧跟在 <H3> 后面 → 那个文件夹自己的内容层；
			//  2. 栈底（文件里第一个 DL）→ 它就是根，**压 root 自己**，
			//     这样后面那个 </DL> 弹栈时才对得上。压一个匿名容器的话，
			//     收尾的 </DL> 会把栈弹穿，而且那个容器从没进过 root.Subs——
			//     树是空的，而报告只会说"迁了 0 条"（实测踩过）。
			//  3. 其余 → 真的是个没名字的文件夹，给它一个空名字并记账。
			switch {
			case pending != nil:
				stack = append(stack, pending)
				pending = nil
			case len(stack) == 1:
				stack = append(stack, stack[0])
			default:
				f := &nbFolder{}
				notes.add(line, "有一层 <DL> 前面没有 <H3>，按匿名容器处理")
				parent := stack[len(stack)-1]
				parent.Subs = append(parent.Subs, f)
				stack = append(stack, f)
			}
		case "/DL":
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			} else {
				notes.add(line, "多出一个 </DL>，栈已经到底了")
			}
		case "H3":
			f := &nbFolder{Name: html.UnescapeString(strings.TrimSpace(textBetween(raw, &i)))}
			// PERSONAL_TOOLBAR_FOLDER 是 Netscape 格式里给书签栏那一层打的标记。
			// 认属性不认名字，理由见 nbFolder.toolbar 那条。
			f.toolbar = strings.EqualFold(strings.TrimSpace(attrs["personal_toolbar_folder"]), "true")
			if f.Name == "" {
				notes.add(line, "有一个 <H3> 没有名字，按匿名容器处理")
				f.Name = ""
			}
			if isTrashedName(f.Name) {
				// 名字清空：它不进分组列表——回收站不是一个用户建的分组，
				// 在列表里留一条空名字的条目只会让人以为建错了一个组。
				f.Name = ""
				f.trashed = true
			}
			stack[len(stack)-1].Subs = append(stack[len(stack)-1].Subs, f)
			pending = f
		case "A":
			href := attrs["href"]
			if strings.TrimSpace(href) == "" {
				notes.add(line, "有一个 <A> 没有 HREF，跳过")
				continue
			}
			b := nbBookmark{
				Name: html.UnescapeString(strings.TrimSpace(textBetween(raw, &i))),
				URL:  html.UnescapeString(strings.TrimSpace(href)),
			}
			if b.Name == "" {
				// 无标题的 <A>：用 URL 顶上，而不是给一个空名字的书签——
				// 空名字在首屏是一个没有文字的方块，而 URL 至少能认出是哪个站。
				b.Name = b.URL
				notes.add(line, "有书签没有标题，用 URL 顶上：%s", b.URL)
			}
			cur := stack[len(stack)-1]
			cur.Items = append(cur.Items, b)
			last = &cur.Items[len(cur.Items)-1]
		case "DD":
			if last == nil {
				notes.add(line, "有一个 <DD> 前面没有书签，描述无处可去")
				continue
			}
			d := html.UnescapeString(strings.TrimSpace(textBetween(raw, &i)))
			if d != "" {
				if last.Desc != "" {
					// 同一 URL 有两条 <DD>：留着第一条并记账。
					// 改成"后一条覆盖前一条"会让导入结果取决于浏览器版本，
					// 而那是文件里的顺序，不是用户的意图。
					notes.add(line, "同一条书签有第二个 <DD>，只保留第一条")
				} else {
					last.Desc = d
				}
			}
		case "HR":
			// 分隔线，没有数据。
		default:
			// TITLE / H1 / META / P / BODY 之类：跳过即可，不记账——
			// 每一个都记账的话，报告会被上百条无意义的噪声淹没，
			// 而"没导入的东西"是真的会被读的那部分。
		}
	}
	if pending != nil {
		notes.add(line, "最后一个 <H3> 没有配对的 <DL>，它的内容算在上一层")
	}
	if len(stack) != 1 {
		notes.add(line, "文件结束时还有 %d 层 <DL> 没关上", len(stack)-1)
	}

	tree := &nbTree{Root: root}
	// 把回收站**从树上摘掉**，只留在 tree.Trashed 上。
	//
	// 留着、靠 trashed 标记区分也行，但那样每一个消费方都得记得判断那个标记——
	// 而漏判的后果是用户已经删掉的书签复活，并且出现在分组列表里。摘出去之后，
	// 遍历 Root.Subs 天然碰不到它，判据在**数据结构层面**而不是每个调用方的自觉。
	//
	// 文件里可能有多个回收站（每个书签栏各带一个）：全部并进 Trashed，跨树的
	// 同一批"已删除"合在一起看才是用户心里的那一份。
	var keep []*nbFolder
	for _, f := range root.Subs {
		if !f.trashed {
			keep = append(keep, f)
			continue
		}
		if tree.Trashed == nil {
			tree.Trashed = f
			continue
		}
		notes.add(line, "文件里有第二个回收站，并进同一份回收站清单")
		tree.Trashed.Items = append(tree.Trashed.Items, f.Items...)
		tree.Trashed.Subs = append(tree.Trashed.Subs, f.Subs...)
	}
	root.Subs = keep
	return tree, notes
}

// isTrashedName 认浏览器各语言写法的"回收站"。认不全不要紧：漏了的那些当普通
// 文件夹处理，用户看得见、也删得掉——而把正常文件夹误判成回收站（于是静默丢掉）
// 才是不可接受的那种错。
func isTrashedName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "回收站", "回收桶", "垃圾桶", "已删除的书签", "trash", "deleted", "trashed":
		return true
	}
	return false
}

// scanTag 从 i 处读一个标签，返回 (标签名, 属性表, 下一个位置)。
// 手工扫而不是用标准库：那些解析器会先"修正"文档，而 Netscape 文件需要的是
// **照原样读**，不是修正后的结果。
func scanTag(raw []byte, i int) (name string, attrs map[string]string, next int) {
	i++ // 跳过 '<'
	// 闭合标签的 '/' **必须先吃掉**，否则下面读名字的循环会在 '/' 上立刻停下，
	// name 变成空串——于是 `</DL>` 谁也不匹配，整棵树的层级从此错位
	// （实测症状是"文件结束时还有 N 层 <DL> 没关上"）。
	closing := false
	if i < len(raw) && raw[i] == '/' {
		closing = true
		i++
	}
	start := i
	for i < len(raw) && !isSpaceByte(raw[i]) && raw[i] != '>' && raw[i] != '/' {
		i++
	}
	name = strings.ToLower(string(raw[start:i]))
	if closing {
		name = "/" + name
	}
	attrs = map[string]string{}
	for i < len(raw) {
		for i < len(raw) && isSpaceByte(raw[i]) {
			i++
		}
		if i >= len(raw) {
			break
		}
		if raw[i] == '/' {
			// 自闭合标记：Netscape 文件里基本不出现，读到就跳过去。
			// 之前这里还往一个 selfClosing 出参里写，而那个出参从来没人读——
			// 一个没人读的出参就是"我当时以为谁需要它"的化石。
			i++
			continue
		}
		if raw[i] == '>' {
			i++
			break
		}
		// 属性名
		ks := i
		for i < len(raw) && !isSpaceByte(raw[i]) && raw[i] != '=' && raw[i] != '>' && raw[i] != '/' {
			i++
		}
		key := strings.ToLower(string(raw[ks:i]))
		for i < len(raw) && isSpaceByte(raw[i]) {
			i++
		}
		if key == "" || i >= len(raw) || raw[i] != '=' {
			continue // 布尔属性（HTML 里合法）：记下名字，值留空
		}
		i++ // 跳过 '='
		for i < len(raw) && isSpaceByte(raw[i]) {
			i++
		}
		if i >= len(raw) {
			break
		}
		var val string
		if q := raw[i]; q == '"' || q == '\'' {
			i++
			vs := i
			for i < len(raw) && raw[i] != q {
				i++
			}
			val = string(raw[vs:i])
			i++ // 跳过收尾的引号
		} else {
			// 无引号的值（HTML 允许，Netscape 文件里少见但存在）
			vs := i
			for i < len(raw) && !isSpaceByte(raw[i]) && raw[i] != '>' {
				i++
			}
			val = string(raw[vs:i])
		}
		if key != "" {
			attrs[key] = val
		}
	}
	return name, attrs, i
}

// textBetween 从 i 处读到"下一个条目开始"之前，返回那段文本并把 i 移到断点处。
//
// **闭合标签是跳过而不是断点**——这是实测踩出来的：`<H3 ...>工具</H3>` 里的
// `</H3>` 是同一个文件夹名的收尾，不是新条目开始。把它当断点，文件夹名就读成空，
// 而一个空名字的分组在界面上根本看不出来少了什么。
//
// 逐字节往 builder 里写而不是最后切一段 raw：跳过的标签如果不从结果里剔掉，
// 书签名就会变成 "工具</H3>"。
func textBetween(raw []byte, i *int) string {
	var b strings.Builder
	for *i < len(raw) {
		if raw[*i] == '<' {
			name, next := peekTagName(raw, *i)
			if isBlockTag(name) {
				break
			}
			*i = next
			continue
		}
		b.WriteByte(raw[*i])
		*i++
	}
	return b.String()
}

// peekTagName 只报"这里是什么标签"，不碰属性——textBetween 只关心它在不在
// 断点集合里，读属性是白费一次扫描。
//
// **返回值必须严格大于入参**，否则调用方 `*i = next` 就是原地踏步，整个解析
// 挂死在一个死循环里（实测踩过：注释 `<!--` 曾经让这里返回原位置）。
// 两道防线：注释跳到它结束之后；scanTag 万一没前进（畸形标签）就硬走一字节。
func peekTagName(raw []byte, i int) (string, int) {
	if i < len(raw) && strings.HasPrefix(string(raw[i:min(i+4, len(raw))]), "<!--") {
		if end := indexFrom(raw, i, "-->"); end >= 0 {
			return "!--", end + 3
		}
		return "!--", len(raw) // 没结尾的注释：后面没什么可解析的了
	}
	name, _, next := scanTag(raw, i)
	if next <= i {
		return name, i + 1
	}
	return name, next
}

// indexFrom 从 from 起找子串 s，返回第一个匹配的起点，找不到给 -1。
// strings.Index 在 []byte 上要转字符串，整份文件转一次是白费——书签文件能到几 MB。
func indexFrom(raw []byte, from int, s string) int {
	for k := from; k+len(s) <= len(raw); k++ {
		if string(raw[k:k+len(s)]) == s {
			return k
		}
	}
	return -1
}

// isBlockTag 判"这里是不是一个新条目 / 新层级的开头"，也就是 textBetween 的断点。
//
// **闭合的 </A> 与 </H3> 不在集合里**，而 </DL> 在：这个区别是层级对不对的分水岭。
// </A> 与 </H3> 是**同一条书签 / 同一个文件夹名的收尾**，读到它们就该跳过继续读文本；
// 而 </DL> 是**一层结束了**，它必须被主循环看到，否则这一层永远不弹栈——
// 症状是每一条书签都飘在最后一层里，而报告只说"还有 N 层 <DL> 没关上"
// （实测踩过，很难一眼看出根因，因为栈确实没弹，只是不该弹的那些弹了）。
func isBlockTag(name string) bool {
	switch name {
	case "dl", "/dl", "dt", "h3", "a", "dd", "hr", "p", "/p":
		return true
	}
	return false
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f'
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
