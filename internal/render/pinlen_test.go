package render

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
)

// PIN 的长度范围**只有一份判据**：auth 那两个常量。
//
// 这个数要出现在两处——服务端判它（ValidPinFormat）、模板上那个
// minlength/maxlength 印它。两处各写一个字面量的话，改一处要记得改另一处，
// 而漏掉的那一处症状是"界面上限与实际判的对不上"（浏览器拦住了、curl 拦不住）。
//
// 这一族测试钉的就是那两半：
//   - 下面那条：模板里**不许出现写死的数字**（要写就写常量）；
//   - 再下面那条：每一页上每一个 PIN 输入框都印着那一对上下限。
//
// 从前印着的三处是引导建档、进门卡、管理页建档，而**管理页那个"给某一份设
// PIN"的确认页**与**台面面板那张「我的 PIN」**压根没有——那一格不设上下限，
// 于是浏览器不拦超长、只有服务端拦，界面上"打进去能过"与"这一格只收 4-6 位"
// 是两句矛盾的话。
var (
	// pinInputRe 认模板里那个装 PIN 的输入框。名字只有三个：进门与建档的 pin、
	// 改自己那道门的 old / new。
	pinInputRe = regexp.MustCompile(`<input[^>]*\bname="(pin|old|new)"[^>]*>`)
	// pinLenRe 取长度属性的**值**（引号里那一段）。
	pinLenRe = regexp.MustCompile(`(?:minlength|maxlength)="([^"]*)"`)
)

// 长度属性的值**不许是数字**：它必须是模板算出来的那一个。
//
// 这一条比"值对不对"更严一档：值对不对是**改范围之后**才红，而这一条**今天**
// 就红——它挡的是"有人又手打了一遍 4"。两者的分工是：这条管形状（不许写死），
// 下一条管结果（印出来的必须是常量那一份）。
func TestPIN的长度上限不写在模板里(t *testing.T) {
	entries, err := webFS.ReadDir("web")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".tmpl") {
			continue
		}
		raw, err := webFS.ReadFile("web/" + name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range pinInputRe.FindAllString(string(raw), -1) {
			for _, lv := range pinLenRe.FindAllStringSubmatch(m, -1) {
				if _, err := fmt.Sscanf(lv[1], "%d", new(int)); err == nil {
					t.Errorf("%s：装 PIN 的输入框把长度写死成 %q——它该取 auth 的那两个常量"+
						"（minlength=\"{{pinMin}}\" / \"{{$f.MinLen}}\"），"+
						"写死一份就会与服务端那份判据漂开", name, lv[1])
				}
			}
		}
	}
}

// 每一页上每一个 PIN 输入框都要印出那一对上下限，而且印的必须是**常量那一份**。
//
// 页面名单是从"哪儿能输 PIN"推出来的，不是手列的：进门卡、引导建档、管理页
// 建档、管理页给某一份设 PIN、台面面板改自己那道门。少列一个页面，那一格就
// 又变成"只有服务端拦"。
func TestPIN的每一格都印着同一份上下限(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	want := fmt.Sprintf(`minlength="%d" maxlength="%d"`, auth.PinMinLen, auth.PinMaxLen)

	// 引导建档那一屏（零档案）与进门卡那一屏（多档案，各带自己的 PIN 输入框）。
	setup, err := r.PickerPage(PickerOptions{Heading: "还没有档案", Setup: true})
	if err != nil {
		t.Fatal(err)
	}
	cards, err := r.PickerPage(PickerOptions{Heading: "谁来用？"})
	if err != nil {
		t.Fatal(err)
	}
	// 管理页那张建档表单。
	admin, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸"}}, "", AdminOptions{}, i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	// 管理页给某一份设 PIN 的确认页（那一格的范围由字段自己带着——见 adminaction.go）。
	action, err := r.AdminActionPage(AdminAction{
		Title: "给「爸爸」加一道 PIN", Heading: "给「爸爸」加一道 PIN",
		Explain: "说明", Action: "/admin/pin", Submit: "加上这个 PIN",
		Fields: []AdminField{{Name: "new", Label: "PIN", Type: "password",
			MinLen: auth.PinMinLen, MaxLen: auth.PinMaxLen}},
	}, i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}
	// 台面面板第四颗 tab「我的 PIN」——那一格有两格（现在 / 新的）。
	prefs, err := r.PrefsPage("dad", "pin", i18n.Zh)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		page  string
		html  string
		names []string
	}{
		{"引导建档", string(setup.Body), []string{"pin"}},
		{"进门卡", string(cards.Body), []string{"pin"}},
		{"管理页建档", string(admin.Body), []string{"pin"}},
		{"管理页设 PIN 确认页", string(action.Body), []string{"new"}},
		{"台面面板我的 PIN", string(prefs.Body), []string{"old", "new"}},
	} {
		for _, name := range tc.names {
			tag := pinInputTag(t, tc.page, tc.html, name)
			if !strings.Contains(tag, want) {
				t.Errorf("%s：装 PIN 的输入框没印 %s（实际那一段是 %s）——"+
					"界面上限与实际判的对不上，浏览器拦住了 curl 拦不住", tc.page, want, tag)
			}
		}
	}
}

// pinInputTag 取出页面里那个 name=… 的输入框那一整段标签。
func pinInputTag(t *testing.T, page, html, name string) string {
	t.Helper()
	re := regexp.MustCompile(`<input[^>]*\bname="` + regexp.QuoteMeta(name) + `"[^>]*>`)
	tag := re.FindString(html)
	if tag == "" {
		t.Fatalf("%s：页面里没有 name=%q 的输入框（这一格整个没了，PIN 输不进去）", page, name)
	}
	return tag
}
