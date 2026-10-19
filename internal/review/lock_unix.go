//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package review

import (
	"os"
	"syscall"
)

// lockDir takes an exclusive advisory lock on the folder that holds a record. Two reviewers who
// edit records in the same folder then take turns, so one rename cannot undo the other. The
// returned function releases the lock.
func lockDir(dir string) (func(), error) {
	f, err := os.Open(dir) //nolint:gosec // the folder of a record the user named
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil { //nolint:gosec // a file descriptor fits in an int
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:gosec // as above
		_ = f.Close()
	}, nil
}
