package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/apierr"
)

// 错误码这套东西**只有在三处说的是同一份时才有意义**：代码里的常量、
// openapi.json 的枚举、AGENTS.md 那张表。任何一处漏一格，症状都不是编译错，而是
// 某个读者学不到那一码存在（AI 照文档生成请求，人照界面猜含义）。
//
// 所以这里判"集合相等"，两个方向都判：
//   - 代码有、文档没有 → 调用方永远学不到它（最坏的一种静默）；
//   - 文档有、代码没有 → 文档在骗人，而 AGENTS.md 开头就写着"文档骗 AI 比骗人更贵"。
func Test错误码三处说的是同一份(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "apierr", "apierr.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := map[string]bool{}
	for _, k := range constKindValues(string(src)) {
		code[k] = true
	}
	if len(code) < 20 {
		t.Fatalf("只扫到 %d 个 kind——扫描器失灵与「真的就这么几个」看起来一模一样", len(code))
	}
	for k := range apierr.Registry {
		if !code[k] {
			t.Errorf("登记表里的 %q 在常量区扫不到：登记表与常量已经分家", k)
		}
	}

	doc := openapiKindEnum(t)
	for k := range code {
		if !doc[k] {
			t.Errorf("openapi 的 kind 枚举少了 %q：照契约生成的客户端学不到它", k)
		}
	}
	for k := range doc {
		if !code[k] {
			t.Errorf("openapi 写了 %q，代码里没有：契约在骗人", k)
		}
	}

	agents := agentsTableKinds(t)
	for k := range code {
		if !agents[k] {
			t.Errorf("AGENTS.md 那张表少了 %q：AI 读者照文档办事时不知道这一码存在", k)
		}
	}
	for k := range agents {
		if !code[k] {
			t.Errorf("AGENTS.md 写了 %q，代码里没有：文档在骗 AI", k)
		}
	}
}

// 每个 kind 的**默认状态码**也得对得上：登记表说 422、文档说 403，
// 那就是有一处已经过期，而界面句子是按它选的。
func Test错误码的默认状态与文档同档(t *testing.T) {
	agents := agentsTableRows(t)
	for k, info := range apierr.Registry {
		row, ok := agents[k]
		if !ok {
			continue // 上一条测试已经报过"表里少了这一码"
		}
		if !strings.Contains(row, "| "+strconv.Itoa(info.Status)+" |") && !strings.Contains(row, "| "+strconv.Itoa(info.Status)+" ") {
			t.Errorf("%q：登记表说默认 %d，AGENTS.md 那一行写的是「%s」", k, info.Status, strings.TrimSpace(row))
		}
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

var kindDocRE = regexp.MustCompile(`(?m)^	([A-Z][A-Za-z]+)\s*= "([a-z0-9][a-z0-9-]*)"`)

func constKindValues(src string) []string {
	var out []string
	for _, m := range kindDocRE.FindAllStringSubmatch(src, -1) {
		out = append(out, m[2])
	}
	sort.Strings(out)
	return out
}

func openapiKindEnum(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Enum []string `json:"enum"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("openapi.json 读不动：%v", err)
	}
	k, ok := doc.Components.Schemas["ErrorKind"]
	if !ok {
		t.Fatal("openapi 里没有 ErrorKind 这个 schema：错误码没进契约")
	}
	out := map[string]bool{}
	for _, v := range k.Enum {
		out[v] = true
	}
	if len(out) == 0 {
		t.Fatal("openapi 的 ErrorKind 枚举是空的")
	}
	return out
}

// agentsTableKinds / agentsTableRows 解析 AGENTS.md 里那张码表：
// 第一格是 kind，第二格是默认状态码。表不存在直接 Fatal——"没有表"与
// "表里正好都对"不能都算通过。
func agentsTableRows(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		l := strings.TrimSpace(line)
		if !strings.HasPrefix(l, "| `") {
			continue
		}
		cells := strings.Split(l, "|")
		if len(cells) < 3 {
			continue
		}
		k := strings.Trim(strings.TrimSpace(cells[1]), "`")
		if !kindDocRE2.MatchString(k) {
			continue
		}
		out[k] = l
	}
	if len(out) == 0 {
		t.Fatal("AGENTS.md 里找不到错误码那张表")
	}
	return out
}

var kindDocRE2 = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)+$`)

func agentsTableKinds(t *testing.T) map[string]bool {
	rows := agentsTableRows(t)
	out := map[string]bool{}
	for k := range rows {
		out[k] = true
	}
	return out
}
