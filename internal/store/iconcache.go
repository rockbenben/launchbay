package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// 图标缓存是 `data/icons/` 下唯一可写之物，而它只由 resolve 写（spec §5）。
// 这里不做"顺手给 CLI 开个口子"：那等于给站级缓存开第二条写通路。
const IconCooldown = 24 * time.Hour

// IconEntry 一条 origin 的缓存状态。成功项有 Hash/Ext，失败项只有 FailAt，
// 两者互斥——把"试过且失败"与"有图"混成一个字段，渲染层就得靠猜。
type IconEntry struct {
	Origin string `json:"origin"`
	Hash   string `json:"hash"`
	Ext    string `json:"ext"`
	At     string `json:"at"`
	FailAt string `json:"failAt,omitempty"`
}

type iconIndexFile struct {
	Entries []IconEntry `json:"entries"`
}

// IconCache 是内存镜像 + 落盘索引 + 一个代次计数。
//
// 代次是这里唯一的目的性设计：首屏渲染缓存的键里不含"图标缓存变了"这件事
// （那把键认的是档案、rev、两份文件的变化信号与图标代次，见 render.cacheKey）。没有 gen，resolve 刚抓到的图要等到下次写书签
// 才显示——那是句谎话，spec §5 第 5 条点名的就是它。
type IconCache struct {
	st   *Store
	mu   sync.Mutex
	byID map[string]IconEntry
	gen  uint64
	now  func() time.Time // 测试注入
}

func (s *Store) LoadIconCache() (*IconCache, error) {
	c := &IconCache{st: s, byID: map[string]IconEntry{}, now: time.Now}
	raw, err := os.ReadFile(s.IconIndexPath())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil // 没有缓存就是没有，不是错误
	}
	if err != nil {
		return nil, fmt.Errorf("读图标索引: %w", err)
	}
	var f iconIndexFile
	// 严格解码与 DecodeBookmarks/decodeProfile 同一形状：DisallowUnknownFields
	// 加 describeJSONErr 统一整形（裁定 R1，不另开第二份解码错误类型——
	// §12 的坏档分诊依赖"store 里 JSON 错误只有一种长相"）。
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		// 索引坏了绝不"清空重来"：那会把 24 小时冷却与所有已抓好的图一起
		// 抹掉，而磁盘上的 blob 还躺着——第二份真相就此诞生。让它报错，
		// 装配点决定怎么退出。
		return nil, fmt.Errorf("图标索引 %s 读不了: %w", s.IconIndexPath(),
			describeJSONErr(s.IconIndexPath(), raw, err))
	}
	for _, e := range f.Entries {
		c.byID[e.Origin] = e
	}
	return c, nil
}

func (c *IconCache) Lookup(origin string) (IconEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byID[origin]
	if !ok || e.Hash == "" {
		return IconEntry{}, false
	}
	return e, true
}

func (c *IconCache) InCooldown(origin string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byID[origin]
	if !ok || e.Hash != "" || e.FailAt == "" {
		return false
	}
	at, err := time.Parse(time.RFC3339, e.FailAt)
	if err != nil {
		return false // 时间戳读不懂就允许再试：宁可多抓一次，不要永远锁死
	}
	return c.now().Sub(at) < IconCooldown
}

// Put 存字节并把 origin 指向它。字节已经是内容哈希命名的，重复抓同一张图
// 只会覆盖同一个文件名——去重由命名保证，不靠额外索引。
func (c *IconCache) Put(origin string, data []byte, ext string) (IconEntry, error) {
	if origin == "" || len(data) == 0 || !iconExts[ext] {
		return IconEntry{}, fmt.Errorf("图标缓存写入参数非法（origin 空 / 零字节 / 扩展名 %q）", ext)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])[:16]
	e := IconEntry{Origin: origin, Hash: hash, Ext: ext, At: c.now().UTC().Format(time.RFC3339)}

	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.byID[origin]; ok && old.Hash == hash && old.Ext == ext && old.FailAt == "" {
		return old, nil // 完全没变：端回盘上那个条目（新造 e 的 At 磁盘上根本没有，端出去就是假时间戳），也不动代次
	}
	if err := c.st.WriteFileAtomic(c.st.IconBlobPath(hash, ext), data); err != nil {
		return IconEntry{}, err
	}
	old, hadOld := c.byID[origin]
	c.byID[origin] = e
	if err := c.saveLocked(); err != nil {
		// 索引落不下去就回滚内存镜像：blob 也许已经写了（内容寻址，孤儿 blob
		// 无害），但 byID 绝不能声称一条盘上的 index.json 里不存在的记录——
		// 否则 Lookup 把这个谎端给后面的 resolve，而 resolve 对它的 MarkFail
		// 因 `Hash != ""` 静默 no-op（B6，证人 TestIconCache_saveLocked失败必须回滚内存镜像）。
		if hadOld {
			c.byID[origin] = old
		} else {
			delete(c.byID, origin)
		}
		return IconEntry{}, err
	}
	c.gen++
	return e, nil
}

func (c *IconCache) MarkFail(origin string) error {
	if origin == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.byID[origin]; ok && e.Hash != "" {
		return nil // 已经有图：失败不该把可用的缓存抹掉
	}
	at := c.now().UTC().Format(time.RFC3339) // 一次采样：At 与 FailAt 说的必须是同一时刻
	// **落盘失败也要回滚内存镜像**（2026-10-19 修）：这一段与 Put 是同一条不变式，
	// 而 MarkFail 漏了它——byID 里留着一条 index.json 上不存在的冷却记录，于是
	// 本次进程里那个 origin 被记 24 小时不重抓（InCooldown 为真），重启即失效。
	// 症状是"图标偶尔抓不到，重启一次又好"，而冷却本该跨重启生效，所以它悄悄
	// 成了进程内状态。Put 那边的证人见 TestIconCache_saveLocked失败必须回滚内存镜像，
	// 这边补同样的形状：回滚的是**内存**，不是冷却意图。
	old, hadOld := c.byID[origin]
	c.byID[origin] = IconEntry{Origin: origin, At: at, FailAt: at}
	if err := c.saveLocked(); err != nil {
		if hadOld {
			c.byID[origin] = old
		} else {
			delete(c.byID, origin)
		}
		return err
	}
	c.gen++
	return nil
}

// Generation 是渲染缓存键的一部分（Task 9）。
func (c *IconCache) Generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// saveLocked 按 origin 排序写出。用切片而不是 map：§4.3 那条"绝不靠 map
// 遍历产生输出键序"在这里的体现是"同一个键集两次写出字节一致"。
// （Go 的 encoding/json 对 map 会按字母排，但那是实现细节而不是契约，
// 而且 map 让"entries 是数组"这条对外形状变得不可能。）
func (c *IconCache) saveLocked() error {
	f := iconIndexFile{Entries: make([]IconEntry, 0, len(c.byID))}
	for _, e := range c.byID {
		f.Entries = append(f.Entries, e)
	}
	sort.Slice(f.Entries, func(i, j int) bool { return f.Entries[i].Origin < f.Entries[j].Origin })
	return c.st.WriteFileJSON(c.st.IconIndexPath(), f)
}

// iconExts 与 icons.Fetcher.Image 认的类型一一对应。两处必须同步：
// 一边多一个后缀，另一边就会抓回来一个写不进去的图。
var iconExts = map[string]bool{"png": true, "ico": true, "svg": true, "webp": true, "gif": true}
