package store

import (
	"sort"
)

// 坏档有两个来源：profile.json 与 bookmarks.json，分开登记、分开清理。
// 混成一条记录不行：LoadProfile 的成功会顺手抹掉 bookmarks.json 的
// 坏记录（Task 4 钉过：坏书签要挺过档案列表），反之亦然。
const (
	brokenProfile   = "profile"
	brokenBookmarks = "bookmarks"
)

func (s *Store) markBroken(id string, source string, reason string) {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	if s.broken == nil {
		s.broken = map[string]map[string]string{}
	}
	if s.broken[id] == nil {
		s.broken[id] = map[string]string{}
	}
	s.broken[id][source] = reason
}

// clearBroken 在对应来源读成功后清掉它的标记：修好的档案必须能从
// Broken() 里消失，否则常驻 Store（main.go 只建一个）会把它坏一辈子，
// 污染坏档案页与 /api/v1/status。
func (s *Store) clearBroken(id string, source string) {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	srcs, ok := s.broken[id]
	if !ok {
		return
	}
	delete(srcs, source)
	if len(srcs) == 0 {
		delete(s.broken, id)
	}
}

// Broken 返回读失败的档案 id（排序后）。刚建好还没写过的空档案不在其中。
func (s *Store) Broken() []string {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	out := make([]string, 0, len(s.broken))
	for id, srcs := range s.broken {
		if len(srcs) > 0 {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Store) BrokenReason(id string) (string, bool) {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	srcs := s.broken[id]
	// profile.json 坏了更根本（档案本身都读不出来），两个来源都坏时先报它。
	if r, ok := srcs[brokenProfile]; ok {
		return r, true
	}
	r, ok := srcs[brokenBookmarks]
	return r, ok
}
