# P1-④ 外壳 Implementation Plan

> **本文是历史实施记录，不是待办。** 它记录的是那一天计划或量的是什么，里面的
> 函数名、测试名与接口形状**可能已经变了**——2026-10-10 的管理员边界收紧一次性改判掉了
> 代管态、模板机制与徽标切档，共享清单也改判为不做。**不要照着这里的命令执行**：
> 权威契约在仓库根的 `AGENTS.md`，设计在 `docs/superpowers/specs/`。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 launchbay 装上零 JavaScript 的外壳——顶栏（`?q=` 搜索 + 主题切换）、侧栏与档案级 layout、管理横幅与 `/switch`、`/admin` 读侧、坏档页人话文案，以及 §7.3 的体积预算闸与仓库第一条 CI。

**Architecture:** 全部渲染仍在服务端（Go `html/template` + `//go:embed`），浏览器通路一律走 `<form method=post>`（表单发不出 JSON，所以不复用 `/api/v1/*`）。渲染内存缓存键从四维扩到七维（补 `home`、`homeFingerprint`、`bookmarksMtime` 之外的 `profileFingerprint`），带 `q` 的请求走"不缓存但照发 ETag"的旁路。权限判定继续只经 `require`，浏览器通道的权限拒绝在响应层面映射成 303 回选择页。

**Tech Stack:** Go 1.27（`CGO_ENABLED=0`，零第三方依赖）、`html/template`、`net/http` ServeMux 路由表、GitHub Actions（本计划新增，仓库此前无 CI）。

**Spec:** `docs/superpowers/specs/2026-10-06-launchbay-p1-4-shell-design.md`（阶段设计）+ `docs/superpowers/specs/2026-10-05-launchbay-design.md`（权威源，§6.1/§7.1/§7.3/§8.x/§12/§13/§14/§15 已含 P1-④ 的 11 处修订）。**执行者两个都要读**：设计文件说"为什么这么切"，权威源说"规则是什么"。

## 修订（验收之后，2026-10-07）

Task 1 与 Task 3 里写的缓存键维度 **`profileMtime`（`Store.ProfileMtime`）已换成
`profileFingerprint`**：`Store.LoadProfileWithFingerprint` 在同一次读盘里给出解码结果与那份
字节的 sha256，`Store.ProfileMtime` 与单独的 `ProfileFingerprint` 都已删除（整枝评审 I-2：
键里的 `home` 同样要带它那份 `profile.json` 的指纹，因为横幅印的是 home 的显示名）。
原因见 `docs/superpowers/acceptance/p1-4-shell/2026-10-07-验收.md` 第四节：Linux 的
tmpfs/overlayfs 在同一时钟刻内两次原子替换会给出纳秒级相同的 mtime，CIFS/9p 常只到秒，
于是"改了主题、页面不动"在真部署上会发生；同一份测试在 Windows 门禁绿、在镜像构建的
Linux 门禁红。下面各段保留**当时的形状**（那是那九个任务的施工记录），只在这一处说明差异；
新代码以 `internal/render/render.go#cacheKey` 与两份 spec 的 §7.1 为准。

## Global Constraints

每个任务的隐含要求，逐字来自 spec 与既有代码约定：

- **Go 1.27**，`CGO_ENABLED=0`；`go.mod` 里**不许出现 `require`**（零第三方依赖，无前端工具链——spec §10 明写构建期零 node）。
- **本阶段零 JavaScript**：`GET /` 的 HTML 里 `<script` 计数必须为 0（Task 8 把它变成断言）。任何"用 JS 才做得动"的需求都属于 P1-⑤，不要在本期偷偷实现。
- **两条通路两种"拒"**（spec §6.1）：`/api/v1/*` 的拒绝仍是 401/403 且永远分开；浏览器通路（`/pin`、`/prefs`、`/switch`、`/admin`）的**权限类**拒绝 = **303 回档案选择页**。**PIN 输错不是权限拒绝**，它继续回选择页并带 `?err=` 提示。
- **渲染内存缓存键 = `(profileId, home, homeFingerprint, rev, bookmarksMtime, profileFingerprint, iconGen)`**（spec §7.1；`profileMtime` 已被内容指纹取代，见上面的修订节）；带 `?q=` 的请求不进该缓存，但照样发 ETag。
- **`routes()` 是唯一注册来源**。新增路由必须同时出现在 `internal/httpapi/openapi.json` 与 `AGENTS.md` 端点表——`TestOpenAPICoversEveryRoute`、`TestOpenAPIHasNoGhostEndpoints`、`TestOpenAPIMethodSetsMatchPerPath`、`TestOpenAPIEndpointServesTheEmbeddedDoc`、`TestAgentsMDMatchesRoutes` 五条闸遍历的是**全部**路由，不只 `/api/v1/*`。豁免名单只有 `/`、`/pin`、`/healthz` 三条，本期不扩大。
- **JSON 写出契约（spec §4.3）**：缩进 2 空格、末尾恰好一个换行、`SetEscapeHTML(false)`、非 ASCII 原样、空数组写 `[]` 绝不写 `null`、键序由 struct 字段声明序固定、绝不靠 map 遍历产生输出；响应体一律 struct，不许 `map[string]any`（测试解码用 map 可以）。错误外壳只有一个：`errorResponse`。
- **§11 明写不做**：不签 cookie、不做速率限制/退避/锁定/失败计数、不做 TLS 终结、**不做"关闭私网"开关**、不给 `/icons/*` 加鉴权、**不加 CSRF 机制**。多出来的防护也算 defect。
- **真实 NAS 网段与地址一个字面量都不许进仓库**；文档、注释、夹具只用 `10.` 段与 `127.0.0.1`；`.env`、`data/`、`.superpowers/` 都不进仓库。Task 8 把这条变成可跑的守卫。
- **不许直接改 `data/` 下任何文件**；书签写入只有 `POST /api/v1/batch` 一条通路。`profile.json` 的写入只有 PIN 与本期 `/prefs`，且都必须 `WithProfileLock`。
- **判据只有一份**：主题/形态值集的权威在 `internal/model`（`allowedTheme`/`allowedLayout`），渲染侧的颜色判据是 `validatedAccent`，图标名判据是索引。任何一处出现第二份字面枚举，评审就会按缺陷处理。
- **TDD**：每个功能步先写失败测试并**看到它红**，再写实现；每条不变量测试都要做一次**【故意破坏】**，证明它会红，然后改回。
- **每任务收尾必跑的门禁**（顺序固定）：`go vet ./...`、`gofmt -l .`（输出必须为空）、`go test ./... -count=1`、`CGO_ENABLED=0 go build ./...`。
- **提交规矩**：按功能域一次提交，message 写实质不写过程词，禁署名；**push 先问人**（本仓库当前无 remote）。
- 中文注释只写"为什么"；不写行号引用，引用代码用 `file#符号` 形式。
- **Windows + Git Bash**：带中文或反引号的脚本先落成文件再跑（内联会被剥引号）；`python` 读文件要显式 `encoding="utf-8"`；Bash 回显会整行吞掉含中文的输出——看中文测试名请重定向到文件再读，或只打计数。

## 文件结构

新增（`Create`）：

```
internal/render/filter.go            ?q= 的命中判据与过滤（唯一一份"什么算命中"）
internal/render/filter_test.go       过滤的表驱动证人
internal/httpapi/prefs.go            POST /prefs（表单通路，写 profile.json）
internal/httpapi/prefs_test.go       合法/非法/缺字段/令牌与无会话不写盘/坏档 503/改完立刻可见
internal/httpapi/switch.go           POST /switch（三态：home / picker / <id>）
internal/httpapi/switch_test.go      三态各一条 + 伪造被拒 + 相邻那条 PIN 输错仍带 err
internal/httpapi/adminpage.go        GET /admin（读侧页面，复用 status 聚合）
internal/httpapi/adminpage_test.go   非管理员 303、管理员 200 且不含条目内容、坏档行无死按钮
internal/render/web/admin.tmpl       管理页模板
internal/render/budget_test.go       §7.3 体积预算闸 + `<script` 计数 0
tools/leakguard/main.go              IP 字面量守卫（CI 用；package main，不进镜像）
tools/leakguard/main_test.go         守卫自身的证人（含"解析器没吞形状"与"跳过要明说"）
.github/workflows/ci.yml             仓库第一条 CI：四道门禁 + 预算测试 + 泄漏守卫
docs/superpowers/acceptance/2026-10-06-launchbay-p1-4-shell.md  Task 9 的机器读数
```

修改（`Modify`）：

```
internal/render/render.go            缓存键七维（含 home 与 profile 两份内容指纹）；ProfilePage 收 PageRequest；新增 AdminRow/AdminPage
internal/render/exec.go              profileView 补 Query/Hits/Total/Manage；新增 adminView 与 executor.admin
internal/render/web/profile.tmpl     顶栏（搜索框 + 主题表单 + 档案徽标/管理入口）、横幅、侧栏、两句空态
internal/render/web/broken.tmpl      人话主句 + <details> 里的原文
internal/render/web/base.css         顶栏栅格/侧栏/抽屉/横幅/管理页样式 + §7.5 硬数字续用
internal/render/render_test.go       缓存键三维的新证人 + 既有断言按 PageRequest 形状更新
internal/render/views_test.go        顶栏/横幅/侧栏/搜索/坏档页的形状钉；TestCSS_硬规格在场 补三条；
                                     TestAdminEntryOnlyForAdmin 那条死链钉翻面（Task 5）
internal/model/check.go              导出 ValidTheme / ValidLayout（判据只有一份）
internal/model/model_test.go         两个新谓词的证人
internal/httpapi/handlers.go         handleRoot 传 Home/Query；新增 redirectPicker 与 homeOf；writePage 不动
internal/httpapi/handlers_test.go     坏档原文与 503 detail 同源（Task 7）、q 不回声（Task 2）
internal/httpapi/routes.go           三条新路由 + Summary
internal/httpapi/admin.go            statusRows 抽成可复用（页面与 JSON 同一份聚合）；statusProfile 补 Modified
internal/httpapi/openapi.json        三条新路由的文档 + /api/v1/status 的 modified 键
internal/httpapi/openapi_test.go     **不改**：xCredentialNames 已有 credSession→"session"，
                                     planned 豁免表保持为空（三条新路由都是真注册）
AGENTS.md                            端点表三行 + 浏览器通路一段
.dockerignore                        加 .github（tools/ 已在列，同判据）
```

`internal/render/web/picker.tmpl` **不改**：进门页本来就有 `data-theme` 的缝（`themeAttr` 给它的是空串，由媒体查询接手），而本期没有新的进门动作。

**任务顺序有依赖**：Task 1（缓存键）是 Task 3（主题）与 Task 4（横幅）的地基——键先长，页面才有地方挂新维度；Task 2（搜索）依赖 Task 1 的"不缓存旁路"；Task 5（`/admin`）依赖 Task 4 的免 PIN 切档与 Task 3 的 `redirectPicker`；Task 6（侧栏）依赖 Task 2 的命中计数；**Task 8 依赖 Task 5**（那条"四种页面都零脚本"要渲染管理页）；Task 7 独立；Task 9 收尾，把前面八条在真浏览器里再判一次。

### 设计 §12 十二条的落点（自查表）

评审时按这张表找证人，别按任务号猜。设计文件是 `docs/superpowers/specs/2026-10-06-launchbay-p1-4-shell-design.md`。

| 设计 §12 的第几条 | 原文（摘要） | 落点 | 证人 |
|---|---|---|---|
| 1 | 缓存键三维：`home`（含其内容指纹）变则 ETag 变、`profileFingerprint` 变则 ETag 变、`q` 不写缓存而同 `q` 两次 ETag 相同 | Task 1 + Task 4 | `…缓存键认得home这一维`（CacheSize 观察条目数）、`…带q不进缓存但照发ETag`、`…横幅只在代管态出现…`（同一档案两种 home 的 ETag 必须不同——这条才是"home 变则 ETag 变"在真内容上的兑现） |
| 2 | 横幅三条 DOM 断言 | Task 4 | 本人无横幅、代管有两个名字、`href="/switch"` 永不出现在页面上 |
| 3 | `/switch` 三态各一条 + 非管理员伪造 `to=` 被拒 | Task 4 | 三态各一条 + `…目标档案读不了不切` + `…令牌走不通` |
| 4 | `/prefs`：合法落盘、非法 422 点名、都缺 422、令牌拒、坏档 503、改完立刻可见 | Task 3 | `…主题改得动且立刻看得见`、`…非法与缺字段都422且点名`、`…令牌与无会话都不写盘`、`…坏profile答503` |
| 5 | `/admin`：非管理员 303、管理员 200 且没有条目内容 | Task 5 | `TestAdmin_…` 四条（含 no-store、坏档行无死按钮、与 `/status` 同一份聚合） |
| 6 | PIN 输错仍带 `?err=`，不是裸 303 | Task 4 | `…PIN输错仍带err提示`（相邻通路的回归项） |
| 7 | 三条新路由过既有契约闸，不放宽豁免 | Task 3/4/5 | 契约闸的条数由测试自己说了算（openapi 四条 + AGENTS.md 一条），豁免名单仍是 `/`、`/pin`、`/healthz` |
| 8 | 搜索：命中数、两句不同的话、侧栏计数随命中、无会话时 `q` 被忽略且不进 PIN 表单 | Task 2 + Task 6 | `…搜索无结果与没书签是两句话`、`…侧栏锚点与计数`、`TestRoot_无会话时q被忽略且不回声` |
| 9 | 坏档页：主句是人话、`<details>` 里有文件名与行号、store 原文未被改写 | Task 7 | `TestBrokenPage_主句是人话原文在details里`、`Test坏档原文页面与503同源` |
| 10 | 预算闸 + `<script` 计数 0 | Task 8 | 两条线（契约天花板 + 操作线）× 两档规模、CSS 上限、四种页面 `<script` 计数 0 |
| 11 | CI 在干净克隆上跑通（含泄漏守卫），且 CI 的输入文件都在仓库里 | Task 8 Step 6 | 本地 `git clone` 彩排四道门禁 + 守卫（真 CI 需要人批准 push） |
| 12 | 端到端：换主题 → 刷新 → 实拍两张证明不是"写了个字段没人读" | Task 9 | 入库的机器读数与实拍清单（不是"符合预期"） |

**本期不做、也不假装做的**（设计 §11 的清单在验收判定里同样是结论）：带指纹的静态资产通路、⌘K 面板、添加浮层、就地编辑、`/admin` 的写动作、Service Worker 与离线队列、`content-visibility` 与 §7.4 的分片、gzip 中间件、CSRF。LCP / 帧率 / SW 命中三条**留人工**，Task 9 Step 7 写清了怎么测。

**写计划时改掉的两句设计原文**（设计文件已同步改正，这里留一句为什么）：

1. 设计 §12 第 1 条原话是"`home` 变则 ETag 变"。④ 的 Task 1 落地时**还没有横幅**，同一份书签
   换 home 渲染出的字节完全相同，那条断言当时只能靠 `CacheSize()` 的条目数来证；"ETag 真的
   不同"要等 Task 4 把横幅送进页面之后才成立——所以拆成两处，各测各能测的那一半。
2. 设计 §12 第 4 条原话是"`/prefs` 令牌 → 401/403"。这与设计 §2 自己那条"浏览器通路的权限拒
   = 303 回选择页"打架（`/prefs` 就在 §6.1 点名的浏览器通路名单里）。**按 §2 走**：拒的长相是
   303，而真正的判据是"盘上一个字节都没变"——状态码骗人，磁盘不骗人。

---

## Task 1: 渲染缓存键补 `home` 与 `profileFingerprint`（当时写作 `profileMtime`，见修订节）

**Files:**
- Modify: `internal/render/render.go`（`cacheKey`、`ProfilePage`、新增 `PageRequest`）
- Modify: `internal/httpapi/handlers.go`（`handleRoot` 的 `ProfilePage` 调用点）
- Modify: `internal/render/render_test.go`、`internal/httpapi/*_test.go` 里所有 `ProfilePage(` 调用点
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: `Store.Current(id)`（给 `bookmarksMtime`）、`Store.LoadProfile(id)`（本期改为顺手 stat `profile.json`）、`IconCache.Generation()`
- Produces:

```go
type PageRequest struct {
	ID    string // 渲染哪个档案（权限已由调用方判过）
	Home  string // 会话的 lb_home；非管理态时等于 ID。缓存键的一维
	Query string // ?q=，空 = 不过滤
}

func (r *Renderer) ProfilePage(req PageRequest) (Page, error)
```

`PickerPage` 与 `BrokenPage` 签名不变。

- [ ] **Step 1: 先写"键少了这一维就红"的失败测试**

```go
// internal/render/render_test.go 追加
//
// 这两条测的是缓存键的维度，不是页面内容：本期还没有横幅、主题按钮也还没上，
// 所以"home 变了内容就该变"在这里测不了。能测的是**同一份数据被当成两个不同
// 页面缓存**这件事——用 CacheSize 观察条目数，比观察 ETag 直接，也不依赖后面
// 几期的渲染细节。
func TestProfilePage_缓存键认得home这一维(t *testing.T) {
	s := seeded(t)
	r := renderer(t, s)
	// 同一个档案，两次请求的 home 不同（本人 / 被人代管）。
	if _, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ProfilePage(PageRequest{ID: "dad", Home: "mom"}); err != nil {
		t.Fatal(err)
	}
	if n := r.CacheSize(); n != 2 {
		t.Errorf("home 变了而缓存只存了 %d 份：这一维没进缓存键，"+
			"以后横幅上线就会把带横幅的页面缓存给本人看（spec §7.1 的串号）", n)
	}
	// 完全相同的请求重复两次必须命中，否则这一维是白加的。
	if _, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad"}); err != nil {
		t.Fatal(err)
	}
	if n := r.CacheSize(); n != 2 {
		t.Errorf("同 (id,home) 第二次又存了一份（%d），缓存旁路写错了", n)
	}
}

// 主题存在 profile.json 里，而现有 mtime 只 stat bookmarks.json——
// 改了 profile.json 而页面纹丝不动，是 spec §7.1 点名的"按了没反应"。
// 这一维现在就测得动：data-theme 从 P1-③ 起就在渲染。
func TestProfilePage_改profile会重算(t *testing.T) {
	s := seeded(t)
	r := renderer(t, s)
	first, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(first.Body, []byte(`data-theme="light"`)) {
		t.Fatal("夹具一开始就是 light，换成 light 就测不出变化了")
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	p.Theme = "light"
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	second, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(second.Body, []byte(`data-theme="light"`)) {
		t.Errorf("改了 profile.json 而页面还是旧的：缓存键里没有 profile.json 的指纹")
	}
}

// 带 q 的请求走"不进缓存、照样发 ETag"的旁路（spec §7.1）。这一版测得动的是三件：
// 写进缓存（键空间随击键膨胀）、旁路时忘了 ETag（浏览器 304 那一路白损失）、
// 同一 q 两次渲染的 ETag 稳定。**"不同 q 绝不互相命中"这一条在本任务测不了**：
// Task 1 只把 query 接进执行器、还不做过滤，两个不同的 q 渲染出的字节完全相同，
// 而 ETag 是内容哈希（§7.1 说的就是 (档案, rev, 内容哈希)）——断言它们不同等于
// 断言一个还没实现的功能。那条证人在 Task 2 落地过滤之后补（见 Task 2 Step 5 的
// TestProfilePage_不同q不互相命中）。这里留一句指针，防止它被当成"已覆盖"漏掉。
func TestProfilePage_带q不进缓存但照发ETag(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	save(t, s, "mom", "妈妈的台", "emby")
	r := renderer(t, s)

	a1, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad", Query: "dsm"})
	if err != nil {
		t.Fatal(err)
	}
	if n := r.CacheSize(); n != 0 {
		t.Errorf("带 q 的渲染进了内存缓存（%d 条）：键空间会随击键膨胀", n)
	}
	if a1.ETag == "" {
		t.Error("旁路不等于不发 ETag：浏览器 304 那一路照样要走")
	}
	a2, _ := r.ProfilePage(PageRequest{ID: "dad", Home: "dad", Query: "dsm"})
	if a2.ETag != a1.ETag {
		t.Errorf("同一个 q 两次渲染 ETag 不同（%q vs %q）：304 永远命不中", a1.ETag, a2.ETag)
	}
	b, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad", Query: "emby"})
	if err != nil {
		t.Fatal(err)
	}
	// "emby" 在 dad 的台里一个都不命中，它**同样不许进缓存**——旁路是每条带 q 的
	// 请求都走，不是只有第一条。而"不同 q 不许互相命中（拿到的 ETag 必须不同）"
	// 现在测不了：本任务渲染器接住 query 但还没过滤，两个 q 长出的字节相同，内容哈希
	// 自然相同。spec §7.1 把 ETag 钉成 (profileId, rev, 内容哈希)，不许为了让这一行
	// 现在就绿而把查询词折进哈希——那是假证人，会把"两个 URL 同一份表示"这种
	// 合法情形也判死。这一维由 Task 2 的 TestProfilePage_不同q不互相命中 接管。
	if b.ETag == "" {
		t.Error("第二条带 q 的请求没发 ETag：旁路不等于不发")
	}
	if n := r.CacheSize(); n != 0 {
		t.Errorf("第二条带 q 的请求也占进了缓存（%d 条）：旁路只对第一条生效", n)
	}
	// 不带 q 的那一条照旧进缓存，且与带 q 的互不干扰。
	plain, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if n := r.CacheSize(); n != 1 {
		t.Errorf("不带 q 的渲染该占一条缓存，got %d", n)
	}
	// 不带 q 的照旧端出全量页面：旁路只关带 q 的请求，不许把缓存写成"谁都拿空页"。
	if plain.ETag == "" || !bytes.Contains(plain.Body, []byte("dsm 台")) {
		t.Errorf("全量页缺 ETag 或没端出条目（etag=%q）", plain.ETag)
	}
}
```

