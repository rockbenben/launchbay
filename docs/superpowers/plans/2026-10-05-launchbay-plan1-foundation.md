# launchbay P1-① 地基 实施计划

> **本文是历史实施记录，不是待办。** 它记录的是那一天计划或量的是什么，里面的
> 函数名、测试名与接口形状**可能已经变了**——2026-10-10 的管理员边界收紧一次性改判掉了
> 代管态、模板机制与徽标切档，共享清单也改判为不做。**不要照着这里的命令执行**：
> 权威契约在仓库根的 `AGENTS.md`，设计在 `docs/superpowers/specs/`。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 launchbay 在局域网里跑起来：浏览器打开 `:7600` → 档案选择页 → 输 PIN 进入某档案 → 看到该档案服务端渲染的书签列表，且两个档案永不串号。

**Architecture:** 一个 Go 单二进制同时做 HTTP 服务与渲染。数据是磁盘上的纯 JSON（每档案一个目录）。渲染结果按 `(profileId, rev, mtime)` 在内存缓存，ETag 带档案名。本期前端只有服务端渲染的语义化 HTML + 一份朴素 CSS；极光玻璃、图标层、离线缓存、ops 写通路在后续计划里。

**Tech Stack:** Go 1.27（`CGO_ENABLED=0`）、标准库 `html/template` + `encoding/json` + `net/http`、Docker 多阶段构建、`go test`。

**Spec:** `docs/superpowers/specs/2026-10-05-launchbay-design.md` —— 本计划只覆盖 spec 的一部分，缺口与去向见文末「Spec 覆盖对账」。**执行时 spec 与本案一起读。**

## Global Constraints

每个任务都隐含遵守本节，值取自 spec 原文。

- 端口固定 **7600**（7570 是 flare，7581 是 Stirling PDF）。
- Go 模块 `github.com/rockbenben/launchbay`，`go 1.27`。
- `CGO_ENABLED=0`；非 root 运行；镜像目标 **≤ 20MB**。
- 不引入前端框架、不引入数据库、不引入 ORM。数据只有 JSON 文本文件。
- **不加严格安全机制**：不签 cookie、不做 PIN 限速/退避/锁定、不用 argon2/bcrypt、不要会话密钥。spec §11 的「明写不防」清单是验收项——多出来的防护也算失败。
- JSON 写出契约：2 空格缩进、`SetEscapeHTML(false)`、非 ASCII 原样、末尾一个换行、空数组写 `[]`、键顺序由 struct 字段顺序决定、**禁止用 map 产生输出键序**。
- 寻址只用稳定 `id`，**不用数组下标**；`id` 是 ASCII slug。item 的 `id` 在**单档案内全局唯一**（跨分组也不复用）。
- 文档、夹具、测试数据里的地址**只允许 `10.` 段**。真实 NAS 网段一个字面量都不许进仓库。
- 界面文案中文；快捷键文案 `⌘/Ctrl` 双轨。
- 数据目录 `./data`（容器内 `/data`），整目录进 `.gitignore`。
- 本机 Windows + Git Bash：新终端才吃得到 `C:\Program Files\Go\bin`；未重开的会话里先 `export PATH="$PATH:/c/Program Files/Go/bin"`。
- 每个任务收尾都跑同一组门禁：`go test ./... && go vet ./... && gofmt -l .`（gofmt **必须无输出**）。

## 前置事实（已实测，不要重复调研）

- Go 已装：`go version go1.27.0 windows/amd64`，`GOROOT=C:\Program Files\Go`，`CGO_ENABLED` 默认 0。
- 临时模块里 `go test ./...` 实测通过（`ok smoke 0.724s`）。
- 机器级 PATH 已含 `C:\Program Files\Go\bin`。
- Docker CLI 28.5.1 已装，**引擎未启动**；Task 12 需要它，启动方式见该任务。
- Node 24.18.0 / esbuild 0.28.2 可用（本期用不到，后续计划的打包器）。

---

### Task 1: 仓库地基与 `/healthz`

**Files:**
- Create: `.gitattributes`
- Create: `go.mod`
- Create: `cmd/launchbay/main.go`
- Create: `internal/httpapi/server.go`
- Test: `internal/httpapi/server_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `httpapi.New() *httpapi.Server`；`(*Server).Handler() http.Handler`；env `LAUNCHBAY_PORT`（默认 `7600`）

- [ ] **Step 1: 先钉行尾，再写第一份源码**

`.gitattributes`：

```gitattributes
* text=auto eol=lf
*.png binary
*.webp binary
*.ico binary
```

```bash
git config core.autocrlf false
git config core.eol lf
```

不做这一步，Windows 上写出的 Go/JS/JSON 会带 CRLF，Linux CI 与本地表现不一致，事后修要重打整仓。

- [ ] **Step 2: 初始化模块**

```bash
go mod init github.com/rockbenben/launchbay
```

确认 `go.mod` 里是 `go 1.27`。

- [ ] **Step 3: 写失败的测试**

`internal/httpapi/server_test.go`：

```go
package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestHealthzServesOK(t *testing.T) {
	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("healthz code = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "ok\n" {
		t.Fatalf("healthz body = %q, want %q", got, "ok\n")
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 4: 跑到它失败**

Run: `go test ./internal/httpapi/ -v`
Expected: 编译失败 `undefined: New`

- [ ] **Step 5: 最小实现**

`internal/httpapi/server.go`：

```go
package httpapi

import "net/http"

// Server 是所有 handler 的装配点。Task 9 会把 New() 换成 New(Dependencies)，
// 届时同步改 cmd/launchbay/main.go 与本包测试的调用处。
type Server struct{ mux *http.ServeMux }

func New() *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	return &Server{mux: mux}
}

func (s *Server) Handler() http.Handler { return s.mux }
```

`cmd/launchbay/main.go`：

```go
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/rockbenben/launchbay/internal/httpapi"
)

func main() {
	port := os.Getenv("LAUNCHBAY_PORT")
	if port == "" {
		port = "7600"
	}
	srv := &http.Server{Addr: ":" + port, Handler: httpapi.New().Handler()}
	log.Printf("launchbay listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}
```

- [ ] **Step 6: 跑到通过**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: `ok`；vet 无输出；gofmt 无输出（有输出就 `gofmt -w .`）

- [ ] **Step 7: 提交**

```bash
git add .gitattributes go.mod cmd internal
git commit -m "feat(boot): go 模块地基与 /healthz

行尾在进第一份源码前用 .gitattributes 钉成 lf。路由集中在 httpapi.New
装配，后续任务的 handler 都从这一处挂上去。"
```

---

### Task 2: JSON 写出契约

`encoding/json` 的默认行为会咬这个项目：`SetEscapeHTML` 默认为真，标题里的 `&` 被写成 `\u0026`，之后每次写出都污染人和 AI 看的 diff。先把它钉成一个函数。

**Files:**
- Create: `internal/store/encode.go`
- Test: `internal/store/encode_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `store.WriteJSON(w io.Writer, v any) error`；`store.MarshalJSONBytes(v any) ([]byte, error)`

- [ ] **Step 1: 写失败的测试**

`internal/store/encode_test.go`：

```go
package store

import (
	"bytes"
	"strings"
	"testing"
)

type payload struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func encode(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, v); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	return buf.String()
}

func TestKeepsChineseAndHTMLEntitiesLiteral(t *testing.T) {
	got := encode(t, payload{Name: "番茄 & <b>加蛋</b> 面", Items: []string{"a"}})
	if strings.Contains(got, `\u`) {
		t.Fatalf("出现 \\u 转义:\n%s", got)
	}
	for _, want := range []string{"&", "<b>加蛋</b>", "面"} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺字面量 %q:\n%s", want, got)
		}
	}
}

func TestTwoSpaceIndentOneTrailingNewline(t *testing.T) {
	want := "{\n  \"name\": \"x\",\n  \"items\": []\n}\n"
	if got := encode(t, payload{Name: "x", Items: []string{}}); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestEmptySliceIsNotNull(t *testing.T) {
	if got := encode(t, payload{Name: "x", Items: []string{}}); strings.Contains(got, "null") {
		t.Fatalf("空数组写成 null:\n%s", got)
	}
}

func TestByteStableAcrossCalls(t *testing.T) {
	p := payload{Name: "稳定", Items: []string{"a", "b"}}
	if a, b := encode(t, p), encode(t, p); a != b {
		t.Fatalf("同内容两次写出不一致:\n%s\n---\n%s", a, b)
	}
}
```

- [ ] **Step 2: 跑到它失败**

Run: `go test ./internal/store/ -v`
Expected: 编译失败 `undefined: WriteJSON`

- [ ] **Step 3: 最小实现**

`internal/store/encode.go`：

```go
package store

import (
	"bytes"
	"encoding/json"
	"io"
)

// WriteJSON 按 spec §4.3 的写出契约编码：2 空格缩进、不转义 <>&、
// 非 ASCII 原样、末尾恰好一个换行。
//
// 键顺序由 struct 字段声明顺序决定。任何会落盘的类型都不许用
// map[string]X：那样"加一个键"就成了结构变更，而这里要的是人和
// AI 写完都能稳定 diff 的固定形状。
func WriteJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Encoder 自带末尾换行；TrimRight 再补一个是为了让"恰好一个"
	// 成为可断言的规则，而不是依赖实现细节。
	out := append(bytes.TrimRight(buf.Bytes(), "\n"), '\n')
	_, err := w.Write(out)
	return err
}

// MarshalJSONBytes 是 WriteJSON 的内存版，供"先校验再落盘"的路径与测试使用。
func MarshalJSONBytes(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 4: 跑到通过**

Run: `go test ./internal/store/ -v`
Expected: 四个测试 PASS

- [ ] **Step 5: 故意破坏，确认证人因对的理由变红**

注释掉 `enc.SetEscapeHTML(false)`：

Run: `go test ./internal/store/ -run TestKeepsChinese`
Expected: FAIL（出现 `\u0026`）

恢复。再把 `SetIndent("", "  ")` 改成 `SetIndent("", "\t")`：

Run: `go test ./internal/store/ -run TestTwoSpaceIndent`
Expected: FAIL

恢复后：`go test ./internal/store/ && gofmt -l .` → 全绿且 gofmt 无输出。

这一步不是仪式：只断言"输出里有 `&`"的测试会在无关改动下假通过，必须确认它是因为转义才红的。

- [ ] **Step 6: 提交**

```bash
git add internal/store
git commit -m "feat(store): JSON 写出契约集中到 WriteJSON

关掉 SetEscapeHTML 并在测试里故意破坏一次——标题里的 & 一旦写成
\\u0026，人和 AI 的每次整理都会把 diff 搞成整片花屏。"
```

---

### Task 3: 数据模型与校验（报错要点出位置）

**Files:**
- Create: `internal/model/model.go`
- Create: `internal/model/check.go`
- Test: `internal/model/model_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `model.Profile{ID,Title string; Admin bool; PIN,CreatedAt,Theme,Layout string}`
  - `model.Bookmarks{Rev int64; Title string; Groups []Group; Search Search; Weather Weather; OpenInNewTab, Clock bool; Footer string}`
  - `model.Group{ID,Title,Layout,Accent string; Items []Item}`、`model.Item{ID,Name,URL,Icon,Desc string; Alias,Tags []string; AINote string}`
  - `model.Search`、`model.SearchEngine`、`model.Weather`
  - 常量 `model.LayoutTiles = "tiles"`、`model.LayoutList = "list"`、`model.ScopeAny`（不属于 model，见 Task 6）
  - `(*Bookmarks).Validate() error`、`(*Profile).Validate() error`、`model.ValidSlug(string) bool`、`model.ErrValidation`

- [ ] **Step 1: 写失败的测试**

`internal/model/model_test.go`：

```go
package model

import (
	"strings"
	"testing"
)

func goodBookmarks() *Bookmarks {
	return &Bookmarks{
		Title: "发射台",
		Groups: []Group{{ID: "nas", Title: "NAS 应用", Layout: LayoutList, Items: []Item{
			{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000",
				Icon: "simple-icons:synology", Alias: []string{"nas"}, Tags: []string{}},
		}}},
		Search:  Search{Enabled: true, Default: "g", Engines: []SearchEngine{{Prefix: "g", Name: "Google", URL: "https://www.google.com/search?q=%s"}}},
		Weather: Weather{Enabled: false, Location: "上海市"},
	}
}

func TestGoodDataPasses(t *testing.T) {
	if err := goodBookmarks().Validate(); err != nil {
		t.Fatalf("合法数据被拒: %v", err)
	}
}

func TestRejects(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*Bookmarks)
		wantInErr string
	}{
		{"非 slug 分组 id", func(b *Bookmarks) { b.Groups[0].ID = "NAS 应用" }, "NAS 应用"},
		{"重复分组 id", func(b *Bookmarks) { g := b.Groups[0]; b.Groups = append(b.Groups, g) }, "重复"},
		{"非法 layout", func(b *Bookmarks) { b.Groups[0].Layout = "grid" }, "layout"},
		{"同组 item id 重复", func(b *Bookmarks) {
			it := b.Groups[0].Items[0]
			b.Groups[0].Items = append(b.Groups[0].Items, it)
		}, "重复"},
		{"跨分组 item id 重复", func(b *Bookmarks) {
			b.Groups = append(b.Groups, Group{ID: "dev", Title: "开发", Layout: LayoutList,
				Items: []Item{b.Groups[0].Items[0]}})
		}, "重复"},
		{"非 http/https 协议", func(b *Bookmarks) { b.Groups[0].Items[0].URL = "ftp://10.0.0.3/" }, "协议"},
		{"空 URL", func(b *Bookmarks) { b.Groups[0].Items[0].URL = "" }, "url"},
		{"缺 name", func(b *Bookmarks) { b.Groups[0].Items[0].Name = "" }, "name"},
		{"缺 group title", func(b *Bookmarks) { b.Groups[0].Title = " " }, "title"},
		{"运行时不许 MDI 图标名", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "mdi:download" }, "icon"},
		{"未知图标前缀", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "fa6-brands:github" }, "icon"},
		{"图标缺前缀", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "github" }, "icon"},
		{"搜索引擎 url 缺 %s", func(b *Bookmarks) { b.Search.Engines[0].URL = "https://x" }, "engines"},
		{"search default 悬空", func(b *Bookmarks) { b.Search.Default = "dd" }, "default"},
		{"nil items 合法", func(b *Bookmarks) { b.Groups[0].Items = nil }, ""},
		{"icon 为空合法（走首字母）", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "" }, ""},
		{"icon=auto 合法", func(b *Bookmarks) { b.Groups[0].Items[0].Icon = "auto" }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := goodBookmarks()
			tc.mutate(b)
			err := b.Validate()
			if tc.wantInErr == "" {
				if err != nil {
					t.Fatalf("want accept, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantInErr)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Fatalf("error %q 不含 %q", err, tc.wantInErr)
			}
		})
	}
}

// 批量整理时，报错没有位置就等于让 AI 猜哪条错了。
func TestErrorNamesLocation(t *testing.T) {
	b := goodBookmarks()
	b.Groups[0].Items[0].URL = "telnet://x"
	err := b.Validate()
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"nas", "dsm"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("报错必须点出分组与条目，缺 %q: %v", want, err)
		}
	}
}

func TestProfileValidate(t *testing.T) {
	base := Profile{ID: "dad", Title: "爸爸", PIN: "sha256$a$b", Theme: "auto", Layout: "sidebar"}
	if err := base.Validate(); err != nil {
		t.Fatalf("合法 profile 被拒: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*Profile)
	}{
		{"大写 id", func(p *Profile) { p.ID = "Dad" }},
		{"空 title", func(p *Profile) { p.Title = "" }},
		{"空 pin", func(p *Profile) { p.PIN = "" }},
		{"非法 theme", func(p *Profile) { p.Theme = "pink" }},
		{"非法 layout", func(p *Profile) { p.Layout = "topbar2" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mut(&p)
			if err := p.Validate(); err == nil {
				t.Fatalf("want reject: %+v", p)
			}
		})
	}
}
```

- [ ] **Step 2: 跑到它失败**

Run: `go test ./internal/model/ -v`
Expected: 编译失败 `undefined: Bookmarks`

- [ ] **Step 3: 写类型**

`internal/model/model.go`：

```go
// Package model 是落在磁盘上的数据结构。字段顺序就是 JSON 的键顺序，
// 改动顺序会让人/rebase 看到无关的整片 diff——要改就在同一处
// 把写出契约的测试一起改。
package model

const (
	LayoutTiles = "tiles"
	LayoutList  = "list"
)

// Item 是一条书签。数组顺序即显示顺序，所以没有 order 字段：
// 两个真相迟早会打架（spec §4.3）。
type Item struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Icon   string   `json:"icon,omitempty"`
	Desc   string   `json:"desc,omitempty"`
	Alias  []string `json:"alias"`
	Tags   []string `json:"tags"`
	AINote string   `json:"aiNote,omitempty"`
}

type Group struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Layout string `json:"layout"`
	Accent string `json:"accent,omitempty"`
	Items  []Item `json:"items"`
}

type SearchEngine struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
	URL    string `json:"url"`
}

type Search struct {
	Enabled bool           `json:"enabled"`
	Default string         `json:"default"`
	Engines []SearchEngine `json:"engines"`
}

type Weather struct {
	Enabled  bool   `json:"enabled"`
	Location string `json:"location"`
}

type Bookmarks struct {
	Rev          int64   `json:"rev"`
	Title        string  `json:"title"`
	Groups       []Group `json:"groups"`
	Search       Search  `json:"search"`
	Weather      Weather `json:"weather"`
	OpenInNewTab bool    `json:"openInNewTab"`
	Clock        bool    `json:"clock"`
	Footer       string  `json:"footer"`
}

