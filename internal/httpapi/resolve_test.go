package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// stubSite 是一个只在本机回环上跑的"目标站点"夹具。
// spec §5 的抓取约束只能靠真 HTTP 测，所以这里用 httptest 而不是假接口：
// 假接口会让"重定向 ≤3"这类判据在实现里失去意义。
//
// 图片端点不按路径注册，而是"带图标后缀的路径都算一次抓图尝试"：冷却的证人
// 必须数得到**失败**的尝试，否则把 InCooldown 那行删掉也不会有任何计数变化，
// 那条用例就成了永真。页面仍由 "/" 兜底服务，所以 /portainer、/stacks
// 这类业务路径不会被误认成图片。
type stubSite struct {
	srv *httptest.Server
	// imgHits 数的是**抓图尝试**，成功失败都算：冷却的证人要看的正是失败
	// 的那几次，只数成功的话破坏 InCooldown 不会改变任何断言。
	imgHits atomic.Int32
}

// isIconPath 判定桩站里哪些路径算"抓图尝试"。后缀集合必须与 store.iconExts、
// httpapi 的 iconFileNameRE 同集（三处表述，改一处要改三处；第四处交叉核对——
// 渲染层引用——随渲染层的任务来）：这里少认一个后缀，冷却证人的计数就静默
// 停摆，冷却测试变回空转，而没有任何一条测试会红。
func isIconPath(p string) bool {
	for _, ext := range []string{".png", ".ico", ".svg", ".webp", ".gif"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func newStubSite(t *testing.T, html string, serveIcon bool) *stubSite {
	s := &stubSite{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isIconPath(r.URL.Path) {
			s.imgHits.Add(1)
			if !serveIcon || r.URL.Path != "/apple-touch-icon.png" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func resolveBody(t *testing.T, profile, url string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"url": url, "profile": profile})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func doResolve(t *testing.T, h http.Handler, body []byte, tok string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/resolve", bytes.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func TestResolve_拿到四项候选(t *testing.T) {
	site := newStubSite(t, `<html><head>
		<title>Docker</title>
		<meta property="og:description" content="容器管理">
		<link rel="apple-touch-icon" href="/apple-touch-icon.png">
		</head><body>正文不要</body></html>`, true)
	_, h := setupBatch(t)
	rec := doResolve(t, h, resolveBody(t, "dad", site.srv.URL+"/portainer"), dadTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	var got resolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "Docker" || got.Desc != "容器管理" {
		t.Errorf("title/desc 不对: %+v", got)
	}
	// 探针实测（见 task-8-report.md）：simple-icons:docker 在索引里、
	// tabler:docker 不在。所以这里点名要那一条，不用"或"——用"或"的话
	// 候选逻辑坏掉一半还能绿。
	if !slices.Contains(got.IconCandidates, "simple-icons:docker") {
		t.Errorf("候选图标里必须有 simple-icons:docker（索引里真有这一条）: %v", got.IconCandidates)
	}
	if got.Favicon == nil || !strings.HasPrefix(*got.Favicon, "/icons/") {
		t.Fatalf("favicon 要落地成 /icons/…，得到 %v", got.Favicon)
	}
	// 把端点给的路径真的 GET 一次：前缀断言挡不住 Put 的命名或后缀漂移——
	// 那会端给每个调用方一个必然 404 的 URL，而这套测试照样绿。这一 GET
	// 把 resolve→图标缓存→/icons/{name} 图片通路连成端到端证人。
	img := get(h, *got.Favicon, "")
	if img.Code != http.StatusOK {
		t.Errorf("resolve 给的 %s 要真取得到，得到 %d: %s", *got.Favicon, img.Code, img.Body)
	}
	if !bytes.Equal(img.Body.Bytes(), []byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4}) {
		t.Errorf("取回的字节与桩站那张图不一致: %v", img.Body.Bytes())
	}
	if strings.Contains(rec.Body.String(), "正文不要") {
		t.Error("正文漏进响应了：spec §6.1 明写不留 HTML")
	}
}

// TestResolve_gif端到端绑住三张生产表 是最终整枝评审 Important 1 的那条交叉
// 证人：五后缀表述散在 fetch 的 media-type 分支、store.iconExts、iconfile 的
// 正则与 Content-Type 表四处，今天一致、断一条却全程静默（fetch 给的 ext 与
// store 的表不匹配 → Put 拒 → resolve 降级 favicon:null，没有一行报错）。
// 这条用 gif（不是 png——png 已被 TestResolve_拿到四项候选 端到端证过）走满
// 整条链：夹具站回 image/gif → 断 favicon 路径以 .gif 结尾（钉 fetch 的 ext
// 与 store.iconExts），再 GET 回 200 且 Content-Type: image/gif（钉
// iconFileNameRE 与 iconContentTypes）。任何一张表漂掉 gif，这条在对应那一环
// 转红，而不是静默降级。
func TestResolve_gif端到端绑住三张生产表(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isIconPath(r.URL.Path) {
			w.Header().Set("Content-Type", "image/gif")
			_, _ = w.Write([]byte("GIF89a-\x01\x02\xf9\x03"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>G</title>
			<link rel="icon" href="/fav.gif"></head></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, h := setupBatch(t)
	rec := doResolve(t, h, resolveBody(t, "dad", srv.URL), dadTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	var got resolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Favicon == nil || !strings.HasSuffix(*got.Favicon, ".gif") {
		t.Fatalf("image/gif 要落成 .gif 结尾的 favicon 路径（fetch 的 ext 与 store.iconExts 都得认它），得到 %v", got.Favicon)
	}
	img := get(h, *got.Favicon, "")
	if img.Code != http.StatusOK {
		t.Fatalf("%s GET 回 %d：iconfile 的正则或缓存读通路漂了：%s", *got.Favicon, img.Code, img.Body)
	}
	if ct := img.Header().Get("Content-Type"); ct != "image/gif" {
		t.Errorf("%s 的 Content-Type = %q，要逐字 image/gif（iconContentTypes 表漂就体现在这里）", *got.Favicon, ct)
	}
}

func TestResolve_第二次不重抓图(t *testing.T) {
	site := newStubSite(t, `<html><head><title>J</title>
		<link rel="apple-touch-icon" href="/apple-touch-icon.png"></head></html>`, true)
	_, h := setupBatch(t)
	body := resolveBody(t, "dad", site.srv.URL)
	doResolve(t, h, body, dadTok)
	site.imgHits.Store(0)
	doResolve(t, h, body, dadTok)
	if site.imgHits.Load() != 0 {
		t.Errorf("已有缓存还去抓第二次，命中 %d 次", site.imgHits.Load())
	}
}

// 冷却的证人（spec §5 第 1 条"失败按 origin 记 24 小时冷却"）。
// 图端点持续 404：第一次 resolve 已经打过它一次，所以计数清零后再来两次
// 才是要断言的那一段——不清零就断言 0，正确的实现反而会红。
func TestResolve_抓图失败后按origin冷却不再重抓(t *testing.T) {
	site := newStubSite(t, `<html><head><title>x</title>
		<link rel="icon" href="/nope.ico"></head></html>`, false)
	_, h := setupBatch(t)
	body := resolveBody(t, "dad", site.srv.URL)
	doResolve(t, h, body, dadTok)
	site.imgHits.Store(0)
	doResolve(t, h, body, dadTok)
	doResolve(t, h, body, dadTok)
	if got := site.imgHits.Load(); got != 0 {
		t.Errorf("抓图失败后该按 origin 冷却，两次 resolve 里又打了图片端点 %d 次", got)
	}
}

func TestResolve_建议分组只从该档案现有分组里选(t *testing.T) {
	site := newStubSite(t, `<html><head><title>Portainer</title></head></html>`, false)
	s, h := setupBatch(t)
	seedItem(t, s, "dad", "nas", "jellyfin", "http://10.0.0.3:8096/") // 不同 host
	// 夹具站点的 host 是 127.0.0.1，与档案里任何分组都不相干：
	// 宁可不给，也不编一个分组出来。
	rec := doResolve(t, h, resolveBody(t, "dad", site.srv.URL), dadTok)
	var got resolveResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.SuggestedGroup != nil {
		t.Errorf("没有依据时不该给建议分组: %+v", got.SuggestedGroup)
	}

	// 同 host 的第二个条目落进 nas 之后，同一个 URL 就该被认到 nas。
	seedItem(t, s, "dad", "nas", "portainer", site.srv.URL+"/stacks")
	rec = doResolve(t, h, resolveBody(t, "dad", site.srv.URL), dadTok)
	got = resolveResponse{}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.SuggestedGroup == nil || got.SuggestedGroup.ID != "nas" {
		t.Errorf("同 host 的兄弟条目在 nas，建议却 = %+v", got.SuggestedGroup)
	}
}

func TestResolve_凭证与档案申报的纪律(t *testing.T) {
	site := newStubSite(t, `<html><head><title>x</title></head></html>`, false)
	_, h := setupBatch(t)
	cases := []struct {
		name, profile, tok string
		want               int
	}{
		{"无凭证", "dad", "", http.StatusUnauthorized},
		{"陌生令牌", "dad", "nope", http.StatusUnauthorized},
		{"作用域不含该档案", "kid", dadTok, http.StatusForbidden},
		{"本人作用域", "dad", dadTok, http.StatusOK},
		// 2026-10-10：`*` 过不了 credScoped，所以这一格换成了"档案作用域令牌"。
		{"档案作用域令牌", "dad", dadTok, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doResolve(t, h, resolveBody(t, tc.profile, site.srv.URL), tc.tok)
			if rec.Code != tc.want {
				t.Errorf("%d，要 %d；body=%s", rec.Code, tc.want, rec.Body)
			}
		})
	}
	// 空申报：credScoped 的定义就是"不申报即拒"（P1-② 历史端点评审的 Critical 口径）。
	// 断言钉死 403 而不是"401 或 403"：凭证是好的（dadTok 认得出来），
	// 它只是没申报目标档案——AGENTS.md 那句"401 与 403 永远分开"要的就是这个答案。
	raw, _ := json.Marshal(map[string]string{"url": site.srv.URL})
	if rec := doResolve(t, h, raw, dadTok); rec.Code != http.StatusForbidden {
		t.Errorf("不申报 profile 的 resolve 必须 403，得到 %d：%s", rec.Code, rec.Body)
	}
}

func TestResolve_形状与错误分流(t *testing.T) {
	site := newStubSite(t, `<html><head><title>x</title></head></html>`, false)
	_, h := setupBatch(t)

	// tok 逐条给：require(credScoped) 跑在 Store.Current 之前，是本案唯一的
	// "凭证→档案"权威（P1-② 裁定）。档案不存在这条要真的走到读档那一步，
	// 就得用**作用域正好是它**的那枚令牌——拿 dadTok 申报 nobody 会是 403，
	// 而 `*` 令牌在 2026-10-10 之后根本到不了 credScoped（见 setupServer 的 ghost 令牌）。
	for _, tc := range []struct {
		name, body, tok string
		want            int
	}{
		{"非 http 协议", `{"url":"file:///etc/passwd","profile":"dad"}`, dadTok, http.StatusUnprocessableEntity},
		{"缺 url", `{"profile":"dad"}`, dadTok, http.StatusUnprocessableEntity},
		{"profile 不是 slug", `{"url":"` + site.srv.URL + `","profile":"../x"}`, dadTok, http.StatusUnprocessableEntity},
		{"未知字段", `{"url":"` + site.srv.URL + `","profile":"dad","by":"ai"}`, dadTok, http.StatusUnprocessableEntity},
		{"档案不存在", `{"url":"` + site.srv.URL + `","profile":"nobody"}`, nobodyTok, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doResolve(t, h, []byte(tc.body), tc.tok)
			if rec.Code != tc.want {
				t.Errorf("得到 %d，要 %d：%s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestResolve_目标连不上是502不是500(t *testing.T) {
	_, h := setupBatch(t)
	// 127.0.0.1:1 没人监听：连接立刻被拒。端口 1 是夹具用的保留位，
	// 不含任何本家拓扑信息（spec §11 的白名单形状）。
	rec := doResolve(t, h, resolveBody(t, "dad", "http://127.0.0.1:1/"), dadTok)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if d, _ := body["detail"].(string); d == "" || strings.Contains(d, "dial tcp") {
		t.Errorf("502 要说人话且不许漏 Go 内部错误: %q", d)
	}
}

func TestResolve_抓图失败只降级不整体失败(t *testing.T) {
	// 页面里有 icon link，但图片端点持续 404：resolve 仍要 200，favicon 为 null，
	// 并且该 origin 进了 24 小时冷却（冷却本身由
	// TestResolve_抓图失败后按origin冷却不再重抓 作证）。
	site := newStubSite(t, `<html><head><title>x</title>
		<link rel="icon" href="/nope.ico"></head></html>`, false)
	_, h := setupBatch(t)
	rec := doResolve(t, h, resolveBody(t, "dad", site.srv.URL), dadTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("抓图失败不该让整次 resolve 失败: %d %s", rec.Code, rec.Body)
	}
	var got resolveResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Favicon != nil {
		t.Error("没抓到图却给了路径")
	}
	if got.Title != "x" {
		t.Error("标题应当照旧返回")
	}
}

func TestResolve_resolve不写书签不写历史(t *testing.T) {
	// 夹具必须走真正 Put 成功落盘的那条通路（apple-touch-icon 声明 + 桩站真
	// 供图）：MarkFail 分支只写索引、不碰 blob，拿它当证人的话，真正往盘上
	// 放字节的那条路从没被这条反证人覆盖过。blob 落 data/icons/ 是允许的，
	// 档案目录（bookmarks/快照/历史）一律不许动。
	site := newStubSite(t, `<html><head><title>x</title>
		<link rel="apple-touch-icon" href="/apple-touch-icon.png"></head></html>`, true)
	s, h := setupBatch(t)
	before := revOf(t, s, "dad")
	_, filesBefore := snapshotCount(t, s, "dad")
	histBefore := historyLines(t, s, "dad")

	rec := doResolve(t, h, resolveBody(t, "dad", site.srv.URL), dadTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("夹具本身要走到 Put 成功（favicon 非 null）：%d %s", rec.Code, rec.Body)
	}
	var got resolveResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Favicon == nil {
		t.Fatal("这条反证人的前提是 blob 真的落了盘：favicon 不能为 null")
	}

	if after := revOf(t, s, "dad"); after != before {
		t.Errorf("resolve 动了 rev（%d→%d）：它不是书签写通路", before, after)
	}
	if _, after := snapshotCount(t, s, "dad"); after != filesBefore {
		t.Error("resolve 产生了快照，spec §5 没允许这件事")
	}
	if after := historyLines(t, s, "dad"); after != histBefore {
		t.Error("resolve 写了历史行：它只在 data/icons/ 落缓存")
	}
}

// 存盘失败也要进冷却（I-1）：抓到了图但 Put 不下去时，不只说一声，还要
// 像抓图失败一样按 origin 冷却 24 小时——否则每次 resolve 都白抓一遍图。
// 夹具：把 blob 的落点预先占成目录，原子 rename 撞目录必然失败
// （与 store/profile_test.go 的回滚夹具同一手法），而 index.json 照写不误，
// 所以冷却记得住。证人：清零计数后再来两次 resolve，图片端点必须一次都没被打到。
func TestResolve_存盘失败后按origin冷却不再重抓(t *testing.T) {
	site := newStubSite(t, `<html><head><title>P</title>
		<link rel="apple-touch-icon" href="/apple-touch-icon.png"></head></html>`, true)
	s, h := setupBatch(t)
	// blob 名 = 内容哈希，与 Put 同一算法算出来（8 个字节就是桩站那张图）。
	sum := sha256.Sum256([]byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4})
	blob := s.IconBlobPath(hex.EncodeToString(sum[:])[:16], "png")
	if err := os.MkdirAll(blob, 0o755); err != nil {
		t.Fatal(err)
	}
	body := resolveBody(t, "dad", site.srv.URL)
	rec := doResolve(t, h, body, dadTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("存盘失败不该让整次 resolve 失败: %d %s", rec.Code, rec.Body)
	}
	var got resolveResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Favicon != nil {
		t.Error("落盘点被占住了却还给出路径：Put 根本没失败？")
	}

	site.imgHits.Store(0)
	doResolve(t, h, body, dadTok)
	doResolve(t, h, body, dadTok)
	if n := site.imgHits.Load(); n != 0 {
		t.Errorf("Put 失败后该按 origin 冷却，两次 resolve 里又打了图片端点 %d 次（冷却没记上）", n)
	}
}

// ——以下三个纯函数表驱动单测不经 HTTP：实现里的注释承诺了"这两个函数要能
// 脱离 handler 单测"，这一节就是兑现。打分通道（score-1/2/3）、每 token
// limit=2、5 条上限与 IP 字面量处理，此前一条证人都没有。

func TestSuggestGroup_打分通道(t *testing.T) {
	b := func(items ...model.Item) *model.Bookmarks {
		return &model.Bookmarks{Groups: []model.Group{{
			ID: "nas", Title: "NAS", Items: items,
		}}}
	}
	item := func(url string) model.Item {
		return model.Item{ID: "x", Name: "x", URL: url, Alias: []string{}, Tags: []string{}}
	}
	cases := []struct {
		name           string
		bookmarks      *model.Bookmarks
		rawURL, title  string
		wantID, wantTi string // wantID 为空串表示期望 nil
	}{
		{
			// score-1：分组名只出现在 hay（整条 URL + 标题）里。
			name: "分组标题出现在标题里给1分",
			bookmarks: &model.Bookmarks{Groups: []model.Group{{
				ID: "media", Title: "影音", Items: []model.Item{item("http://10.9.9.9/")},
			}}},
			rawURL: "http://10.0.0.5/status", title: "Media 首页",
			wantID: "media", wantTi: "影音",
		},
		{
			// score-2：不同主机、同首标签。media 分组靠标题拿 1 分排在前面，
			// 赢的必须是 ops（2 分）——这一条同时是打分层级的证人。
			name: "同首标签不同主机给2分且压过1分",
			bookmarks: &model.Bookmarks{Groups: []model.Group{
				{ID: "media", Title: "影音", Items: []model.Item{item("http://10.9.9.9/")}},
				{ID: "ops", Title: "运维", Items: []model.Item{item("http://portainer.lan:9000/")}},
			}},
			rawURL: "http://portainer.example.net/stacks", title: "Media Panel",
			wantID: "ops", wantTi: "运维",
		},
		{
			// M-3 的正面证人（动机见修复清单原文"两个毫不相干的 192.x 主机
			// 可以靠首标签相同拿到 2 分"；夹具按本案纪律只用 127. 回环段）：
			// 两个不同 IP 主机不该靠首标签 "127" 拿到 2 分。
			// 修前红——今天 len("127")>2 放行首标签相等。
			name:      "不同IP主机不靠首标签拿2分",
			bookmarks: b(item("http://127.0.0.1:5000/")),
			rawURL:    "http://127.0.0.30/", title: "",
		},
		{
			name:      "10段IP首标签同样不得分",
			bookmarks: b(item("http://10.0.0.9:5000/")),
			rawURL:    "http://10.0.0.30/", title: "",
		},
		{
			// 去掉数字段后，内网同主机的建议能力还在：整主机相等（含 IP
			// 字面量）走的本来就是 score-3 通道，与首标签无关。
			name:      "IP字面量整主机相等仍拿3分",
			bookmarks: b(item("http://10.0.0.9:5000/")),
			rawURL:    "http://10.0.0.9:8096/", title: "",
			wantID: "nas", wantTi: "NAS",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := suggestGroup(tc.bookmarks, tc.rawURL, tc.title)
			if tc.wantID == "" {
				if got != nil {
					t.Errorf("不该有任何建议: %+v", got)
				}
				return
			}
			if got == nil || got.ID != tc.wantID || got.Title != tc.wantTi {
				t.Errorf("建议分组不对, 得到 %+v, 要 %s/%s", got, tc.wantID, tc.wantTi)
			}
		})
	}
}

func TestURLTokens_分段与过滤(t *testing.T) {
	cases := []struct {
		name          string
		rawURL, title string
		want          []string
	}{
		{
			// 内网主流形态是 IP 字面量：八位组不是"服务名"，不该产 token。
			// 修前红——今天 "127" 会进候选并吃掉一格名额。
			name: "IP字面量不产数字token", rawURL: "http://127.0.0.1:8080/", title: "", want: []string{},
		},
		{
			name: "10段IP同样不产", rawURL: "http://10.0.0.9/", title: "", want: []string{},
		},
		{
			name: "数字段滤掉字母段留下", rawURL: "http://10.127.0.9/admin", title: "", want: []string{"admin"},
		},
		{
			name: "路径里的纯数字段也滤", rawURL: "http://host.example/2024/report", title: "",
			want: []string{"host", "example", "report"},
		},
		{
			name: "含字母的段不受影响", rawURL: "http://ubuntu22/room1", title: "", want: []string{"ubuntu22", "room1"},
		},
		{
			name: "保留词段被滤掉", rawURL: "http://www.local/", title: "", want: []string{},
		},
		{
			name: "标题里的纯数字词滤掉", rawURL: "http://nas/room1", title: "Ubuntu 22",
			want: []string{"nas", "room1", "ubuntu"},
		},
		{
			// 顺序证人：主机段 → 路径段 → 标题词（iconCandidates 的优先级
			// 全靠它，标题词排后面是因为主机更接近"这是什么服务"）。
			name: "主机先于路径先于标题", rawURL: "http://docker.local/stacks", title: "Portainer",
			want: []string{"docker", "stacks", "portainer"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := urlTokens(tc.rawURL, tc.title)
			if !slices.Equal(got, tc.want) {
				t.Errorf("token 不对: got=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestIconCandidates_真索引打分与上限(t *testing.T) {
	// 用真索引而不是 stub：候选"来自索引里的规范名"这件事，stub 索引证不了。
	ix := testIndex()
	// 探针实测（数字取自修复轮 1 的索引探针，见 task-8-report.md「修复轮 1」
	// 第 2 节）：box 在 simple-icons 层
	// [simple-icons:box simple-icons:apachepdfbox]、tabler 层
	// [tabler:box tabler:box-align-bottom]——每层 Search 的 limit=2 就钉在这里。
	boxFour := []string{"simple-icons:box", "simple-icons:apachepdfbox",
		"tabler:box", "tabler:box-align-bottom"}
	t.Run("单token每层limit2生效", func(t *testing.T) {
		got := iconCandidates(ix, "http://box", "")
		if !slices.Equal(got, boxFour) {
			t.Errorf("单 token 该恰好每层 2 条共 4 条: %q", got)
		}
	})
	t.Run("超过5条截断到5", func(t *testing.T) {
		// box(4 条) + grid 的前 1 条 = 6 条 → 截到 5。上限放宽到 10 这条就红。
		got := iconCandidates(ix, "http://box.grid/chart", "")
		want := append(append([]string{}, boxFour...), "simple-icons:griddotai")
		if !slices.Equal(got, want) {
			t.Errorf("该恰好 5 条且按 token 优先级排列: got=%q want=%q", got, want)
		}
		if again := iconCandidates(ix, "http://box.grid/chart", ""); !slices.Equal(again, got) {
			t.Errorf("同一输入两次结果顺序不稳: %q vs %q", got, again)
		}
	})
	t.Run("IP字面量不吃候选名额", func(t *testing.T) {
		// 修前 "127" 占掉 urlTokens 的一个 token 位（上限 4），IP+长路径
		// 时把真正的服务名挤出局；过滤后 box 的候选与单 token 情形一致。
		got := iconCandidates(ix, "http://127.0.0.1/box", "")
		if !slices.Equal(got, boxFour) {
			t.Errorf("IP 段不该参与候选: %q", got)
		}
	})
}
