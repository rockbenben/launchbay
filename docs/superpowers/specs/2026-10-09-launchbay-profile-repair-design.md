# launchbay 坏档案的机器出路（profile rebuild）· 设计（P1-④.6）

日期：2026-10-09。上游权威源：`2026-10-05-launchbay-design.md`（下称 §x）。
状态：**已落地（2026-10-10 晚）**。`store.RebuildProfile` 与它的测试在 2026-10-09 就写好了，
而端点、路由、openapi、AGENTS.md 与 CLI 那一层**一直缺席到 2026-10-10 补上**——
中间那一天里本文件、README §10 与 admin 页面都在指着一条**打不通**的命令
（实测：`POST /api/v1/profiles/rebuild` 405、`launchbay profile rebuild` 打印 create 的用法）。
教训写在下面，别让它再来一次：**设计批准 ≠ 已交付**，而文档不会因为实现没跟上就自动闭嘴。

> **2026-10-12：本文里凡带 `admin` 的契约形状全部作废**（档案不再有管理员属性）。
> 照抄即失败的那几处，正文里已就地改写；其余由这段覆盖：
>
> - 请求体是 **`{profile, title, pin, nopass}` 四格**，**没有 `admin`**。送 `admin` 来会得到
>   **422 且点名该字段**（不是静默忽略——沉默会让脚本以为"建成管理员了"）。
>   而 `pin` 与 `nopass` **二选一**：`nopass:true` 重建出一份**不设防**的档案
>   （rebuild 必须能造出这个形状，它是唯一能整份重写 `profile.json` 的动作），
>   两个都给答 422 点名 `nopass`、都不给答 422 点名 `pin`。
> - 成功响应是 **`{id, title}`**，没有 `admin` 那一格。
> - CLI 是 `launchbay profile rebuild --id <slug> --title <名字> (--pin <4-6位> | --nopass)`，
>   **没有 `--admin` 旗标**。凭证是 `LAUNCHBAY_ADMIN_TOKEN` 那把机器级 key，
>   不是从 `LAUNCHBAY_WRITE_TOKENS` 里挑一枚。
> - 下面"必填字段"那一行、`--admin` 的开关与"省略发 false"那条接线证人、以及
>   §9 残余风险里"把唯一管理员 rebuild 成 `admin:false`"那一整条**都不存在了**：
>   管理员与档案无关，删光档案也还管得了，也就不存在"唯一管理员"可以被弄没。
> - **本文其余部分（锁的形状、分诊、`ErrReadable` / `ErrNotFound` 的判据、
>   运维读数）一字未改且仍然成立。** 机器可读的契约以 `internal/httpapi/openapi.json`
>   与 `AGENTS.md` 为准，本文正文不是。

定位：一片小而硬的**写通路**——它不新增子系统，只补一条被支持的坏档出路，并顺手把
"PIN 判据不许有第二份"这道闸改成能覆盖它的形状。
触发：P1-④.5 验收结转的死路①（唯一档案的 `profile.json` 读不动 → 进门页零张卡）。那一片
把"说不清"改成了"说得清"（`ab0b113`），但**出路本身仍不存在**，人在 2026-10-09 要求把这条路
也做成设计。

## 0. 问题（四条实测事实，不是推测）

锚点按字面量给（行号会腐烂）。

1. **进门页只能点名，不能救人。** `internal/render/render.go` 的 `PickerPage` 现在会画
   「有档案的数据文件读不了，暂时进不去。要在这台机器上修好那个文件才能继续。」并列出坏档 id——
   这句话今天是一句**没有对应工具**的指示：产品里没有任何一条通路能修好它。
   （2026-10-10 文案统一改判：原句是「……读不了，进不去。……才进得去。」，
   一句里把「进不去」说了两遍。三个被测试逐字钉住的片段一个没动，详见
   `2026-10-08-launchbay-first-run-design.md` §6.1。）
2. **`CreateProfile` 走不通。** 它在锁内第一件事就是
   `if _, err := os.Stat(s.ProfilePath(in.ID)); err == nil { return ErrExists }`——
   坏掉的 `profile.json` 也是"存在"。
