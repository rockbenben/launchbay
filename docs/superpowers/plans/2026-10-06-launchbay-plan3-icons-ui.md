# P1-③ 图标索引、resolve 通路与首屏外观 Implementation Plan

> **本文是历史实施记录，不是待办。** 它记录的是那一天计划或量的是什么，里面的
> 函数名、测试名与接口形状**可能已经变了**——2026-10-10 的管理员边界收紧一次性改判掉了
> 代管态、模板机制与徽标切档，共享清单也改判为不做。**不要照着这里的命令执行**：
> 权威契约在仓库根的 `AGENTS.md`，设计在 `docs/superpowers/specs/`。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 launchbay 装上"图标名不许靠猜"的完整通路——服务端内嵌双层图标索引、`GET /api/v1/icons` 搜索、图标名只卡写路径的存在性校验、`POST /api/v1/resolve` 把 URL 解析成书签候选（含只经它触发的 favicon 抓取与内容哈希缓存），并把首屏图标真正内联进 HTML 与极光玻璃卡片外观。

**Architecture:** 新增叶子包 `internal/icons`（索引加载/校验/搜索 + 抓取内核），持久化侧新增 `internal/store/iconcache.go`（`data/icons/` 的内容哈希缓存与 origin 索引），`internal/ops` 的写校验多接一个显式的 `IconSet` 参数（读路径不接，索引漂移不许把好档案打成坏档），`internal/render` 拿到索引与缓存做三级兜底（内联 SVG → 缓存 favicon → 首字母色块）。三个新端点全部走既有唯一路由表 + `require` 凭证权威，契约三处同步由既有测试闸守着。

**Tech Stack:** Go 1.27（`CGO_ENABLED=0` 静态二进制，**零第三方依赖**）、标准库 `net/http` `html/template` `encoding/xml` `archive/tar` `compress/gzip` `go:embed`、无头 Chrome + CDP 做界面实拍、Docker 多阶段 → `scratch`。

**Spec:** `docs/superpowers/specs/2026-10-05-launchbay-design.md`（§5 图标系统、§6.1/§6.2 契约与校验、§7.1/§7.5 缓存与可访问性、§8.1 卡片外观、§10 部署、§11 风险、§13 测试）。本计划的每一条判据都从那里来；spec 已于 `a1e2de3` 按本期六处裁定修订，**计划与 spec 冲突时以 spec 为准并停下报告**。

---

## Global Constraints

每个任务的隐含要求，逐字来自 spec 与既有代码约定：

- **Go 1.27**，`CGO_ENABLED=0`；`go.mod` 里**不许出现 `require`**（本仓库零第三方依赖，抓取与解析一律标准库）。
- **JSON 写出契约（spec §4.3）**：缩进 2 空格、文件末尾恰好一个换行、`SetEscapeHTML(false)`、非 ASCII 原样输出、**空数组写 `[]` 绝不写 `null`**、键序由 struct 字段声明序固定、**绝不靠 map 遍历产生输出**。这条对磁盘文件与 API 响应体同样成立，所以本计划里所有响应体都是 struct，不允许 `map[string]any`。
- **运行时图标只有两层**：`tabler`（通用层，`stroke` 呈现）与 `simple-icons`（品牌层，`fill` 呈现）。`mdi` 只在 P1-⑤ 导入器里当别名源，不进运行时（spec §5）。
- **§7.5 硬数字**：触摸目标 ≥44px；磁贴内图标渲染尺寸下限 **20px**；tabler 是 2px 线条，**磁贴内 `stroke-width` 提到 2.5**。
- **§5 抓取约束（一条都不许松，也不许自己加）**：GET、**3 秒超时**、响应 **≤256KB**、图片 `Content-Type` 必须是 `image/*`、**重定向 ≤3**、非 2xx 即失败不解析、失败按 origin **24 小时冷却**。
- **§10 体积预算**：镜像 ≤20MB；本计划给索引产物另加一条自设红线——两套 gzip 产物合计 **> 3MB 即停手改判**（后备阶梯见 spec §5）。
- **明写不做**（spec §11，"多出来的防护也算失败"）：不签 cookie、不做速率限制/退避/锁定/失败计数、不做 TLS 终结、**不做"关闭私网"开关**、不给 `/icons/*` 加鉴权。
- **真实 NAS 网段与地址一个字面量都不许进仓库**。文档、注释、测试夹具只用 `10.` 段示例地址与 `127.0.0.1`（抓取测试必须走 `httptest` 的本机回环，**不许在测试里访问公网**）。
- **令牌只从 env `LAUNCHBAY_WRITE_TOKENS` 读**，不进日志、不回显、不进错误详情。
- **不许直接改 `data/` 下的任何文件**；书签写入只有 `POST /api/v1/batch` 一条通路（本计划不给 `data/icons/` 开第二条写入通路——它只由 `resolve` 内部写）。
- **契约三处同步**：`routes()` 是唯一注册来源，新增端点必须同时改 `internal/httpapi/openapi.json` 与 `AGENTS.md`，由 `TestAgentsMDMatchesRoutes`、`TestOpenAPICoversEveryRoute`、`TestOpenAPIHasNoGhostEndpoints`、`TestOpenAPIOperationShapes` 四条闸钉着。
- **TDD**：每个功能步先写失败测试并**看到它红**，再写实现；每条不变量测试都要做一次**【故意破坏】**，证明它会红，然后改回。
- **每任务收尾必跑的门禁**（顺序固定）：`go vet ./...`、`gofmt -l .`（输出必须为空）、`go test ./... -count=1`、`CGO_ENABLED=0 go build ./...`。
- **提交规矩**：按功能域一次提交，message 写实质不写过程词，禁署名；**push 先问人**。
- 中文注释只写"为什么"（既有代码风格）；不写行号引用，引用代码用 `file#符号` 形式。

---

## 文件结构

新增（`Create`）：

```
tools/genicons/main.go              图标索引生成器 + `-probe` 实测模式（构建期不参与）
tools/genicons/probe.go             npm tarball 取数 + 上游 SVG 元素拆解 + 词表统计
internal/icons/whitelist.go         白名单四件套与谓词（生产方与消费方共用，判据只有一份）
internal/icons/index/tabler.json.gz        提交进仓库的产物（实测 gzip 315KB）
internal/icons/index/simple-icons.json.gz  提交进仓库的产物（实测 gzip 1.81MB）
internal/icons/doc.go               包文档 + 产物 JSON 形状说明（唯一一份）
internal/icons/index.go             解码、校验、Has/Get/Canonical/Count/Names、Merge
internal/icons/load.go              //go:embed 两套 gz + Load/MustLoad
internal/icons/search.go            Search：精确 > 前缀 > 包含，alias 归一到规范名
internal/icons/fetch.go             Fetcher：页面与图片抓取（§5 全部约束）
internal/icons/extract.go           HTML → title / og:description / favicon link（不留正文）
internal/store/iconcache.go         data/icons/ 内容哈希缓存 + origin 索引 + 24h 冷却 + 代次
internal/httpapi/iconsearch.go      GET /api/v1/icons
internal/httpapi/iconfile.go        GET /icons/{name}（内容哈希图片通路）
internal/httpapi/resolve.go         POST /api/v1/resolve（含建议分组打分）
internal/render/icon.go             三级兜底决策 + 受控 SVG 拼接（唯一拼 markup 的地方）
internal/render/helpers_test.go     渲染测试夹具的唯一装配点（New 改三参数时只改这里）
internal/httpapi/testing_test.go    测试装配唯一入口（newDeps + testIndex）
internal/icons/whitelist_test.go    谓词证人
internal/icons/index_test.go        解码与白名单（夹具驱动）
internal/icons/testfixture_test.go  夹具拼装 helper（唯一一份）
internal/icons/load_test.go         真实产物自证（含 tabler:podcast 必须不存在）
internal/icons/search_test.go       排序与 alias
internal/icons/fetch_test.go        抓取约束（httptest 夹具，只走 127.0.0.1）
internal/icons/extract_test.go      title/og/favicon 提取
internal/store/iconcache_test.go    缓存、冷却、代次、字节一致
internal/httpapi/iconsearch_test.go 搜索端点
internal/httpapi/iconfile_test.go      图片通路的形状与缓存头
internal/httpapi/resolve_test.go    端点分流（401/403/404/422/502/503）+ 分组打分
internal/render/icon_test.go        三级兜底的 DOM 形状
```

修改（`Modify`）：

```
internal/ops/ops.go                 Run 第三参数 IconSet；add/set 两条路检查 icon 名
internal/ops/*_test.go              所有调用点补参数（约二十处）
internal/httpapi/server.go          Dependencies 增 Icons/Fetch/Cache；路由注册
internal/httpapi/routes.go          三条新路由（含 /icons/{name}）
internal/httpapi/admin.go           /status 行加 iconUnmatched 与总计
internal/httpapi/handlers_test.go   装配改走 newDeps
internal/httpapi/render_test.go     装配改走 newDeps
internal/httpapi/server_test.go     装配改走 newDeps
internal/httpapi/openapi.json       icons/resolve 去 planned、补形状、加 /icons/{name}
internal/httpapi/openapi_test.go    planned 豁免表随上线清空
internal/httpapi/batch.go           ops.Run 调用点接上真索引
cmd/launchbay/main.go               装配索引与缓存，读不到即退出
cmd/launchbay/clisync_test.go       装配补字段
internal/render/render.go           New 收 index/cache；cacheKey 加 iconGen
internal/render/exec.go             executor 收 index/cache，itemView 带图标
internal/render/web/profile.tmpl    三级兜底渲染 + 卡片结构
internal/render/web/base.css        极光玻璃外观 + §7.5 硬数字 + tiles/list + theme
internal/model/check.go             checkIcon 注释指向写路径闸门（行为不变）
internal/model/model_test.go        读路径不认索引的反向证人
internal/store/load_test.go         陌生图标名不进坏档的证人
internal/httpapi/admin_test.go      /status 的 iconUnmatched 用例
internal/render/web/picker.tmpl     data-theme 与卡片类名
.dockerignore                       排除 tools/（生成器带网络，不该进镜像）
.gitattributes                       *.json.gz binary（换行转换会毁产物）
AGENTS.md                           端点表三行 + 图标段（撤"待 P1-③"的免责句）
```

**依赖顺序**：0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → **11** → 8 → 9 → 10 → 12。Task 11 提到 Task 8 之前是有理由的：`resolve` 的响应里那条 `favicon: "/icons/<hash>.<ext>"` 指向的就是 Task 11 的路由，按原编号执行会让中间状态端出一个 404 链接——静态通路只依赖 Task 7 的缓存命名，不依赖 resolve，所以它能提前。Task 12 是唯一的端到端关口，前面任何任务没绿就别往它走。每个任务都以"门禁 + 一次功能域提交"结尾。

---

## Task 0: 生成器骨架与上游实测（`-probe`）

**Files:**
- Create: `tools/genicons/main.go`
- Create: `tools/genicons/probe.go`
- Create: `tools/genicons/helpers.go`（`clip`/`sortCount`/`topN`/`fatal`/`jsonMust`/`gzipBytes`）
- Create: `tools/genicons/build.go`（`productDoc`/`buildSet`/`encodeElements`/`marshalProduct`/`licenseOf` 在本任务落地；只有**写盘**是会响的桩，Task 2 补那一步）
- Create: `internal/icons/whitelist.go`
- Create: `internal/icons/whitelist_test.go`（谓词的证人：Task 1 之前这几个函数没有消费者，没有测试就是纯死码）

**Interfaces:**
- Consumes: 无（纯外部数据源）
- Produces: `genicons -probe` 打印上游词表与体积；`icons.IsDrawable/IsKeptAttr/ValidAttrValue/IsLayerDefaultFill` 与包内 `drawableTags/keepAttrs/attrValueRE/layerFillDefaults`（Task 1 的 `Decode` 与 Task 2 的生成器共用，判据只有一份）；`npmPkg`、`collectionDoc`、`elementsOf` 由 Task 2 的 `buildAndWrite` 复用。

**为什么先做这一步**：spec §5 的产物格式（只存 path 的 `d`）建立在一个假设上——上游 body 里除了 `path` 没别的元素。2026-10-06 用 `@iconify-json` tarball 实测的结论是：**假设基本成立但不完全**，tabler 有 `g`/`use`/`defs`/`circle`/`rect`。必须先量再写白名单，否则生成器会静默丢掉元素、把三个图标画成半个。

- [ ] **Step 1: 

```go
// tools/genicons/probe.go
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// npmPkg 是一个 @iconify-json 套的下载入口。用 npm registry 而不是
// api.iconify.design/collection：后者只回元数据加 4 个样本（实测 2026-10-06，
// tabler 那条响应 107KB 且 icons 为空），拿不到全量 body。
type npmPkg struct {
	Prefix string // tabler
	Pkg    string // @iconify-json/tabler
	Layer  string // stroke / fill，决定 CSS 怎么呈现
}

// collectionDoc 是 npm 包里 icons.json 的形状（只用得到的字段）。
type collectionDoc struct {
	Prefix  string `json:"prefix"`
	Total   int    `json:"total"`
	Aliases map[string]struct {
		Parent string `json:"parent"`
	} `json:"aliases"`
	Icons map[string]struct {
		Body string `json:"body"`
	} `json:"icons"`
}

// fetchIconsJSON 取 registry 元数据 → 下载 tarball → 掏 package/icons.json
// 与 package/info.json（后者带许可与 total，产物头部要用它）。
// 两份一起返回：分两次下载同一个 tarball 是纯粹的浪费，也是"许可读不到"
// 这种偷懒写法的原因。
func fetchIconsJSON(client *http.Client, p npmPkg, version string) (iconsJSON, infoJSON []byte, upstream string, err error) {
	metaRaw, err := httpGet(client, "https://registry.npmjs.org/"+p.Pkg)
	if err != nil {
		return nil, nil, "", err
	}
	var meta struct {
		DistTags map[string]string `json:"dist-tags"`
		Versions map[string]struct {
			Dist struct {
				Tarball string `json:"tarball"`
			} `json:"dist"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil, nil, "", fmt.Errorf("%s 的 registry 元数据读不了: %w", p.Pkg, err)
	}
	if version == "" {
		version = meta.DistTags["latest"]
	}
	v, ok := meta.Versions[version]
	if !ok {
		return nil, nil, "", fmt.Errorf("%s 没有版本 %s，latest 是 %s", p.Pkg, version, meta.DistTags["latest"])
	}
	tgz, err := httpGet(client, v.Dist.Tarball)
	if err != nil {
		return nil, nil, "", err
	}
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, nil, "", fmt.Errorf("%s 的 tarball 不是 gzip: %w", p.Pkg, err)
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, "", fmt.Errorf("读 tar: %w", err)
		}
		// npm 包内路径固定是 package/icons.json；按基名认，少一种漂移
		// 就少一次"CI 上路径分隔符不一样"的意外。
		switch path.Base(h.Name) {
		case "icons.json":
			iconsJSON, err = io.ReadAll(tr)
		case "info.json":
			infoJSON, err = io.ReadAll(tr)
		}
		if err != nil {
			return nil, nil, "", fmt.Errorf("读 %s: %w", h.Name, err)
		}
		if iconsJSON != nil && infoJSON != nil {
			return iconsJSON, infoJSON, p.Pkg + "@" + version, nil
		}
	}
	if iconsJSON == nil {
		return nil, nil, "", fmt.Errorf("tarball 里没有 icons.json")
	}
	return iconsJSON, infoJSON, p.Pkg + "@" + version, nil
}

// httpGet 不设超时上限给探针：抓取约束是**运行时**的（spec §5），构建期
// 跑生成器是一次性人工动作，用 60 秒兜住"网络挂了别卡死终端"。
func httpGet(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", "launchbay-genicons")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s -> HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
```

`probe.go` 的 import 是 `"archive/tar" "bytes" "compress/gzip" "encoding/json" "encoding/xml" "fmt" "io" "net/http" "path" "strings"`——`path` 与 `strings` 都要，前者认 tar 基名，后者喂 `strings.NewReader`。

- [ ] **Step 2: 

```go
// elementsOf 把上游 body 拆成元素序列。这里用 encoding/xml 而不是正则：
// 属性值里出现 `>` 的正则拆分会把一条图标切成两半，而这类图标恰好是
// 路径数据最长、最容易出事的那批。
func elementsOf(body string) ([]svgEl, error) {
	dec := xml.NewDecoder(strings.NewReader("<svg " + body + "</svg>"))
	dec.Strict = false // 上游有自闭合不一致的写法，宽松模式照样报错在下一步
	var out []svgEl
	var stack []string
	for {
		tk, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("SVG 解析失败: %w", err)
		}
		switch el := tk.(type) {
		case xml.StartElement:
			e := svgEl{Tag: el.Name.Local}
			for _, a := range el.Attr {
				e.Attrs = append(e.Attrs, attr{Name: a.Name.Local, Value: a.Value})
			}
			// 只有 g/svg 是容器。defs 与 use **不是**：产物里没有 id 引用体系，
			// 把它们当容器下钻会掏出一张谁都引用不到的孤图（画一半），
			// 所以原样留在结果里，由 encodeElements 把整条图标判为不支持。
			if isContainer(e.Tag) {
				stack = append(stack, e.Tag)
				continue
			}
			out = append(out, e)
		case xml.EndElement:
			if isContainer(el.Name.Local) && len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return out, nil
}

type svgEl struct {
	Tag   string
	Attrs []attr
}

type attr struct {
	Name  string
	Value string
}

func isContainer(tag string) bool { return tag == "g" || tag == "svg" }
```

- [ ] **Step 3: 

**判据只有一个来源。**白名单是"产物能装什么"的契约，消费方（Task 1 的 `Decode`）与生产方（本任务的生成器）必须共用它；两边各写一份的话，最先出现的症状是"生成器自称合法而解码器拒收"，那时没人会怀疑常量。

```go
// internal/icons/whitelist.go
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
	"fill": true, "fill-rule": true, "clip-rule": true, "opacity": true, "transform": true,
}

// attrValueRE 钉住"数值 + 路径指令字母"。它挡的是引号、尖括号与等号，
// 也就是所有能从属性值里逃出去的字符。渲染层还会再转义一次（Task 9）：
// 两道闸互不信任。
var attrValueRE = regexp.MustCompile(`^[A-Za-z0-9(),.\- ]+$`)

// layerFillDefaults 是每层由 CSS 负责的 fill 呈现。与默认值相同的 fill
// 不进产物：省下的每一字节都是重复常量，而"与默认不同"才是要留的信息
// （tabler 的 -filled 变体就靠这一条保住实心）。
var layerFillDefaults = map[string]string{"stroke": "none", "fill": "currentColor"}

// 生成器用的谓词。导出这四个而不是导出 map：契约的点在于"这几个问题"，
// 把 map 交出去只会诱导调用方绕过谓词自己判。
func IsDrawable(tag string) bool   { return drawableTags[tag] }
func IsKeptAttr(name string) bool  { return keepAttrs[name] }
func ValidAttrValue(v string) bool { return attrValueRE.MatchString(v) }
func IsLayerDefaultFill(layer, value string) bool {
	return layerFillDefaults[layer] == value
}
```

- [ ] **Step 4: 

```go
// tools/genicons/main.go
package main

func main() {
	var (
		probe   = flag.Bool("probe", false, "只实测上游词表与产物体积，不写文件")
		version = flag.String("version", "", "钉住 @iconify-json 的版本；空 = registry latest")
		only    = flag.String("prefix", "", "只处理某一个套（tabler / simple-icons）")
	)
	flag.Parse()

	sets := []npmPkg{
		{Prefix: "tabler", Pkg: "@iconify-json/tabler", Layer: "stroke"},
		{Prefix: "simple-icons", Pkg: "@iconify-json/simple-icons", Layer: "fill"},
	}
	if *only != "" {
		var filtered []npmPkg
		for _, p := range sets {
			if p.Prefix == *only {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) == 0 {
			fatal(fmt.Errorf("--prefix 只能是 tabler / simple-icons，得到 %q", *only))
		}
		sets = filtered
	}

	client := &http.Client{Timeout: 60 * time.Second}
	for _, p := range sets {
		raw, infoRaw, upstream, err := fetchIconsJSON(client, p, *version)
		if err != nil {
			fatal(err)
		}
		var doc collectionDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			fatal(fmt.Errorf("%s 的 icons.json 读不了: %w", p.Prefix, err))
		}
		if *probe {
			probeSet(p, doc, infoRaw, upstream)
			continue
		}
		if err := buildAndWrite(p, doc, infoRaw, upstream, *version); err != nil {
			fatal(err)
		}
	}
}

// probeSet 打印实测的四件事：条目数、元素与属性词表、要整条跳过的图标名，
// 以及按本任务的编码规则编出来的产物体积。词表决定白名单，体积决定要不要走
// spec §5 的后备阶梯——两个都是改判依据，不是日志装饰。
// 体积必须由本任务自己量：引用别人某一次的读数为红线设防，等于没有红线。
func probeSet(p npmPkg, doc collectionDoc, infoJSON []byte, upstream string) {
	tags := map[string]int{}
	attrs := map[string]int{}
	// 丢弃清单要报到图标名：只给个数字，下一个读产物的人会以为
	// "丢了 9 个元素"等于"少了 9 个图标"，而实际是三个图标画不全。
	for name, ic := range doc.Icons {
		els, err := elementsOf(ic.Body)
		if err != nil {
			fmt.Printf("%s:%s 解析失败：%v\n", p.Prefix, name, err)
			continue
		}
		for _, e := range els {
			tags[e.Tag]++
			for _, a := range e.Attrs {
				attrs[a.Name+"="+a.Value]++
			}
		}
	}
	// 产物与体积走同一条编码路径（buildSet），探针量到的就是最终提交的字节形状。
	entries, aliases, skipped, err := buildSet(p, doc)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("\n=== %s %s ===\n条目 %d（上游声明 total=%d）收录 %d\n",
		p.Prefix, upstream, len(doc.Icons), doc.Total, len(entries))
	fmt.Println("tag 词表:", sortCount(tags))
	fmt.Println("attr=值 词表（top 30）:", topN(sortCount(attrs), 30))
	sort.Strings(skipped)
	fmt.Printf("要整条跳过的图标 %d 个：%s\n", len(skipped), strings.Join(clip(skipped, 12), " "))

	raw, err := marshalProduct(p, doc, infoJSON, entries, aliases, upstream, time.Now().UTC())
	if err != nil {
		fatal(err)
	}
	gz, err := gzipBytes(raw)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("产物 raw %.2fMB  gzip %.1fKB（别名 %d 条）\n",
		float64(len(raw))/1048576, float64(len(gz))/1024, len(aliases))
}
```

`sortCount`/`topN`/`clip`/`fatal`/`jsonMust` 是二三十行的排序、截断与错误辅助，照 `cmd/launchbay/cli.go` 里同类输出打印的写法来；`fatal(err)` 打 stderr 后 `os.Exit(1)`。`probeSet` 里的 `icons` 是包名限定（`internal/icons`），别再在同一个函数里用 `icons` 当局部变量名——Go 会当场拒绝，这条提醒是为了少一次返工。

- [ ] **Step 5: 

`-probe` 要打印体积，所以编码（`buildSet`/`encodeElements`/`marshalProduct`/`licenseOf`）
必须是真代码；只有写盘那一步留成一个**会响的桩**。桩必须响：如果非 `-probe` 分支静默
成功，Task 2 的验收（产物已生成、体积达标、可复现）会被一次假绿蒙过去。
产物形状只有一份实现——探针量的字节与提交进仓库的字节因此不可能分家。

```go
// tools/genicons/build.go

// productDoc 与 internal/icons 的 doc 分属两个包、靠 json tag 对齐；
// Task 1 的 Decode 开了 DisallowUnknownFields，所以字段名一漂就会在
// buildAndWrite 的自校验里直接报错——这就是把自校验放在写盘之前的全部理由。
type productDoc struct {
	Prefix        string                     `json:"prefix"`
	Layer         string                     `json:"layer"`
	ViewBox       string                     `json:"viewBox"`
	Count         int                        `json:"count"`
	UpstreamTotal int                        `json:"upstreamTotal"`
	Upstream      string                     `json:"upstream"`
	GeneratedAt   string                     `json:"generatedAt"`
	License       string                     `json:"license"`
	Icons         map[string]json.RawMessage `json:"icons"`
	Aliases       map[string]string          `json:"aliases"`
}

// buildSet 是产物内容的唯一生成点：探针的体积读数与 Task 2 写盘的字节
// 都出自这里，所以两者不可能分家（这是本任务把编码提前落地的原因）。
func buildSet(p npmPkg, doc collectionDoc) (entries map[string]json.RawMessage,
	aliases map[string]string, skipped []string, err error) {
	entries = map[string]json.RawMessage{}
	aliases = map[string]string{}
	for name, ic := range doc.Icons {
		els, err := elementsOf(ic.Body)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s:%s 解析失败: %w", p.Prefix, name, err)
		}
		flat, ok := encodeElements(p, els)
		if !ok {
			skipped = append(skipped, name) // use/defs/未知元素：整条不要，不画半个
			continue
		}
		entries[name] = flat
	}
	// 别名只保留父名真的进了产物的那些：留着指向被跳过图标的别名，
	// Task 1 的解码器会判整份产物非法——宁可在生成时就少一条别名。
	for alias, rel := range doc.Aliases {
		if _, ok := entries[rel.Parent]; ok {
			aliases[alias] = rel.Parent
		}
	}
	return entries, aliases, skipped, nil
}

