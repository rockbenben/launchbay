package bookimport

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// mkItems 造一组 flare 侧条目，省得每支都写一遍五个字段。
func mkItems(pairs ...[3]string) []srcItem {
	var out []srcItem
	for _, p := range pairs {
		out = append(out, srcItem{Name: p[0], URL: p[1], Desc: p[2]})
	}
	return out
}

// mkDst 把 src 原样搬成 dst（不改任何字段），并全部放进 group "g1"。
// "什么都没整理"的那一支靠它——整理动作的用例才在它之上单独改字段。
func mkDst(src []srcItem, group string) []dstItem {
	var out []dstItem
	for _, s := range src {
		out = append(out, dstItem{Item: model.Item{
			ID: "x", Name: s.Name, URL: s.URL, Desc: s.Desc, Icon: s.Icon,
			Alias: []string{}, Tags: []string{},
		}, GroupID: group})
	}
	return out
}

// wantIn 说"所有条目按映射规则都该落在 group"，也就是"没有分类调整"。
// 原样搬运那一支靠它——**它绿着，才轮得到下面几支去证明"整理了"不会被判红**。
func wantIn(group string) func(srcItem) string {
	return func(srcItem) string { return group }
}

var ex1 = [3]string{"A", "https://a.example/", "描述A"}
var ex2 = [3]string{"B", "https://b.example/", "描述B"}
var ex3 = [3]string{"C", "https://c.example/", "描述C"}

// 什么都没整理的那一支必须全绿。这是其余所有用例的对照：**它绿着，"没整理"
// 不会被误判成失败**；它红着，上面那些"整理了"的红绿就都失去意义了。
func TestReconcile_原样搬运三条全过(t *testing.T) {
	src := mkItems(ex1, ex2)
	dst := mkDst(src, "g1")
	got := reconcile(src, dst, wantIn("g1"))
	if !got.OK() {
		t.Fatalf("原样搬运应当三条全过，实际没过：%v", got.Failures())
	}
	if len(got.Remaps) != 0 {
		t.Errorf("没整理就不该有整理动作，实际 %d 条：%v", len(got.Remaps), got.Remaps)
	}
}

func TestReconcile_三条各自能红(t *testing.T) {
	src := mkItems(ex1, ex2)

	t.Run("条数不等", func(t *testing.T) {
		got := reconcile(src, mkDst(src[:1], "g1"), wantIn("g1"))
		if got.CountOK {
			t.Error("少了一条，条数对账却说过")
		}
		if len(got.Missing) != 1 {
			t.Errorf("少的那条要进差集，实际 %d 条", len(got.Missing))
		}
	})

	t.Run("URL 差集：丢", func(t *testing.T) {
		dst := mkDst(src, "g1")
		dst = append(dst[:1], dst[2:]...) // 摘掉中间那条
		got := reconcile(src, dst, wantIn("g1"))
		if len(got.Missing) != 1 || len(got.Extra) != 0 {
			t.Errorf("丢 1 条应只有 Missing=1，实际 Missing=%d Extra=%d", len(got.Missing), len(got.Extra))
		}
	})

	t.Run("URL 差集：凭空多", func(t *testing.T) {
		dst := append(mkDst(src, "g1"), dstItem{
			Item:    model.Item{ID: "y", Name: "C", URL: "https://c.example/", Alias: []string{}, Tags: []string{}},
			GroupID: "g1",
		})
		got := reconcile(src, dst, wantIn("g1"))
		if len(got.Extra) != 1 || len(got.Missing) != 0 {
			t.Errorf("多 1 条应只有 Extra=1，实际 Missing=%d Extra=%d", len(got.Missing), len(got.Extra))
		}
	})

	t.Run("name 不一致判红", func(t *testing.T) {
		dst := mkDst(src, "g1")
		dst[0].Name = "A改名了"
		got := reconcile(src, dst, wantIn("g1"))
		if len(got.FieldDiffs) == 0 {
			t.Fatal("name 被改了却没报——第 3 条形同虚设")
		}
		// **配对键是 URL 而不是 (URL, Name)**：所以改名之后这条仍然配得上，
		// 于是它以"name 不一致"的面貌出现。如果配对键里带了 name，这一支就会
		// 变成 Missing+Extra，报成"丢了 A、凭空多出 A改名了"——那时点进去看到的
		// 是差集而不是 name，指不到要改的地方。
		if len(got.Missing) != 0 || len(got.Extra) != 0 {
			t.Errorf("改名不该被算成丢失或增生（那说明配对键里混进了 name）：Missing=%v Extra=%v",
				got.Missing, got.Extra)
		}
		if !strings.Contains(got.FieldDiffs[0].Field, "name") {
			t.Errorf("要指明是 name 这一栏变了，实际 %q", got.FieldDiffs[0].Field)
		}
		if got.FieldDiffs[0].Want != "A" || got.FieldDiffs[0].Got != "A改名了" {
			t.Errorf("要同时写出原来是什么、现在是什么，实际 %+v", got.FieldDiffs[0])
		}
	})

	t.Run("desc 不一致判红", func(t *testing.T) {
		dst := mkDst(src, "g1")
		dst[0].Desc = "换了个描述"
		got := reconcile(src, dst, wantIn("g1"))
		if len(got.FieldDiffs) != 1 {
			t.Fatalf("desc 被改了却没报（或报多了）：%d 处", len(got.FieldDiffs))
		}
		if got.FieldDiffs[0].Want != "描述A" || got.FieldDiffs[0].Got != "换了个描述" {
			t.Errorf("要同时写出原来是什么、现在是什么，实际 %+v", got.FieldDiffs[0])
		}
	})
}

