package httpapi

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/render"
)

// apiPathRE 抓 AGENTS.md 里出现的每一个 /api/v1/... 字面量。字符类含点是为了
// 完整抓到 /api/v1/openapi.json；末尾要求一个字母，是为了不吃掉紧跟其后的中文
// 标点/句号/星号——"/api/v1/data." 只抓 /api/v1/data，"/api/v1/profiles*" 只抓
// /api/v1/profiles。路由表里的模式全由小写字母、数字、/ 与一个 .json 组成。
var apiPathRE = regexp.MustCompile(`/api/v1/[a-z0-9./]*[a-z0-9]`)

// plannedPaths 从 openapi_test.go 的 planned 表（键是 "METHOD /path"）抽出裸路径，
// 供反向闸判断"文档写了但路由表没有"是不是有意的计划端点。planned 只有一份，
// 就是 openapi_test.go 那张豁免表——这里不另立一份，否则两张表会各自漂。
func plannedPaths() map[string]bool {
	m := map[string]bool{}
	for k := range planned {
		if i := strings.IndexByte(k, ' '); i >= 0 {
			m[k[i+1:]] = true
		}
	}
	return m
}

func knownAPIPath(s *Server, p string) bool {
	for _, rt := range s.routes() {
		if rt.Pattern == p {
			return true
		}
	}
	return false
}

// 真实指纹由资源字节生成，路由与 OpenAPI 必须使用当前具体路径。
// AGENTS.md 只记录稳定模式，避免每次修改资源都改操作说明；只对当前这两条
// 已注册资源归一化，不能把任意带指纹的路径当作豁免。
// LB_SYNC_FINGERPRINT=1 自动同步 OpenAPI，之后继续执行同一套校验。
var syncFingerprint = os.Getenv("LB_SYNC_FINGERPRINT") != ""

func Test静态资产指纹契约一致(t *testing.T) {
	if syncFingerprint {
		syncAssetFingerprints(t)
	}
	doc, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	for _, rt := range New(Dependencies{}).routes() {
		routes[rt.Pattern] = true
	}
	raw, err := os.ReadFile("openapi.json")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{render.AppJSPath, render.AppCSSPath} {
		if !routes[want] {
			t.Errorf("路由表里没有 %s", want)
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("openapi.json 里没有 %s", want)
		}
		if !strings.Contains(string(doc), agentsDocPath(want)) {
			t.Errorf("AGENTS.md 里没有 %s", agentsDocPath(want))
		}
	}

	// 操作说明不再维护具体值，连当前指纹也不应被同步器写回。
	for _, m := range regexp.MustCompile(`/app\.[0-9a-f]{12}\.(js|css)`).FindAllString(string(doc), -1) {
		t.Errorf("AGENTS.md 不应写死静态资产指纹 %s，请使用 <hash> 模式", m)
	}
}

// agentsDocPath 只归一当前已注册的两个资源，其余路径原样校验。
func agentsDocPath(path string) string {
	switch path {
	case render.AppJSPath:
		return "/app.<hash>.js"
	case render.AppCSSPath:
		return "/app.<hash>.css"
	default:
		return path
	}
}

