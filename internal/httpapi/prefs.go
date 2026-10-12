package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
)

// handlePrefs 是浏览器表单通路：改的是"这个人对自己台面的偏好"（主题 +
// 档案形态）。它**不是** API 端点，也不建 JSON 版——表单发不出 JSON，而 AI
// 替你换主题没有真实需求；每多一条 API 面就要 openapi 与 AGENTS.md 各同步一次
// （spec §6.1）。
//
// 为什么只认会话不认令牌：与 self/pin 同族同判据——"谁能改谁"。管理员令牌
// 替别人改偏好是另一回事，本期没有这个需求，也不预留。
//
// 拒的长相是 303 回选择页，不是 401/403（spec §6.1 的浏览器通路映射）：
// 这条路只由页面表单走进来，走到这里就没人能消费一段 JSON 错误。
// 422（形状错）与 503（文件读不了）照旧走 JSON 外壳——它们不是权限拒绝。
//
// 为什么不写历史行：它不动书签、不产生 rev，与"改自己 PIN"同类。四条不写
// 审计行的动作一起记在 spec §15，防止下一个人在这里"顺手补一行"。
// maxPageTitleRunes 是"这一页叫什么"那一格的长度上限。按**字**（rune）数而不是
// 按字节：这一格写中文是常态，按字节算会让中文比英文少 3 倍额度而没人说得清为什么。
const maxPageTitleRunes = 60

func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		redirectPicker(w)
		return
	}
	// 上限必须在这里设：ParseForm 会整段读 body，不套 MaxBytesReader 的话
	// 承诺的 1 MiB 对这条无 JS 通路是假话（handlePin 的表单分支同一条教训）。
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			// 字段名跟着 handlePin 的同一失败走（body）：同一个病在两个端点上叫两个名，
			// AI 学到的词汇表就分家了。
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	theme, hasTheme := r.Form["theme"]
	layout, hasLayout := r.Form["layout"]
	title, hasTitle := r.Form["title"]
	if !hasTheme && !hasLayout && !hasTitle {
		// "没说要改什么"不是"什么都不做然后成功"：那会让一个拼错的字段名
		// 静默返回 303，而调用方以为改成了。
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
			{Field: "theme", Reason: "要带 theme、layout 或 title 至少一个"},
			{Field: "layout", Reason: "要带 theme、layout 或 title 至少一个"},
			{Field: "title", Reason: "要带 theme、layout 或 title 至少一个"},
		})
		return
	}
	// 同名多值（?theme=dark&theme=light）不是"取第一个"，是没说清要改什么。
	if hasTheme && (len(theme) != 1 || !model.ValidTheme(theme[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "theme", Reason: "只能是 " + strings.Join(model.ThemeValues, "|")}})
		return
	}
	if hasLayout && (len(layout) != 1 || !model.ValidLayout(layout[0])) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "layout", Reason: "只能是 " + strings.Join(model.LayoutValues, "|")}})
		return
	}
	// title 是**这一页叫什么**（profile.PageTitle），不是"这是谁的那一份"。
	// 上限必须在服务端判：表单那个 maxlength 只有浏览器认，而 curl 与扩展不认——
	// 这一格会进每一张档案页的 <h1> 与 <title>，一条几 KB 的名字等于把首屏撑大。
	// 空串是**合法值**，意思是"没起过名字"：渲染侧按界面语言给通用默认，
	// 不往盘里编一个字（清名字与改名走同一格表单）。
	if hasTitle && (len(title) != 1 || len([]rune(title[0])) > maxPageTitleRunes) {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "title", Reason: fmt.Sprintf("最多 %d 个字，空着 = 用默认名", maxPageTitleRunes)}})
		return
	}
	var saveErr error
	id := sess.Active
	// 读→改→验→写整段进锁，与 handleSelfPin 同形（评审 Important 1 的教训：
	// SaveProfile 不自带锁，裸 RMW 与并发的管理员设 PIN 撞车会互相盖掉）。
	// 没给的字段不碰：只改 layout 不许把 theme 顺手归位。
	err := s.deps.Store.WithProfileLock(id, func() error {
		p, err := s.deps.Store.LoadProfile(id)
		if err != nil {
			return err // 分诊在锁外做：404 / 503 / 500
		}
		if hasTheme {
			p.Theme = theme[0]
		}
		if hasLayout {
			p.Layout = layout[0]
		}
		if hasTitle {
			// 空串照旧写下去（= 清除），不是"没给"：没给的是 !hasTitle 那一支。
			p.PageTitle = strings.TrimSpace(title[0])
		}
		saveErr = s.deps.Store.SaveProfile(p)
		return nil
	})
	if err != nil {
		// 与 handleSelfPin 同一条分诊，不新造第四种口径。
		s.writeProfileReadFailure(w, id, err)
		return
	}
	if saveErr != nil {
		writeInternalError(w, saveErr)
		return
	}
	// 成功与拒都回 `/`：字节相同，语义不同（拒是"你没资格动这条路"，成功是
	// "改完了，回首页看新主题"）。判据在 require 那一份里，外壳在这一句里。
	redirectPicker(w)
}
