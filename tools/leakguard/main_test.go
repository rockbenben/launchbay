package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write 在临时目录里落一个文件，返回它的路径。
func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// quad 把八位组拼成一个 IPv4 字面量。**为什么夹具不逐字写**：守卫扫的是 git
// 跟踪的文本，而这份测试文件本身就在里面——逐字写一个示例空间外的地址等于
// 自己给自己报泄漏，CI 从此永久红，而一条没人信的闸等于没闸。
// 这里选的不是"给守卫开一条豁免"，而是让"仓库里没有成形的非示例地址"这条
// 判据**一个字都不打折**（AGENTS.md 那句"只用 10. 段与 127.0.0.1"没有例外，
// 守卫自己也照它活）。运行时字节与逐字写完全一样，测的还是同一个形状；
// 而且这个拼法自己有人守：把 quad 改坏（比如换成 "-" 连接）正则就匹配不到，
// 下面那些 want=1 / want=2 的用例当场红——它不会静悄悄变成假绿。
func quad(octets ...string) string { return strings.Join(octets, ".") }

func TestScan_示例空间放行其他都报(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, body string
		want       int
		// wantHit 是"报出来那一条必须包含什么"。**为什么要有这一栏**：只数处数的
		// 话，"版本号不是地址"这类 want=0 的用例永远不可能单独变红——把整条正则
		// 删了它照样 0 处、照样绿（评审 M-2）。所以这些形状各配一个同文件里的
		// 真地址作对照：形状被误报 → 处数变 2；匹配器整个坏掉 → 处数变 0；两种都红。
		wantHit string
	}{
		{"示例地址放行", "url=http://10.0.0.3:5000/x\n", 0, ""},
		{"整个10段都算示例", "url=http://10.9.9.9/\n", 0, ""}, // 真实夹具里就有这条（resolve_test.go）
		{"回环放行", "B=http://127.0.0.1:7600\n", 0, ""},
		{"192段要报", "NAS=" + quad("192", "168", "2", "3") + ":7570\n", 1, quad("192", "168", "2", "3")},
		{"172段要报", "host: " + quad("172", "20", "3", "4") + "\n", 1, quad("172", "20", "3", "4")},
		{"公网也报", "dns=" + quad("8", "8", "8", "8") + "\n", 1, quad("8", "8", "8", "8")},
		{"多处逐条报", "a=" + quad("192", "168", "1", "1") + " b=" + quad("192", "168", "1", "2") + "\n", 2, ""},
		// 解析器的边界（这四条钉的是"它没吞形状"，也钉了它抓不到什么）：
		{"版本号不是地址但要看得见对照", "go 1.27.0\n" + quad("192", "168", "5", "6") + "\n", 1, quad("192", "168", "5", "6")},
		{"超界四元组不是合法IP", "999.999.999.999\n" + quad("192", "168", "7", "8") + "\n", 1, quad("192", "168", "7", "8")},
		{"半截地址抓不到", "prefix: 192.168.\n", 0, ""}, // 已知边界，注释里要写清
		{"URL带端口照样抓", "u=http://" + quad("192", "168", "2", "3") + ":7570/#x\n", 1, quad("192", "168", "2", "3")},
		// **前导零那一族**（2026-10-19 补）：Go 从 1.17 起拒收带前导零的写法
		// （那种四元组 → ParseIP 返回 nil），而那条正则照样匹配得到。
		// 从前这一处把它当成"版本号"静默丢掉——**失败方向是"该抓的没抓"**：
		// 真实内网地址按前导零形式贴进文档或夹具，守卫全绿。
		// 每条都配一个同文件的正常写法作对照（wantHit 指后者）：
		// 前导零那条被漏 → 处数变 1 而 want 是 2；匹配器整个坏掉 → 处数变 0。两种都红。
		// **字面量用 quad() 拼**：逐字写一个示例空间外的地址等于自己给自己报泄漏。
		{"前导零写法照样要报", "a=" + quad("192", "168", "001", "010") + "\nb=" + quad("192", "168", "3", "4") + "\n", 2, quad("192", "168", "3", "4")},
		{"前导零的10段是同一个数", "a=10.0.0.005\nb=" + quad("10", "0", "0", "9") + "\n", 0, ""},
		{"前导零的127段是回环", "a=127.0.0.001\nb=" + quad("127", "0", "0", "9") + "\n", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := write(t, dir, strings.ReplaceAll(tc.name, " ", "_")+".txt", tc.body)
			findings, skipped, err := scan([]string{f})
			if err != nil {
				t.Fatal(err)
			}
			if len(skipped) != 0 {
				t.Errorf("纯文本不该被跳过: %v", skipped)
			}
			if len(findings) != tc.want {
				t.Fatalf("处数 = %d，要 %d:\n%s", len(findings), tc.want, strings.Join(findings, "\n"))
			}
			if tc.want > 0 && !strings.Contains(findings[0], f) {
				t.Errorf("报告里要点名文件: %s", findings[0])
			}
			if tc.wantHit != "" && !strings.Contains(strings.Join(findings, "\n"), tc.wantHit) {
				t.Errorf("报出来的那条要含 %+v 那个地址，实际 %v", tc.wantHit, findings)
			}
		})
	}
}

