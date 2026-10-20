//go:build windows

package review

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// lockExclusive is LOCKFILE_EXCLUSIVE_LOCK. Without LOCKFILE_FAIL_IMMEDIATELY the call waits.
const lockExclusive = 2

// lockDir takes an exclusive lock on a file named .review.lock in the folder that holds a record.
// Windows cannot lock a folder, so the lock file stays in the folder. Two reviewers who edit
// records in that folder take turns, and the returned function releases the lock.
func lockDir(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, ".review.lock"), os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // the folder of a record the user named
	if err != nil {
		return nil, err
	}
	var ol syscall.Overlapped
	h := f.Fd()
	if r, _, err := procLockFileEx.Call(h, lockExclusive, 0, 1, 0, uintptr(unsafe.Pointer(&ol))); r == 0 { //nolint:gosec // the Windows API takes a pointer
		_ = f.Close()
		return nil, err
	}
	return func() {
		var ol syscall.Overlapped
		_, _, _ = procUnlockFileEx.Call(h, 0, 1, 0, uintptr(unsafe.Pointer(&ol))) //nolint:gosec // as above
		_ = f.Close()
	}, nil
}
