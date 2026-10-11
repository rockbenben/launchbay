# P1-② 写通路（ops / batch / 快照 / 历史 / 契约）Implementation Plan

> **本文是历史实施记录，不是待办。** 它记录的是那一天计划或量的是什么，里面的
> 函数名、测试名与接口形状**可能已经变了**——2026-10-10 的管理员边界收紧一次性改判掉了
> 代管态、模板机制与徽标切档，共享清单也改判为不做。**不要照着这里的命令执行**：
> 权威契约在仓库根的 `AGENTS.md`，设计在 `docs/superpowers/specs/`。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 AI 能用一批 ops 安全地改任何档案的书签，并且每一次改动都可查（history）、可退（snapshot + revert）。

**Architecture:** 三层收敛：`internal/ops` 是纯函数（路径文法 + 四种 op，不碰磁盘）；`internal/store` 加快照、历史、点击计数三种磁盘原语；`internal/httpapi` 把两者编排成 REST，并且**所有写路径（含 CLI 子命令）都只能经这条 REST 进服务进程**——进程内互斥锁罩不住另一个进程，这是 spec §4.5 的硬约束。`openapi.json` 与路由表逐条对照，文档骗 AI 比骗人更贵。

**Tech Stack:** Go 1.27（`net/http` ServeMux 方法 pattern、`go:embed`、`encoding/json`）、无新依赖（spec 的"不引入数据库/ORM/前端框架"继续成立）。

**Spec:** `docs/superpowers/specs/2026-10-05-launchbay-design.md` —— 本计划覆盖 §4.4、§4.5（`sync-template` 部分）、§4.6、§6.1（除 `icons`/`resolve`）、§6.2、§6.4、§12（写失败/快照失败分流）、§13（store/auth/api/usage/sync-template 五组单测 + 契约测试）。**执行时 spec 与本案一起读。**

## 写在前面的三个发现（要人拍板，不许实现者自己猜）

1. **`LAUNCHBAY_ADMIN_PROFILE` 是 spec 里的孤儿。** §10 的 compose 片段列了它，但全文没有任何一处消费它——§6.1 明写管理员资格"由服务端回查磁盘"（`profile.json` 的 `admin` 字段），§11 又明写不做签名/密钥。本计划**不实现它**，并在 Task 13 的收尾里把它从 spec §10 删掉（一句话改动，不碰代码）。若人要求保留，它必须有一个明确读者，否则就是下一个"看起来有用其实没人读"的配置。
2. **`/api/v1/icons` 与 `/api/v1/resolve` 不在本案。** 两者都依赖图标 name→path 索引（P1-③）。`openapi.json`（Task 11）里**照 spec 列出这两个端点并标 `"x-status": "planned"`**，契约测试只对照已注册的路由——理由是：AI 读 openapi 时应当看见全貌并知道这两个还没上线，而不是在文档里完全找不到它们、然后靠试错去猜。
3. **点击计数（`/api/v1/usage`）归本案，不归 UI slab。** 它要证明的三条（不产生 rev、不进快照、不进历史）恰好是本案刚建好的那套机制的否定面，spec §13 还点名要"故意刷一次再断言文件数没变"。放在 P1-④ 就得把那套机制的测试重写一遍。

## Global Constraints

每个任务都隐含遵守本节，值取自 spec 原文或与 P1-① 计划一致。

- 端口固定 **7600**（7570 是 flare）。Go 模块 `github.com/rockbenben/launchbay`，`go 1.27`。
- `CGO_ENABLED=0`；非 root 运行；镜像 **≤ 20MB**（新增代码全在同一个二进制里，不新增阶段）。
- 不引入前端框架、不引入数据库、不引入 ORM、**不引入新的第三方 Go 依赖**。数据只有 JSON 文本文件。
- **不加严格安全机制**：不签 cookie、不做 PIN 限速/退避/锁定、不用 argon2/bcrypt、不要会话密钥。spec §11 的「明写不防」清单是验收项——**多出来的防护也算失败**。
- JSON 写出契约：2 空格缩进、`SetEscapeHTML(false)`、非 ASCII 原样、末尾一个换行、空数组写 `[]` 不写 `null`、键顺序由 struct 字段顺序决定、**禁止用 map 产生输出键序**（`history.jsonl` 与 `usage.json` 同样受这条约束，见 Task 3 / Task 10）。
- 寻址只用稳定 `id`，**不用数组下标**；`id` 是 ASCII slug；item 的 `id` 在单档案内全局唯一（跨分组不复用）。`index` 只出现在 op 的"插到第几位"参数里，不是寻址手段。
- 一次写入 = 一批 ops，**全成或全败**，落到同一个新 `rev`；`op` 只有 `add`/`remove`/`set`/`move` 四种，**不给整表替换型 op**。
- 写失败一律结构化返回 `{code, detail, opsIndex?}`；快照目录写失败 → **`507` 并且不写书签**（不许"跳过快照继续写"）。
- `history.jsonl` 与任何状态输出走**字段白名单**：不记令牌值，不记 ops 的 `value`（可能含 URL 与描述）。
- 文档、夹具、测试数据里的地址**只允许 `10.` 段**。真实 NAS 网段一个字面量都不许进仓库。
- 界面与给 AI 读的文案用中文；错误 `detail` 用中文，机器可读的键用英文 camelCase（`expectedRev` / `currentRev` / `opsIndex`）。
- 本机 Windows + Git Bash：新终端才吃得到 `C:\Program Files\Go\bin`；未重开的会话里先 `export PATH="$PATH:/c/Program Files/Go/bin"`。
- 每个任务收尾都跑同一组门禁：`go test ./... && go vet ./... && gofmt -l .`（gofmt **必须无输出**）。
- 判「文件不存在」一律用 `errors.Is(err, os.ErrNotExist)`——`internal/store` 全包就是这个写法；计划片段里若出现 `os.IsNotExist(err)`，以包内惯例为准（Task 3 评审定的这条）。
- **API 响应体一律用 struct 产生键序，不许 `map[string]any`**：Go 对 map 按键名排序输出，`{code, currentRev, detail}` 会变成字母序，且同一端点的多种 422 会漂成两种形状（Task 5 评审定的这条）。错误响应统一一个 `errorResponse{Code, Detail, CurrentRev, Errors}` 结构体，可选字段 `omitempty`。这条追溯性地适用于 P1-① 已有的 `writeJSON`/`writeFieldErrors` 调用点——本案新写的每一处都按此办，旧的在最终整枝评审里一并收。
- 鉴权失败必须分得清「没认出来」与「认出来但没资格」：未知/吊销的令牌是 **401**，只有作用域不含该档案才是 **403**（spec §6.1 的响应分流）。把两者合并会让调用方拿着一个死令牌反复重试。
- 计划里标了 **【故意破坏】** 的步骤必须真做一次、看到测试变红、再把代码改回去；红的原因必须就是那一步，"相邻断言也红"不算过（P1-① 的教训：证人要因对的理由变红）。

## 前置事实（已实测，不要重复调研）

- P1-① 已合入 `main`（`caf71de`）并部署验收通过：`go test ./...` 六包全绿；本机容器 `launchbay` 用 `LAUNCHBAY_HOST_PORT=17600` 跑在 17600（宿主 7600 落在 Hyper-V 保留段 `7532-7631`），`Up (healthy)`。
- 已有磁盘原语（`internal/store`）：`New(root)`、`ListProfiles`、`LoadProfile`、`LoadBookmarks`、`Current(id) (Snapshot{B,Rev,Modified}, error)`、`SaveBookmarks(id, *model.Bookmarks) error`（内部 `normalize` + `Validate`，**不碰 rev**）、`SaveProfile`、`CreateProfile(NewProfileInput{ID,Title,PinHash,Admin,FromTemplate})`、`WithProfileLock(id, func() error)`、`WriteFileJSON(path, v)`、`WriteFileAtomic(path, data)`、`LoadTemplate(name)`、`Broken()`、`BrokenReason(id)`、`SnapshotsDir(id)`、`HistoryPath()`。
- 已有 HTTP 件（`internal/httpapi`）：`New(Dependencies{Store,Tokens,Render}).Handler()`；`/api/v1` 前缀的 `private, no-store` 中间件已经罩住全部 API 路由（含 404）；`authorize(r) (auth.Scope, bool)` 读 `Authorization: Bearer`；`writeJSON`、`writeFieldErrors`、`writeInternalError`（500 只回 `{code,detail}`，真因进 stderr）、`decodeBody`（`DisallowUnknownFields` + 1 MiB 上限）、`sessionLegal(sess)`、`handlePin` 的 `fromForm` 分岔（`59c26b3`）。
- 已有 auth 件：`ParseScopes` / `Scopes.Authorize(secret) (Scope, bool)` / `Scope.IsAdmin()`（`Profile == "*"`）/ `Scope.Allows(profileID)` / `HashPIN` / `VerifyPIN` / `ValidPinFormat` / `ReadSession(r) Session{Active,Home}` / `WriteSession(w, home, active)` / `SetActive` / `ClearSession` / `Session.Managing()`（代管的唯一判据）。
- 已有 model 件：`Bookmarks{Rev,Title,Groups,Search,Weather,OpenInNewTab,Clock,Footer}`、`Group{ID,Title,Layout,Accent,Items}`、`Item{ID,Name,URL,Icon,Desc,Alias,Tags,AINote}`、`(*Bookmarks).Validate() error`（包 `model.ErrValidation`）、`ValidSlug`、`ExternalScheme(raw) bool`、`checkIcon`（只验前缀 `tabler|simple-icons`）。
- ServeMux 用的是 Go 1.22+ 的方法 pattern（`"GET /healthz"`），**没有**用路径变量；端点参数一律走 query 或 body。
- 部署验收遗留、本案接手的三条：`#5` 请求体非法 UTF-8 被 `encoding/json` 静默替换成 U+FFFD（Task 5 修）；`#6` `search.default` 必须是 `engines[].prefix` 而不是 `name`（Task 13 写进 `AGENTS.md`）；`#1/#3/#4` 是渲染层的事，归 P1-④，本案不碰。

## 文件结构（本案的分解决定在此锁定）

**新建**

| 文件 | 单一职责 |
|---|---|
| `internal/ops/ops.go` | `Op` 与 `Run`：把一批 op 顺序作用到 `*model.Bookmarks` 的副本上，逐条收集错误与告警。不 import `net/http`、不碰磁盘 |
| `internal/ops/path.go` | 路径文法 `groups/<groupId>[/items[/<itemId>]][/<field>]` → 目标描述；字段名白名单 |
| `internal/ops/ops_test.go` `internal/ops/path_test.go` | 上述两者的表驱动测试 |
| `internal/store/snapshot.go` | 写前快照：`WriteSnapshot` / `ListSnapshots` / `LoadSnapshot` / 保留 50 份的裁剪 |
| `internal/store/history.go` | `HistoryEntry`（键序=字段序）、`AppendHistory`、`ReadHistory(profile, limit)` |
| `internal/store/usage.go` | `usage.json` 的读写：条目列表按 `id` 排序，保证同内容两次写出字节一致 |
| `internal/store/snapshot_test.go` `history_test.go` `usage_test.go` | 同上 |
| `internal/httpapi/routes.go` | 路由表：`method + pattern + 凭证类别`，注册与枚举共用一份数据（契约测试的地基） |
| `internal/httpapi/batch.go` | `POST /api/v1/batch` 的编排：鉴权 → 读 → 校验 → 快照 → 应用 → 写 → 历史 |
| `internal/httpapi/read.go` | `GET /api/v1/data`、`GET /api/v1/export`、`GET /api/v1/history` |
| `internal/httpapi/revert.go` | `POST /api/v1/revert` |
| `internal/httpapi/admin.go` | `GET /api/v1/status`、`GET /api/v1/profiles`、`DELETE /api/v1/profiles` |
| `internal/httpapi/pinapi.go` | `POST /api/v1/profiles/self/pin`、`POST /api/v1/profiles/pin` |
| `internal/httpapi/usage.go` | `POST /api/v1/usage` |
| `internal/httpapi/openapi.go` `internal/httpapi/openapi.json` | 契约文档与 `GET /api/v1/openapi.json`（`go:embed`） |
| `internal/httpapi/*_test.go`（各端点一个） | 每个端点的 401/403/404/409/422/507 分流 |
| `cmd/launchbay/client.go` | 只读 env 拿管理员令牌、对本机 REST 发请求的极简客户端（CLI 不许直接写盘） |
| `cmd/launchbay/cli.go` | `profile create` / `revert` / `sync-template` 三个子命令 |
| `cmd/launchbay/client_test.go` `cli_test.go` | 用 `httptest` 假服务验证 CLI 真的只走 REST |
| `AGENTS.md` | 给 AI 读的契约（Task 13） |

**修改**

| 文件 | 改什么 |
|---|---|
| `internal/httpapi/server.go` | 注册改成走路由表（Task 4）；`Dependencies` 不变 |
| `internal/httpapi/handlers.go` | `decodeBody` 加"非法 UTF-8 直接拒"（Task 5）；`handleCreateProfile` 迁进路由表（Task 4） |
| `internal/store/paths.go` | 加 `UsagePath(id)`（Task 10） |
| `cmd/launchbay/main.go` | 子命令分发（Task 7 起） |
| `docs/superpowers/specs/...design.md` | 删掉 §10 的 `LAUNCHBAY_ADMIN_PROFILE`（Task 13） |

---

## Task 1: ops 路径文法与四种 op 的纯应用

**Files:**
- Create: `internal/ops/path.go`
- Create: `internal/ops/ops.go`
- Test: `internal/ops/path_test.go`
- Test: `internal/ops/ops_test.go`

**Interfaces:**
- Consumes: `model.Bookmarks` / `model.Group` / `model.Item`、`model.ValidSlug`、`model.ExternalScheme`
- Produces:
  - `type ops.Op struct { Op, Path string; Value json.RawMessage; Index *int; To string }`（JSON 键：`op` `path` `value` `index` `to`）
  - `type ops.Error struct { OpsIndex int `json:"opsIndex"`; Field string `json:"field"`; Reason string `json:"reason"` }`
  - `func ops.Run(b *model.Bookmarks, in []Op) (*model.Bookmarks, []Error, []string)`
  - `func ops.ParsePath(path string) (target, error)`（包内私有，测试同包可见）

- [ ] **Step 1: 写路径文法的失败测试**

`internal/ops/path_test.go`：

```go
package ops

import "testing"

func TestParsePathAcceptsGrammar(t *testing.T) {
	for _, tc := range []struct {
		path                    string
		wantKind                kind
		wantGroup, wantItem     string
		wantField               string
	}{
		{path: "groups", wantKind: kindGroups},
		{path: "groups/nas", wantKind: kindGroup, wantGroup: "nas"},
		{path: "groups/nas/title", wantKind: kindGroupField, wantGroup: "nas", wantField: "title"},
		{path: "groups/nas/items", wantKind: kindItems, wantGroup: "nas"},
		{path: "groups/nas/items/jellyfin", wantKind: kindItem, wantGroup: "nas", wantItem: "jellyfin"},
		{path: "groups/nas/items/jellyfin/url", wantKind: kindItemField, wantGroup: "nas", wantItem: "jellyfin", wantField: "url"},
	} {
		got, err := ParsePath(tc.path)
		if err != nil {
			t.Fatalf("ParsePath(%q) 意外报错: %v", tc.path, err)
		}
		if got.kind != tc.wantKind || got.groupID != tc.wantGroup || got.itemID != tc.wantItem || got.field != tc.wantField {
			t.Fatalf("ParsePath(%q) = %+v", tc.path, got)
		}
	}
}

func TestParsePathRejectsEverythingElse(t *testing.T) {
	for _, path := range []string{
		"", "items", "groups/", "groups/nas/items/", "groups/nas/items/a/b/c",
		"groups/0/items", "groups/nas/nonsense", "groups/nas/items/jellyfin/password",
		"groups/nas/items/jellyfin/Id",
	} {
		if _, err := ParsePath(path); err == nil {
			t.Fatalf("ParsePath(%q) 该报错，却成功了", path)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/ops/ -run TestParsePath -v`
Expected: 编译失败（`undefined: ParsePath` / `undefined: kind`）——这就是 RED。

- [ ] **Step 3: 写 `path.go`**

```go
// Package ops 把"一批写请求"作用到书签上。它是纯函数：不碰磁盘、不碰 HTTP。
// 全部错误都要回报到具体是第几条 op 的哪个字段——AI 一次改 30 条时，
// "改坏一半"比"全没改"难收拾得多（spec §6.2）。
package ops

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
)

type kind int

const (
	kindGroups kind = iota + 1 // groups —— 组表本身（add 组用）
	kindGroup                  // groups/<gid>
	kindGroupField             // groups/<gid>/<field>
	kindItems                  // groups/<gid>/items —— 条目表（add 条目用）
	kindItem                   // groups/<gid>/items/<iid>
	kindItemField              // groups/<gid>/items/<iid>/<field>
)

type target struct {
	kind    kind
	groupID string
	itemID  string
	field   string
}

// groupFields 与 itemFields 是 set 能碰的字段白名单。不在表里的字段一律拒，
// 因为"能 set 什么"就是写接口的攻击面：rev、id 都不许被 set 改
// （id 改了等于换了一个寻址键，历史与快照会指错条目）。
var groupFields = map[string]bool{"title": true, "layout": true, "accent": true}

var itemFields = map[string]bool{
	"name": true, "url": true, "icon": true, "desc": true,
	"alias": true, "tags": true, "aiNote": true,
}

// ParsePath 认 spec §4.4 的文法：groups/<groupId>[/items[/<itemId>]][/<field>]。
// 段里的 id 必须是 ASCII slug——数组下标在这里没有意义，插入一条之后
// 下标就指错条目了。
func ParsePath(path string) (target, error) {
	parts := strings.Split(strings.TrimSpace(path), "/")
	if parts[0] != "groups" {
		return target{}, errors.New("路径必须以 groups 开头")
	}
	switch len(parts) {
	case 1:
		return target{kind: kindGroups}, nil
	case 2:
		if err := idErr(parts[1]); err != nil {
			return target{}, err
		}
		return target{kind: kindGroup, groupID: parts[1]}, nil
	case 3:
		if err := idErr(parts[1]); err != nil {
			return target{}, err
		}
		if parts[2] == "items" {
			return target{kind: kindItems, groupID: parts[1]}, nil
		}
		if !groupFields[parts[2]] {
			return target{}, fmt.Errorf("分组没有 %q 字段，能改的是 title/layout/accent", parts[2])
		}
		return target{kind: kindGroupField, groupID: parts[1], field: parts[2]}, nil
	case 4:
		if err := idErr(parts[1]); err != nil {
			return target{}, err
		}
		if parts[2] != "items" {
			return target{}, fmt.Errorf("groups/%s 后面要跟 items，得到 %q", parts[1], parts[2])
		}
		if err := idErr(parts[3]); err != nil {
			return target{}, err
		}
		return target{kind: kindItem, groupID: parts[1], itemID: parts[3]}, nil
	case 5:
		if err := idErr(parts[1]); err != nil {
			return target{}, err
		}
		if parts[2] != "items" {
			return target{}, fmt.Errorf("groups/%s 后面要跟 items，得到 %q", parts[1], parts[2])
		}
		if err := idErr(parts[3]); err != nil {
			return target{}, err
		}
		if !itemFields[parts[4]] {
			return target{}, fmt.Errorf("条目没有 %q 字段，能改的是 name/url/icon/desc/alias/tags/aiNote", parts[4])
		}
		return target{kind: kindItemField, groupID: parts[1], itemID: parts[3], field: parts[4]}, nil
	}
	return target{}, fmt.Errorf("路径 %q 段数太多", path)
}

func idErr(s string) error {
	if !model.ValidSlug(s) {
		return fmt.Errorf("id %q 不是 ASCII slug", s)
	}
	return nil
}
```

- [ ] **Step 4: 跑路径测试确认通过**

Run: `go test ./internal/ops/ -run TestParsePath -v`
Expected: 两条 PASS。

- [ ] **Step 5: 写单条 op 的应用测试（先只测 `add` 与 `set`）**

`internal/ops/ops_test.go`：

```go
package ops

import (
	"encoding/json"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func seed() *model.Bookmarks {
	return &model.Bookmarks{
		Rev:   7,
		Title: "发射台",
		Groups: []model.Group{{
			ID: "nas", Title: "NAS", Layout: model.LayoutList,
			Items: []model.Item{
				{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
				{ID: "qbit", Name: "下载", URL: "http://10.0.0.3:8181", Alias: []string{}, Tags: []string{}},
			},
		}},
	}
}

func mustValue(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAddItemAppendsAndInserts(t *testing.T) {
	item := map[string]any{"id": "jellyfin", "name": "影视库", "url": "http://10.0.0.3:8096"}
	b, errs, warns := Run(seed(), []Op{
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, item)},
	})
	if len(errs) != 0 {
		t.Fatalf("add 失败: %+v", errs)
	}
	last := b.Groups[0].Items[len(b.Groups[0].Items)-1]
	if last.ID != "jellyfin" || last.Name != "影视库" {
		t.Fatalf("没追加上新条目: %+v", last)
	}
	// 没带 icon 是可用的（首字母兜底），但必须告警：AI 该知道自己漏了。
	if len(warns) != 1 {
		t.Fatalf("缺 icon 该有一条告警, got %+v", warns)
	}
	// index=0 插到最前面。
	b, errs, _ = Run(seed(), []Op{
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, item), Index: ptr(0)},
	})
	if len(errs) != 0 || b.Groups[0].Items[0].ID != "jellyfin" {
		t.Fatalf("index=0 插入失败: %+v %+v", errs, b.Groups[0].Items)
	}
	// 越界 index 是错误，不是"插到最后"。
	_, errs, _ = Run(seed(), []Op{
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, item), Index: ptr(9)},
	})
	if len(errs) != 1 || errs[0].Field != "index" {
		t.Fatalf("越界 index 该报 index 字段错误: %+v", errs)
	}
}

func TestSetFieldAndNoOpWarning(t *testing.T) {
	b, errs, warns := Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/name", Value: mustValue(t, "Synology DSM")},
		{Op: "set", Path: "groups/nas/title", Value: mustValue(t, "NAS")},
	})
	if len(errs) != 0 {
		t.Fatalf("set 失败: %+v", errs)
	}
	if b.Groups[0].Items[0].Name != "Synology DSM" {
		t.Fatalf("名字没改掉: %+v", b.Groups[0].Items[0])
	}
	// 第二条把 title 设成它本来的值：改了个没变的东西值得一条告警。
	if len(warns) != 1 {
		t.Fatalf("未变化的 set 该告警, got %+v", warns)
	}
	// 类型不对：字段要字符串，给了数组。
	_, errs, _ = Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/name", Value: mustValue(t, []string{"a"})},
	})
	if len(errs) != 1 || errs[0].Field != "name" {
		t.Fatalf("类型不符该报字段错: %+v", errs)
	}
	// id 与 rev 不在白名单里。
	_, errs, _ = Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/id", Value: mustValue(t, "other")},
	})
	if len(errs) != 1 {
		t.Fatalf("set id 必须被拒: %+v", errs)
	}
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 6: 跑测试确认失败**

Run: `go test ./internal/ops/ -run 'TestAdd|TestSet' -v`
Expected: 编译失败（`undefined: Run` / `undefined: Op`）。

- [ ] **Step 7: 写 `ops.go`（本步只到 `add`/`set` 能过，`remove`/`move` 在 Step 9）**

```go
package ops

import (
	"encoding/json"
	"fmt"

	"github.com/rockbenben/launchbay/internal/model"
)

// Op 是一条写请求。Value 保持 RawMessage：字段类型要等到落到 name/url/alias
// 哪一个上才知道，提前解码会把"类型错"报成"body 读不了"。
type Op struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
	Index *int            `json:"index,omitempty"`
	To    string          `json:"to,omitempty"`
}

// Run 把 ops 顺序作用在 b 的副本上。
//
// 返回的 errs 非空时，**结果必须整个丢掉**：spec §4.4 要"全成或全败"。
// 但错误仍然逐条收集而不是首条即返——第 3 条拼错组名不该连带让第 4~30 条
// 的错误看不见，AI 一轮只能修一条的话要跑三十轮。跳过失败的那条继续跑
// 不会掩盖级联错误：后续 op 若依赖失败 op 的产物，它自己也会报错。
//
// 告警不是错误：值没变、缺 icon、非 http 协议都照常写，只是要告诉调用方。
func Run(b *model.Bookmarks, in []Op) (*model.Bookmarks, []Error, []string) {
	out := clone(b)
	var errs []Error
	var warns []string
	last := -1 // 最后一条真正改成了东西的 op，收尾兜底要挂在它身上
	for i, op := range in {
		w, err := applyOne(out, op)
		if err != nil {
			errs = append(errs, Error{OpsIndex: i, Field: fieldOf(op), Reason: err.Error()})
			continue
		}
		last = i
		warns = append(warns, prefix(i, w)...)
	}
	if verr := out.Validate(); verr != nil {
		// 兜底：op 层没拦住但整档不合法（例如 set 出个空 title），
		// 一律按"最后一条 op 之后"的批失败处理。
		errs = append(errs, Error{OpsIndex: len(in) - 1, Field: "bookmarks", Reason: verr.Error()})
	}
	return out, errs, warns
}

func fieldOf(op Op) string {
	if op.Path == "" {
		return "path"
	}
	t, err := ParsePath(op.Path)
	if err != nil || t.field == "" {
		return "path"
	}
	return t.field
}

func prefix(i int, ws []string) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, fmt.Sprintf("op %d: %s", i, w))
	}
	return out
}

// clone 用 JSON 往返做深拷贝。手写逐层 copy 会漏字段（Item 加一个字段
// 就得记得在这里也加），往返一次让 model 自己保证。
func clone(b *model.Bookmarks) *model.Bookmarks {
	raw, err := json.Marshal(b)
	if err != nil {
		// model.Bookmarks 全是可序列化字段，走到这里说明代码写错了，
		// 不是数据问题——没有可恢复的分支，panic 比返回半份数据诚实。
		panic(fmt.Sprintf("克隆书签失败: %v", err))
	}
	var out model.Bookmarks
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(fmt.Sprintf("反克隆书签失败: %v", err))
	}
	return &out
}

func applyOne(b *model.Bookmarks, op Op) ([]string, error) {
	switch op.Op {
	case "add":
		return add(b, op)
	case "set":
		return set(b, op)
	case "remove":
		return remove(b, op)
	case "move":
		return move(b, op)
	}
	return nil, fmt.Errorf("op %q 不存在，只有 add/remove/set/move", op.Op)
}

