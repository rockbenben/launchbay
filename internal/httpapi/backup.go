package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/apierr"
	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/bookimport"
	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

// 这一族是**整份的进出**：书签 + 这一页的台面偏好（主题、排法、整页形态、
// 第二行、页面名、界面语言）打包成一份文件，导出与导入是同一份格式。
//
// **它与 GET /api/v1/export 是两件事，不许合并**：那一条导的是磁盘上
// bookmarks.json 的**原字节**，契约是「可以直接喂 batch 对照」——AI 脚本靠它
// 拿一份与盘上逐字相同的东西去比。改它的形状就是毁掉那条契约。所以另起
// 一对端点，导出的是**这一份的完整状态**，不是书签文档的原字节。
//
// **备份里没有什么**（这几条是判据，不是遗漏）：
//   - **PIN**：门闩不进备份。导出 PIN 等于把钥匙印在纸上。
//   - **档案 id / 标题 / createdAt / 停用状态**：那是这台机器上的**身份**，
//     不是这一份的内容。导到另一台机器上时，档案由那边自己建。
//   - **背景图**：背景是一个**文件**（`data/backgrounds/<id>/…`），不是一串
//     字符。导出一个名字而没有那个文件，恢复完就是"这张背景不见了"。
//     它跟着这台机器走（model.Profile.Background 的注释同一句）。
const (
	backupKind    = "launchbay-backup"
	backupVersion = 1
)

// backupPackage 是**导出的那份文件**，也正是**导入时吃的那一份**。
//
// 导出什么就能导入什么：两种形状写成同一个结构体，人才不用对着两种格式
// 翻译（而人翻译出来的形状，服务端只能猜）。
type backupPackage struct {
	// Kind 与 Version 是**要核的**：一份别的文件（浏览器导出的书签、随手改过
	// 的 JSON）要有一句能说的话，而不是"读出来是空的，什么都没做"。
	Kind    string `json:"kind"`
	Version int    `json:"version"`
	// Profile 是**这份备份从哪一份导出来的**，只作来路说明。导入的目标档案
	// **从凭证推导**（令牌的作用域 / 会话的那一份），不接受申报——与
	// `/api/v1/import` 同一族纪律："令牌与申报对不上"这件事在协议层就无法表达。
	Profile    string `json:"profile"`
	ExportedAt string `json:"exportedAt"`
	// Settings 与 Bookmarks **两段都可缺省**：包里有什么就写什么。
	// 只带设置的那份只改台面偏好，带书签的那份按导入的规矩来（目标必须空）。
	Settings  *backupSettings `json:"settings,omitempty"`
	Bookmarks *backupMarks    `json:"bookmarks,omitempty"`
}

// backupSettings 是**台面偏好**那一段。
//
// **每一格都是指针**：nil = 备份里没有这一格（导入时**不动它**），
// 非 nil = 有这一格（哪怕它指向空串——空串是"没选过"，是**值**不是"没有"）。
// 这一族形状（view / pageTitle / language / addrLevel 空串也要落盘）与
// model.Profile 是同一条纪律：空串与"没有"分不开时，"没选过"与"选了默认"
// 就在盘上成了两件事。
type backupSettings struct {
	Theme     *string `json:"theme,omitempty"`
	Layout    *string `json:"layout,omitempty"`
	View      *string `json:"view,omitempty"`
	AddrLevel *string `json:"addrLevel,omitempty"`
	PageTitle *string `json:"pageTitle,omitempty"`
	Language  *string `json:"language,omitempty"`
}

// backupMarks 是**书签文档**那一段。
//
// **它不是 bookmarks.json 的原样**：rev 不在里面（rev 是并发判据本身，
// 导入时由服务端分配），search / weather 也不在里面（它们是对象、页面一次
// 都不渲染，而 ops 那一族白名单里根本没有它们的写口——导出来再导不回去的
// 东西是骗人的导出）。留下的五格正是**可恢复的那一部分**：四个根级标量 +
// 分组树（每一组连它的条目一起）。
type backupMarks struct {
	Title        string        `json:"title"`
	Footer       string        `json:"footer"`
	OpenInNewTab bool          `json:"openInNewTab"`
	Clock        bool          `json:"clock"`
	Groups       []model.Group `json:"groups"`
}

