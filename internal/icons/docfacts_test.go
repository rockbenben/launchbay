package icons

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// doc.go 里那个产物样例里的 `"count"` 是**自证的锚**：读者照着它去解那份
// gzip 索引，能验证"这一层到底收了多少条"。而它**会漂**——上游的图标词表更新
// 一次，真实条数就变，而 doc.go 里的样例数字还停在从前。
//
// 2026-10-20 那一轮审计就查到过：doc.go 写 6268，真实是 6265（上游的
// upstreamTotal 仍是 6220——那个差是**词表与实际收录之间的口径差**，如实留着 ✓）。
//
// **这条钉住它**：真实条数变了而 doc.go 没跟着改，当场红，并指出该改哪一行。
func TestDocGo里那个自证的锚与真实条数一致(t *testing.T) {
	raw, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	ix, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	// doc.go 的样例是 **tabler** 那一层（上游是 @iconify-json/tabler）。
	re := regexp.MustCompile(`"count":\s*(\d+)`)
	m := re.FindSubmatch(raw)
	if m == nil {
		t.Fatal("doc.go 里找不到样例的 count 字段——那个自证的锚不见了，这条测试要改")
	}
	want, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	got := ix.Count("tabler")
	if want != got {
		t.Errorf("doc.go 里写 count=%d，真实是 %d——那个自证的锚漂了（上游的图标词表更新过？）", want, got)
	}
}
