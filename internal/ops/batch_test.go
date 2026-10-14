package ops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 第 1 条失败不许影响第 0 条已经做成的改动——但整批最终仍不落盘，
// 因为调用方（Task 5）看到 errs 非空就丢弃结果。这里同时钉两件事：
// 错误逐条收集、结果仍是"跑完的样子"。
func TestBatchCollectsEveryIndependentError(t *testing.T) {
	b, errs, _ := Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/name", Value: mustValue(t, "改名")},
		{Op: "set", Path: "groups/ghost/items/dsm/name", Value: mustValue(t, "x")},
		{Op: "set", Path: "groups/nas/items/nothing/name", Value: mustValue(t, "y")},
	}, nil)
	if len(errs) != 2 {
		t.Fatalf("两条独立错误都要报出来, got %+v", errs)
	}
	if errs[0].OpsIndex != 1 || errs[1].OpsIndex != 2 {
		t.Fatalf("opsIndex 必须指到出错的那条: %+v", errs)
	}
	if b.Groups[0].Items[0].Name != "改名" {
		t.Fatalf("失败不该回滚已成功的条目（整批由调用方丢弃）")
	}
}

// 前一条 add 出来的分组，后一条要能往里加条目：校验必须按"演化中的状态"
// 做，不能拿原始快照判存在性。
func TestBatchSeesItsOwnEarlierWrites(t *testing.T) {
	b, errs, _ := Run(seed(), []Op{
		{Op: "add", Path: "groups", Value: mustValue(t, map[string]any{
			"id": "read", "title": "读", "layout": "tiles"})},
		{Op: "add", Path: "groups/read/items", Value: mustValue(t, map[string]any{
			"id": "weread", "name": "微信读书", "url": "https://weread.qq.com", "icon": "tabler:book"})},
	}, nil)
	if len(errs) != 0 {
		t.Fatalf("级联写入该成功: %+v", errs)
	}
	if len(b.Groups) != 2 || len(b.Groups[1].Items) != 1 {
		t.Fatalf("新分组或新条目没落地: %+v", b.Groups)
	}
}

// 级联：第 0 条建组失败（这里用非法 layout，组 id 是全新的），第 1 条往
// 那个组加条目也该报错，而不是静默写进不存在的组。
// 注意别用重名建组：seed() 里已经有 nas，第 1 条会写进那条**预先存在**的
// 组，断言看着绿、实际什么都没测（评审 Important #2）。
func TestFailedOpDoesNotFabricateItsDependent(t *testing.T) {
	_, errs, _ := Run(seed(), []Op{
		{Op: "add", Path: "groups", Value: mustValue(t, map[string]any{
			"id": "read", "title": "读", "layout": "columns"})},
		{Op: "add", Path: "groups/read/items", Value: mustValue(t, map[string]any{
			"id": "weread", "name": "微信读书", "url": "http://10.0.0.9", "icon": "tabler:book"})},
	}, nil)
	if len(errs) != 2 {
		t.Fatalf("建组失败，依赖它的第 1 条也该报错，一共两条: %+v", errs)
	}
	if errs[0].OpsIndex != 0 || errs[1].OpsIndex != 1 {
		t.Fatalf("两条错误该各自指到自己那条 op: %+v", errs)
	}
}

// 整档不合法（这里把条目的 url 清空）必须由收尾的 Validate 抓住，
// 且这条错误是兜底的形状：field 是 bookmarks、挂在真正改成东西的那条 op 上。
// 只断言"有错误"的话，错误来自 op 层还是兜底根本分不出来（评审 Important #4）。
func TestBatchFailsOnResultThatIsNotValid(t *testing.T) {
	_, errs, _ := Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/url", Value: mustValue(t, "")},
	}, nil)
	if len(errs) != 1 {
		t.Fatalf("空 URL 该被拒（model.Validate 兜底）: %+v", errs)
	}
	if errs[0].Field != "bookmarks" {
		t.Errorf("兜底的 field 该是诚实的 bookmarks，got %q (%+v)", errs[0].Field, errs[0])
	}
	if errs[0].OpsIndex != 0 {
		t.Errorf("兜底该挂在改成脏结果的那条 op（0）上，got %+v", errs[0])
	}
}

