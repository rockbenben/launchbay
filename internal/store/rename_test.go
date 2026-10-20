package store

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
)

// 「目标正被别的进程占着」那几种错要认得出，别的错不能认。
//
// 认错了的后果是两边都难受：认少了 → 本机（Windows + 火绒）每轮随机红一两条
// 与改动无关的测试；认多了 → 磁盘满、只读文件系统这些真故障也被拖着重试
// 250ms 才报出来。
func TestRenameBusy_只认被占着那几种错(t *testing.T) {
	// 5 = ERROR_ACCESS_DENIED、32 = ERROR_SHARING_VIOLATION、33 = ERROR_LOCK_VIOLATION
	for _, code := range []uintptr{5, 32, 33} {
		if !isBusyWindowsErr(syscall.Errno(code)) {
			t.Errorf("错误码 %d 应当算「目标被占着」", code)
		}
	}
	// 别的错不算：2 = ERROR_FILE_NOT_FOUND、13 = ERROR_INVALID_DATA（Linux 的 EACCES 恰好也是 13，
	// 而在 Linux 上 rename 失败就不是"被占着"，所以那个数不能算）。
	for _, code := range []uintptr{1, 2, 13, 1117} {
		if isBusyWindowsErr(syscall.Errno(code)) {
			t.Errorf("错误码 %d 不该算「目标被占着」：重试它没有意义", code)
		}
	}
	// 根本不是 errno 的错（自己定义的那个）不算。
	if isBusyWindowsErr(errors.New("boom")) {
		t.Error("不是 errno 的错不该算「目标被占着」")
	}
	// **包在别处的也要认得出**：os.Rename 交回来的是 *os.LinkError，它 Unwrap 到 errno。
	le := &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.Errno(5)}
	if !isBusyWindowsErr(le) {
		t.Error("包在 os.LinkError 里的「被占着」没认出来——而那正是 os.Rename 的形状")
	}
	// 再包一层（写盘那一步还会 fmt.Errorf %w 一次）也要认得出。
	wrapped := fmt.Errorf("rename 到 x: %w", le)
	if !isBusyWindowsErr(wrapped) {
		t.Error("包了两层的「被占着」没认出来")
	}
}

// 重试只发生在**被占着**这一种错上：别的错第一次就该报出来（不能被拖着
// 重试 250ms 才好让人看见）。
//
// **Windows 上有一处分不开**（2026-10-20 实测）："目标是个目录"与"目标正被
// 占着"**都是 ERROR_ACCESS_DENIED**。所以那种情形会白等 250ms 才报出来——
// 代价只是慢一点，答案仍然是真的（同一个错原样返回）。而这一条**不能**
// 拿来当"只重试被占着"的证据，所以这里用另一个错来钉。
func TestRenameWithRetry_别的错原样报出(t *testing.T) {
	dir := t.TempDir()
	// 源文件不存在 → ERROR_FILE_NOT_FOUND（2），那个数**不在**"被占着"里。
	src := dir + "/not-there"
	target := dir + "/target"
	err := renameWithRetry(src, target)
	if err == nil {
		t.Fatal("rename 一个不存在的源文件该失败")
	}
	if isBusyWindowsErr(err) {
		t.Errorf("这个错不该被当成「被占着」：%v", err)
	}
}
