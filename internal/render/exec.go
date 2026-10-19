package render

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// i18nView 是每一页都要的那两格，所以内嵌进每个视图而不是各写一遍字段。
//
// **T 由执行器注入，调用方一律不许填**——与 CSS 同一条纪律：让 httpapi 那头
// 能填 T，就等于允许它塞一段未经查表校验的字符串进页面（而查表正是"这一句话
// 在两种语言里都存在"的唯一保证）。
//
// 语言以**位置参数**递进执行器方法而不是放在视图结构体上，与 HasSnapshots 同一
// 条理由：字段能忘填，位置参数不能。一个忘了传语言的渲染函数会立刻在测试里
// 显出 ⟨key⟩ 而不是安静地退回中文。
type i18nView struct {
	// Lang 是 <html lang> 的值：zh-CN / en。取自 HTMLLang 而不是 Lang 本身——
	// 前者是标准写法（RFC 5646），读屏软件与浏览器的翻译提示认它。
	Lang string
	// Langs 是语言下拉框的选项，**每页都带着**：引导页建档时选一次，管理页随时改。
	// 两个入口都要，所以放共用的这一格而不是各页各算一遍。
	Langs []LangOption
	// cat 是文案本体，**不许导出**：导出就等于让 httpapi 那头能填，一份未经查表
	// 校验的字符串就能进页面（而查表正是"这一句话在两种语言里都存在"的唯一保证）。
	// 模板只能拿到下面那个 T 方法。
	cat *i18n.Catalog
}

// LangOption 是下拉框里的一格。名字用该语言自己写（「中文」「English」而不是
// 「Chinese」「英文」）：让一个看不懂当前语言的人也能从列表里认出自己要的那一个。
type LangOption struct {
	Value    string
	Name     string
	Selected bool
}

// langOptions 由渲染器算，不让调用方传：它同时知道"机器现在是什么语言"与
// "有哪些语言可选"，这两件事只有它两边都有。**新增一种语言只有 i18n.All 一处要动**。
func langOptions(current i18n.Lang) []LangOption {
	out := make([]LangOption, 0, len(i18n.All))
	for _, l := range i18n.All {
		out = append(out, LangOption{Value: string(l), Name: l.DisplayName(), Selected: l == current})
	}
	return out
}

// T 是绑定了当前语言的查表。模板里只有 `{{.T "key"}}` 这一种形状——**不许**出现
// `{{if eq .Lang "en"}}` 那种把语言分支写进模板的写法：那会让"这一页有几种说法"
// 从语言包里的一行变成散落在标记里的一串判断。
//
// **必须是方法，不能是函数字段**：Go 模板不认结构体上的函数值，连无参的都不认
// （报 "T has arguments but cannot be invoked as function"），只认方法。实测三种
// 形态——变参、定参、无参——作为字段一律失败，作为方法一律成功。因为这一格是
// 内嵌进每个视图的，方法会被提升到外层，模板里写的仍然是 `{{.T "key"}}`。
func (v i18nView) T(key string, args ...any) string {
	if v.cat == nil {
		return "⟨" + key + "⟩"
	}
	return v.cat.T(key, args...)
}

// newI18nView 把一份文案绑成视图要的那两格。
//
// cat 传 nil 时落默认语言而不是崩：一个漏传语言的调用点会拿到一份**能显示**的
// 界面（中文），而不是 panic。真正的"忘了传"由那个位置参数在代码审查里拦，
// 而崩溃只会让一台跑着的机器因为某次改动打不开。
func newI18nView(cat *i18n.Catalog) i18nView {
	if cat == nil {
		cat = i18n.Load(i18n.Default)
	}
	return i18nView{cat: cat, Lang: cat.HTMLLang(), Langs: langOptions(cat.Lang())}
}

type executor struct {
	profileT *template.Template
	pickerT  *template.Template
	brokenT  *template.Template
	adminT   *template.Template
	// adminActionT 与 adminT 分开挂着而不是共用一个模板集：它们的 CSS 注入
	// 方式一样，但共用会把"这一页渲染的是哪张表"这件事变成隐式的——而
	// 确认页带着档案 id 与后果说明，它出错时必须在名字里就看得出来。
	adminActionT *template.Template
	// importT 同理单独挂着：导入那三步（选文件/看报告/确认）带着上传报告，
	// 它出错时不该被误读成"管理页的确认页坏了"。
	importT *template.Template
	// historyT 单独挂着，理由与 importT 同一条：每一页各自带着自己的数据形状，
	// 共用一个模板集会把"渲染坏了的是哪一页"变成要猜的事。
	historyT *template.Template
	// adminLoginT 单独挂着：它是**没有管理权时**的那一页，形状与 adminT 相反
	// （一张输入框 vs 一张表）。共用会让"渲染坏了管理页还是登录页"看不出来。
	adminLoginT *template.Template
	// notFoundT 是浏览器打错地址那一页。它单独挂着与 adminLoginT 同一个理由，
	// 但它还有第二条：这一页**不属于任何档案**，也不属于管理那一族，
	// 借任何一页的模板都会让 X-LB-Profile 与 SW 的缓存桶跟着猜错。
	notFoundT *template.Template
	cssT      template.CSS
	ix        *icons.Index
	fc        *store.IconCache
}

type itemView struct {
	ID, Name, URL, Desc string
	// Icon 是盘上那串图标名（tabler:box / simple-icons:x / auto / 空）。屏上它只是
	// 一个小图形，DOM 里没有一处写着这串字——编辑那一格要说"现在用的是哪一个"，
	// 只能由模板把它带到 li[data-icon] 上（batch9 U7）。
	Icon        string
	Letter      string
	Alias, Tags []string
	External    bool // 非 http/https：浏览器不让从网页跳过去
	SVG         template.HTML
	Img         string
	Missing     bool
	ChipHue     int
}