func findGroup(b *model.Bookmarks, id string) (*model.Group, error) {
	for i := range b.Groups {
		if b.Groups[i].ID == id {
			return &b.Groups[i], nil
		}
	}
	return nil, fmt.Errorf("没有分组 %q", id)
}

func findItem(g *model.Group, id string) (*model.Item, error) {
	for i := range g.Items {
		if g.Items[i].ID == id {
			return &g.Items[i], nil
		}
	}
	return nil, fmt.Errorf("分组 %q 里没有条目 %q", g.ID, id)
}

func add(b *model.Bookmarks, op Op) ([]string, error) {
	t, err := ParsePath(op.Path)
	if err != nil {
		return nil, err
	}
	switch t.kind {
	case kindGroups:
		var g model.Group
		if err := decode(op.Value, &g); err != nil {
			return nil, err
		}
		if g.Layout != model.LayoutTiles && g.Layout != model.LayoutList {
			return nil, fmt.Errorf("新分组的 layout 只能是 tiles/list，得到 %q", g.Layout)
		}
		for _, ex := range b.Groups {
			if ex.ID == g.ID {
				return nil, fmt.Errorf("分组 %q 已存在", g.ID)
			}
		}
		b.Groups = append(b.Groups, g)
		return warnsForItems(g.Items), nil
	case kindItems:
		g, err := findGroup(b, t.groupID)
		if err != nil {
			return nil, err
		}
		var it model.Item
		if err := decode(op.Value, &it); err != nil {
			return nil, err
		}
		if it.ID == "" {
			return nil, fmt.Errorf("新条目必须带 id")
		}
		if dupItem(b, it.ID) {
			return nil, fmt.Errorf("条目 id %q 在本档案里已用过（跨分组也不行）", it.ID)
		}
		at := len(g.Items)
		if op.Index != nil {
			if *op.Index < 0 || *op.Index > len(g.Items) {
				return nil, fmt.Errorf("index %d 越界（0..%d）", *op.Index, len(g.Items))
			}
			at = *op.Index
		}
		g.Items = append(g.Items, model.Item{})
		copy(g.Items[at+1:], g.Items[at:])
		g.Items[at] = it
		return warnsForItem(it), nil
	}
	return nil, fmt.Errorf("add 只能指向 groups 或 groups/<gid>/items")
}

// decode 用严格模式解 op 的 value：未知键直接拒。AI 把 `aiNote` 写成
// `note` 时报"未知字段 note"比悄悄存一条没备注的书签有用。
func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("add 必须带 value")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("value 读不了: %w", err)
	}
	return nil
}

func dupItem(b *model.Bookmarks, id string) bool {
	for _, g := range b.Groups {
		for _, it := range g.Items {
			if it.ID == id {
				return true
			}
		}
	}
	return false
}
```

`ops.go` 的 import 块里要有 `"bytes"`（`decode` 用它把 `RawMessage` 喂给严格解码器）。

- [ ] **Step 8: 补 `set`（Step 7 的 `set` 分支要用它）**

```go
func set(b *model.Bookmarks, op Op) ([]string, error) {
	t, err := ParsePath(op.Path)
	if err != nil {
		return nil, err
	}
	switch t.kind {
	case kindGroupField:
		g, err := findGroup(b, t.groupID)
		if err != nil {
			return nil, err
		}
		var s string
		if err := decode(op.Value, &s); err != nil {
			return nil, err
		}
		var warn []string
		switch t.field {
		case "title":
			warn = changed(g.Title == s)
			g.Title = s
		case "layout":
			if s != model.LayoutTiles && s != model.LayoutList {
				return nil, fmt.Errorf("layout 只能是 tiles/list，得到 %q", s)
			}
			warn = changed(g.Layout == s)
			g.Layout = s
		case "accent":
			warn = changed(g.Accent == s)
			g.Accent = s
		}
		return warn, nil
	case kindItemField:
		g, err := findGroup(b, t.groupID)
		if err != nil {
			return nil, err
		}
		it, err := findItem(g, t.itemID)
		if err != nil {
			return nil, err
		}
		return setItemField(it, t.field, op.Value)
	}
	return nil, fmt.Errorf("set 只能指向 groups/<gid>/<field> 或 groups/<gid>/items/<iid>/<field>")
}

func setItemField(it *model.Item, field string, raw json.RawMessage) ([]string, error) {
	switch field {
	case "name", "url", "icon", "desc", "aiNote":
		var s string
		if err := decode(raw, &s); err != nil {
			return nil, err
		}
		warn := changed(false)
		switch field {
		case "name":
			warn = changed(it.Name == s)
			it.Name = s
		case "url":
			warn = changed(it.URL == s)
			it.URL = s
		case "icon":
			warn = changed(it.Icon == s)
			it.Icon = s
		case "desc":
			warn = changed(it.Desc == s)
			it.Desc = s
		case "aiNote":
			warn = changed(it.AINote == s)
			it.AINote = s
		}
		return warn, nil
	case "alias", "tags":
		var ss []string
		if err := decode(raw, &ss); err != nil {
			return nil, err
		}
		if ss == nil {
			// 显式 null 与 [] 是两件事：写 null 进磁盘就违反了 §4.3。
			ss = []string{}
		}
		var warn []string
		if field == "alias" {
			warn = changed(sameStrings(it.Alias, ss))
			it.Alias = ss
		} else {
			warn = changed(sameStrings(it.Tags, ss))
			it.Tags = ss
		}
		return warn, nil
	}
	return nil, fmt.Errorf("条目没有可 set 的字段 %q", field)
}
```

- [ ] **Step 9: 补 `remove` / `move` 与告警、辅助函数**

```go
func remove(b *model.Bookmarks, op Op) ([]string, error) {
	t, err := ParsePath(op.Path)
	if err != nil {
		return nil, err
	}
	switch t.kind {
	case kindGroup:
		for i, g := range b.Groups {
			if g.ID == t.groupID {
				b.Groups = append(b.Groups[:i], b.Groups[i+1:]...)
				return nil, nil
			}
		}
		return nil, fmt.Errorf("没有分组 %q", t.groupID)
	case kindItem:
		g, err := findGroup(b, t.groupID)
		if err != nil {
			return nil, err
		}
		for i, it := range g.Items {
			if it.ID == t.itemID {
				g.Items = append(g.Items[:i], g.Items[i+1:]...)
				return nil, nil
			}
		}
		return nil, fmt.Errorf("分组 %q 里没有条目 %q", g.ID, t.itemID)
	}
	return nil, fmt.Errorf("remove 只能指向 groups/<gid> 或 groups/<gid>/items/<iid>")
}

// move 把条目挪到另一个分组（同组挪位也走它：to 填本组 id）。
// 位置只由 to + index 决定，绝不用"下标寻址"——那是 §4.4 立意的反面。
func move(b *model.Bookmarks, op Op) ([]string, error) {
	t, err := ParsePath(op.Path)
	if err != nil {
		return nil, err
	}
	if t.kind != kindItem {
		return nil, fmt.Errorf("move 只能指向 groups/<gid>/items/<iid>")
	}
	if op.To == "" {
		return nil, fmt.Errorf("move 必须带 to（目标分组 id）")
	}
	src, err := findGroup(b, t.groupID)
	if err != nil {
		return nil, err
	}
	dst, err := findGroup(b, op.To)
	if err != nil {
		return nil, err
	}
	at := -1
	for i, it := range src.Items {
		if it.ID == t.itemID {
			at = i
			break
		}
	}
	if at < 0 {
		return nil, fmt.Errorf("分组 %q 里没有条目 %q", src.ID, t.itemID)
	}
	item := src.Items[at]
	src.Items = append(src.Items[:at], src.Items[at+1:]...)
	pos := len(dst.Items)
	if op.Index != nil {
		if *op.Index < 0 || *op.Index > len(dst.Items) {
			return nil, fmt.Errorf("index %d 越界（0..%d）", *op.Index, len(dst.Items))
		}
		pos = *op.Index
	}
	dst.Items = append(dst.Items, model.Item{})
	copy(dst.Items[pos+1:], dst.Items[pos:])
	dst.Items[pos] = item
	return nil, nil
}

func warnsForItem(it model.Item) []string {
	var w []string
	if it.Icon == "" {
		w = append(w, fmt.Sprintf("条目 %s 没有 icon，首屏走首字母兜底", it.ID))
	}
	if model.ExternalScheme(it.URL) {
		w = append(w, fmt.Sprintf("条目 %s 的协议浏览器不会从页面跳过去，界面会标「需手动打开」", it.ID))
	}
	return w
}

func warnsForItems(items []model.Item) []string {
	var w []string
	for _, it := range items {
		w = append(w, warnsForItem(it)...)
	}
	return w
}

func changed(noOp bool) []string {
	if noOp {
		return []string{"设的值与当前值相同，什么都没改"}
	}
	return nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 10: 跑测试确认通过**

Run: `go test ./internal/ops/ -v`
Expected: 全 PASS。`remove`/`move` 的测试在 Task 2 补（那里连批语义一起测）。

- [ ] **Step 11: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .   # gofmt 必须无输出
git add internal/ops/
git commit -m "feat(ops): 路径文法与 add/set/remove/move 的纯函数应用"
```

---

## Task 2: 整批语义——全成或全败、逐条错误、级联与告警

**Files:**
- Modify: `internal/ops/ops.go`
- Test: `internal/ops/batch_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Run` / `Op` / `Error`
- Produces: `func ops.Run(b *model.Bookmarks, in []Op) (*model.Bookmarks, []Error, []string)` 的**批语义**（Task 5 依赖：`errs` 非空即整批不写）；`func (Error) String() string`（CLI 干跑打印用）

- [ ] **Step 1: 写批语义测试**

`internal/ops/batch_test.go`：

```go
package ops

import (
	"encoding/json"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 第 1 条失败不许影响第 0 条已经做成的改动——但整批最终仍不落盘，
// 因为调用方（Task 5）看到 errs 非空就丢弃结果。这里同时钉两件事：
// 错误逐条收集、结果仍是"跑完的样子"。
func TestBatchCollectsEveryIndependentError(t *testing.T) {
	b, errs, _ := Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/name", Value: mustValue(t, "改名")},
		{Op: "set", Path: "groups/ghost/items/dsm/name", Value: mustValue(t, "x")},
		{Op: "set", Path: "groups/nas/items/nothing/name", Value: mustValue(t, "y")},
	})
	if len(errs) != 2 {
		t.Fatalf("两条独立错误都要报出来, got %+v", errs)
	}
	if errs[0].OpsIndex != 1 || errs[1].OpsIndex != 2 {
		t.Fatalf("opsIndex 必须指到出错的那条: %+v", errs)
	}
	if b.Groups[0].Items[0].Name != "改名" {
		t.Fatalf("失败不该回滚已成功的条目（整批由调用方丢弃）")
	}
}

// 前一条 add 出来的分组，后一条要能往里加条目：校验必须按"演化中的状态"
// 做，不能拿原始快照判存在性。
func TestBatchSeesItsOwnEarlierWrites(t *testing.T) {
	b, errs, _ := Run(seed(), []Op{
		{Op: "add", Path: "groups", Value: mustValue(t, map[string]any{
			"id": "read", "title": "读", "layout": "tiles"})},
		{Op: "add", Path: "groups/read/items", Value: mustValue(t, map[string]any{
			"id": "weread", "name": "微信读书", "url": "https://weread.qq.com", "icon": "tabler:book"})},
	})
	if len(errs) != 0 {
		t.Fatalf("级联写入该成功: %+v", errs)
	}
	if len(b.Groups) != 2 || len(b.Groups[1].Items) != 1 {
		t.Fatalf("新分组或新条目没落地: %+v", b.Groups)
	}
}

// 级联：第 0 条建组失败（这里用重名），第 1 条往那个组加条目也该报错，
// 而不是静默写进不存在的组。
func TestFailedOpDoesNotFabricateItsDependent(t *testing.T) {
	_, errs, _ := Run(seed(), []Op{
		{Op: "add", Path: "groups", Value: mustValue(t, map[string]any{
			"id": "nas", "title": "重复", "layout": "tiles"})},
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, map[string]any{
			"id": "x1", "name": "x", "url": "http://10.0.0.9", "icon": "tabler:x"})},
	})
	if len(errs) != 1 || errs[0].OpsIndex != 0 {
		t.Fatalf("第 0 条重名该报错，第 1 条往已存在的 nas 加条目该成功: %+v", errs)
	}
}

// 整档不合法（例如把唯一的分组删空之后 title 还在但 layout 写错）
// 必须由收尾的 Validate 抓住。
func TestBatchFailsOnResultThatIsNotValid(t *testing.T) {
	_, errs, _ := Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/url", Value: mustValue(t, "")},
	})
	if len(errs) == 0 {
		t.Fatalf("空 URL 该被拒（model.Validate 兜底）")
	}
}

// 告警不许冒充错误：缺 icon 与非 http 协议都照常写，只是要告诉调用方。
// 用 ftp:// 而不是 magnet:/mailto: ——后者没有主机名，model.Validate 会拒
// （spec §6.2 只拒"空/解析不了/缺主机名"三种，无 host 正是其中一种）。
func TestWarningsDoNotBlockTheWrite(t *testing.T) {
	_, errs, warns := Run(seed(), []Op{
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, map[string]any{
			"id": "share", "name": "共享盘", "url": "ftp://10.0.0.3/pub"})},
	})
	if len(errs) != 0 {
		t.Fatalf("非 http 协议允许存（spec §6.2）: %+v", errs)
	}
	if len(warns) < 2 {
		t.Fatalf("缺 icon + 外部协议该各一条告警, got %+v", warns)
	}
}

// 空批不是"写坏了"，是"什么都没做"：不许报错，也不许有告警。
func TestEmptyBatchIsHarmless(t *testing.T) {
	_, errs, warns := Run(seed(), nil)
	if len(errs) != 0 || len(warns) != 0 {
		t.Fatalf("空批该静默: %+v %+v", errs, warns)
	}
}

// op 的 JSON 形状是对外契约的一部分，键名钉一次。
func TestOpJSONShape(t *testing.T) {
	var o Op
	raw := `{"op":"add","path":"groups/nas/items","value":{"id":"a","name":"b","url":"http://10.0.0.3"},"index":0,"to":"nas"}`
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatal(err)
	}
	if o.Op != "add" || o.Path != "groups/nas/items" || o.Index == nil || *o.Index != 0 || o.To != "nas" {
		t.Fatalf("op JSON 键没解出来: %+v", o)
	}
}

var _ = model.LayoutTiles
```

- [ ] **Step 2: 跑测试看哪几条红**

Run: `go test ./internal/ops/ -run TestBatch -v`
Expected: `TestBatchCollectsEveryIndependentError` 与 `TestEmptyBatchIsHarmless` 红——前者因为 Task 1 的收尾 `Validate` 用了 `len(in)-1`，空批时会算出 `-1`；后者因为空批时 `errs` 里塞了那条兜底。这正是批语义要修的地方。

- [ ] **Step 3: 修收尾兜底**

把 `Run` 里的收尾改成只在"跑完仍不合法"时报错，且 `opsIndex` 指到**最后一条真正改动了结果**的 op：

```go
	if len(in) == 0 {
		return out, nil, nil
	}
	if verr := out.Validate(); verr != nil && last >= 0 {
		// 收尾兜底抓的是"op 层没拦住但整档不合法"。挂在**最后一条真正
		// 改成了东西的 op** 上：`len(in)-1` 会指到一条本来就失败、或根本没
		// 跑过的 op，那是把错误报给无辜的人（评审 Important #1）。
		// field 用 bookmarks 而不是伪造某个字段名——它确实没有唯一责任人。
		errs = append(errs, Error{OpsIndex: last, Field: "bookmarks", Reason: verr.Error()})
	}
```

- [ ] **Step 4: 加 `Error.String()`（CLI 打印与测试可读性都用它）**

```go
func (e Error) String() string {
	return fmt.Sprintf("第 %d 条 op 的 %s：%s", e.OpsIndex, e.Field, e.Reason)
}
```

- [ ] **Step 5: 【故意破坏】确认"逐条收集"不是空话**

把 `Run` 循环里的 `continue` 改成 `return out, errs, warns`（首错即返），跑：

```
go test ./internal/ops/ -run TestBatchCollectsEveryIndependentError -v
```

Expected: 红，且报的是 `两条独立错误都要报出来, got [{...opsIndex:1}]`（只有一条）。看到红之后**把 `continue` 改回去**，再跑一次确认绿。红的原因必须是错误条数，不是别的断言顺带炸。

- [ ] **Step 6: 【故意破坏】确认 `clone` 真在防别名**

把 `clone` 换成 `out := *b`（浅拷贝）。跑 `go test ./internal/ops/ -v`。
Expected: 至少 `TestAddItemAppendsAndInserts` 红——因为 `seed()` 返回的切片被就地改了，同一份夹具在两个用例之间会互相污染。改回去，确认全绿。若浅拷贝下**全绿**，说明测试没钉住不变量，要补一条"跑完 Run 之后原 b 的条目数没变"的断言再重做本步。

- [ ] **Step 7: 补 `remove` / `move` 的用例（Task 1 只实现了、没测）**

Task 1 把 `remove`/`move` 的代码写完了但按它的 Step 10 说法"测试在本案补"。这不是可选项：`move` 同组换位那条走的是 `findGroup` 返回的同一个 `*model.Group` 指针（源与目标是同一张活切片），先删后插的正确性**没有任何测试钉过**，改坏了不会有人知道。

`internal/ops/batch_test.go` 追加（夹具用 Task 1 的 `seed()`：组 `nas` 有 `dsm`、`qbit` 两条）：

```go
func TestRemoveItemAndGroup(t *testing.T) {
	b, errs, _ := Run(seed(), []Op{
		{Op: "remove", Path: "groups/nas/items/dsm"},
	})
	if len(errs) != 0 {
		t.Fatalf("remove 条目失败: %+v", errs)
	}
	if len(b.Groups[0].Items) != 1 || b.Groups[0].Items[0].ID != "qbit" {
		t.Fatalf("该只剩 qbit: %+v", b.Groups[0].Items)
	}
	b, errs, _ = Run(seed(), []Op{
		{Op: "remove", Path: "groups/nas"},
	})
	if len(errs) != 0 || len(b.Groups) != 0 {
		t.Fatalf("remove 整组失败: %+v %+v", errs, b.Groups)
	}
	// 不存在的目标该报错并指到那一条 op，不是静默成功。
	_, errs, _ = Run(seed(), []Op{
		{Op: "remove", Path: "groups/nas/items/ghost"},
	})
	if len(errs) != 1 || errs[0].OpsIndex != 0 {
		t.Fatalf("remove 不存在的条目该红: %+v", errs)
	}
}

// 同组换位：源与目标是同一个 *model.Group。这条是"指针别名有没有写坏"
// 唯一的证人——跨组 move 测不到它。
func TestMoveWithinOneGroupReorders(t *testing.T) {
	b, errs, _ := Run(seed(), []Op{
		{Op: "move", Path: "groups/nas/items/qbit", To: "nas", Index: ptr(0)},
	})
	if len(errs) != 0 {
		t.Fatalf("同组 move 失败: %+v", errs)
	}
	got := []string{}
	for _, it := range b.Groups[0].Items {
		got = append(got, it.ID)
	}
	if strings.Join(got, ",") != "qbit,dsm" {
		t.Fatalf("没换到位: %+v", got)
	}
}

func TestMoveAcrossGroupsAndMissingTargets(t *testing.T) {
	base := seed()
	base.Groups = append(base.Groups, model.Group{
		ID: "read", Title: "读", Layout: model.LayoutTiles,
		Items: []model.Item{{ID: "book", Name: "书", URL: "http://10.0.0.8", Alias: []string{}, Tags: []string{}}},
	})
	b, errs, _ := Run(base, []Op{
		{Op: "move", Path: "groups/nas/items/dsm", To: "read", Index: ptr(0)},
	})
	if len(errs) != 0 {
		t.Fatalf("跨组 move 失败: %+v", errs)
	}
	if len(b.Groups[0].Items) != 1 || b.Groups[1].Items[0].ID != "dsm" {
		t.Fatalf("搬完两边形状不对: %+v %+v", b.Groups[0].Items, b.Groups[1].Items)
	}
	// 目标组不存在 / 缺 to / 越界 index 三样都该指名道姓地红。
	for _, tc := range []struct{ name string; op Op }{
		{"目标组不存在", Op{Op: "move", Path: "groups/nas/items/dsm", To: "ghost"}},
		{"缺 to", Op{Op: "move", Path: "groups/nas/items/dsm"}},
		{"index 越界", Op{Op: "move", Path: "groups/nas/items/dsm", To: "read", Index: ptr(9)}},
	} {
		_, errs, _ := Run(base, []Op{tc.op})
		if len(errs) != 1 {
			t.Fatalf("%s 该报错: %+v", tc.name, errs)
		}
	}
}
```

`strings` 与 `model` 要进 `batch_test.go` 的 import。

- [ ] **Step 8: 【故意破坏】一次——确认同组 move 那条证人真会咬**

把 `move` 里"先从源摘除、再插入目标"的两步顺序调换（先按 `dst.Items` 算好位置插入，再从 `src.Items` 删除），跑 `go test ./internal/ops/ -run TestMoveWithinOneGroupReorders -v`。
Expected: 红（同组时插入把源切片撑长，摘除的位置就错了）。这条红是本案少见的"顺序即正确性"现场，值得一次真破坏。改回后确认全绿。

- [ ] **Step 9: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/ops/
git commit -m "feat(ops): 整批逐条报错与演化态校验，收尾过 model.Validate 兜底"
```

---
## Task 3: 快照与 `history.jsonl` 的磁盘原语

**Files:**
- Create: `internal/store/snapshot.go`
- Create: `internal/store/history.go`
- Modify: `internal/store/encode.go`（加 `EncodeJSONL`）
- Test: `internal/store/snapshot_test.go`
- Test: `internal/store/history_test.go`

**Interfaces:**
- Consumes: `Store.BookmarksPath` / `SnapshotsDir` / `HistoryPath` / `WriteFileAtomic` / `DecodeBookmarks(path string, data []byte)`
- Produces:
  - `func (s *Store) WriteSnapshot(id string) (int64, error)` —— 把**当前** `bookmarks.json` 复制进 `snapshots/<当前 rev>/`，返回被保住的 rev；顺带裁剪到最近 50 份
  - `func (s *Store) ListSnapshots(id string) ([]int64, error)` —— rev 从新到旧
  - `func (s *Store) LoadSnapshot(id string, rev int64) ([]byte, error)` —— 原始字节，交给调用方 `DecodeBookmarks`
  - `func (s *Store) SnapshotCount(id string) int`
  - `type store.HistoryEntry struct { At, Profile, Actor, By string; Rev int64; Ops []HistoryOp }`
  - `type store.HistoryOp struct { Op, Path string }`
  - `func (s *Store) AppendHistory(e HistoryEntry) error`
  - `func (s *Store) ReadHistory(profile string, limit int) ([]HistoryEntry, error)`
  - `var store.ByValues = []string{"ai","bookmarklet","extension","web","import","sync-template"}` 与 `func store.ValidBy(by string) bool`
  - `func store.EncodeJSONL(v any) ([]byte, error)`（包级函数，放 `encode.go`）

- [ ] **Step 1: 写快照测试**

`internal/store/snapshot_test.go`：

```go
package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func seededWithContent(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸"}); err != nil {
		t.Fatal(err)
	}
	b := &model.Bookmarks{Rev: 3, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutTiles,
		Items: []model.Item{{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}}},
	}}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWriteSnapshotCopiesCurrentRev(t *testing.T) {
	s := seededWithContent(t)
	rev, err := s.WriteSnapshot("dad")
	if err != nil {
		t.Fatal(err)
	}
	if rev != 3 {
		t.Fatalf("快照目录名该是被保住的 rev, got %d", rev)
	}
	raw, err := s.LoadSnapshot("dad", 3)
	if err != nil {
		t.Fatal(err)
	}
	// 快照必须与磁盘原件逐字节相同——revert 靠的就是这份字节。
	want, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(want) {
		t.Fatalf("快照与原件不一致")
	}
}

func TestSnapshotOfMissingBookmarksIsNotAnError(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "kid", Title: "小满"}); err != nil {
		t.Fatal(err)
	}
	// 空档案（还没有 bookmarks.json）第一次写入前无东西可保：0 份快照、不是错误。
	rev, err := s.WriteSnapshot("kid")
	if err != nil {
		t.Fatalf("空档案的快照该静默跳过: %v", err)
	}
	if rev != 0 || s.SnapshotCount("kid") != 0 {
		t.Fatalf("空档案不该造出快照: rev=%d count=%d", rev, s.SnapshotCount("kid"))
	}
}

func TestSnapshotsPrunedToNewest50(t *testing.T) {
	s := seededWithContent(t)
	for i := 1; i <= 55; i++ {
		b, err := s.LoadBookmarks("dad")
		if err != nil {
			t.Fatal(err)
		}
		b.Rev = int64(i)
		if err := s.SaveBookmarks("dad", b); err != nil {
			t.Fatal(err)
		}
		if _, err := s.WriteSnapshot("dad"); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListSnapshots("dad")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 50 {
		t.Fatalf("只该留 50 份, got %d", len(list))
	}
	if list[0] != 55 || list[49] != 6 {
		t.Fatalf("该留最新 50 份（55..6）, got %d..%d", list[0], list[49])
	}
	if _, err := os.Stat(filepath.Join(s.SnapshotsDir("dad"), "5")); !os.IsNotExist(err) {
		t.Fatalf("第 5 份该被裁掉, stat err=%v", err)
	}
}

// 非数字目录名（有人手工往里塞了 notes）不许让快照列表整个崩掉，
// 也不许被当成一份可 revert 的快照。
func TestNonNumericSnapshotDirsAreIgnored(t *testing.T) {
	s := seededWithContent(t)
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(s.SnapshotsDir("dad"), "notes")
	if err := os.MkdirAll(stray, 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListSnapshots("dad")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0] != 3 {
		t.Fatalf("got %+v", list)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/store/ -run 'Snapshot|Snapshots' -v`
Expected: 编译失败（`undefined: WriteSnapshot` 等）。

- [ ] **Step 3: 写 `snapshot.go`**

