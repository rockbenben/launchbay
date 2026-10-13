package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"

	"github.com/rockbenben/launchbay/internal/model"
)

// Op 是一条写请求。Value 保持 RawMessage：字段类型要等到落到 name/url/alias
// 哪一个上才知道，提前解码会把"类型错"报成"body 读不了"。
type Op struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
	Index *int            `json:"index,omitempty"`
	To    string          `json:"to,omitempty"`
}

// Error 把失败定位到"第几条 op 的哪个字段"。
//
// Kind 是**机器可读**的那一格（internal/apierr 那一族常量）：Reason 是给人排查的
// 中文原话，而界面上的句子必须按 Kind 选——"那一组不在了"与"这一条已经存过了"
// 都是 422，出路却完全相反。源头没贴分类的落 field-invalid（不是空串：
// 空 kind 会让客户端静默退回状态码那一档，症状是"某几条错的说法永远不对"）。
type Error struct {
	OpsIndex int    `json:"opsIndex"`
	Kind     string `json:"kind"`
	Field    string `json:"field"`
	Reason   string `json:"reason"`
}

// IconSet 是图标名的存在性判据。ops 只认这一个方法，不认索引的实现：
// 索引换成别的产物格式，写校验这边一个字都不用改（裁定一）。真实实现是
// *icons.Index，但 ops 不 import internal/icons——写校验与索引解耦。
type IconSet interface {
	Has(icon string) bool
}

// checkIconName 是唯一的图标名闸门调用点。"" 与 "auto" 在这里判掉：
// 它们是有语义的取值（"没设"/"抓 favicon"），不是索引成员——
// 把这两个特例放进 icons.Index.Has 里，索引就得替写校验想事（裁定二）。
// icons == nil 直接放过：装配点必须传真索引，nil 只允许出现在"这条测试
// 不关心图标"的场合（Task 5 之前 batch.go 暂时传 nil）。
func checkIconName(icons IconSet, id, icon string) error {
	if icons == nil || icon == "" || icon == "auto" {
		return nil
	}
	if !icons.Has(icon) {
		// 包成 fieldErr{"icon", ...}：字段归因由它决定，否则 fieldOf 会
		// 按路径推断（add 整组时路径根本没有 icon 段，会掉回 path）。
		return fieldErr{"icon", kerr(apierr.IconUnknown, fmt.Errorf(
			"条目 %s 的图标名 %q 不在服务端索引里；先查 GET /api/v1/icons?q=<关键词>，"+
				"图标名不许靠猜（spec §5）", id, icon))}
	}
	return nil
}

// Run 把 ops 顺序作用在 b 的副本上。icons 是图标名白名单：
// **只有写路径认它**（spec §6.2），读路径（model.Validate / LoadBookmarks）
// 与渲染层都宽容——把索引绑进读路径等于让"图标快照与磁盘不同步"把档案打成
// 坏档，§12 的坏档隔离是给数据文件真损坏用的。装配点必须传真索引；传 nil
// 等于关掉这道闸，只允许测试这么用。
//
// 返回的 errs 非空时，**结果必须整个丢掉**：spec §4.4 要"全成或全败"。
// 但错误仍然逐条收集而不是首条即返——第 3 条拼错组名不该连带让第 4~30 条
// 的错误看不见，AI 一轮只能修一条的话要跑三十轮。跳过失败的那条继续跑
// 不会掩盖级联错误：后续 op 若依赖失败 op 的产物，它自己也会报错。
//
// 告警不是错误：值没变、缺 icon、非 http 协议都照常写，只是要告诉调用方。
func Run(b *model.Bookmarks, in []Op, icons IconSet) (*model.Bookmarks, []Error, []string) {
	out := clone(b)
	var errs []Error
	var warns []string
	last := -1 // 最后一条真正改成了东西的 op，收尾兜底要挂在它身上
	for i, op := range in {
		w, err := applyOne(out, op, icons)
		if err != nil {
			errs = append(errs, Error{OpsIndex: i, Kind: kindOf(err), Field: fieldOf(op, err), Reason: err.Error()})
			continue
		}
		last = i
		warns = append(warns, prefix(i, w)...)
	}
	if len(in) == 0 {
		return out, nil, nil
	}
	if verr := out.Validate(); verr != nil && last >= 0 {
		// 收尾兜底抓的是"op 层没拦住但整档不合法"。挂在**最后一条真正
		// 改成了东西的 op** 上：`len(in)-1` 会指到一条本来就失败、或根本没
		// 跑过的 op，那是把错误报给无辜的人（评审 Important #1）。
		// field 用 bookmarks 而不是伪造某个字段名——它确实没有唯一责任人。
		errs = append(errs, Error{OpsIndex: last, Kind: kindOf(verr), Field: "bookmarks", Reason: verr.Error()})
	}
	return out, errs, warns
}

