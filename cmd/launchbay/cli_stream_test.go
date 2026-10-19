package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/model"
)

// CLI 的两条流（2026-10-19）。这一格从前的形状是**只有一个 writer**：成功的结论、
// 用法错误、服务端拒绝、`-h` 帮助全都走 stdout，于是
//
//	launchbay revert --profile dad --to 0 | tee run.log
//
// 把一句用法错误写进了 run.log，而脚本读 stdout 时没法把"没成"与"成了"分开。
// 顶层 preConfigDispatch 早就两个流了（--help→stdout/0、未知子命令→stderr/2），
// healthcheck 的探活失败也走 os.Stderr——同一个进程里三套规矩。
//
// **分流判据只有一条，不要另立标准**：这句话是在**报告成功**，还是在**解释为什么
// 没成功**。于是下面三格各自钉住一头：
//
//	TestCLISuccessGoesToStdout	成功结论、导入报告、dry-run 计划、子命令 -h → stdout
//	TestCLIFailureGoesToStderr	用法错误、配置缺令牌、服务端拒绝、"已经有 N 条" → stderr
//	TestCLISubcommandHelpIsNotAUsageError	`-h` 与顶层 --help 同码（0），与用法错误分开
//
// 每一格都同时断言**另一个流是空的**：只断言"诊断在 stderr"不够——一句诊断被
// 印两遍（一遍 stdout 一遍 stderr）也照样满足它，而那正是把 stdout 当数据源的
// 脚本最难排除的形状。

// runCLI 的两个出口。失败那族要求 stdout **一个字节都没有**。
type cliStreams struct {
	out, err bytes.Buffer
}

func (s *cliStreams) run(t *testing.T, cfg config, args ...string) int {
	t.Helper()
	return runCLI(args, cfg, &s.out, &s.err)
}

// assertSuccessToStdout 断言"成功的话只在 stdout 上，且 stderr 一个字都没有"。
func (s *cliStreams) assertSuccessToStdout(t *testing.T, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(s.out.String(), w) {
			t.Errorf("成功的话该在 stdout 上且含 %q，实际 out=%q", w, s.out.String())
		}
	}
	if s.err.Len() != 0 {
		t.Errorf("成功的命令不该往 stderr 印东西: %q", s.err.String())
	}
}

// assertFailureToStderr 断言"失败的话只在 stderr 上，且 stdout 一个字节都没有"。
func (s *cliStreams) assertFailureToStderr(t *testing.T, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(s.err.String(), w) {
			t.Errorf("诊断该在 stderr 上且含 %q，实际 err=%q", w, s.err.String())
		}
	}
	if s.out.Len() != 0 {
		t.Errorf("失败的命令不该往 stdout 印东西（读 stdout 的脚本会把它当结果）: %q", s.out.String())
	}
}

// —— 成功族：结论、报告、dry-run 计划 ——

