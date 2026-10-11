package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/rockbenben/launchbay/internal/bookimport"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

// 回退到某一版的**零 JavaScript** 落点（§8.8 那套两步确认的第四张脸）。
//
// 为什么不排到 ⑤ 的 JS 后面：它是"改坏了能回来"这条承诺的**唯一入口**。
// 一条承诺了却没有按钮的信任等于没有承诺——而人点不到的东西不算数。
//
// 谁能用：**只有当前会话自己的那一份**（credSession，目标从会话推导、
// 表单里不申报）。这与 2026-10-10 的管理员边界同一条纪律——管理员管账号、
// 不碰内容，所以管理员不能回退别人的书签，哪怕他就是给那个人设的 PIN。
// 令牌一律 303 回 `/`：CLI 与机器那条走 `/api/v1/revert`，浏览器表单与机器
// 各写一半鉴权，两处迟早差一次。

// revertGuard 是这两页共有的准入。返回的 id 是**服务端**从会话读出来的那一份，
// 不是任何人能填的东西。
//
// 返回 false 表示已经答完了，调用方直接 return。
func (s *Server) revertGuard(w http.ResponseWriter, r *http.Request) (string, bool) {
	if bearer(r) != "" {
		redirectPicker(w)
		return "", false
	}
	id, _, ok := s.require(r, credSession, "")
	if !ok {
		redirectPicker(w)
		return "", false
	}
	return id, true
}

// redirectRevertErr 是失败那一支。**调用点一律写字面量**（Test提示键只发白名单里有的
// 那条 AST 闸按字面量收键；传变量的话键就脱离了这道闸，而未登记的键到这里没人拦）。
func redirectRevertErr(w http.ResponseWriter, code string) {
	w.Header().Set("Location", "/revert?err="+code)
	w.WriteHeader(http.StatusSeeOther)
}

// redirectProfile 是成功那一支：303 回 `/`。与 redirectPicker 同一个 Location
// ——GET / 在有会话时渲染的就是档案页（无会话才是选择页）——单独起个名字是为了
// 让"退完了回哪"在代码里读得出来，而不是让下一个人去查 redirectPicker 的注释。
func redirectProfile(w http.ResponseWriter) {
	redirectPicker(w)
}

// snapshotChoice 是页面上的一格：**认得出它的那句话**，不是一个裸版本号。
//
// 一个只有 "rev 12" 的列表没法用来做决定——人不知道哪一版是他要的。条目数与
// 分组名是"当时大概长什么样"的两个代理指标（§8.8 删档页用条目数与快照数写死
// 后果，同一条纪律）。
type snapshotChoice struct {
	// When 是那份快照落盘的时间（"上周三那版"里的那个"上周三"）。
	When  string
	Rev   int64
	Items int
	// Groups 是页面上**印得下**的那几个组名，Total 是那一版**实际**有几个分组。
	// 两个数必须分开：收组名时一截断，"等 N 组"拿 len(Groups) 就说成了截断后的
	// 个数，于是有八个分组的人读到的是"等 3 组"——这一页唯一要说清的就是
	// "那一版大概长什么样"，报少了就是把它没看全的事说成看全了。
	Groups []string
	Total  int
}

// maxChoiceGroupNames 是页面上印几个分组名。三个够认出"自建服务、常用、影音"
// 这类清单了；把十几个分组名全印出来只会把这一页变成第二张书签页。
const maxChoiceGroupNames = 3