`render_test.go` 的 import 需要 `bytes`（若尚未引入）。

- [ ] **Step 2: 跑它，确认它红**

Run: `go test ./internal/render -count=1 -run 'ProfilePage_缓存键认得home|ProfilePage_改profile会重算|ProfilePage_带q不进缓存'`
Expected: 编译失败——`PageRequest` 未定义、`ProfilePage` 参数不匹配。**这就是本期合法的 RED**（先改签名再补实现）。

- [ ] **Step 3: 改缓存键与签名**

`internal/render/render.go`：

```go
type cacheKey struct {
	profile  string
	home     string // 会话从哪进来：管理横幅取决于它（spec §7.1）
	rev      int64
	modified time.Time // bookmarks.json：外部改盘也失效
	profMod  time.Time // profile.json：主题与形态存在那里，缺这维改了不重算
	iconGen  uint64    // 图标缓存的代次
}

// PageRequest 是渲染一页所需的全部输入。用 struct 而不是三个 string 参数：
// ProfilePage(id, home, query) 里三个同类型参数被调用方写反是编译器抓不到的，
// 而写反的后果（把别人的缓存条目当成自己的）正是本设计最贵的那类 bug。
type PageRequest struct {
	ID    string
	Home  string
	Query string
}
```

`ProfilePage` 改成：

```go
func (r *Renderer) ProfilePage(req PageRequest) (Page, error) {
	snap, err := r.s.Current(req.ID)
	if err != nil {
		return Page{}, err
	}
	prof, err := r.s.LoadProfile(req.ID)
	if err != nil {
		return Page{}, err
	}
	profMod, err := r.s.ProfileMtime(req.ID)
	if err != nil {
		return Page{}, err
	}
	// Query 非空时整个缓存旁路：键空间会从"每档案一份"变成"每档案每次击键一份"，
	// 为一个局域网页面养无界缓存不值（spec §7.1）。旁路不等于不发 ETag——
	// 下面算的就是内容哈希，浏览器 304 那一路一分不损失。
	key := cacheKey{profile: req.ID, home: req.Home, rev: snap.Rev,
		modified: snap.Modified, profMod: profMod, iconGen: r.fc.Generation()}
	if req.Query == "" {
		r.mu.Lock()
		if p, ok := r.cache[key]; ok {
			r.mu.Unlock()
			return p, nil
		}
		r.mu.Unlock()
	}

	body, err := r.tmpl.profile(prof, snap.B, req.Query)
	if err != nil {
		return Page{}, err
	}
	page := Page{Body: body, ETag: makeETag(req.ID, body), Profile: req.ID, Rev: snap.Rev}

	if req.Query == "" {
		r.mu.Lock()
		if len(r.cache) > 32 {
			r.cache = map[cacheKey]Page{}
		}
		r.cache[key] = page
		r.mu.Unlock()
	}
	return page, nil
}
```

`internal/store/paths.go` 或 `load.go` 加（放 `load.go` 里，紧挨 `Current`）：

```go
// ProfileMtime 是 profile.json 的指纹：主题与档案形态存在那个文件里，
// 渲染缓存键需要它，否则改了不重算（spec §7.1）。
func (s *Store) ProfileMtime(id string) (time.Time, error) {
	fi, err := os.Stat(s.ProfilePath(id))
	if err != nil {
		return time.Time{}, fmt.Errorf("stat %s: %w", filepath.Base(s.ProfilePath(id)), err)
	}
	return fi.ModTime(), nil
}
```

> 注意错误文本用 `filepath.Base`：这条消息可能被家属看到的解释页复用（与 `decodeProfile` 同口径）。

- [ ] **Step 4: 改所有调用点**

`internal/httpapi/handlers.go#handleRoot`：

```go
	page, err := s.deps.Render.ProfilePage(render.PageRequest{ID: sess.Active, Home: sess.Home})
```

`internal/render/render_test.go`、`views_test.go`、`internal/httpapi/*_test.go` 里每一处 `ProfilePage("dad")` 改成 `ProfilePage(PageRequest{ID: "dad"})`（跨包处写 `render.PageRequest{...}`）。用
`grep -rn "ProfilePage(" --include=*.go .` 列全，一处都不许漏——漏了是编译错，不会静默。
其中 `views_test.go#mustRender(t, s, id)` 这个 helper 的**签名保持不变**（它自己包里被十几条测试用着），只改它函数体里那一行调用：签名跟着改成 `PageRequest` 会把十几条与本期无关的测试搅进来。

`internal/render/exec.go#profile` 收第三个参数（本期先接住不用，Task 2 才过滤）：

```go
func (e *executor) profile(p *model.Profile, b *model.Bookmarks, query string) ([]byte, error) {
```

**两处注释里写着旧的四维键，必须跟着改**（它们是下一个人判断"这一维要不要加"的依据，
写在文件里就是判据的一部分）：`internal/render/render.go` 的 `Page.Body` 那句
"同一 (档案,rev,mtime,iconGen)"、`internal/render/views_test.go` 的
`TestProfilePage_theme属性只表显式选择` 上面那句"页面缓存键是 (档案,rev,mtime,iconGen)"。
后者还说"每个 theme 一个新渲染器"——`profileMtime` 进键之后这句话不再是要钉的理由，
把它改成"新渲染器只是夹具习惯，键本身已经认 profile.json"（夹具保持原样，别动断言）。

- [ ] **Step 5: 跑测试确认它绿**

Run: `go test ./internal/render/ ./internal/httpapi/ -count=1`
Expected: 全绿。新增的是三条：`…缓存键认得home这一维`、`…改profile会重算`、`…带q不进缓存但照发ETag`。

- [ ] **Step 6: 四次【故意破坏】**

1. 把 `key` 里的 `home: req.Home` 删掉（字段留着不填）→ `TestProfilePage_缓存键认得home这一维` 红（条目数 1 而非 2）。**编译器抓不到这一维丢失**，所以这条测试是它唯一的证人。
2. 把 `profMod` 从键里去掉 → `TestProfilePage_改profile会重算` 红。
3. 把 `if req.Query == ""` 那两处旁路判断删掉（让带 q 的也写缓存）→ `TestProfilePage_带q不进缓存但照发ETag` 第一条红（CacheSize 变 1）。
4. 把带 q 的分支改成"不发 ETag"（`page.ETag` 留空）→ 同一条测试的第二断言红。
5. 每次 `git checkout -- internal/render/render.go` 还原、复跑绿。

- [ ] **Step 7: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/render internal/httpapi internal/store
git commit -m "feat(render): 缓存键补 home 与 profile.json 指纹两维，带 q 的请求走不缓存旁路"
```

---

## Task 2: `?q=` 服务端搜索

**Files:**
- Create: `internal/render/filter.go`、`internal/render/filter_test.go`
- Modify: `internal/render/exec.go`（视图带 Query/Hits/Total）、`internal/render/web/profile.tmpl`（搜索框 + 两句空态）、`internal/render/web/base.css`（顶栏样式）
- Modify: `internal/httpapi/handlers.go`（把 `?q=` 传进 `PageRequest`）
- Test: `internal/render/filter_test.go`、`internal/render/views_test.go`

**Interfaces:**
- Consumes: `PageRequest.Query`（Task 1）、`model.Bookmarks`/`model.Item`
- Produces:

```go
// filter.go
func matches(q string, b *model.Bookmarks) (*model.Bookmarks, int, int) // 过滤后、命中数、总条数
func matchItem(needle string, it model.Item) bool                       // needle 必须已经折叠过小写
func containsFold(needle, hay string) bool
```

`profileView` 补 `Query string`、`Hits int`、`Total int`（`Total` = 该档案全部条目数，与过滤无关）。

- [ ] **Step 1: 写过滤的失败测试**

```go
// internal/render/filter_test.go
package render

