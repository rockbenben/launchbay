package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/render"
)

// LB_SYNC_FINGERPRINT：每改一次前端就要手改两份文档，而那两份改动**都是机械替换**
// ——机械替换正是最该由机器做、也最容易被手滑漏掉的一类（漏一处要等下一轮 CI
// 才红，而那轮可能隔了几天）。这一族钉的是那道同步命令的**判据**，不是它的存在。
//
// 它最要紧的形状是：**同步完必须继续跑同一条校验**，所以它既是「改」也是「验」。
// 一个只会写不读的开关，写坏了当场没人知道。
func Test指纹同步写完仍要过同一条校验(t *testing.T) {
	src := readTestFile(t, "agents_test.go")

	// 一：命令**在跑校验之前先同步**（顺序反了就是"写完不验"）。
	syncAt := strings.Index(src, "syncAssetFingerprints(t)")
	if syncAt < 0 {
		t.Fatal("闸里没有调用 syncAssetFingerprints")
	}
	// 判据的读取必须在同步之后 —— 上面那句按后文比较：同步调用之后应当出现读文件。
	after := src[syncAt:]
	if !strings.Contains(after, `os.ReadFile("../../AGENTS.md")`) {
		t.Error("同步之后没有重新读 AGENTS.md：它写完就用旧内容验，等于没验")
	}
	if !strings.Contains(after, `os.ReadFile("openapi.json")`) {
		t.Error("同步之后没有重新读 openapi.json")
	}

	// 二：**按后缀分派**（.js→AppJSPath、.css→AppCSSPath）。
	// 一个不带 alternation 的正则会把 js 换成 css——那是把一份对文档改成另一份错文档，
	// 而两处一致地错着正是这道闸要抓的那种漂移。
	for _, p := range []struct{ kind, want string }{
		{`js`, render.AppJSPath},
		{`css`, render.AppCSSPath},
	} {
		re := regexp.MustCompile(`/app\\\.\[0-9a-f\]\{12\}\\\.` + p.kind)
		if !re.MatchString(src) {
			t.Errorf("同步用的正则不是按后缀分开的那一条（.%s 会被换成另一个的值）", p.kind)
		}
	}
	// 反向证人：**不许出现一个不分后缀的** `/app\.[0-9a-f]{12}\.` 直接替换——
	// 那正是上面那一种错的写法。
	if regexp.MustCompile(`/app\\\.\[0-9a-f\]\{12\}\\\.(js\|css)`).MatchString(src) {
		t.Error("同步正则带了 (js|css) 分派：一个值要同时替换两种后缀，会把 js 换成 css")
	}

	// 三：**按位置抓，不按"旧值是 X"反推**。文档里可能同时留着当前值与更旧的值
	// （那正是要清掉的），而"反推旧值"要求先知道旧的是什么——它恰好在漂着的时候失效。
	if strings.Contains(src, "strings.ReplaceAll(body, \"app.") {
		t.Error("同步是按旧值反推的：文档里残留着更旧的那个值时就改不掉")
	}

	// 四：它是**闸的一部分**，不是独立脚本——所以它跑在 `go test ./...` 里，
	// 而环境变量（不是 flag）是开关：flag 每个包各自注册，`go test ./... -update`
	// 会让没定义它的那些包因未知 flag 而失败。
	if !strings.Contains(src, `os.Getenv("LB_SYNC_FINGERPRINT")`) {
		t.Error("同步开关不是环境变量：用 flag 的话全量跑会被未定义的 flag 卡住")
	}
}

// readTestFile 把测试文件读成字符串（找不到就当场红，而不是静默跳过）。
func readTestFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(".", name))
	if err != nil {
		t.Fatalf("读 %s: %v", name, err)
	}
	return string(b)
}
