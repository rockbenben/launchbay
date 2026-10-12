package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddUsageSortsByIDForAStableFile(t *testing.T) {
	s := New(t.TempDir())
	// PinHash 是 CreateProfile 的必填项（P1-①/8 的契约，snapshot_test.go 钉过），
	// 简报的测试代码漏了它，这里补上，其余逐字照抄。
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	// 故意乱序上报：文件里的顺序必须稳定，否则同内容两次写出不一致，
	// 违反"禁止用 map 产生输出键序"的同一族约束。
	u, unknown, err := s.AddUsage("dad", []string{"z", "a", "m"}, "2026-10-05T20:00:00+08:00")
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown) != 3 {
		t.Fatalf("这些 id 不在书签里，该全部报出来: %+v", unknown)
	}
	ids := []string{}
	for _, it := range u.Items {
		ids = append(ids, it.ID)
	}
	if strings.Join(ids, ",") != "a,m,z" {
		t.Fatalf("没排序: %+v", u.Items)
	}
	first, _ := os.ReadFile(s.UsagePath("dad"))
	// 磁盘字节序（评审 Important 4）：只断言上面那份内存顺序的话，把
	// sort.Slice 挪到 WriteFileJSON 之后三条断言照样全绿，而文件落的就是
	// z,a,m——"输出键序稳定"这条标题约束恰恰是内存证人咬不住的形状，
	// 只能按原始字节比位置。
	pos := map[string]int{}
	for _, id := range []string{"a", "m", "z"} {
		at := strings.Index(string(first), `"id": "`+id+`"`)
		if at < 0 {
			t.Fatalf("盘上找不到 id %q 的条目: %s", id, first)
		}
		pos[id] = at
	}
	if !(pos["a"] < pos["m"] && pos["m"] < pos["z"]) {
		t.Fatalf("盘上键序不是稳定的 a,m,z（排序该在写盘之前）: %s", first)
	}
	if _, _, err := s.AddUsage("dad", []string{"z"}, "2026-10-05T20:01:00+08:00"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(s.UsagePath("dad"))
	if string(first) == string(second) {
		t.Fatal("第二次上报该让 z 的计数变化")
	}
	// 磁盘形状是 §4.3 的 2 空格缩进，标量键后有一个空格（简报写的
	// `"clicks":2` 永远命不中，这里是原格式的证人）。
	if !strings.Contains(string(second), `"clicks": 2`) {
		t.Fatalf("z 该累计到 2: %s", second)
	}
}

// 空上报不该造出一个文件：没点过就是没有。
func TestEmptyUsageReportCreatesNothing(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddUsage("dad", nil, "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.UsagePath("dad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("空上报造出了文件: %v", err)
	}
}

// usage.json 坏了不许拖垮书签：LoadUsage 报 ErrCorrupt，
// AddUsage 在这种情况下**拒绝写**（不覆盖掉一份可能可修的坏文件）。
func TestBrokenUsageIsolation(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.UsagePath("dad"), []byte("{坏"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddUsage("dad", []string{"a"}, "t"); err == nil {
		t.Fatal("坏 usage 该拒绝写")
	}
	// 拒绝写 = 坏文件原样留在盘上：覆盖掉它就真修不回来了。
	raw, err := os.ReadFile(s.UsagePath("dad"))
	if err != nil || string(raw) != "{坏" {
		t.Fatalf("坏 usage 被改写了: %q %v", raw, err)
	}
	// 拖垮方向只许是 usage→书签这一头断掉：书签读照常。
	if _, err := s.LoadBookmarks("dad"); err != nil {
		t.Fatalf("坏 usage 拖垮书签读取了: %v", err)
	}
}

// 档案不存在：AddUsage 要把 LoadBookmarks 的 ErrNotFound 原样传出去，而且
// 一个字都不许落盘（评审 Critical）。原来那句 `!errors.Is(err, ErrNotFound)`
// 的吞法是照着"空档案也会 ErrNotFound"的误解写的——load.go:86 只在
// profile.json 同时缺失时才报 ErrNotFound，所以吞掉它换不来空档案，
// 只换来幽灵目录：profiles/<ghost>/ 里只躺一份 usage.json，ListProfiles
// 既列不出它、DELETE /profiles 也删不掉它，下一个用同一 id 建档的人
// 继承死人的点击史。
func TestAddUsageMissingProfileIsNotFoundAndCreatesNothing(t *testing.T) {
	s := New(t.TempDir())
	u, unknown, err := s.AddUsage("ghost", []string{"a"}, "t")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("该把 ErrNotFound 传出来, got %v", err)
	}
	if u != nil || unknown != nil {
		t.Fatalf("失败还返回半成品: %+v %v", u, unknown)
	}
	if _, err := os.Stat(s.ProfileDir("ghost")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("404 的档案被造出了目录: %v", err)
	}
	// 空上报不占写通路，同样一个字都不落。
	if _, _, err := s.AddUsage("ghost", nil, "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ProfileDir("ghost")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("空上报造出了幽灵目录: %v", err)
	}
}

