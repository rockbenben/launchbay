package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/httpapi"
	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/render"
	"github.com/rockbenben/launchbay/internal/store"
)

// dispatchAction 是 preConfigDispatch 的三种结论。
type dispatchAction int

const (
	// dispatchServe：什么都没给——起服务。
	dispatchServe dispatchAction = iota
	// dispatchExit：话已经印完，按这个码退出（--help 是 0，未知子命令是 2）。
	dispatchExit
)

// preConfigDispatch 是 main 在 loadConfig **之前**走的一格，抽出来是因为它
// 三条判据每一条都值得有证人，而 main() 里的 os.Exit 测不了。
//
// **为什么必须在 loadConfig 前面**：`--help` 是"还不知道该怎么配"那一刻按的键，
// 而 loadConfig 要的正是配好的东西（管理员 key 缺失是启动错误）。实测原先
// `launchbay --help` 的回答是一句「LAUNCHBAY_ADMIN_TOKEN: 未配置」——唯一那份
// 说明在唯一需要它的场合拿不到。
//
// **为什么未知子命令在这里拦而不是在 runCLI**：runCLI 的 notCLICommand 同时
// 表示"没给子命令"（该起服务）与"给错了"（该报错），这两件事到 main 这一层还
// 合在一起，于是敲错一个字母的人看到的是一句"端口被占用"。
func preConfigDispatch(args []string, stdout, stderr io.Writer) (dispatchAction, int) {
	if len(args) == 0 {
		return dispatchServe, 0
	}
	switch a := args[0]; {
	case a == "-h" || a == "--help" || a == "help":
		usage(stdout)
		return dispatchExit, 0
	case a != "healthcheck" && !strings.HasPrefix(a, "-") && !isCLICommand(a):
		fmt.Fprintf(stderr, "未知子命令 %q\n\n", a)
		usage(stderr)
		return dispatchExit, 2
	}
	return dispatchServe, 0
}

