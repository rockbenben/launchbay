// Package apierr 是这台机器上**错误分类**的唯一定义处。
//
// 为什么要有这么一层：界面上的句子只能按"错误是哪一类"来选，而 HTTP 状态码
// 不够分——422 既可能是"那一组已经不在了"（该说：刷新看看），也可能是
// "这一条已经存过了"（该说：去改它，别再存一遍）。以前只有状态码可查，
// 于是 422 在 2026-10-13 那一版被一律说成"已经不在了"，而重复添加同一网址的人
// 被告知的是"东西没了，刷新一下"——两句都不挨着。
//
// 所以：**每个 JSON 错误响应都带一个 kind**（kebab-case，本包的常量），
// `detail`/`reason` 仍是给人排查的中文原话，两边各管各的读者：
// 界面、扩展通知、CLI 从 kind 出句子；日志与 AI 读原话。
//
// 三条规矩：
//   - kind **只增不改不删**。改一个已发布的 kind 等于让旧版扩展与脚本静默落进
//     兜底句子，那比多一个码糟得多；要换说法就改中文原话，别动码。
//   - 取值必须是本表里的一个（`TestKind表与文档一致` 钉代码 == openapi == AGENTS.md）。
//   - 拿不到分类时落 `invalid-request` 或 `ops-invalid`，**不许留空**：
//     空 kind 会让客户端退回状态码那一档，症状是"某几条错的说法永远不对"。
package apierr

import "errors"

// 凭证与进门。
const (
	CredentialMissing   = "credential-missing"    // 什么都没带
	CredentialUnknown   = "credential-unknown"    // 带了，认不出来（换一枚，重试同一发没意义）
	CredentialScope     = "credential-scope"      // 认出来了，但作用域不含这个档案（换对档案/对角色）
	CredentialAdminOnly = "credential-admin-only" // 这一条只认管理员
	// CredentialAdminToken 是**填了管理员那把 key**（2026-10-19 加）。
	//
	// 它与 CredentialScope 分开，是因为那两句出路是**相反**的：
	//   - `credential-scope`：你有一枚档案令牌，只是管别的档案 → 换一个档案 id
	//   - `credential-admin-token`：你填的是 LAUNCHBAY_ADMIN_TOKEN → 这把钥匙
	//     **根本不开书签这一面**，扩展/CLI 要配的是那份档案自己的令牌
	//
	// 从前管理员 key 落到 `credential-scope` 上（`importapi.go` 那处单独拦的
	// 403），于是扩展把这句话翻成"它管的是别的档案，或者那个 id 打错了"——
	// **而真相是"你拿错了钥匙"**。用户照着这句话去改档案 id，改到天亮也进不去。
	// 密钥是层级不对，不是作用域不对。
	CredentialAdminToken = "credential-admin-token"
	PinWrong             = "pin-wrong" // PIN 不对 / 旧 PIN 不对
)

// 档案本身。
const (
	ProfileNotFound       = "profile-not-found"
	ProfileBroken         = "profile-broken"
	ProfileExists         = "profile-exists"
	ProfileDisabled       = "profile-disabled"
	ProfileNotRebuildable = "profile-not-rebuildable"
)

// 写入通路。
const (
	RevConflict    = "rev-conflict"
	SnapshotFailed = "snapshot-failed"
	OpsInvalid     = "ops-invalid"  // ops 校验未过、整批未写入（总称）
	DuplicateID    = "duplicate-id" // 这一条已经在了：不是"东西没了"，是"别存第二遍"
	MissingTarget  = "missing-target"
	IconUnknown    = "icon-unknown"
)

// 请求形状。
const (
	FieldInvalid   = "field-invalid"
	FieldUnknown   = "field-unknown"
	FieldDuplicate = "field-duplicate"
	BodyUnreadable = "body-unreadable"
	QueryInvalid   = "query-invalid"
	URLIncomplete  = "url-incomplete"
	InvalidRequest = "invalid-request" // 兜底：形状不对但说不出是哪一格
)

// 路径与主机。
const (
	RouteNotFound = "route-not-found"
	HostInvalid   = "host-invalid"
)

// 导入那一条通路。
const (
	ImportNoBookmarks     = "import-no-bookmarks"
	ImportNothingAccepted = "import-nothing-accepted"
	ImportPlanStale       = "import-plan-stale"
	// ImportTargetNotEmpty 是"这一份里已经有书签了"那个形状（2026-10-19）。
	// 导入是**把浏览器那套导进来**，不是往现有档案上叠加——所以它与
	// `import-no-bookmarks` 同族：**都是这一次的内容不对**，422。
	ImportTargetNotEmpty = "import-target-not-empty"
)

// 抓目标网页这一条（resolve）。
const (
	TargetUnreachable = "target-unreachable"
)

