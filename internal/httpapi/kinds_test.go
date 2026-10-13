package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/apierr"
)

// 这一族测试是 B 方案（2026-10-14）的钉子。它存在的理由只有一条：
// **状态码不够分**。422 既可能是"那一组已经不在了"（该说：刷新看看），
// 也可能是"这一条已经存过了"（该说：去改它，别再存第二遍）——两句出路相反，
// 而按状态码选句子只能说中一句。上一轮实测到的正是这个：重复添加同一网址的人
// 被告知"东西没了，刷新一下"。
//
// 三件事在这里钉住：
//  1. 每个错误出口都**真的**带 kind（源码扫，漏一处就红）；
//  2. 用到的 kind 都在登记表里（写错一个字母的常量不会编译错，但会被文档漏掉）；
//  3. 人真正会撞的那几类，**码分得对**（端到端诱发，不是比对字符串）。

// 一、源码扫：每个 errorResponse / fieldError 字面量都带 Kind。
//
// 判"有没有 Kind:"而不是"值对不对"——值由第二条管。扫描器按花括号配对走，
// 因为 []fieldError{{...},{...}} 的每一项都得单独算一处。
func Test每个错误出口都带kind(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, lit := range errorLiterals(string(src), "errorResponse", "fieldError") {
			checked++
			if !strings.Contains(lit, "Kind:") {
				t.Errorf("%s 里有一处字面量没带 kind：%s", f, oneLine(lit))
			}
		}
	}
	// 反空转守卫：扫到 0 处意味着扫描器本身坏了（改了类型名、换了构造方式），
	// 而"通过"看起来与真的通过一模一样。
	if checked < 60 {
		t.Fatalf("只扫到 %d 处错误字面量——这一族里 errorResponse/fieldError 有上百处，扫描器八成失灵了", checked)
	}
}

// 二、用到的 kind 必须在登记表里。
func Test用到的kind都在登记表里(t *testing.T) {
	re := regexp.MustCompile(`apierr\.([A-Z][A-Za-z]*)`)
	for _, f := range mustGlob(t, "*.go") {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			name := m[1]
			if name == "Kind" || name == "With" || name == "New" || name == "Info" || name == "Registry" || name == "Kinded" {
				continue // 函数与类型，不是常量
			}
			k, ok := constValue(t, name)
			if !ok {
				t.Errorf("%s 引用了 apierr.%s，但常量表里没有这个名字", f, name)
				continue
			}
			if _, ok := apierr.Registry[k]; !ok {
				t.Errorf("%s 引用了 apierr.%s = %q，登记表里没有：文档与闸都看不见它", f, name, k)
			}
		}
	}
}

