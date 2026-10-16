// Package uicheck 是**唯一一条会真的点界面的门禁**：它起一个真的 launchbay、一个
// 无头 Chrome，然后让 `sweep.mjs` 把档案页上每一条会写盘的交互走一遍。
//
// 为什么值得一条闸（2026-10-16 加）：
//
// 这个项目里反复出现的 bug 是同一句——**屏上说的与盘上存的分家**——而 2026-10-15 之前
// **没有任何东西会跑界面**：go test 只判服务端契约与若干条源码级断言。于是每一个界面
// 缺陷都要等人手动碰到才浮出来，一次一个，「修完发现还有下一个」。这道闸把「每次写盘
// 之后屏上必须与盘上一致」变成机械核对，26 步一次跑完。
//
// 跑法：`LB_UI=1 go test ./tools/uicheck/`
//
// **默认跳过**，而且理由写在这条 t.Skip 里而不是藏起来：
//   - 它要 node 与一个无头浏览器。两者都不在这个仓库的零依赖承诺里（本包只用标准库，
//     但进程要起 node 与 Chrome）；
//   - 它要十几秒，而 `go test ./...` 的其余部分只要几秒——每次都跑它，开发者就会开始
//     `--short` 绕过去，那之后这条闸就只剩一个绿色的历史。
//
// 所以它**平时跳过、被显式点名才跑**，而 CI 想接它就是加一个设了 LB_UI=1 的 job。
//
// 零依赖：本包只用标准库（depguard 那条闸照样过）。
package uicheck

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// enabled 是那道闸的开关。与指纹同步那道（LB_SYNC_FINGERPRINT）同一个形状：
// **让测试自己把机械活干完**，而不是让人手改两处文档。
func enabled() bool { return os.Getenv("LB_UI") != "" }

// 这道闸自己那台机器上的令牌。**只在这里写一次**：同一个值既发给被测服务，也发给
// 横扫脚本（见下面 cmd.Env 那两行）。它们是一次性实验机的凭据，不是产品配置。
const (
	adminToken = "uicheck-admin"
	writeToken = "uicheck-write"
)

func Test档案页交互横扫(t *testing.T) {
	if !enabled() {
		t.Skip("跳过界面横扫：设 LB_UI=1 才跑（要 node 与一个无头浏览器；" +
			"浏览器找不到时可以用 LB_CDP 指向一个已经开着的调试口）")
	}
	repo := repoRoot(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("找不到 node：这道闸靠它驱动浏览器（%v）", err)
	}

	tmp := t.TempDir()
	data := filepath.Join(tmp, "data")
	// Windows 上可执行文件要带 .exe，否则 exec 把它当普通文件名去 PATH 里找。
	bin := filepath.Join(tmp, "launchbay")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build(t, repo, bin)

	port := freePort(t)
	startServer(t, bin, data, port)
	waitHealthy(t, port, 20*time.Second)

	cdp, stop := browser(t, tmp)
	defer stop()

	// 两道横扫跑同一台服务、同一份数据：第一道走档案页上那些写盘交互，第二道走
	// 别的族（离线队列、设置面板其余各格、搜索两路、窄屏、减弱动效、键盘、零 JS
	// 通路）。两份共用 harness.mjs 里那一份连接与那条不变量——抄一份到第二个脚本
	// 里，就是这个项目刚花一轮消灭掉的那种分叉。
	//
	// 第二道排在第一道之后：它会把视图与导航位置改掉（那是全机器的偏好），反过来
	// 跑会让第一道读到一份不是它自己铺出来的版面。
	for _, script := range []string{"sweep.mjs", "sweep-variants.mjs"} {
		runSweep(t, node, repo, script, port, data, cdp)
	}
}

func runSweep(t *testing.T, node, repo, script string, port int, data, cdp string) {
	t.Helper()
	cmd := exec.Command(node, filepath.Join("tools", "uicheck", script),
		fmt.Sprint(port), data, cdp)
	cmd.Dir = repo
	// 令牌**只在这里写一份**：同一个值既发给被测服务，也发给横扫脚本（脚本读环境变量）。
	// 两边各留一个字面量的话，下一次改一边就变成"建档 401"这种与判据无关的红。
	cmd.Env = append(os.Environ(), "LB_UI_ADMIN="+adminToken, "LB_UI_WRITE="+writeToken)
	out, err := cmd.CombinedOutput()
	t.Log("\n" + indent(string(out)))
	if err != nil {
		t.Fatalf("横扫没过（%v）。上面是逐步读数：每一行是「屏上与盘上是否一致」的结论。", err)
	}
}

