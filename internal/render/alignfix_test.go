package render

import (
	"strings"
	"testing"
)

// 两处 2026-10-21 实拍出来的对齐（用户实拍图：台面面板「我的 PIN」那一行的键浮在
// 输入框上面；档案页那枚 GitHub 猫头孤零零挂在栏里）。
//
// 这一族是**源码级**的钉：两条都是「某一行的上下边必须重合」这类判据，而 Go 里
// 量不到多栏排布与 flex 对齐——真正量像素的那一道在 tools/uicheck（浏览器横扫），
// 这里钉的是**承载它的那条规则不许被改回去**。两者是同一件事的两半：这一份钉住
// 「为什么」，那一道钉住「现在是不是真的平」。
//
// **为什么值得钉**：这两处的症状都是「屏上看着没对齐」，而**功能测试全绿**——
// 键照样能按，图标照样能点。上一轮就是靠人眼实拍逮到的（用户报「按钮没平行」）。

// 「我的 PIN」那一行：键与输入框**平齐**。
//
// 形状：`.prefs-ctl` 是那一行的容器（页面名字那一行是「输入框 + 键」两个平级盒子，
// 「我的 PIN」那一行里键是**输入框那一列的兄弟**——那一列 = 标签 + 缝 + 输入框）。
// 居中对齐时，键对到的是**整列**的中线，于是它浮在输入框上方 15px
// （实测 2026-10-21，1440：键 145..189、输入框 160..204）。
//
// 钉的是**底边对齐**这一个词：两个盒子都是 44px，底边对齐等于上下边完全重合。
// 写成 `align-items:center` 就红——那不是"另一种也行的写法"，它是**这个病**。
func Test台面面板的键与输入框平齐(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	row := extractRule(css, ".prefs .prefs-ctl{display:flex")
	if row == "" {
		t.Fatal("base.css 里找不到 .prefs .prefs-ctl")
	}
	if !strings.Contains(row, "align-items:flex-end") {
		t.Errorf(".prefs-ctl 不是 align-items:flex-end：\n%s\n"+
			"「我的 PIN」那一行里键是输入框那一列的兄弟，居中对齐会让它浮在框的上沿"+
			"（实测差 15px），底边对齐才与输入框上下边重合", row)
	}
	if strings.Contains(row, "align-items:center") {
		t.Errorf(".prefs-ctl 又写回 align-items:center 了：\n%s", row)
	}
}

// 档案页的页脚在**两档多栏/网格**里必须自己横跨整幅。
//
// 形状：全局（wall）那一档 `main` 是 CSS 多栏，密集（dense）那一档 `main` 是 grid。
// 页脚是 `main` 的普通流子元素，于是它被当成「某一栏里的一个盒子」——
// 实测 2026-10-21：全局那一档它**碎成两片**（栏 2 底部 14px + 栏 3 顶部 30px），
// 那时页脚里放的是那枚 GitHub 猫头，它落在栏 3 的顶上、与其他分组的标题齐平，
// 读起来像「这一组里多了一条没有名字的书签」；用户实拍里它孤零零挂在最后一栏
// 的下面。（2026-10-21 那枚猫头搬去设置面板了，页脚现在只剩用户自己那段文字——
// **同一栏的毛病，同一个位置**，所以那两条规则照旧要写。）
//
// **与大卡带子同一族**（featview_test.go 那一条钉的是带子）：那两条
// `[data-profile-view=wall] .feat{column-span:all}` /
// `[data-profile-view=dense] .feat{grid-column:1/-1}` 就是同一件事的另一个实例。
// 页脚漏了，于是同一个毛病长在另一个地方。
func Test页脚在两档多列视图里横跨全部列(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/base.css"))
	// 按**规则**取，不逐字绑整行：`column-span` 那一条还带一个 `-webkit-` 前缀
	// （旧 Safari 才认），绑死了前缀就等于把兼容写法也一起钉住。
	for _, want := range []struct{ what, sel, needle string }{
		{"全局（多列）", "[data-profile-view=wall] footer{", "column-span:all"},
		{"密集（grid）", "[data-profile-view=dense] footer{", "grid-column:1/-1"},
	} {
		rule := extractRule(css, want.sel)
		if rule == "" {
			t.Errorf("base.css 里没有 %q 那一条规则", want.sel)
			continue
		}
		if !strings.Contains(rule, want.needle) {
			t.Errorf("base.css 里 %s 那一档的页脚没有 %q：\n%s\n"+
				"页脚会被多栏/栅格关进某一栏里，那枚 GitHub 猫头于是读作栏里的一条无名字条目",
				want.what, want.needle, rule)
		}
	}
}
