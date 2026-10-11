# P1-④.5 首次运行与默认单人 Implementation Plan

> **本文是历史实施记录，不是待办。** 它记录的是那一天计划或量的是什么，里面的
> 函数名、测试名与接口形状**可能已经变了**——2026-10-10 的管理员边界收紧一次性改判掉了
> 代管态、模板机制与徽标切档，共享清单也改判为不做。**不要照着这里的命令执行**：
> 权威契约在仓库根的 `AGENTS.md`，设计在 `docs/superpowers/specs/`。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让零配置装好的 launchbay 第一次打开就能自己走完「建第一个档案 → 输 PIN 进门 → 管理页加人」，并把「只有一个档案的人也要先回答谁来用」这个默认值改掉。

**Architecture:** 首页三形状（0/1/≥2 个档案）由 `httpapi` 一次算出、`render` 只印；新增两条浏览器表单通路（`POST /setup` 引导建档、`POST /admin` 管理页建档），两者**共用同一个 `createProfile` 判据函数**，只换外壳（JSON vs 303+`?err=`）。引导通路的准入判据（磁盘为空）落在 store 的一把名单级锁内，与写入同属一个临界区。

**Tech Stack:** Go 1.27（`CGO_ENABLED=0`，零第三方依赖）、`html/template` + `//go:embed`、`net/http` ServeMux 路由表、GitHub Actions。

**Spec:** `docs/superpowers/specs/2026-10-08-launchbay-first-run-design.md`（本阶段设计，含拍板记录与残余风险）+ `docs/superpowers/specs/2026-10-05-launchbay-design.md`（权威源；Task 6 按设计 §8 的清单改它的字）。**执行者两个都要读**：设计说「为什么这么切」，权威源说「规则是什么」。

## Global Constraints

每个任务的隐含要求，逐字来自 spec 与既有代码约定：

- 零第三方依赖：`go.mod` 不许出现 `require`；不引入拼音转写、不引入任何模板引擎。
- 本期与本片**零 JavaScript**：新页面必须留在「所有页面 `<script` 计数为 0」那条证人罩住的名单里。
- 判据只有一份：建档的校验/哈希/落盘/错误分诊只能在 `createProfile` 里出现一次；两条壳不许各自抄一遍（Task 3 有 AST 证人）。
- 引导判据是**目录数**（`CountProfiles()`），不是 `ListProfiles()` 的长度：坏档案占位，不能让「装坏一个」把窗口重新打开。
- 浏览器通路的权限类拒绝一律 303 回 `/` 且不解释（`redirectPicker`）；形状错/内部错仍走 JSON 外壳（spec §6.1）。
- `?err=` 只当键用：任何写进 Location 的键都必须在 `pinNotices` 表里；用户可控的原文一个字都不进页面。
- 表单通路不认令牌：`POST /setup` 无凭证要求（判据是磁盘），`POST /admin` 带 Bearer 一律 303 拒。
- 引导成功**不发会话 cookie**（不调 `auth.WriteSession`/`SetActive`）。
- 硬规格一族只写一遍：新输入框/按钮并进既有的 44px joint 规则，不许新开第四个位点（三族：输入、按钮、导航+summary）。
- 真实内网地址一个字面量都不许进仓库；文档与夹具只用 `10.` 段与 `127.0.0.1`；`data/`、`.env`、`.superpowers/` 不入库。
- 不许手改 `data/` 下的文件；建档/写入只经服务进程（§4.5）。验收里唯一允许的例外是「坏档彩排」，且必须逐字节复原。
- 预算三条操作线（16,000 / 48,000 / 24,000）**一条都不抬**；超线当发现上报。
- JSON 写出契约 §4.3（缩进 2、末尾一个换行、不转义 HTML、非 ASCII 原样、空数组写 `[]`、键序由 struct 决定）。
- 每条新路由必须同批进 `routes()`、`openapi.json`、`AGENTS.md`（五道契约闸 + 方法列闸 + 枚举同集闸会咬）。

## 文件结构（谁负责什么）

- 新建 `internal/store/bootstrap.go` — 名单级判据与建档串行化：`CountProfiles`、`NextProfileID`、`slugProfileID`、`CreateFirstProfile`、`ErrBootstrapClosed`。
- 新建 `internal/store/bootstrap_test.go` — 上面每一件的同包测试。
- 修改 `internal/store/paths.go` — `Store` 加一把 `pmu sync.Mutex`（档案名单级串行化）。
- 修改 `internal/render/render.go` — `PickerOptions` 加 `Heading`/`Setup`；`PickerPage` 组装 `pickerView` 后交给执行器。
- 修改 `internal/render/exec.go` — `pickerView` 加两字段；`executor.picker` 改吃结构体（不再吃六个位置参数）。
- 修改 `internal/render/web/picker.tmpl` — 三形状分支。
- 修改 `internal/render/web/base.css` — 引导表单与提示的少量样式（不新增 44 位点）。
- 修改 `internal/httpapi/handlers.go` — `handleRoot` 的三形状判据；`handleCreateProfile` 瘦身成壳；新增 `createProfile` 判据函数。
- 新建 `internal/httpapi/setup.go` — `handleSetup` 与 `redirectSetup`、`setupFieldKey`。
- 修改 `internal/httpapi/adminpage.go` — `handleAdminCreate`（含「令牌一律拒」那道闸）。
- 修改 `internal/httpapi/routes.go` — 两条新路由。
- 修改 `internal/httpapi/openapi.json`、`AGENTS.md` — 契约同步。
- 修改 `internal/render/web/admin.tmpl` — notice 行 + 新档案表单。
- 新建 `internal/httpapi/bootstrap_e2e_test.go` — 三形状与两条通路的行为证人（含「不发会话」「关窗后磁盘不动」）。
- 新建 `internal/httpapi/createrule_test.go` — AST 静态证人：建档判据符号只出现在 `createProfile` 里。
- 新建 `docs/superpowers/acceptance/p1-45-first-run/2026-10-08-验收.md` + 实拍 WebP。

---

### Task 1: store 的引导判据与 id 派生

**Files:**
- Create: `internal/store/bootstrap.go`
- Create: `internal/store/bootstrap_test.go`
- Modify: `internal/store/paths.go:16-24`（`Store` struct 加字段）

**Interfaces:**
- Consumes: `Store.profilesDir()`、`Store.ProfileDir(id)`、`Store.CreateProfile(NewProfileInput)`（均已存在）
- Produces:
  - `func (s *Store) CountProfiles() (int, error)`
  - `func (s *Store) NextProfileID(title string) string`
  - `func (s *Store) CreateFirstProfile(in NewProfileInput) (*model.Profile, error)`
  - `var ErrBootstrapClosed = errors.New("引导期已过：磁盘上已有档案")`

- [ ] **Step 1: 写失败测试——`CountProfiles` 数目录，坏档案也占位**

`internal/store/bootstrap_test.go`：

```go
package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// 引导判据要的是"这台机器有没有被人用过"，那是目录级的事实。
// 用 ListProfiles() 会漏：它读不动 profile.json 时静默跳过，
// 于是"把一个档案装坏了"等于把引导窗口重新打开（spec §12 与 §11 的交叉点）。
func TestCountProfiles_坏档案照样占位(t *testing.T) {
	s := New(t.TempDir())
	if n, err := s.CountProfiles(); err != nil || n != 0 {
		t.Fatalf("空安装该是 (0, nil)，得到 (%d, %v)", n, err)
	}
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountProfiles(); n != 1 {
		t.Errorf("建了一个档案后 CountProfiles = %d，要 1", n)
	}
	// 把它装坏：profile.json 写成读不出的字节。
	raw, err := os.ReadFile(s.ProfilePath("dad"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ProfilePath("dad"), append([]byte("{坏"), raw...), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountProfiles(); n != 1 {
		t.Errorf("档案坏了以后 %d，要仍是 1（坏档案占位，窗口不许重开）", n)
	}
	if _, err := s.ListProfiles(); err != nil {
		t.Fatalf("ListProfiles 对坏档案的容错是本测试的前提: %v", err)
	}
}

// 前提证人：上面那句"ListProfiles 会跳过坏档案"必须是真的，
// 否则这条测试测的是一个不存在的差别（它红了就说明 store 的行为变了，本任务的
// 判据选择要重新讨论，不是改测试）。
func TestCountProfiles_前提没变(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ProfilePath("dad"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	ps, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 0 {
		t.Fatalf("坏档案仍被 ListProfiles 列出（%d 个）：本任务改用目录数判据的理由不再成立", len(ps))
	}
}
```

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/store/ -run TestCountProfiles -count=1 -v`
Expected: 编译失败，报 `s.CountProfiles undefined`。

- [ ] **Step 3: 写实现——`CountProfiles` + `pmu` 字段**

`internal/store/paths.go` 的 `Store` struct 改成（新增 `pmu` 一行）：

```go
type Store struct {
	root string

	bmu    sync.Mutex
	broken map[string]map[string]string // id → 来源(profile/bookmarks) → 原因

	// pmu 是档案**名单**级的串行化，与按 id 的那把锁（lmu/locks）两回事：
	// 它管"引导窗口只开得下一个人"，按 id 的锁看不见"两个不同 id 各建一个"。
	pmu   sync.Mutex
	lmu   sync.Mutex
	locks map[string]*sync.Mutex
}
```

`internal/store/bootstrap.go`：

```go
package store

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrBootstrapClosed 说的是"引导窗口已经关上"。handler 把它映射成不带
// ?err= 的 303——"这台机器已经建过档了"本身就是枚举线索（spec §6.1）。
var ErrBootstrapClosed = errors.New("引导期已过：磁盘上已有档案")

