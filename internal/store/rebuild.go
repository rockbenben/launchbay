package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
)

// RebuildInput 是"只重写 profile.json"所需的最小信息。刻意没有 Theme/Layout：
// 那两个读不出来就按默认重建——不假装能从半截 JSON 里抢救出来（设计 §8）。
type RebuildInput struct {
	ID      string
	Title   string
	PinHash string
	Admin   bool
}

// RebuildProfile 修一份读不动的 profile.json：书签、快照、历史已有内容一个字不碰。
//
// 三条判据都在这一次按 id 的临界区里，调用方一律不许自己先看一眼再决定：
//   - "读得动吗" = 这一次 loadProfile 的结果。读得动返回 ErrReadable——这条通路不是
//     改档用的，只忘了 PIN 该走 profiles/pin。（与 CreateFirstProfile 把"零档案"判据
//     搬进锁内是同一条纪律，权威源 §4.5：判据与写入必须同属一次串行化，否则两次读
//     之间档案被人修好，这里就会盖掉一份读得动的文件。）
//   - "有没有这份档案" = `idTaken` 的结论（引导通路那一份实现，验的是"是个目录"）。
//     不在就 ErrNotFound，**不建目录**：这条通路只写 profile.json，建不出可用档案，
//     所以真没有这份档案时它必须闭嘴——建档仍然只有 CreateProfile/CreateFirstProfile。
//   - 只有"内容错"(ErrCorrupt) 与"文件不在"(ErrNotFound 且目录在) 允许覆盖。其它
//     读失败（权限、盘坏、profile.json 是个目录）原样返回：那时读不动不等于内容错，
//     覆盖它就是二次事故。
//
// 与 CreateProfile 最关键的区别在失败处理：那里的回滚会把整个档案目录 Remove 掉
// （正常建档时目录里只有它自己刚建的东西），而这里目录里可能装着真书签——所以
// **什么都不回滚**，写失败就报错，坏档登记留着。
//
// 写完在同一把锁内回读一次，返回的就是那一份从盘上读回来的档案（不是内存里递出去
// 的那个 p）。回读仍读不动就返回错误，让调用方答 500——不许给一个"成功但进门页那张
// 卡没回来"的 200。清坏档登记没有第二条路：它只在 loadProfile 读成功时发生
// （load.go 的 clearBroken），这里回读走的正是那一条，不另开判据。
//
// 【重入陷阱】本函数自己拿按 id 的锁：调用方**不许**再包一层
// `WithProfileLock(id, …)`——sync.Mutex 不可重入，那样是当场死锁（与 AddUsage
// 同一句话）。handler 只做外壳与分诊。
func (s *Store) RebuildProfile(in RebuildInput) (*model.Profile, error) {
	// 两道闸（ASCII slug + 不越出 profiles/）走既有实现，与 DeleteProfile、usage
	// 原语同一份——这里的 ErrInvalid 也顺便挡掉 id 里带 "/" 或 ".." 的形态。
	if _, err := s.profileDirChecked(in.ID); err != nil {
		return nil, err
	}
	p := &model.Profile{
		ID: in.ID, Title: in.Title, Admin: in.Admin, PIN: in.PinHash,
		CreatedAt: time.Now().Format(time.RFC3339), Theme: "auto", Layout: "sidebar",
	}
	var got *model.Profile
	err := s.WithProfileLock(in.ID, func() error {
		if !s.idTaken(in.ID) {
			return fmt.Errorf("%w: 档案 %q", ErrNotFound, in.ID)
		}
		// 判据排在一切之前（设计 §2.1 的第 1 步）：读得动就是 409 的事，哪怕递进来的
		// 字段也不合法——"这份档案不需要重建"比"你的字段有问题"更接近调用方的错处，
		// 而且这条通路一旦允许覆盖就没有第二次机会去猜。
		if _, _, err := s.loadProfile(in.ID); err == nil {
			return ErrReadable
		} else if !errors.Is(err, ErrCorrupt) && !errors.Is(err, ErrNotFound) {
			return err
		}
		// 校验排在任何落盘之前（与 CreateProfile 同口径；SaveProfile 里还会再验一次，
		// 这一道是为了"没过校验就一个字节都不写"）。
		if err := p.Validate(); err != nil {
			return errors.Join(ErrInvalid, err)
		}
		// 目录在上面那一步已经确认存在，所以这里不需要 CreateProfile 那样的 MkdirAll。
		if err := s.SaveProfile(p); err != nil {
			return err
		}
		// 回读走导出的 LoadProfile：与 handler、进门页读的是同一个入口（它转发的就是
		// 上面那个 loadProfile，没有第二条路径），返回给调用方的因此是盘上那份。
		profile, readErr := s.LoadProfile(in.ID)
		if readErr != nil {
			return fmt.Errorf("重建后仍读不动 %q: %w", in.ID, readErr)
		}
		got = profile
		return nil
	})
	if err != nil {
		return nil, err
	}
	return got, nil
}