// encodeElements 把实测到的上游元素压成产物写法。ok=false 表示这条图标含产物格式
// 表达不了的东西（use 引用），调用方整条跳过并报告。
//
// 丢掉的属性都是本层默认值：stroke 层的 fill="none"、fill 层的 fill="currentColor"，
// 以及 stroke / stroke-width / stroke-linecap / stroke-linejoin（那四个由 CSS 全权
// 负责，§7.5 要磁贴内 2.5 就只改 CSS 一处）。
func encodeElements(p npmPkg, els []svgEl) (json.RawMessage, bool) {
	if len(els) == 0 {
		return nil, false
	}
	var kept [][]string
	for _, e := range els {
		if !icons.IsDrawable(e.Tag) {
			return nil, false
		}
		row := []string{e.Tag}
		for _, a := range e.Attrs {
			if !icons.IsKeptAttr(a.Name) {
				continue
			}
			if a.Name == "fill" && icons.IsLayerDefaultFill(p.Layer, a.Value) {
				continue
			}
			if !icons.ValidAttrValue(a.Value) {
				return nil, false
			}
			row = append(row, a.Name, a.Value)
		}
		kept = append(kept, row)
	}
	if len(kept) == 1 && len(kept[0]) == 3 && kept[0][0] == "path" && kept[0][1] == "d" {
		return jsonMust(kept[0][2]), true // 裸 d 写法
	}
	return jsonMust(kept), true
}

// marshalProduct 编出未压缩的产物 JSON。键序可预测：头部字段按 productDoc 的声明序，
// icons 的键由 encoding/json 按字典序输出。
func marshalProduct(p npmPkg, doc collectionDoc, infoJSON []byte,
	entries map[string]json.RawMessage, aliases map[string]string,
	upstream string, generatedAt time.Time) ([]byte, error) {
	out := productDoc{
		Prefix: p.Prefix, Layer: p.Layer, ViewBox: "0 0 24 24",
		Count: len(entries), UpstreamTotal: doc.Total, Upstream: upstream,
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339),
		License:     licenseOf(infoJSON, upstream),
		Icons:       entries, Aliases: aliases,
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '
'), nil
}

// licenseOf 从 info.json 的 license 字段取 SPDX 名（实测 MIT / CC0 1.0）。
// 取不到就写 "?"：留空会让这套图标的许可变成考古题。
func licenseOf(infoJSON []byte, upstream string) string {
	var info struct {
		License struct {
			Spdx string `json:"spdx"`
			Tit  string `json:"title"`
		} `json:"license"`
	}
	if len(infoJSON) > 0 {
		_ = json.Unmarshal(infoJSON, &info)
	}
	if info.License.Spdx != "" {
		return info.License.Spdx
	}
	if info.License.Tit != "" {
		return info.License.Tit
	}
	return "?"
}

// buildAndWrite 的写盘部分在本任务留桩，Task 2 替换。
func buildAndWrite(p npmPkg, doc collectionDoc, infoJSON []byte, upstream, pinned string) error {
	return fmt.Errorf("产物还没写盘（Task 2 落地）；本步只支持 -probe，它已经打印了体积")
}
```

`jsonMust`/`gzipBytes`/`clip`/`sortCount`/`topN`/`fatal` 在 `tools/genicons/helpers.go`：
十行上下的排序、截断与错误打印辅助，照 `cmd/launchbay/cli.go` 里同类输出打印的写法来；
`gzipBytes` 就是 `gzip.NewWriter` 包一层（level 9，`Name` 固定、`ModTime` 归零）。

- [ ] **Step 6: 

Run: `go run ./tools/genicons -probe`
Expected（2026-10-06 已实测一次，复跑应当一致或按上游发版小幅增长）:

```
=== tabler @iconify-json/tabler@1.2.41 ===
条目 6268（声明 total=6220）
tag 词表: path:8975 g:2302 use:6 defs:3 circle:2 rect:2
上游 icons.json 2134102 字节  产物 raw 1.47MB  gzip 315.5KB  aliases 194
必须跳过：defs/use 那 3 个图标

=== simple-icons @iconify-json/simple-icons@1.2.99 ===
条目 3737（声明 total=3464）
tag 词表: path:3737
上游 icons.json 4775072 字节  产物 raw 4.48MB  gzip 1.81MB  aliases 12
```

`条目 6268 vs 声明 total 6220` 是上游自己的口径差（两边的计法不同）。**产物自证只认"实际收录条数"**（Task 1 的 `count == len(icons)` 断言用前者），这个差值如实写进产物头部的 `count` 与 `upstreamTotal` 两个字段，不假装一致。

- [ ] **Step 7: 

Step 3 给的预期集已覆盖 2026-10-06 实测到的全部 tag 与 attr。若词表里冒出预期集之外的名字：**加进 `internal/icons/whitelist.go` 并在附录 A 记一行**（写清是哪个上游版本带来的），同时确认渲染层认得它——`Element` 已经是 tag+attrs 的通用形状，加 tag 不动模板。

**不许**把"不认识"实现成"跳过该属性/该元素"：那会让一个图标静默少一条线，首屏上没人知道它被切过，而白名单存在的唯一理由就是让这种事变成报错。

- [ ] **Step 8: 

`internal/icons/whitelist_test.go`：

```go
package icons

import "testing"

func Test白名单谓词真的在判事(t *testing.T) {
	if IsDrawable("use") || IsDrawable("script") {
		t.Error("use/script 不该算可绘制元素")
	}
	if !IsDrawable("path") || !IsDrawable("polyline") {
		t.Error("path/polyline 必须在白名单内")
	}
	if IsKeptAttr("stroke-width") || IsKeptAttr("onclick") {
		t.Error("呈现属性与事件属性都不该进产物")
	}
	if !IsKeptAttr("points") {
		t.Error("polyline 靠 points 活")
	}
	for bad := range map[string]bool{`"><script>`: true, "a b"c": true, "x='y'": true} {
		if ValidAttrValue(bad) {
			t.Errorf("ValidAttrValue(%q) 不该为真", bad)
		}
	}
	if !ValidAttrValue("M3 12h18") || !ValidAttrValue("currentColor") || !ValidAttrValue("rotate(45 12 12)") {
		t.Error("合法值被拒了：路径指令、颜色名、transform 都得放行")
	}
	// 层默认值：判错方向 = tabler 的实心变体被当成默认丢掉（外观错，无报错）
	if !IsLayerDefaultFill("stroke", "none") || IsLayerDefaultFill("stroke", "currentColor") {
		t.Error("stroke 层的默认 fill 判错了")
	}
	if !IsLayerDefaultFill("fill", "currentColor") || IsLayerDefaultFill("fill", "none") {
		t.Error("fill 层的默认 fill 判错了")
	}
}
```

- [ ] **Step 9: 

把 `IsLayerDefaultFill` 实现成 `return true` → 上面最后两组断言红。这是本任务唯一的新逻辑，
它必须在被 Task 1/2 消费之前就先证明自己会红。

- [ ] **Step 10: 

`go vet ./...`（`tools/genicons` 是 `package main`，会被扫到）、`gofmt -l .`、`go test ./... -count=1`、`CGO_ENABLED=0 go build ./...`。

再加一次"桩确实会响"的确认：

```bash
go run ./tools/genicons 2>&1 | tail -2   # 必须报错说产物生成还没落地
go run ./tools/genicons -probe | head -6 # 必须正常打印词表
```

```bash
git add tools/genicons internal/icons
git commit -m "feat(genicons): 图标索引生成器骨架、上游词表实测模式与共享白名单"
```

**注意**：`tools/` 不参与 `go build ./cmd/...`，但它带网络调用，所以 `.dockerignore` 要确认已经把 `tools/` 排除在外；没排除就在本步补一行（镜像里不该躺一个能出网的构建期工具）。

---
## Task 1: `internal/icons` 的产物格式、解码与校验器

**Files:**
- Create: `internal/icons/doc.go`
- Create: `internal/icons/index.go`
- Create: `internal/icons/index_test.go`
- Create: `internal/icons/testfixture_test.go`

**Interfaces:**
- Consumes: 无（本任务是叶子）
- Produces：

```go
type Attr struct{ Name, Value string }
type Element struct {
	Tag   string
	Attrs []Attr
}
type Icon struct {
	Prefix  string
	Name    string
	ViewBox string // "0 0 24 24"
	Layer   string // "stroke" | "fill"
	Elements []Element
}

type Index struct{ /* 未导出字段 */ }

func Decode(raw []byte) (*Index, error)          // 一套（未压缩 JSON）
func Merge(parts ...*Index) *Index               // 把两套并成一个查询面
func Load() (*Index, error)                      // 内嵌的两套 .json.gz
func MustLoad() *Index                           // 装配点用：读不了就是启动失败
func (x *Index) Has(icon string) bool            // "tabler:home"，alias 也算
func (x *Index) Get(icon string) (Icon, bool)    // alias 解析后返回规范条目
func (x *Index) Canonical(icon string) (string, bool)
func (x *Index) Count(prefix string) int         // 该层收录条数
func (x *Index) Names(prefix string) []string    // 已排序，搜索用
```

- [ ] **Step 1: 写包文档，把产物格式钉成唯一一份说明**

```go
// Package icons 是服务端内嵌的双层图标索引：tabler（通用层，线条）与
// simple-icons（品牌层，实心）。它做三件事——按名字给出 path 数据、按关键词
// 搜索候选、把 URL 抓成标题与 favicon（fetch.go）。
//
// 索引产物是提交进仓库的 gzip 文件（spec §5、§10）：构建期不联网、不装 node，
// 本地 go build、门禁跑的测试与镜像里嵌的是同一份字节。刷新图标库是人工跑
// `go run ./tools/genicons` 并重新提交，不是重建前端。
//
// 产物形状（一套一个文件，gzip 后的 JSON）：
//
//	{
//	  "prefix": "tabler",
//	  "layer": "stroke",                  // stroke | fill，决定 CSS 呈现
//	  "viewBox": "0 0 24 24",
//	  "count": 6268,                      // 实际收录条数：自证的锚
//	  "upstreamTotal": 6220,              // 上游声明的 total；口径差如实留着
//	  "upstream": "@iconify-json/tabler@1.2.41",
//	  "generatedAt": "2026-10-06T12:00:00Z",
//	  "license": "MIT",
//	  "icons": {
//	    "home": "M9 4H5a2 2 0 0 0-2 2v10a2 2 ...",
//	    "alarm-filled": [["path", "fill", "currentColor", "d", "M..."]]
//	  },
//	  "aliases": { "alarm-light": "alarm" }
//	}
//
// 一条图标有两种写法：单个 `<path>` 且没有任何偏离本层默认的呈现属性时，
// 值就是裸的 `d` 字符串；其余是元素数组，`[tag, 名, 值, 名, 值...]` 扁平排开。
// 之所以值得为常见情形省掉整个包装：simple-icons 的 3737 条全是单 path 且
// `fill="currentColor"`（实测 2026-10-06），那是本层的默认值，存下来等于把
// 1.8MB 花在重复同一个常量上。
package icons
```

- [ ] **Step 2: 写会失败的解码与校验测试**

```go
// internal/icons/testfixture_test.go
package icons

import (
	"encoding/json"
	"testing"
)

// fixtureDoc 拼一份最小合法产物。icons 的键直接就是产物里的键，
// 值可以是裸 d 字符串或元素数组——两种写法每条测试都要走一遍。
func fixtureDoc(t *testing.T, prefix, layer string, count int, icons map[string]any, aliases map[string]string) []byte {
	t.Helper()
	doc := map[string]any{
		"prefix":        prefix,
		"layer":         layer,
		"viewBox":       "0 0 24 24",
		"count":         count,
		"upstreamTotal": count,
		"upstream":      "fixture@0.0.0",
		"generatedAt":   "2026-10-06T00:00:00Z",
		"license":       "MIT",
		"icons":         icons,
		"aliases":       aliases,
	}
	if aliases == nil {
		doc["aliases"] = map[string]string{}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("夹具拼不出来: %v", err)
	}
	return raw
}

func mustDecode(t *testing.T, raw []byte) *Index {
	t.Helper()
	ix, err := Decode(raw)
	if err != nil {
		t.Fatalf("合法夹具不该报错: %v", err)
	}
	return ix
}
```

```go
// internal/icons/index_test.go
package icons

import (
	"strings"
	"testing"
)

func TestDecode_两种写法都收(t *testing.T) {
	raw := fixtureDoc(t, "tabler", "stroke", 2, map[string]any{
		"home": "M3 12h18",
		"bell": []any{[]any{"path", "d", "M6 18v-6"}, []any{"path", "d", "M18 18v-6"}},
	}, nil)
	ix := mustDecode(t, raw)

	if got := ix.Count("tabler"); got != 2 {
		t.Errorf("收录条数 = %d，要 2", got)
	}
	h, ok := ix.Get("tabler:home")
	if !ok {
		t.Fatal("tabler:home 查不到")
	}
	if len(h.Elements) != 1 || h.Elements[0].Tag != "path" || h.Elements[0].Attrs[0].Value != "M3 12h18" {
		t.Errorf("裸 d 写法解出来不对: %+v", h.Elements)
	}
	if h.Layer != "stroke" || h.ViewBox != "0 0 24 24" {
		t.Errorf("层与 viewBox 没带上: %+v", h)
	}
	b := mustGet(t, ix, "tabler:bell")
	if len(b.Elements) != 2 {
		t.Fatalf("两 path 图标应当各留一条，得到 %d", len(b.Elements))
	}
}

func TestDecode_自证count与实际条数(t *testing.T) {
	raw := fixtureDoc(t, "tabler", "stroke", 7, map[string]any{"home": "M0 0"}, nil)
	if _, err := Decode(raw); err == nil {
		t.Fatal("count 说 7 条实际 1 条，必须报错——这条断言防的就是解析器吞掉一半")
	} else if !strings.Contains(err.Error(), "count") {
		t.Errorf("错误里要点了 count，得到: %v", err)
	}
}

func TestDecode_未知tag未知attr与越界值都拒(t *testing.T) {
	cases := map[string]map[string]any{
		"未知 tag":      {"home": []any{[]any{"script", "src", "x"}}},
		"未知 attr":     {"home": []any{[]any{"path", "onclick", "alert(1)", "d", "M0 0"}}},
		"值里藏引号":      {"home": []any{[]any{"path", "d", `"/><script>alert(1)</script>`}}},
		"值里藏尖括号":    {"home": []any{[]any{"path", "d", "<svg onload=x>"}}},
		"空元素数组":     {"home": []any{}},
		"元素不成对":     {"home": []any{[]any{"path", "d"}}},
	}
	for name, icons := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(fixtureDoc(t, "tabler", "stroke", 1, icons, nil)); err == nil {
				t.Fatal("该拒的没拒")
			}
		})
	}
}

func TestDecode_fill只存与层默认值不同的那个(t *testing.T) {
	// stroke 层的默认是 fill:none；出现 currentColor 就是 filled 变体，必须留着，
	// 否则实心图标会被 CSS 描成空心——这是外观错误，不是省字节的对象。
	solid := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1,
		map[string]any{"home": []any{[]any{"path", "fill", "currentColor", "d", "M0 0"}}}, nil))
	el := mustGet(t, solid, "tabler:home").Elements[0]
	if len(el.Attrs) != 2 || el.Attrs[0].Name != "fill" {
		t.Errorf("stroke 层的 fill=\"currentColor\" 被丢了: %+v", el.Attrs)
	}
	// fill 层的默认是 currentColor：存进来只会被丢，别在产物里留它。
	if _, err := Decode(fixtureDoc(t, "simple-icons", "fill", 1,
		map[string]any{"github": []any{[]any{"path", "fill", "none", "d", "M0 0"}}}, nil)); err == nil {
		t.Error("fill 层出现 fill=\"none\" 是白名单外的情形，宁可报错也别静默改语义")
	}
}

func TestDecode_alias要指到真名字(t *testing.T) {
	if _, err := Decode(fixtureDoc(t, "tabler", "stroke", 1,
		map[string]any{"home": "M0 0"}, map[string]string{"house": "not-here"})); err == nil {
		t.Fatal("悬空 alias 必须报错")
	}
	ix := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1,
		map[string]any{"home": "M0 0"}, map[string]string{"house": "home"}))
	if !ix.Has("tabler:house") {
		t.Error("alias 应当算有")
	}
	if got, ok := ix.Canonical("tabler:house"); !ok || got != "tabler:home" {
		t.Errorf("Canonical 归一化 = %q %v，要 tabler:home true", got, ok)
	}
	// alias 不是第二个条目：Names 里只许出现规范名。
	for _, n := range ix.Names("tabler") {
		if n == "tabler:house" {
			t.Error("alias 混进了可写名单，磁盘上就会出现两种写法")
		}
	}
}

func TestDecode_层与viewBox形状(t *testing.T) {
	raw := fixtureDoc(t, "tabler", "gradient", 1, map[string]any{"home": "M0 0"}, nil)
	if _, err := Decode(raw); err == nil {
		t.Fatal("layer 只认 stroke/fill")
	}
}

func TestHas_空前缀与陌生前缀都是假(t *testing.T) {
	ix := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1, map[string]any{"home": "M0 0"}, nil))
	for _, bad := range []string{"", "auto", "home", "tabler", "tabler:", "mdi:home", "tabler:home-x", "tabler:HOME"} {
		if ix.Has(bad) {
			t.Errorf("Has(%q) 不该为真", bad)
		}
	}
}

func mustGet(t *testing.T, ix *Index, icon string) Icon {
	t.Helper()
	got, ok := ix.Get(icon)
	if !ok {
		t.Fatalf("%s 查不到", icon)
	}
	return got
}
```

- [ ] **Step 3: 跑测试确认它红**

Run: `go test ./internal/icons/ -count=1`
Expected: 编译失败（`undefined: Decode`）。**必须先看到这一步**——没有红的解码测试，白名单就只是注释。

- [ ] **Step 4: 写 index.go**

```go
package icons

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// viewBoxRE 钉住本层统一的视口形状。元素与属性白名单四件套
// （drawableTags / keepAttrs / attrValueRE / layerFillDefaults）在 Task 0 的
// internal/icons/whitelist.go 里，本文件只消费——生成器与解码器共用一份判据。
var viewBoxRE = regexp.MustCompile(`^[0-9. ]+$`)

type Attr struct {
	Name  string `json:"-"`
	Value string `json:"-"`
}

type Element struct {
	Tag   string
	Attrs []Attr
}

type Icon struct {
	Prefix   string
	Name     string
	ViewBox  string
	Layer    string
	Elements []Element
}

// Index 是两套合并后的查询面。它一旦装配就是只读的：渲染并发读它，
// 没有任何写路径，所以不带锁。
type Index struct {
	icons    map[string]Icon
	alias    map[string]string
	byPrefix map[string][]string
	counts   map[string]int
}

type doc struct {
	Prefix        string                      `json:"prefix"`
	Layer         string                      `json:"layer"`
	ViewBox       string                      `json:"viewBox"`
	Count         int                         `json:"count"`
	UpstreamTotal int                         `json:"upstreamTotal"`
	Upstream      string                      `json:"upstream"`
	GeneratedAt   string                      `json:"generatedAt"`
	License       string                      `json:"license"`
	Icons         map[string]json.RawMessage  `json:"icons"`
	Aliases       map[string]string           `json:"aliases"`
}

// Decode 解一套图标并全量校验。校验严格到"宁可整套不要"：一个坏属性值
// 如果被跳过，坏图标就会带着半截数据上首屏，那时没人知道它被切过。
func Decode(raw []byte) (*Index, error) {
	var d doc
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields() // 产物形状漂了要当场炸，不要静默少解一个字段
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("图标产物读不了: %w", err)
	}
	if d.Layer != "stroke" && d.Layer != "fill" {
		return nil, fmt.Errorf("%s 的 layer %q 只能是 stroke/fill", d.Prefix, d.Layer)
	}
	if !viewBoxRE.MatchString(d.ViewBox) {
		return nil, fmt.Errorf("%s 的 viewBox %q 形状非法", d.Prefix, d.ViewBox)
	}
	// 自证：声明条数与实际解出条数必须相等。上游 JSON 的键、解析器、
	// 校验循环任何一处吞了东西，这里就红——而不是首屏少几个图标没人发现。
	if d.Count != len(d.Icons) {
		return nil, fmt.Errorf("%s 的 count=%d 与实际收录 %d 条不符",
			d.Prefix, d.Count, len(d.Icons))
	}
	ix := &Index{
		icons:    make(map[string]Icon, len(d.Icons)),
		alias:    map[string]string{},
		byPrefix: map[string][]string{},
		counts:   map[string]int{},
	}
	for name, rawEl := range d.Icons {
		els, err := decodeElements(d, name, rawEl)
		if err != nil {
			return nil, err
		}
		key := d.Prefix + ":" + name
		ix.icons[key] = Icon{Prefix: d.Prefix, Name: name, ViewBox: d.ViewBox, Layer: d.Layer, Elements: els}
		ix.byPrefix[d.Prefix] = append(ix.byPrefix[d.Prefix], key)
	}
	for alias, parent := range d.Aliases {
		key := d.Prefix + ":" + alias
		target := d.Prefix + ":" + parent
		if _, ok := ix.icons[target]; !ok {
			return nil, fmt.Errorf("%s 的别名 %s 指向不存在的 %s", d.Prefix, alias, parent)
		}
		ix.alias[key] = target
	}
	ix.counts[d.Prefix] = len(d.Icons)
	for _, list := range ix.byPrefix {
		sort.Strings(list)
	}
	return ix, nil
}

// decodeElements 认两种写法：裸 d 字符串，或扁平元素数组。
func decodeElements(d doc, name string, rawEl json.RawMessage) ([]Element, error) {
	trim := strings.TrimSpace(string(rawEl))
	if strings.HasPrefix(trim, `"`) {
		var s string
		if err := json.Unmarshal(rawEl, &s); err != nil {
			return nil, fmt.Errorf("%s:%s 的裸 d 读不了: %w", d.Prefix, name, err)
		}
		return checkElements(d, name, []Element{{Tag: "path", Attrs: []Attr{{Name: "d", Value: s}}}})
	}
	var flat [][]string
	if err := json.Unmarshal(rawEl, &flat); err != nil {
		return nil, fmt.Errorf("%s:%s 既不是裸 d 也不是元素数组: %w", d.Prefix, name, err)
	}
	els := make([]Element, 0, len(flat))
	for _, e := range flat {
		if len(e) < 3 || (len(e)-1)%2 != 0 {
			return nil, fmt.Errorf("%s:%s 的元素要写成 [tag, 名, 值...]，得到 %v", d.Prefix, name, e)
		}
		el := Element{Tag: e[0]}
		for i := 1; i < len(e); i += 2 {
			el.Attrs = append(el.Attrs, Attr{Name: e[i], Value: e[i+1]})
		}
		els = append(els, el)
	}
	return checkElements(d, name, els)
}

// checkElements 是唯一的白名单执行点。属性顺序保留上游顺序：产物是
// 一次性生成的静态文件，稳定顺序由生成器排序保证，这里不重排。
func checkElements(d doc, name string, els []Element) ([]Element, error) {
	if len(els) == 0 {
		return nil, fmt.Errorf("%s:%s 没有任何可绘制元素", d.Prefix, name)
	}
	for _, el := range els {
		if !drawableTags[el.Tag] {
			return nil, fmt.Errorf("%s:%s 有白名单外的元素 %q", d.Prefix, name, el.Tag)
		}
		for _, a := range el.Attrs {
			if !keepAttrs[a.Name] {
				return nil, fmt.Errorf("%s:%s 有白名单外的属性 %q", d.Prefix, name, a.Name)
			}
			if !attrValueRE.MatchString(a.Value) {
				return nil, fmt.Errorf("%s:%s 的 %s 值形状非法", d.Prefix, name, a.Name)
			}
			if a.Name == "fill" {
				if a.Value != "currentColor" && a.Value != "none" {
					return nil, fmt.Errorf("%s:%s 的 fill %q 只认 currentColor/none", d.Prefix, name, a.Value)
				}
				if a.Value == layerFillDefaults[d.Layer] {
					return nil, fmt.Errorf("%s:%s 的 fill %q 与本层默认值相同，产物不该存它",
						d.Prefix, name, a.Value)
				}
			}
		}
	}
	return els, nil
}

// Merge 把几套并成一个查询面。前缀重叠是编程错误：两套同前缀的索引
// 意味着产物被复制过，静默后者覆盖前者只会让"哪条图标存在"变成猜谜。
func Merge(parts ...*Index) *Index {
	out := &Index{icons: map[string]Icon{}, alias: map[string]string{},
		byPrefix: map[string][]string{}, counts: map[string]int{}}
	for _, p := range parts {
		for k, v := range p.icons {
			if _, dup := out.icons[k]; dup {
				panic(fmt.Sprintf("图标 %s 在两份产物里都出现", k))
			}
			out.icons[k] = v
		}
		for prefix, list := range p.byPrefix {
			out.byPrefix[prefix] = append(out.byPrefix[prefix], list...)
		}
		for k, v := range p.alias {
			out.alias[k] = v
		}
		for prefix, n := range p.counts {
			out.counts[prefix] += n
		}
	}
	for prefix := range out.byPrefix {
		sort.Strings(out.byPrefix[prefix])
	}
	return out
}