// CountProfiles 数 profiles/ 下的目录个数。
//
// 不用 ListProfiles()：它读不动某个 profile.json 时会静默跳过（§12 的隔离），
// 于是"把一个档案装坏了"等于把引导窗口重新打开，第二个人可以合法地再建一个
// 管理员。引导判据要的是磁盘上的占位事实。
// 目录还不存在 = 一个档案都没有，返回 (0, nil)：全新安装的形状本来就长这样。
func (s *Store) CountProfiles() (int, error) {
	ds, err := os.ReadDir(s.profilesDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("数档案目录: %w", err)
	}
	n := 0
	for _, d := range ds {
		if d.IsDir() {
			n++
		}
	}
	return n, nil
}
```

- [ ] **Step 4: 跑测试确认它绿**

Run: `go test ./internal/store/ -run TestCountProfiles -count=1`
Expected: PASS（两条都过）。

- [ ] **Step 5: 写失败测试——id 派生表驱动**

追加到 `bootstrap_test.go`：

```go
func TestNextProfileID(t *testing.T) {
	s := New(t.TempDir())
	cases := []struct{ title, want string }{
		{"Dad", "dad"},
		{"nas box", "nas-box"},
		{"MiXeD--Case!!", "mixed-case"},
		{"  爸爸  ", "home"},   // 中文一律 slug 不出东西 → 基名序列
		{"---", "home"},        // 只有分隔符 → 同上
		{"日本語", "home"},
	}
	for _, c := range cases {
		if got := s.NextProfileID(c.title); got != c.want {
			t.Errorf("NextProfileID(%q) = %q，要 %q", c.title, got, c.want)
		}
	}
	// 长度上界：model.ValidSlug 只收到 32 个字符。
	long := s.NextProfileID(strings.Repeat("a", 50))
	if len(long) > 32 || !model.ValidSlug(long) {
		t.Errorf("超长名字派生出 %q（%d 字符），要合法 slug 且 ≤32", long, len(long))
	}
}

