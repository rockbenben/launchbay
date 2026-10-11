# 坏档案的机器出路（profile rebuild）实施计划（P1-④.6）

> **本文是历史实施记录，不是待办。** 它记录的是那一天计划或量的是什么，里面的
> 函数名、测试名与接口形状**可能已经变了**——2026-10-10 的管理员边界收紧一次性改判掉了
> 代管态、模板机制与徽标切档，共享清单也改判为不做。**不要照着这里的命令执行**：
> 权威契约在仓库根的 `AGENTS.md`，设计在 `docs/superpowers/specs/`。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给"档案的 `profile.json` 读不动"这一种坏法加一条只重写 `profile.json` 的通路（REST 端点 + CLI 子命令），书签、快照、历史一个字都不碰。

**Architecture:** store 里新增 `RebuildProfile`，把"读得动吗 / 有没有这份档案"两个判据收进同一次按 id 的临界区；httpapi 只加一个 `credAdmin` 端点做外壳与分诊；PIN 的"格式 + 哈希"抽成建档族共用的一个私有函数，并同轮改那道 AST 闸，使这条新端点不能成为 PIN 判据的第二处。零新页面形状、零新 CSS、零 JS。

**Tech Stack:** Go 1.27（`CGO_ENABLED=0`）、标准库 `net/http` + `html/template`、`encoding/json`、`testing` + `httptest`；契约面是 `routes()` ↔ `openapi.json` ↔ `AGENTS.md` 三处，加那道逐条点名的契约闸（名单在 Task 4 Step 9）。

**Spec:** `docs/superpowers/specs/2026-10-09-launchbay-profile-repair-design.md`（下称"设计 §x"）。冲突一律以设计文档与权威源 `docs/superpowers/specs/2026-10-05-launchbay-design.md` 为准，权威源改字由 Task 1 做掉。

## Global Constraints

- Go 1.27；`CGO_ENABLED=0`（本机跑不了 `-race`，并发证人要在 Linux 容器里补跑一遍）；**零第三方依赖**。
- 本期**零 JavaScript**、**零新页面形状**：本片不动 `internal/render/**`。
- 不许直改 `data/`；书签与档案的写入只经 REST（CLI 只经本机 REST，不直读磁盘）。
- 磁盘 JSON 形状（权威源 §4.3）：缩进 2 空格、文件末尾恰好一个换行、`SetEscapeHTML(false)`、非 ASCII 原样、空数组写 `[]`、键序由 struct 字段声明顺序固定。写盘一律走 `store.WriteFileJSON` / `WriteFileAtomic`，不许自己 `json.Marshal`。
- 令牌只从 `LAUNCHBAY_WRITE_TOKENS` 取；不打印、不回显、不写进历史。文档与测试夹具只用 `10.` 段示例地址与 `127.0.0.1`；真实内网地址与真实档案目录不进仓库。
- **不加严格安全**：不做限流、不做失败计数/锁定、不给 cookie 签名、不引入 TLS 终结、不加"改管理员要二次凭证"。多出来的防护也算失败（权威源 §11）。
- **判据只有一份**：新端点不许成为第二条建档通路（不建目录）、PIN 判据不许有第二处（Task 3）。
- 预算三条操作线 `16_000 / 48_000 / 24_000` **不许抬**；本片读数应当一个字都不动，若动了必须在 Task 6 整组重测并说明为什么。
- 证人要**因它声称要钉的那个缺陷**变红：每条新断言配一把只打它的枪，破坏脚本必须先断 `mutated != original`、区分 `build_err`、跑完用 sha 证还原。
- 提交按功能域分组，message 写实质不写过程词；**push 先问**（这个仓库目前没有 remote）。
- 每个任务结束时 `go test ./... -count=1` 必须 exit 0、`gofmt -l internal cmd` 必须为空、`go vet ./...` 必须为 0。

---

## 文件结构（谁负责什么）

| 文件 | 责任 | 动作 |
|---|---|---|
| `docs/superpowers/specs/2026-10-05-launchbay-design.md` | 权威源：判据与契约的单一真相 | 改九处（Task 1） |
| `internal/store/paths.go` | store 的哨兵错误与路径 | 加 `ErrReadable`（Task 2） |
| `internal/store/rebuild.go` | "只重写 profile.json"这件事的唯一实现与唯一判据 | 新建（Task 2） |
| `internal/store/rebuild_test.go` | 上面那些判据的证人 | 新建（Task 2） |
| `internal/httpapi/handlers.go` | 建档唯一落点 `createProfile` + 共用的 PIN 判据 | 抽 `hashFormPIN`（Task 3） |
| `internal/httpapi/createrule_test.go` | "判据只有一份"那道 AST 闸 | 改口径（Task 3） |
| `internal/httpapi/rebuild.go` | 端点外壳：解码、鉴权、分诊、历史行 | 新建（Task 4） |
| `internal/httpapi/rebuild_test.go` | 端点行为证人（401/403/404/409/422/500/200 + 端到端） | 新建（Task 4） |
| `internal/httpapi/routes.go` | 路由表的唯一注册源 | 加一行（Task 4） |
| `internal/httpapi/openapi.json` | 机器可读契约 | 加一个 operation（Task 4） |
| `AGENTS.md` | 给 AI 读的写通路契约 | 加一行 + 一处措辞（Task 4） |
| `cmd/launchbay/cli.go` | CLI 子命令（只经本机 REST） | 加 `profile rebuild`（Task 5） |
| `cmd/launchbay/cli_test.go` | CLI 的参数→请求体接线证人 | 加两条（Task 5） |
| `docs/superpowers/acceptance/p1-46-rebuild/` | 端到端验收记录与读数 | 新建（Task 6） |

---

### Task 1: 权威源改字（设计 §6 的九条清单）

**Files:**
- Modify: `docs/superpowers/specs/2026-10-05-launchbay-design.md`（九处，锚点按字面量给，别用行号）

**Interfaces:**
- Consumes: 设计文档 `2026-10-09-launchbay-profile-repair-design.md` 的 §6 表。
- Produces: 权威源里"坏档案怎么出去"的口径。Task 4 写 AGENTS.md 那一行时要与这里一致。

- [ ] **Step 1: 先读九处锚点，确认它们现在长什么样**

Run: `cd 仓库根 && grep -n "也吃同一条纪律\|免凭证的四条\|^| \`/api/v1/profiles/pin\` | POST\|^8\. \*\*管理页\*\*\|零档案时被抢装\|明写不防（有意为之\|一个人的手误不该让全家\|P1-④.5\|某档案 JSON 被写坏时用\|全新安装（0 档案）首页是一张空列表死路" docs/superpowers/specs/2026-10-05-launchbay-design.md`
Expected: 每一处都要命中，特别是 `某档案 JSON 被写坏时用`（Step 10 要改的就是那一句）与 `明写不防（有意为之`（Step 6 落的是那一串 bullet，不是上面的表）。**实测记录**：权威源没有 §15 运维节（§15 是开放项），设计 §6 那句"图标缓存坏了就删掉 `data/icons/` 再重启"在权威源与全仓库都不存在；§8.8 指的是 §8 界面清单里编号 8 那条「管理页」（列表项，不是标题）；`^### P1-④.5` 这种写法命不中，那行同理是列表项。命不中的锚点一律先核对再动手，别凭记忆改。

- [ ] **Step 2: §4.5 串行化那段末尾加一句**

在 `那条免凭证的引导建档（\`POST /setup\`，§8.0）也吃同一条纪律` 那一段的末尾追加：

> 同一条纪律还管着坏档案的出路 `POST /api/v1/profiles/rebuild`（§6.1、§12）：**"这份档案读得动吗"与"要不要覆盖它"必须发生在同一次按 id 的临界区里**。handler 若先自己 `LoadProfile` 看一眼再决定写不写，两次读之间档案被人修好，它就会把一份已经读得动的 `profile.json` 盖掉——那是"改档"的第二份判据，也是这条端点唯一真正的危险。

- [ ] **Step 3: §6.1 免凭证那句加一条"没变"的说明**

在 `**免凭证的四条**` 那一段末尾追加：

> （P1-④.6 新增的 `POST /api/v1/profiles/rebuild` **要**凭证，免凭证的还是这四条——写这条端点的人不许顺手把"大多要凭证"改成别的口径来迁就它。）

- [ ] **Step 4: §6.1 端点表加一行**

紧接 `| \`/api/v1/profiles/pin\` | POST |` 那一行之后插入：

```markdown
| `/api/v1/profiles/rebuild` | POST | 只重写 `profile.json` 的坏档出路。仅管理员（`*` 令牌或管理员会话；档案作用域令牌打它是 403，这是设计不是故障）。体 `{profile, title, admin, pin}`，四格都必须出现。判据在 store 锁内：**读得动 → 409**（改 PIN 请走 `profiles/pin`）；目录不存在 → 404（**这条端点不建目录**，建档案是 `POST /api/v1/profiles` 的事）；只有"内容错"与"文件不在"才允许覆盖，其它读失败原样上报不许盖。写完在同一把锁内回读一次，读不动答 500——"200"意味着进门页那张卡真的回来了。`bookmarks.json`/`snapshots/`/历史**已有内容**不碰一个字节；历史追一行 `{rev:0, ops:[{op:"set", path:"profile/rebuild"}]}`；`X-LB-By` 头可选，缺省 `web`。`theme`/`layout`/`createdAt` 按默认重建（读不出来就没有"保留"这个选项，明写在 §12） |

**凭证档那句不能省**（复评抓到第一版只说了"改 PIN 请走 profiles/pin"）：邻居那两行都写着"仅管理员"，而 Task 4 的 AGENTS 行与 openapi 的 `x-credential` 要"与这里一致"——这里没写档位，那条一致就没有锚。**"已有内容"四个字也不能省**：设计 §7 第 1 条原文写的是 `history.jsonl` 逐字节不变，而同一份设计的 §3 要它追一行——那是自相矛盾，权威源按"已写下的字节不动、新行照追"落，设计那边同步改掉。
```

- [ ] **Step 5: §8.8 写动作分级补一句**

在 §8.8 那条讲管理页的条目末尾追加：

> 写动作的分级跟着 ④.6 改一次口：**整份重写 `profile.json` 这件事的机器通路归 ④.6**（端点 `POST /api/v1/profiles/rebuild`，CLI 侧 `launchbay profile rebuild`——它是服务端件，不是 ⑤ 的 JS 件），页面入口仍随 ⑤：管理页那张表单要二次确认，而"确认"的形状归 ⑤ 那批 JS。读到这里别把它理解成"rebuild 也在 ⑤"：上面那句"设 PIN 与删档要二次确认……随 ⑤ 落地"管的仍是这两条**页面**动作，它们各自的机器通路早就在 §6.1 里。