// snapshotChoices 列出可回退的版本，**按时间从新到旧**。
//
// 读不出来的快照不列出来，但**要点名报数**（返回 unreadable）——静默少列
// 几份会让人以为"只有这几个版本"，而一份读不出来的快照躺在那里不吭声才是
// 真正该被说出口的事（与导入报告"## 没能进来"逐条点名同一纪律）。
//
// 代价是这一页要把快照读一遍（最多 50 份）。这是"出事了才打开"的一页，
// 而一个只显示版本号的列表解决不了任何问题。
func (s *Server) snapshotChoices(id string) (choices []snapshotChoice, unreadable int, err error) {
	revs, err := s.deps.Store.ListSnapshots(id)
	if err != nil {
		return nil, 0, err
	}
	for _, rev := range revs {
		raw, err := s.deps.Store.LoadSnapshot(id, rev)
		if err != nil {
			unreadable++
			continue
		}
		b, err := store.DecodeBookmarks(s.deps.Store.BookmarksPath(id), raw)
		if err != nil {
			unreadable++
			continue
		}
		c := snapshotChoice{Rev: rev, Items: bookimport.CountItems(b), Total: len(b.Groups)}
		if when, ok := s.deps.Store.SnapshotMTime(id, rev); ok {
			c.When = when.Local().Format("01-02 15:04")
		}
		for _, g := range b.Groups {
			if len(c.Groups) == maxChoiceGroupNames {
				break
			}
			c.Groups = append(c.Groups, clipRunes(g.Title, maxChoiceGroupNameRunes))
		}
		choices = append(choices, c)
	}
	return choices, unreadable, nil
}

// choiceLabel 把一格说成人话。
// 它是 **Server 的**方法而不是 snapshotChoice 的：这一格要拼给人看的短名，
// 而语言是机器级的、只有 Server 读得到 store。做成快照自己的方法就只能把
// 语言当参数传进来——那等于让每一个调用点都要记得传。
func (s *Server) choiceLabel(c snapshotChoice) string {
	label := s.t("revert.choice.label", c.Rev, c.Items)
	if c.When != "" {
		label = c.When + " · " + label
	}
	if len(c.Groups) == 0 {
		return label + s.t("revert.choice.noGroups")
	}
	// 组名只印收进来的那几个、每个按字截：一行六十八字的长句在下拉里会被裁，
	// 在电话上会占三行，而读屏的人要把整串组名念完才听到最后那句"等 N 组"。
	out := label + s.t("revert.choice.groups", strings.Join(c.Groups, s.t("revert.sep")))
	// N 用 Total 而不是 len(Groups)：后者是"页面上收下了几个名字"，
	// 拿它当总数就等于把截断说成那一版只有这么几个分组。
	if c.Total > len(c.Groups) {
		out += s.t("revert.choice.more", c.Total)
	}
	return out
}

// maxChoiceGroupNameRunes 是一个组名在页面上最多占几个字。这一格要回答的是
// "那一版大概有哪几组"，不是"每一组的全名"，而全名一长就把整条列表撑成书签页。
const maxChoiceGroupNameRunes = 12

// clipRunes 按字截（一个汉字算一个字），超出加省略号。
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// currentLine 是"现在这一版"那一行。它与下面每个候选并排摆着，这一页才回答得了
// 人真正在问的那句问题——"我想退回到哪一版"，而这个判断离不开"我现在是什么样"。
//
// 读不出来时照实说读不出来：那正是坏档，而这正是 revert 的出路，不该被这一页
// 挡在门外。
func (s *Server) currentLine(id string) (string, error) {
	cur, err := s.deps.Store.Current(id)
	if err != nil {
		if !isUnreadable(err) {
			return "", err
		}
		return s.t("revert.now.unreadable"), nil
	}
	return s.t("revert.now.ok", cur.Rev, bookimport.CountItems(cur.B)), nil
}

