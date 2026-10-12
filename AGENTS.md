# AGENTS.md — launchbay 写通路契约（给 AI 读的）

本文件是给 AI agent 读的操作契约。它由 `internal/httpapi/agents_test.go` 的
`TestAgentsMDMatchesRoutes` 与路由表逐条锁死：**这里写到的每个 `/api/v1/...`
端点都必须真的存在，路由表里新增的端点也必须出现在这里**——文档骗 AI 比骗人
更贵，AI 会照着它生成错的请求。机器可读的完整契约见 `GET /api/v1/openapi.json`。

launchbay 是局域网家庭书签导航台：每人一个档案，各存各的，互不干扰。定位是
方便优先、安全做到"不顺手看到、不误改、不泄进仓库"这一档（spec §11）。

## 令牌与鉴权

**管理员是一把机器级的 key（`LAUNCHBAY_ADMIN_TOKEN`）；档案作用域令牌是另一份
列表（`LAUNCHBAY_WRITE_TOKENS`）。两者从 2026-10-12 起不再混在一起。**

- **管理员：`LAUNCHBAY_ADMIN_TOKEN`**，一把裸 key（**不是** `name:secret:profile`）。
  写进 compose 的 `environment`，空值是启动错误。它不属于任何档案。
  `Authorization: Bearer <key>` 携带。浏览器那条路是 `POST /admin/login`：交一次 key，
  服务端发一条 `lb_admin` cookie，**里面装的是它的 HMAC 派生值，不是 key 本身**
  （原始 key 只在那一次 POST 里出现过——cookie 会进 devtools、进导出、进同步）。
- **`LAUNCHBAY_WRITE_TOKENS` 只放档案作用域令牌**，逗号分隔，每条三段式
  `name:secret:profile`；`Authorization: Bearer <secret>` 携带。
  **档案段写 `*` 是明确报错**（不是当成管理员，也不是静默忽略）：报错里直接指去
  `LAUNCHBAY_ADMIN_TOKEN`。静默忽略会把那一条变成永远匹配不上的死令牌，
  症状是"建档接口永远 403"。
- **作用域决定能写哪个档案**：`profile` 段是一个档案 id 时，这把令牌只能动那
  一个档案。误写别人的档案会变成 403，而不是"写进去"。
- **管理员只管账号，不碰内容**（2026-10-10 改判，2026-10-12 换了形状）。带它能过的
  只有账号层面那一族：`/admin` 系列、`POST /api/v1/profiles`（建档）、
  `DELETE /api/v1/profiles`、`/api/v1/profiles/pin`、`/api/v1/profiles/disable`、
  `GET /api/v1/status`、`GET /api/v1/history`（省略 profile 的全家庭聚合）。
  **凡是带 profile 的内容端点（`/api/v1/data`、`/api/v1/export`、`/api/v1/batch`、
  `/api/v1/revert`、`/api/v1/usage`、`/api/v1/resolve`、`/api/v1/import`）一律 403**
  ——写某个人的书签必须用作用域正好是那个人的令牌（`name:secret:dad`）。CLI 的
  `revert` / `import-bookmarks` 因此逐档案换令牌。
- **管理员 cookie 与档案会话是两件独立的事**（2026-10-12）。管理员可以一边登着自己
  的档案用书签、一边仍带着管理权；反过来，只拿得出 `lb_admin` 而没登任何档案也完全
  正常（那是最常见的管理员样子）。`lb_admin` **永远进不了 credScoped**。
- **档案上不再有 `admin` 属性**（2026-10-12）。"第一个档案自动是管理员"这条规则随之
  消失：管理员与档案无关，于是 `/setup` 建出来的就是第一个人的书签页。相应地
  `POST /api/v1/profiles` 与 `POST /api/v1/profiles/rebuild` 的请求体里那个 `admin`
  **不存在了**——送来它是 422 且点名该字段（脚本作者需要知道该删哪一行）。
  盘上的 `profile.json` 里升级前留下的 `"admin": true` 被 `decodeProfile` **悄悄放过**
  （否则升级就把全机器的档案变成坏档），下一次写盘自然消失。
- **401 与 403 永远分开**：没带凭证 / 令牌认不出 = 401（换正确的凭证，重试同
  一个没意义）；凭证认出来了但作用域不含该档案 = 403（换对档案/对角色，别烧
  死一个可用令牌）。管理员那把 key 与管理员 cookie 都算"认出来了"，所以打错门
  （例如只认会话的 `/api/v1/profiles/self/pin`）是 **403 而不是 401**——说成"令牌无效"
  等于告诉人一把好钥匙是废的。
- 浏览器通路的档案会话是 cookie（`lb_profile`），不是令牌；bookmarklet 与页面浮层
  零令牌。令牌只发给 AI（REST）、扩展和 CLI。
- **`lb_home` 是升级前那条 cookie**（2026-10-10 代管态下线之后服务端不再发它）。
  读侧仍然认它，且只为一个目的：**认出升级前发出去的代管 cookie 并判它非法**。
  两条相等 = 升级前的本人会话，**照旧有效，升级不要求任何人重新输 PIN**；
  两条不等 = 旧代管会话，那枚 cookie 自包含（读的人只看到 `lb_profile`），判非法、
  清 cookie 回选择页。读侧要是干脆不认它，那枚 cookie 就重新变成能改别人书签的后门。

## 端点清单

除 `/`、`/pin`、`/healthz` 三条浏览器通路外，每一条都要列在这里。方法、要哪
种凭证、成功响应都对照 `openapi.json`。