别写成"机器通路**已经可用**"（第一版就是这么写的，复评抓到）：Task 1 落下去的那一刻路由还不存在，权威源用现在时讲一条还没登记的端点就是骗人；这一段对未落地动作的既有写法是标相位（"归 ④.6""随 ⑤ 落地"），跟着它走。

- [ ] **Step 6: §11 的"明写不防"清单加一条残余**

**这一条落的是 §11 下面那个 bullet 清单（"明写不防（有意为之，实现时不要顺手加回来）"），不是上面那张"防这些"表**——复评抓到第一版把它塞进了表里：那张表的中列是"为什么必须防"，而这条讲的是明写不防，塞进去就是自相矛盾的一行。在 `- 不做内容安全过滤` 那条之前插入：

```markdown
- **不防"管理员凭证能整份重写任何档案的 `profile.json`"（④.6）。** 后果明写：拿到 `*` 令牌或管理员会话就能把某人降为非管理员、把 `theme`/`layout` 重置成默认。对策只有两条不另加件的：读得动就 409（所以这条通路拿不来当改档用）、`admin` 必填（省略不等于 false，静默降级唯一管理员是要紧事）。限流、"改管理员要二次凭证"、"是不是最后一个管理员"的检查都不要——那需要 §4.5 之外的第二份判据，而能建档能删档的凭证本来就能做更少的动作就毁掉这个档案。多出来的防护在这一档局域网定位里也算失败。
```

- [ ] **Step 7: §12 隔离那段补"怎么出去"**

在 `一个人的手误不该让全家开不了导航页。` 之后插入：

> 隔离不把它藏起来，也不把人困住：进门页点名坏档并说一句出路（§8.0），而**唯一被支持的修法是 `POST /api/v1/profiles/rebuild`**（只重写 `profile.json`，书签与快照不动）。手工路线一律算错——尤其"删掉坏文件再建档"：`CreateProfile` 无条件写一份 `bookmarks.json`，会把盘上那份**还能读的**书签覆盖掉。

- [ ] **Step 8: §13 验收清单加一片**

在 P1-④.5 那一块之后加：

```markdown
P1-④.6 坏档案的机器出路（`profile rebuild`）

- 坏档 rebuild 之后 `profile.json` 读得动，而 `bookmarks.json`、`snapshots/` 下每个文件、`history.jsonl` 的**已有内容**逐字节不变（历史只多一行）。判据是读磁盘的 sha256，不是看返回值。
- 读得动的档案打它 → 409，且**没有发生任何写**（回读一份"没被动过"的档案要用 sha，不是"看起来一样"）。
- 半件事目录（有目录、没有 `profile.json`）→ 允许重建，且目录里只多出一个 `profile.json`。
- 目录根本不存在 → 404，且**不创建任何目录**：这条端点不是第二条建档通路，建档案仍然只有 `POST /api/v1/profiles`。
- `LoadProfile` 报的不是"内容错"而是权限/磁盘那一类读失败 → 原样上报、不覆盖。读不动不等于内容错，那时盘上那份还可能救。
- 写完回读仍读不动 → 500 且坏档登记**留着**；成功 → `/status` 的 `brokenProfiles` 里那条消失。这一条钉的是"响应说成功 = 进门页那张卡真的回来了"，不是"写盘返回了 nil"。
- 并发：同一档案两路 rebuild、以及 rebuild 与设 PIN（同一把按 id 的锁的另一个用户）交错 → 不产生半写文件，后写的赢且文件永远是能解的 JSON。枪：把 `WithProfileLock` 去掉 → 这条与 `-race` 一起红。本机 `CGO_ENABLED=0` 跑不了 `-race`，那一条在 Linux 容器里补跑。
- CLI：`launchbay profile rebuild` 的参数→请求体接线，含一个不能含糊的形状——`--admin` 旗标省略时**显式发 `admin:false`**，不是"省略那一格"。失败退出码非 0 且把服务端 `detail` 原样说出来；成功打印新档案的 `id`/`title`/`admin`。
- 端到端（人跑一遍，不是断言）：真起实例 → 写坏唯一档案 → 进门页那句点名 → `launchbay profile rebuild` → 那张卡回来 → 书签内容与坏掉之前逐字节一致。
- 契约面：`routes()` ↔ `openapi.json` ↔ `AGENTS.md` 三处过既有闸；PIN 的格式与哈希抽成建档族共用的一份之后，"在 rebuild 的壳里自己哈希一遍"必须让 `createrule_test.go` 那道 AST 闸红。
```

**这一片的形状跟着邻块（P1-④.5 那一块用的是"标题行 + `- ` bullet"，不是编号列表）**，条数由设计 §7 说了算而不是反过来：**§7 的 1/2/3/3b/4/5/6/7-11/12 都要有落点**——第一版漏了并发（§7.6）与 CLI 接线（§7.11）两条，复评抓到；§7.1 那句"`history.jsonl` 逐字节不变"与设计 §3"追一行"自相矛盾，这里按"已有内容不变、新行照追"落，设计文档同步改掉。

- [ ] **Step 9: §14 阶段表加 ④.6 这一片**

在 `- **④.5 首次运行与默认单人**` 那条之后插入一条：

```markdown
- **④.6 坏档案的机器出路**（`2026-10-09-launchbay-profile-repair-design.md`）：`POST /api/v1/profiles/rebuild` + `launchbay profile rebuild`，纯服务端、零 JS、零新页面形状。页面入口（管理页那张带二次确认的重建表单）随 ⑤ 一起做，判据不变、只换外壳。
```

- [ ] **Step 10: §10 部署那句"坏了怎么办"分诊成两种坏**

**（计划自纠：设计 §6 的最后一行写的是"§15（运维）"，并引了一句"图标缓存坏了就删掉 `data/icons/` 再重启"——两处都对不上权威源。Step 1 实测：权威源没有 §15 运维节（§15 是开放项），全仓库也没有那句图标缓存的出路；真正的运维句子在 §10 部署的"升级/回滚/备份"那句里。裁定见台账。）**

把 §10 那句
`升级 = 换镜像 + 重启；数据在 \`./data\` 卷里不动。回滚 = 换回旧镜像；某档案 JSON 被写坏时用 \`revert\` 或直接从 \`snapshots/\` 取。`
里"某档案 JSON 被写坏时…"这一段换成按坏的是哪个文件分诊（`revert` 与快照**只救书签**，这正是本片要补的洞）：

```markdown
某档案 JSON 被写坏时**分两种坏**：`bookmarks.json` 坏了用 `revert` 或直接从 `snapshots/` 取（快照只存书签）；**`profile.json` 坏了这两条都救不了**，唯一的出路是 `POST /api/v1/profiles/rebuild`（CLI 是同一条通路的壳：`launchbay profile rebuild --id <slug> --title <名字> --pin <4-6位> [--admin]`，只经本机 REST，不直改磁盘），它只重写那一个文件。**没有**"删掉坏文件再建档"这种走法：`CreateProfile` 会无条件写一份 `bookmarks.json`，把盘上那份还能读的书签一起盖掉（§12）。
```

- [ ] **Step 11: 自证（按设计那一行一行证，不是"我改完了"）**

Run: `python .superpowers/tmp/p46/proof1.py && cat .superpowers/tmp/p46/proof1.out`
Expected: `problems=0`。脚本把设计 §6 表里每一行对到权威源里**恰好出现一次**的那句原文（11 条正向），再加三条反向：§11 那行不许留在"防这些"表里（它讲的是**明写不防**，放错表就成了自相矛盾）、不许留 `x-status: planned`、§10 那句"某档案 JSON 被写坏时用 `revert` 或直接从 `snapshots/` 取。"必须被换掉而不是并存。

> 原先这条写的是 `grep -c "profiles/rebuild" >= 4`——复评抓到它**证明不了九行**：命中的六行里 §10/§11/§13 一条都不含那个串（它们写的是 `profile rebuild` 与 `rebuild`），而报告把第六处记成了 §10（其实是 §8.8）。一条只会通过的 grep 不是证人。

- [ ] **Step 12: 提交**

```bash
git add docs/superpowers/specs/2026-10-05-launchbay-design.md
git commit -m "docs(specs): 权威源给坏档案一条出路——只重写 profile.json，且判据在锁内问一次"
```

---

### Task 2: store 层的 `RebuildProfile`

**Files:**
- Create: `internal/store/rebuild.go`
- Create: `internal/store/rebuild_test.go`
- Modify: `internal/store/paths.go`（`ErrNotFound`/`ErrExists` 那一组里加一个哨兵）

**Interfaces:**
- Consumes: 既有 `(*Store).loadProfile`（读 + 坏档登记/清除的唯一实现）、`(*Store).SaveProfile`、`(*Store).WithProfileLock`、`(*Store).ProfileDir`、`(*Store).ProfilePath`、`model.Profile`、`model.ValidSlug`。
- Produces:
  - `var ErrReadable = errors.New("档案读得动，不需要重建")`（`internal/store`，与 `ErrExists` 并列）
  - `type RebuildInput struct { ID, Title, PinHash string; Admin bool }`
  - `func (s *Store) RebuildProfile(in RebuildInput) (*model.Profile, error)`
  Task 4 的 handler 只认这三个名字。

- [ ] **Step 1: 加哨兵**

在 `internal/store/paths.go` 的 `ErrCorrupt` 那行之后加：

```go
	// ErrReadable 不是"失败了"，是"这件事不该做"：档案读得动，重建会盖掉一份
	// 只是 PIN 忘了的档案。调用方（httpapi）把它答成 409 而不是 5xx。
	ErrReadable = errors.New("档案读得动，不需要重建")
```

- [ ] **Step 2: 写失败测试（六条，逐条给判据）**

新建 `internal/store/rebuild_test.go`：

