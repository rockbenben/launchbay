package httpapi

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

// 建档这条链路的判据只许有一份。壳（API 的 JSON，与 Task 4/5 的两张表单）各自只读
// 外壳、写外壳；校验/哈希/落盘的调用点一次都不许出现在壳里。
//
// 为什么用 AST 而不是行为测试：把 PIN 格式判据从 createProfile 删掉，**全部行为证人
// 照样绿**（auth.HashPIN 内部还兜一次格式，响应仍是 422 点名 pin）。行为测试能证明
// "现在是对的"，证明不了"判据没被抄第二遍"——而下一个人加第三条壳时最先做的正是复制粘贴。
//
// 取数范围由 `packageFuncs` 统一推导（整包非测试源码、剥注释、抹空白），这里只说
// 这条闸自己的两件事：
//   - 断言范围 = **整包**，不按"谁调了建档函数"筛。只盯调用者的版本防不住它要防的那件事：
//     复制粘贴判据的人会把那行调用一起省掉（直接调 store），新函数于是根本进不了名单。
//     实测过：往包里加一个不调 `createProfile`、自己走一遍四个判据符号的函数，
//     只盯调用者的版本全绿。
//   - 调用者名单仍然推一遍，但只当**正对照**用：它证明"确实有人调这个共享函数"
//     （判据函数变成没人调的死码时，整包扫会静默地只比它自己）。接收者叫什么一律不管
//     （复扫 Minor：认 `s.createProfile` 的版本会放走 `func probeForm(srv *Server)` 那种
//     换名的壳）。
func Test建档判据只有一处(t *testing.T) {
	const owner = "createProfile"
	// 判据符号分两档，两档的"唯一持有者"不是同一个函数：
	//   - 落点三件套（建档与它的 id 派生）整包只许 `createProfile` 一处持有；
	//   - PIN 的两件（格式与哈希）唯一持有者是 `hashFormPIN`，另有**点名批准的第二批
	//     用户**——`pinapi.go` 那两个设 PIN 的 handler 本来就要验同一格式并哈希。把它们
	//     也按整包禁掉，禁的是别人家的事，那条"闸"会天天红到有人把它删掉。
	// 第二档按"持有者名单"钉，不按"谁调了建档"钉：判据抽进 hashFormPIN 之后，"owner 必须
	// 持有"这句话的主语换了人，而只禁壳里的旧口径管不住新长出来的第三条路。名单口径把这件
	// 事一次说完：**合法持有者恰好这三个函数**，多一个都算判据被抄。
	// 判据符号**不带括号**：方法值写法（`f := auth.HashPIN`）与调用写法抄的是同一条判据。
	// 代价是判得偏严：出现这两个符号，哪怕藏在字符串字面量里，也会红——宁可能红
	// 一点，不放走真抄了判据的那一种。
	writeSymbols := []string{".CreateProfile", ".CreateFirstProfile", ".NextProfileID"}
	pinSymbols := []string{"auth.ValidPinFormat", "auth.HashPIN"}
	const pinOwner = "hashFormPIN"
	// 2026-10-09：管理页要加一个设 PIN 的表单端点，而 spec §8.8 要求它与 API
	// **共用同一个落点**（两个壳不许各写一遍判据），于是把 handleAdminPin 里的
	// 判据搬进了 adminPin，handleAdminPin 退化成纯壳。
	// 名单跟着**搬**而不是放宽成"两个都算"：放宽之后，将来 handleAdminPin 与
	// adminPin 各持一份判据时这条闸仍然通过——而那正是它存在的理由。
	// 2026-10-20：本人自助改 PIN 的面板表单（/prefs/pin）要长出来，而 spec §8.8
	// 要求两个壳**共用同一个落点**——于是把 handleSelfPin 里的判据搬进了 selfPin，
	// handleSelfPin 退化成纯壳。名单跟着**搬**（与 2026-10-09 那次一样）：判据在
	// 落点上，壳只管读入与答出。
	pinHolders := []string{pinOwner, "selfPin", "adminPin"}
	// 建档族：每条壳都必须回来调 hashFormPIN。
	// 2026-10-10：handleRebuildProfile 落进这一族（Task 4 定的名单，那天只写下
	// 了名字——端点本身没写，所以这条路一直是"闸认着一个不存在的函数"，直到
	// 这条闸落地才第一次真的照上）。它同样要重建一份 profile.json，同样要哈希
	// 一枚 PIN，同样不许自己写一遍格式判据。
	pinFamily := []string{"createProfile", "handleRebuildProfile"}
	// 壳的识别符：调用 `x.createProfile(...)` 或取它的方法值，都不带括号找。
	const caller = ".createProfile"

	found := packageFuncs(t)
	byName := map[string][]string{}
	for _, g := range found {
		byName[g.name] = append(byName[g.name], g.body)
	}
	// soleBody 只认"整包唯一"的那个函数体：0 处 = 这条闸正在认一个不存在的东西，
	// >1 处 = 判据函数自己长出了第二份。两种都必须当场红——放过去，下面的
	// strings.Count 就是在对空字符串成立，什么都钉不住。
	soleBody := func(name string) string {
		bodies := byName[name]
		switch len(bodies) {
		case 0:
			t.Fatalf("%s 在整包里没有定义：这条闸正在空转（认了一个不存在的函数）", name)
		case 1:
			return bodies[0]
		default:
			t.Fatalf("%s 在整包里有 %d 处定义，要恰好 1 处", name, len(bodies))
		}
		return ""
	}

	ownerBody := soleBody(owner)
	// 正对照：除了判据函数自己，至少还要有一个调用建档的壳。一条都没匹配上说明
	// caller 的形状与源码对不上，那时下面每个断言都是对空集合成立的假绿。
	// 接收者不参与匹配（复扫 Minor：认 `s.createProfile` 的版本会放走
	// `func probeForm(srv *Server)` 那种换名的壳）。
	shells := []string{}
	for _, g := range found {
		if g.name == owner || strings.Contains(g.body, caller) {
			shells = append(shells, g.display())
		}
	}
	sort.Strings(shells)
	if len(shells) < 2 {
		t.Fatalf("只认出 %v：没识别出任何调用 %q 的壳——判据函数变成没人调的死码，或匹配形状错了",
			shells, caller)
	}

	// 第一档：建档落点三件套。owner 必须持有，其余整包必须为零。只盯调用者的版本防不住
	// 它要防的那件事——复制粘贴建档流程的人会把 `createProfile(...)` 那行一起省掉、直接调
	// store，新函数根本进不了壳的名单。实测（sab9）：往包里加一个不调 createProfile、
	// 自己走一遍 NextProfileID + CreateProfile 的函数，只盯壳的版本全绿。
	for _, sym := range writeSymbols {
		if n := strings.Count(ownerBody, sym); n == 0 {
			t.Errorf("%s 里一次都没有 %s：判据不在唯一落点上", owner, sym)
		}
		for _, g := range found {
			if g.name == owner {
				continue
			}
			if n := strings.Count(g.body, sym); n != 0 {
				t.Errorf("%s 里出现了 %d 次 %s——建档落点被抄出 %s 了（整包只许一处：壳、绕过者、第三条路都算）",
					g.display(), n, sym, owner)
			}
		}
	}

	// 第二档：PIN 的格式与哈希。两个方向都要钉，才对得起"恰好这三个"这句话：
	//   - 名单里每一个都必须**真的持有**（正对照）。少了这一条，名单会腐烂成指向不存在
	//     之物的空承诺，而"名单外必须为零"照样绿——那是这道闸最体面的死法。
	//   - 名单之外整包必须为零：这一条同时管住壳与新长出来的第三条路。
	for _, holder := range pinHolders {
		body := soleBody(holder)
		for _, sym := range pinSymbols {
			if n := strings.Count(body, sym); n == 0 {
				t.Errorf("%s 里没有 %s：持有者名单与源码不符（它要么不再持有判据=名单该删它，"+
					"要么这条闸正在认一个空壳）", holder, sym)
			}
		}
	}
	for _, g := range found {
		if slices.Contains(pinHolders, g.name) {
			continue
		}
		for _, sym := range pinSymbols {
			if n := strings.Count(g.body, sym); n != 0 {
				t.Errorf("%s 里出现了 %d 次 %s——PIN 判据被抄出合法持有者了（整包只许 %v）",
					g.display(), n, sym, pinHolders)
			}
		}
	}

	// 第三档（与第二档不重复）：建档族每条壳都必须调 hashFormPIN。第二档管"判据没被抄出去"，
	// 这一档管"新壳有没有老老实实回来调用那一份"——把 PIN 校验整段删掉、指望 store 兜底的壳，
	// 第二档抓不到（它体内一个符号都没有），只有这一条抓得到。
	for _, name := range pinFamily {
		if n := strings.Count(soleBody(name), pinOwner+"("); n == 0 {
			t.Errorf("%s 里一次都没调 %s：PIN 判据在它里面被自己写了一遍，或被整段删掉了",
				name, pinOwner)
		}
	}
}

