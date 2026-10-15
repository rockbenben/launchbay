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
  **空值是合法配置**（2026-10-12）：这台机器不发档案作用域令牌，服务照常起。
  书签页/管理页/回退/导入全走会话 cookie；要令牌的是浏览器扩展、CLI 与 AI 脚本，
  而那三样本来就要人主动去装。硬性要求它等于让每个装机的人都得填一个、再留成一个
  公开的默认值——那份默认值恰好能写最常见的档案 id。
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

## 顶栏那一行说的是谁

档案页顶栏那行大字是**这一页叫什么**（`Profile.PageTitle`，可在顶栏「改名」那一格
改，走 `/prefs`），没起过名字时按界面语言显示通用默认（中文「我的书签」/英文
「My bookmarks」——默认值**只在渲染时算，不落盘**，与 theme 那格同一形状）。
**「这是谁的那一份」住在右上角那枚常驻徽标上**（档案名，`aria-label` 是
「%s · 换档案」）。浏览器标签页两件都写：`页面名 · 档案名`。

## 端点清单

除 `/`、`/pin`、`/healthz` 三条浏览器通路外，每一条都要列在这里。方法、要哪
种凭证、成功响应都对照 `openapi.json`。

**`/admin`、`/setup`、`/prefs`、`/switch`、`/revert` 与 `/history` 是浏览器表单/导航通路，AI 不走这六条**：其中 `/setup`、`/prefs`、`/switch`、`/revert`
与 `POST /admin` 只由页面里的 `<form method=post>` 走进来，收 `application/x-www-form-urlencoded`，没有 JSON 版，
也没有 `by` 字段；`/admin` 是 GET 的 HTML 管理页，`*` 令牌打它拿到的也是这张页面而不是
另一份 JSON——这不是第二条 API 面，要机器可读的同一份跨档案聚合请打
`GET /api/v1/status`。`/setup` 是**零档案时的一次性通路**（判据是磁盘上 `profiles/` 的目录数，
不是"谁有资格"），它同样是浏览器表单通路而不是第二条 API 面，脚本别去 POST 它——
有档案之后它只会 303。浏览器通路的**权限类**拒绝答 303 回档案选择页而不是 401/403
（spec §6.1）：`GET /admin`、`/prefs`、`/switch`、`/admin/pin`、`/admin/delete`、`/revert` 与
`POST /admin`、`/admin/pin`、`/admin/delete`、`/revert` 的权限类（无会话、非管理员、
带 `*` 令牌）都是**不带提示**的 303；只有**字段没填对**才带 `?err=<码>`——`/pin` 说
`pin`/`profile`/`disabled`（**没有 `missing`**：2026-10-12 起空表单与错 PIN 是同一件事，
给它们分两把键只会让其中一句有一半是假的），`/setup` 与 `POST /admin` 用建档那几种码
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

## PIN 可以不设（2026-10-12）

**一份档案可以不设 PIN**，此后 `Profile.PIN` 是空串，进它不需要任何 PIN。这是给
公共电视屏、走廊那块平板、以及"这台机器反正只在自家客厅里"这类场合的。

- **判据只有 `auth.HasPin` / `auth.VerifyEntry` 两处**：空哈希 = 没设 PIN。
  `VerifyPIN` 保持严格（空串恒假），它答的是"这个 PIN 对不对"，不是"能不能进门"。
  `Profile.Validate` 与 `store.CreateProfile` 里那两条"空哈希就判坏档"整条删掉——
  哈希的形状归 auth 管，别的层不复述第二次（两处各判一次，就有两处会漂）。
- **「不要这道门」必须是一个被说出来的决定**。所以 PIN 是**选填**，但空值不等于
  不设防：不设防要单独勾一格（表单 `nopass=1`）或单独给一格（API `nopass: true`、
  CLI `--nopass`）。少了这一格，"忘了敲一位就回车"会静悄悄地建出一份局域网里谁
  都能进的书签页——而这个产品最核心的那句承诺正是「各人一份，别人看不见」。
  **不勾而 PIN 为空照旧被拒**（API 422 点名 `pin`；表单 `?err=badpin`；CLI 退 2）。
- **选择页上不设防的那一份不给 PIN 输入框**，卡上就是名字 + 服务图标 + 一枚
  「进入」，按下去直接进——**不多说一句**（2026-10-12 撤掉了原先那句
  「不设 PIN，点一下就进」）。名字**要**留在卡上：它是区分档案的那一维，而两
  份档案完全可能常用同一个服务（都用 GitHub），那时两张卡的图标一模一样。图标
  是"这个人常用的那个服务"的图，负责好看与认得出来，不顶替身份。脸由
  `icon.go` 的 `faceOf` 挑（favicon 优先于图标名，tiles 分组先挑），三取一仍
  是 `pieceFor`，与档案页同一份判据。要不要给框的判据是 `pickerEntry.NeedsPIN`
  （`auth.HasPin(p.PIN)`），**逐份**算而不是按机器算——隔壁有 PIN 的那份仍然要有框。
- **能设就能撤**：`new` 留空 = 取消这一份的 PIN，`isNoPass` 是唯一那处判据（表单与
  API 共用）。本人走 `POST /api/v1/profiles/self/pin`，管理员走
  `POST /api/v1/profiles/pin`。**本人撤自己那道锁仍然要给旧 PIN**——没有的东西不需要
  证明，但一道**锁着的**门撤锁要证明一次。
- **代价写在页面上**：取消之后，这一份对**局域网里所有能访问这台机器的人**开放。
  §11「不防局域网冒充」是整个产品的前提，这一格不是例外，是那一节里的一个选项。
- **空的分组在页面与面板上照画**（2026-10-15 改判，曾判"空的自动隐藏"）：页面上的组
  列表是「这个人放的每一个分类」，而空组仍然是一个**存在的目标**——它能被加书签
  进去（添加浮层的分组下拉、拖拽落点、「移动到」名单都是从页面上这些标题读的），把它藏起来
  等于让那几处一起失效，于是"这一组怎么才能再放东西进去"没有答案。空组照画并说清自己是空的：
  正文显示「这一组还没有书签」，大卡带子里的空组在标题上印条目数徽标「0」；台面面板里亦完整显示，
  清掉空组通过设置面板「分组」那一节进行。

