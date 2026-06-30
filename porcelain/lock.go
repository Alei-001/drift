package porcelain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// workspaceLockData is the JSON payload stored in the workspace lock file.
type workspaceLockData struct {
	PID       int   `json:"pid"`
	Timestamp int64 `json:"timestamp"`
}

// lockStaleTimeout is how long a lock may live before being considered stale.
const lockStaleTimeout = 600 * time.Second

// ErrLocked is returned by AcquireWorkspaceLock when the workspace lock is
// held by another live operation. Callers may test for it with errors.Is.
var ErrLocked = errors.New("workspace is locked by another operation")

// AcquireWorkspaceLock creates a workspace lock file at .drift/workspace.lock.
// It coordinates access between workspace-modifying commands (switch, restore)
// and the watch daemon so the daemon does not observe an inconsistent state
// (files rewritten but index not yet rebuilt) during a transition.
//
// Acquisition is race-free: the lock file is created atomically with
// O_CREATE|O_EXCL, so two processes can never both create the lock and then
// each believe they hold it. If a stale lock exists (older than
// lockStaleTimeout or whose PID is no longer alive) it is removed and
// acquisition is retried once. A lock that cannot be parsed (e.g. an empty
// file being written concurrently) is treated as held rather than stale, so a
// writer is never clobbered mid-write.
func AcquireWorkspaceLock(cwd string) error {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return fmt.Errorf("create drift dir: %w", err)
	}

	lock := workspaceLockData{
		PID:       os.Getpid(),
		Timestamp: time.Now().Unix(),
	}
	data, _ := json.Marshal(&lock)

	// Attempt 1: atomic create.
	if err := createLockFile(lockPath, data); err == nil {
		return nil
	} else if !errors.Is(err, ErrLocked) {
		return err
	}

	// The lock exists. Inspect it to decide whether it can be stolen.
	existing, err := readWorkspaceLock(lockPath)
	if err != nil {
		return err
	}
	if !isLockStale(existing) {
		return fmt.Errorf("workspace is locked by another operation (PID %d): %w", existing.PID, ErrLocked)
	}

	// Stale lock: remove and retry the atomic create once.
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale workspace lock: %w", err)
	}
	if err := createLockFile(lockPath, data); err != nil {
		return err
	}
	return nil
}

// createLockFile atomically creates the lock file with O_CREATE|O_EXCL and
// writes data to it. It returns ErrLocked if the file already exists.
func createLockFile(lockPath string, data []byte) error {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return ErrLocked
		}
		return fmt.Errorf("create workspace lock: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(lockPath)
		return fmt.Errorf("write workspace lock: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(lockPath)
		return fmt.Errorf("sync workspace lock: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(lockPath)
		return fmt.Errorf("close workspace lock: %w", err)
	}
	return nil
}

// readWorkspaceLock reads and parses the lock file. It returns ErrLocked
// whenever the lock cannot be proven stale — i.e. the file is missing, empty,
// or unparseable — so that the caller does not remove a lock that may be
// mid-write.
func readWorkspaceLock(lockPath string) (*workspaceLockData, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("read workspace lock: %w", err)
	}
	var lock workspaceLockData
	if json.Unmarshal(data, &lock) != nil {
		// Empty or partially-written file: do not treat as stale.
		return nil, ErrLocked
	}
	return &lock, nil
}

// ReleaseWorkspaceLock removes the workspace lock file.
func ReleaseWorkspaceLock(cwd string) {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")
	os.Remove(lockPath)
}

// IsWorkspaceLocked returns true if a valid (non-stale) workspace lock exists.
// Stale locks are cleaned up as a side effect. A lock that cannot be parsed
// (e.g. an empty file being written concurrently) is treated as held rather
// than removed, to avoid clobbering a writer mid-write.
func IsWorkspaceLocked(cwd string) bool {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	var lock workspaceLockData
	if json.Unmarshal(data, &lock) != nil {
		// Likely a half-written file; do not remove it.
		return true
	}
	if isLockStale(&lock) {
		os.Remove(lockPath)
		return false
	}
	return true
}

func isLockStale(lock *workspaceLockData) bool {
	if time.Since(time.Unix(lock.Timestamp, 0)) > lockStaleTimeout {
		return true
	}
	if lock.PID > 0 && !processExists(lock.PID) {
		return true
	}
	return false
}
