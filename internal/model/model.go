// Package model 是落在磁盘上的数据结构。字段顺序就是 JSON 的键顺序，
// 改动顺序会让人/rebase 看到无关的整片 diff——要改就在同一处
// 把写出契约的测试一起改。
package model

const (
	LayoutTiles = "tiles"
	LayoutList  = "list"
)

// Item 是一条书签。数组顺序即显示顺序，所以没有 order 字段：
// 两个真相迟早会打架（spec §4.3）。
type Item struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Icon   string   `json:"icon,omitempty"`
	Desc   string   `json:"desc,omitempty"`
	Alias  []string `json:"alias"`
	Tags   []string `json:"tags"`
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
	// "wall" = 全局视图（各组作为列头排进多栏，条目压成紧凑单行，一屏看全）。
	// 与 Layout 不是一件事：sidebar/topbar 说的是"分组导航住在哪一边"，
	// 这一格说的是"条目怎么铺"，两格可以任意组合。
	//
	// **空串是合法值而不是坏档**：这一格比老盘晚，升级时每一份 profile.json 都
	// 没有它，判坏档等于让全机器的档案进不去（Profile.Validate 挂在读路径上）。
	// 与 PageTitle 同一形状：默认值只在渲染时算，不往盘里编一个字。
	View string `json:"view"`
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
}
