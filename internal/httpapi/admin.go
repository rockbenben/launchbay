package httpapi

import (
	"errors"
	"github.com/rockbenben/launchbay/internal/apierr"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// statusProfile / statusTotals / statusResponse / profileMeta /
// listProfilesResponse / deleteProfileResponse 一律用 struct 而不是
// map[string]any：响应体键序由字段序固定（§4.3 的写出契约对 API 响应
// 同样成立，AI 会 diff 它），同一条 endpoint 的多种返回也不会漂成两套形状。
type statusProfile struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Disabled      bool   `json:"disabled"` // 停用：数据不动、进不来，仍占 CountProfiles
	Rev           int64  `json:"rev"`
	Modified      string `json:"modified"` // RFC3339 UTC；坏档/读不出来 = 空串，不编造
	Items         int    `json:"items"`
	IconUnmatched int    `json:"iconUnmatched"`
	Snapshots     int    `json:"snapshots"`
	Broken        bool   `json:"broken"`
	// PinSet = 这一份**现在有 PIN 吗**（判据只有 auth.HasPin 一处）。
	//
	// **它不进 JSON**（`json:"-"`）：/api/v1/status 是机器那一面的契约，
	// 里面没有这一格也不影响谁；管理页要它，是为了说清"这一份还没锁上"
	//（不设防的档案在局域网里谁都能进）。两个壳读同一份聚合，但各自的
	// 视图形状不同——这一格只属于页面那一面。
	PinSet bool `json:"-"`
}

type statusTotals struct {
	Profiles      int `json:"profiles"`
	Items         int `json:"items"`
	IconUnmatched int `json:"iconUnmatched"`
	Snapshots     int `json:"snapshots"`
}

type statusResponse struct {
	Profiles       []statusProfile `json:"profiles"`
	BrokenProfiles []string        `json:"brokenProfiles"`
	Totals         statusTotals    `json:"totals"`
}

// profileMeta **没有 admin 字段**（2026-10-12）：档案不再有管理员属性，
// "列档案"里就不该有一格回答"这个是不是管理员"。
type profileMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
}

type listProfilesResponse struct {
	Profiles []profileMeta `json:"profiles"`
}

type deleteProfileResponse struct {
	Deleted string `json:"deleted"`
}

// handleStatus 是跨档案聚合信息，只给管理员（spec §6.1）。档案作用域的
// 令牌拿它会 403——AGENTS.md 要专门写明，否则 AI 会当成 bug 去"修"。
// 遍历本身在 statusRows：这一层只负责鉴权与写响应。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	rows, broken, tot, err := s.statusRows()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Profiles: rows, BrokenProfiles: broken, Totals: tot})
}

