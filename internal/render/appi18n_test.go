package render

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// app.js 是**静态资产**：它在浏览器里跑，读不到服务端的 i18n catalog。
// 所以语言包里的 app.* 那一片在这里另有一份拷贝（APP_I18N 块）。
//
// 拷贝就是会漂的东西：改了语言包忘了同步、或直接手改了这一块，两边就对不上。
// 下面这两条是那个漂移的机械守卫——它们不检查"文案好不好"，只检查
// **两边是不是同一份**，以及**可执行代码里还有没有漏翻的硬编码**。

const appJSPath = "web/app.js"

var (
	appI18nBlock = regexp.MustCompile(`(?s)// BEGIN APP_I18N.*?// END APP_I18N\n`)
	// APP_I18N = {...};  取等号到最后一个分号之间的 JSON。
	appI18nJSON = regexp.MustCompile(`(?s)const APP_I18N = (\{.*?\});\n`)
)

func readAppJS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(appJSPath)
	if err != nil {
		t.Fatalf("读 %s: %v", appJSPath, err)
	}
	return string(b)
}

// 守恒①：app.js 里那一份，与语言包的 app.* 切片**逐条相等**（key 与值都要）。
func TestAppI18N_与语言包逐条一致(t *testing.T) {
	src := readAppJS(t)
	m := appI18nJSON.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("app.js 里找不到 APP_I18N 定义——那一块被删掉了？界面文案会全部变成 ⟨key⟩")
	}
	var embedded map[string]map[string]string
	if err := json.Unmarshal([]byte(m[1]), &embedded); err != nil {
		t.Fatalf("APP_I18N 不是合法 JSON: %v", err)
	}

	for _, lang := range i18n.All {
		want := map[string]string{}
		for k, v := range i18n.Load(lang).Keys() {
			_ = k
			_ = v
		}
		// Load 只给 key；值要从 catalog 里查。逐条比：
		cat := i18n.Load(lang)
		got := embedded[string(lang)]
		if got == nil {
			t.Errorf("APP_I18N 里没有 %s 那一半", lang)
			continue
		}
		// 语言包里 app.* 的条数与内嵌的一致？
		wantCount := 0
		for _, k := range cat.Keys() {
			if strings.HasPrefix(k, "app.") {
				wantCount++
				if cv := cat.T(k); got[k] != cv {
					t.Errorf("%s 的 %q 两边不一致：\n  语言包 %q\n  app.js  %q", lang, k, cv, got[k])
				}
			}
		}
		if wantCount != len(got) {
			t.Errorf("%s 的条数对不上：语言包 %d 条，app.js %d 条", lang, wantCount, len(got))
			_ = want
		}
	}
}

