package icons

import (
	"encoding/json"
	"testing"
)

// fixtureDoc 拼一份最小合法产物。icons 的键直接就是产物里的键，
// 值可以是裸 d 字符串或元素数组——两种写法每条测试都要走一遍。
//
// 这里的 map[string]any 是**测试输入**（拿去喂 Decode 的样本），不是磁盘/响应
// 的输出形状：AGENTS.md §4.3 那条"输出键序由 struct 字段声明顺序固定、绝不靠
// map 遍历"管的是写出去的字节（书签走 internal/store/encode.go 的 WriteJSON，
// 图标产物走 tools/genicons/build.go 的 productDoc + marshalProduct）。输入夹具
// 的键序没人消费，map 在此合法。别把这段当形状样板抄进产品代码——要看形状样板
// 请照 doc.go 的产物形状与 productDoc。
func fixtureDoc(t *testing.T, prefix, layer string, count int, icons map[string]any, aliases map[string]string) []byte {
	t.Helper()
	doc := map[string]any{
		"prefix":        prefix,
		"layer":         layer,
		"viewBox":       "0 0 24 24",
		"count":         count,
		"upstreamTotal": count,
		"upstream":      "fixture@0.0.0",
		"generatedAt":   "2026-10-06T00:00:00Z",
		"license":       "MIT",
		"icons":         icons,
		"aliases":       aliases,
	}
	if aliases == nil {
		doc["aliases"] = map[string]string{}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("夹具拼不出来: %v", err)
	}
	return raw
}

func mustDecode(t *testing.T, raw []byte) *Index {
	t.Helper()
	ix, err := Decode(raw)
	if err != nil {
		t.Fatalf("合法夹具不该报错: %v", err)
	}
	return ix
}
