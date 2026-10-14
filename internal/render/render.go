// Package render 把一个档案渲染成完整 HTML。渲染在服务端做，
// 因此首屏不需要 JS、数据直接内联，关掉脚本也能看（spec §7.2）。
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
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
	// lang 是**站级**界面语言（data/settings.json）。它进键的理由与 HasSnapshots
	// 不进键正好相反：快照只在写入时增删，而每一次写入都推进 rev，所以"键没变
	// 而快照变了"不可能发生，漏一维无害。语言**不推进任何 rev**——管理员在管理页
	// 改一次语言，所有档案的 rev、mtime、指纹全都一模一样，键却必须变，否则每个
	// 档案都会拿缓存里那份旧语言的页面继续发，而那个旧条目的键与新的完全相等。
	//
	// 放在键里而不是"改语言时遍历清一遍缓存"：清缓存是**要记得做**的事，忘了就是一个
	// "改了不生效、得重启容器"的现象；进键是**不用记得**的事，判据只有一份。
	lang i18n.Lang
	// hasAdmin 是"这次请求有没有带着一把真的管理权"（2026-10-12）。它与 lang
	// 同理必须进键：顶栏那枚「管理」徽标**由它决定画不画**，而它不推进 rev、不改
	// mtime、不改 profile.json——同一个人在同一个浏览器里先输了 key、又清了 key，
	// 键却必须变，否则第二个人会拿到带着管理入口的缓存页面。
	hasAdmin bool
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
	// HasAdmin 只决定顶栏那枚「管理」徽标画不画。判据由调用方（httpapi）算——
	// 本次请求带的是不是一把真的 lb_admin。进缓存键的理由见 cacheKey 那条。
	HasAdmin bool
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
	lang := r.s.Language()
	key := cacheKey{profile: req.ID, rev: snap.Rev,
		modified: snap.Modified, profFp: profFp, iconGen: r.fc.Generation(), lang: lang,
		hasAdmin: req.HasAdmin}
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

	body, err := r.tmpl.profile(prof, snap.B, req.Query, req.Add, req.AddTitle, hasSnaps, req.HasAdmin, i18n.Load(lang))
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
	// HasAdmin 为真时页面顶端给一个进管理页的口子（2026-10-12）。判据在 httpapi
	// （本次请求带的是不是一把真的管理权），渲染器不判断。
	HasAdmin bool
}

func (r *Renderer) PickerPage(opts PickerOptions) (Page, error) {
	profiles, err := r.s.ListProfiles()
	if err != nil {
		return Page{}, err
	}
	// 选择页只取名字与 id：条目、URL、分组名一个都不进视图。
	//
	// **2026-10-12 例外**：视图里多了一张脸（Icon）。判断本身在 icon.go 的
	// faceOf，而它要问的是"这份档案最常点的是哪个站"——那答案只能从书签里去
	// 拿。这里**只带一张脸进模板**：一个图标与它藏在哪条书签后面，不泄露一份
	// 收藏里有什么（这页是给全家都能看到的进门页）。
	//
	// **停用的档案不占一张卡**（2026-10-10）：停用是"输对 PIN 也进不去"，给一张
	// 点不进去的卡等于骗人点了。而它**仍然占 CountProfiles 的位置**——那是
	// httpapi 那一层的判据（目录数），所以引导窗口不会因为停用一个人而重开。
	// 两个数字不一样是刻意的：目录数判"这台机器装好了没有"，这张表判"谁进得来"。
	//
	// 代价：这一页因此要为**每份**档案读一次 bookmarks.json（只读文件，不 stat ——
	// `Current` 会多读一次 mtime，而这一页要的是书签本身）。
	//
	// **量过**（两轮交错 A/B，各 60 次，免得机器抖动全砸在一边）：本机 3 份档案、
	// 书签文件都只有几百字节，选择页 p50 从 17.0/15.0ms 变成 21.2/18.4ms——多出
	// 的是**每份档案一次文件读**的量级（约 1ms/份），不是解析的量级。Windows 的
	// Docker bind 挂载每次文件操作约 1ms；Linux 本地盘是零点几毫秒，NAS 上取决于
	// 它的文件系统（SMB 上会明显更贵——这一格要在真 NAS 上重测，本机测不出来）。
	//
	// 为什么不给这张脸加缓存：能当键的 rev 或 mtime 都得先读/先 stat 才拿得到，
	// 省不掉那一次文件操作；而缓存本身会成为"这一份现在几 rev"的第二处真相，
	// 错了就是一张永远不换的脸。这里选择每次老实读一次。
	//
	// 档案读不出来（坏档）时那一格走首字母色块——点进去之后才有解释页，不在这一
	// 页重复。
	var views []pickerEntry
	listed := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		listed[p.ID] = true
		if p.Disabled {
			continue
		}
		b, err := r.s.LoadBookmarks(p.ID)
		if err != nil {
			b = nil
		}
		views = append(views, pickerEntry{ID: p.ID, Title: p.Title,
			Icon:     faceOf(p, b, r.tmpl.ix, r.fc),
			NeedsPIN: auth.HasPin(p.PIN)})
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
		Notice: opts.Notice, HasAdmin: opts.HasAdmin}
	body, err := r.tmpl.picker(v, i18n.Load(r.s.Language()))
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, ETag: makeETag("picker", body), Profile: "picker", NoStore: true}, nil
}

