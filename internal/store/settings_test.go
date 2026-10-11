package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/i18n"
	"github.com/rockbenben/launchbay/internal/model"
)

// 缺文件 = 中文。这是**升级不改变任何人现有界面**的那一条：
// 带多语言的版本装到一台已经建过档的机器上，data/ 下还没有 settings.json，
// 于是 Language() 落回中文，界面与升级前一字不差。
func TestLanguage_没有settings文件时落中文(t *testing.T) {
	s := New(t.TempDir())
	if got := s.Language(); got != i18n.Zh {
		t.Errorf("没有配置文件时应当是中文，拿到 %q", got)
	}
	if got := s.Language(); got != i18n.Default {
		t.Errorf("中文应当就是默认，拿到 %q", got)
	}
}

func TestLanguage_写进去读得回来(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetLanguage(i18n.En); err != nil {
		t.Fatalf("写语言: %v", err)
	}
	if got := s.Language(); got != i18n.En {
		t.Errorf("写了英文却读回 %q", got)
	}
	if err := s.SetLanguage(i18n.Zh); err != nil {
		t.Fatalf("写回中文: %v", err)
	}
	if got := s.Language(); got != i18n.Zh {
		t.Errorf("写回中文却读回 %q", got)
	}
}

// 三种「读不出来」都要落默认，而不是让整台机器的页面 500：文件不存在（已单独测）、
// 内容是坏的、里面存了一个不认识的值。
func TestLanguage_坏文件与不认识的值都落默认(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	for _, body := range []string{
		`{ 坏了`,              // JSON 语法错
		`{"language":"fr"}`, // 不受支持的值
		`{"language":"EN"}`, // 大写，不认
		`{}`,                // 缺字段
		`null`,              // 整个是 null
	} {
		if err := os.WriteFile(s.SettingsPath(), []byte(body), 0o644); err != nil {
			t.Fatalf("造夹具: %v", err)
		}
		if got := s.Language(); got != i18n.Default {
			t.Errorf("内容 %q 时应当落回中文，拿到 %q", body, got)
		}
	}
}

// 存一份自己读不回来的配置，是那种几个月后才有人发现的坏形状。
// 所以写入前就归一化：不受支持的值落成默认，**磁盘上不留垃圾**。
func TestSetLanguage_非法值归一化后再落盘(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetLanguage(i18n.Lang("fr")); err != nil {
		t.Fatalf("写: %v", err)
	}
	if got := s.Language(); got != i18n.Default {
		t.Errorf("非法值应当被归一化成中文，拿到 %q", got)
	}
	raw, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		t.Fatalf("读: %v", err)
	}
	if strings.Contains(string(raw), "fr") {
		t.Errorf("磁盘上留下了读不回来的值：%s", raw)
	}
}

// 落盘形状必须与其它 JSON 一致（spec §4.3）：2 空格缩进、末尾恰好一个换行。
// 这条在这里不是洁癖——AI 与人都会直接读 data/ 下的文件改配置，
// 形状与旁边那些文件不一样就会让 diff 变得难读。
func TestSetLanguage_落盘形状照spec4点3(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetLanguage(i18n.En); err != nil {
		t.Fatalf("写: %v", err)
	}
	raw, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		t.Fatalf("读: %v", err)
	}
	got := string(raw)
	want := "{\n  \"language\": \"en\"\n}\n"
	if got != want {
		t.Errorf("落盘形状不对。\n 实际 %q\n 期望 %q", got, want)
	}
}

// 站级设置与档案是两回事：删掉一个档案目录**不许**把语言设置一起带走。
func TestLanguage_删档不动站级设置(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := s.SetLanguage(i18n.En); err != nil {
		t.Fatalf("写: %v", err)
	}
	p := &model.Profile{ID: "dad", Title: "爸爸", PIN: "x", Theme: "auto", Layout: "sidebar"}
	if err := s.SaveProfile(p); err != nil {
		t.Fatalf("建档: %v", err)
	}
	if err := s.DeleteProfile("dad"); err != nil {
		t.Fatalf("删档: %v", err)
	}
	if got := s.Language(); got != i18n.En {
		t.Errorf("删档把站级语言设置弄丢了，拿到 %q", got)
	}
}

// 每次读都走磁盘（不缓存），所以管理员改完立刻生效，不必重启容器。
// 这条钉住的是**行为**：改完立刻生效，比省下一次几十字节的读重要。
func TestLanguage_改了立刻生效不需重启(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetLanguage(i18n.Zh); err != nil {
		t.Fatalf("写: %v", err)
	}
	if got := s.Language(); got != i18n.Zh {
		t.Fatalf("前置状态不对：%q", got)
	}
	if err := s.SetLanguage(i18n.En); err != nil {
		t.Fatalf("改: %v", err)
	}
	if got := s.Language(); got != i18n.En {
		t.Errorf("同一个 Store 实例里改完没有立刻生效，读到 %q", got)
	}
}

// settings.json 必须落在 data/ 根下，与 profiles/ 平级，而不是被塞进某个档案目录。
func TestSettingsPath_落在data根下与profiles平级(t *testing.T) {
	s := New("/data")
	got := s.SettingsPath()
	want := filepath.Join("/data", "settings.json")
	if got != want {
		t.Errorf("路径不对：%q，期望 %q", got, want)
	}
	if strings.Contains(got, "profiles") {
		t.Errorf("站级设置不许落进 profiles/ 下：%q", got)
	}
}
