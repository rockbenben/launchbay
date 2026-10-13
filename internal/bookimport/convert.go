package bookimport

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
)

// Options 是一次导入的输入。**只有一个出口函数 Convert**——其余全是内部，
// 导出面小到没法被误用。
type Options struct {
	// OverridesPath 是手工图标覆盖表；不存在也没关系（那次导入全靠猜）。
	OverridesPath string
}

// Result 是一次导入的全部产出。
type Result struct {
	Bookmarks *model.Bookmarks
	Report    *Report
}

// Convert 解析 Netscape 书签文件，产出书签与对账报告。
//
// 它只产出**一份**书签：模板机制 2026-10-10 整条拆掉之后，"一份共用的起步
// 清单"这件事本身不存在了，`Result` 里曾有的 Template 字段也没有了。
//
// **它不写盘**。写盘一律走 POST /api/v1/batch：快照、历史、rev 递增都在那条
// 通路内部发生，绕过它就等于造出第二份真相（AGENTS.md 的禁止事项第一条）。
// 调用方拿 Bookmarks 去生成 ops 提交即可。
func Convert(raw []byte, ix *icons.Index, opt Options) (*Result, error) {
	if ix == nil {
		return nil, fmt.Errorf("图标索引是 nil——没有它就猜不了图标，而导入必须能跑")
	}
	overrides, err := loadOverrides(opt.OverridesPath, ix)
	if err != nil {
		return nil, err
	}
	guesser := &iconGuesser{ix: ix, overrides: overrides}

	tree, parseNotes := parseNetbook(raw)
	rep := &Report{Parse: parseNotes, IconGaps: newIconGaps()}

	// 文件标题（<TITLE>）当档案标题。浏览器导出里它常是 "Bookmarks" 这种
	// 什么都没说的默认值——那种情况下留空，让人自己在界面上改，比把
	// "Bookmarks" 当成一个真标题写进档案好。
	fileTitle := strings.TrimSpace(treeTitle(raw))
	if fileTitle == "" || strings.EqualFold(fileTitle, "bookmarks") || strings.EqualFold(fileTitle, "书签") {
		fileTitle = ""
	}
	rep.FileTitle = fileTitle
	// 对账的源**只含会进档案的书签**。
	//
	// 回收站那些不在里面：它们被 parseNetbook 摘到了 tree.Trashed，本来就不进档案。
	// 早先我把它们也并进 src 想让第 2 条对账涵盖它们，结果是**每次导入都必然报
	// "丢了 N 条"**——而那 N 条是有意不导的。一条永远红的对账会让人把判据调松，
	// 调松之后真正丢的东西就跟着看不见了。
	//
	// 它们不是被"处理掉"了：单独计进 TrashedCount，并在报告里说清去哪了。
	src := flattenNb(tree.Root)
	rep.TrashedCount = len(flattenNb(tree.Trashed))

	bm, remaps, gaps := convertFolders(tree, src, guesser, rep)
	bm.Title = fileTitle
	rep.IconGaps = gaps
	rep.Remaps = remaps
	rep.SourceCount = len(src)
	rep.ResultCount = len(flatten(bm))

	// kept 是**真正落到档案里的那几条**（按 identity 与 bm 对上），对账与
	// "非 http 协议"清单都以它为准。
	//
	// 为什么不是拿 src 去对账：被挑出去的那几条是有意不导的（过不了写路径的
	// 判据），拿 src 去对就会**每次导入都报"丢了 N 条"**——而那 N 条是设计上
	// 没打算要的。一条永远红的对账会让人把判据调松，调松之后真正丢的东西就
	// 跟着看不见了。它们另有去处：Skipped 那一节逐条点名。
	groupOf := wantGroupOf(src, bm)
	kept := make([]srcItem, 0, len(src))
	for _, s := range src {
		if groupOf(s) != "" {
			kept = append(kept, s)
		}
	}
	rep.Source = kept
	rep.Rec = reconcile(kept, flatten(bm), groupOf)
	rep.Specials(kept)
	return &Result{Bookmarks: bm, Report: rep}, nil
}

