package httpapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/rockbenben/launchbay/internal/apierr"
	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/render"
)

// handleAdminPage 是 §8.8 的管理页（读侧）。④ 交付时这一页只读；④.5 起它同时是
// 建档表单的落点（handleAdminCreate 在同一文件，同一份 require 判据）。
//
// 权限判据还是 require(credAdmin)——判据只有一份。本期改的是**拒的长相**：
// 浏览器通路的权限拒绝 = 303 回选择页、不报错（spec §6.1）。给家属吐一段
// 403 是坏体验，而"你在不在管理员名单里"本身是可枚举信息。
//
// 管理员令牌打它会拿到同一张 HTML 页面（credAdmin 认它）。这不是第二条 API
// 面：要聚合数据请打 GET /api/v1/status，同一个 statusRows、两种壳。AGENTS.md
// 的说明列写"AI 不走这条"。
//
// 页面上没有条目内容（只有数量与时间，spec §8.8），页面 no-store
// （AdminPage 的 NoStore——跨档案页面进缓存等于把全家庭名单留给同一台电脑的
// 下一个人）。坏档案那一行照占一行（聚合只有 statusRows 一份）。
//
// **没有管理权时不是 303 回选择页，而是渲染登录页**（2026-10-12）：管理员不再
// 是某个档案，于是"进管理页"需要那把 key，而升级完还没有人输过它——沿用旧的
// 303 形状会让整个账号层面锁死在一个没有人能开的页面上。
func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	// **判据是 require(credAdmin) 一处**（2026-10-20 修）：从前这里问的是
	// `hasAdminCookie(r)`——**只读 cookie**，于是管理员那把 key 打过来拿到的是
	// 登录页（200 + 一个表单）。而契约（AGENTS.md 端点表、openapi 的 security、
	// 以及这一段注释自己写的"管理员令牌打它会拿到同一张 HTML 页面"）都说它认。
	// 形状上"成功"、内容却是错的，脚本会以为自己没登录——这是最容易误判的一种
	// 不一致。
	//
	// require(credAdmin) 认**管理员那把 key 或 `lb_admin` cookie**（两条管理员
	// 通路），而**档案会话不算管理权**（登进 dad 也不等于能管这台机器）。
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		s.handleAdminLoginPage(w, r)
		return
	}
	rows, _, _, err := s.statusRows()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	// statusProfile → AdminRow 的映射只在这一个地方做：模板不吃 API 的
	// JSON 标签，契约与页面因此不会被迫同一天一起改。
	views := make([]render.AdminRow, 0, len(rows))
	// "这一行能不能设 PIN / 能不能删"分两族判据，各有各的理由：
	//   - **设 PIN**（加/改）：只要求"这一行不是坏档"。管理员是**机器级**的
	//     一把 key，不属于任何档案，所以"改自己的 PIN 要旧 PIN"那条保护在
	//     这里让开——本人自己那边（self/pin）照样要旧 PIN，而管理员这一边
	//     是"家人忘了 PIN 的出路"，它必须能管**每一份**档案。
	//   - **删档 / 停用**：要"不是当前会话正在用的那一份"。删掉自己正在用
	//     的那一份会把会话悬空，而停用自己正在用的那一份等于自己把自己
	//     关在门外——那是**会话**层面的问题，与权限无关。
	active := sessionProfileID(r)
	for _, row := range rows {
		isActive := row.ID == active
		views = append(views, render.AdminRow{
			ID: row.ID, Title: row.Title, Disabled: row.Disabled,
			Rev:      row.Rev,
			Modified: row.Modified, Items: row.Items, Snapshots: row.Snapshots,
			Broken:    row.Broken,
			PinSet:    row.PinSet,
			CanSetPin: !row.Broken,
			CanDelete: !isActive,
			// 停用/启用是**同一个动作的两头**：能不能点取决于"不是自己正在用的
			// 那一份"，而这一枚按钮今天是停还是启由 Disabled 翻（模板里翻）。
			// 坏档不给——改不了 profile.json 就翻不动这个标志。
			CanToggleDisabled: !isActive && !row.Broken,
			IsActive:          isActive,
		})
	}
	// ?err= 只当键用：不在白名单里的值一个字都不进页面（与进门页同一套，
	// 判据由 httpapi 算，渲染器只印）。
	notice := s.notice(r, r.URL.Query().Get("err"))
	opts := s.adminOptions()
	// 字段错重画时把刚输入的名字带回来（2026-10-24，与 /setup、/new 同一处置）：
	// 建档失败一次就清空名字，人得从头再打一遍；PIN 错的那一发更会顺手变成
	// noname，白跑两趟。PIN 一个字都不回填——它是这一页的凭证。
	opts.FormTitle = strings.TrimSpace(r.URL.Query().Get("title"))
	page, err := s.deps.Render.AdminPage(views, notice, opts, s.machineLang(r))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// handleAdminCreate 是管理页的建档表单。判据一条都不新写：权限走现成的
