package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// goCallKey 认 Go 源码里的**字面量**查表：`s.t("admin.delete.title")`。
//
// 那些 key 不出现在任何模板里——admin/delete/disable/import/revert/picker 的标题与
// 说明是在 httpapi 侧查好再传进视图的（带运行时值的句子只能这么做，见 Server.t）。
// 只扫模板会把它们全报成"孤儿"，那是这条测试第一版的毛病：自己制造了 50 条假警报。
var (
	goCallKey = regexp.MustCompile(`\.[tT]\("([a-zA-Z0-9_.]+)"`)
	goSrcGlob = []string{"../*/*.go", "./*.go"}
)

// 模板里查表用的是**字面量 key**（`{{.T "profile.empty"}}`）还是**变量**
// （`{{.T .Heading}}`，那个字段在 Go 侧已经翻好）——两种都要认，只校验前一种。
var (
	tmplLiteralKey = regexp.MustCompile(`\{\{\$?\.T\s+"([a-zA-Z0-9_.]+)"`)
	// 模板文件：只查 web/ 下的 .tmpl
	tmplFiles, _ = filepath.Glob("web/*.tmpl")
)

// 模板查了一个语言包里没有的 key，页面上就会印 `⟨那个 key⟩`——一眼能看出漏翻，
// 而空白（返回空串的那种写法）看着像设计。守恒测试守的是语言包**之间**的对应，
// 这一条守的是**模板与语言包之间**的对应：加了文案忘了插进模板，或删了 key 而模板
// 还在用，只有这一条抓得到。
func Test模板查的每个key都在语言包里(t *testing.T) {
	if len(tmplFiles) == 0 {
		t.Fatal("没找到任何模板文件，glob 写错了？")
	}
	var missing []string
	seen := map[string]bool{}

	for _, f := range tmplFiles {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读 %s: %v", f, err)
		}
		for _, m := range tmplLiteralKey.FindAllStringSubmatch(string(raw), -1) {
			key := m[1]
			seen[key] = true
			for _, lang := range i18n.All {
				// T 查不到时返回 ⟨key⟩ 而不是空串，这里要判的正是"查得到"。
				if got := i18n.Load(lang).T(key); strings.HasPrefix(got, "⟨") {
					missing = append(missing, key+"（"+string(lang)+"，"+filepath.Base(f)+"）")
				}
			}
		}
	}

	if len(missing) > 0 {
		t.Errorf("模板里查了 %d 个语言包没有的 key（页面上会印出尖括号）：\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(seen) == 0 {
		t.Error("一个字面量 key 都没扫到——那条正则是不是写坏了，" +
			"于是这一条从此什么都不检查")
	}
}

// 建档页上的语言下拉框是**装机时唯一的语言入口**：填完 PIN 的下一步就是按「创建」，
// 而它在 PIN 与按钮之间，不特意说一句基本会被跳过去。之后进管理页看见中文，
// 人会以为这软件不支持。
func Test建档页上语言那一格与它的提示都在(t *testing.T) {
	ix, fc, st := assets(t)
	r := New(st, ix, fc)
	page, err := r.PickerPage(PickerOptions{Setup: true})
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	for _, must := range []string{`name="language"`, `value="zh"`, `value="en"`,
		i18n.Load(i18n.Zh).T("lang.hintLater")} {
		if !strings.Contains(body, must) {
			t.Errorf("建档页少了 %q", must)
		}
	}
}

// 语言包里有、模板一处也没用上的 key：不是错（语言包可以先备着），但会让
// "翻全了没有"这句话说不清——它要么是刚加还没插进去，要么是插错了地方。
// 所以报告出来，不让它把 CI 弄红。
func Test语言包里有模板一处没用上的key(t *testing.T) {
	used := map[string]bool{}
	// 模板那一半
	for _, f := range tmplFiles {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读 %s: %v", f, err)
		}
		for _, m := range tmplLiteralKey.FindAllStringSubmatch(string(raw), -1) {
			used[m[1]] = true
		}
	}
	// Go 源码那一半：字面量 T("…") 调用。测试文件本身不算——它们钉的是行为不是入口。
	var srcs []string
	for _, g := range goSrcGlob {
		fs, _ := filepath.Glob(g)
		srcs = append(srcs, fs...)
	}
	for _, f := range srcs {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range goCallKey.FindAllStringSubmatch(string(raw), -1) {
			used[m[1]] = true
		}
	}

	// app.* 是**例外**：它们在 app.js 里（浏览器侧的第二份拷贝，由
	// appi18n_test.go 守恒），本来就查不到 .go 源码里，不算孤儿。
	//
	// 另有两处**已知的扫描盲区**，报出来的一定是误报，所以一并豁免（否则这条
	// 每次都喊十几条假警报，喊久了就没人看它了——一个持续误报的检查比没有更糟）：
	//   · notice.* 走 pinNotices 映射，map 的**值**是查表键而不是调用点字面量；
	//   · admin.disable.title.* 写在 map[bool]string{...}[disabled] 里，
	//     是键也不是调用。
	var orphans []string
	for _, k := range i18n.Load(i18n.Zh).Keys() {
		if !used[k] && !strings.HasPrefix(k, "app.") &&
			!strings.HasPrefix(k, "notice.") && !strings.HasPrefix(k, "admin.disable.title.") {
			orphans = append(orphans, k)
		}
	}
	if len(orphans) > 0 {
		t.Logf("语言包里有 %d 条模板与 Go 源码都没查到的 key（不失败，但值得看一眼是不是插漏了）：%s",
			len(orphans), strings.Join(orphans, "、"))
	}
	if len(used) == 0 {
		t.Error("一个字面量 key 都没扫到——那条正则是不是写坏了，于是这一条从此什么都不检查")
	}
}
