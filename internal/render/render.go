// Package render 把一个档案渲染成完整 HTML。渲染在服务端做，
// 因此首屏不需要 JS、数据直接内联，关掉脚本也能看（spec §7.2）。
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/store"
)

type Page struct {
	// Body 是缓存切片：同一 (档案, rev, bookmarks.mtime, profile 指纹, iconGen)
	// 的后续请求拿到的是同一份底层数组，调用方不许原地改；确实要改就先复制一份。
	Body []byte
	ETag string
	// Profile 是 handler 写 X-LB-Profile 的依据：SW 读不到 cookie，
	// 只能靠这个响应头决定把页面存进哪个档案的缓存（spec §7.1）。
	Profile string
	Rev     int64
	NoStore bool // 选择页与坏档案页：跨档案命中就是隐私事故
}

type cacheKey struct {
	profile  string
	rev      int64
	modified time.Time // bookmarks.json：外部改盘也失效
	profFp   string    // 主角 profile.json 的内容指纹：主题与形态存在那里
	iconGen  uint64    // 图标缓存的代次
}

// PageRequest 是渲染一页所需的全部输入。用 struct 而不是三个 string 参数：
// ProfilePage(id, home, query) 里三个同类型参数被调用方写反是编译器抓不到的，
// 而写反的后果（把别人的缓存条目当成自己的）正是本设计最贵的那类 bug。
type PageRequest struct {
	ID    string // 渲染哪个档案（权限已由调用方判过）
	Query string // ?q=，空 = 不过滤
	// Add / AddTitle 是 bookmarklet 带来的那一条（§6.3 的 `?add=` 与 `?t=`），
	// 形状与 Query 同一族：它会随每个请求变化（每条书签的地址都不一样），
	// 所以**同样不进渲染缓存**——见下面那个 `if`。理由比 ?q= 更硬：?q= 漏进去
	// 只是键空间膨胀，而 ?add= 漏进去会让**上一条待添加的地址留在缓存里**，
	// 下一个人打开首页看到的是别人 bookmarklet 带过来的 URL。
	Add      string
	AddTitle string
	// 这里曾经还有两个字段，都随 2026-10-10 的管理员边界改判一起作废了：
	// Home（"是不是代管态"，曾是缓存键的一维与横幅的判据）与 Others
	// （徽标下拉里"别人"那几行）。代管态整条下线之后，`sessionLegal` 只认
	// Home==Active，于是**没有任何调用方能再产生它们**——留着两个恒空的字段，
	// 等于给"把代管接回来"留一个看着已经打通的入口。渲染层不该有它们。
}

type Renderer struct {
	s *store.Store
	// ix 不存字段：索引唯一的读者是执行器（exec.go 的 e.ix），New 收进来
	// 就原样交给 newExecutor。留一个没人读的副本 = 下一个人会以为它有用。
	fc    *store.IconCache
	tmpl  *executor
	mu    sync.Mutex
	cache map[cacheKey]Page
}

// New 的三个依赖都不许传 nil：真实装配点（cmd/launchbay）要么给全，
// 要么根本不启动。索引读不出来 = 内嵌产物损坏，降级成"没有图标"
// 会让一个坏镜像跑得很正常（Task 5 的 main.go 同理）。
func New(s *store.Store, ix *icons.Index, fc *store.IconCache) *Renderer {
	return &Renderer{s: s, fc: fc, tmpl: newExecutor(ix, fc), cache: map[cacheKey]Page{}}
}

func (r *Renderer) CacheSize() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cache)
}

func (r *Renderer) Invalidate(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.cache {
		if k.profile == id {
			delete(r.cache, k)
		}
	}
}