// restoreResponse 是"导入一份备份"这件事的结论。
//
// **Settings 列的是改了哪几格**（不是格数）：台面偏好不产生 rev、不做快照，
// 所以它进不了「回退」——调用方得能从这句话里知道自己的哪几格被换了。
type restoreResponse struct {
	Profile  string   `json:"profile"`
	Rev      int64    `json:"rev"`
	Applied  int      `json:"applied"`
	Settings []string `json:"settings"`
	Warnings []string `json:"warnings"`
}

// backupError 带着"该怎么答"的那种错：状态码、kind、给人看的那一句，
// 外加**网页那一路要用的 i18n key**。
//
// **为什么自己带一套**：这一族的失败有五六种（文件不是这份格式、版本不认、
// 目标档案不空、设置里有一格不认、写盘失败），而机器那一路要 JSON、网页
// 那一路要页面上的一句话——两处各判一次就会有一处漂，而漂掉的那一处正是
// "人看见的失败原因与机器看见的不是同一件事"。
type backupError struct {
	code   int
	kind   string
	detail string
	// key 是网页那一路的句子（空 = 用那句通用的）。
	key string
}

func (e *backupError) Error() string { return e.detail }

func backupErr(code int, kind, key, detail string) *backupError {
	return &backupError{code: code, kind: kind, detail: detail, key: key}
}

// handleBackup 导出这一份的整份备份。
//
// **档案从 query 取**（与 /api/v1/data、/api/v1/export 同一族）："导出哪一份"
// 没有默认值，缺了就是 400。
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	id, ok := s.targetProfile(w, r, credScoped)
	if !ok {
		return
	}
	p, err := s.deps.Store.LoadProfile(id)
	if err != nil {
		s.writeProfileReadFailure(w, id, err)
		return
	}
	snap, err := s.deps.Store.Current(id)
	if err != nil {
		s.writeProfileReadFailure(w, id, err)
		return
	}
	pkg := backupPackage{
		Kind:       backupKind,
		Version:    backupVersion,
		Profile:    id,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Settings:   settingsOf(p),
		Bookmarks:  marksOf(snap.B),
	}
	raw, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		writeInternalError(w, err)
		return
	}
	raw = append(raw, '\n')
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`-backup.json"`)
	_, _ = w.Write(raw)
}

// settingsOf 把 profile.json 里那几格台面偏好取出来。
//
// **六格全给**（连空串也写进去）：空串是"没选过"，是**值**而不是"没有这一格"。
// 少了它，恢复到另一台机器时那一格会留在原样——于是"没选过"与"选了默认"
// 在盘上分不开（model.Profile 那一族形状纪律）。
func settingsOf(p *model.Profile) *backupSettings {
	theme, layout := p.Theme, p.Layout
	view, addr := p.View, p.AddrLevel
	title, lang := p.PageTitle, p.Language
	return &backupSettings{
		Theme:     &theme,
		Layout:    &layout,
		View:      &view,
		AddrLevel: &addr,
		PageTitle: &title,
		Language:  &lang,
	}
}

// marksOf 取书签文档里**可恢复的那一部分**。
func marksOf(b *model.Bookmarks) *backupMarks {
	groups := b.Groups
	if groups == nil {
		// 空档案给一个空数组而不是 null：这份文件是要被人打开看的，
		// 而 "groups": null 读起来像"这一段坏了"。
		groups = []model.Group{}
	}
	return &backupMarks{
		Title:        b.Title,
		Footer:       b.Footer,
		OpenInNewTab: b.OpenInNewTab,
		Clock:        b.Clock,
		Groups:       groups,
	}
}