import (
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func book() *model.Bookmarks {
	return &model.Bookmarks{Title: "T", Groups: []model.Group{
		{ID: "nas", Title: "NAS 与内网", Items: []model.Item{
			{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{"nas"}, Tags: []string{"存储管理"}},
			{ID: "qb", Name: "qBittorrent", URL: "http://10.0.0.3:8181", Alias: []string{"bt"}, Tags: []string{}},
		}},
		{ID: "read", Title: "读与收藏", Items: []model.Item{
			{ID: "weread", Name: "微信读书", URL: "https://weread.qq.com", Desc: "看书用这个", Alias: []string{}, Tags: []string{"书"}},
		}},
	}}
}

func TestMatches_六个字段都算命中(t *testing.T) {
	for _, tc := range []struct {
		name, q string
		want    []string
	}{
		{"名字", "群晖", []string{"nas/dsm"}},
		{"别名", "bt", []string{"nas/qb"}},
		{"地址", "8181", []string{"nas/qb"}},
		{"标签", "存储", []string{"nas/dsm"}},
		{"描述", "看书", []string{"read/weread"}},
		// 分组标题命中 → 该组全部条目都算（"搜 NAS 把内网那组捞出来"是真实用法）。
		{"分组标题", "读与", []string{"read/weread"}},
		{"大小写不敏感", "QB", []string{"nas/qb"}},
		{"无命中", "不存在的词", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, hits, total := matches(tc.q, book())
			var got []string
			fb, _, _ := matches(tc.q, book())
			for _, g := range fb.Groups {
				for _, it := range g.Items {
					got = append(got, g.ID+"/"+it.ID)
				}
			}
			if total != 3 {
				t.Errorf("总数 = %d，要 3（总数与过滤无关）", total)
			}
			if hits != len(tc.want) {
				t.Fatalf("命中 %d 条 %v，要 %d 条 %v", hits, got, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if got[i] != w {
					t.Errorf("第 %d 条 = %q，要 %q", i, got[i], w)
				}
			}
		})
	}
}

// 空 query 与"没命中"必须是两个不同答案：模板要靠它说两句不同的话。
func TestMatches_空query与无命中是两个状态(t *testing.T) {
	if fb, hits, total := matches("", book()); hits != total || len(fb.Groups) != 2 {
		t.Errorf("空 query 该原样全给：hits=%d total=%d groups=%d", hits, total, len(fb.Groups))
	}
	if _, hits, _ := matches("zzz", book()); hits != 0 {
		t.Errorf("无命中 hits=%d，要 0", hits)
	}
}

// 过滤不许改到调用方传进来的那份数据：它是 store 缓存里的书签对象。
// 顺手也钉住"非 Groups 字段一个都不丢"——逐字段抄的实现会在这里红。
func TestMatches_不改入参且不吃掉别的字段(t *testing.T) {
	b := book()
	b.Clock = true
	b.OpenInNewTab = true
	b.Rev = 7
	fb, _, _ := matches("群晖", b)
	if len(b.Groups[0].Items) != 2 {
		t.Errorf("入参被改了：nas 组剩 %d 条", len(b.Groups[0].Items))
	}
	if len(fb.Groups[0].Items) != 1 {
		t.Errorf("结果条数不对：%d", len(fb.Groups[0].Items))
	}
	if fb.Clock != b.Clock || fb.OpenInNewTab != b.OpenInNewTab || fb.Title != b.Title {
		t.Errorf("过滤把非 Groups 的字段吃了：clock=%v newtab=%v title=%q",
			fb.Clock, fb.OpenInNewTab, fb.Title)
	}
}
```

- [ ] **Step 2: 跑它，确认它红**

Run: `go test ./internal/render -count=1 -run Matches`
Expected: 编译失败（`matches` 未定义）。

- [ ] **Step 3: 写 filter.go**

```go
package render

import (
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

// matches 按 q 过滤一份书签，返回**新的** Bookmarks、命中数与总条数。
//
// 判据只有这一份（spec §8.1）：条目看 name/alias/url/tags/desc，分组标题命中
// 就把整组捞出来。大小写折叠一次、CJK 按子串（不分词——局域网书签量级不需要
// 分词，而分词会带来一套没人维护的词典）。
//
// 不原地改入参：调用方给的是 store 里那份书签对象，改它就是改磁盘真相的内存副本。
func matches(q string, b *model.Bookmarks) (*model.Bookmarks, int, int) {
	needle := strings.ToLower(strings.TrimSpace(q))
	// 整体复制再换掉 Groups：逐字段抄一份的话，model.Bookmarks 以后加的字段
	// （Clock 就是这么差点被吃掉）会在搜索结果里静默消失——那不是过滤，是丢功能。
	out := *b
	out.Groups = nil
	total, hits := 0, 0
	for _, g := range b.Groups {
		total += len(g.Items)
		if needle == "" {
			out.Groups = append(out.Groups, g)
			hits += len(g.Items)
			continue
		}
		groupHit := containsFold(needle, g.Title)
		ng := g
		ng.Items = nil
		for _, it := range g.Items {
			if groupHit || matchItem(needle, it) {
				ng.Items = append(ng.Items, it)
				hits++
			}
		}
		if len(ng.Items) > 0 {
			out.Groups = append(out.Groups, ng)
		}
	}
	if needle == "" {
		return &out, total, total
	}
	return &out, hits, total
}

func matchItem(needle string, it model.Item) bool {
	if containsFold(needle, it.Name) || containsFold(needle, it.URL) ||
		containsFold(needle, it.Desc) {
		return true
	}
	for _, s := range it.Alias {
		if containsFold(needle, s) {
			return true
		}
	}
	for _, s := range it.Tags {
		if containsFold(needle, s) {
			return true
		}
	}
	return false
}

// containsFold 是"needle（已折叠过）出现在 hay 里"。参数顺序刻意与
// iconCandidates 那套分开：这里 hay 是被搜的字段，needle 是查询词。
func containsFold(needle, hay string) bool {
	return needle != "" && strings.Contains(strings.ToLower(hay), needle)
}
```

> `internal/render/icon.go` 里若已有同名 `containsFold`，把本文件的这个改名 `haystackHas`，**不要**合并两个判据——一个是渲染兜底用的，一个是搜索用的，合并迟早分家。先 `grep -n "func containsFold" internal/render/` 确认。

- [ ] **Step 4: 接进视图与模板**

`internal/render/exec.go`：`profileView` 补三字段，`profile()` 用过滤后的数据：

```go
type profileView struct {
	Profile *model.Profile
	Theme   string
	// Query 非空 = 这一页是搜索结果。模板靠它决定说"没有匹配"还是"还没有书签"，
	// 也靠它决定要不要在顶栏搜索框里回填。
	Query   string
	Hits    int
	Total   int
	Bookmarks profileData
	CSS     template.CSS
}
```

```go
func (e *executor) profile(p *model.Profile, b *model.Bookmarks, query string) ([]byte, error) {
	fb, hits, total := matches(query, b)
	v := profileView{Profile: p, Theme: themeAttr(p.Theme), CSS: e.cssT,
		Query: strings.TrimSpace(query), Hits: hits, Total: total,
		Bookmarks: profileData{Title: b.Title, Footer: b.Footer, OpenInNewTab: b.OpenInNewTab}}
	for _, g := range fb.Groups { // 注意：遍历过滤后的 fb
```

`profile.tmpl` 的 `<header>` 换成（`{{define}}` 结构不变）：

```gotemplate
<header>
  <h1>{{.Bookmarks.Title}}</h1>
  <form class="search" method="get" action="/" role="search">
    <input type="search" name="q" value="{{.Query}}" aria-label="搜索书签" placeholder="搜名字、地址、标签">
    <button type="submit">搜</button>
  </form>
  {{if .Query}}<p class="hits">命中 {{.Hits}} 条 · 共 {{.Total}} 条</p>{{end}}
  <nav aria-label="档案">
    <span class="badge">{{.Profile.Title}}</span>
    {{if .Profile.Admin}}<span class="badge">管理</span>{{end}}
  </nav>
</header>
```

> **就地改，不要新加一条 `header{}`**：`base.css` 现在有一行
> `header{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:18px}`，
> 把它的 `display:flex` 换成栅格（保留 `margin-bottom:18px`）。同名选择器写两条
> 靠后者覆盖靠前者，读的人只看得到其中一条，而下一个人删"多余"那条时会删错。

`main` 开头那段空态改成两句：

```gotemplate
{{if .Query}}
  {{if not .Bookmarks.Groups}}<p>没有匹配「{{.Query}}」的书签。<a href="/">看全部</a></p>{{end}}
{{else if not .Bookmarks.Groups}}
  <p>这个档案还没有书签。</p>
{{end}}
```

`internal/httpapi/handlers.go#handleRoot` 传 `Query`：

```go
	page, err := s.deps.Render.ProfilePage(render.PageRequest{
		ID: sess.Active, Home: sess.Home, Query: r.URL.Query().Get("q")})
```

`base.css`：把既有 `header{…}` 那一行改成栅格，并在其后追加三条（放在 `header` 那一段里，别打散既有分节）：

```css
header{display:grid;grid-template-columns:1fr auto auto;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:18px}
header h1{grid-column:1;margin:0;font-size:19px}
/* 顶栏搜索：零 JS 的 GET 表单。input 的 min-height 已在 §7.5 那条里给过 44px，
   这里只补布局；.hits 是搜索态的读数，不搜索时整行不渲染。 */
.search{display:flex;gap:6px;align-items:center}
.search input{min-width:180px}
.hits{grid-column:1/-1;margin:0;font-size:13px;color:var(--ink2)}
```

（原来那行 `header h1{font-size:19px;margin:0}` 也并进来，别留两条 `header h1`。）

> 若既有 `header{display:flex;…}` 与上面冲突，改那一行为准（保留一处声明），不要两条并存在靠后者覆盖靠前者。

- [ ] **Step 5: 补两条形状钉（两句空态 + q 不许回声）**

`internal/render/views_test.go` 追加：

```go
// 两句空态：有书签的档案搜不到东西，不能说"还没有书签"。
// 夹具必须真的有书签——seeded() 建的档案是空的，空档案里两句都会绿。
func TestProfilePage_搜索无结果与没书签是两句话(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm") // dad 有 1 条
	// mom 保持 seeded 的形状（有档案、零书签）：另一句话由它来测。
	r := renderer(t, s)

	empty, err := r.ProfilePage(PageRequest{ID: "dad", Query: "zzzzz不存在"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(empty.Body)
	if !strings.Contains(body, "没有匹配") {
		t.Errorf("搜索无结果要说\"没有匹配\":\n%.200s", body)
	}
	if strings.Contains(body, "这个档案还没有书签") {
		t.Error("有书签的档案搜不到东西，不该说\"还没有书签\"——那是一句谎")
	}
	if !strings.Contains(body, "命中 0 条 · 共 1 条") {
		t.Errorf("搜索态要报出命中数与总数:\n%.300s", body)
	}
	// 反面对照：真的空档案、不带 q，才说那句"还没有书签"。
	blank, err := r.ProfilePage(PageRequest{ID: "mom"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blank.Body), "这个档案还没有书签") {
		t.Errorf("空档案要说没书签:\n%.200s", blank.Body)
	}
	if strings.Contains(string(blank.Body), "没有匹配") {
		t.Error("没搜索却说\"没有匹配\"")
	}
	// 命中态：报数、且只渲染命中那条。
	hit, err := r.ProfilePage(PageRequest{ID: "dad", Query: "dsm"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hit.Body), "命中 1 条 · 共 1 条") {
		t.Errorf("命中态读数不对:\n%.300s", hit.Body)
	}
}

// **这条接手 Task 1 留下的那条指针**："不同 q 绝不互相命中"要等过滤真的进渲染
// 才测得动（Task 1 时两个 q 出的字节相同，而 ETag 是内容哈希，断言它们不同就是
// 断言一个没实现的功能）。现在过滤有了，三条一起钉：
//   - 命中 A 与命中 B 的页面必须有不同 ETag（同 ETag = 缓存里互相命中 = 搜 A 拿到 B）；
//   - 命中 A 与全量页必须不同（搜索态确实改了内容，也顺手证明旁路没把全量页端出来）；
//   - 完全无命中的那个 q 必须与"命中一条"的 q 不同（空结果页与命中页不能长一样）。
func TestProfilePage_不同q不互相命中(t *testing.T) {
	s := seeded(t)
	if err := s.SaveBookmarks("dad", func() *model.Bookmarks {
		b := store.EmptyBookmarks()
		b.Rev = 1
		b.Title = "爸爸的台"
		b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList,
			Items: []model.Item{
				{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
				{ID: "qb", Name: "qBittorrent", URL: "http://10.0.0.3:8181", Alias: []string{}, Tags: []string{}},
			}}}
		return b
	}()); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	a, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad", Query: "群晖"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad", Query: "qBittorrent"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ETag == b.ETag {
		t.Fatalf("搜「群晖」与搜「qBittorrent」拿到同一个 ETag：%q——它们互相命中了", a.ETag)
	}
	none, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad", Query: "zzzzz不存在"})
	if err != nil {
		t.Fatal(err)
	}
	if none.ETag == a.ETag {
		t.Error("零命中的页面与命中的页面同 ETag：过滤没真的作用到渲染")
	}
	all, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if all.ETag == a.ETag {
		t.Error("全量页与搜索页同 ETag：搜索态没改变内容")
	}
	// 同一 q 再来一次仍要稳定（304 那一路的前提）。
	again, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad", Query: "群晖"})
	if err != nil {
		t.Fatal(err)
	}
	if again.ETag != a.ETag {
		t.Errorf("同一个 q 两次渲染 ETag 不稳定：%q vs %q", a.ETag, again.ETag)
	}
}
```

`internal/httpapi/handlers_test.go` 追加（这条测的是反射面，不是搜索功能）。**顺手在这个文件里定义一个常量**，Task 3 与 Task 4 的测试都复用同一条 cookie 串——它必须在这里定义，因为本任务先用它：

```go
// dadFormCookie 是"爸爸本人看自己"的合法会话形状（两条都要有，缺一服务端
// 就当非法会话清掉——见 handlers_test.go 里既有的 PIN 成功用例）。
// 名字带 Form：handlers_test.go:392 与 pinapi_test.go:187 各有一个**函数内局部变量**
// 叫 dadCookie/kidCookie（那是 sessionCookie() 算出来的），同名包级常量合法但会
// 遮蔽出两种含义——一个文件里两个同名不同源的东西正是下一个人读错的地方。
const dadFormCookie = "lb_home=dad; lb_profile=dad"

// q 是用户可控的 query。没有会话时它必须**一个字都不进页面**：选择页没有
// 搜索框，把 q 回声进去就等于开一个反射点（与 ?err= 只查表、不解释同一口径）。
func TestRoot_无会话时q被忽略且不回声(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/?q=%3Cimg%20src%3Dx%20onerror%3Dalert(1)%3E", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "onerror") || strings.Contains(body, "&lt;img") ||
		strings.Contains(body, `name="q"`) {
		t.Errorf("选择页把 q 回声进去了:\n%.400s", body)
	}
}

// 有会话时 q 进搜索框的 value——html/template 在那里转义，两句话都要钉：
// 一条钉它被回填（搜索态），一条钉它没变成可执行上下文。
func TestRoot_搜索框里的q是转义过的(t *testing.T) {
	_, h := setupBatch(t)
	rec := get(h, `/?q=<b>"x`, dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `<b>"x`) {
		t.Fatalf("q 没转义:\n%.400s", body)
	}
	if !strings.Contains(body, `value="`) || !strings.Contains(body, "搜索书签") {
		t.Errorf("搜索框没回填:\n%.400s", body)
	}
}
```

（`get` 是本包既有 helper；`dadFormCookie` 是上面刚定义的那个常量，同包直接可见。）

- [ ] **Step 6: 跑测试确认绿**

Run: `go test ./internal/render/ ./internal/httpapi/ -count=1`

- [ ] **Step 7: 四次【故意破坏】**

1. 把 `matchItem` 里的 `it.Desc` 那条判据删掉 → `TestMatches_六个字段都算命中/描述` 红。
2. 把 `matches` 改成原地过滤（`g.Items = ng.Items` 直接写回入参），或改回"逐字段抄一份 Bookmarks"（少了 `Clock`）→ `TestMatches_不改入参且不吃掉别的字段` 红（前者红入参那半，后者红字段那半）。
3. 把模板里 `{{if .Query}}` 那层去掉，让搜索无结果也走"还没有书签" → `TestProfilePage_搜索无结果与没书签是两句话` 红。
4. 把 `e.profile` 里的 `matches(query, b)` 改成 `matches("", b)`（接住 query 却让过滤永不生效）→ `TestProfilePage_不同q不互相命中` 与 `TestProfilePage_搜索无结果与没书签是两句话` 同时红。**这条就是 Task 1 那条指针的接手证明**。别用"把遍历的 `fb.Groups` 换成 `b.Groups`"那种写法——那会让 `fb` 变成未使用变量，得是一条**编译失败**而不是行为红，看着像红其实什么都没证（2026-10-07 实测）。
4. 把搜索框的 `value="{{.Query}}"` 换成裸拼（改用 `template.HTML` 之类）→ `TestRoot_搜索框里的q是转义过的` 红。

- [ ] **Step 8: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/render internal/httpapi
git commit -m "feat(render,httpapi): GET /?q= 服务端搜索，六个字段算命中且两句空态分开说"
```

---

## Task 3: `POST /prefs`——主题与档案形态从顶栏真的能改

**Files:**
- Create: `internal/httpapi/prefs.go`、`internal/httpapi/prefs_test.go`
- Modify: `internal/model/check.go`（导出 `ValidTheme`/`ValidLayout`）、`internal/model/model_test.go`
- Modify: `internal/httpapi/routes.go`、`openapi.json`、`AGENTS.md`
- Modify: `internal/render/web/profile.tmpl`（主题表单）、`base.css`（顶栏按钮位）

**Interfaces:**
- Consumes: `Store.WithProfileLock`、`Store.LoadProfile`、`Store.SaveProfile`、`auth.ReadSession`、`s.require(r, credSession, id)`、`s.writeProfileReadFailure`
- Produces:

```go
func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request)
func ValidTheme(s string) bool
func ValidLayout(s string) bool
```

- [ ] **Step 1: 先给两个谓词写测试（判据只有一份）**

`internal/model/model_test.go` 追加：

```go
func TestValidThemeLayout_枚举只有一份(t *testing.T) {
	for th, want := range map[string]bool{"auto": true, "dark": true, "light": true,
		"": false, "Auto": false, "pink": false, "system": false} {
		if got := ValidTheme(th); got != want {
			t.Errorf("ValidTheme(%q) = %v，要 %v", th, got, want)
		}
	}
	for ly, want := range map[string]bool{"sidebar": true, "topbar": true,
		"": false, "Sidebar": false, "list": false} {
		if got := ValidLayout(ly); got != want {
			t.Errorf("ValidLayout(%q) = %v，要 %v", ly, got, want)
		}
	}
}
```

- [ ] **Step 2: 导出谓词**

`internal/model/check.go` 在 `allowedTheme`/`allowedLayout` 声明之后加：

```go
// ValidTheme / ValidLayout 是这两组枚举的唯一出口：校验走 map，
// 表单端点走这两个谓词——判据一份，两处消费。
func ValidTheme(s string) bool   { return allowedTheme[s] }
func ValidLayout(s string) bool  { return allowedLayout[s] }
```

Run: `go test ./internal/model -count=1 -run ValidThemeLayout` → 绿。

- [ ] **Step 3: 写 `/prefs` 的失败测试**

```go
// internal/httpapi/prefs_test.go
package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// 发请求的 helper 与常量本包都有了：`postForm(h, path, body, cookie)`、
// `get(h, path, cookie)`、`cookieMap(t, rec)`、`dadFormCookie`（前三件在
// handlers_test.go，`dadFormCookie` 由 Task 2 加在同一文件）。
// **不要再定义第二遍**——同包重名是编译错误，换个名字就是第二套夹具。

func TestPrefs_主题改得动且立刻看得见(t *testing.T) {
	s, h := setup(t)
	// **必须先烤一次首页**。这一条测的是"改了立刻看得见"，而"看不见"有两种原因：
	// 端点没写进去，或写进去了而缓存端出来的还是旧的。不先请求一次，第二次请求是
	// 冷缓存、必然重算——把 `profMod` 从键里删掉这条照样绿（Task 3 实测过），
	// 于是 §7.1 与 §13 承诺的那位证人是个空位。
	warm := get(h, "/", dadFormCookie)
	if strings.Contains(warm.Body.String(), `data-theme="light"`) {
		t.Fatal("夹具一开始就是 light，换成 light 就测不出变化了")
	}
	rec := postForm(h, "/prefs", "theme=light", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("改成功要 303 回首页，得到 %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q，要 /", loc)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "light" {
		t.Errorf("盘上 theme = %q，要 light", p.Theme)
	}
	// 关键一步：紧接着读首页，必须已经带上新主题。这一条同时是
	// Task 1 那个 profileMtime 缓存键维度的端到端证人。
	page := get(h, "/", dadFormCookie)
	if !strings.Contains(page.Body.String(), `data-theme="light"`) {
		t.Error("改了主题而页面没变：缓存键里少了 profile.json 的指纹")
	}
}

func TestPrefs_形态与主题可以各改各的(t *testing.T) {
	s, h := setup(t)
	if rec := postForm(h, "/prefs", "layout=topbar", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("只改 layout 要 303，得到 %d", rec.Code)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Layout != "topbar" || p.Theme != "auto" {
		t.Errorf("只给 layout 却动了别的字段：layout=%q theme=%q", p.Layout, p.Theme)
	}
	// 写出去的文件仍守 §4.3：缩进 2 空格、末尾一个换行、中文不转义。
	raw, err := os.ReadFile(s.ProfilePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(raw, []byte("}\n")) || bytes.HasSuffix(raw, []byte("\n\n")) {
		t.Errorf("profile.json 的写出形状破了 §4.3: %q", raw[len(raw)-8:])
	}
}

// 非法值与"没说改什么"都是 422，而且要**点名是哪个字段**——422 不指字段，
// 调用方只能猜；而"两个字段都不给"静默 303 会让一个拼错的字段名假装成功。
func TestPrefs_非法与缺字段都422且点名(t *testing.T) {
	_, h := setup(t)
	for _, tc := range []struct {
		name, body, wantField string
	}{
		{"非法主题", "theme=pink", `"field": "theme"`},
		{"非法形态", "layout=list", `"field": "layout"`},
		{"大小写不放过", "theme=Light", `"field": "theme"`},
		{"两个都不给", "", `"field": "theme"`},
		{"只有无关字段", "color=red", `"field": "layout"`},
		{"同名多值", "theme=dark&theme=light", `"field": "theme"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postForm(h, "/prefs", tc.body, dadFormCookie)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"code": 422`) {
				t.Errorf("要走统一错误外壳: %s", rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tc.wantField) {
				t.Errorf("422 要点名字段 %q，得到: %s", tc.wantField, rec.Body)
			}
		})
	}
}

// 浏览器通路的**权限类**拒绝 = 303 回选择页、不写盘（spec §6.1）。
// 令牌走 /prefs 与没带会话走 /prefs 是同一种长相：这条路本来就不给 AI 用，
// 而"你是令牌所以我拒"这种区分只在 API 通路有意义（那里 401/403 永远分开）。
// 断言的核心不是状态码，是**盘上没变**——把 credSession 换成 credScoped，
// 管理员令牌就能替所有人改偏好，那时状态码仍是 303 而这条会红。
func TestPrefs_令牌与无会话都不写盘(t *testing.T) {
	for _, tc := range []struct {
		name, cookie, tok string
	}{
		{"没有凭证", "", ""},
		// 令牌必须**带着自己的会话 cookie**来：不带 cookie 时 sess.Active 是空串，
		// require 开头那条"credScoped 不申报档案一律拒"会先把它挡下来——于是把
		// credSession 换成 credScoped 照样拒、盘照样不动、状态码照样 303，这条就
		// 永远测不到它要测的那个错（Task 3 实测过一次：按原写法三支全不红）。
		{"管理员令牌带着 dad 的会话", dadFormCookie, adminTok},
		{"档案作用域令牌带着 dad 的会话", dadFormCookie, dadTok},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 每个子用例各起一套 setup(t)：共用一个 store 时第一支把盘写脏，
			// 后面两支只是继承那份脏盘，"到底哪一支漏了"就答不出来了。
			s, h := setup(t)
			req := httptest.NewRequest(http.MethodPost, "/prefs", strings.NewReader("theme=dark"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.tok != "" {
				req.Header.Set("Authorization", "Bearer "+tc.tok)
			}
			if tc.cookie != "" {
				req.Header.Set("Cookie", tc.cookie)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
				t.Errorf("得到 %d %q，要 303 回选择页（浏览器通道的拒不该吐 401/403 页面）",
					rec.Code, rec.Header().Get("Location"))
			}
			p, err := s.LoadProfile("dad")
			if err != nil {
				t.Fatal(err)
			}
			if p.Theme == "dark" {
				t.Fatalf("被拒的一支真的落盘了：theme=%q", p.Theme)
			}
		})
	}
}

// 管理员在代管态下按主题，改的是**正在看的那个档案**（active），不是自己。
// 与"管理员能替她设 PIN"同一条权力线，横幅那句话本来就在提醒这件事。
func TestPrefs_代管态改的是active档案(t *testing.T) {
	s, h := setup(t)
	rec := postForm(h, "/prefs", "theme=dark", "lb_home=dad; lb_profile=kid")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	kid, err := s.LoadProfile("kid")
	if err != nil {
		t.Fatal(err)
	}
	dad, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if kid.Theme != "dark" || dad.Theme == "dark" {
		t.Errorf("改错了对象：kid=%q dad=%q", kid.Theme, dad.Theme)
	}
}

// profile.json 坏了要答 503（与 self/pin 同口径），不许把"文件坏了"说成"服务坏了"。
// 注意这**不是**权限拒绝：读失败是第三种东西，422/503 都照旧走 JSON 外壳，
// 只有"你没资格"这一类才映射成 303。
func TestPrefs_坏profile答503(t *testing.T) {
	s, h := setup(t)
	if err := os.WriteFile(s.ProfilePath("dad"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postForm(h, "/prefs", "theme=dark", dadFormCookie)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "profile.json") {
		t.Errorf("503 的 detail 要点名文件（spec §12）: %s", rec.Body)
	}
}
```

`prefs_test.go` 的 import 用得到 `bytes`（写出形状那条）与 `os`，用不到 `model`——判据在端点里引用 `model.ValidTheme`，测试不该再引用一次。`setup(t)` 就建了 dad（管理员）与 kid 两个档案（`setupServer` 里写的），所以代管那条不必换成 `setupBatch`。

- [ ] **Step 4: 跑它，确认它红**

Run: `go test ./internal/httpapi -count=1 -run Prefs`
Expected: FAIL——`/prefs` 未注册（404），以及 `redirectPicker` 未定义。

- [ ] **Step 5: 写 prefs.go**

```go
package httpapi

import (
	"errors"
	"net/http"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// handlePrefs 是浏览器表单通路：改的是"这个人对自已台面的偏好"
// （主题 + 档案形态）。它**不是** API 端点，也不建 JSON 版——表单发不出
// JSON，而 AI 替你换主题没有真实需求；每多一条 API 面就要 openapi 与
// AGENTS.md 各同步一次（spec §6.1）。
//
// 为什么只认会话不认令牌：与 self/pin 同族同判据——"谁能改谁"。管理员令牌
// 替别人改偏好是另一回事，本期没有这个需求，也不预留。
//
// 拒的长相是 303 回选择页，不是 401/403（spec §6.1 的浏览器通路映射）：
// 这条路只由页面表单走进来，走到这里就没人能消费一段 JSON 错误。
// 422（形状错）与 503（文件读不了）照旧走 JSON 外壳——它们不是权限拒绝。
//
// 为什么不写历史行：它不动书签、不产生 rev，与"改自己 PIN"同类。四条不写
// 审计行的动作一起记在 spec §15，防止下一个人在这里"顺手补一行"。
func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		redirectPicker(w)
		return
	}
	// 上限必须在这里设：ParseForm 会整段读 body，不套 MaxBytesReader 的话承诺的
	// 1 MiB 对这条无 JS 通路是假话（handlePin 的表单分支为同一件事红过一次）。
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		// 字段名跟着 handlePin 的同一个失败走（body）：同一个病在两个端点上叫两个名，
		// AI 照着文档生成的请求就分家了。
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	theme, hasTheme := r.Form["theme"]
	layout, hasLayout := r.Form["layout"]
	if !hasTheme && !hasLayout {
		// "没说要改什么"不是"什么都不做然后成功"：那会让一个拼错的字段名
		// 静默返回 200，而调用方以为改成了。
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
			{Field: "theme", Reason: "要带 theme 或 layout 至少一个"},
			{Field: "layout", Reason: "要带 theme 或 layout 至少一个"},
		})
		return
	}
	// 同名多值（?theme=dark&theme=light）不是"取第一个"，是没说清要改什么。
	if hasTheme && (len(theme) != 1 || !model.ValidTheme(theme[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "theme", Reason: "只能是 auto|dark|light"}})
		return
	}
	if hasLayout && (len(layout) != 1 || !model.ValidLayout(layout[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "layout", Reason: "只能是 sidebar|topbar"}})
		return
	}
	var saveErr error
	id := sess.Active
	// 读→改→验→写整段进锁，与 handleSelfPin 同形（评审 Important 1 的教训：
	// SaveProfile 不自带锁，裸 RMW 与并发的管理员设 PIN 撞车会互相盖掉）。
	err := s.deps.Store.WithProfileLock(id, func() error {
		p, err := s.deps.Store.LoadProfile(id)
		if err != nil {
			return err // 分诊在锁外做：404 / 503 / 500
		}
		// "没给的字段不碰"由这两个 if 各自保证（TestPrefs_形态与主题可以各改各的钉着）。
		// 不要再抄一对 newTheme/newLayout 局部变量：它们在闭包里被赋值、闭包外没人读。
		if hasTheme {
			p.Theme = theme[0]
		}
		if hasLayout {
			p.Layout = layout[0]
		}
		saveErr = s.deps.Store.SaveProfile(p)
		return nil
	})
	if err != nil {
		// 一次分诊就够：writeProfileReadFailure 内部已经分 404 / 503（坏档，带文件与位置）/
		// 500。先判两个哨兵再判 err，等于把同一条口径抄两遍——抄两遍就会分家。
		s.writeProfileReadFailure(w, id, err)
		return
	}
	if saveErr != nil {
		writeInternalError(w, saveErr)
		return
	}
	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusSeeOther)
}
```

`handlers.go` 里紧跟 `redirectPinFail` 之后加这一个函数（三条浏览器通路的拒都从这里走，判据一份）：

```go
// redirectPicker 是浏览器通路的"拒"（spec §6.1）：303 回档案选择页、不报错、
// 不带 ?err=。它与 API 通路的 writeAuthFailure 是**两种长相、一套判据**——
// 判据永远在 require 那一份里，这里只改外壳。
//
// 为什么不复用 redirectPinFail：那条是给"输错了"用的（带 ?err= 才说得出人话），
// 权限类拒绝带提示反而是在告诉调用方"你认得的这个人不是管理员"。
func redirectPicker(w http.ResponseWriter) {
	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusSeeOther)
}
```

`/prefs` 与 `/switch`、`/admin` 成功后的 303 也用它（成功回 `/`、拒也回 `/`，两处语义不同但字节相同——差别在**注释**与调用位置，不在状态码）。

- [ ] **Step 6: 注册路由并同步契约三处**

`routes.go`：

```go
		{Method: "POST", Pattern: "/prefs", Cred: credSession,
			Summary: "改自己的台面偏好（主题/形态，仅会话，表单通路）", H: s.handlePrefs},
```

`openapi.json` 加 `paths["/prefs"]`：`post` + `summary` **逐字等于**路由表的 Summary、`x-credential: "session"`、`security: [{"sessionCookie": []}]`、`requestBody` 是 `application/x-www-form-urlencoded` 的 `{theme?, layout?}`（`enum` 写全 `auto|dark|light` 与 `sidebar|topbar`）、`description` 写明"浏览器表单通路，AI 不走这条；权限类拒绝答 303 回选择页（不吐 401/403 页面），形状错 422、`profile.json` 读不了 503"、`responses` 给 `303` / `422` / `503` 三条——**没有 401/403**，那两条属于 API 通路。

`AGENTS.md` 端点表加一行（说明列要写清"仅会话、浏览器表单通路、AI 不走这条"）：

```
| POST | `/prefs` | 仅会话（表单） | 303 回 `/` | 改自己的主题与档案形态；`theme` ∈ `auto|dark|light`、`layout` ∈ `sidebar|topbar`，两个字段都不给 = 422；令牌一律拒 |
```

- [ ] **Step 7: 顶栏挂上主题按钮**

`profile.tmpl` 的 `<nav aria-label="档案">` 之前插：

```gotemplate
  {{/* 主题循环：auto→dark→light→auto（spec §8.1 的三态）。两态互切是错的——
       auto 是建档默认值（store.CreateProfile），一旦离开就再也回不去，
       "跟随系统"这个选项等于不存在。值由服务端算并写死在 hidden 里：
       不用 JS 读当前值，因为"当前值"服务端本来就知道。 */}}
  <form method="post" action="/prefs" class="theme">
    <input type="hidden" name="theme" value="{{if eq .Profile.Theme "auto"}}dark{{else if eq .Profile.Theme "dark"}}light{{else}}auto{{end}}">
    <button type="submit">{{if eq .Profile.Theme "auto"}}切到暗色{{else if eq .Profile.Theme "dark"}}切到亮色{{else}}切回跟随系统{{end}}</button>
  </form>
```

`base.css` 的顶栏栅格补一列（`grid-template-columns:1fr auto auto auto`），并给 `.theme button` 复用 `.pick button` 那套 44px 触摸目标。

- [ ] **Step 8: 跑测试确认绿**

Run: `go test ./internal/httpapi/ ./internal/render/ ./internal/model -count=1`
Expected: 全绿，含四条契约闸（`/prefs` 已进 openapi 与 AGENTS.md）。

- [ ] **Step 9: 八次【故意破坏】**

1. 把 `profMod` 那一维从 Task 1 的键里去掉 → `TestPrefs_主题改得动且立刻看得见` 的最后一断言红（**这条就是配对纪律的证人**：端点与键两半，缺任一半这条都红）。
2. 把 `!hasTheme && !hasLayout` 那条去掉 → `TestPrefs_非法与缺字段都422且点名/两个都不给` 红（会静默 303）。
3. 把 `require(r, credSession, …)` 换成 `credScoped` → `TestPrefs_令牌与无会话都不写盘/管理员令牌带着 dad 的会话` 红（管理员令牌替所有人改了偏好，而状态码仍是 303——**这条测的是盘，不是码**）。**这一支必须带着会话 cookie 来**：不带 cookie 时 `sess.Active` 是空串，`credScoped` 开头那条「空申报一律拒」会先把它挡下，于是破坏与不破坏都绿——那是一个测不到任何东西的证人。
4. 把拒的那一支从 `redirectPicker(w)` 换回 `s.writeAuthFailure(w, r)` → 三个子用例全红（浏览器通路吐出一段 401/403 JSON，家人看到的是裸错误页）。
5. 去掉 `r.Body = http.MaxBytesReader(...)` 那一行 → `TestPrefs_超限表单被拒且不落盘` 红。
6a. 把主题循环退化成两态互切（`light→dark`、其余→`light`）→ `TestProfilePage_顶栏主题表单交得出合法值` 在 `theme=auto` 与 `theme=light` 两格红。**"值合法且不等于当前值"放过这个 bug**：两态完全满足那两条，却把 `auto` 变成回不去的状态——所以要逐格钉 `wantNext`/`wantLabel`（三态循环的完整形状），文案也要钉，因为按钮上的字说的就是下一档。
6b. 去掉 `ParseForm` 出错那一支的点名（把 `Field: "body"` 改回空字段或整支删掉）→ `TestPrefs_表单字节坏422点名body` 红：状态码仍是 422（空表单落进"两个都不给"那一格），所以**只有点名的断言**能把"读不出来"和"没说改什么"这两种病分开。

- [ ] **Step 10: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/httpapi internal/model internal/render AGENTS.md
git commit -m "feat(httpapi,model): POST /prefs 表单通路改主题与形态，枚举判据收进 model 一份"
```

---

## Task 4: `POST /switch` 与管理横幅

**Files:**
- Create: `internal/httpapi/switch.go`、`internal/httpapi/switch_test.go`
- Modify: `internal/httpapi/routes.go`、`openapi.json`、`AGENTS.md`
- Modify: `internal/render/exec.go`（`profileView` 补 `Manage`）、`internal/render/web/profile.tmpl`（横幅）、`base.css`（sticky + 高对比）
- Modify: `internal/httpapi/handlers.go`（把 home 的显示名传进渲染）

**Interfaces:**
- Consumes: `auth.Session{Active, Home}`、`s.activeIsAdmin`、`auth.SetActive`、`auth.ClearSession`、Task 1 的 `PageRequest.Home`
- Produces:

```go
func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request)
// render 侧
type manageView struct{ HomeTitle, ActiveTitle string }
// profileView 补 Manage *manageView
```

- [ ] **Step 1: 写 `/switch` 的失败测试**

```go
// internal/httpapi/switch_test.go
package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
)

