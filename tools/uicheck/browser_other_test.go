//go:build !unix

package uicheck

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

func configureBrowserProcess(*exec.Cmd) {}

func stopBrowser(process *os.Process, done <-chan struct{}) {
	select {
	case <-done:
		return
	default:
	}
	if runtime.GOOS == "windows" {
		// 连同子进程一起结束，否则它们仍可能占用 Chrome 档案里的文件。
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(process.Pid)).Run()
	}
	_ = process.Kill()
	<-done
}
