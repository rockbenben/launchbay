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

// repoIcon 是那枚**GitHub 猫头**——源码与需求在 GitHub 上那个链接的图形。
//
// **从图标索引取，不另画一个**（2026-10-20）：这个项目里所有图形都出自 icons
// 索引，内联 SVG 的 markup 由 svgMarkup 拼（那是全案唯一拼 markup 的出口），
// 这里手写一个猫头就等于第二处出处。
//
// 名字写死 `simple-icons:github`：simple-icons 那一层是**品牌标志**，内嵌在
// 二进制里（不联网、不外链、离线可用），而这一枚链接的落点正好就是 GitHub——
// 图标与去处是同一个名字，不需要解释。
//
// **查不到就画空**（图标Piece 三个字段全空 → 模板那条 `icon` define 会落到首字母
// 色块，而 Letter 也是空的 → 一个空盒子）。真出现这个形状说明索引里少了那份图标，
// 那时候一个空盒子比一个画错的猫头好。
func repoIcon(ix *icons.Index) iconPiece {
	if ix == nil {
		return iconPiece{}
	}
	if ic, ok := ix.Get("simple-icons:github"); ok {
		return iconPiece{SVG: svgMarkup(ic)}
	}
	return iconPiece{}
}

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

// accent 的判据**以 model.ValidAccent 为准**（客户端另有一份镜像，见 model 那边）：写侧（表单端点）用它拒掉
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

