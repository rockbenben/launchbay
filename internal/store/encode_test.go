package store

import (
	"bytes"
	"strings"
	"testing"
)

type payload struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func encode(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, v); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	return buf.String()
}

func TestKeepsChineseAndHTMLEntitiesLiteral(t *testing.T) {
	got := encode(t, payload{Name: "番茄 & <b>加蛋</b> 面", Items: []string{"a"}})
	if strings.Contains(got, `\u`) {
		t.Fatalf("出现 \\u 转义:\n%s", got)
	}
	for _, want := range []string{"&", "<b>加蛋</b>", "面"} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺字面量 %q:\n%s", want, got)
		}
	}
}

func TestTwoSpaceIndentOneTrailingNewline(t *testing.T) {
	want := "{\n  \"name\": \"x\",\n  \"items\": []\n}\n"
	if got := encode(t, payload{Name: "x", Items: []string{}}); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestEmptySliceIsNotNull(t *testing.T) {
	if got := encode(t, payload{Name: "x", Items: []string{}}); strings.Contains(got, "null") {
		t.Fatalf("空数组写成 null:\n%s", got)
	}
}

func TestByteStableAcrossCalls(t *testing.T) {
	p := payload{Name: "稳定", Items: []string{"a", "b"}}
	if a, b := encode(t, p), encode(t, p); a != b {
		t.Fatalf("同内容两次写出不一致:\n%s\n---\n%s", a, b)
	}
}

// TestNilSliceSerializesAsNull 钉住当前 encoder 的边界：nil 切片写出
// null，而非 []。[] 不 null 是调用方把切片初始化成非 nil 的结果（见
// TestEmptySliceIsNotNull），不是这里提供的保证。若日后想让 encoder
// 自动把 nil 强制改写成 []，本测试会先变红，提醒这是一次契约变更。
func TestNilSliceSerializesAsNull(t *testing.T) {
	got := encode(t, payload{Name: "x", Items: nil})
	want := "{\n  \"name\": \"x\",\n  \"items\": null\n}\n"
	if got != want {
		t.Fatalf("nil 切片应写出 null\ngot  %q\nwant %q", got, want)
	}
	if !strings.Contains(got, "null") {
		t.Fatalf("nil 切片没写成 null:\n%s", got)
	}
}

// TestMarshalJSONBytesMatchesWriteJSON 钉住两条通路逐字节一致：
// "先 marshal 校验再落盘"和"直接落盘"若写出两份不同形状，
// 磁盘上的文件就会随调用路径漂移，git diff 失去意义。
func TestMarshalJSONBytesMatchesWriteJSON(t *testing.T) {
	p := payload{Name: "番茄 & <b>加蛋</b> 面", Items: []string{"a"}}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, p); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	got, err := MarshalJSONBytes(p)
	if err != nil {
		t.Fatalf("MarshalJSONBytes: %v", err)
	}
	if string(got) != buf.String() {
		t.Fatalf("两条通路不一致\ngot  %q\nwant %q", got, buf.String())
	}
}

// TestMarshalJSONBytesUnencodableReturnsNilErr 钉住失败形态：编不出去时
// 返回 nil 而不是半截字节，调用方才不会把残包写进文件。
func TestMarshalJSONBytesUnencodableReturnsNilErr(t *testing.T) {
	got, err := MarshalJSONBytes(func() {})
	if err == nil {
		t.Fatal("不可编码的值要报错")
	}
	if got != nil {
		t.Fatalf("出错时要返回 nil 字节，got %q", got)
	}
}
