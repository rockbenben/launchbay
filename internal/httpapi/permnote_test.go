package httpapi

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/store"
)

// 这一片讲的是同一个故障的两副面孔：宿主上的数据目录不归跑服务的那个用户
// （群晖/威联通上把 ./data 挂成 root 所属、容器以 uid 65534 跑，装机的头一个
// 撞的就是它）。一副给机器看（500 + stderr），一副给人看（303 + 一句教下一步
// 在哪台机器上做什么的话）。判据只有 permDenied 一个。

// 判据本身：只认 fs.ErrPermission 这一个哨兵，而且是隔着任意层 %w 认的。
//
// 三个"否"才是这条的重量所在：磁盘满、只读文件系统、撞名——它们都发生在同一
// 个 call site，都不是权限。只读文件系统（EROFS）尤其不能顺手算进来：那句话
// 教的是 chown，而 chown 治不了一个被挂成 ro 的卷，把人引到错的地方比不说更坏。
func Test落盘失败_只把权限这一族判成permission(t *testing.T) {
	denied := &os.PathError{Op: "mkdir", Path: "/data/profiles/dad", Err: syscall.EACCES}
	cases := map[string]struct {
		err  error
		want bool
	}{
		"裸的 EACCES": {denied, true},
		"EPERM 同样是权限": {
			&os.PathError{Op: "open", Path: "/data/profiles/dad/profile.json", Err: syscall.EPERM}, true,
		},
		"隔着两层 %w 也要认得出": {
			// store 的真实形状：CreateProfile 包一层，WriteFileAtomic 那条再包一层。
			fmt.Errorf("建档案目录: %w", fmt.Errorf("写档案 %q: %w", "dad", denied)), true,
		},
		"磁盘满不是权限": {&os.PathError{Op: "write", Err: syscall.ENOSPC}, false},
		"只读文件系统不是权限": {
			&os.PathError{Op: "mkdir", Err: syscall.EROFS}, false,
		},
		"不是目录也不是权限": {&os.PathError{Op: "mkdir", Err: syscall.ENOTDIR}, false},
		"撞名有自己的分支":  {store.ErrExists, false},
		"nil":       {nil, false},
	}
	for name, tc := range cases {
		if got := permDenied(tc.err); got != tc.want {
			t.Errorf("%s：permDenied = %v，要 %v", name, got, tc.want)
		}
	}
}

// 那句话必须**教得出下一步**，否则它只是把 EACCES 翻译成了中文：它得说清是
// 宿主机那一侧的权限、跑服务的是哪个用户、去哪台机器上改。
func Test权限那句话说得出出路(t *testing.T) {
	say := noticeOf(i18n.Default, "permission")
	if say == "" {
		t.Fatal("pinNotices 缺 permission 键：两张表单会转到这个键，而页面上只有一个空位置")
	}
	for _, want := range []string{"数据目录", "权限", "65534"} {
		if !strings.Contains(say, want) {
			t.Errorf("那句话里没有 %q，读到的是 %q", want, say)
		}
	}
	// 白名单纪律照旧：它不许复读用户的输入，也不许把磁盘路径写死进去。
	if strings.Contains(say, "/data") {
		t.Errorf("那句话把磁盘路径写死了（换一台机器就是错的）: %q", say)
	}
}