// statusRows 是"一行一个档案"这份聚合的唯一实现（spec §12）：
// GET /api/v1/status 的 JSON 与 GET /admin 的表格都从它出来。分两处各写一遍
// 遍历，"ListProfiles ∪ Broken()"这个并集迟早只在一处修——而漏掉的那一处
// 表现为某个坏档案整行消失，正是 §12 点名的形状。
//
// 它不写响应：错误一律往上抛，由调用方决定是 500 还是页面上那句人话。
//
// 行集是「ListProfiles 列出的档案 ∪ Broken() 登记在册的档案」：profile.json
// 坏掉的档案根本进不了 ListProfiles（load.go 会 continue 掉它），只遍历
// ListProfiles 就会让它整行消失——spec §12 明写「坏档案仍要在列表里占一行，
// 消失比报 0 更坏」。坏到列不出来的档案：id 取自坏档表，title 尽力而为
// （读不出就是空串，绝不编造），rev/items 归 0，snapshots 仍从磁盘数。
func (s *Server) statusRows() ([]statusProfile, []string, statusTotals, error) {
	profiles, err := s.deps.Store.ListProfiles()
	if err != nil {
		return nil, nil, statusTotals{}, err
	}
	byID := make(map[string]model.Profile, len(profiles))
	ids := make([]string, 0, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = *p
		ids = append(ids, p.ID)
	}
	// 并上坏档登记表：ListProfiles 里被 continue 掉的档案只活在这里。
	// ListProfiles 自身会调 LoadProfile 触发登记，所以这一步一定看到最新状态。
	for _, id := range s.deps.Store.Broken() {
		if _, listed := byID[id]; !listed {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	out := make([]statusProfile, 0, len(ids))
	broken := []string{}
	tot := statusTotals{Profiles: len(ids)}
	for _, id := range ids {
		p, listed := byID[id]
		row := statusProfile{ID: id, Title: p.Title, Disabled: p.Disabled}
		if listed {
			// 这一份有没有门锁：判据只有 auth.HasPin 一处（空哈希 = 没设 PIN）。
			// 坏档读不出 profile.json，这一格自然是 false——页面那一侧知道
			// 那一行是坏档，所以不会拿它说"没设防"。
			row.PinSet = auth.HasPin(p.PIN)
			snap, err := s.deps.Store.Current(id)
			switch {
			case err == nil:
				row.Rev = snap.Rev
				row.Modified = snap.Modified.UTC().Format(time.RFC3339)
				for _, g := range snap.B.Groups {
					row.Items += len(g.Items)
				}
				// 与 Items 同一趟遍历口径：只数这个档案自己书签里的条目。
				row.IconUnmatched = countIconUnmatched(snap.B, s.deps.Icons, s.deps.Cache)
			default:
				// 坏档案仍然占一行，rev/items/modified 留 0/空：消失比报 0 更坏。
				if _, isBroken := s.deps.Store.BrokenReason(id); isBroken {
					row.Broken = true
				} else if !errors.Is(err, store.ErrNotFound) {
					return nil, nil, statusTotals{}, err
				}
			}
		} else {
			// 只在坏档表里出现（profile.json 坏到列不出来）：直接标坏，
			// title/rev/items 留空/0，不编造。
			row.Broken = true
		}
		if row.Broken {
			broken = append(broken, id)
		}
		row.Snapshots = s.deps.Store.SnapshotCount(id)
		tot.Items += row.Items
		tot.IconUnmatched += row.IconUnmatched
		tot.Snapshots += row.Snapshots
		out = append(out, row)
	}
	return out, broken, tot, nil
}

// iconUnmatched 数的是"看着像图标名、索引里却没有"的条目，加上"写了 auto
// 但缓存里没图"的条目。空 icon 不进这个数：它是"没设"，在 GET /api/v1/data
// 里一眼可见，不需要计数器提醒（spec §5）。
//
// 读时算、不记盘：缺图是索引与数据的差集，索引一刷新数字就该跟着变。
// 把它记成事件会让 GET / 产生写盘副作用，而且旧记录会变成谎话。
func countIconUnmatched(b *model.Bookmarks, ix *icons.Index, fc *store.IconCache) int {
	if ix == nil || fc == nil {
		// 没装索引/缓存就没有"缺图"可言：拿空依赖去判会把整个站统计成 0，
		// 而真实服务永远两个都有（装配点读不到就退出）。
		return 0
	}
	n := 0
	for _, g := range b.Groups {
		for _, it := range g.Items {
			switch {
			case it.Icon == "":
			case it.Icon == "auto":
				o := icons.Origin(it.URL)
				if o == "" {
					n++ // chrome:// 这类协议没有可抓的图，如实算缺
					continue
				}
				if _, ok := fc.Lookup(o); !ok {
					n++
				}
			case !ix.Has(it.Icon):
				n++
			}
		}
	}
	return n
}

// handleListProfiles 只给档案元信息，条目/URL/分组名一个都不漏：
// 列表端点不是书签的第二个读口（spec §6.1 / Task 8 用例的 banned 列表）。
func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	profiles, err := s.deps.Store.ListProfiles()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	out := make([]profileMeta, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, profileMeta{ID: p.ID, Title: p.Title, CreatedAt: p.CreatedAt})
	}
	writeJSON(w, http.StatusOK, listProfilesResponse{Profiles: out})
}

// handleDeleteProfile 是删档的**机器通路**（浏览器那一条是 `POST /admin/delete`，
// 两步走、落点同一份 store.DeleteProfile），且三道闸
// （ASCII slug / 目录须是 profilesDir 下的同名直接子目录 / 档案写锁）全在
// store.DeleteProfile 内部——端点只负责鉴权与形状转换，绝不在它这里再
// "规范化"id（那等于绕过闸）。
//
// **凭证是 credAdminKey 而不是 credAdmin**（2026-10-23，用户拍板「删除需要
// 有确认警示，并要求密码」）：删档是这台机器上唯一**不可逆**的动作（快照跟着
// 档案一起没了，`revert` 救不了它），而 `lb_admin` 是那把机器级 key 的派生值——
// 它进 devtools、进导出、进浏览器同步。于是这一条要**那把 key 本身**
// （Authorization: Bearer），只带管理 cookie 一律 403。
//
// 顺序是"先鉴权后查存在"：作用域外的令牌问档案在不在，本身就是信息泄漏，
// 所以 dadTok 删 kid 该 403，不是 404（Task 6/7 同口径，用例钉住）。
//
// **鉴权排在形状校验之前**（2026-10-23 修，与同族的 `POST /api/v1/profiles/pin`
// 同一纪律，2026-10-20 那次定的）：没带凭证的人问"这个 id 合不合法"，答案不该
// 随载荷变——从前是 400（形状错），带对了凭证时同一个请求却是 401。
// 于是顺序是**凭证 → 形状 → 存在性**。
func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("profile"))
	if _, _, ok := s.require(r, credAdminKey, id); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	if id == "" || !model.ValidSlug(id) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Code: 400, Kind: apierr.QueryInvalid, Detail: "要带 ?profile=<ASCII slug>"})
		return
	}
	err := s.deps.Store.DeleteProfile(id)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, deleteProfileResponse{Deleted: id})
	case errors.Is(err, os.ErrNotExist):
		// store 的"档案不存在"错误同时满足 os.ErrNotExist 与 store.ErrNotFound，
		// 端点用 errors.Is(err, os.ErrNotExist) 分 404——判"文件不存在"一律走
		// errors.Is，被 fmt.Errorf 包装也认得（与 store.ErrCorrupt 等分诊点同口径）。
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.ProfileNotFound, Detail: "档案不存在"})
	default:
		writeInternalError(w, err)
	}
}