```go
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// snapshotKeep 是 spec §4.1 的"保留 50 份"。它是恢复窗口的全部长度：
// 再多也没人回头翻 51 版，再少会让一次成批误删变成不可恢复。
const snapshotKeep = 50

// WriteSnapshot 把当前的 bookmarks.json 复制进 snapshots/<rev>/。
//
// 必须在任何写入之前调用：快照保的是"这一次写入之前的状态"，反过来
// 先写再快照等于什么都没保。
//
// rev 取磁盘上那份的 rev，不取调用方内存里的：两者不一致时（外部程序
// 改过文件）快照要保的是即将被覆盖的那一份，因为 revert 会拿它覆盖磁盘。
//
// 档案还没有 bookmarks.json（新建档案）时返回 (0, nil)：无东西可保，
// 不是错误——否则第一个 add op 会永远写不进去。
func (s *Store) WriteSnapshot(id string) (int64, error) {
	src := s.BookmarksPath(id)
	raw, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("读 %s 做快照: %w", src, err)
	}
	b, err := DecodeBookmarks(src, raw)
	if err != nil {
		return 0, fmt.Errorf("快照前 %s 就读不了，拒绝写入: %w", src, err)
	}
	dir := filepath.Join(s.SnapshotsDir(id), strconv.FormatInt(b.Rev, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("建快照目录 %s: %w", dir, err)
	}
	if err := s.WriteFileAtomic(filepath.Join(dir, "bookmarks.json"), raw); err != nil {
		return 0, fmt.Errorf("写快照 %s: %w", dir, err)
	}
	if err := s.pruneSnapshots(id); err != nil {
		// 裁剪失败不影响这一次写入的正确性，只让磁盘多留几份：
		// 报出来，但 rev 仍然有效（调用方按"快照成功"处理）。
		return b.Rev, fmt.Errorf("裁剪快照: %w", err)
	}
	return b.Rev, nil
}

func (s *Store) pruneSnapshots(id string) error {
	list, err := s.ListSnapshots(id)
	if err != nil {
		return err
	}
	if len(list) <= snapshotKeep {
		return nil
	}
	for _, rev := range list[snapshotKeep:] {
		p := filepath.Join(s.SnapshotsDir(id), strconv.FormatInt(rev, 10))
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("删旧快照 %s: %w", p, err)
		}
	}
	return nil
}

// ListSnapshots 从新到旧。非数字目录名一律忽略。
func (s *Store) ListSnapshots(id string) ([]int64, error) {
	ents, err := os.ReadDir(s.SnapshotsDir(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("列快照: %w", err)
	}
	var revs []int64
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		rev, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil {
			continue
		}
		revs = append(revs, rev)
	}
	sort.Slice(revs, func(i, j int) bool { return revs[i] > revs[j] })
	return revs, nil
}

func (s *Store) LoadSnapshot(id string, rev int64) ([]byte, error) {
	p := filepath.Join(s.SnapshotsDir(id), strconv.FormatInt(rev, 10), "bookmarks.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("rev %d 的快照不存在: %w", rev, ErrNotFound)
		}
		return nil, fmt.Errorf("读快照 %s: %w", p, err)
	}
	return raw, nil
}

func (s *Store) SnapshotCount(id string) int {
	list, err := s.ListSnapshots(id)
	if err != nil {
		// 计数是给人看的汇总信息，不是判据：列不出来就报 0，
		// 但**别把它当"没有快照"**——真要判断得用 ListSnapshots 的 error。
		return 0
	}
	return len(list)
}
```

- [ ] **Step 4: 跑快照测试确认通过**

Run: `go test ./internal/store/ -run 'Snapshot|Snapshots' -v`
Expected: 全 PASS。

- [ ] **Step 5: 写历史测试**

`internal/store/history_test.go`：

```go
package store

import (
	"os"
	"strings"
	"testing"
)

func TestAppendHistoryWritesOneLinePerEntry(t *testing.T) {
	s := New(t.TempDir())
	for _, rev := range []int64{1, 2, 3} {
		if err := s.AppendHistory(HistoryEntry{
			At: "2026-10-05T20:00:00+08:00", Profile: "dad", Actor: "ai-dad",
			By: "ai", Rev: rev,
			Ops: []HistoryOp{{Op: "set", Path: "groups/nas/items/dsm/name"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(s.HistoryPath())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("三条该三行, got %d: %q", len(lines), string(raw))
	}
	// 非 ASCII 原样、不出现 \uXXXX：与磁盘上书签同一套写出契约。
	if strings.Contains(lines[0], `\u`) {
		t.Fatalf("history 里不许出现 \\u 转义: %s", lines[0])
	}
	// 键序 = struct 字段序，且是紧凑单行。
	if !strings.HasPrefix(lines[0], `{"at":"2026-10-05T20:00:00+08:00","profile":"dad"`) {
		t.Fatalf("键序/形状不对: %s", lines[0])
	}
}

// ops 只存动作与目标路径，不存 value：日志是"谁动过什么"的审计线，
// 不是第二条翻书签的路（spec §4.6）。value 里可能有 URL、描述、备注。
func TestHistoryHasNoOpValues(t *testing.T) {
	s := New(t.TempDir())
	e := HistoryEntry{At: "x", Profile: "dad", Actor: "ai-dad", By: "ai", Rev: 1,
		Ops: []HistoryOp{{Op: "add", Path: "groups/nas/items"}}}
	if err := s.AppendHistory(e); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.HistoryPath())
	if strings.Contains(string(raw), "value") || strings.Contains(string(raw), "http") {
		t.Fatalf("history 里出现了 ops 的内容: %s", raw)
	}
}

// 三条 rev 全为 1、profile 依次 dad/mom/dad：按 rev 排是稳定序=文件序，
// 只有"文件顺序倒过来"才能得到 dad,mom。这条断言专门用来抓
// "顺手按 rev 排序"的实现。
func TestReadHistoryFiltersAndNewestFirst(t *testing.T) {
	s := New(t.TempDir())
	for _, p := range []string{"dad", "mom", "dad"} {
		if err := s.AppendHistory(HistoryEntry{At: "t", Profile: p, Actor: p, By: "web", Rev: 1}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ReadHistory("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d", len(all))
	}
	dad, err := s.ReadHistory("dad", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(dad) != 2 {
		t.Fatalf("按档案筛失败: %+v", dad)
	}
	two, err := s.ReadHistory("", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(two) != 2 || two[0].Profile != "dad" || two[1].Profile != "mom" {
		t.Fatalf("limit 要取最近 N 条且按文件倒序: %+v", two)
	}
}

// 崩溃可能留下半行 JSON。半行只该丢掉那一行，不该让整条历史读不出来
// ——与"坏一个档案不坏全家"同一个取舍。
func TestTornLastLineIsSkippedNotFatal(t *testing.T) {
	s := New(t.TempDir())
	if err := s.AppendHistory(HistoryEntry{At: "t", Profile: "dad", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"at":"t","profile":"da`); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	got, err := s.ReadHistory("", 0)
	if err != nil {
		t.Fatalf("半行不该让整档失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}

// 一次大批次的 op 列表会让 jsonl 那一行远超任何"合理上限"。这条用例是
// "读用 bufio.Reader 而不是带上限的 Scanner"的证人：换成 Scanner 时
// ErrTooLong 会中止整档，这条要么红要么把后面的行全丢掉。
func TestLongHistoryLineIsNotSilentlyDropped(t *testing.T) {
	s := New(t.TempDir())
	e := HistoryEntry{At: "t", Profile: "dad", By: "ai", Rev: 9}
	for i := 0; i < 4000; i++ {
		e.Ops = append(e.Ops, HistoryOp{Op: "set", Path: "groups/nas/items/some-item-id-with-a-longish-slug/name"})
	}
	if err := s.AppendHistory(e); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadHistory("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Ops) != 4000 {
		t.Fatalf("长行被吞了: %d 条记录", len(got))
	}
}

func TestValidBy(t *testing.T) {
	for _, ok := range []string{"ai", "bookmarklet", "extension", "web", "import", "sync-template"} {
		if !ValidBy(ok) {
			t.Fatalf("%q 该合法", ok)
		}
	}
	for _, bad := range []string{"", "AI", "cli", "cron"} {
		if ValidBy(bad) {
			t.Fatalf("%q 该被拒", bad)
		}
	}
}
```

- [ ] **Step 5b: 夹具必须带非 ASCII 与 HTML 敏感字符（否则契约是空断言）**

只用 `dad` / `ai-dad` / `2026-10-05T20:00:00+08:00` 这类纯 ASCII 值时，"不许出现 `\u`"永远为真、`SetEscapeHTML(false)` 从来没被跑到——把它翻成 `SetEscapeHTML(true)` 这条用例照样绿（评审 Important #1）。夹具里必须有一条带中文与 `&` 的值（例如 `Actor: "爸爸"`、`Path: "groups/影音/items/片单清单"`），并断言两件事：磁盘上出现**字面**的中文与 `&`，且不出现反斜杠 u 转义。

- [ ] **Step 6: 跑历史测试确认失败**

Run: `go test ./internal/store/ -run 'History|ValidBy' -v`
Expected: 编译失败（`undefined: HistoryEntry`）。

- [ ] **Step 7: 在 `encode.go` 加 `EncodeJSONL`**

```go
// EncodeJSONL 编一行 JSONL：紧凑、不转义 HTML、非 ASCII 原样、结尾一个换行。
// 不复用 WriteJSON——那是给人读的 2 空格缩进，多行会把 jsonl 的一行拆成
// 几十行，`tail -f` 与逐行解析都跟着废掉。
// json.Encoder.Encode 自带结尾换行，正好一行一条。
func EncodeJSONL(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 8: 写 `history.go`**

```go
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ByValues 是 spec §4.6 的渠道枚举。它是调用方自报的口径，只用于历史
// 筛选与界面展示，**不参与任何权限判定**——所以校验它是为了数据整洁，
// 不是为了安全；别在这条路上长出"未知 by 拒绝启动"之类的东西。
var ByValues = []string{"ai", "bookmarklet", "extension", "web", "import", "sync-template"}

func ValidBy(by string) bool {
	for _, v := range ByValues {
		if v == by {
			return true
		}
	}
	return false
}

// HistoryOp 只记"做了什么、对哪个路径"。没有 value 字段是刻意的：
// 一旦有了它，日志就变成第二份书签数据（还会带出 URL 与备注）。
type HistoryOp struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// HistoryEntry 的字段顺序就是 jsonl 的键顺序（同 §4.3 的写出契约）。
type HistoryEntry struct {
	At      string      `json:"at"`
	Profile string      `json:"profile"`
	Actor   string      `json:"actor"`
	By      string      `json:"by"`
	Rev     int64       `json:"rev"`
	Ops     []HistoryOp `json:"ops"`
}

// AppendHistory 用 O_APPEND 追加一行。它不是原子写：jsonl 是只增的审计线，
// 整档重写反而会在写坏时丢掉全部历史。半行的最坏后果是丢最后一条，
// ReadHistory 会跳过它。
func (s *Store) AppendHistory(e HistoryEntry) error {
	if e.Ops == nil {
		e.Ops = []HistoryOp{}
	}
	line, err := EncodeJSONL(e)
	if err != nil {
		return fmt.Errorf("编码历史行: %w", err)
	}
	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("开 %s: %w", s.HistoryPath(), err)
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("写历史: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("关历史文件: %w", err)
	}
	return nil
}

// ReadHistory 返回**最新在前**的列表，"最新"按文件顺序倒过来算。
//
// 不按 rev 排：rev 是每个档案自己的计数器，dad 的 rev 5 与 mom 的 rev 9
// 之间没有先后关系，跨档案按 rev 排会把审计线搅乱。
// profile 为空 = 全部；limit<=0 = 不限。坏行跳过，整文件读不了才报错。
func (s *Store) ReadHistory(profile string, limit int) ([]HistoryEntry, error) {
	f, err := os.Open(s.HistoryPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("读 %s: %w", s.HistoryPath(), err)
	}
	defer f.Close()

	var out []HistoryEntry
	// 用 bufio.Reader 而不是 Scanner：Scanner 有行长上限，超限是
	// ErrTooLong **且不可恢复**——一条超长行就让整档历史读不出来，
	// 正是"坏一行不许坏全家"反着来的方向（评审 Important #2）。
	// ReadString 没有上限，行长多少就吃多少，坏行只剩"解不开"一种。
	br := bufio.NewReader(f)
	for {
		line, readErr := br.ReadString('\n')
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			var e HistoryEntry
			// 解不开就丢这一行，继续读后面的。
			if json.Unmarshal([]byte(trimmed), &e) == nil && (profile == "" || e.Profile == profile) {
				out = append(out, e)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("扫历史: %w", readErr)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
```

- [ ] **Step 9: 【故意破坏】两次，各红各的**

1. 把 `ReadHistory` 里解析失败的 `continue` 改成 `return nil, fmt.Errorf("坏行")` → `go test ./internal/store/ -run TestTornLastLine` 该红。改回。
2. 把倒序那段换成 `sort.SliceStable(out, func(i,j int) bool { return out[i].Rev > out[j].Rev })`（顺手按 rev 排）→ `TestReadHistoryFiltersAndNewestFirst` 该红在 `two[1].Profile != "mom"`。改回。
3. 把 `ReadHistory` 的读取换回 `bufio.Scanner`（默认 64KB 上限）两种写法各试一次：留着 `sc.Err()` 返回错误 → 该红在「扫历史」那条 Fatalf；把 `sc.Err()` 咽掉 → 该红在「长行之后的记录丢了」。改回。

- [ ] **Step 10: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/store/
git commit -m "feat(store): 写前快照（保留 50 份）与 history.jsonl 逐行审计线"
```

---

## Task 4: 路由表——把注册收敛成可枚举的一份数据

**Files:**
- Create: `internal/httpapi/routes.go`
- Modify: `internal/httpapi/server.go`
- Test: `internal/httpapi/routes_test.go`

**Interfaces:**
- Consumes: 现有 `Server{mux, deps}`、`Handler()` 的 no-store 前缀中间件、`auth.Scopes`/`auth.Session`
- Produces:
  - `type cred int` 与常量 `credNone` / `credScoped` / `credAdmin` / `credSession`
  - `type route struct { Method, Pattern string; Cred cred; Summary string; H http.HandlerFunc }`
  - `func (s *Server) routes() []route` —— 唯一的注册来源
  - `func (s *Server) require(r *http.Request, want cred, profileID string) (actor string, sc auth.Scope, ok bool)` —— 凭证→档案判定的唯一实现，返回的 `actor` 写进历史
  - `func bearer(r *http.Request) string`
  - `func (s *Server) activeIsAdmin(sess auth.Session) bool` —— 跨档案资格只看 `lb_profile`（评审 Critical 后的定案，`sessionIsAdmin` 这个错名已废）
  - `func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request)`

- [ ] **Step 1: 写路由表测试**

`internal/httpapi/routes_test.go`：

```go
package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestRoutesTableIsComplete(t *testing.T) {
	s := New(Dependencies{})
	seen := map[string]bool{}
	for _, rt := range s.routes() {
		key := rt.Method + " " + rt.Pattern
		if seen[key] {
			t.Fatalf("路由表里 %s 出现了两次", key)
		}
		seen[key] = true
		if rt.Summary == "" {
			t.Fatalf("%s 没有 Summary：openapi.json 与 AGENTS.md 都从这里取一句话说明", key)
		}
		if rt.H == nil {
			t.Fatalf("%s 没有 handler", key)
		}
	}
	// P1-① 就有的四条必须在表里，一条都不能被"顺手重构"弄丢。
	for _, want := range []string{"GET /healthz", "POST /pin", "GET /", "POST /api/v1/profiles"} {
		if !seen[want] {
			t.Fatalf("路由表少了 %s", want)
		}
	}
}

// 注册方式换了，行为一个字不许变：/healthz 仍是 text/plain "ok\n"，
// 未知路径仍 404，/api/v1 前缀仍带 no-store。
func TestExistingBehaviorSurvivesTheRefactor(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/healthz", "")
	if rec.Code != 200 || rec.Body.String() != "ok\n" {
		t.Fatalf("/healthz 变了: %d %q", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
	rec = get(h, "/api/v1/nothing-here", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未知 API 路径该 404, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("no-store 中间件掉了: %q", cc)
	}
}

// require 的判据表：这是"AI 误写别人档案"那条防线唯一的实现处。
// 夹具事实：setup() 建 dad(管理员)/kid 两档案，令牌 ai-dad:tok-dad:dad 与
// admin:tok-admin:*。所以 tok-dad 的作用域只有 dad——拿它请求 kid 必须拒，
// 且不许"回落"到 dad。
func TestRequireMatrix(t *testing.T) {
	s, _ := setup(t)
	req := func(tok, cookie string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/data", nil)
		if tok != "" {
			r.Header.Set("Authorization", "Bearer "+tok)
		}
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		return r
	}
	for _, tc := range []struct {
		name            string
		tok, cookie     string
		profile         string
		want            bool
		wantActor       string
	}{
		{"管理员令牌动任何档案", "tok-admin", "", "kid", true, "admin"},
		{"管理员令牌动自己范围内的", "tok-admin", "", "dad", true, "admin"},
		{"档案作用域令牌动别人", "tok-dad", "", "kid", false, ""},
		{"档案作用域令牌动自己", "tok-dad", "", "dad", true, "ai-dad"},
		{"未知令牌", "nope", "", "dad", false, ""},
		{"管理员会话代管", "", "lb_profile=dad; lb_home=dad", "kid", true, "dad"},
		{"管理员会话用管理员令牌位", "", "lb_profile=dad; lb_home=dad", "dad", true, "dad"},
		{"非管理员会话跨档案", "", "lb_profile=kid; lb_home=kid", "dad", false, ""},
		{"非管理员会话动自己", "", "lb_profile=kid; lb_home=kid", "kid", true, "kid"},
		// 下面两行是 Task 4 评审的 Critical 的证人：cookie 不签名，所以
		// "kid 伪造 lb_home=dad"与"dad 真的在代管 kid"是**同一种形状**，
		// 代码分不出来（spec §11 明写不防冒充）。分得清的是结论：
		// 正在用的档案是 kid，就动不了 dad —— 判定只看 Active。
		{"在用 kid + home 是 dad，动 dad 该拒", "", "lb_profile=kid; lb_home=dad", "dad", false, ""},
		{"在用 kid + home 是 dad，动 kid 放行且 actor 记 dad", "", "lb_profile=kid; lb_home=dad", "kid", true, "dad"},
		{"无凭证", "", "", "dad", false, ""},
	} {
		actor, _, ok := s.require(req(tc.tok, tc.cookie), credScoped, tc.profile)
		if ok != tc.want {
			t.Fatalf("%s: ok=%v want %v", tc.name, ok, tc.want)
		}
		if ok && actor != tc.wantActor {
			t.Fatalf("%s: actor=%q want %q（actor 要进历史）", tc.name, actor, tc.wantActor)
		}
	}
}

// credAdmin 与 credSession 各挡一半：档案作用域令牌不许走管理员端点，
// 令牌也不许走"改自己 PIN"。
func TestRequireAdminAndSessionClasses(t *testing.T) {
	s, _ := setup(t)
	r := httptest.NewRequest("POST", "/api/v1/status", nil)
	r.Header.Set("Authorization", "Bearer tok-dad")
	if _, _, ok := s.require(r, credAdmin, "dad"); ok {
		t.Fatal("档案作用域令牌不该过 credAdmin，哪怕 profile 就是它自己的")
	}
	r2 := httptest.NewRequest("POST", "/api/v1/profiles/self/pin", nil)
	r2.Header.Set("Authorization", "Bearer tok-admin")
	if _, _, ok := s.require(r2, credSession, "dad"); ok {
		t.Fatal("令牌不该过 credSession：改自己 PIN 要的是那个人的会话")
	}
}
```

`httptest` 要进 import。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/httpapi/ -run 'TestRoutes|TestExistingBehavior|TestRequireMatrix' -v`
Expected: 编译失败（`undefined: routes` / `undefined: credNone` / `undefined: get`）。`get` 已在 `59c26b3` 落地，所以这里只缺前两个。

- [ ] **Step 3: 写 `routes.go`**

```go
package httpapi

import (
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/auth"
)

// cred 是一个端点接受的凭证类别。写在路由表里而不是散进每个 handler，
// 有两个好处：契约测试能逐条对照 openapi；新加端点时"忘了鉴权"会变成
// 表里一眼看得见的空缺，而不是要等有人来打才知道。
type cred int

const (
	credNone    cred = iota // 不要凭证：/healthz、GET /、/pin、openapi.json
	credScoped              // 令牌或会话，且只能动自己有资格的那个档案
	credAdmin               // 作用域为 * 的令牌，或磁盘上真是管理员的会话
	credSession             // 只有会话（改自己 PIN 这类，令牌不许代改）
)

type route struct {
	Method  string
	Pattern string
	Cred    cred
	Summary string // 一句话说明，openapi.json 与 AGENTS.md 都复用它
	H       http.HandlerFunc
}

// routes 是唯一的注册来源。**别在 New 之外再 mux.HandleFunc**——
// 那会让契约测试与文档漏掉一条，而漏掉的正是 AI 最需要知道的那条。
func (s *Server) routes() []route {
	return []route{
		{Method: "GET", Pattern: "/healthz", Cred: credNone, Summary: "存活探针", H: s.handleHealthz},
		{Method: "GET", Pattern: "/", Cred: credNone, Summary: "选择页或该档案首屏", H: s.handleRoot},
		{Method: "POST", Pattern: "/pin", Cred: credNone, Summary: "用 PIN 进门/切档", H: s.handlePin},
		{Method: "POST", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "建档（可带模板）", H: s.handleCreateProfile},
	}
}

func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// require 回答"这把凭证能不能动 profileID"，返回要写进 history 的 actor。
//
// 三条规则都来自 spec §6.1：
//   - 令牌作用域是具体档案时 profile 必须等于它；`*` 才是管理员令牌。
//   - 会话的权限只看 lb_profile；跨档案要求**正在用的那个档案**（Active）
//     在磁盘上真是管理员。lb_home 不参与判定，只当审计 actor 记名
//     （spec §6.1、§11：伪造 lb_home 拿不到任何东西）。
//   - 不符一律拒绝，**不回落到某个默认档案**。
func (s *Server) require(r *http.Request, want cred, profileID string) (string, auth.Scope, bool) {
	if tok := bearer(r); tok != "" {
		sc, ok := s.deps.Tokens.Authorize(tok)
		if !ok || want == credSession {
			// 令牌不许走只认会话的端点：改自己 PIN 用令牌等于"谁有令牌
			// 谁能改谁"，那正是 §6.1 把 self/pin 划给会话的理由。
			return "", auth.Scope{}, false
		}
		if want == credAdmin && !sc.IsAdmin() {
			return "", sc, false
		}
		if !sc.Allows(profileID) {
			return "", sc, false
		}
		return sc.Name, sc, true
	}
	sess := auth.ReadSession(r)
	if !sess.Valid() {
		return "", auth.Scope{}, false
	}
	if want == credNone {
		return sess.Active, auth.Scope{}, true
	}
	if want == credAdmin && !s.activeIsAdmin(sess) {
		return "", auth.Scope{}, false
	}
	if want == credScoped && profileID == "" {
		// 空申报不是"没申报所以放行"，而是"没申报所以无范围可读"。
		// 要跨档案聚合必须显式改要 credAdmin（Task 6 评审的 Critical
		// 就是这个形状漏出来的：/history 不填 profile 曾让任何会话读到
		// 全家庭审计线）。判据只在这一处，端点不许自己绕。
		return "", auth.Scope{}, false
	}
	if profileID != "" && profileID != sess.Active && !s.activeIsAdmin(sess) {
		return "", auth.Scope{}, false
	}
	// 会话的 actor 是 home：本人时 home==active，代管时 home 才是
	// "动手的那个人"（spec §4.6 要的就是这个）。actor 只进审计线，
	// 不参与上面任何一条判定。
	return sess.Home, auth.Scope{}, true
}

// activeIsAdmin 回答"当前在用的这个档案能不能跨档案动别人的数据"。
//
// 只看 lb_profile 指向的档案（spec §6.1：权限永远只看它；§11：伪造
// lb_home 拿不到单 cookie 时代拿不到的任何东西）。
//
// 这条与 sessionLegal 是**两个问题**，不许合并成一次查表：
//   - sessionLegal 问"会话形状合法吗"——home==active 或 home 是管理员，
//     Home 参与判定是对的（§6.1 的第三种形态）；
//   - activeIsAdmin 问"这把凭证能不能动别人"——只看 Active。
// 把两者揉成一次 Home 查表，等于让 kid 伪一条 lb_home=dad 就拿到 dad 的
// 写权限（Task 4 评审的 Critical 就是这么来的）。
func (s *Server) activeIsAdmin(sess auth.Session) bool {
	if !sess.Valid() {
		return false
	}
	p, err := s.deps.Store.LoadProfile(sess.Active)
	return err == nil && p.Admin
}
```

- [ ] **Step 4: 改 `server.go` 走表注册**

```go
func New(d Dependencies) *Server {
	s := &Server{mux: http.NewServeMux(), deps: d}
	for _, rt := range s.routes() {
		s.mux.HandleFunc(rt.Method+" "+rt.Pattern, rt.H)
	}
	return s
}
```

删掉原来那四行注册与内联的 healthz 闭包，在 `routes.go` 或 `handlers.go` 里补：

```go
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}
```

`Handler()` 与 `apiPrefix` 不动。`handlers.go` 里原有的 `authorize` 改成调用 `bearer(r)`，**不留两份 Bearer 解析实现**。

- [ ] **Step 5: 跑测试**

Run: `go test ./internal/httpapi/ -v`
Expected: 全 PASS——包括 P1-① 已有的那批 handlers 测试（它们就是"行为不变"的证据）。

- [ ] **Step 6: 【故意破坏】两次**

1. 注释掉 `routes()` 里 `POST /api/v1/profiles` 那行 → `TestRoutesTableIsComplete` 该红在"少了 POST /api/v1/profiles"。改回。
2. 把它的 `Summary` 改成空串 → 该红在"没有 Summary"。改回。
3. 把 `require` 里 `if !sc.Allows(profileID)` 那行删掉 → `TestRequireMatrix` 的"档案作用域令牌动别人"该红。改回。

- [ ] **Step 7: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/httpapi/
git commit -m "refactor(httpapi): 注册收敛成路由表，鉴权类别与一句话说明进表"
```

---

## Task 5: `POST /api/v1/batch`——一次写入的全部编排

**Files:**
- Create: `internal/httpapi/batch.go`
- Modify: `internal/httpapi/handlers.go`（`decodeBody` 加严格 UTF-8）
- Test: `internal/httpapi/batch_test.go`

**Interfaces:**
- Consumes: Task 1/2 的 `ops.Run`、Task 3 的 `WriteSnapshot`/`AppendHistory`/`ValidBy`/`ByValues`、Task 4 的 `require`/`bearer`
- Produces:
  - `func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request)`
  - 请求体 `{profile, expectedRev?, by, ops:[...]}`；成功 `200 {profile, rev, applied, warnings[]}`
  - `422 {code, detail, errors:[{opsIndex, field, reason}]}`、`409 {code, detail, currentRev}`、`507 {code, detail}`、`401`/`403`/`404`/`503`
  - `func ops.Describe(in []ops.Op) []ops.Brief` 与 `type ops.Brief struct{Op, Path string}`
  - `func (s *Server) historyActorAndScope(r *http.Request, want cred, profileID string) (actor string, ok bool)`（`require` 的薄封装，只回 actor）

- [ ] **Step 0: 先补 `require` 的 actor 兜底（Task 4 复审带过来的裁定）**

`activeIsAdmin` 不再顺手过 `sessionLegal`，于是 `lb_profile=dad; lb_home=ghost`（ghost 是任意合法 slug）这种形状：写权限成立（dad 在磁盘上确实是管理员，与 §11 "伪 `lb_profile=dad` 本来就已经是 dad" 一致），但**审计 actor 会记成一个不存在的档案名**。不是越权，是审计线说谎——而 Task 5/8/9 都要把这个 actor 写进 `history.jsonl`。

修法（只改 `routes.go` 会话成功分支，权限判定一个字不动）：

```go
	// actor 只进审计线。形状非法（home 缺失/已删除/非管理员而又不等于
	// active）时不记那个无法认领的名字，退回 Active：它才是磁盘上
	// 授权这次写入的档案。权限判定在上面已经做完，这里改不动它。
	if !s.sessionLegal(sess) {
		return sess.Active, auth.Scope{}, true
	}
	return sess.Home, auth.Scope{}, true
```

证人加在 `routes_test.go` 的矩阵里（一行即可）：`lb_profile=dad; lb_home=ghost` 动 `kid` → `ok=true` 且 `wantActor="dad"`。这条同时钉住两件事：越权仍然被拒的判定没被顺手改松，以及 actor 不再说谎。

- [ ] **Step 1: 写测试**

`internal/httpapi/batch_test.go`：

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

const (
	adminTok = "tok-admin" // setup(): admin:tok-admin:*
	dadTok   = "tok-dad"   // setup(): ai-dad:tok-dad:dad
)

// setupBatch 在 setup() 之上给 dad 与 kid 各放一份有内容的书签。
func setupBatch(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	s, h := setup(t)
	if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 1, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList,
		Items: []model.Item{{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBookmarks("kid", &model.Bookmarks{Rev: 1, Title: "小满的", Groups: []model.Group{{
		ID: "g1", Title: "看", Layout: model.LayoutTiles,
		Items: []model.Item{{ID: "cartoon", Name: "动画", URL: "http://10.0.0.9", Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	return s, h
}

func TestBatchHappyPathBumpsRevAndSnapshotsAndLogs(t *testing.T) {
	s, h := setupBatch(t)
	body := `{"profile":"dad","by":"ai","expectedRev":1,"ops":[
	  {"op":"add","path":"groups/nas/items","value":{"id":"jellyfin","name":"影视库","url":"http://10.0.0.3:8096","icon":"simple-icons:jellyfin"}}]}`
	rec := post(h, "/api/v1/batch", body, "", adminTok)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Profile  string   `json:"profile"`
		Rev      int64    `json:"rev"`
		Applied  int      `json:"applied"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Profile != "dad" || out.Rev != 2 || out.Applied != 1 {
		t.Fatalf("响应不对: %+v", out)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 2 || b.Groups[0].Items[1].ID != "jellyfin" {
		t.Fatalf("磁盘没写对: %+v", b.Groups[0].Items)
	}
	// 写前快照保的是 rev 1 那一份。
	raw, err := s.LoadSnapshot("dad", 1)
	if err != nil {
		t.Fatalf("该有 rev1 的快照: %v", err)
	}
	if !strings.Contains(string(raw), `"rev":1`) {
		t.Fatalf("快照里是写入之后的状态，顺序错了: %s", raw)
	}
	hist, err := s.ReadHistory("dad", 0)
	if err != nil || len(hist) != 1 {
		t.Fatalf("历史该有一条: %v %+v", err, hist)
	}
	if hist[0].By != "ai" || hist[0].Actor != "admin" || hist[0].Rev != 2 {
		t.Fatalf("历史内容不对: %+v", hist[0])
	}
	if len(hist[0].Ops) != 1 || hist[0].Ops[0].Op != "add" || hist[0].Ops[0].Path != "groups/nas/items" {
		t.Fatalf("历史该记下动作与路径: %+v", hist[0].Ops)
	}
}

// 全成或全败：第 2 条错，第 1 条也不许留在磁盘上；失败的批次
// 既不留快照也不写历史——什么都没发生才对。
func TestBatchIsAllOrNothing(t *testing.T) {
	s, h := setupBatch(t)
	body := `{"profile":"dad","by":"ai","ops":[
	  {"op":"set","path":"groups/nas/items/dsm/name","value":"改名成功但白做"},
	  {"op":"set","path":"groups/nas/items/ghost/name","value":"x"}]}`
	rec := post(h, "/api/v1/batch", body, "", adminTok)
	if rec.Code != 422 {
		t.Fatalf("want 422, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Errors []struct {
			OpsIndex int    `json:"opsIndex"`
			Field    string `json:"field"`
			Reason   string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) != 1 || out.Errors[0].OpsIndex != 1 {
		t.Fatalf("要逐条指到出错的那条: %+v", out.Errors)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 1 || b.Groups[0].Items[0].Name != "群晖" {
		t.Fatalf("整批失败却把第一条写进去了: rev=%d name=%q", b.Rev, b.Groups[0].Items[0].Name)
	}
	if _, err := s.LoadSnapshot("dad", 1); err == nil {
		t.Fatal("失败的批次不该做快照")
	}
	if hist, _ := s.ReadHistory("dad", 0); len(hist) != 0 {
		t.Fatalf("失败的批次不该写历史: %+v", hist)
	}
}

func TestBatchConflictScopeAndMissing(t *testing.T) {
	_, h := setupBatch(t)
	rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"ai","expectedRev":9,"ops":[]}`, "", adminTok)
	if rec.Code != 409 {
		t.Fatalf("rev 冲突该 409, got %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"currentRev":1`) {
		t.Fatalf("409 要带上现 rev 让调用方重读: %s", rec.Body)
	}
	// ai-dad 的作用域是 dad：写 kid 必须 403，且不能"回落"到 dad。
	rec = post(h, "/api/v1/batch", `{"profile":"kid","by":"ai","ops":[]}`, "", dadTok)
	if rec.Code != 403 {
		t.Fatalf("越作用域该 403, got %d %s", rec.Code, rec.Body)
	}
	// 无凭证 401。
	if rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"ai","ops":[]}`, "", ""); rec.Code != 401 {
		t.Fatalf("无凭证该 401, got %d", rec.Code)
	}
	// 档案不存在 404，与"无权限"分开返回（spec §12）。
	if rec := post(h, "/api/v1/batch", `{"profile":"ghost","by":"ai","ops":[]}`, "", adminTok); rec.Code != 404 {
		t.Fatalf("档案不存在该 404, got %d", rec.Code)
	}
}

func TestBatchRejectsUnknownBy(t *testing.T) {
	_, h := setupBatch(t)
	rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"cron","ops":[]}`, "", adminTok)
	if rec.Code != 422 {
		t.Fatalf("未知 by 该 422, got %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"field":"by"`) {
		t.Fatalf("要点名是哪个字段: %s", rec.Body)
	}
}

// 部署验收遗留 #5：请求体里的非法 UTF-8 不许被 encoding/json 静默
// 替换成 U+FFFD 存进磁盘（实测过一次，盘上落的是 d0 a1 + 两个 U+FFFD）。
func TestBatchRejectsInvalidUTF8Body(t *testing.T) {
	s, h := setupBatch(t)
	raw := "{\"profile\":\"dad\",\"by\":\"ai\",\"ops\":[{\"op\":\"set\"," +
		"\"path\":\"groups/nas/items/dsm/name\",\"value\":\"\xd0\xa1\"}]}"
	rec := post(h, "/api/v1/batch", raw, "", adminTok)
	if rec.Code != 422 {
		t.Fatalf("非法 UTF-8 该 422, got %d %s", rec.Code, rec.Body)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(b.Groups[0].Items[0].Name, '') {
		t.Fatalf("U+FFFD 被写进磁盘了: %q", b.Groups[0].Items[0].Name)
	}
}

// 空 ops 不是错误，但不该产生新 rev / 快照 / 历史：
// 一次什么都没做的写入留在审计线里是噪声。
func TestBatchEmptyOpsIsNoop(t *testing.T) {
	s, h := setupBatch(t)
	rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"ai","ops":[]}`, "", adminTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	b, _ := s.LoadBookmarks("dad")
	if b.Rev != 1 {
		t.Fatalf("空批不该 bump rev: %d", b.Rev)
	}
	if hist, _ := s.ReadHistory("dad", 0); len(hist) != 0 {
		t.Fatalf("空批不该写历史: %+v", hist)
	}
	if n := s.SnapshotCount("dad"); n != 0 {
		t.Fatalf("空批不该做快照: %d 份", n)
	}
}

// 快照写不下去 = 这次写入整个取消（spec §12：不许"跳过快照继续写"）。
// 等价夹具：把 snapshots 路径做成一个普通文件，MkdirAll 必然失败。
// 真实场景是磁盘满；这里不装盘，只验"失败即取消"。
func TestBatchSnapshotFailureRefusesWrite(t *testing.T) {
	s, h := setupBatch(t)
	dir := s.SnapshotsDir("dad") // 父目录 profiles/dad 已由建档建好
	if err := os.WriteFile(dir, []byte("挡路"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"改不动"}]}`
	rec := post(h, "/api/v1/batch", body, "", adminTok)
	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("快照失败该 507, got %d %s", rec.Code, rec.Body)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	if b.Rev != 1 || b.Groups[0].Items[0].Name == "改不动" {
		t.Fatalf("507 却把书签写了: rev=%d", b.Rev)
	}
}

// 会话通路：管理员在自己会话里代管 kid 的档案，历史要落在 kid 上、
// actor 写 dad（"谁动过我的书签"随时可查，spec §4.6）。
func TestBatchViaAdminSessionRecordsActor(t *testing.T) {
	s, h := setupBatch(t)
	rec := post(h, "/pin", `{"profile":"dad","pin":"1234"}`, "", "")
	cm := cookieMap(t, rec)
	cookie := "lb_profile=dad; lb_home=" + cm["lb_home"]
	body := `{"profile":"kid","by":"web","ops":[{"op":"set","path":"groups/g1/items/cartoon/name","value":"改了"}]}`
	rec = post(h, "/api/v1/batch", body, cookie, "")
	if rec.Code != 200 {
		t.Fatalf("管理员会话代管该放行, got %d %s", rec.Code, rec.Body)
	}
	hist, _ := s.ReadHistory("kid", 0)
	if len(hist) != 1 || hist[0].Actor != "dad" || hist[0].Profile != "kid" {
		t.Fatalf("代管要在被管理档案的历史里留名: %+v", hist)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/httpapi/ -run TestBatch -v`
Expected: 全红（路由未注册 → 404）。

- [ ] **Step 3: 给 ops 加 `Brief`/`Describe`**

`internal/ops/ops.go` 追加：

```go
// Brief 是 op 的可公开形状：只有动作与路径。历史的白名单由类型系统保证——
// 这里根本没有 value 可以漏出去。
type Brief struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

func Describe(in []Op) []Brief {
	out := make([]Brief, 0, len(in))
	for _, o := range in {
		out = append(out, Brief{Op: o.Op, Path: o.Path})
	}
	return out
}
```

`internal/ops` 不许 import `internal/store`（纯函数层）。磁盘形状 `store.HistoryOp` 的转换写在 `batch.go`，**一处**：

```go
func historyOps(briefs []ops.Brief) []store.HistoryOp {
	out := make([]store.HistoryOp, 0, len(briefs))
	for _, b := range briefs {
		out = append(out, store.HistoryOp{Op: b.Op, Path: b.Path})
	}
	return out
}
```

- [ ] **Step 4: 写 `batch.go`**

```go
package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

type batchRequest struct {
	Profile     string   `json:"profile"`
	ExpectedRev *int64   `json:"expectedRev,omitempty"`
	By          string   `json:"by"`
	Ops         []ops.Op `json:"ops"`
}

type batchResponse struct {
	Profile  string   `json:"profile"`
	Rev      int64    `json:"rev"`
	Applied  int      `json:"applied"`
	Warnings []string `json:"warnings"`
}

// handleBatch 是本案唯一改书签的入口（CLI、sync-template 也都走它）。
//
// 顺序照 spec §4.4：锁 → 读 → 校验 → 快照 → 应用 → 写 → rev+1 → 历史。
// 校验排在快照之前：失败的批次不该在 snapshots/ 里留一份"什么都没改"的
// 副本，那会把 50 份的窗口填满足是无效现场。
func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	var in batchRequest
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if !model.ValidSlug(in.Profile) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "profile", Reason: "必须是 ASCII slug"}})
		return
	}
	if !store.ValidBy(in.By) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "by",
			Reason: "只能是 " + strings.Join(store.ByValues, "/")}})
		return
	}
	actor, _, ok := s.require(r, credScoped, in.Profile)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}

	var (
		resp     batchResponse
		conflict *int64
		verrs    []ops.Error
		missing  bool
		broken   string
		snapErr  error
	)
	err := s.deps.Store.WithProfileLock(in.Profile, func() error {
		snap, err := s.deps.Store.Current(in.Profile)
		switch {
		case errors.Is(err, store.ErrNotFound):
			missing = true
			return nil
		case err != nil:
			if reason, isBroken := s.deps.Store.BrokenReason(in.Profile); isBroken {
				broken = reason
				return nil
			}
			return err
		}
		if in.ExpectedRev != nil && *in.ExpectedRev != snap.Rev {
			c := snap.Rev
			conflict = &c
			return nil
		}
		if len(in.Ops) == 0 {
			resp = batchResponse{Profile: in.Profile, Rev: snap.Rev, Applied: 0, Warnings: []string{}}
			return nil
		}
		next, errs, warns := ops.Run(snap.B, in.Ops)
		if len(errs) > 0 {
			verrs = errs
			return nil
		}
		// 快照错误要分诊，不能一律当 507（Task 5 评审的 Important）：
		// rev>0 说明这份快照已经落地，只是裁剪失败——按"快照成功"继续写，
		// 把裁剪错误降级成告警；rev==0 且报错才是真写不下去 → 507。
		// store 侧的契约本来就是这么写的（snapshot.go：裁剪失败照报但
		// rev 有效），这里不照做就会把"磁盘还剩一点空间"报成"写不进去"。
		// 注意用赋值而不是 `:=`：闭包外已经声明了 snapErr，写成
		// `snapRev, snapErr := ...` 会**新声明一个同名局部变量**，
		// 外层那个永远是 nil——507 分支就此变死码（Task 5 修复轮实测）。
		var snapRev int64
		snapRev, snapErr = s.deps.Store.WriteSnapshot(in.Profile)
		if snapErr != nil && snapRev == 0 {
			return nil // 真写不下去：snapErr 已置位，收尾按 507 处理
		}
		if snapErr != nil {
			warns = append(warns, "旧快照裁剪失败: "+snapErr.Error())
			snapErr = nil
		}
		next.Rev = snap.Rev + 1
		if err := s.deps.Store.SaveBookmarks(in.Profile, next); err != nil {
			return err
		}
		resp = batchResponse{Profile: in.Profile, Rev: next.Rev, Applied: len(in.Ops), Warnings: warns}
		if resp.Warnings == nil {
			resp.Warnings = []string{}
		}
		histErr := s.deps.Store.AppendHistory(store.HistoryEntry{
			At:      time.Now().Format(time.RFC3339),
			Profile: in.Profile,
			Actor:   actor,
			By:      in.By,
			Rev:     next.Rev,
			Ops:     historyOps(ops.Describe(in.Ops)),
		})
		if histErr != nil {
			// 书签已经写成功了。这时候报 5xx 会诱导调用方重试、把同一批
			// op 再应用一次——那才是真正的数据事故。所以：照实返回成功，
			// 历史缺失进 warnings 并打到 stderr。审计线少一行不该让
			// "没人敢用这个接口"。
			resp.Warnings = append(resp.Warnings, "历史没写进去（书签已写入）")
			fmt.Fprintf(os.Stderr, "history append failed profile=%s: %v\n", in.Profile, histErr)
		}
		return nil
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	switch {
	case missing:
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "detail": "档案不存在"})
	case broken != "":
		// 坏档案不是 500（不是服务的错）也不是 422（不是这次请求的错）。
		// 503 + store 给的文件名与行号：AI 拿着它能直接去修那个文件。
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"code": 503, "detail": broken})
	case conflict != nil:
		writeJSON(w, http.StatusConflict, map[string]any{
			"code": 409, "detail": "rev 已变，请重读后重试", "currentRev": *conflict})
	case len(verrs) > 0:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"code": 422, "detail": "ops 校验未过，整批未写入", "errors": verrs})
	case snapErr != nil:
		writeJSON(w, http.StatusInsufficientStorage, map[string]any{
			"code": 507, "detail": "快照写不下去，本次写入已取消"})
	default:
		writeJSON(w, http.StatusOK, resp)
	}
}

