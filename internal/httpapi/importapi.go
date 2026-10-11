package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/bookimport"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/render"
)

// 上传上限。maxBodyBytes 的 1 MiB 是给 JSON 表单用的（建档/PIN 用不到更大），
// 而浏览器收藏夹导出动辄好几 MB——重度用户的 Chrome HTML 很容易上兆。
// 8 MiB 是"绝大多数人够用"与"不把一台家用机器的内存吃穿"之间那道线。
const maxImportBytes = 8 << 20

// 暂存一份**已解析但未写入**的导入计划，从"看报告"走到"确认写入"之间要用。
//
// 为什么要有暂存而不是让用户再选一次文件：确认页那个表单里没法预填
// `<input type=file>`（浏览器不允许），让人为了点一次确认重选一遍文件是
// 纯折磨。
//
// 边界三条，都是为了不让它长成一堆躺在内存里的用户数据：
//   - 绑档案：令牌带着 profile，按钮页也只能用同一个档案的会话提交；
//   - 15 分钟过期、总量 64 份（懒清扫：每次新建时顺带扔掉过期的）；
//   - **一次性**：apply 成功或失败都删掉，重放要重新上传。
const (
	importStashTTL = 15 * time.Minute
	importStashMax = 64
)

type importStash struct {
	profile string
	raw     []byte
	at      time.Time
}

func (s *Server) stashImport(profile string, raw []byte) string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// 系统熵不可用时不退化成"没有令牌也能写"：这条路径直接报错更安全。
		return ""
	}
	token := hex.EncodeToString(buf[:])
	now := time.Now()

	s.importMu.Lock()
	defer s.importMu.Unlock()
	for k, v := range s.imports {
		if now.Sub(v.at) > importStashTTL {
			delete(s.imports, k)
		}
	}
	// 超量时先扔最早的那几份——不因为"存满了"就拒掉一次正当的上传。
	for len(s.imports) >= importStashMax {
		oldest, oldestAt := "", time.Now()
		for k, v := range s.imports {
			if v.at.Before(oldestAt) {
				oldest, oldestAt = k, v.at
			}
		}
		if oldest == "" {
			break
		}
		delete(s.imports, oldest)
	}
	s.imports[token] = importStash{profile: profile, raw: raw, at: now}
	return token
}

func (s *Server) takeImport(token, profile string) ([]byte, bool) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	v, ok := s.imports[token]
	if !ok || v.profile != profile || time.Since(v.at) > importStashTTL {
		delete(s.imports, token)
		return nil, false
	}
	// 一次性：取走就删。同一枚令牌再发一次会拿到"过期"，而不是把同一批
	// ops 应用两遍。
	delete(s.imports, token)
	return v.raw, true
}

// importGuard 是这三条端点共用的入口闸：只认**本人的会话**、只动自己的档案、
// 且档案必须还是空的。
//
// "必须是空的"不是洁癖，是 CLI 已有的规矩（import-bookmarks 同样拒非空档案，
// 说的是"换一个空档案，或者先自己在界面上清空它"）。真要合并就得有一套
// 对着既有档案做对账的判据——那是另一件事，不该由"网页上多一个入口"顺带
// 发明出来。导入卡片只在档案一条书签都没有时出现，这个闸与那条提示因此
// 天然对齐：看得见入口的时候，它一定进得去。
func (s *Server) importGuard(w http.ResponseWriter, r *http.Request) (string, bool) {
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		redirectPicker(w)
		return "", false
	}
	id := sess.Active
	snap, err := s.deps.Store.Current(id)
	if err != nil {
		s.writeProfileReadFailure(w, id, err)
		return "", false
	}
	if n := bookimport.CountItems(snap.B); n > 0 {
		// **不 303 带 ?err=**：那把键只在选择页被印，而登录着的会话回到 "/"
		// 渲染的是**档案页**，提示会被无声吞掉——正是
		// Test提示键只发白名单里有的 那道 AST 闸在防的事（"表单失败了而
		// 人不说话"）。所以直接在这一页把话说清楚。
		s.renderImport(w, r, render.ImportView{
			Mode: "blocked", Heading: "不用导了",
			Notice: fmt.Sprintf("这个档案里已经有 %d 条书签了。导入只对「一条书签都没有」的档案开放——"+
				"想加书签直接在页面上「＋ 添加」，那才是往已有清单里加东西的路。", n),
		})
		return "", false
	}
	return id, true
}

