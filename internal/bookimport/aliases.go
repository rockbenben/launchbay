package bookimport

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/rockbenben/launchbay/internal/icons"
)

// embeddedOverrides 是随二进制走的那份手工覆盖表（2026-10-10）。
//
// 为什么要有它：导入报告里那句"想手动指定：在 internal/bookimport/icons.json 里
// 写…"必须**指着一个真东西**。而网页上的导入（/import）跑在容器里，那条相对路径
// 根本不存在——于是报告教人去改一个它够不着的文件，一句纯装饰的话。
//
// 键是 host（可含端口），值必须是内嵌索引里真实存在的图标名——loadOverrides 逐项
// 对索引校验，不过就中止整个导入。表**不存在或为空都是合法的**：那意味着这次导入
// 全靠猜，猜不到就列进"没猜到图标"的清单。
//
//go:embed icons.json
var embeddedOverrides []byte

// 浏览器导出的书签里**没有图标名**——只有 favicon（HTML 里是 base64 或一个
// 指向浏览器缓存的 URL），而 launchbay 的 icon 字段认的是内嵌索引里的名字。
// 所以导入时的图标只有一条来路：**拿网址去索引里猜**。
//
// 猜的**token 顺序**是 icons.URLTokens（主机段 → 路径段 → 标题词），与 resolve
// 共用同一份；但**不用 icons.Candidates**（见下面那段注释：导入要精确命中，
// 不要模糊候选的前 N 条）——
// 它本来是 resolve 的 iconCandidates，两处各写一份顺序逻辑迟早分家。
//
// 猜不准的时候不硬凑一个形似的图标：置空走首字母色块，并列进"想补图标"的清单。
// 42 条看上去都有图标、直到有人逐条点开才发现全是错的，比一片色块糟得多。

// iconGuesser 是猜图标的那一套。覆盖表是可选的（没配也能跑）。
type iconGuesser struct {
	ix        *icons.Index
	overrides map[string]string // host（可含端口）→ 内嵌索引里的图标名
}

// loadOverrides 读手工覆盖表：键是 host，值是图标名。
//
// **值逐项对索引校验，不过就中止**——覆盖表是人手写的，写错一个字符的概率不低，
// 而写错的 icon 存进 bookmarks.json 不会当场报错：它会安静地走首字母兜底渲染，
// 直到某天有人动 batch 改这一条才被 422 拒，那时没人记得它是哪次导入带进来的。
//
// 表不存在或为空都是合法的：那意味着这次导入全靠猜，猜不到就列清单。
func loadOverrides(path string, ix *icons.Index) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		// 读不到就退到**内嵌的那份**（空 path 是最常见的那种：网页上的导入
		// 不传路径，它要的就是随二进制走的那份）。两者都没有才算空表——
		// 那意味着这次导入全靠猜，猜不到就列进"想补图标"的清单。
		if os.IsNotExist(err) {
			raw = embeddedOverrides
		} else {
			return nil, fmt.Errorf("读图标覆盖表 %s: %w", path, err)
		}
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]string{}, nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("图标覆盖表 %s 不是合法 JSON: %w", path, err)
	}
	norm := make(map[string]string, len(m))
	for k, v := range m {
		host := strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if host == "" || v == "" {
			continue
		}
		if !ix.Has(v) {
			return nil, fmt.Errorf(
				"图标覆盖表 %s：%q → %q，而内嵌索引里没有 %q。\n"+
					"  写进去会变成坏档数据（当场不报错，几个月后动别的条目时才被 422 拒），所以这里中止。\n"+
					"  用 GET /api/v1/icons?q=... 查真实存在的名字，或留空让导入器自己猜。",
				path, host, v, v)
		}
		norm[host] = v
	}
	return norm, nil
}

// guess 猜一条书签的图标名，返回 (图标名, 来源)。
//
// **这里刻意不用 icons.Candidates，尽管它就在隔壁**——那个是给添加浮层用的：
// 它模糊匹配（精确 > 前缀 > 包含）并返回 5 条，交给人挑。而导入是**自动落库，
// 没有人在旁边挑**，于是"取第一条"等于取一个模糊匹配的结果。实测症状很具体：
// `https://zzz-随便.example/` 会猜出 `tabler:zzz`（前缀匹配），
// `chrome://extensions/` 会猜出 `simple-icons:gitextensions`（路径段包含匹配）——
// 两条都是错的，而它们看上去和有图标一模一样，直到有人逐条点开。
//
// 所以导入只认**完整键精确存在**（ix.Has）。两处判据不同不是"分家"：
// 一个是"给人挑几个像的"，一个是"只认准的那个，没有就没有"。
//
// 非 http/https 的地址压根不猜：它在浏览器里点不动（§9 特例一），
// 连 host 都不该拿去搜——`extensions` 这个路径段撞上 gitextensions 就是这么来的。
func (g *iconGuesser) guess(rawURL, title string) (string, string) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", ""
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return "", ""
	}
	if v, ok := g.overrides[host]; ok {
		return v, "覆盖表"
	}
	for _, tok := range icons.URLTokens(rawURL, title) {
		for _, prefix := range []string{"simple-icons", "tabler"} {
			if cand := prefix + ":" + tok; g.ix.Has(cand) {
				return cand, "按网址精确命中"
			}
		}
	}
	return "", ""
}

func hostOf(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

// iconGap 是一条"没猜到图标"的记录：想补的人需要知道**是哪个站**。
type iconGap struct {
	Name, URL, Host string
}

type iconGaps struct {
	m map[string]iconGap // host → 记录（同一站的多条只记一次）
	n int                // 没猜到的总条数
}

func newIconGaps() *iconGaps { return &iconGaps{m: map[string]iconGap{}} }

func (g *iconGaps) add(name, rawURL, host string) {
	g.n++
	if host == "" {
		host = "（解析不出 host）"
	}
	if _, ok := g.m[host]; ok {
		return
	}
	g.m[host] = iconGap{Name: name, URL: rawURL, Host: host}
}

// list 按 host 排序返回；同一份输入永远给出同一份顺序。
func (g *iconGaps) list() []iconGap {
	out := make([]iconGap, 0, len(g.m))
	for _, v := range g.m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}