func main() {
	if act, code := preConfigDispatch(os.Args[1:], os.Stdout, os.Stderr); act != dispatchServe {
		if act == dispatchExit {
			os.Exit(code)
		}
		return
	}
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	// 子命令分发在 loadConfig 之后：探活端口与服务端口必须读同一份 env，
	// 否则 healthcheck 探错端口，容器会被判死循环重启。CLI 写子命令同样
	// 只认这一份 env（端口与令牌单一来源），且只经本机 REST 落盘。
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck(cfg.Port, cfg.DataDir))
	}
	// **子命令名单只有 cli.go 那一份**：runCLI 认得就返回退出码，不认得返回 -1。
	// 早先这里另写了一份子命令名单，于是加子命令
	// 时漏改这一处，敲进去的命令会**静默落进 serve 分支**——用户看到的是
	// "端口被占用"，而不是"这个子命令不存在"，能白查很久。
	if code := runCLI(os.Args[1:], cfg, os.Stdout); code != notCLICommand {
		os.Exit(exitCode(code))
	}
	// 走到这里就是 serve 那一支：管理员 key 是**服务端**的启动错误（AGENTS.md
	// 「令牌与鉴权」——缺了它这台机器装完没人能管）。CLI 子命令在上面已经返回了，
	// 它们要的是档案作用域令牌，所以这条闸不拦它们。
	if err := serveNeedsAdmin(cfg); err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	s := store.New(cfg.DataDir)
	ix, err := icons.Load()
	if err != nil {
		// 图标索引是内嵌产物：读不出来只有两种原因（embed 路径写错、产物损坏），
		// 都不该被"降级成没有图标"悄悄咽下——那会让一个损坏的镜像跑得很"正常"。
		log.Fatalf("图标索引装配失败: %v", err)
	}
	fc, err := s.LoadIconCache()
	if err != nil {
		// 与索引同口径：缓存坏了宁可启动失败，不静默清空——盘上的 blob 还躺着，
		// 静默重来就是第二份真相（spec §5）。
		// 运维出路（评审指出这条哪儿都没写）：data/icons/index.json 坏了就删掉
		// 整个 data/icons/ 目录再重启——那是可重建的缓存，不是用户数据；
		// 代价只是冷却清零、favicon 下次 resolve 重抓一遍。
		log.Fatalf("图标缓存装配失败: %v\n"+
			"出路：删掉 %s 整个目录再重启——那是可重建的缓存，不是用户数据；"+
			"代价只是冷却清零、favicon 下次 resolve 重抓一遍。", err, filepath.Join(cfg.DataDir, "icons"))
	}
	h := httpapi.New(httpapi.Dependencies{
		Store:  s,
		Tokens: cfg.Tokens,
		// AdminToken 与 Tokens 是两个字段（2026-10-12）。少写这一行不会编译失败、
		// 测试也不会红（测试有自己那份 newDeps），只会在真机上让每一把管理员 key
		// 都变成 401——所以这里逐个列出，与注释里那句"新加依赖字段"的教训对着看。
		AdminToken: cfg.AdminToken,
		Render:     render.New(s, ix, fc),
		Icons:      ix,
		Fetch:      icons.NewFetcher(),
		Cache:      fc,
	}).Handler()
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: h,
		// 四个超时一个都不能省。零值是"无限等"，而这台机器是**裸**挂在家里
		// 的网上、前面没有任何反代替它挡连接：一个只把请求头发一半就停下的
		// 客户端，能把一个 goroutine 和一个 fd 一直占到天亮（gosec G112
		// 说的就是这件事）。局域网不等于可信——访客网络、手机热点、
		// 装错的智能设备都算。
		//
		// ReadHeaderTimeout 是慢速首部攻击真正的那一道闸：首部在 10 秒内没
		// 读完就断，请求体都不用开始收。ReadTimeout 给整个请求一个上界；
		// 本案的请求体都是几百字节的 JSON，没有流式上传，它拦不住任何
		// 正常请求。
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// IdleTimeout 只管 keep-alive 的空转连接，不碰在途请求。
		IdleTimeout: 60 * time.Second,
		// WriteTimeout 必须**远大于** resolve 的出网预算，否则会把一个慢站点
		// 变成一个半截响应：/api/v1/resolve 出去抓目标页面，每一跳 3 秒
		// （icons.FetchTimeout），跟着最多 3 跳，再加一次 favicon，最坏约
		// 12 秒。给 60 秒是五倍余量——它是兜底的安全阀，不是计时器。
		WriteTimeout: 60 * time.Second,
	}
	log.Printf("launchbay listening on :%s (data=%s)", cfg.Port, cfg.DataDir)

	// 优雅退出：docker stop / compose down 发的是 SIGTERM。不接住它，进程会
	// 被就地掐断——**书签本身不会丢**（写入是同目录临时文件 + rename，见
	// store.WriteFileAtomic，锁也是进程内的 sync.Mutex，崩了不留残骸），
	// 丢的是"这一批到底成没成"这句话：调用方只看到连接重置。
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	go func() {
		<-ctx.Done()
		// 摘掉信号处理再退：第二次 Ctrl+C 恢复默认行为。人等得不耐烦时
		// 按两下就能硬退，不用干等这 10 秒。
		stopSignals()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("优雅退出没能在 10 秒内收干净: %v", err)
		}
	}()

	// 这里是全案最后一处曾被漏掉的坑：Shutdown 正常完成时 ListenAndServe
	// 返回的是 http.ErrServerClosed，那**不是**故障。原来这里直接 log.Fatal，
	// 于是一次干净的停机会在日志里留下一条 "http: Server closed" 再退 1，
	// 长得跟崩溃一模一样——而按 spec 的纪律，"把正常说成失败"比失败更贵。
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// serveNeedsAdmin 是"这台机器要能管"那一道闸：管理员 key 缺失对**服务端**是启动错误
// （缺了它装完没人能建档、删档、给别人设 PIN），对 CLI 子命令却不是——`revert` 与
// `import-bookmarks` 用的是档案作用域令牌，管理员那把在内容端点上恰恰是 403。
// 所以这条判据单独成一个函数：main 里那三行没法被测到，而"谁必填谁选填"这件事
// 一旦挪错，症状是所有人卡在建档页而日志一个字不打。
func serveNeedsAdmin(cfg config) error {
	if cfg.AdminToken.Secret() == "" {
		return fmt.Errorf("LAUNCHBAY_ADMIN_TOKEN: %w", auth.ErrNoAdminToken)
	}
	return nil
}
