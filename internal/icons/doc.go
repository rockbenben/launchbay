// Package icons 是服务端内嵌的双层图标索引：tabler（通用层，线条）与
// simple-icons（品牌层，实心）。它做三件事——按名字给出 path 数据、按关键词
// 搜索候选、把 URL 抓成标题与 favicon（fetch.go）。
//
// 索引产物是提交进仓库的 gzip 文件（spec §5、§10）：构建期不联网、不装 node，
// 本地 go build、门禁跑的测试与镜像里嵌的是同一份字节。刷新图标库是人工跑
// `go run ./tools/genicons` 并重新提交，不是重建前端。
//
// 产物形状（一套一个文件，gzip 后的 JSON）：
//
//	{
//	  "prefix": "tabler",
//	  "layer": "stroke",                  // stroke | fill，决定 CSS 呈现
//	  "viewBox": "0 0 24 24",
//	  "count": 6268,                      // 实际收录条数：自证的锚
//	  "upstreamTotal": 6220,              // 上游声明的 total；口径差如实留着
//	  "upstream": "@iconify-json/tabler@1.2.41",
//	  "generatedAt": "2026-10-06T12:00:00Z",
//	  "license": "MIT",
//	  "icons": {
//	    "home": "M9 4H5a2 2 0 0 0-2 2v10a2 2 ...",
//	    "alarm-filled": [["path", "fill", "currentColor", "d", "M..."]]
//	  },
//	  "aliases": { "alarm-light": "alarm" }
//	}
//
// 一条图标有两种写法：单个 `<path>` 且没有任何偏离本层默认的呈现属性时，
// 值就是裸的 `d` 字符串；其余是元素数组，`[tag, 名, 值, 名, 值...]` 扁平排开。
// 之所以值得为常见情形省掉整个包装：simple-icons 的 3737 条全是单 path 且
// `fill="currentColor"`（实测 2026-10-06），那是本层的默认值，存下来等于把
// 1.8MB 花在重复同一个常量上。
//
// 三个入口的分工（谁做什么、谁还没做）：
//
//   - Decode：一套**未压缩**产物 JSON → 校验过的 *Index。全量校验都在它身上
//     （layer/viewBox 形状、count 自证、白名单四件套、alias 不许悬空），
//     任一处不合就整份返回 error，绝不带半截数据上首屏。本任务已实现、已证。
//   - Merge：几套 *Index → 一个查询面。前缀重叠（同一个 `<prefix>:<name>` 出现
//     两次）是编程错误，直接 panic，不静默覆盖。本任务已实现，panic 分支由
//     TestMerge_同前缀的两套产物必须炸 作证。
//   - Load / MustLoad：读内嵌的 `index/*.json.gz`、解压，再走上面两条（load.go，
//     已实现、已证）。每一处失败都包上 ErrNoIndex——索引读不出来是装配错误而不是
//     运行时数据问题，装配点（cmd/launchbay）直接退出，不降级成"没有图标"。
//     MustLoad 给装配与测试用；刷新图标库仍是人工跑 `go run ./tools/genicons`。
package icons