// skippedItem 是一条**没能进档案**的书签。理由是判据给的原话（model.CheckURL），
// 不是我们自己编的分类——编出来的分类会与写路径那份判据慢慢分家。
type skippedItem struct {
	Name, URL, Reason string
}

// SkippedItem 是 skippedItem 的**对外形状**。它是个别名而不是一个新类型：
// 报告里的字段与判据都还在这一处，导出只是让"直接导入"那一页也能逐条点名
// （2026-10-12）。没有它的话，网页上那条路只能显示一个数字，于是用户既不知道
// 少了几条、也不知道少的是哪几条——而那正是导入唯一会让人后悔的信息。
type SkippedItem = skippedItem

// skipReason 说这条为什么进不来。空串 = 进得来。
//
// **判据只有 model.CheckURL 这一份**：写路径（ops → model.Validate）用它，
// 这里也用它，所以"它在这里被放过、到了写路径却被拒"这种形状不会发生。
// 空名字这一条是导入侧的：浏览器导出里 `<A HREF=...></A>` 真的存在，
// 而 model 的 name 判据同样不许空——两条拼起来才判得全。
//
// 为什么是"挑出去"而不是"整批拒绝"：写路径的整批拒绝（§6.2）是给 AI 与 CLI 的
// 纪律——一次改 30 条时"改坏一半"比"全没改"难收拾。导入不是同一件事：源文件是
// 用户几十年的收藏，一条 javascript: 链接不该让另外两百条进不来。
func skipReason(b nbBookmark) string {
	if strings.TrimSpace(b.Name) == "" {
		return "名字是空的"
	}
	if err := model.CheckURL(b.URL); err != nil {
		return err.Error()
	}
	return ""
}

