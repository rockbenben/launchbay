package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

// npmPkg 是一个 @iconify-json 套的下载入口。用 npm registry 而不是
// api.iconify.design/collection：后者只回元数据加 4 个样本（实测 2026-10-06，
// tabler 那条响应 107KB 且 icons 为空），拿不到全量 body。
type npmPkg struct {
	Prefix string // tabler
	Pkg    string // @iconify-json/tabler
	Layer  string // stroke / fill，决定 CSS 怎么呈现
}

// collectionDoc 是 npm 包里 icons.json 的形状（只用得到的字段）。
// 注意：icons.json 里没有 total——上游声明的 total 在 info.json，用 totalOf 取。
type collectionDoc struct {
	Prefix  string `json:"prefix"`
	Aliases map[string]struct {
		Parent string `json:"parent"`
	} `json:"aliases"`
	Icons map[string]struct {
		Body string `json:"body"`
	} `json:"icons"`
}

// fetchIconsJSON 取 registry 元数据 → 下载 tarball → 掏 package/icons.json
// 与 package/info.json（后者带许可与 total，产物头部要用它）。
// 两份一起返回：分两次下载同一个 tarball 是纯粹的浪费，也是"许可读不到"
// 这种偷懒写法的原因。
func fetchIconsJSON(client *http.Client, p npmPkg, version string) (iconsJSON, infoJSON []byte, upstream string, err error) {
	metaRaw, err := httpGet(client, "https://registry.npmjs.org/"+p.Pkg)
	if err != nil {
		return nil, nil, "", err
	}
	var meta struct {
		DistTags map[string]string `json:"dist-tags"`
		Versions map[string]struct {
			Dist struct {
				Tarball string `json:"tarball"`
			} `json:"dist"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil, nil, "", fmt.Errorf("%s 的 registry 元数据读不了: %w", p.Pkg, err)
	}
	if version == "" {
		version = meta.DistTags["latest"]
	}
	v, ok := meta.Versions[version]
	if !ok {
		return nil, nil, "", fmt.Errorf("%s 没有版本 %s，latest 是 %s", p.Pkg, version, meta.DistTags["latest"])
	}
	tgz, err := httpGet(client, v.Dist.Tarball)
	if err != nil {
		return nil, nil, "", err
	}
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, nil, "", fmt.Errorf("%s 的 tarball 不是 gzip: %w", p.Pkg, err)
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, "", fmt.Errorf("读 tar: %w", err)
		}
		// npm 包内路径固定是 package/icons.json；按基名认，少一种漂移
		// 就少一次"CI 上路径分隔符不一样"的意外。
		switch path.Base(h.Name) {
		case "icons.json":
			iconsJSON, err = io.ReadAll(tr)
		case "info.json":
			infoJSON, err = io.ReadAll(tr)
		}
		if err != nil {
			return nil, nil, "", fmt.Errorf("读 %s: %w", h.Name, err)
		}
		if iconsJSON != nil && infoJSON != nil {
			return iconsJSON, infoJSON, p.Pkg + "@" + version, nil
		}
	}
	if iconsJSON == nil {
		return nil, nil, "", fmt.Errorf("tarball 里没有 icons.json")
	}
	return iconsJSON, infoJSON, p.Pkg + "@" + version, nil
}

// httpGet 不设超时上限给探针：抓取约束是**运行时**的（spec §5），构建期
// 跑生成器是一次性人工动作，用 60 秒兜住"网络挂了别卡死终端"。
func httpGet(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", "launchbay-genicons")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s -> HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// elementsOf 把上游 body 拆成元素序列。这里用 encoding/xml 而不是正则：
// 属性值里出现 `>` 的正则拆分会把一条图标切成两半，而这类图标恰好是
// 路径数据最长、最容易出事的那批。
func elementsOf(body string) ([]svgEl, error) {
	// 包裹标签必须闭合：brief 里写的是 "<svg " + body，拼出来是
	// "<svg <path ..."——svg 的开始标签永远不闭合，上游每一条 body 都会
	// 被当成 svg 标签里的属性而全量报 "expected attribute name"。
	dec := xml.NewDecoder(strings.NewReader("<svg>" + body + "</svg>"))
	dec.Strict = false // 上游有自闭合不一致的写法，宽松模式照样报错在下一步
	var out []svgEl
	for {
		tk, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("SVG 解析失败: %w", err)
		}
		el, ok := tk.(xml.StartElement)
		if !ok {
			continue // 结束标签/字符数据都不承载元素信息，直接跳过
		}
		e := svgEl{Tag: el.Name.Local}
		for _, a := range el.Attr {
			e.Attrs = append(e.Attrs, attr{Name: a.Name.Local, Value: a.Value})
		}
		// 只有 g/svg 是容器。defs 与 use **不是**：产物里没有 id 引用体系，
		// 把它们当容器下钻会掏出一张谁都引用不到的孤图（画一半），
		// 所以原样留在结果里，由 encodeElements 把整条图标判为不支持。
		// 拍平不需要跟踪深度：输出只看"当前元素是不是容器"，容器本身不进
		// 结果、其子元素照常作为 StartElement 出现（评审 Minor ②：原先
		// 那个只压不读的 stack 是看着像逻辑的装饰，已删；行为由
		// TestElementsOf拍平容器但defs与use原样保留钉住）。
		if isContainer(e.Tag) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

type svgEl struct {
	Tag   string
	Attrs []attr
}

type attr struct {
	Name  string
	Value string
}

func isContainer(tag string) bool { return tag == "g" || tag == "svg" }

// scanVocab 把 body 里**全部**起始元素（含 g/svg 容器本身）的 tag 与属性名
// 计入两张 counts 表。探针词表是"上游出现过什么"的量尺：g 必须被数到——
// brief Step 2 的 elementsOf 把容器拍平掉了，照它统计的"tag 词表"永远不会
// 出现 g:2302，白名单也就不知道要防它；同理，只挂在容器上的属性名（tabler
// 实测的 class）也必须进 attr 名表，两张表同范围，门禁才无死角。
// 量尺与编码器分职，encodeElements 仍用 elementsOf 的拍平结果。
func scanVocab(body string, tags, attrNames map[string]int) error {
	dec := xml.NewDecoder(strings.NewReader("<svg>" + body + "</svg>"))
	dec.Strict = false
	depth := 0 // 包裹用的根 <svg> 不计数：词表量的是 body 里出现过的元素
	for {
		tk, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("SVG 解析失败: %w", err)
		}
		switch el := tk.(type) {
		case xml.StartElement:
			if depth > 0 {
				tags[el.Name.Local]++
				for _, a := range el.Attr {
					attrNames[a.Name.Local]++
				}
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

// totalOf 从 info.json 取上游声明的 total。icons.json 里没有 total 键
// （2026-10-06 实测两包都没有），所以"上游声明 total"只能来自 info.json，
// brief 的 collectionDoc.Total 挂在 icons.json 上永远是 0。
func totalOf(infoJSON []byte) int {
	var info struct {
		Total int `json:"total"`
	}
	if len(infoJSON) > 0 {
		_ = json.Unmarshal(infoJSON, &info)
	}
	return info.Total
}

// vocabTables 扫一遍 doc 的全部 body，产出探针的三张词表与解析失败清单：
//   - tags：全量起始标签（含 g/svg 容器，量尺走 scanVocab）；
//   - attrValues：name=value 计数（拍平后进 encodeElements 视野的元素），
//     top30 用来看常见组合；
//   - attrNames：**仅按属性名**统计的全量词表（与 tags 同范围，含容器），
//     Step 7 门禁的可执行形态。
//
// attr 名必须独立成表：所有 d=<路径> 的计数都是 1，name=value 的 top30 按
// 次数降序，任何新出现的稀有属性名（恰恰最需要人看）永远挤不进去。白名单
// 对不在 keepAttrs 的属性是静默 continue（呈现属性归 CSS），"看得见新属性"
// 的唯一防线就是这张表——它必须整份打出来，不许再靠一次性脚本或人眼扫 top30。
func vocabTables(doc collectionDoc) (tags, attrValues, attrNames map[string]int, failed []string) {
	tags = map[string]int{}
	attrValues = map[string]int{}
	attrNames = map[string]int{}
	for name, ic := range doc.Icons {
		if err := scanVocab(ic.Body, tags, attrNames); err != nil {
			failed = append(failed, fmt.Sprintf("%s 解析失败：%v", name, err))
			continue
		}
		els, err := elementsOf(ic.Body)
		if err != nil {
			continue // scanVocab 已报过同一条解析错误，不重复刷屏
		}
		for _, e := range els {
			for _, a := range e.Attrs {
				attrValues[a.Name+"="+a.Value]++
			}
		}
	}
	return tags, attrValues, attrNames, failed
}

// attrNameFullTable 是 attr 名全量词表的输出形状：sortCount 排序（次数降序、
// 同次数名字典序）、**绝不截断**。任何截断（topN）都会把稀有新属性名藏掉，
// 等于把 Step 7 的门禁重新变回一次性人工动作。
func attrNameFullTable(attrNames map[string]int) []string {
	return sortCount(attrNames)
}
