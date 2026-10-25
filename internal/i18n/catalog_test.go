package i18n

import (
	"fmt"
	"strings"
	"testing"
)

// 守恒：两种语言的 key 集合必须逐条相等。
//
// 这是"翻全了"唯一的机械保证。靠人眼一条一条数文案，早晚会漏一处；而漏一处的
// 后果不是编译错，是那一行英文界面印出 ⟨admin.act.delete⟩ 这样一眼可辨的东西——
// 它至少不是静默的空白，但仍然是坏的页面。
func TestCatalog_两种语言的key守恒(t *testing.T) {
	zh := Load(Zh)
	en := Load(En)

	zhKeys := map[string]bool{}
	for _, k := range zh.Keys() {
		zhKeys[k] = true
	}
	enKeys := map[string]bool{}
	for _, k := range en.Keys() {
		enKeys[k] = true
	}

	var missingInEn, missingInZh []string
	for _, k := range zh.Keys() {
		if !enKeys[k] {
			missingInEn = append(missingInEn, k)
		}
	}
	for _, k := range en.Keys() {
		if !zhKeys[k] {
			missingInZh = append(missingInZh, k)
		}
	}
	if len(missingInEn) > 0 {
		t.Errorf("英文包少了 %d 条：%s", len(missingInEn), strings.Join(missingInEn, "、"))
	}
	if len(missingInZh) > 0 {
		t.Errorf("中文包少了 %d 条：%s", len(missingInZh), strings.Join(missingInZh, "、"))
	}
}

// 占位符守恒：同一个 key 在两种语言里的 %s/%d 序列必须逐位相同。
//
// 顺序也算：把 "找到 %d 条，这个档案一共 %d 条" 译成 "%d total, %d found"
// 数量对得上、位置全错——界面上就会说"一共 6 条，找到 20 条"，而且完全读得通，
// 只是说反了。这一条比上一条更难靠眼睛发现。
func TestCatalog_占位符序列守恒(t *testing.T) {
	zh := Load(Zh)
	en := Load(En)

	for _, k := range zh.Keys() {
		zp := placeholders(zh.msgs[k])
		ep := placeholders(en.msgs[k])
		if zp != ep {
			t.Errorf("key %q 的占位符对不上：\n  zh = %s\n  en = %s", k, zp, ep)
		}
	}
}

func placeholders(s string) string {
	var out []string
	for i := 0; i < len(s)-1; i++ {
		if s[i] != '%' {
			continue
		}
		switch s[i+1] {
		case 's', 'd', 'v', 'q':
			out = append(out, string(s[i:i+2]))
			i++
		case '%':
			i++
		}
	}
	return strings.Join(out, " ")
}

// 占位符个数不得超过实参个数，否则 fmt 会印出 %!d(MISSING) ——那种东西出现在页面上
// 会被当成"这个项目的作者不会写 Go"，而它真正的原因只是调用点少传了一个。
func TestCatalog_实参够了不会印MISSING(t *testing.T) {
	c := Load(En)
	got := c.T("profile.hits", 6, 20)
	if strings.Contains(got, "%!") {
		t.Errorf("占位符没填满：%q", got)
	}
	if got != "6 found out of 20 in this profile" {
		t.Errorf("英文渲染不对：%q", got)
	}
	gotZh := Load(Zh).T("profile.hits", 6, 20)
	if gotZh != "找到 6 条，这个档案一共 20 条" {
		t.Errorf("中文渲染不对：%q", gotZh)
	}
}

// 缺 key 要**看得见**，不能是空串——空串会变成界面上的一块空白，而空白看着像设计如此。
func TestCatalog_缺key返回尖括号而不是空串(t *testing.T) {
	for _, l := range All {
		got := Load(l).T("no.such.key")
		if got == "" {
			t.Fatalf("%s：缺 key 印出了空串", l)
		}
		if !strings.Contains(got, "no.such.key") {
			t.Errorf("%s：缺 key 的返回值里没有 key 自己：%q", l, got)
		}
	}
}

