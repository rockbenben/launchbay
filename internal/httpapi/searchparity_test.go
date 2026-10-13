package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readWebFile 读一份源码/样式文件当形状用——这条闸比的是"两边都认这五个字段"，
// 而那两边一份在 Go 里、一份在浏览器里，只有读原文才比得了。
func readWebFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ⌘K 与整页搜索读同一组字段（batch9 U8）。
//
// 起点是实测：同一份数据、同一个词，两条通路给两个答案。面板从前只比名字与地址
// （`name.includes(q) || url.includes(q)`），而服务端 filter.go 的 matchItem 比
// 名字/地址/说明/每一个别名/每一个标签，分组标题命中还把整组带出来。11 个词里
// 5 个不一致，其中 4 个面板回的是「没有叫「X」的书签」——一句话把"我只比这两个
// 字段"说成了"这台机器上没有这个东西"。
//
// 这一条闸不重跑那张表（真表在浏览器里量，见 design-preview batch9 的活格子④），
// 它钉的是**分家的形状**：模板带出那几个字段、客户端去读它们、而服务端那一份
// 判据若加了第六个字段，这里会当场点名——漏配的那一边从来不会有任何功能测试变红。
func TestSearch_面板与整页读同一组字段(t *testing.T) {
	_, h := setupBatch(t)
	// 夹具那份 dad 的条目本来 desc/alias/tags 全空——先经**唯一的写通路**给它补上，
	// 才有东西可验。tags 里放一个带空格的值，是为了让"用空格连接多值"那种写法
	// 当场暴露：那一格会被拆成两个标签，还会与邻格拼出一个根本不存在的标签。
	if rec := post(h, "/api/v1/batch", `{"profile":"dad","by":"ai","expectedRev":1,"ops":[
	  {"op":"set","path":"groups/nas/items/dsm/desc","value":"存储总管"},
	  {"op":"set","path":"groups/nas/items/dsm/alias","value":["ds","群晖 后台"]},
	  {"op":"set","path":"groups/nas/items/dsm/tags","value":["NAS","存储 备份"]}]}`, "", dadTok); rec.Code != 200 {
		t.Fatalf("补 desc/alias/tags 要 200，得到 %d: %s", rec.Code, rec.Body)
	}
	page := get(h, "/", dadFormCookie).Body.String()
	// 夹具里 dad 的条目带 desc / alias / tags（seed 那边给的），必须真的到了 DOM 上。
	for _, want := range []string{`data-desc="`, `data-alias="`, `data-tags="`} {
		if !strings.Contains(page, want) {
			t.Errorf("档案页的 li[data-item] 上没有 %s：⌘K 就读不到这一维，判据会重新分家", want)
		}
	}
	// 多值用换行连接，不许用空格——标签与别名自己可能带空格。
	if regexp.MustCompile(`data-tags="[^"]*&#10;`).FindString(page) == "" {
		t.Error("data-tags 不是以换行连接的：空格会把一个标签拆成两个，还会跨两个标签误配")
	}

	appJS := readWebFile(t, "..", "render", "web", "app.js")
	for _, want := range []string{
		"li.dataset.desc", "li.dataset.alias", "li.dataset.tags",
		// 分组标题命中带出整组，靠的是节标题上那颗 .sec-leaf。
		`querySelector('.sec-leaf')`,
	} {
		if !strings.Contains(appJS, want) {
			t.Errorf("app.js 里没有 %q：面板那一屏又退回只比名字与地址了", want)
		}
	}
	if strings.Contains(appJS, "if (!q || name.includes(q) || url.includes(q)) {") {
		t.Error("app.js 里还留着那份只比名字与地址的判据")
	}

	// 服务端那一份判据是**唯一的一份**，它读几个字段这条闸要知道。数的是 matchItem
	// 函数体里的字段访问，不是在整份文件里找词——注释里提到 Desc 不算数。
	filterGo := readWebFile(t, "..", "render", "filter.go")
	body := filterGo[strings.Index(filterGo, "func matchItem"):]
	if i := strings.Index(body, "\nfunc "); i > 0 {
		body = body[:i]
	}
	for _, field := range []string{"it.Name", "it.URL", "it.Desc", "it.Alias", "it.Tags"} {
		if !strings.Contains(body, field) {
			t.Errorf("matchItem 不再读 %s：把这一格从面板的判据里也拿掉，或者把两边一起补上——两边不一致就是本轮报的那件事", field)
		}
	}
	// 反向也要咬：服务端加了第六个字段而面板没读，就是重新分家。
	extra := regexp.MustCompile(`it\.[A-Z]\w+`).FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	for _, m := range extra {
		seen[m[0]] = true
	}
	if len(seen) > 5 {
		t.Errorf("matchItem 读了 %d 个字段（%v），而 app.js 只认五个：新增的那一维必须同时进 data-* 与面板", len(seen), keysOf(seen))
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