type Profile struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Admin     bool   `json:"admin"`
	PIN       string `json:"pin"`
	CreatedAt string `json:"createdAt"`
	Theme     string `json:"theme"`
	Layout    string `json:"layout"`
}
```

- [ ] **Step 4: 写校验**

`internal/model/check.go`：

```go
package model

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ErrValidation 让调用方能区分"形状合法但内容不合法"与"文件读不了"。
var ErrValidation = errors.New("数据不合法")

var (
	slugRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	iconName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// 运行时只带这两套（spec §5）。MDI 只在导入器里当别名源，不进运行时。
var allowedIconPrefixes = map[string]bool{"tabler": true, "simple-icons": true}
var allowedTheme = map[string]bool{"auto": true, "dark": true, "light": true}
var allowedLayout = map[string]bool{"sidebar": true, "topbar": true}

func ValidSlug(s string) bool { return slugRE.MatchString(s) }

func checkURL(u string) error {
	if u == "" {
		return errors.New("url 为空")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("url 解析失败 %q: %w", u, err)
	}
	switch parsed.Scheme {
	case "http", "https":
		if parsed.Host == "" {
			return fmt.Errorf("url 缺主机名 %q", u)
		}
		return nil
	case "":
		return fmt.Errorf("url 缺协议 %q", u)
	default:
		return fmt.Errorf("协议 %q 不在白名单 http/https", parsed.Scheme)
	}
}

// checkIcon 只验形状与前缀。图标是否真的存在归图标索引（P1-③）。
func checkIcon(icon string) error {
	if icon == "" || icon == "auto" {
		return nil
	}
	prefix, name, ok := strings.Cut(icon, ":")
	if !ok {
		return fmt.Errorf("icon %q 要写成 <prefix>:<name>", icon)
	}
	if !allowedIconPrefixes[prefix] {
		return fmt.Errorf("icon 前缀 %q 不在运行时双层内（tabler / simple-icons）", prefix)
	}
	if !iconName.MatchString(name) {
		return fmt.Errorf("icon 名 %q 形状非法", name)
	}
	return nil
}

func (b *Bookmarks) Validate() error {
	seenGroup := map[string]bool{}
	seenItem := map[string]bool{}
	for gi := range b.Groups {
		g := &b.Groups[gi]
		if !ValidSlug(g.ID) {
			return fmt.Errorf("%w: group %q: id 不是 ASCII slug", ErrValidation, g.ID)
		}
		if seenGroup[g.ID] {
			return fmt.Errorf("%w: group id 重复: %q", ErrValidation, g.ID)
		}
		seenGroup[g.ID] = true
		if strings.TrimSpace(g.Title) == "" {
			return fmt.Errorf("%w: group %q: title 为空", ErrValidation, g.ID)
		}
		if g.Layout != LayoutTiles && g.Layout != LayoutList {
			return fmt.Errorf("%w: group %q: layout %q 只能是 tiles/list", ErrValidation, g.ID, g.Layout)
		}
		for ii := range g.Items {
			it := &g.Items[ii]
			if !ValidSlug(it.ID) {
				return fmt.Errorf("%w: group %q item %d: id %q 不是 ASCII slug", ErrValidation, g.ID, ii, it.ID)
			}
			// item id 在整个档案内唯一，不只组内：move 之后按 id 寻址
			// 才有唯一解（spec §4.4 的寻址前提）。
			if seenItem[it.ID] {
				return fmt.Errorf("%w: item id 在档案内重复: %q", ErrValidation, it.ID)
			}
			seenItem[it.ID] = true
			if strings.TrimSpace(it.Name) == "" {
				return fmt.Errorf("%w: group %q/item %q: name 为空", ErrValidation, g.ID, it.ID)
			}
			if err := checkURL(it.URL); err != nil {
				return fmt.Errorf("%w: group %q/item %q: %v", ErrValidation, g.ID, it.ID, err)
			}
			if err := checkIcon(it.Icon); err != nil {
				return fmt.Errorf("%w: group %q/item %q: %v", ErrValidation, g.ID, it.ID, err)
			}
		}
	}
	for _, e := range b.Search.Engines {
		if e.Prefix == "" || !strings.Contains(e.URL, "%s") {
			return fmt.Errorf("%w: search engines %q: prefix 必填且 url 要含 %%s", ErrValidation, e.Name)
		}
	}
	if b.Search.Enabled && b.Search.Default != "" {
		found := false
		for _, e := range b.Search.Engines {
			if e.Prefix == b.Search.Default {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: search default %q 不在 engines 里", ErrValidation, b.Search.Default)
		}
	}
	return nil
}

func (p *Profile) Validate() error {
	if !ValidSlug(p.ID) {
		return fmt.Errorf("%w: profile id %q 不是 ASCII slug", ErrValidation, p.ID)
	}
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("%w: profile %q: title 为空", ErrValidation, p.ID)
	}
	if p.PIN == "" {
		return fmt.Errorf("%w: profile %q: pin 哈希为空", ErrValidation, p.ID)
	}
	if !allowedTheme[p.Theme] {
		return fmt.Errorf("%w: profile %q: theme %q 只能是 auto/dark/light", ErrValidation, p.ID, p.Theme)
	}
	if !allowedLayout[p.Layout] {
		return fmt.Errorf("%w: profile %q: layout %q 只能是 sidebar/topbar", ErrValidation, p.ID, p.Layout)
	}
	return nil
}
```

- [ ] **Step 5: 跑到通过**

Run: `go test ./internal/model/ -v`
Expected: 全 PASS（`TestRejects` 的 17 个子测试都要过）

- [ ] **Step 6: 破坏验证**

把 item id 的查重从「档案内」改回「组内」（`seenItem` 移进组循环里初始化）：

Run: `go test ./internal/model/ -run TestRejects/跨分组`
Expected: FAIL —— 跨分组重复 id 被放行，`move` 之后寻址会歧义

恢复。再把 `checkIcon` 里 `allowedIconPrefixes` 加上 `"mdi"`：

Run: `go test ./internal/model/ -run TestRejects/运行时不许`
Expected: FAIL

恢复。跑总门禁。

- [ ] **Step 7: 提交**

```bash
git add internal/model
git commit -m "feat(model): 书签与档案类型，校验把位置写进报错

item id 在档案内唯一而不是每组唯一：move 之后按 id 寻址才有唯一解。
图标前缀只认 tabler/simple-icons，MDI 只在导入器里当别名源。
每条报错都带分组与条目 id，否则批量整理等于让 AI 猜哪条错了。"
```

---

### Task 4: 按档案读取，坏档案只坏它自己

**Files:**
- Create: `internal/store/paths.go`
- Create: `internal/store/broken.go`
- Create: `internal/store/load.go`
- Test: `internal/store/load_test.go`
- Testdata: 由测试用 `t.TempDir()` 现造，**不在仓库里放真实结构**

**Interfaces:**
- Consumes: `model`（Task 3）
- Produces:
  - `store.New(root string) *store.Store`
  - `(*Store).Root() string`、`ProfileDir(id)`、`ProfilePath(id)`、`BookmarksPath(id)`、`TemplatePath(name)`、`SnapshotsDir(id)`、`HistoryPath()`、`profilesDir()`
  - `(*Store).ListProfiles() ([]*model.Profile, error)`
  - `(*Store).LoadProfile(id string) (*model.Profile, error)`
  - `(*Store).LoadBookmarks(id string) (*model.Bookmarks, error)`
  - `type store.Snapshot struct { B *model.Bookmarks; Rev int64; Modified time.Time }`
  - `(*Store).Current(id string) (Snapshot, error)`
  - `(*Store).Broken() []string`、`(*Store).BrokenReason(id string) (string, bool)`
  - 哨兵错误 `store.ErrNotFound`、`store.ErrExists`、`store.ErrInvalid`、`store.ErrCorrupt`
  - `store.DecodeBookmarks(path string, data []byte) (*model.Bookmarks, error)`（Task 7 的模板读取要用）

- [ ] **Step 1: 写失败的测试**

`internal/store/load_test.go`：

```go
package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

const dadBookmarks = `{"rev":3,"title":"爸爸","groups":[{"id":"nas","title":"NAS 应用","layout":"list","items":[{"id":"dsm","name":"群晖 DSM","url":"http://10.0.0.3:5000","alias":[],"tags":[]}]}],"search":{"enabled":false,"default":"","engines":[]},"weather":{"enabled":false,"location":"上海市"},"openInNewTab":false,"clock":false,"footer":""}`

const dadProfile = `{"id":"dad","title":"爸爸","admin":true,"pin":"sha256$a$b","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar"}`

const kidProfile = `{"id":"kid","title":"小满","admin":false,"pin":"sha256$c$d","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar"}`

func put(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, "profiles/dad/profile.json", dadProfile)
	put(t, root, "profiles/dad/bookmarks.json", dadBookmarks)
	put(t, root, "profiles/kid/profile.json", kidProfile) // 故意没有 bookmarks.json
	return root
}

func TestListAndLoad(t *testing.T) {
	s := New(fixtureRoot(t))
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("want 2, got %d", len(profiles))
	}
	if profiles[0].ID != "dad" || profiles[1].ID != "kid" {
		t.Fatalf("ListProfiles 要按 id 排序: %+v", profiles)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("LoadBookmarks: %v", err)
	}
	if b.Rev != 3 || b.Groups[0].Items[0].ID != "dsm" {
		t.Fatalf("got %+v", b)
	}
	if len(s.Broken()) != 0 {
		t.Fatalf("干净夹具不该有坏档: %v", s.Broken())
	}
}

func TestMissingProfileIsErrNotFound(t *testing.T) {
	_, err := New(fixtureRoot(t)).LoadBookmarks("ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// 刚建好、还没写过书签 = 空档案，不是坏档案。
func TestProfileWithoutBookmarksIsEmptyNotBroken(t *testing.T) {
	s := New(fixtureRoot(t))
	b, err := s.LoadBookmarks("kid")
	if err != nil {
		t.Fatalf("want empty, got %v", err)
	}
	if b.Rev != 0 || len(b.Groups) != 0 {
		t.Fatalf("want zero, got %+v", b)
	}
	if b.Groups == nil {
		t.Fatal("空档案的 groups 要能安全 range 且后续写出为 []")
	}
	if len(s.Broken()) != 0 {
		t.Fatalf("want no broken, got %v", s.Broken())
	}
}

// 一个人的手误不该让全家开不了导航页。
func TestCorruptBookmarksBreakOnlyThatProfile(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json", `{"rev":3,"groups":`)
	s := New(root)

	if _, err := s.LoadBookmarks("dad"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("坏档案不该带走别人: %d", len(profiles))
	}
	if got := s.Broken(); len(got) != 1 || got[0] != "dad" {
		t.Fatalf("Broken() = %v want [dad]", got)
	}
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatalf("kid 应照常可用: %v", err)
	}
}

// 报错要给行号：改文件的人是打开文件改的，不是打开字节偏移量改的。
func TestCorruptErrorNamesFileAndLine(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json", "{\n  \"rev\": 3,\n  \"groups\": [\n    { \"id\": \"nas\" }\n  ],\n")
	_, err := New(root).LoadBookmarks("dad")
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bookmarks.json") {
		t.Fatalf("要点出文件名: %s", msg)
	}
	if !strings.Contains(msg, "第 6 行") {
		t.Fatalf("要点出正确行号（截断发生在第 6 行后）: %s", msg)
	}
}

// 拼错的键必须报错：静默忽略会让人以为改动生效了。
func TestUnknownKeyIsRejected(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json", strings.Replace(dadBookmarks, `"groups"`, `"groupz"`, 1))
	_, err := New(root).LoadBookmarks("dad")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	if !strings.Contains(err.Error(), "groupz") {
		t.Fatalf("要点出未知键名: %v", err)
	}
}

// profile.json 里的 id 与目录名不一致是真实事故（手复制目录留下的）。
func TestProfileIDMustMatchDir(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/kid/profile.json", kidProfile) // id=kid 一致，先确认正常
	put(t, root, "profiles/mom/profile.json", kidProfile) // 内容还是 id=kid
	_, err := New(root).LoadProfile("mom")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt for id mismatch, got %v", err)
	}
	if !strings.Contains(err.Error(), "目录名") {
		t.Fatalf("报错要说明是目录名不一致: %v", err)
	}
}

func TestValidateRunsOnLoad(t *testing.T) {
	root := fixtureRoot(t)
	put(t, root, "profiles/dad/bookmarks.json",
		strings.Replace(dadBookmarks, `"layout":"list"`, `"layout":"grid"`, 1))
	_, err := New(root).LoadBookmarks("dad")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt wrapping validation, got %v", err)
	}
	if !errors.Is(err, model.ErrValidation) {
		t.Fatalf("要同时保住 model.ErrValidation 供上层分流, got %v", err)
	}
}

func TestCurrentCarriesMtimeForCacheKey(t *testing.T) {
	s := New(fixtureRoot(t))
	snap, err := s.Current("dad")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Rev != 3 || snap.Modified.IsZero() {
		t.Fatalf("Snapshot 要带 rev 与 mtime: %+v", snap)
	}
}

// 手工 mkdir 出来的空目录不该被报成"某某档案读不了"。
func TestStrayEmptyDirIsSkippedNotBroken(t *testing.T) {
	root := fixtureRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "profiles", "stray"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New(root)
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("want 2, got %d", len(profiles))
	}
	if got := s.Broken(); len(got) != 0 {
		t.Fatalf("空目录不该进 Broken(): %v", got)
	}
}

func TestEmptyDataDirListsNothing(t *testing.T) {
	s := New(t.TempDir())
	p, err := s.ListProfiles()
	if err != nil || len(p) != 0 {
		t.Fatalf("want empty ok, got %v %v", p, err)
	}
}
```

- [ ] **Step 2: 跑到它失败**

Run: `go test ./internal/store/ -v`
Expected: 编译失败 `undefined: New`

- [ ] **Step 3: 路径与哨兵错误**

`internal/store/paths.go`：

```go
package store

import (
	"errors"
	"path/filepath"
	"sync"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("exists")
	ErrInvalid  = errors.New("invalid")
	ErrCorrupt  = errors.New("数据文件读不了")
)

type Store struct {
	root string

	bmu    sync.Mutex
	broken map[string]string

	lmu   sync.Mutex
	locks map[string]*sync.Mutex
}

func New(root string) *Store { return &Store{root: root} }

func (s *Store) Root() string { return s.root }

func (s *Store) profilesDir() string { return filepath.Join(s.root, "profiles") }

func (s *Store) ProfileDir(id string) string { return filepath.Join(s.profilesDir(), id) }

func (s *Store) ProfilePath(id string) string {
	return filepath.Join(s.ProfileDir(id), "profile.json")
}

func (s *Store) BookmarksPath(id string) string {
	return filepath.Join(s.ProfileDir(id), "bookmarks.json")
}

func (s *Store) TemplatePath(name string) string {
	return filepath.Join(s.root, "templates", name+".json")
}

func (s *Store) SnapshotsDir(id string) string {
	return filepath.Join(s.ProfileDir(id), "snapshots")
}

func (s *Store) HistoryPath() string { return filepath.Join(s.root, "history.jsonl") }
```

`internal/store/broken.go`：

```go
package store

import (
	"sort"
)

func (s *Store) markBroken(id string, reason string) {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	if s.broken == nil {
		s.broken = map[string]string{}
	}
	s.broken[id] = reason
}

// Broken 返回读失败的档案 id（排序后）。刚建好还没写过的空档案不在其中。
func (s *Store) Broken() []string {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	out := make([]string, 0, len(s.broken))
	for id := range s.broken {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *Store) BrokenReason(id string) (string, bool) {
	s.bmu.Lock()
	defer s.bmu.Unlock()
	r, ok := s.broken[id]
	return r, ok
}
```

- [ ] **Step 4: 读取实现**

`internal/store/load.go`：

```go
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
)

// Snapshot 是渲染所需的最小信息。Modified 进缓存键，这样"外部手改了
// 基线文件但 rev 没变"也会失效（spec §7.1 的一致性要求）。
type Snapshot struct {
	B        *model.Bookmarks
	Rev      int64
	Modified time.Time
}

func (s *Store) ListProfiles() ([]*model.Profile, error) {
	ents, err := os.ReadDir(s.profilesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("列档案失败: %w", err)
	}
	out := []*model.Profile{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		p, err := s.LoadProfile(e.Name())
		if errors.Is(err, ErrNotFound) {
			// 有目录但没 profile.json：这是"半件事"，不是坏数据。
			// 跳过但不计入 Broken()——否则一个手滑 mkdir 出来的空目录
			// 会在家用页面上挂出一条"某某档案读不了"的红字。
			continue
		}
		if err != nil {
			s.markBroken(e.Name(), err.Error())
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) LoadProfile(id string) (*model.Profile, error) {
	path := s.ProfilePath(id)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: profile %q", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("读 %s: %w", path, err)
	}
	p, err := decodeProfile(path, data)
	if err != nil {
		s.markBroken(id, err.Error())
		return nil, err
	}
	if p.ID != id {
		err := fmt.Errorf("%w: %s 里 id=%q 与目录名 %q 不一致（复制档案目录时常见）",
			ErrCorrupt, path, p.ID, id)
		s.markBroken(id, err.Error())
		return nil, err
	}
	return p, nil
}

func (s *Store) LoadBookmarks(id string) (*model.Bookmarks, error) {
	path := s.BookmarksPath(id)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(s.ProfilePath(id)); err != nil {
			return nil, fmt.Errorf("%w: profile %q", ErrNotFound, id)
		}
		return EmptyBookmarks(), nil // 新档案：空而不是坏
	}
	if err != nil {
		return nil, fmt.Errorf("读 %s: %w", path, err)
	}
	b, err := DecodeBookmarks(path, data)
	if err != nil {
		s.markBroken(id, err.Error())
		return nil, err
	}
	if err := b.Validate(); err != nil {
		// 同时裹住 ErrValidation 与 ErrCorrupt：上层要能分清
		// "形状没问题但内容不合法"与"根本读不了"。
		wrapped := fmt.Errorf("%w: %s: %v", ErrCorrupt, path, err)
		s.markBroken(id, wrapped.Error())
		return nil, errors.Join(ErrCorrupt, err)
	}
	if b.Rev < 0 {
		err := fmt.Errorf("%w: %s: rev 不能为负(%d)", ErrCorrupt, path, b.Rev)
		s.markBroken(id, err.Error())
		return nil, err
	}
	return b, nil
}