// chmodNoWrite 把目录改成"进得去、写不了"，并在**这台机器造不出这个状态**时
// 明说后跳过：Windows 上以管理员身份跑测试时 chmod 挡不住自己，容器里以 root
// 跑测试同理。跳过不是放过——判据那条纯函数证人照跑，端到端这条在 Linux CI
// 上是真的跑（CI 的 runner 以非 root 用户执行）。
func chmodNoWrite(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("这台机器改不动目录权限（%v），端到端那条跳过", err)
	}
	probe := filepath.Join(dir, ".perm-probe")
	if err := os.WriteFile(probe, nil, 0o200); !errors.Is(err, fs.ErrPermission) {
		_ = os.Remove(probe)
		_ = os.Chmod(dir, 0o700)
		t.Skipf("这台机器（%s）在 0500 的目录里照样写得进去，造不出权限失败", runtime.GOOS)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

// 引导建档是**装机后第一个会撞上它的动作**：零档案的人在浏览器里填两格就点提交，
// 而 host 上的 ./data 归不归 65534 与他毫无关系。这时给他一段 500 JSON 等于让他
// 猜"500 是什么意思"；给他一句"去宿主机把目录交给 65534"才是能接着往下走的答案。
func TestSetup_数据目录不可写说得出出路(t *testing.T) {
	dir := t.TempDir()
	scopes, err := auth.ParseScopes("ai-dad:tok-dad:dad")
	if err != nil {
		t.Fatal(err)
	}
	h := New(newDeps(t, store.New(dir), scopes)).Handler()
	chmodNoWrite(t, dir)

	rec := postForm(h, "/setup", "title=%E7%88%B8&pin=1234", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("数据目录不可写要 303 回引导页说人话，得到 %d %.200s", rec.Code, rec.Body.String())
	}
	// **断的是那把键，不是整串**：Location 上还带着刚输入的名字（2026-10-24
	// 那一族改动：名字用 `?title=` 带回，PIN 一个字不回填）。
	//
	// 这一条从"整串逐字相等"改成"读出 err 这把键"，与同族的管理页那条同形
	// （2026-10-25 那一笔已经这么改了，**漏了本条**）。漏改的代价由 ubuntu
	// 那格付：NTFS 上 0500 挡不住自己，`chmodNoWrite` 让这条测试在 Windows 上
	// 整条跳过——于是"只有 Linux 看得见的那条断言"红了两天才被逮到。
	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("Location 解析不了：%q", loc)
	}
	if got := u.Query().Get("err"); got != "permission" {
		t.Fatalf("err = %q，要 permission（Location=%q）", got, loc)
	}
	// 名字也要一起带回来：这一族失败之后，人是**要去宿主机改完再来一次**的
	// ——名字被清空就等于让人白打一遍。三张建档表单同一条纪律（另两张见下面
	// 那条形状证人）。
	if got := u.Query().Get("title"); got != "爸" {
		t.Errorf("名字没被带回：title = %q，要 爸（Location=%q）", got, loc)
	}
	if cs := rec.Result().Cookies(); len(cs) != 0 {
		t.Fatalf("建档没成不许发会话 cookie: %v", cs)
	}
	// 转过去的那一页必须真的印出那句话。这一条才是"说得出出路"的正对照：
	// 只断 303 与 Location 时，把 redirectSetup 换成裸 303 也照样全绿。
	page := get(h, "/?err=permission", "")
	if !strings.Contains(page.Body.String(), noticeOf(i18n.Default, "permission")) {
		t.Errorf("引导页没印出登记的那句话:\n%.500s", page.Body.String())
	}
	// 内部文本不外泄：磁盘路径与 OS 原文一个字都不许进页面。
	if body := page.Body.String(); strings.Contains(body, "permission denied") ||
		strings.Contains(body, "Access is denied") {
		t.Errorf("OS 的原始错误文本被渲染进页面了:\n%.500s", body)
	}
	// 磁盘上什么也没建成：这一族最怕"回了一句话，其实建成功了"。
	if n, err := countProfileDirs(dir); err != nil || n != 0 {
		t.Errorf("建档没成却留下了 %d 个档案目录（err=%v）", n, err)
	}
}

// 管理页那张表单是同一个故障的第二张脸。它与 /setup 共用判据与那句话，
// 所以这里断的是**接线**（这处壳自己那支分支在不在），不是判据本身。
func TestAdmin_数据目录不可写说得出出路(t *testing.T) {
	s, h := setup(t)
	chmodNoWrite(t, filepath.Join(s.Root(), "profiles"))

	// **凭证要换成管理员 cookie**：2026-10-12 起档案会话不算管理权，用
	// dadFormCookie 打这里会先被权限拒掉、回 `/`，而那条 303 与权限类拒绝
	// 长得一模一样——症状是"Location 是 / 而不是 /admin?err=permission"。
	//
	// 这条在 **Windows 上验不到**（`chmodNoWrite` 会先自查并跳过：NTFS 上
	// 0500 的目录照样写得进去），所以它只在 Linux runner 上真的跑。改这一格
	// 之后第一次由 CI 的 ubuntu job 抓到。
	rec := postForm(h, "/admin", "title=%E5%A6%88%E5%A6%88&pin=5678", adminOnlyCookie(t))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("数据目录不可写要 303 回管理页说人话，得到 %d %.200s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("Location 解析不了：%q", loc)
	}
	if got := u.Query().Get("err"); got != "permission" {
		t.Fatalf("err = %q，要 permission（Location=%q）", got, loc)
	}
	page := get(h, "/admin?err=permission", adminOnlyCookie(t))
	if !strings.Contains(page.Body.String(), noticeOf(i18n.Default, "permission")) {
		t.Errorf("管理页没印出登记的那句话:\n%.500s", page.Body.String())
	}
}