func TestAgentsDocPath只归一已注册静态资产(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{render.AppJSPath, "/app.<hash>.js"},
		{render.AppCSSPath, "/app.<hash>.css"},
		{"/app.000000000000.js", "/app.000000000000.js"},
		{"/app.000000000000.css", "/app.000000000000.css"},
		{"/other.000000000000.css", "/other.000000000000.css"},
		{"/api/v1/data", "/api/v1/data"},
		{"/sw.js", "/sw.js"},
	} {
		if got := agentsDocPath(tc.path); got != tc.want {
			t.Errorf("agentsDocPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// syncAssetFingerprints 只维护 OpenAPI；AGENTS.md 的模式不随内容变化。
func syncAssetFingerprints(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	// 按后缀分别替换，不把脚本与样式互换。
	body = regexp.MustCompile(`/app\.[0-9a-f]{12}\.js`).ReplaceAllString(body, render.AppJSPath)
	body = regexp.MustCompile(`/app\.[0-9a-f]{12}\.css`).ReplaceAllString(body, render.AppCSSPath)
	if body == string(raw) {
		return
	}
	if err := os.WriteFile("openapi.json", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AGENTS.md 里写到的每个端点都必须真的在路由表里；反过来路由表里除浏览器
// 通路外的每条也都必须被文档点名。文档漂了要红，而不是等 AI 打出一个 404 才
// 发现——这条闸防的是"文档比代码旧"（新加端点忘了写进 AGENTS.md）。
func TestAgentsMDMatchesRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatalf("AGENTS.md 读不到: %v", err)
	}
	text := string(raw)
	s := New(Dependencies{})
	plannedSet := plannedPaths()
	// 正向：路由表里除浏览器通路外每条都要在 AGENTS.md 出现。
	for _, rt := range s.routes() {
		if rt.Pattern == "/" || rt.Pattern == "/pin" || rt.Pattern == "/healthz" {
			continue // 浏览器通路，AI 契约里不必逐条点名
		}
		if !strings.Contains(text, agentsDocPath(rt.Pattern)) {
			t.Errorf("路由表里有 %s，AGENTS.md 没提", rt.Pattern)
		}
	}
	// 反向：AGENTS.md 里出现的 /api/v1/... 字面量必须是已注册或 planned 的。
	for _, m := range apiPathRE.FindAllString(text, -1) {
		if !knownAPIPath(s, m) && !plannedSet[m] {
			t.Errorf("AGENTS.md 写了 %s，但服务里没有这个端点", m)
		}
	}
}

// AGENTS.md 的端点表里出现的方法名。表头是「方法」，正文里任何以这五个词
// 开头的行都当作一行契约来核对。
var methodCells = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

// docRows 抽出 AGENTS.md 表格里每一行的 (方法, 路径)。路径外层可能裹着反引号，
// 剥掉再比。表以外的散文（如 `/api/v1/profiles*`）不在这里——那条由
// TestAgentsMDMatchesRoutes 的反向闸按字面量管。
func docRows(text string) [][2]string {
	var out [][2]string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		m := strings.TrimSpace(cells[1])
		if !methodCells[m] {
			continue
		}
		path := strings.Trim(strings.TrimSpace(cells[2]), "`")
		if !strings.HasPrefix(path, "/") {
			continue
		}
		out = append(out, [2]string{m, path})
	}
	return out
}

// 文档点名了路径还不够：方法写错，AI 会照着发一个永远 405 的请求，而
// TestAgentsMDMatchesRoutes 那条"路径出现在文档里"的闸照样绿。这里把两列
// 一起核——表里每一行都得是路由表里真的那条 (方法, 路径)，路由表里每一条
// 非浏览器通路也都得有一行方法对得上。
func TestAgentsMDMethodColumn(t *testing.T) {
	raw, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatalf("AGENTS.md 读不到: %v", err)
	}
	s := New(Dependencies{})
	rows := docRows(string(raw))

	registered := map[[2]string]bool{}
	for _, rt := range s.routes() {
		registered[[2]string{rt.Method, agentsDocPath(rt.Pattern)}] = true
	}
	// 正向：表里每一行都必须是真路由。文档编出一条 POST /api/v1/data，
	// 就是给 AI 发了一张不存在的门牌。
	for _, r := range rows {
		if !registered[r] {
			t.Errorf("AGENTS.md 表里有 %s %s，路由表里没有这条（方法或路径不符）", r[0], r[1])
		}
	}
	// 反向：路由表里每条非浏览器通路，表里都得有同一 (方法, 路径) 的那一行。
	seen := map[[2]string]bool{}
	for _, r := range rows {
		seen[r] = true
	}
	for _, rt := range s.routes() {
		if rt.Pattern == "/" || rt.Pattern == "/pin" || rt.Pattern == "/healthz" {
			continue
		}
		if !seen[[2]string{rt.Method, agentsDocPath(rt.Pattern)}] {
			t.Errorf("路由表里有 %s %s，AGENTS.md 的表里没有带这个方法的行", rt.Method, rt.Pattern)
		}
	}
	// 解析器自己也要有证人：表格是这条闸唯一的输入，行格式一旦漂了（比如方法
	// 并进路径那一列），上面两个循环会同时看见空集、双双通过（P1-③ 点名的
	// "闸在空转"）。真实表有 55 行（2026-10-20 那次；加一条端点就变，所以这是快照），40 是余量。
	if len(rows) < 40 {
		t.Fatalf("只从 AGENTS.md 解析出 %d 行端点表，表格形状变了：这条闸正在空转", len(rows))
	}
}

// prefsEnumRE 抓 AGENTS.md 里「`theme` ∈ `auto`、`dark`、`light`」这种写法：
// 组 1 是字段名，组 2 是**限定词**（2026-10-13 加，见下），组 3 是档位清单
// （**一串反引号包着的词**，档位之间用顿号分隔）。
//
// **清单为什么按反引号认，而不是截到分号/逗号为止**（2026-10-19 改判）：从前
// 截到第一个分号或逗号为止，于是「`addrLevel` ∈ `host`、`full`、`none`（**卡片
// 第二行印什么**，2026-10-19 加）」那一段会把括号里那半句一起截进来，截到的就
// 不是清单了——而要覆盖 `view` 与 `addrLevel`，先得让这一段能被干净地读出来。
// 反引号包着的那一串**就是**清单：文档里每一处枚举都这么写，于是括号、破折号
// 与破折号后面那半句说明自然落在它外面，不需要为了机器好读去改文档的句子。
//
// **限定词为什么必须有**：台面面板新增了 `POST /prefs/group`，那里的 `layout`
// 收的是 `tiles`/`list`（一个分组内部的排法），而 `POST /prefs` 的 `layout`
// 收的是 `sidebar`/`topbar`（分组导航放哪）。**两个字段同名、值集不同**——
// 而这一条闸恰恰是要让文档的档位清单与 model 那张表同集，所以它不能把两者
// 看成同一段：认错的那一天，文档会拿一份枚举去校另一份，绿得毫无意义。
// 文档里写成「`layout`（组） ∈ …」与「`layout`（页） ∈ …」，各认各的表。
//
// **收进哪几个字段**（2026-10-19 补）：`view` 与 `addrLevel` 从前不在这一闸里，
// 于是它们那两段文档里的档位清单**没有人对着 model 校**——而它们恰恰是 2026-10-19
// 才加的两格（加进去的那一轮只补了服务端与界面，文档这一侧漏了）。`view` 与
// `layout` 的值集都只有一份（`view` 没有分组内部那一版），所以不需要限定词。
var prefsEnumRE = regexp.MustCompile("`(theme|layout|view|addrLevel|linkMode)`(?:（(组|页)）)? ∈ ((?:`[^`]+`、?)+)")

// 文档里的值集也必须与 model 那张表同集，两个方向都核：少一档是 AI 写不出
// 那一档（它不知道有），多一档是 AI 照着文档写出去吃 422。422 的拒绝语已经
// 从 model 生成了（见 prefs_test.go），AGENTS.md 是这组枚举的最后一份副本
// （P1-④ 评审 I-6）。
func TestAgentsMD枚举与model同集(t *testing.T) {
	raw, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatalf("AGENTS.md 读不到: %v", err)
	}
	text := string(raw)
	want := map[string][]string{
		"theme":     model.ThemeValues,
		"layout":    model.LayoutValues,
		"layout/组":  model.GroupLayoutValues,
		"view":      model.ViewValues,
		"addrLevel": model.AddrValues,
		"linkMode":  model.LinkModeValues,
	}
	matches := prefsEnumRE.FindAllStringSubmatch(text, -1)
	// 解析器不能空转：五段都必须出现（theme、页 layout、组 layout、view、addrLevel），
	// 否则下面的循环有一次不跑，这条闸就只是一句"没找到，跳过"。数的是**段**，不是
	// 字段名——两个 layout 段只按字段名去重会把它们合并成一段，那正是要防的那件事。
	if len(matches) != len(want) {
		t.Fatalf("从 AGENTS.md 只抓到 %d 段枚举清单，要 %d 段（theme、layout（页）、layout（组）、view、addrLevel）：写法变了，这条闸正在空转",
			len(matches), len(want))
	}
	for _, m := range matches {
		key := m[1]
		if m[2] != "" {
			key += "/" + m[2]
		}
		list, ok := want[key]
		if !ok {
			t.Errorf("文档里多出一段认不出的枚举清单：%s（组 %q）——要么它写错了字段名，要么 model 那边少了一张表", key, m[1])
			continue
		}
		var got []string
		for _, tok := range strings.Split(m[3], "、") {
			tok = strings.Trim(strings.TrimSpace(tok), "`")
			if tok != "" {
				got = append(got, tok)
			}
		}
		if strings.Join(got, ",") != strings.Join(list, ",") {
			t.Errorf("%s 的档位清单 = %v，model 的表里是 %v（顺序也要一致：文档是 AI 的词汇表）", key, got, list)
		}
	}
}
