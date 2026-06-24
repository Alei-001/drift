//go:build !windows

package storage

import (
	"errors"
	"os"
	"syscall"
)

// platformTryLock attempts a non-blocking exclusive lock. Returns nil on
// success, a non-nil error if the lock is held by another process.
func platformTryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func platformUnlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// isLockBusy reports whether err indicates the lock is currently held by
// another process (a transient condition worth retrying).
func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}