func (s *Store) Current(id string) (Snapshot, error) {
	b, err := s.LoadBookmarks(id)
	if err != nil {
		return Snapshot{}, err
	}
	fi, err := os.Stat(s.BookmarksPath(id))
	if err != nil {
		// 空档案没有 bookmarks.json，用 profile.json 的时间做指纹，
		// 保证缓存键稳定而不是每次不同。
		fi, err = os.Stat(s.ProfilePath(id))
		if err != nil {
			return Snapshot{}, fmt.Errorf("stat %s: %w", id, err)
		}
	}
	return Snapshot{B: b, Rev: b.Rev, Modified: fi.ModTime()}, nil
}

func EmptyBookmarks() *model.Bookmarks {
	return &model.Bookmarks{
		Title:  "发射台",
		Groups: []model.Group{},
		Search: model.Search{Engines: []model.SearchEngine{}},
		Weather: model.Weather{Location: "上海市"},
	}
}

// DecodeBookmarks 拒未知键、把语法错误翻成行号、统一归到 ErrCorrupt。
func DecodeBookmarks(path string, data []byte) (*model.Bookmarks, error) {
	var b model.Bookmarks
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return nil, describeJSONErr(path, data, err)
	}
	if b.Groups == nil {
		b.Groups = []model.Group{}
	}
	return &b, nil
}

func decodeProfile(path string, data []byte) (*model.Profile, error) {
	var p model.Profile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, describeJSONErr(path, data, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrCorrupt, path, err)
	}
	return &p, nil
}

// describeJSONErr 报"第 N 行"而不是字节偏移量：要改文件的人是打开文件改的。
func describeJSONErr(path string, data []byte, err error) error {
	base := filepath.Base(path)
	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		return fmt.Errorf("%w: %s 第 %d 行附近: %v", ErrCorrupt, base, lineOf(data, int(syn.Offset)), err)
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Errorf("%w: %s 第 %d 行附近: 字段 %q 类型应为 %s", ErrCorrupt,
			base, lineOf(data, int(te.Offset)), te.Field, te.Type)
	}
	if strings.Contains(err.Error(), "unknown field") {
		return fmt.Errorf("%w: %s: %v", ErrCorrupt, base, err)
	}
	return fmt.Errorf("%w: %s: %v", ErrCorrupt, base, err)
}

func lineOf(data []byte, offset int) int {
	if offset > len(data) {
		offset = len(data)
	}
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}
```

执行期订正（Task 4 实测得出，Task 11 的坏档案页依赖它）：`json.Decoder` 遇到被截断的文件返回的是 `io.ErrUnexpectedEOF`，**不是** `*json.SyntaxError`——只认 `SyntaxError` 的 `describeJSONErr` 会走进兜底分支，报不出行号，本案的"文件+行号"承诺当场落空。必须保留 `io.ErrUnexpectedEOF` 分支并报最后一行。另外 `broken.go` 原文多带一个未使用的 `sync` import（`sync` 属于 `paths.go`），照抄会编译不过。

`TestValidateRunsOnLoad` 用 `errors.Join(ErrCorrupt, err)` 才同时满足 `ErrCorrupt` 与 `model.ErrValidation` 两条断言——这不是取巧：上层确实要能分辨两种成因。

- [ ] **Step 5: 跑到通过**

Run: `go test ./internal/store/ ./internal/model/ -v`
Expected: 全 PASS

- [ ] **Step 6: 破坏验证（三处，逐条来）**

1. 把 `lineOf` 直接 `return 1`：Run `go test ./internal/store/ -run TestCorruptErrorNamesFileAndLine` → Expected: FAIL（第 6 行被报成第 1 行）。
2. 注释掉 `dec.DisallowUnknownFields()`：Run `go test ./internal/store/ -run TestUnknownKeyIsRejected` → Expected: FAIL（拼错的键被静默吞掉）。
3. 把 `LoadBookmarks` 里"文件不存在返回空档案"改成返回 `ErrCorrupt`：Run `go test ./internal/store/ -run TestProfileWithoutBookmarksIsEmptyNotBroken` → Expected: FAIL（新建档会被当成坏档案，全家导航页跟着变红）。

逐条恢复后跑总门禁。

- [ ] **Step 7: 提交**

```bash
git add internal/store
git commit -m "feat(store): 按档案读取，坏档案只坏它自己

未知键一律拒：手改或 AI 改时拼错的键被静默忽略会让人以为改动生效了。
语法错误翻成行号，改的人对着文件就能定位。刚建好还没写过的档案算空
档案而不是坏档案——否则每次建档都会先在自己页面看到红字。"
```

---

### Task 5: 原子写、按档案加锁、编码失败不动原文件

**Files:**
- Create: `internal/store/write.go`
- Test: `internal/store/write_test.go`

**Interfaces:**
- Consumes: `store.WriteJSON`（Task 2）、`New` 与路径方法（Task 4）
- Produces:
  - `(*Store).SaveBookmarks(id string, b *model.Bookmarks) error`（**rev 由调用方递增**，本函数只校验 + 编码 + 落盘）
  - `(*Store).SaveProfile(p *model.Profile) error`
  - `(*Store).WriteFileJSON(path string, v any) error`
  - `(*Store).WriteFileAtomic(path string, data []byte) error`
  - `(*Store).WithProfileLock(id string, fn func() error) error`

- [ ] **Step 1: 写失败的测试**

`internal/store/write_test.go`：

```go
package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func bookmarksFixture(rev int64, title string) *model.Bookmarks {
	b := EmptyBookmarks()
	b.Rev = rev
	b.Title = title
	b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList, Items: []model.Item{
		{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
	}}}
	return b
}

func TestSaveAndReload(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SaveBookmarks("dad", bookmarksFixture(1, "爸爸&<b>台")); err != nil {
		t.Fatal(err)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 1 || b.Title != "爸爸&<b>台" {
		t.Fatalf("got %+v", b)
	}
	raw, _ := os.ReadFile(s.BookmarksPath("dad"))
	if strings.Contains(string(raw), `\u003c`) {
		t.Fatalf("写出仍被 HTML 转义:\n%s", raw)
	}
}

func TestNoTempFilesLeftBehind(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SaveBookmarks("dad", bookmarksFixture(1, "x")); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(s.ProfileDir("dad"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("临时文件残留: %s", e.Name())
		}
	}
}

// 编码失败时原文件必须一个字节都不动，也不留临时文件。
// 顺序写反（先开目标文件再编码）就会把已有书签截成半个 JSON。
func TestEncodeFailureLeavesOriginalIntact(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SaveBookmarks("dad", bookmarksFixture(7, "原样")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.BookmarksPath("dad"))

	type unencodable struct {
		Rev int64  `json:"rev"`
		Ch  chan int `json:"ch"`
	}
	path := s.BookmarksPath("dad")
	if err := s.WriteFileJSON(path, unencodable{Rev: 8, Ch: make(chan int)}); err == nil {
		t.Fatal("want encode error")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatalf("编码失败却动了原文件:\n%s", after)
	}
	for _, e := range mustReadDir(t, filepath.Dir(path)) {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("失败路径残留临时文件: %s", e.Name())
		}
	}
}

func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ents
}

