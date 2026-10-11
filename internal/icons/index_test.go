package icons

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecode_两种写法都收(t *testing.T) {
	raw := fixtureDoc(t, "tabler", "stroke", 2, map[string]any{
		"home": "M3 12h18",
		"bell": []any{[]any{"path", "d", "M6 18v-6"}, []any{"path", "d", "M18 18v-6"}},
	}, nil)
	ix := mustDecode(t, raw)

	if got := ix.Count("tabler"); got != 2 {
		t.Errorf("收录条数 = %d，要 2", got)
	}
	h, ok := ix.Get("tabler:home")
	if !ok {
		t.Fatal("tabler:home 查不到")
	}
	if len(h.Elements) != 1 || h.Elements[0].Tag != "path" || h.Elements[0].Attrs[0].Value != "M3 12h18" {
		t.Errorf("裸 d 写法解出来不对: %+v", h.Elements)
	}
	if h.Layer != "stroke" || h.ViewBox != "0 0 24 24" {
		t.Errorf("层与 viewBox 没带上: %+v", h)
	}
	b := mustGet(t, ix, "tabler:bell")
	if len(b.Elements) != 2 {
		t.Fatalf("两 path 图标应当各留一条，得到 %d", len(b.Elements))
	}
}

func TestDecode_自证count与实际条数(t *testing.T) {
	raw := fixtureDoc(t, "tabler", "stroke", 7, map[string]any{"home": "M0 0"}, nil)
	if _, err := Decode(raw); err == nil {
		t.Fatal("count 说 7 条实际 1 条，必须报错——这条断言防的就是解析器吞掉一半")
	} else if !strings.Contains(err.Error(), "count") {
		t.Errorf("错误里要点了 count，得到: %v", err)
	}
}

func TestDecode_未知tag未知attr与越界值都拒(t *testing.T) {
	cases := map[string]map[string]any{
		"未知 tag":  {"home": []any{[]any{"script", "src", "x"}}},
		"未知 attr": {"home": []any{[]any{"path", "onclick", "alert(1)", "d", "M0 0"}}},
		"值里藏引号":   {"home": []any{[]any{"path", "d", `"/><script>alert(1)</script>`}}},
		"值里藏尖括号":  {"home": []any{[]any{"path", "d", "<svg onload=x>"}}},
		"空元素数组":   {"home": []any{}},
		"元素不成对":   {"home": []any{[]any{"path", "d"}}},
	}
	for name, icons := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(fixtureDoc(t, "tabler", "stroke", 1, icons, nil)); err == nil {
				t.Fatal("该拒的没拒")
			}
		})
	}
}

func TestDecode_fill只存与层默认值不同的那个(t *testing.T) {
	// stroke 层的默认是 fill:none；出现 currentColor 就是 filled 变体，必须留着，
	// 否则实心图标会被 CSS 描成空心——这是外观错误，不是省字节的对象。
	solid := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1,
		map[string]any{"home": []any{[]any{"path", "fill", "currentColor", "d", "M0 0"}}}, nil))
	el := mustGet(t, solid, "tabler:home").Elements[0]
	if len(el.Attrs) != 2 || el.Attrs[0].Name != "fill" {
		t.Errorf("stroke 层的 fill=\"currentColor\" 被丢了: %+v", el.Attrs)
	}
	// fill 层的默认是 currentColor：存进来只会被丢，别在产物里留它。
	if _, err := Decode(fixtureDoc(t, "simple-icons", "fill", 1,
		map[string]any{"github": []any{[]any{"path", "fill", "none", "d", "M0 0"}}}, nil)); err == nil {
		t.Error("fill 层出现 fill=\"none\" 是白名单外的情形，宁可报错也别静默改语义")
	}
}

func TestDecode_alias要指到真名字(t *testing.T) {
	if _, err := Decode(fixtureDoc(t, "tabler", "stroke", 1,
		map[string]any{"home": "M0 0"}, map[string]string{"house": "not-here"})); err == nil {
		t.Fatal("悬空 alias 必须报错")
	}
	ix := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1,
		map[string]any{"home": "M0 0"}, map[string]string{"house": "home"}))
	if !ix.Has("tabler:house") {
		t.Error("alias 应当算有")
	}
	if got, ok := ix.Canonical("tabler:house"); !ok || got != "tabler:home" {
		t.Errorf("Canonical 归一化 = %q %v，要 tabler:home true", got, ok)
	}
	// alias 不是第二个条目：Names 里只许出现规范名。
	for _, n := range ix.Names("tabler") {
		if n == "tabler:house" {
			t.Error("alias 混进了可写名单，磁盘上就会出现两种写法")
		}
	}
}

