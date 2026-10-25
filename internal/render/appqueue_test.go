package render

import (
	"os"
	"strings"
	"testing"
)

// 机器不可达时的那条添加要进待办，重放要按盘上现状对账（2026-10-24）。
//
// 之前只认 `navigator.onLine === false`：Wi-Fi 还在、NAS 不在时只报一句错，
// 关掉浮层这次输入就没了。现在网络错误也进同一个待办；重放时先看盘上是否
// 已经有这个条目/分组，避免“请求其实写进去了、只是响应丢了”被写成第二条。
func Test离线待办_网络错误也进队列且重放对账(t *testing.T) {
	js, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	for _, want := range []string{
		"e.network = true",
		"if (e.network) {",
		"enqueue({ gid: g.gid, ops: g.ops, value });",
		"app.netQueued",
		"app.offline.retry",
		// 重放对账：同 id 已在且 URL 相同 → 算已到；同组同 URL → 也算已到；
		// 组已在就跳过建组那一条 op。
		"if (itemURLs.has(key)) {",
		"if (knownURLs && knownURLs.has(entry.value.url)) {",
		"if (groupIds.has(entry.gid) || createdGroups.has(entry.gid)) ops = [];",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("app.js 里少了 %q：网络错误的待办/对账逻辑被拆掉了", want)
		}
	}
}
