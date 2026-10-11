package store

import (
	"bytes"
	"encoding/json"
	"io"
)

// WriteJSON 按 spec §4.3 的写出契约编码：2 空格缩进、不转义 <>&、
// 非 ASCII 原样、末尾恰好一个换行。
//
// 键顺序由 struct 字段声明顺序决定。任何会落盘的类型都不许用
// map[string]X：那样"加一个键"就成了结构变更，而这里要的是人和
// AI 写完都能稳定 diff 的固定形状。
//
// 空切片必须由调用方初始化为非 nil（[]T{}）；本函数不区分 nil 与空，
// nil 切片仍写出 null。稳定 diff 契约靠调用方保证切片非 nil，而非
// 依赖这里把 null 强制改写成 []。
func WriteJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Encoder 自带末尾换行；TrimRight 再补一个是为了让"恰好一个"
	// 成为可断言的规则，而不是依赖实现细节。
	out := append(bytes.TrimRight(buf.Bytes(), "\n"), '\n')
	_, err := w.Write(out)
	return err
}

// MarshalJSONBytes 是 WriteJSON 的内存版：WriteFileJSON 的"先编码完整、
// 再原子落盘"走的就是这一条（同一份缓冲步骤，不另起炉灶），测试用它
// 对照磁盘形状。
func MarshalJSONBytes(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeJSONL 编一行 JSONL：紧凑、不转义 HTML、非 ASCII 原样、结尾一个换行。
// 不复用 WriteJSON——那是给人读的 2 空格缩进，多行会把 jsonl 的一行拆成
// 几十行，`tail -f` 与逐行解析都跟着废掉。
// json.Encoder.Encode 自带结尾换行，正好一行一条。
func EncodeJSONL(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
