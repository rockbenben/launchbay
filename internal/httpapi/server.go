package httpapi

import (
	"net/http"
	"strings"
	"sync"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

// Dependencies 是装配点。Render 必须非 nil：GET / 离开占位页后就靠它。
type Dependencies struct {
	Store  *store.Store
	Tokens auth.Scopes
	Render *render.Renderer
	// Icons 是内嵌的双层图标索引：搜索端点、写路径的图标名闸门、渲染层
	// 与 /status 的缺图统计都读它。真实服务必须非 nil（装配点读不到就退出），
	// 但 `New(Dependencies{})` 仍要能构造——契约测试靠它枚举路由。
	Icons *icons.Index
	// Fetch 是运行时唯一的出网通路，只由 /api/v1/resolve 使用（spec §5）。
	// Task 8 才接上 handler；这里先装配，为的是"唯一通路"从上线第一天起
	// 就是依赖注入的一部分，而不是 handler 里现 new 一个。
	Fetch *icons.Fetcher
	// Cache 是 data/icons/ 的站级图标缓存。/status 的缺图统计读它，写入
	// 只归 resolve（Task 8）。真实服务必须非 nil；契约测试用的
	// `New(Dependencies{})` 允许留空（它不发请求）。
	Cache *store.IconCache
}

type Server struct {
	mux  *http.ServeMux
	deps Dependencies

	// imports 是浏览器收藏夹导入的**未写入**计划暂存（importapi.go）。
	// 它带一个 TTL 与总量上限：导入页面在"看报告"与"确认写入"之间必须把
	// 解析结果留一会儿，而留一份用户上传的字节在内存里就得说清什么时候扔。
	importMu sync.Mutex
	imports  map[string]importStash
}

func New(d Dependencies) *Server {
	s := &Server{mux: http.NewServeMux(), deps: d, imports: map[string]importStash{}}
	for _, rt := range s.routes() {
		s.mux.HandleFunc(rt.Method+" "+rt.Pattern, rt.H)
	}
	return s
}

// Handler 套一层 /api/v1 前缀的 no-store 中间件：§7.1 要求会话相关的
// API 响应一律 `private, no-store`。设在前缀上而不是每个 handler 里——
// P1-② 还要加七八个端点，逐个记得设等于迟早漏一个；错误/404 响应
// 同享，不看状态码。
//
// 为什么在 mux 外面而不用 `s.mux.Handle("/api/v1/", ...)`：Go 的
// ServeMux 会让"不限方法的 /api/v1/ 前缀"与已注册的 "GET /" 兜底
// 冲突（同一路径区、方法数不同 → panic），前缀中间件挂在外层没有
// 这个约束。
func (s *Server) Handler() http.Handler {
	mux := s.mux
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == apiPrefix || strings.HasPrefix(r.URL.Path, apiPrefix+"/") {
			w.Header().Set("Cache-Control", "private, no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

// apiPrefix 是 no-store 的前缀边界。
const apiPrefix = "/api/v1"
