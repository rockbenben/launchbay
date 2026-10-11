package icons

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func searchIndex(t *testing.T) *Index {
	t.Helper()
	si := mustDecode(t, fixtureDoc(t, "simple-icons", "fill", 3, map[string]any{
		"docker": "M0 0", "docker-alt": "M0 1", "portainer": "M0 2",
	}, map[string]string{"moby": "docker"}))
	tab := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 3, map[string]any{
		"container": "M0 3", "container-off": "M0 4", "box": "M0 5",
	}, nil))
	return Merge(si, tab)
}

func TestSearch_排序是精确大于前缀大于包含(t *testing.T) {
	got := searchIndex(t).Search("docker", "", 10)
	if got.Total != 2 {
		t.Fatalf("total = %d，要 2（docker 与 docker-alt）", got.Total)
	}
	if got.Icons[0] != "simple-icons:docker" {
		t.Errorf("精确命中必须排第一，得到 %s", got.Icons[0])
	}
}

func TestSearch_品牌层先于通用层(t *testing.T) {
	// "container" 在两层都能匹配上 tabler:container 与 simple-icons 无；
	// 换成两侧都有子串的查询词才测得出层序。
	ix := searchIndex(t)
	got := ix.Search("o", "", 20)
	var firstLayer []string
	for _, k := range got.Icons {
		if strings.HasPrefix(k, "simple-icons:") {
			firstLayer = append(firstLayer, k)
		}
	}
	if len(firstLayer) == 0 {
		t.Fatal("没匹配到品牌层条目，夹具不对")
	}
	// 同 rank 内品牌层必须整体排在 tabler 前面。
	seenTabler := false
	for _, k := range got.Icons {
		if strings.HasPrefix(k, "tabler:") {
			seenTabler = true
			continue
		}
		if seenTabler {
			t.Errorf("%s 排在 tabler 之后：层优先级丢了", k)
		}
	}
}

func TestSearch_alias命中归一到规范名且不重复(t *testing.T) {
	got := searchIndex(t).Search("moby", "", 10)
	if got.Total != 1 {
		t.Fatalf("total = %d，要 1（别名不该变成第二个条目）", got.Total)
	}
	if got.Icons[0] != "simple-icons:docker" {
		t.Errorf("要规范名，得到 %s", got.Icons[0])
	}
}

func TestSearch_带前缀的查询词按整键查(t *testing.T) {
	// 这条只钉"整键通路的形状"：夹具的 simple-icons 层里没有 box，所以它看不见
	// "另一层同名条目插到前面"这个 bug（真实产物上 q=tabler:box 的 Total 是 111、
	// 第一条是 simple-icons:box）。钉那条的是 TestSearch_整键命中只回那一条_夹具
	// 与 TestSearch_真实产物整键回查只回那一条，别只看这条就放心。
	got := searchIndex(t).Search("tabler:box", "", 10)
	if got.Total != 1 || got.Icons[0] != "tabler:box" {
		t.Errorf("得到 %+v", got.Icons)
	}
	// 前缀过滤同时生效：查 simple-icons 就不该看到它。
	if other := searchIndex(t).Search("tabler:box", "simple-icons", 10); other.Total != 0 {
		t.Errorf("prefix=simple-icons 却返回了 tabler 条目：%+v", other.Icons)
	}
}

