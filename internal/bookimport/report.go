package bookimport

import (
	"fmt"
	"sort"
	"strings"
)

// Report 是一次导入的全部结论。
//
// 它的组织原则只有一条：**读者最先要回答的是「迁干净了吗」**，所以对账在第
// 一节；"哪些东西我没照搬"紧随其后，因为那是迁不干净的**原因**；清单式的
// 细节放最后。把对账放最后的话，读者得先读完三屏 icon 缺口才看到结论。
type Report struct {
	Parse           nbNotes // 解析时跳过/顶替掉的每一件事
	SourceCount     int
	ResultCount     int
	TrashedCount    int
	NonHTTP         []nonHTTP
	DuplicateTitles []dupTitle
	SlugFallbacks   []string
	SlugCollisions  []slugCollision
	IconGaps        *iconGaps
	Remaps          []remap
	Rec             reconcileReport
	Source          []srcItem
	// Skipped 是**没能进档案**的那几条，逐条带理由。EmptyGroups 是筛完一条
	// 都没剩下的文件夹标题。
	//
	// 它们单列一节而不是混进"对账"里：对账判的是"该进的进了没"，而这几条是
	// **判定它进不来**的——混进去会让三条对账永远不过，于是"不过"这件事
	// 就不再携带信息了。写进档案的那部分因此仍然可以理直气壮地说全过。
	Skipped     []skippedItem
	EmptyGroups []string
	// FileTitle 是书签文件里 <TITLE> 的内容。浏览器导出的默认值（"Bookmarks"）
	// 会被换成空串——把那种字符串当标题写进档案，等于替用户起了一个他没起的名。
	FileTitle string
}

type nonHTTP struct {
	Name, URL, Scheme string
}

// Specials 挑出 §9 那四类"必须显式标出而不修"的东西。
//
// 浏览器导出这一侧对应的是：**非 http 协议**（浏览器里点不动，URL 仍留着，
// 渲染时给「需手动打开」角标）、**回收站**（用户已经删了，不进档案）、
// **同一站多条共用一个图标名**（看着整齐，其实多半该各用各的 logo）。
//
// 它们一律**只标记、不自动修**：换图标、补描述、重分类都是目视判断，
// 机器替人做出来的选择大概率是错的，而错的图标比没有图标更难被发现。
func (r *Report) Specials(src []srcItem) {
	for _, s := range src {
		if sch := schemeOfURL(s.URL); sch != "http" && sch != "https" {
			r.NonHTTP = append(r.NonHTTP, nonHTTP{Name: s.Name, URL: s.URL, Scheme: sch})
		}
	}
	sort.Slice(r.NonHTTP, func(i, j int) bool { return r.NonHTTP[i].URL < r.NonHTTP[j].URL })

	sort.Slice(r.DuplicateTitles, func(i, j int) bool { return r.DuplicateTitles[i].title < r.DuplicateTitles[j].title })
	sort.Strings(r.SlugFallbacks)
	sort.Slice(r.SlugCollisions, func(i, j int) bool { return r.SlugCollisions[i].slug < r.SlugCollisions[j].slug })
}

