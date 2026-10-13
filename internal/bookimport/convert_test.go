package bookimport

import (
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
	"github.com/rockbenben/launchbay/internal/model"
)

func convertFixture(t *testing.T, opt Options) *Result {
	t.Helper()
	res, err := Convert([]byte(nbFixture), realIndex(t), opt)
	if err != nil {
		t.Fatalf("转换失败：%v", err)
	}
	return res
}

func countItems(b *model.Bookmarks) int {
	n := 0
	for _, g := range b.Groups {
		n += len(g.Items)
	}
	return n
}

// 端到端：夹具进去，三条对账全过、报告能读。
//
// 这是整套东西的**总闸**。上面那些单件测试各自只证明自己那一块，
// 而"解析 → 分组 → 猜图标 → 对账"这条链上任何一处错位，
// 只在这支里才看得出来。
func TestConvert_端到端三条对账全过(t *testing.T) {
	res := convertFixture(t, Options{})

	if !res.Report.Rec.OK() {
		t.Fatalf("三条对账没过：\n%s", strings.Join(res.Report.Rec.Failures(), "\n"))
	}
	// 夹具里：GitHub / 带参数的例子 / 扩展 / 嵌套层里的 / 删掉的 = 5 条
	// （回收站那条也进档案，只是不进"结果"意义上的普通分组）。
	if n := countItems(res.Bookmarks); n != 5 {
		t.Errorf("档案里应有 5 条书签，实际 %d 条", n)
	}
	// id 必须全局唯一且非空——寻址全靠它（§4.4 不用下标）
	seen := map[string]bool{}
	for _, g := range res.Bookmarks.Groups {
		if g.ID == "" {
			t.Errorf("分组 %q 的 id 是空的", g.Title)
		}
		for _, it := range g.Items {
			if it.ID == "" {
				t.Errorf("分组 %q 里的 %q 没有 id", g.Title, it.Name)
			}
			if seen[it.ID] {
				t.Errorf("id %q 重复了——两条书签会指到同一条", it.ID)
			}
			seen[it.ID] = true
			if it.Alias == nil || it.Tags == nil {
				t.Errorf("%q 的 alias/tags 是 nil：契约要求写 [] 不写 null（§4.3）", it.Name)
			}
		}
	}
}

// 报告是给人看的第一产物：它必须能在人还没决定看 JSON 之前就回答
// 「迁干净了吗」。这一支钉的是**结构**，不是每一行文案。
func TestConvert_报告能回答该回答的问题(t *testing.T) {
	res := convertFixture(t, Options{})
	txt := res.Report.Text()

	for _, want := range []string{
		"三条对账全过",          // 结论
		"回收站",             // 浏览器里删掉的东西去了哪
		"非 http/https 协议", // chrome:// 那条
		"没有自动改",           // 标出但不擅自修
		"对账（三条全过才算导完）",    // 三条逐条列出
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("报告里缺少「%s」：\n%s", want, txt)
		}
	}
	// 结论必须在最前面：读者最先回答的是"迁干净了吗"
	if i, j := strings.Index(txt, "## 结论"), strings.Index(txt, "对账（三条"); i < 0 || (j >= 0 && i > j) {
		t.Errorf("结论要排在对账明细之前\n%s", txt)
	}
	// 「三条全过」与「没能进来」并排时的那一句，**在带跳过条目的那份夹具上验**
	// ——见 screen_test.go 的 `Test报告里全过与少东西要能同时成立`。这一份夹具里
	// 一条都跳不出去，两个词也就不会同时出现，在这儿钉它是空转。
}

// 浏览器导出的 <TITLE> 常是 "Bookmarks" 这种什么都没说的默认值。
// 把它当标题写进档案，等于替用户起了一个他没起的名。
func TestConvert_默认标题不当作档案标题(t *testing.T) {
	const withTitle = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>Bookmarks</TITLE>
<DL><p><DT><A HREF="https://example.com/">x</A></DL><p>`
	res, err := Convert([]byte(withTitle), realIndex(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Bookmarks.Title != "" {
		t.Errorf("默认标题 \"Bookmarks\" 不该被当成档案标题，实际 %q", res.Bookmarks.Title)
	}

	const realTitle = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>我的收藏</TITLE>
<DL><p><DT><A HREF="https://example.com/">x</A></DL><p>`
	res2, err := Convert([]byte(realTitle), realIndex(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Bookmarks.Title != "我的收藏" {
		t.Errorf("真标题该被采用，实际 %q", res2.Bookmarks.Title)
	}
}

// 图标索引是 nil 就没法跑——它必须**报错而不是悄悄产出没图标的档案**，
// 因为那种失败在界面上看不出来。
func TestConvert_没有索引就报错(t *testing.T) {
	if _, err := Convert([]byte(nbFixture), nil, Options{}); err == nil {
		t.Fatal("没有图标索引时必须报错——不能悄悄产出一份全走兜底的档案")
	}
}

// 覆盖表写错时**中止**而不是照写：写错的 icon 存进 bookmarks.json 不会当场报错。
func TestConvert_覆盖表写错就中止(t *testing.T) {
	p := writeOverrides(t, `{"nas.example.internal":"simple-icons:肯定不存在-zzz"}`)
	if _, err := Convert([]byte(nbFixture), realIndex(t), Options{OverridesPath: p}); err == nil {
		t.Fatal("覆盖表指向不存在的图标却导入了——那会写出坏档数据")
	}
}

// 空输入不许 panic，也不许报"导了 0 条"了事——那看不出是文件坏了还是真的没书签。
func TestConvert_空输入有话说(t *testing.T) {
	res, err := Convert(nil, realIndex(t), Options{})
	if err != nil {
		t.Fatalf("空输入不该报错（它就是空的），实际 %v", err)
	}
	if res.Bookmarks == nil {
		t.Fatal("不许返回 nil")
	}
	if countItems(res.Bookmarks) != 0 {
		t.Errorf("空输入不该产出书签，实际 %d 条", countItems(res.Bookmarks))
	}
}

var _ = icons.Candidates // 保留：guess.go 的注释引用了它，说明为什么不复用