// 并发写之后任何一次读都必须拿到完整文件。
func TestConcurrentSavesKeepFileParseable(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SaveBookmarks("dad", bookmarksFixture(0, "seed")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := s.WithProfileLock("dad", func() error {
				return s.SaveBookmarks("dad", bookmarksFixture(int64(i), "并发"))
			})
			if err != nil {
				t.Errorf("save %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("并发写后读失败: %v", err)
	}
	if len(b.Groups) != 1 || len(b.Groups[0].Items) != 1 {
		t.Fatalf("内容不完整: %+v", b)
	}
}

// dad 被锁住时 mom 不该跟着排队。
func TestProfileLocksAreIndependent(t *testing.T) {
	s := New(t.TempDir())
	dadIn := make(chan struct{})
	finish := make(chan struct{})
	go func() {
		_ = s.WithProfileLock("dad", func() error {
			close(dadIn)
			<-finish
			return nil
		})
	}()
	<-dadIn
	done := make(chan struct{})
	go func() {
		_ = s.WithProfileLock("mom", func() error { return nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("mom 被 dad 的锁挡住了：锁没按档案分")
	}
	close(finish)
}

func TestSaveRejectsInvalidBeforeTouchingDisk(t *testing.T) {
	s := New(t.TempDir())
	bad := bookmarksFixture(1, "坏")
	bad.Groups[0].Layout = "grid"
	err := s.SaveBookmarks("dad", bad)
	if !strings.Contains(err.Error(), "layout") {
		t.Fatalf("want layout error, got %v", err)
	}
	if _, err := os.Stat(s.BookmarksPath("dad")); err == nil {
		t.Fatal("校验没过就不该落盘")
	}
}

func TestSaveProfileCreatesDir(t *testing.T) {
	s := New(t.TempDir())
	p := model.Profile{ID: "dad", Title: "爸爸", Admin: true, PIN: "sha256$a$b",
		CreatedAt: "2026-10-05T12:00:00+08:00", Theme: "auto", Layout: "sidebar"}
	if err := s.SaveProfile(&p); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Admin {
		t.Fatalf("got %+v", got)
	}
}
```

测试文件顶部 import 要含 `"time"`。

- [ ] **Step 2: 跑到它失败**

Run: `go test ./internal/store/ -run 'TestSave|TestNoTemp|TestEncodeFailure|TestConcurrent|TestProfileLocks' -v`
Expected: 编译失败 `undefined: (*Store).SaveBookmarks`

- [ ] **Step 3: 实现**

`internal/store/write.go`：

```go
package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rockbenben/launchbay/internal/model"
)

// WithProfileLock 把同一档案的读写串起来，不同档案互不阻塞。
// 它只在本进程内有效——所以所有写路径都必须经服务进程进来，
// CLI 不许直接写盘（spec §4.5）。
func (s *Store) WithProfileLock(id string, fn func() error) error {
	l := s.profileLock(id)
	l.Lock()
	defer l.Unlock()
	return fn()
}

func (s *Store) profileLock(id string) *sync.Mutex {
	s.lmu.Lock()
	defer s.lmu.Unlock()
	if s.locks == nil {
		s.locks = map[string]*sync.Mutex{}
	}
	l, ok := s.locks[id]
	if !ok {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	return l
}

func (s *Store) SaveBookmarks(id string, b *model.Bookmarks) error {
	if err := b.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return s.WriteFileJSON(s.BookmarksPath(id), b)
}

func (s *Store) SaveProfile(p *model.Profile) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return s.WriteFileJSON(s.ProfilePath(p.ID), p)
}

// WriteFileJSON 先编码到内存再落盘。顺序反过来就会在编码失败时
// 把已有内容截断成半个 JSON——那是唯一会真正丢数据的失败模式。
func (s *Store) WriteFileJSON(path string, v any) error {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, v); err != nil {
		return fmt.Errorf("编码 %s 失败: %w", filepath.Base(path), err)
	}
	return s.WriteFileAtomic(path, buf.Bytes())
}

// WriteFileAtomic 用同目录临时文件 + rename。Windows 上 os.Rename 能
// 覆盖已存在的目标（Go 内部走 MoveFileEx + REPLACE_EXISTING），跨平台一致。
func (s *Store) WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("建目录 %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("建临时文件: %w", err)
	}
	name := tmp.Name()
	discard := func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}
	if _, err := tmp.Write(data); err != nil {
		discard()
		return fmt.Errorf("写临时文件: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		discard()
		return fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("rename 到 %s: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 4: 跑到通过**

Run: `go test ./internal/store/ -race -v`
Expected: 全 PASS，无 data race

- [ ] **Step 5: 破坏验证**

把 `WriteFileJSON` 改成先 `s.WriteFileAtomic(path, nil)` 建空文件、再把 encoder 直接接到目标文件上：

Run: `go test ./internal/store/ -run TestEncodeFailureLeavesOriginalIntact`
Expected: FAIL —— 原文件被截断

恢复。把 `profileLock` 改成返回同一把全局锁：

Run: `go test ./internal/store/ -run TestProfileLocksAreIndependent`
Expected: FAIL —— mom 被 dad 挡住，1 秒超时

恢复后跑总门禁 + `-race`。

- [ ] **Step 6: 提交**

```bash
git add internal/store
git commit -m "feat(store): 原子写与按档案加锁，编码失败不动原文件

先在内存编码再 rename：顺序反了会在写失败时把已有书签截成半个 JSON。
锁按档案分，不同人不排队；它只在本进程有效——CLI 因此必须走服务写。"
```

---

### Task 6: PIN 哈希与令牌配置

**Files:**
- Create: `internal/auth/pin.go`
- Create: `internal/auth/token.go`
- Test: `internal/auth/pin_test.go`、`internal/auth/token_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `auth.HashPIN(pin string) (string, error)`、`auth.VerifyPIN(stored, pin string) bool`、`auth.ValidPinFormat(string) bool`、`auth.ErrBadPin`
  - `auth.Scope{Name, Secret, Profile string}` + 方法 `IsAdmin()`、`Allows(profileID string)`
  - `auth.Scopes []auth.Scope` + `auth.ParseScopes(envValue string) (auth.Scopes, error)`、`(auth.Scopes).Authorize(secret string) (auth.Scope, bool)`
  - 常量 `auth.ScopeAny = "*"`

- [ ] **Step 1: 写失败的测试**

`internal/auth/pin_test.go`：

```go
package auth

import "testing"

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := HashPIN("4812")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPIN(h, "4812") {
		t.Fatalf("自己的 PIN 验不过: %s", h)
	}
	if VerifyPIN(h, "4813") {
		t.Fatal("错 PIN 竟然通过")
	}
}

func TestSaltedSoSamePinGivesDifferentHash(t *testing.T) {
	a, _ := HashPIN("4812")
	b, _ := HashPIN("4812")
	if a == b {
		t.Fatal("没有随机盐：同 PIN 同哈希")
	}
}

func TestPinFormat(t *testing.T) {
	for _, ok := range []string{"1234", "123456"} {
		if !ValidPinFormat(ok) {
			t.Fatalf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"123", "1234567", "12a4", "", "１２３４"} {
		if ValidPinFormat(bad) {
			t.Fatalf("%q 应非法", bad)
		}
	}
	if _, err := HashPIN("12a4"); err == nil {
		t.Fatal("HashPIN 必须自己再拦一次格式，不能只靠调用方")
	}
}

func TestVerifyRejectsMalformedStored(t *testing.T) {
	for _, stored := range []string{"", "sha256$a", "sha256$a$b$c", "md5$a$b", "sha256$zz$b"} {
		if VerifyPIN(stored, "4812") {
			t.Fatalf("畸形 stored 竟然通过: %q", stored)
		}
	}
}

// 全角数字必须走"格式不合法"这条路，而不是变成一个能过的 PIN。
func TestFullWidthPinNeverMatches(t *testing.T) {
	h, _ := HashPIN("1234")
	if VerifyPIN(h, "１２３４") {
		t.Fatal("全角数字被判通过")
	}
}
```

`internal/auth/token_test.go`：

```go
package auth

import "testing"

func TestParseScopes(t *testing.T) {
	scopes, err := ParseScopes("ai-dad:xxx:dad,ext-mom:yyy:mom,admin:zzz:*")
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 3 {
		t.Fatalf("got %d", len(scopes))
	}
	if scopes[2].Profile != ScopeAny || !scopes[2].IsAdmin() {
		t.Fatalf("admin scope 解析错: %+v", scopes[2])
	}
}

func TestParseScopesRejects(t *testing.T) {
	for _, bad := range []string{"a:b", "a:b:c:d", "a::c", ":b:c", "a:b:", "a:b:c,,d:e:f",
		"a:b:Mom", "a:b:../etc", "   "} {
		if _, err := ParseScopes(bad); err == nil {
			t.Fatalf("应拒: %q", bad)
		}
	}
	if _, err := ParseScopes("a:b:c,x:b:y"); err == nil {
		t.Fatal("同一 secret 两个条目要视为配置错误：无法分别吊销")
	}
}

func TestAuthorize(t *testing.T) {
	scopes, _ := ParseScopes("ai-dad:xxx:dad,admin:zzz:*")
	if s, ok := scopes.Authorize("xxx"); !ok || s.Profile != "dad" || s.IsAdmin() {
		t.Fatalf("got %+v %v", s, ok)
	}
	if s, ok := scopes.Authorize("zzz"); !ok || !s.IsAdmin() {
		t.Fatalf("admin: %+v %v", s, ok)
	}
	for _, secret := range []string{"nope", "", "XXX", "xxx ", "x"} {
		if _, ok := scopes.Authorize(secret); ok {
			t.Fatalf("secret %q 不该通过", secret)
		}
	}
}

func TestScopeAllows(t *testing.T) {
	scopes, _ := ParseScopes("ai-dad:xxx:dad,admin:zzz:*")
	dad, _ := scopes.Authorize("xxx")
	admin, _ := scopes.Authorize("zzz")
	if !dad.Allows("dad") || dad.Allows("mom") {
		t.Fatalf("dad scope 判定错: %+v", dad)
	}
	if !admin.Allows("mom") || !admin.Allows("dad") {
		t.Fatalf("admin scope 应放行任何档案: %+v", admin)
	}
}
```

- [ ] **Step 2: 跑到它失败**

Run: `go test ./internal/auth/ -v`
Expected: 编译失败 `undefined: HashPIN`

- [ ] **Step 3: 实现**

`internal/auth/pin.go`：

```go
// Package auth 只管三件事：PIN 哈希与比对、令牌配置解析、会话 cookie 读写。
// 按 spec §11 的取舍，这里刻意不做：限速、锁定、失败计数、cookie 签名。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const pinAlgo = "sha256"

var ErrBadPin = errors.New("PIN 要 4-6 位数字")

// ValidPinFormat 只接受 ASCII 数字。全角数字不算合法：有人用输入法打成
// 全角时要的是"这条被拒"，而不是"PIN 悄悄变成另一个值"。
func ValidPinFormat(pin string) bool {
	if len(pin) < 4 || len(pin) > 6 {
		return false
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// HashPIN 产出 "sha256$<盐>$<摘要>"。
func HashPIN(pin string) (string, error) {
	if !ValidPinFormat(pin) {
		return "", ErrBadPin
	}
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("取随机盐: %w", err)
	}
	sum := sha256.Sum256(append(salt, pin...))
	return fmt.Sprintf("%s$%s$%s", pinAlgo, hex.EncodeToString(salt), hex.EncodeToString(sum[:])), nil
}

func VerifyPIN(stored, pin string) bool {
	algo, saltHex, wantHex, err := splitPinHash(stored)
	if err != nil || algo != pinAlgo {
		return false
	}
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(wantHex)
	if err != nil {
		return false
	}
	if !ValidPinFormat(pin) {
		// 仍然算一次，免得"格式错"成一个能用耗时量出来的早退分支。
		pin = "0000"
	}
	sum := sha256.Sum256(append(salt, pin...))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}

func splitPinHash(stored string) (algo, salt, digest string, err error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 {
		return "", "", "", errors.New("PIN 哈希形状应为 algo$盐$摘要")
	}
	return parts[0], parts[1], parts[2], nil
}
```

`internal/auth/token.go`：

```go
package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

const ScopeAny = "*"

type Scope struct {
	Name    string
	Secret  string
	Profile string
}

func (s Scope) IsAdmin() bool { return s.Profile == ScopeAny }

func (s Scope) Allows(profileID string) bool {
	return s.Profile == ScopeAny || s.Profile == profileID
}

type Scopes []Scope

// ParseScopes 吃 LAUNCHBAY_WRITE_TOKENS 的值：name:secret:profile 逗号分隔。
// 段数不对、任一段为空、profile 不是合法 slug、或两个条目共用同一个
// secret，都算配置错误——后者会让人没法单独吊销某一端。
func ParseScopes(envValue string) (Scopes, error) {
	out := Scopes{}
	seen := map[string]string{}
	for _, raw := range strings.Split(envValue, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("令牌 %q 应为 name:secret:profile 三段", raw)
		}
		for i, p := range parts {
			if strings.TrimSpace(p) == "" {
				return nil, fmt.Errorf("令牌 %q 的第 %d 段为空", raw, i+1)
			}
		}
		name, secret, profile := parts[0], parts[1], parts[2]
		if profile != ScopeAny && !model.ValidSlug(profile) {
			return nil, fmt.Errorf("令牌 %q 的档案段 %q 不是 ASCII slug", raw, profile)
		}
		if prev, ok := seen[secret]; ok {
			return nil, fmt.Errorf("secret 同时用于 %q 和 %q，无法分别吊销", prev, name)
		}
		seen[secret] = name
		out = append(out, Scope{Name: name, Secret: secret, Profile: profile})
	}
	if len(out) == 0 {
		return nil, errors.New("未配置任何令牌")
	}
	return out, nil
}

// Authorize 遍历全部条目做常量时间比较，不提前返回。
// 这在局域网里谈不上防护，但成本几乎为零。
func (l Scopes) Authorize(secret string) (Scope, bool) {
	if secret == "" {
		return Scope{}, false
	}
	var hit Scope
	found := 0
	for _, s := range l {
		if subtle.ConstantTimeCompare([]byte(s.Secret), []byte(secret)) == 1 {
			hit = s
			found++
		}
	}
	if found != 1 {
		return Scope{}, false
	}
	return hit, true
}
```

- [ ] **Step 4: 跑到通过**

Run: `go test ./internal/auth/ -v && gofmt -l .`
Expected: 全 PASS

- [ ] **Step 5: 破坏验证**

1. 删掉 `ParseScopes` 里的 secret 重复检查：Run `go test ./internal/auth/ -run TestParseScopesRejects` → Expected: FAIL。
2. 把 `ValidPinFormat` 的长度下限从 4 改成 3：Run `go test ./internal/auth/ -run TestPinFormat` → Expected: FAIL。
3. 把 `VerifyPIN` 里的 `if !ValidPinFormat(pin)` 早退改成 `return false`：Run `go test ./internal/auth/ -run TestFullWidthPinNeverMatches` → Expected: **仍 PASS**。这是有意的——两条路径都拒绝全角，测不出差别。此时**不要为了让它红去改判据**；把这条断言的价值记在注释里（它守的是"别把全角当数字"，不是"别早退"）。

恢复后跑总门禁。

- [ ] **Step 6: 提交**

```bash
git add internal/auth
git commit -m "feat(auth): PIN 加盐哈希与三段式令牌解析

4-6 位数字 PIN 用 sha256+随机盐就够：spec §11 明写不做爆破防护，
所以不引 argon2、不加限速。全角数字按格式非法拒收，不静默变值。
同 secret 出现在两个条目里就拒绝启动——那等于无法单独吊销。"
```

---

### Task 7: 会话 cookie 的三种形态

**Files:**
- Create: `internal/auth/session.go`
- Test: `internal/auth/session_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `auth.CookieProfile = "lb_profile"`、`auth.CookieHome = "lb_home"`、`auth.SessionCookieMaxAge = 30*24*3600`
  - `auth.Session{Active, Home string}` + `IsEmpty()`、`Valid()`、`Managing()`
  - `auth.ReadSession(r *http.Request) Session`、`auth.WriteSession(w, home, active string)`、`auth.SetActive(w, active string)`、`auth.ClearSession(w)`

- [ ] **Step 1: 写失败的测试**

`internal/auth/session_test.go`：

```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func requestWith(pairs ...[2]string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	for _, kv := range pairs {
		r.AddCookie(&http.Cookie{Name: kv[0], Value: kv[1]})
	}
	return r
}

func TestEmptyWhenNoCookies(t *testing.T) {
	if s := ReadSession(httptest.NewRequest("GET", "/", nil)); !s.IsEmpty() {
		t.Fatalf("want empty, got %+v", s)
	}
	rec := httptest.NewRecorder()
	WriteSession(rec, "dad", "dad")
	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("want 2 cookies, got %d", len(cookies))
	}
	for _, c := range cookies {
		if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
			t.Fatalf("cookie 属性不全: %+v", c)
		}
		if c.Secure {
			t.Fatal("本站跑在局域网 http 上，设 Secure 浏览器会直接丢掉 cookie")
		}
		if c.MaxAge != SessionCookieMaxAge {
			t.Fatalf("MaxAge = %d, want %d", c.MaxAge, SessionCookieMaxAge)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteSession(rec, "dad", "dad")
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	got := ReadSession(r)
	if got.Active != "dad" || got.Home != "dad" || !got.Valid() || got.Managing() {
		t.Fatalf("round trip 坏: %+v", got)
	}
}

func TestThreeLegalShapes(t *testing.T) {
	self := Session{Active: "mom", Home: "mom"}
	if !self.Valid() || self.Managing() {
		t.Fatalf("本人形态: %+v", self)
	}
	managing := Session{Active: "mom", Home: "dad"}
	if !managing.Valid() || !managing.Managing() {
		t.Fatalf("代管形态: %+v", managing)
	}
}

// active 有值但 home 缺失：无从判断是谁验证进来的、能不能代管，
// 一律判无效，让 handler 清 cookie 回选择页。
func TestMissingHomeIsInvalid(t *testing.T) {
	if s := ReadSession(requestWith([2]string{CookieProfile, "mom"})); s.Valid() {
		t.Fatalf("缺 lb_home 却判有效: %+v", s)
	}
	if s := (Session{Active: "mom", Home: ""}); s.Valid() {
		t.Fatalf("Valid 没拦: %+v", s)
	}
}

func TestPathTraversalishCookieValuesRejected(t *testing.T) {
	for _, v := range []string{"", ".", "..", "a/b", "a\\b", "Mom", "nope!", "a%2Fb"} {
		if ReadSession(requestWith([2]string{CookieProfile, v}, [2]string{CookieHome, v})).Valid() {
			t.Fatalf("非法档案名 %q 被判有效", v)
		}
	}
}

func TestSetActiveKeepsHome(t *testing.T) {
	rec := httptest.NewRecorder()
	SetActive(rec, "mom")
	var sawActive, sawHome bool
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case CookieProfile:
			sawActive = c.Value == "mom"
		case CookieHome:
			sawHome = true
		}
	}
	if !sawActive {
		t.Fatal("active 没改")
	}
	if sawHome {
		t.Fatal("SetActive 不该动 lb_home")
	}
}

func TestClearExpiresBothWithoutValue(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearSession(rec)
	n := 0
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge >= 0 || c.Value != "" {
			t.Fatalf("清除要用过期而非删值: %+v", c)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
}
```

- [ ] **Step 2: 跑到它失败**

Run: `go test ./internal/auth/ -run 'Session|Clear|SetActive' -v`
Expected: 编译失败 `undefined: ReadSession`

- [ ] **Step 3: 实现**

`internal/auth/session.go`：

```go
package auth

import (
	"net/http"
	"regexp"
)

const (
	CookieProfile       = "lb_profile"
	CookieHome          = "lb_home"
	SessionCookieMaxAge = 30 * 24 * 3600
)

var cookieValueRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Session 描述"当前在用哪个档案"（Active，权限只看它）和
// "当初是谁验证进来的"（Home，只喂管理横幅与历史 actor）。
// 两条 cookie 都不签名——spec §11 明写不防局域网内冒充。
type Session struct {
	Active string
	Home   string
}

func (s Session) IsEmpty() bool { return s.Active == "" && s.Home == "" }

func (s Session) Managing() bool {
	return s.Active != "" && s.Home != "" && s.Active != s.Home
}

func (s Session) Valid() bool {
	// Active 有值时 Home 必须同时存在且合法：没有 Home 就无从
	// 判断能不能代管，也不能决定横幅该不该出现。
	return cookieValueRE.MatchString(s.Active) && cookieValueRE.MatchString(s.Home)
}

func ReadSession(r *http.Request) Session {
	return Session{Active: cookieOf(r, CookieProfile), Home: cookieOf(r, CookieHome)}
}

func cookieOf(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil || !cookieValueRE.MatchString(c.Value) {
		return ""
	}
	return c.Value
}

func WriteSession(w http.ResponseWriter, home, active string) {
	setCookie(w, CookieHome, home, SessionCookieMaxAge)
	setCookie(w, CookieProfile, active, SessionCookieMaxAge)
}

// SetActive 只在管理员代管时用：动 Active，不动 Home。
func SetActive(w http.ResponseWriter, active string) {
	setCookie(w, CookieProfile, active, SessionCookieMaxAge)
}

func ClearSession(w http.ResponseWriter) {
	setCookie(w, CookieHome, "", -1)
	setCookie(w, CookieProfile, "", -1)
}

func setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
```

- [ ] **Step 4: 跑到通过**

Run: `go test ./internal/auth/ -v`
Expected: 全 PASS

- [ ] **Step 5: 破坏验证**

1. `Valid()` 改成 `return s.Active != ""`：Run `go test ./internal/auth/ -run TestMissingHomeIsInvalid` → Expected: FAIL（非法形态会去渲染他人书签）。
2. `cookieValueRE` 放宽成 `^.+$`：Run `go test ./internal/auth/ -run TestPathTraversalish` → Expected: FAIL（`..`、`Mom` 会被当成档案名拼进路径）。

恢复后跑总门禁。

- [ ] **Step 6: 提交**

```bash
git add internal/auth
git commit -m "feat(auth): 会话 cookie 与三种合法形态判定

active 有值而 home 缺失一律判无效：没有 home 就无从确定能不能代管、
横幅该不该出现。档案名按 slug 白名单校验，.. 与大写都进不了路径。"
```

---

### Task 8: 建档、模板克隆与半成品回收

**Files:**
- Create: `internal/store/profile.go`
- Test: `internal/store/profile_test.go`

**Interfaces:**
- Consumes: `store`（Task 4/5）、`model`（Task 3）
- Produces:
  - `store.NewProfileInput{ID, Title, PinHash string; Admin bool; FromTemplate string}`
  - `(*Store).CreateProfile(in store.NewProfileInput) (*model.Profile, error)`
  - `(*Store).LoadTemplate(name string) (*model.Bookmarks, error)`

- [ ] **Step 1: 写失败的测试**

`internal/store/profile_test.go`：

```go
package store

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const tplNas = `{
  "rev": 0,
  "title": "家庭服务模板",
  "groups": [
    {"id":"nas","title":"NAS 应用","layout":"list","items":[
      {"id":"dsm","name":"群晖 DSM","url":"http://10.0.0.3:5000","icon":"simple-icons:synology","alias":[],"tags":[]},
      {"id":"portainer","name":"Portainer","url":"http://10.0.0.3:9000","icon":"simple-icons:portainer","alias":[],"tags":[]}
    ]},
    {"id":"media","title":"影视","layout":"tiles","items":[
      {"id":"emby","name":"Emby","url":"http://10.0.0.3:8098","icon":"simple-icons:emby","alias":[],"tags":[]}
    ]}
  ],
  "search": {"enabled":false,"default":"","engines":[]},
  "weather": {"enabled":false,"location":"上海市"},
  "openInNewTab": false, "clock": false, "footer": ""
}`

func withTemplate(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	if err := os.MkdirAll(strings.TrimSuffix(s.TemplatePath("nas"), "/nas.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFileAtomic(s.TemplatePath("nas"), []byte(tplNas)); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateProfileWritesTwoFiles(t *testing.T) {
	s := New(t.TempDir())
	p, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b", Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "dad" || !p.Admin || p.CreatedAt == "" {
		t.Fatalf("got %+v", p)
	}
	for _, path := range []string{s.ProfilePath("dad"), s.BookmarksPath("dad")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("缺文件 %s", path)
		}
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 0 || len(b.Groups) != 0 {
		t.Fatalf("新档案要空且 rev 0: %+v", b)
	}
	raw, _ := os.ReadFile(s.BookmarksPath("dad"))
	if !strings.Contains(string(raw), `"groups": []`) {
		t.Fatalf("groups 要写成空数组而不是 null:\n%s", raw)
	}
}

func TestCreateProfileFromTemplate(t *testing.T) {
	s := withTemplate(t)
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h", Admin: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(NewProfileInput{ID: "mom", Title: "妈妈", PinHash: "h", FromTemplate: "nas"}); err != nil {
		t.Fatal(err)
	}
	b, err := s.LoadBookmarks("mom")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range b.Groups {
		for _, it := range g.Items {
			got = append(got, g.ID+"/"+it.ID)
		}
	}
	want := []string{"nas/dsm", "nas/portainer", "media/emby"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("克隆结果 %v, want %v", got, want)
	}
	if b.Rev != 0 {
		t.Fatalf("克隆出的基线 rev 要是 0，got %d", b.Rev)
	}
	// 克隆是复制不是引用：改掉模板不能回头动已建档案。
	if err := s.WriteFileAtomic(s.TemplatePath("nas"),
		[]byte(strings.Replace(tplNas, `"id":"emby","name":"Emby"`, `"id":"emby","name":"Emby 改名"`, 1))); err != nil {
		t.Fatal(err)
	}
	again, err := s.LoadBookmarks("mom")
	if err != nil {
		t.Fatal(err)
	}
	if again.Groups[1].Items[0].Name != "Emby" {
		t.Fatalf("模板改动渗进了已建档案: %+v", again.Groups[1].Items[0])
	}
}

func TestCreateProfileRejectsAndCleansUp(t *testing.T) {
	s := withTemplate(t)
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   NewProfileInput
		want error
	}{
		{"id 已存在", NewProfileInput{ID: "dad", Title: "又", PinHash: "h"}, ErrExists},
		{"id 非法", NewProfileInput{ID: "Dad", Title: "爸", PinHash: "h"}, ErrInvalid},
		{"title 空", NewProfileInput{ID: "kid", Title: " ", PinHash: "h"}, ErrInvalid},
		{"pin 空", NewProfileInput{ID: "kid", Title: "娃", PinHash: ""}, ErrInvalid},
		{"模板不存在", NewProfileInput{ID: "kid", Title: "娃", PinHash: "h", FromTemplate: "ghost"}, ErrNotFound},
		{"模板名非法", NewProfileInput{ID: "kid", Title: "娃", PinHash: "h", FromTemplate: "Na S"}, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.CreateProfile(tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
	// 失败的建档不许留下半个目录：否则选单里会出现永远进不去的档案。
	for _, id := range []string{"kid"} {
		if _, err := os.Stat(s.ProfileDir(id)); err == nil {
			t.Fatalf("失败建档残留目录 %s", id)
		}
	}
	if entries, err := os.ReadDir(s.profilesDir()); err == nil {
		if len(entries) != 1 {
			t.Fatalf("profiles 目录该只剩 dad, got %d", len(entries))
		}
	}
}

// 模板里内容不合法要当场拒，不能把坏数据当基线发出去。
func TestCreateProfileRejectsInvalidTemplate(t *testing.T) {
	s := withTemplate(t)
	if err := s.WriteFileAtomic(s.TemplatePath("nas"),
		[]byte(strings.Replace(tplNas, `"layout":"tiles"`, `"layout":"grid"`, 1))); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateProfile(NewProfileInput{ID: "kid", Title: "娃", PinHash: "h", FromTemplate: "nas"})
	if !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want invalid/corrupt, got %v", err)
	}
	if _, statErr := os.Stat(s.ProfileDir("kid")); statErr == nil {
		t.Fatal("要拒在落盘之前")
	}
}
```

- [ ] **Step 2: 跑到它失败**

Run: `go test ./internal/store/ -run TestCreateProfile -v`
Expected: 编译失败 `undefined: NewProfileInput`

- [ ] **Step 3: 实现**

`internal/store/profile.go`：

```go
package store

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
)

type NewProfileInput struct {
	ID           string
	Title        string
	PinHash      string
	Admin        bool
	FromTemplate string
}

// CreateProfile 建目录 + profile.json + bookmarks.json（rev 0）。
// 从模板克隆时把模板 groups 复制成新档案的基线：是复制品不是引用，
// 之后模板变更与该档案无关（spec §4.5）。
//
// 半途失败必须回收：留下只有 profile.json 的目录会让人在选单里
// 看到一个永远进不去的档案。
func (s *Store) CreateProfile(in NewProfileInput) (*model.Profile, error) {
	if !model.ValidSlug(in.ID) {
		return nil, fmt.Errorf("%w: 档案 id %q 必须是 ASCII slug", ErrInvalid, in.ID)
	}
	if in.PinHash == "" {
		return nil, fmt.Errorf("%w: 档案 %q 缺 PIN 哈希", ErrInvalid, in.ID)
	}
	if _, err := os.Stat(s.ProfilePath(in.ID)); err == nil {
		return nil, fmt.Errorf("%w: 档案 %q 已存在", ErrExists, in.ID)
	}

	groups := []model.Group{}
	if in.FromTemplate != "" {
		tpl, err := s.LoadTemplate(in.FromTemplate)
		if err != nil {
			return nil, err
		}
		groups = tpl.Groups
	}

	// 全部校验都排在落盘之前。
	p := model.Profile{
		ID: in.ID, Title: in.Title, Admin: in.Admin, PIN: in.PinHash,
		CreatedAt: time.Now().Format(time.RFC3339), Theme: "auto", Layout: "sidebar",
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	b := EmptyBookmarks()
	b.Groups = groups
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("%w: 模板 %s 内容不合法: %v", ErrInvalid, in.FromTemplate, err)
	}

	if err := os.MkdirAll(s.ProfileDir(in.ID), 0o755); err != nil {
		return nil, fmt.Errorf("建档案目录: %w", err)
	}
	if err := s.SaveProfile(&p); err != nil {
		s.rollbackDir(in.ID)
		return nil, err
	}
	if err := s.SaveBookmarks(in.ID, b); err != nil {
		s.rollbackDir(in.ID)
		return nil, err
	}
	return &p, nil
}

func (s *Store) rollbackDir(id string) {
	err := os.RemoveAll(s.ProfileDir(id))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// 回滚失败必须喊出来：留下半个档案比报错更难查。
		fmt.Fprintf(os.Stderr, "launchbay: 建档回滚失败 %s: %v\n", id, err)
	}
}

func (s *Store) LoadTemplate(name string) (*model.Bookmarks, error) {
	if !model.ValidSlug(name) {
		return nil, fmt.Errorf("%w: 模板名 %q 必须是 ASCII slug", ErrInvalid, name)
	}
	path := s.TemplatePath(name)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: 模板 %q", ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("读模板 %s: %w", path, err)
	}
	b, err := DecodeBookmarks(path, data)
	if err != nil {
		return nil, err
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("%w: 模板 %q: %v", ErrCorrupt, name, err)
	}
	return b, nil
}
```

- [ ] **Step 4: 跑到通过**

Run: `go test ./internal/store/ -race -v`
Expected: 全 PASS

- [ ] **Step 5: 破坏验证**

1. 把 `os.Stat` 的"已存在"检查删掉：Run `go test ./internal/store/ -run TestCreateProfileRejects/id_已存在` → Expected: FAIL（同名档案被覆盖，等于删档）。
2. 把 `rollbackDir` 调用从两个失败分支删掉：Run `go test ./internal/store/ -run TestCreateProfileRejectsAndCleansUp` → Expected: FAIL（残留目录）。若这条仍绿，说明失败发生在 `SaveProfile` 之前、测试没打到那一步——那就补一个"模板合法但 `SaveBookmarks` 失败"的夹具再测，别改判据。

恢复后跑总门禁。

- [ ] **Step 6: 提交**

```bash
git add internal/store
git commit -m "feat(store): 建档与模板克隆，失败回收半成品目录

校验全部排在落盘之前；克隆是复制不是引用，模板改动不回头动已建档案。
失败留下半个档案目录的话，选单里会出现一个永远进不去的档案。"
```

---

### Task 9: HTTP 装配 —— 建档、PIN 验证、管理员切档

**Files:**
- Create: `internal/httpapi/handlers.go`
- Modify: `internal/httpapi/server.go`
- Create: `cmd/launchbay/env.go`
- Test: `internal/httpapi/handlers_test.go`
- Test: `cmd/launchbay/env_test.go`

**Interfaces:**
- Consumes: `store`、`model`、`auth`（含 Task 7 的 Session）
- Produces:
  - `httpapi.Dependencies{Store *store.Store; Tokens auth.Scopes}`
  - `httpapi.New(d httpapi.Dependencies) *httpapi.Server`（**签名从 Task 1 的 `New()` 改为带依赖**）
  - `cmd/launchbay.loadConfig() (config, error)`、`type config{Port, DataDir string; Tokens auth.Scopes}`
  - 端点：`POST /pin`、`GET /`、`POST /api/v1/profiles`

- [ ] **Step 1: 先写配置加载的失败测试**

`cmd/launchbay/env_test.go`：

```go
package main

import (
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("LAUNCHBAY_PORT", "")
	t.Setenv("LAUNCHBAY_DATA_DIR", "")
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:zzz:*")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "7600" {
		t.Fatalf("port = %q", cfg.Port)
	}
	if cfg.DataDir != "data" {
		t.Fatalf("dataDir = %q", cfg.DataDir)
	}
}

// 令牌配置写错要拒绝启动，而不是跑起来之后每个写请求都 401——
// 后者会让人以为是 PIN 的问题，白查半天。
func TestLoadConfigRejectsBadTokens(t *testing.T) {
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:zzz")
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for malformed tokens")
	}
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for empty tokens")
	}
}
```

- [ ] **Step 2: 实现配置加载**

`cmd/launchbay/env.go`：

```go
package main

import (
	"fmt"
	"os"

	"github.com/rockbenben/launchbay/internal/auth"
)

type config struct {
	Port    string
	DataDir string
	Tokens  auth.Scopes
}

func loadConfig() (config, error) {
	c := config{Port: getenvOr("LAUNCHBAY_PORT", "7600"), DataDir: getenvOr("LAUNCHBAY_DATA_DIR", "data")}
	scopes, err := auth.ParseScopes(os.Getenv("LAUNCHBAY_WRITE_TOKENS"))
	if err != nil {
		return config{}, fmt.Errorf("LAUNCHBAY_WRITE_TOKENS: %w", err)
	}
	c.Tokens = scopes
	return c, nil
}

func getenvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

Run: `go test ./cmd/... -v` → Expected: PASS

- [ ] **Step 3: 写 handler 的失败测试**

`internal/httpapi/handlers_test.go`：

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

func setup(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	s := store.New(t.TempDir())
	hash := func(pin string) string {
		h, err := auth.HashPIN(pin)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "dad", Title: "爸爸", PinHash: hash("1234"), Admin: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "kid", Title: "小满", PinHash: hash("5678")}); err != nil {
		t.Fatal(err)
	}
	scopes, err := auth.ParseScopes("ai-dad:tok-dad:dad,admin:tok-admin:*")
	if err != nil {
		t.Fatal(err)
	}
	return s, New(Dependencies{Store: s, Tokens: scopes}).Handler()
}

func post(h http.Handler, path, body, cookie, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cookieMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, c := range rec.Result().Cookies() {
		out[c.Name] = c.Value
	}
	return out
}

func TestPinGrantsSessionWithTwoCookies(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/pin", `{"profile":"dad","pin":"1234"}`, "", "")
	if rec.Code != 200 {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	cm := cookieMap(t, rec)
	if cm[auth.CookieProfile] != "dad" || cm[auth.CookieHome] != "dad" {
		t.Fatalf("cookies: %v", cm)
	}
}

func TestPinWrongIs401UnknownProfileIs404(t *testing.T) {
	_, h := setup(t)
	if rec := post(h, "/pin", `{"profile":"dad","pin":"0000"}`, "", ""); rec.Code != 401 {
		t.Fatalf("错 PIN want 401, got %d", rec.Code)
	}
	if rec := post(h, "/pin", `{"profile":"ghost","pin":"1234"}`, "", ""); rec.Code != 404 {
		t.Fatalf("不存在 want 404, got %d", rec.Code)
	}
	// 401 与 404 必须可区分：前者是"这档案不是你的"，后者是"没这档案"。
}

func TestAdminCanSwitchWithoutPin(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/pin", `{"profile":"kid","pin":""}`, "lb_profile=dad; lb_home=dad", "")
	if rec.Code != 200 {
		t.Fatalf("管理员免 PIN 切档被拒: %d %s", rec.Code, rec.Body)
	}
	cm := cookieMap(t, rec)
	if cm[auth.CookieProfile] != "kid" {
		t.Fatalf("active 没切: %v", cm)
	}
	if cm[auth.CookieHome] != "dad" {
		t.Fatalf("切档不该动 home: %v", cm)
	}
	var body struct {
		Managing bool `json:"managing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.Managing {
		t.Fatalf("响应要告诉前端现在在代管: %s", rec.Body)
	}
}

func TestNonAdminCannotSwitchWithoutPin(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct{ name, cookie string }{
		{"本人会话", "lb_profile=kid; lb_home=kid"},
		{"无会话", ""},
		{"非法形态（缺 home）", "lb_profile=kid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(h, "/pin", `{"profile":"dad","pin":""}`, tc.cookie, "")
			if rec.Code != 401 {
				t.Fatalf("cookie %q want 401, got %d %s", tc.cookie, rec.Code, rec.Body)
			}
		})
	}
}

// 非管理员即便把 home 伪造成 dad（管理员）也不能免 PIN 切档：
// 服务端要回查 profile 的 admin 字段，不能信 cookie 的形状。
func TestForgedHomeDoesNotGrantAdmin(t *testing.T) {
	_, h := setup(t)
	rec := post(h, "/pin", `{"profile":"dad","pin":""}`, "lb_profile=kid; lb_home=dad", "")
	if rec.Code != 401 {
		t.Fatalf("伪造 home=dad 竟可切档: %d", rec.Code)
	}
}

// 注意这条与上一条的区别：伪造 home 本身拿不到权限，但本站
// 明写不防冒充（spec §11）——lb_profile=kid 直接就能进 kid 的档案。
// 这条测试钉的是"免 PIN 切档"这个额外动作需要真的管理员。
func TestMalformedSessionIsClearedNotRendered(t *testing.T) {
	_, h := setup(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "kid"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200 选择页, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "小满") {
		t.Fatalf("非法会话渲染出了档案页:\n%s", rec.Body)
	}
	var cleared int
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			cleared++
		}
	}
	if cleared != 2 {
		t.Fatalf("want 两条 cookie 被清, got %d", cleared)
	}
}

