package render

import (
	"strings"
	"testing"
)

// 图标选择器那一格（2026-10-14）：内嵌图库在界面上此前一个出口都没有——
// 「＋ 添加」与就地编辑都只给三样选择（不选图标 / 自动按网址抓 / resolve 给的
// 至多 5 条候选），而库里那六千多个名字只能靠打接口用。
//
// 这几条钉的是**形状**，不是像素：每格必须是一枚能按的按钮、必须带着页面自己那份
// 内联 SVG、必须带可及名。少任何一条的症状都不响——功能测试全绿，而人在屏上看见的
// 是「选不了图标」或一排读屏念不出名字的方块。

func TestIconTiles_每格是一枚能按的按钮并带真SVG(t *testing.T) {
	ix, _, _ := assets(t)
	markup, total := IconTiles(ix, "docker", 24)
	got := string(markup)

	if total < 1 {
		t.Fatalf("simple-icons:docker 必须命中，total=%d", total)
	}
	if !strings.Contains(got, `data-icon="simple-icons:docker"`) {
		t.Errorf("方格里没有图标名：界面上那一格点了不知道要把什么写回去\n%.300s", got)
	}
	// **type=button 不能省**：选择器整个建在添加浮层那个 <form> 里，一个没写 type
	// 的按钮点下去会提交那个表单（存了一条没选完的书签出来）。
	if !strings.Contains(got, `<button type="button"`) {
		t.Error("方格不是 type=button：它住在添加浮层的表单里，点一下会提交那个表单")
	}
	// SVG 必须是**内联**的（页面画卡片图标的那一份），不是 <img src>：图标是描边 +
	// currentColor 的，进 <img> 之后拿不到页面的字色，暗色页上等于没画。
	if !strings.Contains(got, `<svg class="ic-svg`) {
		t.Errorf("方格里没有内联 SVG（应由 svgMarkup 画，与卡片上那枚同源）\n%.300s", got)
	}
	// 可及名：里面的 SVG 是 aria-hidden 的，没有 aria-label 这一格对读屏就是无名按钮。
	if !strings.Contains(got, `aria-label="simple-icons:docker"`) {
		t.Error("方格没有 aria-label：里面的 SVG 是 aria-hidden 的，这一格对读屏没有名字")
	}
	// limit 是真的在限：搜一个宽词要拿到的不超过 limit 格。
	if _, wideTotal := IconTiles(ix, "a", 5); wideTotal < 5 {
		t.Errorf("夹具不成立：q=a 至少要能命中 5 条，实际 %d", wideTotal)
	}
	narrow, _ := IconTiles(ix, "a", 5)
	if n := strings.Count(string(narrow), "<button"); n != 5 {
		t.Errorf("limit=5 却画了 %d 格", n)
	}
}

func TestIconTiles_零命中是空串不是404也不是一格占位(t *testing.T) {
	ix, _, _ := assets(t)
	markup, total := IconTiles(ix, "zzzzz-zzz-nope", 24)
	if total != 0 {
		t.Errorf("零命中的 total 要是 0，得到 %d", total)
	}
	if markup != "" {
		t.Errorf("零命中要回空串（界面自己说「没有叫这个名字的」），得到 %.200s", markup)
	}
}

