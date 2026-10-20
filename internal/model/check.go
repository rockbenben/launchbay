package model

import (
	"errors"
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/url"
	"regexp"
	"strings"
)

// ErrValidation 让调用方能区分"形状合法但内容不合法"与"文件读不了"。
var ErrValidation = errors.New("数据不合法")

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ThemeValues / LayoutValues 是这两组枚举的**唯一来源**，按界面要呈现的顺序写。
// 判据（下面的 map 与 Valid*）和面向人的那句话（"只能是 auto|dark|light"）都
// 从这两张表生成：加一档只改这里一次，422 的拒绝语与校验文案自己跟着变。
// 端点里硬抄一份列表的话，下一次加档就会让拒绝语报出一句不含新档的话，
// 而判据明明已经认它（P1-④ 评审 I-6）。
var (
	ThemeValues  = []string{"auto", "dark", "light"}
	LayoutValues = []string{"sidebar", "topbar"}
	// ViewValues 是**整页怎么排**：groups = 一组一节（默认），
	// wall = 全局视图，各组作为列头、条目压成紧凑单行，多栏流式排满一屏。
	ViewValues = []string{"groups", "wall", "dense"}

	// AddrValues 是**卡片第二行印什么**（2026-10-19 加）：
	//
	//   - host（默认）：有说明印说明，没说明印主机名（`docs.google.com`）——
	//     这是 2026-10-16 实拍改判定下来的样子，48 条真实数据里只有 4 张有说明，
	//     而人认一个站点靠的恰恰是它在哪个域上。
	//   - full：第二行一律印**完整地址**。这是给人"我想看到多细"用的——同一台机器
	//     上三个人对这一格的要求几乎肯定不一样，而只有一个人能配。
	//   - none：一律不印第二行。想要纯图标墙、或者卡片已经够满的人用这一档。
	//
	// **它与 `view` 是两件事**：view 说的是整页怎么排（会不会本来就不印第二行），
	// 这一格说的是第二行**说什么**。所以 full 那一档要压过 view 自己的隐藏规则
	// ——否则这个开关在 wall/dense 与窄屏磁贴下会**开了没反应**，那正是 §8.1
	// 「点了没反应的东西不进界面」禁掉的那种东西（用户显式选了却拿不到，
	// 与"没选"分不开）。判据与代价见 render 那边那处注释。
	AddrValues = []string{AddrHost, AddrFull, AddrNone}

	// GroupLayoutValues 是**一个分组内部**条目的排法：tiles 磁贴 / list 单列 /
	// feature 大卡（大卡那一档会被渲染层挪到页面顶部独占一块，见 LayoutFeature）。
	//
	// 它与 LayoutValues（sidebar/topbar，分组**导航**放在哪）不是一件事，两个
	// 枚举都叫 "Layout" 是历史遗留——所以这一张带 Group 前缀，把"排的是哪一层"
	// 写进名字里。判据仍然只有一份：`ValidGroupLayout` 与 `ValidLayout`。
	GroupLayoutValues = []string{LayoutTiles, LayoutList, LayoutFeature}

	// MaxGroupTitleRunes 是**分组名**的长度上限，按字（rune）数而不是按字节——
	// 这一格写中文是常态，按字节算会让中文比英文少 3 倍额度而没人说得清为什么。
	//
	// **它住在 model 而不是某一层**：同一个数要出现在三处——服务端那条表单通路
	// 判它、模板上那个 maxlength 印它、openapi 若要写契约也得是它。三处各写一个
	// 字面量的话，改一处要记得改另外两处，而漏掉的那一处症状是"界面上限与实际
	// 判的对不上"（要么浏览器拦住了 curl 拦不住，要么反过来）。
	//
	// 为什么分组名要上限：它印在**每一个**分组标题、分组导航与导出文件里，
	// 而分组是这一页的一级结构——一个 800 字的组名会把导航那一行撑破。
	// 与 `Profile.PageTitle` 的上限（httpapi 的 maxPageTitleRunes = 60）同数，
	// 但那是两件事：那是"这一页叫什么"，这一格是"这一组叫什么"。
	MaxGroupTitleRunes = 60

	// GroupAccentColorKeys 给 GroupAccentPalette 那一排色块各配一个**语言包键名后缀**。
	// **它必须与 palette 一一对应、同序**：色块是看得见的颜色，看不见的名字，
	// 而读屏与色觉障碍的人拿到的只有这个名字——所以"这一颗叫什么"不是可选项。
	// 数目不等时下面 init 里的 panic 会当场点出是哪一张表。
	//
	// 这里存的是**键名而不是话**（2026-10-13 改判）：从前这张表直接写着「蓝 青 绿…」，
	// 于是英文界面上那八颗键的 aria-label 是中文单字——"文案只有一份，在语言包里"
	// 那一条纪律被一张 Go 表绕过了。句子归 `prefs.color.*`，本表只管配对。
	GroupAccentColorKeys = []string{
		"blue", "cyan", "green", "yellow", "orange", "red", "purple", "pink",
	}

	// GroupAccentPalette 是台面面板上那排色块的取值。**它不是白名单**：
	// accent 收任意 3/6 位 hex（render 的 hexColorRE），「自定义」那一格就是
	// 走这条的。这张表回答的是另一个问题——"大多数人选哪几个"，
	// 一家人手边没有取色器，让一个 color input 承担"给 30 个分组配色"这件事
	// 会让它在触摸设备上变成一个笨重的入口。
	//
	// **判据：每一颗在暗底与亮底都要过 WCAG 1.4.11 的 3:1**（非文本图形元素
	// 的下限）。色点是 `h2::before` 那个 10px 方块，而 `section{margin:13px 0}`
	// **没有自己的底**——它画在页面底（`--bg`）上，所以按 `--bg` 算。
	//
	// 2026-10-14 改判：原来的八颗（#4c8dff #22b8a0 #57b95a #d3a52c
	// #e8823c #e0574f #9b6ce8 #dd6aa6）暗档是 5.2–8.5:1，**亮档全部只有
	// 2.08–3.38:1**——除红与紫外，六颗连 3:1 都不到。而上面那句注释写着
	// "亮底与暗底两档都得看得见"，那是一句**没有量的断言**，它让人以为这一档
	// 已经验过。实测推翻它之后才动的值。
	//
	// **为什么八个 hex 不能按主题给两份**：面板上那排色块写进 bookmarks.json 的
	// `accent` 是**一个数**，渲染时也只有一份——按主题给两份等于把 accent 变成
	// (theme → hex)，而分组颜色这件事的整个前提是"一个分组一个颜色"（导出、
	// 复制、别人看你那一份）。所以取舍只能是"哪一档差一点"或"两边都退让"，
	// 这里选后者：六颗压暗到 **两档都 5.8:1 上下**，远宽于 3:1 的地板。
	// 代价是暗档从 7–8:1 降到 5.8:1 —— 那一档本来就是这个产品的主场景
	//（theme 默认 auto，跟随系统，而多数系统是暗色），5.8:1 仍远高于 3:1。
	//
	// **黄是最难的一颗**，也是多数产品干脆不给黄色的原因：它在亮底上要过
	// 3:1 就必须压到土黄（#ae8826 一类），色相已经不太"黄"了。保住它是因为
	// 八个色名一一对应（GroupAccentColorKeys），少一颗就要少一个可及名。
	//
	// **第一颗与各自 --accent 的关系**：暗档 --accent 是 #4c8dff、亮底是
	// #2a63bd，两者对各自页面底分别是 6.04:1 与 5.28:1。第一格取 #478aff
	// （暗档那一档的近邻），它在暗底上是 5.85:1、在亮底上 3.01:1 —— 与
	// 「跟随页面主色」并排时两者一眼能看出是不是同一个，这一点从前后两版都保留着。
	GroupAccentPalette = []string{
		"#4488ff", "#1e9c88", "#449e47", "#ab8626",
		"#de6b1b", "#e0574f", "#9b6ce7", "#db61a1",
	}
)

