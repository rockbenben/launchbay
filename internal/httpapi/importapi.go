package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/rockbenben/launchbay/internal/apierr"
	"io"
	"net/http"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/bookimport"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
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
		tr := s.trFor(r, id)
		s.renderImport(w, r, render.ImportView{
			Mode: "blocked", Heading: tr.t("import.blockedHeading"),
			Notice:  tr.t("import.notice.blocked", n),
			Profile: id,
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
	id, ok := s.importGuard(w, r)
	if !ok {
		return
	}
	tr := s.trFor(r, id)
	s.renderImport(w, r, render.ImportView{
		Mode: "pick", Heading: tr.t("import.heading"),
		Explain: tr.t("import.explain"),
		Profile: id,
	})
}

// handleImportUpload 是第二步（POST /import）：收文件、解析，然后**按 `go` 分两路**。
//
// `go=preview`（默认）：给报告，等人点头——**解析成功不写盘**，写入发生在
// POST /import/apply。这是 CLI 那条"不许跑 --apply 除非人明确要求"纪律在网页上
// 的形状。
//
// `go=direct`：收下就写（见 directImport）。同一个 multipart 表单里两个 submit
// 按钮各带一个 value，所以这一分叉不需要任何脚本。
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
	tr := s.trFor(r, profile)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: tr.t("import.heading"),
			Notice:  tr.t("import.notice.unreadable"),
			Explain: tr.t("import.explainStep"),
			Profile: profile,
		})
		return
	}
	file, hdr, err := r.FormFile("bookmarks")
	if err != nil {
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: tr.t("import.heading"),
			Notice:  tr.t("import.notice.nofile"),
			Explain: tr.t("import.explainStep"),
			Profile: profile,
		})
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil || len(raw) > maxImportBytes {
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: tr.t("import.heading"),
			Notice:  tr.t("import.notice.toolarge"),
			Explain: tr.t("import.explainStep"),
			Profile: profile,
		})
		return
	}
	// 浏览器那一屏：报告里的句子跟着站点语言（人读的）。
	plan, err := s.planImport(profile, raw, tr.t)
	if err != nil {
		notice := tr.t("import.notice.parseFail", err.Error())
		switch {
		case errors.Is(err, errNothingToImport):
			// **Convert 不拒绝非 Netscape 文件**：喂一段普通 HTML，它解析出零条书签
			// 并照常返回。不判这一条，用户会看到"导入成功 0 条"，像真的一样。
			notice = tr.t("import.notice.notABookmarkFile")
		case errors.Is(err, errAllSkipped):
			notice = tr.t("import.notice.allSkipped")
		case errors.Is(err, errNoStash):
			notice = tr.t("import.notice.noStash")
		}
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: tr.t("import.heading"),
			Notice:  notice,
			Explain: tr.t("import.explainStep"),
			Profile: profile,
		})
		return
	}
	if r.FormValue("go") == "direct" {
		s.directImport(w, r, profile, hdr.Filename, plan)
		return
	}
	s.renderImport(w, r, render.ImportView{
		Mode: "report", Heading: tr.t("import.reportHeading"),
		Count: plan.Count, Skipped: plan.Skipped, Groups: plan.Groups,
		Report: plan.Report, Token: plan.Token,
		FileName: hdr.Filename,
		Profile:  profile,
	})
}