// 服务端自己。
const (
	InternalError = "internal-error"
)

// Info 是一个 kind 的登记项。**默认状态码**记在这儿，为的是文档与代码对得上：
// 同一个码在两个端点上答不同状态，读者就该怀疑其中一处写错了。
type Info struct {
	Status int
	// WayOut 是这一类错误**该让人做什么**的中文摘要。它只给文档与一致性闸用，
	// 界面上不印它——界面句子在 internal/i18n/messages 里，那儿再写一份就会漂。
	WayOut string
}

// Registry 是本包所有 kind 的登记表。加常量必须同时加这里，
// 否则 TestKind表与文档一致 会当场红（漏登记的码在文档里不存在，
// 而 AI 读者照文档生成请求时永远学不到它）。
var Registry = map[string]Info{
	CredentialMissing:     {401, "把凭证带上：浏览器那条路是登录，扩展与脚本那条路是 Bearer 令牌"},
	CredentialUnknown:     {401, "这一把不被认：多半是少复制了字符，或者机器那边换过令牌"},
	CredentialScope:       {403, "换一枚作用域正好对上这个档案的令牌（管理员那把只管账号）"},
	CredentialAdminOnly:   {403, "这一条要管理员身份，档案令牌不行"},
	CredentialAdminToken:  {403, "这是管理密钥，扩展要配的是那份档案自己的令牌"},
	PinWrong:              {401, "再输一次；忘了就走管理页那格设新 PIN"},
	ProfileNotFound:       {404, "确认这个 id 还在，或先建一份"},
	ProfileBroken:         {503, "先修好那个数据文件（管理页会说要重建）"},
	ProfileExists:         {409, "换一个名字或 id"},
	ProfileDisabled:       {401, "这一份被停用了：去管理页点「启用」，或者换一份进"},
	ProfileNotRebuildable: {409, "这份读得动，不需要重建；改 PIN 走 profiles/pin"},
	RevConflict:           {409, "重读一次再写（别人刚动过）"},
	SnapshotFailed:        {507, "什么都没改；腾出空间或稍后再试"},
	OpsInvalid:            {422, "整批没写入；看 errors[] 逐条的 kind 与 field"},
	DuplicateID:           {422, "这一条已经在了：去改它，别再存第二遍"},
	MissingTarget:         {422, "那一组/那一条不在了：刷新之后再看"},
	IconUnknown:           {422, "换一个查得到的图标名"},
	FieldInvalid:          {422, "改那一格的值"},
	FieldUnknown:          {422, "删掉不认识的字段再发"},
	FieldDuplicate:        {422, "同一字段只带一次"},
	BodyUnreadable:        {422, "重新提交一次"},
	QueryInvalid:          {400, "把查询参数补全/改对"},
	URLIncomplete:         {422, "地址要有协议和主机名"},
	InvalidRequest:        {422, "看 detail 说的那一格"},
	RouteNotFound:         {404, "这条路径不存在"},
	HostInvalid:           {400, "换个地址访问"},
	ImportNoBookmarks:     {422, "确认选的是浏览器导出的那份 .html"},
	ImportNothingAccepted: {422, "这份文件里的地址都进不来，换一份"},
	ImportPlanStale:       {410, "重新发起一次导入，拿一份新报告"},
	ImportTargetNotEmpty:  {422, "换一个空档案，或先在界面上清空这一份"},
	TargetUnreachable:     {502, "那台机器可能没开；照样存下来也行，回头补标题"},
	InternalError:         {500, "稍后再试；持续发生看容器日志"},
}

// Kinded 是"错误自己知道自己属于哪一类"。源头知道就得在这儿说——
// 到了 HTTP 那一层再靠**中文原话做子串匹配**去猜，是这条通路最容易烂掉的地方：
// 改一个标点，分类就静默错。
type Kinded interface {
	APIKind() string
}

type wrapped struct {
	err  error
	kind string
}

func (w *wrapped) Error() string   { return w.err.Error() }
func (w *wrapped) Unwrap() error   { return w.err }
func (w *wrapped) APIKind() string { return w.kind }

// With 给一个错误贴上分类。kind 必须是本包的常量（不在这儿重复校验，
// 登记表与常量的对应由测试钉）。
func With(err error, kind string) error {
	if err == nil {
		return nil
	}
	return &wrapped{err: err, kind: kind}
}

// New 直接造一个带分类的错误。
func New(kind, msg string) error {
	return &wrapped{err: errors.New(msg), kind: kind}
}

// Kind 沿错误链找**第一个**带分类的；没有就返回 ""。
// 返回空串是合法结果，调用方要落一个兜底码而不是留着空。
func Kind(err error) string {
	var k Kinded
	if errors.As(err, &k) {
		return k.APIKind()
	}
	return ""
}
