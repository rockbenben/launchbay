package bookimport

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// Report 是一次导入的全部结论。
//
// 它的组织原则只有一条：**读者最先要回答的是「迁干净了吗」**，所以对账在第
// 一节；"哪些东西我没照搬"紧随其后，因为那是迁不干净的**原因**；清单式的
// 细节放最后。把对账放最后的话，读者得先读完三屏 icon 缺口才看到结论。
type Report struct {
	// Words 是报告句子的来源，nil = 中文那一份，逐字不变（CLI 与不发
	// Accept-Language 的脚本读的正是它）。
	//
	// **为什么可以只给浏览器那一屏换语言**：AGENTS.md 把"导入的对账报告"划在
	// 不翻那一栏，理由是调用方要稳定可对照的文本——那句话成立的是**脚本**那一侧。
	// 同一个字符串现在也印在家人看的 `GET /import` 上（实测：英文站上那一屏仍有
	// 453 个汉字，节名一字未改，只有按钮翻了），于是那条豁免保的东西与它的读者
	// 分家了。收窄之后：句子按站点语言查表，**写路径的判定原话**（"没有分组 x"、
	// 校验理由那一类）仍是不变的中文——它们与 /api/v1/* 的 detail 是同一族，
	// 翻成两份就会漂。
	Words func(key string, args ...any) string

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

// t 查一句话。没注入 Words 就是中文那一份——那正是 CLI 与任何不发
// Accept-Language 的调用方一直在读的字节。
func (r *Report) t(key string, args ...any) string {
	if r.Words != nil {
		return r.Words(key, args...)
	}
	return i18n.Load(i18n.Zh).T(key, args...)
}

// Text 渲染成给人读的文本（markdown）。CLI、页面与扩展读的都是这一份，
// 区别只在 Words 给的是哪一本 catalog。
func (r *Report) Text() string {
	var b strings.Builder
	b.WriteString("# " + r.t("import.rep.docTitle") + "\n\n## " + r.t("import.rep.verdict") + "\n\n")

	// 1. 结论
	//
	// **先说完整画面，再说对账**（2026-10-13 实拍改判）。从前是「三条对账全过」
	// 打头、"另有 N 条没能进来" 跟在后面——于是读者第一眼读到的是"全过"，第二眼
	// 才看到少了东西，而这两句为什么能同时成立，报告从头到尾没有解释。
	//
	// 它**必须**解释，因为这一对词真会同时出现：对账判的是"该进的进了没"，
	// 而那 N 条是**判定它进不来**的。混在一起不解释，等于把一句"没丢东西"摆在
	// 一句"丢了东西"旁边，谁读都会怀疑自己看错了。
	fmt.Fprintf(&b, r.t("import.rep.items"), r.SourceCount, r.ResultCount)
	if n := len(r.Skipped); n > 0 {
		fmt.Fprint(&b, r.t("import.rep.skippedLead", n))
	}
	if r.TrashedCount > 0 {
		fmt.Fprint(&b, r.t("import.rep.trashedHead", r.TrashedCount))
	}
	b.WriteString("\n\n")
	if r.Rec.OK() {
		b.WriteString(r.t("import.rep.recOK"))
	} else {
		b.WriteString(r.t("import.rep.recFail"))
		for _, f := range r.Rec.Failures() {
			b.WriteString("- " + f + "\n")
		}
		b.WriteString("\n")
	}

	// 2. 没照搬的东西 —— 这是"迁不干净"的原因
	r.writeSkipped(&b)
	r.writeSpecials(&b)
	r.writeIconGaps(&b)
	r.writeParseNotes(&b)

	// 3. 分组与 slug
	if len(r.SlugFallbacks) > 0 || len(r.SlugCollisions) > 0 || len(r.DuplicateTitles) > 0 {
		b.WriteString("## " + r.t("import.rep.groupsH") + "\n\n")
		if len(r.SlugFallbacks) > 0 {
			fmt.Fprint(&b, r.t("import.rep.slugFallback", len(r.SlugFallbacks), strings.Join(r.SlugFallbacks, "、")))
		}
		for _, c := range r.SlugCollisions {
			fmt.Fprint(&b, r.t("import.rep.slugCollide", c.slug, c.with))
		}
		for _, d := range r.DuplicateTitles {
			fmt.Fprint(&b, r.t("import.rep.dupTitle", d.n, d.title))
		}
		b.WriteString("\n")
	}

	// 5. 清单式的细节
	if len(r.Remaps) > 0 {
		byKind := summarizeRemaps(r.Remaps)
		fmt.Fprint(&b, "## "+r.t("import.rep.detailH")+"\n\n")
		fmt.Fprint(&b, r.t("import.rep.detailLead", byKind["icon"], byKind["category"]))
		for _, x := range r.Remaps {
			// 空值**不能**写成一对反引号：markdown 里那叫"空的 code"，落到页面上
			// 会被行内规则从第二个反引号起配对，整行搅成
			// `icon `<code> → </code>simple-icons:youtube``（本轮实测撞到）。
			from := x.From
			if from == "" {
				from = r.t("import.rep.noneBefore")
			}
			fmt.Fprint(&b, r.t("import.rep.remapLine", x.Kind, from, x.To, x.Name))
		}
		b.WriteString("\n")
	}

	// 6. 对账三条
	b.WriteString("## " + r.t("import.rep.reconcileH") + "\n\n")
	fmt.Fprint(&b, r.t("import.rep.checkCount", r.Rec.CountSrc, r.Rec.CountDst, r.okFail(r.Rec.CountOK)))
	fmt.Fprint(&b, r.t("import.rep.checkURL", len(r.Rec.Missing), len(r.Rec.Extra),
		r.okFail(len(r.Rec.Missing) == 0 && len(r.Rec.Extra) == 0)))
	fmt.Fprint(&b, r.t("import.rep.checkFields", len(r.Rec.FieldDiffs), r.okFail(len(r.Rec.FieldDiffs) == 0)))
	b.WriteString(r.t("import.rep.checkNote"))
	return b.String()
}

func (r *Report) writeSpecials(b *strings.Builder) {
	if len(r.NonHTTP) == 0 && r.TrashedCount == 0 {
		return
	}
	b.WriteString("## " + r.t("import.rep.specialsH") + "\n\n")
	if len(r.NonHTTP) > 0 {
		fmt.Fprint(b, r.t("import.rep.nonHTTPLead", len(r.NonHTTP)))
		for _, n := range r.NonHTTP {
			fmt.Fprint(b, r.t("import.rep.itemLine", n.Name, n.URL, n.Scheme))
		}
		b.WriteString("\n")
	}
	if r.TrashedCount > 0 {
		// **这里曾经写着"它们进了共享模板"——那是句假话**：回收站的书签不进档案，
		// 而模板是从档案里挑出来的，于是它们哪儿都没去。一份报告里混进一句
		// "已经处理好了"的假话，比不提更糟：人会以为不用再管这件事。
		fmt.Fprint(b, r.t("import.rep.trashed", r.TrashedCount))
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
	b.WriteString("## " + r.t("import.rep.skippedH") + "\n\n")
	if len(r.Skipped) > 0 {
		fmt.Fprint(b, r.t("import.rep.skippedCountLead", len(r.Skipped)))
		for _, s := range r.Skipped {
			// 最后一格是**写路径的判定原话**，它不翻（与 /api/v1/* 的 detail 同族，
			// 两份就会漂），所以外面那句用站点语言把它框出来。
			fmt.Fprint(b, r.t("import.rep.skippedLine", s.Name, s.URL, s.Reason))
		}
		b.WriteString("\n" + r.t("import.rep.skippedExplain") + "\n\n")
	}
	if len(r.EmptyGroups) > 0 {
		fmt.Fprint(b, r.t("import.rep.emptyGroups", len(r.EmptyGroups), strings.Join(r.EmptyGroups, "、")))
	}
}

func (r *Report) writeIconGaps(b *strings.Builder) {
	if r.IconGaps == nil || r.IconGaps.n == 0 {
		return
	}
	fmt.Fprint(b, "## "+r.t("import.rep.iconGapsH", r.IconGaps.n)+"\n\n")
	fmt.Fprint(b, r.t("import.rep.iconGapsLead"))
	for _, g := range r.IconGaps.list() {
		fmt.Fprint(b, r.t("import.rep.hostLine", g.Host, g.Name))
	}
	// 这一句从前写着"这一版页面里改不了图标，导完之后用 POST /api/v1/batch 逐条设
	// icon"——那句话当时是真的（编辑模态只有三格），而它是把家里人往接口上指。
	// batch9 的 U7 给编辑那一格补上了图标，所以句子跟着事实改。
	b.WriteString("\n" + r.t("import.rep.iconAdvice") + "\n\n")
}

func (r *Report) writeParseNotes(b *strings.Builder) {
	if len(r.Parse) == 0 {
		return
	}
	fmt.Fprint(b, "## "+r.t("import.rep.parseH", len(r.Parse))+"\n\n")
	for _, n := range r.Parse {
		fmt.Fprint(b, r.t("import.rep.parseLine", n.Line, n.What))
	}
	b.WriteString("\n")
}

// okFail 是对账那一行的第 3 格。它**曾经**直写"过/不过"——那是界面词，不是
// 写路径的判定原话，所以英文报告里不许再出现它，改成查表（r.t 的 zh 兜底
// 与从前逐字相同，CLI 与扩展读到的字节不变）。
func (r *Report) okFail(ok bool) string {
	if ok {
		return r.t("import.rep.pass")
	}
	return r.t("import.rep.fail")
}