// Text 渲染成给人读的文本。
func (r *Report) Text() string {
	var b strings.Builder
	b.WriteString("# 浏览器书签导入报告\n\n")

	// 1. 结论
	if r.Rec.OK() {
		b.WriteString("## 结论\n\n三条对账全过：条目数相等、URL 差集为空、name/desc 逐条一致。\n\n")
	} else {
		b.WriteString("## 结论：**没导干净**\n\n")
		for _, f := range r.Rec.Failures() {
			b.WriteString("- " + f + "\n")
		}
		b.WriteString("\n")
	}
	// 两个"源"以前打架：结论说"源 12 → 新库 9"，对账第 1 条说"源 9，新库 9 —— 过"。
	// 后者比的是**过滤后**的条目数，所以它永远过——"三条全过"于是掩盖了"12 条里
	// 3 条没进来"。改成各自说清自己数的是哪一批。
	fmt.Fprintf(&b, "条目：文件里 %d 条 → 进档案 %d 条。", r.SourceCount, r.ResultCount)
	if n := len(r.Skipped); n > 0 {
		fmt.Fprintf(&b, "另有 %d 条**没能进来**（过不了写路径的校验，逐条列在下面）。", n)
	}
	if r.TrashedCount > 0 {
		fmt.Fprintf(&b, "另有 %d 条在浏览器回收站里（用户已经删了），**没有进档案**。", r.TrashedCount)
	}
	b.WriteString("\n\n")

	// 2. 没照搬的东西 —— 这是"迁不干净"的原因
	r.writeSkipped(&b)
	r.writeSpecials(&b)
	r.writeIconGaps(&b)
	r.writeParseNotes(&b)

	// 3. 分组与 slug
	if len(r.SlugFallbacks) > 0 || len(r.SlugCollisions) > 0 || len(r.DuplicateTitles) > 0 {
		b.WriteString("## 分组名与 id\n\n")
		if len(r.SlugFallbacks) > 0 {
			fmt.Fprintf(&b, "- %d 个分组标题取不到 ASCII，id 退化成 `group-N`（%s）。改标题能改成 ASCII，id 就跟着变——**改 id 会让别人的链接指空**，所以先想清楚。\n",
				len(r.SlugFallbacks), strings.Join(r.SlugFallbacks, "、"))
		}
		for _, c := range r.SlugCollisions {
			fmt.Fprintf(&b, "- id `%s` 与「%s」撞了，已去重。\n", c.slug, c.with)
		}
		for _, d := range r.DuplicateTitles {
			fmt.Fprintf(&b, "- 有 %d 个分组同名「%s」，已合并成一个。\n", d.n, d.title)
		}
		b.WriteString("\n")
	}

	// 5. 清单式的细节
	if len(r.Remaps) > 0 {
		byKind := summarizeRemaps(r.Remaps)
		fmt.Fprintf(&b, "## 逐条明细\n\n图标 %d 条、分类调整 %d 条。\n\n", byKind["icon"], byKind["category"])
		for _, x := range r.Remaps {
			// 空值**不能**写成一对反引号：markdown 里那叫"空的 code"，落到页面上
			// 会被行内规则从第二个反引号起配对，整行搅成
			// `icon `<code> → </code>simple-icons:youtube``（本轮实测撞到）。
			from := x.From
			if from == "" {
				from = "原来没有"
			}
			fmt.Fprintf(&b, "- %s %s → `%s`（%s）\n", x.Kind, from, x.To, x.Name)
		}
		b.WriteString("\n")
	}

	// 6. 对账三条
	b.WriteString("## 对账（三条全过才算导完）\n\n")
	fmt.Fprintf(&b, "1. 条目数：该进的 %d 条，档案里 %d 条 —— %s\n", r.Rec.CountSrc, r.Rec.CountDst, okFail(r.Rec.CountOK))
	fmt.Fprintf(&b, "2. URL 差集：丢 %d 条，凭空多 %d 条 —— %s\n",
		len(r.Rec.Missing), len(r.Rec.Extra), okFail(len(r.Rec.Missing) == 0 && len(r.Rec.Extra) == 0))
	fmt.Fprintf(&b, "3. name/desc 逐条：不一致 %d 处 —— %s\n\n",
		len(r.Rec.FieldDiffs), okFail(len(r.Rec.FieldDiffs) == 0))
	b.WriteString("第 3 条只查 name 与 desc：图标**本来就该不一样**（源里没有图标名，是猜的），那是允许的整理动作，不在这里判红。\n")
	return b.String()
}

