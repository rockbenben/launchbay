# 全量审计（第二轮）：色板刷新之后的注释、文档与契约

日期：2026-11-19（下午）
基线：`c0c5730`（`style(ui): refresh the palette and the profile page chrome`）
范围：与第一轮相同——代码注释、`docs/`、`AGENTS.md`、`README.md` / `README.zh.md`——外加第一轮之后新增的色板、顶栏与选择页改动。

第一轮记录见 [audit-2026-11-19.md](audit-2026-11-19.md)。那一份写在色板刷新之前，里面的色值比例是**当时**的；本轮不回头改历史数字，只把被 `c0c5730` 推翻的现在时句子改对。

## 方法

五个只读子代理分别扫 `internal/httpapi`、`internal/render`（含 JS/CSS/模板）、`internal/{store,model,auth,ops,i18n,bookimport,icons}`、`cmd/launchbay` + `extension` + `tools` + CI/部署、以及契约文档（`AGENTS.md` 与两份 README）。每个代理只报**能用当前代码复核**的条目，不改文件；主代理逐条回到代码、测试或资源文件复核之后才落笔。

判据仍是那句：**只修“现在这句话是假的”，不改写历史**。带日期的旧判断、旧数字、旧形状保留；只有当它被写成现在时、而现在已经不成立时才动。

## 一、门禁：对比度判据搬回 render

`c0c5730` 把暗色底从 `#0b0e14` 改成 `#131b29`、亮色底从 `#f6f4ef` 改成 `#f0f4fa`，而 `internal/model/palette_test.go` 里那条对比度测试仍然拿着两个**写死的十六进制常量**算比例——它测的不再是页面真正用的底，改色板的人也不会被它拦住。

- 把对比度判据搬到 `internal/render/views_test.go`：`TestCSS_分组色板在两种页面底上都过3比1` 直接读 `web/base.css`，分别按暗色、显式亮色与媒体查询亮色三档取 `--bg`，再与 `model.GroupAccentPalette` 比。**判据只有一份，底也从活的 CSS 里读**。
- `model/palette_test.go` 只留两件事：色板不许退回亮档看不见的那一批、色名与色值一一对应；注释指向 render 那条测试，避免下一个人在这里再抄一份 `--bg`。
- `base.css` 里被这次刷新推翻的现在时数字逐条重算：`--scrim` 的 0.84 档从 9.13 改成 9.36、0.87 档 13.73；亮色底、ring/accent 的 4.42/5.26；`bad-text` 的卡片比例 3.95 → 5.79；节距 13px → 20px；设置面板小标题 13px/大写 → 15px/650。
- 引用行号改成引用规则原文（`section{margin:20px 0}`、`h2{font-size:14px…}`）。行号在注释变长之后会漂，这一轮已经漂过一次。

## 二、契约（`openapi.json` / `AGENTS.md`）

| 原来写的 | 实际 | 修法 |
|---|---|---|
| 停用自己正在用的那一份 → `/admin?err=self` | `disableapi.go` 发 `selfdisable` | 两处描述统一成 `selfdisable` |
| `model.CheckURL` 拒 `file:` | `file:` 是显式放行的例外 | 删掉 `file:`，只留 `javascript:`/`data:` |
| 删档权限看 `lb_profile` 指向的档案在磁盘上是不是管理员 | 管理权只由机器级 key / `lb_admin` 判，档案没有 admin 属性 | 改成机器级 key 的说法 |
| `usage` 上限「五千条」 | `usageMaxIDs = 500` | 改成五百条 |
| 建档只有 `POST /api/v1/profiles` 与 `POST /setup` | 还有 `/api/v1/profiles/new` 与 `/new` | 四条通路都点名 |
| `nametaken` 只有管理页会发 | `/new` 也发；`/setup` 撞车仍回 `/` 不带键 | 改发件人名单 |
| `/profiles/new` 与 `/profiles` 的差别含「没有 admin 那一格」 | 两条都没有 admin 那一格；真正差别是免凭证与 id 派生 | 改差别清单 |
| rebuild 体「三格」 | 四个字段（`nopass` 选填） | 改四格 |
| `wantsNoPin` 三处调用（含建档族的 nopass） | 只有 self/pin、admin/pin 两处；建档族走 `NoPass` 布尔 | 改两处并说明 |
| 建档表单「两条」 | `/setup`、`/admin`、`/new` 三条 | 全改三条 |
| 词汇表「两个页面」 | 管理页、三张确认页与选择页都用 | 改「几页」并点名 |
| `adminPageGuard`「这两页」 | 设 PIN / 删档 / 停用三族六处 | 改「这几页」 |
| 停用见 `deleteBlocked` | 停用走 `disableSelfBlocked` | 两个函数名分开写 |
| `selfProfileOnly`「六条」且含 `/prefs/pin` | 七条；`/prefs/pin` 直接 `require(credSession)` | 改七条并说明 |
| `icons/tiles`「空查询回一张空图」 | 空串直接 400（JSON 那条与 tiles 都是） | AGENTS、openapi、`iconsearch.go` 的 detail、`iconstarter_test.go` 与 `app.js` 的转述全改 400 |
| 被停用档案「令牌那一族一律 403」 | 只有 data/export/backup/batch/revert/import/import-apply/restore 八条有闸；usage/resolve/history 仍可带停用档案令牌 | 收窄成「八条内容通路」，并点名那三条不碰 `bookmarks.json` |
| 管理页建档错误键只列 `noname/badpin/nametaken` | 还会发 `pinboth`/`titlelong` | 补齐 |
| `/admin/pin` 锁内读失败 404/503/500 | 这条是 404/500；503 在机器端点 | 改 404/500 |
| `/admin/disable` 坏档回 `notfound` | GET 照画确认页；POST 读失败才 404/503/500 | 分开写 |
| CLI `import-bookmarks` 打 `/import`+`/import/apply` | 它走 `GET /data` + `POST /batch`；那两条 import 是 `credTokenOnly`，只给扩展 | 改端点 |