3. **`POST /api/v1/profiles/pin` 走不通。** `handleAdminPin` 在 `WithProfileLock` 里
   `LoadProfile` → 坏档返回 `ErrCorrupt` → `writeProfileReadFailure` 答 503。
   也就是说"给任意档案设 PIN"这条管理员出路，恰恰对最需要它的档案失效。
4. **手工路线是产品自己禁止的，而且它会吃数据。** AGENTS.md 禁止事项：不许直改 `data/`。
   更要紧的是：如果有人照"删掉坏文件再建档"去救，`CreateProfile` 在
   `SaveProfile` 之后无条件 `s.SaveBookmarks(in.ID, b)`——那份**还在盘上的** `bookmarks.json`
   会被空档案覆盖掉（当时还有"或模板"，那半句 2026-10-10 随模板机制整条删掉了）。同理不能复用 `createProfile` 来"重建"：它的回滚是
   `rollbackDir` = `os.RemoveAll(s.ProfileDir(id))`，对一个已有数据的档案，失败一次就把
   书签与快照一起删了。

结论：需要一条**只重写 `profile.json`、失败时不碰别的文件**的通路。

## 1. 拍板记录（人是决定者，实现者不许改口径）

| 决定 | 选了什么 | 为什么不选另一支 |
|---|---|---|
| 范围 | 只补机器通路（端点 + CLI），不给页面入口 | 页面入口要二次确认，而 ④.5 定下"设 PIN/删档这类动作的确认随 ⑤ 的 JS 落地"。现在进去要么发明一种零 JS 的确认形状（新纪律），要么不带确认就动一个正在坏的文件 |
| 端点形状 | 新端点 `POST /api/v1/profiles/rebuild` | 扩展现有 `profiles/pin` 会让一个端点有两种语义——AI 以为只改 PIN，实际重置了 `theme`/`layout`/`createdAt`；AGENTS.md 是给 AI 读的那份契约，这种含糊最贵。给 `POST /profiles` 加 `rebuild` 旗标则让 `ErrExists` 判据长分支，且"旗标 × 模式"的组合会丢选择 |
| 必填字段 | `title`、`admin`、`pin` 三个都必须出现 | `admin` 省略即 `false` 意味着一次省略就能把这台盒子唯一的管理员弄没（而那时 `/admin`、设 PIN、删档全部不可达，且不会报错）。"只给 pin、title 用 id 顶"会把成本转嫁给下一个读进门页的人 |
| 前置判据 | 只在读不动时允许；读得动答 409 指回 `profiles/pin` | 任何时候都允许 = 这条与设 PIN、与 ⑤ 将来的"改档案"叠成三条路，且健康档案的字段会被悄悄重置 |
| CLI | 要：`launchbay profile rebuild` | 修档案的人通常就在那台机器上、令牌就在 env 里；而 AGENTS 第 4 条坑（带中文的 body 要从文件发）CLI 天然绕过。"救急通路越难打，实际就约等于没有" |

## 2. 判据与落点（store）

### 2.1 唯一判据：锁内那一次读

新文件 `internal/store/rebuild.go`，新方法：

```go
// RebuildProfile 只重写 profile.json：书签、快照、历史一个字不碰。
type RebuildInput struct {
	ID      string
	Title   string
	PinHash string
	Admin   bool
}

func (s *Store) RebuildProfile(in RebuildInput) (*model.Profile, error)
```

锁内顺序（`WithProfileLock(in.ID, …)`，与 `CreateProfile`/`handleAdminPin` 同一把按 id 的锁）：

1. `LoadProfile(in.ID)`：
   - **成功** → 返回 `ErrReadable`（新哨兵，`internal/store` 里与 `ErrExists` 并列）。
     判据只在这里问一次，handler 不再查 `Broken()`、不再数目录。
   - `ErrNotFound` **且档案目录存在**（半件事目录：有 `profiles/<id>/`，没有
     `profile.json`）→ 继续第 2 步。
   - `ErrNotFound` 且目录也不在 → 原样返回（handler 答 404）。**rebuild 不建目录**：
     建一份新档案是 `POST /api/v1/profiles` 的事，从这条端点建出来会是一份没有
     `bookmarks.json` 的半件事档案，而那等于给"建档判据只有一份"开第二个门。
   - `ErrCorrupt` → 继续。
   - **其它错误**（权限、I/O、盘坏）→ 原样返回，**不覆盖**。这条不是装饰：读不动不等于
     内容错，那时盘上那份文件可能还能救，覆盖它就是二次事故。