// directImport 是 `go=direct` 那一支：收下文件就写，不先给报告（2026-10-12）。
//
// **它省掉的只是"人读一遍再点头"这个动作，没有省掉任何一道闸**：解析走
// planImport、写入走 applyImport、落盘走 applyBatch（快照/历史/rev 一次不落），
// 令牌照样一次性。所以它不是一个新的写路径，而是"少一个页面"——
// 与 /admin/pin、/revert 那两步页面相比，被拿掉的是第一步，不是那两步。
//
// 为什么要有这一支：导入这件事在界面上被说成"看这份会导入什么"，于是想快点
// 导完的人必须先读一屏报告才知道导入根本没坏。而报告答的是"会导入什么"，
// 写完之后人真正要的是"成了没有、少了哪几条"——那正是这一屏印的东西。
func (s *Server) directImport(w http.ResponseWriter, r *http.Request, profile, fileName string, plan importPlan) {
	tr := s.trFor(r, profile)
	out, err := s.applyImport(plan.Token, profile, profile, "import")
	if err != nil {
		// 令牌取不到（过期/被用过）或解析这一趟出了意外：回第一步重来，
		// 形状与 /import/apply 那条完全一致（importOutcomeText 说得出的话它说得出）。
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: tr.t("import.heading"),
			Notice:  tr.t("import.notice.notSaved", s.importPlainOutcome(tr, err)),
			Explain: tr.t("import.explainStep"),
			Profile: profile,
		})
		return
	}
	switch {
	case out.missing, out.broken != "", len(out.verrs) > 0, out.snapErr != nil:
		// 写入没成：把结论说清楚，绝不跳回一个"什么都没变"的首页。
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: tr.t("import.heading"),
			Notice:  tr.t("import.notice.notSaved", s.importOutcomeText(tr, out, plan.Bookmarks)),
			Explain: tr.t("import.explainStep"),
			Profile: profile,
		})
	default:
		s.renderImport(w, r, render.ImportView{
			Mode: "done", Heading: tr.t("import.done.heading"),
			// **条数给的是书签条数**（plan.Count），不是 op 条数（out.resp.Applied）：
			// 建一个分组也是一条 op，说成「写进了 N 条书签」会把分组的数目算成
			// 书签的数目（openapi.json 与 AGENTS.md 都写明了这一点）。
			Count: plan.Count, SkippedItems: skipEntries(plan.SkippedItems),
			FileName: fileName,
			Profile:  profile,
		})
	}
}