// packageFuncs 是本包（或指定目录那份包）所有**非测试**源码里的顶层函数体，
// 供源码级证人用。三条纪律都写在这一块，因为三条闸共用它：
//   - 名单是**推出来的**：写死文件名的版本，新壳落地那天忘了登记，闸就少扫一个
//     文件而没人发现；测试文件必须排除，否则同包测试里的同名函数体自我印证成假绿。
//   - mode 0 不收集注释：断言的是代码，抄在注释里的符号不算判据被搬进代码。
//     反过来说，注释里出现被禁的字面量也不会假红（这一条是复扫抓过的形状）。
//   - 空白全部抹掉：源码把 `auth.` 与 `HashPIN(` 拆成两行、或把实参写成多行，
//     都不该让判据从证人眼前走过去。
//
// **它看不见什么**（要写清，别让人把它当兜底担保）：只收顶层 `FuncDecl`。把一次
// store 调用藏进包级变量（`var probe = func() { store.CreateProfile(...) }`）或
// 方法表达式里，这一份扫不到。这一族闸钉的是"判据没被抄成第二份"，真兜住那条
// 通路的是行为证人与 store 自己的锁；要连闭包一起扫得换 `ast.Inspect`，等它真
// 长出形状再说。
func packageFuncs(t *testing.T) []goFunc { return funcsInDir(t, ".") }