func TestDecode_层与viewBox形状(t *testing.T) {
	raw := fixtureDoc(t, "tabler", "gradient", 1, map[string]any{"home": "M0 0"}, nil)
	if _, err := Decode(raw); err == nil {
		t.Fatal("layer 只认 stroke/fill")
	}
}

func TestHas_空前缀与陌生前缀都是假(t *testing.T) {
	ix := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1, map[string]any{"home": "M0 0"}, nil))
	for _, bad := range []string{"", "auto", "home", "tabler", "tabler:", "mdi:home", "tabler:home-x", "tabler:HOME"} {
		if ix.Has(bad) {
			t.Errorf("Has(%q) 不该为真", bad)
		}
	}
}

// 下面三条补的是 brief 的漏：它把 DisallowUnknownFields、"整份 JSON 读不了"、
// "值越界即拒"三条不变量写进了实现，却只给了 count 自证一条证人。少了证人，
// 将来谁把那三行放宽掉都不会有人看见——而 DisallowUnknownFields 正是与
// tools/genicons 的 productDoc 对齐的唯一连锁闸（Task 2 写盘前自校验靠它响）。

func TestDecode_产物形状漂了整份拒(t *testing.T) {
	good := string(fixtureDoc(t, "tabler", "stroke", 1, map[string]any{"home": "M0 0"}, nil))
	drifted := strings.Replace(good, `{`, `{"generatorVersion":"x",`, 1)
	if drifted == good {
		t.Fatal("漂移样本没拼出来，这条测试自己失效了")
	}
	if _, err := Decode([]byte(drifted)); err == nil {
		t.Fatal("产物多出一个字段必须报错：生成器与解码器的 json tag 一旦漂了，这里就得炸")
	}
}

func TestDecode_截断的产物读不出来(t *testing.T) {
	good := string(fixtureDoc(t, "tabler", "stroke", 1, map[string]any{"home": "M0 0"}, nil))
	// 6MB 产物被截半的三种切口：刚起头、中间、少一个收尾括号。
	// 断言错误出处是解析器而不是 count 自证——截断必须由解析器挡下，
	// 否则自证就成了那道本不该独自承重的闸。
	for _, cut := range []int{1, len(good) / 2, len(good) - 1} {
		_, err := Decode([]byte(good[:cut]))
		if err == nil {
			t.Fatalf("截到 %d 字节却解成功了——静默返回半截索引就是首屏少图标的来源", cut)
		}
		if !strings.Contains(err.Error(), "图标产物读不了") {
			t.Errorf("截到 %d 字节的错误出处不对，要的是解析器： %v", cut, err)
		}
	}
	typed := strings.Replace(good, `"count":1`, `"count":"1"`, 1)
	if typed == good {
		t.Fatal("类型漂移样本没拼出来，这条断言自己失效了")
	}
	if _, err := Decode([]byte(typed)); err == nil {
		t.Error("count 的类型漂成字符串必须报错，不许静默放过")
	}
}

func TestDecode_白名单外的写法都拒(t *testing.T) {
	cases := map[string]struct {
		prefix, layer string
		icons         map[string]any
		why           string
	}{
		"fill 的第三色": {"tabler", "stroke", map[string]any{"home": []any{[]any{"path", "fill", "red", "d", "M0 0"}}}, "只认 currentColor/none"},
		"值不是字符串":    {"tabler", "stroke", map[string]any{"home": 42}, "既不是裸 d 也不是元素数组"},
		"元素不是数组":    {"tabler", "stroke", map[string]any{"home": []any{"path"}}, "既不是裸 d 也不是元素数组"},
		// 与本层默认值相同的 fill 不该进产物：生成器正是靠这条把 simple-icons 的
		// 3737 个 fill="currentColor" 全丢掉才换成裸 d 写法的体积。解码器认不下
		// 它，就等于这条体积契约只由生成器一面之词守着。
		"fill 层留了本层默认":   {"simple-icons", "fill", map[string]any{"github": []any{[]any{"path", "fill", "currentColor", "d", "M0 0"}}}, "与本层默认值相同"},
		"stroke 层留了本层默认": {"tabler", "stroke", map[string]any{"home": []any{[]any{"path", "fill", "none", "d", "M0 0"}}}, "与本层默认值相同"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(fixtureDoc(t, c.prefix, c.layer, 1, c.icons, nil))
			if err == nil {
				t.Fatal("该拒的没拒")
			}
			if !strings.Contains(err.Error(), c.why) {
				t.Errorf("错误出处不对，要的是「%s」，得到: %v", c.why, err)
			}
		})
	}
}