type groupView struct {
	ID    string
	Title string
	// Leaf 与 Parents 是同一串标题的两半（2026-10-12）：导入把嵌套文件夹拍平成
	// "父 / 子" 的标题，于是整页上每一节都印着一长串路径——侧栏成了一面读不完的
	// 墙，正文里"工作 / 项目 A / 会议记录"与"工作 / 项目 A"看上去是两组并列，
	// 而它们本来是一家的。
	//
	// 拆成两半**只是为了好看**：磁盘上的标题一个字都没动（id、寻址、已有档案全不
	// 受影响），侧栏与节标题按这两半分别排版。判据只有 pathSplit 一处。
	Leaf    string // 最后一段，节标题里要重的那一半
	Parents string // 前面的段，弱化显示（空串 = 这一层没有父级）
	// Depth 是路径的段数（0 = 顶层分组）。节标题按它缩进，于是**正文也读得出
	// 层级**：四十五个分组的档案动辄三万多像素一页，那一长串标题若一样齐地摆着，
	// 眼睛找不到"哪几个是一家的"——它与侧栏那棵树是同一件事的两半。
	Depth int
	Items []itemView
	// Count 是**这一组有几条书签**（2026-10-15，删掉一组的确认句要用它）。
	//
	// 为什么不写 `{{len .Items}}` 就完事：面板里那一列是 `prefsGroups` 另一次读的
	// 结果（render.go），它**不带条目**——只带标题/颜色/排法，于是那条模板在
	// 面板上根本没有 Items 可数。同一格由两处填，数就得由 model 那份数出来，
	// 两处各数一次会漂（页面那份与面板那份来自不同的读）。
	//
	// **空组照样画**（2026-10-15 改判，曾判"空的自动隐藏"）：页面上的组列表是
	// 「这个人放的每一个分类」，而空组仍然是一个**存在的目标**——它能被加书签
	// 进去（添加浮层的分组下拉、拖拽落点都是从页面上这些标题读的），把它藏起来
	// 等于让那几处一起失效，于是"这一组怎么才能再放东西进去"没有答案。
	// 空组的样子是"一个标题 + 一句没有条目的提示"，那是诚实的现状，不是噪声。
	Count  int
	Accent string // 已验过的 hex，空串 = 没有；唯一判据是 validatedAccent
	// AccentCSS 是同一个值包成 template.CSS 的那一份，供模板写进 section 的
	// `style="--sec-accent:…"`（见 CSSAccent）。Accent 留给形状判据与比较，
	// 模板上色用这一份——**同源，不是各自算一遍**。
	AccentCSS template.CSS
	Layout    string // tiles|list 原样带出，Task 10 的 CSS 靠它
}

// pathSep 是导入器拍平嵌套文件夹时用的分隔符。它在这里被**认出**，不是为了再去
// 生成同样的标题——而是把已经是那个形状的标题拆回层级给人看。
//
// 一份书签导出动辄三十个文件夹，标题全是"工具 / 同步 / 脚本"这样的串：侧栏三十行
// 长文本、正文三十个长标题，而**看不出哪几个是一家的**。导入器不可能不做这件事
// （model 的分组只有 id/title/layout/items，没有父子），所以层级只存在于标题里；
// 于是渲染层把它**读回来**。
const pathSep = " / "

// pathSplit 把一条分组标题按分隔符拆成（父链, 叶名）。没有分隔符时父链为空。
func pathSplit(title string) (parents, leaf string) {
	i := strings.LastIndex(title, pathSep)
	if i < 0 {
		return "", strings.TrimSpace(title)
	}
	return strings.TrimSpace(title[:i]), strings.TrimSpace(title[i+len(pathSep):])
}

// navNode 是侧栏那棵树的一格。
//
// **是一棵拍平的树**（带 Depth），不是嵌套结构：窄屏那条横向分组带也要用同一份
// 数据（.gnav.top 与 ≤700px 的 .gnav 都走 grouplist），嵌套 <ul> 在横向排布里没法
// 表达，而 Depth 一个整数在两种形态里都成立。
//
// Href 为空 = 合成出来的父级（自己不是分组，只是一段路径），因此不可点、也不带计数。
type navNode struct {
	Label  string
	Href   string
	Count  int
	Accent string
	// AccentCSS 是**同一个值**包成 template.CSS 之后的那一份，供模板写进
	// `style="--sec-accent:{{.}}"`（见 CSSAccent：html/template 会把一个像
	// 颜色的值判为不可信并替换成 ZgotmplZ，而色点就是靠这一个变量上色的）。
	// Accent 与 AccentCSS 同时留着：前者的形状判据/比较仍走它，模板那一处用
	// 后者——**两份同源，不各自算**。
	AccentCSS template.CSS
	Depth     int
	// Leaf/Parents 与 groupView 同一份拆法：父级节点自己也要显示成"叶子 + 弱化的
	// 前缀"，否则"工具 / 同步"这一行在树里又会变回一长串。
	Leaf    string
	Parents string
	IsGroup bool
}

// groupNav 把分组列表折成一棵拍平的树。
//
// 合成父节点与真实分组**必须分得清**：只有真实分组能链到锚点、才有计数与色点。
// 一段路径恰好也是某个分组标题时（"工具" 与 "工具 / 同步" 同时存在），那一段按
// **真实分组**画——否则那个分组在侧栏里就没有入口了，而它明明是可进去的。
func groupNav(groups []groupView) []navNode {
	if len(groups) == 0 {
		return nil
	}
	// 先知道哪些整串标题是真实分组：合成父节点靠它判断"我该不该可点"。
	real := make(map[string]bool, len(groups))
	for _, g := range groups {
		real[g.Title] = true
	}
	var out []navNode
	seen := map[string]bool{} // 已经画过的那一整串路径
	for _, g := range groups {
		full := ""
		for _, seg := range splitAll(g.Title) {
			if full == "" {
				full = seg
			} else {
				full += pathSep + seg
			}
			if seen[full] {
				continue
			}
			seen[full] = true
			isGroup := real[full]
			// 合成父节点的标题就是它自己那一段；真实分组用原标题（两者在拆开之后
			// 得到的是同一组 Leaf/Parents——差别只在这一串给不给锚点与计数）。
			title := seg
			if isGroup {
				title = full
			}
			node := navNode{Label: title, IsGroup: isGroup,
				Depth: strings.Count(title, pathSep)}
			node.Parents, node.Leaf = pathSplit(title)
			if isGroup {
				node.Href = "#g-" + g.ID
				node.Count = len(g.Items)
				node.Accent = g.Accent
				node.AccentCSS = CSSAccent(g.Accent)
			}
			out = append(out, node)
		}
	}
	return out
}

// splitAll 把标题按分隔符全拆开（pathSplit 只拆最后一处，这里要每一段）。
func splitAll(title string) []string {
	var out []string
	for _, seg := range strings.Split(title, pathSep) {
		if s := strings.TrimSpace(seg); s != "" {
			out = append(out, s)
		}
	}
	return out
}

type profileData struct {
	Title        string
	Footer       string
	OpenInNewTab bool
	Groups       []groupView
	// Nav 是侧栏那棵树（groupNav 的拍平结果）。**与 Groups 分开**：Groups 是
	// "磁盘上有哪几个分组"，Nav 是"给人看的层级"——同一个分组在两处各出现一次，
	// 判据都在 groupNav 里算一次，模板不再自己拼路径。
	Nav []navNode
}