// handleRestore 是机器通路：导入一份备份。
//
// **凭证在前，体在后**（与 importapi 那两条同一纪律）：目标档案从凭证推导，
// 不接受申报，于是"读哪一份"在读体之前就定了——而未鉴权的请求不该有机会
// 先把整份体读进内存。
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	id, actor, ok := s.restoreTarget(w, r)
	if !ok {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxImportBytes+1))
	if err != nil || len(raw) > maxImportBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{
			Code: 413, Kind: apierr.BodyUnreadable, Detail: "读不到请求体，或超过了 8 MB 上限"})
		return
	}
	pkg, berr := parseBackup(raw)
	if berr != nil {
		s.writeBackupError(w, berr)
		return
	}
	if s.profileDisabledRefuse(w, id) {
		return
	}
	out, berr := s.restoreInto(id, actor, pkg, byOfRequestDefault(r, "import"))
	if berr != nil {
		s.writeBackupError(w, berr)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRestoreForm 是浏览器那条路：台面面板里那个「导入」表单。
//
// **与 POST /api/v1/restore 共用 restoreInto 与 parseBackup**：判据只有一份。
// **只认会话**：档案从会话推导，令牌一律 303 回选择页（浏览器表单通路的
// 权限类拒绝答 303，不答 401/403——spec §6.1）。
func (s *Server) handleRestoreForm(w http.ResponseWriter, r *http.Request) {
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		redirectPicker(w)
		return
	}
	id := sess.Active
	tr := s.trFor(r, id)
	// **上限挂在 body 上**：ParseMultipartForm 的参数是内存缓冲上限，不是体积
	// 上限——超过它的部分会先被整份解析并落成临时文件，等我们 LimitReader
	// 到手时磁盘已经写过了（与 /import 同一处教训）。
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		s.renderPrefsNotice(w, r, id, tr.t("backup.err.tooLarge"))
		return
	}
	file, _, err := r.FormFile("backup")
	if err != nil {
		s.renderPrefsNotice(w, r, id, tr.t("backup.err.nofile"))
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil || len(raw) > maxImportBytes {
		s.renderPrefsNotice(w, r, id, tr.t("backup.err.tooLarge"))
		return
	}
	pkg, berr := parseBackup(raw)
	if berr != nil {
		s.renderPrefsNotice(w, r, id, tr.t(berr.key))
		return
	}
	if s.profileDisabled(id) {
		s.renderPrefsNotice(w, r, id, tr.t("backup.err.disabled"))
		return
	}
	if _, berr := s.restoreInto(id, id, pkg, "web"); berr != nil {
		s.renderPrefsNotice(w, r, id, tr.t(berr.key))
		return
	}
	// 成功：回面板的「这一页」那一节——**导入进来的设置就在那里看得见**，
	// 而那是这一次唯一需要人确认的东西（书签在回我的页面上看得见）。
	http.Redirect(w, r, "/prefs?tab=look", http.StatusSeeOther)
}

// restoreTarget 从凭证推导目标档案：令牌取它的作用域，会话取自己那一份。
//
// **不接受申报**（与 /api/v1/import 同一族纪律）：一枚 `name:secret:dad` 的
// 令牌只能写进 dad，而"令牌与申报对不上"这件事在协议层就无法表达。
func (s *Server) restoreTarget(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	if tok := bearer(r); tok != "" {
		// 管理员那把 key 只管账号，不管内容——它落到下面那次 Authorize 上只会
		// 得到"认不出来"的 401，而那是错的答案：它不是坏了，是**层级不对**。
		if s.deps.AdminToken.Matches(tok) {
			writeJSON(w, http.StatusForbidden, errorResponse{
				Code: 403, Kind: apierr.CredentialAdminToken,
				Detail: "管理员令牌只管账号；导入要用作用域正好是它的令牌"})
			return "", "", false
		}
		sc, ok := s.deps.Tokens.Authorize(tok)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResponse{
				Code: 401, Kind: apierr.CredentialUnknown, Detail: "令牌认不出来"})
			return "", "", false
		}
		if _, _, ok := s.require(r, credScoped, sc.Profile); !ok {
			s.writeAuthFailure(w, r)
			return "", "", false
		}
		return sc.Profile, sc.Name, true
	}
	sess := auth.ReadSession(r)
	if !sess.Valid() {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Code: 401, Kind: apierr.CredentialMissing, Detail: "需要令牌或会话"})
		return "", "", false
	}
	if _, _, ok := s.require(r, credScoped, sess.Active); !ok {
		s.writeAuthFailure(w, r)
		return "", "", false
	}
	return sess.Active, sess.Active, true
}

