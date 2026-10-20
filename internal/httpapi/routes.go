package httpapi

import (
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/render"
)

// cred 是一个端点接受的凭证类别。写在路由表里而不是散进每个 handler，
// 有两个好处：契约测试能逐条对照 openapi；新加端点时"忘了鉴权"会变成
// 表里一眼看得见的空缺，而不是要等有人来打才知道。
type cred int

const (
	credNone    cred = iota // 不要凭证：/healthz、GET /、/pin、/setup、openapi.json
	credScoped              // 令牌或会话，且只能动自己有资格的那个档案
	credAdmin               // 管理员令牌（LAUNCHBAY_ADMIN_TOKEN）或管理员 cookie（lb_admin）
	credSession             // 只有会话（改自己 PIN 这类，令牌不许代改）
	// credTokenOnly 是**档案作用域令牌、且不认会话**（2026-10-19 加）。
	//
	// 为什么要有这一档：原先 import 那两条挂在 credScoped 上，而 credScoped 在
	// 文档侧一律报成 `bearerAuth|sessionCookie`（openapi_test.go 的那两张表）。
	// 于是契约写着"会话也行"，行为是会话一律 401 —— **fail closed，不构成越权**，
	// 但照契约写脚本的扩展会拿会话 cookie 换来一句「令牌认不出来」，进而以为自己
	// 的会话坏了。
	//
	// 行为本身是对的，别去放宽 handler：那条通路是**给浏览器扩展用的**，扩展读得到
	// 整棵书签树、却读不到会话 cookie（§6.3），所以它本来就只能带令牌；而"档案从
	// 令牌作用域推导、不接受申报"是它更强的一格性质（申报字段根本不存在，
	// "令牌与档案对不上"在协议层就无法表达）。收会话等于把那条更强的性质削一半，
	// 只为迁就一份写错的文档——那是本末倒置。
	credTokenOnly
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
		// ⑤ 的带指纹静态资产。**路径是指纹算出来的那一条具体路径**，不是通配：
		// 指纹在 render 包初始化时由内容算出，而 routes() 在 New 里跑，于是注册时
		// 它已经是一个确定的字符串。好处是它照常进契约闸（路由表 → openapi →
		// AGENTS.md 三处一起对），而通配路径做不到这件事。
		// 凭据是 credNone：脚本里没有任何档案数据，页面要它也只是为了让首屏能过
		// CSP 的 script-src 'self'（内联脚本那条路是被 CSP 关着的）。
		{Method: "GET", Pattern: render.AppJSPath, Cred: credNone,
			Summary: "⑤ 的应用脚本（内容指纹命名，immutable）", H: s.handleAppJS},
		// ⑤/⑥ 的**交互**样式（浮层、⌘K、就地编辑）。与 app.js 同款：内容指纹 +
		// immutable。走外部而不内联，是因为 base.css 那份内联进每个页面，
		// 而这些交互样式只有档案页用得到。
		{Method: "GET", Pattern: render.AppCSSPath, Cred: credNone,
			Summary: "⑤/⑥ 的交互样式（内容指纹命名，immutable）", H: s.handleAppCSS},
		// ⑥ 的 Service Worker。**路径不带指纹**：浏览器自己按注册处检查 /sw.js 的
		// 字节变化来决定更新，指纹只会多一处要同步的地方。
		// 必须在根路径——SW 的作用域是它所在目录，挪到 /assets/ 下面就管不到首页，
		// 而首页正是离线要保的那一张。
		{Method: "GET", Pattern: "/sw.js", Cred: credNone,
			Summary: "⑥ 的 Service Worker（根路径，作用域覆盖整站）", H: s.handleSWJS},
		{Method: "GET", Pattern: "/manifest.webmanifest", Cred: credNone,
			Summary: "⑥ 的 Web App Manifest（PWA 独立窗口安装，支持添加到主屏幕）", H: s.handleManifest},
		// ⑥ 的 bookmarklet（§6.3）：吐出那段拖进书签栏的 JS，**里面没有令牌**——
		// 家里人不需要在任何地方配令牌，写走会话 cookie。
		{Method: "GET", Pattern: "/bookmarklet", Cred: credNone,
			Summary: "⑥ 的 bookmarklet（零令牌，拖进书签栏即用）", H: s.handleBookmarklet},
		{Method: "GET", Pattern: "/", Cred: credNone, Summary: "选择页或该档案首屏", H: s.handleRoot},
		{Method: "POST", Pattern: "/pin", Cred: credNone, Summary: "用 PIN 进门/切档", H: s.handlePin},
		{Method: "POST", Pattern: "/lang", Cred: credNone,
			Summary: "引导页上那个语言下拉自己的通路（零档案时可用；按下去页面立刻换语言）", H: s.handleSetupLanguage},
		{Method: "POST", Pattern: "/setup", Cred: credNone,
			Summary: "引导建档：只在磁盘零档案时有效（浏览器表单通路；建出来的是第一个人的书签页，不是管理员）", H: s.handleSetup},
		// 管理员的浏览器通路（2026-10-12）。管理员不再是某个档案，于是"怎么进管理页"
		// 需要一条**自己的**门：POST 一次 key，服务端发一条 lb_admin cookie（装的是
		// 派生值，不是 key 本身）。
		//
		// 两条都是 credNone：**未登录不是权限类拒绝**，那正是这一页要处理的形状。
		// GET /admin 因此改成"没管理权就渲染登录页"而不是 303 回选择页——否则升级
		// 之后没有任何人进得去管理页（档案不再是管理员，而 key 还没输过）。
		{Method: "POST", Pattern: "/admin/login", Cred: credNone,
			Summary: "输管理 key 进管理页（浏览器表单通路；成功后发 lb_admin）", H: s.handleAdminLogin},
		{Method: "POST", Pattern: "/admin/logout", Cred: credNone,
			Summary: "丢掉管理员身份（清 lb_admin，档案会话不动）", H: s.handleAdminLogout},
		{Method: "POST", Pattern: "/prefs", Cred: credSession,
			Summary: "改自己的台面偏好（主题/形态/排法，仅会话，表单通路）", H: s.handlePrefs},
		// 台面面板「分组」那一节。**不并进 /prefs**：建组、accent、分组自己的
		// layout、名字与删掉这一组（remove）都落在 bookmarks.json 的分组上，属于
		// 书签数据，要走 applyBatch（快照、审计、rev 一道闸不少），而 /prefs 只写
		// profile.json。
		// 建组与删组（都是 2026-10-15）也在这条上而不是新开端点：落点与判据与改
		// 颜色完全同一份，而新开一条只会让「这一族面板表单走哪」多一个答案。
		{Method: "POST", Pattern: "/prefs/pin", Cred: credSession,
			Summary: "改自己这一份的 PIN（面板里那张表单；与 self/pin 同一个落点）", H: s.handlePrefsPin},
		{Method: "POST", Pattern: "/prefs/group", Cred: credSession,
			Summary: "建一个新分组、改一个分组的颜色/名字/排法，或删掉这一组（仅会话，表单通路；书签写通路）", H: s.handlePrefsGroup},
		// 背景那一节（2026-10-14）。**必须 multipart**，所以不并进上面那条
		// urlencoded 的 /prefs：两件事的 422 字段名单各管各的。三个动作
		// （上传/移除/暂时不显示）共用这一个端点，一次按下 = 一个意图。
		{Method: "POST", Pattern: "/prefs/background", Cred: credSession,
			Summary: "改这一页自己的背景图（上传/移除/暂时不显示，仅会话，multipart 表单通路）", H: s.handlePrefsBackground},
		// 「点开书签」那一格：当前标签页 / 新标签页。**它走 applyBatch 那条书签写
		// 通路**（rev + 快照 + 审计），因为 openInNewTab 住在书签文档里而不是
		// profile.json——与 /prefs/group 同一份收尾（applyPrefsOps）。
		{Method: "POST", Pattern: "/prefs/tabs", Cred: credSession,
			Summary: "点开一条书签是在当前标签页还是新标签页（仅会话，表单通路；书签写通路）", H: s.handlePrefsTabs},
		{Method: "POST", Pattern: "/switch", Cred: credSession,
			Summary: "会话动作：回选择页（免 PIN 直达已下线）", H: s.handleSwitch},
		// 浏览器收藏夹导入（2026-10-10）：三步零 JS——选文件 → 看报告 →
		// 确认写入。只认**本人的会话**，只写**自己的**档案，且档案必须还是
		// 空的（与 CLI 的 import-bookmarks 同一规矩）。写入走 applyBatch，
		// 与 POST /api/v1/batch 同一个落点。
		{Method: "GET", Pattern: "/import", Cred: credSession,
			Summary: "导入第一步：选浏览器导出的书签文件", H: s.handleImportPage},
		{Method: "POST", Pattern: "/import", Cred: credSession,
			Summary: "导入第二步：收文件、解析、给报告（不写盘）", H: s.handleImportUpload},
		{Method: "POST", Pattern: "/import/apply", Cred: credSession,
			Summary: "导入第三步：确认写入（一次性令牌）", H: s.handleImportApply},
		// 令牌通路的两步，给**浏览器扩展**用：扩展读得到浏览器的整棵书签树，却
		// 读不到会话 cookie，于是只能带令牌来。体是原始 Netscape HTML，档案
		// **从令牌作用域推导、不接受申报**——一枚 dad 的令牌只能导进 dad。
		// 与上面那两条共用 planImport/applyImport，服务端只有一份判据。
		{Method: "POST", Pattern: "/api/v1/import", Cred: credTokenOnly,
			Summary: "扩展导入第一步：原始 Netscape HTML → 一份报告（不写盘）", H: s.handleImportAPI},
		{Method: "POST", Pattern: "/api/v1/import/apply", Cred: credTokenOnly,
			Summary: "扩展导入第二步：确认写入（一次性令牌）", H: s.handleImportApplyAPI},
		// 回退到某一版的零 JS 两步确认（2026-10-11）。与上面那三步一样是浏览器
		// 表单通路，但**目标不接受申报**：档案从会话推导，所以 credSession 而
		// 不是 credScoped——管理员进得了这一页也只能退自己那一份（管理员管账号、
		// 不碰内容）。落点与 POST /api/v1/revert 同一个 revertLocked。
		// 台面面板的独立页（2026-10-13）。顶栏那一格改的全是同一件事——这一页怎么排、
		// 叫什么、能退到哪——原先是六颗散着的控件（改名 details、主题循环、视图三颗、
		// 档案形态、回退、改动历史），顶栏两行都塞不下。
		//
		// 它是**不经 JS 的那条路**：档案页里页内嵌的浮层与这一页渲染同一个 define。
		// 准入与 /revert、/history 同一族（selfProfileOnly，档案从会话推导），
		// 所以管理员进得了也只能改自己那一份。
		{Method: "GET", Pattern: "/prefs", Cred: credSession,
			Summary: "台面面板的独立页（改名/视图/档案形态/主题 + 回退与改动历史两条路；左侧一列分类 tab；JS 关着时顶栏那颗「台面」落在这里）", H: s.handlePrefsPage}, {Method: "GET", Pattern: "/revert", Cred: credSession,
			Summary: "回退到某一版的确认页（第一步；浏览器表单通路）", H: s.handleRevertPage},
		{Method: "POST", Pattern: "/revert", Cred: credSession,
			Summary: "确认回退（第二步；与 POST /api/v1/revert 同一落点）", H: s.handleRevertForm},
		// 改动历史那一屏（2026-10-13）：审计行从写通路落地那天起就在写，
		// `GET /api/v1/history` 也一直读得出来，缺的是屏——于是 spec §2 那句
		// "改动有历史可查"在界面上是空的。准入与 /revert 同一族（selfProfileOnly）：
		// 档案从会话推导、不接受申报，所以管理员进得了这一页也只能看自己那一份。
		{Method: "GET", Pattern: "/history", Cred: credSession,
			Summary: "这一份的改动历史（浏览器通路；只列动作，不列内容）", H: s.handleHistoryPage},
		{Method: "GET", Pattern: "/admin", Cred: credAdmin,
			Summary: "管理页（读侧：档案一览与账号操作，不含条目内容）；无管理权时渲染登录页", H: s.handleAdminPage},
		{Method: "POST", Pattern: "/admin", Cred: credAdmin,
			Summary: "管理页建档（浏览器表单通路；机器建档走 POST /api/v1/profiles）", H: s.handleAdminCreate},
		// 界面语言是**站级**设置（data/settings.json），不是某个档案的字段：它改了会立刻
		// 影响这台机器上的所有人，所以只给管理员 cookie（lb_admin），且带令牌一律 303 拒。落点与
		// store.Language 同一份，没有第二条读入通路。
		{Method: "POST", Pattern: "/admin/language", Cred: credAdmin,
			Summary: "改全机界面语言（浏览器表单通路；仅管理员 cookie，带令牌一律 303 拒）", H: s.handleAdminLanguage},
		// 设 PIN 与删档的零 JS 两步确认（§8.8：改判，不等 ⑤ 的 JS）。这四条
		// **不是新的 API 面**：落点分别与 POST /api/v1/profiles/pin 和
		// DELETE /api/v1/profiles 同一份，差别只在怎么读入、怎么答出。
		{Method: "GET", Pattern: "/admin/pin", Cred: credAdmin,
			Summary: "给某个档案设 PIN 的确认页（第一步；浏览器表单通路）", H: s.handleAdminPinPage},
		{Method: "POST", Pattern: "/admin/pin", Cred: credAdmin,
			Summary: "提交新的 PIN（第二步；与 POST /api/v1/profiles/pin 同一落点）", H: s.handleAdminPinForm},
		{Method: "GET", Pattern: "/admin/delete", Cred: credAdmin,
			Summary: "删档的确认页（第一步，写明会删掉多少条目与快照）", H: s.handleAdminDeletePage},
		{Method: "POST", Pattern: "/admin/delete", Cred: credAdmin,
			Summary: "确认删档（第二步；与 DELETE /api/v1/profiles 同一落点）", H: s.handleAdminDeleteForm},
		{Method: "GET", Pattern: "/admin/disable", Cred: credAdmin,
			Summary: "停用/启用的确认页（第一步，写明会与不会动什么）", H: s.handleAdminDisablePage},
		{Method: "POST", Pattern: "/admin/disable", Cred: credAdmin,
			Summary: "确认停用/启用（第二步；与 POST /api/v1/profiles/disable 同一落点）", H: s.handleAdminDisableForm},
		{Method: "GET", Pattern: "/api/v1/openapi.json", Cred: credNone,
			Summary: "机器可读契约（无个人信息，公开）", H: s.handleOpenAPI},
		{Method: "GET", Pattern: "/api/v1/icons", Cred: credNone,
			Summary: "本地图标索引搜索（公开）", H: s.handleIconsSearch},
		// 同一条搜索的**界面那一半**：吐的是画好的 HTML 方格，不是名字数组。
		// 公开（credNone），与上面那条同族——它说不出任何人有什么书签。
		// 为什么多一条而不是给上面那条加个 format 参数：这条的响应类型根本不是
		// JSON（text/html），而 §4.3 那份契约按形状写形状，混在一个端点的两半里
		// 会让"这条端点返回什么"在 openapi 里没法一句话说清。
		{Method: "GET", Pattern: "/api/v1/icons/tiles", Cred: credNone,
			Summary: "图标选择器那一格：同一条搜索，画好的 HTML 方格（公开）", H: s.handleIconTiles},
		// 图标选择器的**浏览**那一档：翻图库，不搜什么。
		// 公开（credNone）：它是一份公共名单，说不出任何人有什么书签。
		{Method: "GET", Pattern: "/api/v1/icons/browse", Cred: credNone,
			Summary: "图标选择器翻图库：按 offset 翻页，画好的 HTML 方格（公开）", H: s.handleIconBrowse},
		// 图标选择器的**默认那一批**：不搜什么，直接给一份常用清单。
		// 为什么不能只靠上面那条搜索：空查询按契约回一张空图（库里一万多条，
		// 倒出来是一屏噪声），而"这一份已经在用的图标"那一格在实测里几乎永远是
		// 空的（书签默认 auto，auto 与"没设"都被排除），于是面板打开什么也没有。
		// 公开（credNone）：它是一份公共名单，说不出任何人有什么书签。
		{Method: "GET", Pattern: "/api/v1/icons/starter", Cred: credNone,
			Summary: "图标选择器的默认那一批：常用图标清单，画好的 HTML 方格（公开）", H: s.handleIconStarter},
		{Method: "GET", Pattern: "/icons/{name}", Cred: credNone,
			Summary: "resolve 抓来的 favicon 缓存（内容哈希命名，immutable）", H: s.handleIconFile},
		// 这一页自己的背景图。**与上面那条 favicon 通路相反**：那条是公开的
		//（任何站都有的图标，而它不含任何档案信息），这一条要会话——背景图
		// 是家里人自己放的图（很可能是孩子的照片），而 blob 按档案分目录，
		// 于是 A 的会话连 B 的那张都读不到，哪怕他知道哈希。判据见
		// background.go 那段三条。
		{Method: "GET", Pattern: "/backgrounds/{name}", Cred: credSession,
			Summary: "这一份档案自己的背景图（内容哈希命名，仅会话）", H: s.handleBackgroundFile},
		{Method: "POST", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "建档（不带任何起步内容，模板机制 2026-10-10 整条拆掉）", H: s.handleCreateProfile},
		{Method: "GET", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "档案元信息列表（零条目）", H: s.handleListProfiles},
		{Method: "DELETE", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "删档（连快照，历史保留）", H: s.handleDeleteProfile},
		{Method: "POST", Pattern: "/api/v1/profiles/self/pin", Cred: credSession,
			// 旧 PIN 只在这一份**现在有 PIN** 时才要（2026-10-12）：一份不设防的档案
			// 可以直接给自己设上第一个 PIN，而 `new` 留空则是取消它（取消仍要旧 PIN）。
			Summary: "改自己的 PIN（仅当这一份现在有 PIN 才要旧 PIN；`new` 留空 = 取消，仅会话）", H: s.handleSelfPin},
		{Method: "POST", Pattern: "/api/v1/profiles/pin", Cred: credAdmin,
			Summary: "管理员给任意档案设 PIN", H: s.handleAdminPin},
		// 坏档的机器出路（§6.1、§12）。credAdmin 与设 PIN 同族：它重写的是
		// profile.json 这个**账号文件**，不碰任何人的书签。2026-10-10 之前这条
		// 只存在于设计文档与 README 里——store 的 RebuildProfile 早就写好并测过，
		// 而端点、CLI 与两处契约表全部缺席，于是坏档在事实上没有出路（实测：
		// profiles/pin 503、profiles/disable 503、profiles 409、CLI 打印别的
		// 子命令的用法），界面却一直叫人"在这台机器上修好那个文件"。
		{Method: "POST", Pattern: "/api/v1/profiles/rebuild", Cred: credAdmin,
			Summary: "重建读不动的档案文件（只重写 profile.json，书签与快照不碰）", H: s.handleRebuildProfile},
		{Method: "POST", Pattern: "/api/v1/profiles/disable", Cred: credAdmin,
			Summary: "停用/启用某档案（数据不动，仍占目录数；与 /admin/disable 同一落点）", H: s.handleAdminDisableAPI},
		{Method: "GET", Pattern: "/api/v1/status", Cred: credAdmin, Summary: "各档案 rev/条目/快照/坏档汇总", H: s.handleStatus},
		{Method: "GET", Pattern: "/api/v1/data", Cred: credScoped, Summary: "该档案全量数据 + rev", H: s.handleData},
		{Method: "GET", Pattern: "/api/v1/export", Cred: credScoped, Summary: "导出磁盘上的 bookmarks.json 原字节", H: s.handleExport},
		{Method: "GET", Pattern: "/api/v1/history", Cred: credScoped, Summary: "动作审计线（不含 ops 内容）；省略 profile = 全家庭聚合，仅管理员（credAdmin）", H: s.handleHistoryAPI},
		{Method: "POST", Pattern: "/api/v1/batch", Cred: credScoped,
			Summary: "一批 ops 原子写入某档案", H: s.handleBatch},
		{Method: "POST", Pattern: "/api/v1/revert", Cred: credScoped,
			Summary: "从快照恢复（产生新 rev，不抹历史）", H: s.handleRevert},
		{Method: "POST", Pattern: "/api/v1/usage", Cred: credScoped,
			Summary: "点击计数上报（只写当前档案，不参与 rev）", H: s.handleUsage},
		{Method: "POST", Pattern: "/api/v1/resolve", Cred: credScoped,
			Summary: "URL 解析成书签候选（含 favicon 抓取）", H: s.handleResolve},
	}
}