// faceOf 给**一份档案**画一张脸（选择页那张卡）。
//
// **就是档案名的首字母色块，永远**（2026-10-17 用户决定：从"这个人最常用站点的
// 图标"改成文字头像）。
//
// 为什么不再挑服务图标：
//  1. **它会认错人**。一份叫「爸爸」的档案配 YouTube 图标，第一次来的人读成"这是
//     谁的账号照片"是自然反应——而图标在这张卡上从不说明自己是谁。
//  2. **两份档案完全可能常用同一个服务**（都用 GitHub），那时候两张卡的图标一模一样，
//     而名字是唯一的区分依据；2026-10-12 写下"图标不许顶掉名字"时已经承认了这件事，
//     却又留着一个认不出来的脸站在名字旁边。
//  3. **它是当着全家人的面说出收藏内容**。这一页每个人都看得见，而"这份收藏里最常
//     点的是哪个站"是一条本来不必公开的信息——render.go 里"只带一张脸进模板"那段
//     的顾虑，被这张脸自己推翻了。
//  4. **它要读整份 bookmarks.json 才画得出来**（挑最常用那个站），而这一页每来一次
//     就是每份档案读一次；这张脸现在不读任何书签——**坏档、空档、读不出来都一样**。
//
// 色相与首字母仍是同一套算法（chipHue/firstChar），所以同一份档案在选择页与档案页
// 那个没有图标的色块是同一个颜色。
func faceOf(p *model.Profile) iconPiece {
	return iconPiece{Letter: firstChar(p.Title), ChipHue: chipHue(p.Title)}
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
// `style="--sec-accent:{{.AccentCSS}}"`。
//
// **这一包防的是什么**（2026-10-14 修判，原因在同一天被重新量过一遍）：
// 改之前那一行写的是 `style="--sec-accent:{{.}};--d:{{.Depth}}"` —— 而那里
// **在 `{{range .Groups}}` 里面，`{{.}}` 是整个 groupView 结构体**，不是它
// 的 Accent 字段。于是 html/template 把一个结构体塞进 CSS 上下文，判为不可信，
// 整个 style 值变成 `ZgotmplZ`（连字面写的 `--d:0` 一起没了）。症状：改了颜色、
// 标题色点不动，而数据与 validatedAccent 全都对。
//
// **注意别记成"颜色值被判为不可信"**——那不是它。实测（Go 1.x 的
// html/template）：`style="--sec-accent:{{.Accent}}"` 这种**字面部分由模板
// 自己写、插值只占一个值的位置**的形状，一个 `#e8823c` 是过得去的；判为危险的
// 是「插值占据了整个 style 值」。仓库里栽过一次同族的（图标名走 `data-icon`，
// 那里的判据是 URL 属性，与 CSS 不同）。
//
// 换成 `{{.AccentCSS}}` 之后，模板读的是**一个类型就说明它是什么**的字段，
// 而不是"在 range 里靠 `.` 拿到的、看起来像颜色的东西"。**这是双保险**：
// 指错字段这一种错（真因）不再可能发生；而万一将来判据改了，类型会在编译期
// 顶住——把一个裸 string 写进 CSS 上下文是编译器不管、运行时才炸的形状。
//
// **包它的代价与安全性**：template.CSS 会**跳过全部转义**，所以安全性完全来自
// 上游那一份判据——`validatedAccent` 只放行 `#rgb` / `#rrggbb`
// （model.ValidAccent 是唯一判据），其余一律返回空串。所以这里**不能**直接
// `template.CSS(原始值)`，必须先过判据。
//
// 空串仍是空串：没设色的分组照旧不发这一格（模板的 `{{if .Accent}}`）。
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
// 为什么不把索引搬进 app.js 让前端画：索引是内嵌产物（一万多个名字，品牌层与通用层两层加起来），让**只看
// 首屏的每一页**都背一份图库，正是 §7.3 那条操作线要挡的事。按搜索词要一小格，
// 就只要一小格。
//
// 每格是 `<button type=button>`：选择器常常整个建在**表单里**（添加浮层），一个没写
// type 的按钮点下去会提交那个表单。`aria-label` 不能省——里面的 SVG 是 aria-hidden 的，
// 去掉它这一格对读屏就是一个没有名字的按钮。
//
// **名字要印在格子上**（2026-10-15 batch11 U1）。从前它只在 `aria-label` 与 `title` 里，
// 而那个值是 `simple-icons:amazondocumentdb` 这种内部全键：搜「doc」出 20 格，屏上
// 一个名字都没有，两枚几乎一样的 docker 鲸鱼无从分辨，读屏念出来的还是"simple-icons
// 冒号"。所以可见的那一行与 `aria-label` 都用**去掉前缀的短名**（前缀是索引键的一部分，
// 不是给人挑的那一维），全键留在 `title` 里——脚本作者与 devtools 要对照的仍是它。
func IconTiles(ix *icons.Index, q string, limit int) (template.HTML, int) {
	res := ix.Search(q, "", limit)
	return iconTiles(ix, res.Icons), res.Total
}

// IconTilesFor 把**给定的**一批图标名画成方格（图标选择器默认那一批用）。
//
// 名字不在索引里的跳过（不画一格空白）：那一格是给人挑的，画一格空的白框
// 等于骗人——"这里有一枚图标"与"这里什么都没有"在屏上长得一模一样。
//
// 与 IconTiles 走**同一个画格子的出口**（iconTiles），于是搜索命中与默认
// 那一批画出来的形状逐像素相同，也不用维护第二套 markup。
func IconTilesFor(ix *icons.Index, names []string) template.HTML {
	return iconTiles(ix, names)
}

// iconTiles 是**画一格**这件事的唯一出口：IconTiles（搜索命中）与
// IconTilesFor（默认那一批）共用它。两处各写一份 markup 就会各画各的，
// 于是"挑的时候看见的"与"存下去之后卡片上那一枚"慢慢分家——而那正是
// IconTiles 上面那段注释要保住的事。
func iconTiles(ix *icons.Index, names []string) template.HTML {
	var b strings.Builder
	for _, name := range names {
		ic, ok := ix.Get(name)
		if !ok {
			continue // 索引与这一批同一个快照，这一条走不到；不发一个空格子
		}
		esc := html.EscapeString(name)
		short := name
		if i := strings.IndexByte(name, ':'); i >= 0 {
			short = name[i+1:]
		}
		escShort := html.EscapeString(short)
		b.WriteString(`<button type="button" class="fld icpick-tile" data-icon="`)
		b.WriteString(esc)
		b.WriteString(`" title="`)
		b.WriteString(esc)
		b.WriteString(`" aria-label="`)
		b.WriteString(escShort)
		b.WriteString(`"><span class="icpick-face">`)
		b.WriteString(string(svgMarkup(ic)))
		b.WriteString(`</span><span class="icpick-name">`)
		b.WriteString(escShort)
		b.WriteString(`</span></button>`)
	}
	return template.HTML(b.String()) //lint:ignore 名字来自内嵌索引、其余走 svgMarkup 的白名单
}