// Has 回答"这个名字能不能写进磁盘"。空串与 auto 不在这里判——它们是
// 语义（"没设"/"抓 favicon"），不是索引成员，判在 ops 那一层（Task 4）。
func (x *Index) Has(icon string) bool {
	_, ok := x.Canonical(icon)
	return ok
}

func (x *Index) Canonical(icon string) (string, bool) {
	if icon == "" {
		return "", false
	}
	if _, ok := x.icons[icon]; ok {
		return icon, true
	}
	if target, ok := x.alias[icon]; ok {
		return target, true
	}
	return "", false
}

func (x *Index) Get(icon string) (Icon, bool) {
	key, ok := x.Canonical(icon)
	if !ok {
		return Icon{}, false
	}
	return x.icons[key], true
}

func (x *Index) Count(prefix string) int { return x.counts[prefix] }

func (x *Index) Names(prefix string) []string {
	out := x.byPrefix[prefix]
	cp := make([]string, len(out))
	copy(cp, out)
	return cp
}

// ErrNoIndex 让 Load 的失败可分诊：索引读不出来是装配错误，
// 不是运行时数据问题，装配点应当直接退出而不是降级成"没有图标"。
var ErrNoIndex = errors.New("图标索引读不出来")
```

- [ ] **Step 5: 跑测试确认它绿**

Run: `go test ./internal/icons/ -count=1`
Expected: PASS。

- [ ] **Step 6: 【故意破坏】证明白名单真的在咬**

三次独立破坏，每次都要看到对应的测试变红，再改回：

1. 把 `drawableTags` 里的 `"path": true,` 删掉 → `TestDecode_两种写法都收` 红（元素白名单没被 `d` 属性绕开）。
2. 把 `attrValueRE` 改成 `^.*$` → `TestDecode_未知tag未知attr与越界值都拒` 的"值里藏引号"子例红。**这一步是本案最要紧的一次破坏**：它证明那条正则不是装饰。
3. 把自证判断改成 `if d.Count < 0` → `TestDecode_自证count与实际条数` 红。

```bash
go test ./internal/icons/ -count=1 -run '自证|未知' # 每次破坏都跑，确认 FAIL
git diff -- internal/icons/index.go                 # 确认已经改回，diff 为空
```

- [ ] **Step 7: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/icons
git commit -m "feat(icons): 双层图标索引的产物格式、解码与白名单校验"
```

---

## Task 2: 生成两套 gzip 产物并入库

**Files:**
- Modify: `tools/genicons/main.go`（`buildAndWrite`）
- Modify: `tools/genicons/probe.go`（元素→产物编码）
- Create: `internal/icons/load.go`（`//go:embed` + `Load`/`MustLoad`）
- Create: `internal/icons/load_test.go`
- Create: `internal/icons/index/tabler.json.gz`
- Create: `internal/icons/index/simple-icons.json.gz`

**Interfaces:**
- Consumes: Task 1 的 `Decode`（生成器用它自校验自己的输出）、Task 0 的 `buildSet`/`marshalProduct`/`encodeElements`（同一段编码，不重写第二份）
- Produces: `icons.Load() (*Index, error)`、`icons.MustLoad() *Index`（Task 5 的装配、Task 9 的渲染都从这里拿）

- [ ] **Step 1: 写真实索引的自证测试（此刻必然红：产物还不存在）**

```go
// internal/icons/load_test.go
package icons

import "testing"

// 这三条把"快照里到底有什么"钉成断言，不是钉成一次人工查看：
// spec §5 的实测结论（podcast 不存在、brand-* 十个缺九）如果哪天变成假话，
// 要由测试来说，而不是由某个人某天点出一个空白图标来说。
func TestLoad_真实产物(t *testing.T) {
	ix := MustLoad()

	if n := ix.Count("tabler"); n < 6000 {
		t.Errorf("tabler 收录 %d 条，少于 6000，产物像是被截了", n)
	}
	if n := ix.Count("simple-icons"); n < 3400 {
		t.Errorf("simple-icons 收录 %d 条，少于 3400", n)
	}

	for _, known := range []string{
		"tabler:home", "tabler:download", "tabler:server", "tabler:certificate",
		"tabler:broadcast",
		"simple-icons:github", "simple-icons:jellyfin", "simple-icons:portainer",
		"simple-icons:syncthing", "simple-icons:homeassistant",
	} {
		if !ix.Has(known) {
			t.Errorf("%s 应当存在——它是本案常用的服务名", known)
		}
	}

	// spec §5 明写"图标名不许靠猜"的那两条实测，钉成冻结断言。
	// broadcast 曾在计划里被当成"不存在"，实测证伪：1.2.41 里 tabler:broadcast 是有的，
	// spec §5 那句"不许靠猜"只点过 podcast。计划与快照不符时以快照为准。
	for _, absent := range []string{"tabler:podcast", "mdi:home", "simple-icons:docker-alt"} {
		if ix.Has(absent) {
			t.Errorf("%s 居然存在了：要么上游改了口径要么产物错位，先查再放行", absent)
		}
	}

	// 每条图标都要能被渲染层真正消费：元素非空、属性合法。
	for _, prefix := range []string{"tabler", "simple-icons"} {
		for _, key := range ix.Names(prefix) {
			got, ok := ix.Get(key)
			if !ok || len(got.Elements) == 0 {
				t.Fatalf("%s 解出来是空的", key)
			}
		}
	}
}

func TestLoad_两套的层标记不同(t *testing.T) {
	ix := MustLoad()
	if got := mustHave(t, ix, "tabler:home").Layer; got != "stroke" {
		t.Errorf("tabler 层标成 %q，CSS 会拿错呈现", got)
	}
	if got := mustHave(t, ix, "simple-icons:github").Layer; got != "fill" {
		t.Errorf("simple-icons 层标成 %q", got)
	}
}

func mustHave(t *testing.T, ix *Index, key string) Icon {
	t.Helper()
	got, ok := ix.Get(key)
	if !ok {
		t.Fatalf("%s 查不到", key)
	}
	return got
}
```

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/icons/ -count=1 -run 'TestLoad'`
Expected: FAIL——`//go:embed index/*.json.gz` 还没有，或 `MustLoad` 报 `ErrNoIndex`。

- [ ] **Step 3: 把 Task 0 的会响桩换成真落盘**

编码（`productDoc`/`buildSet`/`encodeElements`/`marshalProduct`/`licenseOf`）与体积读数都在
Task 0 的 `tools/genicons/build.go` 里落地了，`-probe` 已经用过它们。本步**只**把
`buildAndWrite` 那个会响的桩换成真写盘——不新增编码器、不改 `marshalProduct` 一个字：
同一份产物形状有两处实现，就等于让「探针量的体积」与「提交进仓库的字节」可以分家。

```go
// buildAndWrite：编码 → 用消费方的解码器读回来自校验 → 才写盘。
// 顺序反过来（先写再测）等于把"产物能不能被消费"交给 CI 去发现：
// 生成器 exit 0 而首屏空白，是最坏的一种"成功"。
func buildAndWrite(p npmPkg, doc collectionDoc, infoJSON []byte, upstream, pinned string) error {
	entries, aliases, skipped, err := buildSet(p, doc) // Task 0 的唯一编码路径
	if err != nil {
		return err
	}
	sort.Strings(skipped)
	raw, err := marshalProduct(p, doc, infoJSON, entries, aliases, upstream, time.Now())
	if err != nil {
		return err
	}
	if _, err := icons.Decode(raw); err != nil {
		return fmt.Errorf("%s 的产物自己都不认: %w", p.Prefix, err)
	}
	gzPath := filepath.Join("internal", "icons", "index", p.Prefix + ".json.gz")
	if err := os.MkdirAll(filepath.Dir(gzPath), 0o755); err != nil {
		return err
	}
	gz, err := gzipBytes(raw) // helpers.go：level 9，Name 固定成 <prefix>.json，ModTime 归零
	if err != nil {
		return err
	}
	if err := os.WriteFile(gzPath, gz, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: 收录 %d 条，跳过 %d 条（%s），raw %d 字节 -> gzip %d 字节\n",
		p.Prefix, len(entries), len(skipped), strings.Join(clip(skipped, 6), " "), len(raw), len(gz))
	return nil
}
```

`gzipBytes` 必须真的把 `ModTime` 归零、`Name` 固定：gzip 头默认塞生成时刻，
每次生成都差一截字节，Step 7 的可复现性检查会假红。


- [ ] **Step 4: 落 embed 与 Load**

```go
// internal/icons/load.go
package icons

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
)

//go:embed index/tabler.json.gz index/simple-icons.json.gz
var indexFS embed.FS

// loadOrder 是解析顺序：品牌层在前，这样"simple-icons 的产物坏了"会先于
// tabler 报出来（品牌层是本案主力，坏得更早该更早知道）。搜索结果里的
// 层优先级由 search.go 自己的 prefixRank 决定，不靠这里的顺序（Task 3）。
var loadOrder = []string{"simple-icons", "tabler"}

func Load() (*Index, error) {
	var parts []*Index
	for _, prefix := range loadOrder {
		raw, err := indexFS.ReadFile("index/" + prefix + ".json.gz")
		if err != nil {
			return nil, fmt.Errorf("%w: 内嵌的 %s 产物不见了: %w", ErrNoIndex, prefix, err)
		}
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: %s 的 gzip 头坏了: %w", ErrNoIndex, prefix, err)
		}
		jsonBytes, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("%w: %s 解压失败: %w", ErrNoIndex, prefix, err)
		}
		if err := zr.Close(); err != nil {
			return nil, fmt.Errorf("%w: %s 收尾失败: %w", ErrNoIndex, prefix, err)
		}
		ix, err := Decode(jsonBytes)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrNoIndex, prefix, err)
		}
		parts = append(parts, ix)
	}
	return Merge(parts...), nil
}

// MustLoad 在装配点与测试里用：索引是内嵌产物，读不出来只有两种可能
// （embed 路径写错、产物损坏），两种都该让进程起来不了。
func MustLoad() *Index {
	ix, err := Load()
	if err != nil {
		panic(err)
	}
	return ix
}
```

- [ ] **Step 5: 生成产物**

```bash
go run ./tools/genicons
go test ./internal/icons/ -count=1
```

Expected: 两行统计（tabler 收录 ~6265 条、gzip ~320KB；simple-icons 3737 条、gzip ~1.8MB），`TestLoad_*` 全绿。

- [ ] **Step 6: 体积闸门（红线在这里，不是注释）**

```bash
ls -l internal/icons/index/                 # 两套合计必须 < 3MB
du -sh internal/icons/index/
CGO_ENABLED=0 go build -o /tmp/lb ./cmd/launchbay && ls -l /tmp/lb
```

Expected: gzip 合计约 2.1MB（实测 315KB + 1.81MB），破 3MB 即**停手**并走 spec §5 的后备阶梯（① 砍 aliases ② 砍 tabler 的 `-filled`/`-alt` 派生变体 ③ 才谈构建期生成）。镜像 ≤20MB 在 Task 12 复量一次。

- [ ] **Step 7: 可复现性检查**

再跑一次生成器，产物字节只差 `generatedAt`：

```bash
cp internal/icons/index/tabler.json.gz /tmp/tabler.a.gz
go run ./tools/genicons -prefix tabler
gzip -dc internal/icons/index/tabler.json.gz > /tmp/tabler.new.json
gzip -dc /tmp/tabler.a.gz > /tmp/tabler.old.json
diff <(grep -v generatedAt /tmp/tabler.old.json) <(grep -v generatedAt /tmp/tabler.new.json) | head
```

Expected: 空输出。非空说明排序不稳定（多半是某处按 map 遍历生成），**停下修**，别把它留给"以后加个排序"。

- [ ] **Step 8: 【故意破坏】证明产物自证真的会咬**

把 `internal/icons/index/tabler.json.gz` 临时用截断版替换（`gzip -dc` 出来删掉最后一条图标再 `gzip` 回去，`count` 不动）：

```bash
go test ./internal/icons/ -count=1 -run TestLoad
```

Expected: FAIL，报 `count=6267 与实际收录 6266 条不符`。改回真产物，复跑绿。**这一步不做，Task 1 的自证就只是给夹具用的。**

