// Package model 是落在磁盘上的数据结构。字段顺序就是 JSON 的键顺序，
// 改动顺序会让人/rebase 看到无关的整片 diff——要改就在同一处
// 把写出契约的测试一起改。
package model

const (
	LayoutTiles = "tiles"
	LayoutList  = "list"
	// LayoutFeature 是**第三种分组排法**：大卡。设成它的分组不再出现在正文的
	// 分组序列里，而是被挪到页面顶部独占一块（判据在 render 的 exec.go，
	// 判据的**名字**在这里）。2026-10-14 加：磁贴与单列是同一套 44px 行的两种
	// 排法，谁点都得眯着眼找；而"每天必点的那几个"要的是**一眼扫到**，不是
	// 排在第七组。
	LayoutFeature = "feature"
)

// 卡片第二行印什么（2026-10-19）。判据的**名字**在 model，渲染与 CSS 都读它。
const (
	// AddrHost：有说明印说明、没说明印主机名。这是默认，也是 2026-10-16 实拍
	// 改判定的样子——48 条真实数据里只有 4 张有说明，而人认一个站点靠的是
	// 它在哪个域上。
	AddrHost = "host"
	// AddrFull：一律印完整地址。给人"我想看到多细"用的一档。
	AddrFull = "full"
	// AddrNone：一律不印第二行。
	AddrNone = "none"
)

