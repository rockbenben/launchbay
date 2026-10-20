// Package ops 把"一批写请求"作用到书签上。它是纯函数：不碰磁盘、不碰 HTTP。
// 全部错误都要回报到具体是第几条 op 的哪个字段——AI 一次改 30 条时，
// "改坏一半"比"全没改"难收拾得多（spec §6.2）。
package ops

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

type kind int

const (
	kindGroups     kind = iota + 1 // groups —— 组表本身（add 组用）
	kindGroup                      // groups/<gid>
	kindGroupField                 // groups/<gid>/<field>
	kindItems                      // groups/<gid>/items —— 条目表（add 条目用）
	kindItem                       // groups/<gid>/items/<iid>
	kindItemField                  // groups/<gid>/items/<iid>/<field>
	kindDocField                   // <field> —— 文档自己的标量字段（set 用，见 docFields）
)

type target struct {
	kind    kind
	groupID string
	itemID  string
	field   string
}

// groupFields 与 itemFields 是 set 能碰的字段白名单。不在表里的字段一律拒，
// 因为"能 set 什么"就是写接口的攻击面：rev、id 都不许被 set 改
// （id 改了等于换了一个寻址键，历史与快照会指错条目）。
var groupFields = map[string]bool{"title": true, "layout": true, "accent": true}

var itemFields = map[string]bool{
	"name": true, "url": true, "icon": true, "desc": true,
	"alias": true, "tags": true, "aiNote": true,
}

// docFields 是**文档自己那几格**的可写白名单，与上面两张表同一条原则
// （"能 set 什么就是写接口的攻击面"）。
//
// 2026-10-14 改判：**原来那道闸是「路径必须以 groups 开头」**，那是这条原则的
// 一个粗粒度实现——它把整张文档的标量一并挡在外面，代价是 `openInNewTab` 这类
// **页面上真的画得出来**的字段一个都改不了（2026-10-10 逐条探过，六条全拒，而
// 那六条里有五条本来就不该拒）。原则从来没说过"set 只能碰树"，它说的是
// "set 能碰的字段必须在表内"——而这本来就是靠白名单实现的机制。
//
// 在表内：`title` / `footer`（页脚那一行）、`openInNewTab`（点开一条书签是不是
// 新标签页）、`clock`——都是**标量，且页面画得出来**。
//
// 不在表内，三条理由各不相同：
//   - `rev`：**它是并发判据本身**（expectedRev 就跟它比），能 set 它等于让调用方
//     自己宣布"我看过最新版本了"。
//   - `groups`：树根。`add path=groups` 是加一个分组（既有形状）；若 `set path=groups`
//     也能整表替换，那正是 §4.4 明写不给的「整表替换型 op」——一次换整表会把别人的
//     并发写入抹掉。
//   - `search` / `weather`：对象不是标量，而且**页面一次都不渲染它们**（最初计划里
//     的搜索引擎与天气，没有做）。给一个画不出来的东西开写口，等于请人来填一个
//     看不见的字段。
var docFields = map[string]bool{
	"title": true, "footer": true, "openInNewTab": true, "clock": true,
}

// docBoolFields 是 docFields 里**布尔**的那两格。类型要在写侧卡住：`openInNewTab`
// 被写成字符串 "true" 时，页面上的 `{{if .OpenInNewTab}}` 会静默当假——症状是
// 「按了新标签页，没反应」，而功能测试一条都不会红。
var docBoolFields = map[string]bool{"openInNewTab": true, "clock": true}