// convertFolders 把文件夹树转成分组。
//
// **嵌套文件夹被拍平成顶层分组**，路径拼进标题（"工具 / 同步"）。理由：
// §4.3 的分组只有 id/title/layout/items 四样，没有父子关系，而寻址全靠 id
// （§4.4 不用下标）。硬造一层父子就得在 id 里编码路径，于是改名就成了改 id，
// 而 id 变了别人的链接就断了——那正是 slug 要"稳定"的原因。
// 拍平之后名字里还留着来路，谁都看得出这条原本在哪个文件夹下。
func convertFolders(tree *nbTree, src []srcItem, g *iconGuesser, rep *Report) (*model.Bookmarks, []remap, *iconGaps) {
	bm := &model.Bookmarks{Groups: []model.Group{}}
	remaps := []remap{}
	gaps := newIconGaps()
	// 这一轮攒下来、**等确认这一组真的建出来**才写进报告的 id 记录（见下面
	// "筛空了就不建"那道闸后面的两行）。
	var groupFallbacks []string
	var groupCollisions []slugCollision

	// 先收集所有文件夹的路径（标题要靠它），再统一分配 id —— slug 必须在
	// 全量标题上算，否则两个同名文件夹在不同批次里会拿到不同的 id。
	var folders []folderRef
	var walk func(f *nbFolder, prefix string)
	walk = func(f *nbFolder, prefix string) {
		for _, sub := range f.Subs {
			if sub.trashed {
				continue
			}
			name := strings.TrimSpace(sub.Name)
			if name == "" {
				name = "未命名"
			}
			path := name
			if prefix != "" {
				path = prefix + " / " + name
			}
			if len(sub.Items) > 0 {
				folders = append(folders, folderRef{path: path, items: sub.Items, toolbar: sub.toolbar})
			}
			walk(sub, path)
		}
	}
	walk(tree.Root, "")

	// 文件里顶层直接挂着书签（没在任何文件夹下）：给它们一个"书签栏"分组。
	var loose []nbBookmark
	loose = append(loose, tree.Root.Items...)
	if len(loose) > 0 {
		// toolbar 一并标上：这些书签按定义就挂在顶层、没有落在任何 <H3> 文件夹下，
		// 而这个分组是我们**合成**的、名字才叫「书签栏」（spec §9 对这张表的原话：
		// 「顶层没有文件夹的 <A> 归进一个合成的『书签栏』分组，layout: tiles」）。
		// 它不是从属性读来的，但位置就是书签栏那一层——不标它，这一组就退成 list，
		// 而它多半正是用户最常看的那一层。
		folders = append([]folderRef{{path: "书签栏", items: loose, toolbar: true}}, folders...)
	}
	if len(folders) == 0 {
		folders = []folderRef{{path: "书签", items: nil}}
	}

	slugs := slugifyAll(folderTitles(folders))
	seenTitle := map[string]int{}
	// usedItem 是**整个档案**范围内的条目 id 占用表，不是每组一张：model 的
	// 判据是"id 在整个档案内唯一"（move 之后按 id 寻址才有唯一解，§4.4）。
	// 每组各判一次，跨组同名的两条会撞成同一个 id 而校验只报一句。
	usedItem := map[string]bool{}

	for i, fr := range folders {
		title := fr.path
		// 同标题的文件夹合并成一个分组（浏览器导出里"书签栏"可能在多处出现）。
		if n := seenTitle[title]; n > 0 {
			rep.DuplicateTitles = append(rep.DuplicateTitles, dupTitle{title: title, n: n + 1})
		}
		seenTitle[title]++

		gr := model.Group{
			ID:    slugs[i].Slug,
			Title: title,
			// **每一组都从磁贴开始**（2026-10-13 改判，用户口径：「默认应该用磁吸」）。
			// 从前只有浏览器声明的"书签栏"那一层拿磁贴、子文件夹一律单列，
			// 于是导完一棵树，人看到的第一屏是十几个单列长条——而这个产品的外观
			// （极光玻璃卡片）本来就是为磁贴画的。
			// 单列仍然是**可选**的：每组标题右边那颗键切得过去，机器也能改
			// （ops 的 groups/<id>/layout）。默认值只是猜，猜错了要有人能改——
			// 这一条与 ⑤ 那把键存在的理由同一条。
			Layout: model.LayoutTiles,
			Items:  []model.Item{},
		}
		// id 那一格的记录**先攒在本地**，等下面那道"筛空了就不建"的闸过完再写进
		// 报告（2026-10-13 实拍改判）。从前直接写 `rep`，于是报告的「分组名与 id」
		// 一节列出了一个**根本不会被建出来**的 id——而同一份报告上面刚说过"那个
		// 文件夹也没建"。两句话并排，一句说没建、一句点名了它的 id，读者只会以为
		// 哪一句错了。
		if slugs[i].FellBack {
			groupFallbacks = append(groupFallbacks, slugs[i].Slug)
		}
		if slugs[i].Displaced != "" {
			groupCollisions = append(groupCollisions, slugCollision{
				slug: slugs[i].Slug, with: slugs[i].Displaced,
			})
		}

		for _, b := range fr.items {
			// 先筛再猜图标：一条进不来的书签不该在图标索引里翻一遍。
			if reason := skipReason(b); reason != "" {
				rep.Skipped = append(rep.Skipped, skippedItem{Name: b.Name, URL: b.URL, Reason: reason})
				continue
			}
			// id 是**第二个**筛：写路径按 model.ValidSlug 拒它，而这一层造出来
			// 的 id 有一道长度预算（itemID 上那段注释）。两层各判一次不是分家——
			// 两边用的是**同一个判据** model.ValidSlug，区别只在谁来跑它：这里跑，
			// 失败就是这一条点名跳过；真漏过去了，写路径会拒掉**整批**，而症状是
			// "有 N 条过不了校验，整批没有写入"——用户看到的是导入失败，却查不出
			// 是哪一条、为什么。这道筛就是那个"查得出"的地方。
			id := itemID(gr.ID, b, usedItem)
			if !model.ValidSlug(id) {
				rep.Skipped = append(rep.Skipped, skippedItem{
					Name: b.Name, URL: b.URL,
					Reason: "没法给它一个合法的 id（文件夹名太长，这条挤不进 id 的长度上限）",
				})
				continue
			}
			icon, how := g.guess(b.URL, b.Name)
			// 非 http 的地址**不计入图标缺口**：它在浏览器里点不动，本来就不该
			// 有图标（§9 特例一）。把它列进"想手动指定"的清单，只会让人去给一个
			// 点不动的链接配图标。
			if icon == "" && (strings.HasPrefix(b.URL, "http://") || strings.HasPrefix(b.URL, "https://")) {
				gaps.add(b.Name, b.URL, hostOf(b.URL))
			}
			gr.Items = append(gr.Items, model.Item{
				ID:    id,
				Name:  b.Name,
				URL:   b.URL,
				Icon:  icon,
				Desc:  b.Desc,
				Alias: []string{},
				Tags:  []string{},
			})
			if how != "" && icon != "" {
				remaps = append(remaps, remap{URL: b.URL, Name: b.Name, Kind: "icon", From: "", To: icon, Reason: "图标由" + how + "得出"})
			}
		}
		if len(gr.Items) == 0 && len(fr.items) > 0 {
			// 筛完空了的分组不留：侧栏上一个点进去什么都没有的文件夹，
			// 只会让用户以为"导漏了"。文件名在报告里点名，漏没漏看得见。
			rep.EmptyGroups = append(rep.EmptyGroups, title)
			continue
		}
		// 过完那道闸才写进报告：报告里点名的每一个 id，都得是**真的会建出来**的
		// 那一个。
		rep.SlugFallbacks = append(rep.SlugFallbacks, groupFallbacks...)
		groupFallbacks = groupFallbacks[:0]
		rep.SlugCollisions = append(rep.SlugCollisions, groupCollisions...)
		groupCollisions = groupCollisions[:0]
		bm.Groups = append(bm.Groups, gr)
	}
	return bm, remaps, gaps
}