func funcsInDir(t *testing.T, dir string) []goFunc {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("解析 %s 源码失败: %v", dir, err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("%s 里应当只有一个包，得到 %d 个：%v", dir, len(pkgs), pkgs)
	}
	var found []goFunc
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Name == nil || fd.Body == nil {
					continue
				}
				var b strings.Builder
				if err := printer.Fprint(&b, fset, fd.Body); err != nil {
					t.Fatalf("取 %s 函数体失败: %v", fd.Name.Name, err)
				}
				found = append(found, goFunc{name: fd.Name.Name, recv: recvName(fd),
					body: strings.Join(strings.Fields(b.String()), "")})
			}
		}
	}
	if len(found) == 0 {
		t.Fatalf("%s 一个函数都没解析出来：这条闸正在空转", dir)
	}
	return found
}

// goFunc 是断言用的最小函数视图：名字、接收者（用来把方法与普通函数区分开）、抹掉空白后的体。
type goFunc struct{ name, recv, body string }

func (g goFunc) display() string {
	if g.recv == "" {
		return g.name
	}
	return g.recv + "." + g.name
}

func recvName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), fd.Recv.List[0].Type)
	return b.String()
}

// createProfile 的分支里，`FirstOnly` 与"写失败带原始 Err"这两支在 Task 3 落地时还没有
// 任何调用者（引导壳要到 Task 4）。没有证人的分支等于没写过的分支，所以这里直接对判据
// 函数下针。Task 4 之后 /setup 也各有一条路由侧证人（TestSetup_ 那几条），两件事不互相
// 顶替：这条钉判据，那条钉接线。
func TestCreateProfile派生id与API那条仍要显式id(t *testing.T) {
	srv, _ := setupServer(t) // 盘上已有 dad（管理员）与 kid
	// 派生按**存在性**走：slug 是 dad 但被占了 → home，再派生一次 → home-2。
	// 计数驱动的那套（"现有 2 个所以第 3 个叫 3"）会随删除漂移，这里钉的是结果。
	start, err := srv.deps.Store.CountProfiles()
	if err != nil {
		t.Fatal(err)
	}
	first := srv.createProfile(createInput{DeriveID: true, Title: "Dad", PIN: "9999"})
	if first.Status != 0 || len(first.FieldErrs) != 0 {
		t.Fatalf("派生建档应当成功，得到 status=%d detail=%q errs=%v", first.Status, first.Detail, first.FieldErrs)
	}
	if first.Profile.ID != "home" {
		t.Errorf("slug dad 已被占，派生要给 home，得到 %q", first.Profile.ID)
	}
	second := srv.createProfile(createInput{DeriveID: true, Title: "Dad", PIN: "9999"})
	if second.Profile == nil || second.Profile.ID != "home-2" {
		t.Errorf("home 也被占了，第二个派生要给 home-2，得到 %+v", second.Profile)
	}
	// API 那条壳没说要派生：空 id 仍然是 422 点名 id。openapi 把 id 写成
	// required，把"空 id"翻译成"我帮你起个名"是**放宽已发布的契约**，不该由一次
	// 重构顺手做掉。
	api := srv.createProfile(createInput{Title: "无 id", PIN: "9999"})
	if len(api.FieldErrs) != 1 || api.FieldErrs[0].Field != "id" {
		t.Errorf("API 通路空 id 要恰好一条 id 错，得到 %+v", api.FieldErrs)
	}
	// 带空格的 id 照样 422：本次是重构，`{"id":" mom "}` 今天不该悄悄变成 mom——
	// 宽容一点点加进去，加的人不会写测试，读的人只会以为一直如此。
	padded := srv.createProfile(createInput{ID: " mom ", Title: "妈妈", PIN: "9999"})
	if len(padded.FieldErrs) != 1 || padded.FieldErrs[0].Field != "id" {
		t.Errorf(`带空格的 id 要恰好一条 id 错（不许静默 trim 收下），得到 %+v`, padded.FieldErrs)
	}
	// 派生不免检：空标题照样 422。上面两条失败请求都不许落盘——所以档案数只涨
	// 两次成功的量（起点是 dad 与 kid 两条）。
	blank := srv.createProfile(createInput{DeriveID: true, Title: "  ", PIN: "9999"})
	if len(blank.FieldErrs) != 1 || blank.FieldErrs[0].Field != "title" {
		t.Errorf("派生时空标题要恰好一条 title 错，得到 %+v", blank.FieldErrs)
	}
	now, err := srv.deps.Store.CountProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if now != start+2 {
		t.Errorf("档案数应当只随两次成功建档增长：起点 %d，现在 %d，要 %d"+
			"（多出来的那一格就是被拒的请求在盘上留了半成品）", start, now, start+2)
	}
}

