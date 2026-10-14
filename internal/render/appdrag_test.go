package render

import (
	"strings"
	"testing"
)

// 拖出去的负载：这一条钉的是**只有拖到书签栏才看得见**的那件事。
//
// 从前卡片 dragstart 把内部条目 id 塞进 `text/plain`（那是排序通道用的字段），
// 于是把一张卡片拖到书签栏，落下来的是一个叫「group-2-chrome-apps」的坏书签。
// 而这条通路在 HTML 里没有任何痕迹：渲染测试、DOM 测试、点击测试全都照过，
// 只有真拖一次才发现——所以这里用源码逐字行钉住，与 `TestCSS_硬规格在场` 同一族。
//
// 判据的形状是「排序走自定义 MIME，text/plain 与 text/uri-list 留给真地址」。
// 自定义 MIME 必须**成对**出现（写侧与读侧各一次）：只改一侧的话排序当场失效，
// 而这条闸要是只数 ≥1 就看不见那个断裂。
func TestAppJS_拖出去的是地址不是内部id(t *testing.T) {
	src := readAppJS(t)

	const mime = "application/x-lb-item"
	wants := []string{
		// 排序通道：写侧与读侧。**读侧现在在 srcOf 里**（2026-10-15）：落点要算
		// 源组，得先把"这次拖的是哪一条"对出来，而那条判据（只认自己那套 MIME）
		// 就住在读的那一处。
		"dt.setData('application/x-lb-item', li.dataset.item);",
		"const id = e.dataTransfer.getData('application/x-lb-item');",
		// 真地址：卡片从自己的 href 取，点不动那行从 <code> 取
		"const href = card.getAttribute('href') || '';",
		"dt.setData('text/uri-list', href);",
		"dt.setData('text/plain', href);",
		"dt.setData('text/uri-list', url);",
		"dt.setData('text/plain', url);",
	}
	for _, w := range wants {
		if !strings.Contains(src, w) {
			t.Errorf("app.js 里少了这一句：%s", w)
		}
	}

	// 反向：内部 id 不许再出现在 text/plain / text/uri-list 里。
	// 这是那条 bug 的原句，留着它等于给"顺手简化回去"开门。
	for _, bad := range []string{
		"setData('text/plain', li.dataset.item)",
		"setData('text/uri-list', li.dataset.item)",
	} {
		if strings.Contains(src, bad) {
			t.Errorf("app.js 里出现了不该有的负载写法：%s", bad)
		}
	}

	// 守卫解析器没吞形状：MIME 恰好两处（写 + 读），少一处就是排序通道断了。
	if n := strings.Count(src, mime); n != 2 {
		t.Errorf("%s 出现 %d 次，要的是 2 次（dragstart 写 + drop 读）；只改一侧的话页内排序当场失效", mime, n)
	}
}
