package icons

// Starter 是图标选择器**默认那一批**的名单：面板打开时先给人看这些，
// 而不是一张空网格。
//
// **为什么需要它**（2026-10-20）：选择器从前有两批来源——
//  ① 搜索（敲一个名字，回一屏命中）；
//  ② 起始格（这一份已经在用的那些图标，app.js 那一族）。
//
// ② 在实测里**几乎永远是空的**：新加的书签默认 icon=auto（去抓 favicon），
// 而 auto 与"没设"都被那一格排除掉。于是面板打开就是一张空网格 + 一句
// 「先搜个名字」——**正是 2026-10-16 那次想治的同一个症状**（那次的注释
// 自己写着"没回答点这颗按钮的人真正想问的'给我看几个'"）。换判据不如换一个
// **一定有货**的来源。
//
// 这份名单是**人工挑的常用图标**：品牌层取人们真的会存进书签的站点
// （中英文各占一半——这台机器上的读者是中文家庭），通用层取"文件夹 / 邮件 /
// 日历"这类与站点无关的用途。顺序即展示顺序，前面的更常用。
//
// 名单里的每个名字都由 TestStarter_名单里的每个名字都在索引里 钉住：上游
// 产物换一版、掉了哪一个，当场红，而不是静悄悄地少一格。
var Starter = []string{
	// 品牌层：人们真的会存进书签的那些站
	"simple-icons:github",
	"simple-icons:google",
	"simple-icons:youtube",
	"simple-icons:bilibili",
	"simple-icons:wechat",
	"simple-icons:qq",
	"simple-icons:taobao",
	"simple-icons:alipay",
	"simple-icons:zhihu",
	"simple-icons:xiaohongshu",
	"simple-icons:douban",
	"simple-icons:csdn",
	"simple-icons:juejin",
	"simple-icons:netflix",
	"simple-icons:spotify",
	"simple-icons:reddit",
	"simple-icons:telegram",
	"simple-icons:discord",
	"simple-icons:slack",
	"simple-icons:notion",
	"simple-icons:figma",
	"simple-icons:docker",
	"simple-icons:cloudflare",
	"simple-icons:wikipedia",
	"simple-icons:openai",
	"simple-icons:anthropic",
	"simple-icons:linux",
	"simple-icons:firefox",
	"simple-icons:microsoft",
	// 通用层：与站点无关的用途
	"tabler:folder",
	"tabler:file",
	"tabler:photo",
	"tabler:camera",
	"tabler:music",
	"tabler:video",
	"tabler:mail",
	"tabler:message",
	"tabler:calendar",
	"tabler:cloud",
	"tabler:book",
	"tabler:bookmark",
}

// StarterNames 给出这份名单在**当前索引**里真实存在的那些（保持名单的顺序）。
//
// 名单里若有名字在索引里不存在（上游产物换了一版），这里**跳过它**而不是
// 报整份错：面板是给人挑图标的，少一格不该让这一格整个开不出来。名单本身的
// 有效性由 TestStarter_名单里的每个名字都在索引里 那条测试钉着——它红了要改的
// 是名单，不是这里。
//
// nil 索引（装配点还没装上）返回空而不是 panic：这一格只是"没东西可挑"，
// 不该把整个进程带下去。
func (x *Index) StarterNames() []string {
	if x == nil {
		return nil
	}
	out := make([]string, 0, len(Starter))
	for _, key := range Starter {
		if _, ok := x.icons[key]; !ok {
			continue
		}
		out = append(out, key)
	}
	return out
}