- [ ] **Step 9: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/icons tools/genicons
git commit -m "feat(icons): 内嵌 tabler 与 simple-icons 的 gzip 索引产物（6265 + 3737 条）"
```

`.gitattributes` 里有 `* text=auto eol=lf`，二进制文件必须不被文本处理——本仓库已列了 `*.png/*.webp/*.ico binary`，**本步要加一行 `*.json.gz binary`**，否则换行转换会把产物毁在 checkout 时。这是会咬人的那种"本地绿、克隆红"。

---
## Task 3: 索引搜索 `Search`

**Files:**
- Create: `internal/icons/search.go`
- Create: `internal/icons/search_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Index.Icons/alias/byPrefix`
- Produces:

```go
type SearchResult struct {
	Query string   `json:"query"`
	Total int      `json:"total"`
	Icons []string `json:"icons"`
}

func (x *Index) Search(q, prefix string, limit int) SearchResult
```

`Total` 是**匹配总数**、`Icons` 是**截断后的前 limit 条**——两者分开是 spec §6.1 那条"零命中与被截断不是一个状态"的实现。

- [ ] **Step 1: 写失败测试**

```go
// internal/icons/search_test.go
package icons

import (
	"encoding/json"
	"strings"
	"testing"
)

func searchIndex(t *testing.T) *Index {
	t.Helper()
	si := mustDecode(t, fixtureDoc(t, "simple-icons", "fill", 3, map[string]any{
		"docker": "M0 0", "docker-alt": "M0 1", "portainer": "M0 2",
	}, map[string]string{"moby": "docker"}))
	tab := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 3, map[string]any{
		"container": "M0 3", "container-off": "M0 4", "box": "M0 5",
	}, nil))
	return Merge(si, tab)
}

func TestSearch_排序是精确大于前缀大于包含(t *testing.T) {
	got := searchIndex(t).Search("docker", "", 10)
	if got.Total != 2 {
		t.Fatalf("total = %d，要 2（docker 与 docker-alt）", got.Total)
	}
	if got.Icons[0] != "simple-icons:docker" {
		t.Errorf("精确命中必须排第一，得到 %s", got.Icons[0])
	}
}

func TestSearch_品牌层先于通用层(t *testing.T) {
	// "container" 在两层都能匹配上 tabler:container 与 simple-icons 无；
	// 换成两侧都有子串的查询词才测得出层序。
	ix := searchIndex(t)
	got := ix.Search("o", "", 20)
	var firstLayer []string
	for _, k := range got.Icons {
		if strings.HasPrefix(k, "simple-icons:") {
			firstLayer = append(firstLayer, k)
		}
	}
	if len(firstLayer) == 0 {
		t.Fatal("没匹配到品牌层条目，夹具不对")
	}
	// 同 rank 内品牌层必须整体排在 tabler 前面。
	seenTabler := false
	for _, k := range got.Icons {
		if strings.HasPrefix(k, "tabler:") {
			seenTabler = true
			continue
		}
		if seenTabler {
			t.Errorf("%s 排在 tabler 之后：层优先级丢了", k)
		}
	}
}

func TestSearch_alias命中归一到规范名且不重复(t *testing.T) {
	got := searchIndex(t).Search("moby", "", 10)
	if got.Total != 1 {
		t.Fatalf("total = %d，要 1（别名不该变成第二个条目）", got.Total)
	}
	if got.Icons[0] != "simple-icons:docker" {
		t.Errorf("要规范名，得到 %s", got.Icons[0])
	}
}

func TestSearch_带前缀的查询词按整键查(t *testing.T) {
	got := searchIndex(t).Search("tabler:box", "", 10)
	if got.Total != 1 || got.Icons[0] != "tabler:box" {
		t.Errorf("得到 %+v", got.Icons)
	}
	// 前缀过滤同时生效：查 simple-icons 就不该看到它。
	if other := searchIndex(t).Search("tabler:box", "simple-icons", 10); other.Total != 0 {
		t.Errorf("prefix=simple-icons 却返回了 tabler 条目：%+v", other.Icons)
	}
}

func TestSearch_零命中是空切片不是nil(t *testing.T) {
	got := searchIndex(t).Search("zzz-not-a-name", "", 10)
	if got.Total != 0 {
		t.Fatal("不该有命中")
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	// §4.3：空数组写 []，写 null 会让 AI 判出"端点坏了"。
	if !strings.Contains(string(raw), `"icons":[]`) {
		t.Errorf("序列化形状不对: %s", raw)
	}
}

func TestSearch_limit截断而total留全(t *testing.T) {
	got := searchIndex(t).Search("o", "", 2)
	if len(got.Icons) != 2 {
		t.Fatalf("返回 %d 条，limit 是 2", len(got.Icons))
	}
	if got.Total < 3 {
		t.Errorf("total = %d，应当是匹配总数而不是返回条数", got.Total)
	}
}

func TestSearch_prefix过滤(t *testing.T) {
	if got := searchIndex(t).Search("container", "simple-icons", 10); got.Total != 0 {
		t.Errorf("过滤没生效：%+v", got.Icons)
	}
	if got := searchIndex(t).Search("container", "tabler", 10); got.Total != 2 {
		t.Errorf("tabler 里 container 与 container-off 都该命中，得到 %d", got.Total)
	}
}

func TestSearch_空查询与大写都归一(t *testing.T) {
	ix := searchIndex(t)
	if got := ix.Search("", "", 10); got.Total != 0 {
		t.Error("空查询一律不返回全量——9 千条的名字没有意义")
	}
	if got := ix.Search("  DOCker  ", "", 10); got.Total != 2 {
		t.Errorf("要去空格与小写化，得到 %+v", got)
	}
}
```

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/icons/ -count=1 -run TestSearch`
Expected: 编译失败（`x.Search undefined`）。

- [ ] **Step 3: 写 search.go**

```go
package icons

import (
	"sort"
	"strings"
)

// SearchResult 的字段序就是 JSON 键序（§4.3 对响应体同样成立）。
type SearchResult struct {
	Query string   `json:"query"`
	Total int      `json:"total"`
	Icons []string `json:"icons"`
}

// rank 小者优先。别名命中按"父名的精确命中"计，因为它本来就指向同一条图标。
const (
	rankExact  = 0 // 名字等于查询词
	rankPrefix = 1 // 名字以查询词开头
	rankAlias  = 2 // 别名等于查询词（等价于父名精确）
	rankInfix  = 3 // 名字包含查询词
)

// prefixRank 决定同 rank 内的层序：品牌层在前（spec §5 的双层用途表）。
// 键序与 load.go 的解析顺序无关——那是"哪套先报错"，这里是"哪套先出现"。
var prefixRank = map[string]int{"simple-icons": 0, "tabler": 1}

type hit struct {
	key   string
	rank  int
	layer int
}

// Search 在本地索引里按关键词找候选图标名。零命中与被截断是两个状态，
// 所以 Total 报匹配总数、Icons 只带 limit 条（spec §6.1）。
//
// 只做子串匹配：这是给人和 AI 用的名字查找，不是全文检索。上倒排索引或
// 拼音是另一件事，而 9 千条名字做子串扫描本来就是一微秒级的事。
func (x *Index) Search(q, prefix string, limit int) SearchResult {
	q = strings.ToLower(strings.TrimSpace(q))
	res := SearchResult{Query: q, Icons: []string{}}
	if q == "" || limit == 0 {
		return res
	}

	var hits []hit
	seen := map[string]bool{}
	add := func(key string, rank int) {
		if seen[key] {
			return // 别名与父名指向同一条：只留一条
		}
		seen[key] = true
		hits = append(hits, hit{key: key, rank: rank, layer: layerOf(key)})
	}

	// 带前缀的查询词（"tabler:box"）按整键查：AI 常拿上一步的结果原样回查。
	// 命中后要过 prefix 过滤——prefix 说了只要 simple-icons，就别把 tabler 端回来。
	if i := strings.IndexByte(q, ':'); i > 0 {
		if key, ok := x.Canonical(q); ok {
			if p := key[:strings.IndexByte(key, ':')]; prefix == "" || prefix == p {
				add(key, rankExact)
			}
		}
	}

	name := q
	if i := strings.IndexByte(name, ':'); i > 0 {
		name = name[i+1:]
	}
	for p, list := range x.byPrefix {
		if prefix != "" && p != prefix {
			continue
		}
		for _, key := range list {
			switch {
			case strings.HasSuffix(key, ":"+name):
				add(key, rankExact)
			case strings.Contains(key, ":"+name+"-"):
				add(key, rankPrefix)
			case strings.Contains(key, name):
				add(key, rankInfix)
			}
		}
	}
	// 别名表单独扫一遍：命中要归一到父名（上面的 byPrefix 循环看不见别名）。
	for alias, target := range x.alias {
		if prefix != "" && !strings.HasPrefix(alias, prefix+":") {
			continue
		}
		aliasName := alias[strings.IndexByte(alias, ':')+1:]
		if aliasName == name {
			add(target, rankAlias)
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		if hits[i].layer != hits[j].layer {
			return hits[i].layer < hits[j].layer
		}
		return hits[i].key < hits[j].key
	})

	res.Total = len(hits)
	for i, h := range hits {
		if i >= limit {
			break
		}
		res.Icons = append(res.Icons, h.key)
	}
	return res
}

func layerOf(key string) int {
	p, _, _ := strings.Cut(key, ":")
	if r, ok := prefixRank[p]; ok {
		return r
	}
	return 9 // 白名单外的前缀进不了索引，这里只是不留 panic 路径
}
```

- [ ] **Step 4: 跑测试确认它绿**

Run: `go test ./internal/icons/ -count=1`
Expected: PASS。

- [ ] **Step 5: 【故意破坏】两次**

1. 把 `rankAlias` 改成 9 → `TestSearch_alias命中归一到规范名且不重复` 红（别名搜索其实是靠 `byPrefix` 循环，改这一档证明第二支真的在跑）。
2. 把 `prefixRank["simple-icons"]` 从 0 改成 5 → `TestSearch_品牌层先于通用层` 红。

- [ ] **Step 6: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./internal/icons/ -count=1
git add internal/icons/search.go internal/icons/search_test.go
git commit -m "feat(icons): 本地图标索引搜索——精确/前缀/包含三档，别名归一到规范名"
```

---

## Task 4: 图标名只卡写路径（`ops.Run` 接 `IconSet`）

**Files:**
- Modify: `internal/ops/ops.go`（`Run`/`applyOne`/`add`/`set`/`setItemField`）
- Modify: `internal/ops/ops_test.go`、`internal/ops/batch_test.go`（调用点补参数 + 新用例）
- Modify: `internal/httpapi/batch.go`（传 `s.deps.Icons`；本任务先传 `nil`，Task 5 接上）
- Modify: `internal/model/check.go`（`checkIcon` 注释指向写路径闸门）
- Modify: `internal/model/model_test.go`、`internal/store/load_test.go`（读路径宽容的反向证人）


> **夹具里的「陌生图标名」必须仍是 ASCII 合法形状**（`tabler:notarealicon` 这类）。
> 不能用中文：`model.checkIcon` 的名字正则 `^[a-z0-9]+(-[a-z0-9]+)*$` 会先把中文形状拒掉，
> 于是「索引里没有」这一档的证人会红错原因、破坏实验会假绿。
> （Task 4 的实现者实测抓到计划这处自相矛盾并改了夹具，见 `task-4-report.md` 的顾虑节。）

**Interfaces:**
- Consumes: 任何实现 `Has(icon string) bool` 的东西（真实实现是 `*icons.Index`）
- Produces:

```go
type IconSet interface{ Has(icon string) bool }
func Run(b *model.Bookmarks, in []Op, icons IconSet) (*model.Bookmarks, []Error, []string)
```

- [ ] **Step 1: 先写"读路径不查索引"的反向证人（这是裁定三的锚）**

`internal/model/model_test.go` 追加：

```go
// spec §6.2：图标名的存在性只卡写路径。形状合法但索引里没有的名字，
// model.Validate 必须放过——否则一次图标库刷新就能把家里人的档案打成坏档。
func TestValidate_索引外的图标名照样合法(t *testing.T) {
	b := goodBookmarks()
	b.Groups[0].Items[0].Icon = "tabler:neverhere"
	if err := b.Validate(); err != nil {
		t.Errorf("model 层不该认图标索引，它没有这个信息: %v", err)
	}
}
```

`goodBookmarks() *Bookmarks` 是 `internal/model/model_test.go` 里既有的合法书签夹具（该文件每条破坏用例都从它起步）。**用它，不要新建第二份夹具。**

`internal/store/load_test.go` 追加：

```go
// 直接往磁盘写一份带陌生图标名的 bookmarks.json，读回来必须成功、
// 且名字一字不改地留着（渲染层会给它首字母，Task 9）。
// 这是"绕过写入通路种数据"的合法用法：测的是读路径的宽容，不是第二条写通路。
func TestLoadBookmarks_陌生图标名不进坏档(t *testing.T) {
	root := fixtureRoot(t) // 既有 helper：建 dad（含 bookmarks.json）与 kid
	// 用 put() 覆写而不是 SaveBookmarks：后者会跑校验与归一化，
	// 用写通路种数据会让这条测试变成自证循环。
	// 文件写成形似磁盘的形状（2 空格缩进、一个尾换行、alias/tags 是 []）：
	// 它验的正是那个解码器，不是随便一段能解析的 JSON。
	put(t, root, "profiles/dad/bookmarks.json", `{
  "rev": 3,
  "title": "t",
  "groups": [{"id": "g", "title": "G", "layout": "list", "items": [
    {"id": "a", "name": "A", "url": "http://10.0.0.1/", "icon": "simple-icons:notinsnapshot", "alias": [], "tags": []}
  ]}],
  "search": {"enabled": false, "default": "", "engines": []},
  "weather": {"enabled": false, "location": ""},
  "openInNewTab": false,
  "clock": false,
  "footer": ""
}
`)

	b, err := New(root).LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("陌生图标名不该让档案读不了: %v", err)
	}
	if got := b.Groups[0].Items[0].Icon; got != "simple-icons:notinsnapshot" {
		t.Errorf("名字被改了: %q", got)
	}
}
```

`fixtureRoot(t) string` 与 `put(t, root, rel, content)` 是 `load_test.go` 顶上的既有 helper，本用例只覆写其中一份文件，不需要新 helper。

- [ ] **Step 2: 跑这两条，确认它们现在就红或本来就绿**

Run: `go test ./internal/model/ ./internal/store/ -count=1 -run '陌生图标名|索引外的图标名'`
Expected: 大概率直接 PASS（今天的代码本来就不查索引）。**这不是白做**：本任务后面要在写路径加闸门，这两条从此钉住"加在写路径，没漏到读路径"。Task 5 接上索引后它们必须仍然绿——那才是这条测试的用武之地。

- [ ] **Step 3: 写写路径拒绝的失败测试**

`internal/ops/ops_test.go` 追加：

```go
// stubIcons 是 ops 测试用的假索引。ops 不 import internal/icons，
// 只认接口——索引那边换实现、换产物格式都不惊动写校验。
type stubIcons map[string]bool

func (s stubIcons) Has(icon string) bool { return s[icon] }

func TestRun_写路径认图标名(t *testing.T) {
	set := stubIcons{"tabler:home": true, "simple-icons:docker": true}

	t.Run("set 陌生名字要拒", func(t *testing.T) {
		_, errs, _ := Run(baseBookmarks(), []Op{{
			Op: "set", Path: "groups/nas/items/dsm/icon", Value: json.RawMessage(`"tabler:home2"`),
		}}, set)
		if len(errs) != 1 {
			t.Fatalf("要一条错误，得到 %+v", errs)
		}
		if errs[0].Field != "icon" {
			t.Errorf("field = %q，要 icon", errs[0].Field)
		}
		if !strings.Contains(errs[0].Reason, "/api/v1/icons") {
			t.Errorf("拒绝理由要把搜索端点交给调用方，否则它只会再猜一个: %q", errs[0].Reason)
		}
	})

	t.Run("set 合法名字要过", func(t *testing.T) {
		out, errs, _ := Run(baseBookmarks(), []Op{{
			Op: "set", Path: "groups/nas/items/dsm/icon", Value: json.RawMessage(`"tabler:home"`),
		}}, set)
		if len(errs) != 0 {
			t.Fatalf("合法名字被拒了: %+v", errs)
		}
		if out.Groups[0].Items[0].Icon != "tabler:home" {
			t.Error("没写进去")
		}
	})

	t.Run("auto 与空串归 ops 判而不是索引判", func(t *testing.T) {
		for _, v := range []string{`""`, `"auto"`} {
			_, errs, _ := Run(baseBookmarks(), []Op{{
				Op: "set", Path: "groups/nas/items/dsm/icon", Value: json.RawMessage(v),
			}}, set)
			if len(errs) != 0 {
				t.Errorf("%s 是语义值，不该被索引拒绝: %+v", v, errs)
			}
		}
	})

	t.Run("add 新条目带陌生名字要拒且不动磁盘状态", func(t *testing.T) {
		before := baseBookmarks()
		_, errs, _ := Run(before, []Op{{
			Op: "add", Path: "groups/nas/items",
			Value: json.RawMessage(`{"id":"new","name":"N","url":"http://10.0.0.2/","icon":"tabler:madedup"}`),
		}}, set)
		if len(errs) != 1 || errs[0].Field != "icon" {
			t.Fatalf("要一条 icon 字段的错误，得到 %+v", errs)
		}
		if errs[0].OpsIndex != 0 {
			t.Errorf("opsIndex = %d，要指到那条 add", errs[0].OpsIndex)
		}
		if len(before.Groups[0].Items) != 1 {
			t.Error("失败的批次必须什么都不留：入参被原地改了")
		}
	})

	t.Run("add 整个分组时逐条检查嵌套条目", func(t *testing.T) {
		_, errs, _ := Run(baseBookmarks(), []Op{{
			Op: "add", Path: "groups",
			Value: json.RawMessage(`{"id":"dev","title":"开发","layout":"list","items":[` +
				`{"id":"ok","name":"OK","url":"http://10.0.0.3/","icon":"tabler:home"},` +
				`{"id":"no","name":"NO","url":"http://10.0.0.4/","icon":"mdi:no"}]}`),
		}}, set)
		if len(errs) != 1 {
			t.Fatalf("嵌套条目里的陌生名字要被抓出来: %+v", errs)
		}
		if !strings.Contains(errs[0].Reason, "no") {
			t.Errorf("理由要点名是哪一条（%q），否则一整组十几条时没人知道改哪条", errs[0].Reason)
		}
	})

	t.Run("索引为 nil 就不检查", func(t *testing.T) {
		// 这不是给运行时留的后门：装配点必须传真索引（Task 5），
		// nil 只出现在"这条测试不关心图标"的场合。写清楚，别让人误以为
		// 服务可以不带索引启动。
		_, errs, _ := Run(baseBookmarks(), []Op{{
			Op: "set", Path: "groups/nas/items/dsm/icon", Value: json.RawMessage(`"mdi:whatever"`),
		}}, nil)
		if len(errs) != 0 {
			t.Errorf("nil 索引不该拦: %+v", errs)
		}
	})
}
```

- [ ] **Step 4: 跑测试确认它红**

Run: `go test ./internal/ops/ -count=1 -run 写路径认图标名`
Expected: 编译失败——`Run` 只接两个参数。

- [ ] **Step 5: 改 ops.go**

```go
// IconSet 是图标名的存在性判据。ops 只认这一个方法，不认索引的实现：
// 索引换成别的产物格式，写校验这边一个字都不用改。
type IconSet interface {
	Has(icon string) bool
}

// Run 把 ops 顺序作用在 b 的副本上。icons 是图标名白名单：
// **只有写路径认它**（spec §6.2），读路径与渲染层都宽容。装配点必须传
// 真索引；传 nil 等于关掉这道闸，只允许测试这么用。
func Run(b *model.Bookmarks, in []Op, icons IconSet) (*model.Bookmarks, []Error, []string) {
	out := clone(b)
	var errs []Error
	var warns []string
	last := -1
	for i, op := range in {
		w, err := applyOne(out, op, icons)
		// …（其余逐字不变）
	}
	// …
}

// checkIconName 是唯一的图标名闸门调用点。"" 与 "auto" 在这里判掉：
// 它们是有语义的取值（"没设"/"抓 favicon"），不是索引成员——
// 把这两个特例放进 icons.Index.Has 里，索引就得替写校验想事。
func checkIconName(icons IconSet, id, icon string) error {
	if icons == nil || icon == "" || icon == "auto" {
		return nil
	}
	if !icons.Has(icon) {
		return fieldErr{"icon", fmt.Errorf(
			"条目 %s 的图标名 %q 不在服务端索引里；先查 GET /api/v1/icons?q=<关键词>，" +
				"图标名不许靠猜（spec §5）", id, icon)}
	}
	return nil
}
```

三处接入点：

```go
// add 的 kindItems 分支，在 dupItem 检查之后、落位之前：
if err := checkIconName(icons, it.ID, it.Icon); err != nil {
	return nil, err
}

// add 的 kindGroups 分支，在重复分组检查之后：
for _, it := range g.Items {
	if err := checkIconName(icons, it.ID, it.Icon); err != nil {
		return nil, err
	}
}

// setItemField 的 icon 分支，赋值之前：
case "icon":
	if err := checkIconName(icons, it.ID, s); err != nil {
		return nil, err
	}
	warn = changed(it.Icon == s)
	it.Icon = s
```

`applyOne`、`add`、`set` 的签名同步加 `icons IconSet` 参数并向下传；`setItemField` 也多接一个参数。`move`/`remove` 不碰图标值，参数不传下去（`move` 移动的是**已经过了闸的**条目，不重查——这是有意的：陌生名字是历史数据，不该因为挪一次组就写不进去）。**这条判断要写在 `move` 的注释里**，否则下一个读代码的人会当成漏了。

- [ ] **Step 6: 更新所有调用点**

`internal/ops/ops_test.go`、`internal/ops/batch_test.go`、`internal/ops/path_test.go` 里每一处 `ops.Run(...)` 补第三个参数：不关心图标的用例传 `nil`，关心图标名的传 `stubIcons{...}`。`internal/httpapi/batch.go` 的 `ops.Run(snap.B, in.Ops)` 改成 `ops.Run(snap.B, in.Ops, s.deps.Icons)`——**本任务先接受 nil**（`Dependencies` 还没有 `Icons` 字段，Task 5 补上），所以这里写成 `ops.Run(snap.B, in.Ops, nil)` 并在 Task 5 一并替换，别在两个任务里各改一半。

Run: `go test ./internal/... -count=1`
Expected: 全绿。`go vet ./...` 会把漏改的调用点全部点出来——**不许**用变参或可选结构绕开它。

- [ ] **Step 7: 【故意破坏】两次**

1. 把 `checkIconName` 的 `if !icons.Has(icon)` 改成 `if false` → `TestRun_写路径认图标名` 的三条拒绝子例红（闸门装上了但没接线，是最容易混过去的一种"完成"）。
2. 删掉 `add` 的 `kindGroups` 循环 → 嵌套检查那条子例红。

- [ ] **Step 8: `model/check.go` 的注释对齐**

`checkIcon` 现在写着"图标是否真的存在归图标索引（P1-③）"。改成指向真正的落点：

```go
// checkIcon 只验形状与前缀。**名字存在性不在这里查**：model.Validate 同时
// 跑在读路径，把索引绑进来等于让"图标快照与磁盘不同步"把档案打成坏档
// （spec §6.2）。存在性由 ops 写路径的 IconSet 闸门负责（internal/ops#checkIconName）。
```

- [ ] **Step 9: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/ops internal/model internal/store internal/httpapi/batch.go
git commit -m "feat(ops): 图标名只卡写路径——ops 接 IconSet，读路径与渲染保持宽容"
```

---
## Task 5: 把索引装进服务——`GET /api/v1/icons` 与测试装配

**Files:**
- Modify: `internal/httpapi/server.go`（`Dependencies.Icons`）
- Modify: `internal/httpapi/routes.go`（注册一条）
- Create: `internal/httpapi/iconsearch.go`
- Create: `internal/httpapi/iconsearch_test.go`
- Create: `internal/httpapi/testing_test.go`（测试装配的唯一入口）
- Modify: `internal/httpapi/handlers_test.go`、`internal/httpapi/render_test.go`、`internal/httpapi/server_test.go`（改走 `newDeps`）
- Modify: `internal/httpapi/batch.go`（Task 4 留的 `ops.Run(..., nil)` 在这里换成 `s.deps.Icons`）
- Modify: `cmd/launchbay/main.go`（装配真索引）
- Modify: `internal/httpapi/openapi.json`、`internal/httpapi/openapi_test.go`（撤 planned）
- Modify: `AGENTS.md`（端点表 + 图标段）

**Interfaces:**
- Consumes: Task 1–3 的 `icons.Load/MustLoad/Search`
- Produces: `Dependencies{Icons *icons.Index}`（`New(Dependencies{})` 仍必须能构造——四条契约测试就靠它枚举路由），`GET /api/v1/icons` 的形状 `{query,total,returned,icons[]}`

- [ ] **Step 1: 先立装配的唯一入口，免得装配点散落**

```go
// internal/httpapi/testing_test.go
package httpapi

import (
	"sync"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

// 解析那两套 gz 索引不便宜（约 2MB 解压 + 解码），整个测试包共用一份：
// Index 一旦装配就只读，所以 sync.Once 之后不需要任何锁。
var (
	loadOnce sync.Once
	loadedIX *icons.Index
)

func testIndex() *icons.Index {
	loadOnce.Do(func() { loadedIX = icons.MustLoad() })
	return loadedIX
}

// newDeps 是 httpapi 测试的唯一装配点。评审教训：装配散落时"新加一个依赖
// 字段"会变成逐个测试红的追查，而这里只需要改一行。
func newDeps(s *store.Store, sc auth.Scopes) Dependencies {
	return Dependencies{Store: s, Tokens: sc, Render: render.New(s), Icons: testIndex()}
}
```

`server_test.go#testDeps`、`handlers_test.go#setupServer` 与 `render_test.go` 里那三处各 new 一遍 `Dependencies` 的写法，本步一并收进 `newDeps`：`testDeps(t)` 与 `setup(t)` 保留成它的薄壳，这样二十个既有用例的调用点一个字都不用改。**这就是把装配收进一个 helper 的全部理由**——Task 9 把 `render.New` 改成三参数时，改的就是上面这一行。本步先保持两参数。

- [ ] **Step 2: 写搜索端点的失败测试**

```go
// internal/httpapi/iconsearch_test.go
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func getJSON(t *testing.T, h http.Handler, url string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s 的响应不是 JSON: %v / %s", url, err, rec.Body)
		}
	}
	return rec.Code, body
}

func TestIconsSearch_公开可查(t *testing.T) {
	_, h := setup(t) // 既有的夹具装配
	code, body := getJSON(t, h, "/api/v1/icons?q=docker")
	if code != http.StatusOK {
		t.Fatalf("匿名查图标要 200（spec §6.1 把它列在公开三件里），得到 %d", code)
	}
	if body["query"] != "docker" {
		t.Errorf("query 不回显原词: %v", body["query"])
	}
	total, _ := body["total"].(float64)
	returned, _ := body["returned"].(float64)
	if total < 1 {
		t.Fatal("simple-icons:docker 必须命中")
	}
	if int(returned) != len(body["icons"].([]any)) {
		t.Errorf("returned=%v 与实际条数不符：这两个数分开报才有意义", body["returned"])
	}
}

func TestIconsSearch_参数分流(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct{ url, want string }{
		{"/api/v1/icons", "缺 q"},
		{"/api/v1/icons?q=docker&prefix=mdi", "prefix 非法"},
		{"/api/v1/icons?q=docker&limit=abc", "limit 非数字"},
		{"/api/v1/icons?q=docker&limit=-1", "limit 负数"},
	} {
		code, body := getJSON(t, h, tc.url)
		if code != http.StatusBadRequest {
			t.Errorf("%s（%s）要 400，得到 %d", tc.url, tc.want, code)
		}
		if body["detail"] == nil || body["detail"] == "" {
			t.Errorf("%s 的 400 要说人话: %v", tc.url, body)
		}
	}
}

func TestIconsSearch_limit上限掐得住(t *testing.T) {
	_, h := setup(t)
	code, body := getJSON(t, h, "/api/v1/icons?q=a&limit=99999")
	if code != http.StatusOK {
		t.Fatalf("得到 %d", code)
	}
	if got := len(body["icons"].([]any)); got > iconsSearchMaxLimit {
		t.Errorf("返回 %d 条，超过上限 %d", got, iconsSearchMaxLimit)
	}
}

func TestIconsSearch_零命中不写null(t *testing.T) {
	_, h := setup(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/icons?q=zzzzz-zzz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("零命中是 200 不是 404：它说的是「索引里没有」，不是「端点坏了」")
	}
	if !strings.Contains(rec.Body.String(), `"icons":[]`) {
		t.Errorf("§4.3 空数组必须写 []，得到 %s", rec.Body)
	}
}
```

（`strings` 要 import。`setup(t)` 与 `get(h, path, cookie)` 都是 `handlers_test.go` 的既有 helper——**不要**在 `iconsearch_test.go` 里再造一份档案夹具或 recorder 包装。）

- [ ] **Step 3: 跑测试确认它红**

Run: `go test ./internal/httpapi/ -count=1 -run IconsSearch`
Expected: FAIL——404（路由还没有），以及 `Dependencies` 没有 `Icons` 字段的编译错。

- [ ] **Step 4: 写 handler**

```go
// internal/httpapi/iconsearch.go
package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/rockbenben/launchbay/internal/icons"
)

// 上限是给 AI 的：一次拿回 6 千个名字既没意义又会淹没上下文。真实用法
// 都是"搜一个词、挑一个"，200 条足够覆盖 prefix 过滤后的任何合理查询。
const (
	iconsSearchDefaultLimit = 20
	iconsSearchMaxLimit     = 200
)

// iconsResponse 用 struct：键序固定（§4.3），returned 与 total 是两个数——
// "零命中"与"命中很多被截断"必须能分开判，否则 AI 会把截断当成没有。
type iconsResponse struct {
	Query    string   `json:"query"`
	Total    int      `json:"total"`
	Returned int      `json:"returned"`
	Icons    []string `json:"icons"`
}

// handleIconsSearch 在服务端内嵌的索引里找图标名。公开（credNone）：
// 它只能说"这个名字有没有"，说不出任何人有什么书签（spec §5/§6.1）。
func (s *Server) handleIconsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Code: 400, Detail: "要带 ?q=<关键词>；这个端点是查名字，不吐全量索引"})
		return
	}
	if p := r.URL.Query().Get("prefix"); p != "" && p != "tabler" && p != "simple-icons" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Code: 400, Detail: "prefix 只能是 tabler 或 simple-icons"})
		return
	}
	limit := iconsSearchDefaultLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Detail: "limit 要是 1 以上的整数"})
			return
		}
		if n > iconsSearchMaxLimit {
			n = iconsSearchMaxLimit
		}
		limit = n
	}
	res := s.deps.Icons.Search(q, r.URL.Query().Get("prefix"), limit)
	writeJSON(w, http.StatusOK, iconsResponse{
		Query: res.Query, Total: res.Total, Returned: len(res.Icons), Icons: res.Icons,
	})
}
```

- [ ] **Step 5: `Dependencies` 加字段、注册路由**

```go
// internal/httpapi/server.go
type Dependencies struct {
	Store  *store.Store
	Tokens auth.Scopes
	Render *render.Renderer
	// Icons 是内嵌的双层图标索引：搜索端点、写路径的图标名闸门、渲染层
	// 与 /status 的缺图统计都读它。真实服务必须非 nil（装配点读不到就退出），
	// 但 `New(Dependencies{})` 仍要能构造——契约测试靠它枚举路由。
	Icons *icons.Index
}
```

`routes()` 追加一行（放在 `openapi.json` 那条之后、`profiles` 之前，与文档表顺序一致）：

```go
{Method: "GET", Pattern: "/api/v1/icons", Cred: credNone,
	Summary: "本地图标索引搜索（公开）", H: s.handleIconsSearch},
```

`cmd/launchbay/main.go` 的装配：

```go
ix, err := icons.Load()
if err != nil {
	// 图标索引是内嵌产物：读不出来只有两种原因（embed 路径写错、产物损坏），
	// 都不该被"降级成没有图标"悄悄咽下——那会让一个损坏的镜像跑得很"正常"。
	log.Fatalf("图标索引装配失败: %v", err)
}
h := httpapi.New(httpapi.Dependencies{
	Store: s, Tokens: cfg.Tokens, Render: render.New(s), Icons: ix,
}).Handler()
```

同一步把 Task 4 留下的那行 `nil` 换成真索引——**这一步不做，写路径的图标闸门就是死码**（编译通过、测试全绿、线上什么都不拦）：

```go
// internal/httpapi/batch.go
next, errs, warns := ops.Run(snap.B, in.Ops, s.deps.Icons)
```

- [ ] **Step 6: 三处契约文档同步**

`openapi.json`：把 `/api/v1/icons` 那条的 `x-status: planned` 与"计划中"文案换成真形状（`parameters` 三个 query、`200` 的 schema 用 `$ref` 指向新增的 `IconsResponse`，`400` 复用既有 `Error`），并**从 `openapi_test.go` 的 `planned` 表里删掉 `"GET /api/v1/icons": true`**——那道豁免表留着会让 `TestOpenAPIHasNoGhostEndpoints` 红，这是设计好的连锁，不是故障。

`AGENTS.md` 的端点表把 icons 那一行从"计划中 / `x-status: planned`"换成真条目：

```
| GET | `/api/v1/icons` | 无（公开） | 200 `{query, total, returned, icons[]}` | 本地索引搜图标名；`q` 必填，`prefix` ∈ tabler/simple-icons，`limit` ≤200 |
```

同时把"## 图标"那一节里"P1-③ 才上线 / 现在标的是 planned / 别靠猜但没有工具"的措辞换成"先查 `GET /api/v1/icons?q=…&prefix=…`，名字不在索引里会被 `POST /api/v1/batch` 直接 422 拒掉"。**那段是 AGENTS.md 里唯一一处"当前做不到"的免责句，撤掉它才是本期交付的验收物**——`TestAgentsMDMatchesRoutes` 只钉端点存在性，钉不住这种语义陈旧。

- [ ] **Step 7: 跑契约闸与全部测试**

Run: `go test ./internal/httpapi/ -count=1`
Expected: 全绿。若 `TestOpenAPICoversEveryRoute` 红，多半是 `x-credential` 与 `Cred` 没对上（`none`）或 summary 空着。

- [ ] **Step 8: 【故意破坏】两次**

1. 把路由的 `Cred: credNone` 改成 `credScoped` → `TestIconsSearch_公开可查` 红（401），且 `TestOpenAPICoversEveryRoute` 红（文档说 none）——**一次破坏被两道闸分别抓到**，正是路由表当唯一来源想要的效果。
2. 把 `Returned: len(res.Icons)` 改成 `Returned: res.Total` → `returned 与实际条数不符` 那条红。

- [ ] **Step 9: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/httpapi cmd/launchbay/main.go AGENTS.md
git commit -m "feat(httpapi): GET /api/v1/icons 上线——索引装配、测试装配入口与契约三处同步"
```

---

## Task 6: 抓取内核（页面与图片，§5 全部约束）

**Files:**
- Create: `internal/icons/fetch.go`
- Create: `internal/icons/extract.go`
- Create: `internal/icons/fetch_test.go`
- Create: `internal/icons/extract_test.go`

**Interfaces:**
- Consumes: 无（标准库）
- Produces:

```go
type Fetcher struct {
	Client   *http.Client
	Timeout  time.Duration
	MaxBytes int64
}
func NewFetcher() *Fetcher
func (f *Fetcher) Page(rawURL string) ([]byte, string, error)        // body, finalURL
func (f *Fetcher) Image(rawURL string) ([]byte, string, error)       // body, ext
func Extract(body []byte, base string) Page                          // 解析，不抓取
type Page struct { Title, Description, Favicon string }
var (
	ErrBadURL, ErrUnreachable, ErrTimeout, ErrStatus,
	ErrTooBig, ErrWrongType, ErrTooManyRedirects error
)
func Origin(rawURL string) string
```

- [ ] **Step 1: 写失败测试（全部走 `httptest`，只碰 127.0.0.1）**

```go
// internal/icons/fetch_test.go
package icons

import (
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
}

func TestOrigin_带端口且形状稳定(t *testing.T) {
	for in, want := range map[string]string{
		"http://10.0.0.3:8096/web/index":  "http://10.0.0.3:8096",
		"https://newzone.top/posts/1":     "https://newzone.top",
		"http://Host.LAN:80/x":            "http://host.lan:80", // 主机名小写化：同一服务别算成两个 origin
		"chrome://extensions/":            "",
	} {
		if got := Origin(in); got != want {
			t.Errorf("Origin(%q) = %q，要 %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/icons/ -count=1 -run 'TestFetcher|TestOrigin'`
Expected: 编译失败（`Fetcher` 未定义）。

- [ ] **Step 3: 写 fetch.go**

```go
package icons

import (
	"errors"
	"fmt"
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
	ErrBadURL            = errors.New("地址不是可用的 http/https 链接")
	ErrUnreachable       = errors.New("目标连不上")
	ErrTimeout           = errors.New("目标在 3 秒内没有响应")
	ErrStatus            = errors.New("目标没给成功状态")
	ErrTooBig            = errors.New("目标响应超过大小上限")
	ErrWrongType         = errors.New("目标返回的不是要求的类型")
	ErrTooManyRedirects  = errors.New("重定向次数超过上限")
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
// `http.Client.Timeout` 覆盖整个交换（含重定向链）而不是单次 dial/read，
// 而"跟着 3 跳的每一跳各自 3 秒"才是这条约束要说的事。测超时的那条测试
// 也依赖这个语义——把 Timeout 挪到 Transport 上，它会假绿。
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
```

- [ ] **Step 4: `go test` 跑到这一步只允许剩 extract 的红**

Run: `go test ./internal/icons/ -count=1 -run 'TestFetcher|TestOrigin'`
Expected: PASS。若 `TestFetcher_超时` 与 `TestFetcher_重定向三次封顶` 一起过不了，检查是不是把 `Timeout` 挂到了 `Transport.ResponseHeaderTimeout` 上——那两条测的是不同的东西。

- [ ] **Step 5: 写 extract 的失败测试**

```go
// internal/icons/extract_test.go
package icons

import (
	"strings"
	"testing"
)

func TestExtract_标题描述与favicon(t *testing.T) {
	body := []byte(`<html><head>
		<title>  Jellyfin &amp; Co  </title>
		<meta property="og:description" content="  家庭媒体服务器 ">
		<link rel="icon" href="/favicon.ico">
		<link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png">
	</head><body><h1>正文不进任何东西</h1></body></html>`)
	got := Extract(body, "http://10.0.0.3:8096/web/index.html")

	if got.Title != "Jellyfin & Co" {
		t.Errorf("标题要去实体与首尾空格，得到 %q", got.Title)
	}
	if got.Description != "家庭媒体服务器" {
		t.Errorf("描述 = %q", got.Description)
	}
	if got.Favicon != "http://10.0.0.3:8096/apple-touch-icon.png" {
		t.Errorf("有多个候选时要 apple-touch-icon（通常 180px，比 16px favicon 清楚），得到 %q", got.Favicon)
	}
	if strings.Contains(got.Title, "正文") || strings.Contains(got.Description, "正文") {
		t.Error("正文漏进来了")
	}
}

func TestExtract_属性值里有尖括号也不截断(t *testing.T) {
	// 这是正则版会栽的那个形状：content="a>b" 里>在引号内，不是标签结束。
	body := []byte(`<meta name="description" content="差 6 > 4 的比分">`)
	got := Extract(body, "http://10.0.0.1/")
	if got.Description != "差 6 > 4 的比分" {
		t.Errorf("被引号里的 > 截断了: %q", got.Description)
	}
}

func TestExtract_缺项就空着不编造(t *testing.T) {
	got := Extract([]byte(`<html><head><meta charset="utf-8"></head></html>`), "http://10.0.0.1/")
	if got.Title != "" || got.Description != "" || got.Favicon != "" {
		t.Errorf("没有就是没有: %+v", got)
	}
}

func TestExtract_标题要限长(t *testing.T) {
	long := "<title>" + strings.Repeat("标", 400) + "</title>"
	got := Extract([]byte(long), "http://10.0.0.1/")
	if n := len([]rune(got.Title)); n > maxTitleRunes {
		t.Errorf("标题 %d 字，上限 %d", n, maxTitleRunes)
	}
}

func TestExtract_相对与绝对href都能落地(t *testing.T) {
	got := Extract([]byte(`<link rel="icon" href="../fav.ico">`), "http://10.0.0.1/a/b/index.html")
	if got.Favicon != "http://10.0.0.1/a/fav.ico" {
		t.Errorf("相对路径解析错: %q", got.Favicon)
	}
	got = Extract([]byte(`<link rel="icon" href="http://cdn.10.0.0.9.fakedomain.test/f.ico">`), "http://10.0.0.1/")
	if got.Favicon == "" {
		t.Error("绝对 href 应当原样保留")
	}
}
```

- [ ] **Step 6: 写 extract.go**

```go
package icons

import (
	"html"
	"net/url"
	"strings"
)

// maxTitleRunes / maxDescRunes：resolve 的结果是给人填表单用的候选，
// 不是存档。抓一个 800 字的 og:description 存进书签只会毁掉卡片布局。
const (
	maxTitleRunes = 160
	maxDescRunes  = 300
)

type Page struct {
	Title       string `json:"title"`
	Description string `json:"desc"`
	Favicon     string `json:"-"` // 只在内部用于抓图，不进响应
}

// Extract 从 HTML 里只取三样：<title>、og:description、favicon link。
// spec §6.1 写死了"不存正文、不留抓到的 HTML"，所以这个函数没有任何
// 返回整份文档的路径，也没有把 body 存进结构体的字段。
//
// 候选状态（best）是函数局部的：写成包级变量就等于让两次 Extract 互相污染，
// 而并发下它是真的会坏——resolve 会同时在几个会话里被调用。
func Extract(body []byte, base string) Page {
	src := string(body)
	lower := strings.ToLower(src)
	var p Page
	bestRank, bestHref := 9, ""

	if i := strings.Index(lower, "<title"); i >= 0 {
		if gt := tagEnd(lower, i); gt > 0 {
			rest := lower[gt:]
			if j := strings.Index(rest, "</title"); j >= 0 {
				p.Title = cleanText(src[gt : gt+j])
			}
		}
	}

	// meta 与 link 共用一次标签扫描：属性解析要认引号，所以不能按 '>' 切。
	for _, tg := range scanTags(src, lower, "<meta", "<link") {
		attrs := parseAttrs(tg.body)
		switch tg.name {
		case "meta":
			key := strings.ToLower(firstNonEmpty(attrs["property"], attrs["name"]))
			if key == "og:description" && p.Description == "" {
				p.Description = truncateRunes(cleanText(attrs["content"]), maxDescRunes)
			}
		case "link":
			rel := strings.ToLower(attrs["rel"])
			if !strings.Contains(rel, "icon") {
				break // "stylesheet"、"preload" 之类
			}
			href := attrs["href"]
			if href == "" {
				break
			}
			// 优先级：apple-touch-icon > icon > shortcut icon。
			// 第一个通常 180px，缩到 24px 也还是一张清楚的小图；
			// 16px 的 favicon.ico 在高 DPI 屏上会糊成灰点。
			rank := 1
			switch {
			case strings.Contains(rel, "apple-touch-icon"):
				rank = 0
			case strings.Contains(rel, "shortcut"):
				rank = 2
			}
			if rank < bestRank {
				bestRank = rank
				bestHref = resolveHref(href, base)
			}
		}
	}
	p.Favicon = bestHref
	return p
}
```

- [ ] **Step 7: 写标签扫描与属性解析**

```go
type tag struct {
	name  string
	start int
	body  string // 含 '<' 与 '>' 的原样子串
}

// scanTags 按 "<name" 找标签，用 tagEnd 找到真正的 '>'。
func scanTags(src, lower string, opens ...string) []tag {
	var out []tag
	for _, open := range opens {
		for i := 0; ; {
			j := strings.Index(lower[i:], open)
			if j < 0 {
				break
			}
			k := i + j
			end := tagEnd(lower, k)
			if end < 0 {
				break
			}
			out = append(out, tag{name: open[1:], start: k, body: src[k : end+1]})
			i = end + 1
		}
	}
	return out
}

// tagEnd 返回从 i 处标签开始的那个 '>' 的下标，跳过引号内的 '>'。
// 这是 extract 唯一一处"自己扫字符串"的地方，也是 TestExtract_属性值里有
// 尖括号也不截断 存在的理由：`content="a>b"` 用正则按 '>' 切会当场截断正文。
func tagEnd(lower string, i int) int {
	var quote byte
	for j := i; j < len(lower); j++ {
		c := lower[j]
		switch {
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && c == '>':
			return j
		}
	}
	return -1
}

// parseAttrs 解一个标签里的属性。只认 name="value" / name='value' /
// name=value 三种写法，键统一小写——HTML 里 PROPERTY 与 property 是同一个东西。
func parseAttrs(t string) map[string]string {
	out := map[string]string{}
	i := strings.IndexByte(t, ' ')
	if i < 0 {
		return out
	}
	src := t[i:]
	for src != "" {
		src = strings.TrimLeft(src, " \t\r\n")
		eq := strings.IndexByte(src, '=')
		if eq < 0 {
			break
		}
		key := strings.ToLower(strings.TrimRight(src[:eq], " \t\r\n"))
		src = src[eq+1:]
		var val string
		if len(src) > 0 && (src[0] == '"' || src[0] == '\'') {
			q := src[0]
			end := strings.IndexByte(src[1:], q)
			if end < 0 {
				break
			}
			val = src[1 : 1+end]
			src = src[end+2:]
		} else {
			sp := strings.IndexAny(src, " \t\r\n>")
			if sp < 0 {
				sp = len(src)
			}
			val = src[:sp]
			src = src[sp:]
		}
		if key != "" {
			out[key] = val
		}
	}
	return out
}

func resolveHref(href, base string) string {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return b.ResolveReference(ref).String()
}

func cleanText(s string) string {
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ") // 折叠所有空白
	return truncateRunes(strings.TrimSpace(s), maxTitleRunes)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 8: 跑测试确认它绿**

Run: `go test ./internal/icons/ -count=1`
Expected: PASS。

- [ ] **Step 9: 【故意破坏】三次**

1. 把 `tagEnd` 的引号分支删掉（只看 `>`）→ `TestExtract_属性值里有尖括号也不截断` 红。**这一次破坏是 Task 6 的核心证人**：没有它，`tagEnd` 与一条 `strings.IndexByte` 无异，而"跳过引号里的 `>`"是这段代码存在的全部理由。
2. 把 `Image` 的类型白名单放宽到"任何 `image/*`"里不认识的也收（`default: return true`）→ `TestFetcher_类型必须对得上` 里"认不得的类型不进缓存"的分支红。
3. 把 `classify` 改成一律 `return ErrUnreachable` → `TestFetcher_超时` 红。

- [ ] **Step 10: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/icons
git commit -m "feat(icons): 抓取内核与 HTML 提取——§5 全部约束与只取三项的解析"
```

---
## Task 7: favicon 内容哈希缓存与"缺图读时算"

**Files:**
- Create: `internal/store/iconcache.go`
- Create: `internal/store/iconcache_test.go`
- Modify: `internal/store/paths.go`（`IconsDir` / `IconBlobPath` / `IconIndexPath`）
- Modify: `internal/httpapi/server.go`（`Dependencies.Fetch`、`Dependencies.Cache`）
- Modify: `internal/httpapi/admin.go`（`iconUnmatched`）
- Modify: `internal/httpapi/admin_test.go`、`internal/httpapi/testing_test.go`
- Modify: `cmd/launchbay/main.go`（装配缓存）

**Interfaces:**
- Consumes: `Store.WriteFileJSON`（原子写既有通路）、`icons.Origin`
- Produces:

```go
type IconEntry struct {
	Origin string `json:"origin"`
	Hash   string `json:"hash"`
	Ext    string `json:"ext"`
	At     string `json:"at"`
	FailAt string `json:"failAt,omitempty"`
}
func (s *Store) IconsDir() string
func (s *Store) IconBlobPath(hash, ext string) string
func (s *Store) IconIndexPath() string
func (s *Store) LoadIconCache() (*IconCache, error)
func (c *IconCache) Lookup(origin string) (IconEntry, bool)
func (c *IconCache) InCooldown(origin string) bool
func (c *IconCache) Put(origin string, data []byte, ext string) (IconEntry, error)
func (c *IconCache) MarkFail(origin string) error
func (c *IconCache) Generation() uint64
```

- [ ] **Step 1: 写失败测试**

```go
// internal/store/iconcache_test.go
package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newCache(t *testing.T) (*IconCache, *Store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	s := New(t.TempDir())
	c, err := s.LoadIconCache()
	if err != nil {
		t.Fatalf("空目录也要能起一个空缓存: %v", err)
	}
	c.now = func() time.Time { return now }
	return c, s, &now
}

func TestIconCache_字节一致只存一份(t *testing.T) {
	c, s, _ := newCache(t)
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}

	e1, err := c.Put("http://10.0.0.3:8096", png, "png")
	if err != nil {
		t.Fatal(err)
	}
	e2, err := c.Put("http://10.0.0.9:8080", png, "png") // 同一张图，另一个服务
	if err != nil {
		t.Fatal(err)
	}
	if e1.Hash != e2.Hash {
		t.Errorf("同字节必须同哈希: %s vs %s", e1.Hash, e2.Hash)
	}
	ents, _ := os.ReadDir(s.IconsDir())
	var blobs int
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), "index.json") {
			blobs++
		}
	}
	if blobs != 1 {
		t.Errorf("内容寻址的意义就是不去重就白占盘，得到 %d 个 blob", blobs)
	}
	got, err := os.ReadFile(s.IconBlobPath(e1.Hash, e1.Ext))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(png) {
		t.Error("blob 字节被改过：内容哈希就不是原字节了")
	}
}

func TestIconCache_哈希与扩展名就是文件名形状(t *testing.T) {
	c, s, _ := newCache(t)
	e, err := c.Put("http://10.0.0.3:8096", []byte("x"), "ico")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Hash) != 16 || strings.TrimRight(e.Hash, "0123456789abcdef") != "" {
		t.Errorf("hash %q 要 16 位小写十六进制——Task 11 的路径校验靠这个形状", e.Hash)
	}
	if filepath.Base(s.IconBlobPath(e.Hash, e.Ext)) != e.Hash+".ico" {
		t.Error("blob 文件名要能被 /icons/{name} 原样认出来")
	}
}

func TestIconCache_代次只在真变化时递增(t *testing.T) {
	c, _, _ := newCache(t)
	g0 := c.Generation()
	if _, err := c.Put("http://10.0.0.3:8096", []byte("a"), "png"); err != nil {
		t.Fatal(err)
	}
	g1 := c.Generation()
	if g1 == g0 {
		t.Fatal("缓存变了而代次没动，首屏会一直端出旧页（spec §5 第 5 条）")
	}
	if _, err := c.Put("http://10.0.0.3:8096", []byte("a"), "png"); err != nil { // 同一 origin 同一字节
		t.Fatal(err)
	}
	if c.Generation() != g1 {
		t.Error("同一张图重复上报不该让所有页面重算")
	}
}

func TestIconCache_失败冷却24小时(t *testing.T) {
	c, _, now := newCache(t)
	if err := c.MarkFail("http://10.0.0.7/"); err != nil {
		t.Fatal(err)
	}
	if !c.InCooldown("http://10.0.0.7/") {
		t.Error("刚失败就该在冷却里")
	}
	*now = now.Add(23 * time.Hour)
	if !c.InCooldown("http://10.0.0.7/") {
		t.Error("23 小时还在冷却")
	}
	*now = now.Add(2 * time.Hour)
	if c.InCooldown("http://10.0.0.7/") {
		t.Error("25 小时后应当允许再试一次")
	}
	// 成功过一次的 origin 不受失败记录影响
	if _, err := c.Put("http://10.0.0.7/", []byte("z"), "png"); err != nil {
		t.Fatal(err)
	}
	if c.InCooldown("http://10.0.0.7/") {
		t.Error("已经拿到图了还冷却，等于把自己锁死")
	}
}

func TestIconCache_落盘可复读且键序稳定(t *testing.T) {
	c, s, _ := newCache(t)
	for _, o := range []string{"http://10.0.0.9/", "http://10.0.0.3/", "http://10.0.0.7/"} {
		if _, err := c.Put(o, []byte(o), "png"); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(s.IconIndexPath())
	if err != nil {
		t.Fatal(err)
	}
	// §4.3：entries 是排序后的切片而不是 map——同一份缓存两次写出字节一致。
	again, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	if again.Generation() != 1 {
		t.Errorf("复读出来的缓存代次 = %d，要 1（读盘不算变化）", again.Generation())
	}
	if !strings.Contains(string(raw), `"origin": "http://10.0.0.3/"`) {
		t.Errorf("index.json 形状不对: %s", raw)
	}
	second, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(s.IconIndexPath())
	if string(a) != string(raw) {
		t.Error("复读后再落盘改了字节：排序不可靠（多半是 map 遍历）")
	}
	_ = second
}

func TestIconCache_坏index.json不许静默清空(t *testing.T) {
	s := New(t.TempDir())
	if err := os.MkdirAll(s.IconsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.IconIndexPath(), []byte("{半"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadIconCache(); err == nil {
		t.Fatal("读坏了还返回一个空缓存，等于把整站图标缓存清零——宁可启动失败")
	}
}

func TestIconCache_Lookup只认成功项(t *testing.T) {
	c, _, _ := newCache(t)
	if _, ok := c.Lookup("http://10.0.0.3:8096"); ok {
		t.Error("没存过不该有")
	}
	if err := c.MarkFail("http://10.0.0.3:8096"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup("http://10.0.0.3:8096"); ok {
		t.Error("失败记录不是图标")
	}
}
```

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/store/ -count=1 -run IconCache`
Expected: 编译失败（`LoadIconCache` 未定义）。

- [ ] **Step 3: 写路径函数**

`internal/store/paths.go` 追加：

```go
// IconsDir 是站级的 favicon 缓存目录：按内容哈希命名，不含档案 id
// （spec §4.1、§5）。删档不动它（spec §6.1 的 profiles DELETE 那条）。
func (s *Store) IconsDir() string { return filepath.Join(s.root, "icons") }

// IconBlobPath 的哈希与扩展名都由 Put 生成，形状固定成 <16 hex>.<ext>，
// 这样 /icons/{name} 那条路由只认这个形状就能同时挡掉路径穿越。
func (s *Store) IconBlobPath(hash, ext string) string {
	return filepath.Join(s.IconsDir(), hash+"."+ext)
}

func (s *Store) IconIndexPath() string { return filepath.Join(s.IconsDir(), "index.json") }
```

- [ ] **Step 4: 写 iconcache.go**

```go
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// 图标缓存是 `data/icons/` 下唯一可写之物，而它只由 resolve 写（spec §5）。
// 这里不做"顺手给 CLI 开个口子"：那等于给站级缓存开第二条写通路。
const IconCooldown = 24 * time.Hour

// IconEntry 一条 origin 的缓存状态。成功项有 Hash/Ext，失败项只有 FailAt，
// 两者互斥——把"试过且失败"与"有图"混成一个字段，渲染层就得靠猜。
type IconEntry struct {
	Origin string `json:"origin"`
	Hash   string `json:"hash"`
	Ext    string `json:"ext"`
	At     string `json:"at"`
	FailAt string `json:"failAt,omitempty"`
}

type iconIndexFile struct {
	Entries []IconEntry `json:"entries"`
}

// IconCache 是内存镜像 + 落盘索引 + 一个代次计数。
//
// 代次是这里唯一的目的性设计：首屏渲染缓存的键是 (profile, rev, mtime)，
// 不含"图标缓存变了"这件事。没有 gen，resolve 刚抓到的图要等到下次写书签
// 才显示——那是句谎话，spec §5 第 5 条点名的就是它。
type IconCache struct {
	st   *Store
	mu   sync.Mutex
	byID map[string]IconEntry
	gen  uint64
	now  func() time.Time // 测试注入
}

func (s *Store) LoadIconCache() (*IconCache, error) {
	c := &IconCache{st: s, byID: map[string]IconEntry{}, now: time.Now}
	raw, err := os.ReadFile(s.IconIndexPath())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil // 没有缓存就是没有，不是错误
	}
	if err != nil {
		return nil, fmt.Errorf("读图标索引: %w", err)
	}
	var f iconIndexFile
	if err := decodeJSON(raw, &f); err != nil {
		// 索引坏了绝不"清空重来"：那会把 24 小时冷却与所有已抓好的图一起
		// 抹掉，而磁盘上的 blob 还躺着——第二份真相就此诞生。让它报错，
		// 装配点决定怎么退出。
		return nil, fmt.Errorf("图标索引 %s 读不了: %w", s.IconIndexPath(), err)
	}
	for _, e := range f.Entries {
		c.byID[e.Origin] = e
	}
	return c, nil
}

func (c *IconCache) Lookup(origin string) (IconEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byID[origin]
	if !ok || e.Hash == "" {
		return IconEntry{}, false
	}
	return e, true
}

func (c *IconCache) InCooldown(origin string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byID[origin]
	if !ok || e.Hash != "" || e.FailAt == "" {
		return false
	}
	at, err := time.Parse(time.RFC3339, e.FailAt)
	if err != nil {
		return false // 时间戳读不懂就允许再试：宁可多抓一次，不要永远锁死
	}
	return c.now().Sub(at) < IconCooldown
}

// Put 存字节并把 origin 指向它。字节已经是内容哈希命名的，重复抓同一张图
// 只会覆盖同一个文件名——去重由命名保证，不靠额外索引。
func (c *IconCache) Put(origin string, data []byte, ext string) (IconEntry, error) {
	if origin == "" || len(data) == 0 || !iconExts[ext] {
		return IconEntry{}, fmt.Errorf("图标缓存写入参数非法（origin 空 / 零字节 / 扩展名 %q）", ext)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])[:16]
	e := IconEntry{Origin: origin, Hash: hash, Ext: ext, At: c.now().UTC().Format(time.RFC3339)}

	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.byID[origin]; ok && old.Hash == hash && old.Ext == ext && old.FailAt == "" {
		return e, nil // 完全没变：不动代次，免得每次 resolve 都让全部页面重算
	}
	if err := c.st.WriteFileAtomic(c.st.IconBlobPath(hash, ext), data); err != nil {
		return IconEntry{}, err
	}
	c.byID[origin] = e
	if err := c.saveLocked(); err != nil {
		return IconEntry{}, err
	}
	c.gen++
	return e, nil
}

func (c *IconCache) MarkFail(origin string) error {
	if origin == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.byID[origin]; ok && e.Hash != "" {
		return nil // 已经有图：失败不该把可用的缓存抹掉
	}
	c.byID[origin] = IconEntry{Origin: origin, At: c.now().UTC().Format(time.RFC3339),
		FailAt: c.now().UTC().Format(time.RFC3339)}
	if err := c.saveLocked(); err != nil {
		return err
	}
	c.gen++
	return nil
}

// Generation 是渲染缓存键的一部分（Task 9）。
func (c *IconCache) Generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// saveLocked 按 origin 排序写出。用切片而不是 map：§4.3 那条"绝不靠 map
// 遍历产生输出键序"在这里的体现是"同一个键集两次写出字节一致"。
// （Go 的 encoding/json 对 map 会按字母排，但那是实现细节而不是契约，
// 而且 map 让"entries 是数组"这条对外形状变得不可能。）
func (c *IconCache) saveLocked() error {
	f := iconIndexFile{Entries: make([]IconEntry, 0, len(c.byID))}
	for _, e := range c.byID {
		f.Entries = append(f.Entries, e)
	}
	sort.Slice(f.Entries, func(i, j int) bool { return f.Entries[i].Origin < f.Entries[j].Origin })
	return c.st.WriteFileJSON(c.st.IconIndexPath(), f)
}

// iconExts 与 icons.Fetcher.Image 认的类型一一对应。两处必须同步：
// 一边多一个后缀，另一边就会抓回来一个写不进去的图。
var iconExts = map[string]bool{"png": true, "ico": true, "svg": true, "webp": true, "gif": true}
```

`decodeJSON` 是 store 已有的解码 helper（`load.go` 里那份）；**没有就复用它，不要在 iconcache.go 里另开一份严格解码**——两份解码器的错误类型迟早分家，而 §12 的坏档分诊依赖其中一份。

- [ ] **Step 5: 装进服务并补 `/status` 的 `iconUnmatched`**

`Dependencies` 再加两个字段（注释同风格）：

```go
	// Fetch 是运行时唯一的出网通路，只由 /api/v1/resolve 使用（spec §5）。
	Fetch *icons.Fetcher
	// Cache 是 data/icons/ 的站级图标缓存。真实服务必须非 nil；
	// 契约测试用的 New(Dependencies{}) 允许留空（它不发请求）。
	Cache *store.IconCache
```

`admin.go` 的 `statusProfile` 在 `Items` 之后插一个字段（键序跟着语义走：条目数、缺图数、快照数）：

```go
	IconUnmatched int `json:"iconUnmatched"`
```

`statusTotals` 同样加一个。统计函数（口径逐字照 spec §5：只数"看着对却查不到的名字"与"auto 且无缓存"，空 `icon` 不算缺）：

```go
// iconUnmatched 数的是"看着像图标名、索引里却没有"的条目，加上"写了 auto
// 但缓存里没图"的条目。空 icon 不进这个数：它是"没设"，在 GET /api/v1/data
// 里一眼可见，不需要计数器提醒（spec §5）。
//
// 读时算、不记盘：缺图是索引与数据的差集，索引一刷新数字就该跟着变。
// 把它记成事件会让 GET / 产生写盘副作用，而且旧记录会变成谎话。
func countIconUnmatched(b *model.Bookmarks, ix *icons.Index, fc *store.IconCache) int {
	if ix == nil || fc == nil {
		// 没装索引/缓存就没有"缺图"可言：拿空依赖去判会把整个站统计成 0，
		// 而真实服务永远两个都有（装配点读不到就退出）。
		return 0
	}
	n := 0
	for _, g := range b.Groups {
		for _, it := range g.Items {
			switch {
			case it.Icon == "":
			case it.Icon == "auto":
				o := icons.Origin(it.URL)
				if o == "" {
					n++ // chrome:// 这类协议没有可抓的图，如实算缺
					continue
				}
				if _, ok := fc.Lookup(o); !ok {
					n++
				}
			case !ix.Has(it.Icon):
				n++
			}
		}
	}
	return n
}
```

`handleStatus` 的循环里，在读到 `snap.B` 成功的那个分支补两行（`row.IconUnmatched = countIconUnmatched(snap.B, s.deps.Icons, s.deps.Cache)` 与 `tot.IconUnmatched += row.IconUnmatched`）。**坏档案那一行仍然报 0**：它的书签根本没读出来，编不出数——与 `rev/items` 同口径（"消失比报 0 更坏"说的是占行，不是造数）。

`cmd/launchbay/main.go` 补：

```go
fc, err := s.LoadIconCache()
if err != nil {
	log.Fatalf("图标缓存装配失败: %v", err)
}
// Dependencies{…, Icons: ix, Fetch: icons.NewFetcher(), Cache: fc}
```

- [ ] **Step 6: 补 `/status` 的测试**

`admin_test.go` 追加。**先改一处既有代码**：`TestStatusIsAdminOnlyAndAggregates` 里内联声明的响应 struct（`Profiles []struct{...}` 与 `Totals struct{...}`）各要补一个 `IconUnmatched int` 字段（tag `json:"iconUnmatched"`）——否则新字段解不出来、旧用例照样绿，那正是"加了字段没人断言"的形态。

```go
func TestStatus_缺图数读时算(t *testing.T) {
	s, h := setupBatch(t)
	// 四条：好名、陌生名、auto 无缓存、空 icon。setupBatch 给 dad 存的那条被这里覆掉。
	if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 1, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList, Accent: "#4c8dff",
		Items: []model.Item{
			{ID: "ok", Name: "好", URL: "http://10.0.0.1/", Icon: "tabler:home", Alias: []string{}, Tags: []string{}},
			{ID: "ghost", Name: "怪", URL: "http://10.0.0.2/", Icon: "tabler:nothere", Alias: []string{}, Tags: []string{}},
			{ID: "fav", Name: "图", URL: "http://10.0.0.3:8096/", Icon: "auto", Alias: []string{}, Tags: []string{}},
			{ID: "bare", Name: "空", URL: "http://10.0.0.4/", Alias: []string{}, Tags: []string{}},
		},
	}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadIconCache(); err != nil { // 建出 data/icons/ 与 index.json
		t.Fatal(err)
	}

	dadRow := func(t *testing.T) (int, int) {
		t.Helper()
		rec := getBearer(h, "/api/v1/status", adminTok)
		if rec.Code != 200 {
			t.Fatalf("status 得到 %d: %s", rec.Code, rec.Body)
		}
		var out struct {
			Profiles []struct {
				ID            string `json:"id"`
				IconUnmatched int    `json:"iconUnmatched"`
			} `json:"profiles"`
			Totals struct {
				IconUnmatched int `json:"iconUnmatched"`
			} `json:"totals"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		for _, pr := range out.Profiles {
			if pr.ID == "dad" {
				return pr.IconUnmatched, out.Totals.IconUnmatched
			}
		}
		t.Fatalf("dad 没在响应里: %+v", out.Profiles)
		return 0, 0
	}

	row, tot := dadRow(t)
	// 陌生名 + auto 无缓存 = 2；空 icon 不算缺（spec §5 的口径）。
	if row != 2 || tot != 2 {
		t.Fatalf("iconUnmatched = 行 %d 合计 %d，要 2/2", row, tot)
	}

	// 关键一步：给那条 auto 的 origin 放进缓存，数字必须当场跟着降。
	// 这就是"读时算"与"记成事件"的分水岭——记成事件的话这里还是 2。
	c, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put(icons.Origin("http://10.0.0.3:8096/"), []byte("图"), "png"); err != nil {
		t.Fatal(err)
	}
	if row, _ := dadRow(t); row != 1 {
		t.Errorf("缓存里已经有图了，iconUnmatched 还是 %d：数字没跟着缓存变", row)
	}
}
```

`admin_test.go` 的 import 要补 `internal/icons` 与 `internal/model`（该文件现在两个都没引——以前的 status 用例只碰档案元信息）。`getBearer(h, path, tok)` 与 `setupBatch(t)` 都是既有 helper。

- [ ] **Step 7: 跑全部测试**

Run: `go test ./... -count=1`
Expected: 全绿。**特别确认 Task 4 的两条反向证人仍然绿**：`/status` 现在拿索引判缺图了，但 `model.Validate` 与 `LoadBookmarks` 必须依旧不查索引。

- [ ] **Step 8: 【故意破坏】三次**

1. 把 `Put` 里"完全没变就不动代次"那三行删掉 → `TestIconCache_代次只在真变化时递增` 红。
2. 把 `InCooldown` 的 `e.Hash != ""` 判据去掉 → `TestIconCache_失败冷却24小时` 的最后一断言红（已有图还被锁死）。
3. 把 `iconExts` 里的 `"ico"` 删掉 → `TestIconCache_哈希与扩展名就是文件名形状` 红（**这一步同时是 Task 11 路由正则的证人**：两处后缀表必须一起改，改一处就该红）。

- [ ] **Step 9: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/store internal/httpapi cmd/launchbay/main.go
git commit -m "feat(store,httpapi): data/icons 内容哈希缓存与 24 小时冷却，/status 的缺图数改读时算"
```

---

## Task 8: `POST /api/v1/resolve`

**Files:**
- Create: `internal/httpapi/resolve.go`
- Create: `internal/httpapi/resolve_test.go`
- Modify: `internal/httpapi/routes.go`、`openapi.json`、`openapi_test.go`、`AGENTS.md`

**Interfaces:**
- Consumes: `Fetcher.Page/Image`、`Extract`、`IconCache.Put/Lookup/MarkFail/InCooldown`、`Index.Search`、`Store.Current`、`require(credScoped, profile)`
- Produces:

```go
type resolveRequest struct {
	URL     string `json:"url"`
	Profile string `json:"profile"`
}
type resolveGroup struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type resolveResponse struct {
	URL            string        `json:"url"`
	Title          string        `json:"title"`
	Desc           string        `json:"desc"`
	SuggestedGroup *resolveGroup `json:"suggestedGroup"`
	IconCandidates []string      `json:"iconCandidates"`
	Favicon        *string       `json:"favicon"`
}
func suggestGroup(b *model.Bookmarks, rawURL, title string) *resolveGroup
func iconCandidates(ix *icons.Index, rawURL, title string) []string
func urlTokens(rawURL, title string) []string
```

- [ ] **Step 1: 写失败测试**

```go
// internal/httpapi/resolve_test.go
package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/store"
)

// stubSite 是一个只在本机回环上跑的"目标站点"夹具。
// spec §5 的抓取约束只能靠真 HTTP 测，所以这里用 httptest 而不是假接口：
// 假接口会让"重定向 ≤3"这类判据在实现里失去意义。
type stubSite struct {
	srv     *httptest.Server
	hits    atomic.Int32
	imgHits atomic.Int32
}

func newStubSite(t *testing.T, html string, serveIcon bool) *stubSite {
	s := &stubSite{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		s.hits.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	})
	if serveIcon {
		mux.HandleFunc("/apple-touch-icon.png", func(w http.ResponseWriter, _ *http.Request) {
			s.imgHits.Add(1)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4})
		})
	}
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
	s, h := setupBatch(t)
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
	if !contains(got.IconCandidates, "simple-icons:docker") && !contains(got.IconCandidates, "tabler:docker") {
		t.Errorf("候选图标里没有 docker 相关的名字: %v", got.IconCandidates)
	}
	if got.Favicon == nil || !strings.HasPrefix(*got.Favicon, "/icons/") {
		t.Errorf("favicon 要落地成 /icons/…，得到 %v", got.Favicon)
	}
	if strings.Contains(rec.Body.String(), "正文不要") {
		t.Error("正文漏进响应了：spec §6.1 明写不留 HTML")
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
		{"管理员令牌也要显式申报", "dad", adminTok, http.StatusOK},
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
	raw, _ := json.Marshal(map[string]string{"url": site.srv.URL})
	if rec := doResolve(t, h, raw, dadTok); rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Errorf("不申报 profile 的 resolve 必须被拒，得到 %d", rec.Code)
	}
}

func TestResolve_形状与错误分流(t *testing.T) {
	site := newStubSite(t, `<html><head><title>x</title></head></html>`, false)
	_, h := setupBatch(t)
	tok := dadTok

	for _, tc := range []struct{ name, body string; want int }{
		{"非 http 协议", `{"url":"file:///etc/passwd","profile":"dad"}`, http.StatusUnprocessableEntity},
		{"缺 url", `{"profile":"dad"}`, http.StatusUnprocessableEntity},
		{"profile 不是 slug", `{"url":"` + site.srv.URL + `","profile":"../x"}`, http.StatusUnprocessableEntity},
		{"未知字段", `{"url":"` + site.srv.URL + `","profile":"dad","by":"ai"}`, http.StatusUnprocessableEntity},
		{"档案不存在", `{"url":"` + site.srv.URL + `","profile":"nobody"}`, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doResolve(t, h, []byte(tc.body), tok)
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
	// 页面里有 icon link，但图片端点回 404：resolve 仍要 200，favicon 为 null，
	// 并且该 origin 进了 24 小时冷却。
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
	site := newStubSite(t, `<html><head><title>x</title></head></html>`, true)
	s, h := setupBatch(t)
	before := revOf(t, s, "dad")
	_, filesBefore := snapshotCount(t, s, "dad")
	histBefore := historyLines(t, s, "dad")

	doResolve(t, h, resolveBody(t, "dad", site.srv.URL), dadTok)

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
```

装配全部走既有夹具：`setupBatch(t)` 建 `dad`（管理员，分组 `nas` 里一条 `dsm` 在 `http://10.0.0.3:5000`）与 `kid`，令牌常量 `adminTok`（作用域 `*`）与 `dadTok`（作用域 `dad`）就在 `batch_test.go` 顶部。本任务新增的小 helper 只有四个，全放 `testing_test.go`：`seedItem(t, s, profile, group, itemID, rawURL)`（经 `Store.Current` + `SaveBookmarks` 追加一条，**不经 REST**：测试种数据不需要绕一圈网络）、`revOf(t, s, id) int64`、`snapshotCount(t, s, id) (rev int64, files int)`、`historyLines(t, s, profile string) int`。**不要**在 `resolve_test.go` 里另造档案夹具；`doResolve` 是 `handlers_test.go#post(h, path, body, cookie, bearer)` 的一行薄壳（Bearer 位它本来就支持）。

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/httpapi/ -count=1 -run Resolve`
Expected: FAIL——404（路由还没注册）。

- [ ] **Step 3: 写 resolve.go**

```go
package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// resolveRequest 没有 by 字段：resolve 不写历史行（它只往 data/icons/ 落
// 缓存），给它一个 by 就是把一个没人消费的字段摆进契约。
type resolveRequest struct {
	URL     string `json:"url"`
	Profile string `json:"profile"`
}

type resolveGroup struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// resolveResponse 的键序 = 字段序（§4.3 对响应体成立）。
// suggestedGroup 用指针、favicon 用指针：这两个的"没有"是有效信息，
// 显式 null 比留空字段更容易让 AI 判出"没建议"而不是"字段丢了"。
type resolveResponse struct {
	URL            string        `json:"url"`
	Title          string        `json:"title"`
	Desc           string        `json:"desc"`
	SuggestedGroup *resolveGroup `json:"suggestedGroup"`
	IconCandidates []string      `json:"iconCandidates"`
	Favicon        *string       `json:"favicon"`
}

// handleResolve 把一条 URL 解析成添加书签时的候选。同一个实现供页面浮层
// 与 AI 共用（spec §6.1），所以它是 credScoped：要读该档案的现有分组才能
// 给"建议分组"，申报档案不是形式。
//
// 分流：形状与协议 → 422；凭证 → 401/403；档案 → 404/503/500；
// 目标抓不到 → 502。抓图失败不在此列——它降级成 favicon:null，因为没人
// 依赖 favicon 才肯存一条书签。
func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	var in resolveRequest
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if !model.ValidSlug(in.Profile) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "profile", Reason: "必须是 ASCII slug"}})
		return
	}
	if _, _, ok := s.require(r, credScoped, in.Profile); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	pageURL, uerr := parseResolveURL(in.URL)
	if uerr != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "url", Reason: uerr.Error()}})
		return
	}

	snap, err := s.deps.Store.Current(in.Profile)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "档案不存在"})
		return
	case err != nil:
		if errors.Is(err, store.ErrCorrupt) {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{
				Code: 503, Detail: corruptDetail(s.deps.Store, in.Profile, err)})
			return
		}
		writeInternalError(w, err)
		return
	}

	body, finalURL, err := s.deps.Fetch.Page(in.URL)
	if err != nil {
		// 502 而不是 500：失败在目标站点那边。detail 用 sentinel 自带的人话，
		// 一次都不拼 Go 内部错误——里面会有 URL、实现细节与换行。
		writeJSON(w, http.StatusBadGateway, errorResponse{Code: 502, Detail: resolveDetail(err)})
		return
	}
	page := icons.Extract(body, finalURL)
	resp := resolveResponse{
		URL:            in.URL,
		Title:          page.Title,
		Desc:           page.Description,
		SuggestedGroup: suggestGroup(snap.B, in.URL, page.Title),
		IconCandidates: iconCandidates(s.deps.Icons, in.URL, page.Title),
	}
	if fav := s.resolveFavicon(in.URL, page, finalURL); fav != "" {
		f := fav
		resp.Favicon = &f
	}
	writeJSON(w, http.StatusOK, resp)
}

// parseResolveURL 只放行 http/https 且必须有主机名。与 model.checkURL 的
// 区别是这里"要拿去抓"，file:// 与 gopher:// 不是可接受的抓取目标，
// 而它们作为书签值本身是合法的（spec §6.2 不设协议白名单）。
func parseResolveURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("resolve 只接受完整的 http/https 地址")
	}
	return u, nil
}

// resolveFavicon 只在缓存没有且不在冷却时抓图（spec §5 第 1 条：
// resolve 是唯一的即时抓取口，且它自己也不该每次重抓）。
func (s *Server) resolveFavicon(rawURL string, page icons.Page, finalURL string) string {
	origin := icons.Origin(rawURL)
	if origin == "" {
		return ""
	}
	if e, ok := s.deps.Cache.Lookup(origin); ok {
		return "/icons/" + e.Hash + "." + e.Ext
	}
	if s.deps.Cache.InCooldown(origin) {
		return ""
	}
	candidate := page.Favicon
	if candidate == "" {
		// 没写 link 标签就按约定路径试一次：内网服务十有八九挂着
		// /favicon.ico 而不在 HTML 里声明它（spec §5 那句"内网服务普遍没
		// favicon"说的正是这批，试一次的成本比让家里人自己填低）。
		candidate = origin + "/favicon.ico"
	}
	data, ext, err := s.deps.Fetch.Image(candidate)
	if err != nil {
		if markErr := s.deps.Cache.MarkFail(origin); markErr != nil {
			// 冷却记不下去，下次 resolve 还会重抓一次：可接受的退化，
			// 但要说出来——否则"失败 24 小时后再试"这句会变成假话。
			logResolveWarn(origin, markErr)
		}
		return ""
	}
	e, err := s.deps.Cache.Put(origin, data, ext)
	if err != nil {
		return "" // 存不下去就回空：抓到了但没落地，与没抓到在响应上等价
	}
	return "/icons/" + e.Hash + "." + e.Ext
}

func resolveDetail(err error) string {
	switch err {
	case icons.ErrBadURL:
		return "地址形状不合法"
	case icons.ErrUnreachable:
		return icons.ErrUnreachable.Error()
	case icons.ErrTimeout:
		return icons.ErrTimeout.Error()
	case icons.ErrStatus:
		return icons.ErrStatus.Error()
	case icons.ErrTooBig:
		return icons.ErrTooBig.Error()
	case icons.ErrWrongType:
		return icons.ErrWrongType.Error()
	case icons.ErrTooManyRedirects:
		return icons.ErrTooManyRedirects.Error()
	default:
		return "目标抓不下来" // 未知错误只回这一句，原始 err 进日志
	}
}
```

`logResolveWarn` 照 `batch.go` 里历史失败那两行的写法打 stderr（不带令牌、不带 URL 之外的个人信息；URL 是调用方自己给的，进 stderr 可接受）。

- [ ] **Step 4: 写建议分组与候选图标**

```go
// suggestGroup 只从**这个档案现有的分组**里挑，规则全是本地打分：
// 同 host 的兄弟条目 > 同首标签 > 分组 id/标题出现在地址或标题里。
// 挑不出就返回 nil——凭空造一个分组等于替用户决定他的书架长什么样。
func suggestGroup(b *model.Bookmarks, rawURL, title string) *resolveGroup {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	first := firstLabel(host)
	hay := strings.ToLower(rawURL + " " + title)

	best, bestScore := -1, 0
	for i, g := range b.Groups { // 档案顺序即优先级：同分时靠前的赢
		score := 0
		for _, it := range g.Items {
			iu, err := url.Parse(it.URL)
			if err != nil {
				continue
			}
			ih := strings.ToLower(iu.Hostname())
			switch {
			case host != "" && ih == host:
				score = max(score, 3)
			case len(first) > 2 && firstLabel(ih) == first:
				score = max(score, 2)
			}
		}
		if score < 1 && (containsFold(hay, g.ID) || containsFold(hay, g.Title)) {
			score = 1
		}
		if score > bestScore {
			bestScore, best = score, i
		}
	}
	if best < 0 {
		return nil
	}
	return &resolveGroup{ID: b.Groups[best].ID, Title: b.Groups[best].Title}
}

// iconCandidates 把地址与标题折成几个词，逐个在索引里找，品牌层优先。
// 上限 5 条：添加浮层里给 5 个候选已经要滚动了，10 个只会让人闭眼选第一个。
func iconCandidates(ix *icons.Index, rawURL, title string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, tok := range urlTokens(rawURL, title) {
		for _, prefix := range []string{"simple-icons", "tabler"} {
			res := ix.Search(tok, prefix, 2)
			for _, k := range res.Icons {
				if seen[k] {
					continue
				}
				seen[k] = true
				out = append(out, k)
			}
		}
		if len(out) >= 5 {
			return out[:5]
		}
	}
	return out
}

// urlTokens 取主机首标签、路径首段、标题前三个词。
// 顺序是有意义的：主机名比标题更接近"这到底是什么服务"。
func urlTokens(rawURL, title string) []string {
	var toks []string
	seen := map[string]bool{}
	push := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if len(s) < 2 || len(s) > 32 || seen[s] {
			return
		}
		seen[s] = true
		toks = append(toks, s)
	}
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		for _, seg := range strings.Split(u.Hostname(), ".") {
			if !reservedLabels[strings.ToLower(seg)] {
				push(seg)
			}
		}
		for _, seg := range strings.Split(strings.Trim(u.Path, "/"), "/") {
			push(seg)
		}
	}
	for _, w := range strings.FieldsFunc(title, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r >= 0x4e00)
	}) {
		push(w)
	}
	if len(toks) > 4 {
		toks = toks[:4]
	}
	return toks // 顺序即优先级：push 的调用次序已经排好（主机首标签 → 路径段 → 标题词）
}

// reservedLabels 滤掉那些出现在每个内网域名里、却说明不了任何事的段。
var reservedLabels = map[string]bool{
	"www": true, "lan": true, "local": true, "localdomain": true,
	"home": true, "internal": true, "intranet": true, "localhost": true,
}

func firstLabel(host string) string {
	if i := strings.IndexByte(host, '.'); i > 0 {
		return host[:i]
	}
	return host
}

func containsFold(hay, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	return len(needle) >= 3 && strings.Contains(hay, needle)
}
```

`sort.SliceStable(toks, func(i, j int) bool { return false })` 是一行**什么都不做还长得像在做**的代码，属于本仓库不养的东西：删掉它，靠 `push` 的调用顺序保证（`urlTokens` 本来就是按优先级追加的）。同理 `reservedLabels` 里 `"localdomain"` 写了两次，只留一个。

- [ ] **Step 5: 注册路由并同步契约**

`routes()` 追加：

```go
{Method: "POST", Pattern: "/api/v1/resolve", Cred: credScoped,
	Summary: "URL 解析成书签候选（含 favicon 抓取）", H: s.handleResolve},
```

`openapi.json`：把 `/api/v1/resolve` 换成真形状（`RequestBody` schema `{url, profile}` + `additionalProperties: false`；`200` 指向新增 `ResolveResponse`；`401/403/404/422/502` 各一条，`503` 复用既有坏档描述），并从 `openapi_test.go` 的 `planned` 表删掉 `"POST /api/v1/resolve": true`。**这张表从此应当为空**——P1-③ 之后没有计划中的端点了。

`AGENTS.md`：
- resolve 那一行换成真条目（`200 {url,title,desc,suggestedGroup,iconCandidates[],favicon}`），并在"## 图标"段落补一句：**"新加书签时可以先 `POST /api/v1/resolve` 拿标题/描述/候选图标，再 `POST /api/v1/batch` 落库；resolve 不写书签，`by` 字段它不接受。"**
- 把"该端点 P1-③ 才上线"两处（icons 与 resolve）全部撤干净。

- [ ] **Step 6: 跑测试确认它绿**

Run: `go test ./internal/httpapi/ -count=1`
Expected: 全绿。

- [ ] **Step 7: 【故意破坏】四次**

1. 把 `resolveFavicon` 里的 `Lookup` 短路删掉 → `TestResolve_第二次不重抓图` 红。
2. 把 `InCooldown` 那行删掉 → 新增一条"图端点持续 404、连点三次 resolve，第三次的 `imgHits` 必须是 0"的用例（本步顺手补上它），然后破坏 → 它红。**没有这条用例，冷却在端点层就是没接线的**。
3. 把 `suggestGroup` 的"挑不出返回 nil"改成返回 `b.Groups[0]` → `TestResolve_建议分组只从该档案现有分组里选` 的第一断言红。
4. 给 `resolveRequest` 加一个 `By string` 字段（哪怕不消费）→ `{"…","by":"ai"}` 那条 422 用例红（它验的正是"契约里没有的字段一个都不许长回来"）。

- [ ] **Step 8: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/httpapi AGENTS.md
git commit -m "feat(httpapi): POST /api/v1/resolve——标题/描述/建议分组/候选图标与只经此触发的 favicon 抓取"
```

---
## Task 9: 首屏图标内联与三级兜底

**Files:**
- Create: `internal/render/icon.go`
- Create: `internal/render/icon_test.go`
- Modify: `internal/render/render.go`（`New` 三参数、`cacheKey` 加 `iconGen`）
- Modify: `internal/render/exec.go`（`executor` 持索引与缓存、`itemView` 带图标）
- Modify: `internal/render/web/profile.tmpl`（三级兜底渲染）
- Modify: `internal/render/render_test.go`、`internal/render/views_test.go`（构造与断言）
- Modify: `internal/httpapi/testing_test.go`、`cmd/launchbay/main.go`（`render.New` 调用点）

**Interfaces:**
- Consumes: `icons.Index.Get/Has`、`icons.Origin`、`store.IconCache.Lookup/Generation`、`model.Item.Icon`
- Produces:

```go
func New(s *store.Store, ix *icons.Index, fc *store.IconCache) *Renderer
type iconPiece struct {
	SVG     template.HTML // 非空 = 内联 SVG
	Img     string        // 非空 = <img src>（缓存 favicon）
	Missing bool          // 名字查不到：卡片角标"图标未匹配"
	Letter  string        // 前两者都空时的首字母
	ChipHue int           // 首字母色块的色相（0-359）
}
func pieceFor(it model.Item, ix *icons.Index, fc *store.IconCache) iconPiece
func svgMarkup(ic icons.Icon) template.HTML
```

- [ ] **Step 1: 写失败的渲染测试**

```go
// internal/render/icon_test.go
package render

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

func TestPiece_有名就内联SVG(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "gh", Name: "GitHub", URL: "https://github.com/", Icon: "simple-icons:github"}, ix, fc)
	svg := string(p.SVG)
	if svg == "" {
		t.Fatal("品牌层命中却没给出 SVG")
	}
	if !strings.Contains(svg, `viewBox="0 0 24 24"`) || !strings.Contains(svg, "<path ") {
		t.Errorf("SVG 形状不对: %s", svg)
	}
	if !strings.Contains(svg, `aria-hidden="true"`) {
		t.Error("图标必须 aria-hidden：可及名由相邻的名字承担（spec §7.5）")
	}
	if !strings.Contains(svg, `class="ic-svg ic-fill"`) {
		t.Errorf("层标记要能让 CSS 选到呈现: %s", svg)
	}
	if p.Missing || p.Img != "" {
		t.Errorf("命中时不该带缺图标记或 img: %+v", p)
	}
}

func TestPiece_线条层带stroke标记(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "h", Name: "Home", URL: "http://10.0.0.1/", Icon: "tabler:home"}, ix, fc)
	if !strings.Contains(string(p.SVG), `ic-stroke`) {
		t.Errorf("tabler 必须是 stroke 层: %s", p.SVG)
	}
}

func TestPiece_陌生名字走首字母并标缺(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "x", Name: "青龙面板", URL: "http://10.0.0.2/", Icon: "tabler:nothere"}, ix, fc)
	if p.SVG != "" || p.Img != "" {
		t.Error("查不到就不该有图")
	}
	if !p.Missing {
		t.Error("名存实亡的图标要能被界面标出来（spec §12 的"不假装成功"）")
	}
	if p.Letter != "青" {
		t.Errorf("首字母要取第一个 rune，得到 %q", p.Letter)
	}
	if p.ChipHue < 0 || p.ChipHue > 359 {
		t.Errorf("色相越界: %d", p.ChipHue)
	}
}

func TestPiece_空icon是没设不是缺(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "x", Name: "X", URL: "http://10.0.0.2/"}, ix, fc)
	if p.Missing {
		t.Error("空 icon 不该打缺图标（口径见 spec §5）")
	}
	if p.Letter != "X" {
		t.Errorf("要首字母兜底，得到 %q", p.Letter)
	}
}

func TestPiece_auto有缓存就出图(t *testing.T) {
	ix, fc, s := assets(t)
	origin := icons.Origin("http://10.0.0.3:8096/")
	if _, err := fc.Put(origin, []byte{0x89, 'P', 'N', 'G'}, "png"); err != nil {
		t.Fatal(err)
	}
	p := pieceFor(model.Item{ID: "j", Name: "Jellyfin", URL: "http://10.0.0.3:8096/web/", Icon: "auto"}, ix, fc)
	if e, ok := fc.Lookup(origin); !ok || p.Img != "/icons/"+e.Hash+"."+e.Ext {
		t.Errorf("img 路径不对: %q", p.Img)
	}
	if p.SVG != "" {
		t.Error("favicon 绝不内联成 SVG：第三方字节只做 img（见 icon.go 注释）")
	}
	if p.Missing {
		t.Error("有图就不算缺")
	}
	_ = s
}

func TestPiece_auto无缓存算缺但不画坏(t *testing.T) {
	ix, fc, _ := assets(t)
	p := pieceFor(model.Item{ID: "j", Name: "Jellyfin", URL: "http://10.0.0.3:8096/", Icon: "auto"}, ix, fc)
	if p.Img != "" || p.SVG != "" {
		t.Error("没缓存不该有图")
	}
	if !p.Missing {
		t.Error("auto 且无缓存要进缺图口径（spec §5）")
	}
	if p.Letter == "" {
		t.Error("界面仍然只能走首字母，不能空着")
	}
}

func TestValidatedAccent_只认hex(t *testing.T) {
	// accent 会进 style 属性，而它是用户可写的字符串：判据只有这一条正则。
	// 非法值一律当"没写"，色块退回名称哈希——不报错、不猜、不洗。
	for accent, want := range map[string]string{
		"#4c8dff": "#4c8dff", "#abc": "#abc", "red": "", "#4c8dgff": "", "#4c8dff1": "", "": "",
	} {
		if got := validatedAccent(accent); got != want {
			t.Errorf("validatedAccent(%q) = %q，要 %q", accent, got, want)
		}
	}
}

// 两道闸的第二道：白名单在加载时拒（Task 1 的 TestDecode_未知tag未知attr与越界值都拒），
// 这里假设"万一有人绕过加载直接构造了 Icon"，markup 拼接仍然逃不出属性值。
func TestSvgMarkup_值一律转义(t *testing.T) {
	hostile := iconFixture("tabler", "x", "stroke", []icons.Element{{
		Tag:   "path",
		Attrs: []icons.Attr{{Name: "d", Value: `"/><script>alert(1)</script>`}},
	}})
	out := string(svgMarkup(hostile))
	if strings.Contains(out, "<script>") || strings.Contains(out, `"><script`) {
		t.Errorf("属性值逃出了转义: %s", out)
	}
	if !strings.Contains(out, "&#34;") && !strings.Contains(out, "&quot;") {
		t.Errorf("引号应当被转义，得到 %s", out)
	}
}

func TestProfilePage_内联数量与缓存代次(t *testing.T) {
	ix, fc, s := assets(t)
	saveBookmarks(t, s, "dad", withIcons(12)) // 12 条都有合法 icon（tabler:home）
	r := New(s, ix, fc)

	page, err := r.ProfilePage("dad")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(page.Body), "<svg "); n != 12 {
		t.Errorf("内联了 %d 个 SVG，要 12 个（每个用到的条目一个，不是整个索引）", n)
	}
	if strings.Contains(string(page.Body), `<img class="ic-img"`) {
		t.Error("这批条目没有 auto，不该出现 img")
	}

	again, _ := r.ProfilePage("dad")
	if again.ETag != page.ETag {
		t.Error("同 (档案,rev,mtime,gen) 必须命中缓存")
	}

	// 缓存 favicon 落进缓存后，页面必须重算——否则 resolve 抓到的图
	// 要等下次写书签才显示（spec §5 第 5 条点名的谎话）。
	auto := withIcons(0)
	auto.Groups[0].Items = []model.Item{{ID: "fav", Name: "F", URL: "http://10.0.0.9/", Icon: "auto"}}
	saveBookmarks(t, s, "dad", auto)
	if _, err := fc.Put(icons.Origin("http://10.0.0.9/"), []byte("img"), "png"); err != nil {
		t.Fatal(err)
	}
	fresh, err := r.ProfilePage("dad")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ETag == page.ETag {
		t.Fatal("图标缓存变了而页面 ETag 没变：缓存键里少了代次")
	}
	if !strings.Contains(string(fresh.Body), `<img class="ic-img"`) {
		t.Errorf("新页面应当端出 favicon: %.200s", fresh.Body)
	}
}
```

上面用到的 helper 里，`seeded(t)`、`save(t, s, id, title, itemID)`、`mustRender(t, s, id)` 是本包既有的；本任务新增的只有四个，全收进 `internal/render/helpers_test.go`：

```go
// assets 返回测试用的索引、缓存与档案目录。索引走真产物解析一次（sync.Once），
// 缓存建在 t.TempDir() 上——每个用例一份干净的，互不影响。
func assets(t *testing.T) (*icons.Index, *store.IconCache, *store.Store)

// saveBookmarks 用 store.SaveBookmarks 落一份给定的 Bookmarks（写路径校验照跑）。
func saveBookmarks(t *testing.T, s *store.Store, id string, b model.Bookmarks)

// withIcons 造 n 条 icon 均为 tabler:home 的书签；n == 0 时给一个空分组的壳。
func withIcons(n int) model.Bookmarks

// iconFixture 直接构造一个 icons.Icon（跳过 Decode），用来验渲染层的第二道闸。
func iconFixture(prefix, name, layer string, els []icons.Element) icons.Icon
```

上面片段的 import 以实际用到的为准（`store` 只有在显式写出类型时才需要引；Go 会因未使用的 import 直接不编译，这条不需要人提醒）。

`mustRender` 的装配点在本任务改成 `New(s, ix, fc)`，所以它内部要改成调 `assets(t)`——**先建 helper 再改构造点**：`render_test.go` 与 `views_test.go` 都会因 `New` 变三参数而红，两处各自临时装配一遍正是本次要收掉的病根。

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/render/ -count=1 -run TestPiece`
Expected: 编译失败（`pieceFor` 未定义、`New` 参数数量不符）。

- [ ] **Step 3: 写 render/icon.go**

```go
package render

import (
	"hash/fnv"
	"html"
	"html/template"
	"regexp"
	"strings"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// iconPiece 是一条书签的图标决定结果。它是**三取一**：SVG > Img > Letter。
// 三态同时非空是不可能的，pieceFor 按这个优先级只填一个。
type iconPiece struct {
	SVG     template.HTML // 非空 = 内联 SVG
	Img     string        // 非空 = <img src>（缓存 favicon）
	Missing bool          // 名查不到 / auto 无缓存：角标"图标未匹配"
	Letter  string        // 前两者都空时的首字母
	ChipHue int           // 首字母色块的色相（0-359）
}

// hexColorRE 是 accent 的唯一判据。它不是装饰：accent 是用户可写的字符串，
// 而它会进 style 属性。只认 #rgb/#rrggbb，其余一律当没写——
// 于是这里既不需要 template.HTML，也不需要"这条是不是颜色"的第二套逻辑。
var hexColorRE = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// pieceFor 决定一个条目怎么长出一张脸。
//
// 分组 accent 不在这里算：它是分组的属性，由 exec.go 用 validatedAccent 算一次
// 放进 groupView。在这里再算一遍就是第二个真相，两处判据迟早分家。
//
// 判据只有两个来源：图标索引（名字在不在）与图标缓存（auto 有没有抓过）。
// 索引里查不到的名字走首字母并标 Missing，**不报错也不清空磁盘上的值**——
// 索引是快照，快照会漂（spec §6.2 因此把存在性只卡在写路径）。
func pieceFor(it model.Item, ix *icons.Index, fc *store.IconCache) iconPiece {
	p := iconPiece{Letter: firstChar(it.Name), Accent: validatedAccent(accent)}
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

func validatedAccent(s string) string {
	if hexColorRE.MatchString(s) {
		return strings.ToLower(s)
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
// 元素名与属性名都来自加载时已过白名单的常量，属性值一律转义——所以即便
// 有人绕过 Decode 直接构造了一个 Icon，也拼不出新的元素或逃逸的属性。
// 加载侧的白名单是第一道闸，这里是第二道，两道互不信任（spec §11 的 XSS 行）。
func svgMarkup(ic icons.Icon) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg class="ic-svg ic-`)
	b.WriteString(ic.Layer)
	b.WriteString(`" viewBox="`)
	b.WriteString(html.EscapeString(ic.ViewBox))
	b.WriteString(`" aria-hidden="true" focusable="false">`)
	for _, el := range ic.Elements {
		b.WriteString("<")
		b.WriteString(el.Tag) // 白名单内的常量：不是数据
		for _, a := range el.Attrs {
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
```

- [ ] **Step 4: `render.New` 收两个依赖，缓存键加代次**

```go
type Renderer struct {
	s     *store.Store
	ix    *icons.Index
	fc    *store.IconCache
	tmpl  *executor
	mu    sync.Mutex
	cache map[cacheKey]Page
}

// New 的三个依赖都不许传 nil：真实装配点（cmd/launchbay）要么给全，
// 要么根本不启动。索引读不出来 = 内嵌产物损坏，降级成"没有图标"
// 会让一个坏镜像跑得很正常（Task 5 的 main.go 同理）。
func New(s *store.Store, ix *icons.Index, fc *store.IconCache) *Renderer {
	return &Renderer{s: s, ix: ix, fc: fc, tmpl: newExecutor(ix, fc), cache: map[cacheKey]Page{}}
}

type cacheKey struct {
	profile  string
	rev      int64
	modified time.Time
	iconGen  uint64 // 图标缓存的代次：spec §7.1
}
```

`ProfilePage` 里组键的地方改成：

```go
key := cacheKey{profile: id, rev: snap.Rev, modified: snap.Modified, iconGen: r.fc.Generation()}
```

`r.tmpl.profile(prof, snap.B)` 保持两参数（索引与缓存在 `newExecutor` 时已注入）。

- [ ] **Step 5: `exec.go` 与模板**

`executor` 多存一份 `ix/fc`（`newExecutor(ix, fc)`），`itemView` 加三个字段：

```go
type itemView struct {
	ID, Name, URL, Desc string
	Letter              string
	Alias, Tags         []string
	External            bool
	SVG                 template.HTML
	Img                 string
	Missing             bool
	ChipHue             int
}
```

`groupView` 加 `Accent string`（已验过的 hex，空串 = 没有）与 `Layout string`（`tiles|list` 原样带出，Task 10 的 CSS 靠它）。构造处：

```go
for _, g := range b.Groups {
	gv := groupView{ID: g.ID, Title: g.Title, Layout: g.Layout,
		Accent: validatedAccent(g.Accent)}
	for _, it := range g.Items {
		p := pieceFor(it, g.Accent, e.ix, e.fc)
		gv.Items = append(gv.Items, itemView{
			ID: it.ID, Name: it.Name, URL: it.URL, Desc: it.Desc,
			Alias: it.Alias, Tags: it.Tags,
			External: model.ExternalScheme(it.URL),
			SVG:      p.SVG, Img: p.Img, Missing: p.Missing,
			Letter: p.Letter, ChipHue: p.ChipHue,
		})
	}
	v.Bookmarks.Groups = append(v.Bookmarks.Groups, gv)
}
```

`profile.tmpl` 的卡片头一块（把原来固定的 `<span class="ic">{{.Letter}}</span>` 换成三取一）：

```gotemplate
{{define "icon"}}
{{- if .SVG}}
{{.SVG}}
{{- else if .Img}}
<img class="ic-img" src="{{.Img}}" alt="" aria-hidden="true" width="22" height="22">
{{- else}}
<span class="ic" style="--chip-h:{{.ChipHue}}{{with .Accent}};--chip-bg:{{.}}{{end}}" aria-hidden="true">{{.Letter}}</span>
{{- end}}
{{- end}}
```

调用处在两个分支（`External` 与非 `External`）各一次，并给缺图那条加角标：

```gotemplate
<a class="card" href="{{.URL}}"{{if $.Bookmarks.OpenInNewTab}} target="_blank" rel="noopener noreferrer"{{end}}>
  {{template "icon" .}}
  <b>{{.Name}}</b>
  {{with .Desc}}<i>{{.}}</i>{{end}}
  {{if .Missing}}<span class="badge">图标未匹配</span>{{end}}
  {{range .Alias}}<span class="tag">{{.}}</span>{{end}}
  {{range .Tags}}<span class="tag">{{.}}</span>{{end}}
</a>
```

**条目身上只带 `--chip-h:{{.ChipHue}}` 这一个数**：`ChipHue` 是 `int`，属性值里没有字符串可逃；分组色的入口只有一个（`section` 上的 `--sec-accent`，值来自 `validatedAccent`）。别把它换成 `{{.Style}}` 之类的"预拼好的串"——那等于把验色这道判断交还给模板，而模板不做判断。

- [ ] **Step 6: 跑测试确认它绿**

Run: `go test ./internal/render/ ./internal/httpapi/ -count=1`
Expected: 全绿（`views_test.go`/`render_test.go` 的既有断言若因结构变化红了，按"断言形状而不是文本"的既有口径改测试，**不许**为了让旧断言过而改模板结构）。

- [ ] **Step 7: 【故意破坏】四次**

1. 把 `svgMarkup` 里的 `html.EscapeString(a.Value)` 改成直接写值 → `TestSvgMarkup_值一律转义` 红。
2. 把 `pieceFor` 的 `case "":` 那行删掉 → 空 icon 会走到 `ix.Get("")` 失败 → `Missing` 被错置为真，`TestPiece_空icon是没设不是缺` 红。
3. 把 `cacheKey.iconGen` 从赋值处去掉（保留字段） → `TestProfilePage_内联数量与缓存代次` 的 ETag 断言红。**这是本任务最重要的一次破坏**：字段存在但没人填，是编译器抓不到的。
4. 把 `validatedAccent` 的正则换成 `^#` → `TestPiece_accent只认hex` 红。

- [ ] **Step 8: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/render internal/httpapi cmd/launchbay/main.go
git commit -m "feat(render): 首屏图标三级兜底内联，渲染缓存键带图标代次"
```

---

## Task 10: 极光玻璃外观与 §7.5 硬数字（接住部署验收 #1/#3）

**Files:**
- Modify: `internal/render/web/base.css`（整份重写）
- Modify: `internal/render/web/profile.tmpl`（卡片结构与 layout）
- Modify: `internal/render/web/picker.tmpl`（`data-theme` 与卡片类名）
- Modify: `internal/render/exec.go`（`profileView` 带 `Theme`、`Layout`）
- Modify: `internal/render/views_test.go`（CSS 关键规格的形状钉）

**Interfaces:**
- Consumes: Task 9 的 `itemView.SVG/Img/Missing/ChipHue`、`groupView.Layout/Accent`、`model.Profile.Theme`
- Produces: 类名契约（CSS 与模板共同遵守）：`.card`、`.ic-svg`、`.ic-fill`/`.ic-stroke`、`.ic-img`、`.ic`、`.badge`、`.tag`、`section[data-layout]`、`html[data-theme]`

- [ ] **Step 1: 先写"关键规格必须在场"的形状钉**

`views_test.go` 追加。**为什么用文本在场断言而不是只靠截图**：这些数字是 spec §7.5 与 §8.1 的硬规格，一次误删（比如重排 CSS 时删掉整个 `@media` 块）在截图上可能几小时都看不出来，而这里一行就红。

```go
func TestCSS_硬规格在场(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	wants := map[string]string{
		"毛玻璃卡片":        `backdrop-filter:blur(12px)`,
		"hover 抬升":       `translateY(-3px)`,
		"图标 hover 变换":   `scale(1.07) rotate(-2deg)`,
		"磁贴内线条加粗":     `.tiles .ic-svg{stroke-width:2.5`,
		"图标尺寸下限 20px":  `.ic-svg,.ic-img{width:22px;height:22px`,
		"触摸目标 44px":    `min-height:44px`,
		"减弱动效豁免":      `@media (prefers-reduced-motion:reduce)`,
		"高对比模式豁免":     `@media (forced-colors:active)`,
		"列表形态":        `[data-layout=list] ul{grid-template-columns:minmax(0,1fr)`,
		"暗亮双主题开关":     `[data-theme=light]`,
		"入场 stagger":     `@keyframes rise{from{opacity:0;transform:translateY(9px)}`,
		"网点纹理":         `background-size:3px 3px`,
	}
	for name, want := range wants {
		if !strings.Contains(css, want) {
			t.Errorf("%s 必须写在 CSS 里（原样找 %q）", name, want)
		}
	}
}
```

- [ ] **Step 2: 跑它，确认它红**

Run: `go test ./internal/render/ -count=1 -run TestCSS`
Expected: FAIL，逐条列出不在场的规格。

- [ ] **Step 3: 重写 base.css**

```css
:root{
  color-scheme:dark;
  --bg:#0b0e14;--ink:#e9edf8;--ink2:#a7b2cd;--line:rgba(255,255,255,.14);
  --card:rgba(255,255,255,.055);--card-hi:rgba(255,255,255,.10);
  --accent:#4c8dff;--ring:#7fa8ff;--warn:#ff9d6c;
  --aurora-1:rgba(76,141,255,.30);--aurora-2:rgba(150,80,255,.22);--aurora-3:rgba(0,200,180,.18);
}
/* theme 只有三个合法值（model.Profile.Theme 的枚举）。auto 交回媒体查询，
   dark/light 是显式覆写——以前只有媒体查询，于是 profile.theme 写了也白写，
   亮暗两档实拍拍到的是同一张图（部署验收发现的第 4 类谎话）。 */
