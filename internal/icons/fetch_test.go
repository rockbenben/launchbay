package icons

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fastFetcher() *Fetcher {
	return &Fetcher{Timeout: 500 * time.Millisecond, MaxBytes: 4096}
}

func TestFetcher_页面拿到字节与最终地址(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><title>x</title></html>"))
	}))
	defer srv.Close()
	body, final, err := fastFetcher().Page(srv.URL)
	if err != nil {
		t.Fatalf("正常页面不该报错: %v", err)
	}
	if !strings.Contains(string(body), "<title>") {
		t.Errorf("字节没拿到: %q", body)
	}
	if final != srv.URL+"/" && final != srv.URL {
		t.Errorf("finalURL 要能用于相对链接解析，得到 %q", final)
	}
}

func TestFetcher_三种URL一律拒(t *testing.T) {
	f := fastFetcher()
	for _, bad := range []string{"", "not a url", "ftp://10.0.0.1/", "file:///etc/passwd", "http://"} {
		if _, _, err := f.Page(bad); err != ErrBadURL {
			t.Errorf("%q 要 ErrBadURL，得到 %v", bad, err)
		}
	}
}

func TestFetcher_大小上限(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(strings.Repeat("a", 4097)))
	}))
	defer srv.Close()
	if _, _, err := fastFetcher().Page(srv.URL); err != ErrTooBig {
		t.Errorf("超 4096 要 ErrTooBig，得到 %v", err)
	}
}

func TestFetcher_超时(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	f := &Fetcher{Timeout: 50 * time.Millisecond, MaxBytes: 4096}
	if _, _, err := f.Page(srv.URL); err != ErrTimeout {
		t.Errorf("要 ErrTimeout，得到 %v", err)
	}
}

// TestFetcher_超时预算是每一跳各自的 区分"每一跳各自 3 秒"与"整个交换 3 秒"：
// 两跳各 150ms、预算 200ms，按每跳算应当成功；若把预算做成覆盖整条重定向链
// （fetch 入口挂一个共享 deadline），累计 300ms 会误报 ErrTimeout。
// TestFetcher_超时 本身区分不了这两者——单跳时两种形状都超时——所以这条不是
// 锦上添花，是那条约束的唯一证人。
func TestFetcher_超时预算是每一跳各自的(t *testing.T) {
	var hops int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 150ms 对 200ms 的预算只留 50ms 余量：在 alpine + QEMU 模拟 + 十三个包
		// 并行抢 CPU 的构建环境里，调度抖动轻松吃掉这 50ms，于是**测试红了而
		// 代码没错**（2026-10-12 真在 CI 上红过一次：docker.yml 的「构建并推送」）。
		// 整个时间轴按**同一个倍数**放大：判别力来自「两跳共 900ms > 预算 600ms」
		// 这个不等式，放大不改变它的成立，所以只放宽抖动余量、不放宽语义。
		time.Sleep(450 * time.Millisecond)
		hops++
		if hops == 1 {
			http.Redirect(w, r, srv.URL+"/end", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer srv.Close()
	f := &Fetcher{Timeout: 600 * time.Millisecond, MaxBytes: 4096}
	if _, _, err := f.Page(srv.URL); err != nil {
		t.Errorf("两跳各 450ms、每跳预算 600ms 不该超时，得到 %v", err)
	}
}

// TestFetcher_慢正文也算超时 钉死"3 秒覆盖整个交换（含正文读取）"这一半语义，
// 是 M-1 的证人。前面两条超时测试都把慢放在响应头之前（服务器先 Sleep 再返回 /
// 先 Sleep 再重定向），所以它们把 Timeout 挂到 Transport.ResponseHeaderTimeout
// 上照样假绿——那种形状只封顶响应头、正文滴完仍成功返回。
//
// 这把夹具用 NewUnstartedServer + Flusher：先把响应头 flush 落地（ResponseHeaderTimeout
// 到此即满足），再逐段慢滴正文、累计远大于预算。
//   - 真实 Client.Timeout（覆盖 dial+头+正文全程）：预算 120ms 到点仍卡在正文读取上，
//     被中断 → classify → ErrTimeout。
//   - ResponseHeaderTimeout（只封顶头）：头秒回、正文 400ms 滴完，err = nil → 红。
func TestFetcher_慢正文也算超时(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush() // 响应头立刻落地，把 ResponseHeaderTimeout 这一段先满足掉
		}
		for i := 0; i < 8; i++ {
			_, _ = io.WriteString(w, "<p>chunk</p>")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(50 * time.Millisecond) // 正文分次慢写，累计约 400ms
		}
	}))
	srv.Start()
	defer srv.Close()

	f := &Fetcher{Timeout: 120 * time.Millisecond, MaxBytes: 4096}
	if _, _, err := f.Page(srv.URL); err != ErrTimeout {
		t.Errorf("正文读取必须计入超时预算，得到 %v", err)
	}
}

