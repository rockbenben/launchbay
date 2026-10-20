package httpapi

import (
	"errors"
	"fmt"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"os"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
	"github.com/rockbenben/launchbay/internal/store"
)

// pinChangedResponse 是两条 PIN 端点共用的成功体。用 struct 不用 map：
// 键序由字段序决定（§4.3，batch.go 的 errorResponse 是同一条裁定的先例）。
type pinChangedResponse struct {
	Profile    string `json:"profile"`
	PinChanged bool   `json:"pinChanged"`
}

// pinFormatReason 是"这个 PIN 的格式不对"那句话（给人看的那句，不给机器）。
//
// 数字取自 auth 那两个常量：这一句与 ValidPinFormat 说的是**同一件事**，
// 而它是一句写死的字面量时，改了范围就会留下"服务端收 4-8、页面说 4-6"。
//
// 为什么不是直接用 auth.ErrBadPin：那句话是"PIN 要 4-6 位数字"（自带主语，
// 给日志与直接打印用的），而这一句是字段级错误理由——字段名（`pin` / `new`）
// 已经在旁边了，再说一遍"PIN"是啰嗦。两句共用同一份数字，不共用同一句。
func pinFormatReason() string {
	return fmt.Sprintf("要 %d-%d 位数字", auth.PinMinLen, auth.PinMaxLen)
}

// writeProfileReadFailure 是两条 PIN 端点共用的读档分诊，与 handleData /
// handleBatch / handleUsage 同口径（评审 Important 2：同一个条件在哪个端点
// 都只能有一个答案；最终整枝评审 #3：503 的判据是**本次错误**，不是
// 坏档登记表——陈旧记录不许把真 I/O 错答成 503）：
//   - 档案不存在（会话还指着刚被删掉的档案）→ 404；
//   - profile.json 坏了（本次 LoadProfile 报 ErrCorrupt）→ 503 + store
//     给的"文件名 + 第几行"。500 会把"家属把文件改坏了"说成"服务坏了"，
//     调用方拿到 500 只会重试，拿到 503+行号（§12 要求报出文件与错误位置）
//     能直接去修那个文件；
//   - 其余（真 I/O 错）才是 500，原因只进 stderr 不回显磁盘路径。
func (s *Server) writeProfileReadFailure(w http.ResponseWriter, id string, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.ProfileNotFound, Detail: "档案不存在"})
		return
	}
	if errors.Is(err, store.ErrCorrupt) {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Code: 503, Kind: apierr.ProfileBroken, Detail: corruptDetail(s.deps.Store, id, err)})
		return
	}
	writeInternalError(w, err)
}

// wantsNoPin 判「这次要的就是不设防」那一格判据（2026-10-12，2026-10-20 改名）。
//
// **它问的是"新 PIN 留空了吗"**，不是"请求里有没有 nopass 勾选框"——那两个
// 字段从前共用一个名字（`isNoPass`），于是下一个人照名字给 /admin/pin 加一个
// nopass 勾选框并写 `isNoPass(r.FormValue("nopass"))`，空串 → true →
// **没勾也算勾了**，正好是这条判据要防的反面。名字改成它实际回答的问题，
// 勾选框那一格另有 `nopassFlag`（值是 "1"）。
//
// 三处用它（self/pin、admin/pin、建档族的 nopass），所以它住在最小的那个包里；
// 而「这一份档案到底有没有 PIN」是 auth.HasPin。两者别混：**一个是请求，
// 一个是盘上的档案。**
func wantsNoPin(newPin string) bool { return newPin == "" }

// nopassFlag 判「请求说不设防」——表单那个勾选框发的是 `nopass=1`，值对不上就
// 当没勾：宁可把一个写错的标记当成没勾（于是仍然要 PIN），也不要把它当成一个
// 明确的选择。
//
// **两条建档表单都要走它**（/setup 与 /admin 各一处，2026-10-20 接上）：
// 从前这两个 handler 各自内联了一句 `r.FormValue("nopass") == "1"`，而这个
// helper 一个调用者也没有——判据于是有**三份**（两份内联 + 一份死的），
// 改可接受的值要记得改两处代码。
func nopassFlag(v string) bool { return v == "1" }