func (e Error) String() string {
	return fmt.Sprintf("第 %d 条 op 的 %s：%s", e.OpsIndex, e.Field, e.Reason)
}

// fieldErr 把出错字段钉在错误上。fieldOf 只看 op 形状有时判不准真正的
// 责任字段——带 index 的 add 也可能栽在 id 重复上，那时报 "index" 是骗人。
// 只有"检查现场明确知道是哪个字段"的错误（index 越界）才钉上来。
type fieldErr struct {
	field string
	err   error
}

func (e fieldErr) Error() string { return e.err.Error() }
func (e fieldErr) Unwrap() error { return e.err }

// kerr 给一条 op 错误贴上机器可读的分类。只在**人真需要分辨**的那几类上用
// （id 重复 / 目标不在 / 图标名查不到 / 地址不完整）；剩下的形状错误一律落
// field-invalid，那已经足够让界面说一句对的出路话。
func kerr(kind string, err error) error { return apierr.With(err, kind) }

// kindOf 是唯一的取值口：源头没贴就落 field-invalid。
func kindOf(err error) string {
	if k := apierr.Kind(err); k != "" {
		return k
	}
	return apierr.FieldInvalid
}

func fieldOf(op Op, err error) string {
	var fe fieldErr
	if errors.As(err, &fe) {
		return fe.field
	}
	if op.Path == "" {
		return "path"
	}
	t, perr := ParsePath(op.Path)
	if perr != nil || t.field == "" {
		return "path"
	}
	return t.field
}

func prefix(i int, ws []string) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, fmt.Sprintf("op %d: %s", i, w))
	}
	return out
}

// clone 用 JSON 往返做深拷贝。手写逐层 copy 会漏字段（Item 加一个字段
// 就得记得在这里也加），往返一次让 model 自己保证。
func clone(b *model.Bookmarks) *model.Bookmarks {
	raw, err := json.Marshal(b)
	if err != nil {
		// model.Bookmarks 全是可序列化字段，走到这里说明代码写错了，
		// 不是数据问题——没有可恢复的分支，panic 比返回半份数据诚实。
		panic(fmt.Sprintf("克隆书签失败: %v", err))
	}
	var out model.Bookmarks
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(fmt.Sprintf("反克隆书签失败: %v", err))
	}
	return &out
}

func applyOne(b *model.Bookmarks, op Op, icons IconSet) ([]string, error) {
	switch op.Op {
	case "add":
		return add(b, op, icons)
	case "set":
		return set(b, op, icons)
	case "remove":
		return remove(b, op)
	case "move":
		return move(b, op)
	}
	return nil, fmt.Errorf("op %q 不存在，只有 add/remove/set/move", op.Op)
}

func findGroup(b *model.Bookmarks, id string) (*model.Group, error) {
	for i := range b.Groups {
		if b.Groups[i].ID == id {
			return &b.Groups[i], nil
		}
	}
	return nil, kerr(apierr.MissingTarget, fmt.Errorf("没有分组 %q", id))
}

func findItem(g *model.Group, id string) (*model.Item, error) {
	for i := range g.Items {
		if g.Items[i].ID == id {
			return &g.Items[i], nil
		}
	}
	return nil, kerr(apierr.MissingTarget, fmt.Errorf("分组 %q 里没有条目 %q", g.ID, id))
}

