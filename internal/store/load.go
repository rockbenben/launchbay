package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
)

// Snapshot 是渲染所需的最小信息。Modified 进缓存键，这样"外部手改了
// 基线文件但 rev 没变"也会失效（spec §7.1 的一致性要求）。
type Snapshot struct {
	B        *model.Bookmarks
	Rev      int64
	Modified time.Time
}

func (s *Store) ListProfiles() ([]*model.Profile, error) {
	ents, err := os.ReadDir(s.profilesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("列档案失败: %w", err)
	}
	out := []*model.Profile{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		p, err := s.LoadProfile(e.Name())
		if errors.Is(err, ErrNotFound) {
			// 有目录但没 profile.json：这是"半件事"，不是坏数据。
			// 跳过但不计入 Broken()——否则一个手滑 mkdir 出来的空目录
			// 会在家用页面上挂出一条"某某档案读不了"的红字。
			continue
		}
		if err != nil {
			s.markBroken(e.Name(), brokenProfile, err.Error())
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// LoadProfile 读并校验一个档案的 profile.json。
func (s *Store) LoadProfile(id string) (*model.Profile, error) {
	p, _, err := s.loadProfile(id)
	return p, err
}

// LoadProfileWithFingerprint 一次读盘同时给出解码结果与那份字节的指纹。
//
// 为什么必须一次读：渲染缓存的 profile 那一维如果来自**第二次**读，两次读之间
// 落进一次写入，就会把按旧字节渲染出的 body 存到新指纹的键下——"按了主题按钮
// 没反应"这个病换个地方复发（P1-④ 评审 I-2）。同一次读拿到的两份不可能各说各话，
// 于是这个形状从构造上消失。这里不留证人：它在单线程测试里观察不到，
// 伪造一条恒绿断言比不测更糟，担保是"只有一个调用点"。
func (s *Store) LoadProfileWithFingerprint(id string) (*model.Profile, string, error) {
	p, raw, err := s.loadProfile(id)
	if err != nil {
		return nil, "", err
	}
	// 指纹是**内容**而不是 mtime：Linux 的 tmpfs/overlayfs 在同一时钟刻内的两次
	// 原子替换会返回纳秒级完全相同的 mtime，CIFS/9p 挂载常只到秒。书签那一维留着
	// mtime 是为了"外部程序直接改盘"也能失效（spec §12），而 profile.json 没有 rev
	// 可看——它只剩这一格信号，粒度粗一点就是按了主题按钮没反应。
	sum := sha256.Sum256(raw)
	return p, hex.EncodeToString(sum[:]), nil
}

// loadProfile 是这个包里读 profile.json 的唯一实现：错误分诊、坏档登记、
// id 与目录名一致的校验都只在这里做一次。
func (s *Store) loadProfile(id string) (*model.Profile, []byte, error) {
	path := s.ProfilePath(id)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("%w: profile %q", ErrNotFound, id)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("读 %s: %w", path, err)
	}
	p, err := decodeProfile(path, data)
	if err != nil {
		s.markBroken(id, brokenProfile, err.Error())
		return nil, nil, err
	}
	if p.ID != id {
		// 解释页会原样渲染这句原因（§12），与语法错误同用 filepath.Base：
		// 给家属看文件名和位置，不给磁盘路径。
		err := fmt.Errorf("%w: %s 里 id=%q 与目录名 %q 不一致（复制档案目录时常见）",
			ErrCorrupt, filepath.Base(path), p.ID, id)
		s.markBroken(id, brokenProfile, err.Error())
		return nil, nil, err
	}
	s.clearBroken(id, brokenProfile)
	return p, data, nil
}

func (s *Store) LoadBookmarks(id string) (*model.Bookmarks, error) {
	path := s.BookmarksPath(id)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(s.ProfilePath(id)); err != nil {
			return nil, fmt.Errorf("%w: profile %q", ErrNotFound, id)
		}
		// 新档案：空而不是坏。bookmarks.json 不存在也算"这个来源没坏"，
		// 清掉它可能留下的旧标记（用户删文件重置同样是一种修好）。
		s.clearBroken(id, brokenBookmarks)
		return EmptyBookmarks(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("读 %s: %w", path, err)
	}
	b, err := DecodeBookmarks(path, data)
	if err != nil {
		s.markBroken(id, brokenBookmarks, err.Error())
		return nil, err
	}
	if err := b.Validate(); err != nil {
		// 同时裹住 ErrValidation 与 ErrCorrupt：上层要能分清
		// "形状没问题但内容不合法"与"根本读不了"。原因文本用
		// filepath.Base——这句会原样上家属看的解释页（§12），
		// 与语法错误分支同一口径，不留磁盘路径（评审 Minor）。
		wrapped := fmt.Errorf("%w: %s: %v", ErrCorrupt, filepath.Base(path), err)
		s.markBroken(id, brokenBookmarks, wrapped.Error())
		return nil, errors.Join(ErrCorrupt, err)
	}
	if b.Rev < 0 {
		err := fmt.Errorf("%w: %s: rev 不能为负(%d)", ErrCorrupt, filepath.Base(path), b.Rev)
		s.markBroken(id, brokenBookmarks, err.Error())
		return nil, err
	}
	s.clearBroken(id, brokenBookmarks)
	return b, nil
}

func (s *Store) Current(id string) (Snapshot, error) {
	b, err := s.LoadBookmarks(id)
	if err != nil {
		return Snapshot{}, err
	}
	fi, err := os.Stat(s.BookmarksPath(id))
	if err != nil {
		// 空档案没有 bookmarks.json，用 profile.json 的时间做指纹，
		// 保证缓存键稳定而不是每次不同。
		profilePath := s.ProfilePath(id)
		fi, err = os.Stat(profilePath)
		if err != nil {
			// 与"读 %s"等同仁一致点名完整路径：运维拿到的每一行报错
			// 都要能直接定位文件，而不是只有一个档案 id。
			return Snapshot{}, fmt.Errorf("stat %s: %w", profilePath, err)
		}
	}
	return Snapshot{B: b, Rev: b.Rev, Modified: fi.ModTime()}, nil
}

// CurrentRev 只解 rev 一个字段：坏档的 bookmarks.json 里 rev 往往仍然在
// 第一行，revert 要的就是它。整档读不了时返回 (0, ErrCorrupt)，调用方
// 按"从 rev 0 之后开始"处理——恢复不该被损坏程度挡住。
func (s *Store) CurrentRev(id string) (int64, error) {
	path := s.BookmarksPath(id)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("读 %s: %w", path, err)
	}
	var only struct {
		Rev int64 `json:"rev"`
	}
	if err := json.Unmarshal(raw, &only); err != nil {
		return 0, fmt.Errorf("rev 都读不出来: %w", ErrCorrupt)
	}
	return only.Rev, nil
}

func EmptyBookmarks() *model.Bookmarks {
	// Title 与 Weather.Location 从前在这里被**编**出默认值（"发射台" / "上海市"）：
	// 两处都没有渲染消费者，写进磁盘只是让每一份新档案都带着一个没人读、
	// 又看起来像配置过的值。形状（空数组不是 null）由 Search.Engines 这一行守住。
	return &model.Bookmarks{
		Groups: []model.Group{},
		Search: model.Search{Engines: []model.SearchEngine{}},
	}
}

// normalize 把每一层的 nil 切片换成空切片。§4.3 的写出契约要求空数组
// 写 [] 不写 null，而磁盘上的文件（人或 AI 写的）完全可能出现 "alias": null
// ——解码成 nil 后原样写回就是 null，稳定 diff 的初衷在一次读→写往返后
// 破产。归一化在解码入口与写出入口两侧都做：P1-② 的 add ops 构造的 Item
// 天然不带 alias/tags，写出前再过一遍这道保险，null 一条都漏不出去。
func normalize(b *model.Bookmarks) {
	if b.Groups == nil {
		b.Groups = []model.Group{}
	}
	for gi := range b.Groups {
		g := &b.Groups[gi]
		if g.Items == nil {
			g.Items = []model.Item{}
		}
		for ii := range g.Items {
			it := &g.Items[ii]
			if it.Alias == nil {
				it.Alias = []string{}
			}
			if it.Tags == nil {
				it.Tags = []string{}
			}
		}
	}
	if b.Search.Engines == nil {
		b.Search.Engines = []model.SearchEngine{}
	}
}

// DecodeBookmarks 拒未知键、把语法错误翻成行号、统一归到 ErrCorrupt。
func DecodeBookmarks(path string, data []byte) (*model.Bookmarks, error) {
	var b model.Bookmarks
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return nil, describeJSONErr(path, data, err)
	}
	normalize(&b)
	return &b, nil
}

