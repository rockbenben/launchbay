package bookimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/icons"
)

// 用**真实的内嵌索引**跑这些用例，别用假的桩。
// 覆盖表的全部意义就是"这个名字在真实产物里到底存不存在"，用桩就验不出这一点。
func realIndex(t *testing.T) *icons.Index {
	t.Helper()
	ix, err := icons.Load()
	if err != nil {
		t.Fatalf("真实图标索引读不出来：%v", err)
	}
	return ix
}

// pickName 取一个确实存在的图标**裸名**（不带 `prefix:` 前缀）。
//
// `Names()` 返回的是**全键**（内部 byPrefix 存的就是 `prefix + ":" + name`），
// 直接拿它的返回值用会拼出 `simple-icons:simple-icons:xxx` 这种查不到的东西，
// 然后你会以为索引有问题。剥完再 Has() 一次，让"剥错了"当场红，
// 而不是让后面某个断言去背这个锅。
func pickName(t *testing.T, ix *icons.Index, prefix string) string {
	t.Helper()
	names := ix.Names(prefix)
	if len(names) == 0 {
		t.Fatalf("%s 这套索引里一个名字都没有", prefix)
	}
	bare := strings.TrimPrefix(names[len(names)/2], prefix+":")
	if bare == names[len(names)/2] {
		t.Fatalf("Names(%q) 返回的不是全键：%q——本用例对 Names 语义的假设错了", prefix, names[len(names)/2])
	}
	if !ix.Has(prefix + ":" + bare) {
		t.Fatalf("剥出裸名 %q 后反而查不到了，索引行为与预期不符", bare)
	}
	return bare
}