[data-theme=light]{color-scheme:light;--bg:#f6f4ef;--ink:#22201c;--ink2:#5d584e;
  --line:#e3ddd1;--card:#fff;--card-hi:#fff;
  --aurora-1:rgba(76,141,255,.16);--aurora-2:rgba(150,80,255,.10);--aurora-3:rgba(0,200,180,.10)}
[data-theme=dark]{color-scheme:dark}
@media (prefers-color-scheme:light){
  :root:not([data-theme]){color-scheme:light;--bg:#f6f4ef;--ink:#22201c;--ink2:#5d584e;
    --line:#e3ddd1;--card:#fff;--card-hi:#fff;
    --aurora-1:rgba(76,141,255,.16);--aurora-2:rgba(150,80,255,.10);--aurora-3:rgba(0,200,180,.10)}
}
*{box-sizing:border-box}
html{background:var(--bg)}
body{margin:0;color:var(--ink);font:15px/1.6 -apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;
  padding:20px;min-height:100vh;position:relative}
/* 极光底：三层径向渐变 + 46px 模糊，固定层不滚（spec §8.1）。
   用 ::before 而不是 body 背景：模糊需要一张独立的层，跟着内容一起滚会糊到字上。 */
body::before{content:"";position:fixed;inset:-120px;z-index:-1;
  background:radial-gradient(50% 40% at 18% 22%,var(--aurora-1),transparent 60%),
    radial-gradient(45% 35% at 82% 12%,var(--aurora-2),transparent 62%),
    radial-gradient(55% 45% at 60% 88%,var(--aurora-3),transparent 60%);
  filter:blur(46px);animation:drift 16s ease-in-out infinite alternate}