// handleRevertPage 是第一步：把可选的版本列出来，让人**认出**要退哪一版。
func (s *Server) handleRevertPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.revertGuard(w, r)
	if !ok {
		return
	}
	// 档案整个没了（会话指着一个已删的档案）→ 回选择页，那里会重新列一遍。
	// **坏档不挡**：LoadProfile 读不出 profile.json 时继续往下走，revert 只回
	// 书签（§4.6），而坏档正是最需要它的时候。
	p, err := s.deps.Store.LoadProfile(id)
	if errors.Is(err, store.ErrNotFound) {
		redirectPicker(w)
		return
	}
	if err != nil && !errors.Is(err, store.ErrCorrupt) && !errors.Is(err, store.ErrInvalid) {
		writeInternalError(w, err)
		return
	}
	name := id
	if err == nil && strings.TrimSpace(p.Title) != "" {
		name = p.Title
	}

	now, err := s.currentLine(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	choices, unreadable, err := s.snapshotChoices(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	// Explain 是**纯文本**，一个字都不能是 markdown：`{{.Explain}}` 走 html/template
	// 的转义，`**这样**` 在页面上就是两个星号（这一版的初稿真这么写过，实拍才发现）。
	// 强调靠句子结构，不靠符号——与导入报告那条证人同一纪律。
	explain := now + "\n\n" + s.t("revert.explain.main")
	if unreadable > 0 {
		explain += "\n\n" + s.t("revert.explain.unreadable", unreadable)
	}
	if len(choices) == 0 {
		explain += "\n\n" + s.t("revert.explain.noversions")
	}

	view := render.AdminAction{
		Title: s.t("revert.title"), Heading: s.t("revert.heading", name),
		Explain: explain,
		Notice:  s.notice(r.URL.Query().Get("err")),
		Action:  "/revert",
		Submit:  s.t("revert.submit"),
		// 回管理页对一个非管理员是死路（他根本没有那一页，点了吃一个不解释的 303）。
		NavHref:  "/",
		NavLabel: s.t("revert.nav"),
	}
	if len(choices) > 0 {
		view.Choices = make([]render.AdminChoice, 0, len(choices))
		view.ChoiceName = "to-rev"
		view.ChoiceLegend = s.t("revert.legend")
		for i, c := range choices {
			view.Choices = append(view.Choices, render.AdminChoice{
				Value: fmt.Sprintf("%d", c.Rev), Label: s.choiceLabel(c),
				// 预选**最新一份快照**（列表降序，所以是 i==0）。
				//
				// 这一条推得出来是因为 WriteSnapshot 在**写之前**拍：写完 rev3 之后
				// 盘上是 rev3，而快照里最新的那一份是 rev2——**正是上一次改动之前的样子**，
				// 而人点进这一页的绝大多数原因是"刚才那次改动不对"。所以"最新"
				// 在这里不是"当前"，别把它当成会退成空操作的那一格。
				//
				// （反过来选次新那一格才是错的：那是两次改动之前，人要么不记得它，
				// 要么它已经被下一次改动改过意思了。）
				Checked: i == 0,
			})
		}
	} else {
		// 零份快照时**整张表单都不画**：一个只有按钮的空表单，点了只会把人送到
		// 一条没有出路的 POST（而那把键还得在页面上说人话，白白多一次往返）。
		view.NoForm = true
	}
	page, err := s.deps.Render.AdminActionPage(view)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// handleRevertForm 是第二步：**不自写落盘判据**，走 revertLocked——与
// POST /api/v1/revert 同一个。差别只在怎么读入、怎么答出。
func (s *Server) handleRevertForm(w http.ResponseWriter, r *http.Request) {
	id, ok := s.revertGuard(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	if n := len(r.Form["to-rev"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "to-rev", Reason: "同一个字段只能出现一次"}})
		return
	}
	rev, ok := parseRevertRev(r.FormValue("to-rev"))
	if !ok {
		redirectRevertErr(w, "badsnap")
		return
	}
	// actor 就是这一份自己：这一页不接受申报，所以历史里的 actor 只能是它。
	out, err := s.revertLocked(id, id, rev, "web")
	if err != nil {
		// **绝不说成功**：这一支里书签有可能已经写下去了（历史追加失败之类），
		// 说失败会让人再点一次，而 revert 再点一次就是又一个新版本。
		fmt.Fprintf(os.Stderr, "revert form failed profile=%s to=%d: %v\n", id, rev, err)
		writeInternalError(w, err)
		return
	}
	switch {
	case out.NotFound:
		// 页面上给过的那个号，到这一刻已经不在了（被别人退掉，或被裁剪掉了）。
		redirectRevertErr(w, "gone")
	case out.SnapFailed:
		// 什么都没变——这一支要说清"没变"，否则人会以为退了一半。
		redirectRevertErr(w, "nosnap")
	default:
		redirectProfile(w)
	}
}
