// Package main 是 CI 的泄漏守卫：扫仓库里被 git 跟踪的文本文件，
// 报告不在"示例地址空间"里的 IPv4 字面量。
//
// 为什么值得一条闸：本仓库挂在公开组合下，而真实内网拓扑（本案的 NAS
// 与它的 192.168. 段地址）一旦进过 git 历史就**收不回来**——那是所有
// 可能错误里唯一不可撤回的一类。spec §15 与 AGENTS.md 都写了"只用 10. 段
// 与 127.0.0.1"，这条把那句修辞变成退出码。
//
// 白名单而不是黑名单：开放集合用黑名单必漏（下一个私有段就是漏的那一个）。
// 需要放行新地址空间时改 allow 并在注释写明"它为什么是示例而不是拓扑"。
//
// **形状判据不借 net.ParseIP**（2026-10-19）：Go 从 1.17 起拒收带前导零的写法
// （那种四段式返回 nil），而本文件那条正则**照样匹配得到它**。
// 拿 ParseIP 当"这是不是地址"的判据，就等于让"该抓的"变成"抓不到的"——真实内网
// 地址按前导零形式贴进文档或夹具时守卫全绿。**失败方向是"该抓的没抓"**，比误报
// 坏得多。所以形状由 `quadLike` 自己答（四段十进制、每段不超 255），落在哪由
// `parseQuad` 答（`001` 与 `1` 是同一个数）。
//
// 判据分两层（spec §15），两层都要有证人：
//   - 第一层：示例地址空间，写死在 allow 里，任何人都能跑；
//   - 第二层：本家真实网段，从环境变量 NAS_SUBNET_PREFIX 读，CI 用 secret 注入。
//     它存在的理由是第一层**故意**看不见本家网段——白名单里写死真实网段等于把
//     拓扑抄进仓库，那正是这条闸要防的事。第二层在进程里比对，不落仓库。
//
// 第二层的三个边界，都是"不许静默失效"的形状：
//   - 未配置 → 只跳过第二层，第一层照跑，且 stderr 与 stdout 都点明**当前只有
//     一层**（fork 与普通 PR 本来就取不到 secret，静默全绿最坏）；
//   - 配错 → 退出码 2（守卫没跑成），不是"当成没配"——一个坏前缀悄悄放过整段
//     本家地址，比它压根不存在更危险；
//   - 全零前缀（`0.0.0` 那类）会放行整段地址 → 同样退出码 2。那等于把守卫关掉。
//     四段的全零写法更早一步就被"不是三段"那条拒掉，落到同一条红色上。
//
// 已知边界：抓不到半截地址（`192.168.`）、主机名、DNS 名与已经 base64 过的
// 内容。它防的是"复制粘贴了一段真配置进文档/夹具"这一种最常见的失误，
// 不防蓄意隐藏——那是 code review 的活。
//
// 它扫的是 `git ls-files`，因此**这个包自己也在扫描范围内**：仓库里不许出现
// 成形的非示例地址，守卫的夹具也不例外（要抓的地址在 main_test.go 里按八位组
// 现拼）。没有给守卫开"豁免名单"这种第二条判据——一条有例外的闸，例外长得多快
// 没人知道；而 CI 因为自己的测试语料永久红，比没闸更糟：没人信的闸等于没闸。
//
// **它真的会抓到自己人**（2026-10-19 实测）：同一天写 README 时顺手用了一个真实
// 私网段的地址当示例，那正是本文件第 5 行说的那种段，于是这条闸当场变红——
// 那批本地提交推上去 CI 就会红，而那个地址若确是真 NAS 就已经进历史了。
// **写文档里的示例地址要跑一遍这条闸**（`go run ./tools/leakguard`，退出码 0 才算
// 干净）；它比任何 review 都早知道，因为它是退出码。
//
// **而这段注释自己就是那个教训的第二遍**：修这一段时把同一个地址又写进去了一次，
// 闸当场第二次变红——所以这里只说"一个真实私网段的地址"，一个字面量都不写。
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var allow = []*net.IPNet{mustNet("10.0.0.0/8"), mustNet("127.0.0.0/8")}

