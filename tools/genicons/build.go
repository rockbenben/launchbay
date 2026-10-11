package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rockbenben/launchbay/internal/icons"
)

// productDoc 与 internal/icons 的 doc 分属两个包、靠 json tag 对齐；
// Task 1 的 Decode 开了 DisallowUnknownFields，所以字段名一漂就会在
// buildAndWrite 的自校验里直接报错——这就是把自校验放在写盘之前的全部理由。
type productDoc struct {
	Prefix        string                     `json:"prefix"`
	Layer         string                     `json:"layer"`
	ViewBox       string                     `json:"viewBox"`
	Count         int                        `json:"count"`
	UpstreamTotal int                        `json:"upstreamTotal"`
	Upstream      string                     `json:"upstream"`
	GeneratedAt   string                     `json:"generatedAt"`
	License       string                     `json:"license"`
	Icons         map[string]json.RawMessage `json:"icons"`
	Aliases       map[string]string          `json:"aliases"`
}

// buildSet 是产物内容的唯一生成点：探针的体积读数与 Task 2 写盘的字节
// 都出自这里，所以两者不可能分家（这是本任务把编码提前落地的原因）。
func buildSet(p npmPkg, doc collectionDoc) (entries map[string]json.RawMessage,
	aliases map[string]string, skipped []string, err error) {
	entries = map[string]json.RawMessage{}
	aliases = map[string]string{}
	for name, ic := range doc.Icons {
		els, err := elementsOf(ic.Body)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s:%s 解析失败: %w", p.Prefix, name, err)
		}
		flat, ok := encodeElements(p, els)
		if !ok {
			skipped = append(skipped, name) // use/defs/未知元素：整条不要，不画半个
			continue
		}
		entries[name] = flat
	}
	// 别名只保留父名真的进了产物的那些：留着指向被跳过图标的别名，
	// Task 1 的解码器会判整份产物非法——宁可在生成时就少一条别名。
	for alias, rel := range doc.Aliases {
		if _, ok := entries[rel.Parent]; ok {
			aliases[alias] = rel.Parent
		}
	}
	return entries, aliases, skipped, nil
}

// encodeElements 把实测到的上游元素压成产物写法。ok=false 表示这条图标含产物格式
// 表达不了的东西（use 引用），调用方整条跳过并报告。
//
// 丢掉的属性都是本层默认值：stroke 层的 fill="none"、fill 层的 fill="currentColor"，
// 以及 stroke / stroke-width / stroke-linecap / stroke-linejoin（那四个由 CSS 全权
// 负责，§7.5 要磁贴内 2.5 就只改 CSS 一处）。
func encodeElements(p npmPkg, els []svgEl) (json.RawMessage, bool) {
	if len(els) == 0 {
		return nil, false
	}
	var kept [][]string
	for _, e := range els {
		if !icons.IsDrawable(e.Tag) {
			return nil, false
		}
		row := []string{e.Tag}
		for _, a := range e.Attrs {
			if !icons.IsKeptAttr(a.Name) {
				continue
			}
			if a.Name == "fill" && icons.IsLayerDefaultFill(p.Layer, a.Value) {
				continue
			}
			if !icons.ValidAttrValue(a.Value) {
				return nil, false
			}
			row = append(row, a.Name, a.Value)
		}
		kept = append(kept, row)
	}
	if len(kept) == 1 && len(kept[0]) == 3 && kept[0][0] == "path" && kept[0][1] == "d" {
		return jsonMust(kept[0][2]), true // 裸 d 写法
	}
	return jsonMust(kept), true
}

// marshalProduct 编出未压缩的产物 JSON。键序可预测：头部字段按 productDoc 的声明序，
// icons 的键由 encoding/json 按字典序输出。
func marshalProduct(p npmPkg, doc collectionDoc, infoJSON []byte,
	entries map[string]json.RawMessage, aliases map[string]string,
	upstream string, generatedAt time.Time) ([]byte, error) {
	out := productDoc{
		Prefix: p.Prefix, Layer: p.Layer, ViewBox: "0 0 24 24",
		Count: len(entries), UpstreamTotal: totalOf(infoJSON), Upstream: upstream,
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339),
		License:     licenseOf(infoJSON),
		Icons:       entries, Aliases: aliases,
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// licenseOf 从 info.json 的 license 字段取 SPDX 名（实测 MIT / CC0 1.0）。
// 取不到就写 "?"：留空会让这套图标的许可变成考古题。
func licenseOf(infoJSON []byte) string {
	var info struct {
		License struct {
			Spdx string `json:"spdx"`
			Tit  string `json:"title"`
		} `json:"license"`
	}
	if len(infoJSON) > 0 {
		_ = json.Unmarshal(infoJSON, &info)
	}
	if info.License.Spdx != "" {
		return info.License.Spdx
	}
	if info.License.Tit != "" {
		return info.License.Tit
	}
	return "?"
}

// buildAndWrite 是产物落盘的唯一通路：编码 → 用消费方的解码器读回来自校验 → 才写盘。
// 顺序反过来（先写再测）等于把"产物能不能被消费"交给 CI 去发现：
// 生成器 exit 0 而首屏空白，是最坏的一种"成功"。
//
// 写盘前自校验用的就是 internal/icons 的 Decode（它开了 DisallowUnknownFields），
// 所以 productDoc 与 doc 的 json tag 一旦漂了，这里当场报错而不是留下坏产物。
// 磁盘路径与 load.go 的 //go:embed 路径是同一处约定，两边改了必须一起改。
func buildAndWrite(p npmPkg, doc collectionDoc, infoJSON []byte, upstream string) error {
	entries, aliases, skipped, err := buildSet(p, doc) // 唯一编码路径，与 -probe 同源
	if err != nil {
		return err
	}
	sort.Strings(skipped)
	raw, err := marshalProduct(p, doc, infoJSON, entries, aliases, upstream, time.Now())
	if err != nil {
		return err
	}
	// 自校验：消费方读不回来自家产物，就不配写进仓库。
	if _, err := icons.Decode(raw); err != nil {
		return fmt.Errorf("%s 的产物自己都不认: %w", p.Prefix, err)
	}
	gz, err := gzipBytes(raw) // helpers.go：level 9，Name 固定，ModTime 归零
	if err != nil {
		return err
	}
	gzPath := filepath.Join("internal", "icons", "index", p.Prefix+".json.gz")
	if err := os.MkdirAll(filepath.Dir(gzPath), 0o755); err != nil {
		return err
	}
	// 先写临时文件再 rename 不是过度设计：产物是提交进仓库的字节，
	// 写一半被 Ctrl-C 打断会留下一个 gzip 头完好的截断档，而截断档会被
	// 下一次的 ModTime 归零"盖住"看不出来。
	tmpPath := gzPath + ".tmp"
	if err := os.WriteFile(tmpPath, gz, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, gzPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	// 别名数也打出来：体积走到后备阶梯第①档（砍 aliases）时的判据就是它。
	fmt.Printf("%s: 收录 %d 条（上游声明 total=%d），跳过 %d 条（%s），别名 %d 条，raw %d 字节 -> gzip %d 字节 -> %s\n",
		p.Prefix, len(entries), totalOf(infoJSON), len(skipped), strings.Join(clip(skipped, 6), " "),
		len(aliases), len(raw), len(gz), gzPath)
	return nil
}
