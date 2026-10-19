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

var manifestBytes = []byte(`{
  "name": "Launchbay",
  "short_name": "Launchbay",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#121417",
  "theme_color": "#121417",
  "icons": [
    {
      "src": "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%23388bfd' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M4 13a8 8 0 0 1 7 7 6 6 0 0 0 3-5 6 6 0 0 0-6-6 6 6 0 0 0-4 4z'/%3E%3Cpath d='m14.5 9.5 5-5'/%3E%3Cpath d='m17 7 2-2'/%3E%3C/svg%3E",
      "sizes": "any",
      "type": "image/svg+xml"
    }
  ]
}`)

// handleManifest 吐出 Web App Manifest（PWA 独立窗口运行所需）。
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(manifestBytes)
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
	// 攻击者能让访客拿到一段指向他主机的脚本）。**host[:port] 整体留下**，
	// 其余形状（含 X-Forwarded-Host 一类）一律不采信——判据在 hostPortOf
	// 与 validHostShape 两处。
	host := hostPortOf(r.Host)
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

// hostPortOf 把 Host 头**归一成 host[:port] 并整体留下**。
//
// **端口不许剥（2026-10-19 修）**：这一段原来在剥端口，理由是"只要主机名"，
// 但那等于把这条地址唯一能让它在非 80 端口上成立的那一半扔了——本案的默认
// 端口是 7600（compose 的 `7600:7600`，spec §6.3 写死的就是
// `http://<NAS_IP>:7600/?add=`），所以照默认配置装好、配好书签栏之后，按它
// 会跳到 80 端口去，那台机器上 80 端口什么都没有：书签栏按钮"点了打不开"。
// 症状形状正是 §8.1 那条「点了没反应的东西不进界面」——而这条按钮唯一的卖点
// 是"配一次就永远对"，剥端口把那个卖点废掉了。只有把这台机器反代到 80 端口
// 的人才会"看起来是好的"，而 §11 明写不挂反代。
//
// 它活到现在是因为那条测试只断言 `javascript:`、`/?add=` 这些**子串**，
// 从没断言过整条地址：写错的地址照样让它绿。测试那一半见
// `TestBookmarklet_地址保留端口`——**钉整条，不钉子串**。
//
// **它几乎什么都不做，恰恰是这一段该有的样子**：host[:port] 原样带出去，
// 唯一的加工是去掉一个**空的**尾随冒号（`nas:` → `nas`）。
//
// 判据与形状校验分开放在两处，是有意的：`validHostShape` 答"这段能不能塞进
// JS 字符串字面量"，这里答"Host 头长什么样"。混在一处就会出现先前那种
// "修一半"——地址被拼成 `10.0.0.5port`（一个**合法**主机名），于是连
// 400 都不给你，直接把一条坏地址写进书签栏。
//
// 返回空串表示"形状里就没有可用的 host"（空串），那是 400 的判据，与
// "切出来但形状不合法"（validHostShape 拒）分开。
func hostPortOf(hostport string) string {
	// 尾随的空冒号：`nas:` → `nas`。**只去这一个**，不去裁任何别的东西。
	if strings.HasSuffix(hostport, ":") {
		return hostport[:len(hostport)-1]
	}
	return hostport
}

// validHostShape 认 **host[:port]** 两种形状。
// 它挡的不是"看起来像地址"，而是"能塞进 JS 字符串字面量的东西"——引号、
// 反斜杠、尖括号一律拒。
//
// **端口是 2026-10-19 跟着 hostPortOf 一起放行的**：这一段原来按单个 host 判，
// 于是即便上面那处不剥端口，带冒号的整条地址也会被它拒成 400——那正是"修一半"
// 的形状（IPv4 通、IPv6 反而挂）。所以端口单独一段判：**只能是数字**，
// 不放行任何别的形状，冒号本身也不许出现第二次（`a:b:c` 不是 host:port）。
func validHostShape(h string) bool {
	host, port := h, ""
	if strings.HasPrefix(h, "[") { // IPv6 的冒号在方括号里，不能按冒号切
		if i := strings.Index(h, "]"); i >= 0 {
			host, port = h[:i+1], strings.TrimPrefix(h[i+1:], ":")
		}
	} else if i := strings.LastIndex(h, ":"); i > 0 {
		host, port = h[:i], h[i+1:]
	}
	if port != "" {
		for _, r := range port {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	if strings.HasPrefix(host, "[") {
		return strings.HasSuffix(host, "]") && !strings.ContainsAny(host, `"'<>`+"`"+`\`)
	}
	if host == "" || strings.ContainsAny(host, `"'<>`+"`"+`\/ \`) {
		return false
	}
	for _, r := range host {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}
