package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// 这组用例对应的端点在 2026-10-10 之前只存在于文档里：README 与 spec §6.1/§10/§12
// 都把 `launchbay profile rebuild` 写成坏档的唯一出路，而 runCLI 里
// `cmdProfile` 认的只有 `create`——敲 rebuild 得到的是 create 的用法与退出码 2。
// 下面这些钉的是"它现在真的能跑，而且接线的每一格都对"。

// TestCLIProfileRebuildAlwaysSendsAdmin 是那条最容易写错的接线：
// **省略 --admin 旗标时也必须显式发 admin:false**。
//
// 端点那一格是必填的（省略 = 422），而壳若"旗标没写就不发"，每一次不带 --admin
// 的调用都会撞 422，且错在调用方完全看不见的地方。判据是**请求体里真的有那一格**，
// 不是"重建成功了"——后者在 --admin 写出来时同样绿。
func TestCLIProfileRebuildAlwaysSendsAdmin(t *testing.T) {
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:tok-admin:*")
	var bodies []map[string]any
	var paths, bys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		bys = append(bys, r.Header.Get("X-LB-By"))
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Errorf("CLI 发出的体不是合法 JSON: %v (%q)", err, raw)
		}
		bodies = append(bodies, m)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"kid","title":"小满","admin":false}`))
	}))
	defer srv.Close()

	// 故意**不写** --admin：这一支就是"省略旗标"
	var buf bytes.Buffer
	if code := runCLI([]string{"profile", "rebuild", "--id", "kid", "--title", "小满",
		"--pin", "5678"}, cliCfg(t, srv), &buf); code != 0 {
		t.Fatalf("该退出 0，实际 %d out=%q", code, buf.String())
	}
	if len(bodies) != 1 {
		t.Fatalf("该发一次请求，实际 %d 次", len(bodies))
	}
	if paths[0] != "/api/v1/profiles/rebuild" {
		t.Errorf("打错了路径: %q", paths[0])
	}
	if bys[0] != "cli" {
		t.Errorf("X-LB-By 该是 cli（重建要写审计行，缺了它会被记成 web），实际 %q", bys[0])
	}
	got := bodies[0]
	for k, want := range map[string]string{"profile": "kid", "title": "小满", "pin": "5678"} {
		if got[k] != want {
			t.Errorf("body[%q] = %v，要 %q", k, got[k], want)
		}
	}
	v, ok := got["admin"]
	if !ok {
		t.Fatalf("body 里没有 admin 那一格（省略旗标时也必须显式发）: %v", got)
	}
	if v != false {
		t.Errorf("不带 --admin 时 admin 该是 false，实际 %v", v)
	}
}

// --admin 写出来时它该是 true：两条合起来才是"这一格接对了"，而不是只钉默认值。
func TestCLIProfileRebuildSendsAdminTrueWhenFlagGiven(t *testing.T) {
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:tok-admin:*")
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"id":"mom","title":"妈妈","admin":true}`))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	if code := runCLI([]string{"profile", "rebuild", "--id", "mom", "--title", "妈妈",
		"--pin", "4321", "--admin"}, cliCfg(t, srv), &buf); code != 0 {
		t.Fatalf("该退出 0，实际 %d out=%q", code, buf.String())
	}
	if body["admin"] != true {
		t.Errorf("--admin 写出来时该发 true，实际 %v", body["admin"])
	}
	if !strings.Contains(buf.String(), "管理员") {
		t.Errorf("成功输出该说清这是管理员档案: %q", buf.String())
	}
}

