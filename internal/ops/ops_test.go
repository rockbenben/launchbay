package ops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

func seed() *model.Bookmarks {
	return &model.Bookmarks{
		Rev:   7,
		Title: "发射台",
		Groups: []model.Group{{
			ID: "nas", Title: "NAS", Layout: model.LayoutList,
			Items: []model.Item{
				{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
				{ID: "qbit", Name: "下载", URL: "http://10.0.0.3:8181", Alias: []string{}, Tags: []string{}},
			},
		}},
	}
}

// baseBookmarks 是图标名写校验用例的夹具：单个分组 nas、单条目 dsm。
// 与 seed() 的区别只有一个条目——"add 新条目被拒且不动入参"那条要断言
// 入参条目数仍为 1，seed() 的两条目会把断言变成永远真（证人自废）。
func baseBookmarks() *model.Bookmarks {
	return &model.Bookmarks{
		Rev:   7,
		Title: "发射台",
		Groups: []model.Group{{
			ID: "nas", Title: "NAS", Layout: model.LayoutList,
			Items: []model.Item{
				{ID: "dsm", Name: "群晖 DSM", URL: "http://10.0.0.3:5000", Alias: []string{}, Tags: []string{}},
			},
		}},
	}
}

func mustValue(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAddItemAppendsAndInserts(t *testing.T) {
	item := map[string]any{"id": "jellyfin", "name": "影视库", "url": "http://10.0.0.3:8096"}
	b, errs, warns := Run(seed(), []Op{
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, item)},
	}, nil)
	if len(errs) != 0 {
		t.Fatalf("add 失败: %+v", errs)
	}
	last := b.Groups[0].Items[len(b.Groups[0].Items)-1]
	if last.ID != "jellyfin" || last.Name != "影视库" {
		t.Fatalf("没追加上新条目: %+v", last)
	}
	// 没带 icon 是可用的（首字母兜底），但必须告警：AI 该知道自己漏了。
	// 断数量还不够——告警的用处是让调用方知道**哪一条**缺什么，所以文案
	// 里的 op 序号与条目 id 也要一起钉住（只断非空等于没断）。
	if len(warns) != 1 {
		t.Fatalf("缺 icon 该有一条告警, got %+v", warns)
	}
	if !strings.Contains(warns[0], "op 0: 条目 jellyfin 没有 icon") {
		t.Errorf("告警要把 op 序号与条目 id 说清楚, got %q", warns[0])
	}
	// index=0 插到最前面。
	b, errs, _ = Run(seed(), []Op{
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, item), Index: ptr(0)},
	}, nil)
	if len(errs) != 0 || b.Groups[0].Items[0].ID != "jellyfin" {
		t.Fatalf("index=0 插入失败: %+v %+v", errs, b.Groups[0].Items)
	}
	// 越界 index 是错误，不是"插到最后"。
	_, errs, _ = Run(seed(), []Op{
		{Op: "add", Path: "groups/nas/items", Value: mustValue(t, item), Index: ptr(9)},
	}, nil)
	if len(errs) != 1 || errs[0].Field != "index" {
		t.Fatalf("越界 index 该报 index 字段错误: %+v", errs)
	}
}

func TestSetFieldAndNoOpWarning(t *testing.T) {
	b, errs, warns := Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/name", Value: mustValue(t, "Synology DSM")},
		{Op: "set", Path: "groups/nas/title", Value: mustValue(t, "NAS")},
	}, nil)
	if len(errs) != 0 {
		t.Fatalf("set 失败: %+v", errs)
	}
	if b.Groups[0].Items[0].Name != "Synology DSM" {
		t.Fatalf("名字没改掉: %+v", b.Groups[0].Items[0])
	}
	// 第二条把 title 设成它本来的值：改了个没变的东西值得一条告警。
	// 同上一条：断的是文案与 op 序号，不只是"有条告警"。
	if len(warns) != 1 {
		t.Fatalf("未变化的 set 该告警, got %+v", warns)
	}
	if warns[0] != "op 1: 设的值与当前值相同，什么都没改" {
		t.Errorf("未变化告警要指到第 1 条并说清什么都没改, got %q", warns[0])
	}
	// 类型不对：字段要字符串，给了数组。
	_, errs, _ = Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/name", Value: mustValue(t, []string{"a"})},
	}, nil)
	if len(errs) != 1 || errs[0].Field != "name" {
		t.Fatalf("类型不符该报字段错: %+v", errs)
	}
	// id 与 rev 不在白名单里。
	_, errs, _ = Run(seed(), []Op{
		{Op: "set", Path: "groups/nas/items/dsm/id", Value: mustValue(t, "other")},
	}, nil)
	if len(errs) != 1 {
		t.Fatalf("set id 必须被拒: %+v", errs)
	}
}