```go
package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
)

// seedBroken 建一份档案、写两条书签，然后把 profile.json 写成读不动的字节。
// 返回它自己算出的书签文件指纹——"重建不许动书签"这件事只比对字节，不比对印象。
func seedBroken(t *testing.T, id string) (*Store, []byte) {
	t.Helper()
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: id, Title: "旧名", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	b := model.Bookmarks{Rev: 3, Title: id, Groups: []model.Group{{
		ID: "g", Title: "组", Layout: model.LayoutList,
		Items: []model.Item{{ID: "i", Name: "条目", URL: "http://10.0.0.1/", Icon: "tabler:home"}},
	}}}
	if err := s.SaveBookmarks(id, &b); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.BookmarksPath(id))
	if err != nil {
		t.Fatal(err)
	}
	BreakProfileJSON(t, s, id)
	return s, raw
}

// BreakProfileJSON 把 profile.json 写成半截 JSON——夹具需要"坏"，只能手工造。
func BreakProfileJSON(t *testing.T, s *Store, id string) {
	t.Helper()
	if err := s.WriteFileAtomic(s.ProfilePath(id), []byte("{\n  \"id\": \""+id+"\",\n  \"title\":\n")); err != nil {
		t.Fatal(err)
	}
}

func TestRebuild_坏档重建后书签逐字节不变(t *testing.T) {
	s, bmBefore := seedBroken(t, "mom")
	h, err := authHash(t)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.RebuildProfile(RebuildInput{ID: "mom", Title: "妈妈", PinHash: h, Admin: true})
	if err != nil {
		t.Fatalf("坏档 rebuild 该成功: %v", err)
	}
	if p.Title != "妈妈" || !p.Admin || p.PIN != h {
		t.Errorf("回显与递进来的不一致: %+v", p)
	}
	bmAfter, err := os.ReadFile(s.BookmarksPath("mom"))
	if err != nil {
		t.Fatal(err)
	}
	if string(bmBefore) != string(bmAfter) {
		t.Errorf("书签被动了:\n改前 %q\n改后 %q", bmBefore, bmAfter)
	}
	got, err := s.LoadProfile("mom")
	if err != nil {
		t.Fatalf("重建后读不动: %v", err)
	}
	// theme/layout/createdAt 按默认重建是能力边界，但必须**是**默认值而不是空串：
	// 空串会让 §4.3 的校验与进门页的 themeAttr 各说各话。
	if got.Theme != "auto" || got.Layout != "sidebar" {
		t.Errorf("默认值没落地: theme=%q layout=%q", got.Theme, got.Layout)
	}
	if _, err := time.Parse(time.RFC3339, got.CreatedAt); err != nil {
		t.Errorf("createdAt 该是 RFC3339（不是空串、不是别的形状）: %q", got.CreatedAt)
	}
	if len(s.Broken()) != 0 {
		t.Errorf("重建后坏档登记还在: %v——回读那一步没生效或 clearBroken 挂错了层", s.Broken())
	}
}

func TestRebuild_读得动的档案不许被盖(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.ProfilePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := authHash(t)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RebuildProfile(RebuildInput{ID: "dad", Title: "改名", PinHash: h})
	if !errors.Is(err, ErrReadable) {
		t.Fatalf("读得动该 ErrReadable，得到 %v", err)
	}
	after, _ := os.ReadFile(s.ProfilePath("dad"))
	if string(before) != string(after) {
		t.Errorf("ErrReadable 那一支还是把文件写了：%q -> %q", before, after)
	}
}

func TestRebuild_半件事目录可以重建且不碰书签(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "kid", Title: "小满", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.ProfilePath("kid")); err != nil {
		t.Fatal(err)
	}
	bmBefore, err := os.ReadFile(s.BookmarksPath("kid"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := authHash(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RebuildProfile(RebuildInput{ID: "kid", Title: "小满", PinHash: h}); err != nil {
		t.Fatalf("目录在、profile.json 不在该允许: %v", err)
	}
	bmAfter, _ := os.ReadFile(s.BookmarksPath("kid"))
	if string(bmBefore) != string(bmAfter) {
		t.Error("重建顺手动了书签")
	}
}

func TestRebuild_目录不存在时不建目录(t *testing.T) {
	s := New(t.TempDir())
	h, err := authHash(t)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RebuildProfile(RebuildInput{ID: "ghost", Title: "幽灵", PinHash: h})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("没这份档案该 ErrNotFound，得到 %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.root, "profiles", "ghost")); !os.IsNotExist(err) {
		t.Errorf("rebuild 造出了目录 %q——它是第二条建档通路了", filepath.Join(s.root, "profiles", "ghost"))
	}
}

func TestRebuild_非内容错的读失败不许覆盖(t *testing.T) {
	s, _ := seedBroken(t, "mom")
	// 把 profile.json 换成一个"读不动但不是坏 JSON"的形状：目录占位。
	// os.ReadFile 对目录返回的是 I/O 错，不是 ErrCorrupt——这条通路必须与坏档分开。
	if err := os.Remove(s.ProfilePath("mom")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.ProfilePath("mom"), 0o755); err != nil {
		t.Fatal(err)
	}
	h, err := authHash(t)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RebuildProfile(RebuildInput{ID: "mom", Title: "妈妈", PinHash: h})
	if err == nil || errors.Is(err, ErrReadable) {
		t.Fatalf("I/O 错该原样上报，得到 %v", err)
	}
	if errors.Is(err, ErrCorrupt) {
		t.Errorf("把 I/O 错说成内容错：%v", err)
	}
	if fi, statErr := os.Stat(s.ProfilePath("mom")); statErr != nil || !fi.IsDir() {
		t.Error("I/O 错那一支把目录/文件改了——覆盖就是二次事故")
	}
}

func TestRebuild_校验排在落盘之前(t *testing.T) {
	s, _ := seedBroken(t, "mom")
	h, err := authHash(t)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.ProfilePath("mom"))
	for _, tc := range []struct{ name, id, title string }{
		{"id 不是 slug", "Bad ID", "x"},
		{"title 全空", "mom", "   "},
		{"缺 PIN 哈希", "mom", "妈妈"},
	} {
		in := RebuildInput{ID: tc.id, Title: tc.title, PinHash: h}
		if tc.name == "缺 PIN 哈希" {
			in.PinHash = ""
		}
		_, err := s.RebuildProfile(in)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s 该 ErrInvalid，得到 %v", tc.name, err)
		}
	}
	after, _ := os.ReadFile(s.ProfilePath("mom"))
	if string(before) != string(after) {
		t.Error("校验没过还是把文件写了")
	}
}

func TestRebuild_并发不产生半写文件(t *testing.T) {
	s, _ := seedBroken(t, "mom")
	h, err := authHash(t)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.RebuildProfile(RebuildInput{ID: "mom", Title: "妈妈", PinHash: h, Admin: i%2 == 0})
		}(i)
	}
	wg.Wait()
	// 四路里可以有 ErrReadable（先到的赢了，后到的看见读得动），但不许有 I/O 错、
	// 不许有半写文件：文件必须始终是一份能解的 JSON。
	for i, e := range errs {
		if e != nil && !errors.Is(e, ErrReadable) {
			t.Errorf("第 %d 路得到 %v", i, e)
		}
	}
	if _, err := s.LoadProfile("mom"); err != nil {
		t.Fatalf("并发之后档案仍读不动: %v", err)
	}
	raw, _ := os.ReadFile(s.ProfilePath("mom"))
	if !strings.HasSuffix(string(raw), "}\n") {
		t.Errorf("profile.json 末尾形状不对（§4.3 要恰好一个换行）: %q", raw[len(raw)-8:])
	}
}
```

- [ ] **Step 3: 补 PIN 哈希的测试助手**

同包测试里不许直接引 `internal/auth`？可以——`internal/store` 的测试引 `internal/auth` 不成环（`auth` 不依赖 `store`）。在 `rebuild_test.go` 里加：

```go
// authHash 造一个真哈希：夹具不许自己拼 "sha256$..." 假装哈希过——那会让
// "rebuild 写进去的哈希能用"这件事永远没被证明（本仓库为这类假夹具改过口径）。
func authHash(t *testing.T) (string, error) {
	t.Helper()
	return auth.HashPIN("4321")
}
```

并在 import 块加 `"github.com/rockbenben/launchbay/internal/auth"`。

- [ ] **Step 4: 跑测试，确认它们因为"方法不存在"而失败**

Run: `go test ./internal/store -run 'TestRebuild' -count=1`
Expected: 编译失败，报 `s.RebuildProfile undefined` 与 `ErrReadable undefined`（Step 1 之前会先报 ErrReadable；补完 Step 1 后报 RebuildProfile）。编译失败**不是**证人红——记下它，别当成"闸不咬"。

- [ ] **Step 5: 写实现**

新建 `internal/store/rebuild.go`：

```go
package store

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rockbenben/launchbay/internal/model"
)

// RebuildInput 是"只重写 profile.json"所需的最小信息。刻意没有 Theme/Layout：
// 那两个读不出来就按默认重建——不假装能从半截 JSON 里抢救出来（设计 §8）。
type RebuildInput struct {
	ID      string
	Title   string
	PinHash string
	Admin   bool
}

// RebuildProfile 修一份读不动的 profile.json：书签、快照、历史一个字不碰。
//
// 三条判据都在这一次按 id 的临界区里，调用方一律不许自己先看一眼再决定：
//   - "读得动吗" = 这一次 loadProfile 的结果。读得动返回 ErrReadable——这条端点
//     不是改档用的，拿它改 PIN 该走 profiles/pin（与 CreateFirstProfile 把"零档案"
//     判据搬进锁内是同一条纪律，权威源 §4.5）。
//   - "有没有这份档案" = 目录在不在。不在就 ErrNotFound，**不 MkdirAll**：从这条
//     端点建目录等于开出第二条建档通路，而且建出来是个没有 bookmarks.json 的半件事档案。
//   - 只有"内容错"(ErrCorrupt) 与"文件不在"(ErrNotFound 且目录在) 允许覆盖。其它
//     I/O 错（权限、盘坏、路径是个目录）原样返回：那时读不动不等于内容错，覆盖它
//     就是二次事故。
//
// 与 CreateProfile 最关键的区别在失败处理：那里回滚是 RemoveAll 整个目录，而这里
// 目录里可能有真书签——所以**什么都不回滚**，写失败就报错，坏档登记留着。
//
// 写完在同一把锁内回读一次：坏档登记的清除只由"读成功"那条既有路径做
// （loadProfile 里的 clearBroken），这里不另开一份。回读仍读不动就返回错误，
// 让调用方拿 500——不许给一个"成功但那张卡没回来"的 200。
func (s *Store) RebuildProfile(in RebuildInput) (*model.Profile, error) {
	if !model.ValidSlug(in.ID) {
		return nil, fmt.Errorf("%w: 档案 id %q 必须是 ASCII slug", ErrInvalid, in.ID)
	}
	if in.PinHash == "" {
		return nil, fmt.Errorf("%w: 档案 %q 缺 PIN 哈希", ErrInvalid, in.ID)
	}
	p := &model.Profile{
		ID: in.ID, Title: in.Title, Admin: in.Admin, PIN: in.PinHash,
		CreatedAt: time.Now().Format(time.RFC3339), Theme: "auto", Layout: "sidebar",
	}
	var out *model.Profile
	err := s.WithProfileLock(in.ID, func() error {
		if _, err := os.Stat(s.ProfileDir(in.ID)); err != nil {
			return fmt.Errorf("%w: 档案 %q", ErrNotFound, in.ID)
		}
		// 校验排在任何落盘之前（与 CreateProfile 同口径）。
		if err := p.Validate(); err != nil {
			return errors.Join(ErrInvalid, err)
		}
		if _, _, err := s.loadProfile(in.ID); err == nil {
			return ErrReadable
		} else if !errors.Is(err, ErrCorrupt) && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := s.SaveProfile(p); err != nil {
			return err
		}
		if _, err := s.LoadProfile(in.ID); err != nil {
			return fmt.Errorf("重建后仍读不动 %q: %w", in.ID, err)
		}
		out = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
```