func TestCreateProfileNeedsAdminToken(t *testing.T) {
	s, h := setup(t)
	if rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", "tok-dad"); rec.Code != 403 {
		t.Fatalf("非管理员令牌 want 403, got %d", rec.Code)
	}
	if rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", "nope"); rec.Code != 401 {
		t.Fatalf("坏令牌 want 401, got %d", rec.Code)
	}
	rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"9999"}`, "", "tok-admin")
	if rec.Code != 201 {
		t.Fatalf("want 201, got %d %s", rec.Code, rec.Body)
	}
	if _, err := s.LoadProfile("mom"); err != nil {
		t.Fatalf("档案没落盘: %v", err)
	}
	if strings.Contains(rec.Body.String(), "9999") {
		t.Fatalf("响应回显了 PIN: %s", rec.Body)
	}
}

func TestCreateProfileRejectsBadBody(t *testing.T) {
	_, h := setup(t)
	cases := []struct {
		body string
		field string
	}{
		{`{"id":"mom"}`, "title"},
		{`{"id":"mom","title":"妈","pin":"12"}`, "pin"},
		{`{"id":"Mom","title":"妈","pin":"9999"}`, "id"},
		{`{"id":"mom","title":" ","pin":"9999"}`, "title"},
		{`{"id":"mom","title":"妈","pin":"9999","extra":1}`, "extra"},
	}
	for _, tc := range cases {
		rec := post(h, "/api/v1/profiles", tc.body, "", "tok-admin")
		if rec.Code != 422 {
			t.Fatalf("body %s want 422, got %d %s", tc.body, rec.Code, rec.Body)
		}
		var out struct {
			Errors []struct {
				Field  string `json:"field"`
				Reason string `json:"reason"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Errors) == 0 {
			t.Fatalf("422 必须逐条给字段: %s", rec.Body)
		}
		var hit bool
		for _, e := range out.Errors {
			if e.Field == tc.field && e.Reason != "" {
				hit = true
			}
		}
		if !hit {
			t.Fatalf("body %s 要报出字段 %q: %s", tc.body, tc.field, rec.Body)
		}
	}
}
```

- [ ] **Step 4: 跑到它失败**

Run: `go test ./internal/httpapi/ -run 'TestPin|TestAdmin|TestNonAdmin|TestForged|TestMalformed|TestCreateProfile' -v`
Expected: 编译失败 `undefined: Dependencies`

- [ ] **Step 5: 实现**

先改一件会卡住编译的事：`New()` 变成 `New(Dependencies)`，**Task 1 写的 `internal/httpapi/server_test.go` 里两处 `New()` 必须同步改**，否则本任务编译不过：

```go
func testDeps(t *testing.T) Dependencies {
	t.Helper()
	scopes, err := auth.ParseScopes("admin:tok:*")
	if err != nil {
		t.Fatal(err)
	}
	return Dependencies{Store: store.New(t.TempDir()), Tokens: scopes}
}
```

两处调用改成 `New(testDeps(t)).Handler()`，并在该文件 import 里补 `auth` 与 `store`。

`internal/httpapi/server.go` 改为：

```go
package httpapi

import (
	"net/http"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// Dependencies 是装配点。Task 11 会往这里加 Renderer。
type Dependencies struct {
	Store  *store.Store
	Tokens auth.Scopes
}

type Server struct {
	mux  *http.ServeMux
	deps Dependencies
}

func New(d Dependencies) *Server {
	s := &Server{mux: http.NewServeMux(), deps: d}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	s.mux.HandleFunc("POST /pin", s.handlePin)
	s.mux.HandleFunc("GET /", s.handleRoot)
	s.mux.HandleFunc("POST /api/v1/profiles", s.handleCreateProfile)
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }
```

`internal/httpapi/handlers.go`：

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

const maxBodyBytes = 1 << 20 // 1 MiB：建档与 PIN 用不到更大

func (s *Server) handlePin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Profile string `json:"profile"`
		PIN     string `json:"pin"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "body", Reason: err.Error()}})
		return
	}
	if !model.ValidSlug(in.Profile) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "profile", Reason: "必须是 ASCII slug"}})
		return
	}
	target, err := s.deps.Store.LoadProfile(in.Profile)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "detail": "档案不存在"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "detail": err.Error()})
		return
	}

	sess := auth.ReadSession(r)

	if in.PIN == "" {
		// 免 PIN 切档只有一种情形成立：会话形态合法，且 home
		// 那个档案真的带 admin——管理员身份回查磁盘，不信 cookie 形状。
		if !sess.Valid() || sess.Home == in.Profile {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "detail": "缺 PIN"})
			return
		}
		home, err := s.deps.Store.LoadProfile(sess.Home)
		if err != nil || !home.Admin {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "detail": "非管理员不能免 PIN 切档"})
			return
		}
		auth.SetActive(w, in.Profile)
		writeJSON(w, http.StatusOK, map[string]any{"active": in.Profile, "managing": true})
		return
	}

	if !auth.VerifyPIN(target.PIN, in.PIN) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "detail": "PIN 不对"})
		return
	}
	auth.WriteSession(w, in.Profile, in.Profile)
	writeJSON(w, http.StatusOK, map[string]any{"active": in.Profile, "managing": false})
}

