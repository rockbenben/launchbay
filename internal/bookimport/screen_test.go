package bookimport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
	"github.com/rockbenben/launchbay/internal/ops"
)

// 这一片是一次真实故障的现场：**用户的浏览器导出一上来就报"有 6 条过不了校验，
// 整批没有写入"**。根因有两处，都不是"用户的数据有问题"：
//
//  1. id 超长。`分组id-书签名` 这种拼法在真实导出上必然破 32（model.ValidSlug
//     的上限）——文件夹名三四十字符，书签名再叠一个长标题，于是每一条都非法。
//  2. 有一条 `javascript:` 地址。写路径的 URL 判据要求有主机名，导入器却
//     根本没先筛，于是一条坏地址让**整份文件**进不来。
//
// 修法分两半：能改的（id）改对，不能改的（地址）**逐条挑出去并点名**——
// 用户的收藏夹里有 javascript: 小工具是常事，让它把另外两百条挡在门外才是 bug。
//
// realWorldFile 照抄 Chrome/Edge 导出的骨架，掺进上面两种形状：
// 长文件夹名、长书签名、javascript:、data:、file:、chrome://（**带主机名，
// 按 §6.2 原样保留**）、空名字、跨组同名。
const realWorldFile = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
  <DT><H3 ADD_DATE="1" PERSONAL_TOOLBAR_FOLDER="true">书签栏</H3>
  <DL><p>
    <DT><A HREF="https://github.com/" ADD_DATE="1">GitHub</A>
    <DT><A HREF="javascript:void(0)">一个书签小工具</A>
    <DT><A HREF="https://www.bilibili.com/video/BV1xx411c7mD">哔哩哔哩 - 那些年我追过的番剧全集</A>
    <DT><A HREF="chrome://extensions/">扩展</A>
    <DT><A HREF="https://example.com/b">GitHub</A>
  </DL><p>
  <DT><H3 ADD_DATE="1">开发工具与那些很长很长长到需要被截断的文件夹名字</H3>
  <DL><p>
    <DT><A HREF="https://go.dev/">Go 官方文档与语言规范</A>
    <DT><A HREF="https://stackoverflow.com/questions/12345678/how-do-i-do-the-thing-in-golang">Stack Overflow 那个超长问题的标题</A>
    <DT><A HREF="data:text/html,hello">data 链接</A>
    <DT><A HREF="file:///C:/Users/Administrator/Downloads/">本地文件</A>
    <DT><H3>全都进不来的文件夹</H3>
    <DL><p>
      <DT><A HREF="javascript:alert(1)">又一个书签小工具</A>
    </DL><p>
  </DL><p>
