package bookimport

import (
	"strings"
	"testing"
)

// 脱敏夹具：地址一律公网或 10. 段，服务名通用。
// **不写真实内网地址** —— 本仓库公开，leakguard 扫完整四段 IPv4，夹具也不例外。
const nbFixture = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<!-- This is an automatically generated file. DO NOT EDIT! -->
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><A HREF="https://github.com/" ADD_DATE="1600000000">GitHub</A>
    <DD>代码托管 &amp; 社区
    <DT><H3 ADD_DATE="1600000000">工具</H3>
    <DL><p>
        <DT><A HREF="https://example.com/a?x=1&amp;y=2">带参数的例子</A>
        <DD>查询参数里的 & 要解码
        <DT><A HREF="chrome://extensions/">扩展</A>
        <DT><A HREF="https://nested.example/">嵌套层里的</A>
    </DL><p>
    <DT><H3>回收站</H3>
    <DL><p>
        <DT><A HREF="https://deleted.example/">删掉的</A>
    </DL><p>
    <DT><A>没有 HREF 的</A>
    <DT><A HREF="https://noname.example/"></A>
</DL><p>
`

func parseFixture(t *testing.T) (*nbTree, nbNotes) {
	t.Helper()
	return parseNetbook([]byte(nbFixture))
}

// 层级是这个解析器的全部难点：<DT> 不闭合、<DL><p> 后面那个多余的 <p>、
// <H3> 与它的 <DL> 分开出现。层级错了，书签会静默落进错误的文件夹——
// 而那正是 §4.4 说的"并发编辑最容易踩的错"，只是这里发生在导入那一刻。
func TestParseNetbook_层级(t *testing.T) {
	tree, _ := parseFixture(t)
	r := tree.Root

	if len(r.Subs) != 1 {
		// 回收站已被摘到 tree.Trashed 上，所以这里只剩「工具」一个
		// （"根下有 2 个文件夹"那个旧断言在摘出去之后就是错的，而它错得
		//  很安静——只是让一个本来该失败的层级 bug 看起来像是修好了）。
		t.Fatalf("根下应只剩「工具」1 个文件夹，实际 %d 个", len(r.Subs))
	}
	tools := r.Subs[0]
	if tools.Name != "工具" {
		t.Errorf("第一个文件夹名 = %q，要「工具」", tools.Name)
	}
	if len(tools.Items) != 3 {
		t.Fatalf("工具下应有 3 条，实际 %d 条：%+v", len(tools.Items), tools.Items)
	}
	if tools.Items[2].Name != "嵌套层里的" {
		t.Errorf("嵌套 </DL><p> 之后那条没落回本层，拿到的是 %q", tools.Items[2].Name)
	}
	// 顶层那两条（GitHub 与无标题那条）不该混进"工具"
	if len(r.Items) != 2 {
		t.Errorf("根下应有 2 条顶层书签，实际 %d 条：%+v", len(r.Items), r.Items)
	}
}

// &amp; 不解码的话，一半带查询参数的网址会存成错的，而它要等到哪天被人点开
// 才发现。这一条钉的是**真的解码了**，不只是"没报错"。
func TestParseNetbook_实体解码(t *testing.T) {
	tree, _ := parseFixture(t)
	tools := tree.Root.Subs[0]

	if got := tools.Items[0].URL; got != "https://example.com/a?x=1&y=2" {
		t.Errorf("URL = %q，&amp; 应解码成 &", got)
	}
	if got := tools.Items[0].Desc; got != "查询参数里的 & 要解码" {
		t.Errorf("描述 = %q，&amp; 应解码成 &", got)
	}
	if got := tree.Root.Items[0].Desc; got != "代码托管 & 社区" {
		t.Errorf("<DD> 描述 = %q", got)
	}
}

// <DD> 描述必须归给**紧挨着它的那条**，而不是"当前文件夹的第一条"。
func TestParseNetbook_DD归属紧邻的那条(t *testing.T) {
	tree, _ := parseFixture(t)
	tools := tree.Root.Subs[0]

	for i, it := range tools.Items {
		if it.Desc != "" && it.Name != "带参数的例子" {
			t.Errorf("第 %d 条 %q 拿到了不属于它的描述 %q", i, it.Name, it.Desc)
		}
	}
	if tree.Root.Items[0].Desc == "" {
		t.Error("顶层第一条的 <DD> 丢了")
	}
	if tree.Root.Items[1].Desc != "" {
		t.Errorf("顶层第二条不该有描述却有 %q", tree.Root.Items[1].Desc)
	}
}

// 回收站是用户**已经删掉**的东西，混进分组等于把它复活。
func TestParseNetbook_回收站拆出来且不进分组(t *testing.T) {
	tree, _ := parseFixture(t)

	if tree.Trashed == nil {
		t.Fatal("回收站没被识别出来")
	}
	if len(tree.Trashed.Items) != 1 || tree.Trashed.Items[0].Name != "删掉的" {
		t.Errorf("回收站内容不对：%+v", tree.Trashed.Items)
	}
	// **摘出去了，而不是靠标记区分**：遍历 Root.Subs 不该再碰得到它。
	// 用标记也行，但那样每个消费方都得记得判断，而漏判的后果是已删的书签复活。
	for _, f := range tree.Root.Subs {
		if f.trashed {
			t.Error("回收站还留在 Root.Subs 里——消费方遍历时必须自己记得跳过，那正是它会漏的地方")
		}
		for _, it := range f.Items {
			if it.Name == "删掉的" {
				t.Errorf("回收站的条目 %q 同时出现在普通分组里", it.Name)
			}
		}
	}
	if len(tree.Root.Subs) != 1 {
		t.Errorf("摘掉回收站后根下应只剩「工具」，实际 %d 个文件夹", len(tree.Root.Subs))
	}
}

// 「跳过」必须逐条记账。静默丢掉的东西会让报告说"迁了 N 条"而实际少了 M 条，
// 而丢在哪一行没有任何记录。
func TestParseNetbook_跳过与顶替都记账(t *testing.T) {
	_, notes := parseFixture(t)
	joined := notesText(notes)

	for _, want := range []string{
		"没有 HREF",  // 跳过的那条
		"用 URL 顶上", // 无标题那条的顶替
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes 里少了「%s」那一类记录，实际：\n%s", want, joined)
		}
	}
	for _, n := range notes {
		if n.Line <= 0 {
			t.Errorf("每条记录都要带行号，收到 %d：%q", n.Line, n.What)
		}
	}
}

// 无标题的书签用 URL 顶上而不是给空名字——空名字在首屏是一个没有文字的方块。
func TestParseNetbook_无标题用URL顶上(t *testing.T) {
	tree, _ := parseFixture(t)
	got := tree.Root.Items[1].Name
	if got != "https://noname.example/" {
		t.Errorf("无标题那条的 name = %q，要用 URL 顶上", got)
	}
	if got == "" {
		t.Error("绝不能给空名字——那在首屏是一个没有文字的方块")
	}
}

func notesText(n nbNotes) string {
	var b strings.Builder
	for _, x := range n {
		b.WriteString(x.What + "\n")
	}
	return b.String()
}

// chrome:// 这类在浏览器里点不动的地址要**原样保留**，不能因为"不是网页"就丢。
func TestParseNetbook_非http地址原样保留(t *testing.T) {
	tree, _ := parseFixture(t)
	var found bool
	for _, it := range tree.Root.Subs[0].Items {
		if it.Name == "扩展" {
			found = true
			if it.URL != "chrome://extensions/" {
				t.Errorf("chrome:// 地址应原样保留，实际 %q", it.URL)
			}
		}
	}
	if !found {
		t.Error("chrome:// 那条被丢了——它在 flare 里点不动，但 URL 仍要留着")
	}
}

// 空文件、坏文件都必须**报错或返回空树**，不许 panic：一个坏书签文件不该让
// 整个导入器崩在栈溢出上。
func TestParseNetbook_坏输入不panic(t *testing.T) {
	for _, in := range []string{
		"",
		"<<<>>>",
		"<DL><p><DT><A HREF=",
		"<A HREF=\"http://a\"></DL></DL></DL>",
		strings.Repeat("<DL><p>", 50), // 故意不闭合
		"<!-- 没有结尾的注释",
	} {
		t.Run(in[:min(len(in), 12)], func(t *testing.T) {
			tree, _ := parseNetbook([]byte(in))
			if tree == nil || tree.Root == nil {
				t.Fatal("不许返回 nil 树")
			}
		})
	}
}
