package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/store"
)

// setupZero 是零档案安装：同一个 newDeps，一个档案都不建。
func setupZero(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	s := store.New(t.TempDir())
	scopes, err := auth.ParseScopes("ai-dad:tok-dad:dad")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(newDeps(t, s, scopes))
	return s, srv.Handler()
}

func TestHome_零档案画引导页(t *testing.T) {
	_, h := setupZero(t)
	rec := get(h, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("零档案首屏该是 200 的引导页，得到 %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="/setup"`) {
		t.Errorf("零档案首屏要画引导表单，不该是空列表死路:\n%.400s", body)
	}
	if strings.Contains(body, "谁来用") {
		t.Error("一台还没装好的机器不该问「谁来用？」")
	}
}

func TestHome_单档案不问谁来用(t *testing.T) {
	s, h := setupZero(t)
	if _, err := s.CreateProfile(store.NewProfileInput{
		ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	body := get(h, "/", "").Body.String()
	if strings.Contains(body, "谁来用") {
		t.Errorf("只有一个档案还要回答「谁来用？」就是这个片要修的默认值:\n%.300s", body)
	}
	if !strings.Contains(body, "<h1>爸爸</h1>") {
		t.Errorf("单人进门页的 h1 要直接是那个人的名字:\n%.300s", body)
	}
	if n := strings.Count(body, `action="/pin"`); n != 1 {
		t.Errorf("恰好一张 PIN 卡片，得到 %d", n)
	}
}

func TestHome_两档案形状不变(t *testing.T) {
	_, h := setup(t) // setup 的夹具是 dad + kid 两个档案
	body := get(h, "/", "").Body.String()
	if !strings.Contains(body, "<h1>谁来用？</h1>") {
		t.Errorf("≥2 的形状要逐字保持:\n%.300s", body)
	}
	if n := strings.Count(body, `action="/pin"`); n != 2 {
		t.Errorf("两张卡片要两个 /pin 表单，得到 %d", n)
	}
	if strings.Contains(body, `action="/setup"`) {
		t.Error("有档案的页面不许画引导表单")
	}
}

// readGoFile 是源码级证人的取数入口（不是给人读的测试数据）：读的是**源码文本**，
// .go 与 .tmpl 都走它——名字里的 Go 指"源码"而不是扩展名。
func readGoFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", name, err)
	}
	return string(raw)
}

func dirCount(t *testing.T, s *store.Store) int {
	t.Helper()
	n, err := s.CountProfiles()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// 引导建档：设计 §3、§9 的 2/3/4/5 条。
//
// **第一个档案不再是管理员**（2026-10-12）：管理员是 docker-compose.yml 里那把
// LAUNCHBAY_ADMIN_TOKEN，与档案无关。建出来的就是第一个人的书签页，而"装完没人
// 能加人"那个老问题也不再成立——管理人一直在机器上，不在任何一份档案上。
func TestSetup_零档案建出普通档案(t *testing.T) {
	s, h := setupZero(t)
	rec := postForm(h, "/setup", "title=%E7%88%B8%E7%88%B8&pin=1234", "") // 爸爸
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("建成要 303 回 /，得到 %d Location=%q", rec.Code, rec.Header().Get("Location"))
	}
	p, err := s.LoadProfile("home") // 中文名 → 基名序列（Task 1）
	if err != nil {
		t.Fatalf("引导建档没落盘: %v", err)
	}
	if !strings.HasPrefix(p.PIN, "sha256$") {
		t.Errorf("PIN 要经 HashPIN 存成三段式，得到 %q", p.PIN)
	}
}

// 这条是防回归证人而不是驱动实现的证人（骨架阶段它就是绿的，见 task-4-report.md
// 的 RED 记账表）：实现最容易顺手写错的地方正是"建完直接登录"。
func TestSetup_成功不发会话(t *testing.T) {
	_, h := setupZero(t)
	rec := postForm(h, "/setup", "title=dad&pin=1234", "")
	for _, c := range rec.Result().Cookies() {
		if c.Name == "lb_profile" || c.Name == "lb_home" {
			t.Fatalf("引导成功发了会话 cookie %s=%s：免令牌建档会升级成免令牌进入", c.Name, c.Value)
		}
	}
}

func TestSetup_建完这条通路自己关(t *testing.T) {
	s, h := setupZero(t)
	if _, err := s.CreateProfile(store.NewProfileInput{
		ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	before := dirCount(t, s)
	rec := postForm(h, "/setup", "title=%E5%A6%88%E5%A6%88&pin=5678", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("窗口已关要 303，得到 %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("拒的时候不解释：Location 要恰好是 /，得到 %q（带 ?err= 就是枚举线索）", loc)
	}
	if got := dirCount(t, s); got != before {
		t.Errorf("被拒的提交在磁盘上留下了东西：%d 个档案目录，要 %d", got, before)
	}
}

func TestSetup_字段错说人话(t *testing.T) {
	_, h := setupZero(t)
	for body, want := range map[string]string{
		"title=&pin=1234":        "noname",
		"title=&pin=":            "noname", // 两格都没填对：先说名字那一格
		"title=%E7%88%B8&pin=12": "badpin",
		"title=dad&pin=abc12":    "badpin",
		"title=dad":              "badpin",
		"pin=1234":               "noname",
	} {
		rec := postForm(h, "/setup", body, "")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%q 要 303，得到 %d", body, rec.Code)
		}
		loc := rec.Header().Get("Location")
		u, err := url.Parse(loc)
		if err != nil || u.Path != "/" {
			t.Errorf("%q 的 Location = %q，要回 /", body, loc)
			continue
		}
		q := u.Query()
		if q.Get("err") != want {
			t.Errorf("%q 的 Location = %q，要 err=%s", body, loc, want)
		}
		if q.Get("new") != "" {
			t.Errorf("%q 的 Location 带了 new=1：引导页不是 /new，折着的那一块不该展开", body)
		}
		// **名字要带回来**（2026-10-24）：PIN 错了只改 PIN，名字没了的下一发
		// 会变成 noname。PIN 本身绝不回填（它是凭证）。
		src, _ := url.ParseQuery(body)
		if got, wantTitle := q.Get("title"), strings.TrimSpace(src.Get("title")); got != wantTitle {
			t.Errorf("%q 的 Location 里 title = %q，要 %q", body, got, wantTitle)
		}
		if title := q.Get("title"); title != "" {
			page := get(h, loc, "")
			if !strings.Contains(page.Body.String(), `value="`+title+`"`) {
				t.Errorf("回跳后的引导页没有把名字 %q 填回去:\n%.500s", title, page.Body.String())
			}
		}
	}
}

// 白名单纪律：页面只能出现表里登记过的句子。用户可控原文一个字都不许进页面。
func TestSetup_提示只走白名单(t *testing.T) {
	for _, k := range []string{"noname", "badpin", "permission"} {
		if noticeOf(i18n.Default, k) == "" {
			t.Errorf("pinNotices 缺键 %q：引导页会说不出话，或者更糟——开始复读输入", k)
		}
	}
	_, h := setupZero(t)
	rec := get(h, "/?err=%E4%B8%8D%E6%98%AF%E9%94%AE", "") // ?err=不是键
	if strings.Contains(rec.Body.String(), "不是键") {
		t.Error("未登记的 ?err= 值被复读进页面了（反射点）")
	}
	// 正对照：登记过的键**会**进页面，否则上面那条 Contains 是空转。
	if !strings.Contains(get(h, "/?err=noname", "").Body.String(), noticeOf(i18n.Default, "noname")) {
		t.Error("登记的键 noname 没把句子印出来——白名单表与页面断了两截")
	}
}

// "判据只有一份"在引导这条路上的形状：store 锁内那一次是真的，handler 里
// **不该有第二次**——数两次会得出两个答案，而"谁有资格写"不能有第二个来源。
// 源码级断言（不是行为断言），因为 handler 多查一次**不改行为**：
// 行为测试对这种复制粘贴完全无感（P1-④ 评审的同一课）。
func TestSetup_handler不数档案(t *testing.T) {
	// 名单按**函数**推：谁是建档壳由"体内有没有 `.createProfile(`"决定。老版本按文件推
	// 名单并整文件排除 handlers.go（理由是 handleRoot 合法地要数档案），那一刀把
	// handleCreateProfile 这个真壳一起排除了——往 API 壳里塞第二次 CountProfiles、
	// 或给它递一个 FirstOnly，这条闸都看不见。按函数筛就不需要点名任何人。
	// 顺带一条：取数从"读原始文件"换成 AST 函数体，注释里提到 CountProfiles 不再假红
	// （注释不是代码，红在注释上只会教人改措辞）。
	funcs := packageFuncs(t)
	var shells, first []string
	for _, g := range funcs {
		if strings.Contains(g.body, "FirstOnly:true") {
			first = append(first, g.display())
		}
		if !strings.Contains(g.body, ".createProfile(") {
			continue
		}
		shells = append(shells, g.display())
		if n := strings.Count(g.body, "CountProfiles"); n != 0 {
			t.Errorf("%s 里出现了 %d 次 CountProfiles：引导判据被抄进壳，成了第二处（判据只有 store 锁内那一次）",
				g.display(), n)
		}
	}
	sort.Strings(shells)
	sort.Strings(first)
	// 正对照：一条都没匹配上就是形状错了（那时上面每个断言都对空集合成立，是假绿）。
	if len(shells) < 2 {
		t.Fatalf("只认出 %v 个调 createProfile 的函数：至少该有 API 与两张表单三条壳——匹配形状漂了", shells)
	}
	// 递 FirstOnly 的函数整包恰好一个：窗口判据只对"建第一个"那条路有意义。
	// 0 个 = 引导那条不再要求窗口内成立（store 的重查成了死码）；2 个 = 第二条路也开始
	// 要"第一个"。管理页带上它建不成第二个人那一半有行为证人，这条钉的是"不许有两个来源"。
	if len(first) != 1 {
		t.Errorf("递 FirstOnly 的函数应当恰好一个，得到 %v（窗口在锁内成立这件事只能由 store 判，"+
			"而只有 /setup 那条路该说要第一个）", first)
	}
}

// errKeyLit 抓一次调用实参里的字面量键（`redirectPinFail(w, r, "pin")` → pin）。
var errKeyLit = regexp.MustCompile(`"([^"]+)"`)

// errKeyReturn 抓键来源函数可能交出的键（它的返回值最终进的就是那些 ?err= 重定向）。
// **不带空格**：packageFuncs 把函数体的空白整个抹掉，`return "badpin"` 在那里长这样
// `return"badpin"`。按带空格的写法扫，这一半永远收不到东西而没人发现（枪单 G5 打出来的）。
var errKeyReturn = regexp.MustCompile(`return"([^"]+)"`)

// errNestedCall 抓实参里嵌套的那次调用（`redirectSetup(w, setupFieldKey(...))` → setupFieldKey）。
var errNestedCall = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\(`)

// callArgs 按括号配对取出 body 里每一次 `name(...)` 的实参文本。
// 不用正则切 `[^)]*`：嵌套调用（`setupFieldKey(res.FieldErrs)`）会让第一个 `)` 停在里层。
func callArgs(body, name string) []string {
	var out []string
	hay := name + "("
	for i := 0; ; {
		j := strings.Index(body[i:], hay)
		if j < 0 {
			return out
		}
		start := i + j + len(hay)
		depth, end := 1, -1
		for k := start; k < len(body); k++ {
			switch body[k] {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 {
				end = k
				break
			}
		}
		if end < 0 {
			return out
		}
		out = append(out, body[start:end])
		i = end
	}
}

// 发给页面的键必须都在白名单里。这条只能扫源码：未登记的键在**行为上**就是
// "页面什么都不印"，你打一遍页面永远测不出来——表单失败了而人不说话，
// 正是 TestAdmin_提示在场 与 TestSetup_字段错说人话 两条证人存在的全部理由。
func Test提示键只发白名单里有的(t *testing.T) {
	funcs := packageFuncs(t)
	// 发件人名单是**推出来的**：体内带 `?err=` 的那条函数就在往 Location 里塞键。
	// 老版本手抄 `{setup.go, adminpage.go}` 两条文件与 `redirect(?:Setup|AdminFail)`
	// 两个名字，而同一次提交刚刚才把另一条闸的手抄名单换成推导——被漏掉的是
	// `redirectPinFail`（进门页那五种失败，7 个调用点）：那五把键今天碰巧都在表里，
	// 靠的是没人动过，不是有人看着。
	byName := map[string]string{}
	senders := []string{}
	for _, g := range funcs {
		byName[g.name] = g.body
		if strings.Contains(g.body, "?err=") && !slices.Contains(senders, g.name) {
			senders = append(senders, g.name)
		}
	}
	sort.Strings(senders)
	if len(senders) < 2 {
		t.Fatalf("只认出 %v 这几个发 ?err= 的函数：一条闸认不出三个发件人，说明取数形状漂了", senders)
	}
	found := map[string]bool{}
	providers := map[string]bool{}
	for _, name := range senders {
		for _, g := range funcs {
			for _, arg := range callArgs(g.body, name) {
				lits := errKeyLit.FindAllStringSubmatch(arg, -1)
				nested := errNestedCall.FindAllStringSubmatch(arg, -1)
				if len(lits) == 0 && len(nested) == 0 {
					// 判据宁可红也不肯静默放行：键从一个看不见的来源进来（变量、字段、
					// 别的包的常量），这条闸就断在那一环。补法是把键写成字面量，或让
					// 来源函数自己 return 字面量。
					t.Errorf("%s 调用 %s 的键来源看不见：%s——未登记的键到这里就没人拦了",
						g.display(), name, arg)
					continue
				}
				for _, m := range lits {
					found[m[1]] = true
				}
				for _, m := range nested {
					providers[m[1]] = true
				}
			}
		}
	}
	// 键来源函数（setupFieldKey 这一族）交出的也是键：逐个收它 return 的字面量。
	// 名单同样是从调用点推出来的，不点名任何人。
	viaProvider := map[string]bool{}
	for p := range providers {
		body, ok := byName[p]
		if !ok {
			continue // 别的包的函数（如 http.Redirect）不产键
		}
		for _, m := range errKeyReturn.FindAllStringSubmatch(body, -1) {
			found[m[1]] = true
			viaProvider[m[1]] = true
		}
	}
	// 这一半单独配正对照：整包字面量收得多不代表嵌套那条路通着——正则一旦与抹过空白的
	// 函数体对不上，keys 照样从直接调用点收满，而 setupFieldKey 交出的键全体隐身。
	if len(providers) == 0 || len(viaProvider) == 0 {
		t.Fatalf("认出 %d 个键来源函数、经它们收到 %d 把键：这一半正在空转（推导或正则形状漂了）",
			len(providers), len(viaProvider))
	}
	if len(found) < 2 {
		t.Fatalf("只认出 %d 个键（%v）：正则与源码形状对不上了", len(found), found)
	}
	for key := range found {
		if noticeOf(i18n.Default, key) == "" {
			t.Errorf("键 %q 被发进 ?err= 却不在 pinNotices 里：那条失败到页面上就是一个字都不说", key)
		}
	}
}

// 「谁来用？」这句问句只许有一份。整枝评审 M3 删掉了 httpapi 里那份硬编码，但删完没人
// 看着：把它加回 handleRoot，渲染结果一字不差，行为证人永远看不见——想钉它只能扫源码。
// **2026-10-13 改判后的形状**：≥2 那一支**也显式递标题**，但递的是**语言包里那一句**
// （`s.t(r, "picker.heading.many")`），不是这里写死的中文串；渲染器里那份降级成兜底。
// 于是这条闸今天守的是同一件事的另一头：**httpapi 侧不许出现写死的问句**。
// want=0 的那一半必须配同支正对照，否则"整包确实没有"与"扫描器坏了"分不出来。
func Test谁来用问句只在渲染器里(t *testing.T) {
	const needle = "谁来用"
	var inAPI []string
	for _, g := range packageFuncs(t) {
		if n := strings.Count(g.body, needle); n != 0 {
			inAPI = append(inAPI, g.display())
		}
	}
	if len(inAPI) != 0 {
		t.Errorf("httpapi 的函数体里出现了 %v 的问句：进门页标题的兜底只许渲染器一份，"+
			"两处各写一遍就是两份判据（≥2 那一支从语言包取标题，不许在这里写死）", inAPI)
	}
	// 渲染器那一份：Go 侧的兜底与模板里的默认值都算。模板整个目录扫，别只点名 picker.tmpl
	// ——标题从渲染器搬进模板也是"两处"。
	goHits, tmplHits := 0, 0
	for _, g := range funcsInDir(t, filepath.Join("..", "render")) {
		goHits += strings.Count(g.body, needle)
	}
	tmpls := 0
	es, err := os.ReadDir(filepath.Join("..", "render", "web"))
	if err != nil {
		t.Fatalf("读模板目录失败: %v", err)
	}
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".tmpl") {
			tmpls++
			tmplHits += strings.Count(readGoFile(t, filepath.Join("..", "render", "web", e.Name())), needle)
		}
	}
	if tmpls < 4 {
		// 下限只防"扫描器自己坏掉/目录读错"，不是说模板该有几个：新增一个不会红，
		// 少到看不见才红（与"数不许写死读数"同一条纪律）。
		t.Fatalf("只扫到 %d 个模板：目录形状与这条证人假设的对不上", tmpls)
	}
	// 正对照：渲染器里必须**恰好一份**（PickerPage 的兜底）。0 份意味着兜底没了，
	// 那时上面"httpapi 里 0 份"那条会对什么都成立；2 份意味着标题又长了第二处。
	if goHits != 1 {
		t.Errorf("渲染器的 Go 侧有 %d 处「谁来用？」，要恰好 1 处（PickerPage 的空串兜底）", goHits)
	}
	if tmplHits != 0 {
		t.Errorf("模板里出现 %d 处「谁来用？」：标题的兜底只能有一份，搬进模板就是第二份", tmplHits)
	}
}

// 形状错的答法也有人真的发出去过（评审 Important 1）：把这两支删掉时全量测试照样绿，
// 而 openapi 与 AGENTS.md 已经把这种答公开给 AI 了——文档说了却没人执行的形状是假契约。
func TestSetup_形状错走JSON外壳(t *testing.T) {
	_, h := setupZero(t)
	for _, tc := range []struct{ body, field string }{
		// 超上限：ParseForm 失败。字段名与 /pin、/prefs 同一词汇（body）。
		{"title=x&pin=" + strings.Repeat("9", maxBodyBytes+4096), "body"},
		{"title=a&title=b&pin=1234", "title"}, // 同名多值：点名是哪一格
		{"title=%E7%88%B8&pin=1234&pin=5678", "pin"},
	} {
		rec := postForm(h, "/setup", tc.body, "")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%.40q 要 422 JSON 外壳，得到 %d", tc.body, rec.Code)
		}
		f, reason, _ := onlyFieldError(t, rec.Body.Bytes(), tc.field)
		if f != tc.field {
			t.Errorf("%.40q 的 422 要恰好点名 %q，得到 %q", tc.body, tc.field, f)
		}
		if reason == "" {
			t.Errorf("%.40q 的 422 每条字段错都要带一句理由（%q 是空的）", tc.body, f)
		}
	}
	// 正对照：合格提交必须是 303 而不是 422——否则上面三条是在认"任何 422 都算过"。
	if rec := postForm(h, "/setup", "title=%E7%88%B8%E7%88%B8&pin=1234", ""); rec.Code != http.StatusSeeOther {
		t.Errorf("合格提交要 303，得到 %d", rec.Code)
	}
}

// errorShell 是 JSON 错误外壳（422/500 同一形状）。整个解出来再断言，不用 Contains：
// 响应与磁盘的 JSON 都是缩进两空格的（§4.3），`"reason": "x"` 里那个空格是格式细节，
// Contains 打的正是它——一改格式就假红（复扫 Minor：本文件一边这么写注释，一边在
// 词汇那条证人里干了 Contains，两条都不能算有效）。
type errorShell struct {
	Code   int    `json:"code"`
	Detail string `json:"detail"`
	Errors []struct {
		Field  string `json:"field"`
		Reason string `json:"reason"`
	} `json:"errors"`
}

func decodeErrorShell(t *testing.T, body []byte) errorShell {
	t.Helper()
	var out errorShell
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("错误外壳解不开: %v\n%s", err, body)
	}
	if out.Code == 0 {
		t.Fatalf("外壳里没有 code——这不是错误外壳:\n%s", body)
	}
	return out
}

// onlyFieldError 断言"恰好一条字段级错，且点名的是 want"，并返回那条错误供理由比对。
func onlyFieldError(t *testing.T, body []byte, want string) (string, string, string) {
	t.Helper()
	sh := decodeErrorShell(t, body)
	if len(sh.Errors) != 1 {
		t.Fatalf("要恰好一条字段级错（点名 %q），得到 %v（整页:%s）", want, sh.Errors, body)
	}
	return sh.Errors[0].Field, sh.Errors[0].Reason, sh.Detail
}

// 三个端点对"表单读不出来"这同一件病必须吐**同一句话**。/prefs 的证人只认字段名
// （field: body），而 handlers.go、prefs.go、setup.go 是三处各写各的 Reason——
// 词汇一分家，AI 学到的表就裂成三份（这条是三处唯一的合证人）。
func TestFormParseFailure_同一句词汇(t *testing.T) {
	_, h := setup(t)
	body := strings.Repeat("9", maxBodyBytes+4096)
	for _, tc := range []struct{ path, form, cookie string }{
		{"/pin", "profile=dad&pin=" + body, ""},
		{"/prefs", "theme=dark&junk=" + body, dadFormCookie},
		// /setup 在这份夹具里窗口已关（有档案），但形状错发生在建档之前：
		// 它照样要吐 422，而不是 303。
		{"/setup", "title=x&pin=" + body, ""},
	} {
		rec := postForm(h, tc.path, tc.form, tc.cookie)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s 的超限表单要 422，得到 %d %.200s", tc.path, rec.Code, rec.Body.String())
		}
		f, reason, _ := onlyFieldError(t, rec.Body.Bytes(), "body")
		if f != "body" {
			t.Errorf("%s 要恰好点名 body，得到 %q", tc.path, f)
		}
		if reason != "表单读不出来" {
			t.Errorf("%s 的 Reason 漂了，三处（/pin、/prefs、/setup）必须同一句话，得到 %q", tc.path, reason)
		}
	}
}

// 落盘失败那一支：profiles 的位置被一个普通文件占住，store 的 MkdirAll 当场硬失败。
// 这一支从正常夹具打不出来，但它答的是"内部错误 + 不发会话"，删掉它测试得有人红。
func TestSetup_落盘失败是500不发会话(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profiles"), []byte("我不是目录"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := New(newDeps(t, store.New(dir), nil))
	rec := postForm(srv.Handler(), "/setup", "title=dad&pin=1234", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("写不进去要 500 JSON 外壳（它不是权限拒，不该 303），得到 %d", rec.Code)
	}
	// 外壳本身也要钉：光认状态码等于放任它吐 HTML/空体——这条证人的名字承诺了"外壳"。
	sh := decodeErrorShell(t, rec.Body.Bytes())
	if sh.Code != http.StatusInternalServerError || sh.Detail != "内部错误" || len(sh.Errors) != 0 {
		t.Errorf("500 的外壳要 code=500、detail=内部错误、无字段级错，得到 %+v", sh)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("500 要 JSON 外壳，Content-Type = %q", ct)
	}
	if cs := rec.Result().Cookies(); len(cs) != 0 {
		t.Fatalf("失败的建档不许发会话 cookie: %v", cs)
	}
	// 内部文本不外泄：磁盘路径只进 stderr。
	if strings.Contains(rec.Body.String(), "profiles") {
		t.Errorf("500 的响应体里出现了磁盘路径片段:\n%.300s", rec.Body.String())
	}
}

// 唯一那个档案读不出来时的标题（评审 Important 2：删掉中性标题那一支时全量测试全绿，
// 而"标题空着递进去"恰好会退回渲染器那个「谁来用？」兜底——正是本片要消掉的默认值）。
func TestHome_唯一档案读不出用中性标题(t *testing.T) {
	s, h := setupZero(t)
	if _, err := s.CreateProfile(store.NewProfileInput{
		ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	// 把 profile.json 写坏：判据数的是目录（仍是 1 个），取名字走 ListProfiles（跳过坏档）。
	if err := s.WriteFileAtomic(s.ProfilePath("home"), []byte("坏得读不了")); err != nil {
		t.Fatal(err)
	}
	body := get(h, "/", "").Body.String()
	if strings.Contains(body, "谁来用") {
		t.Errorf("档案读不出时也不许退回「谁来用？」:\n%.300s", body)
	}
	if !strings.Contains(body, "<h1>我的书签</h1>") {
		t.Errorf("要用语言包里的中性话（profile.title.default），不许编造名字也不许留空白标题:\n%.300s", body)
	}
	// 0 张卡仍然是对的（PIN 根本无从验证），但这一页不再什么都不说：坏掉的那份要
	// 点名，出路要说实话（2026-10-09 的死路①对策；卡片与说明由渲染器画，这里钉的是
	// "端到端拿得到"——渲染层测试只证明渲染器会画，这一条证明它真进了进门页）。
	if n := strings.Count(body, `action="/pin"`); n != 0 {
		t.Errorf("唯一档案读不出时页面有 %d 张 PIN 卡，要 0（ListProfiles 跳过坏档）。"+
			"给它补一张卡等于把死路换成更响的死路：那份档案的 PIN 在读不动的文件里", n)
	}
	if !strings.Contains(body, "<code>home</code>") || !strings.Contains(body, "数据文件读不了") {
		t.Errorf("这一页必须说出是哪一份进不去、为什么:\n%.700s", body)
	}
	// 判据一行没动，所以坏掉一个**不开**引导窗口。这一条是整处对策的地面：把 handleRoot
	// 的判据从目录数换成"列出来的档案数"，下面这支就会画出 action="/setup"，第二个
	// 人于是可以合法地再建一个管理员（§11 明写要防的那件事）。
	if strings.Contains(body, `action="/setup"`) {
		t.Error("坏掉一个档案就把引导窗口重开了：进门判据数的必须是目录数，不是读得动的档案数")
	}
}

// 引导页的语言下拉：**按它自己的按钮，语言必须立刻变**（2026-10-12）。
//
// 这一格钉的是**人的感受**而不是存储：改下拉按下去之后回到这一页，`<html lang>`
// 必须已经翻过来了。它从前绑在 `POST /setup` 那张表单里，于是"改语言"的生效
// 时刻是"按另一个按钮"——拨完下拉页面纹丝不动，连下拉自己的标签都还是原来
// 那种语言，看上去就是坏的，而那是新装机的人看到的第一块屏幕。
//
// 正对照：切到**同一种**语言也要重画（不是"变了才重画"）；建完档案之后这条路
// 必须关掉（语言从此只归管理员），答 303 回选择页且不带键。
func TestSetup_语言下拉按一下就生效(t *testing.T) {
	_, h := setupZero(t)

	for _, tc := range []struct{ lang, wantLangAttr, wantButton string }{
		{"en", "en", "Switch language"},
		{"zh", "zh-CN", "切换语言"},
	} {
		rec := postForm(h, "/lang", "language="+tc.lang, "")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("切到 %s 要 303，得到 %d", tc.lang, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("回 /（零档案时那就是引导页本身），得到 %q", loc)
		}
		body := get(h, "/", "").Body.String()
		if got := langOfPage(body); got != tc.wantLangAttr {
			t.Errorf("切到 %s 之后这一页的 html lang 该是 %s，实际 %q", tc.lang, tc.wantLangAttr, got)
		}
		// 按钮上的字按**当前**语言写：切完之后同一颗按钮该改写。不改写的话，
		// 人点完看见的还是上一种语言的按钮，更像坏了。
		if !strings.Contains(body, tc.wantButton) {
			t.Errorf("切到 %s 之后那颗按钮该写「%s」", tc.lang, tc.wantButton)
		}
	}
}

// 建完档案之后这条路必须关闭：语言从此是管理员的事，且匿名请求改不动它。
func TestSetup_语言下拉在装机窗口外不可用(t *testing.T) {
	_, h := setupZero(t)
	if rec := postForm(h, "/setup", "title=Dad&pin=1234&language=zh", ""); rec.Code != http.StatusSeeOther {
		t.Fatalf("建档要 303，得到 %d", rec.Code)
	}
	before := langOfPage(get(h, "/", "").Body.String())
	rec := postForm(h, "/lang", "language=en", "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("窗口关了之后该 303，得到 %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("要回选择页且**不带键**（带键就是一条枚举线索），得到 %q", loc)
	}
	if after := langOfPage(get(h, "/", "").Body.String()); after != before {
		t.Errorf("语言被一个匿名请求改了：%s -> %s", before, after)
	}
}

// 形状错的两条：不带值 303 + ?err=nolang，不认识的值 303 + ?err=badlang。
// **不静默落默认**是它们存在的理由——回一句"好了"而界面没换，是最难查的那种。
func TestSetup_语言下拉的形状错说人话(t *testing.T) {
	_, h := setupZero(t)
	for _, tc := range []struct{ body, want string }{
		// **字段没给** = 这个请求没说语言（脚本漏了）→ nolang。
		{"", "nolang"},
		// 给了空串 = 「跟随浏览器」那一档（取消站级默认）→ 正常 303，不带键。
		{"language=", ""},
		{"language=fr", "badlang"},
		{"language=EN", "badlang"},
		{"language=zh&language=en", "body"},
	} {
		rec := postForm(h, "/lang", tc.body, "")
		if tc.want == "body" {
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%q 要 422（同名多值是「没说清要哪个」），得到 %d", tc.body, rec.Code)
			}
			continue
		}
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%q 要 303，得到 %d", tc.body, rec.Code)
			continue
		}
		// want 为空 = **不带键**（那一条是"取消默认"，不是失败）。
		want := "/"
		if tc.want != "" {
			want = "/?err=" + tc.want
		}
		if rec.Header().Get("Location") != want {
			t.Errorf("%q 的 Location = %q，要 %s", tc.body, rec.Header().Get("Location"), want)
		}
	}
}

// langOfPage 取 <html lang="…">，读不出来时给一个一眼看得出是读失败的串。
func langOfPage(body string) string {
	i := strings.Index(body, `<html lang="`)
	if i < 0 {
		return "<no-html-lang>"
	}
	rest := body[i+len(`<html lang="`):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return "<unterminated>"
	}
	return rest[:j]
}