</DL><p>`

func convertRealWorld(t *testing.T) *Result {
	t.Helper()
	res, err := Convert([]byte(realWorldFile), realIndex(t), Options{})
	if err != nil {
		t.Fatalf("转换失败：%v", err)
	}
	return res
}

// 书签栏那一层拿 tiles，判据必须是**浏览器打的属性**，不是文件夹名字。
//
// 从前那条判据是 `strings.HasSuffix(title, "书签栏")` —— 中文硬编码，于是：
//   - 英文 Chrome 导出的 "Bookmarks bar" 永远拿不到 tiles，
//   - 而用户自己建一个叫「我的书签栏」的普通文件夹反而被当成书签栏。
//
// 这一条钉的是**属性认人**：名字带不带"书签栏"、是不是中文，都不影响结果。
// 正反两面都写：认得出真的、认不出假的。
func Test书签栏靠属性判而不是名字(t *testing.T) {
	const file = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
  <DT><H3 PERSONAL_TOOLBAR_FOLDER="true">Bookmarks bar</H3>
  <DL><p>
    <DT><A HREF="https://github.com/">GitHub</A>
    <DT><A HREF="https://example.com/b">Example</A>
  </DL><p>
  <DT><H3>我的书签栏（自己建的普通文件夹）</H3>
  <DL><p>
    <DT><A HREF="https://example.com/c">Another</A>
  </DL><p>
  <DT><H3>Ordinary folder</H3>
  <DL><p>
    <DT><A HREF="https://example.com/d">Third</A>
  </DL><p>
</DL><p>`
	res, err := Convert([]byte(file), realIndex(t), Options{})
	if err != nil {
		t.Fatalf("转换失败：%v", err)
	}
	layout := map[string]string{}
	for _, g := range res.Bookmarks.Groups {
		layout[g.Title] = g.Layout
	}
	// 2026-10-13 改判：**导入的每一个分组都从磁贴开始**（用户口径"默认应该用磁吸"）。
	// 从前这一格断的是"书签栏 tiles、普通文件夹 list"，那条区分现在归**顺序**管：
	// toolbar 那一层排在最前面，排法不再由它决定。
	for _, want := range []string{"Bookmarks bar", "我的书签栏（自己建的普通文件夹）", "Ordinary folder"} {
		if got := layout[want]; got != model.LayoutTiles {
			t.Errorf("分组 %q 该从磁贴开始，得到 %q", want, got)
		}
	}
	if n := len(res.Bookmarks.Groups); n != 3 {
		t.Fatalf("夹具该出三个分组，实际 %d", n)
	}
	if first := res.Bookmarks.Groups[0].Title; first != "Bookmarks bar" {
		t.Errorf("浏览器声明的书签栏那一层该排在最前（toolbar 现在管的就是这一格），实际第一组是 %q", first)
	}
}

// decodeOps 走 httpapi 里那份解码器同样的形状（严格模式），好让这条证人
// 断的是"从真实文件到写路径"的**整条**链路，而不是 Convert 单独一块。
func decodeOps(t *testing.T, b *model.Bookmarks) []ops.Op {
	t.Helper()
	var out []ops.Op
	for _, raw := range Ops(b) {
		data, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		var op ops.Op
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&op); err != nil {
			t.Fatalf("ops 解不出来（少写了一个字段？）：%v", err)
		}
		out = append(out, op)
	}
	return out
}

// 总闸：真实导出形状跑完 Convert → Ops → ops.Run，**一条校验错都不许有**。
//
// 少了这一条，前面那些单件测试各自都绿，而用户拿自己的收藏夹一点就是
// "有 6 条过不了校验，整批没有写入"。
func Test真实导出形状过得了写路径(t *testing.T) {
	res := convertRealWorld(t)
	_, errs, _ := ops.Run(&model.Bookmarks{Groups: []model.Group{}}, decodeOps(t, res.Bookmarks), nil)
	for _, e := range errs {
		t.Errorf("写路径拒了它：%s", e.String())
	}
	if len(errs) > 0 {
		t.Fatalf("真实导出里有 %d 条过不了校验", len(errs))
	}
	// 文档自己也要合法——ops 判的是逐条，写进磁盘前还会整体 Validate 一次。
	if err := res.Bookmarks.Validate(); err != nil {
		t.Errorf("产出的书签文档自己不合法：%v", err)
	}
}

// id 的三条硬判据：合法（ValidSlug）、不超长、全档案唯一。
// 寻址全靠 id（§4.4 不用下标），所以这三条任何一条破了都是"书签互相指错"。
func Test条目与分组的id都合法(t *testing.T) {
	res := convertRealWorld(t)
	seen := map[string]bool{}
	for _, g := range res.Bookmarks.Groups {
		if !model.ValidSlug(g.ID) {
			t.Errorf("分组 id %q 不是合法 slug", g.ID)
		}
		if len(g.ID) > slugMax {
			t.Errorf("分组 id %q 有 %d 个字符，超过 %d", g.ID, len(g.ID), slugMax)
		}
		for _, it := range g.Items {
			if !model.ValidSlug(it.ID) {
				t.Errorf("条目 id %q 不是合法 slug（名字 %q）", it.ID, it.Name)
			}
			if len(it.ID) > slugMax {
				t.Errorf("条目 id %q 有 %d 个字符，超过 %d（名字 %q）", it.ID, len(it.ID), slugMax, it.Name)
			}
			if seen[it.ID] {
				t.Errorf("条目 id %q 重复：两条书签会指到同一条", it.ID)
			}
			seen[it.ID] = true
		}
	}
}