// BrokenPage 渲染坏档案的解释页：只有档案 id 与 store 给的人话原因
// （文件名+行号），不含书签内容、PIN 哈希或任何令牌。
// 档案列表页可能随时变，且给缓存起来就是留着看旧解释，所以不缓存。
func (r *Renderer) BrokenPage(id, reason string) (Page, error) {
	body, err := r.tmpl.broken(id, reason, i18n.Load(r.s.Language()))
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
	ID    string
	Title string
	// **没有 Admin**（2026-10-12）：档案不再有管理员属性，表格里也就没有那一列徽标。
	// "谁能管这台机器"是**这一页的访问权**，不是**那一行档案的属性**——把它印在
	// 每一行上只会让人以为每个档案各有各的管理权。
	Disabled  bool // 停用：数据不动、进不来。徽标与按钮的措辞都由它翻。
	Rev       int64
	Modified  string // RFC3339 UTC；空串 = 读不出来（坏档），不编造
	Items     int
	Snapshots int
	Broken    bool
	// CanSetPin / CanDelete 决定这一行的两个写入口渲不渲染。**判据在 httpapi
	// 那一侧算好传进来，模板不判断**——同 validatedAccent 那条纪律。
	//
	// 它们的作用是**不给做不到的承诺**：管理页里一个点了必被 403/303 的按钮，
	// 就是一条死按钮，而死按钮比没有更费人（它让人以为自己能删自己）。
	// 判据是"这一行是不是当前会话正在用的那一份"——**不是"它是不是管理员"**：
	// 管理员身份是**请求级**的（这一次有没有那把 key），而这一行是**档案级**的，
	// 两者不在一个维度上，所以登录在 kid 那份的管理员给自己家（dad）设 PIN 是
	// 合法且该出链接的。
	CanSetPin bool
	// CanToggleDisabled 决定停用/启用那一枚链接渲不渲染（2026-10-10）。
	// 判据同样在 httpapi 那一侧算：不是当前会话正在用的那一份，且不是坏档。
	// 模板用 Disabled 翻措辞（今天停、明天启），不自己判状态。
	CanToggleDisabled bool
	CanDelete         bool
	// IsActive = 这一行就是当前会话正在用的那一份。**它只为了把那个空格填上**：
	// 三个 Can* 全为假的时候这一格会渲染成一个空的 `<span class="acts">`，而
	// 1440 实拍里那一行读起来像"这一份没有动作"——可它不是没有，是**按下去必然
	// 被拒**（`/admin/delete` 对自己会 303 `?err=self`）。空格子与"没有这回事"在
	// 屏上是同一种样子，而这一格本来就有答案：**它正被你自己用着**。
	//
	// 单独一个字段而不是让模板拿三个 Can* 反推：反推要把"三个都假"这个组合
	// 当成一条判据，而组合判据迟早会跟新的动作列分家。
	IsActive bool
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
	body, err := r.tmpl.admin(rows, notice, i18n.Load(r.s.Language()))
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
	// ChoiceNote 是 legend 底下那一句**读法说明**，空串不画。
	//
	// 它存在的理由是回退页那一处：只有分组真的变过的那些候选才列组名，其余行
	// 光秃秃只剩「第 N 版 · M 条书签」。这个取舍是对的（13 份快照的实拍里，
	// 12 行的组名一字不差地重复），但**取舍本身没有出现在页面上**——于是
	// 同一张列表里，"没写分组"读起来像"那一版没有分组"，而它真正的意思是
	// "与上一行一样"。两句话差别很大：前者让人以为退回去会丢目录。
	ChoiceNote string
	// NavHref/NavLabel 覆盖右上角那枚返回徽标。空值回落 "/admin" 与「回管理页」：
	// 管理页那三页本来就都回管理页，不必各写一遍；而回退页要回**自己的档案页**，
	// 那里回管理页对没有管理权的人是一条多余的中转（他会落到登录页，输完 key
	// 又回到管理页，而不是回到自己那份书签）。
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
// ImportView 是浏览器收藏夹导入那几屏的视图。
//
// 一个类型带几种形状（Mode）而不是几个页面：这几屏是**同一个任务的进度**，
// 拆成几个类型就要在几处各写一遍"标题/提示/回哪"。判据在 httpapi
// （能不能导、这份文件读出多少、这次写成了没有），模板只印——与管理页那两张
// 确认页同一纪律。
type ImportView struct {
	// Mode 是 "pick"（选文件，两个按钮）、"report"（看报告 → 确认写入）、
	// "done"（直接导入之后的结论）或 "blocked"（档案非空，这条路不通）。
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
	// 下面两个只在 done 形状下有值（2026-10-12 的"直接导入"）。
	Applied int
	// SkippedItems 逐条点名。**不许拿 Skipped 那个数字代替它**：导入唯一会让人
	// 后悔的信息就是"少的是哪几条"，只给个数的话那几条就没人知道了。
	SkippedItems []SkippedEntry
}

// SkippedEntry 一条没能进档案的书签。它是 render 自己的形状而不是
// bookimport.SkippedItem：渲染层不认识解析器，知道的只有"这三样要印出来"。
type SkippedEntry struct{ Name, URL, Reason string }

// ImportPage 渲染导入页。no-store：报告与令牌都在这一次响应的上下文里，
// 缓存住它等于把"这个人打算导什么"留给下一个共用这台电脑的人。
func (r *Renderer) ImportPage(v ImportView) (Page, error) {
	body, err := r.tmpl.importPage(v, i18n.Load(r.s.Language()))
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, NoStore: true, Profile: v.Profile}, nil
}