// parseBackup 把一份文件读成一份备份，并核它的 kind 与 version。
//
// **严格解码**（未知字段就拒）：AI 把 `settings.pageTitle` 写成 `title` 时
// 报"未知字段"比悄悄导进一个空页面名有用。
func parseBackup(raw []byte) (*backupPackage, *backupError) {
	var pkg backupPackage
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pkg); err != nil {
		return nil, backupErr(http.StatusUnprocessableEntity, apierr.BodyUnreadable, "backup.err.unreadable",
			"这份文件读不成一份备份（要的是 GET /api/v1/backup 导出的那份 JSON）")
	}
	if pkg.Kind != backupKind {
		return nil, backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.unreadable",
			"这不是 launchbay 的备份文件（kind 要是 "+backupKind+"）")
	}
	if pkg.Version != backupVersion {
		return nil, backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.version",
			"这份备份的版本是 "+strconv.Itoa(pkg.Version)+"，这台机器认的是 "+strconv.Itoa(backupVersion))
	}
	if pkg.Settings == nil && pkg.Bookmarks == nil {
		return nil, backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.unreadable",
			"这份备份里既没有 settings 也没有 bookmarks 段，没什么可导入的")
	}
	return &pkg, nil
}

// restoreInto 是这一族**唯一**的落点：机器那条与网页那条都走它。
//
// **先校验，后写盘**：设置那一段的错（不认识的主题、超过 60 个字的页面名）
// 在写任何东西之前就要说清——不然书签写进去了而设置没写进去，调用方拿到
// 的是"一半成功"，而它以为自己导入成功了。
func (s *Server) restoreInto(id, actor string, pkg *backupPackage, by string) (restoreResponse, *backupError) {
	if pkg.Settings != nil {
		if berr := checkBackupSettings(pkg.Settings); berr != nil {
			return restoreResponse{}, berr
		}
	}
	out := restoreResponse{Profile: id}
	// 书签那一段的门槛：**目标档案必须一条书签都没有**（与 importGuard 同一份
	// 判据）。导入的语义是"把这一份搬进来"，不是"往现有档案上叠加"——对着
	// 一个已经装了几十条书签的档案跑一遍，那几十条会无声消失。
	var cur *model.Bookmarks
	if pkg.Bookmarks != nil {
		snap, err := s.deps.Store.Current(id)
		switch {
		case errors.Is(err, store.ErrNotFound):
			return restoreResponse{}, backupErr(http.StatusNotFound, apierr.ProfileNotFound, "backup.err.notFound",
				"档案不存在")
		case err != nil:
			return restoreResponse{}, backupErr(http.StatusServiceUnavailable, apierr.ProfileBroken, "backup.err.broken",
				corruptDetail(s.deps.Store, id, err))
		}
		if n := bookimport.CountItems(snap.B); n > 0 {
			return restoreResponse{}, backupErr(http.StatusUnprocessableEntity, apierr.ImportTargetNotEmpty, "backup.err.notEmpty",
				"这一份里已经有 "+strconv.Itoa(n)+" 条书签了，导入要一个空的档案")
		}
		cur = snap.B
	}
	// **书签先写，设置后写**：设置那几格是六个标量，写下去几乎不会失败；而
	// 书签那一段要过快照与 rev，中途失败的话"什么都没写"这句话就说不清了。
	// 反过来先写设置，书签那段的"目标不空"就会留一个已经改过的台面。
	if pkg.Bookmarks != nil {
		outcome, err := s.applyBackupMarks(id, actor, by, cur, pkg.Bookmarks)
		if err != nil {
			return restoreResponse{}, backupErr(http.StatusInternalServerError, apierr.InternalError, "backup.err.write",
				"写盘失败")
		}
		if len(outcome.verrs) > 0 {
			// **书签那一段没通过校验**：这不是设置的事，所以不说"设置有一格不认"
			// （那会把人引去查设置，而设置没问题）。整批未写入。
			return restoreResponse{}, backupErr(http.StatusUnprocessableEntity, apierr.OpsInvalid, "backup.err.bookmarks",
				"书签那一段没通过校验，整批未写入")
		}
		if outcome.snapErr != nil {
			return restoreResponse{}, backupErr(http.StatusInsufficientStorage, apierr.SnapshotFailed, "backup.err.write",
				"快照写不下去，本次导入已取消")
		}
		if outcome.missing {
			return restoreResponse{}, backupErr(http.StatusNotFound, apierr.ProfileNotFound, "backup.err.notFound",
				"档案不存在")
		}
		if outcome.broken != "" {
			return restoreResponse{}, backupErr(http.StatusServiceUnavailable, apierr.ProfileBroken, "backup.err.broken",
				outcome.broken)
		}
		out.Rev = outcome.resp.Rev
		out.Applied = outcome.resp.Applied
		out.Warnings = outcome.resp.Warnings
	} else if rev, err := s.deps.Store.CurrentRev(id); err == nil {
		// **只导设置的那一次，rev 一个字都没动**：报当前那一个，不是 0——报 0
		// 会被读成「回到最初了」，而这一份压根没变。
		out.Rev = rev
	}
	if pkg.Settings != nil {
		written, err := s.applyBackupSettings(id, pkg.Settings)
		if err != nil {
			// **半份成功必须说清是哪半**：书签已经写进去了而设置没写进去时，
			// 调用方若以为整份都回滚了，就会拿同一份文件再导一次——那一次照样
			// 撞「目标档案不空」，而真正的缺口是设置没写进去。
			detail := "台面设置没写进去（" + err.Error() + "）"
			if pkg.Bookmarks != nil {
				detail = "书签已经写进去了，但" + detail
			}
			return restoreResponse{}, backupErr(http.StatusInternalServerError, apierr.InternalError, "backup.err.write", detail)
		}
		out.Settings = written
	}
	return out, nil
}