func add(b *model.Bookmarks, op Op, icons IconSet) ([]string, error) {
	t, err := ParsePath(op.Path)
	if err != nil {
		return nil, err
	}
	switch t.kind {
	case kindGroups:
		var g model.Group
		if err := decode(op.Value, &g); err != nil {
			return nil, err
		}
		if !model.ValidGroupLayout(g.Layout) {
			return nil, fmt.Errorf("新分组的 layout 只能是 tiles/list，得到 %q", g.Layout)
		}
		for _, ex := range b.Groups {
			if ex.ID == g.ID {
				return nil, kerr(apierr.DuplicateID, fmt.Errorf("分组 %q 已存在", g.ID))
			}
		}
		// 整组一次带进来的每条图标都要过闸：陌生名字埋在十几条里时，理由必须
		// 点名是哪一条（it.ID），否则调用方只能整组重猜。
		for _, it := range g.Items {
			if err := checkIconName(icons, it.ID, it.Icon); err != nil {
				return nil, err
			}
		}
		b.Groups = append(b.Groups, g)
		return warnsForItems(g.Items), nil
	case kindItems:
		g, err := findGroup(b, t.groupID)
		if err != nil {
			return nil, err
		}
		var it model.Item
		if err := decode(op.Value, &it); err != nil {
			return nil, err
		}
		if it.ID == "" {
			return nil, fmt.Errorf("新条目必须带 id")
		}
		if dupItem(b, it.ID) {
			return nil, kerr(apierr.DuplicateID, fmt.Errorf("条目 id %q 在本档案里已用过（跨分组也不行）", it.ID))
		}
		if err := checkIconName(icons, it.ID, it.Icon); err != nil {
			return nil, err
		}
		at := len(g.Items)
		if op.Index != nil {
			if *op.Index < 0 || *op.Index > len(g.Items) {
				return nil, fieldErr{"index", fmt.Errorf("index %d 越界（0..%d）", *op.Index, len(g.Items))}
			}
			at = *op.Index
		}
		g.Items = append(g.Items, model.Item{})
		copy(g.Items[at+1:], g.Items[at:])
		g.Items[at] = it
		return warnsForItem(it), nil
	}
	return nil, fmt.Errorf("add 只能指向 groups 或 groups/<gid>/items")
}

// decode 用严格模式解 op 的 value：未知键直接拒。AI 把 `aiNote` 写成
// `note` 时报"未知字段 note"比悄悄存一条没备注的书签有用。
func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		// 文案不点名 add：decode 由 add/set 共用，set 缺值时报"add 必须带
		// value"会把人支去改错的那条 op（Task 1 遗留 Minor，最终整枝收掉）。
		return fmt.Errorf("这条 op 必须带 value")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("value 读不了: %w", err)
	}
	return nil
}

func dupItem(b *model.Bookmarks, id string) bool {
	for _, g := range b.Groups {
		for _, it := range g.Items {
			if it.ID == id {
				return true
			}
		}
	}
	return false
}

func set(b *model.Bookmarks, op Op, icons IconSet) ([]string, error) {
	t, err := ParsePath(op.Path)
	if err != nil {
		return nil, err
	}
	switch t.kind {
	case kindGroupField:
		g, err := findGroup(b, t.groupID)
		if err != nil {
			return nil, err
		}
		var s string
		if err := decode(op.Value, &s); err != nil {
			return nil, err
		}
		var warn []string
		switch t.field {
		case "title":
			warn = changed(g.Title == s)
			g.Title = s
		case "layout":
			if !model.ValidGroupLayout(s) {
				return nil, fmt.Errorf("layout 只能是 tiles/list，得到 %q", s)
			}
			warn = changed(g.Layout == s)
			g.Layout = s
		case "accent":
			warn = changed(g.Accent == s)
			g.Accent = s
		}
		return warn, nil
	case kindItemField:
		g, err := findGroup(b, t.groupID)
		if err != nil {
			return nil, err
		}
		it, err := findItem(g, t.itemID)
		if err != nil {
			return nil, err
		}
		return setItemField(it, t.field, op.Value, icons)
	}
	return nil, fmt.Errorf("set 只能指向 groups/<gid>/<field> 或 groups/<gid>/items/<iid>/<field>")
}

func setItemField(it *model.Item, field string, raw json.RawMessage, icons IconSet) ([]string, error) {
	switch field {
	case "name", "url", "icon", "desc", "aiNote":
		var s string
		if err := decode(raw, &s); err != nil {
			return nil, err
		}
		// 不预填 warn（旧写法 `warn := changed(false)` 是死初始化：
		// 下面每个 case 都会整个覆写它）。
		var warn []string
		switch field {
		case "name":
			warn = changed(it.Name == s)
			it.Name = s
		case "url":
			warn = changed(it.URL == s)
			it.URL = s
		case "icon":
			if err := checkIconName(icons, it.ID, s); err != nil {
				return nil, err
			}
			warn = changed(it.Icon == s)
			it.Icon = s
		case "desc":
			warn = changed(it.Desc == s)
			it.Desc = s
		case "aiNote":
			warn = changed(it.AINote == s)
			it.AINote = s
		}
		return warn, nil
	case "alias", "tags":
		var ss []string
		if err := decode(raw, &ss); err != nil {
			return nil, err
		}
		if ss == nil {
			// 显式 null 与 [] 是两件事：写 null 进磁盘就违反了 §4.3。
			ss = []string{}
		}
		var warn []string
		if field == "alias" {
			warn = changed(sameStrings(it.Alias, ss))
			it.Alias = ss
		} else {
			warn = changed(sameStrings(it.Tags, ss))
			it.Tags = ss
		}
		return warn, nil
	}
	return nil, fmt.Errorf("条目没有可 set 的字段 %q", field)
}

