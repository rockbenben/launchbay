package store

import (
	"errors"
	"path/filepath"
	"sync"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("exists")
	ErrInvalid  = errors.New("invalid")
	ErrCorrupt  = errors.New("数据文件读不了")
	// ErrReadable 不是"失败了"，是"这件事不该做"：档案读得动，重建会盖掉一份
	// 只是 PIN 忘了的档案。调用方（httpapi）把它答成 409 而不是 5xx。
	ErrReadable = errors.New("档案读得动，不需要重建")
)

type Store struct {
	root string

	bmu    sync.Mutex
	broken map[string]map[string]string // id → 来源(profile/bookmarks) → 原因

	// pmu 是档案**名单**级的串行化，与按 id 的那把锁（lmu/locks）两回事：
	// 它管"引导窗口只开得下一个人"。按 id 的锁看不见"两个不同 id 各建一个"
	// 这种组合，那正是引导通路最贵的失败形状（见 bootstrap.go）。
	pmu   sync.Mutex
	lmu   sync.Mutex
	locks map[string]*sync.Mutex
}

func New(root string) *Store { return &Store{root: root} }

func (s *Store) Root() string { return s.root }

func (s *Store) profilesDir() string { return filepath.Join(s.root, "profiles") }

func (s *Store) ProfileDir(id string) string { return filepath.Join(s.profilesDir(), id) }

func (s *Store) ProfilePath(id string) string {
	return filepath.Join(s.ProfileDir(id), "profile.json")
}

func (s *Store) BookmarksPath(id string) string {
	return filepath.Join(s.ProfileDir(id), "bookmarks.json")
}

func (s *Store) SnapshotsDir(id string) string {
	return filepath.Join(s.ProfileDir(id), "snapshots")
}

func (s *Store) UsagePath(id string) string {
	return filepath.Join(s.ProfileDir(id), "usage.json")
}

func (s *Store) HistoryPath() string { return filepath.Join(s.root, "history.jsonl") }

// IconsDir 是站级的 favicon 缓存目录：按内容哈希命名，不含档案 id
// （spec §4.1、§5）。删档不动它（spec §6.1 的 profiles DELETE 那条）。
func (s *Store) IconsDir() string { return filepath.Join(s.root, "icons") }

// IconBlobPath 的哈希与扩展名都由 Put 生成，形状固定成 <16 hex>.<ext>，
// 这样 /icons/{name} 那条路由只认这个形状就能同时挡掉路径穿越。
func (s *Store) IconBlobPath(hash, ext string) string {
	return filepath.Join(s.IconsDir(), hash+"."+ext)
}

func (s *Store) IconIndexPath() string { return filepath.Join(s.IconsDir(), "index.json") }
