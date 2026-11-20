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

const appJSPath = "web/app.js"

func readAppJS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(appJSPath)
	if err != nil {
		t.Fatalf("读 %s: %v", appJSPath, err)
	}
	return string(b)
}

// 每个版本只发送当前语言。检查实际发送的字节与词典一致，并覆盖脚本字节预算。
func TestAppI18N_与语言包逐条一致(t *testing.T) {
	for _, lang := range i18n.All {
		t.Run(string(lang), func(t *testing.T) {
			body := appScripts[lang].body
			line, _, ok := bytes.Cut(body, []byte(";\n"))
			if !ok || !bytes.HasPrefix(line, []byte("const MSGS = ")) {
				t.Fatal("本地化脚本缺少文案")
			}
			var got map[string]string
			if err := json.Unmarshal(bytes.TrimPrefix(line, []byte("const MSGS = ")), &got); err != nil {
				t.Fatal(err)
			}
			cat := i18n.Load(lang)
			count := 0
			for _, key := range cat.Keys() {
				if strings.HasPrefix(key, "app.") {
					count++
					if got[key] != cat.T(key) {
						t.Errorf("%s 缺失或与词典不一致", key)
					}
				}
			}
			if len(got) != count {
				t.Errorf("文案条数 = %d，要 %d", len(got), count)
			}
			if n := gzipSize(body); n > 125_000 {
				t.Errorf("本地化脚本 gzip %d B，超出操作线", n)
			}
		})
	}
}

func TestAppI18N_可执行代码里没有漏翻的中文(t *testing.T) {
	src := readAppJS(t)

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
	// 脚本中的每个文案键必须存在，所有语言的键集合由 i18n 的测试核对。
	cat := i18n.Load(i18n.Default)
	for _, match := range regexp.MustCompile(`['"](app\.[A-Za-z][A-Za-z0-9_.]*)['"]`).FindAllStringSubmatch(stripJSComments(src), -1) {
		key := match[1]
		if cat.T(key) == "⟨"+key+"⟩" {
			t.Errorf("脚本引用了未翻译的 %s", key)
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
	"pin-old-wrong":           "同上：改自己 PIN 那一族也是零 JS 表单（self/pin 的 403）",
	"profile-disabled":        "会话进不来这一份，浏览器这一侧不会收到",
	"profile-exists":          "建档只有 POST /api/v1/profiles、POST /api/v1/profiles/new 与三张表单（/setup、/new、管理页那张），这一份脚本都不走",
	"field-too-long":          "那是**档案名**的长度上限（2026-10-23 加），只有建档那几条通路会答它，而建档是零 JS 表单——这一份脚本不建档",
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
	"import-target-not-empty": "同上（这一码只在令牌/扩展那条 import 通路上产生，网页那三步走零 JS 表单）",
	"credential-admin-token":  "同上（这一码只在令牌通路上产生：填了管理员那把 key 去打档案作用域端点，网页那条路用的是会话 cookie）",
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
