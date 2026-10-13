package model

import (
	"errors"
	"fmt"
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

	// GroupLayoutValues 是**一个分组内部**条目的排法：tiles 磁贴 / list 单列。
	//
	// 它与 LayoutValues（sidebar/topbar，分组**导航**放在哪）不是一件事，两个
	// 枚举都叫 "Layout" 是历史遗留——所以这一张带 Group 前缀，把"排的是哪一层"
	// 写进名字里。判据仍然只有一份：`ValidGroupLayout` 与 `ValidLayout`。
	GroupLayoutValues = []string{LayoutTiles, LayoutList}

	// GroupAccentNames 给 GroupAccentPalette 那一排色块各自起一个中文名。
	// **它必须与 palette 一一对应、同序**：色块是看得见的颜色，看不见的名字，
	// 而读屏与色觉障碍的人拿到的只有这个名字——所以"这一颗叫什么"不是可选项。
	// 数目不等时下面 init 里的 panic 会当场点出是哪一张表。
	GroupAccentNames = []string{"蓝", "青", "绿", "黄", "橙", "红", "紫", "粉"}

	// GroupAccentPalette 是台面面板上那排色块的取值。**它不是白名单**：
	// accent 收任意 3/6 位 hex（render 的 hexColorRE），「自定义」那一格就是
	// 走这条的。这张表回答的是另一个问题——"大多数人选哪几个"，
	// 一家人手边没有取色器，让一个 color input 承担"给 30 个分组配色"这件事
	// 会让它在触摸设备上变成一个笨重的入口。
	//
	// 取值全是**中间调**：亮底与暗底两档都得看得见，而暗底那一档的
	// `--accent` 是 #4c8dff、亮底是 #2a63bd，所以第一格刻意选暗底那个数——
	// 「跟随页面主色」与「蓝」并排时，两者一眼能看出是不是同一个。
	GroupAccentPalette = []string{
		"#4c8dff", "#22b8a0", "#57b95a", "#d3a52c",
		"#e8823c", "#e0574f", "#9b6ce8", "#dd6aa6",
	}
)

func init() {
	if len(GroupAccentPalette) != len(GroupAccentNames) {
		panic("色板与色名必须同数同序：加了一颗色忘了起名字，读屏就少一个可及名")
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

var allowedGroupLayout = valueSet(GroupLayoutValues)

// ValidTheme / ValidLayout 是这两组枚举的唯一出口：校验走 map，
// 表单端点走这两个谓词——判据一份，两处消费。
func ValidTheme(s string) bool  { return allowedTheme[s] }
func ValidLayout(s string) bool { return allowedLayout[s] }

// ValidView 认的是**写侧**的值。读侧另有一条：空串是合法的"没选过"，
// 见 Profile.Validate 里那一句与 Profile.View 的注释。
func ValidView(s string) bool { return allowedView[s] }

// ValidGroupLayout 认**一个分组自己的**排法。与 ValidLayout（导航放哪）分开，
// 是因为这两个"layout"改的是不同层的东西，而把它们合并成一个谓词的那一天，
// 422 的拒绝语就会开始报出一句与调用方无关的枚举。
func ValidGroupLayout(s string) bool { return allowedGroupLayout[s] }

func ValidSlug(s string) bool { return slugRE.MatchString(s) }

// accentHexRE 是 accent 的**唯一**判据，两个方向各用一次：写侧
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
		return fmt.Errorf("url 缺主机名 %q", u)
	}
	return nil
}

// ExternalScheme 报告这个 URL 的协议是不是非 http/https（url.Parse 已把
// scheme 转小写）。浏览器禁止从网页跳这类地址，渲染层据此标「需手动
// 打开」而不是渲染 href——spec §6.2 的"标警告"、§9 的 chrome:// 特例。
// ExternalScheme 报告这个 URL 的协议是不是非 http/https（url.Parse 已把
//
//	scheme 转小写）。
//
// **名单是量出来的，不是印象**（2026-10-13 真点击实验）：在一个 http 页面上放一个
// 链接、用真鼠标点下去（带用户手势，因为 Chrome 对这类放行看的正是手势），
// 然后看地址走没走、控制台说了什么：
//
//	chrome://extensions/   不走。security error「Not allowed to load local resource」
//	chrome://settings/     同上
//	file:///C:/...         同上
//	mailto:nobody@…        走。「Launched external handler for 'mailto:…'」
//	about:blank            走
//	http://example.com/    走（对照）
//
// 所以"能不能当普通链接点"与"我们认不认这个协议"是两件事，而**只有前者决定这一格
// 画成链接还是画成"需手动打开 + 复制键"**。把 mailto: 画成一条点不动的行，
// 是把一条本来会唤起邮件客户端的地址做成了一个假死链接；反过来把 chrome:// 画成
// 链接，得到的是一按下去什么都不发生、只有控制台里一句报错——那正是 §8.1
// 要避免的「点了没反应的东西」。
//
// `tel:` 与 `sms:` 与 `mailto:` 走的是浏览器里同一条外部协议处理通路
// （external handler），只是本机没装能收 tel: 的程序所以量不了；
// 装了的机器上它与 mailto: 同命运，所以一起放行。
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
			return fmt.Errorf("%w: group id 重复: %q", ErrValidation, g.ID)
		}
		seenGroup[g.ID] = true
		if strings.TrimSpace(g.Title) == "" {
			return fmt.Errorf("%w: group %q: title 为空", ErrValidation, g.ID)
		}
		if !ValidGroupLayout(g.Layout) {
			return fmt.Errorf("%w: group %q: layout %q 只能是 tiles/list", ErrValidation, g.ID, g.Layout)
		}
		for ii := range g.Items {
			it := &g.Items[ii]
			if !ValidSlug(it.ID) {
				return fmt.Errorf("%w: group %q item %d: id %q 不是 ASCII slug", ErrValidation, g.ID, ii, it.ID)
			}
			// item id 在整个档案内唯一，不只组内：move 之后按 id 寻址
			// 才有唯一解（spec §4.4 的寻址前提）。
			if seenItem[it.ID] {
				return fmt.Errorf("%w: item id 在档案内重复: %q", ErrValidation, it.ID)
			}
			seenItem[it.ID] = true
			if strings.TrimSpace(it.Name) == "" {
				return fmt.Errorf("%w: group %q/item %q: name 为空", ErrValidation, g.ID, it.ID)
			}
			if err := CheckURL(it.URL); err != nil {
				return fmt.Errorf("%w: group %q/item %q: %v", ErrValidation, g.ID, it.ID, err)
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
	return nil
}