// 撞名走基名序列，而且判据是"这个目录在不在"——不是"现在有几个人"：
// 删过档的计数会撒谎（spec §13 的口径之一）。
func TestNextProfileID_撞名与删除后回到基名(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("dad"); got != "home" {
		t.Errorf("slug 撞了已有 id 要退回基名序列，得到 %q", got)
	}
	if err := os.RemoveAll(s.ProfileDir("dad")); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("dad"); got != "dad" {
		t.Errorf("目录删掉以后该能重新用 dad，得到 %q（判据像在数东西而不是看在不在）", got)
	}
	if got := s.NextProfileID("爸爸"); got != "home" {
		t.Errorf("基名序列起点要是 home，得到 %q", got)
	}
	if err := os.MkdirAll(s.ProfileDir("home"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("爸爸"); got != "home-2" {
		t.Errorf("home 被占要递增到 home-2，得到 %q", got)
	}
	if err := os.MkdirAll(s.ProfileDir("home-2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := s.NextProfileID("妈妈"); got != "home-3" {
		t.Errorf("要 home-3，得到 %q", got)
	}
}
```

- [ ] **Step 6: 跑测试确认它红**

Run: `go test ./internal/store/ -run TestNextProfileID -count=1`
Expected: 编译失败，`s.NextProfileID undefined`。

- [ ] **Step 7: 写实现——`slugProfileID` + `NextProfileID`**

追加到 `bootstrap.go`（并在 import 里补 `path/filepath`? 不需要；`os`、`strings`、`fmt`、`errors` 已够）：

```go
// slugProfileID 把显示名折成 ASCII slug：小写、非 [a-z0-9] 的段当一个分隔符、
// 首尾不留 '-'、截到 32 个字符（model.ValidSlug 的上界）。
//
// 中文名一律 slug 不出东西，返回空串由调用方走基名序列。这不是待修缺陷：
// 表单不问 id，而做拼音转写要么加依赖（违零第三方）要么加一张表（违 §11
// "多出来的件也算失败"）。设计 §11.2 把这条代价写死在明处。
func slugProfileID(title string) string {
	var b strings.Builder
	prevSep := true // 起始当作"刚出过分隔符"，好把开头的 '-' 全吃掉
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSep = false
			continue
		}
		if !prevSep && b.Len() > 0 {
			b.WriteByte('-')
			prevSep = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 32 {
		out = strings.Trim(out[:32], "-")
	}
	return out
}

// NextProfileID 给一个"还不在盘上"的档案 id（设计 §5）：
// slug 得动且没被占 → 用它；否则基名 home 的递增序列。
func (s *Store) NextProfileID(title string) string {
	if id := slugProfileID(title); id != "" && !s.idTaken(id) {
		return id
	}
	for n := 1; ; n++ {
		id := "home"
		if n > 1 {
			id = fmt.Sprintf("home-%d", n)
		}
		if !s.idTaken(id) {
			return id
		}
	}
}

func (s *Store) idTaken(id string) bool {
	fi, err := os.Stat(s.ProfileDir(id))
	return err == nil && fi.IsDir()
}
```

测试文件顶部需要 `model` 的导入：`"github.com/rockbenben/launchbay/internal/model"`。

- [ ] **Step 8: 跑测试确认它绿**

Run: `go test ./internal/store/ -run TestNextProfileID -count=1`
Expected: PASS。

- [ ] **Step 9: 写失败测试——`CreateFirstProfile` 的窗口与并发**

追加到 `bootstrap_test.go`：

```go
// 主证人：判据必须在 store 内部、写入之前。把判据搬到 handler（"先查后写"）
// 就是这条打红——它比任何并发测试都可靠，因为它不靠时钟运气。
func TestCreateFirstProfile_窗口只开得下一个人(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateFirstProfile(NewProfileInput{ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
		t.Fatalf("空安装引导建档失败: %v", err)
	}
	_, err := s.CreateFirstProfile(NewProfileInput{ID: "home-2", Title: "妈妈", PinHash: "sha256$a$b"})
	if !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("第二次引导要 ErrBootstrapClosed（errors.Is 链要在），得到 %v", err)
	}
	if n, _ := s.CountProfiles(); n != 1 {
		t.Errorf("拒的那一次磁盘上多出了东西：%d 个目录，要 1", n)
	}
}

// 空目录（不是合法档案）也占位：判据是"有没有人用过这台机器"，
// 不是"有没有一个进得去的档案"。
func TestCreateFirstProfile_半个目录也占位(t *testing.T) {
	s := New(t.TempDir())
	if err := os.MkdirAll(filepath.Join(s.Root(), "profiles", "ghost"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFirstProfile(NewProfileInput{ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("ghost 占位时要拒，得到 %v", err)
	}
}

// 辅助证人（不是主证人）：真并发。单跑不足以证明锁的范围——判据搬到锁外
// 这条常常照样绿，所以主证人上面那条。留着它的理由是 -race 能抓到
// "两个不同 id 各建一个管理员"这种真实组合，而前一条测不到。
func TestCreateFirstProfile_并发提交只有一个成(t *testing.T) {
	s := New(t.TempDir())
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, title := range []string{"alpha", "intruder"} {
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			<-start
			_, err := s.CreateFirstProfile(NewProfileInput{
				ID: s.NextProfileID(title), Title: title, PinHash: "sha256$a$b"})
			results <- err
		}(title)
	}
	close(start)
	wg.Wait()
	close(results)
	var ok, closed int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrBootstrapClosed):
			closed++
		default:
			t.Fatalf("意外的错: %v", err)
		}
	}
	if ok != 1 || closed != 1 {
		t.Errorf("建成 %d、拒 %d，要 1/1", ok, closed)
	}
	if n, _ := s.CountProfiles(); n != 1 {
		t.Errorf("磁盘上有 %d 个档案目录，要 1 个（两个都建成管理员是最贵的形状）", n)
	}
}
```

- [ ] **Step 10: 跑测试确认它红**

Run: `go test ./internal/store/ -run TestCreateFirstProfile -count=1`
Expected: 编译失败，`s.CreateFirstProfile undefined` / `ErrBootstrapClosed undefined`。

- [ ] **Step 11: 写实现——`CreateFirstProfile`**

追加到 `bootstrap.go`（import 补 `"github.com/rockbenben/launchbay/internal/model"`）：

```go
// CreateFirstProfile 是引导建档唯一的落点：判据（磁盘为空）与写入同属
// `pmu` 的一次临界区。
//
// 为什么必须这样：按 id 的那把锁管不了这件事——两台设备各拿 `alpha` 与
// `intruder` 两个名字，派生出两个互不相撞的 id，各自过各自的 exists 检查，
// 于是安静地建出**两个**管理员档案。"只在 0 档案时开着"这句话要在写入
// 时刻也为真，就只能把判据和写入放进同一次串行化里（设计 §3、§11.1）。
func (s *Store) CreateFirstProfile(in NewProfileInput) (*model.Profile, error) {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	n, err := s.CountProfiles()
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrBootstrapClosed
	}
	return s.CreateProfile(in)
}
```

- [ ] **Step 12: 跑测试确认它绿（含 -race）**

Run: `go test ./internal/store/ -count=1 -race`
Expected: 全包 PASS，无 race 报告。

- [ ] **Step 13: 四次【故意破坏】并还原**

每次破坏后跑**指定的那条**测试，必须红在那条上，然后原样还原、复跑绿：

| # | 破坏 | 必须变红 |
|---|---|---|
| 1 | `CreateFirstProfile` 里删掉 `n > 0` 那段判断 | `TestCreateFirstProfile_窗口只开得下一个人` |
| 2 | 判据换成 `len(已存在档案目录) == 0` 但改调 `ListProfiles()` 数 | `TestCountProfiles_坏档案照样占位` |
| 3 | `NextProfileID` 的基名序列改成 `fmt.Sprintf("home-%d", s.count()+1)`（按计数） | `TestNextProfileID_撞名与删除后回到基名` |
| 4 | `slugProfileID` 末尾不再 `Trim("-")`（让 `---` 吐出非法 slug） | `TestNextProfileID` |

- [ ] **Step 14: 门禁四条 + 提交**

```bash
gofmt -l .
go vet ./...
go test ./... -count=1
CGO_ENABLED=0 go build ./...
```

```bash
git add internal/store/bootstrap.go internal/store/bootstrap_test.go internal/store/paths.go
git commit -m "feat(store): 引导建档的名单级判据与档案 id 派生

窗口要只开得下一个人：按 id 的锁看不见"两个名字各派生一个 id"的组合，
那样会安静地建出两个管理员。判据（目录数，坏档案也占位）与写进同一次
pmu 临界区。id 由名字派生，slug 不动就走 home/home-2 基名序列——判据是
"目录在不在"，不是"现在几个人"，删过档的计数会撒谎。"
```

---

### Task 2: render 的首页三形状

**Files:**
- Modify: `internal/render/render.go`（`PickerOptions`、`PickerPage`）
- Modify: `internal/render/exec.go`（`pickerView` 两字段、`executor.picker` 吃结构体）
- Modify: `internal/render/web/picker.tmpl`
- Modify: `internal/render/web/base.css`（引导表单的少量样式）
- Test: `internal/render/views_test.go`、`internal/render/budget_test.go`

**Interfaces:**
- Consumes: `executor.picker`、`webFS` 内嵌模板（已存在）
- Produces:
  - `PickerOptions` 增加 `Heading string`、`Setup bool`
  - `func (r *Renderer) PickerPage(opts PickerOptions) (Page, error)` 签名不变

- [ ] **Step 1: 写失败测试——三形状**

`internal/render/views_test.go` 末尾追加：

```go
// 首页三种形状（设计 §2）。判据由 httpapi 算，这里只验"给了什么就印什么"，
// 以及**模板不数档案**：它要是自己数了，就有了第二处判据。
func TestPickerPage_三形状(t *testing.T) {
	s := seeded(t) // dad、mom 两个档案
	save(t, s, "dad", "爸爸的台", "dsm")
	save(t, s, "mom", "妈妈的台", "emby")
	r := renderer(t, s)

	two, err := r.PickerPage(PickerOptions{Heading: "谁来用？"})
	if err != nil {
		t.Fatal(err)
	}
	// 基线：≥2 的形状与 P1-④ 验收时逐字一致的两条硬事实。
	if !strings.Contains(string(two.Body), "<h1>谁来用？</h1>") {
		t.Errorf("多人形状 h1 不对:\n%.300s", two.Body)
	}
	if n := strings.Count(string(two.Body), `action="/pin"`); n != 2 {
		t.Errorf("两张档案卡要两个 /pin 表单，得到 %d", n)
	}
	if strings.Contains(string(two.Body), `action="/setup"`) {
		t.Error("有档案的页面不许画引导表单")
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
}
```

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/render/ -run TestPickerPage_三形状 -count=1`
Expected: 编译失败，`PickerOptions` 无 `Heading` 字段。

- [ ] **Step 3: 写实现——结构体侧**

`internal/render/render.go` 的 `PickerOptions` 替换为：

```go
// PickerOptions 控制进门页的三形状。Heading 与 Setup 都由调用方（httpapi）
// 算好递进来：**档案数怎么翻成"该问谁 / 该问口令 / 该让他建第一个"是权限与
// 产品判据，不在渲染器里重复第二遍**（与 themeAttr、validatedAccent 同纪律）。
type PickerOptions struct {
	Preselect string
	// KeepAdd 是 bookmarklet 带进来的 ?add=，必须穿过 PIN 步骤活着回来。
	KeepAdd string
	// Notice 是进门失败后要说的一句人话（由 httpapi 的白名单给出）。
	Notice string
	// Heading 是 h1 的全部内容：0 档案「还没有档案」、1 档案那个人的名字、
	// ≥2「谁来用？」。空串按"谁来用？"渲染（既有测试与任何没传名字的调用点
	// 都拿到旧文案，不会静默变成空白标题）。
	Heading string
	// Setup 为真时这一页画引导建档表单，而不是档案卡列表。
	Setup bool
}
```

`PickerPage` 内组装视图那一段替换为（`picker` 改吃视图结构体）：

```go
	heading := opts.Heading
	if heading == "" {
		heading = "谁来用？"
	}
	v := pickerView{Entries: views, Preselect: opts.Preselect, KeepAdd: opts.KeepAdd,
		Notice: opts.Notice, Heading: heading, Setup: opts.Setup}
	body, err := r.tmpl.picker(v)
```

- [ ] **Step 4: 写实现——执行器与模板**

`internal/render/exec.go` 的 `pickerView` 加两字段、`executor.picker` 换签名：

```go
type pickerView struct {
	Entries   []pickerEntry
	Heading   string
	Setup     bool
	CSS       template.CSS
	Preselect string
	KeepAdd   string
	Notice    string
}

func (e *executor) picker(v pickerView) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.pickerT.ExecuteTemplate(&buf, "picker", v); err != nil {
		return nil, fmt.Errorf("渲染进门页: %w", err)
	}
	return buf.Bytes(), nil
}
```

`internal/render/web/picker.tmpl` 整份替换成：

```gotemplate
{{define "picker"}}<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Heading}}</title>
<style>{{.CSS}}</style>
</head>
<body>
<main role="main">
  <h1>{{.Heading}}</h1>
  {{/* 提示放在卡片之前：读屏的人不该先听完一屏档案才知道刚才没进去。
       文案由 httpapi 的白名单给，这里只负责说。 */}}
  {{with .Notice}}<p class="notice" role="alert">{{.}}</p>{{end}}
  {{if .Setup}}
    {{/* 引导建档（设计 §3）：只有两个输入框，id 与"这个人是不是管理员"都不问——
         id 由 store 按名字派生，管理员身份由服务端固定。页面自己不做二次确认，
         也不承诺"建完就能进"：建完照样要 PIN 才进门。 */}}
    <form method="post" action="/setup" class="pick setup">
      <label>怎么叫 <input name="title" type="text" maxlength="60" required autocomplete="off" autofocus></label>
      <label>PIN <input name="pin" type="password" inputmode="numeric" minlength="4" maxlength="6" required></label>
      <button type="submit">建好，开始用</button>
      <p class="hint">这个人是这台导航台的管理员，能进管理页看与改所有人的档案。
        建完这一步就不能再建第二次——加人请进管理页。</p>
    </form>
  {{else}}
  <ul class="profiles">
  {{range .Entries}}
    <li>
      {{/* 每张卡片都是一个自包含的表单：profile 走 hidden 字段，
           选档案不需要任何脚本。?add= 同样逐卡片带上，穿过 PIN 步骤。 */}}
      <form method="post" action="/pin" class="pick">
        <input type="hidden" name="profile" value="{{.ID}}">
        {{with $.KeepAdd}}<input type="hidden" name="keep_add" value="{{.}}">{{end}}
        <span class="pick-name"><span class="ic" aria-hidden="true">{{.Letter}}</span><b>{{.Title}}</b></span>
        <label>PIN <input name="pin" type="password" inputmode="numeric" autocomplete="off"
                          minlength="4" maxlength="6" required{{if eq .ID $.Preselect}} autofocus{{end}}></label>
        <button type="submit">进入</button>
      </form>
    </li>
  {{end}}
  </ul>
  {{end}}
</main>
</body>
</html>{{end}}
```

`base.css` 在 `.pick` 一族之后加（**不加新的 44 位点**：`.setup` 的输入框与按钮已被输入族/按钮族的 joint 规则罩住）：

```css
.setup{flex-direction:column;align-items:stretch;gap:10px}
.setup label{display:flex;flex-direction:column;gap:6px;color:var(--ink2);font-size:13px}
.hint{color:var(--ink2);font-size:12px;margin:2px 0 0}
```

- [ ] **Step 5: 跑测试确认它绿 + 44px 名单证人自动覆盖**

Run: `go test ./internal/render/ -count=1`
Expected: PASS。若 `TestCSS_搜索控件的44px由哪条规则保证` 变红，说明 `.setup` 的输入框/按钮掉出了名单——**改选择器并进那条 joint 规则**，不许新开一处 `min-height:44px`。

- [ ] **Step 6: 把引导页放进零脚本与预算名单**

`internal/render/budget_test.go` 的 `TestBudget_四种页面都零脚本` 改名为 `TestBudget_五种页面都零脚本`，并在 `pages` 组装处加一段：

```go
	su, err := r.PickerPage(PickerOptions{Heading: "还没有档案", Setup: true})
	if err != nil {
		t.Fatal(err)
	}
	pages["setup"] = su.Body
```

同文件那条内联 CSS 上限的测试会重取读数；把 `TestBudget_首屏字节在两条线内` 注释块里的读数按 HEAD 改写（**线不动**），并加一行说明引导页与单人页远小于 25 条首屏。

- [ ] **Step 7: 三次【故意破坏】并还原**

| # | 破坏 | 必须变红 |
|---|---|---|
| 1 | 模板里 `{{if .Setup}}` 改成 `{{if .Entries}}`（引导分支永不出现） | `TestPickerPage_三形状` |
| 2 | `PickerPage` 里把 `heading` 无条件写回 `"谁来用？"`（忽略传入的 Heading） | `TestPickerPage_三形状`（单人那两半） |
| 3 | `.setup label input` 那族改成 `min-height:30px` 新写一条 | `TestCSS_搜索控件的44px由哪条规则保证` |

- [ ] **Step 8: 门禁四条 + 提交**

```bash
git add internal/render/
git commit -m "feat(render): 进门页三形状——零档案画引导表单、单档案直接是那人首页

h1 与形状由调用方算好递进来：模板不数档案，判据就不许有第二处。
≥2 人的形状逐字不动（基线断言摆在那里）；引导分支没有二次确认、
也不承诺建完就进。"
```

---

### Task 3: `createProfile` 判据抽取（纯重构）+ AST 静态证人

**Files:**
- Modify: `internal/httpapi/handlers.go:355-420`（`handleCreateProfile` 拆成壳 + `createProfile`）
- Test: `internal/httpapi/createrule_test.go`

**Interfaces:**
- Consumes: `decodeBody`、`decodeFieldErrors`、`writeFieldErrors`、`fieldError`、`s.deps.Store.CreateProfile`、`auth.ValidPinFormat`、`auth.HashPIN`、`model.ValidSlug`
- Produces:
  - `type createInput struct { ID, Title, PIN, FromTemplate string; Admin, FirstOnly bool }`
  - `type createResult struct { Profile *model.Profile; FieldErrs []fieldError; Status int; Detail string }`（`Status == 0` 表示成功）
  - `func (s *Server) createProfile(in createInput) createResult`

- [ ] **Step 1: 先立静态证人（它现在就该红，因为 `createProfile` 还不存在）**

`internal/httpapi/createrule_test.go`：

```go
package httpapi

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

// 建档这条链路的判据只许有一份。两个壳（API 的 JSON、管理页/引导页的表单）
// 各自只读外壳、写外壳；校验/哈希/落盘的调用点一次都不许出现在壳里。
//
// 为什么用 AST 而不是行为测试：行为测试能证明"现在是对的"，
// 证明不了"判据没被抄第二遍"——而下一个人加第三条壳时最先做的正是复制粘贴。
func Test建档判据只有一处(t *testing.T) {
	// 建档动作的三个符号。登录与改 PIN 也哈希 PIN，但它们不在建档链路上，
	// 所以断言按**函数体**取，不按整包 grep。
	symbols := []string{"auth.ValidPinFormat(", "auth.HashPIN(", ".CreateProfile(", ".CreateFirstProfile("}
	// want[i] 是每个符号允许出现的位置。
	wants := map[string]string{
		"auth.ValidPinFormat(":  "createProfile",
		"auth.HashPIN(":         "createProfile",
		".CreateProfile(":       "createProfile",
		".CreateFirstProfile(":  "createProfile",
	}
	// 名单随任务增长：本任务只有这两个，Task 4 加 handleSetup、Task 5 加
	// handleAdminCreate。**不为了凑齐名单而建未注册的骨架**——那是两个没人调用的
	// 函数存活的中间提交，正是本项目点名的死形状。
	shells := []string{"handleCreateProfile", "createProfile"}

	fset := token.NewFileSet()
	bodies := map[string]string{}
	for _, name := range []string{"handlers.go"} { // Task 4/5 各自把自己那个文件加进名单
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", name, err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name == nil {
				continue
			}
			for _, want := range shells {
				if fd.Name.Name == want {
					var b strings.Builder
					if err := printer.Fprint(&b, fset, fd.Body); err != nil {
						t.Fatalf("取 %s 函数体失败: %v", want, err)
					}
					bodies[want] = b.String()
				}
			}
		}
	}
	// 解析器不能空转：四个函数都必须取到函数体，否则下面的断言全是对空串比较。
	for _, name := range shells {
		if bodies[name] == "" {
			t.Fatalf("没取到 %s 的函数体：这条闸正在空转", name)
		}
	}
	for sym, owner := range wants {
		for _, name := range shells {
			n := strings.Count(bodies[name], sym)
			switch name {
			case owner:
				if n == 0 {
					t.Errorf("%s 里一次都没有 %s：判据不在唯一落点上", owner, sym)
				}
			default:
				if n != 0 {
					t.Errorf("%s 里出现了 %d 次 %s——建档判据被抄进壳里了（唯一落点是 %s）", name, n, sym, owner)
				}
			}
		}
	}
}
```

文件名单是 `handlers.go`、`setup.go`、`adminpage.go` 三个显式路径——不扫整包，因为整包扫描会把 `_test.go` 里的同名函数体也算进来，那是一条自我印证的假绿。**这条测试现在必须红在"读不到 setup.go"**，而不是红在断言不满足：那说明它测的是一个还不存在的函数，而不是测错了东西。

同文件还要留一个 helper 给后面的源码级证人用（Task 4 的「handler 不数档案」要用它）：

```go
// readGoFile 是同包源码级证人的取数入口（不是给人读的测试数据）。
func readGoFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", name, err)
	}
	return string(raw)
}
```

import 需要 `"os"`。

- [ ] **Step 2: 先验证这条证人真的会红（不写完实现就宣布它有效）**

Run: `go test ./internal/httpapi/ -run Test建档判据只有一处 -count=1`
Expected: 此刻 `createProfile` 还不存在 → **编译失败**（`s.createProfile undefined`），
这条不算证人的 RED。所以 Step 3 写完 `createProfile` 之后、把 `handleCreateProfile` 接上之前，
先跑一次：它必须红在「handleCreateProfile 里出现了 N 次 auth.HashPIN(」——那才是这条闸的 RED。

- [ ] **Step 3: 写 `createProfile`（把现有校验与分诊原样搬进来）**

`internal/httpapi/handlers.go`，紧跟 `createProfileResponse` 之后新增。（`createProfile` 用到 `errors`、`net/http`、`strings`、`model`、`auth`、`store`——本文件已经全部导入，抽完函数后 `gofmt -l .` 与 `go vet ./...` 都要静。）

```go
// createInput 是建档的全部输入。ID 留空 = 由 store 按名字派生（两条表单通路
// 都不问 id，设计 §5）。FirstOnly 把落点换成 CreateFirstProfile——引导通路
// 的"磁盘为空"判据与写入必须在 store 的同一次串行化里，handler 不数档案。
type createInput struct {
	ID           string
	Title        string
	PIN          string
	FromTemplate string
	Admin        bool
	FirstOnly    bool
}

// createResult 是建档的结果外壳无关部分：Status == 0 是成功；FieldErrs 非空
// 是 422 级；其余 Status 直接是 JSON 壳要写的状态码。表单壳不写这些码——
// 它把 FieldErrs 折成 ?err= 白名单键、把 ErrBootstrapClosed 折成不带 err 的 303。
type createResult struct {
	Profile   *model.Profile
	FieldErrs []fieldError
	Status    int
	Detail    string
}

func (s *Server) createProfile(in createInput) createResult {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = s.deps.Store.NextProfileID(in.Title)
	}
	var errs []fieldError
	if in.ID != "" && !model.ValidSlug(id) { // API 壳才问 id，表单壳不问
		errs = append(errs, fieldError{Field: "id", Reason: "必须是 ASCII slug（小写字母、数字、连字符）"})
	}
	if strings.TrimSpace(in.Title) == "" {
		errs = append(errs, fieldError{Field: "title", Reason: "不能为空"})
	}
	if !auth.ValidPinFormat(in.PIN) {
		errs = append(errs, fieldError{Field: "pin", Reason: "要 4-6 位数字"})
	}
	if len(errs) > 0 {
		return createResult{FieldErrs: errs}
	}
	hash, err := auth.HashPIN(in.PIN)
	if err != nil {
		return createResult{FieldErrs: []fieldError{{Field: "pin", Reason: err.Error()}}}
	}
	pi := store.NewProfileInput{ID: id, Title: in.Title, PinHash: hash,
		FromTemplate: in.FromTemplate, Admin: in.Admin}
	var p *model.Profile
	if in.FirstOnly {
		p, err = s.deps.Store.CreateFirstProfile(pi)
	} else {
		p, err = s.deps.Store.CreateProfile(pi)
	}
	switch {
	case errors.Is(err, store.ErrBootstrapClosed):
		return createResult{Status: http.StatusConflict, Detail: "引导期已过"}
	case errors.Is(err, store.ErrExists):
		return createResult{Status: http.StatusConflict, Detail: "档案已存在"}
	case errors.Is(err, store.ErrNotFound):
		return createResult{FieldErrs: []fieldError{{Field: "fromTemplate", Reason: "模板不存在"}}}
	case errors.Is(err, store.ErrInvalid), errors.Is(err, store.ErrCorrupt):
		return createResult{FieldErrs: []fieldError{{Field: "fromTemplate",
			Reason: "模板档案不存在或数据文件读不了，请检查 templates/ 下该模板的 bookmarks.json"}}}
	case err != nil:
		return createResult{Status: http.StatusInternalServerError, Detail: err.Error()}
	}
	return createResult{Profile: p}
}
```

`handleCreateProfile` 的函数体改成（保留原有的 `decodeBody` 与 `require` 顺序和注释意图）：

```go
func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		PIN          string `json:"pin"`
		FromTemplate string `json:"fromTemplate"`
		Admin        bool   `json:"admin"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	// 访问判定交回 require 这一份唯一判据（最终整枝评审 #1）；判据本身
	// 与表单壳完全同一份——两条通路唯一的不同是怎么读入、怎么答出。
	if _, _, ok := s.require(r, credAdmin, in.ID); !ok {
		s.writeAuthFailure(w, r)
		return
	}
	res := s.createProfile(createInput{ID: in.ID, Title: in.Title, PIN: in.PIN,
		FromTemplate: in.FromTemplate, Admin: in.Admin})
	switch {
	case len(res.FieldErrs) > 0:
		writeFieldErrors(w, http.StatusUnprocessableEntity, res.FieldErrs)
	case res.Status == http.StatusConflict:
		writeJSON(w, res.Status, errorResponse{Code: res.Status, Detail: res.Detail})
	case res.Status != 0:
		writeInternalError(w, errors.New(res.Detail))
	default:
		writeJSON(w, http.StatusCreated, createProfileResponse{
			ID: res.Profile.ID, Title: res.Profile.Title, Admin: res.Profile.Admin})
	}
}
```

- [ ] **Step 4: 本任务到此为止——不建任何未注册的骨架**

`setup.go` / `adminpage.go` 的壳在 Task 4、Task 5 各自的任务里**与路由注册同批**出现，
并把各自的名字加进 `shells` 与文件名单。

- [ ] **Step 5: 跑全量测试确认重构没改行为**

Run: `go test ./... -count=1`
Expected: PASS。`admin_test.go` 里建档的 422/409/201 断言必须原样通过——它们是这次重构的行为证人。

- [ ] **Step 6: 跑静态证人确认它绿，并当场验一次它会红**

Run: `go test ./internal/httpapi/ -run Test建档判据只有一处 -count=1` → PASS。
然后在 `handleCreateProfile` 里临时加一行 `_ = auth.HashPIN("1234")` 再跑：必须红在
「handleCreateProfile 里出现了 1 次 auth.HashPIN(」，删掉还原、复跑绿。从没红过的证人不算证人。

- [ ] **Step 7: 三次【故意破坏】并还原**

| # | 破坏 | 必须变红 |
|---|---|---|
| 1 | 在 `handleCreateProfile` 里加一行 `_ = auth.ValidPinFormat(in.PIN)` | `Test建档判据只有一处`（点名壳里有符号） |
| 2 | `createProfile` 里删掉 `!auth.ValidPinFormat(...)` 那段校验 | `admin_test.go` 的建档 422 那条（**行为证人先红，静态证人不红**——这正说明两者测的不是同一件事，都要留） |
| 3 | 静态证人里把 `shells` 少写一个名字（比如去掉 `handleSetup`） | `Test建档判据只有一处` 的空转守卫（`没取到 handleSetup 的函数体`） |

- [ ] **Step 8: 门禁四条 + 提交**

```bash
git add internal/httpapi/handlers.go internal/httpapi/createrule_test.go
git commit -m "refactor(httpapi): 建档判据收进 createProfile，两壳只换外壳

校验/哈希/落盘/错误分诊原来长在 handleCreateProfile 的函数体里；引导页与管理页
都要走同一条链路，复制那段就是第二份判据。抽成一个函数，并用 AST 证人钉住
"这三个符号只许出现在 createProfile 里"——行为测试证明不了没被抄第二遍，
而下一个人加第三条壳时最先做的正是复制粘贴。"
```

---

### Task 4: `POST /setup` 与首页三形状的接线

**Files:**
- Modify: `internal/httpapi/setup.go`（真实现 + `redirectSetup` + `setupFieldKey`）
- Modify: `internal/httpapi/handlers.go`（`handleRoot` 的判据；`pinNotices` 加两键）
- Modify: `internal/httpapi/routes.go`（注册）
- Modify: `internal/httpapi/openapi.json`、`AGENTS.md`
- Test: `internal/httpapi/bootstrap_e2e_test.go`

**Interfaces:**
- Consumes: `Store.CountProfiles()`、`Store.NextProfileID()`（Task 1）、`PickerOptions.Heading/Setup`（Task 2）、`s.createProfile`（Task 3）、`redirectPicker`、`maxBodyBytes`
- Produces:
  - `func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request)`（已注册）
  - `func redirectSetup(w http.ResponseWriter, code string)`
  - `func setupFieldKey(errs []fieldError) string`
  - `pinNotices` 新增键：`noname`、`badpin`

- [ ] **Step 1: 写失败测试——首页三形状（走 handler，不是走渲染器）**

`internal/httpapi/bootstrap_e2e_test.go`：

```go
package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/auth"
	"github.com/rockbenben/launchbay/internal/store"
)

