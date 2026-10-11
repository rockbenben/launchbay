package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// snapshotKeep 是 spec §4.1 的"保留 50 份"。它是恢复窗口的全部长度：
// 再多也没人回头翻 51 版，再少会让一次成批误删变成不可恢复。
const snapshotKeep = 50

// WriteSnapshot 把当前的 bookmarks.json 复制进 snapshots/<rev>/。
//
// 必须在任何写入之前调用：快照保的是"这一次写入之前的状态"，反过来
// 先写再快照等于什么都没保。
//
// rev 取磁盘上那份的 rev，不取调用方内存里的：两者不一致时（外部程序
// 改过文件）快照要保的是即将被覆盖的那一份，因为 revert 会拿它覆盖磁盘。
//
// 档案还没有 bookmarks.json（新建档案）时返回 (0, nil)：无东西可保，
// 不是错误——否则第一个 add op 会永远写不进去。
func (s *Store) WriteSnapshot(id string) (int64, error) {
	src := s.BookmarksPath(id)
	raw, err := os.ReadFile(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("读 %s 做快照: %w", src, err)
	}
	b, err := DecodeBookmarks(src, raw)
	if err != nil {
		return 0, fmt.Errorf("快照前 %s 就读不了，拒绝写入: %w", src, err)
	}
	dir := filepath.Join(s.SnapshotsDir(id), strconv.FormatInt(b.Rev, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("建快照目录 %s: %w", dir, err)
	}
	if err := s.WriteFileAtomic(filepath.Join(dir, "bookmarks.json"), raw); err != nil {
		return 0, fmt.Errorf("写快照 %s: %w", dir, err)
	}
	if err := s.pruneSnapshots(id); err != nil {
		// 裁剪失败不影响这一次写入的正确性，只让磁盘多留几份：
		// 报出来，但 rev 仍然有效（调用方按"快照成功"处理）。
		return b.Rev, fmt.Errorf("裁剪快照: %w", err)
	}
	return b.Rev, nil
}

func (s *Store) pruneSnapshots(id string) error {
	list, err := s.ListSnapshots(id)
	if err != nil {
		return err
	}
	if len(list) <= snapshotKeep {
		return nil
	}
	for _, rev := range list[snapshotKeep:] {
		p := filepath.Join(s.SnapshotsDir(id), strconv.FormatInt(rev, 10))
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("删旧快照 %s: %w", p, err)
		}
	}
	return nil
}

// ListSnapshots 从新到旧。非数字目录名一律忽略。
func (s *Store) ListSnapshots(id string) ([]int64, error) {
	ents, err := os.ReadDir(s.SnapshotsDir(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("列快照: %w", err)
	}
	var revs []int64
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		rev, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil {
			continue
		}
		revs = append(revs, rev)
	}
	sort.Slice(revs, func(i, j int) bool { return revs[i] > revs[j] })
	return revs, nil
}

// SnapshotMTime 是那份快照**落盘的时间**。回退页要回答的是"上周三那版"，
// 而版本号本身不带时间；读不到（文件不在）返回零值，由调用方决定怎么说。
func (s *Store) SnapshotMTime(id string, rev int64) (time.Time, bool) {
	p := filepath.Join(s.SnapshotsDir(id), strconv.FormatInt(rev, 10), "bookmarks.json")
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}, false
	}
	return st.ModTime(), true
}

func (s *Store) LoadSnapshot(id string, rev int64) ([]byte, error) {
	p := filepath.Join(s.SnapshotsDir(id), strconv.FormatInt(rev, 10), "bookmarks.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("rev %d 的快照不存在: %w", rev, ErrNotFound)
		}
		return nil, fmt.Errorf("读快照 %s: %w", p, err)
	}
	return raw, nil
}

func (s *Store) SnapshotCount(id string) int {
	list, err := s.ListSnapshots(id)
	if err != nil {
		// 计数是给人看的汇总信息，不是判据：列不出来就报 0，
		// 但**别把它当"没有快照"**——真要判断得用 ListSnapshots 的 error。
		return 0
	}
	return len(list)
}
