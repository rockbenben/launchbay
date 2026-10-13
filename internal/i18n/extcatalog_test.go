package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 这一族测试管的是**浏览器扩展**那份语言包：`extension/_locales/<语言>/messages.json`。
//
// 为什么主站的闸管不到它：扩展跑在浏览器里，够不到 Go 的 embed，也够不到
// `internal/i18n` 那份 catalog——它必须有自己的一份，而"两份手写、没有闸"正是
// 主站 app.js 那份拷贝曾经踩过的坑（那边靠 appi18n_test.go 钉着）。这一族就是
// 扩展侧的同一颗钉子，钉的是四件事：两种语言的 key 一一对应、占位符逐位相同、
// 代码/页面/manifest 里引用的每个 key 都在册、在册的 key 每一个都有人引用。
//
// 为什么这些必须机械化而不是"下次改的时候注意"：漏翻的症状不是编译错，是页面上
// 一块空白或一个 ⟨key⟩；而**最容易被漏掉的恰好是英文那一半**——写的人自己看中文
// 那一份就够了，界面上他一辈子不会切到英文去检查。

// extLocalesDir 等路径都从包目录（internal/i18n）出发。扩展在主站 embed 之外，
// 没有任何 Go 代码 import 它，所以这里是全仓库**第一处**按路径读 extension/ 的
// 测试：它判的是"仓库里有没有这份东西"，与跑在哪台机器无关。
var (
	extDir       = filepath.Join("..", "..", "extension")
	extLocaleDir = filepath.Join(extDir, "_locales")
)

type extMessage struct {
	Message      string                    `json:"message"`
	Placeholders map[string]extPlaceholder `json:"placeholders"`
}

type extPlaceholder struct {
	Content string `json:"content"`
	Example string `json:"example"`
}

// loadExtCatalog 读一份语言目录。文件不存在与 JSON 读不动都是**测试失败**而不是
// 跳过：扩展的 default_locale 指不到目录时，Chrome 装不上这个扩展，那比一条红
// 测试严重得多，绝不能让它绿着进仓库。
func loadExtCatalog(t *testing.T, locale string) map[string]extMessage {
	t.Helper()
	p := filepath.Join(extLocaleDir, locale, "messages.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读不到扩展语言包 %s：%v", p, err)
	}
	var got map[string]extMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s 不是合法 JSON：%v（这个文件被 Chrome 在装载时解析，一个逗号就让整个扩展装不上）", p, err)
	}
	return got
}