其中**没有改行为**的一条要在以后决定：停用档案仍能用令牌打 `usage`/`resolve`/`history`；这是权限模型的决定，不是这一轮顺手能改的。文档现在说的是实际边界，不再用「一律」把洞盖住。

## 三、README：对外事实

- CLI 管理账号那一格：原文说 `launchbay profile …` 能建档、删档、设 PIN、停用；实际只有 `profile create` / `rebuild` / `fix-names`。删档、设 PIN、停用只有 `/admin` 与 API。
- 删档那条「CLI path」改成「machine path」——`DELETE /api/v1/profiles` 是 HTTP 端点，不是 CLI 子命令。
- 档案作用域令牌的使用者补上 `profile fix-names`（它确实走逐档案换令牌）。
- 「唯一的外部请求是抓一次 favicon」改成「抓一次目标页（取标题）和它的 favicon」——`/api/v1/resolve` 两步都做。
- 「限流是为了不让人拿这台机器当探测跳板」删掉——`internal/icons/fetch.go` 明写这里不设速率限制，只有 24 小时冷却。
- 发布标签 `0.1.0`/`0.1`/`0` 改成当前形状 `<version>`/`<major>.<minor>`/`<major>`（今天 `0.3.2`/`0.3`/`0`）。
- 两份语言同步改；结构未动，repo-facade 的 `check-readme.mjs` 仍全绿。

## 四、代码注释与用户可见文案

**render**

- 分组色的挂载点从「两处」改「三处」（大卡组标题、正文 section、侧栏色点）——`c0c5730` 给大卡组标题加了 `--sec-accent`，旧注释还写着「没有第三个」。
- `AdminSentinel` 的注释不再说「SW 还不存在」：`/sw.js` 已启用，且 `storeHTML` 对 `-admin` 一律不存。
- `app.js` 开头「所有写入都经 batch」收窄成「书签数据的写入」；台面偏好、背景与备份导入各有表单落点。
- 「这一族是唯一必须整页重载的」删掉「唯一」；`/prefs` 其余几格与备份导入也重载。
- 拖放委托那段：`profile.tmpl` 有两个 `<main>`（档案页 142、`/prefs` 独立页 1129），旧的「只有一个 / 第 120 行 / 816 行」全错。
- 选择页卡片宽度 150–190px → 约 218px（`c0c5730` 的 `.picker-page .profiles>li`）。
- 图标选择器列宽 56 → 72；布局键的命中区从「盒子 44」改成「盒子 32×28 + `::before` 撑到 46×46，上边被 section 裁到实测 45×34」。
- 密集档的 `minmax(224px,1fr)` 改成实际值 200。
- `profile.tmpl` 的「其它四张一级页」改成五张（括号里本来就列了五张）。

**core / httpapi / CLI**