// §9 第 3 条的核心：**整理动作只能出现在「图标重映射」「分类调整」两类里，
// 不得混进 URL/name**。所以这两类差异**不许判红**，但必须逐条进报告。
//
// 把它们判红的后果不是"太严"，而是这一支永远红——于是所有人都会去把
// "redundant"的判定关掉，而那时真正该红的东西也跟着关了。
func TestReconcile_图标与分类变了不判红但要记账(t *testing.T) {
	src := mkItems(ex1, ex2)
	src[0].Icon = "mdi:plex"
	src[1].Category = "播客"

	dst := mkDst(src, "g1")
	dst[0].Icon = "simple-icons:plex" // 图标重映射
	dst[1].GroupID = "media"          // 分类调整：按映射规则它本该在 g1
	// wantGroup 说"两条都该在 g1"，于是 ex2 落在 media 是一次**被检出的**调整；
	// ex1 落在 g1 就没有调整。wantGroup 说话，才能有一处"实际 ≠ 应当"可言。
	got := reconcile(src, dst, wantIn("g1"))
	if !got.OK() {
		t.Fatalf("图标重映射与分类调整是允许的整理动作，不该判红。实际判红：%v", got.Failures())
	}
	if len(got.Remaps) != 2 {
		t.Fatalf("整理动作要逐条记账，实际 %d 条", len(got.Remaps))
	}
	byKind := summarizeRemaps(got.Remaps)
	if byKind["icon"] != 1 || byKind["category"] != 1 {
		t.Fatalf("两类各一条，实际 %v", byKind)
	}
	for _, r := range got.Remaps {
		if r.Reason == "" {
			t.Errorf("%s 这条整理动作没有理由——说不清改成什么了的整理，等于一次没记录的数据改动", r.Kind)
		}
		if r.From == r.To {
			t.Errorf("%s 这条整理前后一样，那它就不该被记成整理", r.Kind)
		}
	}
}

// 报告的确定性和读数顺序都要一样：同一份输入两次跑出的顺序不同，人会以为
// 多了一条或者少了一条。
func TestReconcile_同一输入两次跑出的报告文本一致(t *testing.T) {
	src := mkItems(ex1, ex2, ex3)
	dst := mkDst(src, "g1")
	dst = dst[:2] // 制造一个固定的失败集
	a := reconcile(src, dst, wantIn("g1"))
	b := reconcile(src, dst, wantIn("g1"))
	if strings.Join(a.Failures(), "|") != strings.Join(b.Failures(), "|") {
		t.Errorf("两次跑的失败清单不一样：\n%v\n%v", a.Failures(), b.Failures())
	}
}
