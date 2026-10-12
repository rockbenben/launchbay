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

// 2026-10-12：管理员从**档案属性**变成一把机器级的 key（LAUNCHBAY_ADMIN_TOKEN），
// 于是 `--admin` 旗标与请求体里的 admin 那一格**整个不存在了**——建档请求体从四格
// 变三格，重建请求体也一样。
//
// 被这一条接替的是原来的两条：
// TestCLIProfileRebuildAlwaysSendsAdmin（钉"省略 --admin 时也必须显式发 admin:false"，
// 因为那一格曾在必填）与 TestCLIProfileRebuildSendsAdminTrueWhenFlagGiven。
// 它们的意图——**壳不许替调用方猜**——没有丢，只是换了形状：现在连"猜"的地方都没有了，
// 于是判据翻面成"body 里不许出现 admin"。删掉的两条里还有路径、X-LB-By 与那三格字段，
// 那些判据在这里一并留着（否则它们会跟着测试一起消失）。
func TestCLIProfileBodiesCarryNoAdminKey(t *testing.T) {
	adminKeyEnv(t)
	type sent struct {
		path, by, auth, raw string
	}
	var got []sent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !json.Valid(raw) {
			t.Errorf("CLI 发出的体不是合法 JSON: %q", raw)
		}
		got = append(got, sent{path: r.URL.Path, by: r.Header.Get("X-LB-By"),
			auth: r.Header.Get("Authorization"), raw: string(raw)})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"mom","title":"妈妈"}`))
	}))
	defer srv.Close()

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"create", []string{"profile", "create", "--id", "mom", "--title", "妈妈", "--pin", "1234"},
			"已建档 mom"},
		{"rebuild", []string{"profile", "rebuild", "--id", "kid", "--title", "小满", "--pin", "5678"},
			"已重建 mom"},
	} {
		var buf bytes.Buffer
		if code := runCLI(tc.args, cliCfg(t, srv), &buf); code != 0 {
			t.Fatalf("%s：该退出 0，实际 %d out=%q", tc.name, code, buf.String())
		}
		if !strings.Contains(buf.String(), tc.want) {
			t.Errorf("%s：输出该含 %q，实际 %q", tc.name, tc.want, buf.String())
		}
	}
	if len(got) != 2 {
		t.Fatalf("该各发一次请求，实际 %d 次", len(got))
	}
	if got[0].path != "/api/v1/profiles" || got[1].path != "/api/v1/profiles/rebuild" {
		t.Errorf("打错了路径: %q / %q", got[0].path, got[1].path)
	}
	// 重建要写审计行，缺了 X-LB-By 会被记成 web（原 TestCLIProfileRebuildAlwaysSendsAdmin 钉的）。
	if got[1].by != "cli" {
		t.Errorf("X-LB-By 该是 cli，实际 %q", got[1].by)
	}
	for i, s := range got {
		// 钉的就是这一条：body 里**不许**有 admin。留着它等于把一个已删除的字段
		// 重新写进服务端——服务端会当未知字段拒（422）或更糟：哪天字段回来了，
		// 这条壳会替调用方偷偷发一个 false 出去。
		if strings.Contains(s.raw, "admin") {
			t.Errorf("%s：body 里不该再有 admin 那一格: %s", []string{"create", "rebuild"}[i], s.raw)
		}
		if !strings.Contains(s.auth, testAdminKey) {
			t.Errorf("该带那把管理员 key 而不是档案作用域令牌: %q", s.auth)
		}
	}
}

// 端到端：坏档 → CLI 重建 → 真的读得动了，而书签**一个字节都没动**。
// 判据读盘（sha256），不看返回值——"响应说成功"与"那张卡真的回来了"是两件事。
func TestCLIProfileRebuildEndToEnd(t *testing.T) {
	s, srv := newRealServer(t)
	defer srv.Close()
	adminKeyEnv(t)

	// 先给 dad 一点书签（重建不许碰它们），再把 profile.json 写坏。
	if err := os.WriteFile(s.ProfilePath("dad"), []byte("{\n  \"id\": \"dad\",\n  \"title\":\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	marksBefore := shaFile(t, s.BookmarksPath("dad"))

	var buf bytes.Buffer
	// **不带 --admin**（2026-10-12）：那个旗标已删，flag 会拒绝它。
	code := runCLI([]string{"profile", "rebuild", "--id", "dad", "--title", "爸爸",
		"--pin", "4321"}, cliCfg(t, srv), &buf)
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
	// 原来的 `|| !p.Admin` 随 `model.Profile.Admin` 一起删掉（钉的字段已不存在），
	// 重建出的形状由还在的字段说。
	if p.Title != "爸爸" || p.PIN == "" {
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
	adminKeyEnv(t)

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
	adminKeyEnv(t)

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