func (r *Renderer) ProfilePage(req PageRequest) (Page, error) {
	snap, err := r.s.Current(req.ID)
	if err != nil {
		return Page{}, err
	}
	prof, profFp, err := r.s.LoadProfileWithFingerprint(req.ID)
	if err != nil {
		return Page{}, err
	}
	// Add/AddTitle 透传给执行器（只进 data 属性，不进缓存键——理由见 PageRequest 那条）。
	// Query 非空时整个缓存旁路：键空间会从"每档案一份"变成"每档案每次击键一份"，
	// 为一个局域网页面养无界缓存不值（spec §7.1）。旁路不等于不发 ETag——
	// 下面算的就是内容哈希，浏览器 304 那一路一分不损失。
	// 带 ?q= 或 ?add= 的请求**不进这个缓存**（§7.1 对 ?q= 的原话，?add= 同理）：
	// 键空间会随每次按键/每条书签膨胀，而为一个局域网页面养无界缓存不值。旁路
	// 不等于不发 ETag——下面算的就是内容哈希，浏览器 304 那一路一分不损失。
	key := cacheKey{profile: req.ID, rev: snap.Rev,
		modified: snap.Modified, profFp: profFp, iconGen: r.fc.Generation()}
	if req.Query == "" && req.Add == "" {
		r.mu.Lock()
		if p, ok := r.cache[key]; ok {
			r.mu.Unlock()
			return p, nil
		}
		r.mu.Unlock()
	}

	// 快照目录读一次，决定顶栏那枚「回退」画不画（exec.go 的 HasSnapshots 那条注释
	// 写着为什么）。**不进缓存键**：快照只在写入时增删，而每一次写入都推进 rev，
	// rev 已经在键里——所以"键没变而快照变了"这件事不存在，漏一维不会读到陈旧的数。
	//
	// 读不出来**按有快照处理**（fail open）：为一次目录读失败把能用的撤销入口藏起来，
	// 比多画一枚可能点不动的按钮更糟——真出问题时人是靠这枚按钮找救的。
	snaps, err := r.s.ListSnapshots(req.ID)
	if err != nil {
		snaps = nil
	}
	hasSnaps := err != nil || len(snaps) > 0

	body, err := r.tmpl.profile(prof, snap.B, req.Query, req.Add, req.AddTitle, hasSnaps)
	if err != nil {
		return Page{}, err
	}
	page := Page{Body: body, ETag: makeETag(req.ID, body), Profile: req.ID, Rev: snap.Rev}

	// **写缓存的条件必须与读缓存的那一个字面相同**（req.Query == "" && req.Add == ""）。
	// 之前读侧加了 Add、写侧没加，于是带 ?add= 的那一页被写进缓存，下一个普通首屏
	// 命中它——于是上一个 bookmarklet 的地址留在了别人的页面上。那是串号，本设计
	// 里最贵的一类 bug，而症状是"偶尔看到一条不属于自己的书签"，极难往缓存上想。
	// 抄一处条件的时候漏了另一处，是这条注释存在的全部理由。
	if req.Query == "" && req.Add == "" {
		r.mu.Lock()
		// 只留最近若干页：NAS 上长跑不该让内存单调涨。
		if len(r.cache) > 32 {
			r.cache = map[cacheKey]Page{}
		}
		r.cache[key] = page
		r.mu.Unlock()
	}
	return page, nil
}

// PickerOptions 控制进门页的形状。Setup、单人那两支的标题由调用方（httpapi）算好
// 递进来：**档案数怎么翻成"该问谁 / 该问口令 / 该让他建第一个"是产品判据，
// 不在渲染器里数第二遍**（与 themeAttr、validatedAccent 同一条纪律）。
// 唯一的例外是 ≥2 那一支：它什么都不递，用下面那句兜底问句——问句是文案而不是判据，
// 两个包各写一遍就是两份，而那份重复渲染结果一字不差，行为证人永远看不见，
// 所以它由 httpapi 的一条源码闸钉着。
type PickerOptions struct {
	Preselect string
	// KeepAdd 是 bookmarklet 带进来的 ?add=，必须穿过 PIN 步骤活着回来。
	KeepAdd string
	// KeepAddTitle 是 ?t=（bookmarklet 带的那一页的标题），与 KeepAdd 一起穿过 PIN 步骤。
	KeepAddTitle string
	// Notice 是进门失败后要说的一句人话（由 httpapi 的白名单给出，
	// 渲染器不参与判断该说什么）。
	Notice string
	// Heading 是 h1 的全部内容：0 档案「还没有档案」、1 档案那个人的名字、
	// ≥2「谁来用？」。空串退回「谁来用？」——标题不许因为漏传而变空白。
	Heading string
	// Setup 为真时这一页画引导建档表单，而不是档案卡列表。
	Setup bool
}