// importPlainOutcome 把 applyImport 的那个 error（令牌过期之类）翻成一句话。
// **只这一支用**：applyBatch 的结论走 importOutcomeText（有 sentinel 可分诊），
// 而 error 这一边没有——令牌是一次性的，说"过期了"等于教人反复点。
//
// 别的 error 一律归到"原因不明"那一档，不把内部错误原文印到页面上：那是给
// stderr 与容器日志看的，而这一屏是给人看的。
func (s *Server) importPlainOutcome(tr tr, err error) string {
	if errors.Is(err, errStashGone) {
		return tr.t("import.outcome.gone")
	}
	return tr.t("import.outcome.unknown")
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
	tr := s.trFor(r, profile)
	if len(out.verrs) > 0 || out.snapErr != nil || out.missing || out.broken != "" {
		// 写入没成：把结论说清楚，别 303 回一个"什么都没变"的首页。
		// 第二格传 nil：这条路手上只有令牌，解析结果在暂存里已经被用掉了。
		// 拿不到 Bookmarks 就退回"有 N 条过不了校验"那句话（宁可少一句点名，
		// 也不要指错一条）；走「直接导入」那条路拿得到，点名照常。
		s.renderImport(w, r, render.ImportView{
			Mode: "pick", Heading: tr.t("import.heading"),
			Notice:  tr.t("import.notice.notSaved", s.importOutcomeText(tr, out, nil)),
			Explain: tr.t("import.explainStep"),
			Profile: profile,
		})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// importOutcomeText 把 applyBatch 的失败结论翻成一句人话。
// 它是 **Server 的**方法而不是包级函数：它拼的是给人看的结论，而语言是机器级的
// 设置，只有 Server 读得到 store。理由与 revert 的 choiceLabel 同一处。
func (s *Server) importOutcomeText(tr tr, out batchOutcome, b *model.Bookmarks) string {
	switch {
	case out.missing:
		return tr.t("import.outcome.missing")
	case out.broken != "":
		return tr.t("import.outcome.broken", out.broken)
	case out.conflict != nil:
		return tr.t("import.outcome.conflict")
	case len(out.verrs) > 0:
		// **点名是哪一条**（2026-10-12）。只给"N 条过不了校验"的话，用户手上
		// 只有一份他改不动的 html 导出文件，而这个数字告诉他"文件有问题"却没
		// 告诉他"哪一条"——于是下一次导入还是同样一句话。
		//
		// 判据是 ops.Run 报的 OpsIndex，翻译在 bookimport.EntryOfOpIndex（与 Ops
		// 同一处，不会与 op 的顺序分家）。b 为 nil（第三步那条路拿得到令牌却拿
		// 不到解析结果）或者翻不出来时，退回原来那句话——宁可少一句，也不要
		// 指错一条。
		if b != nil && len(out.verrs) > 0 {
			name, url, ok := bookimport.EntryOfOpIndex(b, out.verrs[0].OpsIndex)
			switch {
			case ok && url != "":
				return tr.t("import.outcome.invalidNamed", name, url, out.verrs[0].Reason)
			case name != "":
				return tr.t("import.outcome.invalidGroup", name, out.verrs[0].Reason)
			}
		}
		return tr.t("import.outcome.invalid", len(out.verrs))
	case out.snapErr != nil:
		return tr.t("import.outcome.snapErr")
	}
	return tr.t("import.outcome.unknown")
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
	// Skipped 是没能进来的条数（过不了写路径的校验：`model.CheckURL` 拒空、拒解析不了、
	// 拒缺主机名——**`file:` 不在其列**，本地路径照收）。
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
//
// **它认令牌不认会话**（`credTokenOnly`，2026-10-19）：会话在这条通路上没有
// 意义——扩展读不到会话 cookie，而浏览器自己那条 `/import` 走的是零 JS 表单。
// 写进契约前，这一档与 `credScoped` 共用，于是文档报成"令牌或会话都行"，
// 照文档写脚本的扩展会拿会话 cookie 换来一句「令牌认不出来」。
// adminTokenGuard 答"这把 bearer 是不是管理员那把 key"，并且答掉。
//
// 它是**返回 true 就已经答出去了**（403），所以调用方的形状是
// `if s.adminTokenGuard(w, r) { return }`。
//
// 管理员那把 key **不在档案作用域令牌列表里**（它是机器级的一把裸 key），
// 所以只查 Tokens 的话，一把**完全有效**的管理员 key 会拿到 401
// 「令牌认不出来」——那是在说一把好钥匙是废的（batch.go 2026-10-12 那段
// 警告说的正是这个形状）。
//
// **kind 用 `credential-admin-token` 而不是 `credential-scope`**（2026-10-19）：
// 那两句出路是相反的——`credential-scope` 说"你管的是别的档案，换个 id"，
// 而这里的事实是"你拿的是一把根本不开书签这一面的钥匙"。从前扩展把管理员
// key 的 403 翻成"它管的是别的档案，或者那个 id 打错了"，用户照着改档案 id
// 改到天亮也进不去。
//
// **两处共用这一份**（2026-10-20）：导入那两步（handleImportAPI 与
// handleImportApplyAPI）从前各写一遍，于是第一步答 403、第二步答 401——
// 出路正好相反，调用方会去换令牌，而它没有令牌问题。
func (s *Server) adminTokenGuard(w http.ResponseWriter, r *http.Request) bool {
	if !s.deps.AdminToken.Matches(bearer(r)) {
		return false
	}
	writeJSON(w, http.StatusForbidden, errorResponse{
		Code: 403, Kind: apierr.CredentialAdminToken,
		Detail: "管理员令牌只管账号；导进某个档案要用作用域正好是它的令牌",
	})
	return true
}

func (s *Server) handleImportAPI(w http.ResponseWriter, r *http.Request) {
	// 管理员令牌（LAUNCHBAY_ADMIN_TOKEN）根本不在档案作用域令牌列表里，所以
	// 它落到下面那次 Authorize 上只会得到"认不出来"的 401——那是错的答案：
	// 它不是坏了，是**层级不对**。所以这里单独拦一句，给出 403 与出路。
	//
	// **kind 用 `credential-admin-token` 而不是 `credential-scope`**（2026-10-19）：
	// 那两句出路是相反的——`credential-scope` 说"你管的是别的档案，换个 id"，
	// 而这里的事实是"你拿的是一把根本不开书签这一面的钥匙"。从前扩展把管理员
	// key 的 403 翻成"它管的是别的档案，或者那个 id 打错了"，用户照着改档案 id
	// 改到天亮也进不去。
	if s.adminTokenGuard(w, r) {
		return
	}
	sc, ok := s.deps.Tokens.Authorize(bearer(r))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Kind: apierr.CredentialUnknown, Detail: "令牌认不出来"})
		return
	}
	if _, _, ok := s.require(r, credTokenOnly, sc.Profile); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	if s.profileDisabledRefuse(w, sc.Profile) {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxImportBytes+1))
	if err != nil || len(raw) > maxImportBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Code: 413, Kind: apierr.BodyUnreadable, Detail: "读不到请求体，或超过了 8 MB 上限"})
		return
	}
	// 接口这一条有两个读者：脚本（CLI、curl）要的是**逐字不变**的那一份，
	// 浏览器扩展要的是**贴给人看**的那一份——它把报告原样画在设置页下面。
	// 判据是 Accept-Language（见 lang.go 的 browserLang）：不发这个头的照旧中文。
	plan, err := s.planImport(sc.Profile, raw, s.apiReportWords(r))
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
	// **凭证在前，体在后**（2026-10-19 修）：原先这里先 `decodeBody` 再
	// `Authorize`，于是凭证根本认不出来的那一次，请求体已经被整份读进内存、
	// JSON 也已经被解了一遍才给出 401。两个后果：未鉴权的请求能拿 CPU 与内存
	// 去消耗这台机器（§11 明写不防局域网内的东西，而这一条正是最省事的一档），
	// 以及**报出去的答案可能是错的**——体不合法时先撞上 422「body-unreadable」，
	// 凭证不对这件事根本没机会说，调用方拿到的 reason 是"你的 JSON 坏了"。
	// 项目自己的纪律就是反的：401 与 403 分得很清，正是为了让"换凭证"与
	// "改请求体"这两件事不要互相冒充。
	//
	// **这里不同于 batch / handleCreateProfile / rebuild 那三处，是有理由的，
	// 别把那三处也照这个改**（2026-10-19 记下，否则下一个读代码的人会以为这是
	// 第四处口径不一）：那三处之所以"先解码再申报"，是因为**档案 id 就在
	// body 里**（`in.Profile` / `in.ID`），`require` 要拿它判作用域——没有体就
	// 无从知道这个请求想动谁，鉴权只能是空的。本条的 body 只有 `{token}`，
	// 档案**从令牌作用域推导**（见函数头那段），`Authorize` 不需要体就能答，
	// 所以这里先问凭证是有底的。**判据是"作用域从哪来"，不是"哪个顺序好看"。**
	// **管理员那把 key 也要单独拦一句**（2026-10-20 修）：它不在档案作用域令牌
	// 列表里，于是只查 Tokens 的话，一把**完全有效**的管理员 key 会拿到 401
	// 「令牌认不出来」——那是在说一把好钥匙是废的（batch.go 2026-10-12 那段
	// 警告说的正是这个形状）。同一个两步流程里，第一步（handleImportAPI）答的是
	// 403「这把钥匙只管账号」，第二步却答 401「你的令牌我不认识」——出路正好
	// 相反，调用方会去换令牌，而它没有令牌问题。
	if s.adminTokenGuard(w, r) {
		return
	}
	sc, ok := s.deps.Tokens.Authorize(bearer(r))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Code: 401, Kind: apierr.CredentialUnknown, Detail: "令牌认不出来"})
		return
	}
	actor, _, ok := s.require(r, credTokenOnly, sc.Profile)
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	if s.profileDisabledRefuse(w, sc.Profile) {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	out, err := s.applyImport(in.Token, sc.Profile, actor, "extension")
	if err != nil {
		importAPIError(w, err)
		return
	}
	switch {
	case out.missing:
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.ProfileNotFound, Detail: "档案不存在"})
	case out.broken != "":
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Code: 503, Kind: apierr.ProfileBroken, Detail: out.broken})
	case out.conflict != nil:
		writeJSON(w, http.StatusConflict, errorResponse{Code: 409, Kind: apierr.RevConflict, Detail: "rev 已变，请重读后重试", CurrentRev: out.conflict})
	case len(out.verrs) > 0:
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Code: 422, Kind: apierr.OpsInvalid, Detail: "ops 校验未过，整批未写入", Errors: opFieldErrors(out.verrs)})
	case out.snapErr != nil:
		writeJSON(w, http.StatusInsufficientStorage, errorResponse{Code: 507, Kind: apierr.SnapshotFailed, Detail: "快照写不下去，本次写入已取消"})
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
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Code: 422, Kind: apierr.ImportNoBookmarks, Detail: "这个文件里没读出书签——请确认它是浏览器「导出书签」出来的那份 .html"})
	case errors.Is(err, errAllSkipped):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Code: 422, Kind: apierr.ImportNothingAccepted, Detail: "读出了书签，但没有一条能进来——它们的地址都是 javascript:、data: " +
			"这类没有主机名、点了会执行东西的地址，这种书签请继续在浏览器里用；" +
			"本地文件 file:// 是能导的"})
	case errors.Is(err, errStashGone):
		writeJSON(w, http.StatusGone, errorResponse{Code: 410, Kind: apierr.ImportPlanStale, Detail: "这次的上传已经过期或被用过了，请重新发起"})
	case errors.Is(err, errNoStash):
		writeJSON(w, http.StatusInsufficientStorage, errorResponse{Code: 507, Kind: apierr.InternalError, Detail: "临时存不下这次上传，稍后再试"})
	case errors.Is(err, errTargetNotEmpty):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Code: 422, Kind: apierr.ImportTargetNotEmpty,
			Detail: "这一份里已经有书签了。导入是「把浏览器那套导进来」，不是往现有档案上叠加——换一个空档案，或先自己在界面上清空它。"})
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
	// SkippedItems 是同一批东西的**逐条明细**（2026-10-12）。直接导入那一屏
	// 印它：报告那一路有全文可看，直接这条路只有这一份清单，所以少的是哪几条
	// 只能从这里知道。
	SkippedItems []bookimport.SkippedItem
	Groups       int
	Report       string
	Token        string
	// Bookmarks 是这次解析出来的文档，**只为把"第 N 条 op 失败"翻译成那条书签
	// 的名字**（importOutcomeText）。它不进任何响应体，也不进暂存——暂存里存
	// 的是原始文件，重新解析一次就好。
	Bookmarks *model.Bookmarks
}

