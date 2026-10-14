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

// ── 这一页自己的背景图（2026-10-14）──────────────────────────────────────
//
// `data/backgrounds/<档案 id>/<内容哈希>.<后缀>`：**按档案分目录**是这里唯一的
// 目录结构决定，理由写在 model.Profile.Background 那段——两个人用同一张图时
// 各存各的，于是"删掉背景"只需要删自己那一个文件（单份内容寻址会让 A 删掉
// 之后 B 的背景跟着消失，而"还有谁在用"这个问题的答案会变）。
//
// 与 IconsDir 的差别：图标缓存是**站级**的（按 origin 共享，谁都可能命中同一张），
// 背景是**按档案**的（§11：各人一份，别人看不见）。删档时也不动这里——
// blob 挂在 `backgrounds/<id>/` 下，而目录连同 profiles/<id>/ 一起没了。
func (s *Store) BackgroundsDir() string { return filepath.Join(s.root, "backgrounds") }

// BackgroundDir 是**一个档案**的背景目录。id 走的是 model.ValidSlug 那一档
// （ASCII slug，无点无斜杠），所以它拼不出越界的路径——判据在调用它的两处
// （上传端点与文件端点）都先过一遍 profile 存在性，那一层已经挡住了非法 id。
func (s *Store) BackgroundDir(id string) string {
	return filepath.Join(s.BackgroundsDir(), id)
}

// BackgroundPath 是 blob 的落盘位置。name 必须是 model.ValidBackground 认的形状
// （`[0-9a-f]{16}\.<白名单后缀>`）——**判据在写侧**，所以这个函数只把它拼进去，
// 不自己再判一次：两处各判一次就会有一处先漂。
func (s *Store) BackgroundPath(id, name string) string {
	return filepath.Join(s.BackgroundDir(id), name)
}
