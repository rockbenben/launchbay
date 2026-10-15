package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/rockbenben/launchbay/internal/bookimport"
	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
)

// cmdImportBookmarks 把浏览器导出的 Netscape 书签文件导进一个档案。
//
// **两条通路的分工，这是本命令最重要的一件事**：
//
//	书签本体 → POST /api/v1/batch。只写 data/ 的那条禁令（AGENTS.md 禁止事项
//	          第一条）针对的就是它：快照、历史、rev 递增都发生在 batch 内部，
//	          绕过它等于造出第二份真相。所以这里生成 ops 提交，不碰磁盘。
//
//	**没有第二条通路**：模板机制整条拆掉了（2026-10-10）——每个档案完全分离，
//	不预置任何内容，也就没有"一份共用的清单往各个档案里铺"这件事可做。
//
// **目标档案必须是空的**：导入的语义是"把浏览器那套导进来"，不是"往现有档案上
// 叠加"。对着一个已经装了几十条书签的档案跑一遍 replace，那几十条会无声消失。
// 撞上了就报错让人自己选，而不是替他决定。
func cmdImportBookmarks(args []string, cfg config, out io.Writer) int {
	fs := flag.NewFlagSet("import-bookmarks", flag.ContinueOnError)
	fs.SetOutput(out)
	profile := fs.String("profile", "", "导入到哪个档案（必填；该档案必须是空的）")
	by := fs.String("by", "cli", "改动历史里这一趟记成哪个渠道（ai / extension / web / import / cli）")
	overrides := fs.String("overrides", "", "图标覆盖表的 JSON 文件路径（不给就用内置那份，再没有就留空）")
	dryRun := fs.Bool("dry-run", false, "只打印报告与 ops，不提交")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(out, "用法: launchbay import-bookmarks [flags] <bookmarks.html>")
		// 少了这一句，走错参数个数的人看到的只有那一行用法，而这条命令**必须**
		// 带 --profile 却一个字都没提它叫什么——三个开关全在错误路径上隐身。
		fs.PrintDefaults()
		return 2
	}
	if strings.TrimSpace(*profile) == "" {
		fmt.Fprintln(out, "必须给 --profile：导入是有去向的，不知道往哪导就不该跑")
		return 2
	}

	raw, err := os.ReadFile(rest[0])
	if err != nil {
		fmt.Fprintf(out, "读不到书签文件: %v\n", err)
		return 1
	}
	ix, err := icons.Load()
	if err != nil {
		fmt.Fprintf(out, "图标索引读不出来: %v\n", err)
		return 1
	}
	res, err := bookimport.Convert(raw, ix, bookimport.Options{OverridesPath: *overrides})
	if err != nil {
		fmt.Fprintf(out, "导入中止: %v\n", err)
		return 1
	}

	// 报告先打，人要能在**任何**失败之前读到"迁了什么、没迁什么"。
	fmt.Fprintln(out, res.Report.Text())

	if n := bookimport.CountItems(res.Bookmarks); n == 0 {
		// **两个形状要说两句**：报告已经把每一条没能进来的列在上面了，而"0 条"
		// 现在有两种来路——文件里除了回收站什么都没有（老情形），或者读出了
		// 书签但一条都没能进来（全是 javascript: 这类没有主机名的地址）。
		// 混成一句会把人引去换一个同样导不进来的文件。
		if len(res.Report.Skipped) > 0 {
			fmt.Fprintf(out, "读出了书签，但 %d 条全都没能进来（javascript:、data: 这类没有主机名、"+
				"点了会执行东西的地址；本地文件 file:// 是能导的）。这种书签请继续在浏览器里用。\n",
				len(res.Report.Skipped))
		} else {
			fmt.Fprintln(out, "这个文件里没有可导入的书签（回收站里的那些有意的没算在内）")
		}
		return 1
	}

	// 导入要读也要写这一份的书签，所以用**那一份自己**的令牌（2026-10-10 改判）：
	// 管理员令牌（*）只管账号层面，`/api/v1/data` 与 `/api/v1/batch` 已经不接受它。
	c, err := newScopedClient(cfg.Port, cfg.Tokens, *profile)
	if err != nil {
		fmt.Fprintf(out, "拿不到 %s 的 REST 令牌: %v\n", *profile, err)
		return 1
	}
	cur, err := c.profileData(*profile)
	if err != nil {
		fmt.Fprintf(out, "读档案 %s 失败: %v\n", *profile, err)
		return 1
	}
	if have := bookimport.CountItems(cur.Data); have > 0 {
		fmt.Fprintf(out, "档案 %s 里已经有 %d 条书签。\n"+
			"  导入是\"把浏览器那套导进来\"，不是往现有档案上叠加——现在跑会把那 %d 条无声顶掉。\n"+
			"  换一个空档案，或者先自己在界面上清空它。\n", *profile, have, have)
		return 1
	}

	ops := bookimport.Ops(res.Bookmarks)
	fmt.Fprintf(out, "将要提交 %d 条 op（%d 个分组 + %d 条书签）到档案 %s，expectedRev=%d\n",
		len(ops), len(res.Bookmarks.Groups), bookimport.CountItems(res.Bookmarks), *profile, cur.Rev)
	if *dryRun {
		fmt.Fprintln(out, "（--dry-run：没有提交）")
		return 0
	}

	body, _ := json.Marshal(map[string]any{
		"profile": *profile, "expectedRev": cur.Rev, "by": *by, "ops": ops,
	})
	rawResp, code, err := c.do("POST", "/api/v1/batch", body,
		map[string]string{"Content-Type": "application/json", "X-LB-By": *by})
	if err != nil {
		fmt.Fprintf(out, "提交失败: %v\n", err)
		return 1
	}
	if err := mustOK(rawResp, code, "/api/v1/batch"); err != nil {
		fmt.Fprintf(out, "提交失败: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "已导入档案 %s\n", *profile)

	return 0
}

func countItems(b *model.Bookmarks) int {
	n := 0
	for _, g := range b.Groups {
		n += len(g.Items)
	}
	return n
}

// dataResp 是 GET /api/v1/data 的响应，只留本命令关心的四格。
//
// 服务端那份叫 dataResponse 且是私有的，这里另起一个同名形状：**这不是分家**——
// CLI 是客户端，只需要读它那几格，一致性由 REST 契约（AGENTS.md 那张端点表）
// 保证，不靠两处 Go 类型长得一样。真正的分家是同一个包里两个判据各算一次。
type dataResp struct {
	Rev  int64            `json:"rev"`
	Data *model.Bookmarks `json:"data"`
}

// profileData 读一个档案当前的 rev 与内容（batch 的 expectedRev 就靠它）。
func (c *client) profileData(profile string) (*dataResp, error) {
	raw, code, err := c.do("GET", "/api/v1/data?profile="+url.QueryEscape(profile), nil, nil)
	if err != nil {
		return nil, err
	}
	if err := mustOK(raw, code, "/api/v1/data"); err != nil {
		return nil, err
	}
	var resp dataResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("响应读不出来: %w", err)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("响应里没有 data 字段——档案 %s 可能是空的或读不出来", profile)
	}
	return &resp, nil
}
