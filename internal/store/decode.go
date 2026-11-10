package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// DecodeJSON 统一校验并解码全案 JSON（2026-11-09 加）：
//  1. 必须是合法 UTF-8：encoding/json 会把非法字节静默替换为 U+FFFD 并写盘，造成数据静默损坏；
//  2. 严格拒绝未知字段（DisallowUnknownFields）：防止调用方拼错字段名时静默失效；
//  3. 必须恰好是一个完整顶层 JSON 值：标准库 Decoder.Decode 默认只读首个对象、静默忽略尾部
//     多余内容或乱码，此处确保数据尾部干净（必须读到 EOF）。
func DecodeJSON(data []byte, v any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON 不是合法 UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var tail json.RawMessage
	if err := dec.Decode(&tail); err != io.EOF {
		return fmt.Errorf("JSON 顶层值结束后还有多余内容")
	}
	return nil
}
