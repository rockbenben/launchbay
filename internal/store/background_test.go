package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
)

// 这一页自己的背景图（2026-10-14）。这些钉守的是**三条会静默出错的判据**：
// 体积上限、落盘形状、以及"按档案分目录"（它换来的那条是"各删各的"）。

// onePixelPNG 造**一张真 PNG**（用标准库编，不手敲字节）：判据是**魔数**，
// 而手敲一串字节的夹具会在某次编辑里少一个引号——那正是这份测试要抓的形状。
func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func bgProfile(t *testing.T, s *Store, id string) *Store {
	t.Helper()
	if _, err := s.CreateProfile(NewProfileInput{ID: id, Title: id, PinHash: "sha256$a$b"}); err != nil {
		t.Fatalf("建 %s: %v", id, err)
	}
	return s
}

func TestBackground_按内容哈希命名且落盘(t *testing.T) {
	s := New(t.TempDir())
	bgProfile(t, s, "dad")

	name, err := s.SaveBackground("dad", onePixelPNG(t), "png")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(onePixelPNG(t))
	want := hex.EncodeToString(sum[:])[:16] + ".png"
	if name != want {
		t.Fatalf("名字 = %q，要 %q（内容寻址：换一张必然换一个 URL）", name, want)
	}
	got, err := os.ReadFile(s.BackgroundPath("dad", name))
	if err != nil || !bytes.Equal(got, onePixelPNG(t)) {
		t.Fatalf("落盘的字节不是一个字节不差：%v", err)
	}
	// 再存一次同样的字节：同一个文件名，不会攒出两份。
	if again, err := s.SaveBackground("dad", onePixelPNG(t), "png"); err != nil || again != name {
		t.Fatalf("同样的一张图存两次得到 %q（%v），要还是 %q", again, err, name)
	}
}

func TestBackground_体积上限与后缀闸(t *testing.T) {
	s := New(t.TempDir())
	bgProfile(t, s, "dad")

	if _, err := s.SaveBackground("dad", nil, "png"); err == nil {
		t.Error("零字节要被拒")
	}
	if _, err := s.SaveBackground("dad", []byte(strings.Repeat("x", MaxBackgroundBytes+1)), "png"); err == nil {
		t.Errorf("超过 %d 字节要被拒（NAS 上解一张 12 MP 的照片够人以为页面坏了）", MaxBackgroundBytes)
	}
	if _, err := s.SaveBackground("dad", onePixelPNG(t), "svg"); err == nil {
		t.Error("svg 要被拒：背景是铺满整屏的一块位图，而 svg 是一整类执行面")
	}
	if _, err := s.SaveBackground("dad", onePixelPNG(t), "exe"); err == nil {
		t.Error("表外的后缀要被拒")
	}
	if _, err := s.SaveBackground("../escape", onePixelPNG(t), "png"); err == nil {
		t.Error("档案 id 不是 slug 时要被拒：那是目录名")
	}
}

func TestBackground_按档案分目录各删各的(t *testing.T) {
	// 两个人用**同一张图**：删掉 A 的那份，B 的必须还在。
	// 单份内容寻址的存储会让 A 删掉之后 B 的背景跟着消失，而"还有谁在用"
	// 这个问题的答案会变——所以存储按档案分目录。
	s := New(t.TempDir())
	bgProfile(t, s, "dad")
	bgProfile(t, s, "mom")

	name, err := s.SaveBackground("dad", onePixelPNG(t), "png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveBackground("mom", onePixelPNG(t), "png"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveBackground("dad", name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.BackgroundPath("dad", name)); !os.IsNotExist(err) {
		t.Error("A 的那份该没了")
	}
	if _, err := s.ReadBackground("mom", name); err != nil {
		t.Fatalf("B 的那份还在吗：%v", err)
	}
}

func TestBackground_读与删的形状闸(t *testing.T) {
	s := New(t.TempDir())
	bgProfile(t, s, "dad")

	for _, bad := range []string{"", "../../etc/passwd", "index.json", "0123456789abcde.png", "0123456789ABCDEF.png"} {
		if _, err := s.ReadBackground("dad", bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("ReadBackground(%q) = %v，要 not found", bad, err)
		}
		if err := s.RemoveBackground("dad", bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("RemoveBackground(%q) = %v，要 invalid（形状非法不是「删掉了」）", bad, err)
		}
	}
	// 真 I/O 故障不许伪装成 not found：那会让上层回 404，而真相是这台机器坏了。
	if err := os.MkdirAll(s.BackgroundDir("dad")+"/x", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadBackground("dad", "0123456789abcdef.png"); err == nil {
		t.Error("读一个读不出来的文件要说得出错，而不是回空")
	}
}

func TestBackgroundExtFor_只认嗅探出来的那四种(t *testing.T) {
	for in, want := range map[string]string{
		"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif",
		"image/svg+xml": "", "text/html": "", "application/octet-stream": "",
	} {
		if got := BackgroundExtFor(in); got != want {
			t.Errorf("BackgroundExtFor(%q) = %q，要 %q", in, got, want)
		}
	}
}