// 夹具复用本包既有的三件：setupBatch(t)（dad=爸爸 管理员 + kid=小满 非管理员，
// 各带一份书签）、postForm(h, path, body, cookie)、cookieMap(t, rec)。
// 不要在这里再定义第二套——同包重名是编译错误，换个名字是第二份夹具。

func TestSwitch_退出管理把active写回home(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=home", "lb_home=dad; lb_profile=kid")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] != "dad" {
		t.Errorf("退出管理没把 active 写回 dad: %v", c)
	}
}

// 非管理态收到 to=home 是无操作，但**不是错误**：幂等，仍然 303 回首页。
func TestSwitch_非管理态退出是无操作不是报错(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=home", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d，要 303（幂等）", rec.Code)
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] != "dad" {
		t.Errorf("无操作也不该把 active 写成别人: %v", c)
	}
}

func TestSwitch_管理员免PIN直达别人(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=kid", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	c := cookieMap(t, rec)
	// SetActive 只写 lb_profile（home 不动）——这里钉的就是"home 没被顺手改写"：
	// 退出管理的路标是 home，把它抹掉管理员就退不回来了。
	if c[auth.CookieProfile] != "kid" {
		t.Errorf("active 该切到 kid: %v", c)
	}
	if v, ok := c[auth.CookieHome]; ok && v != "" && v != "dad" {
		t.Errorf("home 被改写了: %v", c)
	}
}

// spec §6.1 那句话的证人：免 PIN 的许可看**当前在用档案**的 admin 标记。
// kid 的会话里伪造 to= 跳第三个人，必须拒——而拒的长相是 303 回选择页，
// 不是 403 页面（浏览器通路，spec §6.1）。
func TestSwitch_非管理员伪造to跳别人被拒(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=dad", "lb_home=kid; lb_profile=kid")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d，要 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q，要回选择页 /（不带 ?err=：那是权限拒绝，不是输错 PIN）", loc)
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] == "dad" {
		t.Error("被拒却把会话切过去了：那是越权写入 cookie")
	}
}

func TestSwitch_回选择页清两条cookie(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=picker", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	c := cookieMap(t, rec)
	if c[auth.CookieProfile] != "" || c[auth.CookieHome] != "" {
		t.Errorf("回选择页要清掉两条 cookie: %v", c)
	}
}

// 目标档案不存在：也是 303 回选择页，不给"这个 id 有没有"这种枚举线索
// ——与"无权"同一句话（spec §6.1：少一种枚举面）。
func TestSwitch_目标不存在不报错也不切(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=nobody", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] == "nobody" {
		t.Error("切到了不存在的档案")
	}
}

// 目标档案存在但 profile.json 读不了：不切过去（切过去就是一页读不了的台），
// 也不把它说成服务坏了。拒的长相仍然是 303，不是 503。
func TestSwitch_目标档案读不了不切(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.ProfilePath("kid"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postForm(h, "/switch", "to=kid", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	if c := cookieMap(t, rec); c[auth.CookieProfile] == "kid" {
		t.Error("切进了一个 profile.json 读不了的档案")
	}
}

func TestSwitch_没有会话一律回选择页(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/switch", "to=home", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d，要 303（浏览器通道的拒不是 401 页面）", rec.Code)
	}
}

// 三种拒绝都不许带令牌分支：/switch 只认会话，与 /prefs 同族。
func TestSwitch_令牌走不通(t *testing.T) {
	_, h := setupBatch(t)
	req := httptest.NewRequest(http.MethodPost, "/switch", strings.NewReader("to=kid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Errorf("令牌打 /switch：得到 %d %q，要 303 回选择页", rec.Code, rec.Header().Get("Location"))
	}
}

// 输错 PIN 不是权限拒绝，它照旧回选择页并带 ?err= 提示（spec §6.1 里那句
// "不许被一起统一成裸 303"的证人）。这条测的是既有通路，本期把它钉成回归项：
// 有人会把 /pin 的失败分支"顺手"改成裸 303。
func TestSwitch_相邻那条_PIN输错仍带err提示(t *testing.T) {
	_, h := setupBatch(t)
	rec := postForm(h, "/pin", "profile=kid&pin=0000", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/?err=") {
		t.Errorf("PIN 输错要带 ?err= 提示，得到 %q——它不是权限拒绝，不许统一成裸 303", loc)
	}
	if pinNotices[strings.TrimPrefix(loc, "/?err=")] == "" {
		t.Errorf("err 键必须在白名单表里: %q", loc)
	}
	if _, ok := rec.Header()["Set-Cookie"]; ok {
		t.Error("输错 PIN 不该发会话 cookie")
	}
}
```

`switch_test.go` 的 import 需要 `net/http/httptest` 与 `os`（其余都在上面）。`pinNotices` 是本包的非导出符号，测试同包可见——这条用它把"提示表还在被读"钉住。

- [ ] **Step 2: 跑它，确认它红**

Run: `go test ./internal/httpapi -count=1 -run Switch`
Expected: FAIL——`/switch` 未注册。

- [ ] **Step 3: 写 switch.go**

```go
package httpapi

import (
	"net/http"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// handleSwitch 是浏览器表单通路的会话动作，一个端点三件事（spec §6.1、§8.9）：
//
//	to=home    退出管理：active 写回 home。非管理态收到也是 303，幂等不报错。
//	to=picker  清两条 cookie 回选择页。
//	to=<id>    免 PIN 直达别的档案。**判据是当前在用档案的 admin 标记**
//	           （回查磁盘，不信 cookie 形状）——不是 home。伪 lb_home
//	           换不到切档；这条规则 §6.1 早就写了，本任务只是给它一条路由。
//
// 四种拒绝（无会话、无权、目标不存在、目标读不了）一律 303 回选择页，不回
// 403 页面：给家属看错误页是坏体验，而"这个 id 存不存在"是可枚举信息。
//
// 不写历史行：它不动别人的书签，只动"我现在在看谁"。
func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request) {
	// 这个处理器每条出路都是 303 回选择页——成功回 "/" 看一眼刚切的台，
	// 拒也回 "/" 什么也不说。外壳取自 redirectPicker（判据一份、长相一份）。
	back := func() { redirectPicker(w) }
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		back()
		return
	}
	if err := r.ParseForm(); err != nil {
		back()
		return
	}
	to := r.FormValue("to")
	switch to {
	case "picker":
		auth.ClearSession(w)
		back()
		return
	case "home":
		if sess.Home == "" {
			auth.ClearSession(w)
		} else {
			auth.SetActive(w, sess.Home)
		}
		back()
		return
	case "":
		back()
		return
	}
	// to=<id>：跨档案，权限只看 active 是不是管理员（activeIsAdmin 回查磁盘）。
	if to == sess.Active {
		back() // 切到自己：无操作，但仍不报错
		return
	}
	if !s.activeIsAdmin(sess) {
		back()
		return
	}
	if _, err := s.deps.Store.LoadProfile(to); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			back() // 没这个档案：与"无权"同一句话，不留枚举面
			return
		}
		// 目标档案的 profile.json 坏了：不切过去（切过去就是一页读不了的台），
		// 也不把它说成服务坏了。
		back()
		return
	}
	auth.SetActive(w, to)
	back()
}
```

补 `errors` 进 import。

- [ ] **Step 4: 注册 + 契约三处同步**

`routes.go`：

```go
		{Method: "POST", Pattern: "/switch", Cred: credSession,
			Summary: "会话动作：退出管理/回选择页/管理员免 PIN 直达他人档案", H: s.handleSwitch},
```

`openapi.json`：`paths["/switch"]`，`x-credential: "session"`、`security: [{"sessionCookie": []}]`、form body `{to}`、响应 `303` + 说明"无会话、无权、目标不存在、目标读不了这四种拒都照旧答 303 回选择页，不回 403 页面，也不给枚举线索"。
`AGENTS.md` 端点表加一行，说明列写清"仅会话、表单通路；四种拒绝都是 303 回选择页，不是 403"。

- [ ] **Step 5: 写横幅的失败测试**

```go
// internal/render/views_test.go 追加（夹具全用本包既有的：seeded 建 dad 与 mom
// 两个档案，Title 就等于 id；save 落一份带 1 条书签的 bookmarks.json）
func TestProfilePage_横幅只在代管态出现且两个名字都在(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	save(t, s, "mom", "妈妈的台", "emby")
	r := renderer(t, s)

	// 同一个档案（mom），两种会话形状：本人看自己 vs 被人代管。
	own, err := r.ProfilePage(PageRequest{ID: "mom", Home: "mom"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(own.Body), "正在管理") {
		t.Error("home==active 却出了横幅——本人会以为自己正在越权看别人")
	}
	mgr, err := r.ProfilePage(PageRequest{ID: "mom", Home: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(mgr.Body)
	if !strings.Contains(body, "正在管理") {
		t.Errorf("代管态缺了管理横幅:\n%.300s", body)
	}
	// 被管的那个名字（页面主角）与操作者的名字（home）都要在 DOM 里：
	// 只有"正在管理"没有对象名等于没提醒，只有对象名没有操作者同样。
	if !strings.Contains(body, "正在管理 <b>mom</b>") {
		t.Errorf("横幅要点名被管的档案:\n%.300s", body)
	}
	if !strings.Contains(body, "dad") {
		t.Error("横幅要说清是谁在管——只有对象名等于没提醒")
	}
	if !strings.Contains(body, `action="/switch"`) {
		t.Error("横幅必须带退出管理的表单按钮（它是防手滑的把手）")
	}
	// 形状钉：横幅要有自己的 class，DOM 断言与实拍都靠它。
	if !strings.Contains(body, `class="banner"`) {
		t.Error("横幅要有自己的 class，DOM 断言与实拍都靠它")
	}
	// 管理员看自己的台也不该有横幅（这一维在 handler 判，渲染器只认 home≠id）。
	dadOwn, err := r.ProfilePage(PageRequest{ID: "dad", Home: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dadOwn.Body), "正在管理") {
		t.Error("本人看自己，不该有管理横幅")
	}
	// **同一个档案、同一份书签，只有 home 不同 → ETag 必须不同**。
	// 这条钉的是 Task 1 那一维在真内容上的兑现：浏览器拿过本人那页的 ETag，
	// 代管态再来一次 304，就会把没横幅的旧页面当成"我现在没在管理"。
	if mgr.ETag == own.ETag {
		t.Errorf("代管页与本人页同 ETag：%q——304 会把带横幅的页面与不带的那页混起来", mgr.ETag)
	}
}
```

`base.css` 那条 `position:sticky` 也要有形状钉：`TestCSS_硬规格在场` 的 `wants` 追加一行（与 Task 6 追加的那两条同一处，别新开一张表）：

```go
		"管理横幅不随滚走": `.banner{position:sticky;top:0`,
```

- [ ] **Step 6: 渲染侧接上横幅数据**

`internal/render/exec.go`：

```go
// manageView 是管理横幅的数据。指针 = "这一页不是代管态"，模板里 {{with}} 一次判掉。
// 判据不在模板里做：模板不做判断（与 validatedAccent 同一条纪律）。
type manageView struct {
	HomeTitle   string // 谁在管
	ActiveTitle string // 管的是谁
}
```

`profileView` 在既有四个字段（`Profile`、`Theme`、`Bookmarks`、`CSS`）之后加一个，并保留 `Query/Hits/Total`（Task 2 加的）：

```go
	Manage *manageView
```

`executor.profile` 的签名加一个 `home *model.Profile`（handler 已经把两条都读出来了，见下），并在构造视图时：

```go
	var manage *manageView
	if home != nil && home.ID != p.ID {
		manage = &manageView{HomeTitle: home.Title, ActiveTitle: p.Title}
	}
```

`Renderer.ProfilePage` 里：`Home` 非空且不等于 `ID` 时，多 `LoadProfile(req.Home)` 一次拿它的 `Title`；**这一步的合法性判据在 handler**（渲染器不认识"谁是管理员"），渲染器只负责"给了 home 档案就画横幅"。

`LoadProfile(req.Home)` 失败时**不画横幅、也不让整页失败**：这一页的主角是 `ID` 那个档案，它的书签与主题都读得出来，页面就该出得来；而 handler 只在 `sessionLegal` 证实 home 可读且是管理员时才传 Home，读到这里还失败说明那一秒之间 home 的 `profile.json` 坏了——把横幅摘掉是正确降级（多说一句"正在管理"比把台面包成错误页更糟）。键里已经带了 `home` 这一维，缓存不会因为少一条横幅而串。

`internal/httpapi/handlers.go#handleRoot` 在调 `ProfilePage` 之前判代管（**这一段取代 Task 2 Step 4 里那条 `Home: sess.Home` 的写法**——那时还没有横幅，home 原样进键；现在必须先把"要不要画"判出来）：

```go
	// 代管态 = 会话合法（sessionLegal 已经查过 home 是管理员）且两条 cookie 不同。
	// 判据只在这里做一次：渲染器不认识权限，模板更不认识。
	managing := sess.Home != "" && sess.Home != sess.Active && s.sessionLegal(sess)
	page, err := s.deps.Render.ProfilePage(render.PageRequest{
		ID: sess.Active, Home: homeOf(managing, sess), Query: r.URL.Query().Get("q")})
```

```go
// homeOf 把"要不要画横幅"翻成渲染器需要的输入：不代管就不传 home。
// 缓存键里 home 传空串而不是 sess.Home，是为了让"本人看自己"这一族只占一条缓存，
// 与代管态天然分开。
func homeOf(managing bool, sess auth.Session) string {
	if !managing {
		return ""
	}
	return sess.Home
}
```

> Task 1 的 `TestProfilePage_缓存键认得home这一维` 用的是显式 home，不受这个改动影响。

`profile.tmpl` 在 `<header>` 之后、`<main>` 之前插：

```gotemplate
{{with .Manage}}
  {{/* 管理横幅：sticky 才"不随页面滚走"（spec §8.9）。两个名字都要在——
       只有"正在管理"没有对象名等于没提醒。不加 aria-live：它是初始内容，
       播报不了，只会给屏读添噪。 */}}
  <div class="banner">
    <span>正在管理 <b>{{.ActiveTitle}}</b> 的书签 · 改动会进<b>{{.ActiveTitle}}</b>的历史 · 由 <b>{{.HomeTitle}}</b> 操作</span>
    <form method="post" action="/switch">
      <input type="hidden" name="to" value="home">
      <button type="submit">退出管理</button>
    </form>
  </div>
{{end}}
```

`base.css`：

```css
/* 管理横幅（spec §8.9）：sticky 不随滚走、高对比、forced-colors 下留边框。
   它是防手滑的把手，不是通知条——所以不参与入场 stagger（动效只给可交互对象）。 */
.banner{position:sticky;top:0;z-index:5;display:flex;gap:12px;align-items:center;
  flex-wrap:wrap;margin:0 0 14px;padding:10px 14px;border-radius:12px;
  border:1px solid var(--ring);background:var(--card-hi);color:var(--ink);
  font-size:13px;backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px)}
.banner button{font:inherit;cursor:pointer;min-height:44px;padding:6px 12px;
  border:1px solid var(--ring);border-radius:10px;background:transparent;color:var(--ink)}
@media (forced-colors:active){.banner{border:2px solid CanvasText;background:Canvas}}
```

- [ ] **Step 7: 顶栏「换档案」= §8.1 那句"档案徽标（点开切档/退出）"在零 JS 下的形状**

没有下拉面板（那要 JS），所以直接给一枚表单按钮走 `/switch` 的 `to=picker`：清两条 cookie 回选择页，从那里可以选别人（要 PIN），管理员也可以免 PIN 代管。**本期不给它单独的测试夹具**——它与横幅同一条通路、同一个端点，Step 5 的横幅测试与下面这条一起把两种 `to=` 都覆盖了。

`profile.tmpl` 的 `<nav aria-label="档案">` 里，徽章之后加：

```gotemplate
    {{/* 换档案：零 JS 下"切档/退出"的唯一形状。to=picker 只清会话、
         不动任何数据，所以任何有效会话都可以按它（spec §6.1）。 */}}
    <form method="post" action="/switch" class="switch">
      <input type="hidden" name="to" value="picker">
      <button type="submit">换档案</button>
    </form>
```

`base.css`：把 Step 6 那条 `.banner button{…}` 换成这一条（同一形状的两颗按钮共用一条声明，同名样式留两份就是等着分家）：

```css
.banner button,.switch button{font:inherit;cursor:pointer;min-height:44px;padding:6px 12px;
  border:1px solid var(--ring);border-radius:10px;background:transparent;color:var(--ink)}
```

`views_test.go` 追加：

```go
func TestProfilePage_顶栏有换档案的表单出路(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	body := string(mustRender(t, s, "dad"))
	if !strings.Contains(body, `name="to" value="picker"`) {
		t.Errorf("顶栏缺换档案的出路（§8.1 的切档/退出在零 JS 下只有这一个形状）:\n%.400s", body)
	}
	// 它是表单不是链接：/switch 只收 POST。
	if strings.Contains(body, `href="/switch"`) {
		t.Error("/switch 不许发 GET 链接")
	}
}
```

- [ ] **Step 8: 跑测试确认绿**

Run: `go test ./internal/render/ ./internal/httpapi -count=1`

- [ ] **Step 9: 五次【故意破坏】**

1. 把 `manage` 的判定改成 `home != nil`（不判 ID 不同）→ 本人看自己也出横幅 → 第一条断言红。
2. 把 `handleSwitch` 里 `if !s.activeIsAdmin(sess)` 那两行删掉 → `TestSwitch_非管理员伪造to跳别人被拒` 红（cookie 被切走）。
3. 把 `to=<id>` 分支的 `LoadProfile(to)` 错误分支改成"照切"→ `TestSwitch_目标不存在不报错也不切` 与 `TestSwitch_目标档案读不了不切` 同时红。
4. 把模板里 `正在管理 <b>{{.ActiveTitle}}</b>` 的 `<b>` 去掉（"名字还在，只是不醒目标"）→ `正在管理 <b>mom</b>` 那条形状钉红。**这一条证的是"对象名要能被 DOM 认出来"**，Task 9 的实拍读数按同一个字符串去抓。
5. 删掉顶栏那枚 `to=picker` 表单 → `TestProfilePage_顶栏有换档案的表单出路` 红。

- [ ] **Step 10: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/httpapi internal/render AGENTS.md
git commit -m "feat(httpapi,render): POST /switch 三态会话动作与 sticky 管理横幅"
```

---

## Task 5: `GET /admin` 读侧——管理页与那一份跨档案聚合

**Files:**
- Create: `internal/httpapi/adminpage.go`、`internal/httpapi/adminpage_test.go`、`internal/render/web/admin.tmpl`
- Modify: `internal/httpapi/admin.go`（`handleStatus` 的遍历抽成 `statusRows`；`statusProfile` 补 `Modified`）
- Modify: `internal/render/render.go`（`AdminRow` + `AdminPage`）、`internal/render/exec.go`（`adminView` + `executor.admin` + `adminT`）
- Modify: `internal/httpapi/routes.go`、`openapi.json`、`AGENTS.md`、`internal/render/web/base.css`
- Modify: `internal/render/web/profile.tmpl`（管理徽章变成链接）、`internal/render/views_test.go`（翻掉那条死链钉）

**Interfaces:**
- Consumes: `Store.ListProfiles/Broken/BrokenReason/Current/SnapshotCount`、`s.require(r, credAdmin, "")`、Task 3 的 `redirectPicker`、Task 4 的 `POST /switch`（页面上的「进入编辑」就是它）
- Produces:

```go
func (s *Server) statusRows() ([]statusProfile, []string, statusTotals, error)
func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request)
// render 侧（字段序与 Step 4 的 struct 一致：ID, Title, Admin, Rev, Modified, Items, Snapshots, Broken）
type AdminRow struct {
	ID, Title string
	Admin     bool
	Rev       int64
	Modified  string // RFC3339 UTC；空串 = 读不出来（坏档），不编造
	Items     int
	Snapshots int
	Broken    bool
}
func (r *Renderer) AdminPage(rows []AdminRow) (Page, error)
```

`statusProfile` 多一个字段 `Modified string \`json:"modified"\``，位置紧跟 `Rev`。这是**新增键**，不是改名：`/api/v1/status` 的既有消费者（CLI、AI）拿不到的字段不会坏，而 `openapi.json` 里那条 schema 必须同步补上——契约测试只比路由与 summary，schema 漂了没人红，所以本任务把它列进改动清单。

- [ ] **Step 1: 写管理页的失败测试**

```go
// internal/httpapi/adminpage_test.go
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

const kidFormCookie = "lb_home=kid; lb_profile=kid"

func TestAdmin_管理员拿到页面且不含条目内容(t *testing.T) {
	s, h := setupBatch(t) // dad=爸爸(admin)、kid=小满；书签里有"群晖/动画"与 10. 段地址
	if err := s.SaveBookmarks("kid", &model.Bookmarks{Rev: 1, Title: "小满的", Groups: []model.Group{{
		ID: "cartoon", Title: "动画片", Layout: model.LayoutList,
		Items: []model.Item{{ID: "ani", Name: "动画站", URL: "http://10.0.0.9:8888", Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/admin", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员会话该拿 200，得到 %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{"爸爸", "小满", `action="/switch"`, `value="kid"`} {
		if !strings.Contains(body, want) {
			t.Errorf("管理页缺 %q:\n%.400s", want, body)
		}
	}
	// 一行一个档案：条目内容一个都不许出现（spec §8.8）。这份名单与
	// /api/v1/profiles 那条用的是同一口径，两处都钉。
	for _, banned := range []string{"动画站", "动画片", "http://10.0.0.9:8888", "群晖", `"url"`} {
		if strings.Contains(body, banned) {
			t.Errorf("管理页漏出了条目内容 %q——它只该报数量", banned)
		}
	}
	// 跨档案页面必须 no-store：缓存住 = 把全家庭的名字与条目数留给下一个
	// 共用这台电脑的人（§7.1 的串号）。
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q，管理页要 no-store", cc)
	}
}

func TestAdmin_非管理员与无会话都303回选择页(t *testing.T) {
	_, h := setupBatch(t)
	for name, ck := range map[string]string{"非管理员会话": kidFormCookie, "没有会话": ""} {
		rec := get(h, "/admin", ck)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s：得到 %d，要 303（浏览器通道的拒不吐 403 页面）", name, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("%s：Location = %q，要 /", name, loc)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("%s：拒的一路不该带页面体: %s", name, rec.Body)
		}
	}
}

// 坏档案那一行：占一行、标出来、**不给「进入编辑」**——/switch 对读不了的
// 档案是不切的（Task 4），渲染一颗点了没反应的按钮进界面，违反本期的规则。
func TestAdmin_坏档案占一行且没有死按钮(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.BookmarksPath("kid"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/admin", dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "kid") {
		t.Errorf("坏档案整行消失了（spec §12：消失比报 0 更坏）:\n%s", body)
	}
	if !strings.Contains(body, "读不了") {
		t.Errorf("坏档案那行要说清读不了:\n%.400s", body)
	}
	if strings.Contains(body, `value="kid"`) {
		t.Error("坏档案不该有「进入编辑」——切过去就是一页读不了的台")
	}
}

// 聚合只有一份：管理页报的 items/rev 必须与 /api/v1/status 逐字一致。
// 两处各写一遍遍历的话，坏档并集这类细节迟早只在一处修。
func TestAdmin_与status同一份聚合(t *testing.T) {
	s, h := setupBatch(t)
	status := getBearer(h, "/api/v1/status", adminTok)
	var out struct {
		Profiles []struct {
			ID       string `json:"id"`
			Items    int    `json:"items"`
			Rev      int64  `json:"rev"`
			Modified string `json:"modified"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	page := get(h, "/admin", dadFormCookie).Body.String()
	for _, p := range out.Profiles {
		if p.Modified == "" {
			t.Errorf("%s 的 status 行没有 modified：页面就没法报最近改动时间", p.ID)
		}
		// 数字以页面文本出现，且与 JSON 同一个数（这里取 items）。
		if !strings.Contains(page, strconv.Itoa(p.Items)) {
			t.Errorf("status 报 %s 有 %d 条，管理页里没有这个数:\n%s", p.ID, p.Items, page)
		}
	}
}
```

`adminpage_test.go` 的 import 需要 `os`、`encoding/json`、`strconv`、`net/http`、`strings`、`testing`，加上 `model`（第一条测试自己造 kid 的书签）。既有 helper 全在这个包里，**不要再定义一遍**：`setupBatch`、`get`、`getBearer`、`cookieMap`。

- [ ] **Step 2: 跑它，确认它红**

Run: `go test ./internal/httpapi -count=1 -run Admin_`
Expected: FAIL——`/admin` 未注册（404），以及 `modified` 键不存在。

- [ ] **Step 3: 把聚合抽成 `statusRows`，并补 `Modified`**

`internal/httpapi/admin.go` 里 `statusProfile` 加字段（紧跟 `Rev`）：

```go
type statusProfile struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Admin         bool   `json:"admin"`
	Rev           int64  `json:"rev"`
	Modified      string `json:"modified"` // RFC3339 UTC；坏档/读不出来 = 空串，不编造
	Items         int    `json:"items"`
	IconUnmatched int    `json:"iconUnmatched"`
	Snapshots     int    `json:"snapshots"`
	Broken        bool   `json:"broken"`
}
```

`handleStatus` 的函数体换成"只负责鉴权与写响应"，遍历整段搬进 `statusRows`：

```go
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	rows, broken, tot, err := s.statusRows()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Profiles: rows, BrokenProfiles: broken, Totals: tot})
}

