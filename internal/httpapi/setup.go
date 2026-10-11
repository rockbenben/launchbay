package httpapi

import (
	"net/http"
)

// handleSetup 是引导建档：零档案时任何人建第一个（建成管理员），建完这条通路自己
// 关闭。三条纪律都在代码里，不在注释里：
//
//   - handler **不数档案**：判据在 store 的锁内（CreateFirstProfile，Task 1）。数两次
//     会得出两个答案，而"谁有资格写"这种问题不能有第二个答案的来源；
//   - 成功只回 303，**不发会话**：免令牌建档不等于免令牌进入（设计 §3）。建完还是
//     要输一次 PIN，这一页本来就是输 PIN 的那一页；
//   - 拒的长相分两种：字段没填对 → 303 带 `?err=` 白名单键（引导页是给人看的）；
//     窗口已关 → 303 回 `/` 且不解释（"这台机器已经建过档了"本身就是枚举线索）；
//     表单读不出形状 → 422 JSON 外壳，与 /prefs、/pin 同族——它不是权限拒绝。
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	// 上限必须在这里设：ParseForm 会整段读 body，不套 MaxBytesReader 的话承诺的
	// 1 MiB 对这条无 JS 通路是假话（handlePin / handlePrefs 的同一条教训）。
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		// 字段名与 /prefs、/pin 的同一失败对齐：同一个病在三个端点上叫两个名，
		// AI 学到的词汇表就分家了。
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Field: "body", Reason: "表单读不出来"}})
		return
	}
	// 同名多值（title=a&title=b）不是"取第一个"，是没说清要建哪个（/prefs 同纪律）。
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
		// 表单里没有 admin 这一格，也不许有：引导建成的这个就是这台盒子的主人，
		// 否则零档案装完后没人能进管理页加第二个人。
		Admin:     true,
		DeriveID:  true,
		FirstOnly: true,
	})
	switch {
	case len(res.FieldErrs) > 0:
		redirectSetup(w, setupFieldKey(res.FieldErrs))
	case res.Status == http.StatusConflict:
		redirectPicker(w)
	case res.Permission:
		// 落盘失败里唯一说得出出路的一族（宿主数据目录不可写）：303 回引导页
		// 带白名单键，而不是给一个刚打开浏览器的人一段 500 JSON——这是装机的
		// 头一个故障，页面上那句话正好告诉他下一步在宿主机上。
		redirectSetup(w, "permission")
	case res.Status != 0:
		// 其余落盘失败不是权限拒，走 JSON 外壳（spec §6.1 的两种长相）。
		writeInternalError(w, res.Err)
	default:
		redirectPicker(w)
	}
}

// redirectSetup 把浏览器能懂的失败送回引导页。code 只来自本包的常量，永远不是
// 用户可控的字符串——页面那一头还要过 pinNotices 白名单。
func redirectSetup(w http.ResponseWriter, code string) {
	w.Header().Set("Location", "/?err="+code)
	w.WriteHeader(http.StatusSeeOther)
}

// setupFieldKey 把 createProfile 的字段错翻成 ?err= 的键。引导表单只有两格，所以
// 这是一张小表而不是拼接：id 由名字派生（不问），建档也不接受任何"起步带几条"的东西。
// 名字排在口令前面——两格都没填对时说"要先起个名字"，因为那是这一页的第一格；
// 出现第三种字段说明判据变了，那种情况下宁可说中性的那句。
func setupFieldKey(errs []fieldError) string {
	for _, e := range errs {
		if e.Field == "title" {
			return "noname"
		}
	}
	for _, e := range errs {
		if e.Field == "pin" {
			return "badpin"
		}
	}
	return "noname"
}