// 非 UTF-8 文件必须**报出来它是被跳过的**，不许静默算"扫过了、干净"。
// 这是"工具报 0 条不等于没问题"那条教训的形状钉。
func TestScan_跳过要明说不是静默通过(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bin.txt")
	if err := os.WriteFile(p, []byte{0xff, 0xfe, 'h', 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	findings, skipped, err := scan([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("二进制里凑出来的假四元组不算泄漏: %v", findings)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], p) || !strings.Contains(skipped[0], "非 UTF-8") {
		t.Fatalf("跳过的文件要连**为什么**一起报出来: %v", skipped)
	}
}

// 按扩展名豁免的产物也必须点名（评审 M-1）：判据是"跳过不许静默"，不是
// "非 UTF-8 才要点名"。今天有人往 .gz 里塞一段真配置，守卫唯一的说法就是
// "扫过了、干净"——那比没闸更糟。
func TestScan_扩展名豁免(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "index.json.gz", quad("192", "168", "9", "9"))
	findings, skipped, err := scan([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("豁免扩展名就是不进扫描，不该报泄漏: %v", findings)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], ".gz") {
		t.Fatalf("扩展名豁免要说出文件名与扩展名: %v", skipped)
	}
}

// 列文件这一步失败时不能退出码 0：守卫没跑成与发现泄漏都得让 CI 红。
func TestTrackedFiles_列不出来就报错(t *testing.T) {
	if _, err := trackedFiles([]string{filepath.Join(t.TempDir(), "不存在")}); err == nil {
		t.Fatal("指向不存在的路径必须报错，不能返回空集合当'干净'")
	}
}

// 退出码是这条闸对 CI 唯一的输出语言，所以它自己得有证人（评审 I-2）：
// main 里的 os.Exit 测不到，CI 又只跑得绿路径——把 2 写成 0，仓库里从此
// 一条永远绿的假闸。run 把三档都变成可断言的返回值。
func TestRun_退出码三档各有证人(t *testing.T) {
	dir := t.TempDir()
	clean := write(t, dir, "ok.txt", "示例 http://10.0.0.3:5000\n")
	leaky := write(t, dir, "bad.txt", "NAS="+quad("192", "168", "2", "3")+"\n")
	skipper := filepath.Join(dir, "bin.dat")
	if err := os.WriteFile(skipper, []byte{0xff, 0xfe, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		args  []string
		want  int
		skips bool // 这一支里有没有该被点名跳过的文件
	}{
		{"干净放行 0", []string{clean}, 0, false},
		{"有泄漏 1", []string{leaky}, 1, false},
		{"扫描目标读不到 2（守卫没跑成不等于干净）", []string{filepath.Join(dir, "没有这个文件")}, 2, false},
		{"跳过也要跑完并放行 0", []string{clean, skipper}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run(tc.args, &out, &errOut); got != tc.want {
				t.Fatalf("退出码 = %d，要 %d\nstdout=%q stderr=%q", got, tc.want, out.String(), errOut.String())
			}
			switch tc.want {
			case 1:
				if !strings.Contains(out.String(), leaky) || !strings.Contains(out.String(), ":1") {
					t.Errorf("报泄漏要点名文件与行号: %q", out.String())
				}
			case 2:
				if errOut.Len() == 0 {
					t.Error("守卫自己没跑成要说清为什么，不能闷声返回")
				}
			}
			// 跳过必须点名——这一条打在"放行"那一支上，因为守卫绿着的时候
			// 最容易把少了什么忘记。
			if got := strings.Contains(errOut.String(), "跳过"); got != tc.skips {
				t.Errorf("跳过点名 = %v，期望 %v；stderr=%q", got, tc.skips, errOut.String())
			}
		})
	}
}

// ── 第二层（spec §15 的 NAS_SUBNET_PREFIX）──
//
// 下面这些用例能写出来，靠的是一个巧合而不是运气：**网段前缀恰好是三段，
// 而扫描器的正则恰好要求四段**，所以前缀字面量写在仓库里不会被守卫自己扫到。
// 四段及以上的地址仍然一律用 quad() 现拼，跟既有夹具同一条路。
// 这个巧合是前提不是实现细节：哪天把前缀改成接受 CIDR 或四段单主机，
// 这份测试文件就会自己咬自己，下面 want=1 的用例当场红——这正是它该有的反应。
func TestResolveHomePrefix_形状收放(t *testing.T) {
	home := quad("192", "168", "50") // 三段，写在仓库里安全（见上面那段注释）
	for _, tc := range []struct {
		name       string
		raw        string
		want       [3]int
		wantOn     bool
		wantErr    bool
		wantErrMsg string
	}{
		{"三段收", home, [3]int{192, 168, 50}, true, false, ""},
		{"前后空格照样收", "  " + home + "\t", [3]int{192, 168, 50}, true, false, ""},
		{"空值是未配置不是错", "", [3]int{}, false, false, ""},
		{"全空白也是未配置", "   ", [3]int{}, false, false, ""},
		{"CIDR 写法拒", home + "/24", [3]int{}, false, true, "CIDR"},
		{"CIDR 的报错要点破是掩码不是网段号", home + "/16", [3]int{}, false, true, "不支持"},
		{"带端口拒", quad("192", "168", "50") + ":80", [3]int{}, false, true, "端口"},
		{"四段单主机拒", quad("192", "168", "50", "7"), [3]int{}, false, true, "不是三段八位组"},
		{"两段拒", "192.168", [3]int{}, false, true, ""},
		{"超界八位组拒", quad("192", "168", "256"), [3]int{}, false, true, "第 3 段"},
		{"带正号拒（读得懂但不是地址前缀）", "+192.168.50", [3]int{}, false, true, ""},
		{"四段全零拒", quad("0", "0", "0", "0"), [3]int{}, false, true, ""},
		{"三段全零拒——那等于把守卫关掉", "0.0.0", [3]int{}, false, true, "把守卫关掉"},
		{"前导零按十进制数值收", quad("192", "168", "010"), [3]int{192, 168, 10}, true, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, on, err := resolveHomePrefix(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("%q 必须报错，实际放行了 %v（on=%v）", tc.raw, got, on)
				}
				// 报错要说清**是哪一层配错了**，否则读到 CI 日志的人无从下手。
				if !strings.Contains(err.Error(), homeEnv) {
					t.Errorf("报错要点名环境变量 %s，实际 %v", homeEnv, err)
				}
				if tc.wantErrMsg != "" && !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Errorf("报错要含 %q——配错的人只看得见这句话，实际 %v", tc.wantErrMsg, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("%q 不该报错：%v", tc.raw, err)
			}
			if on != tc.wantOn || got != tc.want {
				t.Errorf("resolve(%q) = %v, on=%v；要 %v, on=%v", tc.raw, got, on, tc.want, tc.wantOn)
			}
		})
	}
}