**`/admin`、`/setup`、`/prefs`、`/switch` 与 `/revert` 是浏览器表单/导航通路，AI 不走这五条**：其中 `/setup`、`/prefs`、`/switch`、`/revert`
与 `POST /admin` 只由页面里的 `<form method=post>` 走进来，收 `application/x-www-form-urlencoded`，没有 JSON 版，
也没有 `by` 字段；`/admin` 是 GET 的 HTML 管理页，`*` 令牌打它拿到的也是这张页面而不是
另一份 JSON——这不是第二条 API 面，要机器可读的同一份跨档案聚合请打
`GET /api/v1/status`。`/setup` 是**零档案时的一次性通路**（判据是磁盘上 `profiles/` 的目录数，
不是"谁有资格"），它同样是浏览器表单通路而不是第二条 API 面，脚本别去 POST 它——
有档案之后它只会 303。浏览器通路的**权限类**拒绝答 303 回档案选择页而不是 401/403
（spec §6.1）：`GET /admin`、`/prefs`、`/switch`、`/admin/pin`、`/admin/delete`、`/revert` 与
`POST /admin`、`/admin/pin`、`/admin/delete`、`/revert` 的权限类（无会话、非管理员、
带 `*` 令牌）都是**不带提示**的 303；只有**字段没填对**才带 `?err=<码>`——`/pin` 说
`pin`/`missing`/`profile`/`disabled`，`/setup` 与 `POST /admin` 用建档那几种码
（`noname`、`badpin`、`permission`，管理页还多一个 `nametaken`）；`/admin/pin` 与 `/admin/delete` 说`badpin`/`notfound`/`selfpin`/`self`/`failed`，`/revert` 说`badsnap`/`gone`/`nosnap`。都不是 JSON 错误；`/setup` 的"窗口已关"是例外，它回 `/` 且**不带**键，
因为"这台机器已经建过档了"本身就是那条枚举线索（同 `/switch` 的判据口径）。`/prefs` 的形状错
（422）与文件读不了（503）仍然走 JSON 外壳，它们不是权限拒绝，`/setup` 的表单读不出来
（超上限、同名多值）与落盘失败（500）同这一族——**只有 `permission` 一个例外**（见下）。`/switch` 连这两种都没有——它每条出路
都是 303，无会话、无权、目标不存在、目标读不了这四种拒说同一句话，因为"这个 id
到底有没有"本身就是那条枚举线索。

`permission` 是**唯一一种说得出出路的落盘失败**，两张建档表单才有：宿主上的数据目录
不归跑服务的那个用户（NAS 上把 `./data` 挂成 root 所属、容器以 uid 65534 跑，装机的
头一个撞的就是它），于是给一句"去宿主机 chown"而不是 500 JSON。判据是
`errors.Is(err, fs.ErrPermission)` 这一个哨兵，**磁盘满与只读文件系统不算**（那句话教
的是 chown，chown 治不了被挂成 ro 的卷，教错出路比不说更坏），它们留在 500 里。
`POST /api/v1/profiles` 那条机器通路也不折成这句话——它回 500，调用方读 stderr 与容器日志。

