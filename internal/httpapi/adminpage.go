package httpapi

import (
	"net/http"

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
// 下一个人）。坏档案那一行照占一行（聚合只有 statusRows 一份），但**不给
// 「进入编辑」按钮**——/switch 对读不了的档案不切，死按钮不进界面。
func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		redirectPicker(w)
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
	// "这一行能不能设 PIN / 能不能删"只取决于**它是不是当前会话正在用的那一份**
	// （active）。判据与 adminaction.go 的 deleteBlocked / selfpin 同一句，
	// 在这里算一次传下去——模板不判断，页面因此不会给出点了必被拒的入口。
	active := sessionProfileID(r)
	for _, row := range rows {
		isActive := row.ID == active
		views = append(views, render.AdminRow{
			ID: row.ID, Title: row.Title, Admin: row.Admin, Disabled: row.Disabled,
			Rev:      row.Rev,
			Modified: row.Modified, Items: row.Items, Snapshots: row.Snapshots,
			Broken:    row.Broken,
			CanSetPin: !isActive && !row.Broken,
			CanDelete: !isActive,
			// 停用/启用是**同一个动作的两头**：能不能点取决于"不是自己正在用的
			// 那一份"，而这一枚按钮今天是停还是启由 Disabled 翻（模板里翻）。
			// 坏档不给——改不了 profile.json 就翻不动这个标志。
			CanToggleDisabled: !isActive && !row.Broken,
		})
	}
	// ?err= 只当键用：不在白名单里的值一个字都不进页面（与进门页同一套，
	// 判据由 httpapi 算，渲染器只印）。
	notice := pinNotices[r.URL.Query().Get("err")]
	page, err := s.deps.Render.AdminPage(views, notice)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.writePage(w, r, page)
}

// handleAdminCreate 是管理页的建档表单。判据一条都不新写：权限走现成的
// require(credAdmin, "")，校验/哈希/落盘走 createProfile（Task 3 抽的唯一落点）。
// 与引导那条（/setup）共用 DeriveID（两张表单都不问 id），差别在三处：要管理员会话、
// admin 由勾选决定、撞名要说得出话（引导那条撞车等于"窗口已关"，回 / 不带键）。
//
// 浏览器通路不认令牌（/prefs、/switch 同族纪律）：credAdmin 本来会放行 `*` 令牌，
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
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	if n := len(r.Form["title"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "title", Reason: "同一个字段只能出现一次"}})
		return
	}
	if n := len(r.Form["pin"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "pin", Reason: "同一个字段只能出现一次"}})
		return
	}
	res := s.createProfile(createInput{
		Title: r.FormValue("title"),
		PIN:   r.FormValue("pin"),
		// 勾选了才是管理员。值 "1" 是 admin.tmpl 那张表单写死的接件：模板改成别的值
		// 或删掉 value（浏览器会发 "on"），Task 5 的破坏 #6 会红在这里。
		Admin:    r.FormValue("admin") == "1",
		DeriveID: true,
	})
	switch {
	case len(res.FieldErrs) > 0:
		redirectAdminFail(w, setupFieldKey(res.FieldErrs))
	case res.Status == http.StatusConflict:
		// 派生的 id 撞上并发提交（两个同名请求算出同一个空位）。落点只在这里分：
		// 少了这一支，409 会掉进下面那条 writeInternalError(res.Err)，而 409 那一支
		// 是不带 Err 的（Detail 才是白名单文本）——等于吐一个"内部错误: <nil>"的 500。
		redirectAdminFail(w, "nametaken")
	case res.Permission:
		// 与 /setup 同一族、同一句：宿主数据目录对跑服务的用户不可写。
		// 管理页是管理员装机后第二眼会看的地方，这里回 500 JSON 等于把
		// "去宿主机 chown" 这句话藏进 stderr。
		redirectAdminFail(w, "permission")
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