// checkBackupSettings 判备份里那几格台面偏好能不能写进 profile.json。
//
// **判据与台面面板那一份同源**（model.Valid* 那几张表 + 页面名 60 个字上限
// + 语言那一份名单），只**多认一个空串**：备份里的空串是"没选过"，它是盘上真实
// 存在的值；而表单那一格要清回默认，该送显式的默认值而不是空串。
func checkBackupSettings(st *backupSettings) *backupError {
	if st.Theme != nil && !model.ValidTheme(*st.Theme) {
		return backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.settings",
			"主题只能是 "+strings.Join(model.ThemeValues, "|"))
	}
	if st.Layout != nil && !model.ValidLayout(*st.Layout) {
		return backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.settings",
			"排法只能是 "+strings.Join(model.LayoutValues, "|"))
	}
	if st.View != nil && *st.View != "" && !model.ValidView(*st.View) {
		return backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.settings",
			"整页形态只能是 "+strings.Join(model.ViewValues, "|"))
	}
	if st.AddrLevel != nil && *st.AddrLevel != "" && !model.ValidAddrLevel(*st.AddrLevel) {
		return backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.settings",
			"第二行只能是 "+strings.Join(model.AddrValues, "|"))
	}
	if st.PageTitle != nil && len([]rune(*st.PageTitle)) > maxPageTitleRunes {
		return backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.settings",
			"页面名最多 "+strconv.Itoa(maxPageTitleRunes)+" 个字")
	}
	if st.Language != nil && *st.Language != "" && !i18n.Lang(*st.Language).Valid() {
		return backupErr(http.StatusUnprocessableEntity, apierr.FieldInvalid, "backup.err.settings",
			"语言只能是 "+langValues())
	}
	return nil
}

