package render

import (
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/store"
)

func TestProfileStructureAndNoJS(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	body := string(mustRender(t, s, "dad"))
	for _, want := range []string{
		`<main`, `role="main"`, // 地标
		// 分组标题成结构；data-group 给添加浮层填分组下拉。
		// 2026-10-12：标题按「叶名 + 弱化的父路径」排版，这一份没有父路径，
		// 所以 sec-leaf 那一格是它自己；父链非空时的形状由
		// Test节标题_父路径弱化叶名突出 单独钉（同一份拆法，不在同一条里塞两种）。
		`<h2 id="g-nas" data-group="nas" title="NAS"><span class="sec-leaf">NAS</span></h2>`,
		`href="http://10.0.0.3:5000"`, // 关掉 JS 也能点
		`dsm 台`,                       // save() 造的条目标题是 itemID+" 台"
		`aria-hidden="true"`,          // 首字母色块是装饰
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("缺 %q\n%s", want, body)
		}
	}
	// ⑤ 之后首屏**可以有**一个脚本，但只能有那一个：一个 type=module 的**外部**
	// 脚本。判据收紧在这里的形状上而不是"有没有脚本"上——
	//   - 外部：`src` 不是内联，CSP 的 script-src 'self' 才放得过（§7.2）；
	//   - module：它不污染全局，而这一页的全部书签都活在服务端渲染的 DOM 里；
	//   - 只有它：第二个脚本意味着有两处逻辑能改这一页，而 §7.2 那句"关掉 JS
	//     仍可见可点"正是靠"只有一处能改"才守得住。
	//
	// 真正的"关掉 JS 能不能用"要真浏览器验（§13 的离线端到端回归），这里只钉形状。
	if n := strings.Count(body, "<script"); n != 1 {
		t.Fatalf("首屏应当恰好 1 处 <script，实际 %d 处：\n%s", n, body)
	}
	if !strings.Contains(body, `<script type="module" src="/app.`) {
		t.Errorf("那一个脚本应当是 type=module 的外部脚本，实际：\n%s", body)
	}
	if strings.Contains(body, "onerror=") || strings.Contains(body, "javascript:") {
		t.Fatalf("出现内联事件或伪协议:\n%s", body)
	}
}

// cssCommentRE 剥掉 /* */ 注释。必须先剥：`.pick button,.search button{…}` 那条规则
// 前面就压着一段说明注释，从 `}` 往回截选择器时它会被当成选择器文本的一部分混进来，
// 精确比对当场假红；同理，注释里只要引用了 `min-height:44px` 这个字面量，按字面量
// 数位点的证人就会假红。
// (?s) 不是可省的：本仓库的 CSS 注释都是跨行的，`.` 默认不吃换行，少了这个 flag 就等于没剥。
var cssCommentRE = regexp.MustCompile(`(?s)/\*.*?\*/`)

// declRule 是"某个属性在一条规则里的落点"：那条规则的选择器名单，与归一后的值。
type declRule struct {
	sels []string
	val  string
}

// normVal 把声明值里的一切空白压掉：`min-height: 44px`、跨行续写的 `min-height:\n44px`
// 与 `min-height:44px` 是同一个位点，只认最后一种的等于给前两种开门（复扫 N2：
// 只剥空格的归一看不见换行续写那一种）。
func normVal(s string) string { return strings.Join(strings.Fields(s), "") }

// rulesWithDeclaration 回答"属性 prop 出现在哪几条规则里、各给的是什么值"：先剥注释，
// 再从每个 `prop:` 往前找最近的 `{`、往前截到上一条规则的 `}` 或 `{`，值截到 `;`、`}`
// 或 `{`。这是给形状钉用的最小解析，不是 CSS 引擎——它只需要回答"这个值属于哪条规则"，
// 好过逐字钉整条规则（选择器顺序一重排就假红，而删掉名单里的一项反而可能假绿）。
//
// at-rule 的条件（`@media (min-height:640px){…}` 括号里那段）不是声明，整条跳过：
// CSS 里声明不可能活在括号条件里，而按名数位点的证人一旦被条件喂一条就假红
// （复扫 N1 探针实测——`@media (min-height:44px)` 会被当成挂在上一条规则选择器上的位点）。
// 判据是"最近那个 `{` 与本条 `prop:` 之间还开着一个没关掉的括号"。
func rulesWithDeclaration(css, prop string) []declRule {
	css = cssCommentRE.ReplaceAllString(css, "")
	var out []declRule
	for i := 0; i < len(css); {
		j := strings.Index(css[i:], prop+":")
		if j < 0 {
			break
		}
		pos := i + j
		vs := pos + len(prop) + 1
		ve := vs
		for ve < len(css) && css[ve] != ';' && css[ve] != '}' && css[ve] != '{' {
			ve++
		}
		i = ve
		open := strings.LastIndexByte(css[:pos], '{')
		if open < 0 {
			continue
		}
		if head := css[open:pos]; strings.LastIndexByte(head, '(') >= 0 &&
			!strings.ContainsRune(head[strings.LastIndexByte(head, '('):], ')') {
			continue
		}
		start := 0
		for k := open - 1; k >= 0; k-- {
			if css[k] == '}' || css[k] == '{' {
				start = k + 1
				break
			}
		}
		var sels []string
		for _, s := range strings.Split(css[start:open], ",") {
			if s = strings.TrimSpace(s); s != "" {
				sels = append(sels, s)
			}
		}
		out = append(out, declRule{sels: sels, val: normVal(css[vs:ve])})
	}
	return out
}

// selectorsWithDeclaration 在同一个解析器上回答"这条 `属性:值` 挂在哪些选择器上"。
// 命中按"值以 want 开头"算而不是全等：`content:"▸"` 后面接别的、或 44px 后面接
// !important，都还是同一个位点，全等会放走它们。
func selectorsWithDeclaration(css, decl string) []string {
	name, value, ok := strings.Cut(decl, ":")
	if !ok {
		return nil
	}
	want := normVal(value)
	var out []string
	for _, r := range rulesWithDeclaration(css, name) {
		if strings.HasPrefix(r.val, want) {
			out = append(out, r.sels...)
		}
	}
	return out
}

// 顶栏搜索表单的两个控件、顶栏的主题按钮（Task 3）、管理横幅与换档案的按钮
// （Task 4）、管理页的「进入编辑」按钮（Task 5）、侧栏的分组链接与抽屉把手
// （Task 6）都必须留在 §7.5 那几条 44px 规则的名单里。
// 搜索框是 input[type=search]（不在旧的 password/text 名单上），提交按钮不在 .pick 之下：
// 两处都容易被"重构 CSS"顺手挤出名单，而 TestCSS_硬规格在场 那条只钉"文件里有
// min-height:44px"，删掉名单里的一项它照样绿。这条钉的是"值属于哪条规则"。
//
// .theme button / .banner button / .switch button / .adm button 走的是同一判据的
// 另一半：它们必须**并进** .pick/.search 那条 joint 规则，而不是新开一条
// min-height:44px——同一个硬数字写两处，改一处就留下一处谎话。
//
// Task 6 的 .gnav a / Task 7 收宽后的把手一族 summary 不是按钮也不是输入框，
// 是"导航目标"这第三族：它们各开一条 joint 规则 `.gnav a,summary{min-height:44px}`，
// 本族里这个数字仍只写一遍（输入族、按钮族、导航族，三族各一次，见 base.css 的注释）。
//
// ④.5 Task 2 的引导表单**不开第四条**：它的两个输入框归输入族，提交按钮靠表单
// 同时带 `pick` 这个类归按钮族。新开一族就是"同一个硬数字写两处"，正是本函数要拦
// 的形状——所以除了逐条认名单，这里还按族数把话说死，并且不许 .setup 一族自己
// 写 min-height（那条会靠层叠盖掉族规则，比多写一遍 44 更难发现）。
func TestCSS_搜索控件的44px由哪条规则保证(t *testing.T) {
	// ⑤ 之后有**两份** CSS：base.css（首屏骨架，内联）与 app.css（交互样式，
	// 外部指纹文件）。这条闸要扫两份——只扫内联的那份，交互界面上的输入框就没人管了，
	// 而 §7.5 的 44px 说的正是"所有触摸目标"，不区分它长在哪个文件里。
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	app, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	rules := append(rulesWithDeclaration(string(raw), "min-height"),
		rulesWithDeclaration(string(app), "min-height")...)
	var joint []declRule
	for _, r := range rules {
		for _, s := range r.sels {
			// 两张建档表单（引导 .setup、管理页 .newprofile）都不许自己写 min-height：
			// 写了要么是多一个 44 的位点，要么靠层叠把族规则盖矮（复扫实测：只禁 .setup 时
			// .newprofile input{min-height:30px} 全绿，只有族数那半抓到东西）。
			for _, ban := range []string{".setup", ".newprofile"} {
				if strings.Contains(s, ban) {
					t.Errorf("%s 一族（%q）不许自己写 min-height:%s——44 由输入族与按钮族那两条 joint 规则接，"+
						"自己写要么是新第四位点，要么用层叠把族规则盖掉", ban, s, r.val)
				}
			}
		}
		if strings.HasPrefix(r.val, "44px") {
			joint = append(joint, r)
		}
	}
	// 规则条数 == 族数。**⑤ 之后是 5，2026-10-10 变 6**：app.css 里那三族是
	// 交互界面的输入面（.fld，收浮层、⌘K、就地编辑三处）、⌘K 的**结果行**
	// （.cmdk-list li，那是一排可点的行，§7.5 的「触摸目标 ≥ 44px」说的正是它），
	// 以及顶栏那枚**由 app.js 创建的**「＋ 添加」按钮（header .add）。
	// 前两族不合并进 .fld：.fld 带着 input 专属的一堆属性（width/padding/border），
	// 套到列表项上会把那一行撑变形——而一条形状坏掉的样式，比两条各自正确的规则糟得多。
	// 第三族也不并进按钮族：那条联合规则带着边框与 padding，会把顶栏那枚实心按钮
	// 的外观改掉；它单独成族是因为 app.css 只在档案页加载，base.css 不该为它
	// 背一份每个页面都要付的字节。
	// 多一条就是有人**又**开了一族（或把一族拆成两条）——上面那圈逐条断言对这种改法全绿，
	// 只有数条规则抓得住。
	if len(joint) != 6 {
		t.Errorf("含 min-height:44px 的规则有 %d 条，要六族各 1 条（输入/按钮/导航/交互输入面/面板结果行/顶栏添加按钮）", len(joint))
	}
	var sels []string
	for _, r := range joint {
		sels = append(sels, r.sels...)
	}
	// 名单按**今天真的存在的控件**列：`.banner button` / `.switch button` 随代管态
	// 一起删了（2026-10-10），换成导入/确认页那一族 `.act button` 与输入族的
	// `input[type=file]`——它们过去不在任何名单里，于是提交按钮与文件选择框
	// 都是 25px，浏览器实测才看出来。同一天补上的还有**所有可点的 `.badge`**：
	// 它们是链接，可 §7.5 的 44px 一样管它们，此前一律 20px 高。
	//
	// `.langsel button`（2026-10-12）：界面语言那颗按钮住在自己的表单里，
	// 同样一张族都没进——实测 24px，与它左边 44px 的下拉并排时像两个产品。
	// `.rename button`（同日第二批）：顶栏「改名」那一格的提交键，实测 21px 高，
	// 同一条毛病——住在自己的容器里就没进过任何族名单。
	for _, want := range []string{"input[type=search]", "input[type=file]", "select", ".search button", ".theme button", ".adm button", ".act button", ".langsel button", ".rename button", ".gnav a", "summary", "a.badge", "button.badge", ".fld", ".cmdk-list li", "header .add"} {
		var found bool
		for _, s := range sels {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s 必须被某条含 min-height:44px 的规则罩住；实际罩住的名单是 %v", want, sels)
		}
	}
	// 解析器自己也要有证人：名单里必须仍然包含 §7.5 那条原始名单的成员，
	// 否则就是这条最小解析什么都没解析出来（假绿的形状）。
	var sawPassword bool
	for _, s := range sels {
		if s == "input[type=password]" {
			sawPassword = true
		}
	}
	if !sawPassword {
		t.Errorf("解析器没在名单里认出 input[type=password]：%v——先修解析器，再谈上面那两条断言", sels)
	}
}

// 管理页的建档表单除了两格文本，还有一格 type=checkbox（`.newprofile .chk`）。
// 文本框那套 border/padding/background 若挂在**裸的** `.newprofile input`（或这张
// 表单带的任何一个类的 `.X input`）上，勾选框就会跟着吃到 1px 边框与 6/8px 内衬——
// 复扫 M-6 实测：把选择器放宽回 `.newprofile input`，既有断言全绿（44px 那条只看
// min-height，看不见边框）。这条钉的是"给输入框上框的选择器必须带 [type=…]"，
// 放宽的三种形状各验过它红（sab6 的 N5 `.newprofile input`、N7 `.adm input`——表单
// 自己也带 adm 这个类、N8 全局 `input{border:…}`）。
//
// 表单自己带哪些类是从**渲染出来的页面**取的（formClasses），不是从这里抄的字符串：
// 模板改类名时这里跟着动，而不是钉死一份和模板无关的名单。
func TestCSS_建档表单的勾选不吃文本框的框(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	page, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	classes := formClasses(t, string(page.Body), "/admin")

	// 正对照（解析器与这条纪律都在场）：两格文本各由一条带 [type=…] 的选择器罩着 border。
	var bordered []string
	for _, rule := range rulesWithDeclaration(css, "border") {
		bordered = append(bordered, rule.sels...)
	}
	for _, want := range []string{".newprofile input[type=text]", ".newprofile input[type=password]"} {
		var found bool
		for _, sel := range bordered {
			if sel == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s 该由一条声明 border 的规则罩住，实际声明 border 的选择器是 %v", want, bordered)
		}
	}

	for _, prop := range []string{"border", "padding", "background"} {
		for _, rule := range rulesWithDeclaration(css, prop) {
			for _, sel := range rule.sels {
				if reachesUncheckedInput(sel, classes) {
					t.Errorf("%q 用不带 [type=…] 的选择器给 %s:%s——建档表单里有 type=checkbox，"+
						"它会一起吃到文本框的框；要上框就点名 [type=text]/[type=password]", sel, prop, rule.val)
				}
			}
		}
	}
}

