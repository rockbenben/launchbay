package httpapi

import (
	"errors"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// usageMaxIDs 是单次上报的上限。前端是节流批量，正常一次十几条；
// 五千条要么是节流坏了，要么是有人在刷计数。
//
// 它只管**每请求**这一刀：usage.json 本身会随时间单调增长（点过的 id
// 永不回收），"总量怎么裁"是留给后面的独立决定，本轮裁定不动它
// （评审 Do-NOT-change 项，别在这里顺手加裁剪）。
const usageMaxIDs = 500

// usageResponse 用 struct 不用 map：键序由字段序决定（§4.3 的写出契约
// 对 API 响应同样成立，batch.go 的 errorResponse 是同一条裁定的先例）。
type usageResponse struct {
	Updated int      `json:"updated"`
	Unknown []string `json:"unknown"`
}

// handleUsage 只写**当前档案**的 usage.json：不参与 rev、不进快照、
// 不进历史（spec §6.1）。它复用的只有原子写，没有书签写通路。
//
// 目标档案的三条来源（"这把凭证能不能动它"永远由 require 裁决，这里
// 只申报）：
//   - 会话：只写 lb_profile（Active）。?profile= 对会话一个字都不认——
//     管理员代管时也不顺手写别人的计数（spec 点名这条）。
//   - 作用域令牌：没申报就是它自己的档案；申报了**按申报进 require**，
//     越界由 require 拒（403）。简报原写法把申报静默改回令牌自己的档案
//     再送检，?profile=kid+tok-dad 会 200 落进 dad——与简报自己的 403
//     用例矛盾，是 plan 缺陷（报告已记账）。
//   - 管理员令牌：?profile= 必填（400 引导）。这是全案唯一"指定要写谁"
//     的通路，跨档案资格同样由 require 判定（Allows("*")）。
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ItemIDs []string `json:"itemIds"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	if len(in.ItemIDs) > usageMaxIDs {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: apierr.FieldInvalid, Field: "itemIds",
			Reason: "一次最多 " + strconv.Itoa(usageMaxIDs) + " 条"}})
		return
	}
	sess := auth.ReadSession(r)
	target := sess.Active
	if tok := bearer(r); tok != "" {
		if s.deps.AdminToken.Matches(tok) {
			// 点击计数是**内容**：它写进某个档案的 clicks。管理员令牌只管账号，
			// 这里必须直说而不是让它落到下面的 require 去拿一个含混的 403。
			writeJSON(w, http.StatusForbidden, errorResponse{Code: 403, Kind: apierr.CredentialScope, Detail: "管理员令牌只管账号；上报点击要用作用域正好是那个档案的令牌"})
			return
		}
		if sc, ok := s.deps.Tokens.Authorize(tok); ok {
			claimed := strings.TrimSpace(r.URL.Query().Get("profile"))
			if claimed != "" && !model.ValidSlug(claimed) {
				writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.FieldInvalid, Detail: "profile 不是 ASCII slug"})
				return
			}
			// 申报了就是它，没申报就是这枚令牌自己的作用域。申报成别的档案时
			// 由 require(credScoped) 收口——它只放行 sc.Allows 的那一个。
			if claimed != "" {
				target = claimed
			} else {
				target = sc.Profile
			}
		}
	}
	if _, _, ok := s.require(r, credScoped, target); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	u, unknown, err := s.deps.Store.AddUsage(target, in.ItemIDs, time.Now().Format(time.RFC3339))
	switch {
	case errors.Is(err, store.ErrNotFound):
		// 档案不存在与"无权限"分开返回（spec §12，batch.go 同一分支形状）。
		// 可达的不只是管理员手打错 id：作用域令牌对应的档案被删了、浏览器
		// 里留着死档案的 lb_profile cookie，都会走到这里。AddUsage 的读排
		// 在写之前，所以这条分支不会造出 profiles/<target>/（评审 Critical
		// 修的就是"200 + 幽灵目录"）。
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.ProfileNotFound, Detail: "档案不存在"})
	case err != nil:
		// 503 由**本次**错误派生（最终整枝评审 #3）：旧写法对任何错误
		// 都查全局坏档登记表，而 usage.json 自己坏了从不 markBroken、
		// 登记表又优先返回 profile 来源——真 I/O 错会被答成 500 之外的
		// "503 + profile.json"（文件与原因都指错），陈旧标记同罪。
		// 文本走 corruptDetail：说的是刚刚真正失败的那个文件；坏 usage
		// .json 现在如实 503（"仍回 500"那句旧注释作废——它不进
		// brokenProfiles[] 的裁定不变，登记表只服务 /status 与全家页面）。
		if errors.Is(err, store.ErrCorrupt) {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Code: 503, Kind: apierr.ProfileBroken, Detail: corruptDetail(s.deps.Store, target, err)})
			return
		}
		writeInternalError(w, err)
	default:
		writeJSON(w, http.StatusOK, usageResponse{Updated: len(u.Items), Unknown: unknown})
	}
}