// setupZero 是零档案安装：同一个 newDeps，一个档案都不建。
func setupZero(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	s := store.New(t.TempDir())
	scopes, err := auth.ParseScopes("admin:tok-admin:*")
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
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
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
}
```

- [ ] **Step 2: 跑测试确认它红**

Run: `go test ./internal/httpapi/ -run 'TestHome_' -count=1`
Expected: FAIL——三条分别红在「`action="/setup"` 不在页面里」「含 `谁来用`」「`<h1>爸爸</h1>` 不在」，而不是 404/编译错。

- [ ] **Step 3: 写实现——`handleRoot` 的判据**

`internal/httpapi/handlers.go` 里 `if sess.IsEmpty() {` 那一支替换为：

```go
	if sess.IsEmpty() {
		// 三形状的判据在这里算一次，模板只印（设计 §2）。
		// 用 CountProfiles（目录数）而不是 ListProfiles：坏档案也占位，
		// 否则"装坏一个"就把引导窗口重新打开（Task 1 的同一条纪律）。
		n, err := s.deps.Store.CountProfiles()
		if err != nil {
			s.handleError(w, err)
			return
		}
		opts := render.PickerOptions{KeepAdd: keepAdd, Notice: notice}
		switch {
		case n == 0:
			opts.Setup, opts.Heading = true, "还没有档案"
		case n == 1:
			// 单人安装不再先问"谁"：标题直接是那个人的名字。名字读不出来
			// （档案坏了）时不编造，用一句中性话——这一页本来就要输 PIN。
			opts.Heading = s.onlyProfileTitle()
		default:
			opts.Heading = "谁来用？"
		}
		page, err := s.deps.Render.PickerPage(opts)
		if err != nil {
			s.handleError(w, err)
			return
		}
		s.writePage(w, r, page)
		return
	}
```

同文件加：

```go
// onlyProfileTitle 取唯一那个档案的显示名；读不出来就退回一句中性的话，
// 不编造名字也不空着。判据（有几个档案）在 handleRoot 已经算过，这里只是
// 取名字——所以它不改判据，也就不违反"判据只有一份"。
func (s *Server) onlyProfileTitle() string {
	ps, err := s.deps.Store.ListProfiles()
	if err != nil || len(ps) != 1 || strings.TrimSpace(ps[0].Title) == "" {
		return "你的书签"
	}
	return ps[0].Title
}
```

- [ ] **Step 4: 跑测试确认三条全绿**

Run: `go test ./internal/httpapi/ -run 'TestHome_' -count=1`
Expected: PASS。

- [ ] **Step 5: 写失败测试——`POST /setup` 的五条判据**

追加到 `bootstrap_e2e_test.go`：

```go
// 引导建档：设计 §3、§9 的 2/3/4/5 条。
func TestSetup_零档案建成管理员(t *testing.T) {
	s, h := setupZero(t)
	rec := postForm(h, "/setup", "title=%E7%88%B8%E7%88%B8&pin=1234", "") // 爸爸
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("建成要 303 回 /，得到 %d Location=%q", rec.Code, rec.Header().Get("Location"))
	}
	p, err := s.LoadProfile("home") // 中文名 → 基名序列（Task 1）
	if err != nil {
		t.Fatalf("引导建档没落盘: %v", err)
	}
	if !p.Admin {
		t.Error("引导建成的第一个档案必须是管理员，否则没人能进管理页加人")
	}
	if !strings.HasPrefix(p.PIN, "sha256$") {
		t.Errorf("PIN 要经 HashPIN 存成三段式，得到 %q", p.PIN)
	}
}

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
	if _, err := s.CreateProfile(store.NewProfileInput{ID: "home", Title: "爸爸", PinHash: "sha256$a$b"}); err != nil {
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
	cases := map[string]string{
		"title=&pin=1234":    "noname",
		"title=%E7%88%B8&pin=12":  "badpin",
		"title=dad&pin=abc12":     "badpin",
	}
	for body, want := range cases {
		rec := postForm(h, "/setup", body, "")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%q 要 303，得到 %d", body, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/?err="+want {
			t.Errorf("%q 的 Location = %q，要 /?err=%s", body, loc, want)
		}
	}
}

