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
	// 升级前落盘的 profile.json 里那个 `"admin": true` 会被 encoding/json 当
	// 未知字段丢掉——不需要迁移脚本，重新写盘之后它自然消失。
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
}