// statusRows 是"一行一个档案"这份聚合的唯一实现（spec §12）：
// GET /api/v1/status 的 JSON 与 GET /admin 的表格都从它出来。分两处各写一遍
// 遍历，"ListProfiles ∪ Broken()"这个并集迟早只在一处修——而漏掉的那一处
// 表现为某个坏档案整行消失，正是 §12 点名的形状。
//
// 它不写响应：错误一律往上抛，由调用方决定是 500 还是页面上那句人话。
func (s *Server) statusRows() ([]statusProfile, []string, statusTotals, error) {
	profiles, err := s.deps.Store.ListProfiles()
	if err != nil {
		return nil, nil, statusTotals{}, err
	}
	byID := make(map[string]model.Profile, len(profiles))
	ids := make([]string, 0, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = *p
		ids = append(ids, p.ID)
	}
	for _, id := range s.deps.Store.Broken() {
		if _, listed := byID[id]; !listed {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	out := make([]statusProfile, 0, len(ids))
	broken := []string{}
	tot := statusTotals{Profiles: len(ids)}
	for _, id := range ids {
		p, listed := byID[id]
		row := statusProfile{ID: id, Title: p.Title, Admin: p.Admin}
		if listed {
			snap, err := s.deps.Store.Current(id)
			switch {
			case err == nil:
				row.Rev = snap.Rev
				row.Modified = snap.Modified.UTC().Format(time.RFC3339)
				for _, g := range snap.B.Groups {
					row.Items += len(g.Items)
				}
				row.IconUnmatched = countIconUnmatched(snap.B, s.deps.Icons, s.deps.Cache)
			default:
				// 坏档案仍然占一行，rev/items/modified 留 0/空：消失比报 0 更坏。
				if _, isBroken := s.deps.Store.BrokenReason(id); isBroken {
					row.Broken = true
				} else if !errors.Is(err, store.ErrNotFound) {
					return nil, nil, statusTotals{}, err
				}
			}
		} else {
			row.Broken = true
		}
		if row.Broken {
			broken = append(broken, id)
		}
		row.Snapshots = s.deps.Store.SnapshotCount(id)
		tot.Items += row.Items
		tot.IconUnmatched += row.IconUnmatched
		tot.Snapshots += row.Snapshots
		out = append(out, row)
	}
	return out, broken, tot, nil
}
```

`admin.go` 的 import 补 `time`。**搬动时逐行对照原来那段**：`handleStatus` 里原来那句"ListProfiles 自身会调 LoadProfile 触发登记，所以这一步一定看到最新状态"的注释跟着搬进 `statusRows`（它说的是并集为什么成立，不是实现细节）。

- [ ] **Step 4: 渲染侧加 `AdminPage` 与 `admin.tmpl`**

`internal/render/render.go`：

```go
// AdminRow 是管理页一行。它是 httpapi statusProfile 的**视图形状**：字段一一
// 对应，映射在 adminpage.go 一次做完。不让 render 直接吃 statusProfile：那个
// struct 带的是 API 的 JSON 标签，模板不该依赖 API 契约（契约一改，页面就跟着
// 漂，而这两件事本不必同时发生）。
type AdminRow struct {
	ID       string
	Title    string
	Admin    bool
	Rev      int64
	Modified string
	Items    int
	Snapshots int
	Broken   bool
}

// AdminPage 渲染 §8.8 的管理页。NoStore 是硬要求：这一页跨所有档案，缓存起来
// 等于把全家庭的名字与条目数留在同一台电脑的下一个人手里（§7.1）。
// 不缓存也不给 ETag——writePage 在 NoStore 那一支本来就不发 ETag。
func (r *Renderer) AdminPage(rows []AdminRow) (Page, error) {
	body, err := r.tmpl.admin(rows)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, ETag: makeETag("admin", body), Profile: "admin", NoStore: true}, nil
}
```

`internal/render/exec.go`：`executor` 多一个 `adminT *template.Template`，`newExecutor` 里 `adminT: parse("admin.tmpl")`，并加：

```go
type adminView struct {
	Rows []AdminRow
	// Theme 恒空串：这一页不属于任何档案，没有 profile.json 可读（同 pickerView）。
	Theme string
	CSS   template.CSS
	Count int
}

func (e *executor) admin(rows []AdminRow) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.adminT.ExecuteTemplate(&buf, "admin", adminView{Rows: rows, CSS: e.cssT, Count: len(rows)}); err != nil {
		return nil, fmt.Errorf("渲染管理页: %w", err)
	}
	return buf.Bytes(), nil
}
```

`internal/render/web/admin.tmpl`（整份新文件，逐字照抄）：

```gotemplate
{{define "admin"}}<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>管理</title>
<style>{{.CSS}}</style>
</head>
<body>
<header>
  <h1>管理</h1>
  <nav aria-label="返回"><a class="badge" href="/">回首页</a></nav>
</header>
<main role="main">
  {{/* 一行一个档案 + 条目数 + rev + 最近改动时间（spec §8.8）。这一页的存在
       本身就是敏感信息，所以它 no-store、非管理员 303，且**一个条目内容都不带**：
       只有数量。*/}}
  <p class="hits">共 {{.Count}} 个档案。这一页只报数量与时间，不含任何书签内容。</p>
  <table class="adm">
    <thead>
      <tr><th>档案</th><th>条目</th><th>rev</th><th>最近改动</th><th>快照</th><th>动作</th></tr>
    </thead>
    <tbody>
    {{range .Rows}}
      <tr>
        <td><b>{{if .Title}}{{.Title}}{{else}}{{.ID}}{{end}}</b>
          {{if .Admin}}<span class="badge">管理</span>{{end}}
          {{if .Broken}}<span class="badge">数据文件读不了</span>{{end}}</td>
        <td>{{if .Broken}}—{{else}}{{.Items}}{{end}}</td>
        <td>{{if .Broken}}—{{else}}{{.Rev}}{{end}}</td>
        <td>{{if .Modified}}{{.Modified}}{{else}}—{{end}}</td>
        <td>{{.Snapshots}}</td>
        <td>
          {{if .Broken}}
            {{/* 点了没反应的东西不进界面：/switch 对读不了的档案不切（Task 4），
                 所以坏档这一格是文字，不是按钮。设 PIN / 建档 / 删档三个动作
                 要发 JSON，表单发不出，它们随 P1-⑤ 的 JS 落地——这一版**不渲染**。 */}}
            <span class="plain">先进那个档案修数据文件</span>
          {{else}}
            <form method="post" action="/switch">
              <input type="hidden" name="to" value="{{.ID}}">
              <button type="submit">进入编辑</button>
            </form>
          {{end}}
        </td>
      </tr>
    {{end}}
    </tbody>
  </table>
</main>
</body>
</html>{{end}}
```

`base.css` 追加（管理页是唯一用表格的页面，样式就地给全）：

```css
/* 管理页表格（spec §8.8）。表头随档案数不动，所以不 sticky；
   forced-colors 下靠 border 分行，颜色不是唯一载体（§7.5）。 */
.adm{width:100%;border-collapse:collapse;font-size:14px}
.adm th{text-align:left;font-weight:600;color:var(--ink2);padding:6px 8px;
  border-bottom:1px solid var(--ring);font-size:12px;letter-spacing:.06em;text-transform:uppercase}
.adm td{padding:8px;border-bottom:1px solid var(--ring);vertical-align:middle}
.adm button{font:inherit;cursor:pointer;min-height:44px;padding:6px 12px;
  border:1px solid var(--ring);border-radius:10px;background:transparent;color:var(--ink)}
@media (forced-colors:active){.adm th,.adm td{border:1px solid CanvasText}}
```

- [ ] **Step 5: `adminpage.go` 与路由注册**

```go
// internal/httpapi/adminpage.go
package httpapi

import (
	"net/http"

	"github.com/rockbenben/launchbay/internal/render"
)

// handleAdminPage 是 §8.8 的管理页，④ 只做读侧。
//
// 权限判据还是 require(credAdmin)——判据只有一份。本期改的是**拒的长相**：
// 浏览器通路的权限拒绝 = 303 回选择页、不报错（spec §6.1）。给家属吐一段
// 403 是坏体验，而"你在不在管理员名单里"本身是可枚举信息。
//
// 管理员令牌打它会拿到同一张 HTML 页面（credAdmin 认它）。这不是第二条 API
// 面：要聚合数据请打 GET /api/v1/status，同一个 statusRows、两种壳。AGENTS.md
// 的说明列写"AI 不走这条"。
func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		redirectPicker(w)
		return
	}
	rows, _, _, err := s.statusRows()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	views := make([]render.AdminRow, 0, len(rows))
	for _, row := range rows {
		views = append(views, render.AdminRow{
			ID: row.ID, Title: row.Title, Admin: row.Admin, Rev: row.Rev,
			Modified: row.Modified, Items: row.Items, Snapshots: row.Snapshots,
			Broken: row.Broken,
		})
	}
	page, err := s.deps.Render.AdminPage(views)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}
```

`routes.go`：

```go
		{Method: "GET", Pattern: "/admin", Cred: credAdmin,
			Summary: "管理页（读侧：档案一览与进入编辑，不含条目内容）", H: s.handleAdminPage},
```

`openapi.json` 加 `"/admin"`：`get.summary` 逐字等于上面的 Summary、`x-credential: "admin"`、`security: [{"bearerAuth": []}, {"sessionCookie": []}]`、`description` 写明"浏览器通路：非管理员一律 303 回选择页，不报错；页面只有数量与时间，无条目内容；要机器可读的同一份聚合请打 GET /api/v1/status"、`responses` 给 `200`（`text/html`）与 `303`。同时把 `/api/v1/status` 那条的 `profiles[]` schema 补上 `"modified": {"type": "string", "description": "RFC3339 UTC；坏档案读不出来时为空串，不编造"}`。

`AGENTS.md` 端点表加一行：

```
| GET | `/admin` | 仅管理员（会话或 `*` 令牌） | 200 HTML | 管理页读侧：一行一个档案 + 条目数 + rev + 最近改动，**不含条目内容**；非管理员 303 回选择页不报错；**AI 不走这条**，要同一份聚合打 `GET /api/v1/status` |
```

并在"浏览器通路"那句说明里补：`/admin`、`/prefs`、`/switch` 是表单/导航通路，权限类拒绝都是 303 回选择页，与 API 通路的 401/403 是两种长相（spec §6.1）。

- [ ] **Step 6: 管理徽章变成真链接**

`profile.tmpl` 的 `<nav aria-label="档案">` 里那行管理员徽章换成锚点（路由此刻真的存在了，才有资格发链接）：

```gotemplate
    {{if .Profile.Admin}}<a class="badge" href="/admin">管理</a>{{end}}
```

`views_test.go#TestAdminEntryOnlyForAdmin` 是**死链钉**，现在要翻面：它原来断言 `href="/admin"` 不许出现（那时路由不存在）。换成按形状钉正反两条，并把 `href="/switch"` 那条**保留**——`/switch` 只收 POST 表单，页面永远不许拿 GET 链接去点它：

```go
	if !strings.Contains(string(dadPage.Body), `href="/admin"`) {
		t.Error("管理员要能在页面上走到管理页——路由已经存在")
	}
	if strings.Contains(string(momPage.Body), `href="/admin"`) {
		t.Error("非管理员不该看见管理入口（看见了也是 303，但那是一颗点了没反应的按钮）")
	}
	for _, body := range []string{string(dadPage.Body), string(momPage.Body)} {
		if strings.Contains(body, `href="/switch"`) {
			t.Fatal("/switch 只收 POST 表单，不许发 GET 链接")
		}
	}
```

- [ ] **Step 7: 跑测试确认绿**

Run: `go test ./internal/httpapi/ ./internal/render -count=1`
Expected: 全绿，含五条契约闸（`/admin` 已进 openapi 与 AGENTS.md）。`/api/v1/status` 的既有测试解码用的是 struct 子集，新增键不会让它们红——**但 `TestStatus_缺图数读时算` 之外如果有人按整键集合断言，就在这里一并改，不要放宽闸**。

- [ ] **Step 8: 四次【故意破坏】**

1. `statusRows` 里去掉 `for _, id := range s.deps.Store.Broken()` 那一段 → `TestAdmin_坏档案占一行且没有死按钮` 与既有的 `TestStatusKeepsRowForCorruptProfileJSON` **同时**红：这就是"聚合只有一份"的证人。
2. 模板去掉 `{{if .Broken}}` 那一支 → 坏档行的 `value="kid"` 断言红（死按钮进了界面）。
3. `handleAdminPage` 的 `require(r, credAdmin, "")` 换成 `credNone` → 非管理员 303 那条红（全家庭名单泄给任何人）。
4. `AdminPage` 的 `NoStore: true` 去掉 → `Cache-Control` 断言红（跨档案页面进了缓存）。
5. 每次破坏后 `git checkout -- <文件>` 还原并复跑绿（**先 `git add` 暂存本任务已写的文件**，避免历史上发生过的 `checkout --` 抹掉未暂存改动）。

- [ ] **Step 9: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/httpapi internal/render AGENTS.md
git commit -m "feat(httpapi,render): GET /admin 读侧管理页，聚合与 /status 共用一份并补 modified"
```

---

## Task 6: 侧栏、`<details>` 抽屉与档案级 layout

**Files:**
- Modify: `internal/render/web/profile.tmpl`（侧栏 / 顶栏两态 + `grouplist` 平级 define）
- Modify: `internal/render/web/base.css`（`.shell` 栅格、抽屉、三个断点、44px 第三条家族名单）
- Modify: `internal/render/views_test.go`（三条形状钉 + `TestCSS_硬规格在场` 加四条）
- Modify: `internal/render/icon_test.go`（分组色的钉从"入口一处"改成"判据一份、挂载点两处"——侧栏色点要复用同一个 `--sec-accent`）

**Interfaces:**
- Consumes: `profileView.Profile.Layout`（`sidebar|topbar`，Task 3 的 `/prefs` 写得动它）、Task 2 的过滤结果（`Bookmarks.Groups` 已经是过滤后的那份）
- Produces: 无新导出符号；模板里多一个平级 `{{define "grouplist"}}`（Go 不许嵌套 define，必须与 `"profile"` 平级——与既有 `"icon"` 同一条约束）

- [ ] **Step 1: 写失败测试**

```go
// internal/render/views_test.go 追加
//
// 侧栏的判据：锚点必须指向 section 已有的 id（一个分组一个），
// 计数必须随命中走——"5 个分组"跟屏幕上实际渲染的内容对不上就是一句谎。
func TestProfilePage_侧栏锚点与计数(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Title = "爸爸的台"
	b.Groups = []model.Group{
		{ID: "nas", Title: "NAS", Layout: model.LayoutList, Items: []model.Item{
			{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
			{ID: "qb", Name: "qBittorrent", URL: "http://10.0.0.3:8181", Alias: []string{}, Tags: []string{}},
		}},
		{ID: "read", Title: "读", Layout: model.LayoutList, Items: []model.Item{
			{ID: "wr", Name: "微信读书", URL: "https://weread.qq.com", Alias: []string{}, Tags: []string{}},
		}},
	}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	all, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(all.Body)
	for _, want := range []string{`<aside`, `href="#g-nas"`, `href="#g-read"`, `>2<`, `>1<`} {
		if !strings.Contains(body, want) {
			t.Errorf("侧栏缺 %q:\n%.400s", want, body)
		}
	}
	// 带 q 时：计数按命中数，且没命中的分组整条从侧栏消失。
	hit, err := r.ProfilePage(PageRequest{ID: "dad", Query: "群晖"})
	if err != nil {
		t.Fatal(err)
	}
	hb := string(hit.Body)
	if !strings.Contains(hb, `href="#g-nas"`) || !strings.Contains(hb, ">1<") {
		t.Errorf("搜索态侧栏计数没按命中走:\n%.400s", hb)
	}
	if strings.Contains(hb, `href="#g-read"`) {
		t.Error("一条都没命中的分组还挂在侧栏上——点了跳到空处")
	}
}

// Profile.layout 是档案级形态（spec §8.2/§8.3）：topbar 时分组行常驻顶栏下方，
// **不渲染侧栏**。两个形态都由同一份数据长出来，靠的是 profile.json 那一个字段。
func TestProfilePage_topbar形态没有侧栏(t *testing.T) {
	s := seeded(t)
	if err := s.SaveBookmarks("dad", func() *model.Bookmarks {
		b := store.EmptyBookmarks()
		b.Rev = 1
		b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList,
			Items: []model.Item{{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}}}}}
		return b
	}()); err != nil {
		t.Fatal(err)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	p.Layout = "topbar"
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	body := string(mustRender(t, s, "dad"))
	if strings.Contains(body, "<aside") {
		t.Errorf("layout=topbar 还渲染侧栏:\n%.400s", body)
	}
	if !strings.Contains(body, `href="#g-nas"`) {
		t.Error("topbar 形态的分组行必须在顶栏下方——锚点一个都不能少")
	}
	// 形态属性要发出去，CSS 与实拍才认得这一页是哪种形态（同 data-layout 的先例）。
	if !strings.Contains(body, `data-profile-layout="topbar"`) {
		t.Error("body 要发 data-profile-layout")
	}
}