// 白名单纪律：页面只能出现表里登记过的句子。用户可控原文一个字都不许进页面。
func TestSetup_提示只走白名单(t *testing.T) {
	for _, k := range []string{"noname", "badpin"} {
		if pinNotices[k] == "" {
			t.Errorf("pinNotices 缺键 %q：引导页会说不出话，或者更糟——开始复读输入", k)
		}
	}
	_, h := setupZero(t)
	rec := get(h, "/?err=%E4%B8%8D%E6%98%AF%E9%94%AE", "") // ?err=不是键
	if strings.Contains(rec.Body.String(), "不是键") {
		t.Error("未登记的 ?err= 值被复读进页面了（反射点）")
	}
}

func dirCount(t *testing.T, s *store.Store) int {
	t.Helper()
	n, err := s.CountProfiles()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// "判据只有一份"在引导这条路上的形状：store 锁内那一次是真的，handler 里
// **不该有第二次**——数两次会得出两个答案，而"谁有资格写"不能有第二个来源。
// 源码级断言（不是行为断言），因为 handler 多查一次**不改行为**：
// 行为测试对这种复制粘贴完全无感（P1-④ 评审的同一课）。
func TestSetup_handler不数档案(t *testing.T) {
	raw := readGoFile(t, "setup.go")
	if strings.Contains(raw, "CountProfiles") {
		t.Error("setup.go 里出现了 CountProfiles：引导判据被抄进 handler，成了第二处")
	}
	if !strings.Contains(raw, "FirstOnly: true") {
		t.Error("handleSetup 没把 FirstOnly 递下去：窗口是否在锁内成立就没人管了")
	}
}
```

- [ ] **Step 6: 临时骨架先立一次（`func (s *Server) handleSetup(...) { redirectPicker(w) }`），跑测试并诚实记账**

Run: `go test ./internal/httpapi/ -run 'TestSetup' -count=1`
Expected: 三条红、两条恰好绿。这个临时骨架（Step 7 就要被真实现替掉）的行为是
「什么都拒」，所以：

| 测试 | 现在 | 红在哪句 |
|---|---|---|
| `TestSetup_零档案建成管理员` | 红 | `引导建档没落盘`（303 对了，磁盘上没东西） |
| `TestSetup_字段错说人话` | 红 | `Location = "/"，要 /?err=noname` |
| `TestSetup_handler不数档案` | 红 | `没把 FirstOnly 递下去` |
| `TestSetup_成功不发会话` | **绿** | 骨架本来就不发 cookie——它是防回归证人，不是驱动实现的证人 |
| `TestSetup_建完这条通路自己关` | **绿** | 同上 |

「成功不发会话」和「关窗不解释」这两条在 RED 阶段就是绿的，这是形状使然（骨架只会拒，不会写）；
把它们留在测试集里的理由是 Task 4 的实现**很容易顺手写错成发会话**（Step 12 的破坏 #2 就是这条路）。
不许因为"它已经绿了"就把它当已验证——Step 12 每条破坏都要由执行者自己跑一遍。

- [ ] **Step 7: 写实现——`setup.go` 的真实形状**

`internal/httpapi/setup.go`（本任务新建；`handleAdminCreate` 到 Task 5 才加）：

```go
package httpapi

import (
	"net/http"
	"strings"
)

// handleSetup 是引导建档：零档案时任何人建第一个（建成管理员），建完这条
// 通路自己关闭。三条纪律都在代码里，不在注释里：
//
//   - handler **不数档案**：判据在 store 的锁内（CreateFirstProfile，Task 1），
//     数两次会得出两个答案，而"谁有资格写"这种问题不能有第二个答案的来源；
//   - 成功只回 303，**不发会话**：免令牌建档不等于免令牌进入（设计 §3）；
//   - 拒的时候分两种：窗口已关 → 303 回 `/` 不解释；字段没填对 → 303 带
//     `?err=` 白名单键（引导页是给人看的，不该吐 JSON）。
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		redirectSetup(w, "noname")
		return
	}
	// 同名多值不是"取第一个"，是没说清要改什么（与 /prefs 同一条纪律）。
	if len(r.Form["title"]) != 1 || len(r.Form["pin"]) != 1 {
		redirectSetup(w, "noname")
		return
	}
	res := s.createProfile(createInput{
		Title: strings.TrimSpace(r.FormValue("title")),
		PIN:   r.Form.Get("pin"),
		Admin: true, // 引导建成的就是这台盒子的主人；表单里根本没有这一格
		FirstOnly: true,
	})
	switch {
	case len(res.FieldErrs) > 0:
		redirectSetup(w, setupFieldKey(res.FieldErrs))
	case res.Status == http.StatusConflict:
		redirectPicker(w) // 窗口已关：不解释（判据来自 store，不是这条 handler 的猜测）
	case res.Status != 0:
		writeInternalError(w, errors.New(res.Detail)) // 落盘失败仍走 JSON 外壳：它不是权限拒
	default:
		redirectPicker(w) // 成功也回 /，接着就是单人进门页（照样要 PIN）
	}
}

// redirectSetup 把浏览器能懂的失败送回引导页。code 只来自本包的常量，
// 永远不是用户可控的字符串——页面那一头还要过 pinNotices 白名单。
func redirectSetup(w http.ResponseWriter, code string) {
	w.Header().Set("Location", "/?err="+code)
	w.WriteHeader(http.StatusSeeOther)
}

// setupFieldKey 把 createProfile 的字段错翻成 ?err= 的键。建档的非法输入只有
// 两种（名字、PIN），所以这里是一张小表而不是拼接。
func setupFieldKey(errs []fieldError) string {
	for _, e := range errs {
		if e.Field == "pin" {
			return "badpin"
		}
	}
	return "noname"
}
```

import 需要补 `"errors"`（`writeInternalError` 那一行用到）。

- [ ] **Step 8: `pinNotices` 加两键**

`internal/httpapi/handlers.go` 的表补两行（措辞是给家里人看的，不是给 AI）：

```go
	"noname": "要先给这个档案起个名字。",
	"badpin": "PIN 要 4-6 位数字。",