// 守恒②：**可执行代码里不许再有硬编码中文**。
//
// 这是"漏翻"唯一的机械检测。漏翻的形状很隐蔽：页面照常出，少一句提示、
// 英文界面里冒出一个中文词。上一批 54 处替换就是靠这条清完的。
//
// 豁免两种：
//   - APP_I18N 数据块本身（那是文案表，不是代码）；
//   - 含 ⟨ 的那一小片——T 的缺 key / 缺参数标记，**故意**要看得见。
func TestAppI18N_可执行代码里没有漏翻的中文(t *testing.T) {
	src := appI18nBlock.ReplaceAllString(readAppJS(t), "")

	var bad []string
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		code := stripJSLine(line)
		if !hasCJK(code) {
			continue
		}
		if strings.Contains(code, "⟨") {
			continue // T 的缺 key / 缺参数标记：故意要看得见
		}
		bad = append(bad, sprintfLine(i+1, trimmed))
	}
	if len(bad) > 0 {
		t.Errorf("app.js 的可执行代码里还有 %d 处硬编码中文（漏翻）：\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

func sprintfLine(n int, s string) string {
	return itoa(n) + ": " + s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// stripJSLine 去掉一行里的注释与字符串**内容**，留下结构。
//
// 为什么要剥字符串：JS 里 `'http://x'` 的 `//` 不是注释，字符串里的中文
// 也可能是有意的（标识符之类）。这里只保证"注释里的中文不被当成漏翻"——
// 而代码里出现中文**字符串**正是要抓的，所以字符串**保留**，只剥注释。
func stripJSLine(line string) string {
	var out bytes.Buffer
	inS, inD, inT := false, false, false // 单引号/双引号/模板
	inBlock := false
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		n := rune(0)
		if i+1 < len(rs) {
			n = rs[i+1]
		}
		if inBlock {
			if c == '*' && n == '/' {
				inBlock = false
				i++
			}
			continue
		}
		if !inS && !inD && !inT {
			if c == '/' && n == '/' {
				break // 行尾注释，剩下的都不要
			}
			if c == '/' && n == '*' {
				inBlock = true
				i++
				continue
			}
		}
		out.WriteRune(c)
		switch c {
		case '\'':
			if !inD && !inT {
				inS = !inS
			}
		case '"':
			if !inS && !inT {
				inD = !inD
			}
		case '`':
			if !inS && !inD {
				inT = !inT
			}
		}
	}
	return out.String()
}

// 失败横幅按**状态码**换句子，屏上不出现服务端那格协议话。
//
// 服务端那句 detail / errors[].reason 是给脚本对照的（AGENTS.md：`/api/v1/*` 的
// detail 固定中文）。app.js 把它原样贴进横幅，那个人读到的就是
// 「切换布局没存上：需要令牌或会话」——这一页上没有"令牌"这个概念，也没有任何地方
// 能让他带上一个。401 那一枪之后，其余状态码走的是同一条路：实测 422 的横幅
// 出来是「这一下没存上：没有分组 "nope"」，英文界面一句中文，末尾还挂着一个
// 人在界面上根本看不见的 id。
//
// 所以这一族现在**每一个状态码都得有话**：删掉 map 里的一档或整张 map，
// 界面上重新出现黑话，而没有任何功能测试会红（横幅照样弹、字照样显示）——
// 这条闸就是为那个"不会红"而存在。原文不进屏，但要进 console（排查的人与 AI
// 失败横幅按**错误码（kind）**换句子，屏上不出现服务端那格协议话。
//
// 这一族经历过一次改判：先前只按 HTTP 状态码选句子，于是"这一条已经存过了"与
// "那一组已经不在了"共用同一句（都是 422），而两句出路相反——实测重复添加同一个
// 网址的人，屏幕上写的是"东西没了，刷新一下"。现在服务端每个错误都带 kind
// （AGENTS.md「错误码」），这一侧按 kind 选，状态码只当兜底。
//
// 这条闸要防的是**静默少一档**：删掉 map 里的一行不会让任何功能测试变红
// （横幅照样弹、字照样显示），只是那一类错误重新变成黑话。所以判据是
// "服务端登记的每一个 kind 都必须在这里有个交代"：要么有句子，要么写清楚
// 为什么这一侧永远收不到它。
func TestAppJS_每种错误码都有交代(t *testing.T) {
	src := readAppJS(t)
	body := stripJSComments(src)

	kinds := serverKinds(t)
	if len(kinds) < 25 {
		t.Fatalf("从 apierr.go 只扫到 %d 个 kind——扫描器失灵与「真的就这么几个」长得一模一样", len(kinds))
	}
	mapped := map[string]bool{}
	for _, m := range regexp.MustCompile(`'([a-z][a-z-]+)': '(app\.[A-Za-z]+)'`).FindAllStringSubmatch(body, -1) {
		mapped[m[1]] = true
	}
	for _, k := range kinds {
		if mapped[k] || notReachable[k] != "" {
			continue
		}
		t.Errorf("kind %q 在 app.js 里既没句子也没写「为什么收不到」：那一发会退回状态码兜底，"+
			"而兜底句对这一类很可能又是一句假话", k)
	}
	for k := range notReachable {
		if !containsString(kinds, k) {
			t.Errorf("未达表里写着 %q，服务端已经没有这一码：这一格该删——留着它，下次真加了这个码反而没人管", k)
		}
	}

	// 状态码兜底那一档：每个都还得有话，且 422 不许再说"已经不在了"。
	// 落到兜底说明这一版根本不认识那个 kind，那句"刷新看看"就是第二次说谎。
	for _, want := range []string{
		"401: 'app.opsGone'", "403: 'app.opsWrongProfile'", "409: 'app.opsConflict'",
		"507: 'app.opsNoSnap'", "422: 'app.opsRejected'",
		"STATUS_KEY[status] || 'app.opsServerError'",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("选句子那两张表里少了 %q：那一档会退回服务端协议话，直接印在人看的横幅上", want)
		}
	}
	if strings.Contains(src, "422: 'app.opsStale'") {
		t.Error("422 的兜底又指回「已经不在了」：兜底走到的时候这一版不认识那个 kind")
	}
	// 明细优先于外壳：batch 的外壳 kind 说的是**这一族**（ops-invalid = 整批没写入），
	// 具体哪一条、为什么在 errors[0]。反过来的话整批 422 永远只会说那一句族名。
	if !strings.Contains(body, "first.kind") ||
		strings.Index(body, "first.kind") > strings.Index(body, "data.kind") {
		t.Error("msgKeyFor 该先读 errors[0].kind 再读外壳那一格，现在反了")
	}
	// 屏上那句话的唯一来源是 T(key)：服务端原文只许进 console。
	if strings.Contains(body, "let why = (first && (first.reason") {
		t.Error("api() 又把服务端的 reason 当成屏上那句话了：那些是给脚本读的，不是给人读的")
	}
	if !strings.Contains(src, "console.warn('lb: request rejected'") {
		t.Error("原文不进屏，但也不许丢：console.warn 那一行是排查的人与 AI 唯一能看到原话的地方")
	}
	// 两张表点名的每个 key 都要在两份语言镜像里各有一份——缺一个，屏上是尖括号。
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`'(app\.[A-Za-z]+)'`).FindAllStringSubmatch(body, -1) {
		seen[m[1]] = true
	}
	for key := range seen {
		if n := strings.Count(src, `"`+key+`"`); n < 2 {
			t.Errorf("%s 在 app.js 的镜像里只出现 %d 次，两份语言各要一份", key, n)
		}
	}
	if strings.Contains(src, "需要令牌或会话") {
		t.Error("app.js 里不该抄一份服务端的协议话——那句话的作者在内层，抄过来就是两处会漂")
	}
}