func remove(b *model.Bookmarks, op Op) ([]string, error) {
	t, err := ParsePath(op.Path)
	if err != nil {
		return nil, err
	}
	switch t.kind {
	case kindGroup:
		for i, g := range b.Groups {
			if g.ID == t.groupID {
				b.Groups = append(b.Groups[:i], b.Groups[i+1:]...)
				return nil, nil
			}
		}
		return nil, kerr(apierr.MissingTarget, fmt.Errorf("没有分组 %q", t.groupID))
	case kindItem:
		g, err := findGroup(b, t.groupID)
		if err != nil {
			return nil, err
		}
		for i, it := range g.Items {
			if it.ID == t.itemID {
				g.Items = append(g.Items[:i], g.Items[i+1:]...)
				return nil, nil
			}
		}
		return nil, kerr(apierr.MissingTarget, fmt.Errorf("分组 %q 里没有条目 %q", g.ID, t.itemID))
	}
	return nil, fmt.Errorf("remove 只能指向 groups/<gid> 或 groups/<gid>/items/<iid>")
}

// move 把条目挪到另一个分组（同组挪位也走它：to 填本组 id）。
// 位置只由 to + index 决定，绝不用"下标寻址"——那是 §4.4 立意的反面。
//
// move 不重查图标名（裁定三）：它搬的是**已经过了闸的**条目，陌生名字是历史
// 数据——不该因为把一个老书签挪一次组，就因为它不在当前索引快照里而整条写不
// 进去。存在性只在真正改图标值时（add/set 的 icon）卡，不在搬运时卡。
func move(b *model.Bookmarks, op Op) ([]string, error) {
	t, err := ParsePath(op.Path)
	if err != nil {
		return nil, err
	}
	if t.kind != kindItem {
		return nil, fmt.Errorf("move 只能指向 groups/<gid>/items/<iid>")
	}
	if op.To == "" {
		return nil, fmt.Errorf("move 必须带 to（目标分组 id）")
	}
	src, err := findGroup(b, t.groupID)
	if err != nil {
		return nil, err
	}
	dst, err := findGroup(b, op.To)
	if err != nil {
		return nil, err
	}
	at := -1
	for i, it := range src.Items {
		if it.ID == t.itemID {
			at = i
			break
		}
	}
	if at < 0 {
		return nil, kerr(apierr.MissingTarget, fmt.Errorf("分组 %q 里没有条目 %q", src.ID, t.itemID))
	}
	// 先校验再动手：Run 的"跳过失败的那条继续跑"前提要求失败的 op 不许
	// 留下半改状态。摘除之后再验 index，越界那条报错时条目已经从源组消失
	// 了，后续 op 便在残档上接着跑（评审裁决）。
	// 同组时源与目标是同一张活切片，摘除会让它短一格，界限要按摘除后算。
	bound := len(dst.Items)
	if src == dst {
		bound--
	}
	pos := bound
	if op.Index != nil {
		if *op.Index < 0 || *op.Index > bound {
			return nil, fieldErr{"index", fmt.Errorf("index %d 越界（0..%d）", *op.Index, bound)}
		}
		pos = *op.Index
	}
	item := src.Items[at]
	src.Items = append(src.Items[:at], src.Items[at+1:]...)
	dst.Items = append(dst.Items, model.Item{})
	copy(dst.Items[pos+1:], dst.Items[pos:])
	dst.Items[pos] = item
	return nil, nil
}

func warnsForItem(it model.Item) []string {
	var w []string
	if it.Icon == "" {
		w = append(w, fmt.Sprintf("条目 %s 没有 icon，首屏走首字母兜底", it.ID))
	}
	if model.ExternalScheme(it.URL) {
		w = append(w, fmt.Sprintf("条目 %s 的协议浏览器不会从页面跳过去，界面标「需手动打开」并给一枚复制键", it.ID))
	}
	return w
}

func warnsForItems(items []model.Item) []string {
	var w []string
	for _, it := range items {
		w = append(w, warnsForItem(it)...)
	}
	return w
}

func changed(noOp bool) []string {
	if noOp {
		return []string{"设的值与当前值相同，什么都没改"}
	}
	return nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Brief 是 op 的可公开形状：只有动作与路径。历史的白名单由类型系统保证——
// 这里根本没有 value 可以漏出去。
type Brief struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

func Describe(in []Op) []Brief {
	out := make([]Brief, 0, len(in))
	for _, o := range in {
		out = append(out, Brief{Op: o.Op, Path: o.Path})
	}
	return out
}