```

- [ ] **Step 9: 注册路由 + 把壳加进静态证人名单**

`createrule_test.go` 的 `shells` 加上 `"handleSetup"`、文件名单加上 `"setup.go"`，
再跑一次 `Test建档判据只有一处`：它必须仍然 PASS（新壳里没有那三个符号）。

`internal/httpapi/routes.go` 的表里，紧跟 `POST /pin` 那行之后插入：

```go
		{Method: "POST", Pattern: "/setup", Cred: credNone,
			Summary: "引导建档：只在磁盘零档案时有效（浏览器表单通路，建成即管理员）", H: s.handleSetup},
```

- [ ] **Step 10: 契约三处同步（openapi + AGENTS.md）**

`openapi.json` 的 `paths` 里新增（放在 `/pin` 之后，形状照 `/prefs` 那条浏览器通路）：

```json
    "/setup": {
      "post": {
        "summary": "引导建档：只在磁盘零档案时有效（浏览器表单通路，建成即管理员）",
        "description": "浏览器表单通路，AI 不走这条。**唯一判据是 profiles/ 下的目录数为 0**（坏档案也占位）。体是 application/x-www-form-urlencoded 的 {title, pin}：不问 id（由名字派生 slug，中文走 home/home-2 基名序列），admin 不由表单决定——固定建成管理员。成功与失败都回 303：成功回 `/`（**不发会话**，进门照样要 PIN）；字段错回 `/?err=noname|badpin`；窗口已关回 `/` 且不带 err，因为「这台机器已经建过档了」本身就是枚举线索。落盘失败是 500 JSON 外壳（与 /prefs 的 saveErr 同一支：503 留给"文件读不了"，写不进去是内部错误）。\n\n这条通路是**新增**的免令牌写通路，设计 2026-10-08-launchbay-first-run-design.md §3、§11 把它的边界与残余风险写死：窗口只开得下一个人（判据与写入同属 store 的一次串行化），建完永久关闭。",
        "x-credential": "none",
        "requestBody": {
          "required": true,
          "content": {
            "application/x-www-form-urlencoded": {
              "schema": {
                "type": "object",
                "properties": {
                  "title": {"type": "string"},
                  "pin": {"type": "string"}
                }
              }
            }
          }
        },
        "responses": {
          "303": {"description": "成功或表单错：回 `/`（表单错带 ?err=noname|badpin）"},
          "500": {"description": "落盘失败，JSON 外壳"}
        }
      }
    },
```

`AGENTS.md` 的端点表在 `POST /pin` 那一段之后加一行（**五格，与其他行等长**）：

```
| POST | `/setup` | 无（仅磁盘零档案时有效） | 303 回 `/` | 引导建档，浏览器表单通路：**AI 不走这条**。体是 `application/x-www-form-urlencoded` 的 `{title, pin}`，不问 id、admin 不由表单决定（固定建成管理员）；成功**不发会话**（进门照样要 PIN），字段错回 `/?err=noname`或`badpin`，窗口已关回 `/` 不带 `err`。AI 要建档仍走 `POST /api/v1/profiles`（带 `id`，可 `fromTemplate`） |
```

并在「禁止事项」之前那句浏览器通路的话里补一句：`/setup` 是零档案时的一次性通路，**不是第二条 API 面**，脚本别去 POST 它（有档案之后它只会 303）。

- [ ] **Step 11: 跑契约闸与全量**

Run: `go test ./... -count=1`
Expected: PASS。若 `TestAgentsMDMatchesRoutes` 或 `TestAgentsMDMethodColumn` 红，说明表格行少格/方法列没写；`TestOpenAPI的枚举与model同集` 不该受影响。

- [ ] **Step 12: 五次【故意破坏】并还原**

| # | 破坏 | 必须变红 |
|---|---|---|
| 1 | `handleSetup` 里在 createProfile 之前自己数一次档案（`if n, _ := s.deps.Store.CountProfiles(); n > 0 { ... }`） | `TestSetup_handler不数档案`——**行为测试全绿**，因为多查一次不改结果；这正是它必须是源码级证人的原因 |
| 2 | 成功分支加一行 `auth.WriteSession(w, "home", "home")` | `TestSetup_成功不发会话` |
| 3 | 窗口已关那支改成 `redirectSetup(w, "closed")` | `TestSetup_建完这条通路自己关`（Location 不该带 err） |
| 4 | `pinNotices` 删掉 `badpin` 键（handler 照发键，页面说不出话） | `TestSetup_提示只走白名单` |
| 5 | 路由注册里删掉 `/setup` 那行 | `TestSetup_零档案建成管理员`（404）+ `TestAgentsMDMatchesRoutes` 反向闸 |

- [ ] **Step 13: 门禁四条 + 提交**

```bash
git add internal/httpapi/ AGENTS.md
git commit -m "feat(httpapi): POST /setup 引导建档 + 首页三形状接线