// 不认识的语言要落回默认，而不是崩：配置是 data/settings.json，手改写错一个字节
// 不该让一台跑着的导航台打不开。
func TestParseLang_不认识的值落回默认(t *testing.T) {
	for _, s := range []string{"", "zh", "en", "EN", "fr", "en-US", "中文", "  "} {
		got := ParseLang(s)
		switch s {
		case "zh", "en":
			if Lang(s) != got {
				t.Errorf("ParseLang(%q) 认错了，拿到 %q", s, got)
			}
		default:
			if got != Default {
				t.Errorf("ParseLang(%q) 应当落回 %q，拿到 %q", s, Default, got)
			}
		}
	}
}

// 语言列表是页面上那个下拉框的**唯一来源**。
// 每种语言都要能用它自己的名字报出自己，否则那个下拉框里会出现一个空项。
func TestLang_每种语言都能报出自己的名字(t *testing.T) {
	if len(All) < 2 {
		t.Fatalf("All 少于两种语言，测试意义不大：%v", All)
	}
	seen := map[string]bool{}
	for _, l := range All {
		n := l.DisplayName()
		if strings.TrimSpace(n) == "" {
			t.Errorf("%s 的 DisplayName 是空的", l)
		}
		if seen[n] {
			t.Errorf("两种语言报了同一个名字 %q，下拉框里分不出来", n)
		}
		seen[n] = true
		if l.HTMLLang() == "" {
			t.Errorf("%s 的 HTMLLang 是空的", l)
		}
	}
}

// 零 JSON 里不许出现裸换行或制表符：那种字节会让整个语言包解析失败，
// 而失败的形状是"服务起不来"，代价与收益完全不成比例。
func TestCatalog_语言包都是合法JSON且非空(t *testing.T) {
	for _, l := range All {
		c := Load(l)
		if len(c.Keys()) == 0 {
			t.Errorf("%s 的语言包是空的", l)
		}
		for _, k := range c.Keys() {
			if strings.HasPrefix(k, " ") || strings.HasSuffix(k, " ") {
				t.Errorf("%s 的 key %q 两头有空格", l, k)
			}
			if c.msgs[k] == "" {
				t.Errorf("%s 的 key %q 的文案是空串——空串在界面上是一块空白，"+
					"而空白看着像设计如此（要删这条文案就删掉整个 key）", l, k)
			}
		}
	}
}

// 每条中文文案都要在两种语言里都出现——这个已经被上面两条覆盖了，
// 这里补的是另一件事：**中文包不许出现英文残留**。翻译时漏改一处会在这里被逮到，
// 而不是等一个英文用户在页面上看到 "已读到标题"。
func TestCatalog_英文包不许夹带中文(t *testing.T) {
	en := Load(En)
	for _, k := range en.Keys() {
		v := en.msgs[k]
		for _, r := range v {
			if r >= 0x4e00 && r <= 0x9fff {
				t.Errorf("英文包的 %q 里还留着中文：%q", k, v)
				break
			}
		}
	}
}

func TestCatalog_每个key都能在每种语言里查到(t *testing.T) {
	for _, l := range All {
		c := Load(l)
		for _, k := range c.Keys() {
			if got := c.T(k); strings.HasPrefix(got, "⟨") {
				t.Errorf("%s：%q 明明在语言包里却查不到，返回 %q", l, k, got)
			}
		}
	}
}

func TestCatalog_缓存给的是同一份(t *testing.T) {
	a := Load(En)
	b := Load(En)
	if a != b {
		t.Error("两次 Load 拿到的不是同一份，语言包被重复解析了")
	}
}

func TestCatalog_非法语言也能拿到可用文案(t *testing.T) {
	// 直接 Load 一个不存在的语言，必须落回默认而不是 panic——
	// 语言包是仓库里的静态文件，真坏了也该由本包的测试报出来，
	// 而不是让一台跑着的机器因为某个字节改错就起不来。
	c := Load(Lang("fr"))
	if c == nil {
		t.Fatal("Load 不认识的语言返回了 nil")
	}
	if c.T("profile.switch") == "" {
		t.Error("落回默认之后仍然没有可用的文案")
	}
	if c.Lang() != Default {
		t.Errorf("落回之后 Lang() 应该是 %q，拿到 %q", Default, c.Lang())
	}
}

func ExampleCatalog_T() {
	c := Load(En)
	fmt.Println(c.T("profile.hits", 6, 20))
	// Output: 6 found out of 20 in this profile
}
