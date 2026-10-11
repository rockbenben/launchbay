package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"

	"github.com/rockbenben/launchbay/internal/model"
)

// notCLICommand 是 runCLI 的"这不是子命令，交给 serve"信号。
//
// **它是 -1 而不是 0**：0 是"子命令成功"，而"没有子命令"和"子命令成功"落到
// 同一个值上时，main 就没法区分该退出还是该继续起服务。
const notCLICommand = -1

// runCLI 处理写通路的子命令。返回进程退出码，notCLICommand 表示"不认识，交给 serve"。
// 子命令名单**只有这一份**（main.go 不再另抄一遍——两份名单差一个的后果是
// 敲进去的命令静默落进 serve 分支，用户看到的是"端口被占用"）。
//
// 分发在 loadConfig 之后（healthcheck 已经在那儿），且直接吃那份 cfg：
// CLI 与服务读同一份 env，端口与令牌只有一份来源（runCLI 自己不碰 env）。
func runCLI(args []string, cfg config, out io.Writer) int {
	if len(args) == 0 {
		return notCLICommand
	}
	switch args[0] {
	case "revert":
		return cmdRevert(args[1:], cfg, out)
	case "profile":
		return cmdProfile(args[1:], cfg, out)
	case "import-bookmarks":
		return cmdImportBookmarks(args[1:], cfg, out)
	}
	return notCLICommand
}

// exitCode 把 -1（不该走到这里——main.go 只对已知子命令调 runCLI）映射成
// 用法错误 2，其余原样。
func exitCode(code int) int {
	if code < 0 {
		return 2
	}
	return code
}

func cmdRevert(args []string, cfg config, out io.Writer) int {
	fs := flag.NewFlagSet("revert", flag.ContinueOnError)
	fs.SetOutput(out)
	profile := fs.String("profile", "", "要恢复的档案 id")
	to := fs.Int64("to", 0, "恢复到哪个 rev（该 rev 的快照）")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	// --to 0 在人嘴里几乎总是"想要最初状态"的手误，而 rev 0 快照存在与否
	// 取决于建档后有没有做过一次写入——CLI 不替人猜，用法错误直接拒；
	// 端点对 to-rev=0 的态度相反（有 snapshots/0 就恢复它），见 revert.go。
	if *profile == "" || *to <= 0 {
		fmt.Fprintln(out, "用法: launchbay revert --profile <id> --to <rev>")
		fmt.Fprintln(out, "（--to 0 不收：rev 0 可能是建档留下的真实快照，确要恢复它请直连 API 的 to-rev=0）")
		return 2
	}
	// revert 要写别人的书签，所以用**那一份自己**的令牌（2026-10-10 改判）。
	// 管理员令牌（*）只管账号层面，这条端点已经不接受它了。
	c, err := newScopedClient(cfg.Port, cfg.Tokens, *profile)
	if err != nil {
		fmt.Fprintf(out, "revert 失败: %v\n", err)
		return 1
	}
	path := revertPath(*profile, *to)
	raw, code, err := c.do("POST", path, nil, map[string]string{"X-LB-By": "cli"})
	if err != nil {
		fmt.Fprintf(out, "revert 失败: %v\n", err)
		return 1
	}
	if err := mustOK(raw, code, path); err != nil {
		fmt.Fprintf(out, "revert 失败: %v\n", err)
		return 1
	}
	var resp struct {
		Rev          int64 `json:"rev"`
		RestoredFrom int64 `json:"restoredFrom"`
	}
	_ = json.Unmarshal(raw, &resp)
	fmt.Fprintf(out, "%s 已恢复到 rev %d 的内容，新的 rev 是 %d\n", *profile, resp.RestoredFrom, resp.Rev)
	return 0
}

// revertPath 拼这条端点的路径。**单独抽出来是为了让"自己转义、不信任 flag"
// 这件事仍然可测**：2026-10-10 改判之后 CLI 先按 --profile 挑令牌，而令牌作用域
// 必须是合法 slug，于是任何**需要**转义的 profile 值在配置里都不可能有一枚对应
// 的令牌——经 runCLI 那条路再也走不到含空格的输入。
//
// 判据仍然要有：token 来自 env，而 env 不是不可信输入的边界——它是运维手写的
// 字符串。哪一天配置解析放宽了、或者有人给别的子命令也加一个自由文本参数，
// 这里就是那个"拼出畸形 query"的入口。测试钉的是这一层。
func revertPath(profile string, to int64) string {
	return "/api/v1/revert?profile=" + url.QueryEscape(profile) + "&to-rev=" + fmt.Sprint(to)
}

// profileUsage 是 `launchbay profile` 的唯一用法文本。两个子命令各有各的必填参数，
// 共用一行 printf 会逼着其中一个把自己的旗标解释成别人的——所以这里给两份。
const profileUsage = "用法:\n" +
	"  launchbay profile create  --id <slug> --title <名字> --pin <4-6 位> [--admin]\n" +
	"  launchbay profile rebuild --id <slug> --title <名字> --pin <4-6 位> [--admin]"