// writeAuthFailure 把"没认出来"与"认出来但没资格"分开：前者 401，后者 403。
// 合并成 403 会让调用方拿着一个死令牌反复重试（Task 5 评审的 Important）。
func (s *Server) writeAuthFailure(w http.ResponseWriter, r *http.Request) {
	if tok := bearer(r); tok != "" {
		if _, ok := s.deps.Tokens.Authorize(tok); !ok {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Detail: "令牌无效或已吊销"})
			return
		}
		writeJSON(w, http.StatusForbidden, errorResponse{Code: 403, Detail: "凭证作用域不含该档案"})
		return
	}
	if !auth.ReadSession(r).Valid() {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Detail: "需要令牌或会话"})
		return
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Code: 403, Detail: "凭证作用域不含该档案"})
}

// errorResponse 是全部错误分支的唯一形状：struct 字段序 = 输出键序
// （§4.3 那条"禁止用 map 产生输出键序"对 API 响应同样成立），
// 也让一个端点的多种 422 不会漂成两种形状。
type errorResponse struct {
	Code       int        `json:"code"`
	Detail     string     `json:"detail"`
	CurrentRev *int64     `json:"currentRev,omitempty"`
	Errors     []ops.Error `json:"errors,omitempty"`
}
```

- [ ] **Step 5: 严格 UTF-8 落进 `decodeBody`**

`decodeBody` 改成先读满、再验字节、再解：

```go
func decodeBody(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return fmt.Errorf("读请求体: %w", err)
	}
	if len(body) > maxBodyBytes {
		return fmt.Errorf("请求体超过 %d 字节上限", maxBodyBytes)
	}
	// encoding/json 会把非法 UTF-8 静默替换成 U+FFFD 再交给我们存盘：
	// 一次编码不端的调用就永久改掉一个书签的名字。宁可拒掉。
	if !utf8.Valid(body) {
		return fmt.Errorf("请求体不是合法 UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
```

这条改动同时作用于 `/pin` 的 JSON 通路与 `/api/v1/profiles`——所有 POST 端点都拒非法 UTF-8。这是想要的效果，不是副作用；P1-① 已有测试发的都是合法 UTF-8，必须仍然全绿。

- [ ] **Step 6: 注册路由**

```go
		{Method: "POST", Pattern: "/api/v1/batch", Cred: credScoped,
			Summary: "一批 ops 原子写入某档案", H: s.handleBatch},
```

- [ ] **Step 7: 跑测试**

Run: `go test ./internal/httpapi/ -run TestBatch -v`
Expected: 全 PASS。

- [ ] **Step 8: 【故意破坏】三次，各红各的**

1. 把 `WriteSnapshot` 挪到 `SaveBookmarks` 之后 → `TestBatchHappyPathBumpsRevAndSnapshotsAndLogs` 该红在"快照里是写入之后的状态"。改回。
2. 删掉 `if len(errs) > 0 { verrs = errs; return nil }` 里的 `return nil` → `TestBatchIsAllOrNothing` 该红在"整批失败却把第一条写进去了"。改回。
3. 把 `case snapErr != nil:` 分支改成"照样 200" → `TestBatchSnapshotFailureRefusesWrite` 该红。改回。

- [ ] **Step 9: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/httpapi/ internal/ops/
git commit -m "feat(httpapi): /api/v1/batch 编排锁-校验-快照-写-历史，请求体严格 UTF-8"
```

---

## Task 6: 读端点 `/data`、`/export`、`/history`

**Files:**
- Create: `internal/httpapi/read.go`
- Test: `internal/httpapi/read_test.go`

**Interfaces:**
- Consumes: `Store.Current` / `BookmarksPath` / `ReadHistory` / `BrokenReason`、`require`、`bearer`
- Produces:
  - `func (s *Server) targetProfile(w http.ResponseWriter, r *http.Request, want cred) (string, bool)` —— query 里的 profile + 作用域判定，缺/坏一律 400，不回落
  - `GET /api/v1/data?profile=` → `200 {profile, rev, modified, data}`
  - `GET /api/v1/export?profile=` → `200` 磁盘原字节 + `Content-Disposition: attachment; filename="<id>-bookmarks.json"`
  - `GET /api/v1/history?profile=&limit=` → `200 {entries:[...]}`
  - 坏档案 → `503 {code, detail}`（detail 是 store 的文件名+行号原因，不含磁盘路径）

- [ ] **Step 1: 写测试**

`internal/httpapi/read_test.go`：

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// getBearer 是给只带令牌不带 cookie 的用例用的。
func getBearer(h http.Handler, path, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestReadDataNeedsCredentials(t *testing.T) {
	_, h := setupBatch(t)
	for _, tc := range []struct {
		name   string
		qs     string
		bearer string
		cookie string
		want   int
	}{
		{"匿名", "?profile=dad", "", "", 401},
		{"作用域外令牌", "?profile=kid", dadTok, "", 403},
		{"管理员令牌", "?profile=dad", adminTok, "", 200},
		{"档案不存在", "?profile=ghost", adminTok, "", 404},
		{"缺 profile", "", adminTok, "", 400},
		{"profile 不是 slug", "?profile=%2B%2B", adminTok, "", 400},
		{"会话本人", "?profile=dad", "", "lb_profile=dad; lb_home=dad", 200},
	} {
		var rec *httptest.ResponseRecorder
		if tc.bearer != "" {
			rec = getBearer(h, "/api/v1/data"+tc.qs, tc.bearer)
		} else {
			rec = get(h, "/api/v1/data"+tc.qs, tc.cookie)
		}
		if rec.Code != tc.want {
			t.Fatalf("%s want %d got %d %s", tc.name, tc.want, rec.Code, rec.Body)
		}
	}
}

func TestReadDataShape(t *testing.T) {
	_, h := setupBatch(t)
	rec := getBearer(h, "/api/v1/data?profile=dad", adminTok)
	var out struct {
		Profile  string          `json:"profile"`
		Rev      int64           `json:"rev"`
		Modified string          `json:"modified"`
		Data     model.Bookmarks `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("不是合法 JSON: %v %s", err, rec.Body)
	}
	if out.Profile != "dad" || out.Rev != 1 || len(out.Data.Groups) != 1 {
		t.Fatalf("got %+v", out)
	}
	if out.Modified == "" {
		t.Fatal("modified 要给 RFC3339，AI 用它判断数据新旧")
	}
	// 顶层键序 profile, rev, modified, data 写死一次：防有人改成 map 输出
	// （§4.3 的"禁止用 map 产生输出键序"对 API 响应同样成立）。
	if raw := rec.Body.String(); !strings.HasPrefix(raw, `{"profile":"dad","rev":1`) {
		t.Fatalf("键序变了: %.60s", raw)
	}
}

// export 的"与磁盘内容一致"要能逐字节比：AI 拿它做改前/改后 diff。
func TestExportIsByteIdenticalToDisk(t *testing.T) {
	s, h := setupBatch(t)
	rec := getBearer(h, "/api/v1/export?profile=dad", adminTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	want, err := os.ReadFile(s.BookmarksPath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Body.String() != string(want) {
		t.Fatalf("export 与磁盘不一致")
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="dad-bookmarks.json"` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.AppendHistory(store.HistoryEntry{At: "t", Profile: "dad", Actor: "admin", By: "ai", Rev: 2}); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/history?profile=dad&limit=5", adminTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Entries []store.HistoryEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Entries) != 1 || out.Entries[0].Rev != 2 {
		t.Fatalf("got %+v", out.Entries)
	}
	// 空历史该是 []，不是 null（§4.3 对 API 同样成立）。
	rec = getBearer(h, "/api/v1/history?profile=kid", adminTok)
	if !strings.Contains(rec.Body.String(), `"entries":[]`) {
		t.Fatalf("空集写成 null 了: %s", rec.Body)
	}
	// limit 不是数字 → 400。"0 = 全部"与"填错了"是两件事。
	if rec := getBearer(h, "/api/v1/history?limit=abc", adminTok); rec.Code != 400 {
		t.Fatalf("坏 limit 该 400, got %d", rec.Code)
	}
	// 档案作用域的令牌只能看自己的历史。
	if rec := getBearer(h, "/api/v1/history?profile=kid", dadTok); rec.Code != 403 {
		t.Fatalf("越作用域该 403, got %d", rec.Code)
	}
}

// 坏档案的读端点该给 503 + 文件名与行号：不是 500（不是我们的错）、
// 不是 404（档案在，只是读不了）、更不回显磁盘绝对路径。
func TestReadBrokenProfileIs503WithReason(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte("{\n  \"rev\": 1,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/data?profile=dad", adminTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "bookmarks.json") {
		t.Fatalf("要报出是哪个文件: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), s.Root()) {
		t.Fatalf("不许把磁盘绝对路径回显出去: %s", rec.Body)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/httpapi/ -run 'TestRead|TestExport|TestHistory' -v`
Expected: 全红（404）。

- [ ] **Step 3: 写 `read.go`**

```go
package httpapi

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// targetProfile 从 query 取 profile 并过作用域。缺 profile 是 400：
// "读哪个档案"没有默认值（spec §6.1 明写不回落）。
func (s *Server) targetProfile(w http.ResponseWriter, r *http.Request, want cred) (string, bool) {
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "要带 ?profile=<id>"})
		return "", false
	}
	if !model.ValidSlug(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "profile 不是 ASCII slug"})
		return "", false
	}
	if _, _, ok := s.require(r, want, id); !ok {
		s.writeAuthFailure(w, r)
		return "", false
	}
	return id, true
}

// dataResponse 用 struct 不用 map：键序由字段序决定（§4.3 的写出契约
// 对 API 响应同样成立，AI 会 diff 它）。
type dataResponse struct {
	Profile  string           `json:"profile"`
	Rev      int64            `json:"rev"`
	Modified string           `json:"modified"`
	Data     *model.Bookmarks `json:"data"`
}

func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	id, ok := s.targetProfile(w, r, credScoped)
	if !ok {
		return
	}
	snap, err := s.deps.Store.Current(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "detail": "档案不存在"})
	case err != nil:
		if reason, isBroken := s.deps.Store.BrokenReason(id); isBroken {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"code": 503, "detail": reason})
			return
		}
		writeInternalError(w, err)
	default:
		writeJSON(w, http.StatusOK, dataResponse{
			Profile:  id,
			Rev:      snap.Rev,
			Modified: snap.Modified.UTC().Format(time.RFC3339),
			Data:     snap.B,
		})
	}
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	id, ok := s.targetProfile(w, r, credScoped)
	if !ok {
		return
	}
	if _, err := s.deps.Store.LoadProfile(id); errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "detail": "档案不存在"})
		return
	}
	raw, err := os.ReadFile(s.deps.Store.BookmarksPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			// 空档案没有 bookmarks.json。给一份"合法的空"而不是 404：
			// 调用方要的是能喂回 batch 的东西。
			raw = []byte("{}\n")
		} else {
			writeInternalError(w, err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`-bookmarks.json"`)
	_, _ = w.Write(raw)
}