| 方法 | 路径 | 凭证 | 成功响应 | 说明 |
|---|---|---|---|---|
| GET | `/app.e1e8f98072f3.js` | 无（公开） | 200 脚本字节 + `Cache-Control: public, max-age=31536000, immutable` | P1-⑤ 的应用脚本：顶栏「＋ 添加」与它的浮层（贴 URL → `POST /api/v1/resolve` 填标题/描述/建议分组/候选图标 → 回车经 `POST /api/v1/batch` 入库）。浮层的「分组」那一格**末尾永远有一颗「＋ 新分组…」**：选中它才多问一句名字与这一组用磁贴还是单列，建组与放条目在同一次 batch 里完成。**它不许只在零分组时出现**——那正是它此前的写法，症状是"有了书签之后网页上再也建不了新分组"，而 ops 一直认 `add path=groups`。**「图标」那一格右侧多一颗「图标库…」**（2026-10-14）：点开是一层窄浮层，输入即搜 `GET /api/v1/icons/tiles`，回车取第一格、点一格即把它写回那颗下拉框（`simple-icons` 品牌图标 + `tabler` 概念图标，与 resolve 的「自动抓 favicon」并列）；**图库不进这份脚本**——它是内嵌产物六千多条名字，进脚本等于让只看首屏的每一页都背一份（§7.3）。就地编辑那一层同样有这一格与同一颗键（此前它连图标都没有）。并注册 ⑥ 的 Service Worker；⌘K 面板里除了搜书签还有两格动作——「把剪贴板里的网址存进来」与「看这一份的改动历史」（后者导航到 `/history`，2026-10-13 那一屏）。零依赖 ES module，**只有档案首屏引它**，别的页面继续零脚本。**路径里那 12 位是内容指纹**（sha256 前 12 位），只在 `internal/render/web/app.js` 改动时变 —— 指纹由 `internal/render/assets.go` 在启动时从 embed 的字节算出，**不需要构建步骤**。改那个文件后本行路径要一起更新——**但不要手改**：`LB_SYNC_FINGERPRINT=1 go test ./...` 会把本文件与 `openapi.json` 两处的指纹同步成当前字节算出的那一个（它写完立刻跑同一条校验，所以它既是「改」也是「验」）。手改的那份重复正是最容易漏的一类，而漏了要等下一轮 CI 才红。用环境变量而不是 `-update` flag，是因为 flag 每个包各自注册，`go test ./... -update` 会让没定义它的那些包直接失败 |
| GET | `/sw.js` | 无（公开） | 200 脚本字节 + `Cache-Control: no-cache` | P1-⑥ 的 Service Worker，**路径不带指纹**（浏览器自己按注册处比字节决定更新）。**必须在根路径**：SW 的作用域是所在目录，挪到 `/assets/` 下就管不到首页。HTML 走 network-first + 1.5 秒超时回退缓存；按响应头 `X-LB-Profile` 分桶存 `html-<id>` 与 `html-picker`，**`-admin` 一律不存**（它含全家庭条目数与 rev，比选择页敏感一档），空头也不存。回退时按 `[currentProfile, picker]` 的顺序查桶，命中即交，**查不到就是失败**（不是"随便拿一份"）。切档时清掉上一个档案的桶——`app.js` 把 `/switch` 表单的 `to=picker` 转成空串发过来，所以 `sw.js` 那一支只删桶、**不写 `currentProfile`**；于是它可能仍指向一个刚被删掉的档案，回退查不到就落到 `html-picker`，与 §15 那条"删档后残留"的边界是同一族 |
| GET | `/manifest.webmanifest` | 无（公开） | 200 manifest JSON + `Cache-Control: public, max-age=86400` | P1-⑥ 的 Web App Manifest：平板/手机「添加到主屏幕」即成独立全屏应用（§6.3） |
| GET | `/app.5e1dd4d4d513.css` | 无（公开） | 200 样式表字节 + `Cache-Control: public, max-age=31536000, immutable` | P1-⑤/⑥ 的**交互**样式（浮层、⌘K 面板、编辑模态、拖拽把手）。走**外部**指纹文件而不内联：首屏骨架在 `base.css` 里内联，而这部分只有档案页用得到——内联等于让选择页/引导页/管理页/坏档页各白带一份，而且会把内联 CSS 顶出 §7.3 的操作线。**路径里那 12 位是内容指纹**，只在 `internal/render/web/app.css` 改动时变 |
| GET | `/bookmarklet` | 无（公开） | 200 JS 源码（no-store） | P1-⑥ 的 bookmarklet：拖进书签栏，在任何网站上一点就把当前地址带回这台机器。**里面绝不会有令牌**——Chrome 书签会云同步，写进 bookmarklet 的令牌等于明文外发；它只做一件事，导航到 `/?add=<当前URL>&t=<标题>`，写走会话 cookie（§6.3）。**它自己从 Host 头算这台机器的地址**，不让人填（填错的那种错要等到用时才发现）。未登录时 `?add=` 穿过 PIN 步骤不丢；登录后 ⑤ 的浮层自动打开预填，**要人再按一次保存才落库** |
| GET | `/api/v1/openapi.json` | 无（公开） | 200 内嵌契约原字节 | 机器可读契约，无个人信息 |
| GET | `/api/v1/data` | **档案作用域**令牌/本人会话 | 200 `{profile, rev, modified, data}` | 单档案全量数据 + rev，`?profile=<id>` 必填；**管理员令牌与管理员 cookie 都 403**（管理员看不见别人的书签） |
| GET | `/api/v1/export` | **档案作用域**令牌/本人会话 | 200 磁盘原字节 + attachment | 导出不重编码，可直接喂 batch 对照；同样只给本人（管理员 403） |
| GET | `/api/v1/history` | 令牌/会话 | 200 `{entries:[{at,profile,actor,by,rev,ops}]}` | 审计线，ops 只有动作与路径无 value；省略 `profile` = 全家庭聚合，**仅管理员**（档案作用域令牌 403）；账号类动作的 `actor` 记 `admin` |
| GET | `/api/v1/status` | 管理员（`LAUNCHBAY_ADMIN_TOKEN` 令牌或 `lb_admin` cookie） | 200 `{profiles[],brokenProfiles[],totals}` | 跨档案 rev/条目/快照/坏档汇总，含 `iconUnmatched`（读时算的缺图数：陌生图标名 + `auto` 且缓存无图，空 `icon` 不算缺，口径见 openapi 的 `/status`）。**每行没有 `admin` 字段**（2026-10-12，档案不再是管理员） |
| GET | `/api/v1/profiles` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{profiles:[{id,title,createdAt}]}` | 档案元信息列表，零条目零 URL；**同样没有 `admin` 字段** |
| POST | `/api/v1/profiles` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 201 `{id,title}` | 建档；**不带任何预置内容**，每个档案从零开始；已存在 409。**请求体没有 `admin` 那一格**（2026-10-12）——送来是 422 且点名该字段，脚本作者据此知道该删哪一行。**PIN 选填**：`{"id","title","nopass":true}` 建出一份**不设防**的档案（局域网里谁都能进）；`pin` 为空且**没给** `nopass` 照旧 422 点名 `pin` |
| DELETE | `/api/v1/profiles` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{deleted}` | 删档连 `snapshots/`，历史保留，`?profile=<id>` |
| POST | `/api/v1/profiles/pin` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{profile,pinChanged}` | 给任意档案设 PIN（不要旧 PIN），进目标档案历史 `rev:0`。**`new` 留空 = 取消这一份的 PIN**（2026-10-12）——管理员是唯一能给别人去/留这道门的人：一个都不锁的家族里，他得能把门重新锁上；非空但格式错仍 422 点名 `new` |
| POST | `/api/v1/profiles/rebuild` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{id,title}` | **坏档的唯一出路**：某档案 `profile.json` 读不动时只重写这一个文件，书签、快照、历史已有内容**逐字节不碰**。体 `{profile,title,pin,nopass}`，**没有 `admin`**（2026-10-12 随档案属性一起删除；送来是 422 且点名该字段）。`nopass:true` 重建出一份**不设防**的档案——rebuild 必须能造出这个形状，它是唯一能整份重写 profile.json 的动作。判据全在 store 锁内：**读得动 → 409**（改 PIN 走 `/api/v1/profiles/pin`，这条不是改档用的）；**目录不在 → 404**（它不建目录，建档仍只有 `POST /api/v1/profiles`）；只有"内容错"与"文件不在"才允许覆盖，权限/磁盘那一类读失败原样上报不许盖。`theme`/`layout`/`pageTitle`/`view`/`createdAt` 按默认重建（读不出来就没有"保留"这个选项；
`pageTitle` 与 `view` 从前没列在这里，是文档漏了两格而不是它们不被重置——
rebuild 写的是一个新的 `model.Profile` 字面量，凡不在里面的都回默认）。200 意味着"进门页那张卡真的回来了"——写完在同一把锁内回读过。历史追一行 `rev:0`、`path:"profile/rebuild"`；审计行写不下去仍报 200（**这条动作不幂等**，重试会拿 409，报 500 等于对已修好的档案说"没修好"），缺口只进 stderr |
| POST | `/api/v1/profiles/self/pin` | 仅会话 | 200 `{profile,pinChanged}` | 改自己 PIN；令牌一律拒（管理员令牌也是 403——它是一把好钥匙，不是废钥匙）。**旧 PIN 只在这一份现在有 PIN 时才要**（2026-10-12）：没有的东西不需要证明，所以一份不设防的档案可以直接给自己设上第一个 PIN（`old` 可以不给，给了也不校验）。**`new` 留空 = 取消这一份的 PIN，且此时仍要 `old`**——撤的是一道锁着的门 |
| POST | `/api/v1/batch` | **档案作用域**令牌/本人会话 | 200 `{profile,rev,applied,warnings}` | 唯一的书签写通路，一批 ops 原子写入；**管理员令牌与管理员 cookie 都 403** |
| POST | `/api/v1/revert` | 令牌/会话 | 200 `{profile,rev,restoredFrom}` | 从快照恢复，产生新 rev，不抹历史 |
| POST | `/api/v1/usage` | 令牌/会话 | 200 `{updated, unknown[]}` | 点击计数，只写当前档案，不参与 rev |
| GET | `/api/v1/icons` | 无（公开） | 200 `{query, total, returned, icons[]}` | 本地索引搜图标名；`q` 必填，`prefix` ∈ tabler/simple-icons，`limit` ≤200 |
| GET | `/api/v1/icons/tiles` | 无（公开） | 200 HTML 方格 | `/api/v1/icons` 的**界面那一半**（2026-10-14，图标选择器）：同一个 `q`、同一份内嵌索引，吐的却是一段**已经画好的 HTML**——每格一个 `<button type=button class="fld icpick-tile" data-icon="<名字>">`，里面是 `render.svgMarkup` 画的**页面自己那份内联 SVG**（全案唯一把图标拼成 markup 的出口），于是挑的时候看见的形状与存下去之后卡片上那一枚逐像素相同。**为什么不并成上面那条的一个 `format` 参数**：响应类型根本不是 JSON，而 §4.3 那份契约按形状写形状，两半混在一个端点里没法一句话说清。`q` 必填（缺了 400；**空查询回一张空图而不是全量六千条**），`limit` ∈1..120（默认 24，超了钳到 120——它是给人看的一格，给 AI 的那条是 200）。响应带 `Content-Type: text/html; charset=utf-8` 与 `X-Content-Type-Options: nosniff`（它要被插进页面里，缺 nosniff 时浏览器照样 sniff）；落在 `/api/v1/` 前缀下，`private, no-store` 由前缀中间件给。**`X-LB-Icons-Total` 报命中总数而不是画出来的条数**：界面据此说「共 N 个，先看前 M 个」——截断必须说出来（§4.3 那条"零命中"与"命中很多被截断"要能分开判），否则人会以为库里只有那几十个。零命中是 200 且正文为空，不是 404。脚本要名字数组仍走上面那条 |
| GET | `/icons/{name}` | 无（公开） | 200 图片原字节 + `Cache-Control: public, max-age=31536000, immutable` | resolve 抓来的 favicon 缓存，名字必须是 `<16 位小写十六进制>.<png/ico/svg/webp/gif>`；形状不对与没这个文件都是 404，同一句话 |
| POST | `/api/v1/resolve` | 令牌/会话 | 200 `{url,title,desc,suggestedGroup,iconCandidates[],favicon}` | URL → 书签候选；体只有 `{url, profile}`（`by` 是未知字段 → 422），`profile` 空申报 = 403；不写书签/历史/rev，只在 `data/icons/` 落缓存 |
| POST | `/lang` | 无（判据是磁盘：`profiles/` 下目录数为 0 才有效） | 303 回 `/` | 引导页上那个界面语言下拉的落点（2026-10-12），**浏览器表单通路**。它原本在 `POST /setup` 那张表单里，于是"改语言"的生效时机成了"按另一个按钮"——人拨了下拉、页面纹丝不动，看上去就是坏的，而那是新装机的人看到的第一块屏幕。现在它有自己的表单与按钮，按下去页面用新语言重画。**只在零档案时开放**（判据与 `/setup` 同一份 `CountProfiles`）：语言是站级设置，装完之后只有管理员能改；零档案是装机窗口，那一刻还没有"谁该替所有人选语言"这个问题。`POST /setup` 的 `language?` 字段**继续收**（一次填完的形状，脚本与 curl 仍可一趟发完），两个入口写的是同一个 `store.SetLanguage`。零 JS |
| POST | `/setup` | 无（判据是磁盘：`profiles/` 下目录数为 0 才有效） | 303 回 `/` | 引导建档，浏览器表单通路：**AI 不走这条，也别拿脚本去打它**。体是 `application/x-www-form-urlencoded` 的 `{title, pin, nopass?, language?}`，不问 id（由名字派生 slug，中文走 `home`、`home-2` 基名序列）；**建出来的是第一个人的书签页，不是管理员**（2026-10-12：管理员是机器级的一把 key，与档案无关，所以"零档案装完没人能加人"那个老问题也不再成立）；成功**不发会话**（进门照样要 PIN——除非这一份是 `nopass=1` 建出来的不设防档案，那时进门不需要 PIN），字段没填对回 `/?err=noname`或`badpin`，**宿主数据目录不可写**回 `/?err=permission`（NAS 上把 `./data` 挂成 root 所属、容器以 uid 65534 跑那个形状，去宿主机 chown 即可），窗口已关（已有档案）回 `/` 且不带 `err`——「这台机器已经建过档了」本身就是枚举线索。AI 要建档仍只有 `POST /api/v1/profiles`（带 `id`，**不带任何起步内容**，模板机制 2026-10-10 整条拆掉了，`fromTemplate` 与 `data/templates/` 都不存在）那一条 |
| GET | `/prefs` | 仅会话 | 200 HTML（no-store） | **台面面板的独立页**（2026-10-13）。这一页把顶栏原先散着的六颗控件收进一屏——改名、排法（三颗）、档案形态（两颗）、主题（三颗）、回退、改动历史——**改的全是同一件事**：这一页怎么排、叫什么、能退到哪。**左侧有一列分类 tab（2026-10-15）**：三颗（外观 / 回退与历史 / 分组）——**就三颗，再往下拆到每一行是噪声**（那一屏只有三个真正的断层，而"多几个"量出来是每格跳 100px，那不叫 tab）。它**零 JS：三颗 radio + `:checked ~ .prefs-panes` 兄弟选择器**，`/prefs` 这张独立页**根本不引 app.js**，而同一份 define 也渲染进档案页的浮层——JS 切 class 只会在一条路上有 tab。**退化方向是"看得全"**：pane 默认可见、只被 checked 的那颗关掉，CSS 没加载时三节全在。**停在哪是 URL 上的 `?tab=`**（`look`/`out`/`groups`，名单只有 render 侧那一份 `render.PrefTabValues`；不认识的值落回 `look` 而不是 400）——它必须活过一次保存：`/prefs/group` 那一族的 action 带 `?tab=`、`applyPrefsOps` 的 303 落回也带着，浮层那条路由 `sessionStorage['lb-prefs-tab-<档案>']` 记着（重载后重新 fetch 面板时带上去）。**只有分组那一族带**：外观那几行改的是台面偏好、落点是 profile.json、成功后回 `/`（那里根本没有左侧这一列），而「回退与历史」那一节里没有表单。可及名走 **fieldset + legend**（"选一节来看"就是 radio 组；那三颗 radio 收进 visually-hidden 而不是 `display:none`，否则键盘进不来，焦点环落在 label 上），44px **并进导航族那条联合规则**（`.gnav a,summary,…,.prefs-nav label`），不新开一族。宽度预算量过：nav 150 + gap 22，面板内容 900 → 728，而最宽的一行是色板 512+44+gap 8 = 564。窄屏（≤520）**那列翻成顶部一条**（248px 内容宽里 150px 竖列会把色板那一行挤没）。**AI 不走这条**，要机器可读的同一份东西打 `POST /api/v1/batch` 与 `/api/v1/data`。**它为什么是页面而不是按钮**：档案页里那颗「设置」是 `<a href="/prefs">`，`app.js` 开着时把它拦成浮层并 **fetch 这一页注入同一份标记**（`profile.tmpl` 里 `prefs-panel` 那一个 define 渲染两处——不另开 `prefs.tmpl` 是因为 `icon` 在两个模板里重名，并成一个集合会让其中一份被静默覆盖），关着时它就是一次普通跳转；浮层拉不到就退回一次整页跳转，**降级路径自己成立**。面板约 2 KB 而**不内嵌进档案页**：§7.3 那条操作线管的是每一页首屏，不打开面板的人不该背它。**它叫「设置」而不是「外观」**（2026-10-14）：这一屏里主题/排列方式/分组列表与分组颜色算是外观，而「回退与历史」是两个出口——「外观」承诺的只有它能给的一半（id 仍是 `lb-prefs-open`，内部形状不跟着界面词漂）。**320–520 窄屏换一套排法**（`@media (max-width:520px)` 在 base.css）：标签列收成"标签在上、控件在下"（92px 那列在 248px 内容里占 37%，而窄屏上标签本来就在控件上面一行），色板**继续用 flex-wrap**，44px 一颗都不缩（§7.5 的触摸地板，窄屏正是最需要它的时候）——这里从前改过一版 `auto-fill` grid，实测代价是 grid 容器在这一档拿到确定高度、两行行高各被撑成 252，于是每一颗色块变成 44×252 的色带、每个分组 688 高、整块面板 5041px（390 ≈6 屏，20 个分组那一份 21.3 屏）；换回 flex-wrap 是 44×44、每组 173–272。另一条同族的：`flex-basis` 是**主轴**上的量，行向写的 basis 翻成列向就成了高度（名字行实测 120 高 → 收回 `auto` 后 21），所以那一档显式收回 basis。断点跟着**内容宽**走不跟着某一台设备走。浮层那一层的宽度由 app.css 单独放宽一档：**宽屏上一个分组占一行**（2026-10-14 实拍后改判），那一行是色板 512 + 排法三颗 212 + 名字下限 96 + 两个 gap 16 = **836**，所以浮层给到 960（内容 899）、`.prefs` 上限 900，独立页与浮层同一份 define 是同一个数；多出的余量是留给"排法那三颗中文键在别的机器上字宽不同"的。**"面板太窄"的真症状不是宽度上限难看，是每个分组被折成两行**（从前的 620 只装得下色板那 512，八个分组成了十六行、面板 1027 高要滚一屏半）；放不下 836 的那些视口由 flex-wrap 自己折回两行，**没有为它开媒体查询**。**准入与 `/revert`、`/history` 同一族**（`selfProfileOnly`，档案从会话推导、不接受申报，管理员进得了也只能改自己那一份）；**令牌一律 303 回选择页，不是 403**（2026-10-14 实测后改判：`selfProfileOnly` 对 bearer 的第一句就是 `redirectPicker`，这一族五条 `/prefs`、`/prefs/group`、`/prefs/tabs`、`/prefs/background` 与 `/history` 都走它；实测管理员令牌/档案令牌/令牌+会话/无凭证四种长相全是 303）；会话指的档案没了 → 303 回选择页；**坏档不挡**（改台面只写 `profile.json` 的 theme/view/layout/pageTitle，坏档正是最需要改排法的时候）最下面还有一节「**分组**」，每个分组**两行**：第一行是**可编辑的名字**（一个平时长得就像那行字、聚焦才浮出边框的输入框，2026-10-15 起改名的出口就在这里）与这一组自己的磁贴/单列/大卡同一行，**这一行右端是那颗「删掉这一组」**（2026-10-15 实拍后从色板那一行移上来：右端那一列是「这一组的键」，删是同族动作，而挂在十颗色点后面它读起来像色板的一部分；移上来之后这一行是 246+87=333，名字仍有约 550px），第二行是颜色（色板 + 自定义）。**删是两步**——第一下展开，第二下才发 op（与就地编辑删一条书签同一处置，§8.8 早就判过不用 `confirm()`），展开那一句**必须念出这一组还有几条**（"这一组还有 N 条书签，删掉它这些也跟着没了。删掉之后还能用「回退」找回这一版。"），空组说的是另一句"这一组是空的，删掉不会少书签。"展开后那块**独占一行**（同顶栏「改名」那条 `.rename[open]`）。**清单最上面还有一行「＋ 新分组」**（2026-10-15，同一骨架：名字 ｜ 磁贴/单列/大卡 ｜ 「建这一组」），一条虚线把它与下面那些行分开——建组此前只有顶栏「＋ 添加」浮层里那个下拉项与 batch 两条路，**设置屏改不了组表本身**（清空了的组、没用上的新分类都建不出来）；它**原生提交**（app.js 那份委托在没有 `input[name=group]` 的表单上早退），所以零 JS 与开着 JS 表现一样，多一次整页跳转换来"两条路行为完全一样"——要在浮层里接管它就得把服务端那段 id 派生在客户端再抄一份，中文那份抄不出 ASCII 就得再抄一遍退化规则。**两行不是排版失误，是那条 96px 逼出来的**：1440 实测面板内容宽 899，扣掉排法 212 与色板 512 与 gap 16，名字只剩 91px，而「自动化与平台」光是**读**就要 99px——一行版式本来就在截断最长的组名，加上"能改"之后连改都改不了。代价是每组 44 → 94px。这是"能配却没有出口"的那一格——`ops` 一直认 `groups/<id>/accent` 与 `groups/<id>/layout`，而界面上一个入口都没有，于是每个类目的色块都是 CSS 的 `--sec-accent,var(--accent)` 兜底值，一整页同一个蓝；落点是 `POST /prefs/group`。**书签文件读不出来时只撤掉这一节**，其余四行照常可用（与"坏档不挡"同一条纪律）。；不进渲染缓存 |
| POST | `/prefs/group` | 仅会话（表单） | 303 回 `/prefs` | 改**一个分组**的名字、颜色与它自己的排法（磁贴 / 单列 / **大卡**），**建一个新分组**，或**删掉这一组**——台面面板「分组」那一节的落点。**`add`（2026-10-15 新增）**：只给 `add=<名字>`（可选 `layout`）就建一个新分组，**id 由服务端派生**（`store.NextGroupID`：slug 得动且没被占就用它，否则基名 `group`/`group-2` 序列；中文名派生不出 ASCII ——那是 §11.2 写死在明处的代价，所以中文名多半落在 `group-N` 上）。**界面上不问你 id**：`/setup` 与管理页那两张建档表单都不问，这里同一族。名字空着/超长（> `model.MaxGroupTitleRunes`）→ 422 点名 `add`；带 `group` 的那些判据在这一支**不参与**（新组还没有 id）。走的是**同一个 `applyPrefsOps`/`applyBatch`**（快照、审计行、rev 一道闸不少，所以建错了能退回来）。**`remove=1`（同日新增）**：发一条 `remove path=groups/<gid>`，同样走那一份落点。**`remove` 与 `rename`/`accent`/`layout`/`add` 互斥**，判据是**名字**而不是"只提交被按下的那个控件"——那张表单里名字输入框与取色器都**常驻**，按删键时它们照样跟着提交，照单发出去就是"顺手把名字和颜色也改了"（顺序写反一点更糟：先 remove 再 set title 会让 ops 报目标不存在而**整批不写**，于是"这一格删不掉"）。**服务端不拦"这一组还有条目"**：代价由界面上那颗红键前面那一句说出来，再按条数拦一遍只会让那颗键说谎。组已经没了 → **200 把面板重画回来并在顶部说清**（与改颜色同一条处置）。**`rename`（2026-10-15 新增）**：分组标题，此前在界面上一个出口都没有（`ops` 一直认 `set path=groups/<gid>/title`，面板只画得出颜色与排法）。它**常驻**在表单里（输入框每次提交都跟着走，与取色器同一性质），所以「没改就不算一次改名」这个判据落在**值**上：与盘上当前标题相同的**不发 op**，而 `applyBatch` 只看 opList 非空——发出去就一定会写一版快照并把 rev +1，于是「按一下已经在用的那颗色块」会在「回退」里留下一条什么都没改的版本（颜色与排法同样按这个判据压掉）。空名字与超长（> `model.MaxGroupTitleRunes`）只在**它就是那一次的全部意图**时判 422（混着颜色或排法时不连坐——那是"输入框里留着没提交的内容"，说清按的是哪一颗更有用）；**id 不随改名而变**（分组 id 是寻址键，导航与别人的链接都指着它）。`layout`（组） ∈ `tiles`、`list`、`feature`，**它与 `POST /prefs` 那个 `layout`（sidebar/topbar）不是一件事**：一个排分组内部的条目，一个放分组导航；两个枚举都叫 Layout 是历史遗留，判据是 `model.ValidGroupLayout` 与 `model.ValidLayout` 各一份。**`feature`（大卡）那一档换的是整块版式与位置与位置**：渲染层（`internal/render/exec.go` 的 profile）把 `Layout == model.LayoutFeature` 的分组从 `Bookmarks.Groups` 里拿走、放进 `Featured`，页面顶部一块——**2026-10-15 放大后**：卡片 112px 高、栏宽 300px、图标 40px、名字 18px、下方一行主机名 13px（窄屏 92px / 30px）；**2026-10-15 第二处**：带子改成**多组合进一块**（一个组一块 → 两段之间夹一条线那不是"一个放大的区域"），分组名从整块的 h2 降成块内 `.feat-g`。**第三处：两档多列视图下必须横跨全部列**——`[data-profile-view=wall] .feat{column-span:all}` 与 `[data-profile-view=dense] .feat{grid-column:1/-1}`：带子只是 `main` 的一个普通 section，而那两档的 `main` 各自是多列与 grid，不写这两句它就退化成**并列的一栏**（全局档 317px 宽、七张卡竖叠 911px；密集档每列 208px 而带内轨道 `minmax(300px,1fr)` 更宽，卡片**压在隔壁那一栏上**）。同时**密集档那几条收窄规则不适用于大卡**（base.css 里用 `:not(.feat-card)` 收窄）——否则 `[data-profile-view=dense] .plain` 的 (0,2,0) 会赢过 `.feat-card` 的 (0,1,0)，把卡片那张脸收掉，于是同一个带子里点不动的几张**没有卡片底**。**挪走而不是复制**（同一组在一页里出现两次那才是真的坏），而**分组导航照旧**（设成大卡只换位置，不让它从导航里消失）。它与档案级 `view`（groups/wall/dense）正交：那两档的「紧凑」说的是分组之间不留大块空白，**不缩常用入口**。`accent` **空串是合法值**（= 没设色，色块退回页面主色），非空只认 `#rgb` / `#rrggbb`。面板上那排色板（`model.GroupAccentPalette`）**不是白名单**——它答的是"大多数人选哪几个"，「自定义」那一格走的正是不在表里的任意 hex；判据只有一份 `model.ValidAccent`（写侧拒、读侧丢）。一张表单里并排摆着色板、排法与取色器三样，**分开它们的判据是名字**：被按下的色块交 `accent`、排法键交 `layout`、取色器那一格叫 `custom`，而 `custom` **只在同一次提交带上 `apply_custom=1`（那颗「用这个颜色」的键名）时**才说话。这里不能写"靠 HTML 只提交被按下的那个控件"——命名过的输入框每一次提交都跟着走，那样一写，按色块就交出两个 `accent`（实测被拒成 422，人在屏上看到的是整页裸 JSON）、按排法还顺手改掉颜色（实测一次按下写两条 op），而零 JS 那一页根本没有那颗能改回去的键可用。两样都没交仍是 422：那是真有人拼错了字段名，不是"什么都不改然后成功"。**带令牌一律 303 回 `/`**（与 `GET /prefs` 同族同答法，见那一条）。**它走书签写通路**（`applyBatch`，与 `POST /api/v1/batch` 同一个落点，`/api/v1/import/apply` 也是这么走的）：快照、审计线、rev 一道闸不少，而 `POST /prefs` 只写 profile.json。`expectedRev` 不参与——面板是服务端渲染的，它画出来那刻的 rev 未必是人按下那刻的，而两件事改的是不同字段，ops 在锁内重读当前值再改。成功 303 回 `/prefs` 而不是 `/`：改颜色本来就是一件要连着改好几组的事。ops 没过（最常见是分组已被删）或快照写不下 → **把面板重画回来并在顶部说清为什么**，绝不跳到一个"看起来像成功了"的页面 |
| POST | `/prefs/background` | 仅会话（表单） | 303 回 `/prefs` | 改**这一页自己的背景图**（2026-10-14）：上传/换一张、移除、暂时不显示/恢复显示，**三件事共用这一个端点**（一次按下 = 一个意图，与 `/prefs/group` 同一处置）。**必须 `multipart/form-data`**——文件走不了 urlencoded；而这**就是不并进 `POST /prefs` 的原因**：那一条的字段名单是 auto\|dark\|light\|view\|title，把一张几 MB 的图塞进去，那句「至少给一个字段」就会开始管到不属于它的东西。字段：`<input type=file name=file>`（**按魔数嗅探类型，不信浏览器给的 Content-Type**；只认 png/jpg/webp/gif，**svg 一律拒**——背景是铺满整屏的位图，而 svg 是一整类执行面；上限 4 MiB）、`remove=1`、`hidden=1\|0`。落点 `data/backgrounds/<档案 id>/<16 位内容哈希>.<后缀>`：**按档案分目录**，于是「删除」只需要删自己那一个文件（单份内容寻址会让 A 删掉之后 B 的背景跟着消失，而「还有谁在用」这个问题的答案会变）；内容寻址那一半让换一张必然换一个 URL（immutable 缓存立刻失效）。写 `profile.json` 的 `background` / `backgroundOff` 两个键：**不动 rev、不做快照、不写审计行**（与那四条台面动作同类），而它**不随书签导出走**（导出只带 bookmarks.json）。换一张会自动把 `backgroundOff` 归位——「暂时不显示」说的是"我不想看**那张**"，而人刚给了新的一张。形状错/不是位图/太大 → 422 点名 `file`；没说要改什么 → 422（不是静默 303） |
| POST | `/prefs/tabs` | 仅会话（表单） | 303 回 `/prefs` | 改「点开书签」那一格：`tabs=0`（当前标签页）/ `tabs=1`（新标签页），落 `Bookmarks.OpenInNewTab`。**它走 applyBatch 那条书签写通路**（快照、审计线、rev 一道闸不少，收尾与 `/prefs/group` 同一份 `applyPrefsOps`），而**不是**并进 `POST /prefs` 的字段名单——那一格改的是台面偏好（只写 profile.json、不产生版本），而这一格住在**书签文档**里（跟着导出/导入走），两族落点不同，混在一处会让「四个字段至少给一个」开始管到不属于它的东西。只认 `0`/`1`（把 `true`/`yes` 也收下等于让人以为按了而其实没按）；令牌一律 **303 回选择页**（这一族只认会话，`selfProfileOnly` 的答法就是 redirectPicker）。**它对 `chrome://` 与 `file://` 那一行不生效**：那类地址从 http 页面点链接会被浏览器挡掉（2026-10-14 实测：`Not allowed to load local resource`），所以它们继续是「需手动打开 + 复制键」，见 `model.ExternalScheme` |
| GET | `/backgrounds/{name}` | 仅会话 | 200 图片原字节 + `Cache-Control: private, max-age=31536000, immutable` | 端出**会话所指那一份**的背景图。**与 `GET /icons/{name}` 那条公开通路相反**，理由是 §11：favicon 是任何站都有的图标，而背景图是家里人自己放的图（很可能是孩子的照片）——「64 位哈希 URL 当秘密」我们没必要顺手造出来，而**按档案分目录**让 A 的会话连 B 的那张都读不到，哪怕他知道那个哈希。形状即边界：`<16 位小写十六进制>\.(png\|jpg\|webp\|gif)`，形状非法与盘上没有是**同一句 404**（这里不是诊断通路）；带令牌 **403**（认出来了，而令牌不带这个门——与 `self/pin` 同一句，不是 401）、无凭证 401。响应带 `X-Content-Type-Options: nosniff` 与 `Content-Security-Policy: sandbox`（裁定 R9 的同一个理由：谁把 URL 直接开进地址栏，响应就是他自己的一个文档）。**它不进 Service Worker 的预缓存清单**（那份只有静态资产），所以离线首屏若没命中浏览器自己的 HTTP 缓存，症状是「背景不见了」而不是页面坏掉 |
| POST | `/prefs` | 仅会话（表单） | 303 回 `/` | 改自己的台面偏好：主题、档案形态、整页排法，以及**这一页叫什么**；`theme` ∈ `auto`、`dark`、`light`；`layout` ∈ `sidebar`、`topbar`；`view` ∈ `groups`、`wall`、`dense`（`groups` 默认一组一节；`wall` **全局视图**：各组作为列头排进多栏、条目仍是磁贴；`dense` **密集视图**：磁贴去壳、每条一行「图标 + 名字」，参考 Flare 那一屏——**去壳只对精确指针生效**，触摸设备仍由 44px 那一族接着）——**`view` 不吃空串**，清回默认要送 `groups`，而盘上没有这个键（升级前的形状）在读侧合法；`title` 是最多 60 个字的页面名（**空串 = 清除**，顶栏回到按语言的通用默认「我的书签」），它落 `profile.json` 而**不是** `bookmarks.json` 的 `title` 字段——改名字不产生 rev 与快照。四个字段都不给 = 422；令牌一律拒。**`view` 为 `wall`/`dense` 时 `layout=sidebar` 的意思变成"那一列不画"**（2026-10-14）：这两档所有分组本来就摆在同一屏，导航的职责「跳到你看不见的那一节」不成立，而那一列 206px + 24px 的缝在 1440 上占 16%——实测（8 组 × 20 条、1440×1000）密集档从 4 栏变 5 栏、一屏 107 → 121 条，全局档 51 → 68 条，而**分组档一个字都不动**（侧栏 206 / 一屏 75 条）。判据两份共用一份事实：渲染层 `compactView`（给文案）与 base.css 的两条 `[data-profile-layout=sidebar][data-profile-view=…]`（给样式）；**服务端不因此改任何数据**，`layout` 仍只是那两个字，而面板里那颗键的**可见文字也不随视图变**（`TestPrefs_开关的标签稳定而状态走aria` 钉着"可见文字稳定、状态走 aria-pressed"），「紧凑档下这一列不画」写在下面那句说明里。`topbar` 那一档一个字不改——它是"想留着目录的人"的出路（名字与条数都在）。**入口**：**台面面板**（`GET /prefs` 那个 define），2026-10-13 从顶栏六颗散控件收进来之后，面板里的每一颗键都是**同一张脸**：标签稳定、状态走 `aria-pressed` 的 pressed 单选钮（`view` 三颗、`layout` 两颗、`theme` 三颗）——此前这三组分属**三种不同的控件**（改名是 `<details>` 表单、主题是一颗说"当前是哪一档"的三态循环键、视图与形态才是 pressed 单选钮），同族不同形是最难维护的一种排法。主题从循环键改成三颗显式键之后，`auto`（跟随系统）不再是一个只能按顺序绕回去才能回来的东西。面板每一行是**四张独立的表单**，一次按一下只改一件事，所以没有"提交"这一步，也就没有"忘了提交"。每颗自带 `name`/`value`，落点与扩展、CLI 收的字段一个字没变。`layout` 在 2026-10-13 之前**没有任何 UI 入口**：CSS 与模板属性一直齐备，但表单没有键、扩展与 CLI 也不收，改它只剩手改 profile.json（禁止事项） |
| POST | `/switch` | 仅会话（表单） | 303 回 `/` | 会话动作，2026-10-10 起只剩 `to=picker`（清两条 cookie 回选择页）。**`to=<档案 id>` 的免 PIN 直达已整条下线**——它是管理员进别人台改书签的唯一入口；`to=home`（原"退出管理"）降级为幂等空操作，因为代管态已经不存在、两条 cookie 不再可能分家。别的 `to`（含 `../` 探针、含曾经合法的档案 id）一律什么都不做地回选择页，与"没这个档案""你不是管理员"同一个回答；不动书签，不写历史行也不产生 rev |
| GET | `/import` | 仅会话 | 200 HTML（no-store） | 浏览器收藏夹导入**第一步：选文件**。只动**自己的**档案，且档案必须一条书签都没有（与 CLI 的 `import-bookmarks` 同一规矩：真要合并得先有一套对着既有档案做对账的判据，那不是"多一个入口"该顺带发明的）。档案非空时**直接在这一页把话说清**、不 303 带 `?err=`——登录着的会话回到 `/` 渲染的是**档案页**，那把键会被无声吞掉（`Test提示键只发白名单里有的` 正是防这个的） |
| POST | `/import` | 仅会话（`multipart/form-data`） | 200 HTML（no-store） | **第二步：收文件、解析，然后按 `go` 分两路**（2026-10-12）。体是 multipart 的 `bookmarks` 文件字段，上限 8 MB（`maxBodyBytes` 那 1 MiB 是给 JSON 表单的，浏览器导出动辄上兆）。`go=preview`（缺省）给报告等一次点头；**`go=direct` 收下就写**（同一张表单里两个 `submit` 按钮各带一个 value，零 JS）——它省掉的只是"人读一遍再点头"这个动作，**解析、筛选、快照、历史、rev 一道闸都没少**（走同一份 `planImport`/`applyImport`/`applyBatch`），不是第二条写路径。**读出 0 条必须说清"这不是书签文件"**——`Convert` 不拒绝非 Netscape 输入，喂一段随便的 HTML 它解析出零条并照常返回，不判就会"导入成功 0 条"。**进不来的条目是"跳过 + 逐条点名"，不是整批失败**：判据是 `model.CheckURL` 与 `model.ValidSlug` 各一份（地址要有主机名；id 必须过 slug 的长度与字符闸），报告里"## 没能进来"一节逐条写地址与理由，筛空的文件夹不建但要点名。写路径的"整批拒绝"没有改动——那是给 AI 与 CLI 的纪律（一次改 30 条时"改坏一半"比"全没改"难收拾），导入这条路**先把坏条目挑出去**再生成 ops。整份文件一条都进不来是**另一个形状**（读出了书签但没一条能进来），报 422 并说清原因，不说成"文件里没书签"（那会把人引去换一个同样导不进来的文件）。`go=direct` 成功回的是**结论页**（写进去几条 + 没能进来的逐条清单），不是报告——报告答的是"会导入什么"，这时候人要的是"成了没有"。**解析成功在 preview 那一路不写盘**：写入发生在 `POST /import/apply`，令牌与它绑定、15 分钟过期、总量 64 份、用过即删 |
| POST | `/import/apply` | 仅会话（表单） | 303 回 `/` | **第三步：确认写入**（只有 `go=preview` 那条路会走到这里）。体是 `{token}`；令牌取走即删，所以重放也写不进第二次——这比"确认后用 JS 锁按钮"可靠且零脚本。ops 由 `bookimport.Ops` 生成，落盘走 `applyBatch`——与 `POST /api/v1/batch` **同一个落点**，快照/历史/rev 一次不落。`by` 记 `import` |
| GET | `/revert` | 仅会话（表单；带令牌一律 303 拒） | 200 HTML（no-store） | 回退到某一版的**第一步**：把可回退的快照逐个列出来，每格带**条目数与分组名**——只印版本号的列表回答不了"我要退哪一版"。**两步二次确认、零 JavaScript**（与 `/admin/pin`、`/admin/delete` 同款范式）。**目标不接受申报**：档案从会话推导，所以 cred 是 credSession 而不是 credScoped——管理员进得了这一页也只能退自己那一份（2026-10-10 边界：管理员管账号、不碰内容，即使 PIN 是他设的）。`bookmarks.json` 读坏了（坏档）**不挡**这一页：revert 正是那种时候的出路。读不出来的快照不列但**报数**（静默少列会让人以为"只有这几个版本"）；一份都没有时只讲清为什么、**表单整个不画** |
| POST | `/revert` | 仅会话（表单；带令牌一律 303 拒） | 303 回 `/` | 回退的**第二步**，体是 `application/x-www-form-urlencoded` 的 `{to-rev}`。**不是第二条 API 面**：落点与 `POST /api/v1/revert` 是同一个 `revertLocked`（快照→写盘→审计，判据只有一份），差别只在怎么读入、怎么答出。成功 303 回 `/`（GET `/` 在有会话时渲染的就是档案页）；`to-rev` 没填/非整数/为负 → 303 `/revert?err=badsnap`；那一版的快照已经不在（被别人退掉，或被 50 份的裁剪清掉）→ `?err=gone`；当前这一版存不下所以**本次恢复已取消、书签一个字都没变** → `?err=nosnap`。落盘失败走 500 JSON 外壳，**绝不说成功**。历史里 `by` 记 `web` |
| GET | `/history` | 仅会话（带令牌一律 303 拒） | 200 HTML（no-store） | 改动历史那一屏（2026-10-13）：这一份的审计线，一天一节、一行一次动作。**AI 不走这条**，要机器可读的同一份数据打 `GET /api/v1/history?profile=<id>`。**目标不接受申报**：档案从会话推导（`selfProfileOnly`，与 `/revert` 同一族准入），所以管理员登着这一页也只能看自己那一份——还是 2026-10-10 那条边界：管账号、不碰内容。页面上只有**动作**没有内容：历史行本来就是 `{op, path}`、没有 value（§4.6）；路径里的 id 换成「那条书签现在叫什么」，被删掉的从 `snapshots/<rev-1>` 里找，两层都认不出就原样印 id（编一个名字是假话）。一页最多 100 次动作、一次最多 6 条短句，**截断在页面上说出来并给出没列出的条数**。`bookmarks.json` 读坏了**不挡**这一页 |
| GET | `/admin` | 管理员（`LAUNCHBAY_ADMIN_TOKEN` 令牌或 `lb_admin` cookie） | 200 HTML（no-store） | 管理页读侧：一行一个档案 + 条目数 + rev + 最近改动（`modified`，RFC3339 UTC，坏档读不出来为空、显示 `—` 不编造），**不含条目内容**；坏档案照占一行、标「数据文件读不了」、不给「进入编辑」；④.5 起这一页带一张建档表单（读侧 + 建档）。动作格只有**账号层面**三枚：**设 PIN**、**停用/启用**、**删档**——**「进入编辑」已下线**（它就是 `POST /switch to=<id>`）；坏档那一格是文字「要在这台机器上修好那个文件」，停用档带「已停用」徽标。**没有管理权时不是 303，而是 200 渲染登录页**（2026-10-12：管理员不再是某个档案，浏览器那条路需要自己的凭证；沿用旧的裸 303 形状会让升级之后没有任何人进得去管理页，账号层面整个锁死）。**只有档案会话不算管理权**——登进 dad 也不等于能管这台机器。**AI 不走这条**，要同一份聚合打 `GET /api/v1/status`（同一个 statusRows、两种壳） |
| POST | `/admin/login` | 无（这一条就是"证明你有管理权"的那一步） | 303 回 `/admin` | **管理员的进门**（2026-10-12）。浏览器表单通路，体是 `application/x-www-form-urlencoded` 的 `{key}`。判据只有 `LAUNCHBAY_ADMIN_TOKEN` 一把。`POST /prefs`、`POST /switch` 与 `/api/v1/profiles/self/pin` 同族纪律——**浏览器表单一律 303 拒令牌**，机器建档走 `POST /api/v1/profiles`。key 不对 → 303 `/admin?err=badkey`（不说"这把 key 是管理员的那把"，那等于漏出它的身份）；同一个字段出现两次或表单读不出来 → 422 JSON 外壳。对了 → 发一条 `lb_admin` cookie（**装的是 HMAC 派生值**）并 303 回 `/admin`。表单读不出来与 key 不对是两个形状，别合成一个 303——那会让人以为 key 写错了 |
| POST | `/admin/logout` | 无 | 303 回 `/` | 丢掉管理员身份（2026-10-12）。**只清 `lb_admin`，不动 `lb_profile`**——管理员完全可能同时登着自己的档案在用书签，"退出管理"不该把他从自己的台子里踢出去，那两件事从此分不开 |
| POST | `/admin` | 管理员**cookie**（表单；带令牌一律 303 拒） | 303 回 `/admin` | 管理页建档，体是 `application/x-www-form-urlencoded` 的 `{title, pin, nopass?}`：**没有 `admin` 那一格**（2026-10-12，档案不再有管理员属性，勾了也不会有任何后果）；**PIN 选填**——`nopass=1` 建出不设防的一份，不勾而 `pin` 为空照旧 `?err=badpin`；不问 id（由名字派生 slug，中文走 `home`、`home-2` 基名序列）；字段没填对回 `/admin?err=noname`、`badpin` 或 `nametaken`（派生 id 撞并发提交那一种），**数据目录不可写**回 `/admin?err=permission`（与 `/setup` 同一族同一句话），与进门页同一张白名单，不回显原文，无管理权都是 303 回选择页且磁盘不变；**AI 要建档走 `POST /api/v1/profiles`**，别打这条 |
| GET | `/admin/pin` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 200 HTML（no-store） | 给某个档案设 PIN 的**第一步**：读 `?profile=<id>`，回一张写明后果的确认页。**两步二次确认、零 JavaScript** —— spec §8.8 原写「表单发不出，随 ⑤ 的 JS 落地」，2026-10-09 改判为用两步页面做，于是关掉 JS 时照样成立。字段标签上写着**留空 = 取消这一份的 PIN**（2026-10-12），后果说明那一段重复一遍。目标就是当前会话正在用的那一份 → 303 `/admin?err=selfpin`（改自己的 PIN 要旧 PIN，走 `POST /api/v1/profiles/self/pin`；这条要让开，否则它就是绕过旧 PIN 的后门）；档案不存在 → 303 `/admin?err=notfound` |
| POST | `/admin/pin` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 303 回 `/admin` | 设 PIN 的**第二步**，体是 `application/x-www-form-urlencoded` 的 `{profile, new}`（**`new` 留空 = 取消这一份的 PIN**，2026-10-12，与 `POST /api/v1/profiles/pin` 同一个 `adminPin` 落点、同一处 `isNoPass` 判据）。**不是第二条 API 面**：校验→哈希→锁内读写→落审计行与 `POST /api/v1/profiles/pin` 走同一个落点，差别只在怎么读入、怎么答出。成功 303 回 `/admin`；字段没填对 303 回 `?err=badpin`；锁内读失败 404/503/500；落盘或审计失败 303 回 `?err=failed`（**绝不说成功** —— 那时 PIN 已改而审计行没落） |
| GET | `/admin/disable` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 200 HTML（no-store） | 停用/启用的确认页（第一步）。**停用不是删档**：数据与快照一个字节不动，仍占 `profiles/` 的目录数（引导窗口不重开），但输对 PIN 也进不去、选择页上不再出现这一份。目标就是当前会话正在用的那一份 → 303 `/admin?err=self`（防手滑）；不存在 → `?err=notfound`；坏档 → `?err=notfound`（改不了 profile.json 就翻不动这个标志） |
| POST | `/admin/disable` | 仅管理员**会话**（表单） | 303 回 `/admin` | 停用/启用的第二步，体是 `{profile}`。判据=取反（今天是停用就启用，反之亦然），落点与 `POST /api/v1/profiles/disable` 同一个 `adminDisable`。**幂等**：同状态再设一次不写盘、不记审计，所以重试安全。落盘或审计失败走 500 JSON 外壳，**绝不说成功** |
| POST | `/admin/language` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 303 回 `/admin` | 改全机**界面语言**（多语言，2026-10-11）。体是 `{language}`，取值 `zh`\|`en`。**语言是站级设置不是档案字段**：落 `data/settings.json`，不进 `profile.json`、不走 `/prefs`——那两条都只改自己，而语言改了立刻影响这台机器上所有人；把语言放进档案等于把"改全机显示"的权力交给任一份档案的所有者。没给 → 303 `?err=nolang`；给了不认识的值 → 303 `?err=badlang`（**不静默落默认**：静默存下去等于"我选了法语，它没告诉我没生效"）；落盘失败 500 JSON 外壳，**绝不说成功**。渲染缓存**不用手动清**：`cacheKey` 带语言这一维，改完下一次渲染的键就变了。另有一条入口是零档案时的 `POST /setup`（建档那趟顺手定下语言） |
| POST | `/api/v1/profiles/disable` | 管理员（`LAUNCHBAY_ADMIN_TOKEN`） | 200 `{profile, disabled}` | 停用/启用某档案（机器那一面；JSON 体 `{profile, disabled}`）。**`disabled: true` = 停用**（进不去、数据不动、仍占目录数），`false` = 启用。只管账号，与 `*` 令牌同族；写书签一律要档案作用域令牌 |
| POST | `/api/v1/import` | **档案作用域**令牌 | 200 `{profile,count,skipped,groups,report,token}` | **给浏览器扩展用的第一步**：读得到浏览器的整棵书签树，读不到会话 cookie，所以只能带令牌来。体是**原始 Netscape HTML**（`Content-Type: text/html`），不是 multipart——扩展在浏览器里就把树拼成了那个格式。**目标档案从令牌作用域推导、不接受申报**：一枚 `name:secret:dad` 的令牌只能导进 dad，这不是"懒得校验"而是更强的形状——申报字段根本不存在，"令牌与档案对不上"在协议层就无法表达。`*` 令牌一律 403（它只管账号）。**不写盘**，写入在 `/api/v1/import/apply`；读出 0 条当场 422（`Convert` 不拒绝非 Netscape 输入，不判就会出现"导入成功 0 条"）。`skipped` 是**没能进来的条数**（过不了 `model.CheckURL`：空、解析不了、缺主机名——**`file:` 不在其列，本地路径照收**，2026-10-12），逐条明细在 `report` 的"## 没能进来"一节——扩展只显示一行结论，这个数就是它唯一能说的话。`report` 的**句子**按 `Accept-Language` 分读者（2026-10-13）：不发这个头的脚本与 CLI 拿到的仍是逐字不变的中文那一份，带它的浏览器扩展拿到那台浏览器界面语言那一份，因为那份报告是原样贴到扩展设置页下面给人看的；**判定、数字与那一节的结构两种语言里同一份** |
| POST | `/api/v1/import/apply` | **档案作用域**令牌 | 200 `{profile,rev,applied,warnings}` | **第二步：确认写入**。体是 `{token}`，令牌取走即删所以重放写不进第二次；落盘走 `applyBatch`——与 `POST /api/v1/batch` **同一个落点**。令牌过期或已用过是 **410 Gone**（资源确实没了，且正是"别拿同一枚重试"的信号；报成 400 会诱使调用方再发一次）。审计线 `by` 记 `extension` |
| GET | `/admin/delete` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 200 HTML（no-store） | 删档的**第一步**：读 `?profile=<id>`，回一张**把后果写死在页面里**的确认页（条目数与快照数取自 `statusRows`，与 `/admin` 那张表同一趟遍历）。删档不可逆，`revert` 救不了它——快照跟着档案一起没了。两道形状闸判据是"这台机器之后还能不能管"：目标是当前会话在用的那一份 → 303 `?err=self`；**没有"最后一个管理员"那一道**——权限只看 `lb_profile`，能进这页的人本身就是管理员，"唯一的管理员"只可能是他自己在用那一份（已被 `self` 拦） |
| POST | `/admin/delete` | 管理员 **cookie**（表单；带令牌一律 303 拒） | 303 回 `/admin` | 删档的**第二步**，体是 `application/x-www-form-urlencoded` 的 `{profile}`。**不是第二条删档通路**：ASCII slug / 目录须是 `profiles/` 下的同名直接子目录 / 档案写锁三道闸全在 `store.DeleteProfile` 内部，端点层不再"规范化" id（那等于绕过闸）。`history.jsonl` 的行保留（删档不抹审计线）。成功 303 回 `/admin`；不存在 303 `?err=notfound`；**落盘失败走 500 JSON 外壳** —— 删档失败必须说得清 |