func (r *Renderer) PickerPage(opts PickerOptions) (Page, error) {
	profiles, err := r.s.ListProfiles()
	if err != nil {
		return Page{}, err
	}
	// 选择页只取名字与 id：条目、URL、分组名一个都不进视图。
	//
	// **停用的档案不占一张卡**（2026-10-10）：停用是"输对 PIN 也进不去"，给一张
	// 点不进去的卡等于骗人点了。而它**仍然占 CountProfiles 的位置**——那是
	// httpapi 那一层的判据（目录数），所以引导窗口不会因为停用一个人而重开。
	// 两个数字不一样是刻意的：目录数判"这台机器装好了没有"，这张表判"谁进得来"。
	var views []pickerEntry
	listed := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		listed[p.ID] = true
		if p.Disabled {
			continue
		}
		views = append(views, pickerEntry{ID: p.ID, Title: p.Title, Letter: firstChar(p.Title)})
	}
	// 进不去的那几份要在这一页说出口（死路①的对策：读不动的档案从前只是被跳过，
	// 于是"唯一那份坏了"等于一张卡都没有、一句解释都没有的空页面）。
	// 两条顺序上的事都要保住：
	//   - **必须排在 ListProfiles 之后**——那份坏档登记表是 ListProfiles 遍历目录时
	//     填的，早一步取到的是空表（这一条由 TestPickerPage_进不去的档案要点名 钉着）；
	//   - 只取没被列出来的那些——bookmarks.json 坏掉而 profile.json 读得动的人照样有卡，
	//     他的解释在点进去之后那一页，不该在进门页重复一遍。
	// 这里不改判据：档案数怎么翻成"该问谁 / 该问口令 / 该让他建第一个"仍在 httpapi，
	// 读不读得动则是 store 登记的事实。
	var blocked []string
	for _, id := range r.s.Broken() {
		if !listed[id] {
			blocked = append(blocked, id)
		}
	}
	// 标题不靠模板猜：空串退回「谁来用？」，漏传不会渲染出一页空白标题。
	heading := opts.Heading
	if heading == "" {
		heading = "谁来用？"
	}
	v := pickerView{Entries: views, Blocked: blocked, Heading: heading, Setup: opts.Setup,
		Preselect: opts.Preselect, KeepAdd: opts.KeepAdd, KeepAddTitle: opts.KeepAddTitle,
		Notice: opts.Notice}
	body, err := r.tmpl.picker(v)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, ETag: makeETag("picker", body), Profile: "picker", NoStore: true}, nil
}

// BrokenPage 渲染坏档案的解释页：只有档案 id 与 store 给的人话原因
// （文件名+行号），不含书签内容、PIN 哈希或任何令牌。
// 档案列表页可能随时变，且给缓存起来就是留着看旧解释，所以不缓存。
func (r *Renderer) BrokenPage(id, reason string) (Page, error) {
	body, err := r.tmpl.broken(id, reason)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, ETag: makeETag(id, body), Profile: id, NoStore: true}, nil
}

// AdminRow 是管理页一行。它是 httpapi statusProfile 的**视图形状**：字段一一
// 对应，映射在 adminpage.go 一次做完。不让 render 直接吃 statusProfile：那个
// struct 带的是 API 的 JSON 标签，模板不该依赖 API 契约（契约一改，页面就跟着
// 漂，而这两件事本不必同时发生）。
type AdminRow struct {
	ID        string
	Title     string
	Admin     bool
	Disabled  bool // 停用：数据不动、进不来。徽标与按钮的措辞都由它翻。
	Rev       int64
	Modified  string // RFC3339 UTC；空串 = 读不出来（坏档），不编造
	Items     int
	Snapshots int
	Broken    bool
	// CanSetPin / CanDelete 决定这一行的两个写入口渲不渲染。**判据在 httpapi
	// 那一侧算好传进来，模板不判断**——同 validatedAccent 与管理横幅那条纪律。
	//
	// 它们的作用是**不给做不到的承诺**：管理页里一个点了必被 403/303 的按钮，
	// 就是一条死按钮，而死按钮比没有更费人（它让人以为自己能删自己）。
	// 判据是"这一行是不是当前会话正在用的那一份"——**不是"它是不是管理员"**：
	// 管理员切进别人档案之后，回管理页给自己家设 PIN 是合法的，那时该出链接。
	CanSetPin bool
	// CanToggleDisabled 决定停用/启用那一枚链接渲不渲染（2026-10-10）。
	// 判据同样在 httpapi 那一侧算：不是当前会话正在用的那一份，且不是坏档。
	// 模板用 Disabled 翻措辞（今天停、明天启），不自己判状态。
	CanToggleDisabled bool
	CanDelete         bool
}

