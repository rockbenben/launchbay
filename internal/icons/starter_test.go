package icons

import (
	"strings"
	"testing"
)

// 名单里的每个名字都要**在真实产物里**（2026-10-20）。
//
// 上游产物换一版（simple-icons / tabler 都在动），掉出去的那个图标名就会
// 静静地从默认那一批里消失：面板照常开，只是少了一格——而没有任何功能
// 测试会红（端点还是 200，还是有方格）。这条闸就是为那个"不会红"存在的：
// 红了要改的是**名单**（补一个替代的或删掉那一条），不是把断言放宽。
func TestStarter_名单里的每个名字都在索引里(t *testing.T) {
	ix := MustLoad()
	for _, key := range Starter {
		if _, ok := ix.Get(key); !ok {
			t.Errorf("默认那一批里的 %q 不在索引里了：上游产物换过一版，"+
				"要么补一个替代的名字，要么把它从名单里删掉", key)
		}
	}
}

// 名单**不能是空的**，也不能只剩一条：默认那一批的全部意义就是"打开就有
// 东西可挑"，它一空，面板就回到从前那张空网格。
func TestStarter_名单不是空的(t *testing.T) {
	if len(Starter) < 12 {
		t.Fatalf("默认那一批只有 %d 条，太少了：面板打开要有一屏可挑的东西", len(Starter))
	}
	names := MustLoad().StarterNames()
	if len(names) != len(Starter) {
		t.Errorf("真实产物里只找得到 %d/%d 条——上面那条测试会红，"+
			"这里先说清楚少了哪几条", len(names), len(Starter))
	}
	if len(names) == 0 {
		t.Fatal("默认那一批是空的：面板打开又是一张空网格")
	}
}

// 顺序是**名单的顺序**（前面的是更常用的），而 StarterNames 不许自己重排。
func TestStarterNames_保持名单的顺序(t *testing.T) {
	got := MustLoad().StarterNames()
	for i, key := range Starter {
		if i >= len(got) {
			t.Fatalf("只找得到 %d 条，比名单短", len(got))
		}
		if got[i] != key {
			t.Errorf("第 %d 条是 %q，名单上写的是 %q", i, got[i], key)
		}
	}
}

// 装配点还没装上索引时（nil），这一格只是"没东西可挑"，不该把进程带下去。
func TestStarterNames_索引为nil不panic(t *testing.T) {
	var ix *Index
	if got := ix.StarterNames(); got != nil {
		t.Errorf("nil 索引要返回空，得到 %v", got)
	}
}

// 名单里不许出现重复：同一个名字画两格，人会以为是两枚不同的图标。
func TestStarter_没有重复的名字(t *testing.T) {
	seen := map[string]bool{}
	for _, key := range Starter {
		if seen[key] {
			t.Errorf("%q 在名单里出现两次", key)
		}
		seen[key] = true
	}
	if !strings.Contains(Starter[0], ":") {
		t.Errorf("名单里要写全键（带集合前缀），得到 %q", Starter[0])
	}
}