// 四条子命令的成功出口各钉一次。为什么不能只钉 revert：这一族错分的代价是
// **成功被当成失败**（结论进了 stderr，管道与脚本读不到）与**失败被当成成功**
// （诊断留在 stdout），而每条命令的"成功"形状不一样——一句话、一份报告、一串
// 计划、一个"确实没得改"。
func TestCLISuccessGoesToStdout(t *testing.T) {
	t.Run("revert 的结论", func(t *testing.T) {
		adminKeyEnv(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"profile":"dad","rev":3,"restoredFrom":1}`))
		}))
		defer srv.Close()

		var s cliStreams
		if code := s.run(t, cliCfg(t, srv), "revert", "--profile", "dad", "--to", "1"); code != 0 {
			t.Fatalf("该退 0，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		s.assertSuccessToStdout(t, "已恢复到 rev 1")
	})

	t.Run("profile create 的结论", func(t *testing.T) {
		_, srv := newRealServer(t)
		defer srv.Close()
		adminKeyEnv(t)

		var s cliStreams
		if code := s.run(t, cliCfg(t, srv), "profile", "create",
			"--id", "mom", "--title", "妈妈", "--pin", "1234"); code != 0 {
			t.Fatalf("该退 0，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		s.assertSuccessToStdout(t, "已建档 mom")
	})

	t.Run("fix-names 没得改也是成功", func(t *testing.T) {
		// **这一句退的是 0**，所以它属于 stdout 那一族：「档案 dad 里没有名字就是
		// 地址的条目」是一条结论（跑完了，确实没有），不是诊断。把它挪进 stderr
		// 会让"整理过了，没什么可改"看起来像失败。
		_, srv := newRealServer(t)
		defer srv.Close()
		adminKeyEnv(t)

		var s cliStreams
		if code := s.run(t, cliCfg(t, srv), "profile", "fix-names", "--profile", "dad"); code != 0 {
			t.Fatalf("该退 0，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		s.assertSuccessToStdout(t, "没有名字就是地址的条目")
	})

	t.Run("fix-names --dry-run 的计划", func(t *testing.T) {
		s, srv := newRealServer(t)
		defer srv.Close()
		adminKeyEnv(t)
		// 给 dad 一条「名字就是地址」的条目（10. 段示例地址），正是这条命令存在的形状。
		if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 2, Title: "发射台",
			Groups: []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList,
				Items: []model.Item{{ID: "dsm", Name: "http://10.0.0.5:8096",
					URL: "http://10.0.0.5:8096"}}}}}); err != nil {
			t.Fatal(err)
		}

		var s2 cliStreams
		if code := s2.run(t, cliCfg(t, srv), "profile", "fix-names",
			"--profile", "dad", "--dry-run"); code != 0 {
			t.Fatalf("该退 0，实际 %d out=%q err=%q", code, s2.out.String(), s2.err.String())
		}
		s2.assertSuccessToStdout(t, "10.0.0.5", "共 1 条", "（--dry-run：没有提交）")
	})

	t.Run("import-bookmarks --dry-run 的报告与计划", func(t *testing.T) {
		s, srv := newRealServer(t)
		defer srv.Close()
		adminKeyEnv(t)
		// 导入只许导进**空档案**（见 cmdImportBookmarks 的注释），所以先把 dad 清空。
		if err := s.SaveBookmarks("dad", &model.Bookmarks{Rev: 2, Title: "发射台"}); err != nil {
			t.Fatal(err)
		}

		var s2 cliStreams
		if code := s2.run(t, cliCfg(t, srv), "import-bookmarks", "--profile", "dad",
			"--dry-run", writeBookmarksHTML(t)); code != 0 {
			t.Fatalf("该退 0，实际 %d out=%q err=%q", code, s2.out.String(), s2.err.String())
		}
		s2.assertSuccessToStdout(t, "将要提交", "（--dry-run：没有提交）")
	})
}

// —— 失败族：用法、配置、请求、服务端 ——

// 四族失败各钉一次。挑的代表是**每一族各自最容易写错的那一句**：
//
//	用法错误      `--to 0` 与"没给子命令"——它们不是"帮助"，是"你打错了"
//	配置错误      作用域里没有这一份的令牌（**连一次请求都不该发**）
//	服务端拒绝    503——detail 要原样带出来，且不许混进 stdout
//	导入门槛      「档案里已经有 N 条」——退 1 的拒绝，与 dry-run 的计划同一族形状
func TestCLIFailureGoesToStderr(t *testing.T) {
	t.Run("用法错误：--to 0", func(t *testing.T) {
		var s cliStreams
		if code := s.run(t, config{Port: "7600"}, "revert", "--profile", "dad", "--to", "0"); code != 2 {
			t.Fatalf("该退 2，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		s.assertFailureToStderr(t, "用法: launchbay revert")
	})

	t.Run("用法错误：不认识的旗标", func(t *testing.T) {
		var s cliStreams
		if code := s.run(t, config{Port: "7600"},
			"revert", "--profile", "dad", "--to", "1", "--pin", "1234"); code != 2 {
			t.Fatalf("该退 2，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		// flag 包自己产出的那句必须原样在 stderr 上（flag 自己写了用法，全在。
		// 从前这句话跟着 fs.SetOutput 走，而 SetOutput 只能一刀切）。
		s.assertFailureToStderr(t, "not defined", "-to")
	})

	t.Run("用法错误：profile 底下没给子命令", func(t *testing.T) {
		var s cliStreams
		if code := s.run(t, config{Port: "7600"}, "profile"); code != 2 {
			t.Fatalf("该退 2，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		s.assertFailureToStderr(t, "用法", "launchbay profile create")
	})

	t.Run("用法错误：PIN 两个都不给 / 两个都给", func(t *testing.T) {
		for _, args := range [][]string{
			{"profile", "create", "--id", "tv", "--title", "客厅"},
			{"profile", "create", "--id", "tv", "--title", "客厅", "--pin", "1234", "--nopass"},
		} {
			var s cliStreams
			if code := s.run(t, config{Port: "7600"}, args...); code != 2 {
				t.Fatalf("%v：该退 2，实际 %d out=%q err=%q", args, code, s.out.String(), s.err.String())
			}
			s.assertFailureToStderr(t, "--pin")
		}
	})

	t.Run("用法错误：import-bookmarks 参数个数不对", func(t *testing.T) {
		var s cliStreams
		if code := s.run(t, config{Port: "7600"},
			"import-bookmarks", "--profile", "dad", "a.html", "b.html"); code != 2 {
			t.Fatalf("该退 2，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		// 旗标说明（fs.PrintDefaults）属于用法的一部分，跟着走 stderr——它是
		// parseFlags 收尾那一句 fs.SetOutput(errOut) 的活儿。
		s.assertFailureToStderr(t, "用法: launchbay import-bookmarks", "-profile")
	})

	t.Run("配置错误：作用域里没有这一份的令牌", func(t *testing.T) {
		// 这一族**连一次请求都不该发**，所以不需要服务：一个假服务只会把
		// "这一格真的没碰网络" 变成一句愿望。
		scopes, err := auth.ParseScopes("ai-dad:tok-dad:dad")
		if err != nil {
			t.Fatal(err)
		}
		var s cliStreams
		if code := s.run(t, config{Port: "7600", Tokens: scopes},
			"revert", "--profile", "ghost", "--to", "1"); code != 1 {
			t.Fatalf("该退 1，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		s.assertFailureToStderr(t, "配置里没有作用域为 ghost 的令牌")
	})

	t.Run("服务端拒绝：503", func(t *testing.T) {
		adminKeyEnv(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":503,"detail":"服务正在维护，恢复已取消"}`))
		}))
		defer srv.Close()

		var s cliStreams
		if code := s.run(t, cliCfg(t, srv), "revert", "--profile", "dad", "--to", "1"); code != 1 {
			t.Fatalf("该退 1，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		s.assertFailureToStderr(t, "服务正在维护")
	})

	t.Run("导入门槛：档案里已经有书签", func(t *testing.T) {
		_, srv := newRealServer(t)
		defer srv.Close()
		adminKeyEnv(t)
		// newRealServer 的夹具 dad 带一条书签——那就是"已经有 N 条"这个形状，
		// 而它退的是 1：**拒绝**属于失败诊断，哪怕人读得懂它也是一句话劝告。
		var s cliStreams
		if code := s.run(t, cliCfg(t, srv), "import-bookmarks", "--profile", "dad",
			writeBookmarksHTML(t)); code != 1 {
			t.Fatalf("该退 1，实际 %d out=%q err=%q", code, s.out.String(), s.err.String())
		}
		// **这一格 stdout 不该是空的**——它是全案唯一一处"正常输出与失败诊断
		// 同一趟里并存"：报告（迁了什么、没迁什么）在任何失败之前就打了（见
		// cmdImportBookmarks 那句注释），它是正常输出，不是诊断。所以这里断言的
		// 不是"stdout 空"，而是"报告在 stdout 上、拒绝在 stderr 上、两者不串"。
		if !strings.Contains(s.err.String(), "已经有 1 条书签") {
			t.Errorf("拒绝该在 stderr 上，实际 err=%q", s.err.String())
		}
		if !strings.Contains(s.out.String(), "浏览器书签导入报告") {
			t.Errorf("报告是正常输出，该留在 stdout 上，实际 out=%q", s.out.String())
		}
		if strings.Contains(s.err.String(), "浏览器书签导入报告") {
			t.Errorf("报告被印进了 stderr（它不是诊断）: %q", s.err.String())
		}
		if strings.Contains(s.out.String(), "已经有 1 条书签") {
			t.Errorf("拒绝混进了 stdout（读 stdout 的脚本会把它当结果）: %q", s.out.String())
		}
	})
}

