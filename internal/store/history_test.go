package store

import (
	"os"
	"strings"
	"testing"
)

func TestAppendHistoryWritesOneLinePerEntry(t *testing.T) {
	s := New(t.TempDir())
	// 第三条带中文与 & < ：纯 ASCII 夹具下"不许出现 \u"永远为真、
	// SetEscapeHTML(false) 从没被跑到（评审 Important #1）。
	entries := []HistoryEntry{
		{
			At: "2026-10-05T20:00:00+08:00", Profile: "dad", Actor: "ai-dad",
			By: "ai", Rev: 1,
			Ops: []HistoryOp{{Op: "set", Path: "groups/nas/items/dsm/name"}},
		},
		{
			At: "2026-10-05T20:00:01+08:00", Profile: "dad", Actor: "ai-dad",
			By: "ai", Rev: 2,
			Ops: []HistoryOp{{Op: "set", Path: "groups/影音/items/片单清单/name"}},
		},
		{
			At: "2026-10-05T20:00:02+08:00", Profile: "爸爸&小明", Actor: "web<dad>",
			By: "web", Rev: 3,
			Ops: []HistoryOp{{Op: "set", Path: "groups/影音/items/片单&收藏/name"}},
		},
	}
	for _, e := range entries {
		if err := s.AppendHistory(e); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(s.HistoryPath())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("三条该三行, got %d: %q", len(lines), string(raw))
	}
	// 非 ASCII 原样、不出现 \uXXXX：与磁盘上书签同一套写出契约。
	// SetEscapeHTML(true) 会把 & < 变成 \u0026 \u003c，这条先红。
	for i, line := range lines {
		if strings.Contains(line, `\u`) {
			t.Fatalf("第 %d 行出现 \\u 转义，HTML 转义没关掉: %s", i, line)
		}
	}
	if !strings.Contains(string(raw), "片单清单") || !strings.Contains(string(raw), "爸爸&小明") ||
		!strings.Contains(string(raw), "web<dad>") {
		t.Fatalf("磁盘上该出现字面的中文与 & < ，不许转义: %s", raw)
	}
	// 键序 = struct 字段序，且是紧凑单行。
	if !strings.HasPrefix(lines[0], `{"at":"2026-10-05T20:00:00+08:00","profile":"dad"`) {
		t.Fatalf("键序/形状不对: %s", lines[0])
	}
	// 中文与 & 都落在同一行里，且仍是紧凑单行。
	if !strings.HasPrefix(lines[2], `{"at":"2026-10-05T20:00:02+08:00","profile":"爸爸&小明"`) {
		t.Fatalf("非 ASCII 那条的键序/形状不对: %s", lines[2])
	}
	// 读回来仍是同一份内容：反转义不是"读方"的责任。
	got, err := s.ReadHistory("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Actor != "web<dad>" || got[0].Ops[0].Path != "groups/影音/items/片单&收藏/name" {
		t.Fatalf("读回的内容与写入不一致: %+v", got)
	}
}

// ops 只存动作与目标路径，不存 value：日志是"谁动过什么"的审计线，
// 不是第二条翻书签的路（spec §4.6）。value 里可能有 URL、描述、备注。
func TestHistoryHasNoOpValues(t *testing.T) {
	s := New(t.TempDir())
	e := HistoryEntry{At: "x", Profile: "dad", Actor: "ai-dad", By: "ai", Rev: 1,
		Ops: []HistoryOp{{Op: "add", Path: "groups/nas/items"}}}
	if err := s.AppendHistory(e); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.HistoryPath())
	if strings.Contains(string(raw), "value") || strings.Contains(string(raw), "http") {
		t.Fatalf("history 里出现了 ops 的内容: %s", raw)
	}
}

