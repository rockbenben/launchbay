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

// isNoPass 是「这次要的就是不设防」那一格判据（2026-10-12）。
//
// **只有空串算**：表单那个勾选框发的是 `nopass=1`，值对不上就当没勾——宁可把一个
// 写错的标记当成没勾（于是仍然要 PIN），也不要把它当成一个明确的选择。
//
// 三处用它（建档族、self/pin、admin/pin），所以它住在最小的那个包里；而「这一份
// 档案到底有没有 PIN」是 auth.HasPin。两者别混：**一个是请求，一个是盘上的档案。**
func isNoPass(v string) bool { return v == "" }

// handleSelfPin 改自己的 PIN：要旧 PIN，走会话，令牌一律拒。
//
// 为什么不记进 history：§4.6 的审计线记的是"谁动了我的书签"，改 PIN
// 不动书签、不动 rev。管理员给别人设 PIN 是另一回事——那是**替别人**
// 做决定，必须出现在那个档案自己的历史里（spec §6.1）。
func (s *Server) handleSelfPin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	sess := auth.ReadSession(r)
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	id := sess.Active
	// 读→验→写整段进 WithProfileLock（评审 Important 1）：SaveProfile 不
	// 自带锁，裸 RMW 与并发的管理员设 PIN 撞车时最坏是这样——管理员先写新
	// 哈希并追加"dad 给 kid 设了 PIN"的审计行，随后本人这条路径拿陈旧副本
	// 把整个文件盖回去：盘上一个 PIN，目标档案的历史里另一句，那条幽灵
	// 补偿行正是管理员端点存在的意义。锁把同档案的 RMW 串起来。
	var (
		badOld, badNew bool
		hashErr        error
		saveErr        error
	)
	err := s.deps.Store.WithProfileLock(id, func() error {
		p, err := s.deps.Store.LoadProfile(id)
		if err != nil {
			return err // 分诊在锁外做：404 / 503 / 500
		}
		// 旧 PIN：**这一份本来就没有 PIN 时不核对**（2026-10-12）。没有的东西不需要
		// 证明，所以一份不设防的档案可以直接给自己设上第一个 PIN——而如果这里仍
		// 要求 old，人就永远关不掉再打开这道门。判据是 `auth.HasPin`，它问的是
		// 盘上这份档案的形状，与"调用方填了什么"无关。
		//
		// `new` 留空 = **取消**这一份的 PIN。它与建档时的 `nopass` 是同一件事的
		// 另一个方向：能设就能撤，否则"不设防"一旦选了就再也回不来。
		if auth.HasPin(p.PIN) && !auth.VerifyPIN(p.PIN, in.Old) {
			badOld = true
			return nil
		}
		hash := ""
		if !isNoPass(in.New) {
			if !auth.ValidPinFormat(in.New) {
				badNew = true
				return nil
			}
			h, err := auth.HashPIN(in.New)
			if err != nil {
				hashErr = err
				return nil
			}
			hash = h
		}
		p.PIN = hash
		saveErr = s.deps.Store.SaveProfile(p)
		return nil
	})
	if err != nil {
		s.writeProfileReadFailure(w, id, err)
		return
	}
	switch {
	case badOld:
		writeJSON(w, http.StatusForbidden, errorResponse{Code: 403, Kind: apierr.PinWrong, Detail: "旧 PIN 不对"})
	case badNew:
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: apierr.FieldInvalid, Field: "new", Reason: "要 4-6 位数字"}})
	case hashErr != nil:
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: kindOf(hashErr, apierr.FieldInvalid), Field: "new", Reason: hashErr.Error()}})
	case saveErr != nil:
		writeInternalError(w, saveErr)
	default:
		writeJSON(w, http.StatusOK, pinChangedResponse{Profile: id, PinChanged: true})
	}
}