// reachesUncheckedInput 回答"这条选择器会不会把样式加到一张表单里**没被 [type=…] 点名**
// 的 input 上"——也就是勾选框会不会跟着吃到。
//
// 判法只到"够不够得着"这一层，不做真正的 CSS 匹配：末段点名的元素是 input（或通配 `*`）
// 且没写 [type=…] 就算候选；再往前看限定它的那些段——全是这张表单不带的类名（`.pick input`、
// `.search input`）就排除，出现元素名或通配（`form input`、`* input`）就保留，因为那种
// 限定挡不住勾选框。宽一点的代价是偶尔误伤一条本来无害的规则，窄一点的代价是 M-6 那种
// 放宽全绿——后者是本条要防的。
func reachesUncheckedInput(sel string, classes map[string]bool) bool {
	segs := strings.FieldsFunc(sel, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '>' || r == '+' || r == '~'
	})
	if len(segs) == 0 {
		return false
	}
	last := segs[len(segs)-1]
	if strings.Contains(last, "[type=") {
		return false
	}
	if i := strings.IndexAny(last, ".#:[]"); i >= 0 {
		last = last[:i]
	}
	// 元素名必须是 input（或通配）。只写类/伪类的复合（`.chk`）不算：它的隐式元素是
	// `*`，理论上够得着勾选框，但 `.newprofile .chk{padding:…}` 这种**给 label 凑
	// 点击面**的规则会因此天天误伤——这里要防的是"把 input 一族一起上框"，不是通配本身。
	if last != "input" && last != "*" {
		return false
	}
	for _, prev := range segs[:len(segs)-1] {
		if prev[0] != '.' && prev[0] != '#' {
			return true // 元素名或通配限定不住勾选框
		}
		if classes[prev[1:]] {
			return true
		}
	}
	return len(segs) == 1
}

func TestLetterFallbackAndDescAndAlias(t *testing.T) {
	s := seeded(t)
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "x", Title: "x", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Groups = []model.Group{{ID: "g", Title: "组", Layout: model.LayoutList, Items: []model.Item{
		{ID: "emby", Name: "Emby 影音", URL: "http://10.0.0.3:8098", Desc: "影视", Alias: []string{"em"}, Tags: []string{}},
		{ID: "qinglong", Name: "青龙面板", URL: "http://10.0.0.3:5700", Alias: []string{}, Tags: []string{"定时"}},
	}}}
	if err := s.SaveBookmarks("x", b); err != nil {
		t.Fatal(err)
	}
	body := string(mustRender(t, s, "x"))
	for _, want := range []string{">E<", ">青<", "影视", "定时"} {
		if !strings.Contains(body, want) {
			t.Fatalf("缺 %q:\n%s", want, body)
		}
	}
}

// chrome:// 之类的条目：浏览器禁止从网页跳转，要标出来并给复制出路。
//
// 评审 Finding 2 之后走完整通路：CreateProfile → SaveBookmarks →
// ProfilePage。原先那版夹具直接喂 executor 绕开 store，是因为旧的
// model.checkURL 有 http/https 白名单、这类 URL 根本存不进来（spec
// §6.2「允许存但标警告」与 §9 的 flare 特例被它挡死）。白名单已取消，
// 绕过 store 的夹具随之删掉——现在这条断言的是"存进去还能原样渲染出来"。
func TestNonHTTPSchemeMarked(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Groups = []model.Group{{ID: "g", Title: "组", Layout: model.LayoutList, Items: []model.Item{
		{ID: "exts", Name: "扩展管理", URL: "chrome://extensions/", Alias: []string{}, Tags: []string{}},
		{ID: "apps", Name: "应用页", URL: "chrome://apps", Alias: []string{}, Tags: []string{}},
		{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
	}}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatalf("非 http 协议要存得进（spec §6.2 允许存但标警告）: %v", err)
	}
	body := string(mustRender(t, s, "dad"))
	for _, want := range []string{"需手动打开", "chrome://extensions/", "chrome://apps", `href="http://10.0.0.3:5000"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("缺 %q（URL 要原样保留并标警告，http 条目照旧可点）:\n%s", want, body)
		}
	}
	// 不许把它做成一个会静默失败的可点链接。
	if strings.Contains(body, `href="chrome://`) {
		t.Fatalf("不许给非 http 协议渲染 href:\n%s", body)
	}
	// 复制键由 app.js **就地把这枚徽标换成**（§8.1：点了没反应的东西不进界面），
	// 于是模板这一侧只钉钩子在：它被删掉时 app.js 那一段静默失效、这一格退回成一句
	// 提醒，而上面那条断言照样全绿。
	if !strings.Contains(body, `class="badge warn ext"`) {
		t.Fatalf("外部地址那枚徽标要带 ext 钩子类（app.js 靠它换成复制键）:\n%s", body)
	}
}

// 简报缺陷（已修）：断言原文 "onerror=alert" 在任何转义结果里都原样
// 存在（这些字符本就不是 HTML 特殊字符），正确转义也会误判红。
// 真正的注入载体是未转义的 "<img src=x onerror"，断言收紧到它——
// 换成 text/template 时照样变红。
func TestEscapesUserData(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Title = "<img src=x onerror=alert(1)>"
	b.Groups = []model.Group{{ID: "g", Title: `<script>bad()</script>`, Layout: model.LayoutList,
		Items: []model.Item{{ID: "i", Name: `a"b`, URL: "http://10.0.0.3/", Desc: "<b>粗</b>", Alias: []string{}, Tags: []string{}}}}}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	body := string(mustRender(t, s, "dad"))
	if strings.Contains(body, "<script>bad()</script>") || strings.Contains(body, "<img src=x onerror") {
		t.Fatalf("用户数据没转义:\n%s", body)
	}
	if !strings.Contains(body, "&lt;") {
		t.Fatal("要看到转义后的实体")
	}
}

// 档案名与内容都可控，模板不许把 id 拼进可执行上下文。
//
// 简报缺陷（已修）：seeded 夹具建的档案都不带 Admin，断言永远绿不了，
// 这里直接建两份形状一样的档案（**管理员不再是档案的属性**，见下）。
//
// 最终整枝评审（死链收口）：/admin 路由当时还不存在，首屏发的锚点点了就是
// 404 JSON，所以那条钉禁止 href="/admin"。Task 5 落地了 GET /admin，这条
// **翻面但不删除**：管理员现在必须能走到管理页（href="/admin" 要在），
// 非管理员仍然不给入口（看见了也是 303，那是点了没反应的按钮）。
// href="/switch" 那条**保留**：/switch 只收 POST 表单，GET 链接打过去是 405，
// 跟当初打 404 一样是坏出路。换档案的出路由同一条通路上的
// TestProfilePage_顶栏有换档案的表单出路 钉（那是 form，不是 href）。
//
// 2026-10-12：判据从「这个档案是不是管理员」换成 `PageRequest.HasAdmin`
// （本次请求带的是不是一把真的 lb_admin）。两条档案现在**形状完全一样**，
// 区别只在 PageRequest 那一格——这正是新契约：管理员可以一边用自己的档案
// 看书签、一边仍带着管理权，两者不再是同一个身份。
func TestAdminEntryOnlyForAdmin(t *testing.T) {
	s := store.New(t.TempDir())
	// 不带 Admin（2026-10-12）：`NewProfileInput.Admin` 与 `model.Profile.Admin` 都删了。
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "dad", Title: "带管理权的那台", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "mom", Title: "没管理权的那台", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	save(t, s, "dad", "爸爸的台", "dsm")
	save(t, s, "mom", "妈妈的台", "emby")
	r := renderer(t, s)
	dadPage, _ := r.ProfilePage(PageRequest{ID: "dad", HasAdmin: true})
	momPage, _ := r.ProfilePage(PageRequest{ID: "mom"})
	if !strings.Contains(string(dadPage.Body), ">管理<") {
		t.Fatal("带着管理权的那次请求要能看到管理标记")
	}
	if strings.Contains(string(momPage.Body), ">管理<") {
		t.Fatal("没带管理权就不该有管理标记")
	}
	// 同一份档案、同一批书签，HasAdmin 一翻就该翻面——钉的是"判据在这一格"，
	// 而不只是"某个档案恰好有那个标记"。
	dadNoAdmin, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dadNoAdmin.Body), ">管理<") {
		t.Fatal("同一份档案不带管理权时也不该有管理标记：判据是这次请求，不是档案的属性")
	}
	if !strings.Contains(string(dadPage.Body), `href="/admin"`) {
		t.Error("管理员要能在页面上走到管理页——路由已经存在")
	}
	if strings.Contains(string(momPage.Body), `href="/admin"`) {
		t.Error("非管理员不该看见管理入口（看见了也是 303，但那是一颗点了没反应的按钮）")
	}
	for _, body := range []string{string(dadPage.Body), string(momPage.Body)} {
		if strings.Contains(body, `href="/switch"`) {
			t.Fatal("/switch 只收 POST 表单，不许发 GET 链接")
		}
	}
}

// 关掉 JS 也要能进门：选择页每个档案一张自包含的 POST 表单——
// hidden profile 把"选谁"答完，PIN 输入与提交按钮在同一张表单里，
// 点卡片就是提交，不需要任何脚本。
func TestPickerNoJSSubmitPathPerProfile(t *testing.T) {
	s := seeded(t)
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	p, err := renderer(t, s).PickerPage(PickerOptions{KeepAdd: "http://10.0.0.3/new"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(p.Body)
	if got := strings.Count(body, `<form method="post" action="/pin"`); got != len(profiles) {
		t.Fatalf("每个档案要一张表单，got %d want %d:\n%s", got, len(profiles), body)
	}
	for _, prof := range profiles {
		if !strings.Contains(body, `<input type="hidden" name="profile" value="`+prof.ID+`">`) {
			t.Fatalf("档案 %q 的表单没有 hidden profile 字段:\n%s", prof.ID, body)
		}
	}
	if got := strings.Count(body, `name="pin"`); got != len(profiles) {
		t.Fatalf("每张表单都要有自己的 PIN 输入，got %d want %d:\n%s", got, len(profiles), body)
	}
	// ?add= 在每张卡片上都得带上，不然换一张卡就丢。
	if got := strings.Count(body, `name="keep_add"`); got != len(profiles) {
		t.Fatalf("keep_add 份数 = %d, want %d:\n%s", got, len(profiles), body)
	}
	if strings.Contains(body, "<script") {
		t.Fatalf("进门这条路不许依赖 JS:\n%s", body)
	}
}

// 提示只在给的时候出现，而且是一个能被读屏立刻念出来的 alert 段落；
// 没有提示时页面上不该多出空壳元素。
func TestPickerNoticeRendersOnlyWhenGiven(t *testing.T) {
	s := seeded(t)
	with, err := renderer(t, s).PickerPage(PickerOptions{Notice: "PIN 不对，再试一次。"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(with.Body)
	if !strings.Contains(body, `role="alert"`) || !strings.Contains(body, "PIN 不对，再试一次。") {
		t.Fatalf("提示没进页面:\n%s", body)
	}
	// 提示要出现在表单之前，不然读屏的人先听完一屏卡片才知道错了。
	if strings.Index(body, `role="alert"`) > strings.Index(body, `<form method="post"`) {
		t.Fatalf("提示应在表单之前")
	}
	plain, err := renderer(t, s).PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain.Body), `role="alert"`) {
		t.Fatalf("没有提示时不该渲染提示元素:\n%s", plain.Body)
	}
}

// 截断的地方必须还能看到全名——悬停。
//
// 这条规矩**侧栏早就立了**（grouplist 的 `title="{{.Title}}"`：190px 里长分组名会被
// 省略号截断，"截断之后必须还有地方能看到完整名字，否则截断就变成信息丢了"）。
// 卡片上的名字与顶栏的档案名当时没跟上：`.card b` 与 `header h1` 都带
// `overflow:hidden;text-overflow:ellipsis`，却谁都没挂 title——于是「群晖 D…」
// 这三个字在页面上就是终点，人得回去翻源文件才知道后面是什么。
//
// 字节是有的（约每条 6 B gzip，1000 条 ≈ +6 KB，仍在 120 KB 契约内），而
// 丢信息是没有的。
func TestProfilePage_会截断的名字都挂着title(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "一个特别长的档案名字用来逼出省略号而不是让它安静地截掉", "dsm")
	body := string(mustRender(t, s, "dad"))
	// 条目名
	if !strings.Contains(body, `<b title="`+`dsm 台`+`">dsm 台</b>`) {
		t.Errorf("条目名没挂 title：截断后无从看全\n%.300s", body)
	}
	// 档案名（顶栏 h1）
	if !strings.Contains(body, `<h1 title="`) {
		t.Error("顶栏档案名没挂 title")
	}
	// 侧栏那条是既有的，一并钉住，免得哪天把「规矩的两边」拆成一边
	if !strings.Contains(body, `<a href="#g-nas" title="NAS">`) {
		t.Error("侧栏分组名没挂 title")
	}
}

// app.css（⑤/⑥ 的交互层）里那几条**承重规则**。
//
// 为什么单开一份：base.css 有 TestCSS_硬规格在场 逐字钉着，而 app.css 只有// 指纹三处一致——那一条钉的是**路径**（改了内容要同步文档），不是**内容**。
// 于是这个季度里 app.css 出了两次没人拦的事：
//   - 就地把手的静息透明度 .5，13 张卡上 13 个各自悬浮的 ✎；
//   - 编辑态写 `inset:0`，被正好钉成卡片那个盒子（磁贴实测 287×109，而表单要
//     ~250px），背景只画盒子那一块，溢出的三个输入框透明地压在相邻卡片上。
//
// 这两条都不是"难看"，是**用不了**——所以钉住。
func TestCSS_交互层的承重规则在场(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	for name, want := range map[string]string{
		"把手触摸目标 44px": `.pencil{flex:0 0 auto;width:44px;height:44px`,
		// 多选那条贴在**视口下沿**，而联合规则 `button:active{transform:translateY(1px)}`
		// 把它往下推 1px——那一族的按下位移要明确豁免，否则按任意一颗都让整条抖一下。
		"多选条按下不位移": `.multibar button:active{transform:none`,
		// **不许写回 inset:0**：那会把编辑态的高度钉成卡片的高度，而表单装不下。
		"编辑态按内容定高": `.inline-edit{position:absolute;top:0;left:0`,
		// 磁贴只有 190px 宽，没有下限就放不下一条完整 URL；列表行接近整幅宽，
		// 没有上限则编辑一块就铺满半屏。
		"编辑态宽度有上下限": `width:min(max(100%,300px),420px)`,
		// 分组标题右端那枚布局切换键：**44×44 是 §7.5 的触摸目标硬线**，
		// 而它看上去只有 11px 字形（盒子守住 44，与 .pencil 同一处置）。
		"布局切换键触摸目标": `.layoutbtn{flex:0 0 auto;width:44px;height:44px`,
		// 这一条是**实测**逼出来的：选择器前多打了一个引号（`'.inline-edit .row{`），
		// 浏览器把整条规则当无效选择器丢掉，`.row` 退回块级——「存」与「取消」各占
		// 一整行 278px 宽（实测那一行 96px 高，横排后 48px），「删掉这条」再占一行。
		// CSS 文件读得进来、构建没错、每一颗按钮都还在、只是排成了两行三行——
		// 所以除了逐字钉，下面另有一道"选择器段里没有游离引号"的形状闸。
		"编辑态底部那一行是横排": `.inline-edit .row{display:flex;gap:8px}`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css 里没有 %q（%s）", want, name)
		}
	}
}