// ParsePath 认 spec §4.4 的文法：groups/<groupId>[/items[/<itemId>]][/<field>]，
// 外加**根级标量字段** `<field>`（只认 docFields 里的四个，见上面那张表）。
// 段里的 id 必须是 ASCII slug——数组下标在这里没有意义，插入一条之后
// 下标就指错条目了。
func ParsePath(path string) (target, error) {
	parts := strings.Split(strings.TrimSpace(path), "/")
	if parts[0] != "groups" {
		// 根级那一格：白名单外的字段在这里就拒掉，而**理由要说得出是哪一类**——
		// "路径必须以 groups 开头"这句话对 rev/groups/search 三种拒绝是同一个答案，
		// 而它们的理由完全不同（见 docFields 上面的三条）。笼统一句的话，调用方
		// 改不动就会以为整条 ops 坏了。
		if len(parts) == 1 && parts[0] != "" {
			if docFields[parts[0]] {
				return target{kind: kindDocField, field: parts[0]}, nil
			}
			if parts[0] == "rev" {
				return target{}, errors.New("rev 不能改：它是并发判据本身（expectedRev 跟它比）")
			}
			if parts[0] == "groups" {
				return target{}, errors.New("groups 不能整表替换（§4.4：一次换整表会把别人的并发写入抹掉）；加一个分组用 add path=groups")
			}
			if parts[0] == "search" || parts[0] == "weather" {
				return target{}, fmt.Errorf("%q 是对象，且页面不渲染它——文档里能 set 的标量只有 %s", parts[0], docFieldList())
			}
			return target{}, fmt.Errorf("文档里没有 %q 这一格；能 set 的根级标量只有 %s", parts[0], docFieldList())
		}
		return target{}, errors.New("路径必须以 groups 开头，或者落在文档的标量字段上")
	}
	switch len(parts) {
	case 1:
		// 单段 `groups`：**add/remove 走它**（加一个分组、删一个分组），
		// 而 `set` 会在这条分支之外被拒（见 ops.go 的 setDocField 与那句
		// "整表替换"）。所以这里不再按 op 分流——分流的那一半在写侧。
		return target{kind: kindGroups}, nil
	case 2:
		if err := idErr(parts[1]); err != nil {
			return target{}, err
		}
		return target{kind: kindGroup, groupID: parts[1]}, nil
	case 3:
		if err := idErr(parts[1]); err != nil {
			return target{}, err
		}
		if parts[2] == "items" {
			return target{kind: kindItems, groupID: parts[1]}, nil
		}
		if !groupFields[parts[2]] {
			return target{}, fmt.Errorf("分组没有 %q 字段，能改的是 %s", parts[2], fieldList(groupFields))
		}
		return target{kind: kindGroupField, groupID: parts[1], field: parts[2]}, nil
	case 4:
		if err := idErr(parts[1]); err != nil {
			return target{}, err
		}
		if parts[2] != "items" {
			return target{}, fmt.Errorf("groups/%s 后面要跟 items，得到 %q", parts[1], parts[2])
		}
		if err := idErr(parts[3]); err != nil {
			return target{}, err
		}
		return target{kind: kindItem, groupID: parts[1], itemID: parts[3]}, nil
	case 5:
		if err := idErr(parts[1]); err != nil {
			return target{}, err
		}
		if parts[2] != "items" {
			return target{}, fmt.Errorf("groups/%s 后面要跟 items，得到 %q", parts[1], parts[2])
		}
		if err := idErr(parts[3]); err != nil {
			return target{}, err
		}
		if !itemFields[parts[4]] {
			return target{}, fmt.Errorf("条目没有 %q 字段，能改的是 %s", parts[4], fieldList(itemFields))
		}
		return target{kind: kindItemField, groupID: parts[1], itemID: parts[3], field: parts[4]}, nil
	}
	return target{}, fmt.Errorf("路径 %q 段数太多", path)
}

// Shape 是路径分解后**给读者看**的那一份。
//
// 为什么不直接把 target 导出：它是写侧的内部枚举与私有字段，导出就等于让显示层
// 能拿 kind 去猜写侧的分支（一处按 `kindItemField` 判断，文法就分家了）。
// 而历史页需要的只是"哪一段是分组 id、哪一段是条目 id、最后那段是字段名"。
//
// 走 ParsePath 而不是自己再切一遍 '/'：文法必须只有一份。写侧将来加一种形状，
// 这里要么跟着多一个 Kind 值，要么落进 Unknown——**不会**出现"显示层自己猜出
// 一个写侧不认的形状"那种看起来能用、实际指着不存在的东西。
type Shape struct {
	Kind    string // Groups / Group / GroupField / Items / Item / ItemField / Unknown
	GroupID string
	ItemID  string
	Field   string
}