// folderRef 是一个待转换的文件夹：path 是带祖先的完整标题，items 是直属书签。
type folderRef struct {
	path    string
	items   []nbBookmark
	toolbar bool // 浏览器声明的书签栏那一层（PERSONAL_TOOLBAR_FOLDER），见 nbFolder.toolbar
}

type dupTitle struct {
	title string
	n     int
}

type slugCollision struct {
	slug, with string
}

func folderTitles(fs []folderRef) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.path
	}
	return out
}

// itemID 给条目造一个稳定、合法的 id。
//
// **用分组 id + 书签名，而不是 URL**：改一次书签标题就会让 id 变，而 id 一变
// 别人存下来的链接就指空了；URL 虽然 rarely 变，却可能带着一长串查询参数，
// 进了 id 之后既难看又可能超长。
//
// **长度预算按 32 算**（model.ValidSlug 的上限）：`分组id-书签名` 这种拼法在真实
// 导出上必然超限——文件夹名三四十字符、书签名再叠一个"哔哩哔哩 - 那些年我追过
// 的番剧全集"，拼出来 46 个字符，于是**每一条都过不了写路径的校验**。
// 现在是把书签名截到"32 − len(分组id) − 1"为止，截完再退化到 URL slug、
// 再退化到 `item`。
//
// **唯一性在整个档案内判**（used 是全档案的表，model 的判据同此）：撞了就按
// `-2`、`-3` 递加，且递加时重新截断——不重新截的话，去重反而会造出一个超长的
// id，把"本来合法的 id"改成非法的。
//
// 预算不足那一档（分组 id 自己就占满 31/32 字节）2026-10-12 补：分号前缀**整个
// 去掉**。那不是"能少则少"的优化，而是"带前缀必然超长"——超长的 id 会被写路径
// 整条拒掉（"id 不是 ASCII slug"），于是**那一组里的书签一条都进不来**，
// 症状是导入走到最后一步回一句"有 N 条过不了校验，整批没有写入"。而这一档
// 产出的 id 今天本来就一条都不合法，所以改掉它动不到任何一份已经能用的档案。
func itemID(groupID string, b nbBookmark, used map[string]bool) string {
	room := slugMax - len(groupID) - 1
	// 退化档：分组 id 长到连一个字符都塞不下（它自己就顶满 slugMax）。名字 slug
	// 本身就是全档案唯一的候选（used 是全档案的表），所以前缀可有可无。
	if room < 1 {
		return takeID(nameSlug(b, slugMax-4), used)
	}
	return takeID(groupID+"-"+nameSlug(b, room), used)
}

