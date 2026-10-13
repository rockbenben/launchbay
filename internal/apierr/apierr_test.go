package apierr

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
)

// kind 的写法本身是契约的一部分：脚本与界面都按这个字面串匹配。
var kebab = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// 常量清单**从源码扫出来**，不是手抄一份。手抄的那份会自己变成第二个真相：
// 加一个常量忘了加进列表，测试照样绿，而登记表与文档就漏了那一格。
func TestKind表每条都登记且形状对(t *testing.T) {
	src, err := os.ReadFile("apierr.go")
	if err != nil {
		t.Fatal(err)
	}
	all := constKinds(string(src))
	if len(all) < 20 {
		t.Fatalf("只扫到 %d 个 kind 常量——扫描器失灵与「真的只剩这么点」看起来一模一样", len(all))
	}
	seen := map[string]bool{}
	for _, k := range all {
		if !kebab.MatchString(k) {
			t.Errorf("kind %q 不是 kebab-case", k)
		}
		if seen[k] {
			t.Errorf("常量表里 %q 出现了两次（两个常量指同一个码，文档就说不清了）", k)
		}
		seen[k] = true
		info, ok := Registry[k]
		if !ok {
			t.Errorf("%q 没登记：文档与一致性闸看不见它，而调用方迟早会收到它", k)
			continue
		}
		if info.Status < 400 || info.Status > 599 {
			t.Errorf("%q 的默认状态码 %d 不是错误码", k, info.Status)
		}
		if strings.TrimSpace(info.WayOut) == "" {
			t.Errorf("%q 没有出路可说：这一类错误到了界面上就只能复述服务端原话", k)
		}
	}
	for k := range Registry {
		if !seen[k] {
			t.Errorf("登记表里有 %q 但常量表没有：要么忘了导出，要么该删", k)
		}
	}
}

var kindConstRE = regexp.MustCompile(`(?m)^	([A-Z][A-Za-z]+)\s*= "([a-z0-9][a-z0-9-]*)"(?:\s*//.*)?$`)

// constKinds 扫出 apierr.go 里所有 kind 常量（名字与字面值都要）。
// 只认"制表符开头、= 一个带连字符的小写串"这一种形状，别的常量（比如以后
// 加个非 kind 的字符串常量）不会被误当成分类码。
func constKinds(src string) []string {
	var out []string
	for _, m := range kindConstRE.FindAllStringSubmatch(src, -1) {
		out = append(out, m[2])
	}
	return out
}

// KindsByName 给文档闸用：常量名 → 字面码（openapi 与 AGENTS.md 里写的是字面码）。
func KindsByName(src string) map[string]string {
	out := map[string]string{}
	for _, m := range kindConstRE.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// With/Kind 必须穿过包装链：ops 那边错误会被 fieldErr 再包一层，
// 分类如果只认"最外面那层"就会在收口处全部丢掉。
func Test分类穿得过包装链(t *testing.T) {
	base := errors.New("条目 id \"x\" 在本档案里已用过（跨分组也不行）")
	mine := With(base, DuplicateID)
	outer := &wrap{mine}
	if got := Kind(outer); got != DuplicateKindWant {
		t.Errorf("穿过一层包装后分类丢了：拿到 %q，要 %q", got, DuplicateKindWant)
	}
	if Kind(base) != "" {
		t.Error("没贴过分类的错误不该有分类（兜底该由调用方显式给）")
	}
	if Kind(nil) != "" {
		t.Error("nil 不该有分类")
	}
	if With(nil, DuplicateID) != nil {
		t.Error("With(nil) 不该造出一个非 nil 的错误：那会让调用方以为失败了")
	}
}

const DuplicateKindWant = DuplicateID

type wrap struct{ err error }

func (w *wrap) Error() string { return w.err.Error() }
func (w *wrap) Unwrap() error { return w.err }

// 原话一个字不许因为贴了码而变：detail/reason 仍是脚本与日志在读的那份，
// kind 是**多出来**的一格，不是替换。
func Test贴码不改原话(t *testing.T) {
	base := errors.New("url 缺主机名 \"javascript:void(0)\"")
	got := With(base, URLIncomplete)
	if got.Error() != base.Error() {
		t.Errorf("包装改了文本：%q != %q", got.Error(), base.Error())
	}
	if !errors.Is(got, base) {
		t.Error("包装必须仍可 errors.Is 到原错误：sentinel 判据（ErrValidation 那一族）不能断")
	}
}
