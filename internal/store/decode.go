package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// DecodeJSON accepts one complete UTF-8 JSON value and rejects unknown fields.
// Decoder.Decode alone accepts a valid prefix and silently ignores its tail.
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