// 兜底不许冤枉无辜的 op：第 1 条自己已经失败（路径里的组不存在），
// 脏结果是第 0 条留下的，错误就该指第 0 条。用 len(in)-1 会指到第 1 条
// ——一条本来就失败、根本没碰过结果的 op（评审 Important #1）。
func TestCatchAllBlamesTheLastOpThatActuallyChanged(t *testing.T) {
	_, errs, _ := Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/url", Value: mustValue(t, "")},
		{Op: "set", Path: "groups/ghost/items/x/name", Value: mustValue(t, "y")},
	}, nil)
	if len(errs) != 2 {
		t.Fatalf("op 层一条 + 兜底一条: %+v", errs)
	}
	if errs[0].OpsIndex != 1 || errs[0].Field != "name" {
		t.Errorf("op 层的错误该指名第 1 条: %+v", errs[0])
	}
	if errs[1].OpsIndex != 0 || errs[1].Field != "bookmarks" {
		t.Errorf("兜底该挂在最后一条真正改动的 op（0）上，got %+v", errs[1])
	}
}

// 告警不许冒充错误：缺 icon 与非 http 协议都照常写，只是要告诉调用方。
// 用 ftp:// 而不是 magnet:/mailto: ——后者没有主机名，model.Validate 会拒
// （spec §6.2 只拒"空/解析不了/缺主机名"三种，无 host 正是其中一种）。
func TestWarningsDoNotBlockTheWrite(t *testing.T) {
	_, errs, warns := Run(seed(), []Op{
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, map[string]any{
			"id": "share", "name": "共享盘", "url": "ftp://10.0.0.3/pub"})},
	}, nil)
	if len(errs) != 0 {
		t.Fatalf("非 http 协议允许存（spec §6.2）: %+v", errs)
	}
	// 断实际文案而不是数量：`len(warns) < 2` 只要攒够两条就算过，谁也不知道
	// 是不是"缺 icon"与"外部协议"这两条各自在场（少一条、多一条同类的都能绿）。
	// 挑的子串不与 id/name/url 等被断言数据重合，不会偶然永真。
	joined := strings.Join(warns, "\n")
	for _, want := range []string{"没有 icon", "协议浏览器不会从页面跳过去"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warns 该含「%s」这一条，实际: %+v", want, warns)
		}
	}
}

// 空批不是"写坏了"，是"什么都没做"：不许报错，也不许有告警。
// 脏入档那条是 Step 3 那个 bug 的证人：兜底若对空批照跑，就会把入档自身
// 的毛病报成一条 op 错误，而 opsIndex 只能是个不存在的下标（评审 Important #3）。
// 现在这道门有两重：`len(in)==0` 早退与 `last >= 0`；两道一起拆才会红
// （plan 的 Step 3 代码块两道都留了）。
func TestEmptyBatchIsHarmless(t *testing.T) {
	t.Run("合法入档", func(t *testing.T) {
		_, errs, warns := Run(seed(), nil, nil)
		if len(errs) != 0 || len(warns) != 0 {
			t.Fatalf("空批该静默: %+v %+v", errs, warns)
		}
	})
	t.Run("脏入档也不许伪造第 -1 条", func(t *testing.T) {
		dirty := seed()
		dirty.Groups[0].Items[0].URL = ""
		if dirty.Validate() == nil {
			t.Fatal("夹具该是不合法的，否则这条证人什么都没测")
		}
		_, errs, warns := Run(dirty, nil, nil)
		if len(errs) != 0 || len(warns) != 0 {
			t.Fatalf("空批什么都没做，不许把入档自身的毛病报成 op 错误: %+v %+v", errs, warns)
		}
	})
}

// op 的 JSON 形状是对外契约的一部分，键名钉一次。
func TestOpJSONShape(t *testing.T) {
	var o Op
	raw := `{"op":"add","path":"groups/nas/items","value":{"id":"a","name":"b","url":"http://10.0.0.3"},"index":0,"to":"nas"}`
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatal(err)
	}
	if o.Op != "add" || o.Path != "groups/nas/items" || o.Index == nil || *o.Index != 0 || o.To != "nas" {
		t.Fatalf("op JSON 键没解出来: %+v", o)
	}
}

