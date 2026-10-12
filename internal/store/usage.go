package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

type UsageItem struct {
	ID     string `json:"id"`
	Clicks int    `json:"clicks"`
	LastAt string `json:"lastAt"`
}

// Usage 用切片不用 map：输出键序要稳定（§4.3 同一族约束）。
type Usage struct {
	Items []UsageItem `json:"items"`
}

// LoadUsage 读一个档案的点击计数。缺文件返回空（没点过就是没有），
// 文件坏了报 ErrCorrupt（调用方拒绝覆盖，见 AddUsage）。
//
// 前置包含闸（评审 Important 2）：与 DeleteProfile 同一道 profileDirChecked。
// handler 今天先过一遍 slug 校验，所以这里够不到——但 Task 8 已经裁定过
// "只有 handler 级闸不算数"，少一处调用点校验，LoadUsage("..") 读的就是
// root/usage.json（UsagePath 里的 Join 会把 ".." 清到上一层）。
func (s *Store) LoadUsage(id string) (*Usage, error) {
	if _, err := s.profileDirChecked(id); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(s.UsagePath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Usage{Items: []UsageItem{}}, nil
		}
		return nil, fmt.Errorf("读 %s: %w", s.UsagePath(id), err)
	}
	var u Usage
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, fmt.Errorf("usage.json 读不了: %w", ErrCorrupt)
	}
	if u.Items == nil {
		u.Items = []UsageItem{}
	}
	return &u, nil
}

// AddUsage 累加点击并返回"这些 id 在书签里找不到"。
//
// 找不到仍要记：前端上报与书签改动会交错（AI 刚删掉一条，有人正好点了它），
// 拒收会让前端的节流逻辑变成"悄悄丢数据"。同一次上报里重复的未知 id
// 只报一条（unknown 是"这些 id 我不认识了"的清单，不是计数器），
// 但每一次都算进 clicks。
//
// 读-改-写整体在档案锁内（CreateProfile 同一先例：串行化放在函数内部，
// 调用方就不可能"忘了包锁"）：usage.json 是读出来改完再写回去的，两个
// 并发上报不加锁会互相丢计数，锁还盖住"拒绝覆盖坏文件"那条判定——
// 检查与写入之间不能有人插队。
//
// 【重入陷阱】锁就在本函数内部，而 sync.Mutex 不可重入：调用方再写
// `WithProfileLock(id, func() { AddUsage(id, ...) })` 就是当场自锁死锁
// （Task 9 的 PIN RMW 是先例的反面教训——要包锁的必须是**没有**自锁的
// 原语）。需要与别的写入串行时，Task 11+ 请改用不自锁的私有 helper。
func (s *Store) AddUsage(id string, ids []string, at string) (*Usage, []string, error) {
	// 包含闸排在最前，也排在"空上报早退"之前：闸要是放在早退之后，
	// 非法 id 靠一次空上报就能溜过去（评审 Important 2，同 LoadUsage）。
	if _, err := s.profileDirChecked(id); err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		// 空上报不造文件：没点过就是没有。
		return &Usage{Items: []UsageItem{}}, nil, nil
	}
	var usage *Usage
	var unknown []string
	err := s.WithProfileLock(id, func() error {
		b, err := s.LoadBookmarks(id)
		if err != nil {
			// 原样传播，**不**吞 ErrNotFound（评审 Critical）：LoadBookmarks
			// 只在 profile.json 同时缺失时才报它——"档案在、bookmarks.json
			// 还没落过盘"走的是 EmptyBookmarks 那条空档案分支（LoadBookmarks 里那句
			// `return EmptyBookmarks(), nil`）。
			// 所以旧代码那句吞法换不来空档案，只换来幽灵目录：给一个不存在
			// 的 id 在 profiles/<id>/ 造出只有 usage.json 的目录，
			// ListProfiles 既列不出也标不了坏，DELETE /profiles 删不到它，
			// 而下一个用同一 id 建档的人直接继承死人的点击史。
			// 读在锁内、排在写之前，所以这条分支一个字都不落盘。
			return err
		}
		known := map[string]bool{}
		for _, g := range b.Groups {
			for _, it := range g.Items {
				known[it.ID] = true
			}
		}
		u, err := s.LoadUsage(id)
		if err != nil {
			// 坏 usage.json 拒绝写：一份可能可修的坏文件不该被计数覆盖。
			return err
		}
		idx := map[string]int{}
		for i, it := range u.Items {
			idx[it.ID] = i
		}
		reported := map[string]bool{}
		for _, want := range ids {
			if !known[want] && !reported[want] {
				reported[want] = true
				unknown = append(unknown, want)
			}
			if i, ok := idx[want]; ok {
				u.Items[i].Clicks++
				u.Items[i].LastAt = at
				continue
			}
			u.Items = append(u.Items, UsageItem{ID: want, Clicks: 1, LastAt: at})
			idx[want] = len(u.Items) - 1
		}
		// 排序必须在 WriteFileJSON 之前：盘上键序才是"输出键序稳定"这条
		// 约束的落点（挪到写盘之后，文件就是 z,a,m，内存断言照样全绿——
		// usage_test.go 的按字节位置证人钉的是这个）。
		sort.Slice(u.Items, func(i, j int) bool { return u.Items[i].ID < u.Items[j].ID })
		if err := s.WriteFileJSON(s.UsagePath(id), u); err != nil {
			return err
		}
		usage = u
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if unknown == nil {
		unknown = []string{}
	}
	return usage, unknown, nil
}
