package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 建档的"已存在"判据**按目录判，不按 profile.json 判**（2026-10-19）。
//
// 原先那道闸是 `os.Stat(s.ProfilePath(in.ID))`。于是"目录在、profile.json 缺"
// 这一个形状（手删、改名、rsync 半途、坏档被登记过）会**穿过**它，接着
// SaveBookmarks 把盘上那份 bookmarks.json 覆盖成空文档。实测：一份 469 字节、
// 含一条书签的档案，建档后变成 244 字节的空文档，**没有快照、没有审计行、
// 返回 201**——书签就这么没了，而"写坏了能回退"是这个产品写进首页的承诺。
//
// AGENTS.md 写的是「已存在 409」，所以这是行为与契约不符，不是有意的宽松。
func TestCreateProfile_目录已存在就算已存在(t *testing.T) {
	s := New(t.TempDir())

	// 造一个"目录在、书签在、profile.json 被删"的档案——正是删除一半的形状
	if err := os.MkdirAll(s.ProfileDir("dad"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := EmptyBookmarks()
	b.Groups = append(b.Groups, model.Group{
		ID: "g1", Title: "常用", Layout: model.LayoutTiles,
		Items: []model.Item{{ID: "keepme", Name: "我的书签", URL: "http://10.0.0.7/x"}},
	})
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}

	// 同一 id 建档：必须被挡住，且**一个字都不许动**
	_, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("目录里已有书签时建档该返回 ErrExists，实际 err=%v", err)
	}

	raw, err := os.ReadFile(filepath.Join(s.ProfileDir("dad"), "bookmarks.json"))
	if err != nil {
		t.Fatalf("盘上的书签读不回来了：%v", err)
	}
	if !strings.Contains(string(raw), "keepme") {
		t.Error("建档被拒之后盘上的书签变了——那道闸必须挡在第一次写入之前")
	}
}

// rollbackDir 是 `RemoveAll(整目录)`，它成立的前提是"目录里只有刚建的东西"。
// 上面那条挡住之后这个前提才重新成立；这一条钉住它真的成立——
// **它一旦不成立，第二次写失败会把连 snapshots/ 一起删掉**。
func TestCreateProfile_新建时目录里只有自己刚建的东西(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸", PinHash: "h"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"profile.json", "bookmarks.json"} {
		if _, err := os.Stat(filepath.Join(s.ProfileDir("dad"), name)); err != nil {
			t.Errorf("建档之后 %s 不在目录里：%v", name, err)
		}
	}
}

// 三处"这个 id 占了没有"必须是一件事：建档、派生下一个 id、rebuild。
// 判据散开时，建档守住了而 rebuild 没守住（或反过来），症状是**同一个 id 在
// 两处得到两个答案**，于是"重名字的坏档"这类东西会从缝里漏过去。
func TestIdTaken与建档闸同口径(t *testing.T) {
	s := New(t.TempDir())

	// 空目录（什么文件都没有）也算占用 —— 与 bootstrap 派生 id 同判据
	if err := os.MkdirAll(s.ProfileDir("ghost"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !s.idTaken("ghost") {
		t.Error("空目录没有被 idTaken 认成占用：派生下一个 id 时会撞上它")
	}
	if _, err := s.CreateProfile(NewProfileInput{ID: "ghost", Title: "鬼", PinHash: "h"}); !errors.Is(err, ErrExists) {
		t.Errorf("空目录却建档成功了（err=%v）：建档闸与 idTaken 不是同一件事", err)
	}
}