func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// require 回答"这把凭证能不能动 profileID"，返回要写进 history 的 actor。
//
// 2026-10-12 起是**三条互斥的路**，判据只有这一份：
//   - 管理员令牌（LAUNCHBAY_ADMIN_TOKEN 那把裸 key）→ 只放行 credAdmin；
//   - 档案作用域令牌（name:secret:<id>）→ 只放行 credScoped，档案必须等于它的作用域；
//   - 档案会话（lb_profile）→ credNone / credScoped / credSession，看当前在用哪一份。
//
// 管理员 cookie（lb_admin，管理页用的第二条管理员通路）单独在下面判：它只放行
// credAdmin，**永远进不了 credScoped**——"管理员读不到别人书签"那条边界在两条
// 管理员通路上都成立。
func (s *Server) require(r *http.Request, want cred, profileID string) (string, auth.Scope, bool) {
	// "不申报档案"不是"不受限"：credScoped 的含义是"只能动我有资格的那一个
	// 档案"，而下面的跨档案检查是靠 profileID 非空才跑的。留空等于让调用方
	// 用"我谁都不针对"这句话拿到针对所有人的读权限（Task 6 评审的
	// Critical：/history 不带 ?profile= 时任何 kid 会话都能拉到全家庭审计
	// 线）。要跨档案聚合就在路由或 handler 里显式写 credAdmin——想清楚一次，
	// 比在每个端点上各补一个特例可靠。
	if want == credScoped && profileID == "" {
		return "", auth.Scope{}, false
	}
	if tok := bearer(r); tok != "" {
		// 令牌不许走只认会话的端点：改自己 PIN 用令牌等于"谁有令牌
		// 谁能改谁"，那正是 §6.1 把 self/pin 划给会话的理由。
		if want == credSession {
			return "", auth.Scope{}, false
		}
		// **管理员令牌只管账号，不管内容**（2026-10-10 改判，2026-10-12 换了形状）。
		// 以前 `*` 能过 credScoped，于是管理员令牌对任何人的书签都能读写——而
		// "管理员看得见、改得动全家书签"正是本设计要拿掉的形状（与 Nextcloud /
		// Linkding 一致）。现在它只进 credAdmin 那一族（建档/删档/改 PIN/
		// status/审计聚合），写书签必须用档案作用域令牌 `name:secret:dad`。
		if s.deps.AdminToken.Matches(tok) {
			if want != credAdmin {
				return "", auth.Scope{}, false
			}
			return adminActor, auth.Scope{}, true
		}
		sc, ok := s.deps.Tokens.Authorize(tok)
		// 档案作用域令牌过不了 credAdmin，哪怕它申报的就是自己的档案。
		if !ok || want == credAdmin {
			return "", auth.Scope{}, false
		}
		if !sc.Allows(profileID) {
			return "", auth.Scope{}, false
		}
		return sc.Name, sc, true
	}
	sess := auth.ReadSession(r)
	// `Valid()` 一并认得出**升级前发出去的代管 cookie**（两条不等即非法），
	// 所以这里不需要第二道判据。cookie 是自包含的（没有服务端会话表），那种
	// cookie 只看得到 `lb_profile=kid`，读的人会当成"kid 自己登录"——判它非法
	// 是 2026-10-10 那次改判的一部分（代管已下线），不判就等于把后门留着。
	//
	// 判在 credNone 之前：否则 `GET /` 会照样把别人的台渲染出来（它不走下面
	// 的权限分支）。判据只有 Session.Valid 这一份，端点层不各自再写一遍。
	//
	// 手上只有管理员 cookie、没有任何档案会话，这也算"有身份"——管理员完全
	// 可能没登任何档案就想去看一眼状态。
	adminOK := s.hasAdminCookie(r)
	if !sess.Valid() && !adminOK {
		return "", auth.Scope{}, false
	}
	if want == credAdmin {
		// 管理员 cookie 与档案会话是**两件独立的事**：管理员可能同时登着自己
		// 的档案在用书签，也可能只拿着管理权。而"是不是管理员"从此只由那把 key
		// 回答——早先这里是回磁盘看这个档案的 admin 是不是 true，那个属性已经
		// 连同"第一个档案自动是管理员"一起删掉了。
		if !adminOK {
			return "", auth.Scope{}, false
		}
		return adminActor, auth.Scope{}, true
	}
	if want == credNone {
		if sess.Valid() {
			return sess.Active, auth.Scope{}, true
		}
		return adminActor, auth.Scope{}, true
	}
	if want == credTokenOnly {
		// **只认令牌的那一档不认会话**（2026-10-20 修）。credTokenOnly 是
		// 「档案作用域令牌，且不认会话」——与 credSession（只认会话）正好相反。
		//
		// 从前 require 里**没有这一支**，于是带会话的请求会一路落到最后一行
		// `return sess.Active, …, true`：那一档的"不认会话"只由两个 import
		// handler 自己在调用 require **之前**多写了一遍 Authorize 撑着。
		// 端点层那一遍是纵深防御，不是判据——下一个人加一条 credTokenOnly
		// 端点而没重复那个前置检查，会话就静默通过，而这一族的全部意义是
		// 「档案从令牌作用域推导、不接受申报」这条更强的性质。
		//
		// 实测（改之前）：require(带 lb_profile=dad, credTokenOnly, "dad") → ok=true。
		return "", auth.Scope{}, false
	}
	if !sess.Valid() {
		// 只有管理员 cookie、没有档案会话：剩下唯一可能落到的 cred 是
		// credScoped，而它要的是某个具体档案——没有会话就没有"自己那一份"，
		// 所以这里必拒。这条同时就是"管理员读不到任何人的书签"的落点。
		return "", auth.Scope{}, false
	}
	// **内容端点跨档案一律拒，管理员也不例外**（2026-10-10 改判）。
	//
	// **只对 credScoped 生效**是这条的关键：管理员管的是**账号**（建档/删档/
	// 改 PIN/停用/看条数与 rev），那些端点是 credAdmin，跨档案当然放行——把这一行
	// 写成对所有 cred 一视同仁，第一个撞上的就是"管理员给 kid 设 PIN 403 了"，
	// 那正是本人拍板要保留的那条出路。
	//
	// 判据只有这一处，端点层不各自再写一遍管理员特例。
	if want == credScoped && profileID != sess.Active {
		return "", auth.Scope{}, false
	}
	// actor 只进审计线，且只可能是 Active：代管态不存在之后，"当初是谁验证
	// 进来的"与"这次是谁的权限"是同一个档案，分成两个字段只会让审计线记下一
	// 个无法认领的名字。
	return sess.Active, auth.Scope{}, true
}

// adminActor 是管理员动作在审计线里的 actor。管理员不再是某个档案（2026-10-12
// 删掉了档案上的 admin 属性），于是这里是一个常量而不是某人的 id——`profile`
// 那一栏照样记"这次动的是哪一份"，两栏分工不变。
const adminActor = "admin"

// hasAdminCookie 是"这次请求带的是不是一把真管理权"的**唯一判据**。
// 它不回磁盘、不看 cookie 形状之外的东西：派生值对不上就是没有管理员。
func (s *Server) hasAdminCookie(r *http.Request) bool {
	return s.deps.AdminToken.MatchesCookie(auth.ReadAdminCookie(r))
}