const (
	ShapeGroups     = "Groups"     // groups —— 组表本身
	ShapeGroup      = "Group"      // groups/<gid>
	ShapeGroupField = "GroupField" // groups/<gid>/<field>
	ShapeItems      = "Items"      // groups/<gid>/items
	ShapeItem       = "Item"       // groups/<gid>/items/<iid>
	ShapeItemField  = "ItemField"  // groups/<gid>/items/<iid>/<field>
	// ShapeUnknown 是"这条路径不在文法里"。历史行里真会出现它：账号类动作
	// 记的是 profile/pin、profile/rebuild、snapshots/<rev>（§4.6），它们动的不是
	// 书签树，本来就不归这套文法管。
	ShapeUnknown = "Unknown"
)

// ShapeOf 把一条历史里的路径拆成 Shape。解不开不报错，返回 ShapeUnknown，
// 由调用方按"这是另一种动作"渲染——历史页要能渲染**整条审计线**，
// 而其中任何一行都不该让这一页 500。
func ShapeOf(path string) Shape {
	t, err := ParsePath(path)
	if err != nil {
		return Shape{Kind: ShapeUnknown}
	}
	k := ShapeUnknown
	switch t.kind {
	case kindGroups:
		k = ShapeGroups
	case kindGroup:
		k = ShapeGroup
	case kindGroupField:
		k = ShapeGroupField
	case kindItems:
		k = ShapeItems
	case kindItem:
		k = ShapeItem
	case kindItemField:
		k = ShapeItemField
	}
	return Shape{Kind: k, GroupID: t.groupID, ItemID: t.itemID, Field: t.field}
}

func idErr(s string) error {
	if !model.ValidSlug(s) {
		return fmt.Errorf("id %q 不是 ASCII slug", s)
	}
	return nil
}

// docFieldList 把根级白名单按**固定顺序**列成一句（给 422/解析失败时的那句话）。
// 手写列表会与 docFields 漂，而这句话是给人读的——它必须说的是同一份表。
func docFieldList() string { return fieldList(docFields) }

// GroupFields / ItemFields / DocFields 是那三张白名单的**只读副本**（字典序）。
//
// **给谁用**：文档闸。AGENTS.md 与机器契约（openapi.json）都逐字列出了这三张表，
// 而它们是**手抄的**——加一档字段而文档没改，于是人照着文档写出一个被 422 拒掉的
// 字段。三张表导出成可排序的副本，那道闸就能拿**代码里那份**去对照文档，
// 而不是拿文档里那份去对照文档。
//
// 为什么是副本而不是直接导出那张 map：map 可写，导出它等于让任何人都能往写侧的
// 白名单里塞字段——判据只有一份，且那份不该由调用方改。
func GroupFields() []string { return sortedKeys(groupFields) }

// ItemFields 是**条目**那一格的白名单副本。
func ItemFields() []string { return sortedKeys(itemFields) }

// DocFields 是**文档自己那几格**（根级标量）的白名单副本。
func DocFields() []string { return sortedKeys(docFields) }

// sortedKeys 把一张白名单的键按字典序取出来（副本，调用方改了它也不影响白名单）。
func sortedKeys(m map[string]bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// fieldList 把一张白名单按**字典序**列成一句。
//
// **为什么生成而不是手抄**（2026-10-20）：这三张表与这三句话是**同一个东西的
// 两种写法**，而它们会分家——2026-10-20 实测到过：组 layout 的枚举加了一档
// （feature），手抄的拒绝语没跟上，于是服务端报出一句不含新档的话，而人照着
// 它去猜"那到底能用哪几档"。字典序是为了**每次都一样的顺序**（map 的遍历顺序
// 是随机的，说出去的话不能每次换位置）。
func fieldList(m map[string]bool) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, " | ")
}