注意两处**必须照做**的细节：`p.Validate()` 排在读判据之前会改变"坏档 + 空 title"的答法——那是 ErrInvalid 优先，测试 `TestRebuild_校验排在落盘之前` 钉的就是它；而 `s.loadProfile` 用于判据（不额外清登记，登记清除只在**写完后的那次 `LoadProfile`** 上有意义）。

- [ ] **Step 6: 跑测试到全绿**

Run: `go test ./internal/store -run 'TestRebuild' -v -count=1`
Expected: 七条全 PASS（六条测试 + 并发那条）。若 `坏档重建后书签逐字节不变` 红在"坏档登记还在"，说明回读用的是私有 `loadProfile` 而不是导出的 `LoadProfile`——清登记挂在这一层，改回来。

- [ ] **Step 7: 四把枪，逐支单独打**

写 `.superpowers/tmp/p46/sab13.py`（规矩：施加前断 `mutated != original`、区分 `build_err`、跑完 sha 证还原、期望红点名到具体测试）：

| 枪 | 破坏 | 必须红在 |
|---|---|---|
| S1 | 把判据那段 `else if` 改成"任何错误都继续"（I/O 错也覆盖） | `TestRebuild_非内容错的读失败不许覆盖` |
| S2 | 把 `os.Stat(ProfileDir)` 那一步删掉、失败时 `MkdirAll` 补上 | `TestRebuild_目录不存在时不建目录` |
| S3 | 删掉写完的回读那一步（`LoadProfile` 那次） | `TestRebuild_坏档重建后书签逐字节不变`（坏档登记没清） |
| S4 | 把 `SaveProfile` 换成"先 `RemoveAll` 再建"（模拟复用 CreateProfile 的回滚） | `TestRebuild_坏档重建后书签逐字节不变`（书签没了） |
| S5 | 把 `ErrReadable` 那一支删掉（任何时候都允许覆盖） | `TestRebuild_读得动的档案不许被盖` |

Run: `python .superpowers/tmp/p46/sab13.py` → 读 `.superpowers/tmp/p46/sab13.out`，Expected: 五把全 `AS-PREDICTED`、每把 `restored-ok`。

- [ ] **Step 8: 门禁 + 提交**

```bash
go build ./... && go vet ./... && gofmt -l internal cmd && go test ./... -count=1
git add internal/store/paths.go internal/store/rebuild.go internal/store/rebuild_test.go
git commit -m "feat(store): 只重写 profile.json 的坏档出路——判据、目录、I/O 错三件事都在锁内分诊"
```

---

### Task 3: 把 PIN 的"格式 + 哈希"抽成建档族共用的一份（纯重构）

**Files:**
- Modify: `internal/httpapi/handlers.go`（`createProfile` 里那两小段）

**Interfaces:**
- Consumes: `auth.ValidPinFormat`、`auth.HashPIN`、`fieldError`。
- Produces: `func hashFormPIN(pin string) (string, []fieldError)`（package `httpapi`，私有，**整包只许一处定义**）。Task 4 的 `handleRebuildProfile` 与既有 `createProfile` 都调它；除它以外直接调那两个 auth 函数的只剩 `pinapi.go` 那两个设 PIN 的 handler（合法的第二批用户）。

这一步**只搬不改行为**：全量测试的行为证人一条都不许红。但 `Test建档判据只有一处` **必须在同一个提交里改口径**——它现在的句子是"owner（`createProfile`）必须持有 `auth.HashPIN`"，Step 2 一搬完这句话就断了，把改闸留到 Task 4 等于逼 Task 3 带红提交（复扫抓到的计划缺陷，裁定见台账）。Task 4 只往名单里加第二条壳。

- [ ] **Step 1: 写实现（先看清要搬的是哪两小段）**

`createProfile` 现在是这个形状（`internal/httpapi/handlers.go`，锚点按字面量给）：

```go
	if !auth.ValidPinFormat(in.PIN) {
		errs = append(errs, fieldError{Field: "pin", Reason: "要 4-6 位数字"})
	}
	if len(errs) > 0 {
		return createResult{FieldErrs: errs}
	}
	hash, err := auth.HashPIN(in.PIN)
	if err != nil {
		return createResult{FieldErrs: []fieldError{{Field: "pin", Reason: err.Error()}}}
	}
```

- [ ] **Step 2: 加共用函数，并把那两小段换成一次调用**

在 `createProfile` 之前加：

```go
// hashFormPIN 是建档族（createProfile / handleRebuildProfile）共用的 PIN 判据：
// 格式 + 哈希一次做完。"判据只有一份"由 Test建档判据只有一处 钉着（Task 4 把
// handleRebuildProfile 加进那条名单）——第二条建档壳落地那天它若绕过这里自己哈希，闸就红。
//
// 两条失败各回各的文本：格式错说"要 4-6 位数字"（给人看的那句），auth 内部的
// 哈希错说它自己的原因（超长/空串这类不该被粉饰成格式问题）。合并成一条会把
// 人话换成一句看不懂的。
func hashFormPIN(pin string) (string, []fieldError) {
	if !auth.ValidPinFormat(pin) {
		return "", []fieldError{{Field: "pin", Reason: "要 4-6 位数字"}}
	}
	hash, err := auth.HashPIN(pin)
	if err != nil {
		return "", []fieldError{{Field: "pin", Reason: err.Error()}}
	}
	return hash, nil
}
```

`createProfile` 里那两小段换成：

```go
	hash, pinErrs := hashFormPIN(in.PIN)
	errs = append(errs, pinErrs...)
	if len(errs) > 0 {
		return createResult{FieldErrs: errs}
	}
```

后面 `var p *model.Profile` 那一段的 `p, err = …` 需要 `err` 先声明（原来那行 `hash, err := auth.HashPIN(...)` 顺带声明了它）：

```go
	var p *model.Profile
	var err error
	if in.FirstOnly {
		p, err = s.deps.Store.CreateFirstProfile(pi)
	} else {
		p, err = s.deps.Store.CreateProfile(pi)
	}
```

- [ ] **Step 3: 同轮改那道 AST 闸（`internal/httpapi/createrule_test.go#Test建档判据只有一处`）**

Step 2 之后 `createProfile` 不再持有那两个符号，而闸现在的第一句话就是"owner 必须持有 `auth.HashPIN`"。整段替换那个测试函数（只换它，`packageFuncs`/`funcsInDir`/`goFunc` 都不动）：

```go
func Test建档判据只有一处(t *testing.T) {
	const owner = "createProfile"
	// 判据符号分两档，两档的"唯一持有者"不是同一个函数：
	//   - 落点三件套（建档与它的 id 派生）整包只许 `createProfile` 一处持有；
	//   - PIN 的两件（格式与哈希）唯一持有者是 `hashFormPIN`，另有**点名批准的第二批
	//     用户**——`pinapi.go` 那两个设 PIN 的 handler 本来就要验同一格式并哈希。把它们
	//     也按整包禁掉，禁的是别人家的事，那条"闸"会天天红到有人把它删掉。
	// 第二档按"持有者名单"钉，不按"谁调了建档"钉：判据抽进 hashFormPIN 之后，
	// "owner 必须持有"这句话的主语换了人，而只禁壳里的旧口径管不住新长出来的第三条
	// 路。名单口径把这件事一次说完：**合法持有者恰好这三个函数**。
	// 判据符号**不带括号**：方法值写法（`f := auth.HashPIN`）与调用写法抄的是同一条判据。
	// 代价是判得偏严：出现这两个符号，哪怕藏在字符串字面量里，也会红——宁可能红
	// 一点，不放走真抄了判据的那一种。
	writeSymbols := []string{".CreateProfile", ".CreateFirstProfile", ".NextProfileID"}
	pinSymbols := []string{"auth.ValidPinFormat", "auth.HashPIN"}
	const pinOwner = "hashFormPIN"
	pinHolders := []string{pinOwner, "handleSelfPin", "handleAdminPin"}
	// 建档族：每条壳都必须回来调 hashFormPIN。Task 4 把 handleRebuildProfile 加进来。
	pinFamily := []string{"createProfile"}

	found := packageFuncs(t)
	byName := map[string][]string{}
	for _, g := range found {
		byName[g.name] = append(byName[g.name], g.body)
	}
	// soleBody 只认"整包唯一"的那个函数体：0 处 = 这条闸正在认一个不存在的东西，
	// >1 处 = 判据函数自己长出了第二份。两种都必须当场红——放过去，下面的
	// strings.Count 就是在对空字符串成立，什么都钉不住。
	soleBody := func(name string) string {
		switch bodies := byName[name]; len(bodies) {
		case 0:
			t.Fatalf("%s 在整包里没有定义：这条闸正在空转（认了一个不存在的函数）", name)
		case 1:
			return bodies[0]
		default:
			t.Fatalf("%s 在整包里有 %d 处定义，要恰好 1 处", name, len(bodies))
		}
		return ""
	}

	ownerBody := soleBody(owner)
	// 正对照：除了判据函数自己，至少还要有一个调用建档的壳。一条都没匹配上说明
	// caller 的形状与源码对不上，那时下面每个断言都是对空集合成立的假绿。
	// 接收者不参与匹配（复扫 Minor：认 `s.createProfile` 的版本会放走换名的壳）。
	const caller = ".createProfile"
	shells := []string{}
	for _, g := range found {
		if g.name == owner || strings.Contains(g.body, caller) {
			shells = append(shells, g.display())
		}
	}
	sort.Strings(shells)
	if len(shells) < 2 {
		t.Fatalf("只认出 %v：没识别出任何调用 %q 的壳——判据函数变成没人调的死码，或匹配形状错了",
			shells, caller)
	}

	// 第一档：建档落点三件套。owner 必须持有，其余整包必须为零。只盯调用者的版本
	// 防不住它要防的那件事——复制粘贴建档流程的人会把 `createProfile(...)` 那行一起
	// 省掉、直接调 store，新函数根本进不了壳的名单。实测（sab9）过。
	for _, sym := range writeSymbols {
		if n := strings.Count(ownerBody, sym); n == 0 {
			t.Errorf("%s 里一次都没有 %s：判据不在唯一落点上", owner, sym)
		}
		for _, g := range found {
			if g.name == owner {
				continue
			}
			if n := strings.Count(g.body, sym); n != 0 {
				t.Errorf("%s 里出现了 %d 次 %s——建档落点被抄出 %s 了（整包只许一处：壳、绕过者、第三条路都算）",
					g.display(), n, sym, owner)
			}
		}
	}

	// 第二档：PIN 的格式与哈希。两个方向都要钉，才对得起"恰好这三个"这句话：
	//   - 名单里每一个都必须**真的持有**（正对照）。少了这一条，名单会腐烂成指向
	//     不存在之物的空承诺，而"名单外必须为零"照样绿——那是这道闸最体面的死法。
	//   - 名单之外整包必须为零：这一条同时管住壳与新长出来的第三条路。
	for _, holder := range pinHolders {
		body := soleBody(holder)
		for _, sym := range pinSymbols {
			if n := strings.Count(body, sym); n == 0 {
				t.Errorf("%s 里没有 %s：持有者名单与源码不符（它要么不再持有判据=名单该删它，" +
					"要么这条闸正在认一个空壳）", holder, sym)
			}
		}
	}
	for _, g := range found {
		if slices.Contains(pinHolders, g.name) {
			continue
		}
		for _, sym := range pinSymbols {
			if n := strings.Count(g.body, sym); n != 0 {
				t.Errorf("%s 里出现了 %d 次 %s——PIN 判据被抄出合法持有者了（整包只许 %v）",
					g.display(), n, sym, pinHolders)
			}
		}
	}

	// 第三档（与第二档不重复）：建档族每条壳都必须调 hashFormPIN。第二档管"判据没被
	// 抄出去"，这一档管"新壳有没有老老实实回来调用那一份"——把 PIN 校验整段删掉、
	// 指望 store 兜底的那种壳，第二档抓不到（它体内一个符号都没有），只有这一条抓得到。
	for _, name := range pinFamily {
		if n := strings.Count(soleBody(name), pinOwner+"("); n == 0 {
			t.Errorf("%s 里一次都没调 %s：PIN 判据在它里面被自己写了一遍，或被整段删掉了",
				name, pinOwner)
		}
	}
}
```