2. 组装 `model.Profile{ID, Title, Admin, PIN: in.PinHash, CreatedAt: time.Now(),
   Theme: "auto", Layout: "sidebar"}`，`p.Validate()` 不过 → `ErrInvalid`（校验排在落盘之前，
   与 `CreateProfile` 同口径）。
3. `SaveProfile(&p)`（目录在第 1 步已经确认存在，这里不 `MkdirAll`）。
   **失败时不回滚、不删任何东西**——这里与 `CreateProfile` 的关键区别必须写进注释：
   那份目录里可能有真数据，`RemoveAll` 是二次事故（见 §0 第 4 条）。
4. 写完**立刻在同一把锁内回读一次** `LoadProfile(in.ID)`：
   - 读得动 → 走既有路径清坏档登记（`load.go` 里那句 `clearBroken(id, brokenProfile)`），
     返回 `&p`。
   - 仍然读不动 → 返回错误（handler 答 500），坏档记录**留着**。
   代价是一次多余的读；换来的是"响应说成功，进门页那张卡就真的回来了"，而不是
   `/status` 里挂一条指向已修好档案的幽灵坏档。为什么不直接在写成功时 `clearBroken`：
   那等于在 store 里再判一次"读得动吗"，判据就有两份了。

### 2.2 PIN 判据不许长出第二份

`createProfile`（`internal/httpapi/handlers.go`）现在自己走 `auth.ValidPinFormat` →
`auth.HashPIN`。rebuild 也要走同一段。**不许在 rebuild 的 handler 里再写一遍**——那正是
P1-④.5 花一整轮闭掉的病。做法：

```go
// hashFormPIN 是表单/API 两条建档族共用的 PIN 判据：格式 + 哈希，一次做完。
// 调用方：createProfile、rebuildProfile。判据只有一份这件事由
// Test建档判据只有一处 钉着。
func hashFormPIN(pin string) (hash string, fe []fieldError)
```

- `createProfile` 与新的 `rebuildProfile`（httpapi 侧的壳）都调它，两处都不再直接出现
  `auth.ValidPinFormat` / `auth.HashPIN`。
- **同轮改那道 AST 闸**（`internal/httpapi/createrule_test.go`）。第二档现在的口径是
  "`auth.ValidPinFormat`/`auth.HashPIN` 在 owner 里必须有、在建档壳里必须为零、`pinapi.go`
  那两个设 PIN handler 是合法的第二批用户"。改完的口径只有两条，逐条钉：
  1. 那两个符号的**合法持有者**恰好三个函数：`hashFormPIN`（定义处）、`handleAdminPin`、
     `handleSelfPin`。多出任何一个都算判据被抄。
  2. `hashFormPIN` 的**定义整包只许一处**，且 `createProfile` 与 `rebuildProfile` 两个壳
     都必须调它（少调一个 = 那条壳自己哈希；多定义一个 = 判据有两份）。
- 招牌枪：在 rebuild 的壳里绕过 `hashFormPIN` 自己 `auth.HashPIN` 一遍 → 闸必须红。
  这一枪不打，这道闸就是新形状的假闸。

## 3. 端点契约（httpapi / openapi / AGENTS）

`POST /api/v1/profiles/rebuild`，注册在 `routes()` 里与 `profiles/pin` 相邻，凭证档
`credAdmin`（`*` 令牌或管理员会话；档案作用域的令牌打它是 403，这是设计不是故障）。

请求体（`DisallowUnknownFields` 照旧——写错一个键整批 422）：

```json
{ "profile": "mom", "title": "妈妈", "pin": "4321" }
```