## 错误码（每个 JSON 错误都带 `kind`）

**JSON 错误外壳是 `{code, kind, detail, currentRev?, errors?}`**，键序由 struct
字段序固定（§4.3 那条对响应同样成立）。`kind` 是机器可读的那一格，`detail` /
`errors[].reason` 仍是给人排查的中文原话——**两边各管各的读者，谁也不替换谁**。

为什么状态码不够：422 既可能是 `missing-target`（那一组已经不在了，该说"刷新看看"），
也可能是 `duplicate-id`（这一条已经存过了，该说"去改它，别再存第二遍"）。两句出路
相反，而按状态码选句子只能说中一句——2026-10-14 实测到的正是这个：重复添加同一个
网址的人，屏幕上写的是"东西没了，刷新一下"。

**batch 的 422 读法**：外壳那一格说的是**这一族**（`ops-invalid` = 整批未写入），
具体哪一条、为什么在 `errors[0].kind`。客户端读 `errors[0].kind`，没有明细才退回
外壳那一格。

**三处读者各自从码选句子，`detail` 不再上屏**：主站横幅（`app.js` 的 `KIND_KEY`）、
扩展通知（`extension/i18n.js` 的 `KIND`）、CLI 的 stderr（`[kind]` 前缀 + 中文原话）。
句子表是三份不是一份——它们本来就该不一样（横幅要说"去页面上改它"，stderr 要说
哪个字段），但**"每一个码都有交代"这一条是同一颗闸**：`appi18n_test.go` 与
`extcatalog_test.go` 各自扫 `internal/apierr` 那份登记表，收不到的码必须写着
为什么收不到。中文原话退到浏览器 console 与容器日志——那里是给排查的人读的。