// notReachable 是"这一侧收不到这个码"的登记表：写理由，不是留空位。
// 理由要能被下一个人复核——"界面上没有那条通路"与"我懒得映射"得区分得开。
var notReachable = map[string]string{
	"pin-wrong":               "PIN 那一族是零 JS 表单，句子在 ?err= 白名单里（render 侧）",
	"profile-disabled":        "会话进不来这一份，浏览器这一侧不会收到",
	"profile-exists":          "建档只有 POST /api/v1/profiles 与管理页两张表单，这一份脚本都不走",
	"credential-admin-only":   "管理页那些表单是零 JS 通路",
	"body-unreadable":         "app.js 只发 JSON，读不出来是解码之前的事",
	"field-duplicate":         "那是表单（urlencoded）那一族的错",
	"field-unknown":           "请求体由这一份脚本组装，多出来的键不会有",
	"field-invalid":           "同上；真出现说明脚本自己写错了，那时该红的是测试",
	"query-invalid":           "浏览器这一侧不发查询参数不合法的请求",
	"route-not-found":         "路径由这一份脚本写死",
	"host-invalid":            "那是直接拿 IP 访问的人才会撞的",
	"import-no-bookmarks":     "导入那一屏是零 JS 页面",
	"import-nothing-accepted": "同上",
	"import-plan-stale":       "同上",
	"profile-not-rebuildable": "rebuild 只有管理员与 CLI 走",
	"ops-invalid":             "外壳的族名：有明细读明细，没明细按状态码兜底",
	"invalid-request":         "兜底码本身，没有更具体的一句话可给",
	"internal-error":          "500 走 app.opsServerError 那一档（带状态码）",
}

// stripJSComments 去掉 // 与 /* */ 注释，留下"会被执行的那一层"。
// 上面那些"表里有没有这一档"的判据必须扫代码而不是扫注释——否则注释里写一句
// `'duplicate-id': 'app.opsDuplicate'` 就能把这条闸喂绿。
func stripJSComments(src string) string {
	var out strings.Builder
	for _, line := range strings.Split(src, "\n") {
		out.WriteString(stripJSLine(line))
		out.WriteString("\n")
	}
	return out.String()
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// serverKinds 从服务端那份常量表扫出所有 kind：判据只有一份，
// 在这里再抄一份清单就等于埋下第二次漂移。
func serverKinds(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "apierr", "apierr.go"))
	if err != nil {
		t.Fatalf("读 apierr.go：%v", err)
	}
	re := regexp.MustCompile("(?m)^\\t([A-Z][A-Za-z]+)\\s*= \"([a-z0-9][a-z0-9-]*)\"")
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		out = append(out, m[2])
	}
	return out
}
