package porcelain

import (
	"encoding/json"
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
const lockStaleTimeout = 60 // seconds

// AcquireWorkspaceLock creates a workspace lock file at .drift/workspace.lock.
// It coordinates access between workspace-modifying commands (switch, restore)
// and the watch daemon so the daemon does not observe an inconsistent state
// (files rewritten but index not yet rebuilt) during a transition.
//
// If a stale lock exists (older than lockStaleTimeout or whose PID is no
// longer alive) it is removed and a new lock is acquired.
func AcquireWorkspaceLock(cwd string) error {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")

	if data, err := os.ReadFile(lockPath); err == nil {
		var lock workspaceLockData
		if json.Unmarshal(data, &lock) == nil {
			if isLockStale(&lock) {
				os.Remove(lockPath)
			} else {
				return fmt.Errorf("workspace is locked by another operation (PID %d)", lock.PID)
			}
		} else {
			os.Remove(lockPath)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read workspace lock: %w", err)
	}

	lock := workspaceLockData{
		PID:       os.Getpid(),
		Timestamp: time.Now().Unix(),
	}
	data, _ := json.Marshal(&lock)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return fmt.Errorf("create drift dir: %w", err)
	}
	if err := os.WriteFile(lockPath, data, 0644); err != nil {
		return fmt.Errorf("write workspace lock: %w", err)
	}
	return nil
}

// ReleaseWorkspaceLock removes the workspace lock file.
func ReleaseWorkspaceLock(cwd string) {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")
	os.Remove(lockPath)
}

// IsWorkspaceLocked returns true if a valid (non-stale) workspace lock exists.
// Stale locks are cleaned up as a side effect.
func IsWorkspaceLocked(cwd string) bool {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	var lock workspaceLockData
	if json.Unmarshal(data, &lock) != nil {
		os.Remove(lockPath)
		return false
	}
	if isLockStale(&lock) {
		os.Remove(lockPath)
		return false
	}
	return true
}

func isLockStale(lock *workspaceLockData) bool {
	if time.Now().Unix()-lock.Timestamp > lockStaleTimeout {
		return true
	}
	if lock.PID > 0 && !processExists(lock.PID) {
		return true
	}
	return false
}
