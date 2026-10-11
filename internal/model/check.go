package model

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ErrValidation 让调用方能区分"形状合法但内容不合法"与"文件读不了"。
var ErrValidation = errors.New("数据不合法")

var (
	slugRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	iconName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// 运行时只带这两套（spec §5）。MDI 只在导入器里当别名源，不进运行时。
var allowedIconPrefixes = map[string]bool{"tabler": true, "simple-icons": true}

// ThemeValues / LayoutValues 是这两组枚举的**唯一来源**，按界面要呈现的顺序写。
// 判据（下面的 map 与 Valid*）和面向人的那句话（"只能是 auto|dark|light"）都
// 从这两张表生成：加一档只改这里一次，422 的拒绝语与校验文案自己跟着变。
// 端点里硬抄一份列表的话，下一次加档就会让拒绝语报出一句不含新档的话，
// 而判据明明已经认它（P1-④ 评审 I-6）。
var (
	ThemeValues  = []string{"auto", "dark", "light"}
	LayoutValues = []string{"sidebar", "topbar"}
)

func valueSet(vs []string) map[string]bool {
	m := make(map[string]bool, len(vs))
	for _, v := range vs {
		m[v] = true
	}
	return m
}

var allowedTheme = valueSet(ThemeValues)
var allowedLayout = valueSet(LayoutValues)

// ValidTheme / ValidLayout 是这两组枚举的唯一出口：校验走 map，
// 表单端点走这两个谓词——判据一份，两处消费。
func ValidTheme(s string) bool  { return allowedTheme[s] }
func ValidLayout(s string) bool { return allowedLayout[s] }

func ValidSlug(s string) bool { return slugRE.MatchString(s) }

// CheckURL 只验"这串能不能当 URL 用"，不验协议白名单：spec §6.2 明写
// 非 http/https「允许存但标警告」，§9 要求 flare 的 chrome://extensions/
// 与 chrome://apps 原样保留。协议该不该被浏览器拒绝跳转是渲染层的事
// （见 ExternalScheme），在这里拒掉会让那些条目根本进不了存储。
//
// **导出是因为导入这条通路也要用同一份判据**：bookimport 在生成 ops 之前
// 拿它把过不了这一关的条目**逐条挑出去**（而不是让整批被写路径 422 掉）。
// 判据拷一份的话，两边迟早漂——漂的那天表现是"网页导不进去，命令行能导"。
func CheckURL(u string) error {
	if u == "" {
		return errors.New("url 为空")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("url 解析失败 %q: %w", u, err)
	}
	// 没有主机名就打不开：mailto:/tel: 这类无 host 的 scheme、"http://"、
	// "/relative" 都在这条上被拒。这仍是拒绝，不是"取消校验"。
	if parsed.Host == "" {
		return fmt.Errorf("url 缺主机名 %q", u)
	}
	return nil
}

// ExternalScheme 报告这个 URL 的协议是不是非 http/https（url.Parse 已把
// scheme 转小写）。浏览器禁止从网页跳这类地址，渲染层据此标「需手动
// 打开」而不是渲染 href——spec §6.2 的"标警告"、§9 的 chrome:// 特例。
func ExternalScheme(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		// 解析不了的字符串没资格当内部链接；交给渲染层标出来，
		// 校验通路自会拒它。
		return true
	}
	switch parsed.Scheme {
	case "http", "https":
		return false
	default:
		return true
	}
}

// checkIcon 只验形状与前缀。**名字存在性不在这里查**：model.Validate 同时
// 跑在读路径（SaveBookmarks 与 LoadBookmarks/decodeProfile 都调它），把索引
// 绑进来等于让"图标快照与磁盘不同步"把档案打成坏档（spec §6.2）。存在性由
// ops 写路径的 IconSet 闸门负责（internal/ops#checkIconName）。
func checkIcon(icon string) error {
	if icon == "" || icon == "auto" {
		return nil
	}
	prefix, name, ok := strings.Cut(icon, ":")
	if !ok {
		return fmt.Errorf("icon %q 要写成 <prefix>:<name>", icon)
	}
	if !allowedIconPrefixes[prefix] {
		return fmt.Errorf("icon 前缀 %q 不在运行时双层内（tabler / simple-icons）", prefix)
	}
	if !iconName.MatchString(name) {
		return fmt.Errorf("icon 名 %q 形状非法", name)
	}
	return nil
}

