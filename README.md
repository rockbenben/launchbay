# launchbay

> 你的常用页面，打开就点。

[![License: MIT](https://img.shields.io/badge/License-MIT-1f6feb)](LICENSE) [![365 开源计划 #042](https://img.shields.io/badge/365%20%E5%BC%80%E6%BA%90%E8%AE%A1%E5%88%92-%23042-1f6feb)](https://github.com/rockbenben/365opensource) [![ci](https://img.shields.io/github/actions/workflow/status/rockbenben/launchbay/ci.yml?label=ci&branch=main)](https://github.com/rockbenben/launchbay/actions/workflows/ci.yml)

[`docker compose up -d`](https://github.com/rockbenben/launchbay#快速开始) → 浏览器打开 http://localhost:7600 · 镜像 ≈ 20MB（实测构建 13MB）

![launchbay 档案页：顶栏搜索、侧栏分组、磁贴与单列两种形态](docs/home-1280.webp)

把记不住的那些地址放进一页：NAS 管理后台、路由后台、公司 OA、监控面板、常去的网站。
把它设成浏览器主页，之后每次点开就是它——**一点就到，不用等它慢慢转圈**。

跑在你自己那台机器上：家里 NAS、树莓派、软路由，或者公司内网的一台服务器都行。数据不出你的局域网。

## 你会在意的三件事

- **打开不用等。** 整页就是一个网页文件，书签和常用图标直接画在页面里，不用下载图片、不用等脚本跑完才显示——同类里要先跑脚本才把链接显示出来的占大多数；纯静态、不需要脚本的那一类确实也快，可它连编辑界面都没有，只能手改配置文件。实测：本机 25 条书签首屏 LCP 77ms、CLS 0.00，首屏 gzip 后 7.4 KB（未压缩 31.4 KB）；定下的目标是局域网首屏 LCP < 300ms，这条要真浏览器真网络才量得出来，NAS 真机冷启动还没实测（见「限制」）。服务器临时打不开，浏览器里的缓存还能接着看。
- **各人一份，别人看不见。** 每人一个页面，进门输一个 4–6 位数字。共用一台电脑不串号——**管理员也看不见你的**：他能建档、删档、给你改密码，但读不到你页面上装了什么。这不是靠前端藏，服务端就是分开的：管理员那把密钥去读别人的书签会被直接拒掉（403）。要说清一句：这不是密码学意义上的"防他"——管理员给你设 PIN 时就知道那个 PIN，局域网里改个 cookie 也能冒充谁；这里指的是**没有一条通路**让他读到你的书签。
- **改坏了能退回去。** 每次批量改动前自动留一份备份，顶栏点「回退」就能看到每一个旧版本长什么样（多少条书签、哪些分组），认出来再点一下就回去——**现在这份也会先存一份**，所以退错了还能再退回来。历史留着不抹。备份就是磁盘上的一个普通文件夹，整个拷走就是全部。

## 支持范围

| 方面 | 认不认你的 |
|---|---|
| 怎么跑 | `docker compose up -d`，或直接跑一个静态二进制（amd64 / arm64） |
| 跑在哪 | 家里 NAS、树莓派、软路由，或公司内网的一台机器 |
| 谁在用 | 一个人、一家人，或一个不想装一堆东西的小团队——每人一份，管理员管账号不管内容 |
| 浏览器 | 任意现代浏览器，**关掉 JavaScript 全部书签照样看得见、点得动** |
| 要装什么 | 除了 Docker 本身没有别的：零第三方库、零数据库、构建期不需要 node |
| 数据在哪 | 一个普通目录 `data/`，备份 = 整目录拷走 |
| 界面语言 | 中文与英文。**装的时候顺手选**，之后只有管理员能改，改完全机立刻生效 |

## 能做什么

- **分好组、搜得快**：书签按分组归类，顶栏直接搜名字、地址、标签——填完按回车就出结果（全程不需要 JavaScript）；开着脚本时按 `/` 直接跳到搜索框，或者用 `⌘/Ctrl K` 面板
- **从浏览器搬进来**：Chrome / Edge / Firefox 导出的书签文件传上来，先给你一份对账报告——哪条进来了、哪条没进来、为什么，都点名；你看过确认了才写进磁盘
- **看到就随手存**：拖一个 bookmarklet 进书签栏，任何网站上一点就把地址带回来；要你再按一次保存才真正落库
- **图标离线也有**：两套图标索引（tabler + simple-icons）gzip 后嵌进二进制，装的时候不联网、运行时也不为出图标去请求第三方
- **也能让脚本和 AI 代劳**：REST 接口 + 按档案作用域的令牌，浏览器扩展、命令行、自动化脚本各拿一把自己的钥匙；令牌越界写别人的档案会得到 403，不是写进去

## 快速开始

```bash
git clone https://github.com/rockbenben/launchbay
cd launchbay
docker compose up -d
```

三条命令，不需要 `.env`，不需要装 Go，镜像直接拉。**令牌就写在 `docker-compose.yml` 里**
（带能直接用的占位值），所以这一趟就能起来；镜像用 `:latest`，**首次** `up` 拉的就是最新一次
发布——已经跑着的机器要先 `docker compose pull`（原因见下面的「镜像从哪来」）。

> **那两个占位密钥是公开的**，写成 `change-me-…` 就是提醒你该换。同一局域网里认得这个名字的
> 人，能拿管理员密钥建档、删档、给别人改 PIN——但**读不到任何人的书签**，那要档案
> 自己的 PIN，而 PIN 只有档案主人知道。这台机器本来就设计成只在局域网内用
> （见下面的「限制」）。要把它开给局域网以外的人，打开 `docker-compose.yml`，把这两个
> `change-me-…` 换成自己的，然后重起：
>
> ```yaml
> LAUNCHBAY_ADMIN_TOKEN: "你自己的管理密钥"
> LAUNCHBAY_WRITE_TOKENS: "ai-dad:你自己的密钥:dad"
> ```
>
> ```bash
> docker compose up -d
> ```

打开 http://localhost:7600 会进**一次性建档页**：给第一个档案起个名字、设 4–6 位 PIN。**没有默认账号密码**——没有 `admin/admin` 可试，你填进去的名字和数字就是你自己的。这个页面只在磁盘上一个档案都没有时有效，建完就永久关闭——所以"这台机器建过档了没有"本身就是一条线索，不会被匿名访问者拿来数你家里有几口人。

建出来的**就是第一个人的书签页，不是管理员**。管这台机器的是 compose 里那把 `LAUNCHBAY_ADMIN_TOKEN`：打开 `/admin` 输它一次，服务端发一条只属于管理身份的 cookie（30 天），之后管理页就是你的。

同一页上还有一个**界面语言**下拉框（中文 / English）——**装的时候顺手在这里选**，填完 PIN 直接点创建容易把它跳过去。选完立刻生效，之后只有管理员能在管理页改（它属于整台机器，不属于某个档案）。

### 镜像从哪来

compose 只管**拉**已发布的镜像，构建是 CI 的事：推 main 或打 `v*` tag，
`.github/workflows/docker.yml` 会构建并推出 amd64 + arm64 两个架构到 Docker Hub 与 GHCR。
用这台机器的人不需要装 Go，也不用看 `Dockerfile`。

镜像标签是 `:latest`。**第一次** `up` 时本机没有这个镜像，会去仓库拉最新一次发布；已经跑着的
机器本机已经有了，`up` 不会回仓库确认，所以要**先 `docker compose pull`** 才换到新版。

发版靠 **git tag**：打一个 `v0.1.0` 推上去，流水线就构建并推出 `0.1.0`、`0.1`、`0` 与
`latest` 四个标签。这个仓库里没有第二处要改的版本号——`image:` 用的是 `:latest`，所以它不
参与发版，版本号只来自 tag。

不想 clone 也行——直接拉发布镜像（amd64 与 arm64 都出了，Docker Hub 与 GHCR 同名）：

```bash
docker run -d --name launchbay -p 7600:7600 \
  -v launchbay-data:/data -e LAUNCHBAY_DATA_DIR=/data \
  -e LAUNCHBAY_ADMIN_TOKEN="换成你自己的管理密钥" \
  -e LAUNCHBAY_WRITE_TOKENS="ai-dad:换成你自己的密钥:dad" -e TZ=Asia/Shanghai \
  --health-cmd "/launchbay healthcheck" --health-interval 30s \
  ghcr.io/rockbenben/launchbay:latest
```

令牌分成**两个变量**，写死的值就是能用的（密钥是 `change-me-…` 占位）：

| 变量 | 格式 | 给谁用 | 管到哪 |
|---|---|---|---|
| `LAUNCHBAY_ADMIN_TOKEN` | 一把裸 key | 管理这台机器：建档、删档、给别人设 PIN、停用/启用、跨档案看状态与审计线 | **只管账号**——打 `/api/v1/data`、`/api/v1/batch`、`/api/v1/export` 一律 403，读不到任何人的书签 |
| `LAUNCHBAY_WRITE_TOKENS` | `名字:密钥:档案`，逗号分隔 | AI 运行时、浏览器扩展、命令行（成批增删改书签） | 每条只管自己那一个档案 |

浏览器那边进管理页靠的是 `LAUNCHBAY_ADMIN_TOKEN`：打开 `/admin` 输它一次就发一条管理 cookie（30 天）。**档案本身没有"是不是管理员"这个属性**——建出来的每一份都只是一个人的书签页。

> **注意档案 id 是从名字派生的，不一定叫 `dad`。** 建档只问名字与 PIN，id 由名字转
> 来的：名字里有 ASCII 字母数字就取那个 slug（`Dad` → `dad`、`nas box` → `nas-box`），
> **中文名字转不出东西，一律落到 `home` / `home-2` 基名序列**。所以拿中文名字建了
> 第一个档案之后，那枚 `…:dad` 的令牌作用域对不上任何档案，打过去是 403。
> 两种修法：把令牌作用域改成实际的档案 id（推荐），或者第一个档案就用 ASCII 名字。

宿主端口默认 7600；Windows 上 7532–7631 整段被 Hyper-V 动态保留，撞上了就在命令行前缀 `LAUNCHBAY_HOST_PORT=7601 docker compose up -d`（容器内端口不动）。

项目名与容器名都写死在 compose 里（`launchbay`），所以 `docker compose …` 那几条命令跟
clone 到哪个目录无关，而 `docker logs launchbay`、`docker exec launchbay …` 也直接能用。

### 浏览器扩展（可选）

`extension/` 里是一个 Chrome / Edge 扩展（Manifest V3，右键把当前页面存进你指定的那一份档案，或一次收藏当前窗口的所有标签页）。它不在发行镜像里，要自己装：

1. 打开 `chrome://extensions`，右上角开**开发者模式**
2. 点**加载已解压的扩展程序**，选仓库里的 `extension/` 目录
3. 点扩展图标进设置页，填三样：
   - **这台机器的地址**——`http://10.0.0.3:7600`，**不带结尾斜杠**
   - **默认档案 id**——右边令牌对应的那一份（见上面那条"id 是从名字派生的"）
   - **那一枚令牌**——`docker-compose.yml` 里 `LAUNCHBAY_WRITE_TOKENS` 中 `ai-dad:` 后面那一段密钥本身，不是 `ai-dad:…:dad` 整串（三段式 `名字:密钥:档案` 里 `Bearer` 后面只放 `密钥` 那段）
4. 点**试一下这台机器**验一下

那个「试一下」按钮答 403 不是网络坏了，是**令牌与档案对不上**：它拿默认档案去读一次，403 就说明这枚令牌的作用域是别的档案——回到 `docker-compose.yml` 把 `LAUNCHBAY_WRITE_TOKENS` 里 `ai-dad:` 那一段的作用域（末尾的 `…:dad`）改成你实际的档案 id，然后 `docker compose up -d` 重起。

装好之后：任意网页右键 → 存到 launchbay；设置页里还能**把浏览器书签树整个导进来**（第一次点才问你要「读书签」权限，设成按需申请——只为右键收藏装它的人，永远不会看到那个提问；导进去之前先给你一份逐条对账报告）。

> 安装时 Chrome 会警告「读取和更改您在所有网站上的数据」：扩展要能在任意网站上响应右键才需要这条，这正是它的用法。它申请的全部权限是 `contextMenus` / `storage` / `tabs` / `notifications`，`bookmarks` 是按需申请的那一项。
> 令牌只存在这台电脑的 `chrome.storage.local` 里，**不进 sync、不上云**；这个设置页没有导出配置的按钮，正是同一个理由。换档案换令牌，某一台泄露了只吊销那一枚。
> Firefox 未验证——`extension/` 按 Chrome / Edge 写的。

### 数据目录的权限

数据默认落在 `docker-compose.yml` 同目录的 `./data`，容器里永远是 `/data`。想放别处就改
compose 的 `volumes` 那一行，比如 NAS 上常见的：

```yaml
    volumes:
      - /volume1/docker/launchbay:/data
```

Windows 上如果写绝对路径，`D:` 的冒号会和分隔符打架，改用长语法（`source` / `target`
分成两格就没有歧义）：

```yaml
    volumes:
      - type: bind
        source: D:/launchbay-data
        target: /data
```

容器以 **uid 65534** 运行，而 NAS 上那个目录默认是 root 所属的——挂上去之后建档、导入、
写书签会全部失败。**在宿主机上把它交给这个用户**（下面以默认的 `./data` 为例）：

```bash
sudo chown -R 65534:65534 ./data
```

不用 `sudo docker run` 或 `--user root` 绕过（那等于让服务以管理员身份写你家每一个文件）。这条不是猜的：探针每次体检都会在数据目录里建删一个临时文件，权限不对时 `docker ps` 直接显示 **unhealthy**，容器日志里那句会告诉你同一个 chown 命令；页面那头也一样——建档表单回一句「数据目录写不进去，请把数据目录交给 uid 65534 之后再重试」。直接跑二进制（不用容器）的话，那句话里的 uid 换成你自己这个系统用户即可。

## 怎么用

| 你要做 | 去哪 |
|---|---|
| 建档、删档、给别人设 PIN、停用某个人 | 顶栏「管理」，或 `POST /api/v1/profiles` |
| 让 AI 或脚本改书签 | `POST /api/v1/batch`，带 `expectedRev` 防覆盖 |
| 坏档救命（`profile.json` 读不动了） | `POST /api/v1/profiles/rebuild`，或 `launchbay profile rebuild --id <slug> --title <名字> --pin <4-6 位>` |
| 点数统计（谁在点哪些书签） | `POST /api/v1/usage`，只写当前档案，不参与版本号 |
| 存一个 URL 顺便取回标题和图标 | `POST /api/v1/resolve`，不写书签，只填缓存 |
| 改这台机器的界面语言 | 管理页的「界面语言」，或首次建档时顺手选 |
| 看改动都发生过什么 | `GET /api/v1/history`（只记动作与路径，不记内容） |

**完整的端点、请求体形状与错误码见 [AGENTS.md](AGENTS.md)** —— 那是给 AI 读的权威契约，字段名逐个对过路由表，改一处会有一道测试跟着红，所以别在 README 里复述第二份。

上面那句「打开不用等」不是形容词：书签和常用图标是服务端直接写进 HTML 的，页面不必先跑脚本才把链接显示出来。**零 JavaScript 是底线，不是上限**：搜索是表单、建档与设 PIN 是原生表单提交，关掉 JavaScript 全部书签照样看得见、点得动——这条判据被测试断言守着。在此之上有一层增强（有 JavaScript 时才出现）：添加浮层、就地改标题与地址、拖拽排序、⌘K 面板、离线可用。两者不冲突：**增强层坏掉时你退回零 JS 那条路，那条路一直在**。

## 限制

- **只在局域网内用，没有 HTTPS。** 书签内容是明文 JSON 躺在磁盘上。放到公网前请自己在前面加一层 TLS 反代
- **一个档案的坏 `profile.json` 会锁住那一份。** 书签坏了能用 `revert` 或快照救回来；档案文件坏了这两条都救不了，出路是 `POST /api/v1/profiles/rebuild`（或 `launchbay profile rebuild`）——只重写那一个文件，书签、快照、历史一个字节都不碰
- **抓图标有冷却**：同一个 origin 失败一次后 24 小时内不再抓，限流是为了防止有人拿这台机器当探测跳板
- **没有共享清单**：不能把一条书签"共享给全家"，也不能公开一个页面。各存各的就是各存各的
- **还没在 NAS 真机上验收过。** 三轮验收都跑在 Windows + Docker Desktop。上面「数据目录的权限」和端口占用那几段来自真实使用经验，但没有留实测记录；局域网 LCP < 300ms 这个目标、真机冷启动 LCP、1,000 条书签的滚动帧率、离线缓存命中延迟，四项至今没测过
- **界面上有几个开关还没有写通路。** 页脚文字、是否在新标签打开这两项目前只能改磁盘上的 `bookmarks.json`，而服务端每次写入都会覆盖它——等于没有正经通路；天气、时钟、搜索引擎这三个开关界面根本不读它们。要改书签本身请走 `POST /api/v1/batch`，形状见 [AGENTS.md](AGENTS.md)
- **英文只覆盖界面，不覆盖给脚本读的那些句子。** `POST /api/v1/*` 的错误 detail、字段校验失败的原因、以及导入时那份逐条对账报告，仍然是中文。理由是调用方要的是**稳定可对照的文本**，而不是一段会随别人家界面语言变的散文；报告那一段则更像分析结果而不是界面 chrome。书签标题、分组名、地址这些本来就是你自己写的内容，同样不翻
- **语言是整台机器一个，不分人。** 一家人共用一台导航台，谁的语言也不该由另一个人的档案决定——所以它落在 `data/settings.json`（不进 `profile.json`），只有管理员能改。这是有意的代价：管理员改语言会立刻影响所有人，而管理员本人可能和家人的语言不一样

## 关于 365 开源计划

[365 开源计划](https://github.com/rockbenben/365opensource) 的第 **#042** 个项目——一个人 + AI，一年 300+ 个开源项目。

[提交你的需求 →](https://365.aishort.top/) · [Discord](https://discord.gg/PZTQfJ4GjX) · [Telegram](https://t.me/aishort_top)