// 界面上那一格的几条机械判据。每一条都对应一个「功能测试不红、人看得见」的后果。
func TestAppJS_图标库那一格两处都进得去(t *testing.T) {
	src := readAppJS(t)
	for name, want := range map[string]string{
		// 两个浮层各有一颗入口键：能改内容的那一层与能选图标的那一层都要有——
		// 就地编辑此前连图标那一格都没有（app.js 那条注释记着这段历史）。
		"添加浮层的入口": `iconLibraryButton(fIcon)`,
		"就地编辑的入口": `iconLibraryButton(ic)`,
		// **那颗键不进 <label>**：label 包着的控件是那个 <select>，点 label 里的任何
		// 一点都会连带激活它——于是点「图标库…」会顺带把下拉弹开，两件事一起发生。
		"入口不在 label 里": `el('div', { class: 'icrow' }, [`,
		// 图库要向服务端要**画好的**那一格：搬进 app.js 等于让只看首屏的每一页都背
		// 一份六千条的名字表（§7.3 那条操作线管的就是这件事）。
		"向服务端要方格":        `/api/v1/icons/tiles?q=`,
		"取回命中总数（截断要说出来）": `res.headers.get('X-LB-Icons-Total')`,
		"插入走 DOMParser":  `new DOMParser().parseFromString(markup, 'text/html')`,
		// 图库给的名字多半不在 resolve 的候选里（候选至多 5 条），所以要自己补一颗
		// option——少了它，点完方格下拉框**值没有变**，看着像「点了没反应」。
		"选完补一颗 option": `sel.append(el('option', { value: name, text: iconLabel(name) }))`,
		// 图库那一层要排在通用 `.sheet` 分支**之前**：那一支是 `dlg.remove()`，
		// 摘掉 DOM 却不同步 syncTrap，整页会留在 inert 里（面板没了，一个字都点不动）。
		//
		// 2026-10-14：这行从 Esc 的内联分支搬进了 `closeTopLayer()`——Esc 与「点外面」
		// 现在共用那一份（点外面那一支是后加的，两处各写一份必然漂）。判据跟着搬家：
		// **仍然要在这个函数里、仍然排在 dlg 那一支之前、仍然带 syncTrap 的收尾**。
		"图标库排在通用 sheet 之前": "if (iconPick.panel) { closeIconPicker(); return true; }",
		// 它比通用分支靠前这件事本身要钉住：顺序反了的那次症状是"按 Esc 摘掉了
		// 添加面板而不是图标库"，而那一次一次按键毁掉了两个状态。
		"closeTopLayer 里有这一层": "function closeTopLayer()",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("app.js 少了 %q（%s）", want, name)
		}
	}
	if strings.Contains(src, "grid.innerHTML =") {
		t.Error("图标库那一格用 innerHTML 插服务端送来的文本：跨了一次 fetch 的东西要 parse 出来再 replaceChildren")
	}
}

// 方格那一族的几条形状：44px 的触摸地板不许缩、面板比添加层窄一档、方格里的图标
// 要比卡片上那一枚大（它是**用来认的**）。
func TestCSS_图标库方格的形状(t *testing.T) {
	raw, err := webFS.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	for _, want := range []string{
		".icrow{display:flex;align-items:flex-end;gap:8px}",
		// **width:auto 是承重的**：`.fld` 的 `width:100%` 是给"整幅铺开"写的，而这两行
		// 里的键都要和别的控件并排。带着 100% 时每颗的 flex 基准宽度就是整行，于是它们
		// 把旁边那一格压成 0px——实测出来的那样：添加那一行 label 0 宽、下拉只剩
		// 22px；就地编辑那行 104 高、每颗 514 宽。
		".icrow>button{flex:0 0 auto;width:auto}",
		".icpick{width:min(460px,calc(100vw - 28px))}",
		".icpick-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(56px,1fr))",
		".icpick-tile{min-height:56px;padding:0}",
		".icpick-tile .ic-svg,.icpick-tile .ic-img{width:28px;height:28px}",
		// 就地编辑那一行从一颗变两颗，得自己排一行（此前它没有任何规则，<p> 里一颗）
		".edit-icon-row{display:flex;gap:8px;flex-wrap:wrap}",
		".edit-icon-row>button{width:auto}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css 里少了 %q", want)
		}
	}
	// **不许缩到 44 以下**：§7.5 的触摸地板，窄屏正是最需要它的时候。
	for _, banned := range []string{".icpick-tile{min-height:36px", ".icpick-tile{min-height:32px", "minmax(36px", "minmax(32px"} {
		if strings.Contains(css, banned) {
			t.Errorf("app.css 里出现 %q：44px 是 §7.5 的触摸地板，谁都不许缩", banned)
		}
	}
}
