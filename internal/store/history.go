package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ByValues 是 spec §4.6 的渠道枚举。它是调用方自报的口径，只用于历史
// 筛选与界面展示，**不参与任何权限判定**——所以校验它是为了数据整洁，
// 不是为了安全；别在这条路上长出"未知 by 拒绝启动"之类的东西。
//
// "cli" 是本案补的缺口：§4.6 原枚举没覆盖"人在终端跑 CLI"这一渠道，
// revert 的自报头 X-LB-By 就要用它（Task 13 把它写回 spec；openapi.json
// 的 by 枚举照七个值写）。
var ByValues = []string{"ai", "bookmarklet", "extension", "web", "import", "cli"}

func ValidBy(by string) bool {
	for _, v := range ByValues {
		if v == by {
			return true
		}
	}
	return false
}

// HistoryOp 只记"做了什么、对哪个路径"。没有 value 字段是刻意的：
// 一旦有了它，日志就变成第二份书签数据（还会带出 URL 与备注）。
type HistoryOp struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// HistoryEntry 的字段顺序就是 jsonl 的键顺序（同 §4.3 的写出契约）。
type HistoryEntry struct {
	At      string      `json:"at"`
	Profile string      `json:"profile"`
	Actor   string      `json:"actor"`
	By      string      `json:"by"`
	Rev     int64       `json:"rev"`
	Ops     []HistoryOp `json:"ops"`
}

// AppendHistory 用 O_APPEND 追加一行。它不是原子写：jsonl 是只增的审计线，
// 整档重写反而会在写坏时丢掉全部历史。半行的最坏后果是丢最后一条，
// ReadHistory 会跳过它。
func (s *Store) AppendHistory(e HistoryEntry) error {
	if e.Ops == nil {
		e.Ops = []HistoryOp{}
	}
	line, err := EncodeJSONL(e)
	if err != nil {
		return fmt.Errorf("编码历史行: %w", err)
	}
	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("开 %s: %w", s.HistoryPath(), err)
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("写历史: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("关历史文件: %w", err)
	}
	return nil
}

// ReadHistory 返回**最新在前**的列表，"最新"按文件顺序倒过来算。
//
// 不按 rev 排：rev 是每个档案自己的计数器，dad 的 rev 5 与 mom 的 rev 9
// 之间没有先后关系，跨档案按 rev 排会把审计线搅乱。
// profile 为空 = 全部；limit<=0 = 不限。坏行跳过，整文件读不了才报错。
func (s *Store) ReadHistory(profile string, limit int) ([]HistoryEntry, error) {
	f, err := os.Open(s.HistoryPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("读 %s: %w", s.HistoryPath(), err)
	}
	defer f.Close()

	var out []HistoryEntry
	// 用 bufio.Reader 而不是 Scanner：Scanner 有行长上限，超限是
	// ErrTooLong **且不可恢复**——一条超长行就让整档历史读不出来，
	// 正是"坏一行不许坏全家"反着来的方向（评审 Important #2）。
	// ReadString 没有上限，行长多少就吃多少，坏行只剩"解不开"一种。
	br := bufio.NewReader(f)
	for {
		line, readErr := br.ReadString('\n')
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			var e HistoryEntry
			// 解不开就丢这一行，继续读后面的。
			if json.Unmarshal([]byte(trimmed), &e) == nil && (profile == "" || e.Profile == profile) {
				out = append(out, e)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("扫历史: %w", readErr)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