// repoRoot 从本包的位置往上找 go.mod：门禁要跑的是**这个仓库**的二进制与资产，
// 不是工作目录恰好在哪。
func repoRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("找不到 go.mod：这道闸要对着仓库跑")
	return ""
}

func build(t *testing.T, repo, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, "./cmd/launchbay")
	cmd.Dir = repo
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build 失败：%v\n%s", err, b)
	}
}

// freePort 要一个**此刻没人听**的端口。取 0 让内核给一个，然后立刻关掉——
// 那一刻的窗口小到可以忽略，比"随机猜一个再重试"好读。
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func startServer(t *testing.T, bin, data string, port int) {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"LAUNCHBAY_ADMIN_TOKEN="+adminToken,
		"LAUNCHBAY_WRITE_TOKENS=dad:"+writeToken+":dad",
		"LAUNCHBAY_DATA_DIR="+data,
		fmt.Sprintf("LAUNCHBAY_PORT=%d", port),
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("起服务失败：%v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
}

func waitHealthy(t *testing.T, port int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	for time.Now().Before(deadline) {
		r, err := http.Get(url)
		if err == nil {
			r.Body.Close()
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("服务 %d 秒内没起来", within/time.Second)
}

// browser 拿一个能用的调试口：先看 LB_CDP（已经开着的），再自己起一个无头浏览器。
// 找不到浏览器就**明说怎么办**，而不是安静地跳过——一条"因为没浏览器所以绿"的闸
// 与没有这条闸一模一样。
func browser(t *testing.T, tmp string) (cdp string, stop func()) {
	t.Helper()
	if v := os.Getenv("LB_CDP"); v != "" {
		if ok, _ := ping(v); ok {
			return strings.TrimRight(v, "/"), func() {}
		}
		t.Fatalf("LB_CDP=%s 上没有调试口", v)
	}
	exe := findBrowser()
	if exe == "" {
		t.Fatalf("找不到无头浏览器：装一个 Chrome/Edge，或设 LB_CDP 指向一个已经开着的调试口" +
			"（chrome --headless=new --remote-debugging-port=9222 --user-data-dir=<临时目录>）")
	}
	port := freePort(t)
	profile := filepath.Join(tmp, "chrome-profile")
	args := []string{
		"--headless=new",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + profile,
		"--no-first-run", "--no-default-browser-check",
		"--disable-gpu", "--hide-scrollbars",
	}
	// 容器里的 Chrome 起来不了（沙箱要 namespaces，而容器里通常没有），那时用
	// LB_CHROME_NO_SANDBOX=1 让它带 --no-sandbox。GitHub 托管的 runner 是普通用户
	// 起进程，不需要它——所以这是一个**给容器用的出口**，不是默认设置。
	if os.Getenv("LB_CHROME_NO_SANDBOX") != "" {
		args = append(args, "--no-sandbox", "--disable-dev-shm-usage")
	}
	args = append(args, "about:blank")
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("起浏览器失败：%v", err)
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _ := ping(base); ok {
			return base, func() {
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	t.Fatalf("浏览器起来了但调试口 %d 没开", port)
	return "", func() {}
}

// findBrowser 按平台各找几处常见位置。**找不到就返回空串**，由调用方决定怎么办
// （这条闸不能假装自己跑过了）。
func findBrowser() string {
	if v := os.Getenv("LB_BROWSER"); v != "" {
		return v
	}
	var names []string
	switch runtime.GOOS {
	case "windows":
		for _, p := range []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		names = []string{"chrome", "msedge", "chromium"}
	case "darwin":
		names = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
	default:
		names = []string{"google-chrome", "chromium", "chromium-browser"}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func ping(cdp string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cdp, "/")+"/json/version", nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("    " + line + "\n")
	}
	return b.String()
}