func (s *Server) handleHistoryAPI(w http.ResponseWriter, r *http.Request) {
	// ?profile= 可缺：管理员看全家庭审计线，其他人只能看自己的。
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	if id != "" && !model.ValidSlug(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "profile 不是 ASCII slug"})
		return
	}
	limit := 0
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "limit 要是非负整数"})
			return
		}
		limit = n
	}
	// 不填 profile = 要全家庭审计线，那是**跨档案聚合信息**，只给管理员
	// （spec §6.1 对 §status 的定位同样管这里）。留空当"不申报"从而跳过
	// 作用域检查，等于任何 kid 会话都能读到别人的动作线（Task 6 评审的
	// Critical）。申报了就按申报的那个档案走常规作用域判定。
	want := credScoped
	if id == "" {
		want = credAdmin
	}
	if _, _, ok := s.require(r, want, id); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	entries, err := s.deps.Store.ReadHistory(id, limit)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if entries == nil {
		entries = []store.HistoryEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
```

`export` 不重新编码、只搬字节：它的契约就是"与磁盘一致"，AI 拿它做改前/改后 diff，走一遍内存模型就会因键序与缩进而对不上。空档案给 `{}\n` 是例外（没有磁盘文件可比）。

- [ ] **Step 4: 注册三条路由**

```go
		{Method: "GET", Pattern: "/api/v1/data", Cred: credScoped, Summary: "该档案全量数据 + rev", H: s.handleData},
		{Method: "GET", Pattern: "/api/v1/export", Cred: credScoped, Summary: "导出磁盘上的 bookmarks.json 原字节", H: s.handleExport},
		{Method: "GET", Pattern: "/api/v1/history", Cred: credScoped, Summary: "动作审计线（不含 ops 内容）", H: s.handleHistoryAPI},
```

- [ ] **Step 4b: 会话通路的证人（Task 6 评审的 Critical：`/history` 一条 cookie 用例都没有）**

`read_test.go` 现有用例全走 bearer，所以"任何 kid 会话不带 `?profile=` 就能读到全家庭审计线"这个洞在绿测试下活了下来。补这几条，形状必须齐：

```go
// 审计线的可见性按"谁在用"判，不是按"有没有申报"判。
func TestHistorySessionVisibility(t *testing.T) {
	s, h := setupBatch(t)
	if err := s.AppendHistory(store.HistoryEntry{At: "t", Profile: "kid", Actor: "kid", By: "web", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(store.HistoryEntry{At: "t", Profile: "dad", Actor: "dad", By: "web", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	kid := "lb_profile=kid; lb_home=kid"
	dad := "lb_profile=dad; lb_home=dad"
	for _, tc := range []struct {
		name, qs, cookie string
		want             int
	}{
		{"kid 不申报（要全家庭）该拒", "", kid, 403},
		{"kid 报自己", "?profile=kid", kid, 200},
		{"kid 报别人", "?profile=dad", kid, 403},
		{"dad 不申报（管理员聚合）放行", "", dad, 200},
		{"匿名不申报该 401", "", "", 401},
	} {
		if rec := get(h, "/api/v1/history"+tc.qs, tc.cookie); rec.Code != tc.want {
			t.Fatalf("%s want %d got %d %s", tc.name, tc.want, rec.Code, rec.Body)
		}
	}
}
```

并在 `routes_test.go` 的 `TestRequireMatrix` 里加一行，把"空申报"这个形状在**唯一的判据处**钉住（否则下一个用 `require` 的端点会重新继承同一个洞）：`{name: "kid 会话空申报不算越权放行", cookie: "lb_profile=kid; lb_home=kid", profile: "", want: false}` —— 走 `credScoped` 时空申报必须拒；`/history` 的聚合读是显式改走 `credAdmin` 才放行的。

同一步补 `/export` 的两条未测分支（评审 Important）：`?profile=ghost` → 404；档案存在但没有 `bookmarks.json` → 200 且 body 是 `{}\n` 并带 `Content-Disposition`。

- [ ] **Step 5: 跑测试 + 【故意破坏】一次**

Run: `go test ./internal/httpapi/ -run 'TestRead|TestExport|TestHistory' -v` → 全 PASS。

破坏：把 `handleExport` 改成"从内存重新编码"（`snap, _ := s.deps.Store.Current(id); writeJSON(w, 200, snap.B)`）→ `TestExportIsByteIdenticalToDisk` 该红（缩进/键序/`rev` 位置都会不一样）。这条用例存在的意义就是防这个"看起来等价的优化"。改回。

- [ ] **Step 6: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/httpapi/
git commit -m "feat(httpapi): /data /export /history 三个读端点，坏档案给 503+文件名行号"
```

---
## Task 7: `POST /api/v1/revert` 与 `launchbay revert`

**Files:**
- Create: `internal/httpapi/revert.go`
- Create: `cmd/launchbay/client.go`
- Create: `cmd/launchbay/cli.go`
- Modify: `cmd/launchbay/main.go`（子命令分发）
- Test: `internal/httpapi/revert_test.go`
- Test: `cmd/launchbay/client_test.go`
- Test: `cmd/launchbay/cli_test.go`

**Interfaces:**
- Consumes: Task 3 的 `LoadSnapshot` / `WriteSnapshot` / `AppendHistory`、Task 4 的 `require`、Task 5 的 `historyOps`、`store.DecodeBookmarks(path, data)`
- Produces:
  - `POST /api/v1/revert?profile=<id>&to-rev=<n>` → `200 {profile, rev, restoredFrom}`；`404`（档案或快照不存在）、`403`/`401`、`400`（参数缺/不是整数）、`507`（快照写不下去）
  - `func (s *Server) handleRevert(w http.ResponseWriter, r *http.Request)`
  - `type cli.client struct { base, token string; hc *http.Client }`
  - `func cliNew(base, token string) *client`（同包，无新目录）
  - `func (c *client) getJSON(path string, v any) error` / `postJSON(path string, body, v any) error`
  - `func runCLI(args []string, out io.Writer) int`（`main.go` 调它；返回退出码）
  - `launchbay revert --profile dad --to 12`

**一处 spec 缺口，实现者照此办理（Task 13 会把它写回 spec）**：§4.6 的 `by` 枚举是 `ai|bookmarklet|extension|web|import|sync-template`，**没有覆盖"人在终端跑 CLI"**；而 `revert` 也不是一条书签 op。本案的取法：`by` 加一个 `"cli"` 值（`store.ByValues` 与 `ValidBy` 同步），历史里 revert 记成一条 `ops:[{op:"revert", path:"snapshots/<to-rev>"}]`——`op` 在历史里描述"做了什么动作"，不必挤进写 ops 的四种。`openapi.json` 的 `by` 枚举（Task 11）也照七个值写。

- [ ] **Step 1: 写 revert 端点测试**

`internal/httpapi/revert_test.go`：

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestRevertRestoresOlderStateAsANewRev(t *testing.T) {
	s, h := setupBatch(t)
	// rev1 已在 setupBatch 里；先做一次写入造出 rev2，快照保住 rev1。
	body := `{"profile":"dad","by":"ai","ops":[{"op":"set","path":"groups/nas/items/dsm/name","value":"改过的"}]}`
	if rec := post(h, "/api/v1/batch", body, "", adminTok); rec.Code != 200 {
		t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
	}
	b, _ := s.LoadBookmarks("dad")
	if b.Rev != 2 || b.Groups[0].Items[0].Name != "改过的" {
		t.Fatalf("前置状态不对: rev=%d", b.Rev)
	}
	rec := post(h, "/api/v1/revert?profile=dad&to-rev=1", "", "", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Profile      string `json:"profile"`
		Rev          int64  `json:"rev"`
		RestoredFrom int64  `json:"restoredFrom"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Rev != 3 || out.RestoredFrom != 1 {
		t.Fatalf("revert 该是一次新写入（rev 前进）: %+v", out)
	}
	b, _ = s.LoadBookmarks("dad")
	if b.Rev != 3 || b.Groups[0].Items[0].Name != "群晖" {
		t.Fatalf("内容没回到 rev1: rev=%d name=%q", b.Rev, b.Groups[0].Items[0].Name)
	}
	// 不抹历史：现在该有两条（batch 一条、revert 一条）。
	hist, _ := s.ReadHistory("dad", 0)
	if len(hist) != 2 {
		t.Fatalf("历史该保留并追加: %+v", hist)
	}
	if hist[0].Ops[0].Op != "revert" || hist[0].Ops[0].Path != "snapshots/1" {
		t.Fatalf("revert 该在历史里自成一式: %+v", hist[0].Ops)
	}
	// revert 之前的状态本身也要保住：rev2 的快照该存在。
	if _, err := s.LoadSnapshot("dad", 2); err != nil {
		t.Fatalf("revert 前没给当前状态做快照: %v", err)
	}
}

func TestRevertArgumentAndScope(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, qs, bearer, cookie string
		want                     int
	}{
		{"缺 profile", "?to-rev=1", adminTok, "", 400},
		{"缺 to-rev", "?profile=dad", adminTok, "", 400},
		{"to-rev 不是整数", "?profile=dad&to-rev=abc", adminTok, "", 400},
		{"快照不存在", "?profile=dad&to-rev=99", adminTok, "", 404},
		{"档案不存在", "?profile=ghost&to-rev=1", adminTok, "", 404},
		{"作用域外令牌", "?profile=kid&to-rev=1", dadTok, "", 403},
		{"无凭证", "?profile=dad&to-rev=1", "", "", 401},
		{"管理员会话代管（kid 无快照）", "?profile=kid&to-rev=1", "", "lb_profile=dad; lb_home=dad", 404},
	} {
		var rec *httptest.ResponseRecorder
		if tc.bearer != "" {
			rec = getBearer(h, "/api/v1/revert"+tc.qs, tc.bearer)
		} else {
			rec = getWithMethod(h, "POST", "/api/v1/revert"+tc.qs, tc.cookie)
		}
		if rec.Code != tc.want {
			t.Fatalf("%s want %d got %d %s", tc.name, tc.want, rec.Code, rec.Body)
		}
	}
}

// revert 只回书签：profile.json 的偏好不在恢复范围内（spec §4.6）。
func TestRevertDoesNotTouchProfileJSON(t *testing.T) {
	s, h := setupBatch(t)
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	p.Title = "改过的名字"
	p.Theme = "light"
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	// 再改一次偏好，然后 revert 书签。
	p2, _ := s.LoadProfile("dad")
	p2.Title = "又改了一次"
	if err := s.SaveProfile(p2); err != nil {
		t.Fatal(err)
	}
	if rec := getWithMethod(h, "POST", "/api/v1/revert?profile=dad&to-rev=1", ""); rec.Code != 200 {
		// 没有 rev1 快照（setupBatch 只写了书签，没快照）→ 404 也算通路正常
		t.Logf("revert 返回 %d", rec.Code)
	}
	now, _ := s.LoadProfile("dad")
	if now.Title != "又改了一次" {
		t.Fatalf("revert 动了 profile.json: %q", now.Title)
	}
}

// 坏档案（bookmarks.json 读不了）恰恰是最想 revert 的时刻：
// 这条通路不许被 Current() 的坏档判断挡死。
func TestRevertWorksOnBrokenProfile(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.WriteSnapshot("dad"); err != nil { // rev1 的快照
		t.Fatal(err)
	}
	if err := os.WriteFile(s.BookmarksPath("dad"), []byte("{坏掉"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := getWithMethod(h, "POST", "/api/v1/revert?profile=dad&to-rev=1", adminTokHeader())
	if rec.Code != 200 {
		t.Fatalf("坏档案该能 revert, got %d %s", rec.Code, rec.Body)
	}
	b, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatalf("revert 后仍读不了: %v", err)
	}
	if b.Rev != 2 {
		t.Fatalf("revert 该产生新 rev: %d", b.Rev)
	}
}
```

`getWithMethod` 与 `adminTokHeader` 两个小 helper 写进本文件（Task 8 的删档用例共用同一份）：

```go
// getWithMethod 是任意方法 + 一条凭证的通路。第四参数按前缀分流：
// "Bearer ..." 进 Authorization，其余进 Cookie。Task 7/8 共用这一份实现。
func getWithMethod(h http.Handler, method, path, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	switch {
	case strings.HasPrefix(header, "Bearer "):
		req.Header.Set("Authorization", header)
	case header != "":
		req.Header.Set("Cookie", header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// adminTokHeader 让坏档那条用例读起来像人话：那条走的是令牌通路。
func adminTokHeader() string { return "Bearer " + adminTok }
```

最后一格走的是 cookie 通路（`getWithMethod` 的第四参数不是 `"Bearer "` 前缀时进 Cookie）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/httpapi/ -run TestRevert -v`
Expected: 全红（404，路由未注册）。

- [ ] **Step 3: 写 `revert.go`**

```go
package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

// handleRevert 从快照恢复。它**是一次新写入**：rev 前进、不抹历史
// （spec §4.6）。回滚成"把 rev 改回旧值"会让 expectedRev 语义失效，
// 也会让两次 revert 互相覆盖。
//
// 坏档案（bookmarks.json 读不了）恰恰是最需要 revert 的时刻，所以这里
// 不走 Current()：直接读快照，当前状态只在"能做快照时"做快照。
func (s *Server) handleRevert(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "要带 ?profile=<id>"})
		return
	}
	if !model.ValidSlug(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "profile 不是 ASCII slug"})
		return
	}
	to := strings.TrimSpace(r.URL.Query().Get("to-rev"))
	rev, err := strconv.ParseInt(to, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "to-rev 要是整数"})
		return
	}
	actor, _, ok := s.require(r, credScoped, id)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	var (
		snapErr   error
		notFound  bool
		newRev    int64
		restored  int64
		histEntry *store.HistoryEntry
	)
	err = s.deps.Store.WithProfileLock(id, func() error {
		if _, err := s.deps.Store.LoadProfile(id); errors.Is(err, store.ErrNotFound) {
			notFound = true
			return nil
		}
		raw, err := s.deps.Store.LoadSnapshot(id, rev)
		if err != nil {
			notFound = true
			return nil
		}
		b, err := store.DecodeBookmarks(s.deps.Store.BookmarksPath(id), raw)
		if err != nil {
			// 快照本身坏了：报 500（这是真的内部错误），不假装恢复成功。
			return err
		}
		// 当前状态先保住——revert 也是一次写入，写坏了还得能回来。
		// 档案是坏档案时 WriteSnapshot 会失败（读不了就保不了），
		// 那种情况下**继续恢复**：坏档没有值得保的东西，而 revert 正是出路。
		if _, err := s.deps.Store.WriteSnapshot(id); err != nil && !isUnreadable(err) {
			snapErr = err
			return nil
		}
		cur, curErr := s.deps.Store.CurrentRev(id)
		if curErr != nil && !isUnreadable(curErr) {
			return curErr
		}
		b.Rev = cur + 1
		if err := s.deps.Store.SaveBookmarks(id, b); err != nil {
			return err
		}
		newRev, restored = b.Rev, rev
		histEntry = &store.HistoryEntry{
			At: time.Now().Format(time.RFC3339), Profile: id, Actor: actor,
			By: byOfRequest(r), Rev: newRev,
			Ops: historyOps([]ops.Brief{{Op: "revert", Path: "snapshots/" + strconv.FormatInt(rev, 10)}}),
		}
		return nil
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	switch {
	case notFound:
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "detail": "档案或该 rev 的快照不存在"})
	case snapErr != nil:
		writeJSON(w, http.StatusInsufficientStorage, map[string]any{
			"code": 507, "detail": "快照写不下去，本次恢复已取消"})
	case histEntry != nil:
		if err := s.deps.Store.AppendHistory(*histEntry); err != nil {
			// 与 batch 同一取舍：恢复已经成功，报 5xx 会诱导重复 revert。
			fmt.Fprintf(os.Stderr, "history append failed profile=%s: %v\n", id, err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"profile": id, "rev": newRev, "restoredFrom": restored})
	default:
		writeInternalError(w, errors.New("revert 没有产出任何结果"))
	}
}
```

四个缺口要补上（都是本案已有件的复用，不新开实现）：

1. `Store.CurrentRev(id) (int64, error)`——坏档案也要能报出"当前是第几 rev"。`revert.go` 依赖它，实现放 `internal/store/load.go`：

```go
// CurrentRev 只解 rev 一个字段：坏档的 bookmarks.json 里 rev 往往仍然在
// 第一行，revert 要的就是它。整档读不了时返回 (0, ErrCorrupt)，调用方
// 按"从 rev 0 之后开始"处理——恢复不该被损坏程度挡住。
func (s *Store) CurrentRev(id string) (int64, error) {
	raw, err := os.ReadFile(s.BookmarksPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("读 %s: %w", s.BookmarksPath(id), err)
	}
	var only struct {
		Rev int64 `json:"rev"`
	}
	if err := json.Unmarshal(raw, &only); err != nil {
		return 0, fmt.Errorf("rev 都读不出来: %w", ErrCorrupt)
	}
	return only.Rev, nil
}
```

2. `isUnreadable(err) bool`：`errors.Is(err, store.ErrCorrupt) || errors.Is(err, store.ErrInvalid) || os.IsNotExist(err)` 三件——放 `revert.go` 里，**只有 revert 用它就别放进 store**。
3. `byOfRequest(r) string`——revert 与改 PIN 的 `by` 从哪来：CLI 与 AI 走同一个端点，渠道是自报的（spec §4.6）。取法：请求头 `X-LB-By`，缺省或非法值退回 `"web"`。Task 9 的 `handleAdminPin` 也用它，**一份实现**：

```go
// byOfRequest 读 X-LB-By 自报渠道。它只喂历史的 by 字段（spec §4.6：
// 自报口径，不参与任何权限判定），所以非法值不值得一个 422——退回 web
// 并把原值打进 stderr，界面上少一个筛选标签比拒掉一次恢复便宜。
func byOfRequest(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("X-LB-By"))
	if raw == "" {
		return "web"
	}
	if !store.ValidBy(raw) {
		fmt.Fprintf(os.Stderr, "忽略非法的 X-LB-By %q，按 web 记
", raw)
		return "web"
	}
	return raw
}
```

写进 `openapi.json`（Task 11 的 `/api/v1/revert` 那条要列这个头）与 `AGENTS.md`（Task 13）。
4. `model` 与 `fmt`/`os` 进 import。

- [ ] **Step 4: 注册路由**

```go
		{Method: "POST", Pattern: "/api/v1/revert", Cred: credScoped,
			Summary: "从快照恢复（产生新 rev，不抹历史）", H: s.handleRevert},
```

- [ ] **Step 5: 跑测试**

Run: `go test ./internal/httpapi/ -run TestRevert -v`
Expected: 全 PASS。

- [ ] **Step 6: 写 CLI 客户端与 `revert` 子命令**

`cmd/launchbay/client.go`：

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// client 是 CLI 子命令唯一的写盘通路：一律经本机 REST。
//
// 为什么不在 CLI 里直接 import store 写文件：进程内互斥锁罩不住另一个
// 进程，CLI 绕过服务写盘 = 与正在跑的容器抢同一个文件（spec §4.5）。
// 服务没起来时这里就报错退出，**不留"离线直写"的兜底分支**。
type client struct {
	base  string
	token string
	hc    *http.Client
}

// newAdminClient 从 env 里挑管理员令牌（作用域 *），base 指向本机端口。
func newAdminClient() (*client, error) {
	scopes, err := auth.ParseScopes(os.Getenv("LAUNCHBAY_WRITE_TOKENS"))
	if err != nil {
		return nil, fmt.Errorf("LAUNCHBAY_WRITE_TOKENS: %w", err)
	}
	var admin *auth.Scope
	for _, sc := range scopes {
		if sc.IsAdmin() {
			s := sc
			admin = &s
			break
		}
	}
	if admin == nil {
		return nil, fmt.Errorf("这份 env 里没有作用域为 * 的管理员令牌，CLI 子命令要它（spec §10）")
	}
	port := os.Getenv("LAUNCHBAY_PORT")
	if port == "" {
		port = "7600"
	}
	return &client{
		base:  "http://127.0.0.1:" + port,
		token: admin.Secret,
		hc:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *client) do(method, path string, body []byte, hdr map[string]string) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("连不上 %s%s（服务起了吗）: %w", c.base, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("读响应: %w", err)
	}
	return raw, resp.StatusCode, nil
}

// mustOK 把非 2xx 变成带 detail 的错误：CLI 的失败要能说清是哪一格拒绝的，
// 而不是"退出码 1"。
func mustOK(raw []byte, code int, path string) error {
	if code >= 200 && code <= 299 {
		return nil
	}
	var e struct {
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(raw, &e)
	if e.Detail == "" {
		e.Detail = strings.TrimSpace(string(raw))
	}
	return fmt.Errorf("%s 返回 %d：%s", path, code, e.Detail)
}
```

`cmd/launchbay/cli.go`：

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
)

// runCLI 处理写通路的三个子命令。返回进程退出码。
// 分发在 loadConfig 之后（healthcheck 已经在那儿）：CLI 与服务读同一份 env，
// 端口与令牌只有一份来源。
func runCLI(args []string, out io.Writer) int {
	if len(args) == 0 {
		return 0 // 不是 CLI 子命令，交给 serve
	}
	switch args[0] {
	case "revert":
		return cmdRevert(args[1:], out)
	case "profile":
		return cmdProfile(args[1:], out)
	case "sync-template":
		return cmdSyncTemplate(args[1:], out) // Task 12
	}
	return -1 // main.go 据此继续走 serve 分支
}

func cmdRevert(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("revert", flag.ContinueOnError)
	fs.SetOutput(out)
	profile := fs.String("profile", "", "要恢复的档案 id")
	to := fs.Int64("to", 0, "恢复到哪个 rev（该 rev 的快照）")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profile == "" || *to <= 0 {
		fmt.Fprintln(out, "用法: launchbay revert --profile <id> --to <rev>")
		return 2
	}
	c, err := newAdminClient()
	if err != nil {
		fmt.Fprintf(out, "revert 失败: %v\n", err)
		return 1
	}
	path := "/api/v1/revert?profile=" + *profile + "&to-rev=" + fmt.Sprint(*to)
	raw, code, err := c.do("POST", path, nil, map[string]string{"X-LB-By": "cli"})
	if err != nil {
		fmt.Fprintf(out, "revert 失败: %v\n", err)
		return 1
	}
	if err := mustOK(raw, code, path); err != nil {
		fmt.Fprintf(out, "revert 失败: %v\n", err)
		return 1
	}
	var resp struct {
		Rev          int64 `json:"rev"`
		RestoredFrom int64 `json:"restoredFrom"`
	}
	_ = json.Unmarshal(raw, &resp)
	fmt.Fprintf(out, "%s 已恢复到 rev %d 的内容，新的 rev 是 %d\n", *profile, resp.RestoredFrom, resp.Rev)
	return 0
}
```

（`cmdProfile` 与 `cmdSyncTemplate` 在 Task 12 落地；本任务先只让 `revert` 可用，`runCLI` 里对另外两个的分支返回 `2` 并打印"尚未实现"——**不许静默当成功**。）

`main.go` 的分发改成：

```go
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(runHealthcheck(cfg.Port, cfg.DataDir))
		case "revert", "profile", "sync-template":
			os.Exit(exitCode(runCLI(os.Args[1:], os.Stdout)))
		}
	}
```

`exitCode` 就是把 `-1`（不该走到这里）映射成 2，其余原样。

- [ ] **Step 7: 写 CLI 测试（用假服务证明"只走 REST"）**

`cmd/launchbay/cli_test.go`：

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// CLI 不许直接写盘：它唯一的通路是 REST。这条用例用假服务当证人——
// 真去写文件的话，这个测试里根本没有 store 可写。
func TestCLIFallsThroughRESTOnly(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		if r.Header.Get("Authorization") == "" {
			t.Errorf("CLI 没带令牌")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":"dad","rev":3,"restoredFrom":1}`))
	}))
	defer srv.Close()

	c := &client{base: srv.URL, token: "tok", hc: srv.Client()}
	raw, code, err := c.do("POST", "/api/v1/revert?profile=dad&to-rev=1", nil, map[string]string{"X-LB-By": "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Fatalf("code %d %s", code, raw)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "POST /api/v1/revert") {
		t.Fatalf("got %+v", got)
	}
}

// 服务没起来时必须报错退出，不许"降级成离线直写"。
func TestCLIFailsLoudlyWhenServiceDown(t *testing.T) {
	c := &client{base: "http://127.0.0.1:1", token: "tok", hc: &http.Client{}}
	_, _, err := c.do("POST", "/api/v1/revert?profile=dad&to-rev=1", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "连不上") {
		t.Fatalf("该给出可行动的失败: %v", err)
	}
}

