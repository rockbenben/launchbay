//go:build unix

package uicheck

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureBrowserProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopBrowser(process *os.Process, done <-chan struct{}) {
	// Chrome 处理 SIGTERM，先让它关完子进程和档案文件，再由 Go 清理目录。
	_ = process.Signal(syscall.SIGTERM)
	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
		// 卡住时收掉独立进程组，避免仅杀父进程后子进程还在写盘。
		_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
		<-done
	}
}
