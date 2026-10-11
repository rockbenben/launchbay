package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// clip 截断字符串列表，探针输出最多报 n 个名字就够定位了。
func clip(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

// sortCount 把计数表按次数从多到少排成 "词:次数"；同次数按词字典序，
// 保证两次运行的输出逐字一致（复现性从打印开始）。
func sortCount(counts map[string]int) []string {
	type kv struct {
		Key   string
		Count int
	}
	ranked := make([]kv, 0, len(counts))
	for k, v := range counts {
		ranked = append(ranked, kv{Key: k, Count: v})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		return ranked[i].Key < ranked[j].Key
	})
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = fmt.Sprintf("%s:%d", r.Key, r.Count)
	}
	return out
}

// topN 取排序后的前 n 项。
func topN(sorted []string, n int) []string {
	if len(sorted) <= n {
		return sorted
	}
	return sorted[:n]
}

// fatal 打 stderr 后退出，CLI 的统一死法。
func fatal(err error) {
	fmt.Fprintln(os.Stderr, "genicons:", err)
	os.Exit(1)
}

// jsonMust 编码或 panic：入参全是本包构造的纯数据（string、[][]string），
// 编不出来就是代码 bug，不是运行期意外。
func jsonMust(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("jsonMust 编不出 %v: %v", v, err))
	}
	return raw
}

// gzipBytes 是可复现的 gzip：level 固定 9、Name 固定、ModTime 归零。
// 同一份 raw 两次压出来必须逐字节相同，否则 Task 2 的"产物可复现"验收
// 会被时间戳和随机头差异骗过去。
func gzipBytes(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	zw.Name = "icons.json"
	zw.ModTime = time.Time{}
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