// 端到端：坏档 → CLI 重建 → 真的读得动了，而书签**一个字节都没动**。
// 判据读盘（sha256），不看返回值——"响应说成功"与"那张卡真的回来了"是两件事。
func TestCLIProfileRebuildEndToEnd(t *testing.T) {
	s, srv := newRealServer(t)
	defer srv.Close()
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:tok-admin:*")

	// 先给 dad 一点书签（重建不许碰它们），再把 profile.json 写坏。
	if err := os.WriteFile(s.ProfilePath("dad"), []byte("{\n  \"id\": \"dad\",\n  \"title\":\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	marksBefore := shaFile(t, s.BookmarksPath("dad"))

	var buf bytes.Buffer
	code := runCLI([]string{"profile", "rebuild", "--id", "dad", "--title", "爸爸",
		"--pin", "4321", "--admin"}, cliCfg(t, srv), &buf)
	if code != 0 {
		t.Fatalf("坏档该重建成功, code=%d out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "已重建 dad") {
		t.Errorf("成功话术不对: %q", buf.String())
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatalf("重建之后该读得动: %v", err)
	}
	if p.Title != "爸爸" || !p.Admin {
		t.Errorf("重建出的档案形状不对: %+v", p)
	}
	if got := shaFile(t, s.BookmarksPath("dad")); got != marksBefore {
		t.Errorf("重建动了 bookmarks.json（重建的承诺是一个字节都不碰）")
	}
	// 只经 REST：CLI 没在本地工作目录里落任何东西。
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "launchbay.go" && !strings.HasSuffix(e.Name(), ".go") {
			t.Errorf("CLI 在工作目录里落了 %q（它唯一的写通路是 REST）", e.Name())
		}
	}
}

// 服务侧的失败原样说出来，退出码非 0。409 是这一族最常撞的一条
// （"这份档案读得动，不需要重建"）。
func TestCLIProfileRebuildSurfacesConflict(t *testing.T) {
	_, srv := newRealServer(t)
	defer srv.Close()
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:tok-admin:*")

	var buf bytes.Buffer
	// dad 是好的（夹具里没写坏），所以重建它必然 409
	code := runCLI([]string{"profile", "rebuild", "--id", "dad", "--title", "爸爸",
		"--pin", "4321"}, cliCfg(t, srv), &buf)
	if code != 1 {
		t.Fatalf("409 该退出 1，实际 %d out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "读得动") {
		t.Fatalf("detail 该原样带出来: %q", buf.String())
	}
}

// 目录不存在也是 404，同样原样说出来。CLI 不许把它说成"建好了"。
func TestCLIProfileRebuildSurfacesNotFound(t *testing.T) {
	_, srv := newRealServer(t)
	defer srv.Close()
	t.Setenv("LAUNCHBAY_WRITE_TOKENS", "admin:tok-admin:*")

	var buf bytes.Buffer
	code := runCLI([]string{"profile", "rebuild", "--id", "ghost", "--title", "幽灵",
		"--pin", "4321"}, cliCfg(t, srv), &buf)
	if code != 1 {
		t.Fatalf("404 该退出 1，实际 %d out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "不存在") {
		t.Fatalf("detail 该原样带出来: %q", buf.String())
	}
	if strings.Contains(buf.String(), "已重建") {
		t.Fatalf("失败却说了成功: %q", buf.String())
	}
}

// 用法文本必须**同时**列出两个子命令：rebuild 落地之前，`profile rebuild` 打印的
// 是 create 的用法——那正是"文档说的命令不存在"在终端上的样子。
func TestCLIProfileUsageListsBothSubcommands(t *testing.T) {
	var buf bytes.Buffer
	if code := runCLI([]string{"profile"}, config{Port: "7600"}, &buf); code != 2 {
		t.Fatalf("没给子命令该是用法错误 2，实际 %d", code)
	}
	for _, want := range []string{"profile create", "profile rebuild"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("用法里该有 %q，实际 %q", want, buf.String())
		}
	}
	// 不认识的子命令同样给出两份用法，不许只报一个
	buf.Reset()
	if code := runCLI([]string{"profile", "rebuild-all"}, config{Port: "7600"}, &buf); code != 2 {
		t.Fatalf("未知子命令该是 2，实际 %d", code)
	}
	if !strings.Contains(buf.String(), "profile rebuild") {
		t.Errorf("未知子命令也要给出完整用法: %q", buf.String())
	}
}

func shaFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