// handleImportPage 是第一步（GET /import）：一张选文件的表单。
//
// 零 JavaScript、multipart/form-data。**没有"记住我导过"这种状态**：
// 档案空就提示、有书签就不提示，判据只有"这个档案现在有几条书签"这一份。
func (s *Server) handleImportPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.importGuard(w, r); !ok {
		return
	}
	s.renderImport(w, r, render.ImportView{
		Mode: "pick", Heading: "从浏览器导入收藏夹",
		Explain: "浏览器导出的书签是一个 HTML 文件。选它，下一步会先给你看一份报告：" +
			"会加多少条、图标有几处猜不中、以及哪些被浏览器标成收藏夹栏/其它文件夹。" +
			"看清楚再决定写不写。",
	})
}

// handleImportUpload 是第二步（POST /import）：收文件、解析、给报告，不写盘。
//
// **解析成功不写盘**：写入发生在 POST /import/apply，中间必须隔一个能让人
// 读报告的动作。这是 CLI 那条"不许跑 --apply 除非人明确要求"纪律在网页上
// 的形状。
func (s *Server) handleImportUpload(w http.ResponseWriter, r *http.Request) {
	profile, ok := s.importGuard(w, r)
	if !ok {
		return
	}
	// **上限必须挂在 body 上，不能只靠下面的 LimitReader**：
	// ParseMultipartForm 的参数是**内存缓冲上限**，不是体积上限——超过它的部分
	// 会先被整份解析并落成临时文件，等我们 LimitReader 到手时磁盘已经写过了。
	// 一台家用机器被一个 2 GB 的 POST 拖爆，正是这个洞。
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: "从浏览器导入收藏夹",
			Notice:  "读不到这个文件（最大 8 MB）。请确认选的是浏览器导出的那个 .html。",
			Explain: importExplain,
		})
		return
	}
	file, hdr, err := r.FormFile("bookmarks")
	if err != nil {
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: "从浏览器导入收藏夹",
			Notice:  "没有选文件。",
			Explain: importExplain,
		})
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil || len(raw) > maxImportBytes {
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: "从浏览器导入收藏夹",
			Notice:  "这个文件读不动，或超过了 8 MB。",
			Explain: importExplain,
		})
		return
	}
	plan, err := s.planImport(profile, raw)
	if err != nil {
		notice := "解析失败：" + err.Error()
		switch {
		case errors.Is(err, errNothingToImport):
			// **Convert 不拒绝非 Netscape 文件**：喂一段普通 HTML，它解析出零条书签
			// 并照常返回。不判这一条，用户会看到"导入成功 0 条"，像真的一样。
			notice = "这个文件里没读出书签。请确认它是浏览器「导出书签」出来的那份 .html，" +
				"而不是某个网页另存的 HTML。"
		case errors.Is(err, errAllSkipped):
			notice = "这份文件里的书签读是读出来了，但没有一条能进来——它们的地址都是" +
				"「javascript:」「data:」「file:」这类没有主机名的地址，浏览器从网页里也点不开。" +
				"这种书签请继续在浏览器里用。"
		case errors.Is(err, errNoStash):
			notice = "临时存不下这次上传，稍后再试。"
		}
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: "从浏览器导入收藏夹",
			Notice:  notice,
			Explain: importExplain,
		})
		return
	}
	s.renderImport(w, r, render.ImportView{
		Mode: "report", Heading: "确认导入",
		Count: plan.Count, Skipped: plan.Skipped, Groups: plan.Groups,
		Report: plan.Report, Token: plan.Token,
		FileName: hdr.Filename,
	})
}