func (r *Report) writeSpecials(b *strings.Builder) {
	if len(r.NonHTTP) == 0 && r.TrashedCount == 0 {
		return
	}
	b.WriteString("## 没照搬的东西（只标出、没有自动改）\n\n")
	if len(r.NonHTTP) > 0 {
		fmt.Fprintf(b, "**非 http/https 协议 %d 条**——浏览器里点不动，URL 原样留着，渲染时会给「需手动打开」角标：\n\n", len(r.NonHTTP))
		for _, n := range r.NonHTTP {
			fmt.Fprintf(b, "- %s —— `%s`（%s）\n", n.Name, n.URL, n.Scheme)
		}
		b.WriteString("\n")
	}
	if r.TrashedCount > 0 {
		// **这里曾经写着"它们进了共享模板"——那是句假话**：回收站的书签不进档案，
		// 而模板是从档案里挑出来的，于是它们哪儿都没去。一份报告里混进一句
		// "已经处理好了"的假话，比不提更糟：人会以为不用再管这件事。
		fmt.Fprintf(b, "**回收站 %d 条**——你在浏览器里已经删过它们，所以没有进档案。它们仍然躺在源文件里；哪天想找回，翻浏览器自己的回收站更快。\n\n", r.TrashedCount)
	}
}

// writeSkipped 是"没能进来"那一节。它排在 specials 前面而不是后面：读者问的
// 第一个问题是"少了什么"，答案不该排在三屏图标缺口之后。
//
// 逐条点名 URL 与理由。**不许只给一个总数**——总数只告诉人"少了 6 条"，
// 不告诉人"是哪 6 条、为什么"，于是用户只能回浏览器里一个文件夹一个文件夹
// 地对。理由用的是写路径那份判据的原话，所以它与"为什么写不进去"是同一句。
func (r *Report) writeSkipped(b *strings.Builder) {
	if len(r.Skipped) == 0 && len(r.EmptyGroups) == 0 {
		return
	}
	b.WriteString("## 没能进来的\n\n")
	if len(r.Skipped) > 0 {
		fmt.Fprintf(b, "下面 %d 条**没有进档案**，理由逐条列着：\n\n", len(r.Skipped))
		for _, s := range r.Skipped {
			fmt.Fprintf(b, "- %s —— `%s`（%s）\n", s.Name, s.URL, s.Reason)
		}
		b.WriteString("\n这些大多是 `javascript:`、`data:`、`file:` 这类没有主机名的地址——" +
			"浏览器从网页里也点不开，写路径一律拒收。**它们没被改写、也没被藏起来**：源文件里原样还在，" +
			"想在浏览器里用就继续在浏览器里用。\n\n")
	}
	if len(r.EmptyGroups) > 0 {
		fmt.Fprintf(b, "另有 %d 个文件夹里的书签全没能进来，所以那个文件夹也没建（空文件夹在侧栏里只会让人以为导漏了）：%s\n\n",
			len(r.EmptyGroups), strings.Join(r.EmptyGroups, "、"))
	}
}

func (r *Report) writeIconGaps(b *strings.Builder) {
	if r.IconGaps == nil || r.IconGaps.n == 0 {
		return
	}
	fmt.Fprintf(b, "## 没猜到图标的 %d 条（走首字母色块）\n\n按网址猜的：拿 host 去内嵌图标索引里找，**猜不到就置空，不配一个形似的图标顶上**——后者会让这批书签看上去都有图标，直到有人逐条点开才发现全是错的。\n\n", r.IconGaps.n)
	for _, g := range r.IconGaps.list() {
		fmt.Fprintf(b, "- %s —— `%s`\n", g.Host, g.Name)
	}
	b.WriteString("\n这一版页面里改不了图标：想给某条补图标，导完之后用 POST /api/v1/batch 逐条设 icon，值要能在 GET /api/v1/icons?q= 里查到（查不到整批会被 422 拒掉）。\n\n")
}

func (r *Report) writeParseNotes(b *strings.Builder) {
	if len(r.Parse) == 0 {
		return
	}
	fmt.Fprintf(b, "## 解析时处理掉的 %d 处\n\n", len(r.Parse))
	for _, n := range r.Parse {
		fmt.Fprintf(b, "- 第 %d 行：%s\n", n.Line, n.What)
	}
	b.WriteString("\n")
}

func okFail(ok bool) string {
	if ok {
		return "过"
	}
	return "**不过**"
}
