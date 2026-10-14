package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
)

// writeOnce 走一次普通写入：每次写之前拍一份快照，于是**写 N 次就有 N 份快照**。
// 回退页那些候选就是它们（每份 = 改动之前的样子）。
func writeOnce(t *testing.T, h http.Handler, rev int, name string) {
	t.Helper()
	body := `{"profile":"dad","by":"ai","expectedRev":` + strconv.Itoa(rev) + `,"ops":[
	  {"op":"set","path":"groups/nas/items/dsm/name","value":"` + name + `"}]}`
	if rec := post(h, "/api/v1/batch", body, "", dadTok); rec.Code != 200 {
		t.Fatalf("前置写入失败: %d %s", rec.Code, rec.Body)
	}
}

// 回退页只在**分组真的变过**的那些候选上列组名（判据见 revertpage.go 的
// `snapshotChoice.SameGroups`：与上一行一字不差就不重印）。这个取舍是对的——
// 13 份快照的实拍里，12 行的组名一字不差地重复，每行四成宽度花在从不变化的信息上。
//
// 但**取舍本身没有出现在页面上**：同一张列表里，"没写分组"读起来像"那一版没有
// 分组"，而它真正的意思是"与上一行一样"。两句话差别很大——前者让人以为退回去
// 会丢目录，于是不敢退。
func Test回退页说清没写分组是什么意思(t *testing.T) {
	_, h := setupBatch(t)
	// 写两次 = 两份快照 = 两个候选。两次都没碰分组，于是靠后那一行必然被收起。
	writeOnce(t, h, 1, "改过的")
	writeOnce(t, h, 2, "又改一次")

	rec := get(h, "/revert", revertDadCookie)
	if rec.Code != 200 {
		t.Fatalf("该 200, got %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	note := i18n.Load(i18n.Zh).T("revert.choice.note")

	if strings.Count(body, `name="to-rev"`) == 0 {
		t.Skip("这份档案没有两份快照，收起不了任何一行（那是 SameGroups 那条判据）")
	}
	if !strings.Contains(body, note) {
		t.Errorf("有行收起了组名，页面上却没有解释它：\n%.800s", body)
	}
	// 它是 legend 底下那一句，不是又一个候选。
	if !strings.Contains(body, "<legend>选一版</legend>") {
		t.Fatal("legend 不见了，说明这一族的组标题被改坏了")
	}
}

// 反向证人：**一行都没收起过**（只有一份快照，第一行没有"上一行"可比）时就不该
// 有这句——一句在解释一个页面上根本没发生的现象，比没有更吵。
func Test回退页没有行收起组名时不加那句(t *testing.T) {
	_, h := setupBatch(t)
	writeOnce(t, h, 1, "只写一次")

	rec := get(h, "/revert", revertDadCookie)
	if rec.Code != 200 {
		t.Fatalf("该 200, got %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="to-rev"`) {
		t.Skip("这份档案没有快照，这一族整张表单都不画（那是 NoForm 那条判据）")
	}
	if strings.Contains(body, i18n.Load(i18n.Zh).T("revert.choice.note")) {
		t.Errorf("一行都没收起过，页面上却印了那句解释：\n%.800s", body)
	}
}