// 三、端到端：人真会撞的那几类，码要分得开。
func Test同一类422按kind分得开(t *testing.T) {
	s, h := setupBatch(t)
	add := `{"profile":"dad","expectedRev":1,"by":"ai","ops":[
	  {"op":"add","path":"groups/nas/items","value":{"id":"dsm2","name":"再来一次","url":"http://10.0.0.3:5001"}}]}`
	if rec := post(h, "/api/v1/batch", add, "", dadTok); rec.Code != 200 {
		t.Fatalf("夹具准备失败：%d %s", rec.Code, rec.Body)
	}
	var first struct {
		Rev int64 `json:"rev"`
	}
	_ = json.Unmarshal(post(h, "/api/v1/batch", add, "", dadTok).Body.Bytes(), &first)

	cases := []struct {
		name  string
		path  string
		body  string
		tok   string
		want  int
		kind  string // 外壳的 kind
		ekind string // errors[0] 的 kind（空 = 这一发没有逐条明细）
	}{
		{"重复添加同一条", "/api/v1/batch",
			`{"profile":"dad","expectedRev":%d,"by":"web","ops":[
			  {"op":"add","path":"groups/nas/items","value":{"id":"dsm2","name":"再来一次","url":"http://10.0.0.3:5001"}}]}`,
			dadTok, 422, apierr.OpsInvalid, apierr.DuplicateID},
		{"那一组已经不在了", "/api/v1/batch",
			`{"profile":"dad","expectedRev":%d,"by":"web","ops":[{"op":"remove","path":"groups/nope"}]}`,
			dadTok, 422, apierr.OpsInvalid, apierr.MissingTarget},
		{"图标名查不到", "/api/v1/batch",
			`{"profile":"dad","expectedRev":%d,"by":"web","ops":[
			  {"op":"set","path":"groups/nas/items/dsm/icon","value":"mdi:does-not-exist"}]}`,
			dadTok, 422, apierr.OpsInvalid, apierr.IconUnknown},
		{"地址不完整（resolve）", "/api/v1/resolve", `{"url":"javascript:alert(1)","profile":"dad"}`,
			dadTok, 422, apierr.URLIncomplete, ""},
		// 同一条判据在**另一条通路**上也得成立：batch 走的是 model.Validate，那句原话
		// 外面还套着 `数据不合法: group/item` 两层。用 %v 套就截断了 kind，这一类掉回
		// field-invalid，界面上那句"这一批没收"就是第二次说谎（实测抓到的正是这一格）。
		{"地址不完整（batch）", "/api/v1/batch",
			`{"profile":"dad","expectedRev":%d,"by":"web","ops":[
			  {"op":"add","path":"groups/nas/items","value":{"id":"nohost","name":"缺主机名","url":"http://"}}]}`,
			dadTok, 422, apierr.OpsInvalid, apierr.URLIncomplete},
		{"rev 撞了", "/api/v1/batch",
			`{"profile":"dad","expectedRev":1,"by":"web","ops":[{"op":"remove","path":"groups/nas/items/dsm"}]}`,
			dadTok, 409, apierr.RevConflict, ""},
		// 空 body 会被**解码**先拦成 422（解码在鉴权之前），那一发测的是形状不是凭证；
		// 要量 401 就得发一条形状完全正确的请求。
		{"令牌不被认", "/api/v1/batch",
			`{"profile":"dad","expectedRev":%d,"by":"web","ops":[{"op":"remove","path":"groups/nas/items/dsm"}]}`,
			"nope-not-a-token", 401, apierr.CredentialUnknown, ""},
		{"作用域不含这一份", "/api/v1/data?profile=mom", "", kidTok, 403, apierr.CredentialScope, ""},
		{"这一份不在盘上", "/api/v1/data?profile=ghost", "", ghostTok, 404, apierr.ProfileNotFound, ""},
	}
	_ = s
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			if strings.Contains(body, "%d") {
				body = fmt.Sprintf(body, curRev(t, h, c.tok))
			}
			rec := post(h, c.path, body, "", c.tok)
			if c.body == "" { // 那两格是 GET 的读端点
				rec = getTok(h, c.path, "", c.tok)
			}
			if rec.Code != c.want {
				t.Fatalf("状态码 %d，要 %d：%s", rec.Code, c.want, rec.Body)
			}
			var out struct {
				Code   int    `json:"code"`
				Kind   string `json:"kind"`
				Errors []struct {
					Kind   string `json:"kind"`
					Field  string `json:"field"`
					Reason string `json:"reason"`
				} `json:"errors"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("外壳解不出来：%v %.200s", err, rec.Body)
			}
			if out.Kind != c.kind {
				t.Errorf("外壳 kind = %q，要 %q（屏幕上的句子按它选）", out.Kind, c.kind)
			}
			if c.ekind != "" {
				if len(out.Errors) == 0 {
					t.Fatalf("这一发该有逐条明细（errors[]），实际 %.200s", rec.Body)
				}
				if out.Errors[0].Kind != c.ekind {
					t.Errorf("errors[0].kind = %q，要 %q（原话：%q）", out.Errors[0].Kind, c.ekind, out.Errors[0].Reason)
				}
			}
		})
	}
}

// 外壳的键序也是契约：{code, kind, detail}，由 struct 字段序固定（§4.3）。
// 这条是 B 方案落地**之前**根本不存在的安全网——那天加的字段插到 detail 后面，
// 所有按前缀比对的脚本都会静默看到不同的东西。
func Test错误外壳的键序(t *testing.T) {
	_, h := setupBatch(t)
	rec := post(h, "/api/v1/batch",
		`{"profile":"dad","expectedRev":1,"by":"web","ops":[{"op":"remove","path":"groups/nope"}]}`, "", dadTok)
	body := rec.Body.String()
	iCode, iKind, iDetail := strings.Index(body, `"code"`), strings.Index(body, `"kind"`), strings.Index(body, `"detail"`)
	if !(0 <= iCode && iCode < iKind && iKind < iDetail) {
		t.Errorf("键序该是 code -> kind -> detail（struct 字段序就是输出键序，§4.3）：%s", body)
	}
	if strings.Contains(body, `"kind": ""`) {
		t.Error("kind 是空串：客户端会静默退回状态码那一档")
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

// curRev 读这一份档案当前的 rev：分类用例共用一份档案，写一次就进一格，
// 钉死数字会让后面几格撞 409（那测的是鉴权/冲突的顺序，不是分类）。
// 凭证错的那一格拿不到 rev，落 0 即可——它要的量的是 401。
func curRev(t *testing.T, h http.Handler, tok string) int64 {
	t.Helper()
	rec := getTok(h, "/api/v1/data?profile=dad", "", tok)
	var out struct {
		Rev int64 `json:"rev"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("读当前 rev 失败：%v %.200s", err, rec.Body)
	}
	return out.Rev
}

// errorLiterals 按花括号配对扫出 `name{...}` 的字面量正文（含内层）。
func errorLiterals(src string, names ...string) []string {
	var out []string
	for _, name := range names {
		for i := 0; i+len(name) < len(src); {
			j := strings.Index(src[i:], name+"{")
			if j < 0 {
				break
			}
			start := i + j + len(name) // 指向 '{'
			depth := 0
			for k := start; k < len(src); k++ {
				switch src[k] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						out = append(out, src[start:k+1])
						i = k + 1
						goto next
					}
				}
			}
			break
		next:
		}
	}
	return out
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func mustGlob(t *testing.T, pat string) []string {
	t.Helper()
	fs, err := filepath.Glob(pat)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

// constValue 把 apierr 里的常量名解析成它的字面值。不复述那张表：
// 复述一份就等于第二处可改，而这两处一定会分家。
func constValue(t *testing.T, name string) (string, bool) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "apierr", "apierr.go"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\b` + name + `\s+= "([^"]+)"`)
	m := re.FindStringSubmatch(string(src))
	if m == nil {
		return "", false
	}
	return m[1], true
}