// AdminSentinel 是管理页在 X-LB-Profile 与 ETag 前缀里的名字。它刻意长成
// `model.ValidSlug` **拒收**的样子（开头一个 `-`）："admin" 本身是个能建出来的
// 档案 id，谁真建了它，⑥ 的 SW 就可能把这张跨档案的名单页当成那个档案的页面
// 存进 `html-admin` 桶。本期这几页是 no-store、SW 还不存在，所以今天不可达，
// 但形状要在这里定下来，而不是等 ⑥ 去猜"admin"这一档是谁。
const AdminSentinel = "-admin"

// AdminPage 渲染 §8.8 的管理页。NoStore 是硬要求：这一页跨所有档案，缓存起来
// 等于把全家庭的名字与条目数留在同一台电脑的下一个人手里（§7.1）。
// 不缓存也不给 ETag——writePage 在 NoStore 那一支本来就不发 ETag。
//
// notice 是由 `?err=` 查白名单查出的一句人话（空串就不画）：建档表单失败后必须
// 能说出"刚才没建上"，否则那条通路是静默失败的。判据不在渲染器里——它只印递进来的
// 那句，白名单与键的取法都在 httpapi（同 PickerOptions 的 Heading/Setup）。
func (r *Renderer) AdminPage(rows []AdminRow, notice string) (Page, error) {
	body, err := r.tmpl.admin(rows, notice)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, ETag: makeETag(AdminSentinel, body), Profile: AdminSentinel, NoStore: true}, nil
}

// AdminField 是确认页上的一个输入格。
type AdminField struct {
	Name      string
	Label     string
	Type      string // text / password
	Hint      string // 字段说明，空串就不印（印了要挂 aria-describedby）
	Autofocus bool
}

// AdminChoice 是"从几个里选一个"的一格。
//
// 与 AdminField 的自由输入是**两种形状**，不是同一种的变体：选一个这件事
// HTML 本来就有原生控件（radio），零 JS 可用、可键盘操作、读屏软件会念出
// 选中项——自己拿 <select> 或一排按钮拼一份，换来的只是"看起来像"。
type AdminChoice struct {
	Value string
	Label string
	Hint  string // 这一格的补充说明，空串不印
	// Checked 是**服务端**算出来的默认值，不用 JS 去读"当前值"：
	// 确认页要提交的那一格，服务端本来就知道该预选哪一格。
	Checked bool
}