type profileView struct {
	i18nView
	Profile *model.Profile
	// PageTitle 是**顶栏那行大字与浏览器标签页的前半**：这一页叫什么。
	// 它取自 Profile.PageTitle（台面偏好，与 theme/layout 同族），空则按界面语言
	// 给一个通用默认（`profile.title.default`）——默认值**只在渲染时算，不落盘**：
	// 往档案里编一个字，等于让"没起过名字"与"名字正好等于那个字"两件事分不开，
	// 而 store/load.go 那条"停止编造默认值"判的就是这个。
	//
	// 它不是 Profile.Title（那是"这是谁的那一份"）。2026-10-10 那轮把顶栏从
	// Bookmarks.Title 换成档案名，是因为当时 Bookmarks.Title 是一个所有人共用的
	// 常量「发射台」——那一格今天仍然不是这一页的名字（见 model.Profile.PageTitle
	// 那段"为什么不住在那里"）。
	PageTitle string
	// Theme 落成 <html data-theme>，但**只带显式选择**：auto|dark|light 里
	// "auto" 在这里被归一成空串，属性根本不发（见 themeAttr）。
	// 只从已经拿在手里的 Profile 取值：渲染一页绝不许再读一次 profile.json。
	// 选择页、解释页与管理页**没有**这个字段——它们不属于任何档案，没有主题可说，
	// 页面跟系统（见 pickerView 上面那段注释）。
	Theme string
	// AddrLevel 是**卡片第二行印什么**，归一后只有三档（host/full/none）。
	//
	// 归一在这里做而不是模板里 `{{if}}`：模板那个三选一要判三遍，而"空串 =
	// host"这条判据只该有一份——它与 themeAttr 那个"auto 归一成空串"是同一族
	// 处置（默认值只在渲染时算，不往盘里编一个字）。
	AddrLevel string
	// Compact 是 view ∈ {wall, dense}：**所有分组都摆在同一屏**的那两档。
	// 判据只有 compactView 那一份，页面（CSS 那两条）与面板（文案）共用它——
	// 不写两遍"或"是这一族的老规矩（themeAttr / paintedBackground 同一处置）。
	Compact bool
	// Query 非空 = 这一页是搜索结果。模板靠它决定说"没有匹配"还是"还没有书签"，
	// 也靠它决定要不要在顶栏搜索框里回填。
	Query string
	Hits  int
	Total int
	// Add / AddTitle 是 bookmarklet 带来的那一条待添加书签（§6.3）。它们只**透传**
	// 给 ⑤ 的浮层预填，页面本身不落库——书签要人再按一次"存进去"才写进去，
	// bookmarklet 从来不是"点一下就自动存"。
	Add       string
	AddTitle  string
	Bookmarks profileData
	// Featured 是**排成大卡的那几个分组**，页面顶部独占一块。判据在下面那句
	// `Layout == model.LayoutFeature`，它们同时**不在** `Bookmarks.Groups` 里
	// （挪走而不是复制——同一组在一页里出现两次，那才是真的坏）。
	//
	// 与 Nav 的关系是**导航照旧**：设成大卡只换位置，不让这一组从分组导航里消失。
	Featured []groupView
	CSS      template.CSS
	// AppJS 是 ⑤ 脚本的**指纹路径**（`/app.<hash>.js`），模板用它拼 <script src>。
	// 指纹只由内容决定（见 assets.go）：改一个字换一条 URL，immutable 缓存才成立。
	// 页面不引它时它是空串，模板 `{{if .AppJS}}` 自然不发那个标签。
	AppJS  string
	AppCSS string
	// Rev 进 body 的 data-rev：添加浮层提交时要带 expectedRev（§6.1），而列表是
	// 服务端渲染的——JS 不为此重新拉一份数据，就在 body 上读这两个属性。
	Rev int64
	// Empty 是"这个档案一条书签都没有"（**数条目，不是数组**）。
	// 导入卡片只在它为真时出现——那个提示与 httpapi 的 importGuard（"非空档案
	// 不许导"）是同一条判据的两头：看得见入口的时候，它一定进得去。
	// 模板不做判断，判据在这里算一次。
	Empty bool
	// HasSnapshots 是"这份档案还有没有可回退的版本"，顶栏那枚「回退」只在它为真
	// 时才画——**与 Empty 同一套纪律**（§8.1：「点了没反应的东西不进界面」）：
	// 一条书签都没写过的档案，snapshots/ 是空的，点进去只有一句"还没有可回退的版本"。
	//
	// 它由调用方以**位置参数**递进来（ProfilePage 手里有 store，执行器没有），
	// 而不是视图结构体上一个可填的字段：字段能忘填，位置参数不能。
	// 判据本身是 `len(ListSnapshots) > 0`，与 /revert 页那个"有没有可选项"是同一个
	// 事实——那一头还要逐份读快照（要给条目数与分组名），所以它比这个严一档：
	// 极端情况下"有快照但全部读不出来"，这里仍会画入口而那一页没有表单。那种档案
	// 要靠手滑把 snapshots/ 全写坏才会出现，比"新建之后一条没改"窄得多，不值得
	// 为它在每次渲染里多读一遍全部快照。
	HasSnapshots bool
	// HasAdmin 与上面那格同一形状：它是**判据**，所以是位置参数而不是视图字段。
	// 含义 2026-10-12 变了——从"这个档案是不是管理员"变成"这次请求有没有带着
	// 一把真的管理权"，于是同一个人可以一边用自己那份书签、一边仍带着管理权。
	HasAdmin bool
	// PrefGroups 是台面面板里那一节「分组」的每一行。**只有 GET /prefs 填**，
	// 档案页那个空壳是 0 长——面板内容是 fetch 一次注入的，档案页不该背这几十行。
	//
	// 它不是 groupNav 那棵树、也不是 Bookmarks.Groups：那一棵是**页面上**的导航
	// （父子折叠、只印叶名），这一列是**面板里**的清单（一个分组一行，带它自己的
	// accent 与 layout）。两种形状各有各的判据，别处把其中一份抄过来用。
	PrefGroups []groupView
	// PrefGroupsBroken 是"分组清单画不出来"的原因（非空 = 那一节不画，并说清为什么）。
	// **坏档不挡面板**：改台面偏好只写 profile.json，而书签文件读坏了正是最需要
	// 改排版看看的时候——所以那一节单独退化，外观那几行照常在。
	PrefGroupsBroken string
	// PrefPalette / PrefPaletteNames 把 model 那张色板原样带上来。**不在这里
	// 再抄一份**：色块渲成什么、读屏念什么，两边必须与 model 同数同序
	// （model 里的 init 会 panic 兜住它们不同数的情形）。
	PrefPalette      []string
	PrefPaletteNames []string
	// PrefTab 是**左侧那一列当前选中哪一节**（`?tab=`）：三个值里的一个，判据
	// 是 `prefsTab` 那张名单（httpapi 侧收，渲染侧兜空串，见 prefsPage）。
	// 它要活过一次保存——而**只有它自己知道怎么把这一格留在原地**。
	PrefTab string
	// MaxGroupTitleRunes 是「分组名」那一格的长度上限，**原样带上 model 那个数**
	// （不为模板抄一份，也不为它加一个 setter）。模板把它印成那个 maxlength：
	// 服务端那条通路判的是同一个数，两处不同数时症状是"浏览器拦住了、curl 拦不住"。
	MaxGroupTitleRunes int
	// Background 是**这一页画背景用的** URL（`/backgrounds/<名字>`），空串 = 不画。
	// BackgroundFile 是**盘上那一份**的 URL，不管画不画。
	//
	// **为什么是两格**：面板要能说出「图还在，只是这一页暂时不画它」，而那一句话
	// 要求"盘上有"，页面要的是"画不画"——后者含 BackgroundOff 那一维，两者不是
	// 同一个事实。把它们并成一格的结果是：暂时不显示时面板也跟着说「还没有背景图」，
	// 于是「恢复显示」那颗键连同整节一起消失（屏上读作"我的背景没了"）。
	//
	// **净化在读侧**（model.ValidBackground 那一份判据），不是渲染时"大概没问题"：
	// 这一格的去向是 `<html style=…>` 与面板里那张预览 `<img src>`，前者会被拼进
	// 一段 CSS 属性值、后者会被浏览器当 URL 取——盘上一个写坏的值（手改
	// profile.json 塞进来的 `../../secret`）最坏的结果必须是"这张背景不见了"。
	//
	// 背景**不进渲染缓存键**：profile.json 的内容指纹（profFp）已经在键里，而
	// 背景就住在那个文件里——改它必然改指纹（见 render.go cacheKey 那一维）。
	Background     string
	BackgroundFile string
}

