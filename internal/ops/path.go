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

func idErr(s string) error {
	if !model.ValidSlug(s) {
		return fmt.Errorf("id %q 不是 ASCII slug", s)
	}
	return nil
}