// 三条 rev 全为 1、profile 依次 dad/mom/dad：按 rev 排是稳定序=文件序，
// 只有"文件顺序倒过来"才能得到 dad,mom。这条断言专门用来抓
// "顺手按 rev 排序"的实现。
//
// 简报原样只有 Profile 断言，实测抓不住破坏#2：dad/mom/dad 是回文，
// 且三条 At 相同、Rev 相同，倒序与"按 rev 稳定排序"（或不排序）给出
// 逐字段相同的列表——Profile 值区分不了第 1 行的 dad 和第 3 行的 dad。
// 每条一个可区分的 At（t1/t2/t3）才能把"文件倒序"钉死，下面补的
// At 断言就是干这个的。
func TestReadHistoryFiltersAndNewestFirst(t *testing.T) {
	s := New(t.TempDir())
	lines := []struct {
		At, Profile string
	}{
		{"t1", "dad"}, {"t2", "mom"}, {"t3", "dad"}, // 文件顺序，rev 全为 1
	}
	for _, l := range lines {
		if err := s.AppendHistory(HistoryEntry{At: l.At, Profile: l.Profile, Actor: l.Profile, By: "web", Rev: 1}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ReadHistory("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d", len(all))
	}
	dad, err := s.ReadHistory("dad", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(dad) != 2 {
		t.Fatalf("按档案筛失败: %+v", dad)
	}
	two, err := s.ReadHistory("", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(two) != 2 || two[0].Profile != "dad" || two[1].Profile != "mom" {
		t.Fatalf("limit 要取最近 N 条且按文件倒序: %+v", two)
	}
	// At="t1/t2/t3" 按文件序写入：倒序后最新两条是 t3、t2。
	// 按 rev 稳定排序（rev 全为 1）会原样给出 t1、t2——这条断言让它变红。
	if two[0].At != "t3" || two[1].At != "t2" {
		t.Fatalf("newest-first 该是文件倒序（第 3 行、第 2 行）, got at=%q,%q", two[0].At, two[1].At)
	}
	// 按档案筛同样要倒序：dad 的两条是第 3 行在前。
	if dad[0].At != "t3" || dad[1].At != "t1" {
		t.Fatalf("筛档案后仍要文件倒序, got at=%q,%q", dad[0].At, dad[1].At)
	}
}

// 崩溃可能留下半行 JSON。半行只该丢掉那一行，不该让整条历史读不出来
// ——与"坏一个档案不坏全家"同一个取舍。
func TestTornLastLineIsSkippedNotFatal(t *testing.T) {
	s := New(t.TempDir())
	if err := s.AppendHistory(HistoryEntry{At: "t", Profile: "dad", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"at":"t","profile":"da`); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	got, err := s.ReadHistory("", 0)
	if err != nil {
		t.Fatalf("半行不该让整档失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}

// 一次大批次的 op 列表会让 jsonl 那一行远超任何"合理上限"。这条用例是
// "读用 bufio.Reader 而不是带上限的 Scanner"的证人（评审 Important #2）：
// 换成带 64KB 上限的 Scanner 时 ErrTooLong 不可恢复，长行之后的好行会整段
// 读丢——所以这里在长行前后各放一条好行，倒序后长行是中间那条。
func TestLongHistoryLineIsNotSilentlyDropped(t *testing.T) {
	s := New(t.TempDir())
	if err := s.AppendHistory(HistoryEntry{At: "before", Profile: "dad", By: "web", Rev: 1}); err != nil {
		t.Fatal(err)
	}
	e := HistoryEntry{At: "t", Profile: "dad", By: "ai", Rev: 9}
	for i := 0; i < 4000; i++ {
		e.Ops = append(e.Ops, HistoryOp{Op: "set", Path: "groups/nas/items/some-item-id-with-a-longish-slug/name"})
	}
	if err := s.AppendHistory(e); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(HistoryEntry{At: "after", Profile: "mom", By: "web", Rev: 2}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ReadHistory("", 0)
	if err != nil {
		t.Fatalf("一行超长不该让整档历史读不出来: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("长行之后的记录丢了: 只读到 %d 条", len(got))
	}
	// 倒序：after、长行、before。
	if got[1].At != "t" || len(got[1].Ops) != 4000 {
		t.Fatalf("长行本身被吞了: %+v", got[1])
	}
	if got[0].At != "after" || got[2].At != "before" {
		t.Fatalf("长行两侧的好行没保住: at=%q,%q,%q", got[0].At, got[1].At, got[2].At)
	}
}

func TestValidBy(t *testing.T) {
	for _, ok := range []string{"ai", "bookmarklet", "extension", "web", "import", "cli"} {
		if !ValidBy(ok) {
			t.Fatalf("%q 该合法", ok)
		}
	}
	// sync-template 随模板机制一起没了（2026-10-10）：没有任何动作再写得出这个值，
	// 留在合法集里就是一个"看起来还能用"的假选项。
	if ValidBy("sync-template") {
		t.Error("sync-template 已不存在，不该还是合法渠道")
	}
	for _, bad := range []string{"", "AI", "cron"} {
		if ValidBy(bad) {
			t.Fatalf("%q 该被拒", bad)
		}
	}
}
