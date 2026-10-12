package store

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

// ErrBootstrapClosed 说的是"引导窗口已经关上"。handler 把它映射成不带
// ?err= 的 303——"这台机器已经建过档了"本身就是枚举线索（spec §6.1 的同一纪律）。
var ErrBootstrapClosed = errors.New("引导期已过：磁盘上已有档案")

// CountProfiles 数 profiles/ 下的目录个数。
//
// 不用 ListProfiles()：它读不动某个 profile.json 时会静默跳过（§12 的隔离），
// 于是"把一个档案装坏了"等于把引导窗口重新打开，第二个人可以合法地再建一个
// 档案——而引导窗口一旦重开就再也不会自动关上。引导判据要的是磁盘上的占位事实。
// 目录还不存在 = 一个档案都没有，返回 (0, nil)：全新安装的形状本来就长这样。
func (s *Store) CountProfiles() (int, error) {
	ds, err := os.ReadDir(s.profilesDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("数档案目录 %s: %w", s.profilesDir(), err)
	}
	n := 0
	for _, d := range ds {
		if d.IsDir() {
			n++
		}
	}
	return n, nil
}

// slugProfileID 把显示名折成 ASCII slug：小写、非 [a-z0-9] 的段当一个分隔符、
// 开头不带分隔符、结尾剥干净、截到 32 个字符（ValidSlug 的长度上界）。
//
// 中文名一律 slug 不出东西，返回空串由调用方走基名序列。这不是待修缺陷：
// 表单不问 id，而做拼音转写要么加依赖（违零第三方）要么加一张表（违 §11
// "多出来的件也算失败"）。设计 §11.2 把这条代价写死在明处。
func slugProfileID(title string) string {
	var b strings.Builder
	prevSep := true // 起始当作"刚出过分隔符"，好把开头的 '-' 全吃掉
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSep = false
			continue
		}
		if !prevSep && b.Len() > 0 {
			b.WriteByte('-')
			prevSep = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 32 {
		// 截断后再剥一次：切在 '-' 上会留下一个尾连字符。ValidSlug 其实放过它，
		// 剥掉是为了 id 的样子稳定——它要写进目录名与快照路径，也要给人看。
		out = strings.Trim(out[:32], "-")
	}
	return out
}

// NextProfileID 给一个"还不在盘上"的档案 id（设计 §5）：
// slug 得动且没被占 → 用它；否则基名 home 的递增序列。
// 判据是"这个目录在不在"，不是"现在有几个档案"——删过档的计数会撒谎。
func (s *Store) NextProfileID(title string) string {
	if id := slugProfileID(title); id != "" && !s.idTaken(id) {
		return id
	}
	for n := 1; ; n++ {
		id := "home"
		if n > 1 {
			id = fmt.Sprintf("home-%d", n)
		}
		if !s.idTaken(id) {
			return id
		}
	}
}

func (s *Store) idTaken(id string) bool {
	fi, err := os.Stat(s.ProfileDir(id))
	return err == nil && fi.IsDir()
}

// CreateFirstProfile 是引导建档唯一的落点：判据（磁盘为空）与写入同属
// pmu 的一次临界区。
//
// 为什么必须这样：按 id 的那把锁管不了这件事——两台设备各拿 `alpha` 与
// `intruder` 两个名字，派生出两个互不相撞的 id，各自过各自的 exists 检查，
// 于是安静地建出**两份**档案，而第二个人以为根本没有这一页。"只在 0 档案时开着"
// 这句话要在写入时刻也为真，就只能把判据和写入放进同一次串行化里（设计 §3、§11.1）。
func (s *Store) CreateFirstProfile(in NewProfileInput) (*model.Profile, error) {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	n, err := s.CountProfiles()
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrBootstrapClosed
	}
	return s.CreateProfile(in)
}