func extLocales(t *testing.T) []string {
	t.Helper()
	ents, err := os.ReadDir(extLocaleDir)
	if err != nil {
		t.Fatalf("读不到 %s：%v", extLocaleDir, err)
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func keysOf(m map[string]extMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// extPlaceholders：消息里真正用到的 $1/$2/…。顺序不查，因为 Chrome 的替换按
// **编号**而不是按位置，编号写错了才会串位——而编号集合与声明集合一比就看出来。
var extDollar = regexp.MustCompile(`\$\d`)

func usedPlaceholders(msg string) []string {
	seen := map[string]bool{}
	for _, m := range extDollar.FindAllString(msg, -1) {
		seen[m] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func declaredPlaceholders(m extMessage) []string {
	out := make([]string, 0, len(m.Placeholders))
	for id, ph := range m.Placeholders {
		out = append(out, ph.Content+"←"+id)
	}
	sort.Strings(out)
	return out
}

// 一、两种语言的 key 守恒 + 每条都有内容。
func TestExtension_两种语言的key守恒(t *testing.T) {
	locales := extLocales(t)
	if len(locales) < 2 {
		t.Fatalf("extension/_locales 只有 %v：双语是这一份目录存在的理由", locales)
	}
	// default_locale 必须真的指到一个目录：指错了不是"退回中文"，是整个扩展装不上。
	manifest, err := os.ReadFile(filepath.Join(extDir, "manifest.json"))
	if err != nil {
		t.Fatalf("读不到 manifest：%v", err)
	}
	var mf struct {
		DefaultLocale string `json:"default_locale"`
	}
	if err := json.Unmarshal(manifest, &mf); err != nil {
		t.Fatalf("manifest 不是合法 JSON：%v", err)
	}
	if mf.DefaultLocale == "" {
		t.Fatal("manifest 没有 default_locale：name/description 的 __MSG_*__ 就没法解析")
	}
	var found bool
	for _, l := range locales {
		if l == mf.DefaultLocale {
			found = true
		}
	}
	if !found {
		t.Errorf("default_locale=%q 在 _locales 里没有这个目录（现有的：%v）", mf.DefaultLocale, locales)
	}

	base := loadExtCatalog(t, locales[0])
	for _, l := range locales[1:] {
		other := loadExtCatalog(t, l)
		var missA, missB []string
		for _, k := range keysOf(base) {
			if _, ok := other[k]; !ok {
				missA = append(missA, k)
			}
		}
		for _, k := range keysOf(other) {
			if _, ok := base[k]; !ok {
				missB = append(missB, k)
			}
		}
		if len(missA) > 0 {
			t.Errorf("%s 比 %s 少了 %d 条：%s", l, locales[0], len(missA), strings.Join(missA, "、"))
		}
		if len(missB) > 0 {
			t.Errorf("%s 比 %s 多了 %d 条：%s", l, locales[0], len(missB), strings.Join(missB, "、"))
		}
	}
	for _, l := range locales {
		cat := loadExtCatalog(t, l)
		for _, k := range keysOf(cat) {
			if strings.TrimSpace(cat[k].Message) == "" {
				t.Errorf("%s：%q 的 message 是空的——空串在页面上是一块空白，而空白看着像设计如此", l, k)
			}
		}
	}
}

// 二、占位符：同一个 key 在两种语言里用到的编号必须一模一样。
//
// 数量相同、编号错位也会说反话："读到 $1 条书签、$2 个分组" 译成
// "$2 bookmarks in $1 groups" 两种语言各自都读得通，界面上的数字却是反的。
func TestExtension_占位符逐位守恒(t *testing.T) {
	locales := extLocales(t)
	base := loadExtCatalog(t, locales[0])
	for _, k := range keysOf(base) {
		want := usedPlaceholders(base[k].Message)
		if got := declaredPlaceholders(base[k]); !sameStrings(got, expectDeclared(want)) {
			t.Errorf("%s：%s 的声明与消息对不上\n  消息用到 %v\n  声明的是 %v", locales[0], k, want, got)
		}
		for _, l := range locales[1:] {
			other := loadExtCatalog(t, l)
			if got := usedPlaceholders(other[k].Message); !sameStrings(got, want) {
				t.Errorf("%s：%q 的占位符与 %s 不一致\n  %s = %v\n  %s = %v", l, k, locales[0], locales[0], want, l, got)
			}
		}
	}
}

func expectDeclared(used []string) []string {
	out := make([]string, 0, len(used))
	for _, u := range used {
		out = append(out, u+"←"+strings.TrimPrefix(u, "$"))
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 三、四份代码 + options.html + manifest 里引用的每个 key 都在册。
//
// 引用是**按写法**扫出来的：`t('key'` 与 `data-i18n="key"`。这两种写法本身就是
// 这一层的规矩，扫描器认它们，认不出的第三种写法会在这里变成"引用了不存在的 key"
// 或"册上多一条没人用的"——两边都不静默。
func TestExtension_引用的key都在册(t *testing.T) {
	cats := map[string]map[string]extMessage{}
	for _, l := range extLocales(t) {
		cats[l] = loadExtCatalog(t, l)
	}
	refs, htmlRefs := extRefs(t)
	// 反空转守卫：扫描器一条都没扫到时，上面两条循环都会**安静地通过**，
	// 而那一天的真实形状是"整页界面话没人核对过"。改写法（`t("key")` 换成双引号、
	// 文件名换掉）必须先在这里红，而不是在那里绿。
	seen := map[string]bool{}
	for _, where := range refs {
		for _, w := range where {
			seen[w] = true
		}
	}
	for _, src := range []string{"bg.js", "options.js", "import.js", "i18n.js", "options.html", "manifest.json"} {
		if !seen[src] {
			t.Errorf("扫描器在 extension/%s 里一条引用都没找到：写法变了还是文件没了？这一族闸从此对它失明", src)
		}
	}
	missing := map[string][]string{}
	for key := range refs {
		for l := range cats {
			if _, ok := cats[l][key]; !ok {
				missing[key] = append(missing[key], l)
			}
		}
	}
	for _, key := range sortedStringKeys(missing) {
		ls := missing[key]
		sort.Strings(ls)
		t.Errorf("%v 里引用了 %q，但扩展目录 %v 没有这一条", refs[key], key, ls)
	}
	// 白放的 key 也要报：一条谁都不引用的文案会一直"看起来是双语的"，
	// 直到有人去改那句界面话时只改了一半。
	for _, l := range extLocales(t) {
		cat := cats[l]
		for _, k := range keysOf(cat) {
			if _, ok := refs[k]; !ok {
				t.Errorf("%s：%q 在册但没有任何地方引用（页面、代码、manifest 都没有）", l, k)
			}
			if _, ok := htmlRefs[k]; ok && !extAllowedTagKey(k) && strings.Contains(cat[k].Message, "<") {
				t.Errorf("%s：%q 被 data-i18n 用着却带标签——textContent 会把 <b> 原样印给人看", l, k)
			}
		}
	}
}

func sortedStringKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// 带标签的句子必须走 data-i18n-rich 那一族；判据就是页面上到底哪几个 key 用了它。
func extAllowedTagKey(key string) bool {
	for _, k := range richKeys() {
		if k == key {
			return true
		}
	}
	return false
}

var reRich = regexp.MustCompile(`data-i18n-rich="([^"]+)"`)

func richKeys() []string {
	raw, err := os.ReadFile(filepath.Join(extDir, "options.html"))
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range reRich.FindAllStringSubmatch(string(raw), -1) {
		out = append(out, m[1])
	}
	return out
}

var reT = regexp.MustCompile(`\bt\('([A-Za-z0-9_]+)'`)
var reMsg = regexp.MustCompile(`__MSG_([A-Za-z0-9_]+)__`)
var rePlain = regexp.MustCompile(`data-i18n="([^"]+)"`)

// extRefs 返回 {key: 出现处}（值只为了报错时说得清），以及只来自 HTML 的那一批。
func extRefs(t *testing.T) (map[string][]string, map[string]bool) {
	t.Helper()
	refs := map[string][]string{}
	fromHTML := map[string]bool{}
	jsFiles := []string{"bg.js", "options.js", "import.js", "i18n.js"}
	for _, f := range jsFiles {
		raw, err := os.ReadFile(filepath.Join(extDir, f))
		if err != nil {
			t.Fatalf("读不到 extension/%s：%v", f, err)
		}
		for _, m := range reT.FindAllStringSubmatch(string(raw), -1) {
			refs[m[1]] = append(refs[m[1]], f)
		}
	}
	for _, f := range []string{"options.html", "manifest.json"} {
		raw, err := os.ReadFile(filepath.Join(extDir, f))
		if err != nil {
			t.Fatalf("读不到 extension/%s：%v", f, err)
		}
		s := string(raw)
		for _, m := range rePlain.FindAllStringSubmatch(s, -1) {
			refs[m[1]] = append(refs[m[1]], f)
			fromHTML[m[1]] = true
		}
		for _, m := range reRich.FindAllStringSubmatch(s, -1) {
			refs[m[1]] = append(refs[m[1]], f)
			fromHTML[m[1]] = true
		}
		for _, m := range reMsg.FindAllStringSubmatch(s, -1) {
			refs[m[1]] = append(refs[m[1]], f)
		}
	}
	return refs, fromHTML
}

// 四、rich 那几条只许带 <b> 与 <code>。
//
// 这一条是 innerHTML 的护栏：填进页面的字符串全部来自仓库里的目录，没有一个字节
// 来自用户输入或那台机器，所以这里唯一的风险是**有人往目录里塞了一个 <script>**。
// 与其写一段"看起来安全"的说明，不如把标签集合钉死。
func TestExtension_rich句子只带白名单标签(t *testing.T) {
	keys := richKeys()
	if len(keys) == 0 {
		t.Fatal("options.html 里没有 data-i18n-rich：那一族是给带 <b>/<code> 的段落用的")
	}
	for _, l := range extLocales(t) {
		cat := loadExtCatalog(t, l)
		for _, k := range keys {
			msg := cat[k].Message
			if msg == "" {
				t.Errorf("%s：%q 被 data-i18n-rich 用着却不在册", l, k)
				continue
			}
			for _, tag := range extTags(msg) {
				switch tag {
				case "b", "code", "/b", "/code":
				default:
					t.Errorf("%s：%q 带了不在白名单里的标签 </%s>（data-i18n-rich 会用 innerHTML 填）", l, k, tag)
				}
			}
		}
	}
}

var reTag = regexp.MustCompile(`<(/?)([a-zA-Z0-9]+)`)

func extTags(s string) []string {
	var out []string
	for _, m := range reTag.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1]+m[2])
	}
	return out
}

// 五、可执行的那一层不许有中文。
//
// 这是"文案只有一份"在扩展侧的形状：句子住在目录里，代码只认 key。漏一句中文的
// 形状是——中文界面一切正常，英文界面里那一句**还是中文**，而改的人看不见它，
// 因为它不在任何一个列表里。
func TestExtension_代码与页面不许夹带中文(t *testing.T) {
	for _, f := range []string{"bg.js", "options.js", "import.js", "i18n.js"} {
		raw, err := os.ReadFile(filepath.Join(extDir, f))
		if err != nil {
			t.Fatalf("读不到 extension/%s：%v", f, err)
		}
		code := extCodeLines(string(raw))
		live, raw2 := 0, 0
		for _, line := range code {
			if strings.TrimSpace(line) != "" {
				live++
			}
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(line) != "" {
				raw2++
			}
		}
		// 反空转守卫：注释剥离要是把代码也吃了，"没有中文"只是因为什么都不剩了。
		// 这一条今天真的抓到了东西——`_locales/*/messages.json` 那句注释里的
		// `*/` 会把整段文件尾吞掉，包括后面所有真该被看的中文。
		if live*3 < raw2 {
			t.Errorf("extension/%s 剥掉注释后只剩 %d 行（原本非空 %d 行）——剥离器吃多了，这一条闸现在形同不设", f, live, raw2)
		}
		for i, line := range code {
			if hasHan(line) {
				t.Errorf("extension/%s:%d 的代码里有中文：%s（句子请写进 _locales/*/messages.json，代码只认 key）",
					f, i+1, strings.TrimSpace(line))
			}
		}
	}
	raw, err := os.ReadFile(filepath.Join(extDir, "options.html"))
	if err != nil {
		t.Fatalf("读不到 options.html：%v", err)
	}
	s := stripOrFatal(t, string(raw), "<!--", "-->")
	s = stripOrFatal(t, s, "/*", "*/")
	// 反空转守卫（同上）：剥完还在的标记里必须真的看得见页面的骨架，
	// 否则"没有中文"只是因为什么都不剩了。
	if !strings.Contains(s, "data-i18n") {
		t.Error("options.html 剥掉注释后连 data-i18n 都不见了——剥离器吃多了，这一条闸现在形同不设")
	}
	if i := indexHan(s); i >= 0 {
		t.Errorf("extension/options.html 的页面文字里有中文：%s（页面的每一格都由 data-i18n 从目录填，这里不留兜底句子）",
			strings.TrimSpace(snippet(s, i)))
	}
	mf, err := os.ReadFile(filepath.Join(extDir, "manifest.json"))
	if err != nil {
		t.Fatalf("读不到 manifest：%v", err)
	}
	if i := indexHan(string(mf)); i >= 0 {
		t.Errorf("extension/manifest.json 里有中文：%s（name/description 必须写 __MSG_key__）", snippet(string(mf), i))
	}
}

// extCodeLines 把 JS 里"会被执行的那一层"挑出来，行号与行数都不变。
//
// 逐行判，不跨行找 `/* */`：扩展这四份只用 `//` 写注释，而跨行的扫描会被注释里
// 一个合法的 `*/` 骗掉——`_locales/*/messages.json` 那一句就当场把文件尾吞了
// 半个多小时（症状是"中文检查通过"，其实是什么都没剩下）。`* ` 与 `/*` 开头的
// 行仍然按注释处理，将来谁写了块注释也不会误报。
func extCodeLines(src string) []string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case trim == "", strings.HasPrefix(trim, "//"), strings.HasPrefix(trim, "*"):
			out = append(out, "")
			continue
		}
		// 行尾注释：` // `（两边都有空格）才算。不留这一手，一条
		// `const x = 1; // 中文` 会被判成"代码里有中文"——那是注释，
		// 而误报的代价是有人把注释删掉来让闸变绿。
		if i := strings.Index(line, " // "); i >= 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	return out
}

// stripBlockComments 把 open..close 之间换成空白，**换行留着**（行号要对得上）。
// 找不到闭合就报错：静默"吃到文件尾"会把后面的代码连同其中的中文一起吞掉，
// 那时这一族闸报的是通过，而它什么也没看。
func stripBlockComments(src, open, close string) (string, error) {
	var sb strings.Builder
	for {
		a := strings.Index(src, open)
		if a < 0 {
			sb.WriteString(src)
			return sb.String(), nil
		}
		sb.WriteString(src[:a])
		rest := src[a+len(open):]
		b := strings.Index(rest, close)
		if b < 0 {
			return "", fmt.Errorf("%q 开了没关：从这一处往后整段都被当成注释了", open)
		}
		nl := strings.Count(rest[:b], "\n")
		sb.WriteString(strings.Repeat("\n", nl))
		src = rest[b+len(close):]
	}
}

func hasHan(s string) bool { return indexHan(s) >= 0 }

func stripOrFatal(t *testing.T, src, open, close string) string {
	t.Helper()
	out, err := stripBlockComments(src, open, close)
	if err != nil {
		t.Fatalf("extension/options.html：%v——注释剥不掉就没法分清哪里是中文哪里是注释", err)
	}
	return out
}

// indexHan 认汉字与中文标点/全角符号两段：「」（） 这类只在中文句子里出现，
// 而它们是漏写一句中文时最先露出来的痕迹。返回的是**字（rune）下标**——
// snippet 也按 rune 切，两边同一把尺子，否则报错指出的那一段根本不是中文那一段。
func indexHan(s string) int {
	for i, r := range []rune(s) {
		if r >= 0x3000 && r <= 0x303F || r >= 0x4e00 && r <= 0x9fff || r >= 0xff00 && r <= 0xffef {
			return i
		}
	}
	return -1
}

func snippet(s string, i int) string {
	r := []rune(s)
	lo := i - 40
	if lo < 0 {
		lo = 0
	}
	hi := i + 60
	if hi > len(r) {
		hi = len(r)
	}
	return string(r[lo:hi])
}

// 六、商店那两个长度上限：超了不是"显示得挤一点"，是提交被拒或名字被截。
func TestExtension_商店名称与描述在长度上限内(t *testing.T) {
	for _, l := range extLocales(t) {
		cat := loadExtCatalog(t, l)
		for key, limit := range map[string]int{"extName": 45, "extDescription": 132} {
			msg, ok := cat[key]
			if !ok {
				t.Errorf("%s：目录里没有 %q，manifest 的 __MSG_%s__ 解析不出来", l, key, key)
				continue
			}
			if n := len([]rune(msg.Message)); n > limit {
				t.Errorf("%s：%s 有 %d 个字，上限是 %d（Chrome 商店的硬限制）", l, key, n, limit)
			}
		}
	}
}
