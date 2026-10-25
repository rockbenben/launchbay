package httpapi

import (
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"net/url"
	"strings"

	"github.com/rockbenben/launchbay/internal/i18n"
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
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	// 同名多值（title=a&title=b）不是"取第一个"，是没说清要建哪个（/prefs 同纪律）。
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
	if n := len(r.Form["language"]); n > 1 {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldDuplicate, Field: "language", Reason: "同一个字段只能出现一次"}})
		return
	}
	res := s.createProfile(createInput{
		Title: r.FormValue("title"),
		PIN:   r.FormValue("pin"),
		// 建出来的就是**第一个人的书签页**，不是这台盒子的管理员（2026-10-12）。
		// 管这台机器是另一把 key 的事——LAUNCHBAY_ADMIN_TOKEN，写在 compose 里。
		// 所以这一趟不需要"必须有个管理员"的照顾：零档案装完后，想加第二个人
		// 就在管理页拿那把 key 进，而想改自己的书签照旧输 PIN。
		NoPass:    nopassFlag(r.FormValue("nopass")),
		DeriveID:  true,
		FirstOnly: true,
	})
	switch {
	case len(res.FieldErrs) > 0:
		redirectSetupForm(w, setupFieldKey(res.FieldErrs), r.FormValue("title"), false)
	case res.Status == http.StatusConflict:
		redirectPicker(w)
	case res.Permission:
		// 落盘失败里唯一说得出出路的一族（宿主数据目录不可写）：303 回引导页
		// 带白名单键，而不是给一个刚打开浏览器的人一段 500 JSON——这是装机的
		// 头一个故障，页面上那句话正好告诉他下一步在宿主机上。
		redirectSetupForm(w, "permission", r.FormValue("title"), false)
	case res.Status != 0:
		// 其余落盘失败不是权限拒，走 JSON 外壳（spec §6.1 的两种长相）。
		writeInternalError(w, res.Err)
	default:
		// **档案已经建成了**，所以语言写不进去不能把它变成一次失败——那会让人以为
		// 什么都没建成，重新建一遍却撞上「窗口已关」。它是独立的一组话：建好了说
		// 建好了，同时说清是"你选的那个语言不认识"还是"没存上去"。
		//
		// 它自己重定向并说清原因，返回 true；false 才表示没拦下，调用方照常 303。
		// **不用"返回错误码、调用方再 redirectSetup"那个形状**：那样 redirectSetup
		// 的键就变成了一个变量，agents 测试里那道静态闸（每个 redirectSetup 的键
		// 都得是登记过的字面量）就再也拦不住一个没登记的键了。两处失败分开说，
		// 就是这里有两条 redirectSetup 而不是一条的原因。
		if s.applySetupLanguage(w, r.FormValue("language"), r.FormValue("title")) {
			return
		}
		redirectPicker(w)
	}
}

// applySetupLanguage 把引导表单里选的语言落到站级配置上。
//
// 返回 true = 已经拦下并且**已经把要说的话说出去了**（调用方直接 return）；
// 返回 false = 没拦下，调用方照常 303 回选择页。
//
// **两种失败必须分开说**，这是它们各有一次 redirectSetup 而不是合成一条的原因：
// 值不认识说 badlang（"你选的那个语言不存在"，人能换一个），
// 落盘失败说 langfail（"没存上"，人去查数据目录的权限）。
// 合成一个键就把第一种说成第二种，人会去 chown 一个根本没坏的东西。
//
// **给语言留空不是错**：那一格是后加的，老的自动装机脚本与测试夹具都在 POST
// 只带 title 与 pin。空 = 不动，仍是缺省的中文——升级不该让一条本来能用的
// 无 JS 通路突然报字段错。
func (s *Server) applySetupLanguage(w http.ResponseWriter, raw, title string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	l := i18n.Lang(raw)
	if !l.Valid() {
		redirectSetupForm(w, "badlang", title, false)
		return true
	}
	// 写不进去（数据目录不可写）**不阻断建档**：档案已经落盘了，把这一趟说成
	// 全失败比只说语言没设上更糟——那会让人以为机子上什么都没有。
	if err := s.deps.Store.SetLanguage(l); err != nil {
		redirectSetupForm(w, "langfail", title, false)
		return true
	}
	return false
}

// redirectSetup 把浏览器能懂的失败送回引导页。code 只来自本包的常量，永远不是
// 用户可控的字符串——页面那一头还要过 pinNotices 白名单。
func redirectSetup(w http.ResponseWriter, code string) {
	w.Header().Set("Location", "/?err="+code)
	w.WriteHeader(http.StatusSeeOther)
}

// redirectSetupForm 与 redirectSetup 同一条白名单，另外把**刚输入的名字**带回去
// （2026-10-24）：字段错重画时名字被清空，人得从头再打一遍；而 PIN 错了之后
// 只改 PIN、名字却没了的下一发会变成 noname，白跑两趟。`new` 让 `/new` 那一块
// 折着的表单在错误后展开——否则提示在顶上、表单看不见。
//
// **PIN 一个字都不回填**：它是这一页的凭证，不该出现在 URL/历史里。
func redirectSetupForm(w http.ResponseWriter, code, title string, isNew bool) {
	q := url.Values{}
	q.Set("err", code)
	if t := strings.TrimSpace(title); t != "" {
		q.Set("title", t)
	}
	if isNew {
		q.Set("new", "1")
	}
	w.Header().Set("Location", "/?"+q.Encode())
	w.WriteHeader(http.StatusSeeOther)
}

// setupFieldKey 把 createProfile 的字段错翻成 ?err= 的键。引导表单只有两格，所以
// 这是一张小表而不是拼接：id 由名字派生（不问），建档也不接受任何"起步带几条"的东西。
// 名字排在口令前面——两格都没填对时说"要先起个名字"，因为那是这一页的第一格；
// 出现第三种字段说明判据变了，那种情况下宁可说中性的那句。
func setupFieldKey(errs []fieldError) string {
	// **名字太长与名字没给是两句话**（2026-10-23 加）：后果不同（一个是"你没起名"，
	// 一个是"你起得太长"），而 `?err=` 那一格只能印一句。
	//
	// 判据是**码**（apierr.FieldTooLong），不是中文字面量——靠原话做子串匹配去猜，
	// 正是这条通路最容易烂掉的地方（改一个标点，分类就静默错）。
	for _, e := range errs {
		if e.Kind == apierr.FieldTooLong {
			return "titlelong"
		}
	}
	for _, e := range errs {
		if e.Field == "title" {
			return "noname"
		}
	}
	// 「PIN 与『不设 PIN』同给」这一格先于格式错：两者的出路不同——一个是
	// "两个只能给一个"，另一个是"PIN 要 4 到 6 位数字"。字段名把它们分开
	// （hashFormPIN 里那一条记在 "nopass" 上，格式错记在 "pin" 上）。
	for _, e := range errs {
		if e.Field == "nopass" {
			return "pinboth"
		}
	}
	for _, e := range errs {
		if e.Field == "pin" {
			return "badpin"
		}
	}
	return "noname"
}