func (r *Renderer) AdminActionPage(v AdminAction) (Page, error) {
	body, err := r.tmpl.adminAction(v, i18n.Load(r.s.Language()))
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

// AdminLoginPage 是 `GET /admin` 在**没有管理权**时渲染的那一页（2026-10-12）。
//
// 它与 AdminPage 共用 AdminSentinel：这一页与管理页同属"管理那一族"，而 ⑥ 的 SW
// 对 `-admin` 一律不存（见那个常量的注释）——登录页里没有敏感内容，但让同一族
// 共用一个哨兵名，少一处将来要记住的例外。
func (r *Renderer) AdminLoginPage(notice string) (Page, error) {
	body, err := r.tmpl.adminLogin(notice, i18n.Load(r.s.Language()))
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, Profile: AdminSentinel, NoStore: true}, nil
}

// HistoryRow 是改动历史页上的一行 = **一次动作**（一批 ops 落进同一个 rev，
// 或一个不产生 rev 的账号类动作）。
//
// 句子都在调用方查好表再递进来（与 AdminAction 同一纪律）：这一行的短句要把
// 路径里的 id 换成"那条书签叫什么"，而只有拿着书签与快照的那一侧算得出。
// At 是唯一留给模板侧格式化的字段——时间的写法只有 exec.go 那一份（shortTime）。
type HistoryRow struct {
	At      string
	Channel string // 渠道名，已经翻好（「网页」「浏览器扩展」）
	Actor   string // 「由 管理员」那一句；经手人就是这一份自己时留空、不画
	Rev     int64  // 0 = 这次动作不产生书签版本（改 PIN、停用、重建那一族）
	Count   int    // 这一次一共几处改动（**可能多于 Ops 的长度**，那是截断后的）
	Ops     []string
	More    string // 「…另外还有 N 处没列出来」，空串 = 没截断
}

