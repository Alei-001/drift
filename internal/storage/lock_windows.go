//go:build windows

package storage

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32    = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = modkernel32.NewProc("LockFileEx")
)

const (
	LOCKFILE_EXCLUSIVE_LOCK   = 0x00000002
	LOCKFILE_FAIL_IMMEDIATELY = 0x00000001
)

type overlapped struct {
	Internal     uintptr
	InternalHigh uintptr
	Offset       uint32
	OffsetHigh   uint32
	HEvent       uintptr
}

func platformLock(f *os.File) error {
	handle := syscall.Handle(f.Fd())
	var o overlapped

	r1, _, err := procLockFileEx.Call(
		uintptr(handle),
		uintptr(LOCKFILE_EXCLUSIVE_LOCK),
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

	r1, _, err := procLockFileEx.Call(
		uintptr(handle),
		0,
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