// backgroundFile 算盘上那一份的 URL（空 = 没有：没设 / 形状坏）。
// 判据只有形状与"有没有"，**不看 BackgroundOff** ——那一位说的是"画不画"，
// 而这一格问的是"还在不在"。
func backgroundFile(p *model.Profile) string {
	if p == nil || p.Background == "" || !model.ValidBackground(p.Background) {
		return ""
	}
	return "/backgrounds/" + p.Background
}

// paintedBackground 是这一页**要画**的那一张（空 = 不画）：形状坏与「暂时不显示」
// 都归成空串，于是模板那边只有"有/没有"两种形状——与那个 8.6 的教训是同一条：
// **留一半判断在模板里，下一个改模板的人就要把两处判据对齐一遍**。
func paintedBackground(p *model.Profile) string {
	if p == nil || p.BackgroundOff {
		return ""
	}
	return backgroundFile(p)
}

// themeAttr 把 theme 枚举里的 "auto" 归一成空串。
//
// 为什么归一而不是给 CSS 再加一条 `[data-theme=auto]`：属性存在的语义是
// "用户显式选了哪一档"，`[data-theme=dark]{color-scheme:dark}` 那条本来就只是
// 表意图、什么都不改。让 CSS 同时理解三种值、媒体查询与属性两条通路各表一次
// "auto"，同一个事实就写了两遍。归一之后 "auto" 在 HTML 里不留痕，媒体查询是
// 它唯一的裁判。
//
// 不归一的代价是实打实的：auto 是建档默认值（store.CreateProfile），而模板的
// {{with .Theme}} 对非空串取真 → 页面发出 data-theme="auto"；CSS 的亮色通路是
// [data-theme=light] 与 :root:not([data-theme])，属性在则媒体查询那条不匹配，
// 值不是 light/dark 则显式那条也不匹配——系统偏好亮色的默认档案被钉死在暗色上。
// compactView 回答"这一页是不是**全在一屏**的那一档"。
//
// 两档的共同点是**所有分组同时在场**：wall 把它们排成多栏、dense 把每条压成一行，
// 而分组视图是一长页一节接一节——导航的职责是"跳到你看不见的那一节"，前两档里
// 没有看不见的节（判据与 base.css 那两条 `[data-profile-view=…]` 是同一个事实的
// 两个出口，一个给样式一个给文案）。
//
// 空串（没选过 = 分组视图）**不是**紧凑档：升级前每一份 profile.json 都没有 view。
func compactView(v string) bool { return v == "wall" || v == "dense" }

func themeAttr(theme string) string {
	if theme == "auto" {
		return ""
	}
	return theme
}

// addrLevelOf 把 Profile.AddrLevel 归一成三档。**空串 = host**：那一格比老盘晚，
// 升级前落盘的 profile.json 里没有这个键，所以"没选过"必须与"选了 host"是同一件事。
//
// 归一在这一处而不是模板里 `{{if}}`：模板那个三选一要判三遍，而"空 = host"这条
// 判据只该有一份（与 themeAttr 那个"auto 归一成空串"同一族处置）。
//
// 盘上不会出现别的值——`Profile.Validate` 挂在读路径上，写侧也只有 `POST /prefs`
// 一个入口。**这里仍然兜一层**：模板与 CSS 都在按这个值做决定，而一个没被挡住的值
// 会变成"屏上什么都不印"这种安静的错误。
func addrLevelOf(v string) string {
	switch v {
	case model.AddrFull, model.AddrNone:
		return v
	default:
		return model.AddrHost
	}
}

// pickerView / brokenView / adminView 都**没有** Theme 字段：这三页不属于任何
// 档案（进门页还没选定、解释页的文件可能读不出来、管理页跨所有档案），没有
// 一份 profile.json 可问"这人显式选了哪档"，于是模板不发 data-theme，页面
// 跟系统（base.css 的 :root:not([data-theme]) 那条通路）。这个形状 2026-10-07
// 整枝评审 Minor 5 之前是"字段恒空串 + 模板支路恒不进"——留一半就是留着一条
// 永远走不到的分支让人以为它有用。要按档案主题渲染解释页，得先决定读不出
// profile.json 时算谁的，那是 ⑤ 之后的决定。
type pickerView struct {
	i18nView
	Entries []pickerEntry
	CSS     template.CSS
	// Heading 与 Setup 是进门页三形状的两格（设计 §2）：h1 的全部文字，以及
	// "这一页画引导建档表单还是档案卡列表"。判据由 httpapi 算，这里只带着走。
	Preselect string
	KeepAdd   string
	// KeepAddTitle 是 ?t=（bookmarklet 带的那一页的标题）。它与 KeepAdd 一起穿过 PIN
	// 步骤——只带地址不带标题的话，进门后浮层里得自己再填一遍。
	KeepAddTitle string
	Notice       string
	Heading      string
	Setup        bool
	// NoWayIn 是"一张卡都画不出、也没有哪一份被点名为读不动"——也就是这一页
	// 没有任何人能进得去（2026-10-16 取景）。它把标题从一句**没人能回答的问话**
	// 换成那句事实本身：全家用档案都被停用、或目录都在但一份 profile.json 读不出
	// 那两格上，原先顶上写着「谁来用？」而下面写着「这台机器现在没有进得去的档案」。
	NoWayIn bool
	// HasAdmin 只决定要不要画那个进管理页的口子（2026-10-12）。判据在 httpapi
	// （这次请求有没有一把真的 lb_admin），模板不判断——没管理权的人看见那个
	// 格子只会落到登录页。管这台机器是另一件事，不该在没有管理权时把入口画出来。
	HasAdmin bool
	// Blocked 与 .Entries 是同一件事的两半：读得动的给卡，读不动的在这一页点名并
	// 说明为什么进不去（死路①）。只能给 id——标题就住在读不动的那个文件里，编一个是假话。
	// 模板不许自己数档案：判据在 httpapi 与 store。
	Blocked []string
}