func decodeProfile(path string, data []byte) (*model.Profile, error) {
	var p model.Profile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, describeJSONErr(path, data, err)
	}
	if err := p.Validate(); err != nil {
		// 与 LoadBookmarks 的校验分支对称：ErrCorrupt 与 model.ErrValidation
		// 都要能被 errors.Is 分出来（%v 会断链，这里用 Join+%w 同时保住两个
		// 哨兵），报错文本仍点名文件——上层分流与坏档页都靠它们。文件名用
		// filepath.Base：这条消息会原样上家属看的解释页（§12）。
		return nil, errors.Join(ErrCorrupt, fmt.Errorf("%s: %w", filepath.Base(path), err))
	}
	return &p, nil
}

// describeJSONErr 报"第 N 行"而不是字节偏移量：要改文件的人是打开文件改的。
func describeJSONErr(path string, data []byte, err error) error {
	base := filepath.Base(path)
	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		return fmt.Errorf("%w: %s 第 %d 行附近: %v", ErrCorrupt, base, lineOf(data, int(syn.Offset)), err)
	}
	// json.Decoder 对截断的 JSON 返回 io.ErrUnexpectedEOF 而不是 *SyntaxError
	// （只有 json.Unmarshal 会把截断报成带 Offset 的 SyntaxError）。这里必须用
	// Decoder，才能拿到 DisallowUnknownFields；所以截断要单独归到行号：断点就
	// 是读到末尾的那一行，按整份数据的行数报，仍满足"报错给行号"的决定。
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%w: %s 第 %d 行附近: JSON 在值结束前就断了", ErrCorrupt, base, lineOf(data, len(data)))
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Errorf("%w: %s 第 %d 行附近: 字段 %q 类型应为 %s", ErrCorrupt,
			base, lineOf(data, int(te.Offset)), te.Field, te.Type)
	}
	if strings.Contains(err.Error(), "unknown field") {
		return fmt.Errorf("%w: %s: %v", ErrCorrupt, base, err)
	}
	return fmt.Errorf("%w: %s: %v", ErrCorrupt, base, err)
}

func lineOf(data []byte, offset int) int {
	if offset > len(data) {
		offset = len(data)
	}
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}