func init() {
	if len(GroupAccentPalette) != len(GroupAccentColorKeys) {
		panic("色板与色名键必须同数同序：加了一颗色忘了起名字，读屏就少一个可及名")
	}
}

// InGroupAccentPalette 判某个 accent 是不是色板上的那一档。它**只给界面用**
// （决定哪一颗是按下的、"自定义"那颗要不要亮着），不参与写入校验——
// accent 收任意 hex，"自定义"走的就是不在表里的那条路。
func InGroupAccentPalette(accent string) bool {
	for _, c := range GroupAccentPalette {
		if c == accent {
			return true
		}
	}
	return false
}

func valueSet(vs []string) map[string]bool {
	m := make(map[string]bool, len(vs))
	for _, v := range vs {
		m[v] = true
	}
	return m
}

var allowedTheme = valueSet(ThemeValues)
var allowedLayout = valueSet(LayoutValues)
var allowedView = valueSet(ViewValues)

var allowedAddr = valueSet(AddrValues)

var allowedGroupLayout = valueSet(GroupLayoutValues)

// ValidTheme / ValidLayout 是这两组枚举的唯一出口：校验走 map，
// 表单端点走这两个谓词——判据一份，两处消费。
func ValidTheme(s string) bool  { return allowedTheme[s] }
func ValidLayout(s string) bool { return allowedLayout[s] }