// homeEnv 是第二层白名单的环境变量名。名字进仓库、值不进——值是本家拓扑。
const homeEnv = "NAS_SUBNET_PREFIX"

// scanPolicy 是两层判据的合集。**显式带 hasHome 而不是拿 homeNet 的零值当
// "未启用"**：全零前缀会被显式拒绝，可它恰好就是 `[3]int{}` 的形状——用一个真
// 会被判据读到的值去表示"判据不存在"，等于给假绿留后门。
type scanPolicy struct {
	allow   []*net.IPNet // 第一层：示例地址空间
	homeNet [3]int       // 第二层：本家网段的前三段
	hasHome bool         // 第二层有没有启用
}

// basePolicy 是只有第一层的默认形态，scan() 用它。
func basePolicy() scanPolicy { return scanPolicy{allow: allow} }

// resolveHomePrefix 解析第二层的前缀，返回 (前三段, 是否启用, 错误)。
// 恰好三段八位组；空值是"未配置"而不是错误——fork PR 取不到 secret 是常态。
func resolveHomePrefix(raw string) ([3]int, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return [3]int{}, false, nil
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		// 写 CIDR（/24）或四段单主机都落到这里。宽容地猜它想表达什么，等于
		// 让人以为第二层开着而它其实按另一个意思开着。
		return [3]int{}, false, fmt.Errorf("%s=%q 不是三段八位组（如 %s）", homeEnv, raw, "192.168.50")
	}
	var out [3]int
	for i, p := range parts {
		// 先说 CIDR：<三段>/<掩码> 拆出来恰好也是三段，长度检查放它过去，
		// 落到下面那句就报成"第 3 段不是八位组"——技术上没错，而配错的人
		// 会以为自己的网段号写错了，改三遍还是同一个红。
		if strings.ContainsAny(p, "/:") {
			return [3]int{}, false, fmt.Errorf("%s=%q 的第 %d 段是 %q——CIDR（/24）与带端口的写法都不支持，写成三段八位组（如 %s）",
				homeEnv, raw, i+1, p, "192.168.50")
		}
		// ParseUint 而不是 Atoi：后者收 "+1" 与前导空格，那两种写法的八位组
		// 没人会当成地址前缀，读到的人只会以为这条判据可信。
		n, err := strconv.ParseUint(p, 10, 8)
		if err != nil {
			return [3]int{}, false, fmt.Errorf("%s=%q 的第 %d 段不是 0-255 的八位组", homeEnv, raw, i+1)
		}
		out[i] = int(n)
	}
	if out == [3]int{} {
		// 回显用户配的原值而不是在这里写死一个示例：那串字面量本身就是这条
		// 守卫要报的形状，写进源码等于让守卫永远红。
		return [3]int{}, false, fmt.Errorf("%s=%q 三段全零会放行整段地址，等于把守卫关掉，不许当网段前缀用", homeEnv, raw)
	}
	return out, true, nil
}

// sameHome 判地址是否落在本家网段。**按数值比八位组而不是比字符串**：前缀写
// "010" 与地址写 "10" 是同一个八位组，比字符串会把它们判成两段，于是前一段
// 悄悄失效——而它失效的方向正是"该抓的没抓"。
func sameHome(ip net.IP, home [3]int) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false // IPv6：四段式正则本来就扫不到，这里只是不让它靠 nil 比中
	}
	return int(v4[0]) == home[0] && int(v4[1]) == home[1] && int(v4[2]) == home[2]
}

var ipRE = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)

