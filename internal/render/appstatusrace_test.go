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
		// submit 是防重入的壳，真正落库的是 submitOnce；序号必须在它一进场就推。
		"async function submitOnce(keepOpen) {\n    ++statusSeq;",
		// bookmarklet 预填在模块初始化时就调 resolveNow；fetchMeta 若写成 const
		// 会撞 TDZ，异常只在 console 里、页面看上去正常。
		"function fetchMeta(url) {",
		// 两发 submit 重叠时第二发必须早退（按钮 disabled 挡不住地址格里按回车）。
		"if (submitting) return;",
		// 保存中不许关浮层；关掉会重载，把还在路上的请求/响应一起丢掉。
		"function closeSheet() {\n    if (submitting) return;",
		// 等读取时锁住地址格，并防止旧响应写进重开后的新表单。
		"fUrl.readOnly = true;",
		"if (gen !== sheetGen || sheet.hidden) return;",
		"if (seq !== statusSeq || gen !== sheetGen) return;",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("app.js 里少了 %q：写入与 resolve 的状态行竞态又回来了", want)
		}
	}
}