// handleRoot 在 Task 11 接渲染器。这一期只按会话形态决定给什么，
// 保证鉴权与 cookie 的行为先被钉住。
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	sess := auth.ReadSession(r)
	switch {
	case sess.IsEmpty():
		writePlaceholder(w, "<h1>谁来用？</h1>")
	case !sess.Valid():
		// 有 cookie 但形态非法（比如只有 active 没 home）：清掉回选择页。
		auth.ClearSession(w)
		writePlaceholder(w, "<h1>谁来用？</h1>")
	default:
		if _, err := s.deps.Store.LoadProfile(sess.Active); err != nil {
			auth.ClearSession(w)
			writePlaceholder(w, "<h1>谁来用？</h1>")
			return
		}
		writePlaceholder(w, "<h1>"+sess.Active+"</h1>")
	}
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	sc, ok := s.authorize(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "detail": "缺有效令牌"})
		return
	}
	if !sc.IsAdmin() {
		writeJSON(w, http.StatusForbidden, map[string]any{"code": 403, "detail": "建档要管理员令牌"})
		return
	}
	var in struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		PIN          string `json:"pin"`
		FromTemplate string `json:"fromTemplate"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "body", Reason: err.Error()}})
		return
	}
	var errs []fieldError
	if !model.ValidSlug(in.ID) {
		errs = append(errs, fieldError{Field: "id", Reason: "必须是 ASCII slug（小写字母、数字、连字符）"})
	}
	if strings.TrimSpace(in.Title) == "" {
		errs = append(errs, fieldError{Field: "title", Reason: "不能为空"})
	}
	if !auth.ValidPinFormat(in.PIN) {
		errs = append(errs, fieldError{Field: "pin", Reason: "要 4-6 位数字"})
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, errs)
		return
	}
	hash, err := auth.HashPIN(in.PIN)
	if err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "pin", Reason: err.Error()}})
		return
	}
	p, err := s.deps.Store.CreateProfile(store.NewProfileInput{
		ID: in.ID, Title: in.Title, PinHash: hash, FromTemplate: in.FromTemplate,
	})
	switch {
	case errors.Is(err, store.ErrExists):
		writeJSON(w, http.StatusConflict, map[string]any{"code": 409, "detail": "档案已存在"})
	case errors.Is(err, store.ErrNotFound):
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "fromTemplate", Reason: "模板不存在"}})
	case errors.Is(err, store.ErrInvalid), errors.Is(err, store.ErrCorrupt):
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "fromTemplate", Reason: err.Error()}})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "detail": err.Error()})
	default:
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, http.StatusCreated, map[string]any{"id": p.ID, "title": p.Title, "admin": p.Admin})
	}
}

func (s *Server) authorize(r *http.Request) (auth.Scope, bool) {
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return s.deps.Tokens.Authorize(got)
}

type fieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// decodeBody 拒未知键，并把剩余 body 抽干再返回——否则同一连接上的
// 下一个请求会读到上一段的尾巴。
func decodeBody(r *http.Request, v any) error {
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxBodyBytes)) }()
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = store.WriteJSON(w, v)
}

func writeFieldErrors(w http.ResponseWriter, code int, errs []fieldError) {
	writeJSON(w, code, map[string]any{"code": code, "errors": errs})
}

// Task 11 的占位响应。刻意最简：它只服务鉴权与 cookie 的行为测试。
func writePlaceholder(w http.ResponseWriter, inner string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>launchbay</title></head><body>" + inner + "</body></html>"))
}
```

`cmd/launchbay/main.go` 改为：

```go
package main

import (
	"log"
	"net/http"

	"github.com/rockbenben/launchbay/internal/httpapi"
	"github.com/rockbenben/launchbay/internal/store"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	h := httpapi.New(httpapi.Dependencies{
		Store:  store.New(cfg.DataDir),
		Tokens: cfg.Tokens,
	}).Handler()
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: h}
	log.Printf("launchbay listening on :%s (data=%s)", cfg.Port, cfg.DataDir)
	log.Fatal(srv.ListenAndServe())
}
```

- [ ] **Step 6: 跑到通过**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 全 PASS

- [ ] **Step 7: 破坏验证（三处，逐条）**

1. 删掉 `handlePin` 里 `!home.Admin` 判断（只看会话形状）：Run `go test ./internal/httpapi/ -run TestForgedHomeDoesNotGrantAdmin` → Expected: FAIL。
2. 把 `!sess.Valid()` 分支删掉（非法会话直接渲染）：Run `go test ./internal/httpapi/ -run TestMalformedSessionIsClearedNotRendered` → Expected: FAIL。
3. 把 `CreateProfile` 成功分支的 `w.WriteHeader(http.StatusCreated)` 去掉：Run `go test ./internal/httpapi/ -run TestCreateProfileNeedsAdminToken` → Expected: FAIL（201 变 200）。
4. 把 `decodeBody` 的 `DisallowUnknownFields()` 注释掉：Run `go test ./internal/httpapi/ -run TestCreateProfileRejectsBadBody` → Expected: **仍 PASS**（因为 `extra` 那例也被 struct 无该字段拒掉？不——不拒未知键时它会静默忽略）。**如果这条真的变绿了，说明测试给的理由不成立**：`extra` 用例只依赖 `DisallowUnknownFields`，所以它必须变红。若它没变红，先查是不是 422 被 `id`/`title` 的报错抢先满足——那就把 `extra` 单独拆一条断言 `len(errors)==1 && field=="extra"`，再重跑。

恢复后跑总门禁。

- [ ] **Step 8: 提交**

```bash
git add internal/httpapi cmd
git commit -m "feat(http): 建档、PIN 验证与管理员免 PIN 切档

权限只看 lb_profile；管理员身份回查 profile.json 的 admin 字段，
不信 cookie 形状——伪造 lb_home 换不到免 PIN 切档。
非法会话清 cookie 回选择页，不渲染书签也不给报错页。
422 逐条点出字段，PIN 与书签内容都不进响应。"
```

---

### Task 10: 渲染器 —— 缓存键带档案，永不串号

**Files:**
- Create: `internal/render/render.go`
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: `store.Store.Current(id)`、`LoadProfile`、`ListProfiles`、`BrokenReason`
- Produces:
  - `render.Page{Body []byte; ETag string; Profile string; Rev int64; NoStore bool}`（`Profile` 为 `"picker"` 表示选择页）
  - `render.New(s *store.Store) *render.Renderer`
  - `(*Renderer).ProfilePage(id string) (Page, error)`
  - `(*Renderer).PickerPage() (Page, error)`
  - `(*Renderer).Invalidate(id string)`、`(*Renderer).CacheSize() int`

- [ ] **Step 1: 写失败的测试**

`internal/render/render_test.go`：

```go
package render

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

func seeded(t *testing.T) *store.Store {
	t.Helper()
	s := store.New(t.TempDir())
	for _, id := range []string{"dad", "mom"} {
		if _, err := s.CreateProfile(store.NewProfileInput{ID: id, Title: id, PinHash: "sha256$a$b"}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func save(t *testing.T, s *store.Store, id, title, itemID string) {
	t.Helper()
	b := store.EmptyBookmarks()
	b.Rev = 5
	b.Title = title
	b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList, Items: []model.Item{
		{ID: itemID, Name: itemID + " 台", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
	}}}
	if err := s.SaveBookmarks(id, b); err != nil {
		t.Fatal(err)
	}
	// 让两个档案的 mtime 一定不同，否则缓存键会真的撞。
	time.Sleep(30 * time.Millisecond)
}

// 本计划最关键的一条：两档案 rev 相同。只按 rev 做键的话，
// 第二个档案会命中第一个的页面——这就是"串号"。
func TestTwoProfilesSameRevDoNotCrossContaminate(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	save(t, s, "mom", "妈妈的台", "emby")
	r := New(s)

	dad, err := r.ProfilePage("dad")
	if err != nil {
		t.Fatal(err)
	}
	mom, err := r.ProfilePage("mom")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dad.Body), "爸爸的台") || strings.Contains(string(dad.Body), "妈妈的台") {
		t.Fatalf("dad 页面串了:\n%s", dad.Body)
	}
	if !strings.Contains(string(mom.Body), "妈妈的台") || strings.Contains(string(mom.Body), "爸爸的台") {
		t.Fatalf("mom 页面串了:\n%s", mom.Body)
	}
	if dad.ETag == mom.ETag {
		t.Fatalf("两档案 ETag 相同，浏览器缓存会跨档案命中: %s", dad.ETag)
	}
	if dad.Profile != "dad" || mom.Profile != "mom" {
		t.Fatalf("Page.Profile 要给 handler 写 X-LB-Profile 用: %q %q", dad.Profile, mom.Profile)
	}
}

// 内容字节完全相同的两个档案，ETag 也必须不同。
func TestIdenticalContentStillDistinctETag(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "同一份", "dsm")
	src, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	clone := *src
	if err := s.SaveBookmarks("mom", &clone); err != nil {
		t.Fatal(err)
	}
	r := New(s)
	dad, _ := r.ProfilePage("dad")
	mom, _ := r.ProfilePage("mom")
	if string(dad.Body) != string(mom.Body) {
		t.Fatal("夹具本应字节相同")
	}
	if dad.ETag == mom.ETag {
		t.Fatal("ETag 要带档案名，内容相同也不许跨档案相同")
	}
}

func TestCacheHitAndExternalEditInvalidates(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "第一版", "dsm")
	r := New(s)

	first, err := r.ProfilePage("dad")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := r.ProfilePage("dad")
	if first.ETag != second.ETag {
		t.Fatal("同内容不该产生不同 ETag")
	}
	if r.CacheSize() == 0 {
		t.Fatal("没进缓存")
	}

	// 外部手改基线文件（rev 不变）：mtime 变了就必须失效重算。
	raw := strings.Replace(string(mustRaw(t, s, "dad")), "第一版", "第二版", 1)
	time.Sleep(30 * time.Millisecond)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte(raw)); err != nil {
		t.Fatal(err)
	}
	third, err := r.ProfilePage("dad")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(third.Body), "第二版") {
		t.Fatalf("手改基线后仍在给旧内容:\n%s", third.Body)
	}
	if third.ETag == first.ETag {
		t.Fatal("内容变了 ETag 却没变")
	}
}

func TestInvalidateDropsOnlyThatProfile(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "甲", "dsm")
	save(t, s, "mom", "乙", "emby")
	r := New(s)
	if _, err := r.ProfilePage("dad"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ProfilePage("mom"); err != nil {
		t.Fatal(err)
	}
	if r.CacheSize() != 2 {
		t.Fatalf("cache = %d, want 2", r.CacheSize())
	}
	r.Invalidate("dad")
	if r.CacheSize() != 1 {
		t.Fatalf("Invalidate 该只清一个档案, cache = %d", r.CacheSize())
	}
}

func TestBrokenProfileSurfacesReason(t *testing.T) {
	s := seeded(t)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{\n \"rev\": 2,\n")); err != nil {
		t.Fatal(err)
	}
	r := New(s)
	_, err := r.ProfilePage("dad")
	if err == nil {
		t.Fatal("坏档案要报错，让 handler 走坏档案页")
	}
	if !strings.Contains(err.Error(), "第 ") {
		t.Fatalf("错误要带行号: %v", err)
	}
}

func TestPickerPageListsNamesOnly(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	r := New(s)
	p, err := r.PickerPage()
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)
	if !strings.Contains(body, "dad") {
		t.Fatalf("选择页要列出档案: %s", body)
	}
	// 关键隐私断言：选择页一个条目、一个 URL、一个分组名都不许有。
	for _, forbidden := range []string{"10.0.0.3", "爸爸的台", "NAS", "dsm 台"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("选择页泄露了 %q:\n%s", forbidden, body)
		}
	}
	if p.Profile != "picker" {
		t.Fatalf("Profile = %q", p.Profile)
	}
	if !p.NoStore {
		t.Fatal("选择页也要 NoStore 级隐私处理")
	}
}

func mustRaw(t *testing.T, s *store.Store, id string) []byte {
	t.Helper()
	b, err := os.ReadFile(s.BookmarksPath(id))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
```

- [ ] **Step 2: 跑到它失败**

Run: `go test ./internal/render/ -v`
Expected: 编译失败 `undefined: New`

- [ ] **Step 3: 实现渲染器**

`internal/render/render.go`：

```go
// Package render 把一个档案渲染成完整 HTML。渲染在服务端做，
// 因此首屏不需要 JS、数据直接内联，关掉脚本也能看（spec §7.2）。
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

type Page struct {
	Body   []byte
	ETag   string
	// Profile 是 handler 写 X-LB-Profile 的依据：SW 读不到 cookie，
	// 只能靠这个响应头决定把页面存进哪个档案的缓存（spec §7.1）。
	Profile string
	Rev     int64
	NoStore bool // 选择页与坏档案页：跨档案命中就是隐私事故
}

type cacheKey struct {
	profile  string
	rev      int64
	modified time.Time
}

type Renderer struct {
	s     *store.Store
	tmpl  *executor
	mu    sync.Mutex
	cache map[cacheKey]Page
}

func New(s *store.Store) *Renderer {
	return &Renderer{s: s, tmpl: newExecutor(), cache: map[cacheKey]Page{}}
}

func (r *Renderer) CacheSize() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cache)
}

func (r *Renderer) Invalidate(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.cache {
		if k.profile == id {
			delete(r.cache, k)
		}
	}
}

func (r *Renderer) ProfilePage(id string) (Page, error) {
	snap, err := r.s.Current(id)
	if err != nil {
		return Page{}, err
	}
	key := cacheKey{profile: id, rev: snap.Rev, modified: snap.Modified}
	r.mu.Lock()
	if p, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return p, nil
	}
	r.mu.Unlock()

	prof, err := r.s.LoadProfile(id)
	if err != nil {
		return Page{}, err
	}
	body, err := r.tmpl.profile(prof, snap.B)
	if err != nil {
		return Page{}, err
	}
	page := Page{Body: body, ETag: makeETag(id, body), Profile: id, Rev: snap.Rev}

	r.mu.Lock()
	// 只留最近若干页：NAS 上长跑不该让内存单调涨。
	if len(r.cache) > 32 {
		r.cache = map[cacheKey]Page{}
	}
	r.cache[key] = page
	r.mu.Unlock()
	return page, nil
}

func (r *Renderer) PickerPage() (Page, error) {
	profiles, err := r.s.ListProfiles()
	if err != nil {
		return Page{}, err
	}
	// 选择页只取名字与 id：条目、URL、分组名一个都不进视图。
	var views []pickerEntry
	for _, p := range profiles {
		views = append(views, pickerEntry{ID: p.ID, Title: p.Title, Letter: firstChar(p.Title)})
	}
	body, err := r.tmpl.picker(views)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, ETag: makeETag("picker", body), Profile: "picker", NoStore: true}, nil
}

// ETag 带档案名：内容字节相同时也必须不同名，否则浏览器与 SW
// 会跨档案命中——那是本设计里最贵的一类 bug。
func makeETag(profile string, body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + profile + "-" + hex.EncodeToString(sum[:])[:16] + `"`
}

func firstChar(s string) string {
	for _, r := range s {
		return string(r)
	}
	return "?"
}

var _ = model.LayoutList
```

- [ ] **Step 4: 写模板执行器（先给能跑通测试的最小版，Task 11 补完整界面）**

`internal/render/exec.go`：

```go
package render

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/rockbenben/launchbay/internal/model"
)

type executor struct{ profileT, pickerT *template.Template }

func newExecutor() *executor {
	funcs := template.FuncMap{"firstChar": firstChar}
	return &executor{
		profileT: template.Must(template.New("profile").Funcs(funcs).Parse(profileTmpl)),
		pickerT:  template.Must(template.New("picker").Funcs(funcs).Parse(pickerTmpl)),
	}
}

type pickerEntry struct {
	ID     string
	Title  string
	Letter string
}

func (e *executor) profile(p *model.Profile, b *model.Bookmarks) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.profileT.Execute(&buf, struct {
		Profile   *model.Profile
		Bookmarks *model.Bookmarks
	}{p, b}); err != nil {
		return nil, fmt.Errorf("渲染档案 %s: %w", p.ID, err)
	}
	return buf.Bytes(), nil
}

func (e *executor) picker(entries []pickerEntry) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.pickerT.Execute(&buf, entries); err != nil {
		return nil, fmt.Errorf("渲染选择页: %w", err)
	}
	return buf.Bytes(), nil
}
```

`internal/render/tmpl.go`（模板文本，Task 11 会把它们移进 `web/` 并 embed）：

```go
package render

const profileTmpl = `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Bookmarks.Title}}</title></head>
<body>
<header><h1>{{.Bookmarks.Title}}</h1></header>
<main role="main">
{{range .Bookmarks.Groups}}<section><h2>{{.Title}}</h2><ul>
{{range .Items}}<li><a href="{{.URL}}"><span aria-hidden="true">{{firstChar .Name}}</span>{{.Name}}</a></li>
{{end}}</ul></section>{{end}}
</main>
{{with .Bookmarks.Footer}}<footer>{{.}}</footer>{{end}}
</body></html>`

const pickerTmpl = `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>谁来用？</title></head>
<body>
<main role="main"><h1>谁来用？</h1>
<form method="post" action="/pin" id="pinf">
  <input type="hidden" name="profile" id="pf-profile" value="">
  <input inputmode="numeric" autocomplete="one-time-code" name="pin" id="pf-pin" placeholder="4-6 位 PIN">
  <button type="submit">进入</button>
</form>
<ul class="profiles">
{{range .}}<li><button type="button" class="pick" data-id="{{.ID}}"><span aria-hidden="true">{{.Letter}}</span>{{.Title}}</button></li>
{{end}}</ul>
</main>
</body></html>`
```

- [ ] **Step 5: 跑到通过**

Run: `go test ./internal/render/ -v && gofmt -l .`
Expected: 全 PASS

- [ ] **Step 6: 破坏验证（本计划最重要的一次）**

把 `cacheKey` 改成只留 `rev int64` 一个字段（删 `profile` 与 `modified`，并同步改 `ProfilePage` 里构造键的那行）：

Run: `go test ./internal/render/ -run TestTwoProfilesSameRev`
Expected: FAIL —— 第二个档案命中了第一个档案的页面

这一步验证的是"串号"这个真实故障确实会被测试抓住。恢复。

再把 `makeETag` 改成只用 `sha256(body)`（去掉档案名）：

Run: `go test ./internal/render/ -run TestIdenticalContentStillDistinctETag`
Expected: FAIL

恢复后：`go test ./... -race` 全绿。

- [ ] **Step 7: 提交**

```bash
git add internal/render
git commit -m "feat(render): 按 (档案,rev,mtime) 缓存页面，ETag 带档案名

键带 mtime 让外部手改基线也失效；ETag 带档案名让两份内容完全相同的
档案不会跨档案命中。测试里故意把键改成只按 rev，确认第二个档案读到
第一个的页面——串号必须被测试抓住。选择页只出名字，条目与 URL 不进视图。"
```

---

### Task 11: 完整首屏：响应头、三档断点、坏档案页、非 http 协议标记

**Files:**
- Create: `internal/render/web/profile.tmpl`
- Create: `internal/render/web/picker.tmpl`
- Create: `internal/render/web/base.css`
- Create: `internal/render/assets.go`（`go:embed`）
- Modify: `internal/render/tmpl.go`（删内联常量）、`internal/render/exec.go`
- Modify: `internal/httpapi/handlers.go`（换掉占位响应、加响应头与 304）
- Test: `internal/render/views_test.go`、`internal/httpapi/render_test.go`

**Interfaces:**
- Consumes: `render.New/ProfilePage/PickerPage`（Task 10）、`store.Store.BrokenReason`
- Produces:
  - `render.View` 与 `render.NewProfileView(p *model.Profile, b *model.Bookmarks) View`
  - `httpapi.Dependencies{Store, Tokens, Render *render.Renderer}`
  - 响应头：`X-LB-Profile`、`ETag`、`Cache-Control: private, no-cache`、`Content-Security-Policy`

- [ ] **Step 1: 写失败的测试（渲染内容与隐私）**

`internal/render/views_test.go`：

```go
package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

