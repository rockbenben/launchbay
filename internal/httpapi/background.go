package httpapi

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/rockbenben/launchbay/internal/apierr"
	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 这一页自己的背景图（2026-10-14）。三条判据长在这一整个文件里，别处各抄一遍
// 就会有一处先漂：
//
//  1. **谁在用**：只有会话能取（credSession），且取的是**会话所指那一份**的目录。
//     与 /icons/{name} 那条公开通路相反——favicon 是任何站都有的图标，而背景图
//     是家里人自己放的图（很可能是孩子的照片）。§11 的承诺是"各人一份，别人
//     看不见"，而"64 位哈希 URL 当秘密"我们没必要顺手造出来：**按档案分目录**
//     让 A 的会话连 B 的那张图都读不到，哪怕他知道那个哈希。
//  2. **形状即边界**：名字必须是 `<16hex>.<png|jpg|webp|gif>`（model.ValidBackground），
//     形状非法与盘上没有是同一句 404（这里不是诊断通路）。
//  3. **字节按字节端出去**：不改写不重编码，immutable + nosniff + CSP sandbox
//     （sandbox 那一条与 favicon 通路同一个理由：谁把 URL 直接开进地址栏，
//     响应就是他自己的一个文档）。
//
// 写侧是 `POST /prefs/background`（表单，带文件），与 `/prefs`、`/prefs/group`
// 同一族：零 JS、面板是服务端画的而改设置不是。它**不写历史行**——与那四条
// 不写审计线的台面动作同类（背景不是书签，不产生 rev 与快照）。

// maxBackgroundBodyBytes 给 multipart 留出信封余量：文件本身的上限是
// store.MaxBackgroundBytes，而 ParseMultipartForm 会把**整段 body** 读进来，
// 所以这层必须比文件上限大一点，否则一个刚好 4 MiB 的合法文件会因为几百字节
// 的 multipart 头被拒——而症状是"明明没超限却说超了"。
const maxBackgroundBodyBytes = store.MaxBackgroundBytes + (1 << 16)

// backgroundContentTypes 按后缀给类型。与 model.ValidBackground 的正则、
// store.BackgroundExts 是同一件事的三处表述（与 iconExts 那"四处"同一条纪律，
// 这里后缀表收窄了）。
var backgroundContentTypes = map[string]string{
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"webp": "image/webp",
	"gif":  "image/gif",
}

// backgroundMissingDetail 两处 404 共用一句（形状非法 / 盘上没有），理由与
// iconMissingDetail 同一条：不回显"为什么没有"。
const backgroundMissingDetail = "没有这张背景"