零档案的人第一次打开就是一条死路（空列表 + 只能命令行建档）。现在首页按
目录数分三形状：0 画引导表单、1 直接是那人的进门页、≥2 逐字不变。
/setup 三条纪律都落在可测的地方：判据只在 store 锁内（handler 不数档案，
源码级证人）、成功不发会话、拒的时候不解释。openapi 与 AGENTS.md 同批改，
AI 建档仍只有一条 API 通路。"
```

---

### Task 5: `POST /admin` 管理页建档

**Files:**
- Modify: `internal/httpapi/setup.go`（把 `handleAdminCreate` 骨架换成真实现；或移到 `adminpage.go`，二选一并保持静态证人能解析到——**静态证人按文件名单解析，函数名不变即可**）
- Modify: `internal/httpapi/routes.go`、`internal/httpapi/openapi.json`、`AGENTS.md`
- Modify: `internal/render/render.go`（`AdminPage(rows, notice)`）、`internal/render/exec.go`（`adminView` 加 `Notice`）、`internal/render/web/admin.tmpl`
- Test: `internal/httpapi/adminpage_test.go`、`internal/render/views_test.go`

**Interfaces:**
- Consumes: `s.require(r, credAdmin, "")`、`bearer(r)`、`s.createProfile`、`redirectPicker`、`redirectSetup` 的形状（新增 `redirectAdminFail(w, code)`）
- Produces:
  - `func (s *Server) handleAdminCreate(w http.ResponseWriter, r *http.Request)`
  - `func redirectAdminFail(w http.ResponseWriter, code string)`
  - `func (r *Renderer) AdminPage(rows []AdminRow, notice string) (Page, error)` ← **签名变更**，全部调用点同批改

- [ ] **Step 1: 写失败测试——管理页的表单与 notice**

`internal/httpapi/adminpage_test.go` 追加：

```go
func TestAdmin_表单建出普通档案(t *testing.T) {
	s, h := setupBatch(t) // dad=admin 会话可用，kid 是普通档案
	before := dirCount(t, s)
	rec := postForm(h, "/admin", "title=%E5%A6%88%E5%A6%88&pin=5678", dadFormCookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("建档成功要 303，得到 %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin" {
		t.Errorf("成功该回管理页，Location = %q", loc)
	}
	if got := dirCount(t, s); got != before+1 {
		t.Fatalf("磁盘上该多出一个档案：%d → %d", before, got)
	}
	// 中文名字 → 基名序列里第一个空位（Task 1 的规则在真实通路上生效）。
	p, err := s.LoadProfile("home")
	if err != nil {
		t.Fatalf("新档案没落在预期位置: %v", err)
	}
	if p.Admin {
		t.Error("不勾「能管理全家」就建成管理员了——勾和没勾得是两个答案")
	}
}

func TestAdmin_勾选才建管理员(t *testing.T) {
	s, h := setupBatch(t)
	if rec := postForm(h, "/admin", "title=nas&pin=5678&admin=1", dadFormCookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("得到 %d", rec.Code)
	}
	p, err := s.LoadProfile("nas")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Admin {
		t.Error("勾了「能管理全家」却没建成管理员")
	}
}

func TestAdmin_表单通路不认令牌(t *testing.T) {
	s, h := setupBatch(t)
	before := dirCount(t, s)
	// * 令牌有管理员权限，但这是浏览器通路：与 /prefs、/switch 同族，一律拒。
	rec := post(h, "/admin", `{"title":"x","pin":"1234"}`, "", adminTok)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("令牌打 POST /admin 要 303，得到 %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/" {
		t.Errorf("要回选择页，Location = %q", rec.Header().Get("Location"))
	}
	if got := dirCount(t, s); got != before {
		t.Errorf("令牌那一路在磁盘上留下了东西：%d → %d", before, got)
	}
}

func TestAdmin_非管理员与无会话都不写盘(t *testing.T) {
	s, h := setupBatch(t)
	before := dirCount(t, s)
	for _, ck := range []string{"", kidFormCookie} {
		rec := postForm(h, "/admin", "title=x&pin=1234", ck)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
			t.Errorf("cookie=%q：要 303 回 /，得到 %d Location=%q",
				ck, rec.Code, rec.Header().Get("Location"))
		}
	}
	if got := dirCount(t, s); got != before {
		t.Errorf("两种拒都在磁盘上留下了东西：%d → %d（判据是磁盘，不是状态码）", before, got)
	}
}

func TestAdmin_字段错回带键的303(t *testing.T) {
	_, h := setupBatch(t)
	cases := map[string]string{"title=&pin=1234": "noname", "title=dad&pin=1": "badpin"}
	for body, want := range cases {
		rec := postForm(h, "/admin", body, dadFormCookie)
		if loc := rec.Header().Get("Location"); loc != "/admin?err="+want {
			t.Errorf("%q 的 Location = %q，要 /admin?err=%s", body, loc, want)
		}
	}
}

// notice 必须真的印出来：管理员看不见"刚才那条没建上"就等于表单静默失败。
func TestAdmin_提示在场(t *testing.T) {
	_, h := setupBatch(t)
	rec := get(h, "/admin?err=badpin", dadFormCookie)
	if !strings.Contains(rec.Body.String(), pinNotices["badpin"]) {
		t.Errorf("管理页要能说出登记的失败:\n%.400s", rec.Body.String())
	}
	rec2 := get(h, "/admin?err=%E4%B8%8D%E6%98%AF%E9%94%AE", dadFormCookie)
	if strings.Contains(rec2.Body.String(), "不是键") {
		t.Error("未登记的 ?err= 被复读进管理页了")
	}
}
```

- [ ] **Step 2: 跑测试确认它红——同样按真实形状记账，不夸大**

Run: `go test ./internal/httpapi/ -run 'TestAdmin_' -count=1`
骨架 `handleAdminCreate` 是"无条件 303 回 `/`"，所以新加的六条里只有三条现在红：

| 测试 | 现在 | 红在哪 |
|---|---|---|
| `TestAdmin_表单建出普通档案` | 红 | `Location = "/"`，要 `/admin`；且磁盘没多出目录 |
| `TestAdmin_勾选才建管理员` | 红 | `LoadProfile("nas")` 失败 |
| `TestAdmin_字段错回带键的303` | 红 | Location 不带 `?err=` |
| `TestAdmin_提示在场` | 红 | 管理页没有 notice 元素（`AdminPage` 还没接 `?err=`） |
| `TestAdmin_表单通路不认令牌` | **绿（蒙对）** | 上一任务的骨架把所有 POST 都当拒答，恰好对上；Step 9 的破坏 #1 才是让它带电的地方 |
| `TestAdmin_非管理员与无会话都不写盘` | **绿（骨架蒙对）** | 同上 |

那两条"蒙对"的是防回归证人，Step 9 的破坏 #1/#2 才是让它们真正带电的地方。
既有那几条（`TestAdmin_管理员拿到页面…` 等）必须**仍绿**，它们是本任务的行为基线。

- [ ] **Step 3: 写实现——`handleAdminCreate`**

在 `setup.go` 里新增 `handleAdminCreate`（与 `handleSetup` 同文件、同读一份判据；`errors`、`strings`、`net/http` 三个导入 Task 4 已经带上）：

同批把它加进 `createrule_test.go` 的 `shells`（`"handleAdminCreate"`），跑静态证人确认仍 PASS。

```go
// handleAdminCreate 是管理页的建档表单。判据一条都不新写：权限走现成的
// `require(credAdmin, "")`，校验/哈希/落盘走 `createProfile`（Task 3 抽的那个
// 唯一落点）。与引导那条的差别只有两处：要管理员会话、id 由名字派生。
//
// 浏览器通路不认令牌（/prefs、/switch 同族纪律）：机器建档有 API 那条路，
// 让令牌走这条只会多出一个"同一个动作两个入口、鉴权各写一半"的形状。
func (s *Server) handleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if bearer(r) != "" {
		redirectPicker(w)
		return
	}
	if _, _, ok := s.require(r, credAdmin, ""); !ok {
		redirectPicker(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		redirectAdminFail(w, "noname")
		return
	}
	if len(r.Form["title"]) != 1 || len(r.Form["pin"]) != 1 {
		redirectAdminFail(w, "noname")
		return
	}
	res := s.createProfile(createInput{
		Title: strings.TrimSpace(r.FormValue("title")),
		PIN:   r.Form.Get("pin"),
		Admin: r.FormValue("admin") == "1", // 只有勾了才是管理员；值由我们的模板写死
	})
	switch {
	case len(res.FieldErrs) > 0:
		redirectAdminFail(w, setupFieldKey(res.FieldErrs))
	case res.Status != 0:
		writeInternalError(w, errors.New(res.Detail))
	default:
		w.Header().Set("Location", "/admin")
		w.WriteHeader(http.StatusSeeOther)
	}
}

func redirectAdminFail(w http.ResponseWriter, code string) {
	w.Header().Set("Location", "/admin?err="+code)
	w.WriteHeader(http.StatusSeeOther)
}
```

- [ ] **Step 4: `handleAdminPage` 接 `?err=`**

`internal/httpapi/adminpage.go`：取 `notice := pinNotices[r.URL.Query().Get("err")]`，并把渲染那行改成 `s.deps.Render.AdminPage(views, notice)`。

- [ ] **Step 5: `AdminPage` 签名加 notice + 模板**

`internal/render/render.go`：

```go
// AdminPage 渲染 §8.8 的管理页。notice 是 `?err=` 白名单查出来的一句人话
// （空串就不画）——建档表单失败后要能告诉管理员刚才没建上，否则那条通路
// 是静默失败的。NoStore 与不给 ETag 的理由不变。
func (r *Renderer) AdminPage(rows []AdminRow, notice string) (Page, error) {
	body, err := r.tmpl.admin(rows, notice)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, ETag: makeETag(AdminSentinel, body), Profile: AdminSentinel, NoStore: true}, nil
}
```

`internal/render/exec.go` 的 `adminView` 加 `Notice string`，`executor.admin(rows []AdminRow, notice string)` 组装它。

`internal/render/web/admin.tmpl` 在表格之后、`</main>` 之前插入：

```gotemplate
  {{with .Notice}}<p class="notice" role="alert">{{.}}</p>{{end}}
  {{/* 建档表单：两格 + 一个勾选。id 不问（由名字派生），PIN 存哈希不存明文。
       勾选的后果写在同一行——"能管理全家"是能看与改所有人的书签，不是个装饰。
       设 PIN 与删档**不在这里**：那两个要二次确认，表单发不出，随 ⑤ 的 JS。 */}}
  <form method="post" action="/admin" class="adm newprofile">
    <label>怎么叫 <input name="title" type="text" maxlength="60" required autocomplete="off"></label>
    <label>PIN <input name="pin" type="password" inputmode="numeric" minlength="4" maxlength="6" required></label>
    <label class="chk"><input type="checkbox" name="admin" value="1"> 能管理全家（可以看与改所有人的书签）</label>
    <button type="submit">新建档案</button>
  </form>
```

`.adm.newprofile` 的输入框与按钮已被输入族/按钮族的 joint 规则罩住（`input[type=password|text|search]`、`.adm button`），**不新增 44 位点**；只补 `.chk` 的排布。

- [ ] **Step 6: 改全部 `AdminPage` 调用点并跑全量**

```bash
grep -rn "AdminPage(" --include=*.go .
```
逐个补第二参数（测试里传 `""`），然后 `go test ./... -count=1` 全绿。这一步是签名变更的代价，**必须同批做完**，不许留 `AdminPage(rows)` 与 `AdminPage(rows, "")` 两种形状。

- [ ] **Step 7: 注册路由 + 契约两处同步**

`routes.go`，紧跟 `GET /admin` 那行之后：

```go
		{Method: "POST", Pattern: "/admin", Cred: credAdmin,
			Summary: "管理页建档（浏览器表单通路；机器建档走 POST /api/v1/profiles）", H: s.handleAdminCreate},
```

`openapi.json` 的 `/admin` path 对象里加 `"post"`，`x-credential: "session"`，description 写明：表单通路、令牌一律拒、字段 `{title, pin, admin?}`、成功 303 回 `/admin`、失败 303 带 `?err=`。

`AGENTS.md` 表格加一行（五格）：

```
| POST | `/admin` | 仅管理员会话（表单） | 303 回 `/admin` | 管理页建档，体是 `application/x-www-form-urlencoded` 的 `{title, pin, admin?}`：`admin` 只有勾了才建管理员；**令牌一律拒**（要机器建档走 `POST /api/v1/profiles`），非管理员与无会话都是 303 回选择页且磁盘不变 |
```

同时把 GET `/admin` 那行说明里那句「坏档案不给『进入编辑』」旁边补一句：这一页现在有建档表单（读侧 + 建档，设 PIN/删档仍在 ⑤）。

- [ ] **Step 8: 补渲染侧形状证人（在破坏之前——下面 #4 要用它）**

`internal/render/views_test.go` 加：

```go
// 管理页的建档表单与提示（形状层面；行为在 httpapi 那侧）。
func TestAdminPage_建档表单与提示在场(t *testing.T) {
	ix, fc, s := assets(t)
	r := New(s, ix, fc)
	page, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸", Admin: true}}, "PIN 要 4-6 位数字。")
	if err != nil {
		t.Fatal(err)
	}
	body := string(page.Body)
	for _, want := range []string{`action="/admin"`, `name="title"`, `name="pin"`, `type="checkbox"`, `能管理全家`} {
		if !strings.Contains(body, want) {
			t.Errorf("管理页建档形状缺 %q:\n%.400s", want, body)
		}
	}
	if !strings.Contains(body, `class="notice"`) {
		t.Error("给了 notice 就该画出来：静默失败的表单等于没告诉管理员刚才没建上")
	}
	// 空 notice 不画空元素（DOM 噪声也是噪声）。
	plain, err := r.AdminPage([]AdminRow{{ID: "dad", Title: "爸爸"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain.Body), `class="notice"`) {
		t.Errorf("没有提示时不该渲染提示元素:\n%s", plain.Body)
	}
}
```

- [ ] **Step 9: 六次【故意破坏】并还原（先删掉临时骨架，逐条破坏真实现）**

| # | 破坏 | 必须变红 |
|---|---|---|
| 1 | 删掉 `if bearer(r) != ""` 那道闸 | `TestAdmin_表单通路不认令牌` |
| 2 | `Admin:` 改成恒 `true` | `TestAdmin_表单建出普通档案` |
| 3 | `redirectAdminFail` 拼上 `r.FormValue("title")`（把用户原文当键） | `TestAdmin_提示在场` 的未登记值那半 + `TestSetup_提示只走白名单`（同源纪律）——如果它没红，说明 notice 断言只查了正例，**回去补反例**（这条本身就是评审点过的 want=0 教训） |
| 4 | 模板里删掉 `<form … newprofile>` 整块 | `TestAdminPage_建档表单与提示在场`（形状）+ `TestAdmin_表单建出普通档案`（行为） |
| 5 | `handleAdminPage` 不再传 notice（恒传空串） | `TestAdmin_提示在场` + `TestAdminPage_建档表单与提示在场` 的 notice 那半 |
| 6 | 模板里 checkbox 的 `value="1"` 改成 `value="on"`（handler 比的是 `== "1"`） | `TestAdmin_勾选才建管理员`——模板与 handler 之间那个约定值是一对接件，改一边必须让另一边喊出来；这条不需要静态证人，行为证人就够 |

- [ ] **Step 10: 门禁四条 + 提交**

```bash
git add internal/httpapi/ internal/render/ AGENTS.md
git commit -m "feat(httpapi,render): POST /admin 管理页建档 + 提示行

建档权从"管理员令牌"扩到"管理员会话"其实不新：handleCreateProfile 早已走
require(credAdmin)。这一笔只是给它接上表单外壳：判据仍只在 createProfile 一处，
勾选「能管理全家」才建管理员，令牌打这条 303 拒（浏览器通路不认令牌）。
AdminPage 签名加 notice——失败不吭声的表单等于没告诉管理员刚才没建上。"
```

---

### Task 6: 权威源改字（设计 §8 的清单）

**Files:**
- Modify: `docs/superpowers/specs/2026-10-05-launchbay-design.md`
- Modify: `docs/superpowers/specs/2026-10-08-launchbay-first-run-design.md`（回填"已落地"与提交号）

**Interfaces:**
- Consumes: 设计 §8 的六处清单
- Produces: 权威源与本设计一致；`AGENTS.md` 与之不矛盾（Task 4/5 已同步）

- [ ] **Step 1: 逐处定位（先 grep，别凭印象改）**

```bash
grep -n "档案选择页（无 cookie 首屏）\|仅管理员 |$\|随 ⑤ 的 JS 落地\|建档与发 PIN 是一次性人工动作\|明写不防" \
  docs/superpowers/specs/2026-10-05-launchbay-design.md
```

- [ ] **Step 2: 改 §8 第 0 条（三形状）**

把那一行整条替换成：

```markdown
0. **进门页**（无 cookie 首屏）三形状，判据是 `profiles/` 下的目录数（坏档案也占位，
   所以不是"有几个读得动的档案"）：**0 个** → 引导建档表单（`POST /setup`，名字 + PIN，
   建成即管理员，成功不发会话）；**1 个** → 直接是那个人的进门页（h1 就是他的名字，
   不再问「谁来用？」）；**≥2 个** → 一排档案卡（首字母头像 + 名字 + PIN），带 `?add=`
   进来的穿过 PIN 步骤不丢。整页零条目、零 URL、零分组名；三种形状都 no-store。
   "多用户"不是配置项——建了第二个档案才是多人。
```

- [ ] **Step 3: 改 §8 第 8 条（管理页）**

把「设 PIN / 建档 / 删档三个动作要发 JSON，表单发不出……随 ⑤ 的 JS 落地，这一版**按钮不渲染**」替换成：

```markdown
   动作：进入编辑（④）、新建档案（④.5，`POST /admin` 表单，勾「能管理全家」才建管理员；
   令牌打这条一律 303——机器建档走 `POST /api/v1/profiles`）。设 PIN 与删档要二次确认，
   表单发不出，仍随 ⑤ 的 JS 落地，这一版按钮不渲染。建档这条**不是第二条 API 面**：
   它与 API 共用同一个 `createProfile`（校验→哈希→落盘→错误分诊只有一份），
   差别只在怎么读入、怎么答出。
```

- [ ] **Step 4: 改 §6.1 端点表与 §4.5**

`/api/v1/profiles` 那行的「仅管理员」后面补：「（管理员会话亦可，经 `POST /admin` 表单；另有零档案时的一次性通路 `POST /setup`，见 §11）」。
§4.5「CLI 只经本机 REST」那段末尾补一句：「引导建档的判据（磁盘为空）也在 store 的同一次串行化里查——handler 不数档案，两处数会得出两个答案。」

- [ ] **Step 5: 改 §11（防这些 / 明写不防）**

「防这些」表加一行：

```markdown
| 零档案时被抢装 | 抢到人就是这台盒子的管理员 | 判据（目录数）与写入同属 store 的一次串行化，窗口只开得下一个人；建完永久关闭；**引导成功不发会话**，进门照样要 PIN |
```

「明写不防」加一条：

```markdown
- **不为引导窗口加安装令牌、二次确认页或速率限制。** 后果明写：装它的人要在场；
  早到一步的人能当第一个档案的主人（他设自己的 PIN，你只能删档重来）。多出来的
  防护在这一档局域网定位里也算失败。
```

- [ ] **Step 6: 改 §12 与 §15、§13**

§12 坏档案那节补一句：「隔离不影响引导判据——判据数目录，坏档案照样占位。」
§15 那句「建档与发 PIN 是一次性人工动作：管理员在管理页建好」后面注明「（④.5 落地：零档案时首页直接建第一个；管理页建其余）」。
§13 验收清单追加本设计 §9 的 11 条（按现有条目风格逐条写，不写「见设计」这种空转引用）。

- [ ] **Step 7: 回填本设计文件**

`docs/superpowers/specs/2026-10-08-launchbay-first-run-design.md` 顶部加一行状态：「状态：已落地（提交号见 §9 之后的修复轮记录）」；§8 改字清单每条后面标注实际改成的节号。

- [ ] **Step 8: 一致性自查（这是文档，不是代码，所以自查用 grep）**

```bash
grep -c "谁来用" docs/superpowers/specs/2026-10-05-launchbay-design.md
grep -n "POST /setup\|POST /admin\|CountProfiles" docs/superpowers/specs/2026-10-05-launchbay-design.md | head
```
Expected：「谁来用」只出现在"≥2 形状"与示例文案里，不再作为唯一首页描述；三处新增判据都在。

- [ ] **Step 9: 提交**

```bash
git add docs/superpowers/specs/
git commit -m "docs(specs): 权威源把进门页与建档的口径改成三形状

§8 第 0 条以前只写"一排档案卡"，第 8 条又把建档推给 ⑤ 的 JS，
而 §15 早就写着"管理员在管理页建好"——两处相反的话让 ④ 收尾时
留了一张空列表死路。这一笔按首次运行设计消掉矛盾：判据是目录数
（坏档案也占位），建档与 API 共用同一份 createProfile 判据、不是第二条 API 面，
§11 明写不为引导窗口加令牌/确认/限速。"
```

---

### Task 7: 端到端验收与实拍

**Files:**
- Create: `docs/superpowers/acceptance/p1-45-first-run/2026-10-08-验收.md`
- Create: 同目录实拍 WebP（≥4 张）

**Interfaces:**
- Consumes: Task 1-6 的全部产物
- Produces: 设计 §9 的 11 条逐条结果 + 门禁与镜像读数

- [ ] **Step 1: 起一个真正干净的实例（空数据目录，不是仓库 `data/`）**

```bash
CGO_ENABLED=0 go build -trimpath -o .superpowers/tmp/p45/launchbay.exe ./cmd/launchbay
mkdir -p .superpowers/tmp/p45/clean
LAUNCHBAY_PORT=17620 LAUNCHBAY_DATA_DIR="$PWD/.superpowers/tmp/p45/clean" \
  LAUNCHBAY_WRITE_TOKENS='probe:localprobe:*' .superpowers/tmp/p45/launchbay.exe &
```
Expected: `curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:17620/` → 200，且页面含 `action="/setup"`。

- [ ] **Step 2: 走设计 §9 的 1–9 条（HTTP 层，逐条记录命令与读数）**

顺序与断言：`GET /`（引导形状）→ `POST /setup` 建「爸爸 / 1234」→ 检查 `.superpowers/tmp/p45/clean/profiles/home/profile.json` 的 `admin:true` 与 `pin:"sha256$…"`（不是仓库 `data/`） → `GET /`（h1 是「爸爸」，无「谁来用？」）→ 响应头无 `lb_profile` → 再 POST 一次 `/setup` 得到不带 err 的 303 且磁盘目录数不变 → `POST /pin` 拿会话 → `GET /admin` → `POST /admin` 建「妈妈」（无勾选，`admin:false`）与 `nas`（勾选，`admin:true`）→ `GET /` 变回「谁来用？」三张卡。每条都要写实际读数，不许写「符合预期」。

- [ ] **Step 3: 第 10 条——实拍（人在浏览器里，不是断言）**

用 chrome-devtools MCP：1280×900 引导页、1280×900 单人进门页、1280×900 管理页（含建档表单与勾选那行）、380px 引导页。存 WebP 到验收目录，命名 `p45-a-setup-1280.webp` 等。逐张写明看见了什么（h1 文案、两个输入框、提示句、勾选标签全文）。**闭环要求：拍完 Read 回来看一眼，不只看文件大小。**

- [ ] **Step 4: 第 11 条——三形状各一份首屏体积**

```bash
go test ./internal/render/ -run Budget -v -count=1
```
Expected: 引导页与单人页都在 25 条操作线内；三条线**一条不抬**；把 raw/gzip 读数写进验收文档。

- [ ] **Step 5: 全部门禁 + Linux + 镜像**

```bash
gofmt -l . && go vet ./... && go test ./... -count=1 && CGO_ENABLED=0 go build ./...
go run ./tools/leakguard
```
Linux 连跑 5 遍 `internal/store internal/httpapi internal/render`（`golang:1.27-alpine`，每遍 `-count=1`），并 `go test ./internal/store/ -race -count=2` 一次；`docker build -t launchbay:p145 .`，**读构建日志本身**确认 `RUN go vet && go test && go build` 那一层是 DONE 而不是 CACHED。

- [ ] **Step 6: 写验收文档并提交**

文档结构照 P1-④ 那份：一、逐条结果表（11 条 + 每条实测）；二、机器读数（体积、门禁、Linux、镜像）；三、实拍清单；四、测量方法账（这次踩到的工具/流程坑，明写）；五、没做的（诚实清单）；六、交付判定。

```bash
git add docs/superpowers/acceptance/p1-45-first-run/
git commit -m "docs(acceptance): 首次运行与默认单人的端到端验收

干净空目录起服务走完整首次流程：建第一个 → 输 PIN → 管理页加两人 →
首页退回三形状。Linux 连跑与镜像构建层各自复现一遍。"
```

---

## 交付判定（整个 ④.5 完成的样子）

1. `go test ./... -count=1` 全绿，且设计 §9 的 11 条各有证人（第 10/11 条是体积与实拍，不是断言）。
2. 空数据目录起服务，`GET /` 是引导表单；建完第一个档案后 `GET /` 是那个人名字的进门页；不存在任何一条路径能在有档案之后经 `/setup` 写入。
3. 引导成功与建档失败都**不发会话**；`POST /setup` 与 `POST /admin` 的失败长相都是 303+白名单键，没有一条把用户原文吐回页面。
4. 建档判据在仓库里只有一份实现（AST 证人 + 行为证人两条都站着）。
5. 两条新路由过了全部契约闸：`routes()`、`openapi.json`、`AGENTS.md`（含方法列与枚举同集那两道新闸）。
6. 三张新页面 `<script` 计数为 0，体积在三条线内且没抬线。
7. 权威源与本设计一致，§15 与 §8.8 那处矛盾消解完毕。

## 遗留与交接（不藏）

- 设 PIN / 删档仍在 ⑤（要二次确认）。
- `home` 这个基名对中文安装是唯一的默认 id；若将来要让中文名字也有可读 id，唯一诚实的路是问一次 id（设计 §11.2 的取舍），不是拼音转写。
- 本片新增第一条免令牌写通路。以后再想加第二条，先回来改这份设计——`/setup` 只写「新建一个档案」这一件事，不许扩展成别的。