// skipEntries 把 bookimport 的跳过清单搬成渲染层的形状。搬一次而不是让渲染层
// 认识解析器——渲染层该知道的只有"这三样要印出来"。
func skipEntries(in []bookimport.SkippedItem) []render.SkippedEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]render.SkippedEntry, 0, len(in))
	for _, s := range in {
		out = append(out, render.SkippedEntry{Name: s.Name, URL: s.URL, Reason: s.Reason})
	}
	return out
}

// planImport 解析 + 建暂存。**它不写盘**——写入在 applyImport，写盘走
// applyBatch（与 POST /api/v1/batch 同一个落点）。
//
// 两个壳调它：网页上的 multipart 上传（handleImportUpload）与浏览器扩展的
// 直读导入（handleImportAPI）。判据只有这一份，所以"扩展导进来的"与"手动
// 传文件导进来的"在服务端看来是同一件事。
// planImport 解析一份浏览器导出并算出报告。
//
// `words` 决定报告里那些**句子**用哪种语言：nil = 中文那一份，逐字不变（AGENTS.md
// 「翻的与不翻的」里给脚本对照用的那个锚点）；给一个查表函数就用那一种语言。
// 两路都走同一份 bookimport.Report：翻的是句子，不是重新生成一份结论。
func (s *Server) planImport(profile string, raw []byte, words func(string, ...any) string) (importPlan, error) {
	res, err := bookimport.Convert(raw, s.deps.Icons, bookimport.Options{})
	if err != nil {
		return importPlan{}, err
	}
	res.Report.Words = words
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
		Count:        n,
		Skipped:      len(res.Report.Skipped),
		SkippedItems: res.Report.Skipped,
		Groups:       len(res.Bookmarks.Groups),
		Report:       res.Report.Text(),
		Token:        token,
		Bookmarks:    res.Bookmarks,
	}, nil
}

