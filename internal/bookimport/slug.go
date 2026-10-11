package bookimport

import (
	"fmt"
	"strings"
	"unicode"
)

// slugify 把一个分类标题变成稳定 ASCII slug。
//
// **"稳定"比"好看"重要**：slug 会写进 bookmarks.json 当作 id，而寻址全靠 id
// （spec §4.4，不用数组下标）。也就是说 slug 一旦定下并在人手里传开，改它等于
// 把别人书签里的链接全部指空。所以判据是**同输入永远同输出**，不是"给这个标题
// 起个最好的名字"。
//
// 中文标题必须产出非空 ASCII：flare 的七个分类里中文占多数（§9 的已知特例里
// 「播客」就是一个）。取不到字就退回 `group-N`，而不是让 id 为空——空 id 会
// 让整个分组无法寻址，那是导入直接失败的原因，不是可以"下次再修"的东西。
//
// 已经全是 ASCII 的标题原样小写化，不做别的加工：`NAS` → `nas`、`影音` →
// 走拼音我们没有词库，硬凑只会产出骗人的 id。
func slugify(title string) string {
	var b strings.Builder
	prevDash := true // 行首不算"前面已有分隔符"，避免产出前导 '-'
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r), r == '-', r == '_', r == '.':
			// 分隔符折叠：连续多个只出一个，尾部去掉。
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			// 丢掉标点与全部非 ASCII（中文、emoji、全角符号）。它们不进 id：
			// id 要进 URL 与文件系统路径，中文与符号在这两处都要额外的编码约定。
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "" // 调用方据此退回 group-N，并把它记进报告
	}
	return out
}

// slugMax 是 model.ValidSlug 的上限（`^[a-z0-9][a-z0-9-]{0,31}$` 里的 32）。
//
// 它在这里不是"起名字的美观上限"，是**硬闸**：超长的 id 会被写路径整条拒掉
// （"id 不是 ASCII slug"），而分组 id 还会把每个条目 id 的预算吃掉一截。
// 真实导出的文件夹名动辄三四十个字符（"开发工具与那些很长很长长到需要被截断的
// 文件夹名字"），不按预算截就等于**每一条书签都进不去**。
const slugMax = 32

// clipSlug 把 slug 截到 max 字节并去掉尾部分隔符。
//
// 字节切分是安全的：slugify 只写 a-z0-9 与 '-'，里面不可能有多字节字符。
// 尾部那个 Trim 是必须的——截断点常常正落在一个分隔符上，留着它拼后缀就会
// 变成 "abc--2"，而那不是一个能被 ValidSlug 认出来的形状。
func clipSlug(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) > max {
		s = s[:max]
	}
	return strings.Trim(s, "-")
}

// suffixSlug 给 base 拼上 "-N" 去重后缀，**并保证整体不超过 slugMax**：
// 截的是 base 而不是拼完之后，于是长度上限在去重时也成立。
func suffixSlug(base string, n int) string {
	suffix := fmt.Sprintf("-%d", n)
	room := slugMax - len(suffix)
	if room < 1 {
		room = 1
	}
	if head := clipSlug(base, room); head != "" {
		return head + suffix
	}
	return fmt.Sprintf("item%s", suffix)
}

// slugify 在一批标题上分配唯一 slug。
//
// **冲突必须显式处理，不许悄悄加后缀**：两个不同分类撞成同一个 id 时，
// 后者会被寻址到前者的分组上——那不是"名字不好看"，是**书书签进了错误的组**。
// 而报告里不出现这一条，就没有人会知道发生了什么。所以每次去重都产出一条
// 记录，连同它顶替掉的是哪个分类。
type slugResult struct {
	Slug      string // 空 = 退化成了 group-N
	FellBack  bool   // title 里取不到 ASCII，退化
	Displaced string // 因与谁冲突而被去重，空表示没冲突
}

// slugifyAll 把分类标题批量转成唯一 slug，顺序即输入顺序。
//
// 返回的下标与输入一一对应：调用方拿第 i 个结果去配第 i 个分类，中间任何
// 一次去重都不改变下标关系。**按顺序而不是按标题排序去重**——同一个分类集合
// 在两次导入里必须得到同一套 id，而排序会把"去重时谁留下"这件事绑到标题的
// 字典序上，那是个和真实输入顺序无关、因而更难解释的规则。
func slugifyAll(titles []string) []slugResult {
	used := make(map[string]int, len(titles)) // slug → 首个占用者的下标
	out := make([]slugResult, len(titles))
	for i, t := range titles {
		r := slugResult{Slug: clipSlug(slugify(t), slugMax)}
		if r.Slug == "" {
			r.Slug = fmt.Sprintf("group-%d", i+1)
			r.FellBack = true
		}
		if first, taken := used[r.Slug]; taken {
			// 顶替掉的不是"第 first 个分类"这个名字，而是那次占用的标题——
			// 报告里写标题才读得懂，写下标没人知道那是谁。
			r.Slug = suffixSlug(r.Slug, i+1)
			r.Displaced = titles[first]
		}
		used[r.Slug] = i
		out[i] = r
	}
	return out
}
