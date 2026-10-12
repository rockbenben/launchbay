package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
)

// NewProfileInput 没有 Admin（2026-10-12）：档案不再有"是不是管理员"这个属性。
// 建出来的每一份都只是一个人的书签页，管这台机器是另一把 key 的事。
type NewProfileInput struct {
	ID      string
	Title   string
	PinHash string
}

// CreateProfile 建目录 + profile.json + bookmarks.json（rev 0）。
// **不带任何起步内容**：每个档案从零开始（模板机制 2026-10-10 整条拆掉，
// `NewProfileInput` 上曾有的 Template 字段也已删除）。
//
// 半途失败必须回收：留下只有 profile.json 的目录会让人在选单里
// 看到一个永远进不去的档案。
//
// 存在性检查、建目录、两次写入整体在同一档案 id 的锁内执行——
// 串行化放在函数内部，调用方就不可能"忘了包锁"；忘了包锁的调用点
// 会让两个同 id 并发请求双双通过 exists 检查再互相覆盖。
// WithProfileLock 仍导出：Task 10+ 的多步写路径还要用。
func (s *Store) CreateProfile(in NewProfileInput) (*model.Profile, error) {
	if !model.ValidSlug(in.ID) {
		return nil, fmt.Errorf("%w: 档案 id %q 必须是 ASCII slug", ErrInvalid, in.ID)
	}
	if in.PinHash == "" {
		return nil, fmt.Errorf("%w: 档案 %q 缺 PIN 哈希", ErrInvalid, in.ID)
	}

	// 全部校验都排在落盘之前。
	p := model.Profile{
		ID: in.ID, Title: in.Title, PIN: in.PinHash,
		CreatedAt: time.Now().Format(time.RFC3339), Theme: "auto", Layout: "sidebar",
	}
	if err := p.Validate(); err != nil {
		// Join 而非 %v：写路径上所有校验错都要同时保住 ErrInvalid 与
		// model.ErrValidation 两个哨兵（与 SaveProfile/SaveBookmarks 对称）。
		return nil, errors.Join(ErrInvalid, err)
	}
	b := EmptyBookmarks()
	if err := b.Validate(); err != nil {
		return nil, errors.Join(ErrInvalid, err)
	}

	var created *model.Profile
	err := s.WithProfileLock(in.ID, func() error {
		if _, err := os.Stat(s.ProfilePath(in.ID)); err == nil {
			return fmt.Errorf("%w: 档案 %q 已存在", ErrExists, in.ID)
		}
		if err := os.MkdirAll(s.ProfileDir(in.ID), 0o755); err != nil {
			return fmt.Errorf("建档案目录: %w", err)
		}
		if err := s.SaveProfile(&p); err != nil {
			s.rollbackDir(in.ID)
			return err
		}
		if err := s.SaveBookmarks(in.ID, b); err != nil {
			s.rollbackDir(in.ID)
			return err
		}
		created = &p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Store) rollbackDir(id string) {
	err := os.RemoveAll(s.ProfileDir(id))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// 回滚失败必须喊出来：留下半个档案比报错更难查。
		fmt.Fprintf(os.Stderr, "launchbay: 建档回滚失败 %s: %v\n", id, err)
	}
}

func (s *Store) profileDirWithin(id string) (string, bool) {
	dir := s.ProfileDir(id)
	if filepath.Base(dir) == id && filepath.Dir(dir) == s.profilesDir() {
		return dir, true
	}
	return "", false
}

// profileDirChecked 是"两道闸"的合体：ASCII slug（挡日常手滑）+
// profileDirWithin（挡越界）。DeleteProfile 与 usage 原语（usage.go 的
// AddUsage/LoadUsage）都走它——Task 8 的裁定是 handler 级闸不算数，
// 每个碰档案目录的原语都得自己带上，否则将来少写一处 handler 校验，
// AddUsage("..") 就是在 data root 落一份 usage.json。
// 合法 id 返回该档案目录，非法/越界一律 error（ErrInvalid 可被 errors.Is 分出）。
func (s *Store) profileDirChecked(id string) (string, error) {
	if !model.ValidSlug(id) {
		return "", fmt.Errorf("档案 id %q 不是 ASCII slug: %w", id, ErrInvalid)
	}
	dir, ok := s.profileDirWithin(id)
	if !ok {
		return "", fmt.Errorf("档案路径越界: %q", id)
	}
	return dir, nil
}

// DeleteProfile 删掉整个档案目录（含 snapshots）。三道闸：
//  1. id 必须是 ASCII slug；
//  2. 目录必须是 profilesDir 下以该 id 命名的直接子目录——用 profileDirWithin
//     的 Base==id && Dir==profilesDir 双检，而不是 filepath.Rel 那种"id 长得干净"
//     的伪包含检查（详见该函数注释，Rel 对 ".." 反而会放行）；
//  3. 拿该档案的写锁，别与正在进行的 batch 抢同一个目录。
//
// 闸 1+2 抽在 profileDirChecked 里（usage 原语走同一道闸，两道闸的口径
// 只有一份实现）。闸 1 挡掉日常手滑；闸 2 是纵深防御：万一将来放宽了
// slug 规则，闸 2 仍能独立把 ".." 这类越界 id 拒掉（删档测试专门绕开闸 1
// 直接喂 helper 验证过这一点）。
//
// history.jsonl 不动：它是全家庭的审计线，删档不等于抹掉"谁动过"。
//
// 删不存在的档案，返回的错误要**同时**满足 errors.Is(err, os.ErrNotExist)
// （端点据此分 404）与 errors.Is(err, ErrNotFound)（调用方的存在性判据）：
// 只满足一头，另一头就会假阴性。RemoveAll 对不存在路径按 Go 约定返回 nil，
// 所以锁内先 Stat 一次再分诊。
func (s *Store) DeleteProfile(id string) error {
	dir, err := s.profileDirChecked(id)
	if err != nil {
		return err
	}
	return s.WithProfileLock(id, func() error {
		if _, err := os.Stat(dir); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// 档案不存在：保住两个哨兵，并像包内其它错误一样点名完整路径。
				return fmt.Errorf("删 %s: %w", dir, errors.Join(os.ErrNotExist, ErrNotFound))
			}
			// 其它 I/O 错按"删不下去"原样上报（仍点名路径），不粉饰成 nil。
			return fmt.Errorf("stat %s: %w", dir, err)
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("删 %s: %w", dir, err)
		}
		// 坏档登记要跟着清，否则 /status 会 union 出一条指向不存在档案的幽灵记录。
		s.clearBroken(id, brokenProfile)
		s.clearBroken(id, brokenBookmarks)
		return nil
	})
}
