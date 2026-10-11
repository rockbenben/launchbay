package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMod 在临时目录里落一个 go.mod（及可选 go.sum），返回该目录。
func writeMod(t *testing.T, gomod, gosum string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	if gosum != "" {
		if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte(gosum), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// cleanMod 是一个合法的零依赖 go.mod。
const cleanMod = "module " + mainModule + "\n\ngo 1.27\n"

// 判据二：改判指令只认行首关键字。
//
// wantHit 那几支是防假绿的对照：把 directiveFindings 改成恒返回 nil，上面 want>0
// 的用例会红；改成"只要文件里出现这个词就报"，wantHit 那几支会红。两种坏法都被钉住。
func TestDirectiveFindings_只认行首关键字(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
		wantHit    string
	}{
		{"干净放行", cleanMod, 0, ""},
		{"require 放行", "module " + mainModule + "\n\ngo 1.27\n\nrequire golang.org/x/text v0.3.0\n", 0, ""},
		{"replace 块要报", cleanMod + "\nreplace (\n\tgolang.org/x/text => ./local\n)\n", 1, "replace"},
		{"单行 replace 要报", cleanMod + "\nreplace golang.org/x/text => ./local\n", 1, "replace"},
		{"exclude 要报", cleanMod + "\nexclude golang.org/x/text v0.3.0\n", 1, "exclude"},
		{"retract 要报", cleanMod + "\nretract v0.1.0\n", 1, "retract"},
		{"三处要报三条", cleanMod + "\nreplace a => b\nexclude c v1\nretract v0.1.0\n", 3, "retract"},
		// **模块路径里带这些词不算**：真实存在这样的模块名。判据是行首关键字，
		// 不是"文件里出现过这个词"——一条会误报的闸第一次红就被人关掉了。
		{"路径里带 replace 不算", "module " + mainModule + "\n\ngo 1.27\n\nrequire example.com/replace/foo v1.0.0\n", 0, ""},
		{"注释里的不算", cleanMod + "\n// replace 曾经在这里\n", 0, ""},
		{"缩进的块内条目要报", cleanMod + "\nreplace (\n\tgolang.org/x/text => ./local\n)\n", 1, "replace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := directiveFindings([]byte(tc.body))
			if len(got) != tc.want {
				t.Fatalf("处数 = %d，要 %d：\n%s", len(got), tc.want, strings.Join(got, "\n"))
			}
			if tc.wantHit != "" && !strings.Contains(strings.Join(got, "\n"), tc.wantHit) {
				t.Errorf("报出来的那条要含 %q，实际 %v", tc.wantHit, got)
			}
			// 报告必须点名是 go.mod 的第几行，否则读日志的人得自己去搜。
			for _, g := range got {
				if !strings.Contains(g, "go.mod:") {
					t.Errorf("每条都要带文件名与行号：%q", g)
				}
			}
		})
	}
}

// 判据三：`go list -m all` 的输出解析。
//
// 这里最容易坏成假绿的是"锚点写错还继续判"——主模块名改了而守卫还按旧名判，
// 于是主模块被当成外部依赖（太吵，会被发现）或被静默放过（太静，不会被发现）。
// 所以"输出里没有主模块"必须单独是一条红，而不是"没匹配到就跳过"。
func TestModuleFindings_主模块之外每条都报(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
		wantHit    string
	}{
		{"只有主模块放行", mainModule + "\n", 0, ""},
		{"主模块带空行也放行", "\n" + mainModule + "\n\n", 0, ""},
		{"一个依赖要报", mainModule + "\ngolang.org/x/text v0.3.0\n", 1, "golang.org/x/text v0.3.0"},
		{"两个依赖要报两条", mainModule + "\ngolang.org/x/text v0.3.0\ngithub.com/spf13/cobra v1.8.0\n", 2, "cobra"},
		// 主模块不在输出里：守卫的锚点过期了。这是红，不是绿。
		{"没有主模块要报", "golang.org/x/text v0.3.0\n", 2, "锚点该跟着改"},
		{"空输出要报", "", 1, "没跑起来"},
		{"只有空白的输出要报", "  \n\t\n", 1, "没跑起来"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := moduleFindings(tc.body)
			if len(got) != tc.want {
				t.Fatalf("处数 = %d，要 %d：\n%s", len(got), tc.want, strings.Join(got, "\n"))
			}
			if tc.wantHit != "" && !strings.Contains(strings.Join(got, "\n"), tc.wantHit) {
				t.Errorf("报出来的那条要含 %q，实际 %v", tc.wantHit, got)
			}
		})
	}
}

