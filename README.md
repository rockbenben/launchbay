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
- **各人一份，别人看不见。** 每人一个页面，进门输一个 4–6 位数字。共用一台电脑不串号——**管理员也看不见你的**：他能建档、删档、给你改密码，但读不到你页面上装了什么。这不是靠前端藏，服务端就是分开的：管理员的令牌去读别人的书签会被直接拒掉（403）。要说清一句：这不是密码学意义上的"防他"——管理员给你设 PIN 时就知道那个 PIN，局域网里改个 cookie 也能冒充谁；这里指的是**没有一条通路**让他读到你的书签。
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
cp .env.example .env      # 填两个令牌（见下）
docker compose up -d
```

打开 http://localhost:7600 会进**一次性建档页**：给第一个档案起个名字、设 4–6 位 PIN。**没有默认账号密码**——没有 `admin/admin` 可试，你填进去的名字和数字就是你自己的。这个页面只在磁盘上一个档案都没有时有效，建完就永久关闭——所以"这台机器建过档了没有"本身就是一条线索，不会被匿名访问者拿来数你家里有几口人。

不想 clone 也行——直接拉发布镜像（amd64 与 arm64 都出了，Docker Hub 与 GHCR 同名）：

```bash
docker run -d --name launchbay -p 7600:7600 \
  -v launchbay-data:/data -e LAUNCHBAY_DATA_DIR=/data \
  -e LAUNCHBAY_WRITE_TOKENS="admin:换成你自己的令牌:*" -e TZ=Asia/Shanghai \
  --health-cmd "/launchbay healthcheck" --health-interval 30s \
  ghcr.io/rockbenben/launchbay:latest
```

令牌在 `.env` 里，两条：

| 变量 | 给谁用 | 作用域 |
|---|---|---|
| `LAUNCHBAY_TOKEN_ADMIN` | 管理员：建档、删档、给别人设 PIN、停用/启用、跨档案读状态与审计线 | `*`，**只管账号**——它打 `/api/v1/data`、`/api/v1/batch`、`/api/v1/export` 一律 403 |
| `LAUNCHBAY_TOKEN_AI_DAD` | AI 运行时（成批增删改书签） | 只能动一个档案 |

> **注意档案 id 是从名字派生的，不一定叫 `dad`。** 建档只问名字与 PIN，id 由名字转
> 来的：名字里有 ASCII 字母数字就取那个 slug（`Dad` → `dad`、`nas box` → `nas-box`），
> **中文名字转不出东西，一律落到 `home` / `home-2` 基名序列**。所以拿中文名字建了
> 第一个档案之后，`.env` 里那枚 `…:dad` 的令牌作用域对不上任何档案，打过去是 403。
> 两种修法：把令牌作用域改成实际的档案 id（推荐），或者第一个档案就用 ASCII 名字。

宿主端口默认 7600；Windows 上 7532–7631 整段被 Hyper-V 动态保留，撞上了就在 `.env` 里加 `LAUNCHBAY_HOST_PORT`（容器内端口不动）。

### 浏览器扩展（可选）

`extension/` 里是一个 Chrome / Edge 扩展（Manifest V3，右键把当前页面存进你指定的那一份档案，或一次收藏当前窗口的所有标签页）。它不在发行镜像里，要自己装：

1. 打开 `chrome://extensions`，右上角开**开发者模式**
2. 点**加载已解压的扩展程序**，选仓库里的 `extension/` 目录
3. 点扩展图标进设置页，填三样：
   - **这台机器的地址**——`http://10.0.0.3:7600`，**不带结尾斜杠**
   - **默认档案 id**——右边令牌对应的那一份（见上面那条"id 是从名字派生的"）
   - **那一枚令牌**——`.env` 里 `LAUNCHBAY_TOKEN_AI_DAD` 的**值本身**，不是 `ai-dad:…:dad` 整串（三段式 `name:secret:profile` 里 `Bearer` 后面只放 `secret` 那段）
4. 点**试一下这台机器**验一下

那个「试一下」按钮答 403 不是网络坏了，是**令牌与档案对不上**：它拿默认档案去读一次，403 就说明这枚令牌的作用域是别的档案——回到 `.env` 把 `LAUNCHBAY_TOKEN_AI_DAD` 的作用域段（`…:dad` 里那个 `dad`）改成你实际的档案 id，然后重启容器。

装好之后：任意网页右键 → 存到 launchbay；设置页里还能**把浏览器书签树整个导进来**（第一次点才问你要「读书签」权限，设成按需申请——只为右键收藏装它的人，永远不会看到那个提问；导进去之前先给你一份逐条对账报告）。

> 安装时 Chrome 会警告「读取和更改您在所有网站上的数据」：扩展要能在任意网站上响应右键才需要这条，这正是它的用法。它申请的全部权限是 `contextMenus` / `storage` / `tabs` / `notifications`，`bookmarks` 是按需申请的那一项。
> 令牌只存在这台电脑的 `chrome.storage.local` 里，**不进 sync、不上云**；这个设置页没有导出配置的按钮，正是同一个理由。换档案换令牌，某一台泄露了只吊销那一枚。
> Firefox 未验证——`extension/` 按 Chrome / Edge 写的。

### 数据目录的权限

容器以 **uid 65534** 运行，而 NAS 上 `./data` 默认是 root 所属的——挂上去之后建档、导入、写书签会全部失败。**在宿主机上把那个目录交给这个用户**：

```bash
sudo chown -R 65534:65534 ./data     # 目录位置由 .env 的 LAUNCHBAY_HOST_DATA 决定
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

## 关于 365 开源计划

[365 开源计划](https://github.com/rockbenben/365opensource) 的第 **#042** 个项目——一个人 + AI，一年 300+ 个开源项目。

[提交你的需求 →](https://365.aishort.top/) · [Discord](https://discord.gg/PZTQfJ4GjX) · [Telegram](https://t.me/aishort_top)
