package httpapi

import (
	"net/http"

	"github.com/rockbenben/launchbay/internal/apierr"
)

// 这一族是**自建档**：非管理员自己建一份档案（用户 2026-10-23 拍板：
// 「非管理员用户有个 new 接口，可以自己建档案」）。
//
// 三条判据，都是这一族与建档那一族（/setup、管理页、POST /api/v1/profiles）
// **共享**的——落点只有 createProfile 一个，所以校验、哈希、落盘、错误分诊
// 在四条壳上都是同一份代码：
//
//  1. **id 由名字派生，不接受申报**（DeriveID）。这一条是这一族与管理员那条
//     最大的差别：管理员建档要显式给 id（那是机器通路，脚本要能指名），
//     而自建的那一份**不许自己起 id**——否则任何人都能建一份 id 叫 `dad`
//     的档案顶替别人的那一格（目录撞名会被 idTaken 挡住，但"名字一样、
//     id 不一样"这件事本身就够冒充了）。
//     派生走 store.NextProfileID：slug 得动且没被占就用它，否则落到
//     home / home-2 的基名序列。
//  2. **没有 admin 那一格**（档案从 2026-10-12 起就没有管理员属性）。
//     请求体送来 `admin` 是 422 且点名该字段——decodeBody 拒未知键。
//  3. **建完不发会话**（与 /setup 同一条纪律）：免凭证建档不等于免凭证
//     进入。建完照样要输一次 PIN 才进门，而 PIN 是建的人自己刚定的那一个。
//
// **为什么凭证是免的**（这一条最容易被人当成漏洞，所以写清理由）：§11 明写
// 本案不防局域网内冒充——cookie 不签名、PIN 是方便不是认证。而自建档给出的
// 东西**不授予任何权限**：它是一份**空的**档案，只属于它自己的 PIN，没有人的
// 书签会因为它多出一分可读。反过来，"要管理员才能建"意味着这台机器上第一个
// 想用的人必须等家长回来开一次管理页——那正是这个产品要拿掉的形状。
//
// 代价明写：局域网里任何一台设备都能建档，于是选单页上会多出一张卡
// （管理员可以随时删掉它），档案总数没有上限。

// handleNewProfile 是自建档的**机器通路**：POST /api/v1/profiles/new。
//
// 体是 `{title, pin, nopass}`——**没有 id**（不接受申报，见上面的第 1 条）。
// 成功回 201 `{id, title}`，与管理员那条建档同一份外壳（同一个 struct，
// 于是两份契约的键序也一模一样）。
//
// 判据的顺序与管理员那条相反：那条是"先解码（形状错 422），再申报进 require"，
// 这一条**没有 require**（免凭证），所以只有解码与建档两支。
func (s *Server) handleNewProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
		PIN   string `json:"pin"`
		// NoPass 是**明确**选择不设防，与建档那一族同形（2026-10-12）：
		// 它必须是一个独立的布尔而不是「pin 恰好是空串」。
		NoPass bool `json:"nopass"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	res := s.createProfile(createInput{
		Title:    in.Title,
		PIN:      in.PIN,
		NoPass:   in.NoPass,
		DeriveID: true,
	})
	switch {
	case len(res.FieldErrs) > 0:
		writeFieldErrors(w, http.StatusUnprocessableEntity, res.FieldErrs)
	case res.Status == http.StatusConflict:
		// 派生 id 撞上并发提交（两个同名请求算出同一个空位）：与管理员那条
		// 同一句话（"档案已存在"），不是"引导期已过"——引导期已过是 /setup
		// 专有的那一句。
		writeJSON(w, res.Status, errorResponse{Code: res.Status, Kind: apierr.ProfileExists, Detail: res.Detail})
	case res.Status != 0:
		writeInternalError(w, res.Err)
	default:
		writeJSON(w, http.StatusCreated, createProfileResponse{
			ID: res.Profile.ID, Title: res.Profile.Title})
	}
}

// handleNewProfileForm 是自建档的**浏览器通路**：POST /new。
//
// 与 /setup 同一张表单（名字 + PIN +「不设 PIN」那一格），判据同一份
// （hashFormPIN 与 createProfile），差别只有两条：
//   - /setup 是"零档案时的第一个"（CreateFirstProfile 在锁内重查目录数），
//     这一条是"随时可建"（CreateProfile）；
//   - 失败的长相：字段没填对 → 303 带 ?err= 白名单键（引导页是给人看的）；
//     落盘失败 → 500 JSON 外壳，唯独"宿主数据目录不可写"那一族折成
//     ?err=permission（那句话教的是去宿主机 chown，是说得出出路的）。
//
// 成功回 303 到选单页：新卡在那儿，**不发会话**——建完照样要输 PIN 才进门
// （与 /setup 同一条纪律，见上面那份理由）。
func (s *Server) handleNewProfileForm(w http.ResponseWriter, r *http.Request) {
	// 上限必须在这里设：ParseForm 会整段读 body，不套 MaxBytesReader 的话
	// 承诺的 1 MiB 对这条无 JS 通路是假话（handleSetup 与 handlePin 同一教训）。
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity,
			[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "表单读不出来"}})
		return
	}
	// 同名多值不是"取第一个"，是没说清要建哪个（/setup、/admin、/prefs 同一纪律）。
	for _, field := range []string{"title", "pin", "nopass"} {
		if n := len(r.Form[field]); n > 1 {
			writeFieldErrors(w, http.StatusUnprocessableEntity,
				[]fieldError{{Kind: apierr.FieldDuplicate, Field: field, Reason: "同一个字段只能出现一次"}})
			return
		}
	}
	res := s.createProfile(createInput{
		Title:    r.FormValue("title"),
		PIN:      r.FormValue("pin"),
		NoPass:   nopassFlag(r.FormValue("nopass")),
		DeriveID: true,
	})
	switch {
	case len(res.FieldErrs) > 0:
		// redirectSetupForm 回的是选单页（`/?err=`）——这条通路与引导那一条共用
		// 那张词表（setupFieldKey），所以键也共用：noname / badpin / pinboth。
		// `new=1` 让那一块折着的表单展开，名字也一起带回去。
		redirectSetupForm(w, setupFieldKey(res.FieldErrs), r.FormValue("title"), true)
	case res.Status == http.StatusConflict:
		// 派生 id 撞车（两个同名请求算出同一个空位）：引导那条撞车等于
		// "窗口已关"，回 / 不带键；这一条不是窗口，是并发，于是说"这个名字
		// 有人在用"那一句（nametaken 与管理页共用同一把键）。
		redirectSetupForm(w, "nametaken", r.FormValue("title"), true)
	case res.Permission:
		// 宿主数据目录对跑服务的那个用户不可写：这一族是唯一说得出出路的
		// 落盘失败（去宿主机 chown），两张建档表单共用那一句。
		redirectSetupForm(w, "permission", r.FormValue("title"), true)
	case res.Status != 0:
		writeInternalError(w, res.Err)
	default:
		// 建成了。**不发会话**（见上面那份理由）：回选单页，新卡在那儿。
		redirectPicker(w)
	}
}