// 三条判据走在一起。**每条都要能单独红**——守卫把三种越界混成一句
// "发现外部依赖"，读日志的人得自己去猜是哪一种。
func TestInspect_三条判据各自独立(t *testing.T) {
	const sumBody = "golang.org/x/text v0.3.0 h1:abc=\ngolang.org/x/text v0.3.0/go.mod h1:def=\n"
	withDep := "module " + mainModule + "\n\ngo 1.27\n\nrequire golang.org/x/text v0.3.0\n"
	withReplace := cleanMod + "\nreplace golang.org/x/text => ./local\n"
	for _, tc := range []struct {
		name    string
		gomod   string
		gosum   string
		want    int
		wantHit string
	}{
		{"全干净", cleanMod, "", 0, ""},
		// go.sum 非空但 go.mod 没有 require：手写或残留的 go.sum，同样是红。
		{"只有 go.sum 要报", cleanMod, sumBody, 1, "go.sum 非空"},
		{"只有 replace 要报", withReplace, "", 1, "replace"},
		{"go.sum 与 replace 都报", withReplace, sumBody, 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeMod(t, tc.gomod, tc.gosum)
			rep, err := inspect(dir)
			if err != nil {
				t.Fatalf("不该报错：%v", err)
			}
			if len(rep.findings) != tc.want {
				t.Fatalf("处数 = %d，要 %d：\n%s", len(rep.findings), tc.want, strings.Join(rep.findings, "\n"))
			}
			if tc.wantHit != "" && !strings.Contains(strings.Join(rep.findings, "\n"), tc.wantHit) {
				t.Errorf("报出来的那条要含 %q，实际 %v", tc.wantHit, rep.findings)
			}
			// 三条判据都要真的读过（go.sum 不存在时它也是被判过的形状之一，
			// 但另两条必须在）。checked 掉到 2 以下说明有条判据悄悄没跑。
			if rep.checked < 2 {
				t.Errorf("只读到 %d 条判据，少于应有的 2 条：%v", rep.checked, rep.findings)
			}
		})
	}
	// 有 require 的 go.mod：go list -m all 会报出那个模块——但这台机器上
	// 模块没下载过，go list 属于"跑不起来"（退出 2）而不是"发现依赖"。
	// 两种都红，只是红的原因不同，所以这里断言的是**非零结论**，不钉具体码。
	t.Run("有 require 的 go.mod 必须红", func(t *testing.T) {
		dir := writeMod(t, withDep, sumBody)
		rep, err := inspect(dir)
		if err == nil && len(rep.findings) == 0 {
			t.Fatal("go.mod 里有外部依赖，守卫却什么都没说")
		}
	})
}

// 退出码是这条闸对 CI 唯一的输出语言，所以它自己得有证人：main 里的 os.Exit
// 测不到，CI 又只跑得绿路径——把 2 写成 0，仓库里从此一条永远绿的假闸。
// run 把三档都变成可断言的返回值。
func TestRun_退出码三档各有证人(t *testing.T) {
	cleanDir := writeMod(t, cleanMod, "")
	sumBody := "golang.org/x/text v0.3.0 h1:abc=\n"
	dirtyDir := writeMod(t, cleanMod, sumBody)
	emptyDir := t.TempDir()
	notAModule := filepath.Join(t.TempDir(), "根本没有go.mod")
	for _, tc := range []struct {
		name string
		dir  string
		want int
	}{
		{"零依赖放行 0", cleanDir, 0},
		{"有 go.sum 报 1", dirtyDir, 1},
		{"目录里没有 go.mod：守卫没跑成 2", emptyDir, 2},
		{"路径不存在：守卫没跑成 2", notAModule, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run([]string{tc.dir}, &out, &errOut); got != tc.want {
				t.Fatalf("退出码 = %d，要 %d\nstdout=%q stderr=%q", got, tc.want, out.String(), errOut.String())
			}
			switch tc.want {
			case 0:
				// 放行那一行必须说清判了几条——守卫绿着的时候最容易忘记少了什么。
				if !strings.Contains(out.String(), "零依赖确认") {
					t.Errorf("放行要说清判据条数：%q", out.String())
				}
			case 1:
				if out.Len() == 0 {
					t.Error("发现外部依赖要在 stdout 逐条列出来")
				}
			case 2:
				if errOut.Len() == 0 {
					t.Error("守卫自己没跑成要说清为什么，不能闷声返回")
				}
			}
		})
	}
}

// 这个仓库**当下**就是零依赖的。这条用例把上面所有纯函数钉在真实仓库上：
// 哪天有人 `go get` 进来一个包，它当场红——而修它的正确动作是走 spec 改判，
// 不是放宽判据。
func TestInspect_本仓库当下是零依赖(t *testing.T) {
	// 测试的进程目录是 tools/depguard，模块根在两级之上。
	root := filepath.Join("..", "..")
	rep, err := inspect(root)
	if err != nil {
		t.Fatalf("本仓库检查没跑成：%v", err)
	}
	if len(rep.findings) != 0 {
		t.Fatalf("本仓库不是零依赖了（共 %d 处）：\n%s\n\n引入依赖是一次改判：先改 spec §7.3，再决定这条闸怎么改。",
			len(rep.findings), strings.Join(rep.findings, "\n"))
	}
	if rep.checked < 3 {
		t.Errorf("只读到 %d 条判据，少于三条——有判据没跑起来，'零依赖'这个结论不完整", rep.checked)
	}
}
