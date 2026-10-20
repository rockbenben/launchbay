package httpapi

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/store"
)

// 文档里那些**不带日期的数字**必须是真的，而且由这一条钉着。
//
// **为什么要有这条**：2026-10-20 那一轮全量审计查出 39 处文档与实现不符，其中
// 好几处是数字漂了——而仓库自带的契约闸**一条都没报**（它们只管"有没有这一条
// 端点"，不管"这一条说的数字对不对"）。于是这类漂移只能靠人记得回去改。
//
// **两类数字，两种处置**（这是这一条的全部规则）：
//
//  1. **不带日期的数字 = 对当下的断言** → 由这里钉住。改了实现忘了改文档，
//     这一条当场红，并指出是哪一句。
//  2. **会变的数字（读数、步数、条数）= 那一天的快照** → 文档里**必须带日期**，
//     于是它不再冒充"当下是这样"，而是明说"那一次是这样"。不钉它：快照本就不该
//     被当成断言，钉它只会在每次改动后多一条假警报。
//
// 现在钉住的是这几个：
//   - by 渠道的个数（六个）——渠道枚举只在真要加一路时才变；
//   - 常用图标清单的条数（41）——它是一份人工挑的名单，只有整理时才变；
//   - 图标库的总条数（"一万多个"）——它涨到两万个时那句就得改。
func Test文档里不带日期的数字都对得上(t *testing.T) {
	agentsMD, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	agents := string(agentsMD)

	// ① by 渠道的个数：AGENTS.md 写"共六个"，真值是 store.ByValues 的长度。
	//     （文档里那个数是**中文数字**——"共六个"而不是"共 6 个"。）
	zhNum := []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九", "十"}
	zh := "共" + strconv.Itoa(len(store.ByValues)) + "个"
	if len(store.ByValues) < len(zhNum) {
		zh = "共" + zhNum[len(store.ByValues)] + "个"
	}
	if !strings.Contains(agents, zh) {
		t.Errorf("AGENTS.md 里 by 渠道的个数不是「%s」了（真实 %d 条）——渠道枚举变了，那一句要跟着改",
			zh, len(store.ByValues))
	}

	// ② 常用图标清单的条数：写进机器契约的那一句（"其中 41 个"）。
	want := strconv.Itoa(len(icons.Starter))
	if !strings.Contains(string(openAPIDoc), want+" 个") {
		t.Errorf("openapi.json 里常用图标清单的条数不是 %s 个（真实 %d 颗）——名单变了，那一句要跟着改",
			want, len(icons.Starter))
	}
	// **那个数只住一处**（在契约里）。AGENTS.md 也写一份就是两份真相，而它们会分家。
	if strings.Contains(agents, "常用清单只是其中 ") {
		t.Errorf("AGENTS.md 里也写了常用清单的条数——那个数只该住在契约里一处")
	}

	// ③ 图标库的总条数：文档说"一万多个"（一个范围，不是一个精确数）。
	//     真值涨到两万个时，那句"一万多个"就成了假话——所以这里守住那个范围。
	ix, err := icons.Load()
	if err != nil {
		t.Fatal(err)
	}
	total := len(ix.AllNames())
	if !regexp.MustCompile(`一万多个|一万个|10,000\+`).MatchString(agents) {
		t.Errorf("AGENTS.md 里没有「一万多个」那句了（真实 %d 个）——那句话是这个范围的断言", total)
	}
	if total < 10000 || total >= 20000 {
		t.Errorf("图标库现在是 %d 个，而文档说的是「一万多个」——那个范围已经不对了，那几句要改", total)
	}
}

// ④ 机器契约里的 `by` 枚举 = `store.ByValues` 那一份。
//
//	渠道枚举住在 `internal/store/history.go`，而 openapi.json 是**另一份手抄**——
//	两份会分家：加了一路渠道而契约没改，AI 照契约写出的 `by` 就被 422 拒
//	（batch 的 `by` 非法值答 422 并点名 `by`）。
//
//	上一条钉的是 AGENTS.md 那一侧的**个数**；个数对而**名字**错是另一种漂
//	（比如把 `extension` 写成 `ext`），所以这里逐个比名字。
func Test契约里的by枚举与store同集(t *testing.T) {
	var doc struct {
		Components struct {
			Schemas struct {
				BatchRequest struct {
					Properties struct {
						By struct {
							Enum []string `json:"enum"`
						} `json:"by"`
					} `json:"properties"`
				} `json:"BatchRequest"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatalf("openapi.json 读不出形状: %v", err)
	}
	got := doc.Components.Schemas.BatchRequest.Properties.By.Enum
	if len(got) == 0 {
		t.Fatal("openapi.json 的 BatchRequest.by 没有 enum——那一格是机器可读的渠道名单，缺了它 AI 只能猜")
	}
	if strings.Join(got, ",") != strings.Join(store.ByValues, ",") {
		t.Errorf("openapi.json 的 by 枚举 = %v，而 store.ByValues = %v——渠道变了，两处都要改",
			got, store.ByValues)
	}
}