// 截断会毁掉唯一性吗：两个只有尾巴不同的长标题，截断后变成同一个前缀。
// 撞了要递加后缀，**并且递加后仍然不超长**（不重新截的话，去重反而造出非法 id）。
func Test长标题截断后仍然唯一(t *testing.T) {
	const twoLong = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>Bookmarks</TITLE>
<DL><p>
  <DT><H3>收藏夹</H3>
  <DL><p>
    <DT><A HREF="https://example.com/a">AAAAAAAAAA 这是一条很长很长的书签名甲</A>
    <DT><A HREF="https://example.com/b">AAAAAAAAAA 这是一条很长很长的书签名乙</A>
    <DT><A HREF="https://example.com/c">AAAAAAAAAA 这是一条很长很长的书签名丙</A>
  </DL><p>
</DL><p>`
	res, err := Convert([]byte(twoLong), realIndex(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, g := range res.Bookmarks.Groups {
		for _, it := range g.Items {
			if prev, dup := seen[it.ID]; dup {
				t.Errorf("id %q 被 %q 与 %q 同时占了", it.ID, prev, it.Name)
			}
			if !model.ValidSlug(it.ID) {
				t.Errorf("去重后的 id %q 反而非法了——递加后缀时没有重新截断", it.ID)
			}
			seen[it.ID] = it.Name
		}
	}
	if len(seen) != 3 {
		t.Errorf("三条书签该拿到三个不同的 id，实际 %d 个", len(seen))
	}
}

// 进不来的那条要**逐条点名**，还要说清理由：只给一个总数的话，用户只能回浏览器
// 里一个文件夹一个文件夹地对。
//
// **file: 那一格 2026-10-12 翻了过来**：本地路径从"挑出去"变成"照收"，所以
// 挑出去的只剩 javascript: 两条与 data: 一条。这一格现在同时钉着两件事——
// 该挡的挡住了（脚本类地址），不该挡的没挡（本地文件）。
func Test进不来的条目逐条点名(t *testing.T) {
	res := convertRealWorld(t)
	skipped := res.Report.Skipped
	// javascript:（书签栏）、data:（工具）、javascript:（子文件夹）。
	// **file:///C:/… 不在其中**：本地文件照收。
	if len(skipped) != 3 {
		t.Fatalf("该挑出去 3 条（file: 已能收进来），实际 %d 条：%+v", len(skipped), skipped)
	}
	for _, s := range skipped {
		if s.Reason == "" || s.URL == "" {
			t.Errorf("每条都要有地址与理由，实际 %+v", s)
		}
		if strings.HasPrefix(s.URL, "file:") {
			t.Errorf("本地文件不该被挑出去：%+v", s)
		}
	}
	// 本地文件真的进来了，而且 URL 一个字都没被改写。
	var kept int
	for _, g := range res.Bookmarks.Groups {
		for _, it := range g.Items {
			if strings.HasPrefix(it.URL, "file:///C:/") {
				kept++
				if it.URL != "file:///C:/Users/Administrator/Downloads/" {
					t.Errorf("本地路径被改写了：%q", it.URL)
				}
			}
		}
	}
	if kept != 1 {
		t.Errorf("那条本地文件该进档案，实际进了 %d 条", kept)
	}
	txt := res.Report.Text()
	for _, want := range []string{"没能进来", "javascript:void(0)", "data:text/html,hello"} {
		if !strings.Contains(txt, want) {
			t.Errorf("报告里没有 %q：\n%s", want, txt)
		}
	}
	// 挑出去的东西不许在"原样保留的非 http 地址"里再说一遍——那条清单的
	// 承诺是"URL 原样留着"，对没进来的条目说这句话就是假话。
	if strings.Contains(txt, "javascript:void(0) —— `javascript:void(0)`") {
		t.Error("被挑出去的条目不该出现在「原样保留」那一节里")
	}
}

// 这份夹具里「三条对账全过」与「没能进来」**同时**出现，所以"这两句为什么能同时
// 成立"在这里是真的要验的（2026-10-13 实拍改判）。
//
// 从前的结论是"三条对账全过"打头、"另有 N 条没能进来"跟在后面——读者第一眼读到
// 的是"全过"，第二眼才看到少了东西，而报告从头到尾没有解释这一对词。对账判的是
// "该进的进了没"，被挑出去的那几条是**判定它进不来**的，两者是不同的集合；
// 不说这一句，它们就互相抵消，读者只会得出"报告自相矛盾"或者"少算了几条"。
func Test报告里全过与少东西要能同时成立(t *testing.T) {
	txt := convertRealWorld(t).Report.Text()
	if !strings.Contains(txt, "三条对账全过") {
		t.Fatalf("这份夹具该是三条全过（它没有进不来的东西混进对账里）：\n%s", txt)
	}
	if !strings.Contains(txt, "没能进来") {
		t.Fatalf("这份夹具挑出去了 3 条，报告该说出「没能进来」：\n%s", txt)
	}
	if !strings.Contains(txt, "不在对账的范围里") {
		t.Errorf("报告同时说了「没能进来」与「三条对账全过」，却没有解释这两句为什么能同时成立——"+
			"读者只会以为报告自相矛盾：\n%s", txt)
	}
	// 顺序：先说少了什么，再解释为什么对账还是过的。反过来排，读者读到"全过"
	// 就往下走了，而"少了 3 条"被压在解释的后面。
	if strings.Index(txt, "没能进来") > strings.Index(txt, "三条对账全过") {
		t.Errorf("「没能进来」要排在「三条对账全过」前面：先说少了什么，再解释为什么对账还是过的\n%s", txt)
	}
}

// 对账判的是"该进的进了没"。被挑出去的那几条是**判定它进不来**的，混进去
// 只会让三条对账永远不过——而"永远不过"这件事一旦常态化就不再携带信息。
func Test挑出去的条目不算对账失败(t *testing.T) {
	res := convertRealWorld(t)
	if !res.Report.Rec.OK() {
		t.Fatalf("三条对账没过（挑出去的条目不该算失败）：\n%s",
			strings.Join(res.Report.Rec.Failures(), "\n"))
	}
	if n := res.Report.SourceCount - res.Report.ResultCount; n != 3 {
		t.Errorf("源 %d 条、新库 %d 条，差的应该正好是挑出去的 3 条（file: 已能收进来）",
			res.Report.SourceCount, res.Report.ResultCount)
	}
}

// 筛完一条都没剩下的文件夹不建：侧栏上一个点进去空无一物的文件夹，
// 只会让人以为"导漏了"。文件名要在报告里点名。
func Test筛空了的文件夹不建但要点名(t *testing.T) {
	res := convertRealWorld(t)
	for _, g := range res.Bookmarks.Groups {
		if strings.Contains(g.Title, "全都进不来的文件夹") {
			t.Errorf("空文件夹不该建出来：%q", g.Title)
		}
	}
	if len(res.Report.EmptyGroups) != 1 ||
		!strings.Contains(res.Report.EmptyGroups[0], "全都进不来的文件夹") {
		t.Errorf("空文件夹要在报告里点名，实际 %v", res.Report.EmptyGroups)
	}
}

// 同一份文件跑两次必须得到同一套 id：id 会写进 bookmarks.json 当作寻址依据，
// 而导入要重试、要对账，两次不一样就分不清"重跑了一遍"还是"换了个人导的"。
func Test同一份文件两次转换id一样(t *testing.T) {
	a, b := convertRealWorld(t), convertRealWorld(t)
	if len(a.Bookmarks.Groups) != len(b.Bookmarks.Groups) {
		t.Fatalf("两次的分组数不同：%d vs %d", len(a.Bookmarks.Groups), len(b.Bookmarks.Groups))
	}
	for i := range a.Bookmarks.Groups {
		ga, gb := a.Bookmarks.Groups[i], b.Bookmarks.Groups[i]
		if ga.ID != gb.ID {
			t.Fatalf("分组 id 不稳：%q vs %q", ga.ID, gb.ID)
		}
		if len(ga.Items) != len(gb.Items) {
			t.Fatalf("分组 %q 的条目数不稳：%d vs %d", ga.ID, len(ga.Items), len(gb.Items))
		}
		for j := range ga.Items {
			if ga.Items[j].ID != gb.Items[j].ID {
				t.Errorf("条目 id 不稳：%q vs %q", ga.Items[j].ID, gb.Items[j].ID)
			}
		}
	}
}

// 一条都进不来的文件是**另一个形状**，不是"文件里没书签"：把两者的提示混成
// 同一句，会让人以为文件选错了，于是去换一个同样导不进来的文件。
func Test全部进不来时报的是另一句话(t *testing.T) {
	const allBad = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<TITLE>Bookmarks</TITLE>
<DL><p>
  <DT><H3>小工具</H3>
  <DL><p>
    <DT><A HREF="javascript:void(0)">甲</A>
    <DT><A HREF="javascript:void(1)">乙</A>
  </DL><p>
</DL><p>`
	res, err := Convert([]byte(allBad), realIndex(t), Options{})
	if err != nil {
		t.Fatalf("转换不该报错（它读出了书签），实际 %v", err)
	}
	if countItems(res.Bookmarks) != 0 {
		t.Fatalf("不该有条目进档案，实际 %d 条", countItems(res.Bookmarks))
	}
	if len(res.Report.Skipped) != 2 {
		t.Errorf("两条都该被点名，实际 %d 条", len(res.Report.Skipped))
	}
}