@keyframes drift{from{transform:translate3d(-1.5%,-1%,0) scale(1.02)}to{transform:translate3d(2%,1.5%,0) scale(1.05)}}
header{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:18px}
header h1{font-size:19px;margin:0}
main{max-width:1180px;margin:0 auto}
section{margin:22px 0}
h2{font-size:13px;letter-spacing:.1em;text-transform:uppercase;color:var(--ink2);margin:0 0 10px;
  display:flex;align-items:center;gap:8px}
h2::before{content:"";width:10px;height:10px;border-radius:3px;background:var(--sec-accent,var(--accent));flex:0 0 auto}
ul{list-style:none;margin:0;padding:0;display:grid;gap:10px;grid-template-columns:repeat(auto-fill,minmax(190px,1fr))}
li{min-width:0}
/* 卡片本体（spec §8.1：半透明 + blur + 1px 描边，hover 抬 3px、描边转亮蓝、图标微变换）。
   .plain 是"需手动打开"那条的非链接形态：与卡片同一张脸，只是不可点。 */
.card,.pick,.plain{display:flex;align-items:center;gap:10px;min-width:0;
  padding:12px;border:1px solid var(--line);border-radius:14px;
  background:var(--card);backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);
  color:inherit;text-decoration:none;min-height:56px;
  transition:transform .16s ease,border-color .16s ease,background .16s ease}