// stubIcons 是 ops 测试用的假索引。ops 不 import internal/icons，
// 只认接口——索引那边换实现、换产物格式都不惊动写校验。
type stubIcons map[string]bool

func (s stubIcons) Has(icon string) bool { return s[icon] }

// legacyIconBookmarks 是"历史数据里带着陌生图标名"的夹具：两组，条目
// legacy 的图标名**形状合法**（过 model.checkIcon 的 ASCII slug 正则，
// 否则红在形状上、证不到存在性闸门），但**真索引里没有**
// （icons.MustLoad().Has("tabler:notarealicon") = false）。
func legacyIconBookmarks() *model.Bookmarks {
	return &model.Bookmarks{
		Rev:   7,
		Title: "发射台",
		Groups: []model.Group{
			{ID: "nas", Title: "NAS", Layout: model.LayoutList, Items: []model.Item{
				{ID: "legacy", Name: "老书签", URL: "http://10.0.0.5/", Icon: "tabler:notarealicon",
					Alias: []string{}, Tags: []string{}},
			}},
			{ID: "tools", Title: "工具", Layout: model.LayoutList, Items: []model.Item{}},
		},
	}
}

// TestRun_move不重查图标名 是裁定三（move 不接图标闸门）在测试层唯一的证人。
// 判据本身写在 ops.go 的 move 注释里，注释不是证人：评审实测把闸门接到 move 上
// 之后整个 ops 套件仍全绿，也就是说"顺手补全一致性"这种退化没人抓得住。
// 非 nil 的 set 里故意没有 tabler:notarealicon —— move 若去查存在性就会红。
func TestRun_move不重查图标名(t *testing.T) {
	set := stubIcons{"tabler:home": true, "simple-icons:docker": true}

	out, errs, _ := Run(legacyIconBookmarks(), []Op{{
		Op: "move", Path: "groups/nas/items/legacy", To: "tools",
	}}, set)
	if len(errs) != 0 {
		t.Fatalf("带陌生图标名的历史条目挪一次组必须写得进去（move 不重查）: %+v", errs)
	}
	if n := len(out.Groups[0].Items); n != 0 {
		t.Errorf("源分组还剩 %d 条，move 该把它搬空", n)
	}
	if n := len(out.Groups[1].Items); n != 1 {
		t.Fatalf("目标分组有 %d 条，要正好搬过去 1 条", n)
	}
	moved := out.Groups[1].Items[0]
	if moved.ID != "legacy" {
		t.Fatalf("搬过去的是 %q，要 legacy", moved.ID)
	}
	// 陌生名字必须原样留着：顺手清掉等于替历史数据编第二份真相，
	// 而且下一次读回来就没人知道它原来设过什么图标。
	if moved.Icon != "tabler:notarealicon" {
		t.Errorf("图标名 = %q，要原样带着 tabler:notarealicon（不许被顺手清掉）", moved.Icon)
	}
}

