package porcelain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Alei-001/drift/internal/util/fsutil"
)

// workspaceLockData is the JSON payload stored in the workspace lock file.
type workspaceLockData struct {
	PID       int    `json:"pid"`
	Timestamp int64  `json:"timestamp"`
	StartTime int64  `json:"start_time,omitempty"`
}

// lockStaleTimeout is how long a lock may live before being considered stale.
//
// A long-running operation that exceeds this timeout will have its lock
// considered stale by another process, which may then steal it and start a
// concurrent workspace-modifying operation. Callers that may exceed the
// timeout should call TouchWorkspaceLock periodically to refresh the
// timestamp.
const lockStaleTimeout = 600 * time.Second

// acquireLockMu serializes stale-lock replacement within a single process.
// Without it, two goroutines that both observe the same stale lock would race
// on the rename-replace step: both would succeed, and the loser would silently
// own a lock file whose content no longer matches its in-memory data.
// Cross-process races are not serialized by this mutex; they are mitigated by
// the small window between the stale check and the rename.
var acquireLockMu sync.Mutex

// AcquireWorkspaceLock creates a workspace lock file at .drift/workspace.lock.
// It coordinates access between workspace-modifying commands (switch, restore)
// and the watch daemon so the daemon does not observe an inconsistent state
// (files rewritten but index not yet rebuilt) during a transition.
//
// This lock is NOT reentrant. Calling AcquireWorkspaceLock from within a
// function that already holds the lock will return ErrLocked. Use the NoLock
// variants for internal calls.
//
// Acquisition is race-free: the lock file is created atomically with
// O_CREATE|O_EXCL, so two processes can never both create the lock and then
// each believe they hold it. If a stale lock exists (older than
// lockStaleTimeout or whose PID is no longer alive) it is replaced atomically
// via a temp-file + os.Rename, eliminating the remove→create window present
// in the previous remove-then-create approach where another process's freshly
// acquired lock could be accidentally deleted by our os.Remove before our
// O_EXCL create ran. A lock that cannot be parsed (e.g. an empty file being
// written concurrently) is treated as held rather than stale, so a writer is
// never clobbered mid-write.
func AcquireWorkspaceLock(cwd string) error {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), fsutil.DefaultDirPerm); err != nil {
		return fmt.Errorf("create drift dir: %w", err)
	}

	lock := workspaceLockData{
		PID:       os.Getpid(),
		Timestamp: time.Now().Unix(),
		StartTime: currentProcessStartTime(),
	}
	data, err := json.Marshal(&lock)
	if err != nil {
		return fmt.Errorf("marshal lock data: %w", err)
	}

	// Attempt 1: atomic O_EXCL create. Handles the "no lock exists" case
	// race-free — two callers can never both observe "the file does not
	// exist" and then each create it.
	if err := createLockFile(lockPath, data); err == nil {
		return nil
	} else if !errors.Is(err, ErrLocked) {
		return err
	}

	// The lock exists. Serialize stale-lock replacement within this process
	// so two goroutines do not both rename over the same stale lock.
	acquireLockMu.Lock()
	defer acquireLockMu.Unlock()

	existing, err := readWorkspaceLock(lockPath)
	if err != nil {
		return err
	}
	if !isLockStale(existing) {
		return fmt.Errorf("workspace is locked by another operation (PID %d): %w", existing.PID, ErrLocked)
	}

	// Stale lock: atomically replace it via temp-file + rename. This
	// eliminates the remove→create window where another process's freshly
	// acquired lock could be accidentally deleted by our os.Remove.
	if err := fsutil.WriteFileAtomic(lockPath, data, fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("replace stale workspace lock: %w", err)
	}
	return nil
}

// createLockFile atomically creates the lock file with O_CREATE|O_EXCL and
// writes data to it. It returns ErrLocked if the file already exists.
func createLockFile(lockPath string, data []byte) error {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fsutil.DefaultFilePerm)
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
// or unparseable — so that the caller does not replace a lock that may be
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

// ReleaseWorkspaceLock removes the workspace lock file, but only if it is
// owned by the current process. This prevents a process from clobbering a
// lock that another operation has acquired after this process stopped using
// it (e.g. a stale lock that was stolen and refreshed between acquire and
// release). The error is intentionally ignored: if the lock is already gone
// or is no longer owned by us there is nothing useful for callers to do, and
// a leftover lock will be reclaimed by the stale-timeout in
// AcquireWorkspaceLock.
func ReleaseWorkspaceLock(cwd string) {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")
	removeLockIfOwned(lockPath, os.Getpid())
}

// removeLockIfOwned removes the workspace lock file only if it was created by
// the given PID. This prevents clobbering a lock that another command has
// acquired after the recorded owner stopped using it. It is shared by
// ReleaseWorkspaceLock (which passes the current PID) and the watch daemon's
// shutdown path (which passes the daemon's PID).
func removeLockIfOwned(lockPath string, pid int) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return
	}
	var lock workspaceLockData
	if json.Unmarshal(data, &lock) != nil {
		return
	}
	if lock.PID == pid {
		os.Remove(lockPath)
	}
}

// TouchWorkspaceLock refreshes the workspace lock's timestamp to prevent it
// from being considered stale during a long-running operation. It should be
// called periodically by operations that may exceed lockStaleTimeout (e.g.
// snapshotting a very large workspace).
//
// TouchWorkspaceLock is best-effort: if the lock has been removed, stolen by
// another process, or cannot be parsed, it returns nil without error. The
// caller is not expected to act on a failed touch — a stale lock will
// eventually be reclaimed by the timeout in AcquireWorkspaceLock.
func TouchWorkspaceLock(workDir string) error {
	lockPath := filepath.Join(workDir, ".drift", "workspace.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read workspace lock: %w", err)
	}
	var lock workspaceLockData
	if json.Unmarshal(data, &lock) != nil {
		return nil
	}
	if lock.PID != os.Getpid() {
		return nil
	}
	lock.Timestamp = time.Now().Unix()
	newData, err := json.Marshal(&lock)
	if err != nil {
		return fmt.Errorf("marshal lock data: %w", err)
	}
	if err := fsutil.WriteFileAtomic(lockPath, newData, fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("rewrite workspace lock: %w", err)
	}
	return nil
}

func isLockStale(lock *workspaceLockData) bool {
	if time.Since(time.Unix(lock.Timestamp, 0)) > lockStaleTimeout {
		return true
	}
	if lock.PID > 0 && !processExistsWithStartTime(lock.PID, lock.StartTime) {
		return true
	}
	return false
}