// handleBackgroundFile 端出一个档案自己的背景图。
//
// `private, immutable` 而**不是** `public`：名字是内容哈希、换一张必然换一个
// URL（所以 immutable 名副其实），而这份字节是这一份档案的东西，不进任何公共
// 缓存。离线时它仍可能命中浏览器自己的 HTTP 缓存；Service Worker 的预缓存清单
// 里没有它（那份清单只有静态资产，见 app.js 末尾），所以离线首屏若没缓存到它，
// 表现是"背景不见了"而不是页面坏掉。
func (s *Server) handleBackgroundFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "sandbox")
	name := r.PathValue("name")
	if !model.ValidBackground(name) || name == "" {
		writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.RouteNotFound, Detail: backgroundMissingDetail})
		return
	}
	sess := auth.ReadSession(r)
	// 与 `/api/v1/profiles/self/pin` 同一句：带令牌访问只认会话的端点 = 403，
	// 而**不是** 401（那是在说一把好钥匙是废的）。形状非法与盘上没有才是 404。
	if _, _, ok := s.require(r, credSession, sess.Active); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	data, err := s.deps.Store.ReadBackground(sess.Active, name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Code: 404, Kind: apierr.RouteNotFound, Detail: backgroundMissingDetail})
			return
		}
		writeInternalError(w, err)
		return
	}
	ext := name[strings.LastIndexByte(name, '.')+1:]
	w.Header().Set("Content-Type", backgroundContentTypes[ext])
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// handlePrefsBackground 是背景那一节的表单通路：**上传/换一张**、**移除**、
// **暂时不显示/恢复显示**，三件事共用一个端点（一次按下 = 一个意图，与
// `/prefs/group` 同一处置）。
//
// **为什么不用 POST /prefs 的那几个字段**：那一格是 urlencoded 的浏览器表单，
// 而文件必须 multipart；而"这一格的值是几 MB 的图片"与"这一格的值是 auto|dark|light"
// 放进同一个 handler 会让那条 422 的字段名单越滚越长。两件事分两个端点，
// 判据各一份。
//
// 拒的长相：会话不够 303 回选择页（与 /prefs 同一族）；形状错 422 + 字段级
// 明细（不是权限拒绝）；档案读不出来走与 handlePrefs 同一条分诊。
func (s *Server) handlePrefsBackground(w http.ResponseWriter, r *http.Request) {
	id, ok := s.selfProfileOnly(w, r)
	if !ok {
		return
	}
	// **两种编码都要收**，而它们对应着面板上的两类表单：上传那张带
	// `enctype=multipart/form-data`（文件走不了 urlencoded），移除与暂时不显示
	// 那三张没有文件字段，用 urlencoded 更省。
	//
	// 判据是**请求自己的 Content-Type**（isFormPost 与解析那一支），不是"有没有
	// file 字段"——按后者判的话，一个 multipart 的空表单会被当成"没给文件"而
	// 回一句"要选一张图"，而人明明点的是「暂时不显示」。
	if isFormPost(r) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := r.ParseForm(); err != nil {
			writeFieldErrors(w, http.StatusUnprocessableEntity,
				[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "这份表单读不出来"}})
			return
		}
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, maxBackgroundBodyBytes)
		if err := r.ParseMultipartForm(maxBackgroundBodyBytes); err != nil {
			writeFieldErrors(w, http.StatusUnprocessableEntity,
				[]fieldError{{Kind: apierr.BodyUnreadable, Field: "body", Reason: "这份表单读不出来（多半是图太大或没选完）"}})
			return
		}
	}
	_, wantsRemove := r.Form["remove"]
	hidden := r.FormValue("hidden")
	wantsHide := hidden == "1"
	wantsShow := hidden == "0"

	file, header, fileErr := r.FormFile("file")
	hasFile := fileErr == nil && header != nil && header.Filename != ""

	if !hasFile && !wantsRemove && !wantsHide && !wantsShow {
		// "没说要改什么"不是"什么都不做然后成功"（与 handlePrefs 同一句）。
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
			{Kind: apierr.FieldInvalid, Field: "file", Reason: "要选一张图，或者按下「移除」/「暂时不显示」/「恢复显示」"},
		})
		return
	}
	if hasFile {
		defer file.Close()
	}

	// answered 是**"响应已经写出去了"**那一格。锁里有三支会直接写 422（形状不对/
	// 不是位图/没背景可藏），而它们写完之后若顺着函数末尾走下去，就会**再写一次
	// 303**：第一次 WriteHeader 赢，于是状态码看着是对的，响应体后面却被追加了一段
	// "See Other"——而测试只断言状态码，这一条会一直是绿的。
	// （同一个坑在锁外不成立：return 就是真的走了。）
	var (
		saveErr  error
		answered bool
	)
	err := s.deps.Store.WithProfileLock(id, func() error {
		p, err := s.deps.Store.LoadProfile(id)
		if err != nil {
			return err // 分诊在锁外：404 / 503 / 500
		}
		switch {
		case hasFile:
			data, rerr := readBackgroundUpload(file, header)
			if rerr != nil {
				// 这一支是**字段形状错**（422），不是落盘失败（500）：文件本身
				// 不被接受，与"这台机器出错了"是两件事，人能自己改。
				writeFieldErrors(w, http.StatusUnprocessableEntity,
					[]fieldError{{Kind: apierr.FieldInvalid, Field: "file", Reason: rerr.Error()}})
				answered = true
				return nil
			}
			ext := store.BackgroundExtFor(http.DetectContentType(data))
			if ext == "" {
				writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
					{Kind: apierr.FieldInvalid, Field: "file", Reason: "只认 png / jpg / webp / gif 位图（这一份的类型看不出来）"}})
				answered = true
				return nil
			}
			name, err := s.deps.Store.SaveBackground(id, data, ext)
			if err != nil {
				saveErr = err
				return nil
			}
			p.Background = name
			// 换了一张新的，"暂时不显示"就不该继续挂着：那是在说"我不想看
			// **那张**"，而人刚给了新的一张。
			p.BackgroundOff = false
		case wantsRemove:
			if p.Background != "" {
				if err := s.deps.Store.RemoveBackground(id, p.Background); err != nil && !errors.Is(err, store.ErrNotFound) {
					saveErr = err
					return nil
				}
			}
			p.Background = ""
			p.BackgroundOff = false
		case wantsHide:
			if p.Background == "" {
				// 没有背景可藏：说清楚，别静默成功（与 handlePrefs 同一纪律）。
				writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{
					{Kind: apierr.FieldInvalid, Field: "hidden", Reason: "这一份还没有背景图，没什么可藏的"}})
				answered = true
				return nil
			}
			p.BackgroundOff = true
		case wantsShow:
			p.BackgroundOff = false
		}
		saveErr = s.deps.Store.SaveProfile(p)
		return nil
	})
	if err != nil {
		s.writeProfileReadFailure(w, id, err)
		return
	}
	if answered {
		return
	}
	if saveErr != nil {
		writeInternalError(w, saveErr)
		return
	}
	// 成功回 `/prefs`：与 `/prefs/group` 同一去处——背景这一节在面板上，
	// 改完接着还能按下一颗（换一张之后看一眼效果要回面板）。
	http.Redirect(w, r, "/prefs", http.StatusSeeOther)
}

// readBackgroundUpload 读上传的字节并判体积。上限是 store.MaxBackgroundBytes，
// 判据住在 store（它才是写的人）——这里只把错误说成人话。
//
// **只多读一个字节**：读到上限+1 就停，于是"超限"是确定的而不是"读满了"。
// io.Copy 会把整个流读进内存，所以那个 LimitReader 不能省。
func readBackgroundUpload(f multipart.File, header *multipart.FileHeader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(f, store.MaxBackgroundBytes+1))
	if err != nil {
		return nil, fmt.Errorf("这份文件读不出来")
	}
	if len(data) == 0 {
		return nil, errors.New("这份文件是空的")
	}
	if len(data) > store.MaxBackgroundBytes {
		return nil, fmt.Errorf("这张图太大（上限 %d MB）", store.MaxBackgroundBytes>>20)
	}
	return data, nil
}
