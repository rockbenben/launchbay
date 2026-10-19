package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// 升级兼容：**2026-10-12 之前落盘的 profile.json 必须照样读得动**。
//
// 这不是一条"顺手补的测试"，它是那次改判里最容易变成灾难的一格。档案的 `admin`
// 字段被主动删掉了（管理员成了机器级的一把 key），而 `decodeProfile` 对未知键是
// 严格的——两个事实相加的后果是：**升级前每一个 profile.json 都变成坏档**，整台机器
// 谁都进不去，只能挨个走 `POST /api/v1/profiles/rebuild` 人工抢救。
//
// 一次"把模型简化干净"的改判，不该要求用户先把全家救一遍。所以 `admin` 是解码器
// 唯一放过的那个键。
func TestLoadProfile_认得升级前那份admin键(t *testing.T) {
	for _, legacy := range []string{
		`{"id":"dad","title":"爸爸","admin":true,"pin":"sha256$a$b","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar"}`,
		`{"id":"dad","title":"爸爸","admin":false,"pin":"sha256$a$b","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar"}`,
		`{"id":"dad","title":"爸爸","pin":"sha256$a$b","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar"}`,
	} {
		s := New(t.TempDir())
		if err := os.MkdirAll(filepath.Join(s.Root(), "profiles", "dad"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(s.Root(), "profiles", "dad", "profile.json"),
			[]byte(legacy), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := s.LoadProfile("dad")
		if err != nil {
			t.Fatalf("这一份该读得动，实际 %v\n盘上内容：%s", err, legacy)
		}
		if p.ID != "dad" || p.Title != "爸爸" {
			t.Errorf("读出来的字段不对: %+v", p)
		}
		// 它不靠"档案现在是管理员"来成立——那个属性已经不存在了。
		if n, err := s.CountProfiles(); err != nil || n != 1 {
			t.Errorf("它不该被记成坏档：CountProfiles = %d err=%v", n, err)
		}
	}
}

// 后门**只对 admin 那一格开**。别的未知键仍然判坏档——那正是 rebuild 那条通路的
// 判据来源，拿掉严格性等于坏档再也发现不了。
func TestLoadProfile_别的未知键仍然判坏档(t *testing.T) {
	s := New(t.TempDir())
	if err := os.MkdirAll(filepath.Join(s.Root(), "profiles", "dad"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"id":"dad","title":"爸爸","pin":"sha256$a$b","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar","admni":true}`,
		`{"id":"dad","title":"爸爸","pin":"sha256$a$b","createdAt":"2026-10-05T12:00:00+08:00","theme":"霓虹","layout":"sidebar"}`,
	} {
		if err := os.WriteFile(filepath.Join(s.Root(), "profiles", "dad", "profile.json"),
			[]byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := s.LoadProfile("dad"); err == nil {
			t.Errorf("该判坏档：%s", bad)
		}
	}
}

// 重写一次之后 admin 那一格自然消失：迁移是"下一次写盘顺手完成"的，不需要脚本。
func TestLoadProfile_重写一次就完成迁移(t *testing.T) {
	s := New(t.TempDir())
	dir := filepath.Join(s.Root(), "profiles", "dad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.json"),
		[]byte(`{"id":"dad","title":"爸爸","admin":true,"pin":"sha256$a$b","createdAt":"2026-10-05T12:00:00+08:00","theme":"auto","layout":"sidebar"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := s.LoadProfile("dad")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"admin"`) {
		t.Errorf("重写之后不该再有 admin 那一格（它该自己迁移掉）:\n%s", raw)
	}
	// 顺带钉住写出契约：末尾恰好一个换行、键序由字段序固定（§4.3）。
	if !strings.HasSuffix(string(raw), "}\n") || strings.HasSuffix(string(raw), "\n\n") {
		t.Errorf("写出形状不对，末尾该恰好一个换行:\n%q", string(raw))
	}
	var _ = model.Profile{}
}

// **Profile 的每一格都要落盘，哪怕它是空串**（2026-10-19）。
//
// `view` / `pageTitle` / `addrLevel` 都是比老盘晚的字段，它们**空串也写**——
// 盘上看得见 `"view": ""`，而那是读侧「这一格被考虑过、只是选默认」的那条判据。
// 省掉它（给字段加 `omitempty`），键存在与键缺失就分不开，于是「没选过」与
// 「选了默认」在盘上成了两件事。
//
// 本项目里 `omitempty` 只用在**条目的可选内容**上（icon / desc / accent，
// 那几格空着是真的没有内容），从不用于 `Profile` 的这一族。`addrLevel` 加进来
// 时顺手带了 `omitempty`——那是一个测试没钉住、于是没人发现的偏差。
//
// 判据从**一份真实落盘的文件**读，而不是从 struct 的 tag 读：读 tag 钉的是
// 声明，而这条要钉的是**写出来的那一格真的在**。
func TestSaveProfile_晚加的那几格空串也落盘(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.CreateProfile(NewProfileInput{ID: "dad", Title: "爸爸"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Root(), "profiles", "dad", "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"view": ""`, `"pageTitle": ""`, `"addrLevel": ""`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("profile.json 里少了 %s：晚加的那几格空串也该落盘（键存在 = 这一格被考虑过）\n%s", key, raw)
		}
	}
}
