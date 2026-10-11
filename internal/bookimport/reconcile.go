package bookimport

import (
	"fmt"
	"sort"

	"github.com/rockbenben/launchbay/internal/model"
)

// srcItem 是 flare 侧的一条，dst 侧的对应物是 model.Item。
// 只带对账要用的字段：flare 还有别的键（颜色、排序权重），那些在映射层就被
// 判成"丢弃并记理由"了，不该在对账这里以"字段缺失"的形式再冒一次。
type srcItem struct {
	Category string
	Name     string
	URL      string
	Desc     string
	Icon     string // flare 的 MDI 名，**未映射**——映射发生在 dst 侧
}

// fieldDiff 是一处对不上的字段。URL 同时带进每条记录里：报告里光说"name 不一样"
// 读的人不知道是哪一条。
type fieldDiff struct {
	URL, Name, Field string
	Want, Got        string
}

// remap 是一处**预期内**的整理动作。§9 第 3 条说整理只能出现在「图标重映射」
// 与「分类调整」两类里，而这两类恰恰是"src 与 dst 本来就该不一样"的地方——
// 把它们当成失败，这一条对账就永远不可能过。
//
// 但它们也不是免检：每一条都进报告，且必须**有据**（旧值 → 新值）。一条说不出
// 改成什么了的整理动作，等于一次没记录的数据改动。
type remap struct {
	URL, Name string
	Kind      string // "icon" | "category"
	From, To  string
	Reason    string
}

// reconcileReport 是三条对账（§9）的全部结论。
type reconcileReport struct {
	CountSrc, CountDst int
	CountOK            bool
	Missing            []string // 有而新库无 = 丢
	Extra              []string // 反之 = 凭空多
	FieldDiffs         []fieldDiff
	Remaps             []remap
}

// OK 是"三条全过"的合取。
func (r reconcileReport) OK() bool {
	return r.CountOK && len(r.Missing) == 0 && len(r.Extra) == 0 && len(r.FieldDiffs) == 0
}

// Failures 把没过的部分逐条列出来，供报告与退出码使用。
//
// **空切片也返回 nil 而不是空数组**：返回值的用途是人读的报告文本，
// 而非空的空切片会在文本里留下一段"没有失败"的空行。
func (r reconcileReport) Failures() []string {
	var out []string
	if !r.CountOK {
		out = append(out, fmt.Sprintf("条目数不等：flare %d 条，新库 %d 条", r.CountSrc, r.CountDst))
	}
	for _, m := range r.Missing {
		out = append(out, "丢了："+m)
	}
	for _, e := range r.Extra {
		out = append(out, "凭空多出："+e)
	}
	for _, d := range r.FieldDiffs {
		out = append(out, fmt.Sprintf("%s 的 %s 不一样：flare 是 %q，新库是 %q",
			d.URL, d.Field, d.Want, d.Got))
	}
	return out
}

// identity 是一条书签在对账里的身份。
//
// 用 (URL, Name) 而不是单用 URL：同一站点挂多个入口是合法的（flare 里就有
// 同域不同页），单用 URL 会让其中一条配不上、报"凭空多出一条"——而那不是错。
// 单用 Name 也不行：不同站点可以有同名条目。
func identity(url, name string) string { return url + "\x00" + name }

// flatten 把 bookmarks.json 的分组压平成一条条，配上它所在的分组 id。
// 对账不看分组结构（分类调整是允许的整理动作），只看条目本身。
func flatten(b *model.Bookmarks) []dstItem {
	var out []dstItem
	for _, g := range b.Groups {
		for _, it := range g.Items {
			out = append(out, dstItem{Item: it, GroupID: g.ID})
		}
	}
	return out
}

type dstItem struct {
	model.Item
	GroupID string
}

