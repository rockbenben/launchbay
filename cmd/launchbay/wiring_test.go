package main

import (
	"os"
	"strings"
	"testing"
)

// 装配点的**源码级**守卫（2026-10-12 补，写下这件事的由来见下面那段）。
//
// `main.go` 里那次真的漏了 `AdminToken:` 那一行。三个地方都看不出来：
//   - 编译通过（字段有零值，Go 不管你填不填）；
//   - `go test ./...` 全绿（测试各有自己的 `newDeps` / `Dependencies{…}`）；
//   - 服务起得来、健康检查也过（只是每一把管理员 key 都变成 401）。
//
// 而"管理员 key 全都 401"的现场是：一把完全有效的密钥，报错却说"令牌无效或已吊销"。
// 那种现象会先被怀疑成密钥写错、被重启、被重新拉镜像——查很久才想到装配点。
//
// 所以这里扫 `main.go` 的**源码**，要求 `httpapi.Dependencies{…}` 字面量里
// 每个字段名都出现一次。这不是行为断言，而且故意不是：行为上"漏一个字段"与
// "那个字段本来就该是零值"完全一样，只有源码能分。
//
// want=0 的那一半必须配同支正对照，否则"字段名一个都没扫到"与"扫描器坏了"
// 这两件事长得一模一样。
func TestMain装配点列全了Dependencies(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)

	start := strings.Index(src, "httpapi.Dependencies{")
	if start < 0 {
		t.Fatal("main.go 里找不到 httpapi.Dependencies{…} 字面量——装配点挪走了，这条闸正在空转")
	}
	end := strings.Index(src[start:], "})")
	if end < 0 {
		t.Fatal("Dependencies 字面量没有闭合")
	}
	literal := src[start : start+end]

	// 这些是 Dependencies 的全部字段（server.go）。少补一条要人记得同步。
	for _, field := range []string{
		"Store:", "Tokens:", "AdminToken:", "Render:", "Icons:", "Fetch:", "Cache:",
	} {
		if !strings.Contains(literal, field) {
			t.Errorf("main.go 的 httpapi.Dependencies{…} 里没有 %q —— "+
				"缺一个依赖字段不会编译失败、测试也不会红，只会在真机上让对应的凭证一律失效", field)
		}
	}

	// 正对照：扫描器确实认得这些字面量（want=0 的那一半配同支正对照）。
	found := 0
	for _, field := range []string{"Store:", "Tokens:", "AdminToken:", "Render:"} {
		if strings.Contains(literal, field) {
			found++
		}
	}
	if found < 4 {
		t.Fatalf("只认出 %d/4 个字段：扫描器与字面量形状对不上了", found)
	}
}