// Item 是一条书签。数组顺序即显示顺序，所以没有 order 字段：
// 两个真相迟早会打架（spec §4.3）。
type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Icon string `json:"icon,omitempty"`
	Desc string `json:"desc,omitempty"`
	// Alias 与 Tags 带 `omitempty`（2026-10-14）：**空集合不落盘**。
	//
	// 实测（用户真实数据 8 组 41 条）：`alias: []` 与 `tags: []` **41/41 全空**，
	// 而每条都照写不误——一份 10 KB 的文件里有 41 对空数组，而它们承载的信息量
	// 是零。加了它之后这份文件是 9.5 KB，且**读侧完全等价**：
	// 搜索（render/filter.go）只 `range` 这两格，nil 与 `[]` 走同一条路；
	// 模板的 `{{if .Alias}}` 对 nil 与 `[]` 都是假；ops 的 sameStrings 逐项比。
	//
	// **它顺带作废了一处读侧归一**（store/load.go 的 normalize）：那一段
	// 存在的唯一理由是"写 [] 不写 null"——磁盘上人为写下的 `"alias": null`
	// 解码成 nil，原样写回就是 null，**一次读→写往返之后稳定 diff 就废了**。
	// 而 omitempty 让 null 与空集合**都不写**，那个理由不再成立；留着它只会变成
	// 「读进来补成 []、写出去又省掉」的一次空转。null 的防护由 omitempty 接手。
	Alias  []string `json:"alias,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	AINote string   `json:"aiNote,omitempty"`
}

type Group struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Layout string `json:"layout"`
	Accent string `json:"accent,omitempty"`
	Items  []Item `json:"items"`
}

type SearchEngine struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
	URL    string `json:"url"`
}

type Search struct {
	Enabled bool           `json:"enabled"`
	Default string         `json:"default"`
	Engines []SearchEngine `json:"engines"`
}

type Weather struct {
	Enabled  bool   `json:"enabled"`
	Location string `json:"location"`
}

type Bookmarks struct {
	Rev          int64   `json:"rev"`
	Title        string  `json:"title"`
	Groups       []Group `json:"groups"`
	Search       Search  `json:"search"`
	Weather      Weather `json:"weather"`
	OpenInNewTab bool    `json:"openInNewTab"`
	Clock        bool    `json:"clock"`
	Footer       string  `json:"footer"`
}

type Profile struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// **这里没有 admin 字段**（2026-10-12）。管理员是机器级的一把 key
	// （LAUNCHBAY_ADMIN_TOKEN），不属于任何档案，于是"这个档案是不是管理员"
	// 这个问题不再有答案，也不该有。
	//
	// 升级前落盘的 profile.json 里那个 `"admin": true` **会**被解码器判成坏档——
	// `store.decodeProfile` 对未知键是严格的，而它必须严格（否则坏档再也发现不了，
	// rebuild 那条通路的判据来源就没了）。`admin` 是**唯一**被放过的那一格，后门在
	// `store/load.go`，理由与升级兼容的证人都在 `store/upgrade_compat_test.go`：
	// 不开后门的话，升级前落盘的每一个 profile.json 一起变成坏档，全家谁都进不去。
	// 不需要迁移脚本——下一次写盘时它自然消失。
	// Disabled 是"停用"，不是"删掉"（2026-10-10）：停用的档案数据一个字节不动、
	// 仍占 CountProfiles 的位置（引导窗口不会重开），但输对 PIN 也进不去。
	// 删档不可逆——`revert` 救不了它，快照跟着档案一起没了；而"某人暂时不用了"
	// （孩子放假、某人住院、租客走了）是常事，不该逼着人在"留着"与"永久删除"
	// 之间二选一。与 Nextcloud 的 enable/disable 同一形状。
	//
	// 它是**布尔而不是三态**：没有"停用中"这种中间态。缺字段解码成 false，
	// 于是升级前落盘的 profile.json 不用动一个字就是合法的（§4.3 的向后兼容）。
	Disabled  bool   `json:"disabled"`
	PIN       string `json:"pin"`
	CreatedAt string `json:"createdAt"`
	Theme     string `json:"theme"`
	Layout    string `json:"layout"`
	// View 是**整页怎么排**："" / "groups" = 一组一节（默认，今天的样子），
	// "wall" = 全局视图（各组作为列头排进多栏），"dense" = 密集视图（去壳紧凑，高信息密度）。
	// 与 Layout 不是一件事：sidebar/topbar 说的是"分组导航住在哪一边"，
	// 这一格说的是"条目怎么铺"，两格可以任意组合。
	//
	// **空串是合法值而不是坏档**：这一格比老盘晚，升级时每一份 profile.json 都
	// 没有它，判坏档等于让全机器的档案进不去（Profile.Validate 挂在读路径上）。
	// 与 PageTitle 同一形状：默认值只在渲染时算，不往盘里编一个字。
	View string `json:"view"`
	// AddrLevel 是**卡片第二行印什么**（2026-10-19 加）："" / "host" = 默认
	// （有说明印说明、没说明印主机名）、"full" = 一律印完整地址、"none" = 不印。
	//
	// 它与 View 正交：View 说的是整页怎么排（会不会本来就不印第二行），这一格
	// 说的是第二行**说什么**。所以 full 要压过 View 自己的隐藏规则——否则这个
	// 开关在 dense 与窄屏磁贴下会"开了没反应"。
	//
	// **空串是合法值而不是坏档**，与 View / PageTitle 同一形状：默认值只在渲染
	// 时算，不往盘里编一个字（store/load.go 那条"停止编造默认值"）。
	// **不写 `omitempty`**（2026-10-19）：与 View / PageTitle 同一形状。
	// 那一族**空串也要落盘**——`"view": ""` 在盘上看得见，而它是读侧那条
	// 「这一格被考虑过、只是选默认」的判据；省掉它，键存在与键缺失就分不开了，
	// 于是"没选过"与"选了默认"在盘上成了两件事。与它相反的写法在本项目里只有
	// 条目那几个可选内容格（icon/desc/accent），那几格空着是真的没有内容。
	AddrLevel string `json:"addrLevel"`
	// PageTitle 是**这一页叫什么**（顶栏那行大字与浏览器标签页的前半），
	// 与上面的 Title（**这是谁的那一份**）是两件事：一家人共用一台机器时，
	// 「爸爸的台」回答的是"谁"，而这一页本身可以叫"我的书签"、"影音室"、
	// "工作用的那些"。空 = 没起过，渲染侧按界面语言给一个通用默认
	// （`profile.title.default`），**不往盘里编一个值**——与 Theme 那格
	// "缺字段就是默认"同一形状（store/load.go 那条"停止编造默认值"的判据）。
	//
	// 为什么不住在 `Bookmarks.Title` 里：那一格是**书签文档**自己的标题，
	// 跟着导出/导入走、改它要过 rev 与快照（它是那份数据的一部分），
	// 而"这一页显示什么名字"与 theme/layout 同类——是台面偏好，不是数据。
	// 住进 profile.json 还顺带避开了"batch 的 set 路径必须以 groups 开头"
	// 那条判据要不要为档案级字段开特例的问题。
	PageTitle string `json:"pageTitle"`
	// Background 是**这一页自己的背景图**（2026-10-14）：形如
	// `<16 位小写十六进制>.<png|jpg|webp|gif>`，指向 `data/backgrounds/<id>/`
	// 下那个按内容哈希命名的文件；空 = 没设。
	//
	// 住 profile.json 而不是 bookmarks.json，理由与 PageTitle 同一句：它是台面
	// 偏好，不是书签数据——改它不该产生 rev 与快照（§6.1 `/prefs` 只写
	// profile.json），也该跟得上导出/导入的边界（导出只走 bookmarks.json，
	// 所以背景**不随导出走**，它跟着这台机器走）。
	//
	// **哈希 + 按档案分目录**是两条判据的合：哈希那一半让同一张图在换背景时
	// 换 URL（浏览器缓存立刻失效，不会拿旧图当新图）；按档案那半让"删掉背景"
	// 只需要删自己那一个文件——两个人用同一张图时，各删各的（内容寻址的单份
	// 存储会让 A 删掉之后 B 的背景跟着消失，而"谁在用"这个问题的答案会变）。
	//
	// BackgroundOff 是**暂时不显示**，不是删：家里来客人时不想让照片挂在电视
	// 墙上，而不想删掉重传。它是这一份的事（电视上关了，手机上也该关），所以
	// 存在档案里而不是浏览器的本地存储。
	//
	// 形状**不在 Validate 里卡**（与 icon 名同一纪律，§5/§6.2）：判据只在写侧
	// （上传那条端点算出来的）与读侧的净化（render）两处，盘上写坏的值最多是
	// "这张背景不见了"，而把档案判成坏档会让连得进这台机器的人都进不去。
	// 那个形状由 check.go 的 ValidBackground 认（判据一份，写侧与读侧共用）。
	Background    string `json:"background"`
	BackgroundOff bool   `json:"backgroundOff"`
	// Language 是**这一份自己的界面语言**（2026-10-20 加）。
	//
	// 空串 = **没选过** = 跟着站级的默认语言（管理员在管理页设的那一个，住在
	// data/settings.json）。判据只有一处：i18n.ProfileLang。
	//
	// 为什么住进档案（2026-10-20 改判）：语言是**阅读偏好**，跟着读的人走。
	// 从前它是站级的，理由是"一家人共用一台导航台，谁也不该被别人改掉阅读
	// 语言"——那条理由在管理员变成一把机器级的 key 之后就不成立了：管理员不
	// 属于任何档案，而"这一份用哪种语言"恰恰是这一份自己的事。改完之后那条
	// 边界反而更干净：**每一份只能改自己那一份**（谁也改不了别人的页面语言），
	// 管理员改的是**默认**，只影响还没自己选过的人。
	//
	// **不写 `omitempty`**：与 View / PageTitle 同一形状——空串也要落盘，
	// 于是"没选过"与"选了跟随默认"在盘上分得开。
	//
	// **不在 Validate 里卡**（与 icon 名、背景同一纪律）：写侧严格（prefs 表单
	// 拒不认识的值），读侧净化（不认识的值落默认）。把档案判成坏档的代价是
	// 那个人再也进不去，而这一格写坏最多是"他看到的是默认语言"。
	Language string `json:"language"`
}