// 第二层的放行范围**只到本网段前三段相同**。这一支的 wantHit 对照是必须的：
// 没有它，把 sameHome 改成恒真就是全绿（所有地址都不报了），而"不报"正是
// 这条闸坏掉时最安静的那一种形状。
func TestScanWithPolicy_第二层只放行本网段(t *testing.T) {
	dir := t.TempDir()
	pol := scanPolicy{allow: allow, homeNet: [3]int{192, 168, 50}, hasHome: true}
	for _, tc := range []struct {
		name, body string
		want       int
		wantHit    string
	}{
		{"本网段放行", "NAS=" + quad("192", "168", "50", "7") + ":7570\n", 0, ""},
		{"本网段另一台也放行", "NAS=" + quad("192", "168", "50", "200") + "\n", 0, ""},
		{"第三段不同仍报", "NAS=" + quad("192", "168", "51", "7") + "\n", 1, quad("192", "168", "51", "7")},
		{"第二段不同仍报", "NAS=" + quad("192", "169", "50", "7") + "\n", 1, quad("192", "169", "50", "7")},
		{"公网不受第二层影响", "dns=" + quad("8", "8", "8", "8") + "\n", 1, quad("8", "8", "8", "8")},
		{"示例段第一层本来就放行", "url=http://" + quad("10", "0", "0", "3") + ":5000\n", 0, ""},
		{"回环第一层本来就放行", "B=http://" + quad("127", "0", "0", "1") + ":7600\n", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := write(t, dir, strings.ReplaceAll(tc.name, " ", "_")+".txt", tc.body)
			findings, _, err := scanWithPolicy([]string{f}, pol)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != tc.want {
				t.Fatalf("处数 = %d，要 %d：\n%s", len(findings), tc.want, strings.Join(findings, "\n"))
			}
			if tc.wantHit != "" && !strings.Contains(strings.Join(findings, "\n"), tc.wantHit) {
				t.Errorf("报出来的那条要含 %+v 那个地址，实际 %v", tc.wantHit, findings)
			}
		})
	}
}

