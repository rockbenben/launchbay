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
		// ⑥ 的 bookmarklet（§6.3）：吐出那段拖进书签栏的 JS，**里面没有令牌**——
		// 家里人不需要在任何地方配令牌，写走会话 cookie。
		{Method: "GET", Pattern: "/bookmarklet", Cred: credNone,
			Summary: "⑥ 的 bookmarklet（零令牌，拖进书签栏即用）", H: s.handleBookmarklet},
		{Method: "GET", Pattern: "/", Cred: credNone, Summary: "选择页或该档案首屏", H: s.handleRoot},
		{Method: "POST", Pattern: "/pin", Cred: credNone, Summary: "用 PIN 进门/切档", H: s.handlePin},
		{Method: "POST", Pattern: "/setup", Cred: credNone,
			Summary: "引导建档：只在磁盘零档案时有效（浏览器表单通路，建成即管理员）", H: s.handleSetup},
		{Method: "POST", Pattern: "/prefs", Cred: credSession,
			Summary: "改自己的台面偏好（主题/形态，仅会话，表单通路）", H: s.handlePrefs},
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
		{Method: "POST", Pattern: "/api/v1/import", Cred: credScoped,
			Summary: "扩展导入第一步：原始 Netscape HTML → 一份报告（不写盘）", H: s.handleImportAPI},
		{Method: "POST", Pattern: "/api/v1/import/apply", Cred: credScoped,
			Summary: "扩展导入第二步：确认写入（一次性令牌）", H: s.handleImportApplyAPI},
		// 回退到某一版的零 JS 两步确认（2026-10-11）。与上面那三步一样是浏览器
		// 表单通路，但**目标不接受申报**：档案从会话推导，所以 credSession 而
		// 不是 credScoped——管理员进得了这一页也只能退自己那一份（管理员管账号、
		// 不碰内容）。落点与 POST /api/v1/revert 同一个 revertLocked。
		{Method: "GET", Pattern: "/revert", Cred: credSession,
			Summary: "回退到某一版的确认页（第一步；浏览器表单通路）", H: s.handleRevertPage},
		{Method: "POST", Pattern: "/revert", Cred: credSession,
			Summary: "确认回退（第二步；与 POST /api/v1/revert 同一落点）", H: s.handleRevertForm},
		{Method: "GET", Pattern: "/admin", Cred: credAdmin,
			Summary: "管理页（读侧：档案一览与进入编辑，不含条目内容）", H: s.handleAdminPage},
		{Method: "POST", Pattern: "/admin", Cred: credAdmin,
			Summary: "管理页建档（浏览器表单通路；机器建档走 POST /api/v1/profiles）", H: s.handleAdminCreate},
		// 界面语言是**站级**设置（data/settings.json），不是某个档案的字段：它改了会立刻
		// 影响这台机器上的所有人，所以只给管理员会话，且带令牌一律 303 拒。落点与
		// store.Language 同一份，没有第二条读入通路。
		{Method: "POST", Pattern: "/admin/language", Cred: credAdmin,
			Summary: "改全机界面语言（浏览器表单通路；仅管理员会话，带令牌一律 303 拒）", H: s.handleAdminLanguage},
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
		{Method: "GET", Pattern: "/icons/{name}", Cred: credNone,
			Summary: "resolve 抓来的 favicon 缓存（内容哈希命名，immutable）", H: s.handleIconFile},
		{Method: "POST", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "建档（不带任何起步内容，模板机制 2026-10-10 整条拆掉）", H: s.handleCreateProfile},
		{Method: "GET", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "档案元信息列表（零条目）", H: s.handleListProfiles},
		{Method: "DELETE", Pattern: "/api/v1/profiles", Cred: credAdmin, Summary: "删档（连快照，历史保留）", H: s.handleDeleteProfile},
		{Method: "POST", Pattern: "/api/v1/profiles/self/pin", Cred: credSession,
			Summary: "改自己的 PIN（要旧 PIN，仅会话）", H: s.handleSelfPin},
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
// 四条规则都来自 spec §6.1：
//   - 令牌作用域是具体档案时 profile 必须等于它；`*` 才是管理员令牌。
//   - 会话的权限只看 lb_profile；跨档案要求当前在用的那个档案（Active）
//     在磁盘上真是管理员（回查磁盘，不信 cookie 形状；lb_home 不参与判定）。
//   - credScoped 却不申报档案，一律拒：见函数体开头那条。
//   - 不符一律拒绝，**不回落到某个默认档案**。
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
		sc, ok := s.deps.Tokens.Authorize(tok)
		if !ok || want == credSession {
			// 令牌不许走只认会话的端点：改自己 PIN 用令牌等于"谁有令牌
			// 谁能改谁"，那正是 §6.1 把 self/pin 划给会话的理由。
			return "", auth.Scope{}, false
		}
		if want == credAdmin && !sc.IsAdmin() {
			return "", sc, false
		}
		// **`*` 令牌只管账号，不管内容**（2026-10-10 改判）。以前 `*` 能过
		// credScoped，于是管理员令牌对任何人的书签都能读写——而"管理员看得见、
		// 改得动全家书签"正是本设计要拿掉的形状（与 Nextcloud / Linkding 一致）。
		// 现在 `*` 只进 credAdmin 那一族（建档/删档/改 PIN/status/审计聚合），
		// 写书签必须用档案作用域令牌 `name:secret:dad`。
		if sc.IsAdmin() && want != credAdmin {
			return "", sc, false
		}
		if !sc.Allows(profileID) {
			return "", sc, false
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
	if !sess.Valid() {
		return "", auth.Scope{}, false
	}
	if want == credNone {
		return sess.Active, auth.Scope{}, true
	}
	if want == credAdmin && !s.activeIsAdmin(sess) {
		return "", auth.Scope{}, false
	}
	// profileID 能留空走到这里的只有 credAdmin 的聚合读（credScoped 的空申报
	// 在函数开头就拒了），所以这里把空 profile 当"没有具体目标"，而不是
	// "目标是所有档案"。
	// **内容端点跨档案一律拒，管理员也不例外**（2026-10-10 改判）。原来这一行是
	// `profileID != sess.Active && !s.activeIsAdmin(sess)`——管理员的会话能读写
	// 任何人的书签，而 §8.9 还专门给这个形状配了一条"正在管理 X 的书签"的横幅。
	//
	// **只对 credScoped 生效**是这条的关键：管理员管的是**账号**（建档/删档/
	// 改 PIN/停用/看条数与 rev），那些端点是 credAdmin，跨档案当然放行——把这一行
	// 写成对所有 cred 一视同仁，第一个撞上的就是"管理员给 kid 设 PIN 403 了"，
	// 那正是本人拍板要保留的那条出路。
	//
	// 判据只有这一处，端点层不各自再写一遍管理员特例。
	if want == credScoped && profileID != "" && profileID != sess.Active {
		return "", auth.Scope{}, false
	}
	// actor 只进审计线，且只可能是 Active：代管态不存在之后，"当初是谁验证
	// 进来的"与"这次是谁的权限"是同一个档案，分成两个字段只会让审计线记下一
	// 个无法认领的名字。
	return sess.Active, auth.Scope{}, true
}

// activeIsAdmin 回答"当前在用的这个档案能不能跨档案动别人的数据"。
//
// 只看 `lb_profile` 指向的档案（spec §6.1：权限永远只看它；§11：伪造 cookie 拿不到
// 单 cookie 时代拿不到的任何东西）。
//
// 这条与 `Session.Valid` 是**两个问题**，不许合并成一次查表：
//   - Valid 问"这份 cookie 是不是合法形状"——纯形状，不查磁盘；
//   - activeIsAdmin 问"这把凭证能不能动别人"——只看 Active，且要回磁盘确认那个
//     档案真是管理员。
//
// 把两者揉成一次"查 home 那一栏"的表，等于让 kid 伪一条 `lb_home=dad` 就拿到 dad
// 的写权限（Task 4 评审的 Critical 就是这么来的）。
func (s *Server) activeIsAdmin(sess auth.Session) bool {
	if !sess.Valid() {
		return false
	}
	p, err := s.deps.Store.LoadProfile(sess.Active)
	return err == nil && p.Admin
}
