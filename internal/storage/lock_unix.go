//go:build !windows

package storage

import (
	"os"
	"syscall"
)

func platformLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// platformTryLock attempts a non-blocking exclusive lock. Returns nil on
// success, a non-nil error if the lock is held by another process.
func platformTryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func platformUnlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
