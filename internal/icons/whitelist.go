// Package icons 是图标索引的生产方与消费方共用的白名单（spec §5）。
package icons

import "regexp"

// 白名单来自 tools/genicons -probe 对上游全量的实测词表（spec §5）。
// 上游出现预期之外的 tag/attr 时必须当场报错、由人显式加宽，
// 不许静默丢掉——"丢一个元素"在首屏上的表现是一个没人知道被切过的图标。
var drawableTags = map[string]bool{
	"path": true, "circle": true, "rect": true, "line": true,
	"polyline": true, "polygon": true, "ellipse": true,
}

var keepAttrs = map[string]bool{
	"d": true, "cx": true, "cy": true, "r": true, "x": true, "y": true,
	"width": true, "height": true, "rx": true, "ry": true, "points": true,
	"x1": true, "y1": true, "x2": true, "y2": true,
	// fill 在这张表里只登记"这个名字合法"；某条 fill 到底收不收，判据只在下面的
	// layerFillDefaults（本层默认值丢、与本层默认不同的留），两处消费方都问它：
	// tools/genicons/build.go 的丢留、index.go checkElements 的拒。
	// 按这个判据：stroke 层**收** fill="currentColor"（tabler 的 -filled 变体靠它
	// 才有实心）、生成器丢掉 fill="none"（那是本层默认）；fill 层丢
	// fill="currentColor"（本层默认）。fill 层的 fill="none" 不是本层默认、生成器
	// 不丢它，于是 checkElements 整套拒（Task 0 的 -probe 实测：上游两套都无此形状）。
	"fill": true, "fill-rule": true, "clip-rule": true, "opacity": true, "transform": true,
}

// attrValueRE 钉住"数值 + 路径指令字母"。它挡的是引号、尖括号与等号，
// 也就是所有能从属性值里逃出去的字符。渲染层还会再转义一次（Task 9）：
// 两道闸互不信任。
var attrValueRE = regexp.MustCompile(`^[A-Za-z0-9(),.\- ]+$`)

// layerFillDefaults 是每层由 CSS 负责的 fill 呈现，也是"这条 fill 收不收"的
// 唯一判据（上面 keepAttrs 那段注释只负责指到这里，不另立标准）：与默认值相同
// 的 fill 不进产物，省下的每一字节都是重复常量，而"与默认不同"才是要留的信息
// （tabler 的 -filled 变体就靠这一条保住实心）。两个消费方：
// tools/genicons 的丢留、index.go checkElements 的拒。
var layerFillDefaults = map[string]string{"stroke": "none", "fill": "currentColor"}

// 生成器用的谓词。导出这四个而不是导出 map：契约的点在于"这几个问题"，
// 把 map 交出去只会诱导调用方绕过谓词自己判。
func IsDrawable(tag string) bool   { return drawableTags[tag] }
func IsKeptAttr(name string) bool  { return keepAttrs[name] }
func ValidAttrValue(v string) bool { return attrValueRE.MatchString(v) }
func IsLayerDefaultFill(layer, value string) bool {
	return layerFillDefaults[layer] == value
}

// IsLayer 判"这个值是不是已知层"。层的枚举**就是** layerFillDefaults 的键，
// 不另立一份字面量：加载侧校验（index.go Decode）与渲染侧第二道闸
// （render/icon.go svgMarkup）都问这同一个谓词——一处判据、量两遍，
// 加第三个消费方不用抄第三个 switch（B10）。
func IsLayer(layer string) bool {
	_, ok := layerFillDefaults[layer]
	return ok
}