// 判据只有 model.CheckURL 那一份：这里用一份假判据说"放过"，就能看到它漂了。
// 反过来说，这条证人断的是**接线**——model.CheckURL 出错时导入会跟着拒。
func Test进不来的判据就是写路径那一份(t *testing.T) {
	cases := map[string]struct {
		url   string
		skip  bool
		whyIt string
	}{
		"http":           {"https://example.com/", false, "普通网页"},
		"chrome 带主机名":    {"chrome://extensions/", false, "§6.2 要求原样保留"},
		"javascript 无主机": {"javascript:void(0)", true, "点了会在这一页里执行"},
		"data 无主机":       {"data:text/html,x", true, "同上"},
		"mailto 无主机":     {"mailto:a@b.c", true, "是动作不是目的地"},
		"file 有路径":       {"file:///C:/Users/Administrator/Downloads/x.html", false, "本地路径照收（2026-10-12）"},
		"file 缺路径":       {"file://", true, "没有路径就不是一条地址"},
		"空":              {"", true, "空地址"},
	}
	for name, tc := range cases {
		got := skipReason(nbBookmark{Name: "某条书签", URL: tc.url}) != ""
		if got != tc.skip {
			t.Errorf("%s：跳过去=%v，要 %v（%s）", name, got, tc.skip, tc.whyIt)
		}
	}
	// 空名字：写路径也拒（model 的 name 判据），所以这里必须一并挑出去。
	if skipReason(nbBookmark{Name: "   ", URL: "https://example.com/"}) == "" {
		t.Error("空名字的书签该被挑出去")
	}
}
