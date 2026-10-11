package httpapi

import (
	"strings"
	"testing"
)

// 2026-10-10 改判之后，**谁**都看不到别人：管理员也一样。
//
// 原来这条只查非管理员（"管理员该看到一列别人"），而那条路连同
// `/switch to=<id>` 已经删掉了。现在徽标是一个直接回选择页的表单，
// 页面上不该出现任何别人的名字，也不该出现任何 `to=<档案 id>`。
//
// 为什么保留这条而不是并进 boundary_test.go：它守的是**页面形状**
// （印没印别人的名字），而 boundary_test 守的是**权限判定**
// （请求会不会被拒）。两者坏掉时长相一样，翻起来时找的地方不一样。
func Test档案徽标谁都不列出别人(t *testing.T) {
	_, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	kid := sessionCookie(t, loginAs(t, h, "kid", "5678"))

	for name, cookie := range map[string]string{"管理员": dad, "非管理员": kid} {
		body := get(h, "/", cookie).Body.String()
		// **切档入口**一个都不许有（曾经有 `name="to" value="<档案 id>"` 那几枚）。
		// 判据看的是 `to` 的值——页面当然可以印出**自己**的名字，那是徽标本身。
		if strings.Contains(body, `name="to" value="dad"`) ||
			strings.Contains(body, `name="to" value="kid"`) {
			t.Errorf("%s那一面出现了直达某个档案的入口：\n%.400s", name, body)
		}
		// 但"回选择页"必须还在：那是任何有效会话都能用的通路（§6.1）。
		if !strings.Contains(body, `name="to" value="picker"`) {
			t.Errorf("%s连「回选择页」都没了——那是任何有效会话都能用的通路：\n%.400s", name, body)
		}
	}
}