// require(credAdmin, "")，校验/哈希/落盘走 createProfile（Task 3 抽的唯一落点）。
// 与引导那条（/setup）共用 DeriveID（两张表单都不问 id），差别只有两处：撞名要说得出话
// （引导那条撞车等于"窗口已关"，回 / 不带键），以及**没有 admin 那一格**——
// 档案不再有管理员属性（2026-10-12）。
//
// 浏览器通路不认令牌（/prefs、/switch 同族纪律）：credAdmin 本来会放行管理员令牌，
// 让令牌走这条路等于同一个动作两个入口、鉴权各写一半；机器建档有 API 那条。
func (s *Server) handleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if bearer(r) != "" {
		redirectPicker(w)
		return
	}
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		redirectPicker(w)
		return
	}
	// 形状错与字段错的两种长相在 /setup 那条上已经定过一次：这里同一套
	// （422 JSON 点名是哪个字段；没填对才 303 带白名单键）。
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	if n := len(r.Form["title"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "title", Reason: "同一个字段只能出现一次"}})
		return
	}
	if n := len(r.Form["pin"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "pin", Reason: "同一个字段只能出现一次"}})
		return
	}
	res := s.createProfile(createInput{
		Title: r.FormValue("title"),
		PIN:   r.FormValue("pin"),
		// **没有 admin 那一格**（2026-10-12）：建出来的就是一个人的书签页。
		// 管理页也不再有那个勾选框——勾它现在已经没有任何后果，留着会让人
		// 以为"建成管理员"是一件事，而它已经不存在了。
		NoPass:   nopassFlag(r.FormValue("nopass")),
		DeriveID: true,
	})
	switch {
	case len(res.FieldErrs) > 0:
		redirectAdminCreateFail(w, r, setupFieldKey(res.FieldErrs))
	case res.Status == http.StatusConflict:
		// 派生的 id 撞上并发提交（两个同名请求算出同一个空位）。落点只在这里分：
		// 少了这一支，409 会掉进下面那条 writeInternalError(res.Err)，而 409 那一支
		// 是不带 Err 的（Detail 才是白名单文本）——等于吐一个"内部错误: <nil>"的 500。
		redirectAdminCreateFail(w, r, "nametaken")
	case res.Permission:
		// 与 /setup 同一族、同一句：宿主数据目录对跑服务的用户不可写。
		// 管理页是管理员装机后第二眼会看的地方，这里回 500 JSON 等于把
		// "去宿主机 chown" 这句话藏进 stderr。
		redirectAdminCreateFail(w, r, "permission")
	case res.Status != 0:
		writeInternalError(w, res.Err)
	default:
		w.Header().Set("Location", "/admin")
		w.WriteHeader(http.StatusSeeOther)
	}
}

// redirectAdminFail 与 redirectSetup 同形，只是回的是管理页。code 只来自本包的常量
// （setupFieldKey 的两个键与 "nametaken"），永远不是用户可控的字符串；页面那头还要
// 过 pinNotices 白名单。
func redirectAdminFail(w http.ResponseWriter, code string) {
	w.Header().Set("Location", "/admin?err="+code)
	w.WriteHeader(http.StatusSeeOther)
}

// redirectAdminCreateFail 是建档表单那一族：键与 redirectAdminFail 同一份白名单，
// 另外把**刚输入的名字**带回去（2026-10-24，与 /setup、/new 同一处置）——失败一次
// 就清空名字，人得从头再打一遍；PIN 错的那一发更会顺手变成 noname，白跑两趟。
// **PIN 一个字都不回填**：它是这一页的凭证，不该出现在 URL/历史里。
//
// 名字从 r 里读而不是由调用方递：那条白名单闸按调用实参里的字面量收键，
// 多递一个 `r.FormValue("title")` 会让 "title" 被当成一个 ?err= 的键。
func redirectAdminCreateFail(w http.ResponseWriter, r *http.Request, code string) {
	loc := "/admin?err=" + code
	if t := strings.TrimSpace(r.FormValue("title")); t != "" {
		loc += "&title=" + url.QueryEscape(t)
	}
	w.Header().Set("Location", loc)
	w.WriteHeader(http.StatusSeeOther)
}

// adminOptions 是"这台机器的管理密钥"那两格：公开的默认值，与"现在用的还是不是它"。
//
// **当前配置的那一把不进这里**（它会进 devtools、进导出、进浏览器同步）；
// 进页面的只有 compose 里那个**公开的**默认值——装好机器的人要知道自己该输
// 什么，而"默认那把人人都猜得到"正是要提醒他换掉它的理由。
//
// 判据在 httpapi（deps.AdminToken 在这儿），渲染器不判断。
func (s *Server) adminOptions() render.AdminOptions {
	return render.AdminOptions{
		DefaultAdminKey:      auth.DefaultAdminToken,
		UsingDefaultAdminKey: s.deps.AdminToken.IsDefault(),
		DefaultLang:          s.rawDefaultLang(),
	}
}

// rawDefaultLang 是**盘上写着的那个**默认语言（"" = 没人设过）。
//
// 它与 machineLang（这一页正在用的那一种）**在"没人设过"时不一样**：那时页面按
// **浏览器说的**那一门显示，而盘上什么都没写。管理页与引导页那两个下拉要显示
// 的正是这个——从前它们显示的是"这一页正在用的那一种"，于是管理员看到"English"
// 被选中就以为这台机器已经是英文的了（而他按一下那颗按钮，就把"没人设过"
// 悄悄变成了"显式设成英文"）。
func (s *Server) rawDefaultLang() string {
	if l, ok := s.deps.Store.DefaultLanguage(); ok {
		return string(l)
	}
	return ""
}