type pickerEntry struct {
	ID string
	// Icon 是这张卡上画的那张脸（2026-10-12）：挑这个人自己常用的那个站点的图标——
	// 图标比字好认，而共用一台电脑的一家人各自认自己那张，靠的正是"这张是那个图标"。
	// 挑法与三取一都在 icon.go 的 faceOf/pieceFor，判据只有一份。
	//
	// Title 是卡片上可见的名字（<b class="pick-name">{{.Title}}</b>）与按钮无障碍名
	// （aria-label）。名字是区分档案的那一维：两份档案完全可能常用同一个服务（都用 GitHub），
	// 此时图标一模一样，名字是唯一的区分依据；图标负责好看，不顶替身份。
	Icon  iconPiece
	Title string
	// NeedsPIN 决定这张卡片要不要给 PIN 输入框（2026-10-12）。
	//
	// **它由 httpapi 算，不由这里问 store**：选择页本来就不读 profile.json 的
	// 那一格（`ListProfiles` 给的就是 Profile，够判），而判据本身是
	// `auth.HasPin`——渲染层复述一遍就会有两处漂。
	//
	// 不设防的档案给一个空输入框，等于在门口摆一个"输什么都行"的假门槛：
	// 人会以为这里该输点什么，而输错之后弹回来的那句比不问更费解。
	NeedsPIN bool
}

// adminLoginView 是"输管理 key"那一页。页面本身极简：一个标题、一条失败提示、
// 一个 password 输入框、一句 key 在哪儿的说明。**它不知道 key 对不对**，那是
// httpapi 判完 303 回来的事。
type adminLoginView struct {
	i18nView
	Notice string
	CSS    template.CSS
}

// notFoundView 是"这条路径不存在"那一页。Back 是**已经翻好**的那句出路
// （带运行时值的句子在 Go 侧查表，见 AGENTS.md 的三条纪律）：这一页有会话时
// 说的是"回我的页面"，没有会话时说的是"回选择页"，而 / 那一头自己会分诊。
type notFoundView struct {
	i18nView
	Back string
	CSS  template.CSS
}

type brokenView struct {
	i18nView
	ProfileID string
	Reason    string
	CSS       template.CSS
}

// adminView 是管理页的数据。Count 由执行器算好递给模板：模板不做判断
// （与 validatedAccent、themeAttr 同一条纪律——判据在 Go 侧那一份）。
type adminView struct {
	i18nView
	Rows []AdminRow
	// Notice 是 `?err=` 白名单查出的一句人话，空串不画（判据在 httpapi，这里只印）。
	Notice string
	CSS    template.CSS
	Count  int
}

// adminActionView 是 AdminAction 加 CSS 之后的样子。**单独一个类型而不是给
// AdminAction 加 CSS 字段**：那是给调用方用的结构，而 CSS 由渲染器自己注入——
// 让 httpapi 那头能填 CSS，就等于允许它塞一段未经 safeCSS 的字符串进来。
type adminActionView struct {
	AdminAction
	i18nView
	CSS template.CSS
}

func newExecutor(ix *icons.Index, fc *store.IconCache) *executor {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		panic(fmt.Errorf("读内嵌 base.css: %w", err))
	}
	// CSS 是我们自己的静态文件，不是用户数据：用 template.CSS 内联进
	// <style> 换一次 RTT（spec §11 已记录 style-src 'unsafe-inline' 的取舍）。
	css := template.CSS(safeCSS(stripCSSComments(string(raw))))
	// shortTime 给管理页那一格「最近改动」、clockTime 给改动历史的每一行。
	// 模板的函数名单只有这一份，别在 httpapi 那头再格式化一次——
	// 同一件事两处做，两处就会漂移。
	funcs := template.FuncMap{"shortTime": shortTime, "clockTime": clockTime, "reportHTML": reportHTML, "hostOf": hostOf}
	parse := func(name string) *template.Template {
		data, err := webFS.ReadFile("web/" + name)
		if err != nil {
			panic(fmt.Errorf("读内嵌模板 %s: %w", name, err))
		}
		return template.Must(template.New(name).Funcs(funcs).Parse(string(data)))
	}
	return &executor{
		profileT:     parse("profile.tmpl"),
		pickerT:      parse("picker.tmpl"),
		brokenT:      parse("broken.tmpl"),
		adminT:       parse("admin.tmpl"),
		adminActionT: parse("adminaction.tmpl"),
		importT:      parse("import.tmpl"),
		adminLoginT:  parse("adminlogin.tmpl"),
		notFoundT:    parse("notfound.tmpl"),
		historyT:     parse("history.tmpl"),
		cssT:         css,
		ix:           ix,
		fc:           fc,
	}
}

// history 渲染改动历史。切到哪一天在这里算（那一节的标题要读语言），
// 而每一行的短句在调用方已经查好表——见 render.HistoryRow 那条。
func (e *executor) history(v HistoryView, cat *i18n.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.historyT.ExecuteTemplate(&buf, "history", historyView{
		HistoryView: v, Days: historyDays(v.Rows, cat),
		CSS: e.cssT, i18nView: newI18nView(cat)}); err != nil {
		return nil, fmt.Errorf("渲染改动历史页: %w", err)
	}
	return buf.Bytes(), nil
}

type historyView struct {
	HistoryView
	// Days 由执行器算，调用方填不了：它是"这些行落在哪几天"，判据里写着本机
	// 时区与界面语言——与 adminView.Count 同一处置（判据在这一层，模板只印）。
	Days []historyDay
	i18nView
	CSS template.CSS
}

// importPage 渲染浏览器收藏夹导入那三步。判据全在 ImportView 里，模板只印。
func (e *executor) importPage(v ImportView, cat *i18n.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.importT.ExecuteTemplate(&buf, "import", importView{
		ImportView: v, CSS: e.cssT, i18nView: newI18nView(cat)}); err != nil {
		return nil, fmt.Errorf("渲染导入页: %w", err)
	}
	return buf.Bytes(), nil
}

type importView struct {
	ImportView
	i18nView
	CSS template.CSS
}

// countItems 数一个书签文档里的条目总数（**数条目，不是数组**——一个分组存在
// 但里面没有条目，不等于"这个人还没有书签"）。
func countItems(b *model.Bookmarks) int {
	n := 0
	for _, g := range b.Groups {
		n += len(g.Items)
	}
	return n
}

// safeCSS 挡一手：内嵌样式里出现 </style> 会截断标签，把后面的内容
// 变成 HTML。本仓库的 base.css 不该有这串，出现就该当构建错误。
func safeCSS(s string) string {
	if strings.Contains(strings.ToLower(s), "</style") {
		panic("base.css 里出现 </style，会截断内联 style 标签")
	}
	return s
}

