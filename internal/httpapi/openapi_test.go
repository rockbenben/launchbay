package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// planned 是"文档里写了但本案还没实现"的端点。列出来而不是藏起来，是因为
// AI 读 openapi 时应当知道一条通路的存在与形状；但每一条都必须
// 带 x-status: planned，测试才认它。
//
// 这张表是**豁免**，不是注册表：端点真的实现并注册之后，这里的一行连同文档
// 里的 x-status: planned 都必须撤掉，否则文档会继续宣称一条没实现的通路。
// 两道闸分别挡两种残留：文档标了 planned 而路由表已注册 → CoversEveryRoute
// 红；这张表还留着已注册的那条 → HasNoGhostEndpoints 红。
//
// P1-③（icons 搜索 + resolve）落地后本案不再欠任何计划中的端点，所以这张表
// **现在为空并保持为空**：它不是"端点登记表"，别把已实现的端点写进来。它得
// 继续存在，因为 plannedPaths()（agents_test.go）与三条 openapi 闸都读它——
// 下一个"P1-④ 才上线"的端点出现时，豁免仍然只能从这一处开。
var planned = map[string]bool{}

// xCredentialNames 把路由表的 Cred 翻译成文档 x-credential 的词汇表。
// 凭证在案有三处来源（路由表 Cred、文档 security、文档 x-credential），
// 只有 x-credential 没人消费——那就由 CoversEveryRoute 把它钉回 Cred，
// security 由同一条闸的 credSecuritySchemes 那半钉回 Cred（两半都不许自己漂）。
// 新增 cred 档位时这里必须同步补一条，否则测试直接红而不是静默漏检。
var xCredentialNames = map[cred]string{
	credNone:      "none",
	credScoped:    "token-or-session",
	credAdmin:     "admin",
	credSession:   "session",
	credTokenOnly: "token", // 只有档案作用域令牌，会话一律 401（2026-10-19）
}

// credSecuritySchemes 是同一份 Cred 在文档 `security` 那一格里该报的形状。写法是
// "每个 alternative 内的方案名排序后用 + 连，alternative 之间排序后用 `|` 连"，例如
// `bearerAuth|sessionCookie`（两种凭证**分别**都行）与 `bearerAuth+sessionCookie`
// （要同时给两种）。这两种形状在 OpenAPI 里语义相反，归一成一个字符串就等于放过
// "把 OR 写成 AND"那种改法——这里是逐 alternative 归一再排序，所以两种写法不同形。
// security 不是装饰：AI 读 openapi 时照着它选凭证，多写一个 bearerAuth 就等于教人
// 拿 Bearer 去打一条其实拒令牌的通路，少写 sessionCookie 则让人以为页面通路要令牌。
// Task 5 评审 I-2 实测到的正是前者——POST /admin 的 security 里留着一开始复制来的 bearerAuth。
var credSecuritySchemes = map[cred]string{
	credNone:      "",
	credSession:   "sessionCookie",
	credScoped:    "bearerAuth|sessionCookie",
	credAdmin:     "bearerAuth|sessionCookie",
	credTokenOnly: "bearerAuth", // 2026-10-19：照实报，不许顺手加 sessionCookie
}