`by` 走 `X-LB-By` 头，**不进请求体**——与 `/api/v1/revert`、`/api/v1/profiles/pin` 同形
（AGENTS.md 的 `by` 段已经把这三条归在一组）。在体里加一个 `by` 字段会让 `DisallowUnknownFields`
那条纪律出现两种答案。

| 情况 | 答 | Detail / Errors 说什么 |
|---|---|---|
| 没凭证 / 认不出 | 401 | 与 `profiles/pin` 同一份 `writeAuthFailure` |
| 凭证认了但作用域不含该档案 | 403 | 同上，不合并 |
| `profile` 不是 ASCII slug、或 `title`/`admin`/`pin` 任一缺席 | 422 | `errors[]` 逐条点名是哪个字段；**不许**把"没给 admin"当 false |
| `pin` 格式不对 | 422 | `field: "pin"`，"要 4-6 位数字" |
| 档案读得动 | 409 | `detail` 指路：这份档案读得动，改 PIN 请用 `POST /api/v1/profiles/pin` |
| 档案目录不存在（不是半件事，是根本没这份档案） | 404 | 与 `profiles/pin` 同一句"没这个档案"，不建目录 |
| 落盘失败 / 回读仍读不动 | 500 | 内部文本只进 stderr，不回显磁盘路径 |
| 成功 | 200 | `{"profile":"mom","rebuilt":true,"title":"妈妈","admin":false}` |

- 成功响应回显 `title`/`admin` 是有意的：调用方要能确认"我说的那两格真按我说的写了"，
  而不是只拿一个 `ok`。
- 历史行与设 PIN 同形状：`rev:0`（不动书签计数器）、
  `ops:[{op:"set", path:"profile/rebuild"}]`——**不新增 op 词汇**，AGENTS.md 的 `by` 段
  已经写明这类动作记成 `set` + 点名 path。写历史失败按设 PIN 那条裁定 C 报 500
  （这里不学 batch/revert 的"200 + warning"：重建是幂等的，重试收敛而不是叠加）。
- `openapi.json`：一条 operation，`security` 与 `xCredential` 与 `profiles/pin` 同值
  （`bearerAuth|sessionCookie`），`x-status` 写 200/401/403/409/422/500。
- `AGENTS.md`：端点表加一行，说明列必须写清两件事——**这不是设 PIN**（它整份重写
  `profile.json`，`theme`/`layout`/`createdAt` 按默认重建），以及它是坏档的**唯一被支持的出路**
  （比"不许直改 `data/`"那条禁令多给一个正向答案，AI 读到 503 时不该去碰文件）。

## 4. CLI

`cmd/launchbay/cli.go` 的 `profile` 子命令族加一支，与 `profile create` 同形：

```
launchbay profile rebuild --id <slug> --title <名字> --pin <4-6位>
```

- 只经本机 REST（`POST /api/v1/profiles/rebuild`），不直读磁盘——CLI 的既有纪律。
- `--admin` 是开关，但**请求体里 `admin` 必须总是出现**（省略 = 显式 `false`）。这条要配
  接线证人：把 CLI 改成"省略就不发这个字段"，必须红在 422 那一条。
- 令牌从 `LAUNCHBAY_WRITE_TOKENS` 取（与 `profile create` 同一份 config），不打印、不回显。
- 中文名字在这里是安全的：请求体由 Go 编码，绕开 Git Bash 按 GBK 送 body 那个坑。

## 5. 契约同步清单（一处都不许漏，漏了闸会红）

1. `internal/httpapi/routes.go`：注册一行（`credAdmin`）。
2. `internal/httpapi/openapi.json`：operation + `x-status` + `security`；五道 openapi 闸覆盖。
3. `AGENTS.md`：端点表一行；`by` 段那节的 path 词汇出现 `profile/rebuild`（三道 AGENTS 闸覆盖）。
4. `internal/httpapi/createrule_test.go`：§2.2 的闸改造 + 招牌枪。
5. `leakguard`：新文件不含真实地址/令牌；夹具只用 `10.` 段与 `127.0.0.1`。
6. 预算：本片**零新页面形状**——进门页与管理页的读数一个字都不该动。若有人顺手加了 UI，
   预算六条要整组重量、零脚本名单要登记新形状。

