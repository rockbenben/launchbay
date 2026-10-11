package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// iconFileNameRE 与 store.iconExts、icons.Fetcher.Image 的后缀表是同一件事
// 的四处表述——改一处必须改四处。钉这个的不再是"某次破坏实验"而是常驻证人：
// internal/httpapi/resolve_test.go#TestResolve_gif端到端绑住三张生产表 拿
// image/gif 走满 fetch→缓存→本通路三张表（png 那条在
// resolve_test.go#TestResolve_拿到四项候选），iconfile_test.go#
// TestIconFile_五种后缀表驱动各端各的 按 iconContentTypes 逐后缀断类型头。
//
// 只认 <16 位小写十六进制>.<白名单后缀>：形状即边界。它同时挡掉路径穿越
// （没有 '/'，也没有 ".."）与"把任意文件当图片端出去"——缓存目录里除了
// blob 还有 index.json，那是缓存的索引而不是图片。
var iconFileNameRE = regexp.MustCompile(`^[0-9a-f]{16}\.(png|ico|svg|webp|gif)$`)

// iconContentTypes 按后缀给类型：这里是查表而不是猜字节，嗅探等于让
// 第三方内容决定自己的身份（下面 nosniff 那句是同一件事的另一半）。
var iconContentTypes = map[string]string{
	"png":  "image/png",
	"ico":  "image/x-icon",
	"svg":  "image/svg+xml",
	"webp": "image/webp",
	"gif":  "image/gif",
}

// iconMissingDetail 两处 404 共用一句：形状非法与文件不存在对调用方是同一个
// 答案（这里不是诊断通路，回显"为什么没有"就是告诉探测者他猜对了形状）。
const iconMissingDetail = "没有这个图标"

// handleIconFile 端出 resolve 抓来的 favicon 缓存（spec §5、§7.1）。
//
// 公开且 immutable：文件名就是内容哈希，改内容必然改名字。它不含任何
// 档案信息，所以 credNone 够用；§11 明写这条通路不加鉴权、不加速率限制，
// 读侧也不许往 data/icons/ 写任何东西（那是 resolve 内部唯一的写口）。
//
// 第三方字节只走 `<img src>`：渲染层绝不把它当 markup 拼进 HTML——浏览器在
// img 语境里不执行 SVG 内脚本，而一旦被内联，那个沙箱假设就没了（Task 9）。
//
// 但那句"只走 <img>"是**客户端**约定：谁把 /icons/<hash>.<ext> 直接开进地址
// 栏（或点开别人发来的链接），响应就是你自己的一个文档，SVG 里的脚本会跑在
// launchbay 这个 origin 上，而会话 cookie 也正落在这个 origin 上。裁定 R9
// 因此把边界挪回服务端：整条通路一律带 CSP sandbox——它不加机构、不给使用
// 添任何摩擦，也不碰 §11 明确不做的那几样。放在分支之前，所以 404 也带着它。
func (s *Server) handleIconFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "sandbox")
	name := r.PathValue("name")
	if !iconFileNameRE.MatchString(name) {
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: iconMissingDetail})
		return
	}
	ext := name[strings.LastIndexByte(name, '.')+1:]
	data, err := os.ReadFile(filepath.Join(s.deps.Store.IconsDir(), name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: iconMissingDetail})
			return
		}
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", iconContentTypes[ext])
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
