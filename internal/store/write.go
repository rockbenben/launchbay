package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rockbenben/launchbay/internal/model"
)

// WithProfileLock 把同一档案的读写串起来，不同档案互不阻塞。
// 它只在本进程内有效——所以所有写路径都必须经服务进程进来，
// CLI 不许直接写盘（spec §4.5）。
func (s *Store) WithProfileLock(id string, fn func() error) error {
	l := s.profileLock(id)
	l.Lock()
	defer l.Unlock()
	return fn()
}

func (s *Store) profileLock(id string) *sync.Mutex {
	s.lmu.Lock()
	defer s.lmu.Unlock()
	if s.locks == nil {
		s.locks = map[string]*sync.Mutex{}
	}
	l, ok := s.locks[id]
	if !ok {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	return l
}

func (s *Store) SaveBookmarks(id string, b *model.Bookmarks) error {
	// 写出契约（spec §4.3「空数组写 [] 不写 null」）在写出入口兜一次底：
	// 内存里构造的 Bookmarks（P1-② 的 add ops 就是现例）不带 alias/tags
	// 时是 nil，不归一化就会把 null 烤进磁盘文件。
	normalize(b)
	if err := b.Validate(); err != nil {
		// 与读路径（decodeProfile/LoadBookmarks）对称：ErrInvalid 与
		// model.ErrValidation 都要能被 errors.Is 分出来。%v 会断链，
		// P1-② 的 422 分流依赖后者（账本 T5→T8/9 承诺）。
		return errors.Join(ErrInvalid, err)
	}
	return s.WriteFileJSON(s.BookmarksPath(id), b)
}

func (s *Store) SaveProfile(p *model.Profile) error {
	// model.Profile 没有数组字段，normalize（只吃 Bookmarks）在这里无事
	// 可做；null 防线对 profile.json 的意义由 bookmarks 侧与解码入口承担。
	if err := p.Validate(); err != nil {
		return errors.Join(ErrInvalid, err)
	}
	return s.WriteFileJSON(s.ProfilePath(p.ID), p)
}

// WriteFileJSON 先编码到内存再落盘。顺序反过来就会在编码失败时
// 把已有内容截断成半个 JSON——那是唯一会真正丢数据的失败模式。
// 编码一律走 MarshalJSONBytes：缓冲步骤只留一份实现，磁盘上的形状
// 不会随调用路径漂移（评审 Minor：MarshalJSONBytes 曾是只有测试
// 续命的死码，与这里重复）。
func (s *Store) WriteFileJSON(path string, v any) error {
	data, err := MarshalJSONBytes(v)
	if err != nil {
		return fmt.Errorf("编码 %s 失败: %w", filepath.Base(path), err)
	}
	return s.WriteFileAtomic(path, data)
}

// WriteFileAtomic 用同目录临时文件 + rename。Windows 上 os.Rename 能
// 覆盖已存在的目标（Go 内部走 MoveFileEx + REPLACE_EXISTING），跨平台一致。
func (s *Store) WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("建目录 %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("建临时文件: %w", err)
	}
	name := tmp.Name()
	discard := func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}
	if _, err := tmp.Write(data); err != nil {
		discard()
		return fmt.Errorf("写临时文件: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		discard()
		return fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("rename 到 %s: %w", path, err)
	}
	return nil
}
