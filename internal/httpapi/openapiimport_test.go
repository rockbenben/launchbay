package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// 导入那两条表单端点的失败形状长期只在描述里写着、响应格却与 handler 对不上
// （2026-10-24 代码审查）：
//   - `/import/apply` 校验/快照/目标没了是 **200 选文件页**，不是 303；
//   - `/import/backup` 失败是 **200 面板重画**，但文档只有 303 一格。
//
// 这条闸把响应格与 handler 的分叉钉住：以后谁再把失败写成 303、或者把
// `/import/backup` 的 200 删掉，测试当场红，而不是让照文档写脚本的人去猜。
func TestOpenAPI_导入表单的失败形状与handler一致(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Description string `json:"description"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatalf("openapi.json 读不出形状: %v", err)
	}

	apply := doc.Paths["/import/apply"]["post"].Responses
	if _, ok := apply["200"]; !ok {
		t.Fatal("/import/apply 没有 200 响应格：校验/快照/目标没了那几种失败就是 200")
	}
	if d := apply["200"].Description; !strings.Contains(d, "没写成") {
		t.Errorf("/import/apply 的 200 没写失败形状：%q", d)
	}
	if d := apply["303"].Description; strings.Contains(d, "校验没过") {
		t.Errorf("/import/apply 的 303 仍把校验失败算成 303：%q", d)
	}

	backup := doc.Paths["/import/backup"]["post"].Responses
	if _, ok := backup["200"]; !ok {
		t.Fatal("/import/backup 没有 200 响应格：失败时 handler 会重画面板")
	}
	if d := backup["200"].Description; !strings.Contains(d, "没写成") {
		t.Errorf("/import/backup 的 200 没说清这是失败形状：%q", d)
	}
	if d := backup["303"].Description; strings.Contains(d, "没写成") {
		t.Errorf("/import/backup 的 303 仍把失败写在自己身上：%q", d)
	}
}