// quadLike 答"这段字面量像不像一个 IPv4 地址"：四段十进制、每段不超 255。
//
// **它不借 net.ParseIP**（2026-10-19 修）：Go 从 1.17 起拒收带前导零的写法
// （那种四元组返回 nil），而 ipRE 照样匹配得到那条。拿 ParseIP 当形状
// 判据，就等于让"该抓的"变成"抓不到的"——真实内网地址按前导零形式贴进文档
// 或夹具时守卫全绿。**失败方向是"该抓的没抓"**，比误报坏得多。
//
// 版本号与超界四元组（每段 > 255）在这里被拒——它们不是地址，不该进白名单讨论。
// 前导零不是超界：`001` 与 `1` 是同一个数。
//
// **这段注释里刻意不写成形四元组**：守卫扫的是 git 跟踪的文本，而本文件就在
// 里面——逐字写一个示例空间外的地址等于自己给自己报泄漏，CI 从此永久红，
// 而一条没人信的闸等于没闸（同 main_test.go 里 `quad()` 那段注释）。
func quadLike(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
			n = n*10 + int(r-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}

// parseQuad 把四段十进制算成 IPv4。**前导零按十进制算**（`001` = 1），
// 于是「带前导零的那一条」与「不带的那一条」落到同一个网段——写的人以为是
// 哪一种，守卫就按哪一种判。
func parseQuad(s string) net.IP {
	parts := strings.Split(s, ".")
	out := make(net.IP, 4)
	for i, p := range parts {
		n := 0
		for _, r := range p {
			n = n*10 + int(r-'0')
		}
		out[i] = byte(n)
	}
	return out
}

// 按扩展名豁免的二进制产物：随机字节会凑出假四元组。
var skipExt = map[string]bool{".gz": true, ".png": true, ".ico": true, ".webp": true, ".jpg": true, ".woff2": true}

func mustNet(cidr string) *net.IPNet {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return n
}

// trackedFiles 拿要扫的文件名单：给了参数就按参数走（测试与本地定向用），
// 没给就跑 `git ls-files`——"进仓库"这件事的判据是 git 跟踪了什么，
// 不是目录里躺着什么（data/ 与 .env 是被 gitignore 的真数据，不归这里管）。
func trackedFiles(args []string) ([]string, error) {
	if len(args) > 0 {
		var out []string
		for _, a := range args {
			st, err := os.Stat(a)
			if err != nil {
				return nil, fmt.Errorf("读不到扫描目标 %s: %w", a, err)
			}
			if !st.IsDir() {
				out = append(out, a)
				continue
			}
			err = filepath.Walk(a, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() {
					out = append(out, p)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("遍历 %s: %w", a, err)
			}
		}
		return out, nil
	}
	cmd := exec.Command("git", "ls-files", "-z")
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files 失败（这不在仓库里，或者 git 没装）: %w", err)
	}
	var out []string
	for _, f := range strings.Split(string(raw), "\x00") {
		if f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("git ls-files 返回空集合——要么仓库是空的，要么这一步没真的跑起来；两种都不该被当成'干净'")
	}
	return out, nil
}

// scan 返回（泄漏处名单, 被跳过的文件名单, 错误）。
// skipped 必须交给调用方打印，而且要带**为什么**跳过：静默跳过等于给守卫开一个
// 盲区——按扩展名豁免的那几个产物，正是"少了多少没人知道"的那一类。
// scan 是"只有第一层"的便捷形状，测试里绝大多数用例要的就是它。
func scan(files []string) (findings, skipped []string, err error) {
	return scanWithPolicy(files, basePolicy())
}

// scanWithPolicy 按给定的两层判据扫。**第二层不进 scan 的内部读 env**：
// 环境变量由 run 解析好传进来，否则测试结果会随开发机的环境漂——
// 一条"本机绿、CI 红"的闸比没有闸更费时间。
func scanWithPolicy(files []string, pol scanPolicy) (findings, skipped []string, err error) {
	for _, f := range files {
		if ext := strings.ToLower(filepath.Ext(f)); skipExt[ext] {
			skipped = append(skipped, fmt.Sprintf("%s: 扩展名 %s 按二进制产物豁免", f, ext))
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, fmt.Errorf("读 %s: %w", f, err)
		}
		if !utf8.Valid(data) {
			skipped = append(skipped, f+": 非 UTF-8，没扫")
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range ipRE.FindAllString(line, -1) {
				ip := net.ParseIP(m)
				if ip == nil {
					// **ParseIP 认不出的也要报**（2026-10-19 修）：Go 从 1.17 起
					// 拒收带前导零的写法（那种四元组返回 nil），而那条正则
					// **照样能匹配到它**。于是这一处从前把它当成"版本号"静默丢掉
					// ——**失败方向是"该抓的没抓"**：真实内网地址按前导零形式贴进
					// 文档或夹具，守卫全绿。
					//
					// 判据因此分成两问：**形状**像不像地址（四段十进制、每段不超
					// 255）由本文件自己答，不再借 ParseIP；**落在哪**由 ParseIP
					// 答。前导零那种形状 ParseIP 不认，就自己按十进制段值算——
					// `001` 与 `1` 是同一个数。
					if !quadLike(m) {
						continue // 版本号、超界四元组：不是地址
					}
					ip = parseQuad(m)
					if ip == nil {
						continue
					}
				}
				if ip.To4() == nil {
					continue
				}
				inside := false
				for _, n := range pol.allow {
					if n.Contains(ip) {
						inside = true
						break
					}
				}
				if !inside && pol.hasHome && sameHome(ip, pol.homeNet) {
					inside = true
				}
				if !inside {
					findings = append(findings, fmt.Sprintf("%s:%d: %s", f, i+1, m))
				}
			}
		}
	}
	return findings, skipped, nil
}

// run 是要做的事，main 只是它的一个外壳。拆成两个是为了让**退出码这件事**有
// 自动证人：退出码是这条闸对 CI 唯一的输出语言（0 放行、1 有泄漏、2 守卫自己
// 没跑成），而 main 里的 os.Exit 谁也测不到——只跑过绿路径的闸坏掉了会一直绿。
func run(args []string, stdout, stderr io.Writer) int {
	targets, err := trackedFiles(args)
	if err != nil {
		fmt.Fprintf(stderr, "leakguard: %v\n", err)
		return 2
	}
	// 第二层先解析再扫：配错时守卫压根没跑成，那必须比"发现泄漏"还早地红。
	pol := basePolicy()
	home, ok, err := resolveHomePrefix(os.Getenv(homeEnv))
	if err != nil {
		fmt.Fprintf(stderr, "leakguard: %v\n", err)
		return 2
	}
	if ok {
		pol.homeNet, pol.hasHome = home, true
		fmt.Fprintf(stderr, "leakguard: 第二层启用——%s 精确匹配本家网段前三段\n", homeEnv)
	} else { // 措辞里刻意不用"跳过"二字：stderr 里"跳过"是"文件按扩展名/编码被豁免"
		// 那条既有判据的关键词，两种意思共用一个词，断言就分不出该期待哪一条。
		fmt.Fprintf(stderr, "leakguard: %s 未设，第二层未启用——第一层照跑，本家网段不在判据内\n", homeEnv)
	}
	findings, skipped, err := scanWithPolicy(targets, pol)
	if err != nil {
		fmt.Fprintf(stderr, "leakguard: %v\n", err)
		return 2
	}
	for _, f := range skipped {
		fmt.Fprintf(stderr, "leakguard: 跳过 %s\n", f)
	}
	if len(findings) > 0 {
		fmt.Fprintln(stdout, "leakguard: 发现不在示例地址空间里的 IPv4 字面量（真实内网地址不许进仓库，spec §15）")
		for _, f := range findings {
			fmt.Fprintln(stdout, "  "+f)
		}
		return 1
	}
	// 绿的那一行也要报层数：读日志的人第一眼看到的是这一行，它得说清这次
	// 判据有几层——否则"只有第一层"和"两层都在"在日志里长得一模一样。
	if pol.hasHome {
		fmt.Fprintf(stdout, "leakguard: %d 个文件扫过，0 处泄漏（两层判据；跳过 %d 个，逐条点名在 stderr）\n", len(targets), len(skipped))
	} else {
		fmt.Fprintf(stdout, "leakguard: %d 个文件扫过，0 处泄漏（**只有第一层**，%s 未设；跳过 %d 个，逐条点名在 stderr）\n",
			len(targets), homeEnv, len(skipped))
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
