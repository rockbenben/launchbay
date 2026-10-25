package render

import (
	"strings"
	"testing"
)

// 全站巡查（2026-10-15）挖出来的两处，都不是"看一眼就知道"的形状，
// 所以各钉一条。

// 一、添加浮层的分组下拉里，选项是「**常用7**」——条目数粘在名字后面。
//
// 根因：大卡那一档的组标题后面挂着一枚条目数徽标（`.feat-n`），而造选项时读的是
// `h2.textContent`，那一段把徽标一起读进来了。症状只出现在**设成大卡的组**上
// （正文组的 h2 没有徽标），所以它看起来像"第一个选项坏了"。
//
// 判据是**选项文字里不许有那一枚徽标的数字**：读 `groupName`（同一族几处已经在用
// 的那份）而不是 `textContent`。
//
// **2026-10-22 改判**：这一族原来叫「选项**不带条目数**」，而它当年的理由是
// **漏出来**的那一枚（徽标粘在名字后面，读的是 textContent）。后来情况变了：
// 重名分组是**合法形状**（台面面板「＋ 新分组」不查重名、「改名」也不查，浏览器
// 导出的书签里也有两个同名文件夹），而侧栏那一份导航从 2026-10-22 起**每一组都
// 有一行**（它带着条目数：常用 1 / 常用 4）。
// 于是「选项只印标题」就从「省事」变成了「**两个同名组长得一模一样**」——那正是
// 这条判据当年想避免的那件事，只是换了个形状。
//
// 现在的判据分成两半：
//   - **不许漏**：选项文字仍然只从 groupName 那一份来（不是 textContent），所以
//     「常用7」那种**粘上去**的数字不会再来；
//   - **要分得开**：只有**同名**的那几组才补上条目数，而且那个数是**这一组自己
//     的**（countNodes 那一处，与侧栏同数），标题唯一时一个字都不多。
func Test添加浮层的分组选项不漏徽标且同名才带条目数(t *testing.T) {
	js := stripJSComments(readAppJS(t))
	if !strings.Contains(js, "text: groupLabel(h2.dataset.group, groupName(h2))") {
		t.Errorf("分组下拉的选项不是从 groupName 那一份来的：大卡那一组会显示成「常用7」\n%s", firstLines(js, "for (const h2 of document.querySelectorAll('main h2[data-group]'))", 3))
	}
	// 补上去的那个数是**这一组自己的**（侧栏那一格里的同一个数），不是徽标里
	// 粘过来的那一个。
	if !strings.Contains(js, "const n = countNodes(gid)[0];") {
		t.Error("补上去的条目数不是侧栏那一格的数：两处会不是同一件事")
	}
	// 反向证人：那一枚徽标确实在标题里，所以这不是"模板里没有它"的另一种解释。
	tmpl := readWeb(t, "web/profile.tmpl")
	if !strings.Contains(tmpl, `class="feat-g"`) || !strings.Contains(tmpl, `class="feat-n"`) {
		t.Error("模板里找不到带徽标的大卡组标题：这条判据的前提变了")
	}
}

// 二、`.layoutbtn` 的命中区上边**被 section 的 paint containment 裁掉**。
//
// 它住在分组的 `<h2>` 里，而 h2 紧贴 section 的上沿；base.css 那条
// `section{content-visibility:auto}` 等价于 layout/style/paint 三重 containment，
// 把溢出裁到 section 的盒子。实测（1440 逐像素打点）：左/右/下 3px 外都还命中，
// **上边 1–14px 一律落到 main**。全局与密集两档是 `content-visibility:visible`，
// 所以那两档一直是好的——差的是默认那一档。
//
// 这条钉的是"扩展要留余量"（-9/-7 而不是 -8/-6）：逐像素量出来的边缘会差一两像素，
// 那取决于布局里的小数取整，不留余量的话"够不够 44"就看运气。
//
// **它没有钉 44**——盒子只有 32×28、命中区实测 45×34，而补满 44 要动版式
// （见 app.css 那段注释里的两条路与代价）。这里钉的是**已知形状 + 不许悄悄变小**。
func Test布局键的命中区留了余量(t *testing.T) {
	css := stripCSSComments(readWeb(t, "web/app.css"))
	rule := extractRule(css, ".layoutbtn::before{")
	if rule == "" {
		t.Fatal("app.css 里找不到 .layoutbtn::before")
	}
	if !strings.Contains(rule, "inset:-9px -7px") {
		t.Errorf("布局键的扩展圈不是 -9/-7：实测边缘会差一两像素（布局取整）\n%s", rule)
	}
	// 盒子不许比 28 更矮：那一版实测 45×42，仍差 2px，而代价是每个分组标题行高 8px
	// ——那是版式决定，要改得明着改，不许在"补命中区"的顺手改动里发生。
	if !strings.Contains(css, ".layoutbtn{position:relative;flex:0 0 auto;width:32px;height:28px;") {
		t.Error("布局键的盒子不是 32×28：补命中区要改标题行的内衬，不是这颗键的盒子")
	}
	// 那条 containment 是前提：它没了，上面那一整族判断都要重写。
	base := stripCSSComments(readWeb(t, "web/base.css"))
	if !strings.Contains(base, "section{content-visibility:auto") {
		t.Error("base.css 里那条 section{content-visibility:auto} 没了：布局键命中区被裁的那一族要重新判")
	}
	for _, v := range []string{"wall", "dense"} {
		if !strings.Contains(base, "[data-profile-view="+v+"] section{") {
			t.Errorf("[data-profile-view=%s] section 那条不见了：两档靠它把 containment 关掉", v)
		}
	}
}
