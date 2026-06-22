package storage

import (
	"errors"
	"os"
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
		return &fileLock{file: f}, nil
	}

	deadline := time.Now().Add(lockTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if err := platformTryLock(f); err == nil {
			return &fileLock{file: f}, nil
		}
	}

	f.Close()
	return nil, ErrLockTimeout
}

func (fl *fileLock) release() {
	platformUnlock(fl.file)
	fl.file.Close()
}