// AdminAction 是"针对某一个档案的一次表单"这一页的全部输入。
//
// 设 PIN 与删档共用它：两者的形状完全一样（针对一个档案 → 说明后果 → 一个
// 表单），为它们各写一个模板只会多出一份要同步的 CSS 与结构。
type AdminAction struct {
	// ID 是目标档案 id，藏在表单里。**空串就不印那一格**：回退页的目标是
	// 当前会话自己（不接受申报），表单里多一个由客户端说了算的 profile 就是
	// 一条可被改写的字段，而这一页的权限判据是会话。
	ID      string
	Title   string // <title>
	Heading string // <h1>
	// Explain 是**后果**，印在提交按钮之前。删档那一页存在的全部意义就是让
	// "会发生什么"先被读到，所以它不是可选项。
	Explain string
	Notice  string // 由 ?err= 查白名单查出的一句人话，空串不画
	Action  string // 表单 action
	Submit  string // 按钮文字
	Danger  bool   // 删档：按钮走危险样式
	Fields  []AdminField
	// Choices 与 Fields 是互斥的两种形状。空 Choices 一格也不画——设 PIN、
	// 删档、停用那三页走的一直是 Fields，它们不会因为这一族存在而多出一个
	// 空 fieldset。
	Choices    []AdminChoice
	ChoiceName string // 这一族的表单字段名（回退页是 to-rev）
	// ChoiceLegend 是那一族的组标题（"选一版"）。印在 fieldset 的 legend 上，
	// 读屏软件靠它知道这一组 radio 是什么。
	ChoiceLegend string
	// NavHref/NavLabel 覆盖右上角那枚返回徽标。空值回落 "/admin" 与「回管理页」：
	// 管理页那三页本来就都回管理页，不必各写一遍；而回退页要回**自己的档案页**，
	// 那里回管理页是死路（非管理员根本没有那一页，点了会吃一个不解释的 303）。
	NavHref  string
	NavLabel string
	// NoForm 是"这一页没有可提交的东西"，整张表单不画（回退页零份快照时那一支：
	// 画一个只有按钮的空表单，等于给人一条点了之后没有出路的路）。
	//
	// **它必须显式，不能从"Fields 与 Choices 都空"推**：删档页正是两者皆空而
	// 仍然要按钮的那一张——它提交的就是那个 hidden profile。推出来的话，
	// 删档会跟着这条一起消失，而且现象是"删档页少了个按钮"，极难联想到这一行。
	NoForm bool
}

// AdminActionPage 渲染"确认页"这一族：设 PIN / 删档 / 停用（管理页那几张）与
// 回退到某一版（本人那一页）。
//
// 后者是 2026-10-11 加进来的第四张脸，形状仍然落在同一个族里——它同样是
// "说明后果 → 一个表单 → 一次提交"，为它另写一个模板只会多出一份要同步的
// CSS 与结构（而 CSS 那条操作线只剩 1.12 倍余量）。
//
// NoStore 与 AdminPage 同理由：这一页点名到具体档案，缓存下来等于把"谁家要
// 被删"留在共用电脑的缓存里。ETag 带上档案 id，同上（跨档案命中是这里最贵的一类 bug）。
// ImportView 是浏览器收藏夹导入那三步的视图。
//
// 一个类型带两种形状（Mode）而不是两个页面：这两步是**同一个任务的进度**，
// 拆成两个类型就要在两处各写一遍"标题/提示/回哪"。判据在 httpapi
// （能不能导、这份文件读出多少），模板只印——与管理页那两张确认页同一纪律。
type ImportView struct {
	// Mode 是 "pick"（选文件）或 "report"（看报告 → 确认写入）。
	Mode    string
	Profile string
	Heading string
	Explain string
	Notice  string
	// 下面这些只在 report 形状下有值。
	Count    int    // 会写进去几条
	Skipped  int    // 没能进来的几条（过不了写路径的校验；报告里有逐条明细）
	Groups   int    // 会建几个分组
	Report   string // bookimport 那份人话报告，原文照贴
	Token    string // 一次性令牌，确认页带着它
	FileName string // 上传时的文件名，只为让人确认"选对了文件"
}

// ImportPage 渲染导入页。no-store：报告与令牌都在这一次响应的上下文里，
// 缓存住它等于把"这个人打算导什么"留给下一个共用这台电脑的人。
func (r *Renderer) ImportPage(v ImportView) (Page, error) {
	body, err := r.tmpl.importPage(v)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, NoStore: true, Profile: v.Profile}, nil
}

func (r *Renderer) AdminActionPage(v AdminAction) (Page, error) {
	body, err := r.tmpl.adminAction(v)
	if err != nil {
		return Page{}, err
	}
	return Page{
		Body:    body,
		ETag:    makeETag(AdminSentinel+"-"+v.ID, body),
		Profile: AdminSentinel,
		NoStore: true,
	}, nil
}

// ETag 带档案名：内容字节相同时也必须不同名，否则浏览器与 SW
// 会跨档案命中——那是本设计里最贵的一类 bug。
func makeETag(profile string, body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + profile + "-" + hex.EncodeToString(sum[:])[:16] + `"`
}

func firstChar(s string) string {
	for _, r := range s {
		return string(r)
	}
	return "?"
}