// stripCSSComments 去掉内联样式里的注释。base.css 的注释是写给读代码的人的，
// 而它**整份内联进每一张页面**（spec §7.2 的零额外请求），于是注释也在按人次
// 下载：实测占掉这份字节的三成上下，而 §7.3 那条最薄的操作线量的正是它。
//
// 逐字符走一遍带引号的状态机而不是 regexp：`content:"▸"` 这类字符串里出现
// `/*` 不是注释，正则一把梭会把规则吃掉。
func stripCSSComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	var quote byte
	for i := 0; i < len(s); {
		c := s[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(s) { // 串内的转义：连下一字节一起吃掉
				b.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == quote {
				quote = 0
			}
			i++
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			b.WriteByte(c)
			i++
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return strings.TrimRight(b.String(), " \t\r\n") // 注释没闭合：当它到结尾
			}
			i += 2 + end + 2
			continue
		}
		b.WriteByte(c)
		i++
	}
	out := b.String()
	// 注释拿掉后会留下整行空白；连行首缩进一起收掉，省的是真字节。
	lines := strings.Split(out, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		kept = append(kept, strings.TrimRight(ln, " \t\r"))
	}
	return strings.Join(kept, "\n")
}

// InlineCSS 返回**真正内联进每一张页面**的那份字节（注释已剥掉）。
//
// 体积闸要量的是这个，而不是磁盘上的源文件：源文件里的注释是写给读代码的人的，
// 不下发；拿源文件的字节数去判"页面里真占的那部分"，量的是一个已经不存在的数。
func InlineCSS() []byte {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		panic(fmt.Errorf("读内嵌 base.css: %w", err))
	}
	return []byte(safeCSS(stripCSSComments(string(raw))))
}

// hostOf 从一条地址里取出**主机名**（连端口），给大卡那一行灰色小字用。
//
// **为什么不是整条地址**：大卡上那一行是给人"扫一眼知道去哪台机器"的，
// 而 `http://10.0.2.3:49160/dashboard` 这么一整条塞进去，一行放不下、
// 也不成扫——人读的是 `10.0.2.3:49160`。
//
// **只认 http/https**（2026-10-14 实拍后补）：`url.Parse("chrome://extensions/")`
// 会把 `extensions` 当成 Host——那不是主机名，那是这条协议的路径段，而屏上
// 显示成「extensions」看上去像一个域名。判据与 `model.ExternalScheme` 分家：
// 那一条判"点不点得开"，这一条判"这一行显示什么"。
//
// 解析不出来或不是 http/https 时**原样返回**：那一格宁可显示一个看不出是地址
// 的词，也不该显示一个编出来的主机名。
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return raw
	}
	return u.Host
}

// shortTime 把 statusRows 的 RFC3339 UTC 原文渲染成这一台机器上的人读得懂的时间// （按服务器时区——它就摆在客厅）。解析不出来**原样返回**：这一格宁可难看，
// 也不能编一个时间出来。
//
// **按语言走形状**：同一个 `01-02 15:04` 在中文读者那里读成"10 月 11 日"，
// 在英语读者那里分不清是 11 月 10 日还是 10 月 11 日——月在前还是日在前，
// 两种读法都成立。英文给月份名（`11 Oct, 15:04`），中文把单位也带上
// （`10月11日 15:04`），两边都不再靠读者猜。
//
// lang 显式传进来而不是自己去读 store：这一层没有 Server，而"用哪种语言排版"
// 是渲染侧判据的一部分，调用点看得见比藏起来好（与 Server.t 同一理由）。
func shortTime(lang, s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	t = t.Local()
	if lang == string(i18n.En) {
		return t.Format("2 Jan, 15:04")
	}
	return t.Format("1月2日 15:04")
}

// clockTime 只给"几点几分"，不给日期。
//
// 改动历史的一行**已经**被那一节的标题说清了是哪天（「今天」「昨天」「10月9日」），
// 行首再印一遍日期就是一行里两处说同一件事，而那一行本来就很窄。
// 解不出时间时返回空串而不是原文：这一格里塞一段 `2026-10-13T09:00:00Z` 会把
// 整行挤掉，而那件事已经由 historyDays 那一节用「时间不明」说过了——同一句
// 解释不需要在两处各印一半。
func clockTime(lang, s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return t.Local().Format("15:04")
}

// historyDay 是页上的一节：一天。
type historyDay struct {
	Label string
	Rows  []HistoryRow
}

// historyDays 把行切成"一天一节"，**节按日期从新到旧**，同一节内的行保持递进来的顺序。
//
// 不按"相邻同一天"切（那是这一条第一版的写法）：审计线的顺序是**追加顺序**，
// 不是时间顺序。一次恢复旧备份、一台走偏的时钟、一行 upgrade 前留下的旧数据，
// 都会让同一个日子在页面上出现两次——取景实拍到的正是这个形状：这台机器上
// 「今天」被切成两节，中间夹着一节 10 月 11 日，而那一页从头到尾没有一行是
// 重复的。节标题重复两次比少一节更坏：人会以为自己漏看了什么。
//
// 也不给行按时间重排：那一天的先后靠的是"上面那条更近"，而行与行之间真实
// 差别的只有分钟，追加顺序就是它。解不出时间的行**不丢**，单独一节、排最后——
// 这一页存在的全部意义是"一行都没漏"（与导入报告"## 没能进来"逐条点名、
// 回退页"另有 N 份快照读不出来"同一条纪律）。
func historyDays(rows []HistoryRow, cat *i18n.Catalog) []historyDay {
	lang := string(cat.Lang())
	now := time.Now().Local()
	type bucket struct {
		day   time.Time // 那一天的零点，只用来排序
		label string
		rows  []HistoryRow
	}
	var dated []*bucket
	var undated *bucket
	seen := map[string]*bucket{}
	for _, r := range rows {
		t, err := time.Parse(time.RFC3339, r.At)
		if err != nil {
			if undated == nil {
				undated = &bucket{label: cat.T("history.day.unknown")}
			}
			undated.rows = append(undated.rows, r)
			continue
		}
		lt := t.Local()
		y, m, d := lt.Date()
		key := fmt.Sprintf("%04d-%02d-%02d", y, m, d)
		if b, ok := seen[key]; ok {
			b.rows = append(b.rows, r)
			continue
		}
		b := &bucket{day: time.Date(y, m, d, 0, 0, 0, 0, lt.Location()), rows: []HistoryRow{r}}
		switch {
		case sameDay(lt, now):
			b.label = cat.T("history.day.today")
		case sameDay(lt, now.AddDate(0, 0, -1)):
			b.label = cat.T("history.day.yesterday")
		default:
			// 日期本身就是那一节的标题，不再包一层句子：`10月4日` 与 `4 Oct`
			// 已经是两种语言各自的样子，而语言包里放一条 `%s` 只会多一处空壳。
			b.label = dayDate(lang, lt, now.Year())
		}
		seen[key] = b
		dated = append(dated, b)
	}
	sort.SliceStable(dated, func(i, j int) bool { return dated[i].day.After(dated[j].day) })
	out := make([]historyDay, 0, len(dated)+1)
	for _, b := range dated {
		out = append(out, historyDay{Label: b.label, Rows: b.rows})
	}
	if undated != nil {
		out = append(out, historyDay{Label: undated.label, Rows: undated.rows})
	}
	return out
}