// 上一条的对照组，也是"传播而不是吞掉"能成立的前提：档案存在但从没写过
// bookmarks.json 是**空档案**，不是**缺档案**（LoadBookmarks 给它
// EmptyBookmarks，不报 ErrNotFound）。上报照常记数、id 全进 unknown。
func TestAddUsageOnProfileWithoutBookmarksFileStillWorks(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	// CreateProfile 会连带写一份 rev 0 的 bookmarks.json（它那次 SaveBookmarks），
	// 所以这里手工删掉它，模拟"档案在、书签还没落过盘"（人或 AI 清了文件）。
	if err := os.Remove(s.BookmarksPath("dad")); err != nil {
		t.Fatal(err)
	}
	u, unknown, err := s.AddUsage("dad", []string{"dsm"}, "t")
	if err != nil {
		t.Fatalf("空档案不是缺档案，该照常记: %v", err)
	}
	if len(u.Items) != 1 || u.Items[0].ID != "dsm" || u.Items[0].Clicks != 1 {
		t.Fatalf("got %+v", u.Items)
	}
	if len(unknown) != 1 || unknown[0] != "dsm" {
		t.Fatalf("书签里没有的 id 该报出来: %v", unknown)
	}
}

// store 侧包含闸（评审 Important 2）：AddUsage / LoadUsage 要有 DeleteProfile
// 那两道闸（ASCII slug + profileDirWithin，合在 profileDirChecked 里）。
// HTTP 今天到不了这里——handler 先过一遍 slug 闸——但 Task 8 的裁定正是
// "handler 级闸不算数"，原语自己得站得住：没有闸时 AddUsage("..") 落在
// root/usage.json、AddUsage("") 落在 root/profiles/usage.json。
// 照 Task 8 的 helper 测法（profile_delete_test.go）直接喂非法 id 给原语，
// 并且**判错误身份**而不是判"有没有报错"：摘掉闸之后 LoadBookmarks 也会
// 因为 profile.json 不在而报 ErrNotFound，弱判据会假绿（ErrInvalid 才是
// "这个 id 根本不配去查盘"的那句答案）。
func TestUsageStoreGateRefusesOutOfBoundsID(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	// 两个漏点位置：各对应一类越界 id。
	leaks := []string{
		filepath.Join(s.Root(), "usage.json"),        // id=".."：Join 清到 data root
		filepath.Join(s.profilesDir(), "usage.json"), // id=""：档案目录塌成 profiles/
	}
	for _, id := range []string{"..", "../profiles", "/", "", "a/b", "dad/../../x"} {
		if _, _, err := s.AddUsage(id, []string{"a"}, "t"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("AddUsage(%q) 该被包含闸以 ErrInvalid 拒掉, got %v", id, err)
		}
		if _, err := s.LoadUsage(id); !errors.Is(err, ErrInvalid) {
			t.Fatalf("LoadUsage(%q) 该被包含闸以 ErrInvalid 拒掉, got %v", id, err)
		}
		// 闸必须排在"空上报早退"之前，否则非法 id 靠一次空上报就能溜过去。
		if _, _, err := s.AddUsage(id, nil, "t"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("AddUsage(%q, 空上报) 该被拒——闸不许排在早退之后, got %v", id, err)
		}
	}
	for _, p := range leaks {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("越界写入漏到了 %s", p)
		}
	}
	// 拒绝不等于"顺手把别人的档案改了"：dad 目录原样。
	if _, err := os.Stat(s.ProfilePath("dad")); err != nil {
		t.Fatalf("dad 的 profile.json 不见了: %v", err)
	}
}

// 同一次上报里重复出现的未知 id：每一次点击都算数，unknown 只报一条
// （评审裁定的 Minor——前端拿 unknown 是"这些 id 我不认识了"的清单，
// 不是计数器；重复项会让它以为删了同一条三次）。
func TestAddUsageDedupesUnknownButCountsEveryClick(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	u, unknown, err := s.AddUsage("dad", []string{"nope", "nope", "nope"}, "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown) != 1 || unknown[0] != "nope" {
		t.Fatalf("unknown 该去重: %v", unknown)
	}
	if len(u.Items) != 1 || u.Items[0].Clicks != 3 {
		t.Fatalf("三次点击都该算进 clicks: %+v", u.Items)
	}
}