## 6. 权威 spec 的改字清单（实施计划的第一个任务做掉）

| 位置 | 改什么 | 为什么 |
|---|---|---|
| §4.5（写通路的串行化） | 加一句：rebuild 的判据（读得动吗）与写入同属一次锁内串行化，handler 不许自己先查一遍再决定 | 与 `CreateFirstProfile` 同一条纪律，不写下来下一个人就会在 handler 里"先看看坏没坏" |
| §6.1 端点表 | 加 `POST /api/v1/profiles/rebuild` 一行 | 表里没这条，AGENTS 的闸会红（AGENTS 写了必须能对上） |
| §6.1 免凭证那句 | **不改**，但在那一段注明"新端点要凭证，免凭证的四条没变" | 那句刚被纠正过（"只有两条"是假话），别再留孪生 |
| §8.8（写动作分级） | 把"整份重写 profile.json"归到"机器通路已可用，页面入口随 ⑤" | ④.5 定下的分级要跟着新能力改口，否则它说"设 PIN/删档在 ⑤"读起来像 rebuild 也在 ⑤ |
| §11（安全表 + 明写不防） | 加一行残余：管理员令牌能整份重写任何档案的 `profile.json`（含把某人降为非管理员）；明写不防——能删档能建档的令牌本来就能做到更少的事，加限流与"改管理员要额外凭证"都属本案不要的严格安全 | 不加这一行，评审会把它当新漏洞报 |
| §12（坏档隔离） | 加一句：`profile.json` 坏掉时唯一被支持的出路是 rebuild；进门页点名 + 一句出路，不给页面按钮 | 让"隔离"这段不再停在"说清问题"而没有"怎么出去" |
| §13（验收） | 新增 P1-④.6 一片，条目见本文 §7 | 验收条要能在权威源里找到 |
| §14（阶段表） | 加 ④.6 这一片（夹在 ④.5 与 ⑤ 之间，纯服务端零 JS） | 与 ④.5 同样：不占 ⑤ 的 JS 预算 |
| §15（运维） | 部署/运维那节写清"坏了怎么办"：`launchbay profile rebuild` 一条命令 | 运维文档里那条"删掉 data/icons/ 重来"是缓存的出路，用户数据的出路今天没有 |

## 7. 测试与验收（每条都要能说出「什么改动会让它红」）

store 层（`internal/store/rebuild_test.go`）：

1. 坏档 rebuild 成功 → `profile.json` 读得动，且 `bookmarks.json`、`snapshots/` 下每个文件、
   `history.jsonl` 的**已有内容**逐字节不变（这条端点会往历史里**追一行**，见 §3——"一个字
   不碰"管的是已写下的那些字节，不是文件长度）。枪：让 rebuild 复用 `CreateProfile` → 这条红（书签被覆盖）。
2. 健康档案 rebuild → `ErrReadable`，且**没有发生任何写**（mtime 与字节都不动）。
   枪：删掉判据 → 红。
3. 半件事目录（有目录、无 `profile.json`）→ 允许 rebuild，且建出来的档案**只多一个
   `profile.json`**（不顺手写 `bookmarks.json`）。枪：把判据写成"只 `ErrCorrupt` 才允许" → 红。
3b. 目录根本不存在 → `ErrNotFound`（404），且**不创建任何目录**。枪：把第 1 步的目录检查
   去掉、让 `MkdirAll` 顺手补 → 红（这条守的是"rebuild 不是第二条建档通路"）。
4. `LoadProfile` 返回非 `ErrCorrupt`/`ErrNotFound` 的错 → 原样上报、**不覆盖**。
   枪：把分支并成"任何错误都当坏档" → 红。
5. 写完回读失败 → 返回错误、坏档记录仍在、不假装成功。枪：删掉回读那一步 → `/status`
   幽灵坏档那条红。
6. 并发：两路 rebuild 同一档案、rebuild 与 `SaveProfile`（设 PIN）交错 → 不产生半写文件，
   后写的赢且文件永远合法 JSON。枪：把 `WithProfileLock` 去掉 → `-race` 与内容断言红。