// tokensRefusedByForm 是 Cred 与 API 同类、却**不认令牌**的浏览器表单通路：
// 文档只能报 sessionCookie。判据的权威不在这张表——"这条路收到 Bearer 就 303
// 且不写盘"由 TestAdmin_表单通路不认令牌 在行为那头钉着；这张表只回答"文档有没有
// 跟着行为走"。所以它的每一行都必须在路由表与文档里真的存在（下面 HasNoGhost…
// 之外这里再查一次），行为哪天变了，那张表要一起删。
// **加第二行之前先给它配同一条行为证人**：这一行的合法性来自 handler 真拒令牌，
// 不来自这张表自己。
// 2026-10-09 加入管理页那两条零 JS 写动作的四个端点：它们的 handler 第一件事
// 就是 `if bearer(r) != "" { redirectPicker(w) }`，与 handleAdminCreate 同一句。
var tokensRefusedByForm = map[string]bool{
	"POST /admin":         true,
	"GET /admin/pin":      true,
	"POST /admin/pin":     true,
	"GET /admin/delete":   true,
	"POST /admin/delete":  true,
	"GET /admin/disable":  true,
	"POST /admin/disable": true,
	// 回退那两条（2026-10-11）：判据同上——selfProfileOnly 第一件事就是
	// `if bearer(r) != "" { redirectPicker(w) }`。行为证人见
	// TestRevertPage_令牌与无会话都拒且不写盘。
	"GET /revert":  true,
	"POST /revert": true,
	// 改界面语言那条（2026-10-11）：判据同上——语言是**站级**设置，改了影响这台
	// 机器上的所有人，所以只给管理员会话，不接受令牌替身。行为证人见
	// TestAdminLanguage_令牌一律303且不写盘。
	"POST /admin/language": true,
}

// docOperation 是测试要读的 operation 字段（其余字段由 json 忽略）。
// Security 用指针：`"security": []`（公开）与"这一格根本没写"是两件事，
// 后者意味着 operation 继承了文档级 security，这条闸要把它揪出来。
type docOperation struct {
	Summary     string                        `json:"summary"`
	XStatus     string                        `json:"x-status"`
	XCredential string                        `json:"x-credential"`
	Security    *[]map[string]json.RawMessage `json:"security"`
}

// securitySchemes 把一条 operation 的 security 数组归成可比较的形状串：每个
// alternative 里的方案名排序后用 `+` 连，alternative 之间排序后用 `|` 连。
// OpenAPI 的语义是"alternative 之间取 OR、同一个 alternative 内的方案取 AND"，
// 两种写法归一之后必须还是两种形状（复扫实测：只按方案名摊平会让 OR 与 AND 同形——
// 把 GET /admin 的两条 alternatives 折成一条，新闸照样绿）。
func securitySchemes(op docOperation) string {
	if op.Security == nil {
		return ""
	}
	var alts []string
	for _, alt := range *op.Security {
		var names []string
		for scheme := range alt {
			names = append(names, scheme)
		}
		sort.Strings(names)
		alts = append(alts, strings.Join(names, "+"))
	}
	sort.Strings(alts)
	return strings.Join(alts, "|")
}

