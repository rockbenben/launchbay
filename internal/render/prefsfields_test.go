package render

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 台面面板里 **POST /prefs 那一族**的字段名（2026-10-19 改判）。
//
// **判据只有一份：那张表单自己。** app.js 从前抄了一份名单
// （theme/view/layout/title），于是两格在浮层里按了等于没按：
//   - 第二行（addrLevel，2026-10-19 加的）没进那份名单；
//   - 「改名」那一行的保存键**没有 name**（它说的是「保存」，不是"改成哪一档"），
//     于是名单里那个 title 永远匹配不上。
//
// 症状是**按了没反应**：preventDefault 已经把原生提交取消了，而 payload 是空的，
// 函数直接 return，一个请求都不发。零 JS 的独立页走原生提交，所以那两格在那里
// 又是好的——同一个设置，两条路两种结果，而坏的那条正是人最常用的那条。
//
// 这条钉的是**形状**：每张 POST /prefs 表单里**恰好一个**名字。app.js 的判据
// （"那张表单里唯一那个名字就是它要改的那一格"）只在形状成立时才有答案。
var (
	prefsFormRE = regexp.MustCompile(`(?s)<form[^>]*action="/prefs"[^>]*>(.*?)</form>`)
	elemNameRE  = regexp.MustCompile(`\bname="([^"]*)"`)
)

func Test面板里每一张prefs表单恰好一个字段名(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "爸爸的书签", "yt")
	html := string(mustPrefs(t, s, "dad"))

	forms := prefsFormRE.FindAllStringSubmatch(html, -1)
	// 五张：改名/排法/第二行/档案形态/主题。少一张就是有一格在面板上画不出来。
	if len(forms) != 5 {
		t.Fatalf("面板里有 %d 张 POST /prefs 的表单，要 5 张（改名/排法/第二行/档案形态/主题）", len(forms))
	}
	for i, f := range forms {
		set := map[string]bool{}
		for _, n := range elemNameRE.FindAllStringSubmatch(f[1], -1) {
			if n[1] != "" {
				set[n[1]] = true
			}
		}
		if len(set) != 1 {
			names := make([]string, 0, len(set))
			for k := range set {
				names = append(names, k)
			}
			sort.Strings(names)
			t.Errorf("第 %d 张 POST /prefs 表单里有 %d 个名字（%v）：app.js 判不出它要改的那一格。"+
				"形状不唯一时它宁可不发（那样是「按了没反应」），也不猜错一格改掉另一格",
				i+1, len(set), strings.Join(names, ", "))
		}
	}
}

// app.js 那一族**不再抄字段名单**（2026-10-19 改判）。
//
// 抄一份就会漏：漏掉的那一格在浮层里按了等于没按（payload 空 → 一个请求都不发）。
// 现在字段名从表单自己读，所以**加一格不必再动 app.js**——那份名单本身就是这个
// bug 的形状。
//
// 判据钉的是**两件事**：从表单读（正面），以及**不再抄**（反向证人）。只钉正面
// 的话，把名单加回去也能绿。
func TestAppJS_台面那一族从表单自己读字段名(t *testing.T) {
	js := stripJSComments(readWeb(t, "web/app.js"))
	i := strings.Index(js, `form[action="/prefs"]`)
	if i < 0 {
		t.Fatal("app.js 里找不到 /prefs 那张表单的委托")
	}
	seg := js[i:]
	// 截到下一族（背景）为止：这一段只管 /prefs 那一族。
	if j := strings.Index(seg, `form[action="/prefs/`); j > 0 {
		seg = seg[:j]
	}
	if !strings.Contains(seg, "querySelectorAll('[name]')") {
		t.Error("那一族不从表单自己读字段名：抄一份名单就会漏一次，而漏掉的那一格在浮层里按了没反应")
	}
	// 反向证人：面板那几格的名字**一个都不该再出现在这一段里**。
	// `theme` 是例外——它是就地换 data-theme 的那一支，本来就该认它。
	for _, name := range []string{"'view'", "'layout'", "'addrLevel'", "'title'"} {
		if strings.Contains(seg, name) {
			t.Errorf("那一族里又出现了 %s：字段名抄成一份就会漏一次"+
				"（实测：漏掉的那一格在浮层里按下去什么都不会发生）", name)
		}
	}
}