// FirstOnly 那一支：引导窗口的判据与写入在 store 的同一次串行化里，handler 只负责
// 把 ErrBootstrapClosed 折成 409。409 的文本与"档案已存在"必须是**两句话**——
// 家属要能分清"这名字被占了"和"这一步早过了"。
func TestCreateProfile首档只成一次(t *testing.T) {
	s := store.New(t.TempDir())
	srv := New(newDeps(t, s, nil))
	ok := srv.createProfile(createInput{DeriveID: true, Title: "Dad", PIN: "9999",
		FirstOnly: true})
	if ok.Status != 0 || ok.Profile == nil {
		t.Fatalf("空盘上的第一档应当建成，得到 status=%d detail=%q", ok.Status, ok.Detail)
	}
	again := srv.createProfile(createInput{DeriveID: true, Title: "Dad", PIN: "9999",
		FirstOnly: true})
	if again.Status != http.StatusConflict || again.Detail != "引导期已过" {
		t.Errorf("窗口关第二次要 409「引导期已过」，得到 status=%d detail=%q", again.Status, again.Detail)
	}
	// 窗口是被**任何人**占的，不是只有引导通路：普通建档先来，第二次走 FirstOnly
	// 同样撞死（这条排除"只在 FirstOnly 之间互斥"那种假实现）。
	occupied := New(newDeps(t, store.New(t.TempDir()), nil))
	if res := occupied.createProfile(createInput{ID: "dad", Title: "爸爸", PIN: "9999"}); res.Status != 0 {
		t.Fatalf("先做一次普通建档应当成功，得到 %+v", res)
	}
	closed := occupied.createProfile(createInput{ID: "kid", Title: "小满", PIN: "8888", FirstOnly: true})
	if closed.Status != http.StatusConflict || closed.Detail != "引导期已过" {
		t.Errorf("普通建档占过位之后，FirstOnly 要 409「引导期已过」，得到 status=%d detail=%q",
			closed.Status, closed.Detail)
	}
	// 与之对照：名字撞车说的是另一句话。
	clash := occupied.createProfile(createInput{ID: "dad", Title: "另一个爸爸", PIN: "8888"})
	if clash.Status != http.StatusConflict || clash.Detail != "档案已存在" {
		t.Errorf("同 id 重复建档要 409「档案已存在」（与引导期已过是两句话），得到 %+v", clash)
	}
}

// 500 那一支带着**原始错误**回来（不是从字符串重建的）：writeInternalError 打进
// stderr 的必须是真正失败的那条链。这里把 profiles 的位置放成一个普通文件，
// store 的 MkdirAll 当场硬失败——这条分支从任何路由上都打不出来。
func TestCreateProfile写失败给原始错误不是字符串(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profiles"), []byte("我不是目录"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := New(newDeps(t, store.New(dir), nil))
	res := srv.createProfile(createInput{ID: "dad", Title: "爸爸", PIN: "9999"})
	if res.Status != http.StatusInternalServerError {
		t.Fatalf("profiles 是个文件时建档要落进 500 那一支，得到 status=%d detail=%q errs=%v",
			res.Status, res.Detail, res.FieldErrs)
	}
	if res.Err == nil {
		t.Fatal("500 那一支必须把原始 error 带出来（Detail 只放白名单文本）")
	}
	if !strings.Contains(res.Err.Error(), "profiles") {
		t.Errorf("原始错误应当点名真正失败的那个路径，得到 %q", res.Err.Error())
	}
}