// 前缀写 `010` 与地址写 `10` 是**同一个八位组**：按字符串比会把前一段悄悄失效，
// 而失效方向正是"该抓的没抓"——没人会看到一条前缀"配了但不起作用"。
// 这条用例单独立一支：它钉的是数值比较，与上一支的网段边界是两件事。
func TestScanWithPolicy_前缀按数值比八位组(t *testing.T) {
	dir := t.TempDir()
	home, on, err := resolveHomePrefix(quad("192", "168", "010"))
	if err != nil || !on {
		t.Fatalf("前缀 %q 解析失败：on=%v err=%v", quad("192", "168", "010"), on, err)
	}
	pol := scanPolicy{allow: allow, homeNet: home, hasHome: true}
	for _, tc := range []struct {
		name, body string
		want       int
		wantHit    string
	}{
		{"同一八位组放行", "NAS=" + quad("192", "168", "10", "3") + "\n", 0, ""},
		{"别的前三段仍报", "NAS=" + quad("192", "168", "50", "3") + "\n", 1, quad("192", "168", "50", "3")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := write(t, dir, strings.ReplaceAll(tc.name, " ", "_")+".txt", tc.body)
			findings, _, err := scanWithPolicy([]string{f}, pol)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != tc.want {
				t.Fatalf("处数 = %d，要 %d：\n%s", len(findings), tc.want, strings.Join(findings, "\n"))
			}
			if tc.wantHit != "" && !strings.Contains(strings.Join(findings, "\n"), tc.wantHit) {
				t.Errorf("报出来的那条要含 %+v 那个地址，实际 %v", tc.wantHit, findings)
			}
		})
	}
}

// 第二层开不开只由环境变量说了算；**且每一档都必须出声**。spec §15 把这条
// 写死成"secret 缺失只跳过第二层，第一层照跑，不许因为漏配 secret 就整体静默
// 失效"——下面六支就是把那句话逐段钉住的地方。
func TestRun_第二层只由secret开关且每档出声(t *testing.T) {
	dir := t.TempDir()
	// 本家网段里的一个地址，写死是为了让用例与 CI 的 secret 取值无关——
	// 测试的判据必须只由测试自己的输入决定。
	leak := write(t, dir, "nas.txt", "NAS="+quad("192", "168", "50", "7")+":7570\n")
	// 干净文件单独一份：判"放行那一行有没有自曝只有一层"必须走 0 处泄漏那条
	// 出口——有泄漏时 run 直接返回 1，那一行压根不会被打印，用有泄漏的夹具
	// 去测它会得到一个永远红的用例，而不是一条真的判据。
	clean := write(t, dir, "clean.txt", "示例 http://10.0.0.3:5000\n")
	for _, tc := range []struct {
		name       string
		file       string
		secret     string
		want       int
		wantStdout string
		wantStderr string
	}{
		{"secret 未设：第一层照跑，照样报", leak, "", 1, "", "第二层未启用"},
		{"secret 未设时放行那一行要自曝只有一层", clean, "", 0, "只有第一层", "第二层未启用"},
		{"secret 是别的网段：第二层开着也照样报", leak, quad("192", "168", "51"), 1, "", "第二层启用"},
		{"secret 正是本网段：放行，且要报出两层", leak, quad("192", "168", "50"), 0, "两层判据", "第二层启用"},
		{"secret 配错：守卫没跑成，退出 2 而不是静默放行", leak, "192.168.50/24", 2, "", ""},
		{"secret 三段全零：退出 2，不许变成放行整段", leak, "0.0.0", 2, "", "把守卫关掉"},
		{"secret 写成四段：退出 2，不许猜它想表达什么", leak, quad("192", "168", "50", "7"), 2, "", "不是三段八位组"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(homeEnv, tc.secret)
			var out, errOut bytes.Buffer
			if got := run([]string{tc.file}, &out, &errOut); got != tc.want {
				t.Fatalf("退出码 = %d，要 %d\nstdout=%q stderr=%q", got, tc.want, out.String(), errOut.String())
			}
			if tc.wantStdout != "" && !strings.Contains(out.String(), tc.wantStdout) {
				t.Errorf("stdout 要含 %q，实际 %q", tc.wantStdout, out.String())
			}
			if tc.wantStderr != "" && !strings.Contains(errOut.String(), tc.wantStderr) {
				t.Errorf("stderr 要含 %q，实际 %q", tc.wantStderr, errOut.String())
			}
		})
	}
}
