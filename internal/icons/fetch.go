package icons

import (
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 抓取约束全部来自 spec §5，一条都不许多（"多出来的防护也算失败"）。
// 运行时唯一的抓取入口是 /api/v1/resolve，它要求凭证，所以这里不设速率限制。
const (
	FetchTimeout   = 3 * time.Second
	FetchMaxBytes  = 256 << 10 // 256KB
	MaxRedirects   = 3
	fetchUserAgent = "launchbay/1.0 (+LAN bookmark navigator; resolve only)"
)

// 每个 sentinel 都带一句人话：它们会被原样放进 502 的 detail。
// 所以这里绝不放 Go 的内部错误文本（里面有 URL、实现细节与换行）。
var (
	ErrBadURL           = errors.New("地址不是可用的 http/https 链接")
	ErrUnreachable      = errors.New("目标连不上")
	ErrTimeout          = errors.New("目标在 3 秒内没有响应")
	ErrStatus           = errors.New("目标没给成功状态")
	ErrTooBig           = errors.New("目标响应超过大小上限")
	ErrWrongType        = errors.New("目标返回的不是要求的类型")
	ErrTooManyRedirects = errors.New("重定向次数超过上限")
)

// Fetcher 只有两个可调项，都是 spec §5 的数字。没有 Client 字段：
// 每请求现建 Client 才是"每一跳各自 3 秒"的正确形状（见 httpClient 的注释），
// 留一个没人读的 Client 只会让人以为它能注入。
type Fetcher struct {
	Timeout  time.Duration
	MaxBytes int64
}

func NewFetcher() *Fetcher {
	return &Fetcher{Timeout: FetchTimeout, MaxBytes: FetchMaxBytes}
}

// httpClient 每请求现建一个 Client 而不是复用 f.Client：
// `http.Client.Timeout` 覆盖单次交换（含这一次 Do 里的 dial、响应头与正文
// 读取）而不是单次 dial/read。重定向由下面的循环自己跟（见 fetch），所以每次
// Do 就是一跳，预算自然是一跳一份——整条链没有共享 deadline，也不该有。
// 测超时的两条测试依赖这个语义——把 Timeout 挪到 Transport 上，它们会假绿。
func (f *Fetcher) httpClient() *http.Client {
	return &http.Client{
		Timeout: f.timeout(),
		// 重定向自己数：默认 Client 跟 10 次，那会把"≤3"变成注释。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (f *Fetcher) timeout() time.Duration {
	if f.Timeout > 0 {
		return f.Timeout
	}
	return FetchTimeout
}

func (f *Fetcher) maxBytes() int64 {
	if f.MaxBytes > 0 {
		return f.MaxBytes
	}
	return FetchMaxBytes
}

// Page 抓一个 HTML 页面，返回原始字节与跟完重定向后的最终地址。
// 字节交给调用方解析（extract.go），这里不解析——不留 HTML 是 resolve 的
// 职责（spec §6.1），而"不解析"就是"不留"最好实现的方式。
func (f *Fetcher) Page(rawURL string) ([]byte, string, error) {
	body, final, err := f.fetch(rawURL, func(ct string) bool {
		media, _, _ := mime.ParseMediaType(ct)
		return media == "text/html" || media == "application/xhtml+xml"
	})
	if err != nil {
		return nil, "", err
	}
	return body, final, nil
}

// Image 抓一张图片，返回字节与扩展名（内容哈希缓存的文件名后缀由它给）。
//
// svg 只会被渲染成 <img src>，绝不内联：浏览器在 <img> 语境里不执行 SVG 内的
// 脚本，而一旦把它当 markup 拼进 HTML，那个沙箱假设就没了。
func (f *Fetcher) Image(rawURL string) ([]byte, string, error) {
	var ext string
	body, _, err := f.fetch(rawURL, func(ct string) bool {
		media, _, _ := mime.ParseMediaType(ct)
		if !strings.HasPrefix(media, "image/") {
			return false
		}
		switch media {
		case "image/png":
			ext = "png"
		case "image/x-icon", "image/vnd.microsoft.icon", "image/ico":
			ext = "ico"
		case "image/svg+xml":
			ext = "svg"
		case "image/webp":
			ext = "webp"
		case "image/gif":
			ext = "gif"
		default:
			return false // 认不得的类型不进缓存（宁缺勿错存）
		}
		return true
	})
	if err != nil {
		return nil, "", err
	}
	return body, ext, nil
}

func (f *Fetcher) fetch(rawURL string, wantType func(string) bool) ([]byte, string, error) {
	u, err := parseHTTP(rawURL)
	if err != nil {
		return nil, "", err
	}
	client := f.httpClient()
	cur := u.String()

	for hop := 0; hop <= MaxRedirects; hop++ {
		req, err := http.NewRequest(http.MethodGet, cur, nil)
		if err != nil {
			return nil, "", ErrBadURL
		}
		req.Header.Set("user-agent", fetchUserAgent)
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", classify(err)
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := strings.TrimSpace(resp.Header.Get("Location"))
			drain(resp.Body)
			if loc == "" {
				return nil, "", ErrStatus
			}
			next, err := url.Parse(loc)
			if err != nil {
				return nil, "", ErrBadURL
			}
			base, _ := url.Parse(cur)
			cur = base.ResolveReference(next).String() // 相对 Location 在这里落地
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			drain(resp.Body)
			return nil, "", ErrStatus
		}
		if !wantType(resp.Header.Get("Content-Type")) {
			drain(resp.Body)
			return nil, "", ErrWrongType
		}
		// 多读 1 字节才能区分"正好到上限"与"超了"：只读 max 会把超限的
		// 内容当成合法内容存进缓存。
		data, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes()+1))
		closeBody(resp.Body)
		if err != nil {
			return nil, "", classify(err)
		}
		if int64(len(data)) > f.maxBytes() {
			return nil, "", ErrTooBig
		}
		return data, cur, nil
	}
	return nil, "", ErrTooManyRedirects
}

func parseHTTP(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrBadURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, ErrBadURL
	}
	return u, nil
}

// classify 把 net 层的一百种错误收成两条：超时与连不上。
// 原始错误里有 URL 与 Go 内部结构，一个字都不许进响应。
func classify(err error) error {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ErrTimeout
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return ErrTimeout
	}
	return ErrUnreachable
}

func drain(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4<<10))
	_ = body.Close()
}

func closeBody(body io.ReadCloser) { _ = body.Close() }

// Origin 把一条书签地址折成缓存键：scheme://host:port。
//
// 端口要留着：同一台机器上 8096 的 Jellyfin 与 32400 的 Plex 是两张不同的图。
// 主机名统一小写，否则同一条服务会因为大小写写两次而各抓一遍。
// 解析不了或不是 http/https（`chrome://extensions/` 这类）返回空串，
// 调用方按"没有图标"处理——不报错，spec §9 要那些条目原样保留。
func Origin(rawURL string) string {
	u, err := parseHTTP(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}