// dayDate 是一天的标题里那个日期。跨年时要带年份：只写「2 Jan」的人读到
// 去年的时间那一节，会以为那是今年一月——而"哪一月的哪一天"正是这一节唯一的职责。
func dayDate(lang string, t time.Time, thisYear int) string {
	if t.Year() == thisYear {
		if lang == string(i18n.En) {
			return t.Format("2 Jan")
		}
		return t.Format("1月2日")
	}
	if lang == string(i18n.En) {
		return t.Format("2 Jan 2006")
	}
	return t.Format("2006年1月2日")
}

func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// profile 的 query 是服务端过滤的输入（spec §8.1）：判据只在 filter.go 那一份，
// 这里只负责把过滤后的 Groups 烤成视图、把命中数与总数交给模板。
//
// **这里曾经有一个 `home` 参数**，它是"这个会话当初是谁验证进来的"，供管理横幅
// 报显示名。代管态 2026-10-10 整条下线之后，ProfilePage 不再读第二份
// profile.json，这个参数与那条横幅一起没了——**它连同 `profileView.Home`
// 一起删除，不是留下的**（2026-10-12 复核时这段注释还在描述那个已经不存在的参数，
// 紧挨着一个没有 `home` 的函数签名，是那种"照着它加回去就会编译不过"的注释）。
//
// 现在这里唯一的身份格是 hasAdmin：顶栏那枚「管理」徽标画不画由它决定，
// 判据在 httpapi（本次请求有没有一把真的管理权），进缓存键的理由见 cacheKey。
// 权限判断不在这里、更不在模板。
func (e *executor) profile(p *model.Profile, b *model.Bookmarks, query, add, addTitle string, hasSnaps, hasAdmin bool, cat *i18n.Catalog) ([]byte, error) {
	fb, hits, total := matches(query, b)
	v := profileView{Profile: p, Theme: themeAttr(p.Theme), Compact: compactView(p.View), AddrLevel: addrLevelOf(p.AddrLevel), CSS: e.cssT,
		i18nView: newI18nView(cat), HasAdmin: hasAdmin,
		Query: strings.TrimSpace(query), Hits: hits, Total: total, Add: add, AddTitle: addTitle,
		Background: paintedBackground(p), BackgroundFile: backgroundFile(p),
		Bookmarks: profileData{Title: b.Title, Footer: b.Footer, OpenInNewTab: b.OpenInNewTab}}
	// 这一页的名字：起了就用，没起就按语言给通用默认（见 profileView.PageTitle）。
	v.PageTitle = strings.TrimSpace(p.PageTitle)
	if v.PageTitle == "" {
		v.PageTitle = v.T("profile.title.default")
	}
	for _, g := range fb.Groups {
		// 分组色只在这里算一次（唯一真相）：条目身上只带 --chip-h，
		// 分组色经 --sec-accent 进 CSS，挂载点两处（section 与侧栏色点），
		// 都由同一个 groupView.Accent 喂（Task 10 与 P1-④ §7 的色点消费）。
		gv := groupView{ID: g.ID, Title: g.Title, Layout: g.Layout,
			Accent: validatedAccent(g.Accent), AccentCSS: CSSAccent(g.Accent)}
		gv.Parents, gv.Leaf = pathSplit(g.Title)
		gv.Depth = strings.Count(g.Title, pathSep)
		for _, it := range g.Items {
			p := pieceFor(it, e.ix, e.fc)
			gv.Items = append(gv.Items, itemView{
				ID: it.ID, Name: it.Name, URL: it.URL, Desc: it.Desc, Icon: it.Icon,
				Alias: it.Alias, Tags: it.Tags,
				// 协议判据只有一个来源：model.ExternalScheme。字符串前缀
				// 各写一份，校验与渲染就会在某天分家。
				External: model.ExternalScheme(it.URL),
				SVG:      p.SVG, Img: p.Img, Missing: p.Missing,
				Letter: p.Letter, ChipHue: p.ChipHue,
			})
		}
		gv.Count = len(gv.Items)
		v.Bookmarks.Groups = append(v.Bookmarks.Groups, gv)
	}
	// 导航在**分组搬走之前**建：设成大卡不该让这一组从分组导航里消失
	// （它只是换了位置，而"它不见了"才是真问题）。
	v.Bookmarks.Nav = groupNav(v.Bookmarks.Groups)
	// **大卡那一组从正文里拿走**（2026-10-14）：设成 feature 的分组不进
	// `v.Bookmarks.Groups`，而是进 `v.Featured`——它在页面顶部独占一块，
	// 而不是排在第七组等着被滚到。
	//
	// 判据只有这一句 `Layout == model.LayoutFeature`，而它用的常量与写侧
	// `ValidGroupLayout` 是同一个名字，所以"哪些分组是大卡"在盘上、ops 与
	// 页面上是同一件事。
	//
	// **顺序**：大卡之间保持盘上的次序，正文之间也是（都按各自那一份的顺序），
	// 而"挪"这件事本身不重排任何人——把第三组设成大卡不会把第五组往前顶。
	{
		rest := v.Bookmarks.Groups[:0]
		for _, gv := range v.Bookmarks.Groups {
			if gv.Layout == model.LayoutFeature {
				v.Featured = append(v.Featured, gv)
				continue
			}
			rest = append(rest, gv)
		}
		v.Bookmarks.Groups = rest
	}
	// ⑤ 的脚本与 rev 由执行器填，不从视图结构体带进来：指纹路径由内容算出，
	// 而 rev 就在 b 上（渲染这一页读的就是它）。视图结构体要是能带这两个，
	// 就会多出"调用方忘了填 → 页面静默没有添加按钮"这种形状。
	v.AppJS = AppJSPath
	v.AppCSS = AppCSSPath
	v.Rev = b.Rev
	v.Empty = countItems(b) == 0
	v.HasSnapshots = hasSnaps
	var buf bytes.Buffer
	if err := e.profileT.ExecuteTemplate(&buf, "profile", v); err != nil {
		return nil, fmt.Errorf("渲染档案 %s: %w", p.ID, err)
	}
	return buf.Bytes(), nil
}

func (e *executor) picker(v pickerView, cat *i18n.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	// CSS 与语言都由执行器填：视图结构体从 httpapi 传来时不该带着内嵌文件的引用，
	// 那等于让调用方负责注入样式（profile/broken/admin 三页同一形状）。
	v.CSS = e.cssT
	v.i18nView = newI18nView(cat)
	if err := e.pickerT.ExecuteTemplate(&buf, "picker", v); err != nil {
		return nil, fmt.Errorf("渲染进门页: %w", err)
	}
	return buf.Bytes(), nil
}