// ValidView 认的是**写侧**的值。读侧另有一条：空串是合法的"没选过"，
// 见 Profile.Validate 里那一句与 Profile.View 的注释。
func ValidView(s string) bool { return allowedView[s] }

// ValidAddrLevel 认**写侧**的值。读侧另有一条：空串是合法的"没选过"（= host），
// 见 Profile.Validate 与 Profile.AddrLevel 的注释。
func ValidAddrLevel(s string) bool { return allowedAddr[s] }

// ValidGroupLayout 认**一个分组自己的**排法。与 ValidLayout（导航放哪）分开，
// 是因为这两个"layout"改的是不同层的东西，而把它们合并成一个谓词的那一天，
// 422 的拒绝语就会开始报出一句与调用方无关的枚举。
func ValidGroupLayout(s string) bool { return allowedGroupLayout[s] }

func ValidSlug(s string) bool { return slugRE.MatchString(s) }

// accentHexRE 是 accent 的**权威**判据，两个方向各用一次：写侧
// `ValidAccent` 拒掉什么都不是的值（accent 是用户可写的字符串，而写进去
// 一个坏值只会得到"按下去什么都没变"——渲染侧会把它悄悄丢掉）；读侧
// `render.validatedAccent` 把盘上已经写坏的值也丢掉。
//
// **为什么不放在 render**：判据住在读侧时，写侧就永远够不着，于是"存下一个
// 垃圾颜色"是一条没人拦的路。两处各抄一份正则，则下一次改格式要记得改两处。
var accentHexRE = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// ValidAccent 认 #rgb / #rrggbb。**空串是合法的**——那是"没设色"，页面与侧栏
// 的色块退回页面主色（CSS 的 `--sec-accent,var(--accent)`）。它不是"无色"，
// "无色"要的是不画那个色块，那是另一件事、另一套判据。
func ValidAccent(s string) bool { return s == "" || accentHexRE.MatchString(s) }

// backgroundNameRE 是**这一页自己的背景图**那一个形状（Profile.Background），
// 与 accent 同一处置：判据住在 model，写侧（上传那条端点算出来的）与读侧
// （render 的净化）两处都够得着。
//
// **只收位图**：背景是铺满整屏的一块，svg 在这里没有任何理由（它要的是一张
// 照片/壁纸），而省掉它就省掉一整类执行面——服务端仍按裁定 R9 带 CSP sandbox，
// 但少一种后缀就少一处要守的形状。
//
// **哈希命名那半边也是这个形状的一部分**：不是 `<16hex>.<白名单后缀>` 的名字
// 一律不当背景，于是 profile.json 被手改成 `../../secret` 或 `index.json` 时
// 读侧只会说"这张背景不见了"，而不会去拼一条越界的路径。
var backgroundNameRE = regexp.MustCompile(`^[0-9a-f]{16}\.(png|jpg|webp|gif)$`)

// ValidBackground 认背景图那一个形状。**空串是合法的**（没设背景），与 ValidAccent
// 同一条处置：清背景与没设过是同一个值，模板据它决定要不要写那一段 style。
func ValidBackground(s string) bool { return s == "" || backgroundNameRE.MatchString(s) }