// 非 2xx 要把服务端的 detail 带出来，不是只给退出码。
func TestCLISurfacesServerDetail(t *testing.T) {
	err := mustOK([]byte(`{"code":404,"detail":"档案或该 rev 的快照不存在"}`), 404, "/api/v1/revert")
	if err == nil || !strings.Contains(err.Error(), "快照不存在") {
		t.Fatalf("got %v", err)
	}
}
```

`http.Client` 要进 import。这里不新开构造函数：假 base 就够，测的是"连不上时给出可行动的失败"。

- [ ] **Step 8: 【故意破坏】两次**

1. 把 `handleRevert` 里 `b.Rev = cur + 1` 改成 `b.Rev = rev`（回滚成旧 rev）→ `TestRevertRestoresOlderStateAsANewRev` 该红在"revert 该是一次新写入"。改回。
2. 把坏档案那条的 `WriteSnapshot` 失败当成致命（删掉 `&& !isUnreadable(err)`）→ `TestRevertWorksOnBrokenProfile` 该红（507 而不是 200）。改回。这条钉的是"最需要 revert 的时刻恰好是快照做不了的時刻"这个真实死角。

- [ ] **Step 9: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/httpapi/ internal/store/ cmd/launchbay/
git commit -m "feat(api,cli): revert 端点与 launchbay revert（只经 REST，坏档也能回）"
```

---

## Task 8: `/api/v1/status` 与档案列表 / 删档（仅管理员）

**Files:**
- Create: `internal/httpapi/admin.go`
- Modify: `internal/store/paths.go`（加 `DeleteProfile` 需要的路径件）+ `internal/store/profile.go`
- Test: `internal/httpapi/admin_test.go`
- Test: `internal/store/profile_delete_test.go`

**Interfaces:**
- Consumes: `Store.ListProfiles` / `Current` / `SnapshotCount` / `Broken` / `BrokenReason`、`require(credAdmin)`
- Produces:
  - `GET /api/v1/status` → `200 {profiles:[{id,title,admin,rev,items,snapshots,broken}], brokenProfiles:[], totals:{profiles,items,snapshots}}`
  - `GET /api/v1/profiles` → `200 {profiles:[{id,title,admin,createdAt}]}`（元信息，零条目）
  - `DELETE /api/v1/profiles?profile=<id>` → `200 {deleted}`；档案不存在 404
  - `func (s *Store) DeleteProfile(id string) error`

- [ ] **Step 1: 写 store 侧删档测试**

`internal/store/profile_delete_test.go`：

```go
package store

import (
	"os"
	"testing"
)

func TestDeleteProfileRemovesDirIncludingSnapshots(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.SnapshotsDir("dad"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.SnapshotsDir("dad")+"/1-bookmarks.json", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProfile("dad"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ProfileDir("dad")); !os.IsNotExist(err) {
		t.Fatalf("档案目录还在: %v", err)
	}
	list, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("列表里还有: %+v", list)
	}
}

// 删档不动 history.jsonl：全家庭审计线要留着（spec §6.1 的删档说明）。
func TestDeleteProfileKeepsHistory(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(HistoryEntry{At: "t", Profile: "dad", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProfile("dad"); err != nil {
		t.Fatal(err)
	}
	hist, err := s.ReadHistory("", 0)
	if err != nil || len(hist) != 1 {
		t.Fatalf("历史该保留: %v %+v", err, hist)
	}
}

// 删不存在的档案是 ErrNotFound，不是 nil：调用方要能分清"删掉了"与"本来就没有"。
func TestDeleteMissingProfileIsNotFound(t *testing.T) {
	s := New(t.TempDir())
	if err := s.DeleteProfile("ghost"); !os.IsNotExist(err) && err == nil {
		t.Fatalf("该报错, got %v", err)
	}
}

// 越界的 id 不许变成"把整个 profiles 目录删了"。
func TestDeleteProfileRefusesPathTraversal(t *testing.T) {
	s := New(t.TempDir())
	for _, id := range []string{"..", "../profiles", "/", ""} {
		if err := s.DeleteProfile(id); err == nil {
			t.Fatalf("DeleteProfile(%q) 该拒", id)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/store/ -run TestDelete -v`
Expected: 编译失败（`undefined: DeleteProfile`）。

- [ ] **Step 3: 写 `DeleteProfile`**

`internal/store/profile.go` 追加：

```go
// DeleteProfile 删掉整个档案目录（含 snapshots）。三道闸：
//  1. id 必须是 ASCII slug——否则 ".." 能把整个 profiles/ 端掉；
//  2. 目录必须在 s.profilesDir() 里面（filepath.Rel 再验一次，
//     防的是有人以后放宽了 slug 规则）；
//  3. 拿该档案的写锁，别与正在进行的 batch 抢同一个目录。
//
// history.jsonl 不动：它是全家庭的审计线，删档不等于抹掉"谁动过"。
func (s *Store) DeleteProfile(id string) error {
	if !model.ValidSlug(id) {
		return fmt.Errorf("档案 id %q 不是 ASCII slug: %w", id, ErrInvalid)
	}
	dir := s.ProfileDir(id)
	rel, err := filepath.Rel(s.profilesDir(), dir)
	if err != nil || rel != id {
		return fmt.Errorf("档案路径越界: %q", id)
	}
	return s.WithProfileLock(id, func() error {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("删 %s: %w", dir, err)
		}
		s.clearBroken(id, "profile")
		s.clearBroken(id, "bookmarks")
		return nil
	})
}
```

（`clearBroken(id, source)` 是 P1-① 已有的两参数形状。删档后该档案的坏档登记必须一起清掉，否则 `/status` 会一直挂着一条指向不存在档案的记录。）

- [ ] **Step 4: 写端点测试**

`internal/httpapi/admin_test.go`：

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

func TestStatusIsAdminOnlyAndAggregates(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/status", adminTok)
	if rec.Code != 200 {
		t.Fatalf("管理员令牌该放行, got %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Profiles []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Admin     bool   `json:"admin"`
			Rev       int64  `json:"rev"`
			Items     int    `json:"items"`
			Snapshots int    `json:"snapshots"`
			Broken    bool   `json:"broken"`
		} `json:"profiles"`
		BrokenProfiles []string `json:"brokenProfiles"`
		Totals         struct {
			Profiles  int `json:"profiles"`
			Items     int `json:"items"`
			Snapshots int `json:"snapshots"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Profiles) != 2 || out.Totals.Profiles != 2 {
		t.Fatalf("got %+v", out.Profiles)
	}
	if out.Totals.Items != 2 {
		t.Fatalf("条目合计不对: %+v", out.Totals)
	}
	if out.Profiles[0].ID != "dad" || out.Profiles[0].Snapshots != 1 || !out.Profiles[0].Admin {
		t.Fatalf("dad 的汇总不对: %+v", out.Profiles[0])
	}
	// 档案作用域令牌与匿名都要拒：这是跨档案聚合信息（spec §6.1）。
	if rec := getBearer(h, "/api/v1/status", dadTok); rec.Code != 403 {
		t.Fatalf("档案作用域令牌该 403, got %d", rec.Code)
	}
	if rec := get(h, "/api/v1/status", ""); rec.Code != 401 {
		t.Fatalf("匿名该 401, got %d", rec.Code)
	}
	// 管理员会话也放行（它是磁盘上确认过的管理员）。
	if rec := get(h, "/api/v1/status", "lb_profile=dad; lb_home=dad"); rec.Code != 200 {
		t.Fatalf("管理员会话该 200, got %d", rec.Code)
	}
}

// 坏档案不许从 /status 上消失：spec §12 要它汇总 brokenProfiles[]，
// 一个人的手误要看得见。
func TestStatusReportsBrokenProfiles(t *testing.T) {
	s, h := setupBatch(t)
	if err := os.WriteFile(s.BookmarksPath("kid"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := getBearer(h, "/api/v1/status", adminTok)
	var out struct {
		Profiles       []map[string]any `json:"profiles"`
		BrokenProfiles []string         `json:"brokenProfiles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.BrokenProfiles) != 1 || out.BrokenProfiles[0] != "kid" {
		t.Fatalf("brokenProfiles 不对: %+v", out.BrokenProfiles)
	}
	for _, p := range out.Profiles {
		if p["id"] == "kid" && p["broken"] != true {
			t.Fatalf("坏档案那条该标 broken=true: %+v", p)
		}
	}
	// 坏档案仍然在列表里（不是被剔除），rev 报 0。
	if len(out.Profiles) != 2 {
		t.Fatalf("坏档案被剔了: %+v", out.Profiles)
	}
}

func TestListProfilesHasMetaOnly(t *testing.T) {
	_, h := setupBatch(t)
	rec := getBearer(h, "/api/v1/profiles", adminTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	// 零条目、零 URL、零分组名：列表端点不是书签的第二个读口。
	body := rec.Body.String()
	for _, banned := range []string{"10.0.0.3", "群晖", "NAS", "dsm"} {
		if strings.Contains(body, banned) {
			t.Fatalf("档案列表漏出了书签内容 %q: %s", banned, body)
		}
	}
}

func TestDeleteProfileEndpoint(t *testing.T) {
	s, h := setupBatch(t)
	if _, err := s.LoadProfile("kid"); err != nil {
		t.Fatal(err)
	}
	rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=kid", "Bearer "+adminTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if _, err := s.LoadProfile("kid"); err == nil {
		t.Fatal("档案还在")
	}
	// 再删一次该 404，不是"幂等成功"。
	if rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=kid", "Bearer "+adminTok); rec.Code != 404 {
		t.Fatalf("重复删档该 404, got %d", rec.Code)
	}
	// 缺 profile / 不是 slug。
	if rec := getWithMethod(h, "DELETE", "/api/v1/profiles", "Bearer "+adminTok); rec.Code != 400 {
		t.Fatalf("缺 profile 该 400, got %d", rec.Code)
	}
	if rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=..", "Bearer "+adminTok); rec.Code != 400 {
		t.Fatalf("非 slug 该 400, got %d", rec.Code)
	}
	// 档案作用域令牌删档该 403（不是 404：先鉴权后查存在）。
	if rec := getWithMethod(h, "DELETE", "/api/v1/profiles?profile=dad", "Bearer "+dadTok); rec.Code != 403 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}
```


- [ ] **Step 5: 写 `admin.go`**

```go
package httpapi

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

type statusProfile struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Admin     bool   `json:"admin"`
	Rev       int64  `json:"rev"`
	Items     int    `json:"items"`
	Snapshots int    `json:"snapshots"`
	Broken    bool   `json:"broken"`
}

// handleStatus 是跨档案聚合信息，只给管理员（spec §6.1）。档案作用域的
// 令牌拿它会 403——AGENTS.md 要专门写明，否则 AI 会当成 bug 去"修"。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	profiles, err := s.deps.Store.ListProfiles()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	out := make([]statusProfile, 0, len(profiles))
	broken := []string{}
	tot := struct {
		Profiles  int `json:"profiles"`
		Items     int `json:"items"`
		Snapshots int `json:"snapshots"`
	}{Profiles: len(profiles)}
	for _, p := range profiles {
		row := statusProfile{ID: p.ID, Title: p.Title, Admin: p.Admin}
		snap, err := s.deps.Store.Current(p.ID)
		switch {
		case err == nil:
			row.Rev = snap.Rev
			for _, g := range snap.B.Groups {
				row.Items += len(g.Items)
			}
		default:
			// 坏档案仍要在表里占一行，rev/items 报 0：消失比报 0 更坏。
			if _, isBroken := s.deps.Store.BrokenReason(p.ID); isBroken {
				row.Broken = true
				broken = append(broken, p.ID)
			} else if !errors.Is(err, store.ErrNotFound) {
				writeInternalError(w, err)
				return
			}
		}
		row.Snapshots = s.deps.Store.SnapshotCount(p.ID)
		tot.Items += row.Items
		tot.Snapshots += row.Snapshots
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profiles": out, "brokenProfiles": broken, "totals": tot})
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	profiles, err := s.deps.Store.ListProfiles()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	type meta struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Admin     bool   `json:"admin"`
		CreatedAt string `json:"createdAt"`
	}
	out := make([]meta, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, meta{ID: p.ID, Title: p.Title, Admin: p.Admin, CreatedAt: p.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	if id == "" || !model.ValidSlug(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "要带 ?profile=<ASCII slug>"})
		return
	}
	if _, _, ok := s.require(r, credAdmin, id); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	err := s.deps.Store.DeleteProfile(id)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
	case os.IsNotExist(err):
		writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "detail": "档案不存在"})
	default:
		writeInternalError(w, err)
	}
}
```

- [ ] **Step 6: 注册三条路由**

```go
		{Method: "GET", Pattern: "/api/v1/status", Cred: credAdmin, Summary: "各档案 rev/条目/快照/坏档汇总", H: s.handleStatus},
		{Method: "GET", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "档案元信息列表（零条目）", H: s.handleListProfiles},
		{Method: "DELETE", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "删档（连快照，历史保留）", H: s.handleDeleteProfile},
```

- [ ] **Step 7: 跑测试 + 【故意破坏】两次**

Run: `go test ./internal/httpapi/ ./internal/store/ -run 'TestStatus|TestListProfiles|TestDelete' -v` → 全 PASS。

1. 把 `handleStatus` 里坏档案那条改成 `continue`（跳过不列）→ `TestStatusReportsBrokenProfiles` 该红（坏档案被剔了）。改回。
2. 把 `DeleteProfile` 的 slug 闸删掉 → `TestDeleteProfileRefusesPathTraversal` 该红。**这条是本案唯一挡得住 `rm -rf profiles/` 的闸**，红过之后确认改回。

- [ ] **Step 8: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/httpapi/ internal/store/
git commit -m "feat(api,store): /status 与档案列表/删档（仅管理员），删档保历史"
```

---

## Task 9: PIN 端点——改自己的与管理员给别人设

**Files:**
- Create: `internal/httpapi/pinapi.go`
- Test: `internal/httpapi/pinapi_test.go`

**Interfaces:**
- Consumes: `auth.HashPIN` / `VerifyPIN` / `ValidPinFormat`、`Store.LoadProfile` / `SaveProfile`、`require(credSession)` / `require(credAdmin)`
- Produces:
  - `POST /api/v1/profiles/self/pin`，体 `{old, new}`，会话作用域；旧 PIN 不对拒改 → `403`；形状不对 → `422`
  - `POST /api/v1/profiles/pin`，体 `{profile, new}`，仅管理员；**进目标档案的历史**
  - `func (s *Server) handleSelfPin(w http.ResponseWriter, r *http.Request)`
  - `func (s *Server) handleAdminPin(w http.ResponseWriter, r *http.Request)`

- [ ] **Step 1: 写测试**

`internal/httpapi/pinapi_test.go`：

```go
package httpapi

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
)

func TestSelfPinNeedsOldPinAndSession(t *testing.T) {
	s, h := setupBatch(t)
	// setup() 里 dad 的 PIN 是 1234。
	rec := loginAs(t, h, "dad", "1234")
	cookie := sessionCookie(t, rec)
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"1234","new":"4321"}`, cookie, ""); got.Code != 200 {
		t.Fatalf("改自己 PIN 该 200, got %d %s", got.Code, got.Body)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPIN(p.PIN, "4321") || auth.VerifyPIN(p.PIN, "1234") {
		t.Fatal("PIN 没换过来，或旧 PIN 还能用")
	}
	// 旧 PIN 不对 → 403，且不改盘。
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"9999","new":"1111"}`, cookie, ""); got.Code != 403 {
		t.Fatalf("旧 PIN 不对该 403, got %d %s", got.Code, got.Body)
	}
	p2, _ := s.LoadProfile("dad")
	if p2.PIN != p.PIN {
		t.Fatal("被拒的改动动了盘")
	}
	// 新 PIN 形状不合法 → 422 点名 new。
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"4321","new":"12"}`, cookie, ""); got.Code != 422 ||
		!strings.Contains(got.Body.String(), `"field":"new"`) {
		t.Fatalf("got %d %s", got.Code, got.Body)
	}
	// 令牌不许走 self/pin（那是"谁有令牌谁能改谁"）。
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"4321","new":"2222"}`, "", adminTok); got.Code != 403 {
		t.Fatalf("令牌该被拒, got %d %s", got.Code, got.Body)
	}
	// 匿名 401。
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"4321","new":"2222"}`, "", ""); got.Code != 401 {
		t.Fatalf("匿名该 401, got %d", got.Code)
	}
}

func TestAdminPinGoesIntoTargetHistory(t *testing.T) {
	s, h := setupBatch(t)
	rec := loginAs(t, h, "dad", "1234")
	cookie := sessionCookie(t, rec)
	// dad 是管理员：给 kid 设 PIN，不带旧 PIN。
	if got := post(h, "/api/v1/profiles/pin", `{"profile":"kid","new":"8765"}`, cookie, ""); got.Code != 200 {
		t.Fatalf("管理员设 PIN 该 200, got %d %s", got.Code, got.Body)
	}
	p, err := s.LoadProfile("kid")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPIN(p.PIN, "8765") {
		t.Fatal("kid 的 PIN 没被设上")
	}
	// 目标档案的历史里该有一条，actor 是 dad。
	hist, _ := s.ReadHistory("kid", 0)
	if len(hist) != 1 || hist[0].Actor != "dad" || hist[0].Profile != "kid" {
		t.Fatalf("管理员设 PIN 要在目标档案留名: %+v", hist)
	}
	// 非管理员会话不许设别人。
	rec2 := loginAs(t, h, "kid", "8765")
	if got := post(h, "/api/v1/profiles/pin", `{"profile":"dad","new":"0000"}`, sessionCookie(rec2), ""); got.Code != 403 {
		t.Fatalf("非管理员设别人 PIN 该 403, got %d %s", got.Code, got.Body)
	}
	// 档案作用域令牌也不行（管理员端点要 *）。
	if got := post(h, "/api/v1/profiles/pin", `{"profile":"kid","new":"1234"}`, "", dadTok); got.Code != 403 {
		t.Fatalf("got %d", got.Code)
	}
}

// 设 PIN 不是书签写入：不许动 rev、不许做快照。
func TestPinChangeDoesNotTouchRevOrSnapshots(t *testing.T) {
	s, h := setupBatch(t)
	before, err := s.Current("kid")
	if err != nil {
		t.Fatal(err)
	}
	n := s.SnapshotCount("kid")
	rec := loginAs(t, h, "kid", "5678")
	if got := post(h, "/api/v1/profiles/self/pin", `{"old":"5678","new":"1357"}`, sessionCookie(t, rec), ""); got.Code != 200 {
		t.Fatalf("got %d %s", got.Code, got.Body)
	}
	after, _ := s.Current("kid")
	if after.Rev != before.Rev {
		t.Fatalf("改 PIN 不该 bump rev: %d → %d", before.Rev, after.Rev)
	}
	if s.SnapshotCount("kid") != n {
		t.Fatalf("改 PIN 不该做快照")
	}
}
```

两个 helper 写进本文件（`cookieMap` 是 P1-① 已有的）：

```go
func loginAs(t *testing.T, h http.Handler, profile, pin string) *httptest.ResponseRecorder {
	t.Helper()
	return post(h, "/pin", `{"profile":"`+profile+`","pin":"`+pin+`"}`, "", "")
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	cm := cookieMap(t, rec)
	return auth.CookieProfile + "=" + cm[auth.CookieProfile] + "; " +
		auth.CookieHome + "=" + cm[auth.CookieHome]
}
```

调用点统一成 `loginAs(t, h, ...)` 与 `sessionCookie(t, rec)`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/httpapi/ -run 'TestSelfPin|TestAdminPin|TestPinChange' -v`
Expected: 全红（404）。

- [ ] **Step 3: 写 `pinapi.go`**

```go
package httpapi

import (
	"net/http"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// handleSelfPin 改自己的 PIN：要旧 PIN，走会话，令牌一律拒。
//
// 为什么不记进 history：§4.6 的审计线记的是"谁动了我的书签"，改 PIN
// 不动书签、不动 rev。管理员给别人设 PIN 是另一回事——那是**替别人**
// 做决定，必须出现在那个档案自己的历史里（spec §6.1）。
func (s *Server) handleSelfPin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	p, err := s.deps.Store.LoadProfile(sess.Active)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if !auth.VerifyPIN(p.PIN, in.Old) {
		writeJSON(w, http.StatusForbidden, map[string]any{"code": 403, "detail": "旧 PIN 不对"})
		return
	}
	if !auth.ValidPinFormat(in.New) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "new", Reason: "要 4-6 位数字"}})
		return
	}
	hash, err := auth.HashPIN(in.New)
	if err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "new", Reason: err.Error()}})
		return
	}
	p.PIN = hash
	if err := s.deps.Store.SaveProfile(p); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": p.ID, "pinChanged": true})
}

// handleAdminPin 给任意档案设 PIN，不要旧 PIN（家人忘了 PIN 的出路），
// 仅管理员，且**在目标档案的历史里留名**。
func (s *Server) handleAdminPin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Profile string `json:"profile"`
		New     string `json:"new"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if !model.ValidSlug(in.Profile) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "profile", Reason: "必须是 ASCII slug"}})
		return
	}
	if !auth.ValidPinFormat(in.New) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "new", Reason: "要 4-6 位数字"}})
		return
	}
	actor, _, ok := s.require(r, credAdmin, in.Profile)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	p, err := s.deps.Store.LoadProfile(in.Profile)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "detail": "档案不存在"})
			return
		}
		writeInternalError(w, err)
		return
	}
	hash, err := auth.HashPIN(in.New)
	if err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "new", Reason: err.Error()}})
		return
	}
	if err := s.deps.Store.WithProfileLock(in.Profile, func() error {
		p.PIN = hash
		if err := s.deps.Store.SaveProfile(p); err != nil {
			return err
		}
		return s.deps.Store.AppendHistory(store.HistoryEntry{
			At: time.Now().Format(time.RFC3339), Profile: in.Profile, Actor: actor,
			By: byOfRequest(r), Rev: 0,
			Ops: historyOps([]ops.Brief{{Op: "set", Path: "profile/pin"}}),
		})
	}); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": in.Profile, "pinChanged": true})
}
```

`Rev: 0` 与 `profile/pin` 这两处要解释一句（写进代码注释）：`rev` 是书签计数器，改 PIN 不产生新 rev，所以记 0；`path` 用 `profile/pin` 而不是 `groups/...`，因为动的不是书签树。历史里出现 `rev:0` 的行是**有意的**，`/history` 的调用方（P1-④ 的界面）要按"`rev:0` 的行不是书签改动"渲染。

`model`、`errors`、`ops` 进 import。

- [ ] **Step 4: 注册两条路由**

```go
		{Method: "POST", Pattern: "/api/v1/profiles/self/pin", Cred: credSession,
			Summary: "改自己的 PIN（要旧 PIN，仅会话）", H: s.handleSelfPin},
		{Method: "POST", Pattern: "/api/v1/profiles/pin", Cred: credAdmin,
			Summary: "管理员给任意档案设 PIN", H: s.handleAdminPin},
```

- [ ] **Step 5: 跑测试 + 【故意破坏】两次**

Run: `go test ./internal/httpapi/ -run 'TestSelfPin|TestAdminPin|TestPinChange' -v` → 全 PASS。

1. 把 `handleSelfPin` 的 `credSession` 改成 `credScoped` → "令牌该被拒"那一格该红。改回。
2. 把 `handleAdminPin` 的 `AppendHistory` 那行删掉 → `TestAdminPinGoesIntoTargetHistory` 该红。改回。

- [ ] **Step 6: 接手 P1-① 遗留的两条 Minor**

1. `internal/auth/session_test.go` 里 `TestAdminLookupFromActiveProfileIgnoresHome` 这个名字与它现在测的东西不符（管理员资格的查法在 Task 4 之后有唯一入口）。跑一次 `go test ./internal/auth/ -run TestAdminLookup -v` 确认它测的到底是哪一条，然后把名字改成能说出那一条的形式（例如 `TestAdminCrossSwitchIgnoresForgedHome`）。**只改名，不改断言**——改完确认仍然绿。
2. `writeInternalError` 的"真 I/O 错误 → 500 且响应不回显内部错误"至今没有证人。补一条：在 `handlers_test.go` 里让 `Store` 指向一个不可读的档案目录（Windows 上用 `os.Chmod(dir, 0o000)` 不稳，改用"把 `profiles/dad/bookmarks.json` 换成一个目录"来制造 `ReadFile` 失败），断言 500 且 body 里没有磁盘路径、没有 Go 错误串。

- [ ] **Step 7: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/httpapi/ internal/auth/
git commit -m "feat(httpapi): 改自己 PIN 与管理员设 PIN，后者进目标档案历史"
```

---

## Task 10: `POST /api/v1/usage`——点击计数不许变成第二次写入

**Files:**
- Create: `internal/store/usage.go`
- Create: `internal/httpapi/usage.go`
- Modify: `internal/store/paths.go`（加 `UsagePath`）
- Test: `internal/store/usage_test.go`
- Test: `internal/httpapi/usage_test.go`

**Interfaces:**
- Consumes: `Store.WriteFileJSON` / `BookmarksPath`、`require(credScoped)`
- Produces:
  - `func (s *Store) UsagePath(id string) string`
  - `type store.Usage struct { Items []UsageItem `json:"items"` }`、`type store.UsageItem struct { ID string; Clicks int; LastAt string }`
  - `func (s *Store) LoadUsage(id string) (*Usage, error)` / `AddUsage(id string, ids []string, at string) (*Usage, []string, error)`
  - `POST /api/v1/usage`，体 `{itemIds:[]}` → `200 {updated, unknown[]}`
  - `func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request)`

- [ ] **Step 1: 写 store 侧测试**

`internal/store/usage_test.go`：

```go
package store

import (
	"os"
	"strings"
	"testing"
)

func TestAddUsageSortsByIDForAStableFile(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸"}); err != nil {
		t.Fatal(err)
	}
	// 故意乱序上报：文件里的顺序必须稳定，否则同内容两次写出不一致，
	// 违反"禁止用 map 产生输出键序"的同一族约束。
	u, unknown, err := s.AddUsage("dad", []string{"z", "a", "m"}, "2026-10-05T20:00:00+08:00")
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown) != 3 {
		t.Fatalf("这些 id 不在书签里，该全部报出来: %+v", unknown)
	}
	ids := []string{}
	for _, it := range u.Items {
		ids = append(ids, it.ID)
	}
	if strings.Join(ids, ",") != "a,m,z" {
		t.Fatalf("没排序: %+v", u.Items)
	}
	first, _ := os.ReadFile(s.UsagePath("dad"))
	if _, err := s.AddUsage("dad", []string{"z"}, "2026-10-05T20:01:00+08:00"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(s.UsagePath("dad"))
	if string(first) == string(second) {
		t.Fatal("第二次上报该让 z 的计数变化")
	}
	if !strings.Contains(string(second), `"clicks":2`) {
		t.Fatalf("z 该累计到 2: %s", second)
	}
}

// 空上报不该造出一个文件：没点过就是没有。
func TestEmptyUsageReportCreatesNothing(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddUsage("dad", nil, "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.UsagePath("dad")); !os.IsNotExist(err) {
		t.Fatalf("空上报造出了文件: %v", err)
	}
}

// usage.json 坏了不许拖垮书签：LoadUsage 报 ErrCorrupt，
// AddUsage 在这种情况下**拒绝写**（不覆盖掉一份可能可修的坏文件）。
func TestBrokenUsageIsolation(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.UsagePath("dad"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddUsage("dad", []string{"a"}, "t"); err == nil {
		t.Fatal("坏 usage 该拒绝写")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/store/ -run Usage -v`