**这套码只增不改不删**：改一个已发布的字面串等于让旧版扩展与脚本静默落进兜底句子，
那比多一个码糟得多。要换说法就改中文原话，别动码。三处（代码常量、
`openapi.json` 的 `ErrorKind` 枚举、本节这张表）由
`internal/httpapi/kindsdoc_test.go` 逐码钉相等——文档骗 AI 比骗人更贵。

| kind | 默认状态 | 这一类错误该让人做什么 |
|---|---|---|
| `body-unreadable` | 422 | 重新提交一次 |
| `credential-admin-only` | 403 | 这一条要管理员身份，档案令牌不行 |
| `credential-missing` | 401 | 把凭证带上：浏览器那条路是登录，扩展与脚本那条路是 Bearer 令牌 |
| `credential-scope` | 403 | 换一枚作用域正好对上这个档案的令牌（管理员那把只管账号） |
| `credential-unknown` | 401 | 这一把不被认：多半是少复制了字符，或者机器那边换过令牌 |
| `duplicate-id` | 422 | 这一条已经在了：去改它，别再存第二遍 |
| `field-duplicate` | 422 | 同一字段只带一次 |
| `field-invalid` | 422 | 改那一格的值 |
| `field-unknown` | 422 | 删掉不认识的字段再发 |
| `host-invalid` | 400 | 换个地址访问 |
| `icon-unknown` | 422 | 换一个查得到的图标名 |
| `import-no-bookmarks` | 422 | 确认选的是浏览器导出的那份 .html |
| `import-nothing-accepted` | 422 | 这份文件里的地址都进不来，换一份 |
| `import-plan-stale` | 410 | 重新发起一次导入，拿一份新报告 |
| `internal-error` | 500 | 稍后再试；持续发生看容器日志 |
| `invalid-request` | 422 | 看 detail 说的那一格 |
| `missing-target` | 422 | 那一组/那一条不在了：刷新之后再看 |
| `ops-invalid` | 422 | 整批没写入；看 errors[] 逐条的 kind 与 field |
| `pin-wrong` | 401 | 再输一次；忘了就走管理页那格设新 PIN |
| `profile-broken` | 503 | 先修好那个数据文件（管理页会说要重建） |
| `profile-disabled` | 401 | 这一份被停用了：去管理页点「启用」，或者换一份进 |
| `profile-exists` | 409 | 换一个名字或 id |
| `profile-not-found` | 404 | 确认这个 id 还在，或先建一份 |
| `profile-not-rebuildable` | 409 | 这份读得动，不需要重建；改 PIN 走 profiles/pin |
| `query-invalid` | 400 | 把查询参数补全/改对 |
| `rev-conflict` | 409 | 重读一次再写（别人刚动过） |
| `route-not-found` | 404 | 这条路径不存在（浏览器导航到它拿到的是同一句话的人话版页面，`Accept: text/html` 是唯一分界；状态码与 kind 两边相同，脚本不发这个头就永远拿 JSON） |
| `snapshot-failed` | 507 | 什么都没改；腾出空间或稍后再试 |
| `target-unreachable` | 502 | 那台机器可能没开；照样存下来也行，回头补标题 |
| `url-incomplete` | 422 | 地址要有协议和主机名 |

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
（**仅管理员 cookie，带令牌一律 303 拒**）。改完**立刻生效**：渲染缓存的 `cacheKey`
带语言这一维，所以不需要手工清缓存——「改完遍历清一遍」是要记得做的事，
进键是不用记得的事。

