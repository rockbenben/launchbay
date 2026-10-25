package render

import (
	"os"
	"strings"
	"testing"
)

// 状态行只有一行，而 resolve 与写入会并发往它上面写（2026-10-24）：点「保存」
// 时地址格的 blur 先发 resolve，慢半步的那一发会把「这一条已经在这份里了」
// 盖成「已读到标题」——浮层没关、屏上一句成功，人以为保存没生效。
// 判据是**写入进场就把序号推到新的一格**，比它早出发的 resolve 回来后不再写。
func Test添加浮层_写入的答案不被慢半步的resolve盖掉(t *testing.T) {
	js, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	for _, want := range []string{
		"let statusSeq = 0;",
		"const seq = ++statusSeq;",
		"async function submit(keepOpen) {\n    ++statusSeq;",
		"if (seq !== statusSeq) return;",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("app.js 里少了 %q：写入与 resolve 的状态行竞态又回来了", want)
		}
	}
}
