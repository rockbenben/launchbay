package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// 门禁的判据必须可机械执行：稀有 attr 名（计数 1 的那种，恰恰是最需要人看
// 的"上游新冒出来的属性"）必须与常见属性名一起**整份**出现在 attr 名全量表
// 里。name=value 的 top30 按次数降序，所有 d=<路径> 的计数都是 1，任何新属性
// 名一旦撞上截断就永远不可见——所以这条测试钉的是"全量、不截断"。
func TestVocabTables稀有属性名整份可见(t *testing.T) {
	// 8 个常见属性名各出现 6 次，再塞一个只有单个图标带的 fill-rule（计数 1）。
	// 若 attr 名表被截成 top5（按次数降序），fill-rule 必然掉出去。
	// 注意：这段是 JSON 串里的字符串值，属性引号必须写成 \"。
	common := `fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\" stroke-linejoin=\"round\" cx=\"12\" cy=\"12\" r=\"10\"`
	var b strings.Builder
	b.WriteString(`{"icons":{`)
	// 6 条常见图标，每条末尾带逗号；最后一条 rare 补上后整串闭合。
	for i := 0; i < 6; i++ {
		b.WriteString(`"icon` + string(rune('a'+i)) + `":{"body":"<path d=\"M3 12h18\" ` + common + `/>"},`)
	}
	b.WriteString(`"rare":{"body":"<path d=\"M3 12h18\" fill-rule=\"evenodd\"/>"},`)
	// class 只挂在容器 g 上（tabler 实测形态）：attr 名表的范围必须与 tag
	// 词表一致（全量元素），否则"只在容器上出现的新属性"又是不可见的。
	b.WriteString(`"gged":{"body":"<g class=\"ico\"><path d=\"M2 2h4\"/></g>"}`)
	b.WriteString(`}}`)

	var doc collectionDoc
	if err := json.Unmarshal([]byte(b.String()), &doc); err != nil {
		t.Fatalf("夹具解析失败: %v", err)
	}
	if len(doc.Icons) != 8 {
		t.Fatalf("夹具应有 8 条图标，得到 %d", len(doc.Icons))
	}

	tags, attrValues, attrNames, failed := vocabTables(doc)
	if len(failed) != 0 {
		t.Fatalf("词表统计不该有解析失败：%v", failed)
	}
	if tags["path"] != 8 {
		t.Errorf("tag 量尺应当数到 path:8，得到 %v", tags)
	}
	if attrValues[`fill=none`] != 6 {
		t.Errorf(`attr=值 表应当数到 fill=none:6，得到 %v`, attrValues)
	}
	if attrNames["fill-rule"] != 1 {
		t.Errorf("稀有属性名必须进 attr 名计数，得到 %v", attrNames)
	}
	if attrNames["class"] != 1 {
		t.Errorf("只挂在容器 g 上的稀有属性名也必须可见（attr 名表与 tag 词表同范围），得到 %v", attrNames)
	}

	table := attrNameFullTable(attrNames)
	if len(table) != len(attrNames) {
		t.Errorf("attr 名全量表被截断了：全量 %d 个名字只打出 %d 个（%s）",
			len(attrNames), len(table), strings.Join(table, " "))
	}
	joined := strings.Join(table, " ")
	if !strings.Contains(joined, "fill-rule:1") {
		t.Errorf("稀有属性名 fill-rule 没出现在 attr 名全量表里——门禁不可执行：%s", joined)
	}
	if !strings.Contains(joined, "class:1") {
		t.Errorf("容器上的稀有属性名 class 没出现在 attr 名全量表里——门禁不可执行：%s", joined)
	}
	for _, want := range []string{"d:8", "stroke-width:6", "fill:6", "cx:6"} {
		if !strings.Contains(joined, want) {
			t.Errorf("attr 名全量表缺常见项 %q：%s", want, joined)
		}
	}
}

// Minor ② 的行为证人：elementsOf 的职责是"容器拍平、非容器原样输出"。
// stack 若真的承重，砸掉承重逻辑（把 defs 当容器）这条必须红；
// 而删掉只压不读的 stack 后这条照常绿——正好证明那个 stack 是装饰。
func TestElementsOf拍平容器但defs与use原样保留(t *testing.T) {
	els, err := elementsOf(`<g><defs><circle cx="1" cy="2" r="3"/></defs><use href="#a"/><svg><rect x="0"/></svg></g><path d="M1 1h2"/>`)
	if err != nil {
		t.Fatalf("elementsOf 解析失败: %v", err)
	}
	var got []string
	for _, e := range els {
		got = append(got, e.Tag)
	}
	want := []string{"defs", "circle", "use", "rect", "path"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("拍平结果不符：%v，期望 %v（g/svg 是容器要拍掉，defs/use 不是容器必须原样留下整条判死）", got, want)
	}
}
