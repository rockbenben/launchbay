package icons

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// viewBoxRE 钉住本层统一的视口形状。元素与属性白名单四件套
// （drawableTags / keepAttrs / attrValueRE / layerFillDefaults）在 Task 0 的
// internal/icons/whitelist.go 里，本文件只消费——生成器与解码器共用一份判据。
var viewBoxRE = regexp.MustCompile(`^[0-9. ]+$`)

type Attr struct {
	Name  string `json:"-"`
	Value string `json:"-"`
}

type Element struct {
	Tag   string
	Attrs []Attr
}

type Icon struct {
	Prefix   string
	Name     string
	ViewBox  string
	Layer    string
	Elements []Element
}

// Index 是两套合并后的查询面。它一旦装配就是只读的：渲染并发读它，
// 没有任何写路径，所以不带锁。
type Index struct {
	icons    map[string]Icon
	alias    map[string]string
	byPrefix map[string][]string
	counts   map[string]int
}

// doc 的 json tag 与 tools/genicons/build.go 的 productDoc 逐字段对齐，
// Decode 又开了 DisallowUnknownFields：两边一旦漂了，Task 2 写盘前的自校验
// 会当场报错。这是设计好的连锁，不是巧合，别在这儿放宽。
type doc struct {
	Prefix        string                     `json:"prefix"`
	Layer         string                     `json:"layer"`
	ViewBox       string                     `json:"viewBox"`
	Count         int                        `json:"count"`
	UpstreamTotal int                        `json:"upstreamTotal"`
	Upstream      string                     `json:"upstream"`
	GeneratedAt   string                     `json:"generatedAt"`
	License       string                     `json:"license"`
	Icons         map[string]json.RawMessage `json:"icons"`
	Aliases       map[string]string          `json:"aliases"`
}

// Decode 解一套图标并全量校验。校验严格到"宁可整套不要"：一个坏属性值
// 如果被跳过，坏图标就会带着半截数据上首屏，那时没人知道它被切过。
func Decode(raw []byte) (*Index, error) {
	var d doc
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields() // 产物形状漂了要当场炸，不要静默少解一个字段
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("图标产物读不了: %w", err)
	}
	if !IsLayer(d.Layer) {
		return nil, fmt.Errorf("%s 的 layer %q 只能是 stroke/fill", d.Prefix, d.Layer)
	}
	if !viewBoxRE.MatchString(d.ViewBox) {
		return nil, fmt.Errorf("%s 的 viewBox %q 形状非法", d.Prefix, d.ViewBox)
	}
	// 自证：声明条数与实际解出条数必须相等。上游 JSON 的键、解析器、
	// 校验循环任何一处吞了东西，这里就红——而不是首屏少几个图标没人发现。
	if d.Count != len(d.Icons) {
		return nil, fmt.Errorf("%s 的 count=%d 与实际收录 %d 条不符",
			d.Prefix, d.Count, len(d.Icons))
	}
	ix := &Index{
		icons:    make(map[string]Icon, len(d.Icons)),
		alias:    map[string]string{},
		byPrefix: map[string][]string{},
		counts:   map[string]int{},
	}
	for name, rawEl := range d.Icons {
		els, err := decodeElements(d, name, rawEl)
		if err != nil {
			return nil, err
		}
		key := d.Prefix + ":" + name
		ix.icons[key] = Icon{Prefix: d.Prefix, Name: name, ViewBox: d.ViewBox, Layer: d.Layer, Elements: els}
		ix.byPrefix[d.Prefix] = append(ix.byPrefix[d.Prefix], key)
	}
	for alias, parent := range d.Aliases {
		key := d.Prefix + ":" + alias
		target := d.Prefix + ":" + parent
		if _, ok := ix.icons[target]; !ok {
			return nil, fmt.Errorf("%s 的别名 %s 指向不存在的 %s", d.Prefix, alias, parent)
		}
		ix.alias[key] = target
	}
	ix.counts[d.Prefix] = len(d.Icons)
	for _, list := range ix.byPrefix {
		sort.Strings(list)
	}
	return ix, nil
}

// decodeElements 认两种写法：裸 d 字符串，或扁平元素数组。
func decodeElements(d doc, name string, rawEl json.RawMessage) ([]Element, error) {
	trim := strings.TrimSpace(string(rawEl))
	if strings.HasPrefix(trim, `"`) {
		var s string
		if err := json.Unmarshal(rawEl, &s); err != nil {
			return nil, fmt.Errorf("%s:%s 的裸 d 读不了: %w", d.Prefix, name, err)
		}
		return checkElements(d, name, []Element{{Tag: "path", Attrs: []Attr{{Name: "d", Value: s}}}})
	}
	var flat [][]string
	if err := json.Unmarshal(rawEl, &flat); err != nil {
		return nil, fmt.Errorf("%s:%s 既不是裸 d 也不是元素数组: %w", d.Prefix, name, err)
	}
	els := make([]Element, 0, len(flat))
	for _, e := range flat {
		if len(e) < 3 || (len(e)-1)%2 != 0 {
			return nil, fmt.Errorf("%s:%s 的元素要写成 [tag, 名, 值...]，得到 %v", d.Prefix, name, e)
		}
		el := Element{Tag: e[0]}
		for i := 1; i < len(e); i += 2 {
			el.Attrs = append(el.Attrs, Attr{Name: e[i], Value: e[i+1]})
		}
		els = append(els, el)
	}
	return checkElements(d, name, els)
}

