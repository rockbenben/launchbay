package httpapi

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/ops"
)

// 文档与机器契约里逐字列着的那三张白名单，必须与**代码里那一份**逐字相同。
//
// **为什么要有这条**：2026-10-20 那一轮全量审计查出的 39 处不符里，有一处是
// `layout` 加了一档（feature）而文档没跟上——手抄的列表与代码分家，于是人照着
// 文档写出一个被 422 拒掉的字段。这一族（分组 / 条目 / 根级三张白名单）全是手抄的，
// 每加一档就会再分家一次，而分家的那一次总是等到有人被拒才知道。
//
// **判据只有一份**：代码里那三张表（`internal/ops/path.go`）。文档照它写，
// 这道闸照它查——文档与代码不一致时，红的是**测试**，而不是下一次 422。
func Test文档与契约里的白名单与代码同集(t *testing.T) {
	agentsMD, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	agents := string(agentsMD)

	// ① 机器契约（openapi.json 的 Op.path 描述）：三张表各写成一段「/」连起来的
	//    名字（`title/layout/accent` 那样）。逐段比**集合**（不比顺序：文档里那三段
	//    按"人先看的顺序"排，而代码那份是字典序——顺序不同不算不符）。
	desc := opPathDescription(t)
	for _, w := range [][]string{ops.GroupFields(), ops.ItemFields(), ops.DocFields()} {
		if !hasTokenRun(desc, w) {
			t.Errorf("openapi.json 的 Op.path 描述里没有「%s」这一串——白名单变了，那一句要跟着改",
				strings.Join(w, "/"))
		}
	}

	// ② AGENTS.md：三张表各在**一句**里，句子有唯一的锚点。取锚点之后的那一段，
	//    把里面的反引号词抠出来比集合。
	for _, c := range []struct {
		anchor string
		fields []string
	}{
		{"**条目能 `set` 的字段只有七个**：", ops.ItemFields()},
		{"**分组能 `set` 的字段是** ", ops.GroupFields()},
		{"**根级标量字段可以 set**（2026-10-14 改判）：", ops.DocFields()},
	} {
		got, ok := tokensAfter(agents, c.anchor)
		if !ok {
			t.Errorf("AGENTS.md 里找不到「%s」那一句——它被挪走了或改写了，那道锚点要跟着改", c.anchor)
			continue
		}
		if !sameSet(got, c.fields) {
			t.Errorf("AGENTS.md 里那句话列的是 %v，而代码那份是 %v——白名单变了，那一句要跟着改",
				got, c.fields)
		}
	}
}

// opPathDescription 取 openapi.json 里 Op schema 的 path 那一格描述。
func opPathDescription(t *testing.T) string {
	t.Helper()
	var doc struct {
		Components struct {
			Schemas struct {
				Op struct {
					Properties struct {
						Path struct {
							Description string `json:"description"`
						} `json:"path"`
					} `json:"properties"`
				} `json:"Op"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatalf("openapi.json 读不出形状: %v", err)
	}
	d := doc.Components.Schemas.Op.Properties.Path.Description
	if d == "" {
		t.Fatal("openapi.json 的 Op.path 没有 description——这一格是 AI 照着写请求时唯一的线索")
	}
	return d
}

// hasTokenRun 说文字里有没有**一串**用「/」连起来的名字，恰好是这些字段。
//
// 为什么按「串」找而不是按句子找：那句描述里还有路径文法（`groups/<gid>/…`）与
// 被拒的那几项（`rev`/`groups`/`search`/`weather`），按句切会切出一堆半截。
// 而「/」连起来的一串名字正是文档列白名单时用的形状——抓那一种就够，
// 抓多了反而会把路径文法也算进去。
var tokenRunRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:/[A-Za-z][A-Za-z0-9]*)+`)

func hasTokenRun(text string, fields []string) bool {
	want := map[string]bool{}
	for _, f := range fields {
		want[f] = true
	}
	for _, m := range tokenRunRe.FindAllString(text, -1) {
		got := map[string]bool{}
		for _, t := range strings.Split(m, "/") {
			got[t] = true
		}
		if len(got) != len(want) {
			continue
		}
		same := true
		for k := range want {
			if !got[k] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// tokensAfter 取锚点之后那一段里的反引号词，直到段末的「；」「。」「，」为止。
//
// 为什么切在标点上：这三句后面都跟着解释（"写别的会被点名拒掉并整批 422"），
// 不切就会把解释里的词也抠进来。**不切换行**——那三句里有两句跨行，
// 而「/」连起来的名单常常被排版折到下一行。
func tokensAfter(s, anchor string) ([]string, bool) {
	i := strings.Index(s, anchor)
	if i < 0 {
		return nil, false
	}
	rest := s[i+len(anchor):]
	if j := strings.IndexAny(rest, "；。，"); j >= 0 {
		rest = rest[:j]
	}
	re := regexp.MustCompile("`([^`]+)`")
	matches := re.FindAllStringSubmatch(rest, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out, true
}

// sameSet 比两个字符串集合（顺序不计）。
func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}