**浏览器扩展那一侧不跟这个站级设置**：`extension/_locales/{zh_CN,en}/messages.json`
按**浏览器自己的界面语言**取（`default_locale: "zh_CN"`，manifest 的
`name`/`description` 走 `__MSG_key__`）。理由是它读不到这台机器的界面语言——那要一次
带凭证的请求，而"这一页说什么"不该依赖一次网络成功；装扩展的人就坐在那台浏览器前面，
浏览器语言就是他正在读的语言。两份 catalog 各管各的，**不许互相"统一"**：主站那份归
`internal/i18n`，扩展那份由 `internal/i18n/extcatalog_test.go` 钉（两种语言 key 一一
对应、占位符逐位相同、代码与页面只认 key、`extension/` 里不许出现中文句子）。扩展是
ES module（`background.type: "module"`）——MV3 的 service worker **没有
`importScripts()`**，四份脚本靠 `import './i18n.js'` 共用同一份句子层。

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
字段校验失败的原因、**接口与 CLI 那一份导入报告**、OpenAPI 的端点描述——它们固定中文，因为调用方
要的是稳定可对照的文本。2026-10-13 把这条豁免**收窄到"给脚本读的那份"**：同一个字符串也印在
家人看的 `GET /import` 那一屏上（实测英文站上那一屏仍有 453 个汉字，节名一字未改，只有按钮翻了），
而那句理由从来不属于那一屏。现在浏览器那一屏的报告句子按站点语言查表（`bookimport.Report.Words`），
`/api/v1/import` 那一份按**调用方是不是浏览器**分：不带 `Accept-Language` 的（curl、Go 的
http.Client、这台机器的 CLI 都不发这个头）拿到的仍是逐字不变的中文锚点，带它的浏览器扩展
拿到的是那台浏览器界面语言那一份——那份报告是原样贴在扩展设置页下面给人读的（判据
`internal/httpapi/lang.go` 的 `browserLang`：只看第一个标签，认不出落默认，不 406）。
两路共用同一份结论，翻的是句子不是判定；
**写路径的判定原话**（"没有分组 x"、校验理由那一类）两路都不翻，它们与 detail 同族，两份就会漂。
缺 key 时 `T` 返回包着尖括号的 key 自己（不是空串），所以
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

   **`set` 的字段面是一张白名单**——这是 ops 引擎的第一道闸（`internal/ops/path.go`
   的 `groupFields` / `itemFields` / `docFields`），因为"能 set 什么就是写接口的攻击面"：
   **`rev` 不许被 set 改**（它是并发判据本身，expectedRev 跟它比）、**`id` 不许**
   （改了等于换了一个寻址键，历史与快照会指错条目）、**`groups` 不许整表替换**
   （§4.4 明写不给整表替换型 op：一次换整表会把别人的并发写入抹掉；加一个分组用
   `add path=groups`）。`search` / `weather` 同样不在表内：它们是对象不是标量，而且
   **页面一次都不渲染**（最初计划里的搜索引擎与天气，没有做）。

   **根级标量字段可以 set**（2026-10-14 改判）：`title` / `footer` / `openInNewTab` /
   `clock`，写成 `{"op":"set","path":"openInNewTab","value":true}`。此前那道闸是
   「路径必须以 groups 开头」——它是上面这条原则的**粗粒度实现**，顺手把整张文档的
   标量一起挡在外面，代价是 `openInNewTab` 这种**页面上真的画得出来的**字段一个都
   改不了（2026-10-10 逐条探过、六条全拒，而那六条里有五条本来就不该拒）。现在它
   与分组/条目字段是同一套白名单机制，`rev`/`groups`/`search`/`weather` 仍然拒，
   而且**每种拒法给自己的理由**（调用方改不动时要知道是"字段不对"还是"这条路有纪律"）。
   `openInNewTab` / `clock` 是**布尔**，写成字符串 `"true"` 会被拒——页面上
   `{{if .OpenInNewTab}}` 只认真布尔，字符串会被静默当假，而那一发 op 是绿的。

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
   `layout` 只收 **`tiles` / `list` / `feature`**，别的值报「新分组的 layout 只能是
   tiles/list/feature」；`accent` 可选（颜色），`items` 是条目数组。这三格都对着
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

**但这条严格只挂在写侧**（2026-10-12 改判）：`internal/model` 里那道
"前缀必须是 tabler/simple-icons"的校验**整条删掉了**，因为 `model.Validate`
同时跑在读路径上，于是磁盘上一个不认识的图标名拥有让一个人**全部书签打不开**
的权力（症状：整档 503「数据文件读不了」）。这台机器的前身（NAS 上的 flare）
图标名整份是 `mdi:` 前缀，恢复旧文件的人撞的就是这一枪。现在读侧对 icon
一个字都不验，渲染层自己兜底（`icon.go` 的 `pieceFor`：索引查不到 → 首字母色块
+「图标未匹配」徽标，计入 `GET /api/v1/status` 的 `iconUnmatched`）。这与
坑 #3 摘掉 `search.default` 致命校验是同一条判据：**一个界面上不存在的功能
不该有毁掉别人台子的权力**。

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
  例地址；``data/`、`.superpowers/` 都不进仓库（`.env` 同理：2026-10-12 起 compose 已经不用它了，但
  谁要是自己又建了一个，也不许进仓库）。
- 不提供 MCP server：REST + curl 已够用，少一个常驻件。