package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

func main() {
	var (
		probe   = flag.Bool("probe", false, "只实测上游词表与产物体积，不写文件")
		version = flag.String("version", "", "钉住 @iconify-json 的版本；空 = registry latest")
		only    = flag.String("prefix", "", "只处理某一个套（tabler / simple-icons）")
	)
	flag.Parse()

	sets := []npmPkg{
		{Prefix: "tabler", Pkg: "@iconify-json/tabler", Layer: "stroke"},
		{Prefix: "simple-icons", Pkg: "@iconify-json/simple-icons", Layer: "fill"},
	}
	if *only != "" {
		var filtered []npmPkg
		for _, p := range sets {
			if p.Prefix == *only {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) == 0 {
			fatal(fmt.Errorf("--prefix 只能是 tabler / simple-icons，得到 %q", *only))
		}
		sets = filtered
	}

	client := &http.Client{Timeout: 60 * time.Second}
	for _, p := range sets {
		raw, infoRaw, upstream, err := fetchIconsJSON(client, p, *version)
		if err != nil {
			fatal(err)
		}
		var doc collectionDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			fatal(fmt.Errorf("%s 的 icons.json 读不了: %w", p.Prefix, err))
		}
		if *probe {
			probeSet(p, doc, len(raw), infoRaw, upstream)
			continue
		}
		if err := buildAndWrite(p, doc, infoRaw, upstream); err != nil {
			fatal(err)
		}
	}
}

// probeSet 打印实测的四件事：条目数、元素与属性词表、要整条跳过的图标名，
// 以及按本任务的编码规则编出来的产物体积。词表决定白名单，体积决定要不要走
// spec §5 的后备阶梯——两个都是改判依据，不是日志装饰。
// 体积必须由本任务自己量：引用别人某一次的读数为红线设防，等于没有红线。
func probeSet(p npmPkg, doc collectionDoc, iconsBytes int, infoJSON []byte, upstream string) {
	tags, attrValues, attrNames, failed := vocabTables(doc)
	sort.Strings(failed)
	// 丢弃清单要报到图标名：只给个数字，下一个读产物的人会以为
	// "丢了 9 个元素"等于"少了 9 个图标"，而实际是三个图标画不全。
	for _, f := range failed {
		fmt.Printf("%s:%s\n", p.Prefix, f)
	}
	// 产物与体积走同一条编码路径（buildSet），探针量到的就是最终提交的字节形状。
	entries, aliases, skipped, err := buildSet(p, doc)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("\n=== %s %s ===\n条目 %d（上游声明 total=%d）收录 %d\n",
		p.Prefix, upstream, len(doc.Icons), totalOf(infoJSON), len(entries))
	fmt.Println("tag 词表:", strings.Join(sortCount(tags), " "))
	fmt.Println("attr=值 词表（top 30）:", strings.Join(topN(sortCount(attrValues), 30), " "))
	// attr 名全量表整份打印、不截断：这是 Step 7 的门禁——keepAttrs 之外
	// 的属性会被 encodeElements 静默丢掉，新属性名只有在这张表里才看得见。
	fmt.Println("attr 名全量词表:", strings.Join(attrNameFullTable(attrNames), " "))
	sort.Strings(skipped)
	fmt.Printf("要整条跳过的图标 %d 个：%s\n", len(skipped), strings.Join(clip(skipped, 12), " "))

	raw, err := marshalProduct(p, doc, infoJSON, entries, aliases, upstream, time.Now().UTC())
	if err != nil {
		fatal(err)
	}
	gz, err := gzipBytes(raw)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("上游 icons.json %d 字节  产物 raw %.2fMB  gzip %.1fKB（别名 %d 条）\n",
		iconsBytes, float64(len(raw))/1048576, float64(len(gz))/1024, len(aliases))
}