// handleImportApply 是第三步：确认写入。ops 由 bookimport.Ops 生成，落盘走
// applyBatch——**与 POST /api/v1/batch 同一个落点**，快照/历史/rev 一次不落。
func (s *Server) handleImportApply(w http.ResponseWriter, r *http.Request) {
	profile, ok := s.importGuard(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/import", http.StatusSeeOther)
		return
	}
	// 令牌取走即删（applyImport 里做）。过期 / 已被用过 / 不是这个档案的：一律回
	// 第一步重新选，不说哪一种——令牌是一次性的，说"过期了"等于教人反复点确认。
	out, err := s.applyImport(r.FormValue("token"), profile, profile, "import")
	if err != nil {
		http.Redirect(w, r, "/import", http.StatusSeeOther)
		return
	}
	if len(out.verrs) > 0 || out.snapErr != nil || out.missing || out.broken != "" {
		// 写入没成：把结论说清楚，别 303 回一个"什么都没变"的首页。
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: "从浏览器导入收藏夹",
			Notice:  "没写进去：" + importOutcomeText(out),
			Explain: importExplain,
		})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// importOutcomeText 把 applyBatch 的失败结论翻成一句人话。
func importOutcomeText(out batchOutcome) string {
	switch {
	case out.missing:
		return "档案不存在。"
	case out.broken != "":
		return "档案的数据文件读不了。" + out.broken
	case out.conflict != nil:
		return "档案已经被改动过，请重试。"
	case len(out.verrs) > 0:
		return fmt.Sprintf("有 %d 条过不了校验，整批没有写入。", len(out.verrs))
	case out.snapErr != nil:
		return "快照写不下去，本次写入已取消。"
	}
	return "原因不明。"
}

// decodeImportOps 把 bookimport 产出的 map 形状 ops 解成 []ops.Op，
// 走的是与 batch 端点 decodeBody **同一份解码器设置**（DisallowUnknownFields）。
// 于是那条闸也管着导入这条路：bookimport 少写一个字段，这里当场报出来，
// 而不是悄悄把一个空 icon 写进别人的台。
func decodeImportOps(raw []map[string]any) ([]ops.Op, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var out []ops.Op
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// importPlanResponse 是 POST /api/v1/import 的成功体：一份"会写什么"。
//
// **不写盘**——写发生在 /api/v1/import/apply，两个端点之间隔着一次调用方能
// 读报告的动作。这与网页上那两步是同一条纪律的两种壳。
type importPlanResponse struct {
	Profile string `json:"profile"`
	Count   int    `json:"count"`
	// Skipped 是没能进来的条数（过不了写路径的校验：地址没有主机名之类）。
	// 报告里有逐条明细，这个数是给调用方一句话显示用的。
	Skipped int    `json:"skipped"`
	Groups  int    `json:"groups"`
	Report  string `json:"report"`
	// ReportHTML is the same text rendered for a human reader. The extension
	// pastes report into the page; without this it shows markdown markers.
	ReportHTML string `json:"reportHtml"`
	Token      string `json:"token"`
}

type importApplyResponse struct {
	Profile  string   `json:"profile"`
	Rev      int64    `json:"rev"`
	Applied  int      `json:"applied"`
	Warnings []string `json:"warnings"`
}

// handleImportAPI 是**令牌通路**的第一步，给浏览器扩展用：扩展读得到浏览器
// 的整棵书签树，却读不到会话 cookie，于是它只能走令牌。
//
// 目标档案**从令牌作用域推导，不接受申报**：一枚 `name:secret:dad` 的令牌只能
// 导进 dad。这不是"懒得校验"，是比校验更强的形状——申报字段根本不存在，
// 于是"令牌与档案对不上"这种错在协议层就无法表达。
//
// 体是**原始 Netscape HTML**（`Content-Type: text/html`），不是 multipart：
// 扩展在浏览器里就把树拼成了这个格式，包一层 multipart 只是多一次编码。
// 服务端拿它喂的仍然是同一个 bookimport.Convert——扩展与网页在服务端看来
// 是同一件事。
func (s *Server) handleImportAPI(w http.ResponseWriter, r *http.Request) {
	sc, ok := s.deps.Tokens.Authorize(bearer(r))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Detail: "令牌认不出来"})
		return
	}
	if sc.IsAdmin() {
		// 与 2026-10-10 的管理员边界同一条：`*` 只管账号，写书签要用档案作用域令牌。
		writeJSON(w, http.StatusForbidden, errorResponse{
			Code: 403, Detail: "管理员令牌（*）只管账号；写这一份的书签要用作用域正好是它的令牌"})
		return
	}
	if _, _, ok := s.require(r, credScoped, sc.Profile); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxImportBytes+1))
	if err != nil || len(raw) > maxImportBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{
			Code: 413, Detail: "读不到请求体，或超过了 8 MB 上限"})
		return
	}
	plan, err := s.planImport(sc.Profile, raw)
	if err != nil {
		importAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, importPlanResponse{
		Profile: sc.Profile, Count: plan.Count, Skipped: plan.Skipped, Groups: plan.Groups,
		Report: plan.Report, ReportHTML: string(render.ReportHTML(plan.Report)), Token: plan.Token})
}