// openapi.json 里 /prefs 的 theme/layout 枚举是这组值集的**第五份副本**：
// 契约与 model 那张表各写一遍，加一档就会让 AI 照着机器契约写出去吃 422
// （评审 I-6 的同一笔账，复扫时点出来的那一份）。openapi.json 是 go:embed 的
// 静态产物、不在运行时拼装，所以这条用测试钉而不是改成生成——它红了要改的是
// 那份文件，绝不是把断言放宽。
func TestOpenAPI的枚举与model同集(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Properties map[string]struct {
							Enum      []string `json:"enum"`
							MaxLength *int     `json:"maxLength"`
						} `json:"properties"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatalf("openapi.json 读不出形状: %v", err)
	}
	form := doc.Paths["/prefs"]["post"].RequestBody.Content["application/x-www-form-urlencoded"]
	want := map[string][]string{"theme": model.ThemeValues, "layout": model.LayoutValues,
		"view": model.ViewValues, "addrLevel": model.AddrValues}
	// 解析器不许空转：带 enum 的四个字段都得真的带出 enum，否则下面的循环一次都不跑。
	// 2026-10-12 起 /prefs 有三个字段（多出来的 `title` 是页面名，无枚举、只有
	// maxLength），2026-10-13 起四个（`view` = 整页排法），2026-10-19 起五个
	// （`addrLevel` = 卡片第二行印什么）。所以这里数的是"四个带 enum 的 + 一个带
	// maxLength 的"，而不是笼统的 5：形状变了这条要能指出变的是哪一格。
	//
	// **2026-10-19 补上 addrLevel**：这一格此前只在 description 里（"五个至少给
	// 一个"），schema 的 properties 却还只有四个——而 description 与 schema 说的
	// 是同一件事。AI 照 schema 写请求就少了这一格，写出来的 `addrLevel` 被服务端
	// 收下（它认），可文档说的字段集合与真实的那一份分家了。
	if len(form.Schema.Properties) != 5 {
		t.Fatalf("/prefs 的表单 schema 有 %d 个字段，要 5 个（theme/layout/view/addrLevel/title）：形状变了这条正在空转",
			len(form.Schema.Properties))
	}
	for field, w := range want {
		got := form.Schema.Properties[field].Enum
		if strings.Join(got, ",") != strings.Join(w, ",") {
			t.Errorf("openapi 里 %s 的枚举 = %v，model 的表是 %v", field, got, w)
		}
	}
	if _, ok := form.Schema.Properties["title"]; !ok {
		t.Fatal("/prefs 的 schema 里没有 title 这一格：端点收了它而契约不说，就是文档骗 AI")
	}
	// 上限要**钉在契约里**并且钉的是那个数：只写"有 maxLength"等于放开到任何值都算过。
	if got := form.Schema.Properties["title"].MaxLength; got == nil || *got != 60 {
		t.Errorf("openapi 里 title 的 maxLength = %v，要 60（与 prefs.go 的 maxPageTitleRunes 同源）", got)
	}
}

func TestOpenAPICoversEveryRoute(t *testing.T) {
	s := New(Dependencies{})
	var doc struct {
		Paths map[string]map[string]docOperation `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatalf("openapi.json 不是合法 JSON: %v", err)
	}
	docOps := map[string]docOperation{}
	for path, methods := range doc.Paths {
		for m, op := range methods {
			key := strings.ToUpper(m) + " " + path
			docOps[key] = op
			if op.Summary == "" {
				t.Errorf("%s 在文档里没有 summary", key)
			}
			if planned[key] && op.XStatus != "planned" {
				t.Errorf("%s 是计划中的端点，必须标 x-status: planned", key)
			}
		}
	}
	var missing []string
	for _, rt := range s.routes() {
		key := rt.Method + " " + rt.Pattern
		op, ok := docOps[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		// summary 逐字对齐路由表的 Summary：文档里那句是 AI 读的，路由表
		// 那句是人写的，只靠实现者当场比对就一定会漂。
		if op.Summary != rt.Summary {
			t.Errorf("%s 的 summary 与路由表不一致：文档 %q，路由表 %q", key, op.Summary, rt.Summary)
		}
		// 已注册的端点不许带 planned 标记：那是在宣称"这条通路还没建"，
		// 而它已经能打了。P1-③ 补 /icons 时最容易忘撤的就是它。
		if op.XStatus == "planned" {
			t.Errorf("%s 已经注册了，文档里却还标 x-status: planned——实现落地就把标记撤掉", key)
		}
		want, ok := xCredentialNames[rt.Cred]
		if !ok {
			t.Errorf("%s 的 Cred(%d) 在 xCredentialNames 里没有对应词汇，测试要补", key, rt.Cred)
		} else if op.XCredential != want {
			t.Errorf("%s 的 x-credential = %q，路由表 Cred 对应 %q", key, op.XCredential, want)
		}
		// security 那一格：公开通路必须显式写 `[]`（少了它 operation 会继承
		// 文档级 security，而文档级那一条将来一加就静默改变全部通路的读法）。
		wantSec, ok := credSecuritySchemes[rt.Cred]
		if !ok {
			t.Errorf("%s 的 Cred(%d) 在 credSecuritySchemes 里没有对应形状，测试要补", key, rt.Cred)
			continue
		}
		if op.Security == nil {
			t.Errorf("%s 的文档没有 security 这一格：要么显式写，要么这条闸在空转", key)
			continue
		}
		if tokensRefusedByForm[key] {
			wantSec = "sessionCookie"
		}
		if got := securitySchemes(op); got != wantSec {
			t.Errorf("%s 的 security = %q，Cred(%s) 该报 %q", key, got, want, wantSec)
		}
	}
	// 豁免表不许留死行：它每加一行就多一处"Cred 说管理员、文档说只认会话"的
	// 合法形状，而那种形状只有在 handler 真拒令牌时才是真话。
	for key := range tokensRefusedByForm {
		if _, ok := docOps[key]; !ok {
			t.Errorf("%s 在 tokensRefusedByForm 里，文档里却没有这条通路：豁免行要删", key)
		}
		var registered bool
		for _, rt := range s.routes() {
			if rt.Method+" "+rt.Pattern == key {
				registered = true
			}
		}
		if !registered {
			t.Errorf("%s 在 tokensRefusedByForm 里，路由表却没有：豁免行要删", key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("路由表里有、文档里没有（文档骗 AI 比骗人更贵）:\n  %s", strings.Join(missing, "\n  "))
	}
}

// 反向：文档不许写幽灵端点。planned 之外每一条都必须真的注册了。
func TestOpenAPIHasNoGhostEndpoints(t *testing.T) {
	s := New(Dependencies{})
	registered := map[string]bool{}
	for _, rt := range s.routes() {
		registered[rt.Method+" "+rt.Pattern] = true
	}
	// planned 表必须与已注册集合互斥：这张表是"本案还没实现"的声明，端点
	// 上线后留着那一行，等于让测试替一条早就建好的通路继续豁免。
	for key := range planned {
		if registered[key] {
			t.Errorf("%s 已经注册了，却还留在 openapi_test.go 的 planned 表里：撤掉这行豁免（连同文档里的 x-status: planned）", key)
		}
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatal(err)
	}
	for path, methods := range doc.Paths {
		for m := range methods {
			key := strings.ToUpper(m) + " " + path
			if !registered[key] && !planned[key] {
				t.Errorf("文档里有 %s，路由表里没有：要么补实现，要么把它加进 openapi_test.go 的 planned 表并在文档里标 x-status: planned（豁免看的是那张表，光在文档里标 x-status 不生效）", key)
			}
		}
	}
}

// 方法集合也要对上：GET /api/v1/profiles 与 DELETE /api/v1/profiles 是两条，
// 文档里只写一条就是契约测试没意义。
func TestOpenAPIMethodSetsMatchPerPath(t *testing.T) {
	s := New(Dependencies{})
	byPath := map[string]map[string]bool{}
	for _, rt := range s.routes() {
		if byPath[rt.Pattern] == nil {
			byPath[rt.Pattern] = map[string]bool{}
		}
		byPath[rt.Pattern][strings.ToLower(rt.Method)] = true
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatal(err)
	}
	for path, got := range byPath {
		want := doc.Paths[path]
		if want == nil {
			t.Fatalf("文档里没有 %s", path)
		}
		for m := range got {
			if _, ok := want[m]; !ok {
				t.Errorf("%s 在路由表里有 %s，文档里没有", path, m)
			}
		}
		for m := range want {
			if !got[m] && !planned[strings.ToUpper(m)+" "+path] {
				t.Errorf("%s 文档里写了 %s，路由表里没有", path, m)
			}
		}
	}
}

// 端点本身公开可取（无个人信息），且字节与内嵌文档一致。
func TestOpenAPIEndpointServesTheEmbeddedDoc(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/api/v1/openapi.json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("公开端点该 200, got %d", rec.Code)
	}
	if rec.Body.String() != string(openAPIDoc) {
		t.Fatal("响应与内嵌文档不一致（中间有加工？）")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
}