Expected: 编译失败（`undefined: UsagePath`）。

- [ ] **Step 3: 写 store 侧**

`internal/store/paths.go` 追加：

```go
func (s *Store) UsagePath(id string) string {
	return filepath.Join(s.ProfileDir(id), "usage.json")
}
```

`internal/store/usage.go`：

```go
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

type UsageItem struct {
	ID     string `json:"id"`
	Clicks int    `json:"clicks"`
	LastAt string `json:"lastAt"`
}

// Usage 用切片不用 map：输出键序要稳定（§4.3 同一族约束）。
type Usage struct {
	Items []UsageItem `json:"items"`
}

func (s *Store) LoadUsage(id string) (*Usage, error) {
	raw, err := os.ReadFile(s.UsagePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return &Usage{Items: []UsageItem{}}, nil
		}
		return nil, fmt.Errorf("读 %s: %w", s.UsagePath(id), err)
	}
	var u Usage
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, fmt.Errorf("usage.json 读不了: %w", ErrCorrupt)
	}
	if u.Items == nil {
		u.Items = []UsageItem{}
	}
	return &u, nil
}

// AddUsage 累加点击并返回"这些 id 在书签里找不到"。
//
// 找不到仍要记：前端上报与书签改动会交错（AI 刚删掉一条，有人正好点了它），
// 拒收会让前端的节流逻辑变成"悄悄丢数据"。
func (s *Store) AddUsage(id string, ids []string, at string) (*Usage, []string, error) {
	if len(ids) == 0 {
		// 空上报不造文件：没点过就是没有。
		return &Usage{Items: []UsageItem{}}, nil, nil
	}
	b, err := s.LoadBookmarks(id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, nil, err
	}
	known := map[string]bool{}
	if b != nil {
		for _, g := range b.Groups {
			for _, it := range g.Items {
				known[it.ID] = true
			}
		}
	}
	u, err := s.LoadUsage(id)
	if err != nil {
		return nil, nil, err
	}
	idx := map[string]int{}
	for i, it := range u.Items {
		idx[it.ID] = i
	}
	var unknown []string
	for _, want := range ids {
		if !known[want] {
			unknown = append(unknown, want)
		}
		if i, ok := idx[want]; ok {
			u.Items[i].Clicks++
			u.Items[i].LastAt = at
			continue
		}
		u.Items = append(u.Items, UsageItem{ID: want, Clicks: 1, LastAt: at})
		idx[want] = len(u.Items) - 1
	}
	sort.Slice(u.Items, func(i, j int) bool { return u.Items[i].ID < u.Items[j].ID })
	if err := s.WriteFileJSON(s.UsagePath(id), u); err != nil {
		return nil, nil, err
	}
	if unknown == nil {
		unknown = []string{}
	}
	return u, unknown, nil
}
```

- [ ] **Step 4: 写端点测试（含 spec §13 点名要"故意刷一次"的那三条）**

`internal/httpapi/usage_test.go`：

```go
package httpapi

import (
	"os"
	"strings"
	"testing"
)

// spec §13：这三条要**故意做一次刷点击再断言 rev 与文件数没变**——
// 只看代码"没调 store 的书签写通路"不算验证，最容易出的错恰恰是
// 顺手复用了写书签的那条通路。
func TestUsageDoesNotTouchRevSnapshotsOrHistory(t *testing.T) {
	s, h := setupBatch(t)
	before, err := s.Current("dad")
	if err != nil {
		t.Fatal(err)
	}
	snapN := s.SnapshotCount("dad")
	histN := len(mustReadHistory(t, s, "dad"))
	dirN := countDirEntries(t, s.SnapshotsDir("dad"))

	for i := 0; i < 3; i++ {
		rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "", dadTok)
		if rec.Code != 200 {
			t.Fatalf("第 %d 次上报失败: %d %s", i+1, rec.Code, rec.Body)
		}
	}
	after, _ := s.Current("dad")
	if after.Rev != before.Rev {
		t.Fatalf("刷点击把 rev 推到了 %d", after.Rev)
	}
	if s.SnapshotCount("dad") != snapN || countDirEntries(t, s.SnapshotsDir("dad")) != dirN {
		t.Fatalf("刷点击做出了快照")
	}
	if len(mustReadHistory(t, s, "dad")) != histN {
		t.Fatalf("刷点击写了历史")
	}
	// 唯一该出现的新文件是 usage.json。
	raw, err := os.ReadFile(s.UsagePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"clicks":3`) {
		t.Fatalf("三次点击该累计成 3: %s", raw)
	}
}

// 不许凭一个凭证给别人档案刷点击：只写"当前档案"。
func TestUsageCannotFloodAnotherProfile(t *testing.T) {
	s, h := setupBatch(t)
	// dadTok 的作用域是 dad，请求里没有 profile 参数——它只能落在 dad 上。
	if rec := post(h, "/api/v1/usage", `{"itemIds":["cartoon"]}`, "", dadTok); rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(s.UsagePath("kid")); !os.IsNotExist(err) {
		t.Fatalf("拿 dad 的凭证刷了 kid 的 usage: %v", err)
	}
	// 想指定别人的档案？带 ?profile=kid 来 → 403，不是"写到 kid"。
	if rec := post(h, "/api/v1/usage?profile=kid", `{"itemIds":["cartoon"]}`, "", dadTok); rec.Code != 403 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

// 会话通路落在 lb_profile 上；管理员代管时**不**顺手写别人的计数。
func TestUsageSessionWritesActiveOnly(t *testing.T) {
	s, h := setupBatch(t)
	rec := loginAs(t, h, "dad", "1234")
	cookie := sessionCookie(t, rec)
	if got := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, cookie, ""); got.Code != 200 {
		t.Fatalf("got %d %s", got.Code, got.Body)
	}
	if _, err := os.Stat(s.UsagePath("dad")); err != nil {
		t.Fatalf("dad 的 usage 没写: %v", err)
	}
	if _, err := os.Stat(s.UsagePath("kid")); !os.IsNotExist(err) {
		t.Fatal("写了别人的")
	}
}

func TestUsageUnknownIdsAreReportedNotRejected(t *testing.T) {
	_, h := setupBatch(t)
	rec := post(h, "/api/v1/usage", `{"itemIds":["nope","dsm"]}`, "", dadTok)
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"unknown":["nope"]`) {
		t.Fatalf("未知 id 该报出来: %s", rec.Body)
	}
}

func TestUsageNeedsCredentialsAndRejectsOversizedList(t *testing.T) {
	_, h := setupBatch(t)
	if rec := post(h, "/api/v1/usage", `{"itemIds":["dsm"]}`, "", ""); rec.Code != 401 {
		t.Fatalf("got %d", rec.Code)
	}
	// 一次报 5000 个 id：前端节流坏了或有人在刷。拒掉，不写盘。
	ids := make([]string, 5000)
	for i := range ids {
		ids[i] = `"x"`
	}
	body := `{"itemIds":[` + strings.Join(ids, ",") + `]}`
	if rec := post(h, "/api/v1/usage", body, "", dadTok); rec.Code != 422 {
		t.Fatalf("超量上报该 422, got %d", rec.Code)
	}
}
```

三个 helper（`mustReadHistory`、`countDirEntries`）写进本文件：

```go
func mustReadHistory(t *testing.T, s *store.Store, id string) []store.HistoryEntry {
	t.Helper()
	h, err := s.ReadHistory(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func countDirEntries(t *testing.T, dir string) int {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return len(ents)
}
```

（`store` 要进 import。）

- [ ] **Step 5: 写 `internal/httpapi/usage.go`**

```go
package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// usageMaxIDs 是单次上报的上限。前端是节流批量，正常一次十几条；
// 五千条要么是节流坏了，要么是有人在刷计数。
const usageMaxIDs = 500

// handleUsage 只写**当前档案**的 usage.json：不参与 rev、不进快照、
// 不进历史（spec §6.1）。它复用的只有原子写，没有书签写通路。
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ItemIDs []string `json:"itemIds"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if len(in.ItemIDs) > usageMaxIDs {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Field: "itemIds",
			Reason: "一次最多 " + strconv.Itoa(usageMaxIDs) + " 条"}})
		return
	}
	// 目标档案 = 凭证自己指向的那个。?profile= 只用来"越界即拒"，
	// 绝不用来"指定要写谁"。
	sess := auth.ReadSession(r)
	target := sess.Active
	if tok := bearer(r); tok != "" {
		if sc, ok := s.deps.Tokens.Authorize(tok); ok {
			if sc.IsAdmin() {
				target = strings.TrimSpace(r.URL.Query().Get("profile"))
				if target == "" {
					writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "管理员令牌要带 ?profile=<id>"})
					return
				}
			} else {
				target = sc.Profile
			}
		}
	}
	if _, _, ok := s.require(r, credScoped, target); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	u, unknown, err := s.deps.Store.AddUsage(target, in.ItemIDs, time.Now().Format(time.RFC3339))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"updated": len(u.Items), "unknown": unknown})
}
```

`strconv` 要进 import。

- [ ] **Step 6: 注册路由 + 跑测试**

```go
		{Method: "POST", Pattern: "/api/v1/usage", Cred: credScoped,
			Summary: "点击计数上报（只写当前档案，不参与 rev）", H: s.handleUsage},
```

Run: `go test ./internal/httpapi/ ./internal/store/ -run Usage -v` → 全 PASS。

- [ ] **Step 7: 【故意破坏】一次（这条最容易"看起来覆盖了"）**

在 `AddUsage` 里顺手加一行 `b.Rev++`（模拟"复用了书签写通路"），跑 `go test ./internal/httpapi/ -run TestUsageDoesNotTouchRev` → 该红在 rev。改回。红过之后再确认：`usage.json` 之外**没有**新文件（`countDirEntries` 那条断言）。

- [ ] **Step 8: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/store/ internal/httpapi/
git commit -m "feat(api,store): /api/v1/usage 点击计数，实测不碰 rev/快照/历史"
```

---

## Task 11: `openapi.json` 与路由契约测试

**Files:**
- Create: `internal/httpapi/openapi.json`
- Create: `internal/httpapi/openapi.go`
- Test: `internal/httpapi/openapi_test.go`

**Interfaces:**
- Consumes: Task 4 的 `routes()`（含 `Cred` 与 `Summary`）
- Produces:
  - `GET /api/v1/openapi.json`（`credNone`，公开，无个人信息）
  - `func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request)`
  - 契约测试：文档里的 (方法, 路径) 集合 == 路由表集合 ∪ 已声明的 planned

- [ ] **Step 1: 写契约测试**

`internal/httpapi/openapi_test.go`：

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// planned 是"文档里写了但本案还没实现"的端点。列出来而不是藏起来，是因为
// AI 读 openapi 时应当知道图标搜索与 resolve 的存在与形状；但每一条都必须
// 带 x-status: planned，测试才认它。
var planned = map[string]bool{
	"GET /api/v1/icons":  true,
	"POST /api/v1/resolve": true,
}

func TestOpenAPICoversEveryRoute(t *testing.T) {
	s := New(Dependencies{})
	var doc struct {
		Paths map[string]map[string]struct {
			Summary string `json:"summary"`
			XStatus string `json:"x-status"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatalf("openapi.json 不是合法 JSON: %v", err)
	}
	inDoc := map[string]bool{}
	for path, methods := range doc.Paths {
		for m, op := range methods {
			key := strings.ToUpper(m) + " " + path
			inDoc[key] = true
			if op.Summary == "" {
				t.Errorf("%s 在文档里没有 summary", key)
			}
			if planned[key] && op.XStatus != "planned" {
				t.Errorf("%s 是计划中的端点，必须标 x-status: planned", key)
			}
		}
	}
	var missing []string
	for _, rt := range s.routes() {
		key := rt.Method + " " + rt.Pattern
		if !inDoc[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("路由表里有、文档里没有（文档骗 AI 比骗人更贵）:\n  %s", strings.Join(missing, "\n  "))
	}
}

// 反向：文档不许写幽灵端点。planned 之外每一条都必须真的注册了。
func TestOpenAPIHasNoGhostEndpoints(t *testing.T) {
	s := New(Dependencies{})
	registered := map[string]bool{}
	for _, rt := range s.routes() {
		registered[rt.Method+" "+rt.Pattern] = true
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatal(err)
	}
	for path, methods := range doc.Paths {
		for m := range methods {
			key := strings.ToUpper(m) + " " + path
			if !registered[key] && !planned[key] {
				t.Errorf("文档里有 %s，路由表里没有：要么补实现，要么标 x-status", key)
			}
		}
	}
}

// 方法集合也要对上：GET /api/v1/profiles 与 DELETE /api/v1/profiles 是两条，
// 文档里只写一条就是契约测试没意义。
func TestOpenAPIMethodSetsMatchPerPath(t *testing.T) {
	s := New(Dependencies{})
	byPath := map[string]map[string]bool{}
	for _, rt := range s.routes() {
		if byPath[rt.Pattern] == nil {
			byPath[rt.Pattern] = map[string]bool{}
		}
		byPath[rt.Pattern][strings.ToLower(rt.Method)] = true
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatal(err)
	}
	for path, got := range byPath {
		want := doc.Paths[path]
		if want == nil {
			t.Fatalf("文档里没有 %s", path)
		}
		for m := range got {
			if _, ok := want[m]; !ok {
				t.Errorf("%s 在路由表里有 %s，文档里没有", path, m)
			}
		}
		for m := range want {
			if !got[m] && !planned[strings.ToUpper(m)+" "+path] {
				t.Errorf("%s 文档里写了 %s，路由表里没有", path, m)
			}
		}
	}
}

// 端点本身公开可取（无个人信息），且字节与内嵌文档一致。
func TestOpenAPIEndpointServesTheEmbeddedDoc(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/api/v1/openapi.json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("公开端点该 200, got %d", rec.Code)
	}
	if rec.Body.String() != string(openAPIDoc) {
		t.Fatal("响应与内嵌文档不一致（中间有加工？）")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/httpapi/ -run OpenAPI -v`
Expected: 编译失败（`undefined: openAPIDoc`）。

- [ ] **Step 3: 写 `openapi.go`**

```go
package httpapi

import (
	"net/http"
)

// openAPIDoc 是手写的机器可读契约。为什么手写不生成：生成的文档只反映
// 代码"长什么样"，不会反映 spec 要求的行为（409 的形状、by 的枚举、
// 只写当前档案这类约定）；而骗 AI 的恰恰是行为。漂移由上面三条契约测试挡。
//
//go:embed openapi.json
var openAPIDoc []byte

// handleOpenAPI 直接吐内嵌字节，不解码再编码：文档里的键序也是给人/AI
// 读的一部分，重新编码会漂。
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(openAPIDoc)
}
```

`import` 要有 `_ "embed"`（`go:embed` 的指令需要它），文件顶部补：

```go
import _ "embed"
```

- [ ] **Step 4: 写 `openapi.json`**

文档必须逐条给全下面这张表的内容——**契约测试只挡"漏了整条路由"，挡不住"某条写得含糊"**，所以每条的 `summary`、凭证、响应码都在这里定死，实现者照抄成 JSON。`summary` 与路由表里的 `Summary` **逐字相同**（两条测试都靠它对齐）。

| 路径 | 方法 | 凭证 | 必须出现的响应 |
|---|---|---|---|
| `/healthz` | GET | 无 | `200` text/plain |
| `/` | GET | 无 | `200` HTML（选择页或首屏）、`304` |
| `/pin` | POST | 无 | `303`（表单）/`200`（JSON）/`401`/`404`/`422` |
| `/api/v1/openapi.json` | GET | 无 | `200` |
| `/api/v1/template` | GET | 无 | `200`/`400`/`404` |
| `/api/v1/batch` | POST | 令牌或会话 | `200 {profile,rev,applied,warnings[]}`/`401`/`403`/`404`/`409 {code,detail,currentRev}`/`422 {code,detail,errors[{opsIndex,field,reason}]}`/`503`/`507` |
| `/api/v1/data` | GET | 令牌或会话 | `200 {profile,rev,modified,data}`/`400`/`401`/`403`/`404`/`503` |
| `/api/v1/export` | GET | 令牌或会话 | `200`（磁盘原字节 + `Content-Disposition`）/`400`/`401`/`403`/`404` |
| `/api/v1/history` | GET | 令牌或会话 | `200 {entries[]}`/`400`/`401`/`403` |
| `/api/v1/revert` | POST | 令牌或会话 | `200 {profile,rev,restoredFrom}`/`400`/`401`/`403`/`404`/`507`；`X-LB-By` 头可选，缺省 `web` |
| `/api/v1/status` | GET | **仅管理员** | `200 {profiles[],brokenProfiles[],totals}`/`401`/`403` |
| `/api/v1/profiles` | GET | **仅管理员** | `200 {profiles[{id,title,admin,createdAt}]}`/`401`/`403` |
| `/api/v1/profiles` | POST | **仅管理员** | `201 {id,title,admin}`/`401`/`403`/`409 {code,detail}`/`422` |
| `/api/v1/profiles` | DELETE | **仅管理员** | `200 {deleted}`/`400`/`401`/`403`/`404` |
| `/api/v1/profiles/self/pin` | POST | **仅会话** | `200 {profile,pinChanged}`/`401`/`403`/`422` |
| `/api/v1/profiles/pin` | POST | **仅管理员** | `200 {profile,pinChanged}`/`401`/`403`/`404`/`422` |
| `/api/v1/usage` | POST | 令牌或会话 | `200 {updated,unknown[]}`/`400`/`401`/`403`/`422` |
| `/api/v1/icons` | GET | 无 | `x-status: planned`（P1-③） |
| `/api/v1/resolve` | POST | 令牌或会话 | `x-status: planned`（P1-③） |

`components` 至少要有：

- `securitySchemes.bearerAuth`（`type: http`、`scheme: bearer`）与 `securitySchemes.sessionCookie`（`type: apiKey`、`in: cookie`、`name: lb_profile`）。
- `schemas.BatchRequest`：`profile`(string, required)、`expectedRev`(integer, 可选)、`by`(string, enum 七值 `ai`/`bookmarklet`/`extension`/`web`/`import`/`sync-template`/`cli`)、`ops`(array of `Op`, required)。
- `schemas.Op`：`op`(enum `add`/`remove`/`set`/`move`)、`path`(string)、`value`(任意 JSON，`add`/`set` 用)、`index`(integer，可选)、`to`(string，`move` 用)。
- `schemas.BatchResponse`、`schemas.OpError`(`opsIndex`/`field`/`reason`)、`schemas.RevConflict`(`code`/`detail`/`currentRev`)。
- `responses.Unauthorized`/`Forbidden`/`NotFound` 三个复用项，body 都是 `{code:int, detail:string}`。

`info.description` 里必须写明两件事（AI 最容易踩）：**所有写一律走 `/api/v1/batch`，不许直接改 `data/`**；**`/api/v1/status` 与 `/api/v1/profiles*` 要管理员令牌，档案作用域令牌拿它们 403 是设计**。

写完之后 `python -c "import json;json.load(open('internal/httpapi/openapi.json',encoding='utf-8'))"` 必须过（JSON 语法错会让三条契约测试全部编译期通过、运行期红，红得没有信息量）。

- [ ] **Step 5: 注册路由**

```go
		{Method: "GET", Pattern: "/api/v1/openapi.json", Cred: credNone,
			Summary: "机器可读契约（无个人信息，公开）", H: s.handleOpenAPI},
```

- [ ] **Step 6: 跑测试 + 【故意破坏】两次**

Run: `go test ./internal/httpapi/ -run OpenAPI -v` → 全 PASS。

1. 从 `openapi.json` 里删掉 `/api/v1/usage` 那条 → `TestOpenAPICoversEveryRoute` 该红在"路由表里有、文档里没有"。补回。
2. 在文档里凭空加一条 `"/api/v1/whatever"` → `TestOpenAPIHasNoGhostEndpoints` 该红。**这两条一起构成"文档与代码不会各漂各的"的闸**——P1-③④ 再加端点时就靠它。

- [ ] **Step 7: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add internal/httpapi/
git commit -m "feat(httpapi): openapi.json 与三条路由契约测试"
```

---

## Task 12: `sync-template` 与 `profile create`（CLI 走 REST，默认干跑）

**Files:**
- Modify: `cmd/launchbay/cli.go`
- Test: `cmd/launchbay/clisync_test.go`

**Interfaces:**
- Consumes: `client.do`、`GET /api/v1/data`、`POST /api/v1/batch`、`POST /api/v1/profiles`、`Store.LoadTemplate`（只在测试夹具里建模板用）
- Produces:
  - `launchbay profile create --id mom --title 妈妈 --pin 1234 [--from-template nas]`
  - `launchbay sync-template --template nas --profile dad,mom [--apply]`
  - `func planSync(tmpl *model.Bookmarks, have *model.Bookmarks) (adds []ops.Op, conflicts []syncConflict)`
  - `type syncConflict struct { ItemID, Field, Have, Template string }`

- [ ] **Step 1: 写 diff 计算的测试（纯函数，先不碰 HTTP）**

`cmd/launchbay/clisync_test.go`：

```go
package main

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func tmpl() *model.Bookmarks {
	return &model.Bookmarks{Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList,
		Items: []model.Item{
			{ID: "portainer", Name: "Portainer", URL: "http://10.0.0.3:9000", Icon: "simple-icons:portainer", Alias: []string{}, Tags: []string{}},
			{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
			{ID: "qbit", Name: "下载", URL: "http://10.0.0.3:8181", Alias: []string{}, Tags: []string{}},
		},
	}}}
}

func TestPlanSyncAddsOnlyMissingAndReportsConflicts(t *testing.T) {
	have := &model.Bookmarks{Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList,
		Items: []model.Item{
			// 同 id 但名字被本人改过：只报告，不覆盖（spec §4.5）。
			{ID: "dsm", Name: "我家的群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
		},
	}}}
	adds, conflicts := planSync(tmpl(), have)
	if len(adds) != 2 {
		t.Fatalf("该补 portainer 与 qbit: %+v", adds)
	}
	for _, a := range adds {
		if a.Op != "add" || a.Path != "groups/nas/items" {
			t.Fatalf("补的必须是 add 到已存在的组: %+v", a)
		}
	}
	if len(conflicts) != 1 || conflicts[0].ItemID != "dsm" || conflicts[0].Field != "name" {
		t.Fatalf("冲突该点名 dsm 的 name: %+v", conflicts)
	}
	// 冲突项不许同时出现在 adds 里（否则 --apply 会把本人的改名盖掉）。
	for _, a := range adds {
		if strings.Contains(string(a.Value), `"dsm"`) {
			t.Fatalf("冲突条目被当成新增: %+v", a)
		}
	}
}

// 幂等：第二次跑该零新增。spec §13 点名的就是这条。
func TestPlanSyncIsIdempotent(t *testing.T) {
	have := &model.Bookmarks{Groups: []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList}}}
	adds, _ := planSync(tmpl(), have)
	if len(adds) != 3 {
		t.Fatalf("空组该补三条: %+v", adds)
	}
	// 把 adds 应用一次（用 ops.Run 而不是手抄，顺带验证 add 的 value 形状合法）。
	b, errs, _ := ops.Run(have, adds)
	if len(errs) != 0 {
		t.Fatalf("应用失败: %+v", errs)
	}
	adds2, conflicts2 := planSync(tmpl(), b)
	if len(adds2) != 0 {
		t.Fatalf("第二次该零新增: %+v", adds2)
	}
	if len(conflicts2) != 0 {
		t.Fatalf("第二次不该有冲突: %+v", conflicts2)
	}
}