// selfPinResult 是"改自己这道门"的结论。
//
// 六个字段各说各的事：Changed 是**有效的那把 PIN 变了没有**（设上 / 撤下 /
// 换成另一个，2026-10-20 加的——它原先恒为 true，等于没说）；BadOld 是
// **证明没给对**（403），BadNew 是**新值不能用**（422），HashErr 是哈希那一步
// 出了错，ReadErr 是档案读不出来，SaveErr 是落盘失败。
type selfPinResult struct {
	Changed bool
	BadOld  bool
	BadNew  bool
	HashErr error
	ReadErr error
	SaveErr error
}

// selfPin 是改自己 PIN 的**唯一落点**：API 端点（`/api/v1/profiles/self/pin`）
// 与台面面板里那张表单（`/prefs/pin`）都走它。
//
// **两个壳不许各写一遍判据**（spec §8.8 对设 PIN 的同一句话）：这一族的判据有
// 五处（旧的要不要、新的格式、空串是不是取消、取消要不要旧 PIN、谁有资格改），
// 抄一遍就是抄错一遍。
//
// newPin 为空 = **取消这一份的 PIN**（"留空"是这个动作本身）；newPin 非空而格式
// 错 = BadNew（422）。**没有"没给"这一种**：那个由调用方在进落点之前就挡住
// （API 那条：字段缺席 → 422 点名 new）。
func (s *Server) selfPin(id, old, newPin string) selfPinResult {
	var res selfPinResult
	// 读→验→写整段进 WithProfileLock（评审 Important 1）：SaveProfile 不
	// 自带锁，裸 RMW 与并发的管理员设 PIN 撞车时最坏是这样——管理员先写新
	// 哈希并追加"dad 给 kid 设了 PIN"的审计行，随后本人这条路径拿陈旧副本
	// 把整个文件盖回去：盘上一个 PIN，目标档案的历史里另一句，那条幽灵
	// 补偿行正是管理员端点存在的意义。锁把同档案的 RMW 串起来。
	err := s.deps.Store.WithProfileLock(id, func() error {
		p, err := s.deps.Store.LoadProfile(id)
		if err != nil {
			res.ReadErr = err
			return nil // 分诊在锁外做：404 / 503 / 500
		}
		// 旧 PIN：**这一份本来就没有 PIN 时不核对**（2026-10-12）。没有的东西不需要
		// 证明，所以一份不设防的档案可以直接给自己设上第一个 PIN——而如果这里仍
		// 要求 old，人就永远关不掉再打开这道门。判据是 `auth.HasPin`，它问的是
		// 盘上这份档案的形状，与"调用方填了什么"无关。
		//
		// `new` 留空 = **取消**这一份的 PIN。它与建档时的 `nopass` 是同一件事的
		// 另一个方向：能设就能撤，否则"不设防"一旦选了就再也回不来。
		if auth.HasPin(p.PIN) && !auth.VerifyPIN(p.PIN, old) {
			res.BadOld = true
			return nil
		}
		before := p.PIN
		hash := ""
		if !wantsNoPin(newPin) {
			if !auth.ValidPinFormat(newPin) {
				res.BadNew = true
				return nil
			}
			h, err := auth.HashPIN(newPin)
			if err != nil {
				res.HashErr = err
				return nil
			}
			hash = h
		}
		// **pinChanged 真算**（2026-10-20）：这个字段名承诺"变了没有"，而从前
		// 恒写 true——对一份本来就没有 PIN 的档案发 `{"new":""}` 也答"变了"，
		// 于是按它判断"要不要提示重新输 PIN"的调用方会误判。
		//
		// 判据是**有效的 PIN 有没有变**：拿新值去核对盘上那一个。VerifyPIN 对空
		// 哈希恒假，所以"从没设过"与"刚设上"天然分得开；而"设成同一个 PIN"
		// 算没变（盐换了一个，有效的 PIN 是同一个）。
		if hash == "" {
			res.Changed = auth.HasPin(before) // 撤锁：本来锁着才算变了
		} else {
			res.Changed = !auth.VerifyPIN(before, newPin)
		}
		p.PIN = hash
		res.SaveErr = s.deps.Store.SaveProfile(p)
		return nil
	})
	if err != nil {
		res.ReadErr = err
	}
	return res
}

