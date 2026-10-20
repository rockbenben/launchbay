package httpapi

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 面板上**每一格**都必须是 `POST /prefs` 收的那几格之一（2026-10-19 补）。
//
// 这一族的名字散在三处：服务端那份（prefs.go 的 hasTheme/hasLayout/…）、
// 机器契约那份（openapi.json 的 properties）与面板那份（模板里每张表单的 name）。
// **三处各写一遍就会各漏一次**——实测漏过一次：第二行（addrLevel）在服务端与
// 面板上都有了，openapi.json 的 schema 里却没有，于是照机器契约写请求的脚本
// 少写这一格（见 openapi_test.go 那条）。
//
// 这条把**面板与契约**钉在一起：面板上出现一个契约里没有的字段名，人按下去
// 就是一次 422（"这一格拼错了"），而屏上读起来完全正常——那正是「点了没反应」
// 的另一种长相。
var (
	prefsPanelFormRE = regexp.MustCompile(`(?s)<form[^>]*action="/prefs"[^>]*>(.*?)</form>`)
	prefsPanelNameRE = regexp.MustCompile(`\bname="([^"]*)"`)
)

func Test面板上的字段名与契约里收的那几格同集(t *testing.T) {
	_, h := setupBatch(t)

	// 一、契约里 /prefs 收的那几格（机器可读那份就是权威）。
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Properties map[string]struct{} `json:"properties"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatalf("openapi.json 读不出形状: %v", err)
	}
	form := doc.Paths["/prefs"]["post"].RequestBody.Content["application/x-www-form-urlencoded"]
	want := map[string]bool{}
	for k := range form.Schema.Properties {
		want[k] = true
	}
	if len(want) != 6 {
		t.Fatalf("契约里 /prefs 只有 %d 个字段，要 6 个：形状变了这条正在空转", len(want))
	}

	// 二、面板上那六张表单各叫什么。
	rec := get(h, "/prefs", dadFormCookie)
	if rec.Code != 200 {
		t.Fatalf("面板页要 200，得到 %d", rec.Code)
	}
	got := map[string]bool{}
	for _, f := range prefsPanelFormRE.FindAllStringSubmatch(rec.Body.String(), -1) {
		for _, n := range prefsPanelNameRE.FindAllStringSubmatch(f[1], -1) {
			if n[1] != "" {
				got[n[1]] = true
			}
		}
	}
	if len(got) != len(want) {
		t.Errorf("面板上有 %d 个字段名，契约里 %d 个（%s / %s）：多出来的那一格服务端不收（422），"+
			"少掉的那一格人按不到",
			len(got), len(want), strings.Join(sortedKeys(got), ", "), strings.Join(sortedKeys(want), ", "))
	}
	for k := range want {
		if !got[k] {
			t.Errorf("契约收 %s，面板上却没有那一格：人按不到它", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("面板上有 %s，契约里不收：按下去就是一次 422", k)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