func writeOverrides(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "icons.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// 表不存在**不是**空表：退到随二进制走的那份内嵌表（2026-10-10）。
//
// 以前这里是"没有表 = 空表"，而网页上的导入（/import）恰恰不传路径——它要的就是
// 内嵌那份。旧形状下报告里那句"在 internal/bookimport/icons.json 里写…"教人去改
// 一个容器里够不着的文件，是一句装饰话。
func TestLoadOverrides_没有文件就退到内嵌表(t *testing.T) {
	m, err := loadOverrides(filepath.Join(t.TempDir(), "没有这个文件.json"), realIndex(t))
	if err != nil {
		t.Fatalf("没有覆盖表文件是完全正常的，不该报错：%v", err)
	}
	if len(m) == 0 {
		t.Fatal("该退到内嵌表，实际是空表——导入报告里那句「去 icons.json 里写」就成了空话")
	}
	// 内嵌表里每一项都必须真实存在于索引：loadOverrides 自己逐项校验，
	// 而这张表是**随每个发行版发出去**的——它错一项就是所有人的导入都中止。
	for host, icon := range m {
		if !realIndex(t).Has(icon) {
			t.Errorf("内嵌覆盖表里的 %s -> %q 不在索引里，这一项会让整次导入中止", host, icon)
		}
	}
}

// 空字符串路径是网页那条路的形状（Options{} 不带 OverridesPath）。
func TestLoadOverrides_空路径走内嵌表(t *testing.T) {
	m, err := loadOverrides("", realIndex(t))
	if err != nil {
		t.Fatalf("空路径该走内嵌表：%v", err)
	}
	if len(m) == 0 {
		t.Error("空路径该拿到内嵌表，实际空")
	}
}

// 覆盖表的值逐项对索引校验，不过就中止。写错的 icon 存进 bookmarks.json
// 不会当场报错——它会安静地走首字母兜底渲染，直到某天有人动 batch 改这一条
// 才被 422 拒，那时没人记得它是哪次导入带进来的。
func TestLoadOverrides_值指不到真实图标就中止(t *testing.T) {
	ix := realIndex(t)
	good := pickName(t, ix, "simple-icons")

	if _, err := loadOverrides(writeOverrides(t, `{"nas.example.internal":"simple-icons:`+good+`"}`), ix); err != nil {
		t.Fatalf("指向真实存在的图标名却报错：%v", err)
	}

	_, err := loadOverrides(writeOverrides(t, `{"nas.example.internal":"simple-icons:肯定不存在-zzz"}`), ix)
	if err == nil {
		t.Fatal("覆盖表指向不存在的图标却放行了——那会写出坏档数据")
	}
	// 报错要说清是哪一项、并且讲清后果：只说"格式错误"的话，
	// 填表的人会以为是自己 JSON 写坏了，而实际上是他填了个不存在的图标名。
	if !strings.Contains(err.Error(), "肯定不存在-zzz") {
		t.Errorf("报错要点名是哪个图标，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "nas.example.internal") {
		t.Errorf("报错要点名是哪一条书签的 host，实际 %v", err)
	}
}

// 猜测是两级：覆盖表优先，其余按网址猜（复用 icons.Candidates 的顺序判据）。
// 猜不到就返回空——**宁可空着走首字母兜底，也不要配一个形似的图标顶上**。
func TestIconGuesser_两级且落空返回空(t *testing.T) {
	ix := realIndex(t)
	good := pickName(t, ix, "tabler")
	g := &iconGuesser{ix: ix, overrides: map[string]string{
		"nas.example.internal:5000": "tabler:" + good,
	}}

	t.Run("覆盖表命中且带端口", func(t *testing.T) {
		got, how := g.guess("http://nas.example.internal:5000/x", "NAS")
		if got != "tabler:"+good || how != "覆盖表" {
			t.Errorf("覆盖表应命中（含端口），得到 %q / %q", got, how)
		}
	})

	t.Run("按网址精确命中", func(t *testing.T) {
		// simple-icons 里躺着大量服务品牌名，拿真名当 host 必然精确命中。
		name := pickName(t, ix, "simple-icons")
		got, how := g.guess("https://"+name+".example.com/", "随便什么标题")
		if got != "simple-icons:"+name {
			t.Fatalf("索引里确实有 simple-icons:%s，该精确命中，实际得到 %q", name, got)
		}
		if how != "按网址精确命中" {
			t.Errorf("来源标记应是「按网址精确命中」，实际 %q", how)
		}
	})

	// **这条是本用例群里最要紧的一支**：导入是自动落库，没有人在旁边挑，
	// 所以"模糊匹配取第一条"等于自动配错。
	//
	// 曾经这支用的是 `zzz` 开头的域名，结果它精确命中了 `tabler:zzz`——
	// **那个图标真的在索引里**（zzz 是 tabler 的睡眠符号）。所以"我觉得这个名字
	// 不存在"是错的判据：这里先向索引问一句，确认不存在再拿它当反例。
	// （当初换成模糊的 Candidates 时，这一支报的也是它，反而掩盖了真正的问题。）
	t.Run("模糊匹配不算命中", func(t *testing.T) {
		const fake = "zzzqqqxx-not-a-real-icon"
		if ix.Has("tabler:"+fake) || ix.Has("simple-icons:"+fake) {
			t.Fatalf("反例的名字 %q 居然真在索引里——换一个，别让这一支变成假绿", fake)
		}
		got, how := g.guess("https://"+fake+".example/", "随便什么标题")
		if got != "" || how != "" {
			t.Errorf("猜不到就该是空：猜出来的 %q 会被当成图标存进档案，而它几乎肯定错。来源 %q", got, how)
		}
	})

	t.Run("非 http 地址不猜", func(t *testing.T) {
		// `chrome://extensions/` 的路径段 extensions 会模糊撞上 gitextensions。
		if got, _ := g.guess("chrome://extensions/", "扩展"); got != "" {
			t.Errorf("chrome:// 点不动，不该猜图标（实测它撞出了 simple-icons:gitextensions），实际 %q", got)
		}
	})
}

// 同一个站的多条没猜到时只记一次：那批书签要的是"去给这个站配个图标"，
// 报 20 遍同一个 host 只会让人以为有 20 件事要做。
func TestIconGaps_同host只记一次但计总数(t *testing.T) {
	g := newIconGaps()
	g.add("第一条", "https://nas.example/a", "nas.example")
	g.add("第二条", "https://nas.example/b", "nas.example")
	g.add("第三条", "https://other.example/c", "other.example")

	got := g.list()
	if len(got) != 2 {
		t.Fatalf("两个 host 应出两条记录，实际 %d 条", len(got))
	}
	if g.n != 3 {
		t.Errorf("没猜到的总条数是 3（用来告诉人「影响了几条」），实际 %d", g.n)
	}
	if got[0].Host != "nas.example" {
		t.Errorf("按 host 排序，第一条应是 nas.example，实际 %q", got[0].Host)
	}
}
