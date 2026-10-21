package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 这一对子命令只是两条端点的壳：读写都走 REST，目标档案从令牌作用域推导。
// 这里的用例跑**真的 runCLI → 真的 handler → 真的 store**，所以"只经 REST"
// 与"落到盘上的是什么"都是实测出来的，不是断言愿望。

// backup 把 dad 导出成一份文件，那份文件的形状要与端点导出的同一份。
func TestCLIBackupEndToEnd(t *testing.T) {
	adminKeyEnv(t)
	_, srv := newRealServer(t)
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "dad.json")
	var out, errBuf bytes.Buffer
	code := runCLI([]string{"backup", "--profile", "dad", "--out", path}, cliCfg(t, srv), &out, &errBuf)
	if code != 0 {
		t.Fatalf("该退 0，实际 %d out=%q err=%q", code, out.String(), errBuf.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Kind    string `json:"kind"`
		Version int    `json:"version"`
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("导出的那份读不出来: %v", err)
	}
	if pkg.Kind != "launchbay-backup" || pkg.Version != 1 || pkg.Profile != "dad" {
		t.Errorf("导出的那份形状不对: %+v", pkg)
	}
	// 落盘的文件里不该有门闩。
	if strings.Contains(string(raw), "\"pin\"") {
		t.Error("导出的文件里有 pin 那一格——门闩不进备份")
	}
}

// 不给 --out 时打到标准输出，于是能直接重定向成文件。
// **那一路一个字提示都不打**：提示混进 JSON 里，那份文件就自己坏了。
func TestCLIBackup_打到标准输出时一个字提示都不打(t *testing.T) {
	adminKeyEnv(t)
	_, srv := newRealServer(t)
	defer srv.Close()
	var out, errBuf bytes.Buffer
	code := runCLI([]string{"backup", "--profile", "dad"}, cliCfg(t, srv), &out, &errBuf)
	if code != 0 {
		t.Fatalf("该退 0，实际 %d err=%q", code, errBuf.String())
	}
	var pkg struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(out.Bytes(), &pkg); err != nil {
		t.Fatalf("标准输出上该是纯 JSON（提示混进去那份文件就坏了）: %v -- %q", err, out.String())
	}
	if pkg.Kind != "launchbay-backup" {
		t.Errorf("kind = %q，要 launchbay-backup", pkg.Kind)
	}
}

func TestCLIRestoreEndToEnd(t *testing.T) {
	adminKeyEnv(t)
	s, srv := newRealServer(t)
	defer srv.Close()
	body := `{"kind":"launchbay-backup","version":1,"profile":"dad","settings":{"theme":"light"}}`
	path := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := runCLI([]string{"restore", "--profile", "dad", path}, cliCfg(t, srv), &out, &errBuf)
	if code != 0 {
		t.Fatalf("该退 0，实际 %d out=%q err=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "theme") {
		t.Errorf("该说清改了哪几格: %q", out.String())
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "light" {
		t.Errorf("盘上 theme = %q，要 light", p.Theme)
	}
}

// 目标档案不空时**服务端**拒，CLI 把那句带出来（退 1，诊断走 stderr）。
// CLI 自己不数一遍——数两遍就会有一处漂，而漂掉的那一处会让人以为能导。
func TestCLIRestore_目标不空时带出服务端那一句(t *testing.T) {
	adminKeyEnv(t)
	_, srv := newRealServer(t)
	defer srv.Close()
	body := `{"kind":"launchbay-backup","version":1,"bookmarks":{"groups":[{"id":"x","title":"X","layout":"tiles","items":[]}]}}`
	path := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := runCLI([]string{"restore", "--profile", "dad", path}, cliCfg(t, srv), &out, &errBuf)
	if code != 1 {
		t.Fatalf("该退 1，实际 %d out=%q err=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "已经有") {
		t.Errorf("诊断该说清是「目标档案不空」: %q", errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("失败不该往 stdout 印东西: %q", out.String())
	}
}

// 用法错误一律退 2，且**在碰网络之前**就拒（参数不对就发一次请求是浪费）。
//
// 四种用法错误：backup 缺 --profile；restore 缺文件；restore 缺 --profile；
// restore 给了两个文件。
func TestCLIBackupRestore_用法错误退2(t *testing.T) {
	adminKeyEnv(t)
	_, srv := newRealServer(t)
	defer srv.Close()
	cfg := cliCfg(t, srv)
	for _, args := range [][]string{
		{"backup"},
		{"restore", "--profile", "dad"},
		{"restore", "x.json"},
		{"restore", "--profile", "dad", "a", "b"},
	} {
		var out, errBuf bytes.Buffer
		code := runCLI(args, cfg, &out, &errBuf)
		if code != 2 {
			t.Errorf("%v：该退 2，实际 %d out=%q err=%q", args, code, out.String(), errBuf.String())
		}
		if out.Len() != 0 {
			t.Errorf("%v：用法错误不该往 stdout 印东西: %q", args, out.String())
		}
		if errBuf.Len() == 0 {
			t.Errorf("%v：诊断该走 stderr", args)
		}
	}
}
