package icons

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
)

//go:embed index/tabler.json.gz index/simple-icons.json.gz
var indexFS embed.FS

// loadOrder 是解析顺序：品牌层在前，这样"simple-icons 的产物坏了"会先于
// tabler 报出来（品牌层是本案主力，坏得更早该更早知道）。搜索结果里的
// 层优先级由 search.go 自己的 prefixRank 决定，不靠这里的顺序（Task 3）。
var loadOrder = []string{"simple-icons", "tabler"}

// Load 把两套内嵌产物解成单个查询面。每一处失败都包上 ErrNoIndex：
// 装配点要能判"没有图"和"索引坏了"是两件事，前者降级、后者起不来。
func Load() (*Index, error) {
	var parts []*Index
	for _, prefix := range loadOrder {
		raw, err := indexFS.ReadFile("index/" + prefix + ".json.gz")
		if err != nil {
			return nil, fmt.Errorf("%w: 内嵌的 %s 产物不见了: %w", ErrNoIndex, prefix, err)
		}
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: %s 的 gzip 头坏了: %w", ErrNoIndex, prefix, err)
		}
		jsonBytes, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("%w: %s 解压失败: %w", ErrNoIndex, prefix, err)
		}
		if err := zr.Close(); err != nil {
			return nil, fmt.Errorf("%w: %s 收尾失败: %w", ErrNoIndex, prefix, err)
		}
		ix, err := Decode(jsonBytes)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrNoIndex, prefix, err)
		}
		parts = append(parts, ix)
	}
	return Merge(parts...), nil
}

// MustLoad 在装配点与测试里用：索引是内嵌产物，读不出来只有两种可能
// （embed 路径写错、产物损坏），两种都该让进程起来不了。
func MustLoad() *Index {
	ix, err := Load()
	if err != nil {
		panic(err)
	}
	return ix
}
