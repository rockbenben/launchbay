package render

import (
	"strings"
	"testing"
)

// 「挑一组」的那些名单里，**同名的组要分得开**（2026-10-22）。
//
// 台面面板「＋ 新分组」不查重名（id 由服务端派生，名字原样留下）、「改名」那一格也不查，
// 浏览器导出的书签里也有两个同名文件夹——所以磁盘上躺着两个叫「常用」的组是**正常形状**，
// 不是脏数据。
//
// 侧栏那一份导航分得清（它带着条目数：常用 1 / 常用 4），而这三处从前只印标题：
// 添加浮层的分组下拉、多选里的「移动到」、编辑模态里的「分组」。于是两个「常用」
// 长得一模一样——**人挑了哪一组全靠猜**，存错了要回头翻。
//
// 这一族钉的是三件事：
//   - 三处都走**同一个**判据（三处各写一遍就会各算一遍）；
//   - 那个判据是「**标题有歧义才**补条目数」（标题唯一时一个字都不多——那一档最常见，
//     不该被噪声打扰）；
//   - 条目数取的是**侧栏那一格里的同一个数**（countNodes），所以这几处与导航永远
//     说同一件事。
func Test挑一组的名单里同名的组要分得开(t *testing.T) {
	js := readWeb(t, "web/app.js")

	// 一：判据只有一处。
	if !strings.Contains(js, "function groupLabel(gid, name)") {
		t.Error("没有 groupLabel 那个判据：三处各写一遍就会各算一遍")
	}
	if !strings.Contains(js, "function countNodes(gid)") {
		t.Error("没有 countNodes 那一处：条目数的取法散在两处就会漂")
	}
	if !strings.Contains(js, "if (same < 2) return name;") {
		t.Error("groupLabel 不是「有歧义才补数」：标题唯一时也会多出一串字")
	}
	if !strings.Contains(js, "const n = countNodes(gid)[0];") {
		t.Error("groupLabel 读的不是导航那一格的数：与侧栏说的不是同一件事")
	}
	// 歧义也**从导航那几行判**（title 属性是那一组的整条路径）：那几行是服务端渲染的，
	// 模块初始化时就在 DOM 里。而 groupsOnPage() 是 const、声明在文件后面——添加浮层的
	// 下拉在初始化时就建好，引用它会撞 TDZ（整个模块抛 ReferenceError，下拉一条选项
	// 都没有）。这一条钉住那个教训，免得有人"顺手改成 groupsOnPage()"。
	if !strings.Contains(js, `document.querySelectorAll('.gnav a[title]')`) {
		t.Error("groupLabel 没有从导航那几行判歧义")
	}
	if strings.Contains(js, "groupsOnPage()) if (groupName(g) === name) same++") {
		t.Error("groupLabel 用上了 groupsOnPage()：那是 const，初始化时还在 TDZ 里，会让整个模块抛 ReferenceError（下拉里一条选项都没有）")
	}

	// 二：三处都走那一个判据（添加浮层的下拉 / 编辑模态的下拉 / 多选的「移动到」）。
	for _, at := range []string{
		"fGroup.append(el('option', { value: h2.dataset.group, text: groupLabel(h2.dataset.group, groupName(h2)) }))",
		"const o = el('option', { value: g.dataset.gid, text: groupLabel(g.dataset.gid, groupName(g)) });",
		"text: groupLabel(g.dataset.gid, groupName(g)),",
	} {
		if !strings.Contains(js, at) {
			t.Errorf("这一处没走 groupLabel：%s", at)
		}
	}

	// 三：改数（bumpCount）与读数（groupLabel）走**同一份** countNodes。
	if !strings.Contains(js, "for (const n of countNodes(gid)) bump(n);") {
		t.Error("bumpCount 没有走 countNodes：改的那一处与读的那一处会不是同一个数")
	}
}
