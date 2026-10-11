package icons

import "testing"

// 这三条把"快照里到底有什么"钉成断言，不是钉成一次人工查看：
// spec §5 的实测结论（podcast 不存在、brand-* 十个缺九）如果哪天变成假话，
// 要由测试来说，而不是由某个人某天点出一个空白图标来说。
func TestLoad_真实产物(t *testing.T) {
	ix := MustLoad()

	if n := ix.Count("tabler"); n < 6000 {
		t.Errorf("tabler 收录 %d 条，少于 6000，产物像是被截了", n)
	}
	if n := ix.Count("simple-icons"); n < 3400 {
		t.Errorf("simple-icons 收录 %d 条，少于 3400", n)
	}

	for _, known := range []string{
		"tabler:home", "tabler:download", "tabler:server", "tabler:certificate",
		"simple-icons:github", "simple-icons:jellyfin", "simple-icons:portainer",
		"simple-icons:syncthing", "simple-icons:homeassistant",
		// tabler:broadcast 被计划里这组断言列为"必须不存在"，实跑证伪：
		// @iconify-json/tabler@1.2.41 的 icons 里就有 broadcast（另有 broadcast-off）。
		// spec §5 那句"图标名不许靠猜"点名的只有 podcast，broadcast 是计划自己加的。
		// 按实测把它钉成"存在"，翻转判据的那次改判由 Task 2 报告与上游读数背书，
		// 而不是为了让套件变绿。
		"tabler:broadcast",
	} {
		if !ix.Has(known) {
			t.Errorf("%s 应当存在——它是本案常用的服务名", known)
		}
	}

	// spec §5 明写"图标名不许靠猜"的那两条实测，钉成冻结断言。
	for _, absent := range []string{"tabler:podcast", "mdi:home", "simple-icons:docker-alt"} {
		if ix.Has(absent) {
			t.Errorf("%s 居然存在了：要么上游改了口径要么产物错位，先查再放行", absent)
		}
	}

	// 每条图标都要能被渲染层真正消费：元素非空、属性合法。
	for _, prefix := range []string{"tabler", "simple-icons"} {
		for _, key := range ix.Names(prefix) {
			got, ok := ix.Get(key)
			if !ok || len(got.Elements) == 0 {
				t.Fatalf("%s 解出来是空的", key)
			}
		}
	}
}

func TestLoad_两套的层标记不同(t *testing.T) {
	ix := MustLoad()
	if got := mustHave(t, ix, "tabler:home").Layer; got != "stroke" {
		t.Errorf("tabler 层标成 %q，CSS 会拿错呈现", got)
	}
	if got := mustHave(t, ix, "simple-icons:github").Layer; got != "fill" {
		t.Errorf("simple-icons 层标成 %q", got)
	}
}

func mustHave(t *testing.T, ix *Index, key string) Icon {
	t.Helper()
	got, ok := ix.Get(key)
	if !ok {
		t.Fatalf("%s 查不到", key)
	}
	return got
}
