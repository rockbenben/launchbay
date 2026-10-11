package httpapi

import (
	"net/http"

	"github.com/rockbenben/launchbay/internal/auth"
)

// handleSwitch 是浏览器表单通路的会话动作（spec §6.1）。2026-10-10 改判后
// 它只剩两件事：
//
//	to=picker  清 cookie 回选择页（连升级前那条 lb_home 一起清）。
//	to=home    **幂等空操作**。原来它叫"退出管理"（active 写回 home），
//	           那是代管态的残骸：能进 `to=<id>` 的那条路删掉之后，会话里
//	           已经没有什么"两个档案"可言，于是这一支什么也不做。**留着
//	           不删**，是因为升级前发出的页面可能还在 POST 它——拿掉会让那些
//	           页面拿到一个与"没登录"不同的回答。
//
// **`to=<id>` 整条删掉了**（2026-10-10 改判）：它唯一的用途就是让管理员免 PIN
// 直达别人的台去改书签，而"管理员看得见、改得动全家书签"正是本设计要拿掉的
// 形状（与 Nextcloud、Linkding 一致，见
// `2026-10-10-launchbay-admin-boundary-design.md`）。连同它下面那段
// ValidSlug 形状闸一起消失——判据没地方判了，留着是死码。
//
// 同日第二轮：**服务端不再发 `lb_home`**（auth.WriteSession 只写一条），但
// `Session.Valid` 仍然读它——那是认出"升级前发出去的代管 cookie"的唯一办法，
// 读侧不看它就等于把后门重新打开（见 auth/session.go 那段）。
//
// 现在这个处理器每条出路都是 303 回选择页：成功回 "/"，拒也回 "/" 什么也不说。
// 不回 403 页面是给家属看错误页的坏体验，而"这个 id 存不存在"是可枚举信息。
//
// 不写历史行：它不动书签，只清或留会话。与 /prefs、改自己 PIN 同口径，
// 一起记在 spec §15 的"不写审计行的动作"清单里。
func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request) {
	// 外壳取自 redirectPicker（判据一份、长相一份）。
	back := func() { redirectPicker(w) }
	sess := auth.ReadSession(r)
	// credSession 已经把令牌与"没有会话 / cookie 形状非法"两类挡在外面（判据是
	// Session.Valid：只有 lb_profile 是合法形状，升级前的代管 cookie 在那里就被
	// 拒了），所以走到下面 Active 必然非空，不需要再兜"Active 为空"那种够不着的分支。
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		back()
		return
	}
	// 上限必须在这里设：ParseForm 会整段读 body，不套 MaxBytesReader 的话
	// 承诺的 1 MiB 对这条无 JS 通路是假话（handlePin、handlePrefs 同一教训）。
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		back()
		return
	}
	switch r.FormValue("to") {
	case "picker":
		auth.ClearSession(w)
	case "home":
		// 幂等：见函数头那条注释。这里刻意不写任何 cookie ——会话里已经不可能
		// 有"两个档案"可写，在一条空操作里改 cookie 反而是做出一个"我切过档"
		// 的假象。
	}
	// 别的 to（含空串、含 `../` 这类穿越探针、含曾经合法的档案 id）：
	// 一律什么都不做地回选择页。形状不对不是一个新的错误——它与"没这个档案"、
	// "你不是管理员"拿到的是同一个回答，少一种枚举面。
	back()
}
