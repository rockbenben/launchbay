package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// probe 只做一次 GET。scratch 镜像里没有 shell，Docker 的 healthcheck
// 必须靠二进制自己完成，所以这条路径要短、要快、要能超时退出。
func probe(url string) error {
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz 返回 %d", resp.StatusCode)
	}
	return nil
}

// checkWritable 断言数据目录真的能写：建临时文件再删掉。
// 为什么探活必须碰磁盘（评审 Important #9）：服务进程在只读 ./data 下
// 照样能起、/healthz 照样 200——Docker 会一直把这样的容器判成 healthy，
// 而它任何写入都会失败。scratch 以 USER 65534 跑、群晖把 ./data 挂成
// root 所属，正是这个形状。compose 注释里教人 chown 是一次性动作，
// 探针每次体检都复核，权限漂移当场变 unhealthy，不用等人去看 500。
//
// 不可写时那句话必须**自带出路**：`docker inspect` 里的 health log 是这件事
// 唯一的现场（人看到的是容器 unhealthy，不是一段栈），只说"permission denied"
// 等于让人自己猜是哪一侧的权限。uid 65534 与 Dockerfile 里的 USER 是同一个数，
// 两处各写一遍在原则上就会分家——由 cmd 里的
// Test提示里的uid与镜像那个USER一致 把它们钉在一起。
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".healthcheck-*")
	if err != nil {
		return fmt.Errorf("容器里的数据目录 %s 不可写：多半是宿主机上挂进去的那个目录不归"+
			"这个用户（容器以 uid 65534 跑，NAS 上把 ./data 挂成 root 所属正是这个形状）。"+
			"在宿主机上把它交给这个用户即可，例如 sudo chown -R 65534:65534 ./data"+
			"（目录由 compose 的 LAUNCHBAY_HOST_DATA 决定，默认就是 ./data），然后重启容器: %w",
			dir, err)
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("临时文件建成了却删不掉（%s）：目录可写但不可删，"+
			"多半是挂了个同步盘或被重新挂成只读；这跟上面那种权限不是一回事，"+
			"先确认容器里 %s 这条路径的挂载方式: %w", name, dir, err)
	}
	return nil
}

// runHealthcheck 是 `launchbay healthcheck` 的入口。
func runHealthcheck(port, dataDir string) int {
	if err := probe("http://127.0.0.1:" + port + "/healthz"); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck 失败: %v\n", err)
		return 1
	}
	if err := checkWritable(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck 失败: %v\n", err)
		return 1
	}
	return 0
}