// applyBackupSettings 把备份里的台面偏好写进 profile.json。
//
// **与 POST /prefs 同一套判据**（见 checkBackupSettings），**不产生 rev、
// 不做快照**：台面偏好不是书签数据（与 /prefs 同一纪律），所以它进不了
// 「回退」——改错了再按一次键就是（面板上每一格都是独立的表单）。
//
// **没给的字段不碰**：只导了主题就不该把排法顺手归位。
func (s *Server) applyBackupSettings(id string, st *backupSettings) ([]string, error) {
	var written []string
	err := s.deps.Store.WithProfileLock(id, func() error {
		p, err := s.deps.Store.LoadProfile(id)
		if err != nil {
			return err
		}
		if st.Theme != nil {
			p.Theme = *st.Theme
			written = append(written, "theme")
		}
		if st.Layout != nil {
			p.Layout = *st.Layout
			written = append(written, "layout")
		}
		if st.View != nil {
			p.View = *st.View
			written = append(written, "view")
		}
		if st.AddrLevel != nil {
			p.AddrLevel = *st.AddrLevel
			written = append(written, "addrLevel")
		}
		if st.PageTitle != nil {
			p.PageTitle = strings.TrimSpace(*st.PageTitle)
			written = append(written, "pageTitle")
		}
		if st.Language != nil {
			p.Language = strings.TrimSpace(*st.Language)
			written = append(written, "language")
		}
		return s.deps.Store.SaveProfile(p)
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

// applyBackupMarks 把备份里的书签段写进档案。
//
// **落点是 applyBatch**：书签写入只有那一个落点（快照、rev、审计线都在它
// 内部），这里生成一批 ops 递进去，不自己写盘——绕过它等于造出第二份真相。
//
// **先删后加**：目标档案可能已经有几个空分组（面板上建了没放东西），不删掉
// 的话恢复完会多出几个空组，而"恢复完与备份一模一样"是这一段的全部意义。
// 删的都是空组（目标必须一条书签都没有，那道闸在上面），所以删它们不丢东西。
//
// **值没变的那几格不发 op**：ops 对"设的值与当前值相同"会给一条告警，而
// 一次恢复动辄四格——于是每次恢复的告警里都躺着四句"什么都没改"。不发那条
// op 的结果一样（值本来就相同），告警却是干净的。
func (s *Server) applyBackupMarks(id, actor, by string, cur *model.Bookmarks, m *backupMarks) (batchOutcome, error) {
	opList := make([]ops.Op, 0, len(cur.Groups)+len(m.Groups)+4)
	for _, g := range cur.Groups {
		opList = append(opList, ops.Op{Op: "remove", Path: "groups/" + g.ID})
	}
	if m.Title != cur.Title {
		opList = append(opList, ops.Op{Op: "set", Path: "title", Value: backupValue(m.Title)})
	}
	if m.Footer != cur.Footer {
		opList = append(opList, ops.Op{Op: "set", Path: "footer", Value: backupValue(m.Footer)})
	}
	if m.OpenInNewTab != cur.OpenInNewTab {
		opList = append(opList, ops.Op{Op: "set", Path: "openInNewTab", Value: backupValue(m.OpenInNewTab)})
	}
	if m.Clock != cur.Clock {
		opList = append(opList, ops.Op{Op: "set", Path: "clock", Value: backupValue(m.Clock)})
	}
	for _, g := range m.Groups {
		opList = append(opList, ops.Op{Op: "add", Path: "groups", Value: backupValue(g)})
	}
	return s.applyBatch(id, actor, by, nil, opList)
}

// backupValue 把一个值变成 op 的 value。
//
// 编码失败等于代码写错了（这里传的都是 model 里的可序列化字段），不是数据
// 问题——所以 panic 比返回半份 op 诚实。
func backupValue(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("备份的 op value 编码失败: %v", err))
	}
	return raw
}

// writeBackupError 把这一族的错按机器那一路的形状答出去。
func (s *Server) writeBackupError(w http.ResponseWriter, berr *backupError) {
	writeJSON(w, berr.code, errorResponse{Code: berr.code, Kind: berr.kind, Detail: berr.detail})
}
