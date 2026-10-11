package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/store"
)

// putIconFile 往 data/icons/ 直接放一个指定文件名的字节。夹具是读侧的：
// 本任务只读缓存，不碰写通路（spec §5 明写 data/icons/ 只由 resolve 内部写），
// 所以测试不假装这些文件是 IconCache.Put 落下去的。
func putIconFile(t *testing.T, s *store.Store, name string, data []byte) {
	t.Helper()
	dir := s.IconsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func putBlob(t *testing.T, s *store.Store, hash, ext string, data []byte) {
	t.Helper()
	putIconFile(t, s, hash+"."+ext, data)
}

func TestIconFile_成功端出字节与immutable(t *testing.T) {
	s, h := setup(t)
	putBlob(t, s, "0123456789abcdef", "png", []byte{0x89, 'P', 'N', 'G', 1})

	rec := get(h, "/icons/0123456789abcdef.png", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("得到 %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q，要 §7.1 那一行逐字一致", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q", got)
	}
	// 这条也在 /api/v1 前缀之外：no-store 中间件不许顺手盖掉 immutable。
	if strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Error("no-store 漏进了 /icons/*")
	}
	if string(rec.Body.Bytes()) != string([]byte{0x89, 'P', 'N', 'G', 1}) {
		t.Error("字节被改过：这个端点的契约就是原样端出")
	}
}

// 裁定 R9 的证人：favicon 是**第三方字节**，却由 launchbay 自己的 origin
// 端出。§5 那句"svg 只走 <img>"是客户端约定——谁把 /icons/<hash>.svg 直接
// 开进地址栏，它就是你自己的文档，脚本跑在这个 origin 上，而会话 cookie
// 也在这里。一条响应头把边界挪回服务端。
//
// 两种响应都查（svg 命中、形状对但文件不在）：这条头属于整条通路而不是某个
// 分支——handler 里删掉那一行，这里两条一起红。
func TestIconFile_第三方字节一律带sandbox(t *testing.T) {
	s, h := setup(t)
	putBlob(t, s, "0123456789abcdef", "svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`))
	for _, p := range []string{
		"/icons/0123456789abcdef.svg", // svg 是这套头唯一真正的靶子
		"/icons/fedcba9876543210.png", // 不存在也带：头在分支之前设
	} {
		rec := get(h, p, "")
		if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
			t.Errorf("%s（%d）的 Content-Security-Policy = %q，要 sandbox", p, rec.Code, got)
		}
	}
}

func TestIconFile_形状不对一律404不解释(t *testing.T) {
	s, h := setup(t)
	// 先把这些"名字不对"的文件真的造出来：不然 404 是"盘上没有这个文件"给的，
	// 而不是形状检查给的，正则就成了没法被证明的装饰。裁定 R10 的那条理由
	// 在这里同样成立——去掉形状检查，下面每一条（含缓存索引 index.json）都会
	// 被当图片 200 端出去，红在这一条上。
	for _, name := range []string{
		"0123456789abcde.png",   // 15 位
		"0123456789abcdef0.png", // 17 位
		"0123456789ABCDEF.png",  // 大写：哈希一律小写，别放宽
		"0123456789abcdef.exe",  // 后缀不在表内
		"index.json",            // 缓存索引不是图片
	} {
		putIconFile(t, s, name, []byte("x"))
	}
	for _, bad := range []string{
		"/icons/0123456789abcde.png",   // 15 位
		"/icons/0123456789abcdef0.png", // 17 位
		"/icons/0123456789ABCDEF.png",  // 大写
		"/icons/0123456789abcdef.exe",  // 后缀不在表内
		"/icons/index.json",            // 索引本身不是图片
		// 编码过的穿越：mux 的 {name} 只吃一段、路径在匹配之前先清洗，
		// 所以"存在即端出"的实现根本收不到带 '/' 的名字——这条是纵深防御，
		// 不是正则的证人（裁定 R10；正则的证人是上面那五条已存在的文件）。
		// 留着是因为它免费，而且换路由层时它会变成第一道。
		"/icons/..%2f..%2fdata%2fprofiles%2fdad%2fbookmarks.json",
	} {
		rec := get(h, bad, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s 要 404，得到 %d", bad, rec.Code)
		}
		// 404 不回显细节：这是图片通路，不是诊断通路。
	}
}

func TestIconFile_形状对但没这个文件(t *testing.T) {
	_, h := setup(t)
	rec := get(h, "/icons/ffffffffffffffff.png", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("得到 %d：不存在与形状非法对调用方是同一句话", rec.Code)
	}
}

// 形状对、读却失败（同名是个目录而不是图片）走 500 那条分支：证人有两半——
// 分支不是死码，而且 500 不替服务说话（磁盘路径与真实错误只进 stderr，响应
// 里恒为「内部错误」）。
func TestIconFile_读失败是500不回显磁盘路径(t *testing.T) {
	s, h := setup(t)
	if err := os.MkdirAll(filepath.Join(s.IconsDir(), "0123456789abcdef.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/icons/0123456789abcdef.png", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("得到 %d，要 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), s.Root()) {
		t.Errorf("磁盘路径漏进了响应：%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "内部错误") {
		t.Errorf("500 的 detail 要恒为「内部错误」，得到 %s", rec.Body.String())
	}
}

func TestIconFile_不公开目录清单(t *testing.T) {
	_, h := setup(t)
	for _, p := range []string{"/icons/", "/icons", "/icons/*"} {
		if rec := get(h, p, ""); rec.Code == http.StatusOK {
			t.Errorf("%q 竟然 200：缓存内容可枚举就等于给站级缓存开了目录", p)
		}
	}
}

// TestIconFile_五种后缀表驱动各端各的 是最终整枝评审 A2（T11 m-1 了结）：
// 此前五个后缀里只有 .png 被正向断言过 Content-Type——iconFileNameRE 与
// iconContentTypes 在同一文件内漏配一个后缀，会静默产出空 Content-Type
// （nosniff 反而让浏览器拒绝），而全套测试仍绿。这里按 iconContentTypes
// 表驱动：每个后缀各写一个 blob、各 GET 一次、各断逐字 Content-Type +
// immutable + sandbox 三件。表就是断言的来源：往表里加/删一项，这里立刻
// 要求同名 blob 的后缀分支被走一遍；正则与表漂开，对应那条红。
func TestIconFile_五种后缀表驱动各端各的(t *testing.T) {
	s, h := setup(t)
	i := 0
	for ext, wantCT := range iconContentTypes {
		i++
		if i > 5 {
			t.Fatalf("iconContentTypes 比五种多了：这张表与 iconFileNameRE、store.iconExts、fetch 的 media-type 分支是同一件事的四处表述，加一项要同步加证人")
		}
		name := fmt.Sprintf("%016x", i*4096) // 每后缀一个独立的 16 位小写十六进制名
		blob := []byte("blob-" + ext)
		putBlob(t, s, name, ext, blob)
		rec := get(h, "/icons/"+name+"."+ext, "")
		if rec.Code != http.StatusOK {
			t.Errorf(".%s 得到 %d：正则与类型表漂了（%s 被形状检查挡在门外）", ext, rec.Code, name)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != wantCT {
			t.Errorf(".%s 的 Content-Type = %q，要逐字 %q", ext, got, wantCT)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Errorf(".%s 的 Cache-Control = %q，要 immutable 那一行逐字一致", ext, got)
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
			t.Errorf(".%s 的 Content-Security-Policy = %q，要 sandbox（R9：第三方字节从本 origin 端出）", ext, got)
		}
		if rec.Body.String() != string(blob) {
			t.Errorf(".%s 的字节被改过", ext)
		}
	}
	if i != 5 {
		t.Errorf("iconContentTypes 只有 %d 项，五种后缀少了证人", i)
	}
}

// TestIconFile_两种404响应体逐字相同 把"同一句话"从注释变成断言（T11 m-4）：
// 形状非法（文件在盘上但正则拒）与形状对而文件不存在，两条 404 的响应体
// 必须逐字等值——回显"为什么没有"就是告诉探测者他猜对了哪一步。
func TestIconFile_两种404响应体逐字相同(t *testing.T) {
	s, h := setup(t)
	putIconFile(t, s, "0123456789abcdef.exe", []byte("x")) // 在盘上、形状非法
	shape := get(h, "/icons/0123456789abcdef.exe", "")
	missing := get(h, "/icons/ffffffffffffffff.png", "") // 形状对、盘上没有
	if shape.Code != http.StatusNotFound || missing.Code != http.StatusNotFound {
		t.Fatalf("两条都要 404，得到 %d 与 %d", shape.Code, missing.Code)
	}
	if shape.Body.String() != missing.Body.String() {
		t.Errorf("两种 404 不是一句话：形状非法=%s 文件不存在=%s", shape.Body, missing.Body)
	}
}
