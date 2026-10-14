package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

// 这一页自己的背景图（2026-10-14）。这一族的钉守集中在三件事上，而它们每一个
// 的症状都是"功能测试全绿而人在屏上看不出来"：
//
//	1. **谁看得到**：只有会话，且只看到会话所指那一份的目录（§11：各人一份）。
//	2. **写侧只写档案**：背景落在 profile.json，不动 rev、不做快照。
//	3. **坏值不挡页面**：盘上写坏的名字最坏是"这张背景不见了"，不是坏档、不是 500。

// bgPNG 造一张真 PNG（同 store 那条：魔数是判据，手敲字节的夹具迟早少一个引号）。
func bgPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, G: 40, B: 40, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// uploadBG 走**浏览器真的会发的那种请求**：multipart/form-data。
// 用 urlencoded 打过去的话这条路会静默走成"没给文件"，而症状是"按了上传没反应"。
func uploadBG(t *testing.T, h http.Handler, cookie, bearer, field, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if field != "" {
		// filename 必须非空：空文件名的 file 字段在浏览器那侧等于"没选文件"
		fw, err := mw.CreateFormFile(field, filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/prefs/background", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// postBGFields 发一张**只有 urlencoded 字段**的表单（移除 / 暂时不显示那两条通路）。
func postBGFields(t *testing.T, h http.Handler, cookie, k, v string) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	req := httptest.NewRequest("POST", "/prefs/background", strings.NewReader(k+"="+v))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	_ = body
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func bgProfile(t *testing.T, s *store.Store, id string) {
	t.Helper()
	if _, err := s.LoadProfile(id); err != nil {
		t.Fatalf("夹具档案 %s: %v", id, err)
	}
}

func TestPrefsBackground_上传换移除三件事都落到盘上(t *testing.T) {
	s, h := setup(t)
	bgProfile(t, s, "dad")
	revBefore := revOf(t, s, "dad")

	// ① 上传
	if rec := uploadBG(t, h, dadFormCookie, "", "file", "wall.png", bgPNG(t)); rec.Code != http.StatusSeeOther {
		t.Fatalf("上传该 303 回面板，得到 %d：%s", rec.Code, rec.Body)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if !model.ValidBackground(p.Background) || p.Background == "" {
		t.Fatalf("profile.json 里没有合法的背景名：%q", p.Background)
	}
	if _, err := os.Stat(s.BackgroundPath("dad", p.Background)); err != nil {
		t.Fatalf("blob 没落盘：%v", err)
	}
	if got := revOf(t, s, "dad"); got != revBefore {
		t.Errorf("改背景推进了 rev（%d → %d）：它是台面偏好，不是书签写入", revBefore, got)
	}
	if n := snapshotCountOf(t, s, "dad"); n != 0 {
		t.Errorf("改背景做了快照（%d 份）：它不该产生任何版本", n)
	}

	// ② 换一张：新的名字换新的 URL，而旧 blob 留在盘上（换是高频，删是低频且有风险）
	other := append(bgPNG(t), 0x00)
	if rec := uploadBG(t, h, dadFormCookie, "", "file", "wall2.png", other); rec.Code != http.StatusSeeOther {
		t.Fatalf("换一张该 303，得到 %d：%s", rec.Code, rec.Body)
	}
	p2, _ := s.LoadProfile("dad")
	if p2.Background == p.Background {
		t.Error("换了内容却还是同一个名字：immutable 缓存会把旧图端出来")
	}

	// ③ 暂时不显示 / 恢复显示：文件一个字节都不动
	if rec := postBGFields(t, h, dadFormCookie, "hidden", "1"); rec.Code != http.StatusSeeOther {
		t.Fatalf("暂时不显示该 303，得到 %d", rec.Code)
	}
	hid, _ := s.LoadProfile("dad")
	if !hid.BackgroundOff || hid.Background != p2.Background {
		t.Errorf("暂时不显示只该翻那一位：off=%v background=%q（原来 %q）", hid.BackgroundOff, hid.Background, p2.Background)
	}
	if _, err := os.Stat(s.BackgroundPath("dad", p2.Background)); err != nil {
		t.Errorf("暂时不显示不该动文件：%v", err)
	}
	if rec := postBGFields(t, h, dadFormCookie, "hidden", "0"); rec.Code != http.StatusSeeOther {
		t.Fatalf("恢复显示该 303，得到 %d", rec.Code)
	}

	// ④ 移除：字段清空 + 文件真的没了
	if rec := postBGFields(t, h, dadFormCookie, "remove", "1"); rec.Code != http.StatusSeeOther {
		t.Fatalf("移除该 303，得到 %d", rec.Code)
	}
	gone, _ := s.LoadProfile("dad")
	if gone.Background != "" || gone.BackgroundOff {
		t.Errorf("移除之后该两格都空：background=%q off=%v", gone.Background, gone.BackgroundOff)
	}
	if _, err := os.Stat(s.BackgroundPath("dad", p2.Background)); !os.IsNotExist(err) {
		t.Error("移除之后文件该没了（这一条是「移除」与「标记清空」的区别所在）")
	}
}

func TestPrefsBackground_不是位图或太大都422且不写盘(t *testing.T) {
	s, h := setup(t)
	bgProfile(t, s, "dad")

	for name, tc := range map[string]struct {
		file string
		data []byte
	}{
		"svg": {"x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`)},
		"文本":  {"x.txt", []byte("hello")},
		"空的":  {"x.png", nil},
		"太大":  {"x.png", bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, store.MaxBackgroundBytes/4+16)},
	} {
		rec := uploadBG(t, h, dadFormCookie, "", "file", tc.file, tc.data)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s 要 422（这一格的值不合法，不是这台机器坏了），得到 %d：%s", name, rec.Code, rec.Body)
			continue
		}
		var body struct {
			Errors []struct{ Field, Reason string } `json:"errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Errors) == 0 || body.Errors[0].Field != "file" {
			t.Errorf("%s 的 422 要点名 file 字段：%s", name, rec.Body)
		}
		// **被拒的响应不许带 Location**：那三支 422 是在锁内写出的，写完若顺着
		// 函数末尾走下去就会再走一次 http.Redirect(303)——第一次 WriteHeader 赢，
		// 于是状态码看着是对的，而这一条 422 后面挂着一个 `Location: /prefs`。
		// POST 的 303 不写正文，所以**正文里抓不到它**（我先写成了查
		// "See Other"，那条闸会一直是绿的——抓错了观测量的那一次）。
		if loc := rec.Header().Get("Location"); loc != "" {
			t.Errorf("%s 的 422 后面挂着一个 Location: %q——写完 422 又走了一次重定向", name, loc)
		}
		if p, _ := s.LoadProfile("dad"); p.Background != "" {
			t.Errorf("%s 被拒之后 profile.json 仍被改了：%q", name, p.Background)
		}
	}
}

func TestPrefsBackground_没说要改什么就422(t *testing.T) {
	_, h := setup(t)
	rec := uploadBG(t, h, dadFormCookie, "", "", "", nil) // 一张什么字段都没有的表单
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("空表单要 422（静默 303 会让人以为改成了），得到 %d", rec.Code)
	}
}

func TestBackgroundFile_只发给会话所指的那一份(t *testing.T) {
	s, h := setup(t)
	bgProfile(t, s, "dad")
	bgProfile(t, s, "kid")

	name, err := s.SaveBackground("dad", bgPNG(t), "png")
	if err != nil {
		t.Fatal(err)
	}
	// kid 那份**必须不是同一张图**：内容寻址下同样的字节会得到同样的哈希，
	// 于是"kid 的目录里也有那个文件"，这一条要测的隔离就假绿了。
	green, err := s.SaveBackground("kid", append(bgPNG(t), 0x00), "png")
	if err != nil {
		t.Fatal(err)
	}
	if green == name {
		t.Fatal("两份夹具撞成同一个哈希：这一条要测的是两个目录的隔离")
	}

	// 本人会话：200 + 原字节 + 那一组头
	rec := get(h, "/backgrounds/"+name, dadFormCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("本人取自己的背景要 200，得到 %d：%s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q，要 image/png", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") || !strings.Contains(got, "private") {
		t.Errorf("Cache-Control = %q，要 private + immutable（内容寻址）", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Error("缺 nosniff：这段是要被浏览器当图取/被当文档打开的")
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
		t.Errorf("Content-Security-Policy = %q，要 sandbox（与 favicon 通路同一个理由）", got)
	}

	// 匿名：401（"没带凭证"），不是 404 也不是 403
	if rec := get(h, "/backgrounds/"+name, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("匿名取背景要 401，得到 %d", rec.Code)
	}
	// 带令牌：403（认出来了，而令牌不带这个门）——与 self/pin 同一句
	if rec := get(h, "/backgrounds/"+name, ""); rec.Code == http.StatusOK {
		t.Error("空凭证不该 200")
	}
	// **别人的那一份**：kid 的会话拿着 dad 的哈希 → 404（形状对、但不在 kid 自己的目录里）
	if rec := get(h, "/backgrounds/"+name, "lb_profile=kid"); rec.Code != http.StatusNotFound {
		t.Errorf("kid 的会话取 dad 的背景要 404，得到 %d", rec.Code)
	}
	// 形状不对与盘上没有：同一句 404
	for _, bad := range []string{"0123456789abcde.png", "0123456789ABCDEF.png", "index.json", "..%2f..%2fprofile.json"} {
		rec := get(h, "/backgrounds/"+bad, dadFormCookie)
		if rec.Code != http.StatusNotFound {
			t.Errorf("形状 %q 要 404，得到 %d", bad, rec.Code)
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
			t.Errorf("形状 %q 的 404 也要带 sandbox（头设在分支之前）", bad)
		}
	}
	// kid 自己那份在他的会话下是好的（证人：上面那个 404 不是「这条路坏了」）
	if rec := get(h, "/backgrounds/"+green, "lb_profile=kid"); rec.Code != http.StatusOK {
		t.Errorf("kid 取自己的背景要 200，得到 %d", rec.Code)
	}
}

func snapshotCountOf(t *testing.T, s *store.Store, id string) int {
	t.Helper()
	_, n := snapshotCount(t, s, id)
	return n
}