// CheckURL 只验"这串能不能当 URL 用"，不验协议白名单：spec §6.2 明写
// 非 http/https「允许存但标警告」，§9 要求 flare 的 chrome://extensions/
// 与 chrome://apps 原样保留。协议该不该被浏览器拒绝跳转是渲染层的事
// （见 ExternalScheme），在这里拒掉会让那些条目根本进不了存储。
//
// **导出是因为导入这条通路也要用同一份判据**：bookimport 在生成 ops 之前
// 拿它把过不了这一关的条目**逐条挑出去**（而不是让整批被写路径 422 掉）。
// 判据拷一份的话，两边迟早漂——漂的那天表现是"网页导不进去，命令行能导"。
//
// **`file:` 单独放行**（2026-10-12）：它的地址住在**路径**里而不在主机名里——
// `file:///D:/Downloads/x.html` 的 Host 恒为空，因为那台"机器"就是本机。按
// "必须有主机名"这条老判据，它与 `mailto:`、`javascript:` 一起被拒，于是**一个
// 人存了十年的本地网页收藏整份导不进来**——用户实测撞到的正是这个。
//
// 它安全且可用：路径非空就是一条真地址，渲染层按 ExternalScheme 标「需手动打开」
// （浏览器禁止从 http 页面跳 file:，那条标徽标的判据本来就管着），图标抓取那条
// 通路对非 http 直接返回空串，压根不会去读本机文件。
//
// **其余无主机名的协议仍然拒**：`javascript:` 与 `data:` 带的是脚本与数据，
// 点一下就在书签页里执行；`mailto:`/`tel:` 是动作不是目的地。放行它们换来的
// 不是"限制少一点"，是别人能往这一页塞脚本。
func CheckURL(u string) error {
	if u == "" {
		return errors.New("url 为空")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("url 解析失败 %q: %w", u, err)
	}
	// url.Parse 会把 scheme 小写化，所以这里比的是小写串。
	if parsed.Scheme == "file" {
		if parsed.Path == "" && parsed.Opaque == "" {
			return fmt.Errorf("url 缺路径 %q", u)
		}
		return nil
	}
	// 没有主机名就打不开："http://"、"/relative"、mailto:/tel: 这类无 host 的
	// scheme 都在这条上被拒。这仍是拒绝，不是"取消校验"。
	if parsed.Host == "" {
		return apierr.With(fmt.Errorf("url 缺主机名 %q", u), apierr.URLIncomplete)
	}
	return nil
}

// ExternalScheme 报告这个 URL 的协议是不是非 http/https（url.Parse 已把
// scheme 转小写）。浏览器禁止从网页跳这类地址，渲染层据此标「需手动
// 打开」而不是渲染 href——spec §6.2 的"标警告"、§9 的 chrome:// 特例。
//
// **名单是量出来的，不是印象**（2026-10-13 真点击实验，2026-10-14 扩到 28 个协议 ×
// Chrome + Edge 两个引擎）：在一个 http 页面上放一个链接、用真鼠标点下去（带用户手势，
// 因为 Chrome 对这类放行看的正是手势），然后看地址走没走、控制台说了什么：
//
//	chrome://version/     不走。security error「Not allowed to load local resource」
//	file:///C:/…          同上
//	view-source:…         同上
//	devtools://…           同上
//	blob:http://…         同上
//	edge://version        Edge 上同 chrome://（两个引擎把它当对方的内部页）
//	mailto:nobody@…       走。「Launched external handler for 'mailto:…'」，且不留标签
//	about:blank            走
//	http://example.com/    走（对照）
//
// **自定义应用协议那一族（vscode:// obsidian:// notion:// slack:// discord:// zoomus://
// msteams: spotify: steam:// whatsapp:// tg:// docker-desktop:// magnet: smb:// …）量出来
// 的是第三种结果：浏览器确实把点击交给了操作系统（这是它的策略，与装没装应用无关），
// 但**没装时用户看到的是"多一个停在 vscode://… 的死标签"**——有时控制台还会说
// 「Failed to launch … because the scheme does not have a registered handler」，有时一句
// 都不说。而**装没装应用，我们在渲染时无从知道**。
//
// 所以"能不能当普通链接点"与"我们认不认这个协议"是两件事，而**只有前者决定这一格
// 画成链接还是画成"需手动打开 + 复制键"**。把 mailto: 画成一条点不动的行，
// 是把一条本来会唤起邮件客户端的地址做成了一个假死链接；反过来把 chrome:// 或
// vscode:// 画成链接，得到的要么是控制台里一句报错、要么是一个停在协议地址上的死标签
// ——而这一类地址在 launchbay 里的真实用途几乎总是"给人一个能复制、能读的字符串"
// （本地路径、浏览器内部页、某个应用的协议地址），不是"点一下就跑起来"。这正是
// §8.1 要避免的「点了没反应的东西」。
//
// `tel:` 与 `sms:` 与 `mailto:` 走的是浏览器里同一条外部协议处理通路
// （external handler）：`mailto:` 在这台机器上量到的是干净交接（不留标签）。而
// tel:/sms: 在 **headless** 下留下的标签看不出是"交接成功"还是"没装"——headless 没有
// 系统 UI，那一档量不准；装了这两种处理器（Windows 的"手机连接"）的机器上它与 mailto:
// 同命运，所以一起放行。
func ExternalScheme(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		// 解析不了的字符串没资格当内部链接；交给渲染层标出来，
		// 校验通路自会拒它。
		return true
	}
	switch parsed.Scheme {
	case "http", "https", "mailto", "tel", "sms":
		return false
	default:
		return true
	}
}