// 三张建档表单的**回跳形状**证人：每一张在"宿主数据目录不可写"那一族都把刚
// 输入的名字用 `?title=` 带回（PIN 一个字都不回填——那是凭证，不该进 URL）。
//
// **为什么另立一条不碰磁盘的**：端到端那两条要 chmod 出"目录不可写"这个状态，
// 而 NTFS 上 0500 挡不住自己——`chmodNoWrite` 自查之后整条跳过。于是"权限那一族
// 的回跳长什么样"在 Windows 上**一个证人也没有**，而它恰恰是 2026-10-25 那一笔
// 漏改的那一处（红了两天，是 ubuntu 那格逮到的；windows 那格因为跳过一直绿）。
// 这一条只断拼出来的那串 Location：每个平台都跑，于是三张表单的形状变了一处
// 当场红，不必等 CI。
//
// **判据是逐字那一串**，不是"里面有 err=permission"：键的**取值**由白名单管
// （TestSetup_提示只走白名单），而**形状**（名字带不带、带几个键、键的顺序）
// 只该住在这里一处。
func Test三张建档表单的权限回跳形状每个平台都钉得住(t *testing.T) {
	const title = "爸"
	for _, tc := range []struct {
		name string
		call func(w http.ResponseWriter)
		want string
	}{
		{
			"引导页",
			func(w http.ResponseWriter) { redirectSetupForm(w, "permission", title, false) },
			"/?err=permission&title=%E7%88%B8",
		},
		{
			"自建那张（折着的表单要展开）",
			func(w http.ResponseWriter) { redirectSetupForm(w, "permission", title, true) },
			"/?err=permission&new=1&title=%E7%88%B8",
		},
		{
			"管理页",
			func(w http.ResponseWriter) { redirectAdminCreateFail(w, adminFormTitled(title), "permission") },
			"/admin?err=permission&title=%E7%88%B8",
		},
	} {
		rec := httptest.NewRecorder()
		tc.call(rec)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s 的回跳要 303，得到 %d", tc.name, rec.Code)
		}
		if got := rec.Header().Get("Location"); got != tc.want {
			t.Errorf("%s 的回跳 = %q，要 %q", tc.name, got, tc.want)
		}
	}
}

// adminFormTitled 造一个带着 title 的表单请求——管理页那张回跳是从**请求**里读
// 名字的（判据在 r.FormValue("title")，不由调用方递进来）。
func adminFormTitled(title string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/admin",
		strings.NewReader("title="+url.QueryEscape(title)))
	// ParseForm 只在 Content-Type 是表单那一种时才读 body——不钉这一行，
	// r.FormValue("title") 读出来是空串，回跳就少了 &title=，而症状是
	// "测试红了"，不是"读不出来"：那一格看着像标题没带回去，其实是请求没被解析。
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		panic(err)
	}
	return r
}

// API 那条壳（POST /api/v1/profiles）**不许**跟着改成 303：它回给机器，
// 调用方读的是 stderr 与容器日志，而 500 是"服务端自己错了、重试没意义"那一句
// 的标准长相。把机器通路也折成一句话，等于把一个人类形状塞进协议里。
func Test建档API_权限失败仍是500(t *testing.T) {
	dir := t.TempDir()
	scopes, err := auth.ParseScopes("ai-dad:tok-dad:dad")
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(dir)
	h := New(newDeps(t, s, scopes)).Handler()
	chmodNoWrite(t, dir)

	rec := post(h, "/api/v1/profiles", `{"id":"mom","title":"妈妈","pin":"5678"}`, "", "tok-admin")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("机器通路要 500 JSON 外壳，得到 %d %.200s", rec.Code, rec.Body.String())
	}
	sh := decodeErrorShell(t, rec.Body.Bytes())
	if sh.Code != http.StatusInternalServerError || sh.Detail != "内部错误" {
		t.Errorf("500 的外壳要 code=500、detail=内部错误，得到 %+v", sh)
	}
}

// countProfileDirs 数 profiles 下的目录——建档有没有真的建成，判据是盘上而不是
// 响应体（与 store 自己的引导判据同源：数目录）。
func countProfileDirs(root string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(root, "profiles"))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n, nil
}
