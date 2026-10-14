package render

import (
	"hash/fnv"
	"html"
	"html/template"
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

// accent 的判据**只有一份**，在 model.ValidAccent：写侧（表单端点）用它拒掉
// 什么都不是的值，读侧（下面 validatedAccent）用它把盘上写坏的值丢掉。
// 它不是装饰：accent 会进 style 属性，而一份正则换来"模板不需要 template.HTML、
// 也不需要'这条是不是颜色'的第二套逻辑"。

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

// faceOf 给**一份档案**挑一张脸（选择页那张卡，2026-10-12）。
//
// 想法：一张卡上印着别人的名字不好看，而"这是谁的那一份"其实有个更好认的
// 答案——**他自己常用的那个站点的图标**。人认图标比认字快，而共用一台电脑的
// 一家人各自点自己那张，靠的正是"这张是那个图标"。
//
// 挑法是**两趟**，第一趟赢：
//  1. 缓存里真的抓过的 favicon（<img>）——那是那个站自己的图，最像"服务图标"；
//  2. 内嵌索引里点名了的图标（内联 SVG）；
//  3. 都没有 → 首字母色块（拿档案名算，与这一份什么图标都没有时同一条退路）。
//
// 第一趟只认 favicon 而不是"随便第一条有图标的"：一份刚导进来、图标还没被抓过
// 的档案会整份落空，于是卡片上出现一个跟人无关的名字首字母——那和改之前一样。
//
// **判据仍然只有 pieceFor 一份**：三取一（SVG > Img > Letter）在它里面，
// 这里只决定"拿哪一条书名的脸去问它"。另写一套挑图规则就会与档案页那张脸分家，
// 症状是"同一个站两个地方长着两张脸"。
//
// 分组顺序：tiles 的先挑（浏览器导出的书签栏是 tiles，见 convert.go 的
// PERSONAL_TOOLBAR_FOLDER），那是用户摆在前面的那些；再按原顺序扫 list 的。
func faceOf(p *model.Profile, b *model.Bookmarks, ix *icons.Index, fc *store.IconCache) iconPiece {
	// 读不出来（坏档、空档）与"没有图标"是同一件事：都给首字母色块。那句解释
	// 在点进去之后那一页，不在这一页重复。
	items := orderedItems(b)
	for _, it := range items {
		if it.Icon == "auto" {
			if p := pieceFor(it, ix, fc); p.Img != "" {
				return p
			}
		}
	}
	for _, it := range items {
		if it.Icon != "" && it.Icon != "auto" {
			if p := pieceFor(it, ix, fc); p.SVG != "" {
				return p
			}
		}
	}
	// 退化：用档案名去问 pieceFor，于是色相与首字母都跟档案页里"没有图标的那条"
	// 是同一套算法（chipHue 对同一个名字永远给同一个色相）。
	return pieceFor(model.Item{Name: p.Title}, ix, fc)
}

// orderedItems 把一份书签按"tiles 的先、其余按原顺序"摊平。
func orderedItems(b *model.Bookmarks) []model.Item {
	if b == nil {
		return nil
	}
	var tiles, rest []model.Item
	for _, g := range b.Groups {
		if g.Layout == model.LayoutTiles {
			tiles = append(tiles, g.Items...)
			continue
		}
		rest = append(rest, g.Items...)
	}
	return append(tiles, rest...)
}

func validatedAccent(s string) string {
	if model.ValidAccent(s) {
		return strings.ToLower(s)
	}
	return ""
}

// CSSAccent 把**已经过 validatedAccent** 的 hex 包成 template.CSS，供模板写进
// `style="--sec-accent:{{.}}"`。
//
// **为什么必须包**：html/template 在 `style` 属性位置上有自己的上下文转义，
// 而它对一个看起来像颜色的值仍然判为不可信（实测 `#e8823c` 被替换成
// `ZgotmplZ`，页面上那一格色点就退回 `--accent` 兜底的蓝——症状是"改了颜色、
// 标题不变"，而数据与判据全都对）。仓库里已经栽过一次同族的：图标名走
// `data-icon` 时也变成过 `#ZgotmplZ`。
//
// **为什么包它是安全的**：template.CSS 会**跳过全部转义**，所以安全性完全来自
// 上游那一份判据——`validatedAccent` 只放行 `#rgb` / `#rrggbb`
// （model.ValidAccent 是唯一判据），其余一律返回空串。所以这里**不能**直接
// `template.CSS(原始值)`，必须先过判据。
//
// 空串仍是空串：没设色的分组照旧不发这一格（模板的 `{{with}}` / `{{if}}`）。
func CSSAccent(s string) template.CSS {
	if v := validatedAccent(s); v != "" {
		return template.CSS(v)
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

// IconTiles 把图标库的命中画成「挑图标」那一格的方格。**每格是页面自己的那枚内联
// SVG**——svgMarkup 是本包唯一把图标拼成 markup 的地方，这里不再写第二份，于是
// 挑的时候看见的形状与存下去之后卡片上画出来的那一枚逐像素相同。
//
// 返回 markup 与 **total**（命中的全量，不是画出来的条数）：截断要由界面说出来
// （§4.3 那条"零命中"与"命中很多被截断"必须能分开判），而 total 只有搜索层知道。
// 一次搜索同时给这两样，端点就不必为了拿总数再搜一遍。
//
// 为什么不走 `/icons/{name}` 那个 <img>：它服务的是 resolve 抓来的 favicon（第三方
// 字节，永远不内联），而库里的图标是**描边 + currentColor** 的——进 <img> 之后
// currentColor 拿不到页面的字色，于是暗色页上是深色图标压在深色卡片上。
//
// 为什么不把索引搬进 app.js 让前端画：索引是内嵌产物（六千多个名字），让**只看
// 首屏的每一页**都背一份图库，正是 §7.3 那条操作线要挡的事。按搜索词要一小格，
// 就只要一小格。
//
// 每格是 `<button type=button>`：选择器常常整个建在**表单里**（添加浮层），一个没写
// type 的按钮点下去会提交那个表单。`aria-label` 不能省——里面的 SVG 是 aria-hidden 的，
// 去掉它这一格对读屏就是一个没有名字的按钮。
func IconTiles(ix *icons.Index, q string, limit int) (template.HTML, int) {
	res := ix.Search(q, "", limit)
	var b strings.Builder
	for _, name := range res.Icons {
		ic, ok := ix.Get(name)
		if !ok {
			continue // 索引与搜索结果同一个快照，这一条走不到；不发一个空格子
		}
		esc := html.EscapeString(name)
		b.WriteString(`<button type="button" class="fld icpick-tile" data-icon="`)
		b.WriteString(esc)
		b.WriteString(`" title="`)
		b.WriteString(esc)
		b.WriteString(`" aria-label="`)
		b.WriteString(esc)
		b.WriteString(`">`)
		b.WriteString(string(svgMarkup(ic)))
		b.WriteString(`</button>`)
	}
	return template.HTML(b.String()), res.Total //lint:ignore 名字来自内嵌索引、其余走 svgMarkup 的白名单
}