httpapi 层（`internal/httpapi/rebuild_test.go`）：

7. 401 / 403（档案作用域令牌）/ 422（缺 `admin` 这一格必须点名，不许当 false）/
   409（读得动）/ 500 各一条，形状与 `profiles/pin` 同族。
8. 端到端：建两份档案 → 把其中一份 `profile.json` 写坏 → CLI 或 REST rebuild →
   `GET /` 那张卡回来、`GET /api/v1/data?profile=mom` 读得动、`/status` 的
   `brokenProfiles` 归零、`GET /api/v1/history?profile=mom` 多一行 `rev:0` +
   `path:"profile/rebuild"`。枪：把历史行去掉 → 红；把 `clearBroken` 改成写成功就清 →
   红在回读那一条的证人上。
9. 契约闸：五道 openapi + 三道 AGENTS 全绿（漏登记自动红）。
10. 建档判据闸改造后：在 rebuild 的壳里绕过 `hashFormPIN` 自己哈希 → 红（招牌枪）。

CLI 层（`cmd/launchbay`）：

11. `profile rebuild` 的参数 → 请求体接线（含 `--admin` 省略时发 `false`）；失败时退出码
    非 0 且把服务端 `detail` 原样说出来；成功打印新档案的 `id`/`title`/`admin`。

运维读数（验收当天做一次，不进 `go test`）：

12. 真起一个实例，把唯一档案写坏 → 进门页那句话 + 点名 → 跑一次 `launchbay profile rebuild`
    → 进门页那张卡回来、书签数据与坏掉之前逐字节一致。这一步是这整片存在的理由，
    必须有读数。

## 8. 明确不做

- 不给页面入口、不做零 JS 的确认形状（⑤ 与设 PIN/删档同批）。
- 不从半截 JSON 里抢救 `title`/`theme`/`layout`/`createdAt`（不猜；猜错的样子比默认更难看）。
- 不做"重建 + 从模板灌书签"（那是第二条数据通路；要模板请走 `POST /api/v1/profiles` 建档）。
- 不做"重建时顺便改 id"（id 是目录名，改 id = 搬三份文件，那是另一件事）。
- 不加限流、不加"改管理员要二次凭证"、不做邀请链接（§11 的档位）。
- 不动 `bookmarks.json`/`snapshots/`/`history.jsonl` 的任何写路径。

## 9. 残余风险（明写，不当 bug 报）

- **字段重置**：`theme`/`layout`/`createdAt` 按默认重建。读不出来就没有"保留"这个选项，
  这是能力边界不是缺陷；响应回显 `title`/`admin` 让调用方至少能核对自己说出口的那两格。
- **把唯一管理员 rebuild 成 `admin:false`**：这台盒子会暂时无人能管（`/admin` 不可达）。
  出路是再 rebuild 一次并显式给 `--admin`。为什么不在服务端拒绝这种操作：那需要知道
  "是不是最后一个管理员"，而那是 §2.1 之外的第二份判据。缓解：字段必填 + 文档 + CLI 的
  显式开关。
- **I/O 错与内容错的分诊**依赖 `LoadProfile` 的错误分类；如果哪天 store 把权限错也包成
  `ErrCorrupt`，§2.1 第 1 步的"其它错误不覆盖"就白写。测试 4 是这条的守卫。
- 坏档期间该档案的书签**仍可能被别的路径读到**（`/api/v1/data` 走 `LoadBookmarks`，与
  `profile.json` 无关）——这不是本片引入的，也不归它修。

## 10. 与 ⑤ 的接缝

- ⑤ 给 `/admin` 补"设 PIN / 删档 / 整份重建"三张带二次确认的表单时，rebuild 的表单壳
  直接接这条端点，**不新写判据**：确认的形状归 ⑤，判据归 store。
- ⑤ 若把进门页那句「要在这台机器上修好那个文件」改成"请联系这台导航台的管理员"，
  那是文案层的收敛，不改变本片任何判据。
- ④.5 已闭的两处死路里，①（说不清）由那一片做，②（怎么修）由本片做；⑤ 接手的是
  "让不拿命令行的人也能点"。