func (e *executor) broken(id, reason string, cat *i18n.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.brokenT.ExecuteTemplate(&buf, "broken", brokenView{
		i18nView:  newI18nView(cat),
		ProfileID: id, Reason: reason, CSS: e.cssT}); err != nil {
		return nil, fmt.Errorf("渲染坏档案页 %s: %w", id, err)
	}
	return buf.Bytes(), nil
}

func (e *executor) admin(rows []AdminRow, notice string, cat *i18n.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.adminT.ExecuteTemplate(&buf, "admin", adminView{
		i18nView: newI18nView(cat),
		Rows:     rows, Notice: notice, CSS: e.cssT, Count: len(rows)}); err != nil {
		return nil, fmt.Errorf("渲染管理页: %w", err)
	}
	return buf.Bytes(), nil
}

// adminLogin 渲染"输管理 key"那一页。判据只有 Notice 一格（httpapi 白名单给的），
// 模板不自己判断该不该说失败。
func (e *executor) adminLogin(notice string, cat *i18n.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.adminLoginT.ExecuteTemplate(&buf, "adminLogin", adminLoginView{
		i18nView: newI18nView(cat), Notice: notice, CSS: e.cssT}); err != nil {
		return nil, fmt.Errorf("渲染管理页登录: %w", err)
	}
	return buf.Bytes(), nil
}

// notFound 渲染打错地址那一页。判据（这一条请求是不是浏览器导航）在 httpapi，
// 模板只印，与 adminLogin 同一处置。
func (e *executor) notFound(back string, cat *i18n.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.notFoundT.ExecuteTemplate(&buf, "notFound", notFoundView{
		i18nView: newI18nView(cat), Back: back, CSS: e.cssT}); err != nil {
		return nil, fmt.Errorf("渲染找不到页: %w", err)
	}
	return buf.Bytes(), nil
}

// adminAction 渲染设 PIN / 删档的确认页。判据全在 AdminAction 里，模板只印。
func (e *executor) adminAction(v AdminAction, cat *i18n.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.adminActionT.ExecuteTemplate(&buf, "adminAction", adminActionView{
		AdminAction: v, CSS: e.cssT, i18nView: newI18nView(cat)}); err != nil {
		return nil, fmt.Errorf("渲染确认页: %w", err)
	}
	return buf.Bytes(), nil
}

// prefsPage 渲染台面面板的**独立页**（GET /prefs）。
//
// 它与档案页里页内嵌的那一份浮层渲染的是**同一个 define**（profile.tmpl 的
// `prefs-panel`），所以两条路永远一致——这正是把面板定义**留在 profile.tmpl 里**
// 而不是另开一个 prefs.tmpl 的理由：跨文件共享模板要把所有模板并成一个集合，而
// `icon` 在 profile.tmpl 与 picker.tmpl 里重名，并进去会让其中一份被静默覆盖。
//
// 视图复用 profileView：面板要读的 `.T`、`.Profile`、`Lang`、`.Theme`、`.CSS`
// 与 `HasSnapshots` 全在那上面，另建一个结构体只会多一份要同步的字段清单。
//
// openInNewTab 走 `v.Bookmarks`（**不是**新增一个字段）：面板上凡是书签文档里的
// 格子都该从同一处读——`.Bookmarks` 已经是那个容器，而档案页上它装的是 Title/
// Footer/OpenInNewTab。为它另开一个字段，模板上就会有两种写法指同一份数据。
// 台面面板左侧那一列的三个值（2026-10-15）。**它们是寻址键**：`?tab=` 就是这一列
// 当前停在哪，而分组那一节里的每一次按色块都是一次提交——停在哪必须活过一次
// 保存，否则"改完第二十组跳回第一组"。
//
// 为什么是三个而不是每行一个：tab 的价值是跳得远，而那一屏只有三处断层
// （三个分节头）；再往下拆量到的是每格跳 100px，那是噪声不是导航。
const (
	PrefTabLook   = "look"   // 外观
	PrefTabGroups = "groups" // 分组
	PrefTabOut    = "out"    // 回退与历史
)

// PrefTabValues 是那张名单的**可查形状**（httpapi 侧按它收 `?tab=`）。三处
// 各写一遍 `switch` 就会有一处忘了改，所以名单在这里、判据由它出去。
var PrefTabValues = []string{PrefTabLook, PrefTabGroups, PrefTabOut}

func (e *executor) prefsPage(p *model.Profile, hasSnaps bool, gs []groupView, broken string, openNewTab bool, cat *i18n.Catalog, tab string) ([]byte, error) {
	v := profileView{Profile: p, Theme: themeAttr(p.Theme), Compact: compactView(p.View), AddrLevel: addrLevelOf(p.AddrLevel), CSS: e.cssT,
		i18nView: newI18nView(cat), HasSnapshots: hasSnaps,
		PrefGroups: gs, PrefGroupsBroken: broken,
		Bookmarks:  profileData{OpenInNewTab: openNewTab},
		Background: paintedBackground(p), BackgroundFile: backgroundFile(p),
		PrefPalette: model.GroupAccentPalette, MaxGroupTitleRunes: model.MaxGroupTitleRunes}
	// 当前停在哪一节。**空值落回第一颗**，不落回"三颗都没按"——后者是空屏。
	// 传进来的值由 httpapi 那一侧按名单收过（prefsTab），所以这里只兜住空串：
	// 面板也从档案页的浮层渲染一次，那条路不带 ?tab=。
	switch tab {
	case PrefTabOut, PrefTabGroups:
		v.PrefTab = tab
	default:
		v.PrefTab = PrefTabLook
	}
	// 色名在这里算，不在 model 里写死：那张表只给键名后缀，句子在语言包里。
	// 英文界面上那八颗从前印的是「蓝 青 绿…」，因为名字住在 Go 的一侧。
	v.PrefPaletteNames = make([]string, len(model.GroupAccentColorKeys))
	for i, slug := range model.GroupAccentColorKeys {
		v.PrefPaletteNames[i] = v.T("prefs.color." + slug)
	}
	// 引 app.css：面板里的输入框走 `.fld`、按钮走按钮族，两条都在那里。**不引**
	// 就得在 base.css 里另抄一份输入框的脸，于是"浮层里那个框"与"整页那个框"
	// 迟早分家（判据只有一份）。不带 AppJS——这一页没有浮层之外的交互，
	// 脚本是档案页才需要的。
	v.AppCSS = AppCSSPath
	v.PageTitle = strings.TrimSpace(p.PageTitle)
	if v.PageTitle == "" {
		v.PageTitle = v.T("profile.title.default")
	}
	var buf bytes.Buffer
	if err := e.profileT.ExecuteTemplate(&buf, "prefs-page", v); err != nil {
		return nil, fmt.Errorf("渲染台面面板页: %w", err)
	}
	return buf.Bytes(), nil
}
