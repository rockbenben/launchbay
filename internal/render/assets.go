package render

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
)

//go:embed web
var webFS embed.FS

// appJSFile 是 ⑤ 的脚本。指纹在进程里算一次，不在构建期生成文件：指纹必须
// 跟着内容变，而内容是 //go:embed 进来的编译期常量，于是"运行时算"与
// "构建期算"结果完全一致，却省掉了构建步骤与一个要提交的产物文件——而那个产物
// 文件会漂（改了 app.js 忘了重新生成，指纹与字节对不上，浏览器把旧 JS 永久
// 缓存住，而本地看着一切正常）。
//
// 指纹取内容哈希的前 12 位十六进制：够短（URL 里不难看）、够宽（同一个内容两次
// 撞上同一短串的概率小到可以当"不可能"），且**只由内容决定**——改一个字换一条
// 路径，这是 immutable 缓存能成立的全部前提。
//
// ⑥ 的 sw.js **不按指纹命名**：Service Worker 的更新由浏览器按注册处自己检查
// （每次导航拿 /sw.js 的字节比一次），指纹加在文件名上只会多一处要同步的地方。
// 真正控制"什么时候让用户看到新版本"的是 §7.1 那条「不静默 skipWaiting，顶栏出
// 提示」——那才是用户看得见的更新时机。
const (
	appJSFile  = "web/app.js"
	appCSSFile = "web/app.css"
	swJSFile   = "web/sw.js"
)

var (
	// appJSBytes 是 ⑤ 的第一段真字节（§8.1 的「＋ 添加」浮层）。
	appJSBytes = mustWebFile(appJSFile)
	// AppJSPath 是它在页面里该被引用的那个指纹路径（不是函数：它在包初始化时
	// 就定下来了，路由表要拿它注册一条**具体**路径，函数会让 New 之前取不到）。
	AppJSPath = fingerprint("/app", ".js", appJSBytes)

	// appCSSBytes 与 AppCSSPath 是 ⑤/⑥ 的**交互**样式（浮层、⌘K、就地编辑）。
	//
	// **它走外部指纹文件而不内联**：base.css 是首屏骨架，内联进 HTML（§7.2），
	// 而这些交互样式只在**档案页**用得到——内联等于让选择页、引导页、管理页、
	// 坏档页白各带一份它们一个都用不上的字节。而 spec §7.1 早就把
	// `/*.<hash>.css|js` 定成 immutable 外部资源了，这里只是补上没做的那一半。
	appCSSBytes = mustWebFile(appCSSFile)
	AppCSSPath  = fingerprint("/app", ".css", appCSSBytes)

	// swJSBytes 是 ⑥ 的 Service Worker。
	swJSBytes = mustWebFile(swJSFile)
)

// mustWebFile 在包初始化时读内嵌文件。它 panic 而不是返回 error：embed 路径写错
// 是**装配错误**（编译期就该发现的），而运行到第一个请求才发现就晚了——
// 降级成"这个页面没有 JS"会让 §7.2 那条判据悄悄变成一句谎。
func mustWebFile(name string) []byte {
	b, err := webFS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("内嵌资源 %s 读不出来: %v", name, err))
	}
	return b
}

// fingerprint 拼出 `/app.<hash>.js` 这样的路径。
func fingerprint(prefix, ext string, raw []byte) string {
	sum := sha256.Sum256(raw)
	return prefix + "." + hex.EncodeToString(sum[:])[:12] + ext
}

// AppJS / AppCSS 供静态资产端点原样吐出。
func AppJS() []byte  { return appJSBytes }
func AppCSS() []byte { return appCSSBytes }

// SWJS 返回 ⑥ 的 Service Worker 字节。
func SWJS() []byte { return swJSBytes }