.card:hover,.pick:hover{transform:translateY(-3px);border-color:var(--accent);background:var(--card-hi)}
.card:hover .ic-svg,.card:hover .ic-img,.card:hover .ic{transform:scale(1.07) rotate(-2deg)}
.card:focus-visible,.pick:focus-visible{outline:3px solid var(--ring);outline-offset:2px}
.card>*{min-width:0}
.card b{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.card i{font-style:normal;font-size:12px;color:var(--ink2);margin-left:auto;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:0 1 auto;max-width:38%}
/* 图标：尺寸下限 20px（§7.5）。22px 是实测选值——20px 时 tabler 的 2px 线在
   渐变底上已经发糊，而再大就会把 190px 的磁贴挤成两行标题。 */
.ic-svg,.ic-img{width:22px;height:22px;flex:0 0 auto;border-radius:6px;transition:transform .16s ease}
.ic-svg{fill:none;stroke:currentColor;stroke-linecap:round;stroke-linejoin:round;stroke-width:2}
.ic-fill{fill:currentColor;stroke:none}
.tiles .ic-svg{stroke-width:2.5} /* §7.5：磁贴里线条加粗，list 形态保持 2 */
.ic-img{object-fit:contain;background:var(--card-hi);padding:1px}
.ic{width:22px;height:22px;flex:0 0 auto;border-radius:7px;display:grid;place-items:center;
  font-size:12px;font-weight:700;color:#fff;transition:transform .16s ease;
  background:hsl(var(--chip-h,222) 62% 46%)}
.ic[style*="--chip-bg"]{background:var(--chip-bg)}
/* 入场：磁贴按序 stagger（spec §8.1：translateY(9px)→0，单条 0.5s）。
   用 nth-child 写死前 12 条，而不是给每个条目注入 --i：那要模板携带序号，
   而 12 条之后已经看不出差分——多出来的那半套机制没有可感知的收益。 */
@keyframes rise{from{opacity:0;transform:translateY(9px)}to{opacity:1;transform:none}}
.card{animation:rise .5s ease backwards}
li:nth-child(2) .card{animation-delay:.04s}
li:nth-child(3) .card{animation-delay:.08s}
li:nth-child(4) .card{animation-delay:.12s}
li:nth-child(5) .card{animation-delay:.16s}
li:nth-child(6) .card{animation-delay:.2s}
li:nth-child(7) .card{animation-delay:.24s}
li:nth-child(8) .card{animation-delay:.28s}
li:nth-child(9) .card{animation-delay:.32s}
li:nth-child(10) .card{animation-delay:.36s}
li:nth-child(11) .card{animation-delay:.4s}
li:nth-child(12) .card{animation-delay:.44s}
/* 网点纹理压在极光之上（spec §8.1 那句"叠一层 3px 网点"）。
   pointer-events:none 不是装饰：固定层盖住内容会把整页点击吃掉。 */
body::after{content:"";position:fixed;inset:0;z-index:-1;pointer-events:none;
  background-image:radial-gradient(rgba(255,255,255,.05) 1px,transparent 1px);
  background-size:3px 3px}
/* 别名/标签是 flex 行里的短词：不给 nowrap + 不许收缩，窄屏上会被挤成一列竖字
   （Task 12 手机档实拍发现，部署验收 #1 的另一半）。 */
.badge,.tag{font-size:11px;border-radius:6px;padding:2px 6px;white-space:nowrap;flex:0 0 auto}
.badge{border:1px solid var(--warn);color:var(--warn)}
.tag{border:1px solid var(--line);color:var(--ink2)}
.tags-row{display:flex;flex-wrap:wrap;gap:4px;min-width:0}
/* 需手动打开那条：整串 URL 是纯文本，让它换到自己那一行，名字与徽标各保一行。 */
.plain{flex-wrap:wrap}
.plain code{flex:1 0 100%;font-size:12px;color:var(--ink2);overflow-wrap:anywhere}
/* layout: list = 单列行；tiles = 大磁贴（spec §8.3 的 min 158px 由 190px 的
   auto-fill 下限覆盖，窄屏两列时仍不至于把标题压成竖排）。 */
[data-layout=list] ul{grid-template-columns:minmax(0,1fr)}
.profiles{grid-template-columns:repeat(auto-fill,minmax(150px,1fr))}
.notice{margin:0 0 16px;padding:10px 14px;border:1px solid var(--warn);border-radius:12px;
  background:rgba(255,157,108,.12);color:var(--ink)}
.pick{font:inherit;color:inherit;flex-direction:column;align-items:stretch;gap:10px;min-height:0}
.pick-name{display:flex;align-items:center;gap:10px;min-width:0}
.pick label{display:flex;align-items:center;gap:8px;font-size:13px;color:var(--ink2)}
.pick input{font:inherit;color:inherit;background:transparent;border:1px solid var(--line);
  border-radius:9px;padding:6px 8px;min-width:0;flex:1 1 auto}
.pick button{font:inherit;cursor:pointer;color:inherit;background:transparent;
  border:1px solid var(--line);border-radius:9px;padding:8px 10px;min-height:44px}
input[type=password],input[type=text]{min-height:44px}
@media (max-width:700px){ul{grid-template-columns:repeat(2,minmax(0,1fr))}.card i{display:none}}
@media (max-width:430px){ul{grid-template-columns:minmax(0,1fr)}}
@media (prefers-reduced-motion:reduce){
  body::before{animation:none}
  .card{animation:none}
  *{transition:none!important}
  .card:hover,.pick:hover{transform:none}
  .card:hover .ic-svg,.card:hover .ic-img,.card:hover .ic{transform:none}
}
/* 高对比模式：毛玻璃与阴影会被系统色盖掉，只留边框与文字（spec §7.5）。 */
@media (forced-colors:active){
  .card,.pick,.plain{backdrop-filter:none;background:Canvas;border:1px solid CanvasText}
  body::before{display:none}
  .ic{border:1px solid CanvasText}
}
```


- [ ] **Step 4: 模板按类名契约改**

关于 §8.1 那句"动效只用 `transform` / `opacity` / View Transitions，不碰会触发重排的属性"：本计划的 hover 除了 `transform` 还过渡了 `border-color` 与 `background`。这两个只触发重绘、不触发重排（改的是 paint 而非几何），所以留着的代价是零，而去掉它们会让"描边转亮蓝"这条 §8.1 明写的效果消失。`outline` 只在 `:focus-visible` 上出现、不参与过渡（它每次都重绘整层，做过渡会掉帧）。

`profile.tmpl` 的 `<li>` 两个分支统一走 `.card` / `.plain`，`section` 带 layout 与 accent：

```gotemplate
{{range .Bookmarks.Groups}}
<section aria-labelledby="g-{{.ID}}" data-layout="{{.Layout}}"{{with .Accent}} style="--sec-accent:{{.}}"{{end}}>
  <h2 id="g-{{.ID}}">{{.Title}}</h2>
  <ul class="{{if eq .Layout "tiles"}}tiles{{else}}rows{{end}}">
  {{range .Items}}
    <li>
      {{if .External}}
        <span class="plain">
          {{template "icon" .}}
          <b>{{.Name}}</b>
          <span class="badge">需手动打开</span>
          <code>{{.URL}}</code>
          {{if .Missing}}<span class="badge">图标未匹配</span>{{end}}
          <span class="tags-row">{{range .Alias}}<span class="tag">{{.}}</span>{{end}}{{range .Tags}}<span class="tag">{{.}}</span>{{end}}</span>
        </span>
      {{else}}
        <a class="card" href="{{.URL}}"{{if $.Bookmarks.OpenInNewTab}} target="_blank" rel="noopener noreferrer"{{end}}>
          {{template "icon" .}}
          <b>{{.Name}}</b>
          {{with .Desc}}<i>{{.}}</i>{{end}}
          {{if .Missing}}<span class="badge">图标未匹配</span>{{end}}
          <span class="tags-row">{{range .Alias}}<span class="tag">{{.}}</span>{{end}}{{range .Tags}}<span class="tag">{{.}}</span>{{end}}</span>
        </a>
      {{end}}
    </li>
  {{end}}
  </ul>
</section>
{{end}}
```


`picker.tmpl` / `broken.tmpl` / `profile.tmpl` 的 `<html>` 都带主题：

```gotemplate
<html lang="zh-CN"{{with .Theme}} data-theme="{{.}}"{{end}}>
```

`exec.go` 的三个视图结构各加 `Theme string`，取 `model.Profile.Theme`；选择页与坏档页给空串（没有档案就没有主题，`auto` 由媒体查询接手）。

- [ ] **Step 5: 跑测试确认绿**

Run: `go test ./internal/render/ -count=1`
Expected: `TestCSS_硬规格在场` 与既有视图测试全绿。若既有断言因类名改了而红，按"断言形状"的口径更新断言（例如原来断 `li a`，现在断 `.card`），**不要**回滚类名去迁就旧断言。

- [ ] **Step 6: 【故意破坏】三次**

1. 删掉 `@media (prefers-reduced-motion:reduce)` 整块 → 形状钉红。
2. 把 `.tiles .ic-svg{stroke-width:2.5}` 的 2.5 改成 2 → 形状钉红（§7.5 那条数字不是装饰）。
3. 把 `data-layout` 属性从模板里去掉 → `[data-layout=list]` 的选择器变成死码；形状钉仍然绿（它只查 CSS 在场）。**所以本步要补一条模板侧断言**：`views_test.go` 里加一条"给两个分组分别设 tiles/list，渲染出的 HTML 必须各自带 `data-layout="tiles"` 与 `data-layout="list"`"，然后破坏它——这条同时钉住部署验收 #3（`Group.layout` 没被用）。

- [ ] **Step 7: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/render
git commit -m "feat(render): 极光玻璃外观、layout/accent/theme 落地与 §7.5 硬数字"
```

---
## Task 11: `GET /icons/{name}` 静态通路

**Files:**
- Create: `internal/httpapi/iconfile.go`
- Create: `internal/httpapi/iconfile_test.go`
- Modify: `internal/httpapi/routes.go`、`openapi.json`、`AGENTS.md`

**Interfaces:**
- Consumes: `Store.IconsDir()`、Task 7 定的文件名形状（`<16 hex>.<ext>`）
- Produces: 路由 `GET /icons/{name}`（`credNone`），响应头 `Cache-Control: public, max-age=31536000, immutable`

- [ ] **Step 1: 写失败测试**

```go
// internal/httpapi/iconfile_test.go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

func putBlob(t *testing.T, s *store.Store, hash, ext string, data []byte) {
	t.Helper()
	dir := s.IconsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, hash+"."+ext), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIconFile_成功端出字节与immutable(t *testing.T) {
	s, h := setup(t)
	putBlob(t, s, "0123456789abcdef", "png", []byte{0x89, 'P', 'N', 'G', 1})

	rec := get(h, "/icons/0123456789abcdef.png", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q，要 §7.1 那一行逐字一致", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q", got)
	}
	// 这条也在 /api/v1 前缀之外：no-store 中间件不许顺手盖掉 immutable。
	if strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Error("no-store 漏进了 /icons/*")
	}
	if string(rec.Body.Bytes()) != string([]byte{0x89, 'P', 'N', 'G', 1}) {
		t.Error("字节被改过：这个端点的契约就是原样端出")
	}
}

func TestIconFile_形状不对一律404不解释(t *testing.T) {
	s, h := setup(t)
	putBlob(t, s, "0123456789abcdef", "png", []byte("x"))
	for _, bad := range []string{
		"/icons/0123456789abcde.png",       // 15 位
		"/icons/0123456789abcdef0.png",     // 17 位
		"/icons/0123456789ABCDEF.png",      // 大写：哈希一律小写，别放宽
		"/icons/0123456789abcdef.exe",      // 后缀不在表内
		"/icons/index.json",                // 索引本身不是图片
		"/icons/..%2f..%2fdata%2fprofiles%2fdad%2fbookmarks.json", // 编码过的穿越
	} {
		rec := get(h, bad, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s 要 404，得到 %d", bad, rec.Code)
		}
		// 404 不回显细节：这是图片通路，不是诊断通路。
	}
}

func TestIconFile_形状对但没这个文件(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/icons/ffffffffffffffff.png", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("得到 %d：不存在与形状非法对调用方是同一句话", rec.Code)
	}
}

func TestIconFile_不公开目录清单(t *testing.T) {
	_, h := setup(t)
	for _, p := range []string{"/icons/", "/icons", "/icons/*"} {
		if rec := get(h, p, ""); rec.Code == http.StatusOK {
			t.Errorf("%q 竟然 200：缓存内容可枚举就等于给站级缓存开了目录", p)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/httpapi/ -count=1 -run IconFile`
Expected: FAIL——404 来自 `GET /` 的兜底（选择页），断言 `Cache-Control` 那步就会红。

- [ ] **Step 3: 写 handler**

```go
// internal/httpapi/iconfile.go
package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// iconFileNameRE 与 store.iconExts、icons.Fetcher.Image 的后缀表是同一件事
// 的三处表述——改一处必须改三处，Task 7 的破坏 #3 就是钉这个的。
//
// 只认 <16 位小写十六进制>.<白名单后缀>：形状即边界。它同时挡掉路径穿越
// （没有 '/'，也没有 ".."）与"把任意文件当图片端出去"。
var iconFileNameRE = regexp.MustCompile(`^[0-9a-f]{16}\.(png|ico|svg|webp|gif)$`)

var iconContentTypes = map[string]string{
	"png":  "image/png",
	"ico":  "image/x-icon",
	"svg":  "image/svg+xml",
	"webp": "image/webp",
	"gif":  "image/gif",
}

// handleIconFile 端出 resolve 抓来的 favicon 缓存（spec §5、§7.1）。
//
// 公开且 immutable：文件名就是内容哈希，改内容必然改名字。它不含任何
// 档案信息，所以 credNone 够用。
//
// 第三方字节只走 `<img src>`：渲染层绝不把它当 markup 拼进 HTML——浏览器在
// img 语境里不执行 SVG 内脚本，而一旦被内联，那个沙箱假设就没了（Task 9）。
func (s *Server) handleIconFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !iconFileNameRE.MatchString(name) {
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "没有这个图标"})
		return
	}
	ext := name[len(name)-strings.LastIndex(name, ".")-1:]
	data, err := os.ReadFile(filepath.Join(s.deps.Store.IconsDir(), name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "没有这个图标"})
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
```

`strings` 要 import；`ext := …` 那行写成 `ext := name[strings.LastIndexByte(name, '.')+1:]`（上面那式子算的是"点号到末尾的长度"，读起来绕，实现者照直写）。

- [ ] **Step 4: 注册并同步契约**

```go
{Method: "GET", Pattern: "/icons/{name}", Cred: credNone,
	Summary: "resolve 抓来的 favicon 缓存（内容哈希命名，immutable）", H: s.handleIconFile},
```

`openapi.json` 加这一条（`parameters` 里 `name` 是 path 参数，`schema.pattern` 抄上面那条正则；`200` 是 `content: image/*` 的 binary；`404` 复用 `Error`）。`AGENTS.md` 的端点表加一行，**逐字含 `/icons/{name}`**——`TestAgentsMDMatchesRoutes` 的正向闸只豁免 `/`、`/pin`、`/healthz` 三条浏览器通路，这条不在其中，文档漏了就红。

`AGENTS.md` 的"## 图标"段补一句给 AI 的话：`resolve` 返回的 `favicon` 字段就是这个路径，它是给浏览器 `<img>` 用的，**不要**把它的值写进 `icon` 字段。

- [ ] **Step 5: 跑测试确认它绿**

Run: `go test ./internal/httpapi/ -count=1`
Expected: 全绿，含四条契约测试。

- [ ] **Step 6: 【故意破坏】两次**

1. 把正则的 `{16}` 放宽成 `+` → `TestIconFile_形状不对一律404不解释` 的 15/17 位两条红。
2. 把 `Cache-Control` 那行删掉 → 第一个测试红（§7.1 那一行由测试逐字钉住）。**再补一次**：把 `iconFileNameRE` 换成"存在即端出"（不做形状检查）→ 编码穿越那条红，证明正则不是装饰。

- [ ] **Step 7: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/httpapi AGENTS.md
git commit -m "feat(httpapi): GET /icons/{name} 内容哈希图片通路（immutable + 形状即边界）"
```

---

## Task 12: 端到端验收——真实补一轮图标 + 三档双主题实拍 + 体积记录

**Files:**
- Create: `.superpowers/tmp/acc/plan3/`（截图与读数，**不进仓库**）
- Modify: 无源码改动（发现问题回到对应任务修，并在那儿的账本记一笔）

**Interfaces:**
- Consumes: 全部前序任务
- Produces: 一份验收报告（`.superpowers/sdd/<本计划>/acceptance.md`）：镜像体积、首屏字节数、内联 SVG 计数、resolve 读数、`iconUnmatched` 前后、九张实拍图的路径、泄漏守卫结果

> 这一步是 spec §13 第四条关键回归。它验的不是"每段代码各自绿"，而是**索引、写校验、resolve、渲染、缓存五段接起来真的通**——这五段各自有单测，接缝处没有。

- [ ] **Step 1: 重建镜像并记体积（§10 的红线在这里复量）**

```bash
LAUNCHBAY_HOST_PORT=17600 docker compose up -d --build
docker image inspect launchbay:0.1.0 --format '{{.Size}}'
docker ps --filter name=launchbay --format '{{.Status}}'
curl -fsS http://127.0.0.1:17600/healthz
```

Expected: 体积 ≤ 20MB（P1-② 结束时是 10.4MB，两套 gz 索引约 2.1MB，预期落在 ~12.5MB）；`Up (healthy)`；`ok`。

**破线就停**：不要"先上线再说"。按 spec §5 的阶梯砍一档（先 `aliases`）再复跑，并在报告里写下砍了什么、为什么。

不 `docker prune`、不动别人起的容器、不删卷——这条是本案运维边界。

- [ ] **Step 2: 看清现状（别信记忆里的数据形状）**

```bash
B=http://127.0.0.1:17600
TOK=$(grep LAUNCHBAY_TOKEN_ADMIN .env | cut -d= -f2)
curl -s -H "Authorization: Bearer $TOK" "$B/api/v1/status" | head -c 900
curl -s -H "Authorization: Bearer $TOK" "$B/api/v1/data?profile=dad" | head -c 500
```

记下 `iconUnmatched` 的**前值**（大概率非零：夹具条目要么没设 icon，要么设的是 P1-② 时代靠猜的名字）。

- [ ] **Step 3: 真实补一轮图标（AI 通路自己吃自己的狗粮）**

对 `dad` 档案逐条走"搜索 → 挑名 → 一批写入"：

```bash
curl -s "$B/api/v1/icons?q=jellyfin" | head -c 300
curl -s "$B/api/v1/icons?q=syncthing&prefix=simple-icons" | head -c 300
```

写回时**必须**照 AGENTS.md 的坑 4/5：body 用 UTF-8 工具写文件、`--data-binary @batch.json`、带 `expectedRev`、一批写完再回读。

```bash
# 例：batch.json（由编辑工具写成 UTF-8，不在命令行里塞中文）
# {"profile":"dad","expectedRev":<前值>,"by":"ai","ops":[
#   {"op":"set","path":"groups/media/items/jellyfin/icon","value":"simple-icons:jellyfin"}, …]}
curl -s -X POST -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  --data-binary @batch.json "$B/api/v1/batch"
curl -s -H "Authorization: Bearer $TOK" "$B/api/v1/data?profile=dad" | head -c 600
```

断言（报告里逐条写结果，不写"看起来对"）：

1. batch 回 `applied` 与 ops 数一致；`rev` = 前值 + 1。
2. 回读 `/api/v1/data` 里每条 `icon` 都是**搜索端点返回过的规范名**（不是猜的）。
3. `GET /api/v1/status` 的 `iconUnmatched` **降到 0**（或报告出降到几条、为什么剩着）。
4. 故意再写一个假名 `tabler:notarealicon` → 必须 **422 且 `errors[0].field == "icon"`、`opsIndex` 指对那条 op**，然后确认档案没变（`rev` 不动、无新快照）。这条是把 Task 4 的闸门在真实通路上再验一次。

- [ ] **Step 4: resolve 端到端（只走本机回环）**

起一个一次性夹具站点（只在本机，端口 18080），它提供 `<title>`、`og:description`、`apple-touch-icon`。

**写法要求**：这个脚本必须先用编辑工具落成文件（`.superpowers/tmp/acc/plan3/site.cjs`）再 `node site.cjs &`，
**不要**用 `node -e '…'` 内联。本机踩过两次：Git Bash 里内联脚本中的引号/反引号会被剥掉，
而带中文的 `-e` 参数会按 GBK 送出去——症状是站点起来了但 `<title>` 是乱码，
然后 resolve 的验收读数变成假的。

```bash
node .superpowers/tmp/acc/plan3/site.cjs &   # 后台起，端口 127.0.0.1:18080
```

```bash
printf '{"url":"http://127.0.0.1:18080/","profile":"dad"}' > /tmp/resolve.json
curl -s -X POST -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  --data-binary @/tmp/resolve.json "$B/api/v1/resolve"
curl -sI "$B/icons/<上一响应的 favicon>" | grep -iE "cache-control|content-type"
```

断言：`title`/`desc` 正确；`iconCandidates` 里有 `simple-icons:syncthing`；`suggestedGroup` 落进 `dad` 档案里已有 syncthing 条目所在的那组（没有就 `null`，不许编）；`favicon` 是 `/icons/<16hex>.png`，`curl -sI` 回 `immutable`；`data/icons/` 里多一个 blob 且 `index.json` 认得它；再打一次 resolve，`data/icons/` 不再长第二个文件（去重与不重抓）。

- [ ] **Step 5: 首屏形状（关 JS 也能看能点）**

```bash
curl -s -o /tmp/page.html -D /tmp/h.txt -b "lb_profile=dad" "$B/"
grep -o "<svg " /tmp/page.html | wc -l
grep -o "<img class=\"ic-img\"" /tmp/page.html | wc -l
grep -c "需手动打开" /tmp/page.html
wc -c /tmp/page.html
grep -iE "cache-control|etag|content-security|x-lb-profile" /tmp/h.txt
```

断言：内联 `<svg>` 数 == 有合法图标名的条目数；除 resolve 抓来的那批 favicon 外**零图片请求**（HTML 里不出现别的 `<img`）；页面里不含 `http://192.168`（泄漏守卫的运行时一面对照静态一面）；字节数量级与 spec §5 的"42 条约 10KB 内联图标"相符（记录实际值，异常增大要查是不是把整个索引端了出去——**那是本期最坏的一种失败**）。

- [ ] **Step 6: 三档 × 双主题实拍 + 目检**

沿用 P1-② 部署验收的 CDP 通路（`.superpowers/tmp/acc/shoot.mjs`），本任务要的参数变化是：`Emulation.setEmulatedMedia` 分别设 `dark`/`light`、`setDeviceMetricsOverride` 分别设 1440×900、768×1024、390×844（**窄屏必须走 override，`--window-size` 会被夹宽再裁**），cookie 带 `lb_profile=dad`，输出到 `.superpowers/tmp/acc/plan3/`，命名 `dad-<宽>-<scheme>.png`，共 6 张；再补两张：`tiles` 与 `list` 两种 `Group.layout` 各一张（改磁盘夹具或走 batch 设 layout 都行，但**报告里要写清是哪一种**）与一张"陌生图标名"的缺图角标形态。

每张都用 `Read` 工具**目检**（`zoom=2` 看细节，整页小图会漏真 bug），核对清单：

- [ ] 卡片没有溢出/挤压：长中文名以省略号收尾，标签不换成一列竖字（验收 #1）
- [ ] `accent` 与首字母色块都看得出颜色差异（验收 #3）
- [ ] 内联 SVG 清楚不糊（tabler 在磁贴里确实比列表粗）
- [ ] 亮档文字对比度肉眼可讀（**算**对比度是 P1-④ 的活，这里只排"糊/看不见"）
- [ ] 暗档焦点环在卡片上可见（Tab 键走一遍，不是只看 CSS）
- [ ] 手机档（390）单列/两列都无横向滚动条
- [ ] `prefers-reduced-motion` 下极光不漂（设 `reduce` 拍一张对比）

- [ ] **Step 7: 泄漏守卫（本期新增面也要扫）**

```bash
git ls-files -z | xargs -0 grep -nEo '([0-9]{1,3}\.){3}[0-9]{1,3}' 2>/dev/null \
  | grep -vE '\b(10\.|127\.|0\.0\.0\.0|255\.255|1\.1\.1\.1)' | head -20
git status --porcelain
git ls-files data .env | head
```

Expected: 第一条只允许 `10.` 段与回环；`git status --porcelain` 为空；`data`、`.env` 一个文件都不在跟踪列表里。**内网拓扑进公开仓是不可撤回的**，这一步不许省。

- [ ] **Step 8: 报告 + 收尾**

把 Step 1–7 的读数写进 `.superpowers/sdd/2026-10-06-launchbay-plan3-icons-ui/acceptance.md`，含九张图的路径与每张的结论；发现的新问题按严重度进账本（`progress.md`），**不在验收步里顺手改代码**。

```bash
git status --porcelain   # 应当为空：验收产物都在 .superpowers/ 里
```

- [ ] **Step 9: 【故意破坏】整条通路一次**

把 `internal/render/web/profile.tmpl` 的 `{{template "icon" .}}` 临时换成固定的 `<span class="ic">?</span>`，重 build + 重拍一张 1440 暗档：**如果实拍还看得出"图标在"就是假验收**（应当整片变成首字母方块）。这一步的意义是让 Step 6 的实拍自己承认"它真的在看图"，而不是走过场。改回后复跑 Step 5 的计数断言。

---

## 附录 A：上游实测（2026-10-06，`tools/genicons -probe` 应当复现同一量级）

| 项 | tabler | simple-icons |
|---|---|---|
| 上游包 | `@iconify-json/tabler@1.2.41` | `@iconify-json/simple-icons@1.2.99` |
| 上游 `icons.json` | 2,134,102 B | 4,775,072 B |
| 实际条目 | 6268（声明 `total` 6220） | 3737（声明 3464） |
| 别名 | 194 | 12 |
| 许可 | MIT | CC0 1.0 |
| 元素词表 | `path:8975 g:2302 use:6 defs:3 circle:2 rect:2` | `path:3737` |
| 属性词表 | `d fill stroke stroke-linecap stroke-linejoin stroke-width id cx cy r x y width height rx ry opacity` | `fill d` |
| `fill` 取值 | `none` \| `currentColor` | `currentColor` |
| 产物 raw | 1.47MB | 4.48MB |
| 产物 gzip | **315.5KB** | **1.81MB** |

三条由此定案，不是偏好：

1. **数据源是 npm tarball，不是 Iconify 的 collection API。** 后者只回元数据加 4 个样本（实测 tabler 那条响应 107KB 且 `icons` 为空），拿不到全量 body。
2. **gzip 合计约 2.13MB < 3MB 红线**，所以不必走 spec §5 的后备阶梯；镜像预期从 10.4MB 走到约 12.5MB，仍在 §10 的 20MB 内。
3. **`use`/`defs` 必须整条跳过并报告**（3 个图标）：产物没有 id 引用体系，把它们当容器下钻会掏出一张谁都引用不到的孤图，那是"画一半"，比少三个图标坏得多。

条目数与上游 `total` 的差（6268 vs 6220、3737 vs 3464）如实留在产物的 `count` / `upstreamTotal` 两个字段里：产物自证只认 `count`，而"上游自己怎么数"不是我们能替它决定的事。

---

## 附录 B：交接给 P1-④（UI 与离线）

本计划把 §8.1 的卡片外观连同图标一起做了，所以 P1-④ 的清单缩成：

- Service Worker 离线与 §7.3 的 CI 性能预算闸（`X-LB-Profile` 已备好，SW 缓存名 `html-<profileId>` 那条防线还没人实现）
- 就地编辑（§8.4）与添加浮层（§8.5）——浮层直接吃本期已上线的 `resolve`
- `⌘/Ctrl K` 命令面板与键盘通路（§7.5 那五条键位，一个都没实现）
- 管理横幅的 DOM（§8.9，判据走 `Session.Managing()`，与渲染缓存键一起设计）
- `Profile.layout` 的侧栏/顶栏形态（§8.2）——本计划只落了 `Group.layout`
- 部署验收 #4：家庭可见的坏档案页里那句 Go 文案
- 对比度的**计算**与 `forced-colors`/`reduced-motion` 的全量实拍（本计划只做了卡片这一面）
- §8.3 的 `content-visibility`：本期 42 条不需要，条目上千再评估

`.superpowers/sdd/2026-10-05-launchbay-plan2-write-path/progress.md` 里的 deferred minors 仍然待接（那是 P1-② 留下的账本，不要在这里重做）。

---

## 附录 C：留给人的三个裁定（本期不自动决定）

1. `data/templates/*.json` 坏了答 **500**，而档案数据坏了答 **503**。本期不动它，但 `resolve` 不读模板，所以这条裂缝不会被本期放大。
2. `GET /api/v1/template` 是 `credNone`（局域网内可枚举服务清单）。本期给它作伴的 `GET /api/v1/icons` 同样公开，但后者只说"某个图标名存不存在"，不含服务清单——这两件事要不要一起收进 `credScoped`，等人点头。
3. 删档与改自己 PIN 不写审计行。`resolve` 同样不写（Task 8 的测试把这条钉住了）：三个都不产生 `rev`，都只在"动了别人的数据"时才该留痕。

---

## 接手执行前必须已完成的事实

不要凭本计划的记忆推断，逐条实测：

- `git log --oneline -1` 在 `feat/p1-3-icons` 上，且 spec 修订 `a1e2de3` 已在其中。
- `go test ./... -count=1` 全绿（P1-② 合并后的基线是 7 个包 `ok`）。
- 容器 `launchbay` 在 17600 上 `Up (healthy)`；夹具 PIN：dad 1357 / mom 2468 / kid 3579。真实 `data/` 与 `.env` 不进仓库。
- Task 0 的 `go run ./tools/genicons -probe` 能出网（npm registry 可达）。**不通就停下报告**：整个索引产物的来源只有这一条路，离线跑不出附录 A 的数字。