// applyImport 把暂存里那份解析一遍并写入。**令牌取走即删**：同一枚再发一次
// 写不进第二次——这比"确认后用 JS 锁按钮"可靠，且关掉 JS 时照样成立。
func (s *Server) applyImport(token, profile, actor, by string) (batchOutcome, error) {
	raw, ok := s.takeImport(token, profile)
	if !ok {
		return batchOutcome{}, errStashGone
	}
	// **目标必须一条书签都没有**（2026-10-19 补上，判据收在这一处）。
	//
	// 浏览器壳（importGuard）与 CLI（`import-bookmarks`）都有这道闸，令牌壳原先
	// 没有：扩展持一枚 dad 的令牌，对**非空**档案跑一遍 import + apply 就能把
	// 浏览器书签**并进**既有档案（ops 全是 add），200、rev+1、快照照做。三个壳
	// 两种判据，而扩展那条最可能是漏掉——理由与另两处一样："导入是**把浏览器
	// 那套导进来**，不是往现有档案上叠加"。
	//
	// 放在 applyImport 而不是 planImport：这是唯一真正写盘的落点，三道壳都
	// 经过它，两处都判会漏掉"申请令牌时是空的、写入时已经有人写了"那一步。
	// 浏览器壳与 CLI 仍各自保留自己那道——它们要在**申请之前**就告诉人，
	// 而不是让人读完一份报告才被拒。
	//
	// **这道闸不能裸返 err**（2026-10-19 修）：它排在 `applyBatch` 之前，
	// 而"档案不存在"与"书签文件坏"这两种形状是由 `applyBatch` 认出来的
	// （它把两者翻成 `missing` / `broken` 两个字段，再由 handler 答 404/503）。
	// 裸返 err 会跳过整套分诊直接落 `default: writeInternalError` → **500**。
	// 症状是令牌指向一个已删档案（DELETE /api/v1/profiles 之后令牌列表还留着）
	// 时，从 404 退化成 500，而 500 会诱使调用方重试——重试多少次都是 500。
	// 判据只用 `applyBatch` 认过的那两个哨兵，别在这里重新发明一遍。
	snap, err := s.deps.Store.Current(profile)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return batchOutcome{missing: true}, nil
	case errors.Is(err, store.ErrCorrupt):
		return batchOutcome{broken: corruptDetail(s.deps.Store, profile, err)}, nil
	case err != nil:
		return batchOutcome{}, err
	}
	if n := bookimport.CountItems(snap.B); n > 0 {
		return batchOutcome{}, errTargetNotEmpty
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
	// errTargetNotEmpty 是"这一份已经有书签了"那个形状：导入是替换而不是叠加。
	errTargetNotEmpty = errors.New("这一份里已经有书签了")
)

func (s *Server) renderImport(w http.ResponseWriter, r *http.Request, v render.ImportView) {
	v.Profile = auth.ReadSession(r).Active
	// 这一页用什么语言：这一份自己记了语言就用它，没记就用默认、再用浏览器说的
	// 那一门。**在渲染之前定**（而不是每个调用点各写一遍）——这一族有十来个
	// 调用点，判据只有一处。
	lang := s.langOf(r, v.Profile)
	page, err := s.deps.Render.ImportPage(v, lang)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}
