package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
)

// PIN 的长度范围**只有一份判据**：auth 那两个常量。
//
// 这个数要说给人听的地方有四类，而它们**不是同一类东西**：
//   - 服务端那句 422 理由（Go 里的字符串）——由常量算出来；
//   - 模板上那对 minlength/maxlength（渲染出来的那一半）——见 render 包的同名测试；
//   - 两份语言包里的文案（**静态 JSON**）；
//   - 机器契约 openapi.json（**静态 JSON**）。
//
// 后两类是静态文件：它们没法从 Go 那个常量算出来，于是**由测试钉**——
// 改了范围而忘了改文案，这两条会当场红，并指名是哪一句。
//
// 为什么值得这么麻烦：这个数一旦两处不同，症状是"页面上限 6、服务端只收 4"
// 与"服务端收 8、页面上写着 4-6"，而两样都不报错——人只会以为自己手滑了。

// pinRangeRe 认"4 到 6"这一族写法：中文的"到"、英文的"to"、契约里的"-"都在内。
//
// 数字取自常量，所以改了范围这条正则跟着改——**它认的是"当前该说的那个范围"**，
// 而不是一个写死的 4-6。
var pinRangeRe = regexp.MustCompile(fmt.Sprintf(`\b%d\b\s*(?:到|to|-|–|—)\s*\b%d\b`,
	auth.PinMinLen, auth.PinMaxLen))

// 服务端那句 422 理由：它是**给人看的那句**，数字必须与判据同源。
func TestPIN的格式理由说的是判据那个范围(t *testing.T) {
	_, h := setupBatch(t)
	cookie := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	rec := post(h, "/api/v1/profiles/self/pin", `{"old":"1234","new":"1"}`, cookie, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("新 PIN 格式错该 422，实际 %d：%s", rec.Code, rec.Body)
	}
	// 理由那句话按值比（§4.3：写出契约是 2 空格缩进，紧凑子串匹配不上自家格式）。
	var body struct {
		Errors []struct {
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("422 的体读不出来: %v", err)
	}
	var reason string
	for _, e := range body.Errors {
		if e.Field == "new" {
			reason = e.Reason
		}
	}
	if reason == "" {
		t.Fatalf("422 没有点名 new 那一条: %s", rec.Body)
	}
	if !pinRangeRe.MatchString(reason) {
		t.Errorf("格式理由说的是 %q，而判据收的是 %d 到 %d 位——这句是给人看的，它得说同一个范围",
			reason, auth.PinMinLen, auth.PinMaxLen)
	}
}

// 两份语言包里那几句提到 PIN 格式的话，说的都必须是**当前那个范围**。
//
// 名单是"哪几句说了格式"逐条列出来的：漏一条，那一句就会在改了范围之后
// 继续说旧的——而它是**给人看的**，人照着它敲就会敲错。
func TestPIN的范围在两份语言包里说的是同一个(t *testing.T) {
	for _, tc := range []struct {
		key  string
		args []any
	}{
		{"picker.setup.pinField", nil},
		{"picker.pin.hint", nil},
		{"notice.badpin", nil},
		{"admin.pin.explain.add", []any{"爸爸"}},
		{"admin.pin.field.add", nil},
		{"admin.pin.field.change", nil},
		{"prefs.pin.badNew", nil},
	} {
		for _, lang := range []i18n.Lang{i18n.Zh, i18n.En} {
			s := i18n.Load(lang).T(tc.key, tc.args...)
			if !pinRangeRe.MatchString(s) {
				t.Errorf("%s 的 %s 说的是 %q，没有说当前那个范围（%d 到 %d 位）",
					lang, tc.key, s, auth.PinMinLen, auth.PinMaxLen)
			}
		}
	}
}

// 机器契约里那七处提到 PIN 格式的地方，说的都必须是**当前那个范围**。
//
// 契约是 AI 照着写请求的那份（AGENTS.md：「文档骗 AI 比骗人更贵」），于是它说错
// 的代价是 AI 照着错的长度去写——于是它比界面文案更要紧。
//
// 七处是**逐条列出来的**：openapi.json 是静态产物、从不在运行时拼装，所以
// 这里是"该说的地方都得说"，而不是"文件里不许出现这个数"（那会连无关的
// "4-6 个字"一起禁掉）。
func TestPIN的范围在机器契约里说的是同一个(t *testing.T) {
	for _, spot := range [][]string{
		{"paths", "/api/v1/profiles", "post", "description"},
		{"paths", "/api/v1/profiles", "post", "requestBody", "content", "application/json", "schema", "properties", "pin", "description"},
		{"paths", "/api/v1/profiles/self/pin", "post", "requestBody", "content", "application/json", "schema", "properties", "new", "description"},
		{"paths", "/api/v1/profiles/pin", "post", "requestBody", "content", "application/json", "schema", "properties", "new", "description"},
		{"paths", "/api/v1/profiles/rebuild", "post", "requestBody", "content", "application/json", "schema", "properties", "pin", "description"},
		{"paths", "/admin/pin", "post", "requestBody", "content", "application/x-www-form-urlencoded", "schema", "properties", "new", "description"},
		{"paths", "/prefs/pin", "post", "requestBody", "content", "application/x-www-form-urlencoded", "schema", "properties", "new", "description"},
	} {
		s := openapiString(t, spot...)
		if !pinRangeRe.MatchString(s) {
			t.Errorf("契约的 %s 说的是 %q，没有说当前那个范围（%d 到 %d 位）",
				strings.Join(spot, "."), s, auth.PinMinLen, auth.PinMaxLen)
		}
	}
}

// openapiString 按路径取 openapi.json 里的一个字符串。
func openapiString(t *testing.T, path ...string) string {
	t.Helper()
	var doc any
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatalf("openapi.json 不是合法 JSON: %v", err)
	}
	cur := doc
	for i, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("openapi.json 的 %s 不是对象", strings.Join(path[:i], "."))
		}
		v, ok := m[p]
		if !ok {
			t.Fatalf("openapi.json 里没有 %s", strings.Join(path[:i+1], "."))
		}
		cur = v
	}
	s, ok := cur.(string)
	if !ok {
		t.Fatalf("openapi.json 的 %s 不是字符串", strings.Join(path, "."))
	}
	return s
}
