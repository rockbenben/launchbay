package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// cmdProfileFixNames 是 `launchbay profile fix-names`：把**名字就是地址**的那些条目
// 改成主机名。
//
// 为什么需要它：浏览器导入抓不到标题时，条目名就落成那整串地址。实测一份 48 条
// 的真实档案里 26 条是这个形状，于是半屏卡片的名字是 `https://www.rfc-editor.org/
// info/rfc833391` 这种 40 来个字符的东西——扫不动，而且名字那格就是每天要扫的那一格。
// 增量的兜底早就有了（新增条目读不到标题就用主机名，app.js 的 itemName），但**存量**
// 不会自己变，而导入是成批来的。
//
// **为什么是 CLI 而不是设置面板里的一颗键**：这是唯一一条会**按规则改写别人书签名字**
// 的动作。放进面板意味着一个误点就把几十条名字改了，而面板上没有"只改这一条"的位置。
// 走 CLI 时它是显式的一条命令，默认还有 --dry-run。
//
// **经 batch 落盘**（与 import 同一份纪律）：快照、审计行、rev 一道不少，所以改错了
// 在「回退」里能退回去。不直读磁盘——进程内互斥锁罩不住另一个进程。
func cmdProfileFixNames(args []string, cfg config, out io.Writer) int {
	fs := flag.NewFlagSet("profile fix-names", flag.ContinueOnError)
	fs.SetOutput(out)
	id := fs.String("profile", "", "要整理的档案 id")
	dryRun := fs.Bool("dry-run", false, "只列出要改哪些，不提交")
	by := fs.String("by", "cli", "审计线上的渠道名")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(out, "用法: launchbay profile fix-names --profile <id> [--dry-run]")
		return 2
	}
	c, err := newScopedClient(cfg.Port, cfg.Tokens, *id)
	if err != nil {
		fmt.Fprintf(out, "拿不到 %s 的 REST 令牌: %v\n", *id, err)
		return 1
	}
	cur, err := c.profileData(*id)
	if err != nil {
		fmt.Fprintf(out, "读档案 %s 失败: %v\n", *id, err)
		return 1
	}

	type change struct{ op, from, to string }
	var ops []map[string]any
	var changes []change
	for _, g := range cur.Data.Groups {
		for _, it := range g.Items {
			to, ok := hostNameFor(it.Name, it.URL)
			if !ok {
				continue
			}
			changes = append(changes, change{
				op:   "groups/" + g.ID + "/items/" + it.ID + "/name",
				from: it.Name, to: to,
			})
			ops = append(ops, map[string]any{
				"op": "set", "path": "groups/" + g.ID + "/items/" + it.ID + "/name", "value": to,
			})
		}
	}
	if len(changes) == 0 {
		fmt.Fprintf(out, "档案 %s 里没有名字就是地址的条目\n", *id)
		return 0
	}
	for _, ch := range changes {
		fmt.Fprintf(out, "  %s\n    ← %s\n", ch.to, ch.from)
	}
	fmt.Fprintf(out, "共 %d 条（档案 %s，当前 rev=%d）\n", len(changes), *id, cur.Rev)
	if *dryRun {
		fmt.Fprintln(out, "（--dry-run：没有提交）")
		return 0
	}
	body, _ := json.Marshal(map[string]any{
		"profile": *id, "expectedRev": cur.Rev, "by": *by, "ops": ops,
	})
	raw, code, err := c.do("POST", "/api/v1/batch", body,
		map[string]string{"Content-Type": "application/json", "X-LB-By": *by})
	if err != nil {
		fmt.Fprintf(out, "提交失败: %v\n", err)
		return 1
	}
	if err := mustOK(raw, code, "/api/v1/batch"); err != nil {
		fmt.Fprintf(out, "提交失败: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "已改 %d 条的名字（改错了可以在「回退」里退回去）\n", len(changes))
	return 0
}

// hostNameFor 判"这个名字其实是地址"，并给出该换成的名字。
//
// 判据只有两条，都要求**名字与地址指向同一个东西**——
//
//	名字以 http:// 或 https:// 开头，且它就是这一条的地址（去掉尾斜杠后逐字相等）；
//	名字与地址去掉尾斜杠之后逐字相等（人自己把地址粘进名字格，可以不带协议）。
//
// 两处刻意的"不改"，因为**多改比少改贵得多**：多改毁的是数据，少改只多几条手动。
//
//	名字自己也是个 URL、但**不是这一条**的地址（两个不同的链接）：不碰。按它自己的
//	  主机名改，会把一条指向别处的书签写成另一处的名字。
//	名字已经是主机名（`example.com` 或 `www.example.com`）：没得改，而它在人嘴里
//	  本来就是合理的短名字。
//
// 一个真实标题里带 URL 的（「AI 提示词库 | ChatGPT、DeepSeek 提示词… https://…」
// 这种，那是这份收藏最值钱的一条）两条判据都不满足，一个字都不动。
//
// 新名字取主机名去掉 `www.`：`www.` 是站点自己的写法，不是这个站点是谁。
func hostNameFor(name, rawURL string) (string, bool) {
	n := strings.TrimSpace(name)
	u := strings.TrimSpace(rawURL)
	if n == "" || u == "" {
		return "", false
	}
	looksLikeURL := strings.HasPrefix(strings.ToLower(n), "http://") || strings.HasPrefix(strings.ToLower(n), "https://")
	sameThing := strings.EqualFold(strings.TrimRight(n, "/"), strings.TrimRight(u, "/"))
	if !sameThing {
		return "", false
	}
	u2, err := url.Parse(u)
	if err != nil || u2.Host == "" {
		return "", false
	}
	host := strings.TrimPrefix(strings.ToLower(u2.Hostname()), "www.")
	if host == "" || host == strings.ToLower(n) || "www."+host == strings.ToLower(n) {
		return "", false // 已经就是主机名，没有可改的
	}
	_ = looksLikeURL // 名字像不像 URL 不参与判定：同一条地址才是（见上面的两条判据）
	return host, true
}