| 方法 | 路径 | 凭证 | 成功响应 | 说明 |
|---|---|---|---|---|
| GET | `/app.4241357415b2.js` | 无（公开） | 200 脚本字节 + `Cache-Control: public, max-age=31536000, immutable` | P1-⑤ 的应用脚本：顶栏「＋ 添加」与它的浮层（贴 URL → `POST /api/v1/resolve` 填标题/描述/建议分组/候选图标 → 回车经 `POST /api/v1/batch` 入库），并注册 ⑥ 的 Service Worker。零依赖 ES module，**只有档案首屏引它**，别的页面继续零脚本。**路径里那 12 位是内容指纹**（sha256 前 12 位），只在 `internal/render/web/app.js` 改动时变 —— 指纹由 `internal/render/assets.go` 在启动时从 embed 的字节算出，**不需要构建步骤**。改那个文件后本行路径要一起更新（`Test静态资产指纹三处一致` 会当场指出该填哪个） |
| GET | `/sw.js` | 无（公开） | 200 脚本字节 + `Cache-Control: no-cache` | P1-⑥ 的 Service Worker，**路径不带指纹**（浏览器自己按注册处比字节决定更新）。**必须在根路径**：SW 的作用域是所在目录，挪到 `/assets/` 下就管不到首页。HTML 走 network-first + 1.5 秒超时回退缓存；按响应头 `X-LB-Profile` 分桶存 `html-<id>` 与 `html-picker`，**`-admin` 一律不存**（它含全家庭条目数与 rev，比选择页敏感一档），空头也不存。回退时按 `[currentProfile, picker]` 的顺序查桶，命中即交，**查不到就是失败**（不是"随便拿一份"）。切档时清掉上一个档案的桶——`app.js` 把 `/switch` 表单的 `to=picker` 转成空串发过来，所以 `sw.js` 那一支只删桶、**不写 `currentProfile`**；于是它可能仍指向一个刚被删掉的档案，回退查不到就落到 `html-picker`，与 §15 那条"删档后残留"的边界是同一族 |
| GET | `/app.06064612355c.css` | 无（公开） | 200 样式表字节 + `Cache-Control: public, max-age=31536000, immutable` | P1-⑤/⑥ 的**交互**样式（浮层、⌘K 面板、就地编辑、拖拽把手）。走**外部**指纹文件而不内联：首屏骨架在 `base.css` 里内联，而这部分只有档案页用得到——内联等于让选择页/引导页/管理页/坏档页各白带一份，而且会把内联 CSS 顶出 §7.3 的操作线。**路径里那 12 位是内容指纹**，只在 `internal/render/web/app.css` 改动时变 |
| GET | `/bookmarklet` | 无（公开） | 200 JS 源码（no-store） | P1-⑥ 的 bookmarklet：拖进书签栏，在任何网站上一点就把当前地址带回这台机器。**里面绝不会有令牌**——Chrome 书签会云同步，写进 bookmarklet 的令牌等于明文外发；它只做一件事，导航到 `/?add=<当前URL>&t=<标题>`，写走会话 cookie（§6.3）。**它自己从 Host 头算这台机器的地址**，不让人填（填错的那种错要等到用时才发现）。未登录时 `?add=` 穿过 PIN 步骤不丢；登录后 ⑤ 的浮层自动打开预填，**要人再按一次保存才落库** |
| GET | `/api/v1/openapi.json` | 无（公开） | 200 内嵌契约原字节 | 机器可读契约，无个人信息 |
| GET | `/api/v1/data` | **档案作用域**令牌/本人会话 | 200 `{profile, rev, modified, data}` | 单档案全量数据 + rev，`?profile=<id>` 必填；**`*` 令牌与管理员会话都 403**（管理员看不见别人的书签） |
| GET | `/api/v1/export` | **档案作用域**令牌/本人会话 | 200 磁盘原字节 + attachment | 导出不重编码，可直接喂 batch 对照；同样只给本人（`*` 403） |
| GET | `/api/v1/history` | 令牌/会话 | 200 `{entries:[{at,profile,actor,by,rev,ops}]}` | 审计线，ops 只有动作与路径无 value；省略 `profile` = 全家庭聚合，仅管理员 |
| GET | `/api/v1/status` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{profiles[],brokenProfiles[],totals}` | 跨档案 rev/条目/快照/坏档汇总，含 `iconUnmatched`（读时算的缺图数：陌生图标名 + `auto` 且缓存无图，空 `icon` 不算缺，口径见 openapi 的 `/status`） |
| GET | `/api/v1/profiles` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{profiles:[{id,title,admin,createdAt}]}` | 档案元信息列表，零条目零 URL |
| POST | `/api/v1/profiles` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 201 `{id,title,admin}` | 建档；**不带任何预置内容**，每个档案从零开始；已存在 409 |
| DELETE | `/api/v1/profiles` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{deleted}` | 删档连 `snapshots/`，历史保留，`?profile=<id>` |
| POST | `/api/v1/profiles/pin` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{profile,pinChanged}` | 给任意档案设 PIN（不要旧 PIN），进目标档案历史 `rev:0` |
| POST | `/api/v1/profiles/rebuild` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{id,title,admin}` | **坏档的唯一出路**：某档案 `profile.json` 读不动时只重写这一个文件，书签、快照、历史已有内容**逐字节不碰**。体 `{profile,title,pin,admin}` **四格都必须出现**（`admin` 省略 = 422，不等于 false：重写整份文件时"没给"与"给了 false"是两件事）。判据全在 store 锁内：**读得动 → 409**（改 PIN 走 `/api/v1/profiles/pin`，这条不是改档用的）；**目录不在 → 404**（它不建目录，建档仍只有 `POST /api/v1/profiles`）；只有"内容错"与"文件不在"才允许覆盖，权限/磁盘那一类读失败原样上报不许盖。`theme`/`layout`/`createdAt` 按默认重建（读不出来就没有"保留"这个选项）。200 意味着"进门页那张卡真的回来了"——写完在同一把锁内回读过。历史追一行 `rev:0`、`path:"profile/rebuild"`；审计行写不下去仍报 200（**这条动作不幂等**，重试会拿 409，报 500 等于对已修好的档案说"没修好"），缺口只进 stderr |
| POST | `/api/v1/profiles/self/pin` | 仅会话 | 200 `{profile,pinChanged}` | 改自己 PIN，要旧 PIN；令牌一律拒 |
| POST | `/api/v1/batch` | **档案作用域**令牌/本人会话 | 200 `{profile,rev,applied,warnings}` | 唯一的书签写通路，一批 ops 原子写入；**`*` 令牌与管理员会话都 403** |
| POST | `/api/v1/revert` | 令牌/会话 | 200 `{profile,rev,restoredFrom}` | 从快照恢复，产生新 rev，不抹历史 |
| POST | `/api/v1/usage` | 令牌/会话 | 200 `{updated, unknown[]}` | 点击计数，只写当前档案，不参与 rev |
| GET | `/api/v1/icons` | 无（公开） | 200 `{query, total, returned, icons[]}` | 本地索引搜图标名；`q` 必填，`prefix` ∈ tabler/simple-icons，`limit` ≤200 |
| GET | `/icons/{name}` | 无（公开） | 200 图片原字节 + `Cache-Control: public, max-age=31536000, immutable` | resolve 抓来的 favicon 缓存，名字必须是 `<16 位小写十六进制>.<png/ico/svg/webp/gif>`；形状不对与没这个文件都是 404，同一句话 |
| POST | `/api/v1/resolve` | 令牌/会话 | 200 `{url,title,desc,suggestedGroup,iconCandidates[],favicon}` | URL → 书签候选；体只有 `{url, profile}`（`by` 是未知字段 → 422），`profile` 空申报 = 403；不写书签/历史/rev，只在 `data/icons/` 落缓存 |
| POST | `/setup` | 无（判据是磁盘：`profiles/` 下目录数为 0 才有效） | 303 回 `/` | 引导建档，浏览器表单通路：**AI 不走这条，也别拿脚本去打它**。体是 `application/x-www-form-urlencoded` 的 `{title, pin, language?}`，不问 id（由名字派生 slug，中文走 `home`、`home-2` 基名序列）；**建出来的是第一个人的书签页，不是管理员**（2026-10-12：管理员是机器级的一把 key，与档案无关，所以"零档案装完没人能加人"那个老问题也不再成立）；成功**不发会话**（进门照样要 PIN），字段没填对回 `/?err=noname`或`badpin`，**宿主数据目录不可写**回 `/?err=permission`（NAS 上把 `./data` 挂成 root 所属、容器以 uid 65534 跑那个形状，去宿主机 chown 即可），窗口已关（已有档案）回 `/` 且不带 `err`——「这台机器已经建过档了」本身就是枚举线索。AI 要建档仍只有 `POST /api/v1/profiles`（带 `id`，**不带任何起步内容**，模板机制 2026-10-10 整条拆掉了，`fromTemplate` 与 `data/templates/` 都不存在）那一条 |
| POST | `/prefs` | 仅会话（表单） | 303 回 `/` | 改自己的主题与档案形态；`theme` ∈ `auto`、`dark`、`light`；`layout` ∈ `sidebar`、`topbar`，两个字段都不给 = 422；令牌一律拒 |
| POST | `/switch` | 仅会话（表单） | 303 回 `/` | 会话动作，2026-10-10 起只剩 `to=picker`（清两条 cookie 回选择页）。**`to=<档案 id>` 的免 PIN 直达已整条下线**——它是管理员进别人台改书签的唯一入口；`to=home`（原"退出管理"）降级为幂等空操作，因为代管态已经不存在、两条 cookie 不再可能分家。别的 `to`（含 `../` 探针、含曾经合法的档案 id）一律什么都不做地回选择页，与"没这个档案""你不是管理员"同一个回答；不动书签，不写历史行也不产生 rev |
| GET | `/import` | 仅会话 | 200 HTML（no-store） | 浏览器收藏夹导入**第一步：选文件**。只动**自己的**档案，且档案必须一条书签都没有（与 CLI 的 `import-bookmarks` 同一规矩：真要合并得先有一套对着既有档案做对账的判据，那不是"多一个入口"该顺带发明的）。档案非空时**直接在这一页把话说清**、不 303 带 `?err=`——登录着的会话回到 `/` 渲染的是**档案页**，那把键会被无声吞掉（`Test提示键只发白名单里有的` 正是防这个的） |
| POST | `/import` | 仅会话（`multipart/form-data`） | 200 HTML（no-store） | **第二步：收文件、解析、给报告，不写盘**。体是 multipart 的 `bookmarks` 文件字段，上限 8 MB（`maxBodyBytes` 那 1 MiB 是给 JSON 表单的，浏览器导出动辄上兆）。**读出 0 条必须说清"这不是书签文件"**——`Convert` 不拒绝非 Netscape 输入，喂一段随便的 HTML 它解析出零条并照常返回，不判就会"导入成功 0 条"。**进不来的条目是"跳过 + 逐条点名"，不是整批失败**：判据是 `model.CheckURL` 那一份（地址要有主机名；`javascript:`、`data:`、`file:` 没有，所以进不来，而 `chrome://extensions/` 有主机名、按 §6.2 原样保留），报告里"## 没能进来"一节逐条写地址与理由，筛空的文件夹不建但要点名。写路径的"整批拒绝"没有改动——那是给 AI 与 CLI 的纪律（一次改 30 条时"改坏一半"比"全没改"难收拾），导入这条路**先把坏条目挑出去**再生成 ops。整份文件一条都进不来是**另一个形状**（读出了书签但没一条能进来），报 422 并说清原因，不说成"文件里没书签"（那会把人引去换一个同样导不进来的文件）。报告与一枚**一次性令牌**一起发下（绑档案、15 分钟过期、总量 64 份、用过即删） |
| POST | `/import/apply` | 仅会话（表单） | 303 回 `/` | **第三步：确认写入**。体是 `{token}`；令牌取走即删，所以重放也写不进第二次——这比"确认后用 JS 锁按钮"可靠且零脚本。ops 由 `bookimport.Ops` 生成，落盘走 `applyBatch`——与 `POST /api/v1/batch` **同一个落点**，快照/历史/rev 一次不落。`by` 记 `import` |
| GET | `/revert` | 仅会话（表单；带令牌一律 303 拒） | 200 HTML（no-store） | 回退到某一版的**第一步**：把可回退的快照逐个列出来，每格带**条目数与分组名**——只印版本号的列表回答不了"我要退哪一版"。**两步二次确认、零 JavaScript**（与 `/admin/pin`、`/admin/delete` 同款范式）。**目标不接受申报**：档案从会话推导，所以 cred 是 credSession 而不是 credScoped——管理员进得了这一页也只能退自己那一份（2026-10-10 边界：管理员管账号、不碰内容，即使 PIN 是他设的）。`bookmarks.json` 读坏了（坏档）**不挡**这一页：revert 正是那种时候的出路。读不出来的快照不列但**报数**（静默少列会让人以为"只有这几个版本"）；一份都没有时只讲清为什么、**表单整个不画** |
| POST | `/revert` | 仅会话（表单；带令牌一律 303 拒） | 303 回 `/` | 回退的**第二步**，体是 `application/x-www-form-urlencoded` 的 `{to-rev}`。**不是第二条 API 面**：落点与 `POST /api/v1/revert` 是同一个 `revertLocked`（快照→写盘→审计，判据只有一份），差别只在怎么读入、怎么答出。成功 303 回 `/`（GET `/` 在有会话时渲染的就是档案页）；`to-rev` 没填/非整数/为负 → 303 `/revert?err=badsnap`；那一版的快照已经不在（被别人退掉，或被 50 份的裁剪清掉）→ `?err=gone`；当前这一版存不下所以**本次恢复已取消、书签一个字都没变** → `?err=nosnap`。落盘失败走 500 JSON 外壳，**绝不说成功**。历史里 `by` 记 `web` |
| GET | `/admin` | 管理员（`LAUNCHBAY_ADMIN_TOKEN` 令牌或 `lb_admin` cookie） | 200 HTML（no-store） | 管理页读侧：一行一个档案 + 条目数 + rev + 最近改动（`modified`，RFC3339 UTC，坏档读不出来为空、显示 `—` 不编造），**不含条目内容**；坏档案照占一行、标「数据文件读不了」、不给「进入编辑」；④.5 起这一页带一张建档表单（读侧 + 建档）。动作格只有**账号层面**三枚：**设 PIN**、**停用/启用**、**删档**——**「进入编辑」已下线**（它就是 `POST /switch to=<id>`）；坏档那一格是文字「要在这台机器上修好那个文件」，停用档带「已停用」徽标。**没有管理权时不是 303，而是 200 渲染登录页**（2026-10-12：管理员不再是某个档案，浏览器那条路需要自己的凭证；沿用旧的裸 303 形状会让升级之后没有任何人进得去管理页，账号层面整个锁死）。**只有档案会话不算管理权**——登进 dad 也不等于能管这台机器。**AI 不走这条**，要同一份聚合打 `GET /api/v1/status`（同一个 statusRows、两种壳） |
| POST | `/admin/login` | 无（这一条就是"证明你有管理权"的那一步） | 303 回 `/admin` | **管理员的进门**（2026-10-12）。浏览器表单通路，体是 `application/x-www-form-urlencoded` 的 `{key}`。判据只有 `LAUNCHBAY_ADMIN_TOKEN` 一把：`admin:LAUNCHBAY_ADMIN_TOKEN:*`、`POST /prefs`、`POST /switch` 与 `/api/v1/profiles/self/pin` 同族纪律——**浏览器表单一律 303 拒令牌**，机器建档走 `POST /api/v1/profiles`。key 不对 → 303 `/admin?err=badkey`（不说"这把 key 是管理员的那把"，那等于漏出它的身份）；同一个字段出现两次或表单读不出来 → 422 JSON 外壳。对了 → 发一条 `lb_admin` cookie（**装的是 HMAC 派生值**）并 303 回 `/admin`。表单读不出来与 key 不对是两个形状，别合成一个 303——那会让人以为 key 写错了 |
| POST | `/admin/logout` | 无 | 303 回 `/` | 丢掉管理员身份（2026-10-12）。**只清 `lb_admin`，不动 `lb_profile`**——管理员完全可能同时登着自己的档案在用书签，"退出管理"不该把他从自己的台子里踢出去，那两件事从此分不开 |
| POST | `/admin` | 管理员**cookie**（表单；带令牌一律 303 拒） | 303 回 `/admin` | 管理页建档，体是 `application/x-www-form-urlencoded` 的 `{title, pin}`：**没有 `admin` 那一格**（2026-10-12，档案不再有管理员属性，勾了也不会有任何后果）；不问 id（由名字派生 slug，中文走 `home`、`home-2` 基名序列）；字段没填对回 `/admin?err=noname`、`badpin` 或 `nametaken`（派生 id 撞并发提交那一种），**数据目录不可写**回 `/admin?err=permission`（与 `/setup` 同一族同一句话），与进门页同一张白名单，不回显原文，无管理权都是 303 回选择页且磁盘不变；**AI 要建档走 `POST /api/v1/profiles`**，别打这条 |
| GET | `/admin/pin` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 200 HTML（no-store） | 给某个档案设 PIN 的**第一步**：读 `?profile=<id>`，回一张写明后果的确认页。**两步二次确认、零 JavaScript** —— spec §8.8 原写「表单发不出，随 ⑤ 的 JS 落地」，2026-10-09 改判为用两步页面做，于是关掉 JS 时照样成立。目标就是当前会话正在用的那一份 → 303 `/admin?err=selfpin`（改自己的 PIN 要旧 PIN，走 `POST /api/v1/profiles/self/pin`；这条要让开，否则它就是绕过旧 PIN 的后门）；档案不存在 → 303 `/admin?err=notfound` |
| POST | `/admin/pin` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 303 回 `/admin` | 设 PIN 的**第二步**，体是 `application/x-www-form-urlencoded` 的 `{profile, new}`。**不是第二条 API 面**：校验→哈希→锁内读写→落审计行与 `POST /api/v1/profiles/pin` 走同一个落点，差别只在怎么读入、怎么答出。成功 303 回 `/admin`；字段没填对 303 回 `?err=badpin`；锁内读失败 404/503/500；落盘或审计失败 303 回 `?err=failed`（**绝不说成功** —— 那时 PIN 已改而审计行没落） |
| GET | `/admin/disable` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 200 HTML（no-store） | 停用/启用的确认页（第一步）。**停用不是删档**：数据与快照一个字节不动，仍占 `profiles/` 的目录数（引导窗口不重开），但输对 PIN 也进不去、选择页上不再出现这一份。目标就是当前会话正在用的那一份 → 303 `/admin?err=self`（防手滑）；不存在 → `?err=notfound`；坏档 → `?err=notfound`（改不了 profile.json 就翻不动这个标志） |
| POST | `/admin/disable` | 仅管理员**会话**（表单） | 303 回 `/admin` | 停用/启用的第二步，体是 `{profile}`。判据=取反（今天是停用就启用，反之亦然），落点与 `POST /api/v1/profiles/disable` 同一个 `adminDisable`。**幂等**：同状态再设一次不写盘、不记审计，所以重试安全。落盘或审计失败走 500 JSON 外壳，**绝不说成功** |
| POST | `/admin/language` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 303 回 `/admin` | 改全机**界面语言**（多语言，2026-10-11）。体是 `{language}`，取值 `zh`\|`en`。**语言是站级设置不是档案字段**：落 `data/settings.json`，不进 `profile.json`、不走 `/prefs`——那两条都只改自己，而语言改了立刻影响这台机器上所有人；把语言放进档案等于把"改全机显示"的权力交给任一份档案的所有者。没给 → 303 `?err=nolang`；给了不认识的值 → 303 `?err=badlang`（**不静默落默认**：静默存下去等于"我选了法语，它没告诉我没生效"）；落盘失败 500 JSON 外壳，**绝不说成功**。渲染缓存**不用手动清**：`cacheKey` 带语言这一维，改完下一次渲染的键就变了。另有一条入口是零档案时的 `POST /setup`（建档那趟顺手定下语言） |
| POST | `/api/v1/profiles/disable` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{profile, disabled}` | 停用/启用某档案（机器那一面；JSON 体 `{profile, disabled}`）。**`disabled: true` = 停用**（进不去、数据不动、仍占目录数），`false` = 启用。只管账号，与 `*` 令牌同族；写书签一律要档案作用域令牌 |
| POST | `/api/v1/import` | **档案作用域**令牌 | 200 `{profile,count,skipped,groups,report,token}` | **给浏览器扩展用的第一步**：读得到浏览器的整棵书签树，读不到会话 cookie，所以只能带令牌来。体是**原始 Netscape HTML**（`Content-Type: text/html`），不是 multipart——扩展在浏览器里就把树拼成了那个格式。**目标档案从令牌作用域推导、不接受申报**：一枚 `name:secret:dad` 的令牌只能导进 dad，这不是"懒得校验"而是更强的形状——申报字段根本不存在，"令牌与档案对不上"在协议层就无法表达。`*` 令牌一律 403（它只管账号）。**不写盘**，写入在 `/api/v1/import/apply`；读出 0 条当场 422（`Convert` 不拒绝非 Netscape 输入，不判就会出现"导入成功 0 条"）。`skipped` 是**没能进来的条数**（过不了 `model.CheckURL`：地址没有主机名），逐条明细在 `report` 的"## 没能进来"一节——扩展只显示一行结论，这个数就是它唯一能说的话 |
| POST | `/api/v1/import/apply` | **档案作用域**令牌 | 200 `{profile,rev,applied,warnings}` | **第二步：确认写入**。体是 `{token}`，令牌取走即删所以重放写不进第二次；落盘走 `applyBatch`——与 `POST /api/v1/batch` **同一个落点**。令牌过期或已用过是 **410 Gone**（资源确实没了，且正是"别拿同一枚重试"的信号；报成 400 会诱使调用方再发一次）。审计线 `by` 记 `extension` |
| GET | `/admin/delete` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 200 HTML（no-store） | 删档的**第一步**：读 `?profile=<id>`，回一张**把后果写死在页面里**的确认页（条目数与快照数取自 `statusRows`，与 `/admin` 那张表同一趟遍历）。删档不可逆，`revert` 救不了它——快照跟着档案一起没了。两道形状闸判据是"这台机器之后还能不能管"：目标是当前会话在用的那一份 → 303 `?err=self`；**没有"最后一个管理员"那一道**——权限只看 `lb_profile`，能进这页的人本身就是管理员，"唯一的管理员"只可能是他自己在用那一份（已被 `self` 拦） |
| POST | `/admin/delete` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 303 回 `/admin` | 删档的**第二步**，体是 `application/x-www-form-urlencoded` 的 `{profile}`。**不是第二条删档通路**：ASCII slug / 目录须是 `profiles/` 下的同名直接子目录 / 档案写锁三道闸全在 `store.DeleteProfile` 内部，端点层不再"规范化" id（那等于绕过闸）。`history.jsonl` 的行保留（删档不抹审计线）。成功 303 回 `/admin`；不存在 303 `?err=notfound`；**落盘失败走 500 JSON 外壳** —— 删档失败必须说得清 |

## `by` 渠道（自报字段，只进历史审计线）

所有写请求体里的 `by`，以及 `/api/v1/revert`、`/api/v1/profiles/pin` 的
`X-LB-By` 头，取值 ∈ `ai | bookmarklet | extension | web | import | cli`
（共七个）。它是调用方自报的口径，**不参与任何权限判定**；非法值按 `web` 记。
`revert` 与改 PIN 这类动作在历史里记成 `ops:[{op:"revert"|"set", path:"snapshots/<rev>"|"profile/pin"}]`，
`rev:0` 表示这次动作不产生书签 rev。

## 界面语言（多语言，2026-10-11 起）

界面有中文与英文两种，**站级**一个：落 `data/settings.json`（`{"language":"zh|en"}`），
**缺文件 = 中文**，所以升级不改变任何人的现有界面。读侧是 `store.Language()`，
它**永不返回错误**——读不出来（文件不存在 / 权限 / 内容坏了）都落中文，一个显示配置
读不出来不该让整台机器的页面 500。

**为什么不进 profile.json**：语言是一家人共用的阅读偏好。放进档案等于把「改全机
显示」的权力交给任何一份档案的所有者，那既是权限模型的破口，也让语言从显示设置
变成需要仲裁的状态。`/prefs` 也管不到它——那条只改自己。

写入口只有两个：零档案时 `POST /setup` 建档顺手选（那一页只在没有任何人时出现，
所以「谁该替所有人选语言」这个问题还不存在），之后一律 `POST /admin/language`
（**仅管理员会话，带令牌一律 303 拒**）。改完**立刻生效**：渲染缓存的 `cacheKey`
带语言这一维，所以不需要手工清缓存——「改完遍历清一遍」是要记得做的事，
进键是不用记得的事。

### 给写代码的 AI：三条纪律

1. **文案只有一份**，在 `internal/i18n/messages/{zh,en}.json`。**模板里不许出现
   `{{if eq .Lang "en"}}` 这类语言分支**——那会让「这一页有几种说法」从语言包里
   的一行变成散落在标记里的一串判断。模板的唯一形状是 `{{.T "key"}}`（或
   `{{$.T "key"}}`，在 `range` 里）。
2. **带运行时值的句子在 Go 侧查表**（`Server.t`），因为模板拿不到"这一页刚算出来的
   数字"。所以 `AdminAction` 这类结构体里放的**已经是翻好的句子**，不是 key。
   与此配套：`snapshotChoice.choiceLabel` 与 `importOutcomeText` 都是 **Server 的
   方法**而不是包级函数——包级函数读不到站级设置。
3. **app.js 里那一份是拷贝，由测试守住**（`appi18n_test.go`）。它跑在浏览器里，
   够不到服务端的 catalog，所以 `app.*` 前缀的文案在那儿另有一份。加一条 app 文案
   的动作是：**只在语言包里加**，然后跑 `go test ./internal/render`；改 app.js 之后
   **必须同步 `openapi.json` 与本文档里的 12 位指纹**（`Test静态资产指纹三处一致`
   会当场指出该填哪个）。

**翻的与不翻的**：界面 chrome 翻（模板、`?err=` 提示、确认页与导入页的整段说明、
app.js 的浮层与命令面板）。**不翻**的是给脚本读的那些——`/api/v1/*` 的错误 detail、
字段校验失败的原因、导入的对账报告、OpenAPI 的端点描述——它们固定中文，因为调用方
要的是稳定可对照的文本。缺 key 时 `T` 返回包着尖括号的 key 自己（不是空串），所以
漏翻在页面上是看得见的，而空白看着像设计如此。

## JSON 写出契约（spec §4.3）

磁盘上每一个 JSON（`bookmarks.json`、`profile.json`、历史行）都由
`internal/store/encode.go` 的 `WriteJSON` 统一编码，规则固定为：

- **缩进 2 空格**，文件末尾**恰好一个换行**。
- **`SetEscapeHTML(false)`**：`<` `>` `&` 原样写、不转成 `\u003c`——书签标题里的
  `&` 一多，每次写都会污染 diff。
- **非 ASCII 原样输出**：中文不转 `\uXXXX`。
- **空数组写 `[]`、绝不写 `null`**：写出前 `SaveBookmarks` 会先 `normalize` 把
  nil 切片归一成 `[]`（`WriteJSON` 本身不区分 nil 与空，归一化由写出入口兜底）。
- **键顺序由 struct 字段声明顺序固定，绝不靠 `map` 遍历**：Go 的 map 顺序随机，
  会把一次小改动烤成整片乱跳的 diff。

对 AI 读者的意思：**你写成 JSON 的任何东西——测试夹具、diff 里的期望值、
任何要跟磁盘文件对照的内容——都必须照这个形状**，否则跟真实磁盘文件对不上、每次
比对都是噪声。而磁盘文件本身由服务端在每次书签写入
（`POST /api/v1/batch` → `SaveBookmarks` → `WriteFileJSON`）强制成这个形状，
你不用、也不能自己手写磁盘 JSON 去绕过它。

## 五条必踩的坑（都实测踩过或验收抓到）

1. **令牌作用域决定能写哪个档案。** 从 `LAUNCHBAY_WRITE_TOKENS` 拿三段式令牌，
   作用域是具体档案的令牌拿 `/api/v1/status`、`/api/v1/profiles*` 这类跨档案端点
   会是 **403，这是设计不是故障**（spec §6.4）。查单个档案请用
   `GET /api/v1/data?profile=<id>`。

   ```bash
   B=http://localhost:7600
   curl -s -H "Authorization: Bearer $TOKEN" "$B/api/v1/data?profile=dad"
   ```

2. **改完必须回到 `GET /api/v1/data` 复核**，别信 batch 的 200 就是"我想要的形
   状"。batch 返回的 `rev` 和 `applied` 只说明"写进去了几条"，不保证内容是你以为
   的那个。

   ```bash
   curl -s -H "Authorization: Bearer $TOKEN" "$B/api/v1/data?profile=dad" | head -c 400
   ```

3. **`search` / `weather` / `clock` / `title` 这四个键是「保留但不生效」**：没有任何
   渲染消费者读它们，也不再参与校验。写错它们**不会**让档案进坏档页（2026-10-10 改判）。
   历史：本案部署验收曾踩过 `"default":"bing"` 而 `engines[].prefix="b"`，那时校验判
   default 不在合法前缀集里，**全档案读不了**——一个界面上根本不存在的功能拥有毁掉
   一个人全部书签的权力，这条已经拿掉（`internal/model/check.go` 里写着理由）。
   也**别把它们从 model 里删**：解码开了 `DisallowUnknownFields`，而每份档案磁盘上
   都带着这几个键（旧版 `EmptyBookmarks` 一直在写），删字段等于让所有老文件在升级后
   变坏档。要改书签行为，改的是有渲染消费者的那些字段（`groups`/`items`/`footer`/
   `openInNewTab`）——**其中 `footer` 与 `openInNewTab` 虽有渲染消费者却没有任何写
   通路**（batch 的路径一律以 `groups` 开头，`/prefs` 只管 `profile.json` 的
   theme/layout）。要动它们得手工改 `bookmarks.json`，那是被禁止的；这一格目前
   就是死的，知道比撞 422 强。

   ```json
   "search": { "enabled": false, "default": "", "engines": [] }
   ```

4. **请求体必须是合法 UTF-8。** 服务端对非法字节直接 422 拒绝，而不是让
   `encoding/json` 静默替换成 U+FFFD 存进盘。从 Git Bash 用 `curl -d '中文'`
   会把中文按 GBK 送出去——**发带中文的 body 一定要从文件发**
   （`--data-binary @body.json`，用 UTF-8 工具写文件）。

5. **一次改一批用 `POST /api/v1/batch` 带 `expectedRev`。** 409 会回
   `currentRev`，重读后再试；**不许**把 `expectedRev` 删掉来绕过冲突检查。
   失败的批次什么都不留（全成或全败），快照写不下去则 507 取消整次写入。

   请求体形状（字段名逐一对照 `internal/ops/ops.go` 与 `internal/httpapi/batch.go`：
   顶层 `profile`/`expectedRev`/`by`/`ops`，每条 op 只有 `op`/`path`/`value`/
   `index`/`to`；解码开了 `DisallowUnknownFields`，写错一个键整批 422）：

   ```json
   {
     "profile": "dad",
     "expectedRev": 12,
     "by": "ai",
     "ops": [
       { "op": "set", "path": "groups/nas/items/dsm/name", "value": "群晖 DSM" },
       { "op": "add", "path": "groups/nas/items", "index": 0,
         "value": { "id": "portainer", "name": "Portainer",
           "url": "http://10.0.0.9:9443/", "icon": "simple-icons:portainer",
           "alias": ["pt"], "tags": ["容器"], "aiNote": "" } }
     ]
   }
   ```

   `op` ∈ `add|remove|set|move`；`move` 用 `to`（目标分组 id）而非 `value`；
   `add`/`set` 才带 `value`，`index` 可选（`add`/`move` 的落位）。地址按 §4.4
   只用 `id` 寻址，不用数组下标。

   **`set` 的路径必须以 `groups` 开头**——这是 ops 引擎的第一道闸。档案级的
   `title` / `footer` / `openInNewTab` / `clock` / `search` / `weather` **走 batch
   改不了**，打上去会被拒成「路径必须以 groups 开头」（2026-10-10 实测逐条探过，
   六条全拒）。所以 batch 能改的只有书签树本身。

   ```json
   { "op": "set", "path": "groups/nas/title", "value": "自建服务（改名）" }
   { "op": "set", "path": "groups/nas/items/dsm/name", "value": "群晖 DSM" }
   ```

   - **条目能 `set` 的字段只有七个**：`name` / `url` / `icon` / `desc` / `alias` /
     `tags` / `aiNote`；写别的会被点名拒掉并整批 422，原话形如「条目没有 "xxx"
     字段，能改的是 name/url/icon/desc/alias/tags/aiNote」。
   - **分组能 `set` 的字段是** `title` / `layout` / `accent`。

   **新增一个分组**（`add` 到 `path: "groups"`）时，`value` 是分组对象，而分组
   的标题字段叫 **`title` 不叫 `name`**——`name` 是**条目**的字段，两层同名最容易
   写错那一格，写错会被拒成 `unknown field "name"`。
   `layout` 只收 **`tiles` 或 `list`**，别的值报「新分组的 layout 只能是
   tiles/list」；`accent` 可选（颜色），`items` 是条目数组。这三格都对着
   `internal/model` 的 `Group` 结构（`internal/model/model.go`）。

   ```json
   { "op": "add", "path": "groups", "index": 0,
     "value": { "id": "nas", "title": "自建服务", "layout": "tiles",
       "items": [ { "id": "portainer", "name": "Portainer",
         "url": "http://10.0.0.9:9443/", "icon": "simple-icons:portainer" } ] } }
   ```

## 图标

挑图标前查 `GET /api/v1/icons?q=...&prefix=...`（公开、无凭证）——它在服务端
内嵌的双层索引（`tabler:<name>` 与 `simple-icons:<name>`）里搜名字，返回
`{query, total, returned, icons[]}`：`total` 是匹配总数、`returned` 是本次条数
（`limit` 默认 20、上限 200），零命中（`icons:[]`）与命中多被截断是两个状态，
别把后者当端点坏了。查询词带前缀且整键命中（如 `q=tabler:box`）时只回那一条。

新加书签时可以先 `POST /api/v1/resolve` 拿标题/描述/候选图标，再
`POST /api/v1/batch` 落库；resolve 不写书签，`by` 字段它不接受。它给的
`iconCandidates[]` 就是索引里的全键，可以原样写进 `icon`；它的 `favicon`
（`/icons/<hash>.<ext>`）是图片路径，**不是**图标名。

图标名不许靠猜：`icon` 必须是索引里有的名字，名字不在索引里会被
`POST /api/v1/batch` 直接 422 拒掉整批，`errors[].field` 点名 `icon`。
`""`（未设，首屏走首字母兜底）与 `"auto"`（抓 favicon）是仅有的两个不进索引
判定的语义值。上面 batch 示例里的 `simple-icons:portainer` 是真条目，可直接
作为"这名字存在"的依据。

`GET /icons/{name}` 是另一回事：那是 `POST /api/v1/resolve` 抓回来的 favicon
缓存（`<16 位小写十六进制>.<png/ico/svg/webp/gif>`），resolve 返回的 `favicon`
字段就是这个路径。它是给浏览器 `<img src>` 用的图片 URL，**不要**把它的值写
进 `icon` 字段——`icon` 只认索引名与上面那两个语义值，写个路径进去只会 422。
读侧不要自己拼文件名：这条通路的字节只由 resolve 内部写入 `data/icons/`。

## 禁止事项（照 spec §6.4 / §11 抄）

- **不许直接改 `data/` 下的任何文件。** 书签写入只有一条 REST 通路
  （`POST /api/v1/batch`）；快照、历史、rev 递增都发生在它内部，绕过它就等于
  制造第二份真相。CLI 子命令也只经本机 REST，不直改磁盘。
- **没有"把一份清单推进各个档案"这种命令。** 模板机制整条拆掉了（2026-10-10）：
  每个档案完全分离，建出来时形状一模一样，没有一份共用的东西可以被复制给谁。
- **没有共享清单，条目也没有 `shared` 字段。** `GET /shared` 不存在、也不许建：
  2026-10-10 当天拍板后又改判为**不做**（"各存各的"就是产品本身），见
  `docs/superpowers/specs/2026-10-10-launchbay-admin-boundary-design.md` §6。
  想给条目写 `shared` 会被 ops 校验点名拒掉，整批 422，原话是「条目没有
  `"shared"` 字段，能改的是 name/url/icon/desc/alias/tags/aiNote」——那正是它
  该有的形状。
- **不许把真实档案目录或真实内网地址提交进仓库。** 文档与夹具只用 `10.` 段示
  例地址；`.env`、`data/`、`.superpowers/` 都不进仓库。
- 不提供 MCP server：REST + curl 已够用，少一个常驻件。