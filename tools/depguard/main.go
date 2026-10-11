// Package main 是 CI 的依赖守卫：本仓库是**零外部依赖**的纯标准库程序，
// 守卫把"零依赖"从一句介绍变成退出码。
//
// 为什么值得一条闸：零依赖是这个项目最大的安全属性——没有 CVE 面、没有供应链、
// 没有 go.sum 可漂移、没有"某个传递依赖更新了"这类半夜告警。§11 的安全取舍
// 有一半架在这个属性上。可它当时**没有任何东西守着**：`go mod tidy` 顺手引进一个
// 包，或者某次"就用 stdlib 那个不方便的替代品吧"，CI 一声不响，而地基已经换了。
//
// 三条判据，都只读仓库自己的文件、不连网络、不需要任何外部工具：
//   - `go.sum` 不存在或为空——外部依赖必然会写它；
//   - `go.mod` 里没有 `replace` / `exclude` / `retract`——那三条能改变解析结果，
//     它们出现在一个零依赖的模块里，本身就是"这里曾经有过依赖"的指纹；
//   - `go list -m all` 只列主模块一行——这是最终判据，前两条是它的交叉验证。
//
// 三条而不是一条：任何单条都能被绕过（手写 go.sum、绕开 go 命令、手改 vendoring），
// 而三条同时成立却仍然有外部依赖，需要的不是疏忽而是刻意。**每一条都要能单独红**——
// 一个守卫把三种越界混成一句"发现外部依赖"，读日志的人得自己去猜是哪一种。
//
// 退出码是这条闸对 CI 唯一的输出语言，所以它自己得有证人：0 放行、1 有外部依赖、
// 2 守卫没跑成（读不到 go.mod、go 命令不存在……）。2 与 1 必须分开——"没查成"和
// "查过了、干净"混成同一个绿，就是一条永远绿的假闸。
//
// 它要真引入依赖时怎么办：先改 spec（§7.3 把"零依赖"写成安全属性的地方），
// 再删掉这条闸或把它降级成报告。**不许为了过闸而放宽判据**——那等于把地基
// 悄悄换成别的，而没人记得换过。
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// mainModule 是本仓库的模块路径。它同时是 `go list -m all` 里**不带版本号的那一行**
// ——判据"只列主模块一行"就是拿它当锚点，而不是假设"第一行总是主模块"。
const mainModule = "github.com/rockbenben/launchbay"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// report 是一次检查的结论：每条 finding 一处越界，逐条说明是哪条判据、哪个文件。
type report struct {
	findings []string
	checked  int // 实际读到并判过的判据条数，用来防"扫了 0 个文件"
}

// inspect 在 dir 里跑三条判据。
func inspect(dir string) (report, error) {
	var rep report

	// 判据一：go.sum。读不到"不存在"是正常的（零依赖时它压根不该有），
	// 读不到但原因是别的那一种（比如是目录）才要报错。
	//
	// **不存在也计数**：这条判据的结论之一就是"它不在"，而"不在"同样是判过的
	// 形状。不计数的话，真实零依赖仓库只会被记成读过两条——报告里那句
	// "N 条判据"就成了假话，而它是这条闸唯一的自证。
	sum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	switch {
	case err == nil:
		rep.checked++
		if len(bytes.TrimSpace(sum)) > 0 {
			rep.findings = append(rep.findings, fmt.Sprintf(
				"go.sum 非空（%d 字节）：零依赖的仓库不该有它，它存在即说明有外部依赖", len(bytes.TrimSpace(sum))))
		}
	case os.IsNotExist(err):
		rep.checked++ // 正是期望的形状
	default:
		return rep, fmt.Errorf("读 go.sum: %w", err)
	}

	// 判据二：go.mod 里的改判指令。
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return rep, fmt.Errorf("读 go.mod: %w", err)
	}
	rep.checked++
	rep.findings = append(rep.findings, directiveFindings(gomod)...)

	// 判据三：go list -m all，最终判据。
	out, err := listModules(dir)
	if err != nil {
		return rep, err
	}
	rep.checked++
	rep.findings = append(rep.findings, moduleFindings(out)...)

	if rep.checked == 0 {
		return rep, fmt.Errorf("一条判据都没读到——那不是'干净'，是这条闸没跑起来")
	}
	return rep, nil
}

// directiveFindings 找出 go.mod 里的 replace / exclude / retract。
//
// 判据是**行首的关键字**，不是"文件里出现过这个词"：go.mod 的 require 行里可能有
// 一个名字带 replace 的模块（真出现过——模块路径里带这些词是合法的），按包含关系
// 判会把它误报成改判指令，而一条会误报的闸第一次红的时候就被关掉了。
func directiveFindings(gomod []byte) []string {
	var out []string
	for i, line := range strings.Split(string(gomod), "\n") {
		trimmed := strings.TrimSpace(line)
		// 单行写法 `replace foo => bar v1` 也存在，带关键字照样收。
		for _, kw := range []string{"replace", "exclude", "retract"} {
			if strings.HasPrefix(trimmed, kw) {
				out = append(out, fmt.Sprintf("go.mod:%d: %s 指令——零依赖的模块里不该有它", i+1, kw))
				break
			}
		}
	}
	return out
}

// moduleFindings 解析 `go list -m all` 的输出，返回每一处越界。
//
// 输出每行一个模块：主模块那行**不带版本号**，其余都带。主模块不在输出里时
// 同样要报——那说明判定用的锚点变了（模块改名或换路径），让守卫继续按旧锚点
// 判下去才是危险的。
func moduleFindings(out string) []string {
	var mods []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			mods = append(mods, line)
		}
	}
	if len(mods) == 0 {
		return []string{fmt.Sprintf("`go list -m all` 返回空集合——要么这里不是模块，要么它没跑起来；两种都不该当成'干净'")}
	}
	sawMain := false
	var findings []string
	for _, m := range mods {
		name, version, _ := strings.Cut(m, " ")
		if name == mainModule {
			sawMain = true
			continue // 主模块自己不是外部依赖
		}
		if version == "" {
			findings = append(findings, fmt.Sprintf("外部模块 %q（这一行不带版本号，读起来不像依赖项）", name))
			continue
		}
		findings = append(findings, fmt.Sprintf("外部依赖 %s %s——引入它是一次改判：先改 spec，再决定放不放行", name, version))
	}
	if !sawMain {
		findings = append(findings, fmt.Sprintf(
			"`go list -m all` 的输出里没有主模块 %s——模块改名或换路径了，这条守卫的锚点该跟着改", mainModule))
	}
	return findings
}

// listModules 在 dir 里跑 go list -m all。
func listModules(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "all")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		// go 命令不在、目录不是模块、网络不通——都归"守卫没跑成"，
		// 不是"没有外部依赖"。这两件事混起来就是一条永远绿的闸。
		return "", fmt.Errorf("`go list -m all` 跑不起来（%v）: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func run(args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	rep, err := inspect(dir)
	if err != nil {
		fmt.Fprintf(stderr, "depguard: %v\n", err)
		return 2
	}
	if len(rep.findings) > 0 {
		fmt.Fprintln(stdout, "depguard: 发现外部依赖——本仓库的定位是零依赖的标准库程序，引入依赖是一次改判（见 spec §7.3）")
		for _, f := range rep.findings {
			fmt.Fprintln(stdout, "  "+f)
		}
		return 1
	}
	fmt.Fprintf(stdout, "depguard: 零依赖确认（%d 条判据：go.sum / go.mod 指令 / go list -m all；模块 %s）\n",
		rep.checked, mainModule)
	return 0
}