口径的两条改动都要说清"禁的是谁"：`handleSetup` 与 `handleAdminCreate` 这两条表单壳本来就不持有 PIN 判据（它们调 `createProfile`），新口径对它们没有新要求；`pinapi.go` 那两个从"壳名单外的豁免"变成"名单内的合法持有者"，判定的还是同一件事。

- [ ] **Step 4: 跑全量，确认行为一字没变**

Run: `go test ./... -count=1`
Expected: 全部 `ok`，**包括改过口径的 `Test建档判据只有一处`**。任何一条**行为**证人红都说明搬动改了行为（最常见是 `err` 声明顺序或 `len(errs) > 0` 的位置），回去看 Step 2，**不许改断言迁就**；只有那道 AST 闸红才可能是 Step 3 自己写错了。

- [ ] **Step 5: 三把枪，逐支单独打**

闸换了口径，就得证明它在新口径下仍然会红——三把各打一个方向：

1. **判据被抄**：把 `createProfile` 里的 `hashFormPIN(in.PIN)` 临时换回自己写一遍（`auth.ValidPinFormat` + `auth.HashPIN`）。
   Expected: FAIL，点名 `createProfile 里出现了 N 次 auth.HashPIN——PIN 判据被抄出合法持有者了`。
2. **名单不是装饰**：从 `pinHolders` 删掉 `"handleSelfPin"`。
   Expected: FAIL，点名 `handleSelfPin 里出现了 N 次 auth.HashPIN`。这一把证明"名单"这个形状真的在参与判定，不是一句注释。
3. **正对照会咬**：把 `handleAdminPin` 里的 `auth.HashPIN(in.New)` 临时换成 `hashFormPIN(in.New)`（一次合法的"它不再持有判据"重构）。
   Expected: FAIL，点名 `handleAdminPin 里没有 auth.HashPIN：持有者名单与源码不符`。这一把最值钱：它证明 Step 3 那条正向断言不是摆设。

Run: `go test ./internal/httpapi -run 'Test建档判据只有一处' -count=1 -v`（每支打完立刻还原，还原后必须绿）。

- [ ] **Step 6: 门禁 + 提交**

```bash
go build ./... && go vet ./... && gofmt -l internal cmd && go test ./... -count=1
git add internal/httpapi/handlers.go internal/httpapi/createrule_test.go
git commit -m "refactor(httpapi): PIN 的格式与哈希抽成建档族共用的一份，AST 闸同轮换口径"
```

---
### Task 4: `POST /api/v1/profiles/rebuild` 与契约三处

**Files:**
- Create: `internal/httpapi/rebuild.go`
- Create: `internal/httpapi/rebuild_test.go`
- Modify: `internal/httpapi/routes.go`
- Modify: `internal/httpapi/openapi.json`
- Modify: `AGENTS.md`
- Modify: `internal/httpapi/createrule_test.go`（把 `handleRebuildProfile` 登进建档族名单）

**Interfaces:**
- Consumes: Task 2 的 `store.RebuildProfile` / `store.RebuildInput` / `store.ErrReadable`；Task 3 的 `hashFormPIN`；既有 `decodeBody`、`s.require(r, credAdmin, in.Profile)`、`writeJSON`、`writeFieldErrors`、`writeInternalError`、`errorResponse`、`fieldError`、`byOfRequest(r)`、`historyOps(...)`、`s.deps.Store.AppendHistory`、`store.HistoryEntry`。
- Produces: 路由键 `POST /api/v1/profiles/rebuild`；handler `(s *Server) handleRebuildProfile`；响应 `rebuildResponse{Profile string \`json:"profile"\`; Rebuilt bool \`json:"rebuilt"\`; Title string \`json:"title"\`; Admin bool \`json:"admin"\`}`。Task 5 的 CLI 打的就是这条路径。

- [ ] **Step 1: 把第二条壳登进 AST 闸的名单，看它红在哪**

Task 3 已经把那道闸换成"持有者名单 + 建档族"的口径，这一轮**只动名单那一行**（`internal/httpapi/createrule_test.go#Test建档判据只有一处`）：

```go
	// 建档族：每条壳都必须回来调 hashFormPIN。
	pinFamily := []string{"createProfile", "handleRebuildProfile"}
```

Run: `go test ./internal/httpapi -run 'Test建档判据只有一处' -count=1 -v`
Expected: FAIL，`handleRebuildProfile 在整包里没有定义：这条闸正在空转（认了一个不存在的函数）`。这条红是本任务要闭的对象；它同时是 Task 3 那个 `soleBody` 的 0 处分支第一次被真调用走到——正对照不是纸上的。

- [ ] **Step 2: 写端点的失败测试**

新建 `internal/httpapi/rebuild_test.go`：

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

// brokenProfile 建一份档案、写两条书签、把 profile.json 写坏，返回书签原文。
func brokenProfile(t *testing.T, s *store.Store, id, title string) []byte {
	t.Helper()
	if _, err := s.CreateProfile(store.NewProfileInput{ID: id, Title: title, PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	b := model.Bookmarks{Rev: 2, Title: title, Groups: []model.Group{{
		ID: "g", Title: "组", Layout: model.LayoutList,
		Items: []model.Item{{ID: "i", Name: "条目", URL: "http://10.0.0.7/", Icon: "tabler:home"}},
	}}}
	if err := s.SaveBookmarks(id, &b); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.BookmarksPath(id))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFileAtomic(s.ProfilePath(id), []byte("{\n  \"id\": \"x\",\n")); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRebuild_坏档端到端修好且书签不动(t *testing.T) {
	s, h := setupBatch(t)
	bmBefore := brokenProfile(t, s, "kid", "小满")
	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","admin":false,"pin":"4321"}`, "", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("坏档 rebuild 该 200，得到 %d %.300s", rec.Code, rec.Body)
	}
	var out struct {
		Profile string `json:"profile"`
		Rebuilt bool   `json:"rebuilt"`
		Title   string `json:"title"`
		Admin   bool   `json:"admin"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	// 回显 title/admin 是有意的：调用方要能确认"我说的那两格真按我说的写了"。
	if out.Profile != "kid" || !out.Rebuilt || out.Title != "小满" || out.Admin {
		t.Errorf("响应回显不对: %+v", out)
	}
	bmAfter, err := os.ReadFile(s.BookmarksPath("kid"))
	if err != nil {
		t.Fatal(err)
	}
	if string(bmBefore) != string(bmAfter) {
		t.Errorf("书签被动了：这条端点的全部意义是不碰它\n改前 %q\n改后 %q", bmBefore, bmAfter)
	}
	// 新 PIN 真能进门（写进去的哈希不是假夹具拼的）。
	if rec := postForm(h, "/pin", "profile=kid&pin=4321", ""); rec.Code != http.StatusSeeOther {
		t.Errorf("新 PIN 该换到会话，得到 %d", rec.Code)
	}
	// 坏档从 /status 里消失（不是靠登记表自己过期）。
	if body := strings.TrimSpace(string(getBearer(h, "/api/v1/status", adminTok).Body.String())); strings.Contains(body, "brokenProfiles\":[{") {
		t.Errorf("/status 还在报坏档: %.200s", body)
	}
	// 历史多一行 rev:0 + path profile/rebuild（不新增 op 词汇）。
	hs := getBearer(h, "/api/v1/history?profile=kid", adminTok).Body.String()
	if !strings.Contains(hs, `"path":"profile/rebuild"`) || !strings.Contains(hs, `"rev":0`) {
		t.Errorf("历史行没落下: %.400s", hs)
	}
}

func TestRebuild_读得动答409且一个字节都不写(t *testing.T) {
	s, h := setupBatch(t)
	before, err := os.ReadFile(s.ProfilePath("kid"))
	if err != nil {
		t.Fatal(err)
	}
	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"改名","admin":true,"pin":"4321"}`, "", adminTok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("读得动该 409，得到 %d %.200s", rec.Code, rec.Body)
	}
	// Detail 要指路：409 不带出路，调用方只能猜。
	var e errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || !strings.Contains(e.Detail, "profiles/pin") {
		t.Errorf("409 该指回设 PIN 那条: %.200s", rec.Body)
	}
	after, _ := os.ReadFile(s.ProfilePath("kid"))
	if string(before) != string(after) {
		t.Error("409 那一支还是把文件写了")
	}
}

func TestRebuild_三格必填缺admin不许当false(t *testing.T) {
	s, h := setupBatch(t)
	brokenProfile(t, s, "kid", "小满")
	for _, body := range []string{
		`{"profile":"kid","title":"小满","pin":"4321"}`,              // 缺 admin
		`{"profile":"kid","admin":false,"pin":"4321"}`,               // 缺 title
		`{"profile":"kid","title":"小满","admin":false}`,             // 缺 pin
		`{"profile":"kid","title":"小满","admin":false,"pin":"4321","x":1}`, // 未知键
	} {
		rec := post(h, "/api/v1/profiles/rebuild", body, "", adminTok)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%.80s 该 422，得到 %d", body, rec.Code)
		}
	}
	// 正对照：三格齐了就不是 422（否则上面四条是在认"任何请求都 422"）。
	if rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","admin":false,"pin":"4321"}`, "", adminTok); rec.Code != http.StatusOK {
		t.Errorf("三格齐了该 200，得到 %d %.200s", rec.Code, rec.Body)
	}
}