// reconcile 跑三条对账。
//
// wantGroup 回答的是"按映射规则，这条**应该**落在哪个分组"，返回空表示这条
// 不参与分类对账。**不能拿 dst 的分组 id 去和 src 的 category 直接比**：
// 那是两个不同命名空间的值（flare 的分类名是中文，launchbay 的是 ASCII slug），
// 拿它们比会把每一次正常导入都记成"分类被调整"——而 §9 允许的整理动作只有两
// 类，判据一旦松到全都在内，第 3 条就再也拦不住任何东西了。
//
// 传 nil 表示不做分类对账（夹具与只想验 URL/name 的场合）。
func reconcile(src []srcItem, dst []dstItem, wantGroup func(srcItem) string) reconcileReport {
	rep := reconcileReport{CountSrc: len(src), CountDst: len(dst)}
	rep.CountOK = len(src) == len(dst)

	// 配对键是 **URL**，这是 §9 第 2 条的原话（"逐 URL 差集"）。
	//
	// 拿 name 一起当键会让"name 被改了"这件事**根本配不上**——于是它报成
	// "丢了 A、凭空多出 A改名了"，把一次改名说成一次丢失 + 一次增生。而那正是
	// 第 3 条要抓的东西：它必须以"name 不一致"的面貌出现，才指得到要改的地方。
	//
	// 同一 URL 有多条时按出现顺序一一配：它可能配错位（谁在前谁在后变了就错），
	// 但错位会以 name 不一致的形式**显形**，不会静悄悄地对上。
	srcByURL := map[string][]srcItem{}
	for _, s := range src {
		srcByURL[s.URL] = append(srcByURL[s.URL], s)
	}
	dstByURL := map[string][]dstItem{}
	for _, d := range dst {
		dstByURL[d.URL] = append(dstByURL[d.URL], d)
	}

	for url, ss := range srcByURL {
		ds := dstByURL[url]
		n := len(ss)
		if len(ds) < n {
			n = len(ds)
		}
		for i := 0; i < n; i++ {
			s, d := ss[i], ds[i]
			// 第 3 条：name 与 desc 逐条比对。desc 用全等而不是"去空格后比"——
			// 导入器做的任何规整（比如去掉 desc 首尾空白）都必须是有意的，
			// 而这里没有第二套判断能证明它有意。
			if s.Name != d.Name {
				rep.FieldDiffs = append(rep.FieldDiffs, fieldDiff{url, s.Name, "name", s.Name, d.Name})
			}
			if s.Desc != d.Desc {
				rep.FieldDiffs = append(rep.FieldDiffs, fieldDiff{url, s.Name, "desc", s.Desc, d.Desc})
			}
			rep.Remaps = append(rep.Remaps, remapsFor(s, d, wantGroup)...)
		}
		for i := n; i < len(ss); i++ {
			rep.Missing = append(rep.Missing, fmt.Sprintf("%s（%s）", url, ss[i].Name))
		}
		for i := n; i < len(ds); i++ {
			rep.Extra = append(rep.Extra, fmt.Sprintf("%s（%s）", url, ds[i].Name))
		}
	}
	for url, ds := range dstByURL {
		if _, ok := srcByURL[url]; ok {
			continue
		}
		for _, d := range ds {
			rep.Extra = append(rep.Extra, fmt.Sprintf("%s（%s）", url, d.Name))
		}
	}

	// 排序只为让同一份输入永远给出同一份报告文本。差集的大小与内容已经由上面
	// 算死了，顺序纯粹是可读性——但"可读性也要确定"，否则两次导入的报告
	// 逐行对不上，人会以为多了一条。
	sort.Strings(rep.Missing)
	sort.Strings(rep.Extra)
	sort.Slice(rep.FieldDiffs, func(i, j int) bool { return diffKey(rep.FieldDiffs[i]) < diffKey(rep.FieldDiffs[j]) })
	sort.Slice(rep.Remaps, func(i, j int) bool { return rep.Remaps[i].URL < rep.Remaps[j].URL })
	return rep
}

func diffKey(d fieldDiff) string { return d.URL + "\x00" + d.Field + "\x00" + d.Want }

// remapsFor 列出这一条上发生的整理动作。
func remapsFor(s srcItem, d dstItem, wantGroup func(srcItem) string) []remap {
	var out []remap
	if d.Icon != s.Icon {
		reason := "MDI 名经别名表映射到内嵌索引里的名字"
		if d.Icon == "" {
			// 空 icon 是"没设"（走首字母兜底），不是"映射失败"，见 §9 的特例二
			reason = "映射不到且别名表未收，置空走首字母兜底"
		}
		out = append(out, remap{s.URL, s.Name, "icon", s.Icon, d.Icon, reason})
	}
	// 只在 wantGroup 说得出"应该去哪组"时才判分类调整：它说得出，才有一处
	// "实际 ≠ 应当"的差异可言。
	if wantGroup != nil {
		if want := wantGroup(s); want != "" && want != d.GroupID {
			out = append(out, remap{s.URL, s.Name, "category", s.Category, d.GroupID,
				"分类被重归：按映射规则应在 " + want + "，实际落在 " + d.GroupID})
		}
	}
	return out
}

// summarizeRemaps 把整理动作按类型汇总，供报告的"整理清单"一节使用。
func summarizeRemaps(rs []remap) map[string]int {
	m := map[string]int{}
	for _, r := range rs {
		m[r.Kind]++
	}
	return m
}