// handleSelfPin 改自己的 PIN：要旧 PIN，走会话，令牌一律拒。
//
// 为什么不记进 history：§4.6 的审计线记的是"谁动了我的书签"，改 PIN
// 不动书签、不动 rev。管理员给别人设 PIN 是另一回事——那是**替别人**
// 做决定，必须出现在那个档案自己的历史里（spec §6.1）。
func (s *Server) handleSelfPin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Old string `json:"old"`
		// New 是**指针**：缺席与"给了空串"是两件事，而这两件事的后果正好相反
		// （见下面那道 422）。值类型分不出这两者，于是"漏了这个字段"会被当成
		// "明确要求取消这道门"——一道门就这么被拆了，而调用方拿到的是 200。
		New *string `json:"new"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	// **缺 new 不是"取消"**（2026-10-20 修）。契约写着 new 必填，而"取消这道门"
	// 是一个必须被**说出口**的决定——与建档那族的 nopass 同一件事的两个说法
	// （AGENTS.md：「不要这道门必须是一个被说出来的决定」）。
	// 从前这里收的是普通 string，缺席与空串同形，于是：
	//   POST /api/v1/profiles/self/pin  {"old":"1234"}
	// 把这一份的门拆了，答 200 + pinChanged:true（实测复现）。
	// 而漏字段的脚本、拼错字段名的脚本、字段名写反的脚本——它们要的是
	// "改 PIN"，不是"拆门"，而服务端替它们把门拆了。
	if in.New == nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.FieldInvalid, Field: "new", Reason: "不能省掉 new：取消这道门就写空串"}})
		return
	}
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	res := s.selfPin(sess.Active, in.Old, *in.New)
	switch {
	case res.BadOld:
		// **自己的旧 PIN 不对**用自己那一个 kind（2026-10-20）：它与"进门时
		// PIN 不对"（pin-wrong，401）**出路相反**——这一件要去管理页改，而那一件
		// 是再输一次。从前它们共用一个码，而那一个在登记表里是 401、在这一条
		// 端点上答的却是 403。
		writeJSON(w, http.StatusForbidden, errorResponse{Code: 403, Kind: apierr.PinOldWrong, Detail: "旧 PIN 不对"})
	case res.BadNew:
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: apierr.FieldInvalid, Field: "new", Reason: pinFormatReason()}})
	case res.HashErr != nil:
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: kindOf(res.HashErr, apierr.FieldInvalid), Field: "new", Reason: res.HashErr.Error()}})
	case res.ReadErr != nil:
		s.writeProfileReadFailure(w, sess.Active, res.ReadErr)
	case res.SaveErr != nil:
		writeInternalError(w, res.SaveErr)
	default:
		writeJSON(w, http.StatusOK, pinChangedResponse{Profile: sess.Active, PinChanged: res.Changed})
	}
}

// adminPinResult 是"给任意档案设 PIN"这个动作的结论。
//
// 五个出口各自有话可说，而**把它们并成一个 error 会丢信息**：ReadErr 是档案
// 读不出来（404/503/500 要分开答），SaveErr 是落盘失败，HistErr 是审计行没写上
// ——最后这个尤其不能被前两个盖住，"PIN 改了但历史里没有"只有它自己在说。
type adminPinResult struct {
	FieldErrs  []fieldError
	ReadErr    error
	SaveErr    error
	HistErr    error
	PinChanged bool
}

func (r adminPinResult) ok() bool {
	return len(r.FieldErrs) == 0 && r.ReadErr == nil && r.SaveErr == nil && r.HistErr == nil
}

// adminPin 是这条动作的**唯一落点**：鉴权 → 校验 → 哈希 → 锁内读写 → 历史。
//
// API 端点（handleAdminPin）与管理页的表单端点（/admin/pin）都走它——
// spec §8.8 明写"两个壳各自不许再写一遍判据"。判据分家之后的形状很具体：
// 表单那条忘了写"要 4-6 位数字"，于是页面上能设一个 1 位 PIN，而 API 那条
// 拒着；两个入口对同一件事给出不同答案，没人知道哪个是真的。
//
// **鉴权排在参数校验之前**（2026-10-20 修）。从前是 slug → 格式 → require，
// 于是同一个"你没这个权限"的事实，答案随载荷变：格式对的请求答 401/403，
// 格式错的请求答 422（"要 4-6 位数字"）。那等于给没资格问的人开了一个
// "这个 id 是不是 slug / 这个 PIN 格式对不对"的小 oracle，也让"401 与 403
// 永远分开"这条纪律在同一条端点上自相矛盾。兄弟端点的顺序是同一份：
// rebuildapi（require 在前）、createProfile（decode → require → 字段校验）。
// 顺序是判据的一部分，不该三处三个样。
func (s *Server) adminPin(r *http.Request, profile string, newPin *string) adminPinResult {
	var res adminPinResult
	actor, _, ok := s.require(r, credAdmin, profile)
	if !ok {
		// 权限类拒绝走浏览器那条 303（credAdmin 带 credSession），所以这里
		// 不能自成一个 FieldErr——它会让页面显示"PIN 格式不对"，而真相是
		// "你不该看见这一页"。由调用方在 !ok 时直接答。
		res.FieldErrs = []fieldError{{Kind: apierr.CredentialAdminOnly, Field: "cred", Reason: "需要管理员 cookie"}}
		return res
	}
	// **缺 new 不是"取消"**（2026-10-20 修，与 self/pin 同一件事）。契约写着
	// new 必填，而"取消这道门"是一个必须被**说出口**的决定——AGENTS.md 那条
	// 「不要这道门必须是一个被说出来的决定」管的就是这件事。
	// 从前这里收的是普通 string，缺席与空串同形，于是
	// `POST /api/v1/profiles/pin {"profile":"kid"}` 把 kid 的门拆了、答 200。
	if newPin == nil {
		res.FieldErrs = []fieldError{{Kind: apierr.FieldInvalid, Field: "new", Reason: "不能省掉 new：取消这道门就写空串"}}
		return res
	}
	if !model.ValidSlug(profile) {
		res.FieldErrs = []fieldError{{Kind: apierr.FieldInvalid, Field: "profile", Reason: "必须是 ASCII slug"}}
		return res
	}
	// newPin 留空 = **取消**这一份的 PIN（2026-10-12）。管理员是唯一能给别人
	// 去/留这道门的人——本人自己那边（self/pin）同样能撤，但一个都不设防的家族
	// 里，管理员得能把门重新锁上。判据是 `wantsNoPin` 一处，表单与 API 共用。
	hash := ""
	if !wantsNoPin(*newPin) {
		if !auth.ValidPinFormat(*newPin) {
			res.FieldErrs = []fieldError{{Kind: apierr.FieldInvalid, Field: "new", Reason: pinFormatReason()}}
			return res
		}
		h, err := auth.HashPIN(*newPin)
		if err != nil {
			res.FieldErrs = []fieldError{{Kind: kindOf(err, apierr.FieldInvalid), Field: "new", Reason: err.Error()}}
			return res
		}
		hash = h
	}
	err := s.deps.Store.WithProfileLock(profile, func() error {
		p, err := s.deps.Store.LoadProfile(profile)
		if err != nil {
			res.ReadErr = err // 分诊在锁外做：404 / 503 / 500
			return err
		}
		before := p.PIN
		p.PIN = hash
		// **pinChanged 真算**（2026-10-20）：这个字段名承诺"变了没有"，而从前
		// 恒写 true——对一份本来就没有 PIN 的档案发 `{"new":""}` 也答"变了"。
		// 判据与 self/pin 那一处同一份：拿新值去核对盘上那一个。
		if hash == "" {
			res.PinChanged = auth.HasPin(before) // 撤锁：本来锁着才算变了
		} else {
			res.PinChanged = !auth.VerifyPIN(before, *newPin)
		}
		res.SaveErr = s.deps.Store.SaveProfile(p)
		if res.SaveErr != nil {
			return nil
		}
		// Rev: 0 是有意的：rev 是书签计数器，改 PIN 不产生新 rev，历史里
		// 出现 rev:0 的行不是书签改动——/history 的调用方（P1-④ 的界面）
		// 要按这条渲染。path 用 profile/pin 而不是 groups/...：动的不是
		// 书签树。
		res.HistErr = s.deps.Store.AppendHistory(store.HistoryEntry{
			At: time.Now().Format(time.RFC3339), Profile: profile, Actor: actor,
			By: byOfRequest(r), Rev: 0,
			Ops: historyOps([]ops.Brief{{Op: "set", Path: "profile/pin"}}),
		})
		return nil
	})
	// WithProfileLock 自己出的错一律记在 ReadErr 里：这是锁本身的问题（多半是
	// profile 目录没了），而 res 里另外三格各自记着自己那一件事——不分开的话
	// 一个锁错误会被读成"PIN 没改成"，于是调用方给出成功。
	if err != nil {
		res.ReadErr = err
	}
	return res
}

// writeAdminPin 把落点的结论按 API 形状答出去。
func (s *Server) writeAdminPin(w http.ResponseWriter, r *http.Request, profile string, res adminPinResult) {
	switch {
	case len(res.FieldErrs) > 0:
		writeFieldErrors(w, http.StatusUnprocessableEntity, res.FieldErrs)
	case res.ReadErr != nil:
		s.writeProfileReadFailure(w, profile, res.ReadErr)
	case res.SaveErr != nil:
		writeInternalError(w, res.SaveErr)
	case res.HistErr != nil:
		// 裁定 C：这里**故意**与 batch/revert 相反。那两个端点遇到历史追加
		// 失败照报 200 + warning，为的是 5xx 会诱使调用方重试、把同一批 ops
		// 再应用一次——那才是真数据事故。设 PIN 是幂等的：重试写回同一个
		// 哈希、把缺的那行补上，收敛而不是叠加，所以那条理由在这里不成立，
		// 而"PIN 已改、审计行没落"必须让调用方知道（500 是唯一能说的）。
		// 别把它"修"成与 batch 一致——那会把审计线的缺口藏进 200 里。
		fmt.Fprintf(os.Stderr, "pin history append failed profile=%s: %v\n", profile, res.HistErr)
		writeInternalError(w, res.HistErr)
	default:
		writeJSON(w, http.StatusOK, pinChangedResponse{Profile: profile, PinChanged: res.PinChanged})
	}
}

// handleAdminPin 给任意档案设 PIN，不要旧 PIN（家人忘了 PIN 的出路），
// 仅管理员，且**在目标档案的历史里留名**。
func (s *Server) handleAdminPin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Profile string `json:"profile"`
		// New 是**指针**：缺席与"给了空串"是两件事，而这两件事的后果正好相反
		// （见 adminPin 里那道 422）。值类型分不出这两者。
		New *string `json:"new"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	res := s.adminPin(r, in.Profile, in.New)
	if len(res.FieldErrs) > 0 && res.FieldErrs[0].Field == "cred" {
		s.writeAuthFailure(w, r)
		return
	}
	s.writeAdminPin(w, r, in.Profile, res)
}
