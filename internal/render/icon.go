package render

import (
	"hash/fnv"
	"html"
	"html/template"
	"regexp"
	"strings"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// iconPiece 是一条书签的图标决定结果。**呈现时**三取一：模板按
// SVG > Img > Letter 的优先级选一个来画。Letter/ChipHue 恒填（pieceFor
// 每条路径都填），是为了让退化不需要第二段判断——SVG 为空退首字母色块时，
// 模板不用再回头问"那 Letter 填了没有"。
type iconPiece struct {
	SVG     template.HTML // 非空 = 内联 SVG
	Img     string        // 非空 = <img src>（缓存 favicon）
	Missing bool          // 名查不到 / auto 无缓存：角标"图标未匹配"
	Letter  string        // 前两者都空时的首字母
	ChipHue int           // 首字母色块的色相（0-359）
}

// hexColorRE 是 accent 的唯一判据。它不是装饰：accent 是用户可写的字符串，
// 而它会进 style 属性。只认 #rgb/#rrggbb，其余一律当没写——
// 于是这里既不需要 template.HTML，也不需要"这条是不是颜色"的第二套逻辑。
var hexColorRE = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// pieceFor 决定一个条目怎么长出一张脸。
//
// 分组 accent 不在这里算：它是分组的属性，由 exec.go 用 validatedAccent 算一次
// 放进 groupView。在这里再算一遍就是第二个真相，两处判据迟早分家。
//
// 判据只有两个来源：图标索引（名字在不在）与图标缓存（auto 有没有抓过）。
// 索引里查不到的名字走首字母并标 Missing，**不报错也不清空磁盘上的值**——
// 索引是快照，快照会漂（spec §6.2 因此把存在性只卡在写路径）。
func pieceFor(it model.Item, ix *icons.Index, fc *store.IconCache) iconPiece {
	p := iconPiece{Letter: firstChar(it.Name)}
	p.ChipHue = chipHue(it.Name)

	switch it.Icon {
	case "":
		return p // "没设"不是"缺"
	case "auto":
		if o := icons.Origin(it.URL); o != "" {
			if e, ok := fc.Lookup(o); ok {
				p.Img = "/icons/" + e.Hash + "." + e.Ext
				return p
			}
		}
		p.Missing = true // 没有可抓的协议，或抓过没成：进缺图口径（spec §5）
		return p
	}
	if ic, ok := ix.Get(it.Icon); ok {
		p.SVG = svgMarkup(ic)
		return p
	}
	p.Missing = true
	return p
}

func validatedAccent(s string) string {
	if hexColorRE.MatchString(s) {
		return strings.ToLower(s)
	}
	return ""
}

// chipHue 由名字哈希出一个稳定色相。用 fnv 而不是 maphash：后者每次进程
// 启动随机化种子，色块会在一夜之间换色，而 spec §5 要的是"同一个名字同个颜色"。
func chipHue(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int(h.Sum32() % 360)
}

// svgMarkup 是本案唯一把图标拼成 markup 的地方。
//
// 属性值一律转义；而元素名、属性名、层这三样**不是靠转义兜的**——它们拼在
// 引号外或属性位上，转义救不了逃逸，只能靠白名单谓词把住：
// icons.IsDrawable / icons.IsKeptAttr 与加载侧（internal/icons/whitelist.go）
// 是同一份判据，但两道闸各查各的、互不信任（spec §11 的 XSS 行）。于是有人
// 绕过 Decode 直接构造 Icon 时：层不是 stroke/fill 就整幅拒（返回空 SVG，
// pieceFor 与模板自然退到首字母色块，这里不编默认层）；元素名/属性名不在
// 白名单就少画那一条——口径是"少画一个元素"，不是"整个页面报错"。
// 第三方字节（缓存 favicon）永远只走 <img>，绝不内联——那条决定长在
// pieceFor 的三取一结构里，不在这里。
func svgMarkup(ic icons.Icon) template.HTML {
	if !icons.IsLayer(ic.Layer) {
		return "" // 层逃不出引号：因为它压根不在白名单里（枚举只有一份，见 icons.IsLayer）
	}
	var b strings.Builder
	b.WriteString(`<svg class="ic-svg ic-`)
	b.WriteString(ic.Layer)
	b.WriteString(`" viewBox="`)
	b.WriteString(html.EscapeString(ic.ViewBox))
	b.WriteString(`" aria-hidden="true" focusable="false">`)
	for _, el := range ic.Elements {
		if !icons.IsDrawable(el.Tag) {
			continue // 第二道闸：加载侧查过，这里再各量一次
		}
		b.WriteString("<")
		b.WriteString(el.Tag)
		for _, a := range el.Attrs {
			if !icons.IsKeptAttr(a.Name) {
				continue
			}
			b.WriteString(` `)
			b.WriteString(a.Name)
			b.WriteString(`="`)
			b.WriteString(html.EscapeString(a.Value))
			b.WriteString(`"`)
		}
		b.WriteString(`/>`)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()) //lint:ignore 只由白名单常量与转义值拼成
}
