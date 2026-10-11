package main

import (
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeOKOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if err := probe(srv.URL); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
}

func TestProbeFailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	if err := probe(srv.URL); err == nil {
		t.Fatal("want error on 500")
	}
}

func TestProbeFailsOnUnreachable(t *testing.T) {
	// 一个没人听的端口。1 秒超时要生效，否则容器会卡在 healthcheck 里。
	if err := probe("http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("want error")
	}
}

// portOf 从 httptest URL 里取端口，喂给 runHealthcheck。
func portOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// 评审 Important #9：探活必须碰磁盘。进程活着 + 数据目录可写才是 healthy；
// 只读挂载下 /healthz 照样 200，光探 HTTP 会把永远写不进磁盘的容器判成健康。
func TestHealthcheckFailsWhenDataDirNotWritable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	port := portOf(t, srv.URL)

	// 挂载失败/权限挡创建的最常见形状：目录根本不存在。
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	if code := runHealthcheck(port, missing); code != 1 {
		t.Fatalf("数据目录不可写要判死, got %d", code)
	}
	// 数据目录被一个文件占了位，同样不许判活。
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runHealthcheck(port, file); code != 1 {
		t.Fatalf("数据目录是文件要判死, got %d", code)
	}
}

func TestHealthcheckOKWhenAliveAndWritable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	dir := t.TempDir()
	if code := runHealthcheck(portOf(t, srv.URL), dir); code != 0 {
		t.Fatalf("want 0, got %d", code)
	}
	// 探针自己的临时文件必须删净：每 30 秒跑一次，漏一次就赖在数据目录里。
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Fatalf("探活残留临时文件: %v", ents)
	}
}

// HTTP 活着但不是 200 时磁盘再能写也没用——两条判据是 AND。
func TestHealthcheckStillFailsWhenServerDown(t *testing.T) {
	dir := t.TempDir()
	if code := runHealthcheck("1", dir); code != 1 {
		t.Fatalf("服务没起来要判死, got %d", code)
	}
}

// 不许只说"不可写"。看这件事的人手里只有 docker inspect 的 health log，
// 句子里没有 chown 就等于让他自己猜是哪一侧的权限。
func TestCheckWritable_那句话教得出下一步(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	err := checkWritable(missing)
	if err == nil {
		t.Fatal("目录不存在要报错")
	}
	say := err.Error()
	for _, want := range []string{"chown", "65534", "宿主机"} {
		if !strings.Contains(say, want) {
			t.Errorf("那句话里没有 %q，读到的是 %q", want, say)
		}
	}
}

// uid 65534 在两个文件里各写一遍（Dockerfile 的 USER 与 healthcheck 的那句话），
// 原则上就会分家——改镜像那个不记得改这句话，这容器就教人跑一条无效的 chown。
// 这条断言把它们钉在一起。
//
// 镜像构建里 Dockerfile 不在上下文内（.dockerignore 把它排掉了，与 tools/ 同理），
// 那里跳过而不是红：这条断言的对象在那个环境里**不存在**，为它改 .dockerignore
// 是把镜像构建上下文换成给测试读文件。仓库与 CI 里它是实跑的。
func Test提示里的uid与镜像那个USER一致(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("这个构建上下文里没有 Dockerfile（镜像构建按 .dockerignore 排掉了它）")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "USER 65534:65534") {
		t.Fatal("Dockerfile 里没有 USER 65534:65534 了：要么是镜像用户变了，" +
			"要么是这一步被改坏——先弄清是哪一种，再改提示里那个数")
	}
	if say := checkWritable(filepath.Join(t.TempDir(), "no-such-dir")).Error(); !strings.Contains(say, "65534") {
		t.Errorf("提示里的 uid 与镜像对不上: %q", say)
	}
}