// handleImportApplyAPI 是令牌通路的第二步。
//
// **档案从令牌暂存里取，不从 body 取**（body 只有 token）：令牌在第一步就绑死
// 了一个档案，于是"令牌与申报对不上"这个错连表达都表达不出来。
func (s *Server) handleImportApplyAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	sc, ok := s.deps.Tokens.Authorize(bearer(r))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Detail: "令牌认不出来"})
		return
	}
	actor, _, ok := s.require(r, credScoped, sc.Profile)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	out, err := s.applyImport(in.Token, sc.Profile, actor, "extension")
	if err != nil {
		importAPIError(w, err)
		return
	}
	switch {
	case out.missing:
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Detail: "档案不存在"})
	case out.broken != "":
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Code: 503, Detail: out.broken})
	case out.conflict != nil:
		writeJSON(w, http.StatusConflict, errorResponse{
			Code: 409, Detail: "rev 已变，请重读后重试", CurrentRev: out.conflict})
	case len(out.verrs) > 0:
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Code: 422, Detail: "ops 校验未过，整批未写入", Errors: opFieldErrors(out.verrs)})
	case out.snapErr != nil:
		writeJSON(w, http.StatusInsufficientStorage, errorResponse{
			Code: 507, Detail: "快照写不下去，本次写入已取消"})
	default:
		writeJSON(w, http.StatusOK, importApplyResponse{
			Profile: sc.Profile, Rev: out.resp.Rev,
			Applied: out.resp.Applied, Warnings: out.resp.Warnings})
	}
}

// importAPIError 把共用核心的 sentinel 分诊成 API 形状。
//
// 状态码选得跟同一个文件夹里 batch 的判据对齐：读出 0 条是 **422**（这一次
// 请求的内容不对），令牌过期是 **410 Gone**（资源确实没了，而且正是"别再重试
// 同一枚"的信号——报成 400 会诱使调用方把它当参数错误再发一次）。
func importAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNothingToImport):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Code: 422, Detail: "这个文件里没读出书签——请确认它是浏览器「导出书签」出来的那份 .html"})
	case errors.Is(err, errAllSkipped):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Code: 422, Detail: "读出了书签，但没有一条能进来——它们的地址都没有主机名" +
				"（javascript:、data:、file: 这类），这种书签请继续在浏览器里用"})
	case errors.Is(err, errStashGone):
		writeJSON(w, http.StatusGone, errorResponse{
			Code: 410, Detail: "这次的上传已经过期或被用过了，请重新发起"})
	case errors.Is(err, errNoStash):
		writeJSON(w, http.StatusInsufficientStorage, errorResponse{
			Code: 507, Detail: "临时存不下这次上传，稍后再试"})
	default:
		writeInternalError(w, err)
	}
}