// adminPinResult 是"给任意档案设 PIN"这个动作的结论。
//
// 四个出口各自有话可说，而**把它们并成一个 error 会丢信息**：ReadErr 是档案
// 读不出来（404/503/500 要分开答），SaveErr 是落盘失败，HistErr 是审计行没写上
// ——最后这个尤其不能被前两个盖住，"PIN 改了但历史里没有"只有它自己在说。
type adminPinResult struct {
	FieldErrs []fieldError
	ReadErr   error
	SaveErr   error
	HistErr   error
}

func (r adminPinResult) ok() bool {
	return len(r.FieldErrs) == 0 && r.ReadErr == nil && r.SaveErr == nil && r.HistErr == nil
}

// adminPin 是这条动作的**唯一落点**：校验 → 哈希 → 锁内读写 → 历史。
//
// API 端点（handleAdminPin）与管理页的表单端点（/admin/pin）都走它——
// spec §8.8 明写"两个壳各自不许再写一遍判据"。判据分家之后的形状很具体：
// 表单那条忘了写"要 4-6 位数字"，于是页面上能设一个 1 位 PIN，而 API 那条
// 拒着；两个入口对同一件事给出不同答案，没人知道哪个是真的。
func (s *Server) adminPin(r *http.Request, profile, newPin string) adminPinResult {
	var res adminPinResult
	if !model.ValidSlug(profile) {
		res.FieldErrs = []fieldError{{Kind: apierr.FieldInvalid, Field: "profile", Reason: "必须是 ASCII slug"}}
		return res
	}
	// newPin 留空 = **取消**这一份的 PIN（2026-10-12）。管理员是唯一能给别人
	// 去/留这道门的人——本人自己那边（self/pin）同样能撤，但一个都不设防的家族
	// 里，管理员得能把门重新锁上。判据是 `isNoPass` 一处，表单与 API 共用。
	hash := ""
	if !isNoPass(newPin) {
		if !auth.ValidPinFormat(newPin) {
			res.FieldErrs = []fieldError{{Kind: apierr.FieldInvalid, Field: "new", Reason: "要 4-6 位数字"}}
			return res
		}
		h, err := auth.HashPIN(newPin)
		if err != nil {
			res.FieldErrs = []fieldError{{Kind: kindOf(err, apierr.FieldInvalid), Field: "new", Reason: err.Error()}}
			return res
		}
		hash = h
	}
	actor, _, ok := s.require(r, credAdmin, profile)
	if !ok {
		// 权限类拒绝走浏览器那条 303（credAdmin 带 credSession），所以这里
		// 不能自成一个 FieldErr——它会让页面显示"PIN 格式不对"，而真相是
		// "你不该看见这一页"。由调用方在 !ok 时直接答。
		res.FieldErrs = []fieldError{{Kind: apierr.CredentialAdminOnly, Field: "cred", Reason: "需要管理员 cookie"}}
		return res
	}
	// 读也搬进锁内（评审 Important 1）：以前只有写在里面，LoadProfile 在锁外
	// ——同一目标两次管理员设 PIN 会各自拿到加锁之前的副本，后写的那份陈旧
	// 副本把先写的哈希连同它的审计行一起盖回去，幽灵历史行的形状与 self
	// 路径一模一样。锁内读还顺带挡住"边读边被人改档"的错值（PIN 之外被改
	// 的字段不会在这一版里丢）。
	err := s.deps.Store.WithProfileLock(profile, func() error {
		p, err := s.deps.Store.LoadProfile(profile)
		if err != nil {
			res.ReadErr = err // 分诊在锁外做：404 / 503 / 500
			return err
		}
		p.PIN = hash
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
		writeJSON(w, http.StatusOK, pinChangedResponse{Profile: profile, PinChanged: true})
	}
}

// handleAdminPin 给任意档案设 PIN，不要旧 PIN（家人忘了 PIN 的出路），
// 仅管理员，且**在目标档案的历史里留名**。
func (s *Server) handleAdminPin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Profile string `json:"profile"`
		New     string `json:"new"`
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