func TestSearch_零命中是空切片不是nil(t *testing.T) {
	got := searchIndex(t).Search("zzz-not-a-name", "", 10)
	if got.Total != 0 {
		t.Fatal("不该有命中")
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	// §4.3：空数组写 []，写 null 会让 AI 判出"端点坏了"。
	if !strings.Contains(string(raw), `"icons":[]`) {
		t.Errorf("序列化形状不对: %s", raw)
	}
}

func TestSearch_limit截断而total留全(t *testing.T) {
	got := searchIndex(t).Search("o", "", 2)
	if len(got.Icons) != 2 {
		t.Fatalf("返回 %d 条，limit 是 2", len(got.Icons))
	}
	if got.Total < 3 {
		t.Errorf("total = %d，应当是匹配总数而不是返回条数", got.Total)
	}
}

func TestSearch_prefix过滤(t *testing.T) {
	if got := searchIndex(t).Search("container", "simple-icons", 10); got.Total != 0 {
		t.Errorf("过滤没生效：%+v", got.Icons)
	}
	if got := searchIndex(t).Search("container", "tabler", 10); got.Total != 2 {
		t.Errorf("tabler 里 container 与 container-off 都该命中，得到 %d", got.Total)
	}
}

func TestSearch_空查询与大写都归一(t *testing.T) {
	ix := searchIndex(t)
	if got := ix.Search("", "", 10); got.Total != 0 {
		t.Error("空查询一律不返回全量——9 千条的名字没有意义")
	}
	if got := ix.Search("  DOCker  ", "", 10); got.Total != 2 {
		t.Errorf("要去空格与小写化，得到 %+v", got)
	}
}

// TestSearch_集合名不是图标名 钉住裁定 1：匹配只看 ':' 之后的名字段。
// 若实现拿全键做子串匹配，q="tabler" 会把 6265 条 tabler 全命中、
// q="simple"/"icons" 会把整个品牌层命中——那是"搜索"的假象，也是 Task 5
// 端点上最容易骗过调用方的行为。q="tabler:"（冒号收尾、名字段为空）同理：
// 空名字段一旦进子串匹配就是 Contains(x, "") 恒真，等于吐全量索引，
// 违反"空查询一律返回空"。
func TestSearch_集合名不是图标名(t *testing.T) {
	ix := searchIndex(t)
	for _, q := range []string{"tabler", "simple", "icons", "tabler:", "simple-icons:"} {
		if got := ix.Search(q, "", 10); got.Total != 0 {
			t.Errorf("q=%q 命中 %d 条，要 0：集合前缀不是图标名，空名字段也不该放行：%+v",
				q, got.Total, got.Icons)
		}
	}
}

// TestSearch_真实产物也能搜到docker 是夹具之外的一层保险：夹具测形状，
// 这条验在真实内嵌产物上"搜 docker 第一屏就该有 simple-icons:docker"，
// 万一 byPrefix 装配或层序在真实数据上漂了，这里会红。
// 实测 tabler 层有 brand-docker（名字段含 docker，属合法的包含命中），
// 所以 prefix=tabler 的断言口径是"命中里有 tabler:brand-docker 且没有
// 品牌层条目"，而不是"零命中"——后者是我对真实数据的错判，实现没错。
func TestSearch_真实产物也能搜到docker(t *testing.T) {
	ix := MustLoad()
	got := ix.Search("docker", "", 10)
	if got.Total < 1 {
		t.Fatalf("真实产物里搜 docker 零命中，得到 %+v", got)
	}
	if got.Icons[0] != "simple-icons:docker" {
		t.Errorf("精确命中且品牌层优先，第一条要 simple-icons:docker，得到 %s", got.Icons[0])
	}
	filtered := ix.Search("docker", "tabler", 10)
	if filtered.Total < 1 {
		t.Fatalf("prefix=tabler 里 brand-docker 应当命中，得到 %+v", filtered)
	}
	want := false
	for _, k := range filtered.Icons {
		if k == "tabler:brand-docker" {
			want = true
		}
		if strings.HasPrefix(k, "simple-icons:") {
			t.Errorf("prefix=tabler 却端回了品牌层条目 %s", k)
		}
	}
	if !want {
		t.Errorf("prefix=tabler 的结果里要见到 tabler:brand-docker，得到 %+v", filtered.Icons)
	}
}

// TestSearch_limit为0时Total仍是匹配总数 钉住 I-1：limit 只管 Icons 的条数，管不
// 着 Total。spec §6.1 的立意就是"零命中"与"命中被截断"是两个可分的状态，把
// limit==0 做成提前返回、连带 Total 归零，就是把两个状态塌成同一个读数；而
// limit<0 走另一套形状（Total 报真数），0 与 -1 不一致本身就是 bug。
// Search 是包内导出的方法，Task 5 的端点钳位救不了 Go 侧直接调它的调用方。
func TestSearch_limit为0时Total仍是匹配总数(t *testing.T) {
	ix := searchIndex(t)
	zero := ix.Search("docker", "", 0)
	if zero.Total != 2 {
		t.Fatalf("limit=0 时 total = %d，要匹配总数 2（docker 与 docker-alt）", zero.Total)
	}
	if zero.Icons == nil {
		t.Fatal("limit=0 时 Icons 也要是非 nil 空切片：Task 5 把它直接序列化出去，null 会被 AI 判成端点坏了")
	}
	if len(zero.Icons) != 0 {
		t.Fatalf("limit=0 不该返回条目，得到 %+v", zero.Icons)
	}
	neg := ix.Search("docker", "", -1)
	if neg.Total != 2 || len(neg.Icons) != 0 || neg.Icons == nil {
		t.Errorf("limit=-1 要与 limit=0 同形（total=2、零条、非 nil），得到 total=%d icons=%+v",
			neg.Total, neg.Icons)
	}
	miss := ix.Search("zzz-not-a-name", "", 0)
	if miss.Total != 0 {
		t.Errorf("零命中要 total=0，得到 %d", miss.Total)
	}
	// 两状态可分：同一个 limit=0 之下，有命中与零命中的 Total 必须不同。
	if miss.Total == zero.Total {
		t.Errorf("零命中(%d)与被截断(%d)塌成了同一个读数", miss.Total, zero.Total)
	}
	// 只有空查询才让 Total 归零：limit 的形状不许把"没查询"变成全量放行。
	if empty := ix.Search("", "", 0); empty.Total != 0 {
		t.Errorf("空查询要 total=0，得到 %d", empty.Total)
	}
	raw, err := json.Marshal(zero)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"icons":[]`) {
		t.Errorf("limit=0 的序列化形状不对: %s", raw)
	}
}

// TestSearch_真实产物limit为0仍报真Total 是 I-1 在真实产物上的读数：夹具只有两条
// 命中，"截断"与"零命中"的差别在真实数据上才看得见（探针实测 q=home 全量 47 条）。
func TestSearch_真实产物limit为0仍报真Total(t *testing.T) {
	ix := MustLoad()
	got := ix.Search("home", "", 0)
	if got.Total < 1 {
		t.Fatalf("limit=0 把有命中的查询报成 total=0：真实产物上 q=home 有几十条匹配，与被截断这个状态就该和零命中同形（I-1）")
	}
	if got.Icons == nil || len(got.Icons) != 0 {
		t.Errorf("limit=0 要非 nil 空 Icons，得到 %+v", got.Icons)
	}
	if miss := ix.Search("zzzz-no-such-icon-name-xyzzy", "", 0); miss.Total != 0 || miss.Total == got.Total {
		t.Errorf("零命中(total=%d)与被截断(total=%d)必须可分", miss.Total, got.Total)
	}
}

// tierIndex 是 I-2 的夹具：一个查询词 "box" 同时命中四个档位——
// simple-icons:box 精确、simple-icons:box-alt 前缀、tabler:cube 别名（tabler 的
// 别名 box→cube，归一后的父名自己不含 box，所以只能从别名这一支进来）、
// simple-icons:toolbox 包含。四条分属四档，任何两档互换都会打乱逐位断言。
func tierIndex(t *testing.T) *Index {
	t.Helper()
	si := mustDecode(t, fixtureDoc(t, "simple-icons", "fill", 3, map[string]any{
		"box": "M0 0", "box-alt": "M0 1", "toolbox": "M0 2",
	}, nil))
	tab := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1, map[string]any{
		"cube": "M0 3",
	}, map[string]string{"box": "cube"}))
	return Merge(si, tab)
}

// TestSearch_档位排序精确大于前缀大于别名大于包含 是档位排序的证人（I-2）。
// 原来那条同名测试只断言了 Total 与 Icons[0]，从没比较过前缀档与包含档的相对
// 次序：评审把 rankPrefix 与 rankInfix 互换后十条全绿，而真实数据 q=home 的第
// 2..6 条肉眼可见地从 tabler:home-2 那一档掉到了包含档之后。spec §12 把
// "精确 > 前缀 > 含" 列为必须测的行为，这条把它逐位钉死，换档必红。
// 别名档（第 3 位）一并纳入逐位断言，M-1 记档的那点就没剩了。
func TestSearch_档位排序精确大于前缀大于别名大于包含(t *testing.T) {
	got := tierIndex(t).Search("box", "", 10)
	want := []string{"simple-icons:box", "simple-icons:box-alt", "tabler:cube", "simple-icons:toolbox"}
	if got.Total != len(want) {
		t.Fatalf("total = %d，要 %d，得到 %+v", got.Total, len(want), got.Icons)
	}
	if len(got.Icons) != len(want) {
		t.Fatalf("返回 %d 条，要 %d 条：%+v", len(got.Icons), len(want), got.Icons)
	}
	for i, w := range want {
		if got.Icons[i] != w {
			t.Errorf("第 %d 位 = %s，要 %s：档位序必须是精确 > 前缀 > 别名 > 包含，任意两档互换这条要红",
				i, got.Icons[i], w)
		}
	}
}

// wholeKeyIndex 是 I-3 的夹具：两层都有 box。病根正是"夹具里没有第二个叫 box 的
// 东西"，所以这里 simple-icons 层必须也放一条 box（外加 tabler:boxhome 给包含档
// 凑热闹），让"整键命中后还继续跑子串"这种实现无处藏。
func wholeKeyIndex(t *testing.T) *Index {
	t.Helper()
	si := mustDecode(t, fixtureDoc(t, "simple-icons", "fill", 2, map[string]any{
		"box": "M0 0", "box-alt": "M0 1",
	}, nil))
	tab := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 2, map[string]any{
		"box": "M0 2", "boxhome": "M0 3",
	}, nil))
	return Merge(si, tab)
}

// TestSearch_整键命中只回那一条_夹具 钉 I-3 的语义：调用方拿全键回查就是为了
// 确认那一条，结果第一条是另一层的同名条目 = "搜到了写不进"的近亲。
func TestSearch_整键命中只回那一条_夹具(t *testing.T) {
	ix := wholeKeyIndex(t)
	got := ix.Search("tabler:box", "", 10)
	if got.Total != 1 || len(got.Icons) != 1 || got.Icons[0] != "tabler:box" {
		t.Errorf("整键命中要只回 tabler:box 一条，得到 total=%d icons=%+v", got.Total, got.Icons)
	}
	// prefix 把命中的那一条排除掉时结果就是空，不许退回去搜同名别的层。
	if other := ix.Search("tabler:box", "simple-icons", 10); other.Total != 0 || len(other.Icons) != 0 {
		t.Errorf("prefix=simple-icons 时整键命中的 tabler:box 被排除，要 0 条，得到 total=%d icons=%+v",
			other.Total, other.Icons)
	}
	// 整键命不中才退回名字段子串搜索：AI 抄来的名字可能带错前缀，这条路要留着。
	back := ix.Search("tabler:box-alt", "", 10)
	if back.Total != 1 || back.Icons[0] != "simple-icons:box-alt" {
		t.Errorf("tabler:box-alt 不存在，本该按名字段搜出 simple-icons:box-alt，得到 total=%d icons=%+v",
			back.Total, back.Icons)
	}
}

// TestSearch_真实产物整键回查只回那一条 是 I-3 在真实产物上的同一件事。
// 探针实测（修复前）：q=tabler:box → total=111、first=simple-icons:box；
// q=tabler:box&prefix=simple-icons → total=21。夹具那条给的是虚假安慰，
// 真实数据这条才是证人。
func TestSearch_真实产物整键回查只回那一条(t *testing.T) {
	ix := MustLoad()
	got := ix.Search("tabler:box", "", 10)
	if got.Total != 1 || len(got.Icons) != 1 || got.Icons[0] != "tabler:box" {
		t.Errorf("真实产物上整键回查 tabler:box 要只回那一条，得到 total=%d icons=%+v", got.Total, got.Icons)
	}
	if !ix.Has(got.Icons[0]) {
		t.Errorf("整键回查返回的名字 %s 连 Has 都不认", got.Icons[0])
	}
	if other := ix.Search("tabler:box", "simple-icons", 10); other.Total != 0 || len(other.Icons) != 0 {
		t.Errorf("prefix=simple-icons 排除了整键命中的 tabler:box，要 0 条，得到 total=%d icons=%+v",
			other.Total, other.Icons)
	}
	// tabler 层没有 docker：整键命不中，退回名字段子串，两条合法命中照旧。
	back := ix.Search("tabler:docker", "", 10)
	if back.Total != 2 || back.Icons[0] != "simple-icons:docker" || back.Icons[1] != "tabler:brand-docker" {
		t.Errorf("整键命不中要退回子串搜索，得到 total=%d icons=%+v", back.Total, back.Icons)
	}
}

// TestSearch_prefix大小写与空格都归一 钉住 M-2：q 做 TrimSpace+ToLower 而 prefix
// 原样比，两个参数口径不一致，prefix=Tabler 会静默 0 命中——与"这个前缀下真的
// 没有"同形。这里只做归一，**不**做非法前缀报错：白名单外的前缀返回空，拒掉
// 非法值是 Task 5 handler 的判据（那里要 422，不是 0 命中）。
func TestSearch_prefix大小写与空格都归一(t *testing.T) {
	ix := searchIndex(t)
	want := ix.Search("container", "tabler", 10)
	if want.Total != 2 {
		t.Fatalf("夹具里 tabler 层本该有 container 与 container-off 两条命中，得到 %+v", want)
	}
	for _, p := range []string{"TABLER", "Tabler", " tabler ", "TaBlEr"} {
		got := ix.Search("container", p, 10)
		if got.Total != want.Total || !reflect.DeepEqual(got.Icons, want.Icons) {
			t.Errorf("prefix=%q 要和 prefix=tabler 同结果（total=%d %v），得到 total=%d %v",
				p, want.Total, want.Icons, got.Total, got.Icons)
		}
	}
	// 品牌层同样归一，且归一后过滤仍然生效：不许因为 ToLower 把 tabler 的条目放进来。
	si := ix.Search("docker", "SIMPLE-ICONS", 10)
	if si.Total != 2 || strings.HasPrefix(si.Icons[0], "tabler:") {
		t.Errorf("prefix=SIMPLE-ICONS 要 2 条且没有 tabler 条目，得到 total=%d icons=%+v", si.Total, si.Icons)
	}
	if got := ix.Search("container", "mdi", 10); got.Total != 0 {
		t.Errorf("白名单外的前缀要返回空（报错归端点层），得到 %d", got.Total)
	}
}