// HistoryView 是改动历史页的输入。行**按时间从新到旧**递进来（ReadHistory 就是
// 这个顺序），分哪几天由执行器算——"今天/昨天"是显示的事，而它离不开语言。
type HistoryView struct {
	// Profile 只用于 X-LB-Profile：这一页点名到具体的人，缓存与离线桶都要按它分。
	Profile string
	Heading string
	Explain string
	Notice  string
	// Foot 是截断结论（"只列最近 100 次 · 一共 342 次"），空串不画。
	Foot string
	Rows []HistoryRow
}

// HistoryPage 渲染改动历史。no-store 与不给 ETag 同 ImportPage：这一页写的是
// "谁在什么时候动过这个人的书签"，把它留在共用这台电脑的下一个人的缓存里，
// 就是这一整个产品要防的那件事。
func (r *Renderer) HistoryPage(v HistoryView) (Page, error) {
	body, err := r.tmpl.history(v, i18n.Load(r.s.Language()))
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, NoStore: true, Profile: v.Profile}, nil
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

// PrefsPage 渲染台面面板的独立页（GET /prefs）。**JS 关着时那条路**：
// 顶栏那颗「台面」是 `<a href="/prefs">`，开着 JS 时 app.js 把它拦成浮层，
// 关着时它就是一次普通跳转——而两种形态渲染的是同一份标记。
//
// 不进渲染缓存：这一页每个字段都是台面偏好（改一个就换一个 profile 指纹），
// 而它是"出了事去找设置"的落点，不该是浏览器缓存里的旧值。
func (r *Renderer) PrefsPage(id string) (Page, error) {
	prof, _, err := r.s.LoadProfileWithFingerprint(id)
	if err != nil {
		return Page{}, err
	}
	// 快照目录读一次决定面板里那枚「回退」画不画，判据与 ProfilePage 同一句
	// （exec.go 的 HasSnapshots 那段）。读不出来按**有快照**处理：为了
	// 一次目录读失败把撤销入口藏起来，比多画一枚可能点不动的按钮更糟。
	snaps, err := r.s.ListSnapshots(id)
	hasSnaps := err != nil || len(snaps) > 0
	cat := i18n.Load(r.s.Language())
	// 面板上凡是**书签文档里**的格子都从这一次读里来（分组清单与「点开书签」的
	// openInNewTab）：两次读之间文件被人换掉的话，屏上那两个格子会来自同一份
	// 读数——而它们都只是显示，写入侧仍在锁内重读（applyBatch）。
	gs, broken, openNewTab := r.prefsGroups(id, cat)
	body, err := r.tmpl.prefsPage(prof, hasSnaps, gs, broken, openNewTab, cat)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, ETag: makeETag(id, body), Profile: id, NoStore: true}, nil
}

// prefsGroups 读出面板「分组」那一节要用的清单。
//
// 它**走的是与 ProfilePage 相同形状的 groupView**（ID/Title/Layout/Accent，Accent
// 经 validatedAccent 过滤），但必须是**另一次读**，不能从 ProfilePage 的结果里截：
// 那一页要为整页渲染标题、条目与导航树，而搜索时只渲染命中的分组——于是"面板里
// 少了几行"会取决于当时有没有在搜东西，而面板与搜索词毫无关系。
//
// 读不出来**不算失败**：把这一节撤掉并说清为什么，其余四行照常可用。这与
// `/prefs` 的"坏档不挡"是同一条纪律——改台面只写 profile.json，而书签文件读坏了
// 正是最需要改排版看看的时候。
func (r *Renderer) prefsGroups(id string, cat *i18n.Catalog) ([]groupView, string, bool) {
	b, err := r.s.LoadBookmarks(id)
	if err != nil {
		return nil, cat.T("prefs.groups.broken"), false
	}
	out := make([]groupView, 0, len(b.Groups))
	for _, g := range b.Groups {
		out = append(out, groupView{ID: g.ID, Title: g.Title, Layout: g.Layout,
			Accent: validatedAccent(g.Accent), AccentCSS: CSSAccent(g.Accent)})
	}
	return out, "", b.OpenInNewTab
}