// 选择器段里不该出现**游离**引号：引号只允许待在属性选择器的方括号里
// （`td[data-k="条目"]` 是合法写法，本案 base.css 的管理页表格就靠它）。
// 出现在括号之外就意味着这条规则整条被浏览器丢掉。本案踩过的形状是行首一个
// 游离的 `'`，它让 `.inline-edit .row{display:flex}` 静默消失。
//
// 这道闸自己也要"解析器没吞形状"的证人：正向对照必须证明它真看见了要保护的那条
// 选择器，否则"一条都没扫到"也会绿。名单按文件给——两个文件各守各的那一条。
func TestCSS_选择器段里没有游离引号(t *testing.T) {
	for _, tc := range []struct{ file, guarded string }{
		{"web/base.css", ".rename label"},
		{"web/app.css", ".inline-edit .row"},
	} {
		raw, err := webFS.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		sels := cssSelectorChunks(stripCSSComments(string(raw)))
		if len(sels) < 40 {
			t.Fatalf("%s 只切出 %d 段选择器：切法坏了，这道闸此刻是空转的", tc.file, len(sels))
		}
		var sawGuarded bool
		for _, s := range sels {
			if strings.ContainsAny(s, "'\"") {
				t.Errorf("%s 的选择器 %q 里混进了方括号外的引号——浏览器会把这一整条规则丢掉", tc.file, s)
			}
			if strings.Contains(s, tc.guarded) {
				sawGuarded = true
			}
		}
		if !sawGuarded {
			t.Errorf("%s：切出来的 %d 段里没有 %s，这道闸没盯住它要盯的东西", tc.file, len(sels), tc.guarded)
		}
	}
}

// 声明里的括号必须配平。本案实测踩过一条：
// `@media (max-width:700px){ul{grid-template-columns:repeat(2,minmax(0,1fr)}}`
// ——`repeat(` 少一个右括号，而 `}` 不是右括号。Chrome 靠错误恢复把它救回来了
// （650px 那一档确实两列），但按 CSS 语法这就是**一条无效声明**，
// 换引擎不保证有同样的运气。配平判据在每一个 `}` 处检查：括号深度必须为 0。
func TestCSS_声明里的括号配平(t *testing.T) {
	for _, f := range []string{"web/base.css", "web/app.css"} {
		raw, err := webFS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		css := stripCSSComments(string(raw))
		paren, brace := 0, 0
		// 报的是**那一段原文**而不是行号：注释被剥掉之后行号会错位，
		// 一个对不上号的行号比没有行号更耽误人。
		report := func(at string, i int) {
			lo := i - 70
			if lo < 0 {
				lo = 0
			}
			t.Errorf("%s %s（括号深度 %d、花括号深度 %d）……%s", f, at, paren, brace,
				strings.Join(strings.Fields(css[lo:i]), " "))
		}
		for i := 0; i < len(css); i++ {
			switch css[i] {
			case '(':
				paren++
			case ')':
				paren--
				if paren < 0 {
					report("右括号多出来了", i)
					return
				}
			case '}':
				if paren != 0 {
					report("这条声明的括号没配平，浏览器会把整条丢掉", i)
					return
				}
				brace--
				if brace < 0 {
					report("花括号多出来一个", i)
					return
				}
			case '{':
				brace++
			}
		}
		if paren != 0 || brace != 0 {
			t.Errorf("%s 文件末尾没收干净：括号 %d、花括号 %d", f, paren, brace)
		}
	}
}

// cssSelectorChunks 返回每个 `{` 之前那段"选择器"（从上一个 `{` 或 `}` 之后算起），
// 并把属性选择器 `[...]` 里的内容换成占位符——那里的引号是合法的。
// 它不是 CSS 解析器，只要能把"声明串到了选择器位"这种形状暴露出来就够了。
func cssSelectorChunks(css string) []string {
	var out []string
	start := 0
	for i := 0; i < len(css); i++ {
		switch css[i] {
		case '{':
			sel := strings.TrimSpace(css[start:i])
			if j := strings.LastIndexByte(sel, '}'); j >= 0 {
				sel = strings.TrimSpace(sel[j+1:])
			}
			if sel != "" {
				out = append(out, stripAttrBrackets(sel))
			}
			start = i + 1
		case '}':
			start = i + 1
		}
	}
	return out
}