func TestFetcher_重定向三次封顶(t *testing.T) {
	var hops int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		if hops < 6 {
			http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer srv.Close()

	if _, _, err := fastFetcher().Page(srv.URL); err != ErrTooManyRedirects {
		t.Errorf("链路 6 跳要 ErrTooManyRedirects，得到 %v", err)
	}
}

func TestFetcher_相对Location也能跟(t *testing.T) {
	var n int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/from") {
			n++
			http.Redirect(w, r, "/to", http.StatusFound) // 相对 Location
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><title>到了</title></html>"))
	}))
	defer srv.Close()
	body, final, err := fastFetcher().Page(srv.URL + "/from")
	if err != nil {
		t.Fatalf("相对 Location 必须能跟: %v", err)
	}
	if n != 1 || !strings.HasSuffix(final, "/to") {
		t.Errorf("final=%s 没落到 /to（命中 %d 次）", final, n)
	}
	_ = body
}

func TestFetcher_非2xx即失败不解析(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><title>404</title></html>"))
	}))
	defer srv.Close()
	if _, _, err := fastFetcher().Page(srv.URL); err != ErrStatus {
		t.Errorf("要 ErrStatus，得到 %v", err)
	}
}

// TestFetcher_Image五种媒体类型各自的后缀 是那张 media-type→ext switch 的
// 全分支证人（最终整枝评审 Important 1）：此前这里只测过 image/png 一条，
// 断掉 ico/svg/webp/gif 任何一支、全套测试仍绿，而那类 favicon 会静默不再
// 落地（fetch 给的 ext 与 store.iconExts 不匹配 → Put 拒 → resolve 降级
// favicon:null）。后缀串逐条断言：ext 是缓存文件名的一部分，"返回了某个
// image/* "不算数，必须是**期望的那一个后缀**。
// image/x-icon、image/vnd.microsoft.icon、image/ico 三个变体各占一行——
// 那是那个 case 的真实分支数，少一行就有一支没人盯。
func TestFetcher_Image五种媒体类型各自的后缀(t *testing.T) {
	cases := []struct {
		media   string
		wantExt string
	}{
		{"image/png", "png"},
		{"image/x-icon", "ico"},
		{"image/vnd.microsoft.icon", "ico"},
		{"image/ico", "ico"},
		{"image/svg+xml", "svg"},
		{"image/webp", "webp"},
		{"image/gif", "gif"},
	}
	for _, tc := range cases {
		t.Run(tc.media, func(t *testing.T) {
			payload := append([]byte{0x00, 0x01, 0x02}, []byte(tc.media)...)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.media)
				_, _ = w.Write(payload)
			}))
			defer srv.Close()
			body, ext, err := fastFetcher().Image(srv.URL)
			if err != nil {
				t.Fatalf("%s 应当收进图片通路: %v", tc.media, err)
			}
			if ext != tc.wantExt {
				t.Errorf("%s 的 ext = %q，要 %q——这一串直接决定 store.Put 认不认、文件叫什么名", tc.media, ext, tc.wantExt)
			}
			if len(body) != len(payload) {
				t.Errorf("字节没原样拿到: %d vs %d", len(body), len(payload))
			}
		})
	}
}

func TestFetcher_类型必须对得上(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4"))
	}))
	defer page.Close()
	if _, _, err := fastFetcher().Page(page.URL); err != ErrWrongType {
		t.Errorf("页面要 text/html，得到 %v", err)
	}

	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer img.Close()
	body, ext, err := fastFetcher().Image(img.URL)
	if err != nil || ext != "png" || len(body) == 0 {
		t.Errorf("png 应当收，得到 %q %v", ext, err)
	}
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer html.Close()
	if _, _, err := fastFetcher().Image(html.URL); err != ErrWrongType {
		t.Errorf("图片通路必须只认 image/*，得到 %v", err)
	}
	// image/* 但白名单认不得的类型也要拒：宁缺勿错存，错存会进内容哈希缓存。
	jpeg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF})
	}))
	defer jpeg.Close()
	if _, _, err := fastFetcher().Image(jpeg.URL); err != ErrWrongType {
		t.Errorf("认不得的 image/* 不能进缓存，得到 %v", err)
	}
}

func TestOrigin_带端口且形状稳定(t *testing.T) {
	for in, want := range map[string]string{
		"http://10.0.0.3:8096/web/index": "http://10.0.0.3:8096",
		"https://newzone.top/posts/1":    "https://newzone.top",
		"http://Host.LAN:80/x":           "http://host.lan:80", // 主机名小写化：同一服务别算成两个 origin
		"chrome://extensions/":           "",
	} {
		if got := Origin(in); got != want {
			t.Errorf("Origin(%q) = %q，要 %q", in, got, want)
		}
	}
}