// 抽屉用原生 <details>：键盘开合与可及性免费，不值得为它写 JS 或加 checkbox 黑话。
func TestProfilePage_侧栏抽屉是原生details(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	body := string(mustRender(t, s, "dad"))
	if !strings.Contains(body, "<details") || !strings.Contains(body, "<summary>") {
		t.Errorf("侧栏抽屉必须是 <details>/<summary>:\n%.400s", body)
	}
	if strings.Contains(body, "<script") {
		t.Error("抽屉不许带脚本来开合")
	}
}
```

`TestCSS_硬规格在场` 的 `wants` map 追加六条（照既有风格：钉逐字 CSS 片段）：

```go
		"侧栏 190px 常驻":  `.shell{display:grid;grid-template-columns:190px minmax(0,1fr)`,
		"平板断点 1100px": `@media (max-width:1100px)`,
		"侧栏不吃磁贴栅格":  `.gnav ul{display:block`,
		"抽屉把手有开合标记": `.drawer>summary::before{content:"▸"`,
		"抽屉把手的展开态":  `.drawer[open]>summary::before{content:"▾"`,
		"侧栏色点画得出来":  `.gnav .dot{flex:0 0 auto;width:10px;height:10px;border-radius:50%`,
```

后两条是评审 N-2 补的：只钉"闭合"那一半时，删掉 `[open]` 那条会让抽屉永远指着
"收起"的方向；色点若没有 CSS 消费者，模板发的那个 `style="--sec-accent:…"` 就是个
隐形格子——生产者证人（`icon_test.go`）与消费者证人（这里）必须成对，缺任一半都测不到。

评审后补的四条（都在 Step 1 那三条旁边，形状与理由一并落地）：

1. **计数锚整行**：`>2<`/`>1<` 只证"页面里有这两个裸数字"，两组计数互换测不出来。
   改成钉整条形状：`href="#g-nas"><span>NAS</span>` 之外再要求 `<b class="cnt">2</b>`
   （色点在前的整行见下一条），并把"裸数字只出现在 .cnt"这一前提写进注释。
2. **色点三件套**：夹具里一个分组有合法 `accent`、一个没有——没有的那个**不许**出现
   `<i class="dot"`，有的那个出现且带 `style="--sec-accent:`。这是设计 §7 那句
   "分组 + 计数 + 色点"唯一的证人。
3. **sidebar 档正向钉**：`data-profile-layout="sidebar"` 也要断言（topbar 测试只钉了
   topbar 那一格），默认形态被改动时不至于只靠 `<aside` 兜。
4. **地标只有一个名字**：`aria-label="分组"` 在整页出现**恰好一次**（sidebar 档给 nav，
   topbar 档也给 nav），`aside` 不带标签。数出现次数比数属性写法更便宜，也更能咬住
   "嵌套同名地标"这种形状。

- [ ] **Step 2: 跑它，确认它红**

Run: `go test ./internal/render -count=1 -run '侧栏|topbar形态'`
Expected: FAIL——模板里根本没有侧栏。

- [ ] **Step 3: 模板长出侧栏**

`profile.tmpl`：`<body>` 开标签换成带形态属性，然后 `<main>` 包进 `.shell`。整段形状（`{{/* */}}` 注释照抄，它们是判据）：

```gotemplate
<body data-profile-layout="{{.Profile.Layout}}">
<header>…Task 2/3 的搜索框 + 主题表单 + 档案徽章…</header>
{{/* 横幅的位置由设计 §5 钉死：`<header>` 之后、`<main>` 之前。
     形状整段搬过来，一个字不改——它连"顶栏下一条"都是判据的一部分。 */}}
{{with .Manage}}…横幅原样不动…{{end}}
{{$top := eq .Profile.Layout "topbar"}}
{{/* 形态判一次、两处用：topbar 那条分组行在 .shell **外面**（它要横跨整幅），
     侧栏在 .shell **里面**（它占栅格第一列），所以这里没法写成 if/else。
     绑成一个变量就是"判据只有一份"的形状——写成两个独立的 eq/ne 条件等于把
     同一个判断抄两遍，将来加第三种形态时其中一处会静默失效。 */}}
{{if $top}}
  {{/* 顶栏形态：分组行常驻顶栏下方。同一份 grouplist，两种挂法——
       判据（分组、计数、色点、锚点）只有一份。 */}}
  <nav class="gnav top" aria-label="分组">{{template "grouplist" .}}</nav>
{{end}}
<div class="shell">
{{if not $top}}
  {{/* aside 不带 aria-label：名字只给里面的 nav，一页一个具名导航地标。
       open 写死：桌面要它展开，平板断点下 summary 才现身当抽屉把手；
       键盘开合与可及性由原生 <details> 负责，零脚本，不补 role/aria-expanded 那类仪式。 */}}
  <aside class="side">
    <details class="drawer" open>
      <summary>分组</summary>
      <nav class="gnav" aria-label="分组">{{template "grouplist" .}}</nav>
    </details>
  </aside>
{{end}}
<main role="main">
{{if .Query}}
  {{if not .Bookmarks.Groups}}<p>没有匹配「{{.Query}}」的书签。<a href="/">看全部</a></p>{{end}}
{{else if not .Bookmarks.Groups}}
  <p>这个档案还没有书签。</p>
{{end}}
{{range .Bookmarks.Groups}}…原有 section 整段不动…{{end}}
</main>
</div>
```

文件末尾与 `"icon"` 平级加（**不许嵌进 `"profile"` 里面**）：

```gotemplate
{{/* 分组导航：锚点复用 section 已有的 id="g-<id>"（§8 的 section 本来就带
     aria-labelledby，两边指同一个东西）。计数取的是当前渲染的那份数据，
     所以带 q 时它天然是命中数——不需要第二个数。
     三件套（分组名 + 计数 + 色点）是设计 §7 与权威源 §8.2 一起列的，少一件
     就是少一件。色点走同一个 --sec-accent 属性名：判色仍只有 validatedAccent
     一份，只是它现在有两个挂载点（section 与它的侧栏行），条目身上照旧一个都不许有。
     没有 accent 的分组不发这一格——`{{with}}` 就是"没设色就不画点"。 */}}
{{define "grouplist"}}
<ul>
{{range .Bookmarks.Groups}}
  <li><a href="#g-{{.ID}}">{{with .Accent}}<i class="dot" style="--sec-accent:{{.}}" aria-hidden="true"></i>{{end}}<span>{{.Title}}</span><b class="cnt">{{len .Items}}</b></a></li>
{{end}}
</ul>
{{end}}
```

- [ ] **Step 4: CSS 三个断点**

```css
/* 外壳栅格（spec §8.2）：桌面 190px 常驻侧栏；≤1100px 收成抽屉（summary 才是
   把手，桌面把 summary 藏掉）；≤700px 变顶部横向标签行。
   注意：base.css 有一条全局 `ul{display:grid;grid-template-columns:auto-fill…}`
   是给磁贴用的，它一样会打到侧栏——所以 .gnav ul 必须显式覆盖 display，
   否则侧栏会变成一堆 190px 宽的格子（这条是实测过的坑，不是假设）。 */
.shell{display:grid;grid-template-columns:190px minmax(0,1fr);gap:24px;align-items:start;
  max-width:1180px;margin:0 auto}
.shell>main{max-width:none;margin:0}
.side{position:sticky;top:12px;align-self:start;min-width:0}
/* 代管态有两个 sticky：横幅钉在 top:0（§8.9），侧栏也 sticky 就有一半压在它底下。
   66px 是横幅那一行的实际高度（上下各 10 的内距 + 44 的按钮 + 描边），侧栏跟着让开。
   :has() 让这件事只在"横幅真的在场"时生效；不支持的浏览器退回 top:12px——压一点，
   不是坏页面。真实读数按 Task 9 的 1440 档代管态那张量。 */
body:has(.banner) .side{top:66px}
.drawer{border:1px solid var(--ring);border-radius:14px;padding:6px 10px;background:var(--card-hi)}
.drawer>summary{cursor:pointer;display:flex;align-items:center;
  font-size:13px;color:var(--ink2)}
/* display:flex 会杀掉 summary 的 UA 展开三角（三角只在 list-item 显示，Blink/Gecko 都这样），
   而抽屉形态唯一的把手就只剩 cursor:pointer 一个暗示——家属看不出那行能点。自己画回来，零脚本。 */
.drawer>summary::before{content:"▸";margin-right:6px}
.drawer[open]>summary::before{content:"▾"}
.gnav ul{display:block;list-style:none;margin:0;padding:0}
.gnav a{display:flex;gap:8px;align-items:center;padding:0 6px;
  border-radius:8px;color:var(--ink);text-decoration:none;font-size:14px}
.gnav a:hover{background:var(--card-hi)}
.gnav .cnt{margin-left:auto;font-weight:400;font-size:12px;color:var(--ink2)}
/* 侧栏是分组名的第三张脸（前两张：卡片、磁贴）。190px 定轨配 CJK 换行会把
   一行一组撑成两三行，配无空格长拉丁词会溢出到 24px 的 gap 里压住主区。 */
.gnav a span{min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.gnav .dot{flex:0 0 auto;width:10px;height:10px;border-radius:50%;
  background:var(--sec-accent,var(--accent))}
[data-profile-layout=topbar] .shell{grid-template-columns:minmax(0,1fr)}
.gnav.top{margin:0 0 14px}
.gnav.top ul{display:flex;gap:6px;overflow-x:auto}
.gnav.top li{flex:0 0 auto}
@media (min-width:1101px){.drawer{border:0;padding:0;background:none}.drawer>summary{display:none}}
@media (max-width:1100px){.shell{grid-template-columns:minmax(0,1fr)}.side{position:static}}
@media (max-width:700px){
  .gnav:not(.top) ul{display:flex;gap:6px;overflow-x:auto}
  .gnav:not(.top) li{flex:0 0 auto}
  .gnav .cnt{display:none}
}
@media (forced-colors:active){.drawer{border:1px solid CanvasText}.gnav a{border:1px solid CanvasText}}
```

44px 触摸目标（§7.5）在这里新增**一族**：侧栏链接与抽屉把手都不是 `<button>`，也不在 input
那条名单上。**给这两个元素各写一条 `min-height:44px` 是错的**——本案的口径是"同一个硬数字
一族写一次"（input 族、按钮族各一条 joint 规则），所以这两个选择器合起来开**第三条**名单：

```css
.gnav a,.drawer>summary{min-height:44px}
```

`TestCSS_搜索控件的44px由哪条规则保证` 的 want 名单跟着加 `.gnav a` 与抽屉把手（Task 7 把 `.drawer>summary` 收宽成 `summary`，名单同步），
base.css 上那段"只有按钮家族"的判据注释改成事实（三族，每族一次）。

`<aside>` 不带 `aria-label`：名字只留给里面那个 `<nav aria-label="分组">`，一页一个具名导航
地标；`<details>` 自带开合状态，不补 role/aria-expanded 那类仪式（§11 反对多出来的件）。

`header{…}` 那一行（现为 `display:flex`）在 Task 2 已改成栅格；本任务只加 `.shell`，**不要再复制一条 `header{}`**（同名选择器两条并存等于靠后者覆盖靠前者）。

- [ ] **Step 5: 跑测试确认绿**

Run: `go test ./internal/render -count=1`
Expected: 全绿。`TestProfileStructureAndNoJS` 与 `TestProfilePage_两种layout都落地` 不许改——它们钉的是 section 形状，侧栏加进去不影响。若 `TestCSS_硬规格在场` 因为 `.shell` 那条文本不完全一致而红，**改 CSS 去迁就钉**（钉是 §7.5/§8.2 的落地形状），不要反过来改钉里的字符串。

- [ ] **Step 6: 六次【故意破坏】**

1. 把 `.gnav ul{display:block…}` 那行删掉 → CSS 不红、页面红不了（形状钉没盖这条），**但浏览器里侧栏会变成 190px 格子**：这一条就是 Task 9 实拍要盯的点，Step 6 里顺手量一次 `ProfilePage` 的 HTML 里 `.gnav` 的宽度判据不在单测能力范围内，所以改为——把 `.gnav ul{display:block` 补进 `TestCSS_硬规格在场` 的 map（第三条），再删 CSS 那行验证它红。
2. 侧栏计数改成 `{{$.Total}}`（`profileView.Total` = 档案总条数，不随过滤变；注意 `model.Bookmarks` **没有** `Total` 字段，写成 `.Total` 是编译失败不是红测试）→ 搜索态 `>1<` 断言红（`>3<`）。
3. 把 `{{$top := eq .Profile.Layout "topbar"}}` 改成常量 `false` → `topbar形态没有侧栏` 红（两处分支同时倒，正是"判据只有一份"的代价面）。
4. 去掉 `<details>` 只留 `<nav>` → 抽屉断言红。
5. 侧栏色点整格删掉（`{{with .Accent}}…{{end}}` 那一小段）→ 三件套的色点证人红；同时 `icon_test.go` 那条"属性形状恰好 1 次"的钉会从 1 变 2 —— **这条钉要跟着改成事实**（每个有合法 accent 的分组两处：section 与它的侧栏行；条目身上照旧 0 处），别把它"简化"回裸 token 计数（那会在 CSS 重写这天假红，注释里写着）。
6. 删掉 `.drawer>summary::before` 那两行 → 抽屉把手的开合指示证人红。这条证人只能是 CSS 文本钉（浏览器才知道三角长什么样），单测能力之外。

- [ ] **Step 7: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/render
git commit -m "feat(render): 侧栏分组导航与档案级 layout 两态，抽屉用原生 details"
```

---

## Task 7: 坏档页的两层文案（人话主句 + 可展开的原文）

**Files:**
- Modify: `internal/render/web/broken.tmpl`（整份重写）
- Modify: `internal/render/web/base.css`（`<details>`/`<pre>` 两条）
- Test: `internal/render/views_test.go`（主句 / 包含关系 / 逐字同源三条形状钉，另加把手一族的判别式名单钉与 `.tech` 类名钉）、`internal/httpapi/handlers_test.go`（原文与 503 detail 同源**两条**：截断类走直通分支、校验类走登记表回退分支）

**Interfaces:**
- Consumes: `Renderer.BrokenPage(id, reason)`（签名不变）、`Store.BrokenReason` 与 store 的错误文本（**一个字都不改**）
- Produces: 无新导出符号

判据（spec §12）：主句是人话，原文进 `<details>` 展开可见，文件名与行号不许丢，`store` 的错误文本不许为了一张页面被改写——它是 API 503 的 `detail`，两处共用同一份真相。

- [ ] **Step 1: 写失败测试**

```go
// internal/render/views_test.go 追加
//
// 两层文案的形状钉。<h1> 里不许出现文件名：那是给家属看的第一句，
// "bookmarks.json 第 3 行附近"不是人话。原文必须在 <details> 里可见：
// 修文件的人就是家长本人，行号是唯一能定位的东西。
func TestBrokenPage_主句是人话原文在details里(t *testing.T) {
	s := seeded(t)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{\n \"rev\": 2,\n")); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	if _, err := r.ProfilePage(PageRequest{ID: "dad"}); err == nil {
		t.Fatal("坏档案该让 ProfilePage 报错")
	}
	reason, ok := s.BrokenReason("dad")
	if !ok || reason == "" {
		t.Fatalf("坏档登记表里没有原因: %v", ok)
	}
	page, err := r.BrokenPage("dad", reason)
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	start := strings.Index(body, "<h1>")
	end := strings.Index(body, "</h1>")
	if start < 0 || end < start {
		t.Fatalf("解释页要有 <h1> 主句:\n%.300s", body)
	}
	h1 := body[start:end]
	if strings.Contains(h1, ".json") || strings.Contains(h1, "第 ") {
		t.Errorf("主句里混进了技术细节（文件名/行号该收进 details）:\n%s", h1)
	}
	if !strings.Contains(h1, "这个档案的数据文件读不了") {
		t.Errorf("主句要是那句人话:\n%s", h1)
	}
	if !strings.Contains(body, `<details class="tech"`) || !strings.Contains(body, "<summary>") {
		t.Error("原文要收进带 .tech 类的 <details>：裸元素写法会把形状漏给侧栏抽屉")
	}
	// 包含关系钉在"原文就在这只块里"：只证串在场的两条互不认识，"块空着、原文印在
	// 别处"能静悄悄进页面；按整页偏移量比又会两头错——块外多一只合法 pre 假红，块里
	// 留个空 pre 而原文在块外假绿。所以先把窗口锚到那只块自己（开标签到它的闭标签）。
	d := strings.Index(body, `<details class="tech"`)
	e := strings.Index(body[d:], "</details>")
	if d < 0 || e < 0 {
		t.Fatalf("那只 .tech 块得是闭合的: details=%d /details=%d", d, e)
	}
	seg := body[d : d+e]
	escaped := string(template.HTMLEscapeString(reason))
	if !strings.Contains(seg, "<pre") || !strings.Contains(seg, escaped) {
		t.Errorf("store 的原文必须折叠在 .tech 的 details 里、且就在 pre 内:\n%.400s", seg)
	}
	// 逐字：store 给的那句必须原样出现在页面上，不改写、不截断。比的是页面真正的
	// 形状——模板出去的是 HTML 转义后的字节；校验类原文带 %q 引号，裸比会假红。
	if !strings.Contains(body, escaped) {
		t.Errorf("store 的原文没原样出现:\n要: %s\n页面上:\n%.600s", reason, body)
	}
	if !strings.Contains(body, "bookmarks.json") || !strings.Contains(body, "第 ") {
		t.Error("文件名与行号不许丢（§12：报得出位置）")
	}
	if strings.Contains(body, "<script") {
		t.Error("解释页同样不许带脚本")
	}
}

// 坏档页不许把磁盘路径吐出去：原因文本用的是 filepath.Base，
// 页面这一层再拼一次路径就是把内部结构交给看页面的人。
func TestBrokenPage_没有磁盘路径(t *testing.T) {
	s := seeded(t)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{坏")); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	_, err := r.ProfilePage(PageRequest{ID: "dad"})
	reason, _ := s.BrokenReason("dad")
	page, perr := r.BrokenPage("dad", reason)
	if perr != nil {
		t.Fatal(perr)
	}
	if strings.Contains(string(page.Body), s.Root()) || strings.Contains(string(page.Body), "profiles/") {
		t.Errorf("解释页漏出磁盘路径:\n%.400s", page.Body)
	}
}
```

`Store.Root()` 是现成的（`internal/store/paths.go`），直接用；不要为了一条测试给 store 加新口子。

httpapi 侧那条（同源证人）。**503 的来源必须选真的读失败通路**：`/prefs` 读的是 `profile.json`，本夹具坏的是 `bookmarks.json`，用它拿不到 503；`GET /api/v1/data` 走的正是书签读通路（`handleData` 的 `ErrCorrupt` 那一支 → `corruptDetail`）。

```go
// internal/httpapi/handlers_test.go 追加
//
// 同一份原文两处消费：家属看的解释页与 AI 看的 503 detail。为了一张页面
// 把详情改成第二套，两处就会各说各话——这条钉住它们逐字相同。
func Test坏档原文页面与503同源(t *testing.T) {
	s, h := setup(t)
	// 截断的 JSON：错误文本是"…第 N 行附近: JSON 在值结束前就断了"，
	// 不含引号与尖括号，页面渲染与 API detail 可以直接逐字比。
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{\n \"rev\": 2,\n")); err != nil {
		t.Fatal(err)
	}
	page := get(h, "/", dadFormCookie)
	if page.Code != http.StatusOK {
		t.Fatalf("坏档案给的是解释页而不是 500，得到 %d", page.Code)
	}
	body := page.Body.String()

	rec := getBearer(h, "/api/v1/data?profile=dad", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("坏书签档案的读通路该 503，得到 %d: %s", rec.Code, rec.Body)
	}
	var errBody struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Detail == "" {
		t.Fatal("503 的 detail 是空的：§12 要求报得出文件与位置")
	}
	// 页面里那份是 HTML 转义之后的字节，比较前先按同一套转义走一遍
	// （直接比会在原文含引号/尖括号时假红；用 HTMLEscapeString 才是在比
	// 同一件事的两个副本）。
	if want := string(template.HTMLEscapeString(errBody.Detail)); !strings.Contains(body, want) {
		t.Errorf("解释页里的原文与 503 的 detail 不是同一句\n页面:\n%.400s\ndetail: %q", body, errBody.Detail)
	}
}
```

`handlers_test.go` 的 import 需要 `encoding/json`、`html/template` 与 `strings`（都有则不动）。

评审后补的三条（控制者当场落地，形状以出货代码为准）：

**一、把手一族的名字要有判别式证人**（子串钉打不过"改回抽屉专用"，因为旧串是新串的后缀）：
`TestCSS_硬规格在场` 的 map 循环之后加这一条，与 44px 那条同一口径——问的是"这条声明挂在哪个选择器上"。

```go
	if got := selectorsWithDeclaration(css, `content:"▸"`); len(got) != 1 || got[0] != "summary::before" {
		t.Errorf(`▸ 标记必须挂在裸 summary::before 上（抽屉与解释页共用一只手），实际名单 %v`, got)
	}