func TestRebuild_PIN格式错点名pin(t *testing.T) {
	s, h := setupBatch(t)
	brokenProfile(t, s, "kid", "小满")
	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"kid","title":"小满","admin":false,"pin":"abc"}`, "", adminTok)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("该 422，得到 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"field":"pin"`) {
		t.Errorf("该点名 pin: %.200s", rec.Body)
	}
}

func TestRebuild_鉴权与不存在的档案(t *testing.T) {
	s, h := setupBatch(t)
	body := `{"profile":"kid","title":"x","admin":false,"pin":"4321"}`
	// 没凭证 → 401；档案作用域的令牌 → 403（这是设计不是故障，spec §6.4）。
	if rec := post(h, "/api/v1/profiles/rebuild", body, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("无凭证该 401，得到 %d", rec.Code)
	}
	// dadTok 的作用域是 dad（batch_test.go 里那条 ai-dad:tok-dad:dad），打 kid 就是 403。
	if rec := post(h, "/api/v1/profiles/rebuild", body, "", dadTok); rec.Code != http.StatusForbidden {
		t.Errorf("档案作用域令牌该 403，得到 %d", rec.Code)
	}
	// 目录都没有 → 404，且**不建目录**（这条端点不是第二条建档通路）。
	before := dirCount(t, s)
	rec := post(h, "/api/v1/profiles/rebuild",
		`{"profile":"ghost","title":"幽灵","admin":false,"pin":"4321"}`, "", adminTok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("没这份档案该 404，得到 %d %.200s", rec.Code, rec.Body)
	}
	if n := dirCount(t, s); n != before {
		t.Errorf("rebuild 造出了 %d 个新目录", n-before)
	}
}
```

本包**没有**"按档案取令牌"的助手，也不要新造：令牌常量在 `batch_test.go` 的 `const` 块里（`adminTok` = `*`，`dadTok` = 作用域 dad）。

- [ ] **Step 3: 跑它，确认红在"404 页面路由不存在"**

Run: `go test ./internal/httpapi -run 'TestRebuild' -count=1 -v`
Expected: 全部 FAIL 在 404/405（路由还没注册）。这是编译通过之后的真红。

- [ ] **Step 4: 写 handler**

新建 `internal/httpapi/rebuild.go`：

```go
package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

// rebuildResponse 用 struct 不用 map：键序由字段序决定（§4.3）。
// 回显 title/admin 是有意的——调用方要能确认自己说出口的那两格真按说的写了，
// 而不是拿一个裸 ok（"重建"这件事没有第二次机会去猜）。
type rebuildResponse struct {
	Profile string `json:"profile"`
	Rebuilt bool   `json:"rebuilt"`
	Title   string `json:"title"`
	Admin   bool   `json:"admin"`
}