// checkElements 是唯一的白名单执行点。属性顺序保留上游顺序：产物是
// 一次性生成的静态文件，稳定顺序由生成器排序保证，这里不重排。
func checkElements(d doc, name string, els []Element) ([]Element, error) {
	if len(els) == 0 {
		return nil, fmt.Errorf("%s:%s 没有任何可绘制元素", d.Prefix, name)
	}
	for _, el := range els {
		if !drawableTags[el.Tag] {
			return nil, fmt.Errorf("%s:%s 有白名单外的元素 %q", d.Prefix, name, el.Tag)
		}
		for _, a := range el.Attrs {
			if !keepAttrs[a.Name] {
				return nil, fmt.Errorf("%s:%s 有白名单外的属性 %q", d.Prefix, name, a.Name)
			}
			if !attrValueRE.MatchString(a.Value) {
				return nil, fmt.Errorf("%s:%s 的 %s 值形状非法", d.Prefix, name, a.Name)
			}
			if a.Name == "fill" {
				if a.Value != "currentColor" && a.Value != "none" {
					return nil, fmt.Errorf("%s:%s 的 fill %q 只认 currentColor/none", d.Prefix, name, a.Value)
				}
				if a.Value == layerFillDefaults[d.Layer] {
					return nil, fmt.Errorf("%s:%s 的 fill %q 与本层默认值相同，产物不该存它",
						d.Prefix, name, a.Value)
				}
				// 前两道闸之后，剩下唯一可能的组合是 fill 层 + none：本层默认是
				// currentColor，偏离它就只有"把整条图标画没了"这一种。生成器按
				// 同一份判据也只会把它留在产物里（实测上游两套都无此形状），
				// 所以这里宁可整套报错，也不静默收下语义已经变掉的图标。
				if d.Layer != "stroke" {
					return nil, fmt.Errorf("%s:%s 的 fill %q 在 fill 层没有合法取值（本层默认 %s）",
						d.Prefix, name, a.Value, layerFillDefaults[d.Layer])
				}
			}
		}
	}
	return els, nil
}

// Merge 把几套并成一个查询面。前缀重叠是编程错误：两套同前缀的索引
// 意味着产物被复制过，静默后者覆盖前者只会让"哪条图标存在"变成猜谜。
func Merge(parts ...*Index) *Index {
	out := &Index{icons: map[string]Icon{}, alias: map[string]string{},
		byPrefix: map[string][]string{}, counts: map[string]int{}}
	for _, p := range parts {
		for k, v := range p.icons {
			if _, dup := out.icons[k]; dup {
				panic(fmt.Sprintf("图标 %s 在两份产物里都出现", k))
			}
			out.icons[k] = v
		}
		for prefix, list := range p.byPrefix {
			out.byPrefix[prefix] = append(out.byPrefix[prefix], list...)
		}
		for k, v := range p.alias {
			out.alias[k] = v
		}
		for prefix, n := range p.counts {
			out.counts[prefix] += n
		}
	}
	for prefix := range out.byPrefix {
		sort.Strings(out.byPrefix[prefix])
	}
	return out
}

// Has 回答"这个名字能不能写进磁盘"。空串与 auto 不在这里判——它们是
// 语义（"没设"/"抓 favicon"），不是索引成员，判在 ops 那一层（Task 4）。
func (x *Index) Has(icon string) bool {
	_, ok := x.Canonical(icon)
	return ok
}

func (x *Index) Canonical(icon string) (string, bool) {
	if icon == "" {
		return "", false
	}
	if _, ok := x.icons[icon]; ok {
		return icon, true
	}
	if target, ok := x.alias[icon]; ok {
		return target, true
	}
	return "", false
}

func (x *Index) Get(icon string) (Icon, bool) {
	key, ok := x.Canonical(icon)
	if !ok {
		return Icon{}, false
	}
	return x.icons[key], true
}

func (x *Index) Count(prefix string) int { return x.counts[prefix] }

func (x *Index) Names(prefix string) []string {
	out := x.byPrefix[prefix]
	cp := make([]string, len(out))
	copy(cp, out)
	return cp
}

// AllNames 按**固定的顺序**给出整个索引的全部名字：品牌层在前，然后通用层，
// 每层内部按名字排序。
//
// **为什么要有它**（2026-10-20）：图标选择器从前只有"搜一个词"与"这一份已经在用的"
// 两批，于是"我想看看库里有什么、随便挑一个"这条路是断的——而库里一万个图标
// 是**内嵌在二进制里**的（spec §5），它不是"别人的东西"，是这台机器自带的东西。
// 浏览这一档要一个**稳定**的全序：翻页的 offset 是位置，顺序变了就等于翻乱了。
//
// 顺序取 loadOrder（品牌层在前）——那是解析顺序，也是"品牌层是本案主力"那件事的
// 同一个判断。每层内部用 byPrefix 里那份排好的（Decode 出来就排过）。
func (x *Index) AllNames() []string {
	out := make([]string, 0, x.total())
	for _, p := range loadOrder {
		out = append(out, x.byPrefix[p]...)
	}
	return out
}

// total 是整个索引的条数。
func (x *Index) total() int {
	n := 0
	for _, c := range x.counts {
		n += c
	}
	return n
}

// ErrNoIndex 让 Load 的失败可分诊：索引读不出来是装配错误，
// 不是运行时数据问题，装配点应当直接退出而不是降级成"没有图标"。
var ErrNoIndex = errors.New("图标索引读不出来")