// profileSub 是 `launchbay profile` 底下的子命令名单。与 runCLI 那份一样，
// **不许在别处再抄一遍**：两份名单差一个的后果是敲进去的命令静默落进 serve 分支，
// 用户看到的是"端口被占用"。
var profileSub = map[string]func([]string, config, io.Writer) int{
	"create":  cmdProfileCreate,
	"rebuild": cmdProfileRebuild,
}

func cmdProfile(args []string, cfg config, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, profileUsage)
		return 2
	}
	if sub, ok := profileSub[args[0]]; ok {
		// args[1:]：flag.Parse 遇到第一个非旗标参数就停，而子命令名正是那一个。
		// 把它留在参数里的话 Parse 立刻返回、**一个旗标都不解析**，--id/--title
		// 全是零值——而建档/重建那两条会照着空值发请求，服务端给出一句与真实
		// 原因无关的 422。这条是这个派发最容易踩的形状。
		return sub(args[1:], cfg, out)
	}
	fmt.Fprintln(out, profileUsage)
	return 2
}

func cmdProfileCreate(args []string, cfg config, out io.Writer) int {
	fs := flag.NewFlagSet("profile create", flag.ContinueOnError)
	fs.SetOutput(out)
	id := fs.String("id", "", "档案 id（ASCII slug）")
	title := fs.String("title", "", "显示名字")
	pin := fs.String("pin", "", "4-6 位数字")
	admin := fs.Bool("admin", false, "建成管理员档案")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := newAdminClient(cfg.Port, cfg.Tokens)
	if err != nil {
		fmt.Fprintf(out, "profile create 失败: %v\n", err)
		return 1
	}
	body, _ := json.Marshal(map[string]any{
		"id": *id, "title": *title, "pin": *pin,
		"admin": *admin})
	raw, code, err := c.do("POST", "/api/v1/profiles", body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		fmt.Fprintf(out, "profile create 失败: %v\n", err)
		return 1
	}
	if err := mustOK(raw, code, "/api/v1/profiles"); err != nil {
		fmt.Fprintf(out, "profile create 失败: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "已建档 %s\n", *id)
	return 0
}

// cmdProfileRebuild 是 `POST /api/v1/profiles/rebuild` 的壳：重写一份**读不动**的
// profile.json，书签、快照与历史已有内容一个字节都不碰（spec §6.1、§12）。
//
// 它用**管理员令牌**（*），与 profile create 同族——这条端点动的是账号文件而不是
// 任何人的书签，所以 2026-10-10 那条"`*` 只管账号"的改判把它留在 credAdmin 那一侧。
// （要改别人的**书签**才需要 `name:secret:<id>` 那种档案作用域令牌，那是 revert 的事。）
//
// 只经本机 REST，不直读磁盘：进程内互斥锁罩不住另一个进程，CLI 绕过服务写盘就是
// 与正在跑的容器抢同一个文件（§4.5）。中文名字在这里是安全的——请求体由 Go 编码，
// 绕开"从 shell 发中文 body"那个坑。
func cmdProfileRebuild(args []string, cfg config, out io.Writer) int {
	fs := flag.NewFlagSet("profile rebuild", flag.ContinueOnError)
	fs.SetOutput(out)
	id := fs.String("id", "", "要重建的档案 id（ASCII slug，磁盘上必须已存在）")
	title := fs.String("title", "", "重建后的显示名字")
	pin := fs.String("pin", "", "4-6 位数字")
	admin := fs.Bool("admin", false, "重建成管理员档案")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := newAdminClient(cfg.Port, cfg.Tokens)
	if err != nil {
		fmt.Fprintf(out, "profile rebuild 失败: %v\n", err)
		return 1
	}
	body, _ := json.Marshal(map[string]any{
		"profile": *id, "title": *title, "pin": *pin,
		// **显式**发 admin，哪怕旗标没写：端点那一格必填，省略是 422，而壳不许
		// 替调用方猜——"忘了 --admin"与"我就要一个非管理员档案"是两件不同的事，
		// 猜错的后果是静默降级一个人的档案权限。接线证人：把它改成省略，
		// TestCLIProfileRebuildAlwaysSendsAdmin 必须红。
		"admin": *admin,
	})
	raw, code, err := c.do("POST", "/api/v1/profiles/rebuild", body,
		map[string]string{"Content-Type": "application/json", "X-LB-By": "cli"})
	if err != nil {
		fmt.Fprintf(out, "profile rebuild 失败: %v\n", err)
		return 1
	}
	if err := mustOK(raw, code, "/api/v1/profiles/rebuild"); err != nil {
		fmt.Fprintf(out, "profile rebuild 失败: %v\n", err)
		return 1
	}
	var resp struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Admin bool   `json:"admin"`
	}
	_ = json.Unmarshal(raw, &resp)
	role := "普通档案"
	if resp.Admin {
		role = "管理员"
	}
	fmt.Fprintf(out, "已重建 %s（%s，%s）\n", resp.ID, resp.Title, role)
	return 0
}

func fetchBookmarks(c *client, id string) (*model.Bookmarks, error) {
	raw, code, err := c.do("GET", "/api/v1/data?profile="+url.QueryEscape(id), nil, nil)
	if err != nil {
		return nil, err
	}
	if err := mustOK(raw, code, "/api/v1/data"); err != nil {
		return nil, err
	}
	var out struct {
		Data model.Bookmarks `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}