func TestProfileStructureAndNoJS(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	body := string(mustRender(t, s, "dad"))
	for _, want := range []string{
		`<main`, `role="main"`,    // 地标
		`<h2 id="g-nas">NAS</h2>`, // 分组标题成结构（Task 10 的 save 夹具里 group Title 就是 "NAS"）
		`href="http://10.0.0.3:5000"`, // 关掉 JS 也能点
		`dsm 台`,                  // save() 造的条目标题是 itemID+" 台"，不是"群晖"
		`aria-hidden="true"`,      // 首字母色块是装饰
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("缺 %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "<script") {
		t.Fatalf("首屏不许依赖 JS:\n%s", body)
	}
	if strings.Contains(body, "onerror=") || strings.Contains(body, "javascript:") {
		t.Fatalf("出现内联事件或伪协议:\n%s", body)
	}
}

func TestLetterFallbackAndDescAndAlias(t *testing.T) {
	s := seeded(t)
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "x", Title: "x", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Groups = []model.Group{{ID: "g", Title: "组", Layout: model.LayoutList, Items: []model.Item{
		{ID: "emby", Name: "Emby 影音", URL: "http://10.0.0.3:8098", Desc: "影视", Alias: []string{"em"}, Tags: []string{}},
		{ID: "qinglong", Name: "青龙面板", URL: "http://10.0.0.3:5700", Alias: []string{}, Tags: []string{"定时"}},
	}}}
	if err := s.SaveBookmarks("x", b); err != nil {
		t.Fatal(err)
	}
	body := string(mustRender(t, s, "x"))
	for _, want := range []string{">E<", ">青<", "影视", "定时"} {
		if !strings.Contains(body, want) {
			t.Fatalf("缺 %q:\n%s", want, body)
		}
	}
}

// chrome:// 之类的条目：浏览器禁止从网页跳转，要标出来并给复制出路。
func TestNonHTTPSchemeMarked(t *testing.T) {
	s := seeded(t)
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "x", Title: "x", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Groups = []model.Group{{ID: "g", Title: "组", Layout: model.LayoutList, Items: []model.Item{
		{ID: "exts", Name: "扩展管理", URL: "chrome://extensions/", Alias: []string{}, Tags: []string{}},
	}}}
	if err := s.SaveBookmarks("x", b); err != nil {
		t.Fatal(err)
	}
	body := string(mustRender(t, s, "x"))
	if !strings.Contains(body, "需手动打开") {
		t.Fatalf("非 http/https 条目要标明打不开:\n%s", body)
	}
	if !strings.Contains(body, "chrome://extensions/") {
		t.Fatal("URL 要原样保留展示")
	}
	// 不许把它做成一个会静默失败的可点链接。
	if strings.Contains(body, `href="chrome://`) {
		t.Fatalf("不许给非 http 协议渲染 href:\n%s", body)
	}
}

func TestEscapesUserData(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Title = "<img src=x onerror=alert(1)>"
	b.Groups = []model.Group{{ID: "g", Title: `<script>bad()</script>`, Layout: model.LayoutList,
		Items: []model.Item{{ID: "i", Name: `a"b`, URL: "http://10.0.0.3/", Desc: "<b>粗</b>", Alias: []string{}, Tags: []string{}}}}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	body := string(mustRender(t, s, "dad"))
	if strings.Contains(body, "<script>bad()</script>") || strings.Contains(body, "onerror=alert") {
		t.Fatalf("用户数据没转义:\n%s", body)
	}
	if !strings.Contains(body, "&lt;") {
		t.Fatal("要看到转义后的实体")
	}
}

// 档案名与内容都可控，模板不许把 id 拼进可执行上下文。
func TestAdminEntryOnlyForAdmin(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "管理员台", "dsm")
	save(t, s, "mom", "普通人台", "emby")
	r := New(s)
	dadPage, _ := r.ProfilePage("dad")
	momPage, _ := r.ProfilePage("mom")
	if !strings.Contains(string(dadPage.Body), "/admin") {
		t.Fatal("管理员档案要能看到管理入口")
	}
	if strings.Contains(string(momPage.Body), "/admin") {
		t.Fatal("非管理员不该有管理入口")
	}
}

func mustRender(t *testing.T, s *store.Store, id string) []byte {
	t.Helper()
	p, err := New(s).ProfilePage(id)
	if err != nil {
		t.Fatal(err)
	}
	return p.Body
}
```

- [ ] **Step 2: 写失败的测试（响应头）**

`internal/httpapi/render_test.go`：

```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

func setupRender(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	s := store.New(t.TempDir())
	hash, _ := auth.HashPIN("1234")
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "dad", Title: "爸爸", PinHash: hash, Admin: true}); err != nil {
		t.Fatal(err)
	}
	b := store.EmptyBookmarks()
	b.Title = "爸爸的台"
	b.Groups = []model.Group{{ID: "nas", Title: "NAS 应用", Layout: model.LayoutList, Items: []model.Item{
		{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
	}}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	sc, _ := auth.ParseScopes("admin:tok-admin:*")
	return New(Dependencies{Store: s, Tokens: sc, Render: render.New(s)}).Handler(), s
}

该测试文件的 import 需要 `"github.com/rockbenben/launchbay/internal/model"`（setupRender 里造夹具用了 `model.Group` / `model.Item`）。

func TestProfilePageHeaders(t *testing.T) {
	h, _ := setupRender(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("code %d body %.200s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("X-LB-Profile"); got != "dad" {
		t.Fatalf("X-LB-Profile = %q", got)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") || !strings.Contains(cc, "private") {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("缺 ETag")
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("CSP = %q", csp)
	}
	if !strings.Contains(rec.Body.String(), "爸爸") {
		t.Fatalf("要渲染出档案页: %.200s", rec.Body)
	}
}

func TestPickerHasItsOwnProfileHeader(t *testing.T) {
	h, _ := setupRender(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := rec.Header().Get("X-LB-Profile"); got != "picker" {
		t.Fatalf("选择页的 X-LB-Profile = %q", got)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("选择页 Cache-Control = %q（跨档案命中就是泄露）", cc)
	}
}

func TestIfNoneMatchReturns304(t *testing.T) {
	h, _ := setupRender(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	first := httptest.NewRecorder()
	h.ServeHTTP(first, req)
	etag := first.Header().Get("ETag")

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req2.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	req2.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	h.ServeHTTP(second, req2)

	if second.Code != http.StatusNotModified {
		t.Fatalf("want 304, got %d", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Fatal("304 不许带 body")
	}
}

// 坏档案：只坏那一个，页面要给出可读原因而不是白屏或全站 500。
func TestBrokenProfileShowsReason(t *testing.T) {
	h, s := setupRender(t)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{\n \"rev\": 1,\n \"groups\": [\n")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "dad"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "dad"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("坏档案要给解释页而不是 5xx, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "读不了") || !strings.Contains(body, "第 4 行") {
		t.Fatalf("要给文件名行号与人话解释:\n%.400s", body)
	}
	if strings.Contains(body, "admin:tok-admin") || strings.Contains(body, "sha256$") {
		t.Fatal("解释页不许把令牌或 PIN 哈希抖出来")
	}
}

func TestUnknownProfileInCookieFallsBackToPicker(t *testing.T) {
	h, _ := setupRender(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieProfile, Value: "ghost"})
	req.AddCookie(&http.Cookie{Name: auth.CookieHome, Value: "ghost"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-LB-Profile") != "picker" {
		t.Fatalf("指向不存在的档案要回选择页, got %q", rec.Header().Get("X-LB-Profile"))
	}
}
```

- [ ] **Step 3: 跑到它失败**

Run: `go test ./internal/render/ ./internal/httpapi/ -v`
Expected: `TestProfileStructureAndNoJS` 与响应头测试 FAIL（现模板缺 `role`、缺 desc/tags、缺非 http 标记；handler 还在给占位页）

- [ ] **Step 4: 把模板搬进 `internal/render/web/` 并 embed**

spec §3 的 `web/` 单元落在这里：`go:embed` 不许跨父目录，所以前端源放在 `internal/render/web/`。

`internal/render/assets.go`：

```go
package render

import "embed"

//go:embed web
var webFS embed.FS
```

`internal/render/web/base.css`（本期只要"能用、可读、焦点可见"；极光玻璃在 P1-④）：

```css
:root{color-scheme:light dark;--bg:#0d1017;--ink:#e9edf8;--ink2:#a7b2cd;--line:rgba(255,255,255,.12);--card:rgba(255,255,255,.06)}
@media (prefers-color-scheme:light){:root{--bg:#f7f5f0;--ink:#22201c;--ink2:#5d584e;--line:#e7e1d5;--card:#fff}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:15px/1.6 -apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;padding:20px}
header{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:18px}
header h1{font-size:19px;margin:0}
main{max-width:1180px;margin:0 auto}
section{margin:22px 0}
h2{font-size:13px;letter-spacing:.1em;text-transform:uppercase;color:var(--ink2);margin:0 0 10px}
ul{list-style:none;margin:0;padding:0;display:grid;gap:10px;grid-template-columns:repeat(auto-fill,minmax(190px,1fr))}
li a,li .plain{display:flex;align-items:center;gap:10px;padding:12px;border:1px solid var(--line);
  border-radius:14px;background:var(--card);color:inherit;text-decoration:none;min-height:56px}
li .ic{width:28px;height:28px;flex:0 0 auto;border-radius:9px;background:#4c8dff;color:#fff;
  display:grid;place-items:center;font-size:13px;font-weight:700}
li b{font-weight:600}
li i{font-style:normal;font-size:12px;color:var(--ink2);margin-left:auto}
.badge{font-size:11px;border:1px solid #ff9d6c;color:#ffb980;border-radius:6px;padding:2px 6px}
.profiles{grid-template-columns:repeat(auto-fill,minmax(150px,1fr))}
.pick{cursor:pointer;font:inherit;color:inherit;background:var(--card);border:1px solid var(--line);
  border-radius:14px;padding:16px;display:flex;align-items:center;gap:10px}
a:focus-visible,button:focus-visible,input:focus-visible{outline:3px solid #7fa8ff;outline-offset:2px}
@media (prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}}
@media (max-width:700px){ul{grid-template-columns:repeat(2,1fr)}}
```

`internal/render/web/profile.tmpl`：

```html
{{define "profile"}}<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Bookmarks.Title}}</title>
<style>{{.CSS}}</style>
</head>
<body>
<header>
  <h1>{{.Bookmarks.Title}}</h1>
  <nav aria-label="档案">
    <a class="badge" href="/switch">{{.Profile.Title}}</a>
    {{if .Profile.Admin}}<a class="badge" href="/admin">管理</a>{{end}}
  </nav>
</header>
<main role="main">
{{if not .Bookmarks.Groups}}
  <p>这个档案还没有书签。</p>
{{end}}
{{range .Bookmarks.Groups}}
<section aria-labelledby="g-{{.ID}}">
  <h2 id="g-{{.ID}}">{{.Title}}</h2>
  <ul>
  {{range .Items}}
    <li>
      {{if .External}}
        <span class="plain">
          <span class="ic" aria-hidden="true">{{.Letter}}</span>
          <b>{{.Name}}</b>
          <span class="badge">需手动打开</span>
          <code>{{.URL}}</code>
        </span>
      {{else}}
        <a href="{{.URL}}"{{if $.Bookmarks.OpenInNewTab}} target="_blank" rel="noopener noreferrer"{{end}}>
          <span class="ic" aria-hidden="true">{{.Letter}}</span>
          <b>{{.Name}}</b>
          {{with .Desc}}<i>{{.}}</i>{{end}}
        </a>
      {{end}}
    </li>
  {{end}}
  </ul>
</section>
{{end}}
</main>
{{with .Bookmarks.Footer}}<footer><p>{{.}}</p></footer>{{end}}
</body>
</html>{{end}}
```

`internal/render/web/picker.tmpl`：

```html
{{define "picker"}}<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>谁来用？</title>
<style>{{.CSS}}</style>
</head>
<body>
<main role="main">
  <h1>谁来用？</h1>
  <ul class="profiles">
  {{range .Entries}}
    <li><button type="button" class="pick" data-id="{{.ID}}"><span class="ic" aria-hidden="true">{{.Letter}}</span><b>{{.Title}}</b></button></li>
  {{end}}
  </ul>
  <form method="post" action="/pin">
    <label>档案 <input name="profile" id="pf" value="{{.Preselect}}" required></label>
    <label>PIN <input name="pin" type="password" inputmode="numeric" autocomplete="off" minlength="4" maxlength="6" required></label>
    {{with .KeepAdd}}<input type="hidden" name="keep_add" value="{{.}}">{{end}}
    <button type="submit">进入</button>
  </form>
</main>
</body>
</html>{{end}}
```

`internal/render/exec.go` 换成从 embed 解析，并给模板准备视图数据（把 `Letter`、`External` 这些渲染判据算在 Go 里，模板不做逻辑）：

```go
package render

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

type executor struct {
	profileT *template.Template
	pickerT  *template.Template
	cssT     template.CSS
}

type itemView struct {
	ID       string
	Name     string
	URL      string
	Desc     string
	Letter   string
	External bool // 非 http/https：浏览器不让从网页跳过去
}

type groupView struct {
	ID    string
	Title string
	Items []itemView
}

type profileView struct {
	Profile   *model.Profile
	Bookmarks profileData
	CSS       template.CSS
}

type profileData struct {
	Title        string
	Footer       string
	OpenInNewTab bool
	Groups       []groupView
}

func newExecutor() *executor {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		panic(fmt.Errorf("读内嵌 base.css: %w", err))
	}
	// CSS 是我们自己的静态文件，不是用户数据：用 template.CSS 内联进
	// <style> 换一次 RTT（spec §11 已记录 style-src 'unsafe-inline' 的取舍）。
	css := template.CSS(safeCSS(string(raw)))
	funcs := template.FuncMap{}
	parse := func(name string) *template.Template {
		data, err := webFS.ReadFile("web/" + name)
		if err != nil {
			panic(fmt.Errorf("读内嵌模板 %s: %w", name, err))
		}
		return template.Must(template.New(name).Funcs(funcs).Parse(string(data)))
	}
	return &executor{profileT: parse("profile.tmpl"), pickerT: parse("picker.tmpl"), cssT: css}
}

// safeCSS 挡一手：内嵌样式里出现 </style> 会截断标签，把后面的内容
// 变成 HTML。本仓库的 base.css 不该有这串，出现就该当构建错误。
func safeCSS(s string) string {
	if strings.Contains(strings.ToLower(s), "</style") {
		panic("base.css 里出现 </style，会截断内联 style 标签")
	}
	return s
}

type pickerView struct {
	Entries   []pickerEntry
	CSS       template.CSS
	Preselect string
	KeepAdd   string
}

func (e *executor) profile(p *model.Profile, b *model.Bookmarks) ([]byte, error) {
	v := profileView{Profile: p, CSS: e.cssT, Bookmarks: profileData{
		Title: b.Title, Footer: b.Footer, OpenInNewTab: b.OpenInNewTab,
	}}
	for _, g := range b.Groups {
		gv := groupView{ID: g.ID, Title: g.Title}
		for _, it := range g.Items {
			gv.Items = append(gv.Items, itemView{
				ID: it.ID, Name: it.Name, URL: it.URL, Desc: it.Desc,
				Letter:   firstChar(it.Name),
				External: !strings.HasPrefix(it.URL, "http://") && !strings.HasPrefix(it.URL, "https://"),
			})
		}
		v.Bookmarks.Groups = append(v.Bookmarks.Groups, gv)
	}
	var buf bytes.Buffer
	if err := e.profileT.ExecuteTemplate(&buf, "profile", v); err != nil {
		return nil, fmt.Errorf("渲染档案 %s: %w", p.ID, err)
	}
	return buf.Bytes(), nil
}

func (e *executor) picker(entries []pickerEntry, preselect, keepAdd string) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.pickerT.ExecuteTemplate(&buf, "picker", pickerView{
		Entries: entries, CSS: e.cssT, Preselect: preselect, KeepAdd: keepAdd}); err != nil {
		return nil, fmt.Errorf("渲染选择页: %w", err)
	}
	return buf.Bytes(), nil
}
```

`executor` 只留 `cssT` 一个字段（`template.CSS` 类型）：CSS 文本来自内嵌的 `base.css`，不是用户数据，才有资格直接进 `<style>`。

`render.Renderer.PickerPage` 签名扩成 `PickerPage(opts PickerOptions)`：

```go
type PickerOptions struct {
	Preselect string
	KeepAdd   string
}
```

测试里 `r.PickerPage()` 的调用同步改成 `r.PickerPage(PickerOptions{})`。

- [ ] **Step 5: handler 接渲染与响应头**

`internal/httpapi/server.go` 的 `Dependencies` 加 `Render *render.Renderer`。三处调用方必须同批改，否则要么编译不过、要么 `handleRoot` 摸到 nil 渲染器 panic：

1. `cmd/launchbay/main.go`：`httpapi.New(httpapi.Dependencies{Store: store.New(cfg.DataDir), Tokens: cfg.Tokens, Render: render.New(s)})`（先建 `s := store.New(cfg.DataDir)` 再传）。
2. Task 9 的 `internal/httpapi/handlers_test.go` 里 `setup(t)` 的 `New(Dependencies{...})` 补 `Render: render.New(s)` —— 那两条走 `GET /` 的测试（`TestMalformedSessionIsClearedNotRendered`）依赖它。
3. `writePlaceholder` / `handleRoot` 的占位分支整段删掉，别留死码。

`handleRoot` 换成：

```go
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// ?add= 是 bookmarklet 带进来的。PIN 步骤不许丢它：选择页里
	// 用 hidden 字段带上，验证成功后由前端（P1-④）或重定向接住。
	keepAdd := r.URL.Query().Get("add")

	sess := auth.ReadSession(r)
	if !sess.IsEmpty() && !sess.Valid() {
		auth.ClearSession(w)
		sess = auth.Session{}
	}
	if !sess.IsEmpty() {
		if _, err := s.deps.Store.LoadProfile(sess.Active); err != nil {
			auth.ClearSession(w)
			sess = auth.Session{}
		}
	}

	if sess.IsEmpty() {
		page, err := s.deps.Render.PickerPage(render.PickerOptions{KeepAdd: keepAdd})
		if err != nil {
			s.handleError(w, err)
			return
		}
		s.writePage(w, r, page)
		return
	}

	page, err := s.deps.Render.ProfilePage(sess.Active)
	if errors.Is(err, store.ErrCorrupt) {
		reason, _ := s.deps.Store.BrokenReason(sess.Active)
		s.writeBrokenProfile(w, r, sess, reason)
		return
	}
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.writePage(w, r, page)
}

func (s *Server) writePage(w http.ResponseWriter, r *http.Request, p render.Page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-LB-Profile", p.Profile)
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'")
	if p.NoStore {
		w.Header().Set("Cache-Control", "private, no-store")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("ETag", p.ETag)
		if r.Header.Get("If-None-Match") == p.ETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	_, _ = w.Write(p.Body)
}
```

`writeBrokenProfile` 渲染一段**不含任何书签、不含 PIN 哈希、不含令牌**的解释页，文本形如：

```
这个档案的数据文件读不了。
data/profiles/dad/bookmarks.json 第 4 行附近： unexpected end of JSON input
改完刷新即可；其它档案不受影响。
```

`handleError` 输出 5xx 时也只写 `err.Error()` 里的文件与行号信息，不写 env 值。

- [ ] **Step 6: 跑到通过**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: 全 PASS。Task 9 里那些断言占位页内容的测试要同步改为断言真实渲染（`TestMalformedSessionIsClearedNotRendered` 仍应过，因为非法会话清 cookie 后给的就是选择页）。

- [ ] **Step 7: 破坏验证**

1. 把 `External` 恒置为 `false`：Run `go test ./internal/render/ -run TestNonHTTPSchemeMarked` → Expected: FAIL（`href="chrome://` 出现，点了静默失败）。
2. 把 `writePage` 里 `p.NoStore` 分支删掉：Run `go test ./internal/httpapi/ -run TestPickerHasItsOwnProfileHeader` → Expected: FAIL（选择页可被缓存 → 别人在这台设备离线时可能看到）。
3. 把模板里 `{{.Name}}` 改成 `{{.Name | safe}}`（假设加了个恒等函数）——这条不需要真做，改而验证：把 `html/template` 换成 `text/template`：Run `go test ./internal/render/ -run TestEscapesUserData` → Expected: FAIL（注入生效）。**这条是本项目里最值得的一次破坏**：模板引擎换错只有一行代码差别，而后果是数据由人和 AI 共同写时的直接 XSS。

恢复后跑总门禁。

- [ ] **Step 8: 提交**

```bash
git add internal/render internal/httpapi
git commit -m "feat(render): 完整首屏模板、内嵌 CSS 与响应头

渲染判据算在 Go 里（External/Letter），模板只做排版。非 http 协议不
渲染 href——浏览器从网页跳 chrome:// 会静默失败，那比不点更坏。
选择页 no-store：跨档案命中缓存就是隐私事故。
X-LB-Profile 让读不到 cookie 的 SW 能决定把页面存进哪个档案的缓存。"
```

---

### Task 12: Docker、healthcheck 子命令、端到端验收

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.env.example`
- Create: `cmd/launchbay/healthcheck.go`
- Modify: `cmd/launchbay/main.go`（分发子命令）
- Create: `data/README.md`? —— **不要**：`data/` 已进 `.gitignore`。改在 `.env.example` 里注释说明。
- Test: `cmd/launchbay/healthcheck_test.go`
- Modify: `.gitignore`（确认含 `data/`、`.superpowers/`、`.qoder-credits/`）

**Interfaces:**
- Consumes: `loadConfig`、`httpapi.New`
- Produces: `launchbay`（服务模式）、`launchbay healthcheck`（探活模式，退出码 0/1）；镜像 `launchbay:<version>`；宿主端口 7600

- [ ] **Step 1: healthcheck 的失败测试**

`cmd/launchbay/healthcheck_test.go`：

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeOKOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if err := probe(srv.URL); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
}

func TestProbeFailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	if err := probe(srv.URL); err == nil {
		t.Fatal("want error on 500")
	}
}

func TestProbeFailsOnUnreachable(t *testing.T) {
	// 一个没人听的端口。1 秒超时要生效，否则容器会卡在 healthcheck 里。
	if err := probe("http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("want error")
	}
}
```

- [ ] **Step 2: 实现 healthcheck 子命令**

`cmd/launchbay/healthcheck.go`：

```go
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// probe 只做一次 GET。scratch 镜像里没有 shell，Docker 的 healthcheck
// 必须靠二进制自己完成，所以这条路径要短、要快、要能超时退出。
func probe(url string) error {
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz 返回 %d", resp.StatusCode)
	}
	return nil
}

// runHealthcheck 是 `launchbay healthcheck` 的入口。
func runHealthcheck(port string) int {
	if err := probe("http://127.0.0.1:" + port + "/healthz"); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck 失败: %v\n", err)
		return 1
	}
	return 0
}
```

`cmd/launchbay/main.go` 顶部加子命令分发（**`LAUNCHBAY_PORT` 要在两条路径都生效**，否则 healthcheck 探错端口，容器会被判死循环重启）：

```go
func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck(cfg.Port))
	}
	// ……服务模式照旧
}
```

Run: `go test ./cmd/... -v` → Expected: PASS

- [ ] **Step 3: Dockerfile**

```dockerfile
# 构建期：Go 静态编译。web/ 已用 go:embed 打进二进制，不需要 node 阶段。
FROM golang:1.27-alpine AS build
WORKDIR /src
# 分层缓存：先依赖后源码。改一行代码不该重下模块。
COPY go.mod ./
RUN go mod download
COPY . .
# 门禁在镜像构建里跑一遍：CI 绿不代表这个镜像可用。
RUN go vet ./... && go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/launchbay ./cmd/launchbay

FROM scratch
COPY --from=build /out/launchbay /launchbay
# 没有 shell：healthcheck 只能靠二进制自己的子命令（Task 12 Step 2）。
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
USER 65534:65534
EXPOSE 7600
ENTRYPOINT ["/launchbay"]
```

> `ca-certificates.crt` 这一行是给 P1-③ 的 favicon 抓取和 P1-⑤ 的天气预留的（scratch 里没有系统 CA，HTTPS 请求会全部失败）。本期还没有外部请求，但**现在就加上**，否则 P1-③ 会在容器里撞一个"本机好、容器里全失败"的坑。

- [ ] **Step 4: compose 与 .env.example**

`docker-compose.yml`：

```yaml
services:
  launchbay:
    build: .
    image: launchbay:0.1.0
    container_name: launchbay
    ports: ["7600:7600"]
    volumes: ["./data:/data"]
    environment:
      LAUNCHBAY_PORT: "7600"
      LAUNCHBAY_DATA_DIR: "/data"
      LAUNCHBAY_WRITE_TOKENS: "ai-dad:${LAUNCHBAY_TOKEN_AI_DAD}:dad,admin:${LAUNCHBAY_TOKEN_ADMIN}:*"
      TZ: Asia/Shanghai
    restart: unless-stopped
    healthcheck:
      # scratch 里没有 shell，所以走 CMD-signal 之外的方式：让二进制自己探。
      test: ["CMD", "/launchbay", "healthcheck"]
      interval: 30s
      timeout: 3s
      retries: 3
      start_period: 5s
```

`.env.example`（**只给形状，值一律假**）：

```dotenv
# 复制成 .env 填真值。.env 与 data/ 都不进仓库。
# 令牌三段式：name:secret:profile，profile 用 * 表示管理员。
LAUNCHBAY_TOKEN_AI_DAD=replace-me-1
LAUNCHBAY_TOKEN_ADMIN=replace-me-2
# 数据落在 NAS 的 ./data（容器内 /data）。整目录已在 .gitignore 里。
```

`.gitignore` 追加确认：

```gitignore
.superpowers/
.qoder-credits/
data/
.env
node_modules/
dist/
*.log
```

`.dockerignore`（**必须建**，否则 `COPY . .` 会把 `data/` 里的真实书签与 `.superpowers/` 里的设计稿一起打进镜像——既是体积灾难也是泄露面）：

```dockerignore
.git
.superpowers
.qoder-credits
data
.env
node_modules
dist
docs
*.log
Dockerfile
docker-compose.yml
```

- [ ] **Step 5: 本机起容器（Docker 引擎需要先起来）**

```bash
# 引擎没起时：只启动 GUI，别把它的 stdout 接进管道（GUI 进程继承句柄会挂）
powershell -NoProfile -Command "Start-Process 'C:\Program Files\Docker\Docker\Docker Desktop.exe'"
# 等引擎就绪（最多 ~90 秒）
for i in $(seq 1 30); do docker info >/dev/null 2>&1 && break; sleep 3; done
docker info >/dev/null 2>&1 || echo "Docker 引擎仍未就绪：先手动启动 Docker Desktop 再跑这一步"
```

```bash
cp .env.example .env   # 然后手工把两个 replace-me 改成真随机值
mkdir -p data          # 档案目录一律由 POST /api/v1/profiles 建，不要手工 mkdir data/profiles/<id>
docker build -t launchbay:0.1.0 .
docker image ls launchbay --format '{{.Repository}}:{{.Tag}} {{.Size}}'
docker compose up -d
```

Expected: 镜像体积 ≤ 20MB（spec §10 的预算）。**超了就是失败**，别用 `echo 差不多` 放过——先查是不是 `go build` 没带 `-ldflags="-s -w"`，或 `COPY . .` 把 `data/`、`node_modules/` 拖进了构建上下文（需要 `.dockerignore`）。

```bash
docker ps --filter name=launchbay --format '{{.Names}} {{.Status}}'
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7600/healthz
```

Expected: `healthy`；`200`

- [ ] **Step 6: 端到端手工验收（真打开浏览器，不是只看接口）**

```bash
# 建第一个档案（管理员）
curl -s -X POST http://127.0.0.1:7600/api/v1/profiles \
  -H "Authorization: Bearer $LAUNCHBAY_TOKEN_ADMIN" \
  -H 'Content-Type: application/json' \
  -d '{"id":"dad","title":"爸爸","pin":"1234"}'
# 模板：用文件写入工具把 Task 7 测试里的 tplNas 内容原样落成 ./data/templates/nas.json
# （compose 已把 ./data 挂进容器 /data，所以放进去就能被读到；该文件同样不进仓库）
ls -l data/templates/nas.json
curl -s -X POST http://127.0.0.1:7600/api/v1/profiles \
  -H "Authorization: Bearer $LAUNCHBAY_TOKEN_ADMIN" \
  -H 'Content-Type: application/json' \
  -d '{"id":"mom","title":"妈妈","pin":"5678","fromTemplate":"nas"}'
```

然后用真实浏览器（chrome-devtools MCP 或本机 Chrome）走完这条路径，**每一步都要看到预期、不许用"逻辑上应该"代替**：

1. 无痕窗口开 `http://127.0.0.1:7600/` → 看到「谁来用？」与两个档案名；**页面上一个网址都没有**。
2. 输 `dad` + `1234` → 进入爸爸档案。
3. 关掉 JavaScript（DevTools → Settings → Debugger 勾 Disable JavaScript）刷新 → 书签仍可见、仍可点（spec §7.2 的验收判据）。
4. `curl -I` 两次对比 `ETag` 稳定；带 `If-None-Match` 请求返回 `304`。
5. 同窗口切到妈妈档案 → 页面标题与内容全是妈妈的；切回来不残留。
6. 把 `data/profiles/mom/bookmarks.json` 手工改成半个 JSON → **只有妈妈档案变解释页**，爸爸的照常，解释页给出文件名与行号。
7. 截图三张（选择页、爸爸档案、坏档案解释页），逐张用 Read 看图（zoom=2 看细节），确认没有挤压、错位、对比度不足。

Run 完把这些现象写进提交信息或 PR 描述里——**没有实拍就不算验收过**。

- [ ] **Step 7: 提交**

```bash
git add Dockerfile docker-compose.yml .env.example .gitignore .dockerignore cmd
git commit -m "feat(deploy): 多阶段 Dockerfile、healthcheck 子命令与 compose

scratch 里没有 shell，探活只能让二进制自己做；端口从同一份 env 读，
否则 healthcheck 探错端口会把容器判死循环重启。
ca-certificates 现在就拷进去：不装系统 CA 的镜像里 P1-③ 的 https
抓取会全部失败，而本机测试全绿。"
```

---

## Spec 覆盖对账（本计划没覆盖的部分与去向）

按 spec 逐节核对，缺口都有明确去向，**不许出现"后面随手补"**：

| spec 节 | 本计划 | 缺口去向 |
|---|---|---|
| §4.1 目录布局 | ✅ Task 4/7/12 | `icons/` 在 P1-③ 建（favicon 缓存） |
| §4.2 profile.json | ✅ Task 3/6/7 | — |
| §4.3 bookmarks.json + 写出契约 | ✅ Task 2/3/5 | — |
| §4.4 ops 与 rev | ⚠️ 只有 `Save*` 与 `rev` 递增 | **P1-②**：ops/batch/校验先于落盘/`expectedRev` HTTP 层 |
| §4.5 模板克隆与 sync-template | ✅ 克隆在 Task 7 | **P1-②**：`sync-template` CLI（须走 REST） |
| §4.6 历史与恢复 | ❌ 只有 `HistoryPath()` | **P1-②**：`history.jsonl` 追加、快照环保留 50、`revert` |
| §5 图标双层 | ⚠️ 只有 `checkIcon` 的前缀形状 + 首字母兜底 | **P1-③**：name→path 索引内嵌、`/api/v1/icons`、SVG 内联、favicon 抓取 |
| §6.1 REST 契约 | ⚠️ 只实现 `/pin`、`/`、`/api/v1/profiles` | **P1-②**：其余端点 + `openapi.json` + 契约测试 |
| §6.2 校验先于落盘 | ✅ 单条数据层面（Task 5） | **P1-②**：ops 批量层面 |
| §6.3 令牌与 bookmarklet | ✅ 令牌 Task 6/9 | **P1-⑤**：bookmarklet |
| §6.4 AGENTS.md | ❌ | **P1-②**（写通路完成后才有内容可写） |
| §7.1 缓存头与 ETag | ✅ Task 11 | SW 部分 → **P1-④**（本期还没有 SW，`X-LB-Profile` 已备好） |
| §7.2 首屏构成 | ✅ Task 11（关 JS 可用） | 图标内联 → P1-③；JS 增强 → P1-④ |
| §7.3 性能预算 | ❌ 还没量 | **P1-④**：CI 预算闸（要有 CSS/JS 才有意义） |
| §7.5 可访问性 | ⚠️ 焦点环、地标、reduced-motion、触摸目标已在 Task 11 的 CSS/模板 | 对比度计算 + 三档实拍全量 → **P1-④** |
| §8 UI 规格（极光玻璃等） | ❌ 本期是朴素 CSS | **P1-④** |
| §8.4 就地编辑 / §8.5 添加浮层 / §8.6 ⌘K | ❌ | **P1-④** |
| §8.9 管理横幅 | ⚠️ `managing` 已在 `/pin` 响应里（由 `auth.Session.Managing()` 推导，唯一判据），DOM 未渲染 | **P1-④**（有真实页面才谈横幅渲染）。修正说明（最终整枝评审）：原表声称"本期由 Task 11 的模板留 `{{if .Managing}}` 位"不实——Task 11 没有留，也不该留：`ProfilePage` 的缓存键是 (profile, rev, mtime)，不含会话形态，把横幅烤进缓存页会让代管横幅跨会话串页。真横幅落地时连同缓存键一起设计，判据仍走 `Session.Managing()`。 |
| §9 flare 导入与对账 | ❌ | **P1-⑤** |
| §10 部署 | ✅ Task 12 | 接管 7570 由人工确认（spec §15） |
| §11 安全 | ✅ 全部落在 Task 4/6/7/9/11（拒未知键、slug 校验、CSP、转义、不签 cookie、限速不做） | 网段守卫进 CI → **P1-⑤**（CI 全套在那里） |
| §12 错误处理 | ✅ 坏档案隔离（Task 4/11） | 写失败结构化 → P1-②；离线队列 → P1-④ |
| §13 测试 | ✅ 含 6 次破坏验证 | 双档案离线端到端 → P1-④；预算闸 → P1-④ |
| §15 开放项：目录改名 | ❌ **未做** | 见下 |

**目录改名 `365/042` → `365/042-launchbay`**：本计划故意不含这一步——它必须由一个不持有该目录为工作根进程的会话来做。执行方式：在 `365` 目录下 `mv 042 042-launchbay`，然后以新路径重开会话。建议放在 P1-① 收尾、跑完全部门禁之后做一次；git 历史随目录一起走，无需重建仓库。

**接手 P1-② 前必须已完成的事实**（不要凭 P1-① 的记忆推断）：`go test ./...` 全绿、容器 `healthy`、Task 12 Step 6 的七条手工验收有实拍。