// 图标名**不在这里验**（2026-10-12 整条删掉了 checkIcon，判据挪去写路径那一处）。
//
// 从前它验"形状与前缀"：`mdi:download` 这种不在 tabler/simple-icons 两层里的前缀
// 会让 `Validate` 报错，而 Validate 同时挂在**读路径**上（LoadBookmarks 与
// decodeProfile 都调它）——于是磁盘上一个不认识的图标名拥有毁掉一个人全部书签的
// 权力：整档 503「数据文件读不了」，页面上只剩"去修那个文件"。这不是假想：
// 这台机器的**前身**（NAS 上那套 flare）图标名整份都是 `mdi:` 前缀，谁把手边那份
// 文件恢复回来，谁就撞这一枪。
//
// 同一件事 §6.2 已经判过一次（坑 #3：`search.default` 写错让全档案进坏档页，摘的
// 就是校验），图标前缀是那条判据的漏网之鱼——原先那段注释说"存在性不在这里查"
// 是对的，而**前缀**恰好是存在性的近亲：它同样只有"索引里有没有"这一个含义。
//
// 现在读侧对 icon 一个字都不验，渲染层自己兜底（icon.go 的 pieceFor：索引查不到
// → 首字母色块 + 「图标未匹配」徽标，并计入 GET /api/v1/status 的 iconUnmatched）。
// 写侧照旧严格：ops 的 checkIconName 要求非 ""/"auto" 的名字**必须在索引里**
// （internal/ops/ops.go），那比"前缀对不对"强得多，且只作用在真要落盘的那批操作上。
//
// 运行时只带 tabler 与 simple-icons 两套（spec §5），MDI 只在导入器里当别名源
// （bookimport/aliases.go）——那条边界由**索引里有什么**与 ops 的存在性闸门共同守，
// 不再由读路径的校验守。
//
// 少了这道校验有没有注入面？没有：icon 这个字符串从不进 markup——索引命中就画内联
// SVG（SVG 来自内嵌索引，不是用户字节），没命中就画首字母，缓存 favicon 的路径由
// store 的内容哈希拼（`<16 hex>.<ext>`），三取一没有一条拼用户串。

func (b *Bookmarks) Validate() error {
	seenGroup := map[string]bool{}
	seenItem := map[string]bool{}
	for gi := range b.Groups {
		g := &b.Groups[gi]
		if !ValidSlug(g.ID) {
			return fmt.Errorf("%w: group %q: id 不是 ASCII slug", ErrValidation, g.ID)
		}
		if seenGroup[g.ID] {
			return apierr.With(fmt.Errorf("%w: group id 重复: %q", ErrValidation, g.ID), apierr.DuplicateID)
		}
		seenGroup[g.ID] = true
		if strings.TrimSpace(g.Title) == "" {
			return fmt.Errorf("%w: group %q: title 为空", ErrValidation, g.ID)
		}
		if !ValidGroupLayout(g.Layout) {
			return fmt.Errorf("%w: group %q: layout %q 只能是 %s", ErrValidation, g.ID, g.Layout,
				strings.Join(GroupLayoutValues, "/"))
		}
		for ii := range g.Items {
			it := &g.Items[ii]
			if !ValidSlug(it.ID) {
				return fmt.Errorf("%w: group %q item %d: id %q 不是 ASCII slug", ErrValidation, g.ID, ii, it.ID)
			}
			// item id 在整个档案内唯一，不只组内：move 之后按 id 寻址
			// 才有唯一解（spec §4.4 的寻址前提）。
			if seenItem[it.ID] {
				return apierr.With(fmt.Errorf("%w: item id 在档案内重复: %q", ErrValidation, it.ID), apierr.DuplicateID)
			}
			seenItem[it.ID] = true
			if strings.TrimSpace(it.Name) == "" {
				return fmt.Errorf("%w: group %q/item %q: name 为空", ErrValidation, g.ID, it.ID)
			}
			if err := CheckURL(it.URL); err != nil {
				// **%w 不是笔误式的多余**：CheckURL 给那句原话贴了 kind（url-incomplete），
				// 用 %v 就把它截在外面，batch 通路上这一类会掉回 field-invalid——
				// 界面上那句"这一批没收"就成了第二次说谎。实测抓到的正是这一格。
				return fmt.Errorf("%w: group %q/item %q: %w", ErrValidation, g.ID, it.ID, err)
			}
			// icon 不验——见文件末尾那段"图标名不在这里验"的理由。
		}
	}
	// Search / Weather / Clock / Title 这四个字段**不在这里校验**。
	//
	// 理由不是"它们永远合法"，是**没有任何渲染消费者读它们**（render 侧 grep 0 处），
	// 而校验失败的代价是整份 bookmarks.json 被判坏档——一个人的全部书签打不开。
	// 一条实测过的形状：`search.default="bing"` 而 engines 里只有 `prefix:"g"`，
	// 改前答 503「数据文件读不了」，改后照常读出整份档案。
	// 一个不生效的字段不该有毁掉别人台子的权力；等哪一天 search 真的长出界面，
	// 再把校验连同那个界面一起加回来。
	//
	// 字段本身**不能从 model 里删**：解码开了 DisallowUnknownFields（load.go），
	// 而每个档案磁盘上都有这几个键（EmptyBookmarks 一直在写）——删字段等于
	// 让所有老文件在升级后变成坏档。保留、不校验、不渲染，是这里唯一不伤数据的形状。
	return nil
}

