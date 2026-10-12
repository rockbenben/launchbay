// Package ops 把"一批写请求"作用到书签上。它是纯函数：不碰磁盘、不碰 HTTP。
// 全部错误都要回报到具体是第几条 op 的哪个字段——AI 一次改 30 条时，
// "改坏一半"比"全没改"难收拾得多（spec §6.2）。
package ops

import (
	"errors"
	"fmt"
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

// ParsePath 认 spec §4.4 的文法：groups/<groupId>[/items[/<itemId>]][/<field>]。
// 段里的 id 必须是 ASCII slug——数组下标在这里没有意义，插入一条之后
// 下标就指错条目了。
func ParsePath(path string) (target, error) {
	parts := strings.Split(strings.TrimSpace(path), "/")
	if parts[0] != "groups" {
		return target{}, errors.New("路径必须以 groups 开头")
	}
	switch len(parts) {
	case 1:
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
			return target{}, fmt.Errorf("分组没有 %q 字段，能改的是 title/layout/accent", parts[2])
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
			return target{}, fmt.Errorf("条目没有 %q 字段，能改的是 name/url/icon/desc/alias/tags/aiNote", parts[4])
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
