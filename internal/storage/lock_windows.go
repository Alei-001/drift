//go:build windows

package storage

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const (
	LOCKFILE_EXCLUSIVE_LOCK   = 0x00000002
	LOCKFILE_FAIL_IMMEDIATELY = 0x00000001
)

// ERROR_LOCK_VIOLATION is the Windows error code returned when a file lock
// cannot be acquired because it is held by another process.
const ERROR_LOCK_VIOLATION syscall.Errno = 0x21

type overlapped struct {
	Internal     uintptr
	InternalHigh uintptr
	Offset       uint32
	OffsetHigh   uint32
	HEvent       uintptr
}

// platformTryLock attempts a non-blocking exclusive lock. Returns nil on
// success, a non-nil error if the lock is held by another process.
func platformTryLock(f *os.File) error {
	handle := syscall.Handle(f.Fd())
	var o overlapped

	r1, _, err := procLockFileEx.Call(
		uintptr(handle),
		uintptr(LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&o)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}

func platformUnlock(f *os.File) error {
	handle := syscall.Handle(f.Fd())
	var o overlapped

	r1, _, err := procUnlockFileEx.Call(
		uintptr(handle),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&o)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}

// isLockBusy reports whether err indicates the lock is currently held by
// another process (a transient condition worth retrying).
func isLockBusy(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == ERROR_LOCK_VIOLATION
	}
	return false
}