func TestRun_写路径认图标名(t *testing.T) {
	set := stubIcons{"tabler:home": true, "simple-icons:docker": true}

	t.Run("set 陌生名字要拒", func(t *testing.T) {
		_, errs, _ := Run(baseBookmarks(), []Op{{
			Op: "set", Path: "groups/nas/items/dsm/icon", Value: json.RawMessage(`"tabler:home2"`),
		}}, set)
		if len(errs) != 1 {
			t.Fatalf("要一条错误，得到 %+v", errs)
		}
		if errs[0].Field != "icon" {
			t.Errorf("field = %q，要 icon", errs[0].Field)
		}
		if !strings.Contains(errs[0].Reason, "/api/v1/icons") {
			t.Errorf("拒绝理由要把搜索端点交给调用方，否则它只会再猜一个: %q", errs[0].Reason)
		}
	})

	t.Run("set 合法名字要过", func(t *testing.T) {
		out, errs, _ := Run(baseBookmarks(), []Op{{
			Op: "set", Path: "groups/nas/items/dsm/icon", Value: json.RawMessage(`"tabler:home"`),
		}}, set)
		if len(errs) != 0 {
			t.Fatalf("合法名字被拒了: %+v", errs)
		}
		if out.Groups[0].Items[0].Icon != "tabler:home" {
			t.Error("没写进去")
		}
	})

	t.Run("auto 与空串归 ops 判而不是索引判", func(t *testing.T) {
		for _, v := range []string{`""`, `"auto"`} {
			_, errs, _ := Run(baseBookmarks(), []Op{{
				Op: "set", Path: "groups/nas/items/dsm/icon", Value: json.RawMessage(v),
			}}, set)
			if len(errs) != 0 {
				t.Errorf("%s 是语义值，不该被索引拒绝: %+v", v, errs)
			}
		}
	})

	t.Run("add 新条目带陌生名字要拒且不动磁盘状态", func(t *testing.T) {
		before := baseBookmarks()
		_, errs, _ := Run(before, []Op{{
			Op: "add", Path: "groups/nas/items",
			Value: json.RawMessage(`{"id":"new","name":"N","url":"http://10.0.0.2/","icon":"tabler:made-up-name"}`),
		}}, set)
		if len(errs) != 1 || errs[0].Field != "icon" {
			t.Fatalf("要一条 icon 字段的错误，得到 %+v", errs)
		}
		if errs[0].OpsIndex != 0 {
			t.Errorf("opsIndex = %d，要指到那条 add", errs[0].OpsIndex)
		}
		if len(before.Groups[0].Items) != 1 {
			t.Error("失败的批次必须什么都不留：入参被原地改了")
		}
	})

	t.Run("add 整个分组时逐条检查嵌套条目", func(t *testing.T) {
		// 坏名字那条的 id 是 qdgroup、好名字那条是 keepgood：两个都是刻意挑的
		// 生僻串，理由里出现它们只能是因为实现**点名了那一条**，不是偶然撞上
		// 某个常用子串（旧断言查 "no" 就撞上过图标名 tabler:not-in-the-set，
		// 坏名字不点名也照样命中，评审 Important #2）。
		_, errs, _ := Run(baseBookmarks(), []Op{{
			Op: "add", Path: "groups",
			Value: json.RawMessage(`{"id":"dev","title":"开发","layout":"list","items":[` +
				`{"id":"keepgood","name":"KG","url":"http://10.0.0.3/","icon":"tabler:home"},` +
				`{"id":"qdgroup","name":"QD","url":"http://10.0.0.4/","icon":"tabler:not-in-the-set"}]}`),
		}}, set)
		if len(errs) != 1 {
			t.Fatalf("嵌套条目里的陌生名字要被抓出来: %+v", errs)
		}
		if errs[0].Field != "icon" {
			t.Errorf("field = %q，要 icon（由闸门抓，不是收尾 Validate 的前缀拒）", errs[0].Field)
		}
		if !strings.Contains(errs[0].Reason, "qdgroup") {
			t.Errorf("理由要点名是哪一条（%q），否则一整组十几条时没人知道改哪条", errs[0].Reason)
		}
		// 反向断言：只把整组名字都念一遍不叫点名——那等于没指认，调用方还是得
		// 逐条试。好名字那条的 id 不许出现在拒绝理由里。
		if strings.Contains(errs[0].Reason, "keepgood") {
			t.Errorf("理由不该连好名字的条目一起念（%q）：点名只点坏的那一条", errs[0].Reason)
		}
	})

	t.Run("索引为 nil 就不检查", func(t *testing.T) {
		// 这不是给运行时留的后门：装配点必须传真索引（Task 5），
		// nil 只出现在"这条测试不关心图标"的场合。写清楚，别让人误以为
		// 服务可以不带索引启动。
		//
		// 图标名取形状合法的 `tabler:...`：这条证的是"存在性闸门关了"，取值
		// 只能是**形状合法、只是索引里没有**的名字。用 `mdi:whatever` 会被
		// 收尾的 model.Validate 按前缀拒掉（红错原因），证不到"不查存在性"。
		_, errs, _ := Run(baseBookmarks(), []Op{{
			Op: "set", Path: "groups/nas/items/dsm/icon", Value: json.RawMessage(`"tabler:whatever-not-in-index"`),
		}}, nil)
		if len(errs) != 0 {
			t.Errorf("nil 索引不该拦: %+v", errs)
		}
	})
}

func ptr[T any](v T) *T { return &v }