// stripAttrBrackets 去掉 `[` … `]` 之间的内容（含嵌套与带引号的值）。
func stripAttrBrackets(sel string) string {
	var b strings.Builder
	depth := 0
	for _, r := range sel {
		switch {
		case r == '[':
			depth++
			b.WriteRune(r)
		case r == ']':
			if depth > 0 {
				depth--
			}
			if depth == 0 {
				b.WriteRune(r)
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// §7.5 与 §8.1 的硬数字由这份 map 逐字钉住。**为什么用文本在场断言而不是
// 只靠截图**：一次误删（比如重排 CSS 时删掉整个 @media 块）在截图上可能
// 几小时都看不出来，而这里一行就红。
func TestCSS_硬规格在场(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	// 两遍：源文件，以及**真正下发的那份**（注释已剥）。内联前的剥离机要是把某条
	// 规则一起吃掉，只扫源文件的这一闸会全绿而页面已经坏了——扫描型闸门必须
	// 自带"解析器没吞形状"的守卫。
	for label, css := range map[string]string{"源文件": string(raw), "内联下发": string(InlineCSS())} {
		t.Run(label, func(t *testing.T) { checkCSSSpecs(t, css) })
	}
}

func checkCSSSpecs(t *testing.T, css string) {
	t.Helper()
	wants := map[string]string{
		"毛玻璃卡片":       `backdrop-filter:blur(12px)`,
		"hover 抬升":    `translateY(-3px)`,
		"图标 hover 变换": `scale(1.07) rotate(-2deg)`,
		"磁贴内线条加粗":     `.tiles .ic-svg{stroke-width:2.5`,
		"图标尺寸下限 20px": `.ic-svg,.ic-img{width:22px;height:22px`,
		"触摸目标 44px":   `min-height:44px`,
		"减弱动效豁免":      `@media (prefers-reduced-motion:reduce)`,
		"高对比模式豁免":     `@media (forced-colors:active)`,
		// 钉的是**判据本身**而不是那个具体列宽：这一格要保证的是"list 形态由
		// [data-layout=list] 选、且按视口自适应分列"，而不是"必须是 360px"——
		// 后者把一个随手可调的像素值抬成了规格，改它就得改这里。
		// 从前钉的是 `minmax(0,1fr)`（单列满宽），2026-10-11 按实拍改成自适应。
		"列表形态":    `[data-layout=list] .rows{grid-template-columns:repeat(auto-fill,minmax(`,
		"暗亮双主题开关": `[data-theme=light]`,
		// 钉的是"有一道从透明淡入的入场"这个形状，不是位移那个像素值：
		// 9px→4px、0.5s→0.14s 都是量出来的调参，钉死了就改不动。
		// （真的把入场删掉，那才是这一行该响的时候——那时它红得对。）
		"入场 stagger": `@keyframes rise{from{opacity:0;transform:translateY(`,
		"网点纹理":       `background-size:3px 3px`,
		// 裁定 M-7：§7.5 的焦点环要有形状钉。上一条"触摸目标 44px"钉的是尺寸，
		// 没有任何一条钉住 outline——删掉 :focus-visible 那一行时 12 条全绿，
		// 键盘用户从此看不见焦点。
		"焦点环形状": `outline:3px solid var(--ring)`,
		// 裁定 M-4：长名字截断要同时盖住 .card 与 .plain 两种形态，
		// 只写 .card 时"需手动打开"那条会换行而不是省略。
		"长名截断两态": `.card b,.plain b{font-weight:600;overflow:hidden;text-overflow:ellipsis`,
		// 管理横幅与"两层 sticky 让位"这两格在 2026-10-10 随代管态一起删了，
		// 对应的 CSS 也清干净了——一条永不触发的规则比没有更糟：它看起来像在那里
		// 守着。替代它们的是下面那条**导入页的触摸目标**——那一族过去不在名单里，
		// 于是提交按钮与文件选择框都是 25px，同样是浏览器实测才发现的。
		// 2026-10-12：按钮族末尾多了 `.langsel`（界面语言那颗「切换语言」住自己的
		// 表单，同样一张族都没进，量出来 24px），同日第二批又多了 `.rename`
		// （顶栏改名的提交键，21px）。**逐字串跟着改了**——改选择器却不改
		// 这条钉，下一个人加容器时它会红，而红的是"你得看看这条钉说的是哪一族"。
		"确认/导入页的按钮入按钮族": `.adm button,.act button,.langsel button,.rename button{`,
		// 顶栏「改名」的浮层在 320 档打开后整页能横滚 5px（`scrollWidth=325`），因为
		// 那一行 `flex:0 0 auto` + `white-space:nowrap` 不参与收缩。钉的是"可缩 +
		// min-width:0"这一形状还在：改回不缩，320 档就回到那个能横滚的样子。
		"改名的标签与输入框要能缩": `.rename label{display:flex;align-items:center;gap:8px;font-size:13px;color:var(--ink2);flex:1 1 220px;min-width:0}`,
		"文件选择框入输入族":    `input[type=search],input[type=file]{min-height:44px}`,
		// 侧栏外壳（Task 6，spec §8.2）的 190px 常驻列是硬规格：grid-template-columns
		// 的第一个值就是它。逐字串截到 minmax(0,1fr)，不钉后面的 gap/margin 顺序。
		// 首列 190 → 206px 是 2026-10-11 给侧栏那根分隔线腾的内衬，
		// **文字宽度仍是 190**（`.side` 的 padding-right:16px）——上面 base.css
		// 那条"190px 定轨"管的是文字，换轨不改字。
		"侧栏常驻一列": `.shell{display:grid;grid-template-columns:206px minmax(0,1fr)`,
		// 满幅上限**三处同数**（2026-10-12）：`main`、`.shell`、`header`。顶栏曾
		// 经压根没有上限，于是 1440 档上它满幅而正文封顶 1180 居中，两边错开一百多
		// 像素——只有实拍看得出来，所以这里钉的是"三处写着同一个数"这件事本身，
		// 数字改了连同这条一起改。
		"正文满幅上限":   `main{max-width:1600px`,
		"顶栏与正文同上限": `header{display:flex;flex-wrap:wrap;align-items:center;gap:10px 12px;` + "\n" + `  max-width:1600px;`,
		// 平板断点 1100px（spec §8.2 的第二档）：侧栏在这一档收成抽屉。删掉整个
		// @media 块在截图上几小时看不出来，这里一行就红。
		"平板断点 1100px": `@media (max-width:1100px)`,
		// 手机档那条抽屉必须是 **grid**，不是 flex（2026-10-12 实测）：flex 那版
		// 里横向可滚的分组带按内容撑到 747px，把整页抬到 845 可横滚，而
		// `min-width:0`/`flex:1 1 0` 都收不住它。这条钉的是"第二轨的最小值被钉成 0"
		// 这件事——改回 flex 就会红，而红得对。
		"手机档抽屉用 grid 收住宽度": `.side .drawer{display:grid;grid-template-columns:auto minmax(0,1fr)`,
		// base.css 有一条全局 `ul{display:grid;grid-template-columns:auto-fill…}`
		// 是给磁贴用的，它一样会打到侧栏。侧栏必须显式把 display 覆盖回 block，
		// 否则浏览器里侧栏变成一堆 190px 宽的格子——这条是实测过的坑，而单测量不到
		// 它的宽度效果，所以只能用文本在场钉住这条覆盖声明本身。
		// 网格**只挂在条目列表那三个类上**：从前它挂在裸 `ul` 上，于是侧栏、⌘K 结果表、
		// 离线队列、报告清单每一条都要自己逃一次，漏一条就变成一排格子（⌘K 那一处
		// 的实测症状是方向键按"下一个"走到右边那一格）。侧栏因此不再需要 display 覆盖。
		"网格只挂在条目列表上": `.tiles,.rows,.profiles{display:grid`,
		// 改动历史那一页（2026-10-13）：卡片里那一簇短句要**自己占一行**，
		// 而 `.plain` 是个 flex 容器——少了这条覆盖，那一簇会挤到上面那两枚
		// 徽标旁边排成一行。钉它的理由与 `.tech` 那次同一条：类挂在对的元素上、
		// 规则却在别的文件里（或压根没有），页面上就是没样式，而浏览器不在 CI 里。
		"历史页的短句独占一行":  `.hist .ops{flex:1 0 100%`,
		"历史页的卡片之间有间距": `.hist{display:grid`,
		// 把手的开合指示（评审 I-3，Task 7 收宽成 summary 一族）：flex 把 UA 的
		// display:list-item 换掉了，展开三角随之消失（三角只在 list-item 显示，
		// Blink/Gecko 都一样），把手只剩 cursor:pointer 一个暗示——家属看不出那行
		// 能点。自己画回来的那两行内容必须由文本在场钉住：单测量不到"三角长什么样"，
		// 只有浏览器知道，所以这里钉的是声明本身（同上面那条覆盖声明的口径）。
		// 名字不带 .drawer 前缀：侧栏抽屉与解释页细节块共用这一只手，只钉抽屉时
		// 解释页没人管——这就是本任务把前缀去掉的理由。
		"抽屉把手有开合标记": `summary::before{content:"▸"`,
		// 开着的另一半也要有人守着：只钉 ▸ 时删掉 [open] 那条会让抽屉永远显示
		// "收起"的箭头，而这条病今天无人问（评审 N-2）。
		"抽屉把手的展开态": `details[open]>summary::before{content:"▾"`,
		// 色点的消费者：模板发的 style="--sec-accent:…" 只是把值送到元素身上，
		// 没有这条规则它就是个隐形格子。页面侧有生产者证人（icon_test.go），
		// CSS 侧缺的是这半边（评审 N-2）。
		"侧栏色点画得出来": `.gnav .dot{flex:0 0 auto;width:10px;height:10px;border-radius:50%`,
		// 解释页的细节块用类名（Task 7 裁定②）：这条钉的是"它是个类而不是裸元素"，
		// 裸元素写法会把 margin/padding 漏给侧栏那个 .drawer。
		"细节块用类名": `.tech{margin:14px 0;border:1px solid`,
		// 细节块里那句原文的**换行形状**（评审 R-3）：store 给的原因是多行、含长
		// JSON 键名，pre-wrap 保换行、break-all 让窄屏折行而不是横向滚出屏幕。
		// 删掉这两条声明在单测里量不到任何后果（渲染字节一模一样），只有浏览器
		// 知道——所以按本函数的口径钉声明本身，同上面那条覆盖声明。
		"细节块原文保换行且折行": `.tech pre{margin:0;white-space:pre-wrap;word-break:break-all`,
		// 引导表单两格竖排时必须显式 stretch：`.pick label` 给的是 center，列向 flex
		// 里 center 管的是水平轴，输入框会被 flex:1 1 auto 收成内容宽的一小块。
		// 这条在单测里量不到宽度，只有浏览器知道——所以按本函数的口径钉声明本身。
		"引导表单的输入框铺满": `.setup label{display:flex;flex-direction:column;align-items:stretch`,
		// 细节块在高对比模式下的**那一条**（评审 R-3 的另一半）：上面"高对比模式
		// 豁免"钉的是 `@media (forced-colors:active)` 这个串在场，而它在文件里出现
		// 四次，删掉 .tech 这一条它照样绿。系统色会盖掉 --ring 描边，没有这条
		// CanvasText 边框，"技术细节"那块就与正文糊成一片。
		"细节块的高对比边框": `@media (forced-colors:active){.tech{border:1px solid CanvasText`,
		// 就地编辑的面板**不能被自己那一节裁掉**：`content-visibility:auto` 带 paint
		// containment，面板比这一节剩下的空间高时，多出来那截不画也点不到（实测
		// `elementFromPoint` 交出下一节的 h2）。这条与上面那条 containment 是一对，
		// 删了它=「删掉这条」那颗键在界面上消失，而功能测试照样全绿。
		"开着编辑器的那一节退出 containment": `section:has(.inline-edit){content-visibility:visible}`,
		// 顶栏搜索表单（Task 2）新加的两个控件必须留在 §7.5 那两条 44px 规则的
		// 名单里。这里**不放逐字串**：逐字串会把选择器顺序也钉住（列表内部重排
		// 不该红），又钉不住"值在哪条规则里"。结构判据见
		// TestCSS_搜索控件的44px由哪条规则保证。
	}
	for name, want := range wants {
		if !strings.Contains(css, want) {
			t.Errorf("%s 必须写在 CSS 里（原样找 %q）", name, want)
		}
	}
	// 开合三角的名字要**判别式**地钉住（评审 I-2）：上面那条子串证人打不过"改回
	// .drawer>summary::before"——那条串是它的后缀，解释页的把手会静悄悄退回只剩
	// cursor:pointer。这里问的是"这条声明挂在哪个选择器上"，名单必须恰好是 summary::before
	// 一个（Task 7 把手一族收宽到全站，判据与 44px 那条同一口径）。
	if got := selectorsWithDeclaration(css, `content:"▸"`); len(got) != 1 || got[0] != "summary::before" {
		t.Errorf(`▸ 标记必须挂在裸 summary::before 上（抽屉与解释页共用一只手），实际名单 %v`, got)
	}
	// 开与合是**一对**，两只都要判别式地钉（评审 R-4）：上面那条 ▸ 的名单证人守着
	// 收合态，而展开态今天只有一条子串证人——把 `details[open]>summary::before`
	// 改成任何别的复合选择器（比如 `.drawer details>summary::before`）子串照样命中，
	// 解释页的细节块就永远显示"收起"的箭头。口径与 ▸ 那条一致：问"这条声明挂在
	// 哪个选择器上"，名单必须恰好是那一条。两只分开钉而不是钉一条联合选择器，
	// 是因为这一对本来就分属两条规则（闭合态全站共用，展开态要 [open] 前缀），
	// 合起来写反而会放过"两条被并成一条、展开态失效"那种改法。
	if got := selectorsWithDeclaration(css, `content:"▾"`); len(got) != 1 || got[0] != "details[open]>summary::before" {
		t.Errorf(`▾ 标记必须只挂在 details[open]>summary::before 上（与 ▸ 成对，别并到全站 summary 上），实际名单 %v`, got)
	}
	// 裁定 V2 的反钉：分组色的**判据**只有一份（exec.go 的 validatedAccent），挂载点
	// 现在是两处——section 的 --sec-accent 与侧栏那枚色点（设计 §7 的三件套）。
	// 条目身上出现 --chip-bg 仍然是给"第二个真相"预留插座，与挂载点数量无关。
	// 页面侧有生产者证人（icon_test.go#TestProfilePage_分组色只从判据进），
	// CSS 侧没有消费者证人，今天有人把规则加回来只有半边守卫会 notice——这条补上另半边。
	if strings.Contains(css, "--chip-bg") {
		t.Error("CSS 里不许出现 --chip-bg：那是分组色的第二条入口，契约判死（见 exec.go 注释）")
	}
}

// 部署验收 #3（Group.layout 存进盘却没人消费）的模板侧证人。CSS 里钉
// [data-layout=list] 只证明"选择器在场"——模板不发属性的话那就是死码，
// 形状钉照样绿。属性必须由页面发出来，所以要在这里断言。
func TestProfilePage_两种layout都落地(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Groups = []model.Group{
		{ID: "gt", Title: "磁贴组", Layout: model.LayoutTiles, Items: []model.Item{
			{ID: "i1", Name: "甲", URL: "http://10.0.0.1/", Alias: []string{}, Tags: []string{}},
		}},
		{ID: "gl", Title: "列表组", Layout: model.LayoutList, Items: []model.Item{
			{ID: "i2", Name: "乙", URL: "http://10.0.0.2/", Alias: []string{}, Tags: []string{}},
		}},
	}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	body := string(mustRender(t, s, "dad"))
	for _, want := range []string{`data-layout="tiles"`, `data-layout="list"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("section 必须把 Group.layout 发成 data-layout 属性，缺 %q:\n%s", want, body)
		}
	}
}

// 裁定 I-1：data-theme 只能表"用户显式选了哪一档"。
//
// "auto" 是 model/check.go 的合法枚举、同时是 store.CreateProfile 的建档默认值，
// 而模板的 {{with .Theme}} 对 "auto" 取真 → 页面发出 data-theme="auto"。CSS 的两条
// 亮色通路是 [data-theme=light] 与 :root:not([data-theme])：属性在，媒体查询那条不
// 匹配；值不是 light/dark，显式那条也不匹配。结果就是"系统偏好亮色 + 默认主题"的
// 家庭成员永远拿到暗色，而且 color-scheme:dark 连原生控件一起压暗。
//
// 归一发生在 exec.go（见那里的注释：属性存在 = 显式选择，不存在 = 交回媒体查询，
// 同一个事实只写一次）。这里三条一页，断的是端到端的结果而不是函数内部。
//
// 夹具一律走磁盘（SaveProfile）而不是直接喂 executor：theme 落成 HTML 要经过
// load → exec → tmpl 三段，绕开任何一段就等于没测那一段。
func TestProfilePage_theme属性只表显式选择(t *testing.T) {
	for _, tc := range []struct {
		theme string
		want  string // 空串 = 这个属性根本不许出现
	}{
		{theme: "auto", want: ""},
		{theme: "light", want: `data-theme="light"`},
		{theme: "dark", want: `data-theme="dark"`},
	} {
		s := seeded(t)
		p, err := s.LoadProfile("dad")
		if err != nil {
			t.Fatal(err)
		}
		p.Theme = tc.theme
		if err := s.SaveProfile(p); err != nil {
			t.Fatal(err)
		}
		// 每个 theme 一个新渲染器只是夹具习惯：页面缓存键是
		// (档案,home+指纹,rev,bookmarks.mtime,profile 指纹,iconGen)，
		// 键本身已经认 profile.json，改 theme 顶掉旧页面不再需要新渲染器。
		body := string(mustRender(t, s, "dad"))
		// 断言范围是 <html> 那一段，不是整页：base.css 内联进每一页，而 CSS 里
		// 就写着 [data-theme=light] 与 :root:not([data-theme]) 两条选择器——
		// 整页 Contains "data-theme" 永远为真，那样这条证人从一开始就是死的。
		head := htmlHead(body)
		if !strings.Contains(head, `<html lang="zh-CN"`) {
			t.Fatalf("没渲染出页面头:\n%s", head)
		}
		if tc.want == "" {
			if strings.Contains(head, "data-theme") {
				t.Errorf("theme=auto 不许发 data-theme 属性（auto 由媒体查询裁定，属性只表显式选择）:\n%s", head)
			}
			continue
		}
		if !strings.Contains(head, tc.want) {
			t.Errorf("theme=%s 要发 %q:\n%s", tc.theme, tc.want, head)
		}
		if strings.Contains(head, `data-theme="auto"`) {
			t.Errorf("theme=%s 却发出了 auto——exec.go 的归一没生效:\n%s", tc.theme, head)
		}
	}
}

// 首页三种形状（设计 §2）。判据由 httpapi 算，这里只验"给了什么就印什么"，
// 以及**模板不数档案**：它要是自己数了，就有了第二处判据。
func TestPickerPage_三形状(t *testing.T) {
	s := seeded(t) // dad、mom 两个档案
	save(t, s, "dad", "爸爸的台", "dsm")
	save(t, s, "mom", "妈妈的台", "emby")
	r := renderer(t, s)

	// 「≥2」这一支**故意不传 Heading**：那三个字只许写在一个地方（render 的默认值），
	// httpapi 只负责判断"现在是多人形状"。传进来就等于两处各存一份文案，
	// 而改文案的人只会改一处——另一处变成没人看见的第二份真相。
	two, err := r.PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// 基线：≥2 的形状与 P1-④ 验收时逐字一致的两条硬事实。
	if !strings.Contains(string(two.Body), "<h1>谁来用？</h1>") {
		t.Errorf("多人形状 h1 不对（默认标题没接上，或漏传时渲染了空白标题）:\n%.300s", two.Body)
	}
	if n := strings.Count(string(two.Body), `action="/pin"`); n != 2 {
		t.Errorf("两张档案卡要两个 /pin 表单，得到 %d", n)
	}
	if strings.Contains(string(two.Body), `action="/setup"`) {
		t.Error("有档案的页面不许画引导表单")
	}
	// 「≥2 的形状逐字不动」要有字节级的证人：{{if}}/{{else}} 包一层时，if 的 end
	// 前那段缩进会漏进输出（评审 F6 实测多 3 个字节），而卡片数、h1 全都照旧。
	// `</ul>\n</main>` 这条接缝就是包 wrapper 之前 HEAD 的输出形状，{{- end}} 收的就是它。
	if !strings.Contains(string(two.Body), "</ul>\n</main>") {
		t.Errorf("≥2 形状的输出接缝变了（wrapper 的缩进漏进页面），尾部是:\n%s",
			tail(string(two.Body), 60))
	}

	one, err := r.PickerPage(PickerOptions{Heading: "爸爸的台"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(one.Body), "<h1>爸爸的台</h1>") {
		t.Errorf("单人形状 h1 要直接是那个人的名字:\n%.300s", one.Body)
	}
	if strings.Contains(string(one.Body), "谁来用") {
		t.Error("只有一个档案还问「谁来用？」——这正是本片要消掉的默认值")
	}

	setup, err := r.PickerPage(PickerOptions{Heading: "还没有档案", Setup: true})
	if err != nil {
		t.Fatal(err)
	}
	sb := string(setup.Body)
	if !strings.Contains(sb, `<form method="post" action="/setup"`) {
		t.Errorf("引导形状要有指向 /setup 的表单:\n%.300s", sb)
	}
	for _, want := range []string{`name="title"`, `name="pin"`, "<h1>还没有档案</h1>"} {
		if !strings.Contains(sb, want) {
			t.Errorf("引导表单缺 %q:\n%.400s", want, sb)
		}
	}
	if strings.Contains(sb, `<ul class="profiles">`) {
		t.Error("零档案的页面不该画档案卡列表（那是空列表死路的形状）")
	}
	if strings.Contains(sb, `action="/pin"`) {
		t.Error("零档案的页面没有档案可 pin")
	}
	// Setup 分支不能自己数档案：它只在显式给了 Setup 时出现。
	if strings.Contains(sb, "谁来用") {
		t.Errorf("引导页不许再问「谁来用？」:\n%.200s", sb)
	}
	// 引导表单的 44px 是**借来的**：输入框归 input[type=text|password] 那条，提交
	// 按钮归 `.pick button` 那条。所以它必须带 pick 这个类——CSS 侧的名单证人只认
	// 选择器，模板掉类名它看不见；这条是消费者那半边（同 --sec-accent 的口径）。
	cl := formClasses(t, sb, `/setup`)
	if !cl["pick"] || !cl["setup"] {
		t.Errorf("引导表单的 class 要同时含 pick（按钮族的 44px 由它接）与 setup，得到 %v", cl)
	}
	// 引导页那一格是**正在创建口令**的那一格：浏览器在这里最容易跳"保存密码"，
	// 一保存就等于把家人的 4-6 位数字存进云端密码库（局域网方便优先的定位里，
	// 这是唯一一条会把 PIN 送出家门的通路）。Task 6 复扫实测：这一格当时漏了
	// autocomplete="off"，而进门那一排卡片有——同一条纪律在两处形状不一致。
	off := assertPINsNoSave(t, sb)
	if n := strings.Count(off, "password"); n != 1 {
		t.Errorf("引导页该恰好一格 PIN，得到 %d 格", n)
	}
	assertPINsNoSave(t, string(two.Body))
}

// assertPINsNoSave 逐个检查页面上每一格 `type="password"` 都带 autocomplete="off"，
// 返回扫过的 input 开标签（拼起来给调用方数格数）。一格都没扫到**不算过**——
// 那是模板把 type 换了写法，正是这条证人要防的漂移。
func assertPINsNoSave(t *testing.T, body string) string {
	t.Helper()
	var seen strings.Builder
	for i := 0; ; {
		j := strings.Index(body[i:], `type="password"`)
		if j < 0 {
			break
		}
		pos := i + j
		open := strings.LastIndex(body[:pos], "<input")
		end := strings.Index(body[pos:], ">")
		if open < 0 || end < 0 {
			t.Errorf("type=\"password\" 外面找不到一个完整的 <input> 标签")
			break
		}
		tag := body[open : pos+end+1]
		seen.WriteString(tag)
		if !strings.Contains(tag, `autocomplete="off"`) {
			t.Errorf("这一格 PIN 没关密码保存，浏览器会提示把家人的口令存进云端: %s", tag)
		}
		i = pos + end + 1
	}
	if seen.Len() == 0 {
		t.Error("页面里一格 type=\"password\" 都没有：这条证人正在空转")
	}
	return seen.String()
}

// formClasses 取出 action 含 want 的那个 <form> 开标签上的 class，按空格拆成集合。
// 不逐字钉 `class="pick setup"`：那会把类名顺序也钉住，而顺序在这里没有语义。
func formClasses(t *testing.T, body, want string) map[string]bool {
	t.Helper()
	i := strings.Index(body, `action="`+want+`"`)
	if i < 0 {
		t.Fatalf("页面里没有 action=%q 的表单", want)
	}
	open := strings.LastIndex(body[:i], "<form")
	if open < 0 {
		t.Fatalf("%q 前面找不到 <form", want)
	}
	tag := body[open:]
	if j := strings.Index(tag, ">"); j >= 0 {
		tag = tag[:j]
	}
	k := strings.Index(tag, `class="`)
	if k < 0 {
		t.Fatalf("表单 %s 没有 class", tag)
	}
	tag = tag[k+len(`class="`):]
	if j := strings.Index(tag, `"`); j >= 0 {
		tag = tag[:j]
	}
	out := map[string]bool{}
	for _, c := range strings.Fields(tag) {
		out[c] = true
	}
	return out
}

// tail 取末尾 n 个字节：接缝类的诊断只在页尾，打整页等于没打。
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// 引导页的标题与表单文案要能被人读懂（这是给人看的第一屏，不是给 AI 的）。
// 钉"建完这一步不能再建第二次"与"管理权不随档案走"两句：它们是承诺，说了就要做。
//
// **"这个人是管理员"那句在 2026-10-12 被撤掉了**：管理员是机器级的那把 key，
// 建出来的档案只是第一个人的书签页。断言跟着换了——钉的是现在该被说的那两句。
func TestPickerPage_引导页把后果说在页面上(t *testing.T) {
	s := seeded(t)
	r := renderer(t, s)
	setup, err := r.PickerPage(PickerOptions{Heading: "还没有档案", Setup: true})
	if err != nil {
		t.Fatal(err)
	}
	body := string(setup.Body)
	for _, want := range []string{"管理密钥", "管理页", "建完这一次它就关掉"} {
		if !strings.Contains(body, want) {
			t.Errorf("引导页缺 %q（它得把后果说清）:\n%.500s", want, body)
		}
	}
	// 反面：这一页**不许**再承诺"建出来的人就是管理员"——那是一条已经删掉的规则，
	// 而它是新装机器的第一屏，撒谎的代价是让人以为不用那把 key 也能管这台机器。
	if strings.Contains(body, "这台导航台的管理员") {
		t.Errorf("引导页还在说「第一份就是管理员」，而管理员已是机器级的一把 key:\n%.500s", body)
	}
}

// 不属于任何档案的三页（选择页/解释页/管理页）一个都不许发 data-theme：
// 没有一份 profile.json 可问"这人显式选了哪档"，替档案发一个属性就是做主。
// 这一条在 2026-10-07 整枝评审 Minor 5 之前是"字段恒空串 + 模板支路恒不进"的
// 死形状，现在字段与支路一起删了——这条钉住它别再长回来一半。
// 正向对照在上面那条主题测试：档案页确实会发这个属性，所以下面的 Contains 不是
// "整页永远找不到 data-theme"的空转（want=0 的形状要配同文件的真值对照）。
func Test无档案的三页都不发data_theme(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	r := renderer(t, s)
	picker, err := r.PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := r.BrokenPage("ghost", "bookmarks.json 读不了")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸的台", Rev: 5, Items: 1}}, "")
	if err != nil {
		t.Fatal(err)
	}
	// 先证这三页真的渲染出了内容（空白页会让下面那句永远为真）。
	for name, pg := range map[string]string{"选择页": string(picker.Body), "解释页": string(broken.Body), "管理页": string(admin.Body)} {
		if !strings.Contains(htmlHead(pg), "<html lang=") {
			t.Fatalf("%s 没渲染出文档头: %.200s", name, pg)
		}
		if strings.Contains(htmlHead(pg), "data-theme") {
			t.Errorf("%s 发了 data-theme：它不属于任何档案，没有主题可说: %.200s", name, htmlHead(pg))
		}
	}
}

// 顶栏那枚主题按钮是无 JS 时唯一改得动主题的通路，两条断言各挡一种谎：
//   - 提交值必须是 /prefs 认的枚举值：写错一个字母，按钮每次点都换来一句 422，
//     而页面看上去一切正常。判据用 model.ValidTheme——模板与端点不许各写一份名单。
//   - 提交值必须不等于当前值："点一下什么也没变"的按钮不如不放。
//
// 这条测的是模板，端点那侧的证人在 httpapi/prefs_test.go；两侧都钉住才叫配对。
func TestProfilePage_顶栏主题表单交得出合法值(t *testing.T) {
	hiddenThemeRE := regexp.MustCompile(`name="theme" value="([^"]*)"`)
	// 这张表和 model 那张表必须是同一套档位：少一格是模板没跟上（循环里那次
	// Fatalf 会点名），多一格是值集删了档而测试还留着——那也会让模板里留下
	// 一条永远走不到的分支。
	if len(wantNext) != len(model.ThemeValues) {
		t.Fatalf("wantNext 有 %d 格，model.ThemeValues 有 %d 档：值集变了，这张表也要跟着变",
			len(wantNext), len(model.ThemeValues))
	}
	for _, theme := range model.ThemeValues {
		s := seeded(t)
		p, err := s.LoadProfile("dad")
		if err != nil {
			t.Fatal(err)
		}
		p.Theme = theme
		if err := s.SaveProfile(p); err != nil {
			t.Fatal(err)
		}
		body := string(mustRender(t, s, "dad"))
		// 钉的是"**带 theme 字段的那张表单**恰好一张"，不是"/prefs 表单恰好一张"：
		// 2026-10-12 顶栏多了「改名」那一格，它也 POST /prefs（同一族、同样只改自己）。
		// 按 action 数会把两件不同的事数成一件事，按字段数才是在说"主题通路没被复制"。
		var withTheme, withTitle int
		for _, f := range regexp.MustCompile(`(?s)<form method="post" action="/prefs".*?</form>`).FindAllString(body, -1) {
			if strings.Contains(f, `name="theme"`) {
				withTheme++
			}
			if strings.Contains(f, `name="title"`) {
				withTitle++
			}
		}
		if withTheme != 1 {
			t.Fatalf("theme=%s：顶栏要恰好一张带 theme 的表单，got %d", theme, withTheme)
		}
		if withTitle != 1 {
			t.Fatalf("theme=%s：顶栏要恰好一张改名表单（带 title 字段），got %d", theme, withTitle)
		}
		m := hiddenThemeRE.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("theme=%s：表单没有 hidden 的 theme 字段——值由服务端算并写死在 hidden 里，无 JS 才有下一档可交:\n%.300s", theme, body)
		}
		next := m[1]
		if !model.ValidTheme(next) {
			t.Errorf("theme=%s 交出去的下一档 %q 不在枚举里，/prefs 会 422 整批", theme, next)
		}
		if next == theme {
			t.Errorf("theme=%s 却交出同一个值：按下去什么都不改的按钮是装饰，不是通路", theme)
		}
		// **三态循环要逐格钉死，不能只钉"合法且不等于当前值"**：两态互切
		// （light↔dark）完全满足那两条，却把 auto 变成回不去的状态——而 auto 是
		// 建档默认值（store.CreateProfile）。spec §8.1 写的是 auto→dark→light→auto。
		// 表里没有这一档 = model 加了档而模板与这张表都还没跟上，点名它比
		// 让上面那条交出空串更清楚（评审 I-6：值集不许只有一份证人）。
		nx, ok := wantNext[theme]
		if !ok {
			t.Fatalf("theme=%q 在 model.ThemeValues 里，wantNext 却没有它：加一档要同时改模板、按钮文案与这张表", theme)
		}
		if wantNext[theme] != next {
			t.Errorf("theme=%s 交出 %q，循环里该交 %q", theme, next, nx)
		}
		// 按钮上的字说的是**当前这一档**：三态循环下"切到亮色"不告诉人现在在哪一档。
		label, ok := wantLabel[theme]
		if !ok {
			t.Fatalf("theme=%q 有循环格却没有按钮文案格：交得出下一档却说不出它是哪档，等于没说", theme)
		}
		if !strings.Contains(body, label) {
			t.Errorf("theme=%s 的按钮文案不含 %q（文案要说的是当前这一档）:\n%.300s", theme, label, body)
		}
	}
}

// 主题循环的完整形状：三格全钉，循环走一遍能回到起点。
var wantNext = map[string]string{"auto": "dark", "dark": "light", "light": "auto"}

var wantLabel = map[string]string{
	"auto":  "主题：自动",
	"dark":  "主题：暗",
	"light": "主题：亮",
}

// htmlHead 只截 <style> 之前的部分：内联 CSS 有几 KB，整页打进失败输出没人看得下去，
// 而 data-theme 就写在 <html> 那一行上。
func htmlHead(body string) string {
	if i := strings.Index(body, "<style>"); i > 0 {
		return body[:i]
	}
	if len(body) > 300 {
		return body[:300]
	}
	return body
}

// 裁定 I-2 / M-3：对比度是**算出来的**，不是看出来的。
//
// 为什么不许靠眼睛：Task 12 才有浏览器，而 §7.5（文字/图标可读）与 §12（缺图徽标要
// 可见）是硬指标。上一轮的病正在这里——两条亮色通路只覆写了九个变量，--accent/--ring/
// --warn 沿用暗档值，实测 warn 压在亮档卡片上只有 2.0:1、焦点环 2.3:1，规则"在场"
// 却名存实亡；这种失败形状文本在场钉根本抓不到，只能把颜色读出来按公式量。
//
// 口径（WCAG 2.x 相对亮度，纯 Go 算术，不引任何前端工具链/外部色值表）：
//   - 线性化阈值 0.04045，L = 0.2126R + 0.7152G + 0.0722B；
//   - 对比度 = (L1+0.05)/(L2+0.05)；
//   - 文本 4.5:1（11px 徽标、12px 色块首字母都够不上大字号门槛 18pt/14pt 粗体）；
//   - 非文本 3:1（焦点环是 3px 描边、h2 前是 10px 小方块，都按 UI 组件判）；
//   - 半透明的 --card 先按 alpha 压在同一通路的 --bg 上再参与计算：极光底被
//     backdrop-filter 糊过一层，近似成底色是这里唯一站得住的算法（真值由 Task 12 实拍）。
func TestCSS_对比度算得过(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	// 注释里出现的十六进制串不是声明，先剥掉注释再读变量。**用产品自己那台
	// 剥离机**（stripCSSComments，字符串感知）而不是这里再写一份正则：两份实现
	// 会漂移，而漂移的结果是"测试读的 CSS 与浏览器收到的不是同一份"。
	css := stripCSSComments(string(raw))

	dark := cssVars(t, css, ":root{")
	// 亮色的两条通路各自 = 暗档默认 + 该块的覆写（CSS 层叠就是这么算的）。
	lightExplicit := cssOverlay(dark, cssVars(t, css, "[data-theme=light]{"))
	lightMedia := cssOverlay(dark, cssVars(t, css, ":root:not([data-theme]){"))

	// 两条亮色通路必须逐字同值：只补一条的结果是"显式选了 light 的人达标、
	// 跟着系统偏好走的人不达标"，这种分裂到实拍里只会呈现成"偶尔不行"。
	for _, name := range cssUnionKeys(lightExplicit, lightMedia) {
		if lightExplicit[name] != lightMedia[name] {
			t.Errorf("亮色两条通路在 %s 上分家：[data-theme=light] 是 %q，媒体查询那条是 %q",
				name, lightExplicit[name], lightMedia[name])
		}
	}

	for _, p := range []struct {
		name string
		vars map[string]string
	}{
		{"暗档 :root", dark},
		{"亮档 [data-theme=light]", lightExplicit},
		{"亮档 :root:not([data-theme]) 媒体查询", lightMedia},
	} {
		checkCSSContrasts(t, p.name, p.vars)
	}
	checkChipWhiteText(t, css)
}

// contrastWants：谁压在谁上、要多少、为什么。
var contrastWants = []struct {
	what   string
	fg, bg string
	min    float64
	why    string
}{
	{"正文 --ink / --bg", "--ink", "--bg", 4.5, "§7.5 正文可读"},
	{"次级 --ink2 / --card", "--ink2", "--card", 4.5, "§7.5 描述与标签可读"},
	{"徽标 --warn / --card", "--warn", "--card", 4.5, "§12 缺图徽标要看得见（11px 小字）"},
	{"焦点环 --ring / --card", "--ring", "--card", 3.0, "§7.5 focus-visible（非文本 3:1）"},
	{"分组色 --accent / --card", "--accent", "--card", 3.0, "§8.1 h2 小方块与 hover 描边（非文本 3:1）"},
	// 实心色块上的字色与"只有字没有块"的报错色。这三对是 2026-10-10 打磨轮补的：
	// 白字压在暗档 --accent 上实测 3.20:1、压在 --bad 上 2.52:1，而它们分别是
	// 顶栏主操作键与删档确认键——最该看清的两枚。证人当时只算了"字压在半透明卡片上"
	// 那一族，色块上的字一个都没量到。
	{"顶栏主操作键字色 --on-accent / --accent", "--on-accent", "--accent", 4.5, "§7.5 主操作键上的 14px 字"},
	{"删档键字色 --on-bad / --bad", "--on-bad", "--bad", 4.5, "§7.5 危险动作键上的字"},
	{"报错状态行 --bad-text / --card", "--bad-text", "--card", 4.5, "§8.7 写失败必须显式说，字得先看得见"},
}

func checkCSSContrasts(t *testing.T, path string, vars map[string]string) {
	t.Helper()
	bg, ok := cssColor(t, vars["--bg"])
	if !ok {
		t.Errorf("%s：读不到 --bg，无法给半透明的 --card 找底色", path)
		return
	}
	for _, w := range contrastWants {
		fgRaw, bgRaw := vars[w.fg], vars[w.bg]
		if fgRaw == "" || bgRaw == "" {
			t.Errorf("%s：%s 或 %s 在这条通路上没有值（证人读不到变量时该补解析，不该删断言）", path, w.fg, w.bg)
			continue
		}
		fg, fgOK := cssColor(t, fgRaw)
		card, bgOK := cssColor(t, bgRaw)
		if !fgOK || !bgOK {
			continue
		}
		card = compositeOver(card, bg)
		r := wcagContrast(fg, card)
		t.Logf("%s：%s = %.2f:1（门槛 ≥%.1f:1，压 %s）", path, w.what, r, w.min, cssHex(card))
		if r < w.min {
			t.Errorf("%s：%s 的 %s 压在 %s(%s) 上只有 %.2f:1，要 ≥%.1f:1（%s）",
				path, w.what, strings.TrimSpace(fgRaw), w.bg, cssHex(card), r, w.min, w.why)
		}
	}

	// 上面那一族全部按 `--card 压在 --bg 上` 算，而**透明底的东西不是压在卡片上**：
	// 管理页表格里的 .badge.warn 就在页面本体上，那一格没有 --card 这一层。
	// 取景按像素量到它压在 `--bg + 极光` 上只有 4.41:1，而按 --card 算是 5.45:1——
	// 证人算的是那一格里根本不存在的底。极光那层是 background-image，静态读
	// background-color 永远看不见，所以这里自己合成一次再量。
	// 只量 --warn：它是透明底上唯一的**小字**彩色件（11px）；--ring/--accent 落在
	// 页底上按非文本 3:1 判，2026-10-12 实测两套都过，没有需要钉的新谎话。
	if a1, ok := cssColor(t, vars["--aurora-1"]); ok {
		page := compositeOver(a1, bg)
		if w, wok := cssColor(t, vars["--warn"]); wok {
			r := wcagContrast(w, page)
			t.Logf("%s：徽标 --warn / 极光页底 = %.2f:1（压 %s）", path, r, cssHex(page))
			if r < 4.5 {
				t.Errorf("%s：--warn %s 压在带极光的页底 %s 上只有 %.2f:1，要 ≥4.5:1"+
					"（管理页表格那枚「数据文件读不了」是透明底，11px 小字）",
					path, strings.TrimSpace(vars["--warn"]), cssHex(page), r)
			}
		}
	}
}

// 首字母色块（.ic）：白字压在 hsl(色相 饱和 亮度) 上。色相来自条目名的 fnv 哈希
// （icon.go 的 chipHue，全域分布），所以这里必须遍历 h=0..359 取**最差**的那个值来
// 断言——不许把某几个色相特判掉，遍历本身就是这条断言的意义（上一轮的 46% 就是被
// h=60° 打红的：黄区亮度最高，白字只剩 1.98:1）。
func checkChipWhiteText(t *testing.T, css string) {
	t.Helper()
	rule := regexp.MustCompile(`(?s)\.ic\{([^}]*)\}`).FindAllStringSubmatch(css, -1)
	var body string
	n := 0
	for _, m := range rule {
		if strings.Contains(m[1], "background:hsl(") {
			body = m[1]
			n++
		}
	}
	if n != 1 {
		t.Fatalf("带 background:hsl(...) 的 .ic 规则有 %d 处，要恰好 1 处（多了就是色块的第二条入口）", n)
	}
	if !strings.Contains(body, "color:#fff") {
		t.Fatal(".ic 的字色不再是写死的 #fff：白字遍历的前提变了，要在这里改解析而不是删断言")
	}
	m := regexp.MustCompile(`background:hsl\(var\(--chip-h,\s*\d+\)\s+([0-9.]+)%\s+([0-9.]+)%\)`).
		FindStringSubmatch(body)
	if len(m) != 3 {
		t.Fatalf("读不出 .ic 的色块底（要 background:hsl(var(--chip-h,N) S%% L%%) 这个形状），得到: %s", body)
	}
	sat, light := mustFloat(t, m[1]), mustFloat(t, m[2])
	white := cssRGBA{1, 1, 1, 1}
	worstH, worst := -1, math.MaxFloat64
	for h := 0; h < 360; h++ {
		if r := wcagContrast(white, hslToRGB(float64(h), sat, light)); r < worst {
			worstH, worst = h, r
		}
	}
	if worst < 4.5 {
		t.Errorf("色块白字：hsl(S=%g%% L=%g%%) 遍历 0..359 的最差色相 h=%d° 只有 %.2f:1，要 ≥4.5:1（12px 粗体不算大字号）",
			sat, light, worstH, worst)
	}
	t.Logf("色块白字：hsl(S=%g%% L=%g%%) 遍历 0..359 最差 h=%d° = %.2f:1（门槛 ≥4.5:1）", sat, light, worstH, worst)
}

// cssRGBA 是 0..1 的颜色，a=1 表示不透明。
type cssRGBA struct{ r, g, b, a float64 }

// cssVars 取出某个选择器块里的 CSS 变量声明。选择器在文件里必须恰好出现一次——
// 证人不能猜读哪一块（:root 与 :root:not([data-theme]) 是两个不同的事实源）。
func cssVars(t *testing.T, css, selector string) map[string]string {
	t.Helper()
	if n := strings.Count(css, selector); n != 1 {
		t.Fatalf("选择器 %q 在 CSS 里出现 %d 次，要恰好 1 次", selector, n)
	}
	open := strings.Index(css, selector) + len(selector) - 1 // 指向 '{'
	depth := 0
	for j := open; j < len(css); j++ {
		switch css[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				out := map[string]string{}
				for _, decl := range strings.Split(css[open+1:j], ";") {
					name, value, ok := strings.Cut(decl, ":")
					name = strings.TrimSpace(name)
					if ok && strings.HasPrefix(name, "--") {
						out[name] = strings.TrimSpace(value)
					}
				}
				if len(out) == 0 {
					t.Fatalf("选择器 %q 里一个 CSS 变量都没有，证人读错了块", selector)
				}
				return out
			}
		}
	}
	t.Fatalf("选择器 %q 的块没有收尾", selector)
	return nil
}

// cssOverlay 复刻层叠：覆写块赢，没覆写的沿用 :root 的默认值。
func cssOverlay(base, over map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

func cssUnionKeys(a, b map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]string{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// cssColor 认三种写法就够：#rgb、#rrggbb、rgba(r,g,b,a)（rgb() 当 alpha=1）。
// 读不懂一律 Fatalf——把颜色静悄悄当成黑色来"过测试"是本文件最坏的失败模式。
func cssColor(t *testing.T, raw string) (cssRGBA, bool) {
	t.Helper()
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 {
			hex = fmt.Sprintf("%c%c%c%c%c%c", hex[0], hex[0], hex[1], hex[1], hex[2], hex[2])
		}
		if len(hex) != 6 {
			t.Fatalf("颜色 %q 不是 #rgb / #rrggbb", raw)
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			t.Fatalf("颜色 %q 解析不了: %v", raw, err)
		}
		return cssRGBA{float64((v>>16)&0xff) / 255, float64((v>>8)&0xff) / 255, float64(v&0xff) / 255, 1}, true
	}
	if strings.HasPrefix(s, "rgb(") || strings.HasPrefix(s, "rgba(") {
		open := strings.Index(s, "(")
		body := strings.TrimSuffix(strings.TrimSpace(s[open+1:]), ")")
		fields := strings.FieldsFunc(body, func(r rune) bool { return r == ',' })
		if len(fields) == 3 {
			fields = append(fields, "1")
		}
		if len(fields) != 4 {
			t.Fatalf("颜色 %q 的函数参数不是 3 或 4 个", raw)
		}
		r := mustFloat(t, strings.TrimSpace(fields[0])) / 255
		g := mustFloat(t, strings.TrimSpace(fields[1])) / 255
		b := mustFloat(t, strings.TrimSpace(fields[2])) / 255
		return cssRGBA{r, g, b, mustFloat(t, strings.TrimSpace(fields[3]))}, true
	}
	t.Fatalf("对比度证人读不懂颜色 %q：新增写法要在这里补解析，不是把断言删掉", raw)
	return cssRGBA{}, false
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%q 不是数字: %v", s, err)
	}
	return f
}

// compositeOver 把半透明前景按 alpha 压在背景上（结果总是 opaque）。
func compositeOver(fg, bg cssRGBA) cssRGBA {
	if fg.a >= 1 {
		return fg
	}
	mix := func(f, b float64) float64 { return f*fg.a + b*(1-fg.a) }
	return cssRGBA{mix(fg.r, bg.r), mix(fg.g, bg.g), mix(fg.b, bg.b), 1}
}

func wcagLuminance(c cssRGBA) float64 {
	lin := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

func wcagContrast(fg, bg cssRGBA) float64 {
	fg = compositeOver(fg, bg)
	l1, l2 := wcagLuminance(fg), wcagLuminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// hslToRGB 是 CSS hsl() 的标准换轴公式（h 取 0..359 的整数度，s/l 是百分数）。
func hslToRGB(h, s, l float64) cssRGBA {
	sf, lf := s/100, l/100
	c := (1 - math.Abs(2*lf-1)) * sf
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := lf - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return cssRGBA{r + m, g + m, b + m, 1}
}

// cssHex 把算完的颜色打回 #rrggbb，让失败信息能被下一个人直接拿去复核。
func cssHex(c cssRGBA) string {
	to255 := func(v float64) int { return int(math.Round(v * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", to255(c.r), to255(c.g), to255(c.b))
}

func mustRender(t *testing.T, s *store.Store, id string) []byte {
	t.Helper()
	p, err := renderer(t, s).ProfilePage(PageRequest{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return p.Body
}

// 两句空态：有书签的档案搜不到东西，不能说"还没有书签"。
// 夹具必须真的有书签——seeded() 建的档案是空的，空档案里两句都会绿。
func TestProfilePage_搜索无结果与没书签是两句话(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm") // dad 有 1 条
	// mom 保持 seeded 的形状（有档案、零书签）：另一句话由它来测。
	r := renderer(t, s)

	empty, err := r.ProfilePage(PageRequest{ID: "dad", Query: "zzzzz不存在"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(empty.Body)
	if !strings.Contains(body, "没有匹配") {
		t.Errorf("搜索无结果要说\"没有匹配\":\n%.200s", body)
	}
	if strings.Contains(body, "这个档案还没有书签") {
		t.Error("有书签的档案搜不到东西，不该说\"还没有书签\"——那是一句谎")
	}
	if !strings.Contains(body, "找到 0 条，这个档案一共 1 条") {
		t.Errorf("搜索态要报出命中数与总数:\n%.300s", body)
	}
	// 反面对照：真的空档案、不带 q，才说那句"还没有书签"。
	blank, err := r.ProfilePage(PageRequest{ID: "mom"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blank.Body), "这个档案还没有书签") {
		t.Errorf("空档案要说没书签:\n%.200s", blank.Body)
	}
	if strings.Contains(string(blank.Body), "没有匹配") {
		t.Error("没搜索却说\"没有匹配\"")
	}
	// 命中态：报数、且只渲染命中那条。
	hit, err := r.ProfilePage(PageRequest{ID: "dad", Query: "dsm"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hit.Body), "找到 1 条，这个档案一共 1 条") {
		t.Errorf("命中态读数不对:\n%.300s", hit.Body)
	}
}

// **这条接手 Task 1 留下的那条指针**："不同 q 绝不互相命中"要等过滤真的进渲染
// 才测得动（Task 1 时两个 q 出的字节相同，而 ETag 是内容哈希，断言它们不同就是
// 断言一个没实现的功能）。现在过滤有了，三条一起钉：
//   - 命中 A 与命中 B 的页面必须有不同 ETag（同 ETag = 缓存里互相命中 = 搜 A 拿到 B）；
//   - 命中 A 与全量页必须不同（搜索态确实改了内容，也顺手证明旁路没把全量页端出来）；
//   - 完全无命中的那个 q 必须与"命中一条"的 q 不同（空结果页与命中页不能长一样）。
func TestProfilePage_不同q不互相命中(t *testing.T) {
	s := seeded(t)
	if err := s.SaveBookmarks("dad", func() *model.Bookmarks {
		b := store.EmptyBookmarks()
		b.Rev = 1
		b.Title = "爸爸的台"
		b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList,
			Items: []model.Item{
				{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
				{ID: "qb", Name: "qBittorrent", URL: "http://10.0.0.3:8181", Alias: []string{}, Tags: []string{}},
			}}}
		return b
	}()); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	a, err := r.ProfilePage(PageRequest{ID: "dad", Query: "群晖"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ProfilePage(PageRequest{ID: "dad", Query: "qBittorrent"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ETag == b.ETag {
		t.Fatalf("搜「群晖」与搜「qBittorrent」拿到同一个 ETag：%q——它们互相命中了", a.ETag)
	}
	// ETag 是**代理**指标，光比它兜不住"接住 q 却不过滤"：搜索框把 q 回填进
	// value，两个不同 q 的页面字节必然不同、ETag 必然不同，不过滤也照样绿
	// （破坏验证实测：exec.go 遍历 b.Groups 时这条全绿）。所以直接钉内容——
	// 搜 A 的页面里不许出现 B 的名字，那才是"搜 A 拿到 B"可见的形状。
	if strings.Contains(string(a.Body), "qBittorrent") {
		t.Error("搜「群晖」的页面里出现了另一条条目：过滤接住了 q 却没作用到渲染")
	}
	if strings.Contains(string(b.Body), "群晖") {
		t.Error("搜「qBittorrent」的页面里出现了另一条条目：过滤接住了 q 却没作用到渲染")
	}
	none, err := r.ProfilePage(PageRequest{ID: "dad", Query: "zzzzz不存在"})
	if err != nil {
		t.Fatal(err)
	}
	if none.ETag == a.ETag {
		t.Error("零命中的页面与命中的页面同 ETag：过滤没真的作用到渲染")
	}
	all, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	if all.ETag == a.ETag {
		t.Error("全量页与搜索页同 ETag：搜索态没改变内容")
	}
	// 同一 q 再来一次仍要稳定（304 那一路的前提）。
	again, err := r.ProfilePage(PageRequest{ID: "dad", Query: "群晖"})
	if err != nil {
		t.Fatal(err)
	}
	if again.ETag != a.ETag {
		t.Errorf("同一个 q 两次渲染 ETag 不稳定：%q vs %q", a.ETag, again.ETag)
	}
}

func TestProfilePage_顶栏有换档案的表单出路(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	body := string(mustRender(t, s, "dad"))
	if !strings.Contains(body, `name="to" value="picker"`) {
		t.Errorf("顶栏缺换档案的出路（§8.1 的切档/退出在零 JS 下只有这一个形状）:\n%.400s", body)
	}
	// 它是表单不是链接：/switch 只收 POST。
	if strings.Contains(body, `href="/switch"`) {
		t.Error("/switch 不许发 GET 链接")
	}
}

// 顶栏那枚「回退」**只在还有可退版本时画**。
//
// §8.1 有一条写在别的功能头上的纪律：「点击没反应的东西不进界面」。回退接进顶栏
// 之后没有跟着走这条：一条书签都没写过的档案 snapshots/ 是空的，点进去只有一句
// "这个档案还没有可回退的版本"——那正是这条纪律说的那种死路。
//
// 两个方向都要钉：零快照时**不许**画（死路），有快照时**必须**画（承诺的入口不能
// 凭空消失——这一条更值钱，它防的是"为了修死路把出口一起删了"）。
func TestProfilePage_回退入口只在有快照时画(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	// SaveBookmarks 不留快照（只有写入路径才留），所以此刻快照为零。
	if snaps, err := s.ListSnapshots("dad"); err != nil {
		t.Fatal(err)
	} else if len(snaps) != 0 {
		t.Fatalf("夹具前提不对：快照 = %v（要零）", snaps)
	}
	if body := string(mustRender(t, s, "dad")); strings.Contains(body, `href="/revert"`) {
		t.Error("零快照的档案上画了「回退」——点进去只有一句「还没有可回退的版本」")
	}
	if _, err := s.WriteSnapshot("dad"); err != nil {
		t.Fatal(err)
	}
	if body := string(mustRender(t, s, "dad")); !strings.Contains(body, `href="/revert"`) {
		t.Error("有快照了却没画「回退」——「改坏了能退回去」那条承诺的入口不能凭空消失")
	}
}

// 侧栏的判据：锚点必须指向 section 已有的 id（一个分组一个），
// 计数必须随命中走——"5 个分组"跟屏幕上实际渲染的内容对不上就是一句谎。
// 计数钉的是**整行形状**而不是裸数字：`>2<` 只证明"页面里有这两个数"，
// 两组计数互换（nas=1、read=2）照样绿。前提写在模板里：页面唯一的裸数字来源是
// <b class="cnt">（顶栏那句是"命中 N 条 · 共 M 条"，数字两侧压着中文与空格；
// 条目只带 --chip-h，而它在属性里不是文本）。
func TestProfilePage_侧栏锚点与计数(t *testing.T) {
	s := seeded(t)
	b := store.EmptyBookmarks()
	b.Rev = 1
	b.Title = "爸爸的台"
	// nas 带合法 accent、read 不带：三件套（分组名 + 计数 + 色点）里
	// "没设色就不画点"这一半，只有这种一个有色一个无色的夹具才量得出来。
	b.Groups = []model.Group{
		{ID: "nas", Title: "NAS", Layout: model.LayoutList, Accent: "#4C8DFF", Items: []model.Item{
			{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
			{ID: "qb", Name: "qBittorrent", URL: "http://10.0.0.3:8181", Alias: []string{}, Tags: []string{}},
		}},
		{ID: "read", Title: "读", Layout: model.LayoutList, Items: []model.Item{
			{ID: "wr", Name: "微信读书", URL: "https://weread.qq.com", Alias: []string{}, Tags: []string{}},
		}},
	}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	all, err := r.ProfilePage(PageRequest{ID: "dad"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(all.Body)
	// 色点在锚点之后、名字之前：整行顺序也是判据的一部分（CSS 靠 flex 序画它）。
	for _, want := range []string{
		`<aside`,
		`data-profile-layout="sidebar"`,
		`href="#g-nas" title="NAS"><i class="dot" style="--sec-accent:#4c8dff" aria-hidden="true"></i><span>NAS</span><b class="cnt">2</b>`,
		`href="#g-read" title="读"><span>读</span><b class="cnt">1</b>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("侧栏缺 %q:\n%.400s", want, body)
		}
	}
	// 色点是设计 §7 三件套的第三件，但"没设色就不画点"：只有 nas 那行该有。
	if n := strings.Count(body, `<i class="dot"`); n != 1 {
		t.Errorf("侧栏色点 %d 个，要恰好 1 个（只有设了合法 accent 的 nas；read 没设就不画）:\n%.400s", n, body)
	}
	// 地标只有一个名字：aside 不带 aria-label，具名导航一页一个（评审 M-6）。
	// 数出现次数比数属性写法更便宜，也更能咬住"嵌套同名地标"那种形状。
	if n := strings.Count(body, `aria-label="分组"`); n != 1 {
		t.Errorf(`aria-label="分组" 出现 %d 次，要恰好 1 次（给 nav，不给 aside）:\n%.400s`, n, body)
	}
	// 带 q 时：计数按命中数，且没命中的分组整条从侧栏消失。
	hit, err := r.ProfilePage(PageRequest{ID: "dad", Query: "群晖"})
	if err != nil {
		t.Fatal(err)
	}
	hb := string(hit.Body)
	if !strings.Contains(hb, `href="#g-nas" title="NAS"><i class="dot" style="--sec-accent:#4c8dff" aria-hidden="true"></i><span>NAS</span><b class="cnt">1</b>`) {
		t.Errorf("搜索态侧栏计数没按命中走:\n%.400s", hb)
	}
	if strings.Contains(hb, `href="#g-read"`) {
		t.Error("一条都没命中的分组还挂在侧栏上——点了跳到空处")
	}
}

// Profile.layout 是档案级形态（spec §8.2/§8.3）：topbar 时分组行常驻顶栏下方，
// **不渲染侧栏**。两个形态都由同一份数据长出来，靠的是 profile.json 那一个字段。
func TestProfilePage_topbar形态没有侧栏(t *testing.T) {
	s := seeded(t)
	if err := s.SaveBookmarks("dad", func() *model.Bookmarks {
		b := store.EmptyBookmarks()
		b.Rev = 1
		b.Groups = []model.Group{{ID: "nas", Title: "NAS", Layout: model.LayoutList,
			Items: []model.Item{{ID: "dsm", Name: "群晖", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}}}}}
		return b
	}()); err != nil {
		t.Fatal(err)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	p.Layout = "topbar"
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	body := string(mustRender(t, s, "dad"))
	if strings.Contains(body, "<aside") {
		t.Errorf("layout=topbar 还渲染侧栏:\n%.400s", body)
	}
	if !strings.Contains(body, `href="#g-nas"`) {
		t.Error("topbar 形态的分组行必须在顶栏下方——锚点一个都不能少")
	}
	// 形态属性要发出去，CSS 与实拍才认得这一页是哪种形态（同 data-layout 的先例）。
	if !strings.Contains(body, `data-profile-layout="topbar"`) {
		t.Error("body 要发 data-profile-layout")
	}
	// 具名导航地标一页只有一个：topbar 档给那行 nav，跟 sidebar 档同一条判据（评审 M-6）。
	if n := strings.Count(body, `aria-label="分组"`); n != 1 {
		t.Errorf(`aria-label="分组" 出现 %d 次，要恰好 1 次（topbar 的那一行 nav 也必须带名字）:\n%.400s`, n, body)
	}
}

// 抽屉用原生 <details>：键盘开合与可及性免费，不值得为它写 JS 或加 checkbox 黑话。
func TestProfilePage_侧栏抽屉是原生details(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的台", "dsm")
	body := string(mustRender(t, s, "dad"))
	if !strings.Contains(body, "<details") || !strings.Contains(body, "<summary>") {
		t.Errorf("侧栏抽屉必须是 <details>/<summary>:\n%.400s", body)
	}
	// 抽屉不许带脚本来开合：`strings.Contains(body, "<script")` 在 ⑤ 之后量的是
	// **整页**，而整页从此有一个应用脚本——于是这条断言会在 ⑤ 落地时红，���它红的
	// 理由跟抽屉无关。真正要守的是"**这个 details 元素上没有**开合脚本"，
	// 判据因此落到那一段上。
	seg := detailsSegment(t, body)
	if strings.Contains(seg, "<script") || strings.Contains(seg, "onclick") || strings.Contains(seg, "on toggle") {
		t.Errorf("抽屉不许带脚本来开合：\n%.400s", seg)
	}
}

// detailsSegment 取出 <details>…</details> 那一段（含尾部标签）。
func detailsSegment(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, "<details")
	if i < 0 {
		t.Fatal("这一页里没有 <details>")
	}
	j := strings.Index(body[i:], "</details>")
	if j < 0 {
		t.Fatal("抽屉的 <details> 没关上")
	}
	return body[i : i+j+len("</details>")]
}

// 两层文案的形状钉。<h1> 里不许出现文件名：那是给家属看的第一句，
// "bookmarks.json 第 3 行附近"不是人话。原文必须在 <details> 里可见：
// 修文件的人就是家长本人，行号是唯一能定位的东西。
func TestBrokenPage_主句是人话原文在details里(t *testing.T) {
	s := seeded(t)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{\n \"rev\": 2,\n")); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	if _, err := r.ProfilePage(PageRequest{ID: "dad"}); err == nil {
		t.Fatal("坏档案该让 ProfilePage 报错")
	}
	reason, ok := s.BrokenReason("dad")
	if !ok || reason == "" {
		t.Fatalf("坏档登记表里没有原因: %v", ok)
	}
	page, err := r.BrokenPage("dad", reason)
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	start := strings.Index(body, "<h1>")
	end := strings.Index(body, "</h1>")
	if start < 0 || end < start {
		t.Fatalf("解释页要有 <h1> 主句:\n%.300s", body)
	}
	h1 := body[start:end]
	if strings.Contains(h1, ".json") || strings.Contains(h1, "第 ") {
		t.Errorf("主句里混进了技术细节（文件名/行号该收进 details）:\n%s", h1)
	}
	if !strings.Contains(h1, "这个档案的数据文件读不了") {
		t.Errorf("主句要是那句人话:\n%s", h1)
	}
	if !strings.Contains(body, `<details class="tech"`) || !strings.Contains(body, "<summary>") {
		t.Error("原文要收进带 .tech 类的 <details>：裸元素写法会把形状漏给侧栏抽屉，两个页面本来不该互相继承")
	}
	// 包含关系要单独钉，而且钉的是"原文就在这只 .tech 块里"：
	// ①只证"串在场"的两条互不认识，"块空着、原文印在别处"能静悄悄进页面；
	// ②按整页偏移量比（第一个 </details>、最后一个 <pre>）会两头出错——块外多一只
	//   合法 pre 就假红，块里留个空 pre、原文印到块外反而假绿。
	// 所以先把窗口锚死到那只块本身（从它的开标签到它自己的闭标签），再在窗口里问
	// "有 pre 吗、有转义后的原文吗"。证人也不靠注释措辞活着：不数整页的 <pre>。
	d := strings.Index(body, `<details class="tech"`)
	e := strings.Index(body[d:], "</details>")
	if d < 0 || e < 0 {
		t.Fatalf("那只 .tech 块得是闭合的（上面已报形状问题，这里不再往下切）: details=%d /details=%d", d, e)
	}
	seg := body[d : d+e]
	escaped := string(template.HTMLEscapeString(reason))
	if !strings.Contains(seg, "<pre") || !strings.Contains(seg, escaped) {
		t.Errorf("store 的原文必须折叠在 .tech 的 details 里、且就在 pre 内:\n%.400s", seg)
	}
	// 逐字：store 给的那句必须原样出现在页面上，不改写、不截断。比的是页面
	// 真正的形状——模板出去的是 HTML 转义后的字节；校验类原文里带 %q 引号
	// （check.go 的 `search default "x" …`），裸比在那类夹具下会假红。
	if !strings.Contains(body, string(template.HTMLEscapeString(reason))) {
		t.Errorf("store 的原文没原样出现:\n要: %s\n页面上:\n%.600s", reason, body)
	}
	if !strings.Contains(body, "bookmarks.json") || !strings.Contains(body, "第 ") {
		t.Error("文件名与行号不许丢（§12：报得出位置）")
	}
	if strings.Contains(body, "<script") {
		t.Error("解释页同样不许带脚本")
	}

	// 转义这一步自己也要有证人（评审 R-6）。上面比的都带 HTMLEscapeString，但今天
	// 这句夹具原文里没有需要转义的字符——escaped 与 raw 是同一个串，把模板的转义
	// 摘掉（改成拿 template.HTML 递进来）这条测试照样绿。这里再拿一句**带引号与
	// 尖括号**的原文渲染一遍：这两份从此是两个串，谁把转义去掉，raw 那份就出现在
	// 页面上、escaped 那份反而找不到，两条断言同时红。
	// 端到端那一半（store 的真原文进 503 与页面）由 httpapi 的
	// Test坏档原文同源_校验类走登记表回退 带着——它那句里有真引号。
	rawQuote := `bookmarks.json 第 3 行附近: 字段 "name" 的值 <b> 不是字符串`
	if got := string(template.HTMLEscapeString(rawQuote)); got == rawQuote {
		t.Fatal("夹具原文不含需要转义的字符，下面这两条证人是空的")
	}
	qp, err := r.BrokenPage("dad", rawQuote)
	if err != nil {
		t.Fatal(err)
	}
	qbody := string(qp.Body)
	if want := string(template.HTMLEscapeString(rawQuote)); !strings.Contains(qbody, want) {
		t.Errorf("带引号的原文没按转义后的形状进页面:\n要: %s\n页面:\n%.600s", want, qbody)
	}
	if strings.Contains(qbody, rawQuote) {
		t.Error("store 那句话未经转义就进了页面：解释页把它当 HTML 用了")
	}
}

// 坏档页不许把磁盘路径吐出去：原因文本用的是 filepath.Base，
// 页面这一层再拼一次路径就是把内部结构交给看页面的人。
func TestBrokenPage_没有磁盘路径(t *testing.T) {
	s := seeded(t)
	if err := s.WriteFileAtomic(s.BookmarksPath("dad"), []byte("{坏")); err != nil {
		t.Fatal(err)
	}
	r := renderer(t, s)
	_, _ = r.ProfilePage(PageRequest{ID: "dad"})
	reason, _ := s.BrokenReason("dad")
	page, perr := r.BrokenPage("dad", reason)
	if perr != nil {
		t.Fatal(perr)
	}
	if strings.Contains(string(page.Body), s.Root()) || strings.Contains(string(page.Body), "profiles/") {
		t.Errorf("解释页漏出磁盘路径:\n%.400s", page.Body)
	}
}

// 管理页的建档表单与提示（形状层面；行为在 httpapi 那侧）。notice 两支都要钉：
// 给了要画出来（失败不吭声的表单等于没告诉管理员刚才没建上），没给不许画空元素。
//
// 2026-10-12：建档表单**只剩两格**（名字 + PIN）。原来那三格里的「能管理全家」
// 勾选随 admin 属性一起删掉了——管理员是那把机器级的 LAUNCHBAY_ADMIN_TOKEN，
// 而建档请求体现在连 admin 这一格都不收。留着勾选框会让人以为勾了就能管这台机器。
// 所以这一条现在钉的是**它不在**：钉过的字段不存在，见下。
func TestAdminPage_建档表单与提示在场(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	page, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸"}}, "PIN 要 4-6 位数字。")
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	for _, want := range []string{`action="/admin"`, `name="title"`, `name="pin"`} {
		if !strings.Contains(body, want) {
			t.Errorf("管理页建档形状缺 %q:\n%.400s", want, body)
		}
	}
	// 「建成管理员」那一格钉的字段已不存在（2026-10-12，`AdminRow.Admin` /
	// `model.Profile.Admin` / 建档请求体的 admin 一起删了），所以断言翻面：
	// 表单里**不许**再有那个勾选。
	//
	// 2026-10-12 又补了一条：建档表单现在有一个 `nopass` 勾选框（不设 PIN），
	// 所以这里不能再拿"页面里不许出现 checkbox"当判据——那会连合法的那个一起
	// 咬掉。判据收窄回**被删掉的那一格**：字段名与文案，两个都钉。
	if strings.Contains(body, `name="admin"`) || strings.Contains(body, `能管理全家`) {
		t.Errorf("建档表单里不许还有「建成管理员」的勾选（2026-10-12 起管理员是一把机器级 key）:\n%.400s", body)
	}
	// PIN 选填的那一格必须在场：不设防是一个**被说出来**的决定，所以它长得
	// 像一个决定（有勾选框、有说明），而不是"忘了填"（那与空 PIN 长得一样）。
	if !strings.Contains(body, `name="nopass"`) {
		t.Errorf("建档表单缺「不设 PIN」那一格:\n%.400s", body)
	}
	if !strings.Contains(body, `class="notice"`) {
		t.Errorf("给了 notice 就该画出来:\n%.400s", body)
	}
	// 表单自己带的两个类各有职责，少一个都静默：`newprofile` 是这张表单的样式钩子
	// （CSS 那条"勾选不吃文本框的框"的证人按它判），`adm` 是**按钮族**的接法——
	// `.adm button{…min-height:44px}` 才是「新建档案」那颗按钮的 44 与边框来源。
	// 实测：把 `adm` 从 class 里去掉，全套测试照样绿，而按钮掉出族外。
	if cl := formClasses(t, body, "/admin"); !cl["adm"] || !cl["newprofile"] {
		t.Errorf("建档表单的 class 要同时含 adm（按钮族的 44px 由它接）与 newprofile，得到 %v", cl)
	}
	// 管理员的边界必须**在这一页上**说清（设计 §6）：他进得来这一页，但看不见、
	// 也改不了任何人的书签。
	//
	// 这一句原本钉的是勾选那一行——**2026-10-12 那个勾选随 admin 属性一起删掉了**，
	// 所以判据落到同一页唯一还讲这件事的地方：admin.count（「看不到任何人存了什么」）。
	// 保留的理由与上面那条一样：这一页是唯一会出现"管理员"这个身份的地方。
	if !strings.Contains(body, `看不到任何人存了什么`) {
		t.Error("这一页没说清管理员**看不到**任何人的书签：能管这台机器不等于能读别人的东西")
	}
	if strings.Contains(body, `可以看与改所有人的书签`) {
		t.Error("这一页还在承诺 2026-10-10 已下线的能力（管理员只管账号，不碰内容）")
	}
	// 「正在创建口令」这一格不许让浏览器提示保存密码——这条纪律在进门页两形状上
	// 早就钉着了，缺的是覆盖面：管理页这张表单是同一条族里的第三格，两席复审都抓到
	// 它没接证人。判据不新写一个，只把已有的 helper 喂给它该喂的那一页。
	if n := strings.Count(assertPINsNoSave(t, body), "password"); n != 1 {
		t.Errorf("管理页建档该恰好一格 PIN，得到 %d 格", n)
	}
	plain, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain.Body), `class="notice"`) {
		t.Errorf("没有提示时不该渲染提示元素:\n%.200s", plain.Body)
	}
}

// pickerStore 是进门页的档案夹具：只关心"有哪些档案、哪份读不动"。assets(t) 那条缝
// 自带 dad 且不跟着我的 store 走，而这一支的形状必须由我自己造出来的坏档决定。
func pickerStore(t *testing.T, ids ...string) *store.Store {
	t.Helper()
	s := store.New(t.TempDir())
	for _, id := range ids {
		if _, err := s.CreateProfile(store.NewProfileInput{
			ID: id, Title: "档案" + id, PinHash: "sha256$a$b"}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func pickerRenderer(t *testing.T, s *store.Store) *Renderer {
	t.Helper()
	ix, _, _ := assets(t)
	fc, err := s.LoadIconCache()
	if err != nil {
		t.Fatal(err)
	}
	return New(s, ix, fc)
}

// breakProfile 把 profile.json 写成读不动的字节。进门页"一张卡都列不出来"这一支只有
// 这一条路走得到：bookmarks.json 坏掉的人照样有卡，他的解释在点进去之后那一页。
func breakProfile(t *testing.T, s *store.Store, id string) {
	t.Helper()
	if err := s.WriteFileAtomic(s.ProfilePath(id), []byte("坏得读不了")); err != nil {
		t.Fatal(err)
	}
}

// pickerBodyOf 按默认选项渲染进门页（≥2 那一支：不递标题，走渲染器的兜底问句）。
func pickerBodyOf(t *testing.T, s *store.Store) string {
	t.Helper()
	page, err := pickerRenderer(t, s).PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return string(page.Body)
}

// noticeOf 切出那句主句（`<p class="notice">…</p>`），用来比对坏一份与坏多份时的措辞。
func noticeOf(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `<p class="notice">`)
	if i < 0 {
		t.Fatal("页面里没有 .notice 那句主句：说明段整个没了")
	}
	j := strings.Index(body[i:], "</p>")
	if j < 0 {
		t.Fatal(".notice 没闭合：切不出主句了")
	}
	return body[i : i+j+4]
}

func TestPickerPage_进不去的档案要点名(t *testing.T) {
	s := pickerStore(t, "dad", "mom")
	breakProfile(t, s, "mom")
	r := pickerRenderer(t, s)
	page, err := r.PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	// 坏掉的那份只能以 id 点名：标题就在读不动的那个文件里，编一个是假话。
	if !strings.Contains(body, "<code>mom</code>") {
		t.Errorf("坏档没点名（要给 <code>mom</code>）:\n%.700s", body)
	}
	if !strings.Contains(body, "数据文件读不了") {
		t.Errorf("没说出为什么进不去:\n%.700s", body)
	}
	// 出路那句必须说"在这台机器上修"：产品里没有页面内的修法，给个按钮就是假出口。
	if !strings.Contains(body, "要在这台机器上修好那个文件") {
		t.Errorf("出路没说实话:\n%.700s", body)
	}
	// 形状：一行小字，不是卡片。卡片这张脸在说"可以进来"，而它进不去——实拍里画成
	// 卡片时它看起来像一张坏掉的卡（大片空白 + 一行小字），比"这一份坏了"更像页面出错。
	if strings.Contains(body, `<li class="plain">`) || strings.Contains(body, "<li><code>mom</code>") {
		t.Errorf("坏档被画成了卡片形状（它进不去，不该占一张卡）:\n%.700s", body)
	}
	if !strings.Contains(body, `<p class="hint">进不去的档案：<code>mom</code></p>`) {
		t.Errorf("坏档那一行没带上它自己的标签（光一行 id 会像隔壁卡片的注脚）:\n%.700s", body)
	}
	// 措辞要说准：坏一份与坏三份的主句必须一字不差。这是结构判据——禁某个词（"这几份"）
	// 换个写法就绕过，而单档时说"这几份"仍然是说谎。
	one := noticeOf(t, body)
	m := pickerStore(t, "dad", "a", "b", "c")
	for _, id := range []string{"a", "b", "c"} {
		breakProfile(t, m, id)
	}
	many := pickerBodyOf(t, m)
	if got := noticeOf(t, many); got != one {
		t.Errorf("主句随坏档数量变脸：坏一份是 %q，坏三份是 %q", one, got)
	}
	// 三份都要点到，一份都不许漏（点名行不是只画第一个的示例）。
	if n := strings.Count(many, "<code>"); n != 3 {
		t.Errorf("坏三份时页面点了 %d 个名字，要 3 个:\n%.700s", n, many)
	}
	// 正对照：能进的那份照旧一张卡，说明段不许把它挤掉。
	if n := strings.Count(body, `action="/pin"`); n != 1 {
		t.Errorf("页面有 %d 张 PIN 卡，要恰好 1 张（dad 读得动）", n)
	}
}

func TestPickerPage_全都进不去时不许只剩空列表(t *testing.T) {
	s := pickerStore(t, "dad")
	breakProfile(t, s, "dad")
	r := pickerRenderer(t, s)
	page, err := r.PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	if !strings.Contains(body, "<code>dad</code>") || !strings.Contains(body, "数据文件读不了") {
		t.Errorf("唯一档案读不动时这页必须说出是哪一份、为什么:\n%.700s", body)
	}
	// 0 张卡是这个形状的定义（PIN 根本无从验证），不是待修的缺陷。这一行防的是
	// "顺手补一张点进去必然失败的卡"——那会把死路换成更响的死路。
	if n := strings.Count(body, `action="/pin"`); n != 0 {
		t.Errorf("读不动的档案发出了 %d 张 PIN 卡，要 0 张", n)
	}
}

func TestPickerPage_半件事目录不编名字(t *testing.T) {
	// 有目录但没有 profile.json：store 有意**不**把它记成坏档（一个手滑 mkdir 出来的
	// 空目录不该在家里页面上挂红字，见 store.ListProfiles 里那支 ErrNotFound）。
	// 于是这一支既没卡也点不出名——它只得到那句总的。
	s := store.New(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(s.ProfilePath("ghost")), 0o755); err != nil {
		t.Fatal(err)
	}
	r := pickerRenderer(t, s)
	page, err := r.PickerPage(PickerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	if !strings.Contains(body, "这台导航台现在没有进得去的档案") {
		t.Errorf("零张卡却什么都不说（这正是要闭的那处死路）:\n%.700s", body)
	}
	if strings.Contains(body, "数据文件读不了") {
		t.Errorf("半件事目录挂上了坏档红字：与 store 的登记口径不一致:\n%.700s", body)
	}
	if strings.Contains(body, "ghost") {
		t.Errorf("还不确认那算不算一份档案，就不许把目录名当档案名印出来:\n%.700s", body)
	}
}

// **带 ?add= 的请求不进渲染缓存**（bookmarklet 那一族，spec §6.3）。
//
// 理由比 ?q= 更硬：?q= 漏进去只是键空间膨胀，而 ?add= 漏进去会让**上一条待添加的
// 地址留在缓存里**——下一个打开首页的人看到的是别人 bookmarklet 带过来的东西。
// 它是本设计里最贵的一类 bug（串号），而症状是"偶尔看到一条不属于自己的书签"，
// 极难往缓存上想。
func TestProfilePage_带add的请求不进渲染缓存(t *testing.T) {
	ix, fc, s := assets(t)
	if err := s.SaveBookmarks("dad", realistic(t, 25)); err != nil {
		t.Fatal(err)
	}
	r := New(s, ix, fc)
	base := PageRequest{ID: "dad"}

	first, err := r.ProfilePage(base)
	if err != nil {
		t.Fatal(err)
	}
	withAdd := base
	withAdd.Add = "http://10.0.0.3:5000/别人的待添加"
	second, err := r.ProfilePage(withAdd)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Body) == string(second.Body) {
		t.Error("带 add 的页面与不带 add 的字节相同——它命中了同一个缓存条目，add 根本没进渲染")
	}
	if !strings.Contains(string(second.Body), "别人的待添加") {
		t.Error("带 add 的那一页里没有 add")
	}
	// 带 add 的那一页**不许**污染之后的不带 add 的那一页
	third, err := r.ProfilePage(base)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(third.Body), "别人的待添加") {
		t.Error("带 add 的请求污染了之后的普通首屏——上一个 bookmarklet 的地址留在了别人的页面上")
	}
}

// 层叠顺序是判据：`.act button` 与 `button.danger` 权重相同，谁写在后面谁赢。
// 把 danger 那条挪到联合规则**之前**，确认页那颗实心红键就会静默变灰——
// 没有任何功能测试会红，只有肉眼看得出来。所以这里用肉眼之外的东西钉住位置。
func TestCSS_危险键的填充写在次级按钮族之后(t *testing.T) {
	raw, err := webFS.ReadFile("web/base.css")
	if err != nil {
		t.Fatal(err)
	}
	css := stripCSSComments(string(raw))
	// 锚点跟着按钮族**末尾那一段**走：先是 `.langsel`，同日第二批又追加了 `.rename`
	// （见 硬规格在场 那条逐字串的注释）。这里找的是这条规则自身的位置，所以锚在
	// 它的最后一段选择器上：`.langsel button{` 已不再是末端，`.rename button{` 才是。
	joint := strings.Index(css, ".rename button{")
	danger := strings.Index(css, "button.danger{")
	if joint < 0 || danger < 0 {
		t.Fatalf("找不到要对比的两条规则（joint=%d danger=%d）：改名可以，但要连这条证人一起改", joint, danger)
	}
	if danger < joint {
		t.Errorf("button.danger 写在 .act button 之前（%d < %d）：等权重下按顺序盖，确认页的红键会变灰", danger, joint)
	}
}