- `auth.redacted` 对**只有一段**的输入改为整段抹掉：那一段可能是名字，也可能是有人只粘了密钥；旧注释说「第一段不是秘密」，在单段输入上不成立。新增测试钉住。
- `model.CheckProfile` 里「PIN 拼错唯一出路是 rebuild」改成「rebuild 会 409；用 `/profiles/pin` 重设」——Validate 不校验 PIN，档案读得动。
- `i18n` 包注释「缺字段=中文」改成实际链条：这一份没选过 → 站级默认 → 浏览器语言 → en。
- `ops` 的布尔字段注释「`decode` 对 `"true"` 与 `true` 都放行」反了：JSON 字符串会被 `encoding/json` 拒掉；错误文案也从「会静默当假」改成「会被拒」。
- `ops` 路径注释「单段 groups 是 add/remove」改成「只有 add 用；删组走 `groups/<gid>`」。
- `store.LoadProfileWithFingerprint` 的「只有一个调用点」改成两个（档案页与台面面板）。
- `bookimport`：重复标题实际**不合并**，报告语言包却写「已合并成一个」——两份语言包改成「已分开建组」；对账失败文本里的 `flare`/`新库` 是上一个项目的残留，改成 `文件`/`导入结果`；`Specials` 注释从「四类」收窄到实际只填非 http 协议。
- `ErrNoAdminToken` 的文案不再说「没人能建档」——免凭证的 `/new` 仍可建；改成「没人能删档、给别人设 PIN 或重建坏档」。
- `fix-names` 的注释删掉「可以不带协议」：代码要求名字与地址去掉尾斜杠后逐字相等，`example.com/x` 对 `https://example.com/x` 不匹配。行为未改。

**extension / tools / 部署**

- `currentProfile` 在默认档案缺令牌时会回退到「第一枚令牌」——多枚令牌时会把书签静默写进别人的档案。现在默认档案明确指定就只认它；缺令牌由 `saveOne` 报 `noToken` 并点名这一份。设置页文案同步。
- 设置页「试一下」的 403 原先一律说「令牌与档案对不上」；改用 `httpErrKey` 按 kind 分：停用、管理密钥、作用域不符各说各的。文案同步。
- `manifest.json` 原来没有 `action`，`bg.js` 却在每次成功后调 `chrome.action.setBadgeText`——`chrome.action` 不存在，成功路径会抛异常、反被当成失败。补 `action`（沿用现有图标），点工具栏图标进设置页；这是本轮唯一的扩展行为改动，需要在 Chrome 里做一次手工烟测。
- `tools/uicheck` 的注释：图标面板打开时先铺 starter/browse，搜索结果才要搜；共享 harness 从「两个脚本」改成四个；fixture 从四组改成五组；`sweep-variants` 引用了一条不存在的 `prefs.addrLevel.hint`，改成引用实际文案并说明设计判据不靠它背书。
- `Dockerfile`「包比仓库里少一半」改成少 `tools/` 那四个（约四分之一）。
- `docker-compose.yml`、`cmd/launchbay/main.go`、`env.go` 的令牌使用者名单补上 `profile fix-names`。
- `cmd/launchbay/import.go` 的 `--by` 说明补上 `bookmarklet`（`store.ByValues` 六项）。

## 五、验证

- `go test ./... -count=1`：全绿（含 `internal/render` 的新对比度测试、`internal/auth` 的单段抹除测试、`internal/i18n` 的扩展词表守恒）。
- `go vet ./...`：无输出。
- `gofmt -l .`：无输出。
- `git diff --check`：无空白错误。
- `node .../check-readme.mjs .`：`✓ 042-launchbay 完全符合 SKILL.md`。
- `TestAgentsMDMatchesRoutes` / `TestOpenAPIHasNoGhostEndpoints` / `Test静态资产指纹契约一致` / `TestExtension_*`：全绿；`app.js`/`app.css` 的注释改动之后重新同步过 OpenAPI 里的指纹。

**没做 / 没验**：

- 没有在 NAS 真机上验收；没有重跑桌面/手机的 render-probe（本轮只动注释，没有改 CSS 值；`c0c5730` 的实测数字沿用）。
- `docs/og.html` 的卡片早于色板刷新，本轮只在源文件里注明「颜色是当时的」，没有重做——GitHub 的 social preview 上传即覆盖，旧图找不回。
- 两份 README 的首屏截图也早于色板刷新；没有重新生成（要起带种子的服务与 Chrome）。
- 扩展本轮没有在 Chrome 里加载；`manifest.json` 新增 `action` 之后需要一次手工烟测（右键收藏成功时角标是否出现、点图标是否进设置页）。
- 停用档案仍能用令牌打 `usage`/`resolve`/`history`，文档已按实际收窄，行为未改。
