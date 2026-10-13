package httpapi

import (
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/render"
)

// handleAppJS 吐出 ⑤ 的应用脚本。
//
// **`Cache-Control: public, max-age=31536000, immutable` 是这一整条通路的理由**
// （§7.1）：URL 里带着内容指纹，所以内容一变 URL 就变，而内容不变时浏览器可以
// 永久缓存。少了 immutable，同一个 URL 每次都要问一次——于是我们还得给 JS 加
// ETag 与 304，那是一条在局域网里换不回任何东西的往返。
//
// ETag **不给**：指纹已经是内容寻址，再叠一层 ETag 只是多一次协商。
//
// 凭据是公开（credNone）：脚本里没有任何档案数据，书签、名字、条目一个都没有。
// 它必须公开是因为 CSP 的 `script-src 'self'` 要求脚本来自自己的源——而不公开的
// 资源在"没登录"的那些页面上根本拿不到。
func (s *Server) handleAppJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(render.AppJS())
}

// handleAppCSS 吐出 ⑤/⑥ 的交互样式。**与 handleAppJS 完全同款**：内容指纹命名 +
// immutable，不给 ETag。
func (s *Server) handleAppCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(render.AppCSS())
}

// handleSWJS 吐出 ⑥ 的 Service Worker。
//
// **`Cache-Control: no-cache` 是刻意的，与 app.js 那条 immutable 相反**：SW 的
// 更新机制正是"每次导航比对 /sw.js 的字节"，而 no-cache 仍然允许条件请求——
// 于是它每次都会被重新校验，浏览器才知道字节变了。给它 immutable 等于让更新
// 检查形同虚设，而 §7.1 那条「检测到 rev 变化时顶栏出提示」的前提就是浏览器
// 真的看得见变化。
//
// **`Service-Worker-Allowed` 不设**：SW 就在根路径，默认作用域已经是整站，设它
// 只会多一行让人以为路径可以挪走。
func (s *Server) handleSWJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(render.SWJS())
}

// handleBookmarklet 吐出那段拖进书签栏的 JS（§6.3）。
//
// **它自己算出这台机器的地址**（从请求的 Host 头），而不是要人来填：填错了的
// bookmarklet 会一直把人送到一个打不开的地址，而那种错误要等到用的时候才发现。
//
// **里面绝不能有令牌**，而且这里也是唯一一处把服务端地址写进用户可复制的地方：
// Chrome 书签会云同步，任何写进 bookmarklet 的令牌都等于明文外发（§6.3 的铁律）。
// 走的是 `?add=` 导航 + 会话 cookie——家里人不需要在任何地方配令牌。
func (s *Server) handleBookmarklet(w http.ResponseWriter, r *http.Request) {
	// Host 头由客户端给出，不能直接拼进输出的 JS 里（那是一次反射点：
	// 攻击者能让访客拿到一段指向他主机的脚本）。只取 host:port 形状，
	// 其余（含 X-Forwarded-Host 一类）一律不采信。
	host := r.Host
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
		host = host[:i]
	}
	if host == "" || !validHostShape(host) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.HostInvalid, Detail: "Host 头形状不对"})
		return
	}
	// scheme 在局域网里几乎总是 http（这台服务自己不上 TLS，§11 明写不防）。
	// 写死而不是从 X-Forwarded-Proto 读：那条头是给"前面挂了反代"准备的，
	// 而本站没有反代，读它等于凭空多一条可伪造的输入。
	js := "javascript:(function(){location.href='http://" + host +
		"/?add='+encodeURIComponent(location.href)+'&t='+encodeURIComponent(document.title);})();"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, js)
}

// validHostShape 只认「标签数字点」与「IPv6 方括号」两种形状。
// 它挡的不是"看起来像地址"，而是"能塞进 JS 字符串字面量的东西"——引号、
// 反斜杠、尖括号一律拒。
func validHostShape(h string) bool {
	if strings.HasPrefix(h, "[") {
		return strings.HasSuffix(h, "]") && !strings.ContainsAny(h, `"'<>`+"`"+`\`)
	}
	if h == "" || strings.ContainsAny(h, `"'<>`+"`"+`\/ \`) {
		return false
	}
	for _, r := range h {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}