// renderImport 是导入页唯一的渲染出口。Profile 在这里填一次而不是在三处
// 各写一遍——它决定响应的 X-LB-Profile（SW 据此分桶）与 no-store 的理由，
// 两份写起来就会有一份开始撒谎。
// importPlan 是一次导入"会写什么"的结论。浏览器表单页与 API 两个壳共用它。
type importPlan struct {
	Count int
	// Skipped 是**没能进来**的条数。页面与 API 都要单独说它：只说"会加 N 条"
	// 会让人以为文件里就这么多书签，而少的那些去了哪里只有报告知道。
	Skipped int
	Groups  int
	Report  string
	Token   string
}

// planImport 解析 + 建暂存。**它不写盘**——写入在 applyImport，写盘走
// applyBatch（与 POST /api/v1/batch 同一个落点）。
//
// 两个壳调它：网页上的 multipart 上传（handleImportUpload）与浏览器扩展的
// 直读导入（handleImportAPI）。判据只有这一份，所以"扩展导进来的"与"手动
// 传文件导进来的"在服务端看来是同一件事。
func (s *Server) planImport(profile string, raw []byte) (importPlan, error) {
	res, err := bookimport.Convert(raw, s.deps.Icons, bookimport.Options{})
	if err != nil {
		return importPlan{}, err
	}
	n := bookimport.CountItems(res.Bookmarks)
	if n == 0 {
		// **Convert 不拒绝非 Netscape 文件**：喂一段随便的 HTML 进去，它解析出
		// 零条书签并照常返回。所以"读出 0 条"必须在这里说清楚——不然用户
		// 点完"导入"只看到"已导入 0 条"，像成功了一样。
		//
		// 而"读出了书签、但一条都没进来"是**另一个形状**（整份文件都是
		// javascript: 这类没有主机名的地址）：说成"没读出书签"会让人以为文件
		// 选错了，于是去换一个同样导不进来的文件。
		if len(res.Report.Skipped) > 0 {
			return importPlan{}, errAllSkipped
		}
		return importPlan{}, errNothingToImport
	}
	token := s.stashImport(profile, raw)
	if token == "" {
		return importPlan{}, errNoStash
	}
	return importPlan{
		Count:   n,
		Skipped: len(res.Report.Skipped),
		Groups:  len(res.Bookmarks.Groups),
		Report:  res.Report.Text(),
		Token:   token,
	}, nil
}

// applyImport 把暂存里那份解析一遍并写入。**令牌取走即删**：同一枚再发一次
// 写不进第二次——这比"确认后用 JS 锁按钮"可靠，且关掉 JS 时照样成立。
func (s *Server) applyImport(token, profile, actor, by string) (batchOutcome, error) {
	raw, ok := s.takeImport(token, profile)
	if !ok {
		return batchOutcome{}, errStashGone
	}
	res, err := bookimport.Convert(raw, s.deps.Icons, bookimport.Options{})
	if err != nil {
		return batchOutcome{}, err
	}
	opList, err := decodeImportOps(bookimport.Ops(res.Bookmarks))
	if err != nil {
		return batchOutcome{}, err
	}
	return s.applyBatch(profile, actor, by, nil, opList)
}

// 这四个 sentinel 是**给调用方分诊**用的，不是给用户看的：每个壳按自己的
// 形状把它翻成页面提示或 JSON detail。
var (
	errNothingToImport = errors.New("这个文件里没读出书签")
	errAllSkipped      = errors.New("读出了书签，但没有一条能进来")
	errNoStash         = errors.New("临时存不下这次上传")
	errStashGone       = errors.New("这次的上传已经过期或被用过")
)

func (s *Server) renderImport(w http.ResponseWriter, r *http.Request, v render.ImportView) {
	v.Profile = auth.ReadSession(r).Active
	page, err := s.deps.Render.ImportPage(v)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

const importExplain = "浏览器导出的书签是一个 HTML 文件：Chrome/Edge 是「书签管理器 → 右上角三点 → 导出书签」，" +
	"Firefox 是「书签 → 管理书签 → 导入与备份 → 导出为 HTML」。导入会加进「这个档案」，不碰别人的。"