func TestDecode_viewBox形状非法整份拒(t *testing.T) {
	// viewBoxRE 钉的是字符集（挡掉引号、等号、逗号这类能逃出属性值的字符），
	// 分量个数由生成器写死的 "0 0 24 24" 保证——这里只拒字符集外的形状。
	// bad 一律按 JSON 字符串内部的写法给（产物里的一个转义引号要写成 \"），
	// 否则样本会在解析器就炸掉，这条测试会绿在错误的地方。
	base := string(fixtureDoc(t, "tabler", "stroke", 1, map[string]any{"home": "M0 0"}, nil))
	for _, bad := range []string{"", "abc", "0 0 24=24", "0,0,24,24", `0 0 24 24\" onload=\"x`} {
		t.Run(bad, func(t *testing.T) {
			raw := strings.Replace(base, `"viewBox":"0 0 24 24"`, `"viewBox":"`+bad+`"`, 1)
			if raw == base {
				t.Fatal("viewBox 样本没替换成功")
			}
			_, err := Decode([]byte(raw))
			if err == nil {
				t.Fatalf("viewBox=%q 居然解通过了", bad)
			}
			if !strings.Contains(err.Error(), "viewBox") {
				t.Errorf("错误出处不对，要的是 viewBox 形状那道闸，得到: %v", err)
			}
		})
	}
}

// TestMerge_两套并成一个查询面 钉的是 Task 2 装配点唯一会走的路径：两套产物并成
// 一个查询面。Merge 是 brief 的 Produces 清单里唯一会 panic 的函数，没有证人就等于
// 让装配点带着一个没测过的 panic 上线。
func TestMerge_两套并成一个查询面(t *testing.T) {
	tabler := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1,
		map[string]any{"home": "M0 0"}, map[string]string{"house": "home"}))
	si := mustDecode(t, fixtureDoc(t, "simple-icons", "fill", 1,
		map[string]any{"github": "M0 0"}, nil))
	ix := Merge(tabler, si)

	if got := ix.Count("tabler"); got != 1 {
		t.Errorf("合并后 tabler 条数 = %d，要 1", got)
	}
	if got := ix.Count("simple-icons"); got != 1 {
		t.Errorf("合并后 simple-icons 条数 = %d，要 1", got)
	}
	if got, ok := ix.Canonical("tabler:house"); !ok || got != "tabler:home" {
		t.Errorf("合并后 alias 归一化 = %q %v，要 tabler:home true", got, ok)
	}
	if got := mustGet(t, ix, "simple-icons:github").Layer; got != "fill" {
		t.Errorf("合并后层 = %q，要 fill", got)
	}
	if _, ok := ix.Get("mdi:home"); ok {
		t.Error("合并后的查询面不该凭空多出第三套的名字")
	}
	if len(ix.Names("tabler")) != 1 || len(ix.Names("simple-icons")) != 1 {
		t.Errorf("合并后每套的可写名单各 1 条，得到 %v / %v", ix.Names("tabler"), ix.Names("simple-icons"))
	}
	// 前缀重叠=产物被复制过，必须炸给程序员看，不能静默覆盖。
	defer func() {
		if recover() == nil {
			t.Error("同前缀重复合并应当 panic，静默覆盖会让「哪条图标存在」变成猜谜")
		}
	}()
	Merge(tabler, tabler)
}

// TestMerge_同前缀的两套产物必须炸 是上面那条 panic 断言的专职版本。
// 上一轮的证人只做了 Merge(tabler, tabler)——同一份索引进两遍，靠 recover()
// 兜住"任何 panic"，既不点名重复的键，也不是"两套各自合法、前缀相同"的产物
// （真实 Load 永远凑不出同前缀的两套，所以这条分支在别处零消费者）。
// 这里喂两份独立解码出来、前缀都是 tabler、home 都存在的产物，并且断言 panic
// 信息点名 tabler:home：删掉 panic 分支（改成静默后者覆盖）必须红在这一行。
func TestMerge_同前缀的两套产物必须炸(t *testing.T) {
	a := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1, map[string]any{"home": "M0 0"}, nil))
	b := mustDecode(t, fixtureDoc(t, "tabler", "stroke", 1, map[string]any{"home": "M3 12h18"}, nil))

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		Merge(a, b)
	}()
	if recovered == nil {
		t.Fatal("同前缀重复合并居然静默返回了：产物被复制过必须炸给程序员看，" +
			"静默后者覆盖会让「哪条图标存在」变成猜谜")
	}
	msg := fmt.Sprint(recovered)
	if !strings.Contains(msg, "tabler:home") {
		t.Errorf("panic 信息要点名重复的那个图标键，得到: %s", msg)
	}
}

func mustGet(t *testing.T, ix *Index, icon string) Icon {
	t.Helper()
	got, ok := ix.Get(icon)
	if !ok {
		t.Fatalf("%s 查不到", icon)
	}
	return got
}
