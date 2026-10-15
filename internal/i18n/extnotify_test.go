package i18n

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 通知点下去落在哪儿那张表的守卫（2026-10-16）。
//
// 为什么要有这一条：bg.js 的 onClicked 从前无条件 openOptionsPage()，而
// errDuplicate / errGoneTarget / errOpsBatch 那几句教的是"去页面上改"——
// 一句话同时指两个地方。修法是给每一个错误 key 配一格出路；而**配漏一格**的
// 症状与没修之前一模一样（点了不知道去哪），并且不会有任何报错。所以这张表
// 的完整性要有证人，不能靠"看着都写了"。
//
// 与 extcatalog_test.go 同一族：它钉两份语言包的 key 一一对应，这一条钉
// "认得出的每一码都说得出去哪儿"。

var (
	// jsMapValues 抓 `const KIND = { 'a': 'b', ... }` 里冒号**右边**那些值，
	// 也就是 NOTE_TARGET 该收的那些 key。
	jsMapBlock  = regexp.MustCompile(`(?s)const (KIND|STATUS|NOTE_TARGET) = \{(.*?)\n\};`)
	jsMapValue  = regexp.MustCompile(`'([A-Za-z0-9_-]+)'\s*:\s*'([A-Za-z0-9_-]+)'`)
	jsMapTarget = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_]+):\s*'(page|settings)',`)
)

func jsBlocks(t *testing.T, src, name string) string {
	t.Helper()
	m := jsMapBlock.FindAllStringSubmatch(src, -1)
	for _, g := range m {
		if g[1] == name {
			return g[2]
		}
	}
	t.Fatalf("i18n.js 里找不到 const %s = {…}；这张表的形状与这条证人假设的对不上", name)
	return ""
}

func Test扩展每个错误key都说得出去哪儿(t *testing.T) {
	src, err := os.ReadFile("../../extension/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)

	targets := map[string]string{}
	for _, m := range jsMapTarget.FindAllStringSubmatch(jsBlocks(t, s, "NOTE_TARGET"), -1) {
		targets[m[1]] = m[2]
	}
	if len(targets) == 0 {
		t.Fatal("NOTE_TARGET 一格都没读到，正则与文件形状对不上（这条证人自己空转了）")
	}

	// KIND 的**值**与 STATUS 的**值**就是那句人话查的 key；两处的每一个都必须有出路。
	var missing []string
	for _, name := range []string{"KIND", "STATUS"} {
		for _, g := range jsMapValue.FindAllStringSubmatch(jsBlocks(t, s, name), -1) {
			if targets[g[2]] == "" {
				missing = append(missing, name+" 的 "+g[1]+" → "+g[2])
			}
		}
	}
	// bg.js 里那几格**本地**判据（没配地址、这个档案没令牌、一个分组都没有）
	// 同样要有一格出路——它们不来自服务端，所以上面那一族扫不到。
	for _, k := range []string{"noBase", "noToken", "noGroup", "netFail"} {
		if targets[k] == "" {
			missing = append(missing, "bg.js 本地的 "+k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("这些错误 key 在 NOTE_TARGET 里没有格子，点了通知就还是不知道去哪：%s",
			strings.Join(missing, "、"))
	}

	// 出路只有两种写法。第三种值（打错成一个 URL）会让 onClicked 那一句
	// `where === 'page' ? 'page' : 'settings'` 悄悄把它折成 settings。
	for k, v := range targets {
		if v != "page" && v != "settings" {
			t.Errorf("NOTE_TARGET[%s] = %q，只许 page / settings", k, v)
		}
	}
	// 兜底那一句必须是 settings：认不出的码说"去页面上改"是假话——
	// 页面不知道发生了什么。
	if !strings.Contains(s, "return NOTE_TARGET[key] || 'settings';") {
		t.Error("noteTarget 的兜底不再是 settings：认不出的错误会把人支到页面上去")
	}
}

// 那句"去页面上改"的文案与它自己的出路不许分家：说这句话的 key 必须落在 page 那一档。
func Test说去页面上改的那几句真的把人带去页面(t *testing.T) {
	src, err := os.ReadFile("../../extension/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	targets := map[string]string{}
	for _, m := range jsMapTarget.FindAllStringSubmatch(jsBlocks(t, s, "NOTE_TARGET"), -1) {
		targets[m[1]] = m[2]
	}
	msgs, err := os.ReadFile("../../extension/_locales/zh_CN/messages.json")
	if err != nil {
		t.Fatal(err)
	}
	m := string(msgs)
	for _, key := range []string{"errDuplicate", "errGoneTarget", "errOpsBatch", "errIconName"} {
		i := strings.Index(m, `"`+key+`"`)
		if i < 0 {
			t.Fatalf("中文包里找不到 %s，这条证人扫错了地方", key)
		}
		window := m[i : i+400]
		saysPage := strings.Contains(window, "去页面上") || strings.Contains(window, "在页面上")
		if saysPage && targets[key] != "page" {
			t.Errorf("%s 的句子教人「去页面上」，NOTE_TARGET 却给的是 %q——点通知会落到设置页",
				key, targets[key])
		}
	}
}