// nameSlug 从书签上取一段可用的 slug：名字 → URL → `item`。
//
// 三段退化都在这里，于是"截多长"只有一个答案要写：调用方给预算，这段自己管
// 取不到字时退到哪。名字是纯中文、URL 里也没有可用字形时只剩 `item`——它照样
// 是合法 id，而空 id 会让整条 op 过不了写路径。
func nameSlug(b nbBookmark, room int) string {
	if s := clipSlug(slugify(b.Name), room); s != "" {
		return s
	}
	if s := clipSlug(slugify(b.URL), room); s != "" {
		return s
	}
	return clipSlug("item", room)
}

// takeID 占用一个 id：撞了就按 `-2`、`-3` 递加。
//
// 递加时截的是**整个 id**而不是书签名那一段——原来截 base 再拼分组前缀，分组前缀
// 本身占满预算时那样拼出来的仍然是超长的，而去重反而把一条本来合法的 id 改坏。
// suffixSlug 自己保证 ≤ slugMax，所以这里不必再截。
func takeID(id string, used map[string]bool) string {
	for n := 2; used[id]; n++ {
		id = suffixSlug(id, n)
	}
	used[id] = true
	return id
}

// flattenNb 压平文件夹树成书签列表。
//
// **nil 判据放在这里而不是每个调用方**：回收站是可选的，而"树里可能没有它"
// 是一件每个调用方都会遇到的事——散着判三遍，漏一处就是一个 panic。
func flattenNb(f *nbFolder) []srcItem {
	if f == nil {
		return nil
	}
	var out []srcItem
	var walk func(f *nbFolder)
	walk = func(f *nbFolder) {
		if f == nil {
			return
		}
		for _, b := range f.Items {
			out = append(out, srcItem{Name: b.Name, URL: b.URL, Desc: b.Desc, Category: f.Name})
		}
		for _, s := range f.Subs {
			walk(s)
		}
	}
	walk(f)
	return out
}

// wantGroupOf 按 (URL, Name) 说出每条**应该**落在哪个分组：就是它自己那一组。
// 导入不做分类重归（§9 允许，但那是目视判断），所以"应当"等于"实际"，
// 于是不会有分类调整被记进报告——真有调整时它会显形，而不是被这条规则吃掉。
func wantGroupOf(src []srcItem, bm *model.Bookmarks) func(srcItem) string {
	want := map[string]string{}
	for _, g := range bm.Groups {
		for _, it := range g.Items {
			want[identity(it.URL, it.Name)] = g.ID
		}
	}
	return func(s srcItem) string { return want[identity(s.URL, s.Name)] }
}

func treeTitle(raw []byte) string {
	_, _, title, _, _ := findTag(raw, "title")
	return strings.TrimSpace(title)
}

// findTag 在 raw 里找第一个 <name ...>…</name>，返回标签名、属性、文本、起止位置。
func findTag(raw []byte, want string) (name string, attrs map[string]string, text string, start, end int) {
	low := strings.ToLower(string(raw))
	open := "<" + want
	for i := 0; i < len(raw); {
		j := strings.Index(low[i:], open)
		if j < 0 {
			break
		}
		j += i
		name, a, next := scanTag(raw, j)
		if name != want {
			i = next
			continue
		}
		closeTag := "</" + want
		k := strings.Index(low[next:], closeTag)
		if k < 0 {
			return want, a, string(raw[next:]), j, len(raw)
		}
		return want, a, string(raw[next : next+k]), j, next + k
	}
	return "", nil, "", -1, -1
}

// schemeOf 的公开版：报告要拿它标出非 http 的地址。
func schemeOfURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme)
}