func (p *Profile) Validate() error {
	if !ValidSlug(p.ID) {
		return fmt.Errorf("%w: profile id %q 不是 ASCII slug", ErrValidation, p.ID)
	}
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("%w: profile %q: title 为空", ErrValidation, p.ID)
	}
	// PIN **可以为空**（2026-10-12）：一份档案可以不设 PIN，此后 `Profile.PIN` 是空串，
	// 进门不需要任何 PIN。
	//
	// 之前这里是一条 `PIN == ""` 就判坏档，那条规则挡的是"建档写了一半留下空哈希"。
	// 它顺手把"故意不设防"也一起挡住了——而这两件事在形状上长得一模一样，正是这条
	// 规则误伤的原因。现在它整个删掉：**哈希的形状归 auth 管**（`HasPin` 是"有没有
	// PIN"的唯一判据，`VerifyPIN` 对拼错的形状恒假），model 不再复述一遍——
	// 两处各判一次，就有两处会漂。
	//
	// 代价要说清：一份 PIN 拼错的档案**进不去**，而唯一出路是管理员用 rebuild
	// 整份重写（spec §12）。这是真写坏了，不是这条规则能救的。
	if !allowedTheme[p.Theme] {
		return fmt.Errorf("%w: profile %q: theme %q 只能是 %s", ErrValidation, p.ID, p.Theme,
			strings.Join(ThemeValues, "/"))
	}
	if !allowedLayout[p.Layout] {
		return fmt.Errorf("%w: profile %q: layout %q 只能是 %s", ErrValidation, p.ID, p.Layout,
			strings.Join(LayoutValues, "/"))
	}
	// 空串**放过**，而且这一条必须放：`view` 是 2026-10-13 新加的一格，而
	// Validate 挂在读路径上（store/load.go 的 decodeProfile），升级前落盘的
	// profile.json 里根本没有这个键。在这儿判它坏档，等于一次升级把全机器的
	// 档案变成坏档，而唯一出路是逐份 rebuild——图标名那一枪刚踩过（AGENTS.md
	// 「写侧严格、读侧不验」同一条判据）。空 = 没选过 = groups，由渲染层兜。
	if p.View != "" && !allowedView[p.View] {
		return fmt.Errorf("%w: profile %q: view %q 只能是 %s（或留空）", ErrValidation, p.ID, p.View,
			strings.Join(ViewValues, "/"))
	}
	// 空串放过，与 View 同一形状、同一理由（2026-10-19）：这一格比老盘晚，
	// 判它坏档等于一次升级把全机器的档案变成坏档。空 = 没选过 = host，
	// 由渲染层兜（见 render 那边）。
	if p.AddrLevel != "" && !allowedAddr[p.AddrLevel] {
		return fmt.Errorf("%w: profile %q: addrLevel %q 只能是 %s（或留空）", ErrValidation, p.ID, p.AddrLevel,
			strings.Join(AddrValues, "/"))
	}
	return nil
}