// handleRebuildProfile 是坏 profile.json 的唯一被支持出路：只重写那一个文件。
//
// 判据一条都不在这里写：读不读得动、目录在不在、哪种错误允许覆盖，全在
// store.RebuildProfile 的那一次临界区里（权威源 §4.5）。handler 只做三件事——
// 外壳（解码/鉴权/作用域）、把 store 的分诊翻成 HTTP、留一行历史。
//
// 与 handleAdminPin 的三处刻意差别，每处都有理由：
//   - ErrCorrupt 在这里**不答 503**：那条端点的 503 是"你想改但档案坏了"，
//     而这条端点的存在理由就是修它——把坏档当拒绝理由，这条通路就永远打不通。
//   - 覆盖成功之后的 store 判据是 ErrReadable → 409（不是 403/422）：档案读得动
//     时该做的是设 PIN，指路那句写在 Detail 里。
//   - 写完仍读不动答 500 而不是 503：那时"读不动"是**我们刚刚写的那份文件**造成的，
//     说成"服务暂时不好"会把调用方引去重试一个自己已经毁掉写的动作。
func (s *Server) handleRebuildProfile(w http.ResponseWriter, r *http.Request) {
	// Admin 是 *bool：encoding/json 分不开"没给这一格"与"给了 false"，而这条端点
	// 必须分得开——把唯一管理员静默降级成普通档案，代价不是"少一个勾"。
	var in struct {
		Profile string `json:"profile"`
		Title   string `json:"title"`
		Admin   *bool  `json:"admin"`
		PIN     string `json:"pin"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if in.Admin == nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "admin", Reason: "必须显式给出（省略不等于 false——静默降级唯一管理员的代价在这里）"}})
		return
	}
	hash, pinErrs := hashFormPIN(in.PIN) // PIN 判据只有一份：建档族共用（Task 3）。
	if len(pinErrs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, pinErrs)
		return
	}
	actor, _, ok := s.require(r, credAdmin, in.Profile)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	p, err := s.deps.Store.RebuildProfile(store.RebuildInput{
		ID: in.Profile, Title: in.Title, PinHash: hash, Admin: *in.Admin,
	})
	switch {
	case errors.Is(err, store.ErrReadable):
		writeJSON(w, http.StatusConflict, errorResponse{
			Code: 409, Detail: "这份档案读得动，不需要重建；改 PIN 请用 POST /api/v1/profiles/pin"})
		return
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "档案不存在"})
		return
	case errors.Is(err, store.ErrInvalid):
		// store 的校验错点名磁盘路径，与 500 同一理由不回显；给一句人话，
		// 但必须说得出是哪一格（title 空 / id 不是 slug 两种）。
		writeFieldErrors(w, http.StatusUnprocessableEntity, rebuildFieldErrs(in.Profile, in.Title))
		return
	case err != nil:
		writeInternalError(w, err)
		return
	}
	// 历史行与设 PIN 同形状：rev:0（不动书签计数器）、path 点名动了什么。
	if err := s.deps.Store.AppendHistory(store.HistoryEntry{
		At: time.Now().Format(time.RFC3339), Profile: in.Profile, Actor: actor,
		By: byOfRequest(r), Rev: 0,
		Ops: historyOps([]ops.Brief{{Op: "set", Path: "profile/rebuild"}}),
	}); err != nil {
		// 与 handleAdminPin 的裁定 C 同口径：重建是幂等的，"文件已修好但审计行没落"
		// 必须让调用方知道（500 是唯一能说的），不许学 batch/revert 的 200+warning。
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rebuildResponse{
		Profile: p.ID, Rebuilt: true, Title: p.Title, Admin: p.Admin})
}
```

- [ ] **Step 5: 把 `rebuildFieldErrs` 补进同一个文件**

`store.ErrInvalid` 的文本含磁盘路径，不能回显（与 500 同一理由），但 422 必须说得出是哪一格没过：

```go
// rebuildFieldErrs 把 store 的 ErrInvalid 翻成点名到格子的 422。判据不在这里——
// 这里只**说**是哪一格没过，不重算任何事（重算就是第二份判据）。
func rebuildFieldErrs(profile, title string) []fieldError {
	var errs []fieldError
	if !model.ValidSlug(profile) {
		errs = append(errs, fieldError{Field: "profile", Reason: "必须是 ASCII slug（小写字母、数字、连字符）"})
	}
	if strings.TrimSpace(title) == "" {
		errs = append(errs, fieldError{Field: "title", Reason: "不能为空"})
	}
	if len(errs) == 0 {
		// store 说不通而这两格都说得通：不猜原因，给一句人话并点名这一族。
		errs = append(errs, fieldError{Field: "profile", Reason: "档案数据不合契约，重建未执行"})
	}
	return errs
}
```

需要 `model` 与 `strings` 两个 import。`in.Admin == nil` 那一支已在 Step 4 落地——Step 2 的第一条测试（缺 `admin` 该 422）就是它的证人，别在这里再加第二处检查。

- [ ] **Step 6: 注册路由（`internal/httpapi/routes.go`）**

在 `{Method: "POST", Pattern: "/api/v1/profiles/pin", …}` 那条之后插入（与设 PIN 相邻——两条都是"管理员替某个档案动档案本身"）：

```go
		{Method: "POST", Pattern: "/api/v1/profiles/rebuild", Cred: credAdmin,
			Summary: "重建坏掉的 profile.json（只重写那一个文件，书签/快照/历史不动）", H: s.handleRebuildProfile},
```

Run: `go test ./internal/httpapi -run 'TestOpenAPICoversEveryRoute|TestAgentsMDMatchesRoutes' -count=1 -v`
Expected: **两条都 FAIL**，各自点名 `/api/v1/profiles/rebuild`。这一步不许绿——它是 Step 7/8 的输入清单。要是它们没红，说明这条注册根本没被闸看见（停下查闸，别接着写文档）。

- [ ] **Step 7: `openapi.json` 加一个 operation**

插入位置：`"/api/v1/profiles/pin"` 那个对象之后、`"/api/v1/usage"` 之前。`summary` 必须与 Step 6 路由表那句**逐字一致**（`TestOpenAPICoversEveryRoute` 按字面比）。

```json
    "/api/v1/profiles/rebuild": {
      "post": {
        "summary": "重建坏掉的 profile.json（只重写那一个文件，书签/快照/历史不动）",
        "x-credential": "admin",
        "security": [
          {
            "bearerAuth": []
          },
          {
            "sessionCookie": []
          }
        ],
        "description": "坏 profile.json 的唯一被支持出路：只重写 profile.json，bookmarks.json、snapshots/、history.jsonl 已有内容一个字不碰。{profile, title, admin, pin} 四格全部必填——admin 省略不等于 false（静默把唯一管理员降级是要紧事，必须显式说）。判据只在 store 的一次临界区里问一次：档案读得动 409（要改 PIN 请走 POST /api/v1/profiles/pin，这条不是第二条改档通路）；档案目录根本不存在 404 且**不创建目录**（建档请走 POST /api/v1/profiles）；有目录、没有 profile.json 的半件事目录允许重建；只有「内容读不动」这一种才允许覆盖，权限/磁盘那一类 I/O 错原样上报、不覆盖。写完会在同一把锁内回读一次，仍读不动回 500 且坏档登记留着——响应说成功，进门页那张卡就真的回来了。X-LB-By 头可选（缺省 web，非法按 web），目标档案历史里追加一条 {rev:0, ops:[{op:\"set\", path:\"profile/rebuild\"}]}。注意：文件已修好而历史行没落时同样回 500——重建幂等、重试收敛，这条与设 PIN 的裁定 C 同口径，与 batch/revert 的\"历史失败照报 200\"相反。",
        "parameters": [
          {
            "name": "X-LB-By",
            "in": "header",
            "required": false,
            "schema": {
              "type": "string",
              "enum": [
                "ai",
                "bookmarklet",
                "extension",
                "web",
                "import",
                "sync-template",
                "cli"
              ]
            },
            "description": "自报渠道，缺省 web；非法值按 web"
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": [
                  "profile",
                  "title",
                  "admin",
                  "pin"
                ],
                "additionalProperties": false,
                "properties": {
                  "profile": {
                    "type": "string",
                    "description": "ASCII slug（档案目录名）"
                  },
                  "title": {
                    "type": "string",
                    "description": "档案显示名，不能为空"
                  },
                  "admin": {
                    "type": "boolean",
                    "description": "必填：省略不等于 false"
                  },
                  "pin": {
                    "type": "string",
                    "description": "4-6 位数字"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "已重建（回显 title/admin，让调用方确认自己说出口的那两格真按说的写了）",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "profile": {
                      "type": "string"
                    },
                    "rebuilt": {
                      "type": "boolean"
                    },
                    "title": {
                      "type": "string"
                    },
                    "admin": {
                      "type": "boolean"
                    }
                  }
                }
              }
            }
          },
          "401": {
            "$ref": "#/components/responses/Unauthorized"
          },
          "403": {
            "$ref": "#/components/responses/Forbidden"
          },
          "404": {
            "$ref": "#/components/responses/NotFound"
          },
          "409": {
            "description": "这份档案读得动，不需要重建（detail 指路 profiles/pin），且没有任何写入发生",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            }
          },
          "422": {
            "description": "字段级错误（profile/title/admin/pin，或未知键）：{code, detail（字段级错误时为空串）, errors[{field, reason}]}"
          }
        }
      }
    },
```

**没有 `500` 这一档**不是漏写：整份文档里没有一条端点把 500 列成 response（`/api/v1/profiles/pin` 也一样，只在 `description` 里说）。跟着这个口径走，别为这一条破例——破了例就得回头给其余每条补，那是十几处与本片无关的改动。

- [ ] **Step 8: `AGENTS.md` 加一行**

在 `POST | /api/v1/profiles/pin` 那行之后插入：

```markdown
| POST | `/api/v1/profiles/rebuild` | 管理员 | 200 `{profile,rebuilt,title,admin}` | **坏 `profile.json` 的唯一被支持修法**：只重写 `profile.json`，书签/快照/历史不动。`profile/title/admin/pin` 四格全必填（`admin` 省略 ≠ false）；读得动 409、目录不存在 404（**不建目录**，建档仍走 `POST /api/v1/profiles`）；只有内容读不动才允许覆盖。**别走"删掉坏文件再建档"**：`CreateProfile` 会无条件写一份 `bookmarks.json`，把还能读的书签一起覆盖掉 |
```

这是本片在 `AGENTS.md` 的唯一改动：那一条端点行。"不许直接改 `data/`"那条禁止事项已经有"绕过 REST 等于制造第二份真相"的话，rebuild 不新增例外，所以不碰它——给一份契约加一句只有本片成立的豁免，是把文档改成补丁堆。

- [ ] **Step 9: 契约闸跑到绿（名单逐条点名）**

Run: `go test ./internal/httpapi -count=1 -v`，再 `go test ./... -count=1`
Expected: 全绿。这批改动里"文档没说谎"的证据就是这几道闸，一条都不许跳过：
`TestRoutesTableIsComplete`、`TestOpenAPICoversEveryRoute`、`TestOpenAPIHasNoGhostEndpoints`、
`TestOpenAPIMethodSetsMatchPerPath`、`TestOpenAPIEndpointServesTheEmbeddedDoc`、`TestOpenAPI的枚举与model同集`、
`TestAgentsMDMatchesRoutes`、`TestAgentsMDMethodColumn`、`TestAgentsMD枚举与model同集`。

- [ ] **Step 10: 四把枪，逐支单独打**

1. **招牌枪（PIN 判据不许长出第二份）**：在 `handleRebuildProfile` 里绕过 `hashFormPIN`，自己写一遍
   （`auth.ValidPinFormat(in.PIN)` + `auth.HashPIN(in.PIN)`）。
   Run: `go test ./internal/httpapi -run 'Test建档判据只有一处' -count=1 -v`
   Expected: FAIL **两条都要**——`handleRebuildProfile 里出现了 N 次 auth.HashPIN——PIN 判据被抄出合法持有者了`
   与 `handleRebuildProfile 里一次都没调 hashFormPIN`。这一枪不打，Task 3 换过口径的那道闸就是新形状的假闸。
2. **文档说谎**：把 `AGENTS.md` 那行的路径改成 `/api/v1/profiles/rebuild2`。
   Expected: `TestAgentsMDMatchesRoutes` FAIL（幽灵端点）。
3. **凭证档写错**：把 openapi 的 `"x-credential": "admin"` 改成 `"token-or-session"`。
   Expected: `TestOpenAPICoversEveryRoute` FAIL，点名 x-credential 与路由表 Cred 不符。这一枪钉的是"AI 照文档
   发出去的请求会不会被自己按 403 挡回来"。
4. **409 那条判据**：把 handler 里 `case errors.Is(err, store.ErrReadable)` 那一支删掉（让它落进
   `case err != nil` 的 500）。
   Expected: Step 2 里"读得动的档案该 409 且没有任何写入"那条测试红。

每支打完立刻还原；四支都还原后 `go test ./internal/httpapi -count=1` 必须全绿（破坏脚本要先断
`mutated != original`，区分 `build_err`，跑完用 sha 证还原）。

- [ ] **Step 11: 门禁 + 提交**

```bash
go build ./... && go vet ./... && gofmt -l internal cmd && go test ./... -count=1
git add internal/httpapi/rebuild.go internal/httpapi/rebuild_test.go internal/httpapi/routes.go internal/httpapi/openapi.json internal/httpapi/createrule_test.go AGENTS.md
git commit -m "feat(httpapi): POST /api/v1/profiles/rebuild——坏 profile.json 有了不碰书签的出路，契约三处同轮换字"
```

---

### Task 5: CLI 子命令 `launchbay profile rebuild`

**Files:**
- Modify: `cmd/launchbay/cli.go`
- Modify: `cmd/launchbay/cli_test.go`

**Interfaces:**
- Consumes: Task 4 的路径 `/api/v1/profiles/rebuild`；既有 `newAdminClient(cfg.Port, cfg.Tokens)`、`(*client).do(method, path, body, headers)`、`mustOK(raw, code, path)`、`runCLI`、`cliCfg`（测试里）。
- Produces: `profile rebuild` 子命令；用法行更新。

- [ ] **Step 1: 写失败测试**

在 `cmd/launchbay/cli_test.go` 末尾加：

```go
// CLI 的参数→请求体接线。两条各自有理由：
//   - --admin 省略必须发 false（**显式**发），不能省略字段：服务端把"没给 admin"
//     当 422，因为静默降级唯一管理员的代价太大。CLI 若"省略就不发"，这条路在
//     命令行上就打不通。
//   - X-LB-By 必须是 cli，否则历史里这条动作会被记成 web。
func TestCLIProfileRebuildWiresEveryField(t *testing.T) {
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:tok-admin:*")
	var bodies []map[string]any
	var bys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		bodies = append(bodies, m)
		bys = append(bys, r.Header.Get("X-LB-By"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":"mom","rebuilt":true,"title":"妈妈","admin":false}`))
	}))
	defer srv.Close()
	var buf bytes.Buffer
	if code := runCLI([]string{"profile", "rebuild", "--id", "mom", "--title", "妈妈", "--pin", "4321"},
		cliCfg(t, srv), &buf); code != 0 {
		t.Fatalf("该 0，实际 %d out=%q", code, buf.String())
	}
	got := bodies[0]
	if got["profile"] != "mom" || got["title"] != "妈妈" || got["pin"] != "4321" {
		t.Errorf("参数没接对: %+v", got)
	}
	if v, ok := got["admin"]; !ok || v != false {
		t.Errorf("--admin 省略时必须显式发 false，得到 %v (present=%v)", v, ok)
	}
	if bys[0] != "cli" {
		t.Errorf("X-LB-By 丢了: %q", bys[0])
	}
	if !strings.Contains(buf.String(), "mom") || !strings.Contains(buf.String(), "妈妈") {
		t.Errorf("成功话术该带上 id 与名字: %q", buf.String())
	}
	// 第二发：--admin 给了就该发 true。
	if code := runCLI([]string{"profile", "rebuild", "--id", "mom", "--title", "妈妈", "--pin", "4321", "--admin"},
		cliCfg(t, srv), &buf); code != 0 {
		t.Fatalf("code %d out=%q", code, buf.String())
	}
	if bodies[1]["admin"] != true {
		t.Errorf("--admin 该发 true，得到 %v", bodies[1]["admin"])
	}
}

// 服务端 409（档案读得动）→ 退出码 1、detail 原样带出，且 CLI 没在本地写任何东西。
func TestCLIProfileRebuildSurfaces409(t *testing.T) {
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:tok-admin:*")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":409,"detail":"这份档案读得动，不需要重建；改 PIN 请用 POST /api/v1/profiles/pin"}`))
	}))
	defer srv.Close()
	var buf bytes.Buffer
	dir := t.TempDir()
	t.Setenv("LAUNCHBAY_DATA_DIR", dir)
	if code := runCLI([]string{"profile", "rebuild", "--id", "mom", "--title", "妈妈", "--pin", "4321"},
		cliCfg(t, srv), &buf); code != 1 {
		t.Fatalf("409 该退出 1，实际 %d out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "profiles/pin") {
		t.Errorf("服务端的指路话该原样带出: %q", buf.String())
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("CLI 在本地写了东西: %v", ents)
	}
}
```

- [ ] **Step 2: 跑它，确认红在"未知子命令/用法行"**

Run: `go test ./cmd/launchbay -run 'TestCLIProfileRebuild' -count=1 -v`
Expected: FAIL（`profile rebuild` 走到用法分支，退出码 2）。

- [ ] **Step 3: 实现**

