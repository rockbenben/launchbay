package render

import (
	"bytes"
	"encoding/json"
	"os"
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

// 401 在人面前说的是**出路**，不是协议。
//
// 服务端那句 detail（"需要令牌或会话"）是给脚本对照的（AGENTS.md：`/api/v1/*` 的
// detail 固定中文）。app.js 要是把它原样贴进横幅，那个人读到的就是
// 「切换布局没存上：需要令牌或会话」——这一页上没有"令牌"这个概念，也没有任何地方
// 能让他带一个令牌。所以 api() 里那一处 401 → T("app.opsGone") 是承重的：
// 删掉它，界面上重新出现黑话，而**没有任何功能测试会红**（横幅照样弹、字照样显示）。
func TestAppJS_401说的是出路不是协议(t *testing.T) {
	src := readAppJS(t)
	if !strings.Contains(src, "if (res.status === 401) why = T('app.opsGone');") {
		t.Error("api() 里没有把 401 换成 app.opsGone：那个人会读到\"需要令牌或会话\"这种词")
	}
	if strings.Contains(src, "需要令牌或会话") {
		t.Error("app.js 里不该抄一份服务端的协议话——那句话的作者在内层（batch.go 的 detail），抄过来就是两处会漂")
	}
}
