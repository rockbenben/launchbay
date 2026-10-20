package store

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"time"
)

// renameRetryTimes / renameRetryDelay 是「目标正被别的进程占着」时的重试参数
// （2026-10-20）。
//
// 次数 × 间隔 = 最长 250ms 的等待。一把杀软/索引器的锁通常只持续几毫秒
// （它扫完就放），而 250ms 对一次写盘来说不算什么——而它换来的是
// 一次"刚才失败了、其实是被人占了一下"不至于把整次写入报成失败。
const (
	renameRetryTimes = 10
	renameRetryDelay = 25 * time.Millisecond
)

// windowsRenameBusy 是「目标被占着」那几种 Windows 错误码：
// 5 = ERROR_ACCESS_DENIED、32 = ERROR_SHARING_VIOLATION、33 = ERROR_LOCK_VIOLATION。
//
// **写死数字而不用 syscall.ERROR_\***：那几个常量只在 Windows 上存在，而本文件
// 要在 Linux（容器）上也编得过（项目两边都跑）。Win32 的错误码是 ABI，不会变。
var windowsRenameBusy = map[uintptr]bool{5: true, 32: true, 33: true}

// renameWithRetry 是 os.Rename 加一次**只在 Windows 上**的重试。
//
// **为什么需要它**（2026-10-20）：`os.Rename` 走的是 MoveFileEx +
// MOVEFILE_REPLACE_EXISTING，而**目标文件正被别的进程打开着**时
// （杀软扫描器、搜索索引、备份程序——本机有火绒）它会答 ERROR_ACCESS_DENIED
// （"Access is denied"）。那把锁是**瞬时**的：扫描器看完就放，所以重试几次
// 就能过。
//
// 症状最难看的地方是**它随机**：同一份代码、同一台机器、同一批测试，有时红
// 有时绿，而失败的那支与当轮改动毫无关系。本机上它每轮随机红一两条
// （internal/store 的快照裁剪与并发写、internal/httpapi 的导入与历史那几支、
// internal/render 的面板那支），于是"是不是这一笔改动弄坏的"这个问题会被
// 反复问起——而它不是。（实测：在改动之前的提交上用 worktree 复现过，形状
// 一模一样。）
//
// **只在这一种错上重试**：别的错（磁盘满、只读、目标是个目录）重试没有意义，
// 而它们各有各的答案（第一次失败就带着原样返回）。**Linux 上完全不改行为**
// （POSIX 的 rename 是原子的，不会因为"目标正开着"而失败）。
func renameWithRetry(oldPath, newPath string) error {
	err := os.Rename(oldPath, newPath)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	if !isBusyWindowsErr(err) {
		return err
	}
	for i := 0; i < renameRetryTimes; i++ {
		time.Sleep(renameRetryDelay)
		err = os.Rename(oldPath, newPath)
		if err == nil {
			return nil
		}
		if !isBusyWindowsErr(err) {
			return err
		}
	}
	return err
}

// isBusyWindowsErr 答"这个错是不是目标被占着"（而不是真的没权限）。
//
// 它是**纯函数**（不看 GOOS）：Windows 上那个重试用它，别处只是拿它做断言。
func isBusyWindowsErr(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return windowsRenameBusy[uintptr(errno)]
}