`cmd/launchbay/cli.go` 的 `cmdProfile` 改成按 `args[0]` 分两路（保持 `create` 那段一字不动，新增 `rebuild` 分支）：

```go
func cmdProfile(args []string, cfg config, out io.Writer) int {
	if len(args) == 0 {
		profileUsage(out)
		return 2
	}
	switch args[0] {
	case "create":
		return cmdProfileCreate(args[1:], cfg, out)
	case "rebuild":
		return cmdProfileRebuild(args[1:], cfg, out)
	}
	profileUsage(out)
	return 2
}

func profileUsage(out io.Writer) {
	fmt.Fprintln(out, "用法: launchbay profile create  --id <slug> --title <名字> --pin <4-6 位> [--from-template nas] [--admin]")
	fmt.Fprintln(out, "      launchbay profile rebuild --id <slug> --title <名字> --pin <4-6 位> [--admin]")
	fmt.Fprintln(out, "      （rebuild 只修读不动的 profile.json，不碰书签；档案读得动它会 409）")
}

// cmdProfileRebuild 只经本机 REST——CLI 唯一的写通路（spec §4.5）。
// admin 这一格**总是发**：省略 = 显式 false，不是不发字段（服务端把"没给"当 422，
// 因为静默降级唯一管理员的代价太大）。
func cmdProfileRebuild(args []string, cfg config, out io.Writer) int {
	fs := flag.NewFlagSet("profile rebuild", flag.ContinueOnError)
	fs.SetOutput(out)
	id := fs.String("id", "", "档案 id（ASCII slug，目录必须已存在）")
	title := fs.String("title", "", "显示名（旧的那个在读不动的文件里，拿不回来）")
	pin := fs.String("pin", "", "4-6 位数字")
	admin := fs.Bool("admin", false, "建成管理员档案")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := newAdminClient(cfg.Port, cfg.Tokens)
	if err != nil {
		fmt.Fprintf(out, "profile rebuild 失败: %v\n", err)
		return 1
	}
	body, _ := json.Marshal(map[string]any{
		"profile": *id, "title": *title, "pin": *pin, "admin": *admin})
	raw, code, err := c.do("POST", "/api/v1/profiles/rebuild", body,
		map[string]string{"Content-Type": "application/json", "X-LB-By": "cli"})
	if err != nil {
		fmt.Fprintf(out, "profile rebuild 失败: %v\n", err)
		return 1
	}
	if err := mustOK(raw, code, "/api/v1/profiles/rebuild"); err != nil {
		fmt.Fprintf(out, "profile rebuild 失败: %v\n", err)
		return 1
	}
	tag := ""
	if *admin {
		tag = "，管理员"
	}
	fmt.Fprintf(out, "已重建 %s（%s%s），书签与快照未改动
", *id, *title, tag)
	return 0
}
```

- [ ] **Step 4: 跑测试到全绿 + 全量**

```bash
go test ./cmd/launchbay -count=1 -v 2>&1 | grep -E "^(--- FAIL|--- PASS: TestCLIProfileRebuild|ok|FAIL)"
go test ./... -count=1
```
Expected: 两条新测试 PASS，全量 `ok`。

- [ ] **Step 5: 两把枪**

| 枪 | 破坏 | 必须红在 |
|---|---|---|
| L1 | 把 body 里的 `"admin": *admin` 改成"仅当 `--admin` 给了才放进 map" | `TestCLIProfileRebuildWiresEveryField` |
| L2 | 去掉 `X-LB-By` 头 | 同一条测试的 by 断言 |
| L3 | 反向：把成功话术里的"书签与快照未改动"删掉 | `TestCLIProfileRebuildWiresEveryField` 的话术断言（它只要求带 id 与名字——**这把枪预期不红**，那就说明话术没被钉；补一条 `strings.Contains(..., "书签与快照未改动")` 再打，别把"不红"读成"没问题"） |

- [ ] **Step 6: 门禁 + 提交**

```bash
gofmt -l internal cmd && go vet ./... && go build ./... && go test ./... -count=1
git add cmd/launchbay/cli.go cmd/launchbay/cli_test.go
git commit -m "feat(cli): launchbay profile rebuild——救急通路要打得动才算存在（admin 总是显式发）"
```

---

### Task 6: 端到端验收与读数

**Files:**
- Create: `docs/superpowers/acceptance/p1-46-rebuild/2026-10-09-验收.md`
- Create（同目录）：坏档前 / 后的进门页实拍各一张，加 rebuild 之后那张

**Interfaces:**
- Consumes: 全部前序任务。
- Produces: 权威源 §13 那八条的实测读数。

- [ ] **Step 1: 起一个干净实例（不碰仓库 `data/`）**

夹具落在 `.superpowers/tmp/p46/{before,after}/`，端口 `17641/17642`，令牌 `LAUNCHBAY_WRITE_TOKENS=admin:tok-fixture-only:*`。
**两条纪律**（都是本轮之前实测踩过的）：起进程后要求"我起的这个还活着"（`p.poll() is None`）才算起来——端口能被上一个没死干净的实例答话；断言响应体里含**当前构建才有的特征串**，缺了就是看错了对象。

- [ ] **Step 2: 走一遍真实救援，全程留读数**

1. `POST /setup` 建第一个档案（表单）→ `POST /api/v1/profiles` 建第二个（`mom`/妈妈）；
2. 用 `mom` 的令牌写两条书签，`GET /api/v1/data?profile=mom` 存下**原文字节**；
3. 把 `mom` 的 `profile.json` 写成半截 JSON；
4. `GET /` 拍一张（应看到「有档案的数据文件读不了…」+「进不去的档案：mom」+ 爸爸那张卡还在）；
5. `GET /api/v1/profiles/pin` 那条通路打一次设 PIN → 记下它 **503**（这就是"旧出路对坏掉的档案失效"的读数）；
6. 跑 `launchbay profile rebuild --id mom --title 妈妈 --pin 4321` → 退出码 0；
7. 再 `GET /api/v1/data?profile=mom` → 与第 2 步的字节**逐字节相同**（`cmp` 或 sha256 两边对比）；
8. `GET /` 再拍一张（mom 的卡回来了）；`GET /api/v1/status` → `brokenProfiles` 为空；
9. `GET /api/v1/history?profile=mom` → 多一行 `rev:0` + `path:"profile/rebuild"`；
10. 拿 `4321` 走 `POST /pin` → 303 + 会话 cookie（新 PIN 真能用）。

Run: 全部读数写进 `.superpowers/tmp/p46/walk.out`（含 CJK 的判定**落盘用 Read**，别扫 stdout）。

- [ ] **Step 3: 实拍两张并目检**

`1280×900`、设备像素比 2，坏档前后各一张；入库前等比降回 CSS 尺寸，降完重新 Read 一遍再提交。

- [ ] **Step 4: 门禁与跨平台复跑**

```bash
go test ./... -count=1            # exit 0
gofmt -l internal cmd             # 空
go vet ./...                      # 0
go run ./tools/leakguard          # 0 处泄漏（读数与提交同批：文件数会变，写进文档时重新量）
SRC="$(cygpath -w "$(git rev-parse --show-toplevel)")"   # 不写死绝对路径：目录改过名，这条照样跑得通
MSYS_NO_PATHCONV=1 docker run --rm -v "$SRC":/src -w /src golang:1.27-alpine \
  sh -c 'go test ./internal/store ./internal/httpapi -race -count=2'
```
Expected: 容器里两条 `ok`（本机 `CGO_ENABLED=0` 跑不了 `-race`，这条方法账要写进验收文档）。

- [ ] **Step 5: 预算与页面形状核对（本片应当一动不动）**

Run: `go test ./internal/render -run TestBudget -v -count=1`
Expected: 六条读数与 `budget_test.go` 注释里的数**完全一致**（本片没加 CSS、没加页面形状）。若有任何一条漂了，说明有人在本片里动了 `internal/render/**`——回去查 diff，别改注释迁就。

- [ ] **Step 6: 写验收文档**

六个小节，形状与 `docs/superpowers/acceptance/p1-45-first-run/2026-10-08-验收.md` 一致：① 八条验收逐条结果表；② 机器读数（含 `walk.out` 的关键行、字节比对、`/status` 归零）；③ 实拍清单；④ 方法账（怎么起实例、怎么算"我起的进程"、为什么 `-race` 在容器里跑、CJK 落盘）；⑤ 没做的（页面入口在 ⑤；"最后一个管理员"不检查；`theme/layout` 不保留）；⑥ 交付标准对照（设计 §7 十二条逐条指回测试名与读数）。

- [ ] **Step 7: 提交**

```bash
git add docs/superpowers/acceptance/p1-46-rebuild
git commit -m "docs(acceptance): 坏档救援走通了一遍——从进门页那句解释到 rebuild 之后逐字节相同的书签"
```

---

## 计划的自查（写完回头核，不另派席位）

- **设计覆盖**：设计 §2.1 → Task 2；§2.2（PIN 判据共用一份 + AST 闸同轮换口径）→ Task 3 Step 2 抽函数、Task 3 Step 3 换闸口径、Task 4 Step 1 把第二条壳登进名单；§3 → Task 4；§4 → Task 5；§5 契约同步清单 → Task 4 Step 6/7/8/9（routes、openapi、AGENTS、闸名单）；§6 权威源改字清单 → Task 1；§7 十二条 → Task 2 Step 2、Task 4 Step 2、Task 5 Step 1、Task 6 Step 2；§8/§9/§10 是"不做与残余"，不需要任务，但 Task 6 第⑤节必须复述一遍。
- **无占位**：每段代码都是可直接抄的形状，没有"稍后补"、没有"参照上一个任务"（跨任务要一致的地方都重复写了字面量）。计划里唯一要求执行者自己判的，是各 Step 的 `Run` + `Expected`——那是每条测试要留的失败证据，不是占位。
- **类型一致**：`store.RebuildInput{ID,Title,PinHash,Admin}`、`(*Store).RebuildProfile(RebuildInput) (*model.Profile, error)`、`store.ErrReadable`、`hashFormPIN(string) (string, []fieldError)`、`(s *Server) handleRebuildProfile`、`rebuildResponse{Profile,Rebuilt,Title,Admin}`、路由键 `POST /api/v1/profiles/rebuild` —— 七处名字在 Task 2/3/4/5 之间逐字对齐。最后那条不是风格问题：AST 闸的建档族名单按**函数名字面量**匹配（`pinFamily := []string{"createProfile", "handleRebuildProfile"}`），handler 改名而名单没改，那条闸就变成认不存在之物的空转——它由 Task 4 Step 1 那次红（`handleRebuildProfile 在整包里没有定义`）与 Step 10 第一把枪一起钉着。