```

**二、`corruptDetail` 的登记表回退分支要有人执行到它**。截断类夹具走的是"本次错误自己点得出文件名"
那支（直通），回退支今天零覆盖：把它删掉，503 的 detail 立刻丢掉文件名，而解释页与 detail 各说各话。
补的那张测试用**合法 JSON、非法内容**（AGENTS.md 坑 3 那一类：`search.default` 指不到任何引擎前缀），
它经 `errors.Join` 返回的串里没有 `.json`，于是必须回退到登记表——同源判据在这条支路上才算真的被测过。

```go
// 503 文本还有第二个来源：corruptDetail 只在"本次错误自己的文本点不出文件"时
// 才回退到坏档登记表。校验类坏档正是这一支——load.go 那边返回的是 errors.Join
// 出来的串（ErrCorrupt 的文案里没有 .json），登记表那份才是包了文件名的 wrapped。
// 这一支今天零覆盖（评审 I-3）：把它改成 return msg，AI 侧的 503 立刻丢掉文件名
// （§12"报得出文件与位置"对 API 那一半失效），而解释页与 detail 各说各话——
// 同源不变量在这条支路上完全没有保护。夹具挑的是 AGENTS.md 坑 3 那一类：
// 合法 JSON、非法内容（search.default 指不到任何引擎前缀）。
func Test坏档原文同源_校验类走登记表回退(t *testing.T) {
	s, h := setup(t)
	doc := `{"rev":1,"title":"爸爸的台","groups":[],"search":{"enabled":true,"default":"x",` +
		`"engines":[{"prefix":"g","name":"Google","url":"https://www.google.com/search?q=%s"}]}}`
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte(doc)); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/data?profile=dad", dadTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("内容非法的书签档案读通路该 503，得到 %d: %s", rec.Code, rec.Body)
	}
	// 登记表是在这次读的时候才被写上的（markBroken 在 LoadBookmarks 里），所以取它
	// 要在请求之后——先钉住它点得出文件名：Join 那份点不出，这正是回退分支存在的
	// 理由，也是"以后有人统一成 return msg"会悄悄丢掉的东西。
	reason, ok := s.BrokenReason("dad")
	if !ok || reason == "" {
		t.Fatalf("校验失败要进坏档登记表（回退分支的取文处）: %v %q", ok, reason)
	}
	if !strings.Contains(reason, "bookmarks.json") {
		t.Fatalf("登记表的原因要点名文件（fixture 前提）: %q", reason)
	}
	var out struct {
		Code   int    `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Detail != reason {
		t.Errorf("回退分支要给的是登记表那句原文：detail=%q reason=%q", out.Detail, reason)
	}
	page := get(h, "/", dadFormCookie)
	if page.Code != http.StatusOK {
		t.Fatalf("同一份坏数据下，家属侧该是解释页而不是 500，得到 %d", page.Code)
	}
	if want := string(template.HTMLEscapeString(out.Detail)); !strings.Contains(page.Body.String(), want) {
		t.Errorf("校验类坏档的解释页与 503 不同源\ndetail: %q\n页面:\n%.400s", out.Detail, page.Body.String())
	}
}
```

**三、机制与测试的边界要说清**：spec §11 反对的是**运行时**多出来的件（签名 cookie、限速、CSRF），
夹具与证人不是机制。上一轮把"不加机制"扩到测试上是错的裁定，这条已按事实改掉。**这条不许改成"页面里含 `bookmarks.json` 就行"**——那等于放弃了同源判据，只留下"报得出文件名"这一半。

- [ ] **Step 2: 跑它，确认它红**

Run: `go test ./internal/render ./internal/httpapi -count=1 -run 'BrokenPage|坏档原文'`
Expected: FAIL——现在的模板把 `{{.Reason}}` 直接印在 `<p>` 里，`<details>` 那条红；主句那条此刻还是绿的（现有 `<h1>` 是人话），改完模板后两条都要绿。

- [ ] **Step 3: 重写 broken.tmpl**

整份替换（`{{define}}` 名与 `<html>` 骨架保持原样）：

```gotemplate
{{define "broken"}}<!doctype html>
<html lang="zh-CN"{{with .Theme}} data-theme="{{.}}"{{end}}>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>这个档案读不了</title>
<style>{{.CSS}}</style>
</head>
<body>
<main role="main">
  {{/* 两层文案（spec §12）：第一句是人话，细节收进 details。
       Reason 由 store 给（filepath.Base + 行号），这里不改写、不截断、
       不拼路径——同一段字节也是 API 503 的 detail，两处共用一份真相。 */}}
  <h1>这个档案的数据文件读不了。</h1>
  <p>档案 <b>{{.ProfileID}}</b> 现在打不开，<b>其它档案不受影响</b>。</p>
  <p>如果你是自己改过那个文件：把它改回读得出内容的样子，刷新一下就好。<br>
     不确定怎么改就把下面那段细节复制给帮你搭这个的人——它写清了是哪个文件、哪里不对。</p>
  <details class="tech">
    <summary>技术细节（文件名与位置）</summary>
    <pre>{{.Reason}}</pre>
  </details>
</main>
</body>
</html>{{end}}
```

`base.css` 追加：

```css
/* 解释页的细节块用**类名**，不用裸元素写法：裸 details 的 margin/padding/border
   会一并打到侧栏那个 .drawer，而两个页面的形状本来就该不同（一个贴内容、一个贴卡片）。
   块里的 pre 保原文换行，但窄屏上要能折行、不许横向滚出屏幕；forced-colors 下还看得出是个块。
   注释里别写尖括号形状串：整份 CSS 内联进每张页面，字面标签会把"页面里有这个标签"
   这类证人喂饱（views_test.go 的包含钉是锚到 .tech 块窗口里问的，不数整页）。 */
.tech{margin:14px 0;border:1px solid var(--ring);border-radius:12px;padding:6px 12px}
.tech pre{margin:0;white-space:pre-wrap;word-break:break-all;font-size:12px;color:var(--ink2)}
@media (forced-colors:active){.tech{border:1px solid CanvasText}}
```

把手这一族在 Task 6 里被写成 `.drawer>summary`（只管侧栏抽屉）。坏档页的 `<details>`
也想要同一只手：把手够大、看得见他是个能按的东西。所以这一任务把**前缀去掉**，让
`summary` 成为一族——顺带修掉一个会在新页面上复发的老 bug：`display:flex` 会杀掉 UA
的展开三角（Task 6 评审 I-3），只钉在 `.drawer` 上的 `::before` 标记罩不到解释页，
解释页的把手就会只剩 `cursor:pointer` 一个暗示。

```css
/* 把手外观与开合标记：所有 summary 共用（侧栏抽屉 + 解释页细节块）。 */
summary{cursor:pointer;display:flex;align-items:center;font-size:13px;color:var(--ink2)}
summary::before{content:"▸";margin-right:6px}
details[open]>summary::before{content:"▾"}
```

于是 Task 6 写的 `.drawer>summary{cursor:pointer;display:flex…}`、
`.drawer>summary::before{…}`、`.drawer[open]>summary::before{…}` 三条**合并改写**成上面
那三条（`.drawer>summary` 只留在"桌面把把手藏掉"那条媒体查询里——那是抽屉独有的行为，
解释页不该藏）。触摸目标的第三条家族名单跟着换名字：`.gnav a,.drawer>summary{min-height:44px}`
改成 `.gnav a,summary{min-height:44px}`（44px 仍然只在每个族里写一次）。
`TestCSS_搜索控件的44px由哪条规则保证` 的 want 名单里 `.drawer>summary` 那一项改成
`summary`；`TestCSS_硬规格在场` 里那两条把手钉的字符串改成
`summary::before{content:"▸"` 与 `details[open]>summary::before{content:"▾"`。

- [ ] **Step 4: 跑测试确认绿**

Run: `go test ./internal/render ./internal/httpapi -count=1`
Expected: 全绿。既有的 `TestBrokenProfileSurfacesReason` 测的是 store 错误带行号，不读模板，不该受影响。

- [ ] **Step 5: 五次【故意破坏】**

1. 把 `<pre>{{.Reason}}</pre>` 挪进 `<h1>`（还原成"细节糊在第一句里"）→ 主句那条红（`.json`/`第 ` 出现在 h1）。
2. 把 `reason` 在 handler 里截断成前 20 个字符 → 红的是 `Test坏档原文页面与503同源` 与既有 `TestBrokenProfileShowsReason`（都在 httpapi 侧）；**render 侧那条够不着**——它直调 `BrokenPage`，不经 handler。曾写过"→ `store 的原文没原样出现` 红"，那是错的预测，照它复跑的人会以为证人失灵。
3. 删掉 `<details>` 只留 `<p>{{.Reason}}</p>` → `details` 形状钉红。
4. 在模板里把 `s.Root()` 那类路径拼进 `Reason` 前面（模拟"顺手多写点上下文"）→ `TestBrokenPage_没有磁盘路径` 红。
5. 把手的合并判据要有人守着它"是两个页面共用"：把 `summary::before{content:"▸"` 那行删掉 →
   `TestCSS_硬规格在场` 的"抽屉把手有开合标记"红。这一条同时是侧栏与解释页的证人——
   只钉 `.drawer` 时解释页没人管，这就是本任务把前缀去掉的理由。

- [ ] **Step 6: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
git add internal/render internal/httpapi
git commit -m "feat(render,httpapi): 坏档页两层文案——人话主句在前，原文收进 details"
```

---

## Task 8: §7.3 预算闸 + 仓库第一条 CI + 泄漏守卫

**Files:**
- Create: `internal/render/budget_test.go`、`tools/leakguard/main.go`、`tools/leakguard/main_test.go`、`.github/workflows/ci.yml`
- Modify（Task 7 复审留下的四条证人，随本任务一起收，因为本任务才让"形状与尺寸"变成机器可查的）:
  `internal/render/views_test.go`（`.tech pre` 的换行规则、forced-colors 下那条 `.tech` 边框、`▾` 的选择器名单）、
  `internal/httpapi/handlers_test.go`（回退分支那张测试的自己前提）、
  `internal/render/web/base.css`（按点名跳过的判据不需要动；这一项只在需要改注释时才出现）
- 登记不改（R-7）：`internal/httpapi/handlers.go#corruptDetail` 最后那条 `return msg` 仍 0 覆盖——
  为它造夹具是加机制，不值得；留给下一次真正需要这条支路的改动。
- Modify: `.dockerignore`（加一行 `.github`，与既有 `tools/` 同判据：CI 件不进镜像）

**Interfaces:**
- Consumes: `Renderer.ProfilePage`、`assets(t)`、`icons.Index.Names(prefix)`
- Produces: `go test ./internal/render -run Budget` 三条 + `go run ./tools/leakguard`（0 处泄漏时退出码 0，发现泄漏 1，守卫自己没跑成 2）

- [ ] **Step 1: 先写预算闸，并把实测数写进注释**

```go
// internal/render/budget_test.go
package render

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 体积预算闸（spec §7.3）。**字节类进 go test，不加 gzip 中间件**：局域网里
// 那点字节换不回一次往返的时间，而中间件会让每个响应都要重新想一遍
// Content-Encoding 与 ETag 怎么相处。这里量的是渲染产物的 gzip 尺寸，
// 与浏览器实际收到的字节同源。
//
// 线是 2026-10-06 在本机实测分布上画的（25 条 = 当前真实规模，图标全内联）。
// 下面三个读数是 Task 6（侧栏 + 色点）与 Task 7（解释页文案）落地之后在 HEAD
// 重新量的；画线那一天记的是另一组数（25 条 gzip 8,814 B、200 条 21,032 B、CSS
// 12,239 B），外壳长了 40% 以上，旧数留在注释里只会让人拿它当今天的余量：
//
//	 25 条  raw  36,728 B / gzip 12,575 B
//	200 条  raw 141,153 B / gzip 25,671 B
//	内联 CSS raw  20,089 B / gzip  9,137 B
//
// 三条操作线**都没有跟着读数往上调**：线是判据，不是读数的函数，一次改动把线
// 也顺手推高 = 这次改动自己给自己发通行证。
//
// 两层线一起钉：契约天花板（§7.3 写死的 120 KB / 40 KB）与**操作线**。只钉天花板
// 的那条在本案规模上差着 5 倍，永远跨不过去，而一条永远绿的闸是假闸——假闸比没闸
// 贵（§7.3 关于 headless 默认值那段就是这个教训）。操作线红了要看注释里的实测数，
// 别顺手把数往上调到刚过。
const (
	budgetHTMLContract   = 120_000 // spec §7.3：首屏 HTML gzip ≤ 120KB
	budget25Operational  = 16_000  // HEAD 实测 gzip 12,575 B 的 1.3 倍
	budget200Operational = 48_000  // HEAD 实测 gzip 25,671 B 的 1.9 倍
	budgetCSSContract    = 40_000  // spec §7.3：CSS 内联 ≤ 40KB
	// HEAD 实测 20,089 B 的 1.2 倍——三条里最薄的一条，而且是有意留薄的：CSS 每加
	// 一族外壳就多几百字节，余量掉到 1 倍以下那天要做的决定是"这 4 KB 花在哪个特性
	// 上、值不值"，不是"把线抬到刚过"。要动这条线必须连上面那段读数一起重写。
	budgetCSSOperational = 24_000
)

func gzipSize(b []byte) int {
	var z bytes.Buffer
	w := gzip.NewWriter(&z)
	if _, err := w.Write(b); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return z.Len()
}

// realistic 造 n 条书签，摊在 4 个分组里，每条一个**互不相同的真实图标名**。
// 为什么必须不同：同名的内联 SVG 在 gzip 眼里就是一份字典，n=200 也只算
// 一条的字节——那会让这条闸测不到真东西（首屏的真实形状就是几百个不同的图标
// markup 堆在一起）。名字从索引产物里取，不手写枚举。
func realistic(t *testing.T, n int) *model.Bookmarks {
	t.Helper()
	ix, _, _ := assets(t)
	names := ix.Names("tabler")
	if len(names) == 0 {
		t.Fatal("索引里没有 tabler 名字，夹具造不出真形状")
	}
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Title = "发射台"
	titles := []string{"内网", "影音", "工具", "读与收藏"}
	for i := 0; i < 4; i++ {
		b.Groups = append(b.Groups, model.Group{
			ID: fmt.Sprintf("g%d", i), Title: titles[i], Layout: model.LayoutTiles,
			Items: []model.Item{},
		})
	}
	for i := 0; i < n; i++ {
		g := &b.Groups[i%4]
		g.Items = append(g.Items, model.Item{
			ID:   fmt.Sprintf("i%d", i),
			Name: fmt.Sprintf("条目 %d 号·内网服务", i),
			URL:  fmt.Sprintf("http://10.0.0.%d:%d/some/path", i%250+1, 8000+i),
			Desc: fmt.Sprintf("这是第 %d 条的说明文字，用来量正文占比", i),
			Icon: names[i%len(names)],
		})
	}
	return b
}

func TestBudget_首屏字节在两条线内(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	for _, tc := range []struct {
		n, operational int
	}{
		{25, budget25Operational},
		{200, budget200Operational},
	} {
		if err := s.SaveBookmarks("dad", realistic(t, tc.n)); err != nil {
			t.Fatal(err)
		}
		// 每个规模一个新渲染器：缓存键含 bookmarks.json 的 mtime，
		// 同一台渲染器连着量两个规模会把上一个的条目当成本次的读数。
		r = New(s, ix, fc)
		p, err := r.ProfilePage(PageRequest{ID: "dad"})
		if err != nil {
			t.Fatal(err)
		}
		raw, gz := len(p.Body), gzipSize(p.Body)
		t.Logf("%d 条: raw=%d gzip=%d", tc.n, raw, gz)
		if gz > tc.operational {
			t.Errorf("%d 条的首屏 gzip = %d B，超了操作线 %d B"+
				"（实测基线见本文件注释；超线先查是谁把字节堆进了首屏）", tc.n, gz, tc.operational)
		}
		if gz > budgetHTMLContract {
			t.Errorf("%d 条的首屏 gzip = %d B，破了 §7.3 的 120KB 契约天花板", tc.n, gz)
		}
		// 首屏必须真的带内容（否则"字节达标"是用空页面换来的假绿）。
		if !bytes.Contains(p.Body, []byte(`<a class="card"`)) {
			t.Errorf("%d 条夹具没渲染出磁贴", tc.n)
		}
	}
}

func TestBudget_内联CSS有上限(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("内联 CSS: raw=%d gzip=%d", len(raw), gzipSize(raw))
	if len(raw) > budgetCSSOperational {
		t.Errorf("内联 CSS = %d B，超了操作线 %d B（§7.3 的 40KB 按压缩后算，"+
			"这里钉的是原始字节：CSS 与整页一起压，单独量 gzip 没意义，而原始字节"+
			"才是页面里真占的那部分）", len(raw), budgetCSSOperational)
	}
	if len(raw) > budgetCSSContract {
		t.Errorf("内联 CSS = %d B，破了 §7.3 的 40KB 契约天花板", len(raw))
	}
}

// 零 JS 的守门人（本期判据）。④ 里 `<script` 计数为 0 是一条真断言，
// 不是一句修辞：四种页面全量数一遍，因为只要有一种页面漏进脚本，
// §7.2 那句"关掉 JavaScript 时全部书签仍可见可点"就开始撒谎。
//
// 到 P1-⑤ 这条会红——那是它该红的时刻：届时按 ⑤ 的预算改写它（加一条
// JS 字节预算），而不是现在预支。
func TestBudget_四种页面都零脚本(t *testing.T) {
	ix, fc, s := assets(t)
	if err := s.SaveBookmarks("dad", realistic(t, 25)); err != nil {
		t.Fatal(err)
	}
	r := New(s, ix, fc)
	pages := map[string][]byte{}
	p, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	pages["profile"] = p.Body
	pk, err := r.PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pages["picker"] = pk.Body
	br, err := r.BrokenPage("dad", "bookmarks.json 第 3 行附近: 测试夹具")
	if err != nil {
		t.Fatal(err)
	}
	pages["broken"] = br.Body
	ad, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸", Items: 25, Rev: 3}})
	if err != nil {
		t.Fatal(err)
	}
	pages["admin"] = ad.Body
	for name, body := range pages {
		if n := strings.Count(string(body), "<script"); n != 0 {
			t.Errorf("%s 页面里有 %d 处 <script——④ 的判据是零 JavaScript", name, n)
		}
	}
}
```

Run: `go test ./internal/render -count=1 -run Budget -v` → 三条绿，`t.Logf` 那三行读数与注释里的实测数**同量级**（不同规模差得远说明夹具没造对）。

- [ ] **Step 2: 写泄漏守卫的失败测试（守卫自己也要有证人）**

```go
// tools/leakguard/main_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write 在临时目录里落一个文件，返回它的路径。
func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// quad 把八位组拼成一个 IPv4 字面量。**为什么夹具不逐字写**：守卫扫的是 git
// 跟踪的文本，而这份测试文件本身就在里面——逐字写一个示例空间外的地址等于
// 自己给自己报泄漏，CI 从此永久红，而一条没人信的闸等于没闸。
// 这里选的不是"给守卫开一条豁免"，而是让"仓库里没有成形的非示例地址"这条
// 判据**一个字都不打折**（AGENTS.md 那句"只用 10. 段与 127.0.0.1"没有例外，
// 守卫自己也照它活）。运行时字节与逐字写完全一样，测的还是同一个形状；
// 而且这个拼法自己有人守：把 quad 改坏（比如换成 "-" 连接）正则就匹配不到，
// 下面那些 want=1 / want=2 的用例当场红——它不会静悄悄变成假绿。
func quad(octets ...string) string { return strings.Join(octets, ".") }

func TestScan_示例空间放行其他都报(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, body string
		want       int // 期望的处数
	}{
		{"示例地址放行", "url=http://10.0.0.3:5000/x\n", 0},
		{"整个10段都算示例", "url=http://10.9.9.9/\n", 0}, // 真实夹具里就有这条（resolve_test.go）
		{"回环放行", "B=http://127.0.0.1:7600\n", 0},
		{"192段要报", "NAS=" + quad("192", "168", "2", "3") + ":7570\n", 1},
		{"172段要报", "host: " + quad("172", "20", "3", "4") + "\n", 1},
		{"公网也报", "dns=" + quad("8", "8", "8", "8") + "\n", 1}, // 文档里没有需要真公网地址的地方
		{"多处逐条报", "a=" + quad("192", "168", "1", "1") + " b=" + quad("192", "168", "1", "2") + "\n", 2},
		// 解析器的边界（这四条钉的是"它没吞形状"，也钉了它抓不到什么）：
		{"版本号不是地址", "go 1.27.0\n", 0},
		{"超界四元组不是合法IP", "999.999.999.999\n", 0},
		{"半截地址抓不到", "prefix: 192.168.\n", 0}, // 已知边界，注释里要写清
		{"URL带端口照样抓", "u=http://" + quad("192", "168", "2", "3") + ":7570/#x\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := write(t, dir, strings.ReplaceAll(tc.name, " ", "_")+".txt", tc.body)
			findings, skipped, err := scan([]string{f})
			if err != nil {
				t.Fatal(err)
			}
			if len(skipped) != 0 {
				t.Errorf("纯文本不该被跳过: %v", skipped)
			}
			if len(findings) != tc.want {
				t.Fatalf("处数 = %d，要 %d:\n%s", len(findings), tc.want, strings.Join(findings, "\n"))
			}
			if tc.want > 0 && !strings.Contains(findings[0], f) {
				t.Errorf("报告里要点名文件: %s", findings[0])
			}
		})
	}
}

// 非 UTF-8 文件必须**报出来它是被跳过的**，不许静默算"扫过了、干净"。
// 这是"工具报 0 条不等于没问题"那条教训的形状钉。
func TestScan_跳过要明说不是静默通过(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bin.txt")
	if err := os.WriteFile(p, []byte{0xff, 0xfe, 'h', 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	findings, skipped, err := scan([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("二进制里凑出来的假四元组不算泄漏: %v", findings)
	}
	if len(skipped) != 1 || skipped[0] != p {
		t.Fatalf("跳过的文件要报出来: %v", skipped)
	}
}

// gz 产物走扩展名豁免（图标索引里随机字节会凑出假四元组）。
func TestScan_扩展名豁免(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "index.json.gz", quad("192", "168", "9", "9"))
	findings, skipped, err := scan([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 || len(skipped) != 0 {
		t.Errorf("二进制扩展名既不报泄漏也不算跳过，而是根本不进扫描: findings=%v skipped=%v", findings, skipped)
	}
}

// 列文件这一步失败时不能退出码 0：守卫没跑成与发现泄漏都得让 CI 红。
func TestTrackedFiles_列不出来就报错(t *testing.T) {
	if _, err := trackedFiles([]string{filepath.Join(t.TempDir(), "不存在")}); err == nil {
		t.Fatal("指向不存在的路径必须报错，不能返回空集合当'干净'")
	}
}
```

Run: `go test ./tools/leakguard -count=1`
Expected: 编译失败（`scan` / `trackedFiles` 未定义）。

- [ ] **Step 3: 写 leakguard**

```go
// tools/leakguard/main.go
// Package main 是 CI 的泄漏守卫：扫仓库里被 git 跟踪的文本文件，
// 报告不在"示例地址空间"里的 IPv4 字面量。
//
// 为什么值得一条闸：本仓库挂在公开组合下，而真实内网拓扑（本案的 NAS
// 与它的 192.168. 段地址）一旦进过 git 历史就**收不回来**——那是所有
// 可能错误里唯一不可撤回的一类。spec §15 与 AGENTS.md 都写了"只用 10. 段
// 与 127.0.0.1"，这条把那句修辞变成退出码。
//
// 白名单而不是黑名单：开放集合用黑名单必漏（下一个私有段就是漏的那一个）。
// 需要放行新地址空间时改 allow 并在注释写明"它为什么是示例而不是拓扑"。
//
// 已知边界：抓不到半截地址（`192.168.`）、主机名、DNS 名与已经 base64 过的
// 内容。它防的是"复制粘贴了一段真配置进文档/夹具"这一种最常见的失误，
// 不防蓄意隐藏——那是 code review 的活。
//
// 它扫的是 `git ls-files`，因此**这个包自己也在扫描范围内**：仓库里不许出现
// 成形的非示例地址，守卫的夹具也不例外（要抓的地址在 main_test.go 里按八位组
// 现拼）。没有给守卫开"豁免名单"这种第二条判据——一条有例外的闸，例外长得多快
// 没人知道；而 CI 因为自己的测试语料永久红，比没闸更糟：没人信的闸等于没闸。
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var allow = []*net.IPNet{mustNet("10.0.0.0/8"), mustNet("127.0.0.0/8")}

var ipRE = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)

// 按扩展名豁免的二进制产物：随机字节会凑出假四元组。
var skipExt = map[string]bool{".gz": true, ".png": true, ".ico": true, ".webp": true, ".jpg": true, ".woff2": true}

func mustNet(cidr string) *net.IPNet {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return n
}

// trackedFiles 拿要扫的文件名单：给了参数就按参数走（测试与本地定向用），
// 没给就跑 `git ls-files`——"进仓库"这件事的判据是 git 跟踪了什么，
// 不是目录里躺着什么（data/ 与 .env 是被 gitignore 的真数据，不归这里管）。
func trackedFiles(args []string) ([]string, error) {
	if len(args) > 0 {
		var out []string
		for _, a := range args {
			st, err := os.Stat(a)
			if err != nil {
				return nil, fmt.Errorf("读不到扫描目标 %s: %w", a, err)
			}
			if !st.IsDir() {
				out = append(out, a)
				continue
			}
			err = filepath.Walk(a, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() {
					out = append(out, p)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("遍历 %s: %w", a, err)
			}
		}
		return out, nil
	}
	cmd := exec.Command("git", "ls-files", "-z")
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files 失败（这不在仓库里，或者 git 没装）: %w", err)
	}
	var out []string
	for _, f := range strings.Split(string(raw), "\x00") {
		if f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("git ls-files 返回空集合——要么仓库是空的，要么这一步没真的跑起来；两种都不该被当成'干净'")
	}
	return out, nil
}

// scan 返回（泄漏处名单, 被跳过的文件名单, 错误）。
// skipped 必须交给调用方打印：静默跳过等于给守卫开一个盲区。
func scan(files []string) (findings, skipped []string, err error) {
	for _, f := range files {
		if skipExt[strings.ToLower(filepath.Ext(f))] {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, fmt.Errorf("读 %s: %w", f, err)
		}
		if !utf8.Valid(data) {
			skipped = append(skipped, f)
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range ipRE.FindAllString(line, -1) {
				ip := net.ParseIP(m)
				if ip == nil || ip.To4() == nil {
					continue // 版本号、超界四元组：不是地址
				}
				inside := false
				for _, n := range allow {
					if n.Contains(ip) {
						inside = true
						break
					}
				}
				if !inside {
					findings = append(findings, fmt.Sprintf("%s:%d: %s", f, i+1, m))
				}
			}
		}
	}
	return findings, skipped, nil
}

func main() {
	targets, err := trackedFiles(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "leakguard: %v\n", err)
		os.Exit(2)
	}
	findings, skipped, err := scan(targets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "leakguard: %v\n", err)
		os.Exit(2)
	}
	for _, f := range skipped {
		fmt.Fprintf(os.Stderr, "leakguard: 跳过（非 UTF-8，没扫）: %s\n", f)
	}
	if len(findings) > 0 {
		fmt.Println("leakguard: 发现不在示例地址空间里的 IPv4 字面量（真实内网地址不许进仓库，spec §15）")
		for _, f := range findings {
			fmt.Println("  " + f)
		}
		os.Exit(1)
	}
	fmt.Printf("leakguard: %d 个文件扫过，0 处泄漏（跳过 %d 个非 UTF-8）\n", len(targets), len(skipped))
}
```

Run: `go run ./tools/leakguard` → 应当打出行数（当前仓库跟踪 **120 个左右**文件，实测数以输出为准）且退出码 0。
再跑一次**带真值的反向验证**：临时建一个文件写 `probe = ` 加一个**示例空间外**的
完整四元组（`192.168.` 段那种；这里**不在计划里逐字写出成形地址**——这份文档本身
就被守卫扫，写成形就等于自己给自己报泄漏，夹具要的值现拼即可），
`go run ./tools/leakguard probe.txt` 必须退出码 1 并点名它，然后删掉那个文件。
这一步防的是"正则根本没匹配任何东西却报干净"。

- [ ] **Step 4: 仓库第一条 CI**

`.github/workflows/ci.yml`（整份新文件）：

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  gate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod   # 版本从 go.mod 拿，不在这里抄一份会漂移的号
      - name: vet
        run: go vet ./...
      - name: gofmt
        run: |
          out=$(gofmt -l .)
          if [ -n "$out" ]; then
            printf '需要 gofmt:\n%s\n' "$out"
            exit 1
          fi
      - name: test
        run: go test ./... -count=1
      - name: build
        run: CGO_ENABLED=0 go build ./...
      - name: leakguard
        run: go run ./tools/leakguard
```

**这四道门禁 + 预算测试 + 泄漏守卫就是 §7.3 说的 CI 全部内容**：LCP、帧率、SW 命中三条**不进 CI**（要真浏览器与真网络；headless 的默认值陷阱本期已实测过——`prefers-reduced-motion` 默认报 `reduce`，六张主图差点全拍成动效关闭态）。它们留在 Task 9 的人工实拍清单里。

`.dockerignore` 追加一行 `.github`（与既有 `tools/` 同判据：CI 与工具件不进构建上下文）。

- [ ] **Step 5: 【故意破坏】三次（预算与守卫都得证明它会红）**

1. 把 `budget25Operational` 改成 `8_000`（低于实测 12,575）→ `TestBudget_首屏字节在两条线内` 红。**这条证的是量的是真字节，不是常量**。
2. 把 `realistic` 的 `Icon` 全写成 `"tabler:home"`（同名）→ 200 条档 gzip 会掉到远低于操作线，读数立刻不对味；这一步不是让它红，而是**看懂它为什么要求不同名**（跑一次记下 gzip 数，与不同名那次对比，写进报告）。
3. 在任一 `.go` 里临时加一行注释，逐字写一个**示例空间外**的完整四元组（`192.168.` 段那种，具体值按 `main_test.go` 的 `quad` 拼出来）→ `go run ./tools/leakguard` 退出码 1 并点名那一行；删掉后回 0。**这一行不能留在仓库里**：守卫连自己的仓库都扫，写成形的那一行就是它该报的那一条。
4. 把 `scan` 里那整条 `ip == nil || ip.To4() == nil` 过滤器**删掉** → `TestScan_…` 的"超界四元组不是合法IP"红（999.999.999.999 被当成地址）。**别只删 `ip == nil` 那半句**：`net.ParseIP` 认不出的串 `To4()` 本来就是 nil，半句删掉 rc 仍是 0——那是一次空破坏，不是证人（上一轮把它写成"去掉 nil 判断（`ip == nil` 那半句）"，照它复跑的人会以为过滤器没被守着）。

- [ ] **Step 6: 干净克隆彩排（CI 的输入必须都在仓库里）**

本仓库当前没有 remote，"CI 跑通"这件事在推上去之前只能靠一次本地彩排：

```bash
CLONE="$(mktemp -d)/clean"
git clone -q "$(git rev-parse --show-toplevel)" "$CLONE"   # 不写死绝对路径：目录改过名照样跑得通
cd "$CLONE"
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
go run ./tools/leakguard; echo "leakguard exit=$?"
```

Expected: 四道全过、`gofmt -l .` 空输出、`leakguard exit=0`。

这一步抓的是三类只在"干净克隆"里暴露的问题：**CI 的输入文件没进仓库**（夹具、
`openapi.json`、图标 `.gz` 产物）、**未跟踪文件在本地撑着绿灯**（`data/` 里的东西
被测试读到了）、**CRLF 让 Linux 侧红**（本条按 `.gitattributes` 已定 `eol=lf`，
彩排在 Windows 上做仍能验出漏配的文件）。任何一条红，都在原仓库补上缺的文件
再重来，**不要**在克隆里修——那份 clone 用完就删。

推上去真正跑 CI 是另一件事：它需要人批准 push（本仓库无 remote，push 先问）。

- [ ] **Step 7: 门禁 + 提交**

```bash
go vet ./... && gofmt -l . && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
go run ./tools/leakguard
git add internal/render/budget_test.go tools/leakguard .github/workflows/ci.yml .dockerignore
git commit -m "ci: 第一条 workflow（四道门禁+预算+泄漏守卫）与 §7.3 体积预算闸"
```

---

## Task 9: 端到端验收（真容器 + 真浏览器 + 入库的机器读数）

**Files:**
- Create: `docs/superpowers/acceptance/2026-10-06-launchbay-p1-4-shell.md`（验收记录：每屏的 DOM 读数 + 实拍文件位置 + 人工项的测法）

这是**人来做的一段**，不是写代码的一段。判据：每条都要有读数，读数要抄到正文（只抄标题等于没读）。

- [ ] **Step 1: 起容器，并用 API 造一份自有的验收夹具**

```bash
cd "$(git rev-parse --show-toplevel)"                      # 仓库根（目录名不作数）
# .env 里有两个令牌变量（LAUNCHBAY_TOKEN_AI_DAD / LAUNCHBAY_TOKEN_ADMIN），
# 数据在 ./data（gitignore，不进仓库）。宿主端口沿用本机验收用的 17600，
# 因为 Windows 把 7532-7631 整段划给了 Hyper-V 动态保留。
LAUNCHBAY_HOST_PORT=17600 docker compose up --build -d
docker ps --filter name=launchbay --format '{{.Names}} {{.Status}} {{.Ports}}'
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:17600/healthz
```

若 Docker Desktop 没在跑：先启动它并**说出来**（这是环境副作用，不是仓库动作）。**不 prune 镜像、不删别人的容器与卷**。构建阶段会跑镜像内 `go test ./...`——它红了就是本期测试红了，别绕。

验收要的是**能被下一个人复现的登录**，所以本期不自问"P1-① 那批数据里的 PIN 是什么"（它只以哈希存在磁盘上，问不出来也不该问），而是用管理员令牌现造两个档案，PIN 用夹具常量：

```bash
B=http://127.0.0.1:17600
set -a; . ./.env; set +a          # LAUNCHBAY_TOKEN_ADMIN 进环境，不回显
mk() { curl -s -H "Authorization: Bearer $LAUNCHBAY_TOKEN_ADMIN" -H 'Content-Type: application/json' \
       --data-binary @- $B/api/v1/profiles; }
printf '{"id":"p4dad","title":"爸爸④","pin":"1234","admin":true}'  | mk; echo
printf '{"id":"p4kid","title":"小满④","pin":"5678"}'                | mk; echo
```

`data/templates/` 在本机是 `nas.json`，但它**不在仓库里**（`data/` 整个被 gitignore），所以这里不靠模板：两个档案建出来都是空台，随后用 `POST /api/v1/batch`（`expectedRev` 照 `GET /api/v1/data` 的回读值）给 `p4dad` 补 25 条、给 `p4kid` 补 3 条——**发带中文的 body 一定从文件发**（Git Bash 的 `-d '中文'` 会按 GBK 送出去，服务端 422 拒）。补完照 §7.3 的读数把 `25 条 / raw / gzip` 抄进验收文件。

- [ ] **Step 2: 浏览器通路逐条走一遍（curl + cookie jar）**

```bash
B=http://127.0.0.1:17600
J=$(mktemp)
# 1) 无 cookie：只有档案名与上次使用时间，零条目零 URL 零分组名
curl -s $B/ | grep -c 'class="card'          # 期望 0
# 2) 表单进门（PIN 是上一步自己设的夹具常量 1234）
curl -s -c $J -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  -d 'profile=p4dad&pin=1234' $B/pin
curl -s -b $J -o /dev/null -w 'X-LB-Profile=%header{X-Lb-Profile}\n' $B/
# 3) 顶栏主题按钮：换主题 → 303 → 紧接着的首页必须带新 data-theme
curl -s -b $J -o /dev/null -w '%{http_code} %{redirect_url}\n' -d 'theme=light' $B/prefs
curl -s -b $J $B/ | grep -o 'data-theme="[a-z]*"' | head -1
curl -s -b $J -o /dev/null -d 'theme=dark' $B/prefs
curl -s -b $J $B/ | grep -o 'data-theme="[a-z]*"' | head -1
# 4) 形态：sidebar ↔ topbar，页面结构要跟着换
curl -s -b $J -o /dev/null -d 'layout=topbar' $B/prefs
curl -s -b $J $B/ | grep -o 'data-profile-layout="[a-z]*"\|<aside' | sort -u
curl -s -b $J -o /dev/null -d 'layout=sidebar' $B/prefs
# 5) 搜索：命中/无命中两句话，侧栏计数随命中变（查询词换成 Step 1 里真造出来的那个名字）
curl -s -b $J "$B/?q=群晖" | grep -o '命中 [0-9]* 条 · 共 [0-9]* 条\|没有匹配[^<]*'
curl -s -b $J "$B/?q=zzzzz不存在" | grep -o '没有匹配[^<]*\|这个档案还没有书签'
# 6) 管理通路：/admin → 进入编辑 p4kid → 首页横幅 → 退出管理
curl -s -b $J -o /dev/null -w '%{http_code}\n' $B/admin
curl -s -b $J -c $J -o /dev/null -w '%{http_code} %{redirect_url}\n' -d 'to=p4kid' $B/switch
curl -s -b $J $B/ | grep -o '正在管理 <b>[^<]*</b>\|小满④\|爸爸④\|退出管理'
curl -s -b $J -c $J -o /dev/null -d 'to=home' $B/switch
curl -s -b $J $B/ | grep -c '正在管理'      # 期望 0
# 7) 非管理员看 /admin = 303 回选择页，不报错页
curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  --cookie 'lb_home=p4kid; lb_profile=p4kid' $B/admin
# 8) 令牌打浏览器通路：也是 303，不是 401 JSON
curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  -H "Authorization: Bearer $LAUNCHBAY_TOKEN_ADMIN" -d 'theme=light' $B/prefs
```

第 7 步那条伪造的 cookie **不是漏洞演示**：cookie 本来就不签名（spec §11 明写不防局域网内冒充），这里要的只是"非管理员这一支的响应长相"。真判据在磁盘上——那个档案的 `admin` 是 false，所以它进不了 `/admin`。

每条把**实际输出**抄进验收文件（含数字与整句正文，不许只写"符合预期"）。

- [ ] **Step 3: 实拍三视口 × 两主题**

用 chrome-devtools MCP 或 CLI 截图，落盘到 `.superpowers/tmp/p14/`（gitignore，图不进仓库；**读数进仓库**）。硬规矩（都踩过）：

1. **`prefers-reduced-motion` 显式设 `no-preference`**，并且等 ≥ 1400 ms 再拍——入场 stagger 最后一条延迟 .44s + .5s 时长，早拍会抓到 `opacity:0` 的空格子，然后你会去修一个不存在的 bug。
2. 窄视口必须 `emulate`（`resize_page` 最小宽约 501px，380px 那档拿不到）。
3. 细节图用 2× 放大看（图标底色、边框、间距），但**布局判定只信 1× 整页图**：`document.body.style.zoom` 会改有效视口，容器查询跟着翻。
4. 每屏都要 DOM 正文读数（标题 + 正文 + 哪颗按钮在场/缺席），落进验收文件的表里。

视角清单：1280×900（侧栏形态）、900×800（抽屉）、380×780（横向标签行）；每档 dark 与 light 各一张；再加一张 `/admin`、一张坏档页（Step 4 造）、一张**带管理横幅的 `p4kid` 台拍在 1280×900**（侧栏的 `top:66px` 让位只在 ≥1101 生效，量就得在这一档量）。

- [ ] **Step 4: 坏档页实拍（部署验收 #4 的那一条）**

```bash
# 只在验收夹具里做：把 p4kid 的 bookmarks.json 写坏（./data 是 gitignore 的本地数据）
cp data/profiles/p4kid/bookmarks.json /tmp/p4kid.bookmarks.bak
docker stop launchbay
python - <<'PY'
import io
p = "data/profiles/p4kid/bookmarks.json"
raw = io.open(p, encoding="utf-8").read()
io.open(p, "w", encoding="utf-8").write(raw[: len(raw)//2])
print("截断到", len(raw)//2, "字节")
PY
docker start launchbay && sleep 3
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:17600/healthz
```

进 `p4kid` 的台（PIN 是 Step 1 里自己设的 5678）→ 拍那一页并抄读数：主句是人话、`<details>` 展开后能看到 `bookmarks.json` 与行号、**`p4dad` 那个档案照常渲染**（"一个人的手误不该让全家开不了导航页"）。做完**把文件恢复原样**（`cp /tmp/p4kid.bookmarks.bak data/profiles/p4kid/bookmarks.json`），复跑一次确认恢复正常，并在验收文件里写"已复原"。

- [ ] **Step 5: 预算与守卫的真实读数**

```bash
go test ./internal/render -count=1 -run Budget -v > .superpowers/tmp/p14/budget.txt
go run ./tools/leakguard > .superpowers/tmp/p14/leakguard.txt; echo "exit=$?"
```
把 `-v` 输出里那三行 `t.Logf` 读数与守卫的"扫了 N 个文件"抄进验收文件。

- [ ] **Step 6: 零 JS 通路真验一次**

浏览器里手动关掉 JavaScript（DevTools 命令菜单 "Rendering → JavaScript" 不可用时，用 `chrome --disable-javascript` 起一个临时实例；**做不到就在验收文件里写明这一条是人工核验，并给出替代证据**）。核验点：档案选择页能进门、书签能点、主题按钮能换、搜索能提交、管理横幅的「退出管理」有效。这条的自动化替身是 `TestBudget_四种页面都零脚本`（页面里根本没有 `<script`，所以关掉它不该有任何差别——**差别一旦出现就是 bug**）。

- [ ] **Step 7: 人工项写清测法（不进 CI，但不能只留一句"以后测"）**

验收文件里给三条各自的操作步骤：
- **LCP < 300ms**：容器起在一台真机上、局域网内一台真客户端，DevTools Performance 录一次冷启动（先 `Empty cache and hard load`），取 LCP 那条标记的毫秒数。
- **1,000 条滚动 ≥ 55fps**：用 `POST /api/v1/batch` 造 1000 条（别改磁盘），在客户端用 Performance 面板的帧率轨读数；这一条同时验 §7.4 的降级触发条件（超线才切分片）。
- **SW 命中 < 50ms**：P1-⑥ 之后才测得到（现在没有 Service Worker）。这一条明确写"本期无此项，⑥ 复测"，不要留一条假绿。

- [ ] **Step 8: 提交验收记录**

```bash
git add docs/superpowers/acceptance/2026-10-06-launchbay-p1-4-shell.md
git commit -m "docs(acceptance): P1-④ 外壳端到端读数——主题即时性、管理通路、坏档页与预算实测"
```

---

## 交付判定（整个 ④ 完成的样子）

1. `go test ./... -count=1` 全绿，其中新增证人至少覆盖 spec §12 那十二条。
2. `GET /` 的 HTML 里 `<script` 计数为 0，且浏览器手动关掉 JavaScript 后所有动作照旧（Task 9 Step 6）。
3. 主题按钮按下去，**刷新前不需要任何别的动作**页面就是新主题（`profileFingerprint` 那一维的端到端证人，Task 3）。
4. 管理通路整条走得通：`/admin` → 进入编辑 → 横幅（两个名字都在）→ 退出管理；非管理员在别人的档案里伪造 `to=` 被拒。
5. 三条新路由（`/prefs`、`/switch`、`/admin`）都过了五条既有契约闸，豁免名单没变大。
6. CI 在干净克隆上跑通（含泄漏守卫），CI 的输入文件都在仓库里。
7. 坏档页第一句是人话，细节里有文件名与行号，且与 API 503 的 `detail` 逐字同源。
8. 验收文件在仓库里，里面是正文读数而不是"符合预期"。
