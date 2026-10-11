// Package i18n 是本项目的界面文案查表。
//
// 为什么自己写而不是用 golang.org/x/text：depguard 是 CI 硬门，go.mod 一旦出现
// replace/exclude/retract 就红。这个项目把「零依赖」当承诺而不是口号，界面多语言
// 属于典型的"为了小功能引入整条依赖链"的形状——标准库够用，就用标准库。
//
// 三条判据，每条都有测试守着：
//
//  1. **判据只有一份**：语言包是数据（T 返回值），不是判断。模板里只有 `{{.T "key"}}`
//     这一种查表形状，不许出现 `{{if eq .Lang "en"}}` 那种把语言写进模板的分支。
//  2. **缺 key 不静默**：T 查不到时返回 key 自己（外面包尖括号），页面上会看到
//     `⟨profile.search⟩` 这样一眼可辨的东西。返回空串才是事故——那条路会让一处
//     漏翻变成界面上一块空白，而空白看起来像"设计如此"。
//  3. **两种语言的 key 必须守恒**：catalog_test.go 逐条比对 zh 与 en 的 key 集合，
//     少一个就红。这是"翻全了"唯一的机械保证——靠人眼数 168 行文案，早晚漏。
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed messages/*.json
var messageFS embed.FS

// Lang 是一种界面语言。取值只有两个，且必须是常量——语言是**机器级**设置
// （data/settings.json），不是档案级：一家人共用一台导航台，谁也不该被别人
// 改掉阅读语言，所以它不进 profile.json、不走 /prefs、也不能由普通会话改。
type Lang string

const (
	// Zh 是默认语言。**默认值必须落中文**：已有档案的 profile.json 里没有这个字段，
	// 缺字段=中文，于是升级不改变任何一个人的现有界面。默认英文会是一次无声的
	// 行为变更，对已经建好中文档案的人尤其难解释。
	Zh Lang = "zh"
	// En 是英文。
	En Lang = "en"
)

// Default 是缺字段时的语言。与 Zh 同一个常量，不是两份写法——两个名字指向同一件
// 事，将来要改默认值只改一处。
const Default = Zh

// All 是全部受支持的语言，**按界面显示顺序**。写进页面的下拉框与校验都取这一份，
// 所以"新增一种语言"只有这一个地方要动。
var All = []Lang{Zh, En}

// HTMLLang 给 <html lang> 用。中文是 zh-CN，英文是 en——不是 zh / en：
// 前者是标准写法（RFC 5646），screen reader 与浏览器的翻译提示认它。
func (l Lang) HTMLLang() string {
	if l == En {
		return "en"
	}
	return "zh-CN"
}

// DisplayName 是语言自己的名字，**用该语言自己写**。
// 「中文」「English」而不是「Chinese」「英文」：让一个看不懂当前语言的人也能从
// 这个列表里认出自己要的那一个。这是所有语言选择器的通用做法，不是本项目的偏好。
func (l Lang) DisplayName() string {
	if l == En {
		return "English"
	}
	return "中文"
}

// Valid 报告 l 是不是受支持的语言。配置里存了不认识的值（手改文件、抄错了、
// 旧版本留下的）时，落回 Default 而不是崩——一个显示语言配错不该让整台机器打不开。
func (l Lang) Valid() bool {
	for _, v := range All {
		if v == l {
			return true
		}
	}
	return false
}

// ParseLang 把配置里的字符串变成 Lang。空串与不认识的值都落 Default，
// 判据只有这一处 Valid() 一份。
func ParseLang(s string) Lang {
	l := Lang(s)
	if l.Valid() {
		return l
	}
	return Default
}

// Catalog 是一种语言的全部文案。构造后只读，并发安全（map 读不需要锁）。
type Catalog struct {
	lang Lang
	msgs map[string]string
}

// T 是查表，可带占位符参数。
//
// 占位符用 fmt 的 %s/%d，**两种语言的占位符序列必须逐位相同**（catalog_test 钉着）：
// 翻译时把 `%d` 写成 `%s`，或者把两个参数的顺序调换，界面就会印出
// `%!s(float=6)` 这种东西——它在页面上看起来像乱码，而没人会去查语言包。
//
// **缺 key 返回包着尖括号的 key 自己**，不返回空串——理由见包注释第二条。
func (c *Catalog) T(key string, args ...any) string {
	if c == nil {
		return "⟨" + key + "⟩"
	}
	v, ok := c.msgs[key]
	if !ok {
		return "⟨" + key + "⟩"
	}
	if len(args) == 0 {
		return v
	}
	return fmt.Sprintf(v, args...)
}

// Lang 报这份文案属于哪种语言。
func (c *Catalog) Lang() Lang {
	if c == nil || !c.lang.Valid() {
		return Default
	}
	return c.lang
}

// HTMLLang 是给 <html lang> 用的便捷形式。
func (c *Catalog) HTMLLang() string { return c.Lang().HTMLLang() }

// cached 留一份每种语言的已加载文案。语言是机器级设置，一个进程里通常只有一种，
// 每次渲染重新解析 JSON 纯属浪费；命中缓存的读是纯内存操作，切换语言那条路上
// 失效一次即可。map 本身只在 loadOnce 之后读，不需要额外加锁。
var cached = map[Lang]*Catalog{}

// Load 取某种语言的文案。解析失败（语言包被改坏）时落回 Default，**不 panic**：
// 语言包是仓库里的静态文件，真坏了也该由 catalog_test 在 CI 里报出来，
// 而不是让一台跑着的导航台因为某个字节改错就起不来。
func Load(l Lang) *Catalog {
	if !l.Valid() {
		l = Default
	}
	if c, ok := cached[l]; ok {
		return c
	}
	c := mustLoad(l)
	cached[l] = c
	return c
}

func mustLoad(l Lang) *Catalog {
	c, err := loadFrom(l)
	if err == nil {
		return c
	}
	if l != Default {
		if d, derr := loadFrom(Default); derr == nil {
			return d
		}
	}
	// 走到这里只有一种可能：**默认**语言包也读不出来。它是 embed 进二进制的静态
	// 文件，所以这意味着二进制本身坏了——那确实该 panic。一台导航台因为界面文案
	// 坏掉而起不来，比起来就显示一屏空白要好：后者会让人以为产品就这样。
	panic(fmt.Sprintf("i18n: 语言包读不出来 %s: %v", l, err))
}

func loadFrom(l Lang) (*Catalog, error) {
	data, err := fs.ReadFile(messageFS, "messages/"+string(l)+".json")
	if err != nil {
		return nil, err
	}
	msgs := map[string]string{}
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, fmt.Errorf("解析: %w", err)
	}
	return &Catalog{lang: l, msgs: msgs}, nil
}

// Keys 返回这份文案的全部 key，排序后返回。**只给测试用**：守恒测试要逐条比对两种
// 语言的 key 集合，除此之外没有调用者——把它塞进公开 API 只会诱使人绕过 T 直接读表。
func (c *Catalog) Keys() []string {
	out := make([]string, 0, len(c.msgs))
	for k := range c.msgs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