// —— `-h` 是求帮助，不是打错 ——

// `flag.ErrHelp` 与「用法错误」都由 flag 包产出、写到同一个 `fs.Output()`，所以
// **`fs.SetOutput` 一刀切不了**（切 stderr 则正常帮助混进错误流，切 stdout 则
// 用法错误混进结果流）。parseFlags 先收进 buffer 再按返回值分流，这一格钉它的
// 两头：**`-h` → stdout + 退 0；别的错 → stderr + 退 2**。
//
// 退 0 是这一格新定的：从前子命令 `-h` 退 2、顶层 `--help` 退 0，同一个动作
// 两种码。定成 0 的理由是 `usage()` 最后一行明确宣传了
// 「每个子命令自己的开关：launchbay <子命令> -h」——求帮助是一次成功的求助。
func TestCLISubcommandHelpIsNotAUsageError(t *testing.T) {
	for _, tc := range []struct {
		args []string
		// 帮助文本里该出现的那面旗标：证明打印的是**它自己**的用法，
		// 而不是碰巧落进 stdout 的一句别的话。
		flag string
	}{
		{[]string{"revert", "-h"}, "-to"},
		{[]string{"revert", "--help"}, "-to"},
		{[]string{"profile", "create", "-h"}, "-nopass"},
		{[]string{"profile", "rebuild", "-h"}, "-nopass"},
		{[]string{"profile", "fix-names", "-h"}, "-dry-run"},
		{[]string{"import-bookmarks", "-h"}, "-overrides"},
	} {
		var s cliStreams
		code := s.run(t, config{Port: "7600"}, tc.args...)
		if code != 0 {
			t.Errorf("%v：求帮助该退 0（与顶层 --help 一致），实际 %d out=%q err=%q",
				tc.args, code, s.out.String(), s.err.String())
			continue
		}
		if !strings.Contains(s.out.String(), tc.flag) {
			t.Errorf("%v：帮助该在 stdout 上且含 %q，实际 out=%q",
				tc.args, tc.flag, s.out.String())
		}
		if s.err.Len() != 0 {
			t.Errorf("%v：求帮助不该往 stderr 印东西: %q", tc.args, s.err.String())
		}
	}

	// 正对照：**同一个子命令**的用法错误仍然是 stderr + 退 2。
	// 没有这一半，"该退 0" 也可以是"这一格对任何输入都退 0"。
	var s cliStreams
	if code := s.run(t, config{Port: "7600"}, "revert", "--profile", "dad", "--to", "0"); code != 2 {
		t.Fatalf("同一条命令的用法错误该退 2，实际 %d out=%q err=%q",
			code, s.out.String(), s.err.String())
	}
	s.assertFailureToStderr(t, "用法")

	// 与顶层 `--help` 对齐的那道正对照（preConfigDispatch 那一侧一个字没动，
	// 由 TestPreConfigDispatch 钉；这里只钉"两边同码"这个跨层判据）。
	var topOut, topErr bytes.Buffer
	if act, code := preConfigDispatch([]string{"--help"}, &topOut, &topErr); act != dispatchExit || code != 0 {
		t.Fatalf("顶层 --help 该退 0，实际 %v/%d", act, code)
	}
}

// writeBookmarksHTML 摆一份最小可导入的书签文件（1 条 https 书签）。
// 域名只用 example.com 与 10. 段示例地址。
func writeBookmarksHTML(t *testing.T) string {
	t.Helper()
	const body = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>Bookmarks</TITLE>
<DL><p>
<DT><A HREF="https://example.com/a">甲</A>
<DT><A HREF="http://10.0.0.5:8096/y">乙</A>
</DL><p>`
	p := filepath.Join(t.TempDir(), "bookmarks.html")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
