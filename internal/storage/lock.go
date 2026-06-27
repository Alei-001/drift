package storage

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrLockTimeout is returned when the file lock cannot be acquired within
// lockTimeout. This prevents drift commands from hanging forever if another
// drift process dies while holding the lock.
var ErrLockTimeout = errors.New("timed out waiting for drift lock (another process may be stuck)")

// lockTimeout is the maximum time to wait for a file lock. P3-#19.
const lockTimeout = 5 * time.Second

type fileLock struct {
	file *os.File
}

func acquireFileLock(lockPath string) (*fileLock, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	// Try non-blocking first; if it fails, poll with a deadline.
	if err := platformTryLock(f); err == nil {
		writeLockPID(f)
		return &fileLock{file: f}, nil
	} else if !isLockBusy(err) {
		// Non-busy errors (e.g. permission denied) should not be masked
		// by the polling loop — return immediately.
		f.Close()
		return nil, err
	}

	// Check for a stale lock before waiting. If the PID recorded in the
	// lock file belongs to a process that no longer exists, the lock is
	// stale (the process crashed without releasing). On Unix the OS lock
	// is released automatically on process death, but on Windows a
	// crashed process may leave a dangling lock file — reporting the PID
	// helps the user diagnose.
	deadline := time.Now().Add(lockTimeout)
	var holderPID int
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if err := platformTryLock(f); err == nil {
			writeLockPID(f)
			return &fileLock{file: f}, nil
		} else if !isLockBusy(err) {
			f.Close()
			return nil, err
		}
		holderPID = readLockPID(f)
		if holderPID > 0 && !pidExists(holderPID) {
			clearLockPID(f)
			if err := platformTryLock(f); err == nil {
				writeLockPID(f)
				return &fileLock{file: f}, nil
			}
		}
	}

	f.Close()
	if holderPID > 0 {
		return nil, fmt.Errorf("%w (held by PID %d)", ErrLockTimeout, holderPID)
	}
	return nil, ErrLockTimeout
}

// pidExists reports whether a process with the given PID is still running.
// On Unix this uses signal 0; on Windows it is best-effort (returns true
// since the OS releases LockFileEx on process exit).
func pidExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func (fl *fileLock) release() {
	platformUnlock(fl.file)
	// Clear the PID we wrote so a future reader doesn't see a stale value.
	clearLockPID(fl.file)
	fl.file.Close()
}

// writeLockPID records the current process ID in the lock file so other
// processes can report which one holds the lock.
func writeLockPID(f *os.File) {
	f.Truncate(0)
	f.Seek(0, 0)
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Sync()
}

// readLockPID reads the PID recorded in the lock file, or 0 if unreadable.
func readLockPID(f *os.File) int {
	f.Seek(0, 0)
	buf := make([]byte, 32)
	n, _ := f.Read(buf)
	if n == 0 {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		return 0
	}
	return pid
}

// clearLockPID erases the PID from the lock file on release.
func clearLockPID(f *os.File) {
	f.Truncate(0)
	f.Seek(0, 0)
}