// 模板里有条目而档案里连那个组都没有：补条目要先有组。
// 这里的取法是"整组新增"（add groups/<gid> 带 items），不是先建空组再补条目
// ——后者会在中间态留下一个空分组。
func TestPlanSyncAddsWholeGroupWhenMissing(t *testing.T) {
	have := &model.Bookmarks{}
	adds, _ := planSync(tmpl(), have)
	if len(adds) != 1 || adds[0].Path != "groups" {
		t.Fatalf("该整组补: %+v", adds)
	}
	b, errs, _ := ops.Run(have, adds)
	if len(errs) != 0 || len(b.Groups) != 1 || len(b.Groups[0].Items) != 3 {
		t.Fatalf("应用后形状不对: %+v %+v", errs, b.Groups)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./cmd/launchbay/ -run TestPlanSync -v`
Expected: 编译失败（`undefined: planSync`）。

- [ ] **Step 3: 写 `planSync` 与两个子命令**

`cmd/launchbay/cli.go` 追加：

```go
type syncConflict struct {
	ItemID   string `json:"itemId"`
	GroupID  string `json:"groupId"`
	Field    string `json:"field"`
	Have     string `json:"have"`
	Template string `json:"template"`
}

// planSync 只补"档案里没有的模板条目"，其余一律不动（spec §4.5）。
// 同 id 但 name/url 不同 → 只报告。不做字段级三方合并、不存同步基线：
// 那要第三份状态，而"报告冲突让人挑"比"自动合并"便宜且不会猜错。
func planSync(tmpl, have *model.Bookmarks) ([]ops.Op, []syncConflict) {
	var adds []ops.Op
	var conflicts []syncConflict
	haveGroup := map[string]*model.Group{}
	for i := range have.Groups {
		haveGroup[have.Groups[i].ID] = &have.Groups[i]
	}
	seenItem := map[string]bool{}
	for _, it := range allItems(have) {
		seenItem[it.ID] = true
	}
	for _, g := range tmpl.Groups {
		hg, ok := haveGroup[g.ID]
		if !ok {
			// 整组新增：一次 add 带 items，不留"空组 + 补条目"的中间态。
			v, _ := json.Marshal(g)
			adds = append(adds, ops.Op{Op: "add", Path: "groups", Value: v})
			for _, it := range g.Items {
				seenItem[it.ID] = true
			}
			continue
		}
		for _, it := range g.Items {
			if seenItem[it.ID] {
				for _, hi := range haveGroup[g.ID].Items {
					if hi.ID != it.ID {
						continue
					}
					if hi.Name != it.Name {
						conflicts = append(conflicts, syncConflict{ItemID: it.ID, GroupID: g.ID,
							Field: "name", Have: hi.Name, Template: it.Name})
					}
					if hi.URL != it.URL {
						conflicts = append(conflicts, syncConflict{ItemID: it.ID, GroupID: g.ID,
							Field: "url", Have: hi.URL, Template: it.URL})
					}
				}
				continue
			}
			v, err := json.Marshal(it)
			if err != nil {
				continue // model.Item 全是可序列化字段，到这里是编码 bug，交给 batch 报错
			}
			adds = append(adds, ops.Op{Op: "add", Path: "groups/" + g.ID + "/items", Value: v})
			seenItem[it.ID] = true
		}
	}
	return adds, conflicts
}

func allItems(b *model.Bookmarks) []model.Item {
	var out []model.Item
	for _, g := range b.Groups {
		out = append(out, g.Items...)
	}
	return out
}
```

`cmdSyncTemplate`：

```go
func cmdSyncTemplate(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("sync-template", flag.ContinueOnError)
	fs.SetOutput(out)
	tmplName := fs.String("template", "nas", "模板名（data/templates/<name>.json）")
	profiles := fs.String("profile", "", "逗号分隔的档案 id")
	apply := fs.Bool("apply", false, "真的写入（默认只干跑打印）")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profiles == "" {
		fmt.Fprintln(out, "用法: launchbay sync-template --template nas --profile dad,mom [--apply]")
		return 2
	}
	c, err := newAdminClient()
	if err != nil {
		fmt.Fprintf(out, "sync-template 失败: %v\n", err)
		return 1
	}
	// 模板是站级资源、不在任何档案上，而 CLI 不许直读磁盘（spec §4.5），
	// 所以走 Step 4 新增的 GET /api/v1/template?name=。
	tmpl, err := fetchTemplate(c, *tmplName)
	if err != nil {
		fmt.Fprintf(out, "sync-template 失败: 模板 %q 读不到: %v
", *tmplName, err)
		return 1
	}
	for _, id := range strings.Split(*profiles, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		have, err := fetchBookmarks(c, id)
		if err != nil {
			fmt.Fprintf(out, "%s: 读不到，跳过（%v）\n", id, err)
			continue
		}
		adds, conflicts := planSync(tmpl, have)
		fmt.Fprintf(out, "== %s ==\n", id)
		if len(adds) == 0 {
			fmt.Fprintln(out, "  没有要补的条目")
		}
		for _, a := range adds {
			fmt.Fprintf(out, "  会新增 %s → %s\n", a.Path, summarizeValue(a.Value))
		}
		for _, cf := range conflicts {
			fmt.Fprintf(out, "  只报告不覆盖：%s.%s 档案里是 %q，模板里是 %q\n", cf.ItemID, cf.Field, cf.Have, cf.Template)
		}
		if !*apply {
			fmt.Fprintf(out, "  干跑：什么都没写。要写加 --apply\n")
			continue
		}
		if len(adds) == 0 {
			continue // 没有改动就不产生 rev
		}
		if err := applyOps(c, id, adds); err != nil {
			fmt.Fprintf(out, "  %v\n", err)
			return 1
		}
		fmt.Fprintf(out, "  已写入 %d 条\n", len(adds))
	}
	return 0
}
```

`fetchBookmarks` / `applyOps` / `summarizeValue`：

```go
// fetchBookmarks 经 REST 读一个档案（或模板）的全量数据。
func fetchBookmarks(c *client, id string) (*model.Bookmarks, error) {
	raw, code, err := c.do("GET", "/api/v1/data?profile="+id, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := mustOK(raw, code, "/api/v1/data"); err != nil {
		return nil, err
	}
	var out struct {
		Data model.Bookmarks `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

// applyOps 一次 batch 写完，by 固定 sync-template：历史里要能看出
// "这一批不是人手工加的，是模板同步带进来的"。
func applyOps(c *client, profile string, adds []ops.Op) error {
	body, err := json.Marshal(map[string]any{
		"profile": profile, "by": "sync-template", "ops": adds})
	if err != nil {
		return err
	}
	raw, code, err := c.do("POST", "/api/v1/batch", body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	return mustOK(raw, code, "/api/v1/batch")
}

func summarizeValue(v json.RawMessage) string {
	var it struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(v, &it)
	if it.Name != "" {
		return it.ID + " " + it.Name
	}
	return it.ID
}
```

`cmdProfile`：

```go
func cmdProfile(args []string, out io.Writer) int {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintln(out, "用法: launchbay profile create --id <slug> --title <名字> --pin <4-6 位> [--from-template nas] [--admin]")
		return 2
	}
	fs := flag.NewFlagSet("profile create", flag.ContinueOnError)
	fs.SetOutput(out)
	id := fs.String("id", "", "档案 id（ASCII slug）")
	title := fs.String("title", "", "显示名字")
	pin := fs.String("pin", "", "4-6 位数字")
	from := fs.String("from-template", "", "从 data/templates/ 下哪个模板克隆")
	admin := fs.Bool("admin", false, "建成管理员档案")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	c, err := newAdminClient()
	if err != nil {
		fmt.Fprintf(out, "profile create 失败: %v\n", err)
		return 1
	}
	body, _ := json.Marshal(map[string]any{
		"id": *id, "title": *title, "pin": *pin,
		"fromTemplate": *from, "admin": *admin})
	raw, code, err := c.do("POST", "/api/v1/profiles", body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		fmt.Fprintf(out, "profile create 失败: %v\n", err)
		return 1
	}
	if err := mustOK(raw, code, "/api/v1/profiles"); err != nil {
		fmt.Fprintf(out, "profile create 失败: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "已建档 %s\n", *id)
	return 0
}
```

**`POST /api/v1/profiles` 现在只吃 `id/title/pin/fromTemplate` 四个键**（P1-① 的实现），`admin` 是本案新增的第五个键：`handleCreateProfile` 的匿名 struct 要加 `Admin bool `json:"admin"``，并传给 `store.NewProfileInput{Admin: in.Admin}`。**`DisallowUnknownFields` 会让不加这个键的实现直接 422**——这条不是可选步骤。

- [ ] **Step 4: 加 `GET /api/v1/template`（模板经服务读，CLI 不直读磁盘）**

`internal/httpapi/read.go` 追加：

```go
// handleTemplate 暴露 data/templates/ 下的模板：sync-template 要拿它做
// diff，而 CLI 不许直读磁盘（spec §4.5）。模板是站级共享资源、不含个人
// 书签，所以 credNone 就够——但**只读**，且只认 ASCII slug 的模板名。
func (s *Server) handleTemplate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" || !model.ValidSlug(name) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "detail": "要带 ?name=<ASCII slug>"})
		return
	}
	b, err := s.deps.Store.LoadTemplate(name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "detail": "模板不存在"})
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "data": b})
}
```

路由：

```go
		{Method: "GET", Pattern: "/api/v1/template", Cred: credNone,
			Summary: "读 data/templates/ 下的建档模板", H: s.handleTemplate},
```

**同一步之内补 `internal/httpapi/openapi.json`**（Task 11 的契约测试逐条对照路由表，加了路由不补文档就会在本案收尾门禁红一次）：

- `paths./api/v1/template.get`：`summary` 与路由表**逐字相同**，凭证为无，响应 `200 {name,data}` / `400` / `404`。
- `paths./api/v1/profiles.post` 的 `requestBody` 所指 schema 里补 `admin`(boolean) —— 本任务给建档加了第五个键，文档不补就是骗 AI。

补完立刻跑 `go test ./internal/httpapi/ -run OpenAPI -v`，三条都该绿。

`fetchTemplate` 与 `fetchBookmarks` 是两个函数，别在一个里塞 if——两者的响应形状不同（`{name,data}` 与 `{profile,rev,modified,data}`）：

```go
func fetchTemplate(c *client, name string) (*model.Bookmarks, error) {
	raw, code, err := c.do("GET", "/api/v1/template?name="+name, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := mustOK(raw, code, "/api/v1/template"); err != nil {
		return nil, err
	}
	var out struct {
		Data model.Bookmarks `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
```

- [ ] **Step 5: 写 CLI 端到端测试（假服务 = 真 handler + 真 store）**

`cmd/launchbay/clisync_test.go` 追加：

```go
// 用真的 httpapi handler 当假服务：CLI 的每一次调用都会真的落到临时目录
// 里的档案上。这样"只经 REST"与"干跑不写盘"都是实测，不是断言愿望。
func newRealServer(t *testing.T) (*store.Store, *httptest.Server) {
	t.Helper()
	s := store.New(t.TempDir())
	hash := func(pin string) string {
		h, err := auth.HashPIN(pin)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	// dad：管理员档案，名字与 URL 都故意与模板不同（造出"只报告不覆盖"的现场）。
	if _, err := s.CreateProfile(store.NewProfileInput{
		ID: "dad", Title: "爸爸", PinHash: hash("1234"), Admin: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 1, Title: "发射台", Groups: []model.Group{{
		ID: "nas", Title: "NAS", Layout: model.LayoutList,
		Items: []model.Item{{ID: "dsm", Name: "我家的群晖", URL: "http://10.0.0.3:5000",
			Alias: []string{}, Tags: []string{}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	// 模板 nas 落在 data/templates/nas.json（服务经 /api/v1/template 暴露它）。
	if err := s.WriteFileJSON(s.TemplatePath("nas"), tmpl()); err != nil {
		t.Fatal(err)
	}
	scopes, err := auth.ParseScopes("ai-dad:tok-dad:dad,admin:tok-admin:*")
	if err != nil {
		t.Fatal(err)
	}
	return s, httptest.NewServer(httpapi.New(httpapi.Dependencies{
		Store: s, Tokens: scopes, Render: render.New(s)}).Handler())
}

func TestSyncTemplateDryRunWritesNothing(t *testing.T) {
	s, srv := newRealServer(t)
	defer srv.Close()
	before, err := s.LoadBookmarks("dad")
	if err != nil {
		t.Fatal(err)
	}
	c := &client{base: srv.URL, token: "tok", hc: srv.Client()}
	adds := mustPlan(t, c, "nas", "dad")
	if len(adds) == 0 {
		t.Fatal("模板里有 dad 没有的条目，plan 该非空")
	}
	after, _ := s.LoadBookmarks("dad")
	if after.Rev != before.Rev {
		t.Fatalf("干跑把 rev 推进了: %d → %d", before.Rev, after.Rev)
	}
}

func TestSyncTemplateApplyWritesOnceAndIsIdempotent(t *testing.T) {
	s, srv := newRealServer(t)
	defer srv.Close()
	c := &client{base: srv.URL, token: "tok", hc: srv.Client()}
	if err := runSyncApply(c, "nas", []string{"dad"}); err != nil {
		t.Fatal(err)
	}
	b, _ := s.LoadBookmarks("dad")
	if b.Rev != 2 {
		t.Fatalf("第一次 apply 该产生一个 rev: %d", b.Rev)
	}
	// 第二次：零新增 → 不产生 rev、不写历史（applyOps 空批直接不调）。
	if err := runSyncApply(c, "nas", []string{"dad"}); err != nil {
		t.Fatal(err)
	}
	b2, _ := s.LoadBookmarks("dad")
	if b2.Rev != 2 {
		t.Fatalf("第二次 apply 不该再动 rev: %d", b2.Rev)
	}
	hist, _ := s.ReadHistory("dad", 0)
	if len(hist) != 1 || hist[0].By != "sync-template" {
		t.Fatalf("历史该只有一条且标 sync-template: %+v", hist)
	}
}

// 冲突只报告：本人改过的名字，--apply 之后仍然是他自己的。
func TestSyncApplyNeverOverwritesAConflict(t *testing.T) {
	s, srv := newRealServer(t)
	defer srv.Close()
	c := &client{base: srv.URL, token: "tok", hc: srv.Client()}
	if err := runSyncApply(c, "nas", []string{"dad"}); err != nil {
		t.Fatal(err)
	}
	b, _ := s.LoadBookmarks("dad")
	for _, g := range b.Groups {
		for _, it := range g.Items {
			if it.ID == "dsm" && it.Name != "我家的群晖" {
				t.Fatalf("--apply 把本人改的名字盖掉了: %q", it.Name)
			}
		}
	}
}
```

本文件的 import 一次给全（前三个用例用纯函数，后三个起才要服务）：

```go
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/httpapi"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)
```

`mustPlan` / `runSyncApply` 是测试用的两个薄封装，落在 `cmdSyncTemplate` 已经有的调用序列上（读模板 → 读档案 → `planSync` → 可选 `applyOps`），**不新开一份逻辑**：

```go
func mustPlan(t *testing.T, c *client, tmplName, profile string) []ops.Op {
	t.Helper()
	tmpl, err := fetchTemplate(c, tmplName)
	if err != nil {
		t.Fatal(err)
	}
	have, err := fetchBookmarks(c, profile)
	if err != nil {
		t.Fatal(err)
	}
	adds, _ := planSync(tmpl, have)
	return adds
}

func runSyncApply(c *client, tmplName string, profiles []string) error {
	tmpl, err := fetchTemplate(c, tmplName)
	if err != nil {
		return err
	}
	for _, id := range profiles {
		have, err := fetchBookmarks(c, id)
		if err != nil {
			return err
		}
		adds, _ := planSync(tmpl, have)
		if len(adds) == 0 {
			continue // 没有改动就不产生 rev
		}
		if err := applyOps(c, id, adds); err != nil {
			return err
		}
	}
	return nil
}
```

`cmdSyncTemplate` 的循环体与 `runSyncApply` 是同一段调用序列（前者多了打印）——**把这段序列抽成一个 `syncOne(c, tmpl, id, apply, out)` 函数，两边都调它**，别让"测的"与"跑的"是两份代码。

```go
// syncOne 是一个档案的一次同步：读 → 算 → （可选）写 → 打印。
// 命令层与测试都走它，"跑的"与"测的"因此是同一段代码；out 传
// io.Discard 就是无声执行。返回本次新增的条数。
func syncOne(c *client, tmpl *model.Bookmarks, profile string, apply bool, out io.Writer) (int, error) {
	have, err := fetchBookmarks(c, profile)
	if err != nil {
		return 0, err
	}
	adds, conflicts := planSync(tmpl, have)
	for _, a := range adds {
		fmt.Fprintf(out, "  会新增 %s → %s
", a.Path, summarizeValue(a.Value))
	}
	for _, cf := range conflicts {
		fmt.Fprintf(out, "  只报告不覆盖：%s.%s 档案里是 %q，模板里是 %q
",
			cf.ItemID, cf.Field, cf.Have, cf.Template)
	}
	if !apply || len(adds) == 0 {
		return len(adds), nil
	}
	return len(adds), applyOps(c, profile, adds)
}
```

`cmdSyncTemplate` 的循环体缩成"取模板 → 逐个 `syncOne` → 按 `apply` 打印收尾"；`mustPlan` 用 `syncOne(..., apply=false, io.Discard)` 之外的通路（直接 `planSync`）拿 adds，`runSyncApply` 用 `syncOne(..., true, io.Discard)`。

- [ ] **Step 6: 【故意破坏】两次**

1. 把 `planSync` 的"同 id 已存在就跳过"改成"同 id 也补一份" → `TestPlanSyncIsIdempotent` 该红（第二次非零新增）。改回。
2. 把 `planSync` 里冲突项也塞进 `adds`（"顺手帮用户同步成模板值"）→ `TestPlanSyncAddsOnlyMissingAndReportsConflicts` 该红在"冲突条目被当成新增"，且 `TestSyncApplyNeverOverwritesAConflict` 该红。**两条都红才对**：一条钉计算，一条钉端到端后果。

- [ ] **Step 7: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add cmd/launchbay/ internal/httpapi/
git commit -m "feat(cli,sync): profile create 与 sync-template（默认干跑、冲突只报告、幂等）"
```

---

## Task 13: `AGENTS.md`、spec 修正与容器内端到端验收

**Files:**
- Create: `AGENTS.md`
- Modify: `docs/superpowers/specs/2026-10-05-launchbay-design.md`
- Test: `internal/httpapi/agents_test.go`（放 `docs/` 下的 `_test.go` 不会被 `go test ./...` 收到，所以闸必须挂在已编译的包里）

**Interfaces:**
- Consumes: 全部前序任务
- Produces: 给 AI 读的契约 + 本案的端到端验收记录

- [ ] **Step 1: 写 `AGENTS.md`**

内容按 spec §6.4 的清单逐条给，每条都要有能直接抄的命令。

**先给端点清单**：路由表里除 `/`、`/pin`、`/healthz`（浏览器通路）之外**每一条**都要在 `AGENTS.md` 里出现，写明方法、要哪种凭证、成功响应长什么样。Step 2 的 `TestAgentsMDMatchesRoutes` 会逐条查这条——它防的是"文档比代码旧"。

**必写的五条坑**（都是本案实测踩过或验收抓到的）：

1. 令牌从 `LAUNCHBAY_WRITE_TOKENS` 拿，三段式 `name:secret:profile`；**作用域决定能写哪个档案**，`*` 才是管理员。`GET /api/v1/status` 与 `/api/v1/profiles*` 是跨档案端点，档案作用域令牌拿它们 **403 是设计不是故障**（spec §6.4 原话）。查单档案用 `GET /api/v1/data?profile=<id>`。
2. **改完必须 `GET /api/v1/data` 复核**，别信 batch 的 200 就是"我想要的形状"。
3. `search.default` 必须是 `search.engines[].prefix` 里的值，**不是 `name`**。写错会让整个档案进坏档页（本案部署验收踩过：`"default":"bing"` + `engines[].prefix="b"` → 全档案读不了）。
4. 挑图标前查 `GET /api/v1/icons?q=...`——该端点 **P1-③ 才上线**，现在标在 openapi 里是 `x-status: planned`。在它上线之前，`icon` 只能用 `tabler:<name>` 或 `simple-icons:<name>` 两种前缀，名字写错只会被 `checkIcon` 按形状放过、渲染时走首字母兜底。
5. 一次改一批用 `POST /api/v1/batch` 带 `expectedRev`；409 就重读再试，**不许**把 `expectedRev` 删掉绕过冲突检查。

禁止事项照 spec 抄：不许直接改 `data/` 下的文件（写通路只有一条 REST）、不许 `--apply` 跑 `sync-template` 除非人明确要求、不许把真实档案目录或真实内网地址提交进仓库（文档与夹具只用 `10.` 段）。

- [ ] **Step 2: 给 `AGENTS.md` 上一道闸（防它漂成假话）**

`internal/httpapi/agents_test.go`：

```go
// AGENTS.md 里写到的每个端点都必须真的在路由表里。文档漂了要红，
// 而不是等 AI 打出一个 404 才发现。
func TestAgentsMDMatchesRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatalf("AGENTS.md 读不到: %v", err)
	}
	text := string(raw)
	s := New(Dependencies{})
	for _, rt := range s.routes() {
		if rt.Pattern == "/" || rt.Pattern == "/pin" || rt.Pattern == "/healthz" {
			continue // 浏览器通路，AI 契约里不必逐条点名
		}
		if !strings.Contains(text, rt.Pattern) {
			t.Errorf("路由表里有 %s，AGENTS.md 没提", rt.Pattern)
		}
	}
	// 反向：AGENTS.md 里出现的 /api/v1/... 字面量必须是已注册或 planned 的。
	for _, m := range apiPathRE.FindAllString(text, -1) {
		if !knownAPIPath(s, m) && !planned[m] {
			t.Errorf("AGENTS.md 写了 %s，但服务里没有这个端点", m)
		}
	}
}

var apiPathRE = regexp.MustCompile(`/api/v1/[a-z/]+`)
```

`knownAPIPath(s, m)` 遍历 `s.routes()` 比 `Pattern`。`planned` 复用 `openapi_test.go` 里那份（同包，一份就够）。

- [ ] **Step 3: spec 的两处修正（本案实施过程中定下来的）**

改 `docs/superpowers/specs/2026-10-05-launchbay-design.md`：

1. §10 的 compose 片段里删掉 `LAUNCHBAY_ADMIN_PROFILE: "dad"` 一行——全文没有任何一处消费它，管理员资格在 §6.1 由磁盘上的 `profile.json` 给出。留着它会让下一个读 spec 的人以为有个开关。
2. §4.6 的 `by` 枚举补一个值：`ai | bookmarklet | extension | web | import | sync-template | cli`，并加一句：`revert` 与改 PIN 这类动作在历史里记成 `ops:[{op:"revert"|"set", path:"snapshots/<rev>"|"profile/pin"}]`，`rev:0` 表示这次动作不产生书签 rev。

两处都是**改 spec 的措辞，不改任何已实现的行为**；diff 要小，逐字改完 `git diff` 复核。

- [ ] **Step 4: 容器内端到端验收（本案的"真跑一遍"）**

```bash
export PATH="$PATH:/c/Program Files/Go/bin"
LAUNCHBAY_HOST_PORT=17600 docker compose up -d --build
B=http://127.0.0.1:17600
```

七条，逐条把输出落盘（含中文的判定**不要扫 stdout**，写文件再读）：

1. `GET $B/api/v1/openapi.json` → 200，且 `paths` 数与路由表一致（`go test` 已经钉过，这里是取一次真字节）。
2. 用 `.env` 里的管理员令牌，`POST /api/v1/batch` 一次 **30 条 ops** 的批量整理（新建一组 + 移 10 条 + 改 10 个 name + 删 5 条 + 加 5 条）。请求体**必须从文件发**（`--data-binary @body.json`，用 node 写文件保证 UTF-8）——Git Bash 的 argv 会把中文按 GBK 送出去，实测踩过。
3. `GET /api/v1/data?profile=dad` 复核：`rev` 前进一次、条目数对得上、30 条改动逐条可见。
4. `GET /api/v1/history?profile=dad&limit=3` → 只有一条新记录，`by:"ai"`，`ops` 有 30 条且**不含任何 value**。
5. `ls data/profiles/dad/snapshots/` → 恰好多一份旧 rev 快照；`POST /api/v1/revert?profile=dad&to-rev=<旧值>` → 200，`GET /api/v1/data` 看到内容回到改前、`rev` 又前进一格、历史多一条 `revert`。
6. `docker exec launchbay /launchbay sync-template --template nas --profile dad,mom` → 打印"会新增 N 条"与冲突清单，且 `GET /api/v1/data` 的 `rev` **没变**（干跑）。加 `--apply` 再跑一次 → `rev` 变、历史多一条 `by:"sync-template"`；**再跑一次** → "没有要补的条目"、`rev` 不动（幂等实测）。
7. 浏览器实拍：`GET /` 的档案页在 1440/390 两档 × 亮/暗 两态仍然正常（本案没动渲染层，这条是防"API 改动连带把首屏弄坏"）。截图落 `.superpowers/tmp/` 后逐张看。

- [ ] **Step 5: 泄漏守卫（本案加了 CLI 与 AGENTS.md，最容易带进真地址）**

```bash
git ls-files -z | xargs -0 grep -nHoE '([0-9]{1,3}\.){3}[0-9]{1,3}' | grep -vE '(^|[^0-9])10\.' | sort -u
```

Expected: 空输出。任何一行都要当场改掉——白名单只放 `10.`（spec §11）。同时确认 `AGENTS.md` 与 `openapi.json` 里没有 `.env` 里的真实令牌值：

```bash
grep -RInE 'replace-me|local-accept|tok-' --include='*.go' --include='*.json' --include='*.md' . | grep -v '^./.superpowers' | grep -v .env.example
```

Expected: 只有 `.env.example` 的 `replace-me-*` 与测试夹具里的 `tok-dad`/`tok-admin`（它们是**假**令牌，形状需要，值不含环境信息）。

- [ ] **Step 6: 门禁 + 提交**

```bash
go test ./... && go vet ./... && gofmt -l .
git add AGENTS.md internal/httpapi/agents_test.go docs/superpowers/specs/
git commit -m "docs: AGENTS.md 写通路契约（含端点表一致性闸）与 spec 两处修正"
```

---

## Spec 覆盖对账（本案没覆盖的部分与去向）

| spec 节 | 本案 | 缺口去向 |
|---|---|---|
| §4.4 ops 与 rev | ✅ Task 1/2/5 | — |
| §4.5 sync-template / 建档 CLI | ✅ Task 12 | — |
| §4.6 历史与恢复 | ✅ Task 3/7（含 `by` 补 `cli` 的 spec 修正） | 历史的前端展示 → **P1-④** |
| §5 图标双层 | ❌ 本案只在 `AGENTS.md` 里预告 | **P1-③**：索引内嵌、`/api/v1/icons`、`/api/v1/resolve`、SVG 内联、favicon 抓取 |
| §6.1 REST 契约 | ✅ 除 `icons`/`resolve` | **P1-③**（那两个端点） |
| §6.2 校验先于落盘 | ✅ Task 2/5（逐条 opsIndex） | — |
| §6.3 令牌与 bookmarklet | ✅ 令牌判定收敛到 `require`（Task 4） | bookmarklet → **P1-⑤** |
| §6.4 `AGENTS.md` | ✅ Task 13（含一致性测试闸） | — |
| §7 渲染与缓存 | ⚠️ 本案的 API 响应都吃 no-store 中间件 | SW/预算闸/首屏构成 → **P1-④** |
| §8 UI 规格 | ❌ 本案零渲染改动 | **P1-④**（含部署验收 #1/#3/#4） |
| §9 flare 导入 | ❌ | **P1-⑤** |
| §10 部署 | ✅ Task 7 的 CLI 走容器 env | 接管 7570 由人工确认（spec §15） |
| §11 安全 | ✅ 令牌作用域唯一实现处、字段白名单（`ops.Brief` 类型层挡 value）、拒非法 UTF-8 | 网段守卫进 CI → **P1-⑤** |
| §12 错误处理 | ✅ 401/403/404/409/422/503/507 分流 + 坏档案 API 侧 | 前端乐观更新回滚 → **P1-④** |
| §13 测试 | ✅ store/auth/api/usage/sync-template 五组 + 契约测试 + 12 次【故意破坏】 | 离线端到端三条、预算闸、三档实拍全量 → **P1-④** |
| §15 目录改名 | ❌ 仍未做 | 必须由不持有该目录为 cwd 的会话做，见下 |

**目录改名 `365/042` → `365/042-launchbay`**：与 P1-① 同样的理由，本案不含这一步。

**接手 P1-③ 前必须已完成的事实**：`go test ./...` 全绿；容器 `healthy`；Task 13 Step 4 的七条端到端有落盘证据；Step 5 的两条泄漏守卫都是空输出。