// Run 是纯函数：改动只许落在副本上，入档一条不许动。浅拷贝（out := *b）
// 会顺着共享的底层数组把调用方那份 b 就地改脏——条目数被摘掉、名字被
// 覆盖——而所有只查返回值、夹具又是现造的用例全都照样绿。
// 断言方式：跑前跑后各序列化一次整档比字节。一条断言覆盖所有嵌套切片与
// 标量（rev/title/alias/tags 都在内），不会因先头的 Fatalf 让后面的检查够不着。
func TestRunNeverTouchesTheInputBookmarks(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   Op
	}{
		{"remove 只许摘副本", Op{Op: "remove", Path: "groups/nas/items/dsm"}},
		{"set 只许改副本", Op{Op: "set", Path: "groups/nas/items/dsm/name", Value: mustValue(t, "改名")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := seed()
			before, err := json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}
			_, errs, _ := Run(in, []Op{tc.op}, nil)
			if len(errs) != 0 {
				t.Fatalf("%s 失败: %+v", tc.name, errs)
			}
			after, err := json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Errorf("跑完 Run 之后入档被就地改脏:\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestRemoveItemAndGroup(t *testing.T) {
	b, errs, _ := Run(seed(), []Op{
		{Op: "remove", Path: "groups/nas/items/dsm"},
	}, nil)
	if len(errs) != 0 {
		t.Fatalf("remove 条目失败: %+v", errs)
	}
	if len(b.Groups[0].Items) != 1 || b.Groups[0].Items[0].ID != "qbit" {
		t.Fatalf("该只剩 qbit: %+v", b.Groups[0].Items)
	}
	b, errs, _ = Run(seed(), []Op{
		{Op: "remove", Path: "groups/nas"},
	}, nil)
	if len(errs) != 0 || len(b.Groups) != 0 {
		t.Fatalf("remove 整组失败: %+v %+v", errs, b.Groups)
	}
	// 不存在的目标该报错并指到那一条 op，不是静默成功。
	_, errs, _ = Run(seed(), []Op{
		{Op: "remove", Path: "groups/nas/items/ghost"},
	}, nil)
	if len(errs) != 1 || errs[0].OpsIndex != 0 {
		t.Fatalf("remove 不存在的条目该红: %+v", errs)
	}
}

// seedThree 给同组挪位用：两条目时"先插后删"那类破坏必然把结果搞成 id
// 重复，证人只能靠"errs 非空"红；三条目时同样的破坏仍是合法档案，
// 只有顺序断言会红——那才是这条要测的东西（评审 Minor）。
func seedThree() *model.Bookmarks {
	b := seed()
	b.Groups[0].Items = append(b.Groups[0].Items, model.Item{
		ID: "jelly", Name: "影视库", URL: "http://10.0.0.3:8096", Alias: []string{}, Tags: []string{},
	})
	return b
}

// 同组换位：源与目标是同一个 *model.Group。这条是"指针别名有没有写坏"
// 唯一的证人——跨组 move 测不到它。
func TestMoveWithinOneGroupReorders(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
		op   Op
	}{
		{"首条挪到末", "qbit,jelly,dsm", Op{Op: "move", Path: "groups/nas/items/dsm", To: "nas", Index: ptr(2)}},
		{"末条挪到首", "jelly,dsm,qbit", Op{Op: "move", Path: "groups/nas/items/jelly", To: "nas", Index: ptr(0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, errs, _ := Run(seedThree(), []Op{tc.op}, nil)
			if len(errs) != 0 {
				t.Fatalf("同组 move 失败: %+v", errs)
			}
			got := []string{}
			for _, it := range b.Groups[0].Items {
				got = append(got, it.ID)
			}
			if strings.Join(got, ",") != tc.want {
				t.Errorf("没换到位: got %v, want %s", got, tc.want)
			}
		})
	}
}

// 失败的那条 op 不许留下半改状态：move 若先摘除再验 index，越界报错时
// 条目已经从源组消失，Run"跳过失败的那条继续跑"的前提就塌了——第 1 条
// 是在残档上跑的，报出来的错误也不诚实。
func TestFailedMoveLeavesTheResultUntouched(t *testing.T) {
	b, errs, _ := Run(seedThree(), []Op{
		{Op: "move", Path: "groups/nas/items/dsm", To: "nas", Index: ptr(9)},
		{Op: "set", Path: "groups/nas/items/qbit/name", Value: mustValue(t, "改名")},
	}, nil)
	if len(errs) != 1 {
		t.Fatalf("只有越界的第 0 条该报错: %+v", errs)
	}
	if errs[0].OpsIndex != 0 || errs[0].Field != "index" {
		t.Errorf("错误该指到第 0 条的 index: %+v", errs[0])
	}
	got := []string{}
	names := map[string]string{}
	for _, it := range b.Groups[0].Items {
		got = append(got, it.ID)
		names[it.ID] = it.Name
	}
	if strings.Join(got, ",") != "dsm,qbit,jelly" {
		t.Errorf("失败的 move 该什么都没改，条目被它偷摘了一条: got %v", got)
	}
	if names["qbit"] != "改名" {
		t.Errorf("第 1 条 set 该照常生效: %+v", b.Groups[0].Items)
	}
}

func TestMoveAcrossGroupsAndMissingTargets(t *testing.T) {
	base := seed()
	base.Groups = append(base.Groups, model.Group{
		ID: "read", Title: "读", Layout: model.LayoutTiles,
		Items: []model.Item{{ID: "book", Name: "书", URL: "http://10.0.0.8", Alias: []string{}, Tags: []string{}}},
	})
	b, errs, _ := Run(base, []Op{
		{Op: "move", Path: "groups/nas/items/dsm", To: "read", Index: ptr(0)},
	}, nil)
	if len(errs) != 0 {
		t.Fatalf("跨组 move 失败: %+v", errs)
	}
	if len(b.Groups[0].Items) != 1 || b.Groups[1].Items[0].ID != "dsm" {
		t.Fatalf("搬完两边形状不对: %+v %+v", b.Groups[0].Items, b.Groups[1].Items)
	}
	// 目标组不存在 / 缺 to / 越界 index 三样：前两条的错误按 path 归责

	// （fieldOf 只认 op 自己的路径段），越界那条由 fieldErr 钉出 index。
	for _, tc := range []struct {
		name  string
		op    Op
		field string
	}{
		{"目标组不存在", Op{Op: "move", Path: "groups/nas/items/dsm", To: "ghost"}, "path"},
		{"缺 to", Op{Op: "move", Path: "groups/nas/items/dsm"}, "path"},
		{"index 越界", Op{Op: "move", Path: "groups/nas/items/dsm", To: "read", Index: ptr(9)}, "index"},
	} {
		_, errs, _ := Run(base, []Op{tc.op}, nil)
		if len(errs) != 1 {
			t.Fatalf("%s 该报错: %+v", tc.name, errs)
		}
		if errs[0].Field != tc.field {
			t.Errorf("%s 的错误该归到 %q，got %q (%+v)", tc.name, tc.field, errs[0].Field, errs[0])
		}
	}
}

// 「挪到队尾」那条通路：**index 缺席就是追加到目标组末尾**。
//
// 2026-10-15 前端加了「拖到组标题上 = 排到那组最后」那个落点，它发出去的 op
// **刻意不带 index**——把"末尾"再自己算一遍下标，正是队尾那个 bug 当初的来路
// （客户端按下标插，而下标永远算不出一条能落在最后面的）。所以这条判据必须在这儿
// 钉住：它是那条 UI 唯一的地基，两种方向都验。
func TestMoveWithoutIndexAppendsToTheEnd(t *testing.T) {
	withTarget := func() *model.Bookmarks {
		b := seedThree() // nas: dsm, qbit, jelly
		b.Groups = append(b.Groups, model.Group{
			ID: "read", Title: "读", Layout: model.LayoutTiles,
			Items: []model.Item{{ID: "book", Name: "书", URL: "http://10.0.0.8", Alias: []string{}, Tags: []string{}}},
		})
		return b
	}
	// 跨组：源组三条，目标组一条，搬过去要落在**目标组最后**（不是开头）。
	b, errs, _ := Run(withTarget(), []Op{{Op: "move", Path: "groups/nas/items/dsm", To: "read"}}, nil)
	if len(errs) != 0 {
		t.Fatalf("跨组且不带 index 的 move 失败: %+v", errs)
	}
	if got := ids(b.Groups[1].Items); got != "book,dsm" {
		t.Errorf("目标组 = %q，要 %q（book 在前，搬过去的落在末尾）", got, "book,dsm")
	}
	if got := ids(b.Groups[0].Items); got != "qbit,jelly" {
		t.Errorf("源组 = %q，要 %q", got, "qbit,jelly")
	}
	// 同组：把首条拖到组标题上（末尾）。源与目标是同一张活切片，摘除会让它短一格，
	// 这条最容易在"bound"那一处写错。
	b, errs, _ = Run(withTarget(), []Op{{Op: "move", Path: "groups/nas/items/dsm", To: "nas"}}, nil)
	if len(errs) != 0 {
		t.Fatalf("同组且不带 index 的 move 失败: %+v", errs)
	}
	if got := ids(b.Groups[0].Items); got != "qbit,jelly,dsm" {
		t.Errorf("同组 = %q，要 %q", got, "qbit,jelly,dsm")
	}
}

func ids(items []model.Item) string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return strings.Join(out, ",")
}