func (b *Bookmarks) Validate() error {
	seenGroup := map[string]bool{}
	seenItem := map[string]bool{}
	for gi := range b.Groups {
		g := &b.Groups[gi]
		if !ValidSlug(g.ID) {
			return fmt.Errorf("%w: group %q: id 不是 ASCII slug", ErrValidation, g.ID)
		}
		if seenGroup[g.ID] {
			return fmt.Errorf("%w: group id 重复: %q", ErrValidation, g.ID)
		}
		seenGroup[g.ID] = true
		if strings.TrimSpace(g.Title) == "" {
			return fmt.Errorf("%w: group %q: title 为空", ErrValidation, g.ID)
		}
		if g.Layout != LayoutTiles && g.Layout != LayoutList {
			return fmt.Errorf("%w: group %q: layout %q 只能是 tiles/list", ErrValidation, g.ID, g.Layout)
		}
		for ii := range g.Items {
			it := &g.Items[ii]
			if !ValidSlug(it.ID) {
				return fmt.Errorf("%w: group %q item %d: id %q 不是 ASCII slug", ErrValidation, g.ID, ii, it.ID)
			}
			// item id 在整个档案内唯一，不只组内：move 之后按 id 寻址
			// 才有唯一解（spec §4.4 的寻址前提）。
			if seenItem[it.ID] {
				return fmt.Errorf("%w: item id 在档案内重复: %q", ErrValidation, it.ID)
			}
			seenItem[it.ID] = true
			if strings.TrimSpace(it.Name) == "" {
				return fmt.Errorf("%w: group %q/item %q: name 为空", ErrValidation, g.ID, it.ID)
			}
			if err := CheckURL(it.URL); err != nil {
				return fmt.Errorf("%w: group %q/item %q: %v", ErrValidation, g.ID, it.ID, err)
			}
			if err := checkIcon(it.Icon); err != nil {
				return fmt.Errorf("%w: group %q/item %q: %v", ErrValidation, g.ID, it.ID, err)
			}
		}
	}
	// Search / Weather / Clock / Title 这四个字段**不在这里校验**。
	//
	// 理由不是"它们永远合法"，是**没有任何渲染消费者读它们**（render 侧 grep 0 处），
	// 而校验失败的代价是整份 bookmarks.json 被判坏档——一个人的全部书签打不开。
	// 一条实测过的形状：`search.default="bing"` 而 engines 里只有 `prefix:"g"`，
	// 改前答 503「数据文件读不了」，改后照常读出整份档案。
	// 一个不生效的字段不该有毁掉别人台子的权力；等哪一天 search 真的长出界面，
	// 再把校验连同那个界面一起加回来。
	//
	// 字段本身**不能从 model 里删**：解码开了 DisallowUnknownFields（load.go），
	// 而每个档案磁盘上都有这几个键（EmptyBookmarks 一直在写）——删字段等于
	// 让所有老文件在升级后变成坏档。保留、不校验、不渲染，是这里唯一不伤数据的形状。
	return nil
}

func (p *Profile) Validate() error {
	if !ValidSlug(p.ID) {
		return fmt.Errorf("%w: profile id %q 不是 ASCII slug", ErrValidation, p.ID)
	}
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("%w: profile %q: title 为空", ErrValidation, p.ID)
	}
	if p.PIN == "" {
		return fmt.Errorf("%w: profile %q: pin 哈希为空", ErrValidation, p.ID)
	}
	if !allowedTheme[p.Theme] {
		return fmt.Errorf("%w: profile %q: theme %q 只能是 %s", ErrValidation, p.ID, p.Theme,
			strings.Join(ThemeValues, "/"))
	}
	if !allowedLayout[p.Layout] {
		return fmt.Errorf("%w: profile %q: layout %q 只能是 %s", ErrValidation, p.ID, p.Layout,
			strings.Join(LayoutValues, "/"))
	}
	return nil
}
