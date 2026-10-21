package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

// cmdBackup 把这一份导出成一份文件（书签 + 这一页的样子）。
//
// **它与 `launchbay import-bookmarks` 是两件事**：那一条吃的是**浏览器**
// 导出的书签 HTML，这一对吃的是 launchbay 自己的备份——导出的与导入的是
// 同一份格式，所以搬一次家不用人在两种文件之间翻译。
//
// **读写都走 REST**：CLI 不直接 import store 写文件（进程内互斥锁罩不住
// 另一个进程，绕过它就是与正在跑的容器抢同一个文件）。
//
// **不给 --out 就打到标准输出**，于是 `launchbay backup --profile dad > dad.json`
// 也能用——那种管道的另一头是文件，所以**那一路一个字提示都不打**
// （提示混进 JSON 里，那份文件就自己坏了）。
func cmdBackup(args []string, cfg config, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	profile := fs.String("profile", "", "导出哪一份（必填）")
	outPath := fs.String("out", "", "写到哪个文件（不给就打到标准输出）")
	if code, ok := parseFlags(fs, args, out, errOut); !ok {
		return code
	}
	if strings.TrimSpace(*profile) == "" {
		fmt.Fprintln(errOut, "必须给 --profile：导出是有对象的，不知道导哪一份就不该跑")
		return 2
	}
	// 导这一份要能读它，所以用**那一份自己**的令牌（管理员令牌只管账号层面）。
	c, err := newScopedClient(cfg.Port, cfg.Tokens, *profile)
	if err != nil {
		fmt.Fprintf(errOut, "拿不到 %s 的 REST 令牌: %v\n", *profile, err)
		return 1
	}
	path := "/api/v1/backup?profile=" + url.QueryEscape(*profile)
	raw, code, err := c.do("GET", path, nil, nil)
	if err != nil {
		fmt.Fprintf(errOut, "导出失败: %v\n", err)
		return 1
	}
	if err := mustOK(raw, code, path); err != nil {
		fmt.Fprintf(errOut, "导出失败: %v\n", err)
		return 1
	}
	if strings.TrimSpace(*outPath) == "" {
		if _, err := out.Write(raw); err != nil {
			fmt.Fprintf(errOut, "写不出去: %v\n", err)
			return 1
		}
		return 0
	}
	if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
		fmt.Fprintf(errOut, "写文件失败: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "已导出 %s 到 %s\n", *profile, *outPath)
	return 0
}

// cmdRestore 把一份备份导回某一份。
//
// **目标档案从令牌作用域推导**：CLI 挑的是「作用域正好是 --profile 那一份」
// 的令牌，于是服务端那边的目标与这里说的 --profile 是同一份（两边各写一个
// 目标ID 就会出现"我导进了 dad，而 CLI 说导的是 mom"这种事）。
//
// **"目标档案不空"那道闸在服务端**：那里是唯一一份判据，CLI 不自己数一遍
// ——数两遍就会有一处漂，而漂掉的那一处会让人以为能导，导到一半才被拒。
func cmdRestore(args []string, cfg config, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	profile := fs.String("profile", "", "导进哪一份（必填）")
	by := fs.String("by", "import", "改动历史里这一趟记成哪个渠道（ai / bookmarklet / extension / web / import / cli）")
	if code, ok := parseFlags(fs, args, out, errOut); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(errOut, "用法: launchbay restore [flags] <backup.json>")
		fs.PrintDefaults()
		return 2
	}
	if strings.TrimSpace(*profile) == "" {
		fmt.Fprintln(errOut, "必须给 --profile：导入是有去向的，不知道往哪导就不该跑")
		return 2
	}
	raw, err := os.ReadFile(rest[0])
	if err != nil {
		fmt.Fprintf(errOut, "读不到备份文件: %v\n", err)
		return 1
	}
	c, err := newScopedClient(cfg.Port, cfg.Tokens, *profile)
	if err != nil {
		fmt.Fprintf(errOut, "拿不到 %s 的 REST 令牌: %v\n", *profile, err)
		return 1
	}
	rawResp, code, err := c.do("POST", "/api/v1/restore", raw,
		map[string]string{"Content-Type": "application/json", "X-LB-By": *by})
	if err != nil {
		fmt.Fprintf(errOut, "导入失败: %v\n", err)
		return 1
	}
	if err := mustOK(rawResp, code, "/api/v1/restore"); err != nil {
		fmt.Fprintf(errOut, "导入失败: %v\n", err)
		return 1
	}
	// 响应读不出来时**不能报成功**：那会把一份形状不对的 200 印成"导入完了"
	//——而它其实什么都没说（cmdRevert 那一处同一教训）。
	var resp struct {
		Rev      int64    `json:"rev"`
		Applied  int      `json:"applied"`
		Settings []string `json:"settings"`
	}
	if err := json.Unmarshal(rawResp, &resp); err != nil {
		fmt.Fprintf(errOut, "导入的响应读不出来（HTTP %d）: %v\n", code, err)
		return 1
	}
	if resp.Applied > 0 {
		fmt.Fprintf(out, "已导入档案 %s 的书签（%d 条 op，新的 rev 是 %d）\n", *profile, resp.Applied, resp.Rev)
	}
	if len(resp.Settings) > 0 {
		fmt.Fprintf(out, "已改台面设置：%s\n", strings.Join(resp.Settings, "、"))
	}
	if resp.Applied == 0 && len(resp.Settings) == 0 {
		fmt.Fprintln(out, "这份备份里没有要写的东西。")
	}
	return 0
}
