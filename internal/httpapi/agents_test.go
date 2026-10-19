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

// 静态资产的指纹路径在**三处**同时出现：路由表、openapi.json、AGENTS.md。
// 改了 app.js 之后指纹就变，而漏改任何一处的症状分别是：路由 404、契约闸报
// "文档里有、路由表里没有"、或者——最难发现的那一种——**文档与路由一起指向一个
// 已经不存在的旧路径**，而 CI 全绿（因为两边一致地错着）。
//
// 这一条闸把第三种形状变成当场红，并顺手报出该填的那个路径。**js 与 css 两条
// 都查**：它们是两个独立的内容哈希，改了样式忘了改文档，与改了脚本忘了改文档
// 是同一种错，而两者都在同一段文字里。
//
// ── 为什么不写泛指（`/app.<hash>.js`）────────────────────────────────────
// 泛指会让这道闸**彻底失效**，而且正好失效在它要防的那一种上：文档与路由
// **一致地**指向一个具体值才是最难发现的那种漂移，泛指把它变成默认状态。
// 泛指还让照抄的人必然 404——而脚本本来就该从页面 HTML 的 `<script src>` 读那个
// 路径，不该硬编码它。所以**这里记具体值**：它今天有用（能被照抄、能被校验），
// 代价是每改一次前端要同步两处——而那份重复由 LB_SYNC_FINGERPRINT 消掉，
// 不用拿契约的有效性去换省事。
//
// ── LB_SYNC_FINGERPRINT=1：让测试自己把两处改对 ────────────────────────
// 一次改前端本来要手改两个文档，而**两处都是机械替换**——而机械替换正是最该由
// 机器做、最容易被人做漏的那一类（漏一处，闸在下一轮才红，而那轮可能隔了几天）。
// 于是 `LB_SYNC_FINGERPRINT=1 go test ./...` 把两份文档同步成当前字节算出的
// 那一个，然后**继续跑同一条校验**——所以这条命令既是"改"也是"验"，不会变成
// 一个只会写不读的开关。
//
// **为什么用环境变量而不是 `-update` 这类 flag**：flag 是每个包各自注册的，
// `go test ./... -update.fingerprint` 会让**没定义它的那些包**因未知 flag 而失败。
// 环境变量没有这个问题，全量跑一遍即可，而全量跑正是这个动作该被执行的场合。
var syncFingerprint = os.Getenv("LB_SYNC_FINGERPRINT") != ""

func Test静态资产指纹三处一致(t *testing.T) {
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
		if !strings.Contains(string(doc), want) {
			t.Errorf("AGENTS.md 里没有 %s", want)
		}
	}

	// 文档里不许残留别的指纹——那是最阴的一种：两份都指向一个旧路径。
	for _, m := range regexp.MustCompile(`/app\.[0-9a-f]{12}\.(js|css)`).FindAllString(string(doc), -1) {
		if m != render.AppJSPath && m != render.AppCSSPath {
			t.Errorf("AGENTS.md 里还残留 %s——它已经不是任何一条静态资产的路径了", m)
		}
	}
}

// syncAssetFingerprints 把两份文档里的静态资产指纹换成当前字节算出的那一个。
//
// **按后缀分派**（`.js` → AppJSPath、`.css` → AppCSSPath）而不是按"当前值反推
// 旧值"：后者要求先知道旧的是什么，而"文档里残留着上一个指纹"正是这道闸要抓的
// 那种漂移——按位置抓，它一次能收干净（文档里可能同时有当前值与更旧的值）。
//
// 只在**两份文档**上动手，路由表不用改（它是算出来的）。
func syncAssetFingerprints(t *testing.T) {
	t.Helper()
	// 两条各自一条：字符类里的 12 是"恰好十二位"，而结尾的 (js|css) 是这一条只管
	// 自己那一种——**不能把 js 换成 css**，而一个不带分 alternation 的正则会。
	pairs := []struct {
		file string
		re   *regexp.Regexp
		want string
	}{
		{"../../AGENTS.md", regexp.MustCompile(`/app\.[0-9a-f]{12}\.js`), render.AppJSPath},
		{"../../AGENTS.md", regexp.MustCompile(`/app\.[0-9a-f]{12}\.css`), render.AppCSSPath},
		{"openapi.json", regexp.MustCompile(`/app\.[0-9a-f]{12}\.js`), render.AppJSPath},
		{"openapi.json", regexp.MustCompile(`/app\.[0-9a-f]{12}\.css`), render.AppCSSPath},
	}
	// 同一份文件可能被两条正则各改一次，所以先读完再一次性写回。
	byFile := map[string]*strings.Builder{}
	order := []string{}
	for _, p := range pairs {
		b, ok := byFile[p.file]
		if !ok {
			raw, err := os.ReadFile(p.file)
			if err != nil {
				t.Fatalf("读 %s: %v", p.file, err)
			}
			b = &strings.Builder{}
			b.Write(raw)
			byFile[p.file] = b
			order = append(order, p.file)
		}
		before := b.String()
		after := p.re.ReplaceAllString(before, p.want)
		if after == before {
			t.Logf("%s 里没有 %s 要替换的（已经是 %s）", p.file, p.re, p.want)
			continue
		}
		n := len(p.re.FindAllString(before, -1))
		t.Logf("%s：把 %d 处 %s 换成 %s", p.file, n, p.re, p.want)
		b.Reset()
		b.WriteString(after)
	}
	for _, file := range order {
		// 无 BOM 的 UTF-8：这两份文件本来就没有 BOM（.gitattributes 管着 eol），
		// 而加一个 BOM 会让 openapi.json 在某些解析器里多出一个键。
		if err := os.WriteFile(file, []byte(byFile[file].String()), 0o644); err != nil {
			t.Fatalf("写 %s: %v", file, err)
		}
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
		if !strings.Contains(text, rt.Pattern) {
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
		registered[[2]string{rt.Method, rt.Pattern}] = true
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
		if !seen[[2]string{rt.Method, rt.Pattern}] {
			t.Errorf("路由表里有 %s %s，AGENTS.md 的表里没有带这个方法的行", rt.Method, rt.Pattern)
		}
	}
	// 解析器自己也要有证人：表格是这条闸唯一的输入，行格式一旦漂了（比如方法
	// 并进路径那一列），上面两个循环会同时看见空集、双双通过（P1-③ 点名的
	// "闸在空转"）。真实表有 52 行，40 是余量。
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
var prefsEnumRE = regexp.MustCompile("`(theme|layout|view|addrLevel)`(?:（(组|页)）)? ∈ ((?:`[^`]+`、?)+)")

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
