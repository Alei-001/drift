package project

import (
	cryptoRand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/Alei-001/drift/internal/util/proc"
)

var (
	// ErrLocked is returned by AcquireWorkspaceLock when the workspace
	// lock is held by another live operation. Callers may test for it
	// with errors.Is.
	ErrLocked = errors.New("workspace is locked by another operation")

	// ErrLockLost is returned by TouchWorkspaceLock when the lock has
	// been stolen by another process during a long-running operation.
	// The caller MUST abort immediately: continuing to modify the store
	// while another process holds the lock will cause corruption.
	ErrLockLost = errors.New("workspace lock lost to another process")
)

// workspaceLockData is the JSON payload stored in the workspace lock file.
type workspaceLockData struct {
	PID       int   `json:"pid"`
	Timestamp int64 `json:"timestamp"`
	StartTime int64 `json:"start_time,omitempty"`
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
// on the claim-file step. Cross-process races are closed by the claim-file
// rename + post-rename PID verification: on POSIX, where rename silently
// overwrites, the loser of the race observes its PID is not the recorded
// owner and aborts; on Windows, rename fails if the target exists, so the
// loser retries and eventually re-checks staleness.
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
// Acquisition is race-free across processes:
//
//  1. Fast path: O_CREATE|O_EXCL atomic create. Two processes can never both
//     create the lock when none exists.
//  2. Stale-replacement path: the would-be acquirer writes a PID-embedded
//     claim file (workspace.lock.claim.<pid>.<rand>) using O_CREATE|O_EXCL,
//     re-checks that the lock is still stale, then atomically renames the
//     claim file onto the lock path. os.Rename is atomic on the same
//     filesystem, so exactly one of the contending processes wins the rename;
//     the loser's rename either fails (Windows) or silently overwrites with
//     its own content (POSIX), but the loser then observes its PID is not the
//     recorded owner and aborts. This closes the cross-process TOCTOU window
//     that the previous WriteFileAtomic-over-stale approach left open, where
//     two processes could both pass isLockStale and both believe they held
//     the lock after their respective temp-file+rename landed.
//
// A lock that cannot be parsed (e.g. an empty file being written concurrently)
// is treated as held rather than stale, so a writer is never clobbered
// mid-write.
func AcquireWorkspaceLock(cwd string) error {
	lockPath := filepath.Join(cwd, ".drift", "workspace.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), fsutil.DefaultDirPerm); err != nil {
		return fmt.Errorf("create drift dir: %w", err)
	}

	lock := workspaceLockData{
		PID:       os.Getpid(),
		Timestamp: time.Now().Unix(),
		StartTime: proc.CurrentProcessStartTime(),
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
	// so two goroutines do not both create claim files for the same lock.
	acquireLockMu.Lock()
	defer acquireLockMu.Unlock()

	existing, err := readWorkspaceLock(lockPath)
	if err != nil {
		return err
	}
	if !isLockStale(existing) {
		return fmt.Errorf("workspace is locked by another operation (PID %d): %w", existing.PID, ErrLocked)
	}

	// Stale lock: claim it atomically via a PID-embedded claim file + rename.
	// The claim file is created with O_CREATE|O_EXCL so concurrent acquirers
	// pick distinct claim file names. After writing our claim, re-check the
	// lock file: if another process already renamed its claim onto the lock,
	// we abort (the lock is no longer the stale one we observed). Finally
	// rename our claim onto the lock path. On POSIX rename is atomic and
	// overwrites; on Windows it fails if the target exists, so we fall back
	// to a remove+create retry loop bounded by claimMaxRetries.
	return claimStaleLock(lockPath, data)
}

// claimMaxRetries bounds the CAS-style retry loop used on platforms where
// os.Rename refuses to overwrite an existing file (Windows). Each retry
// re-checks staleness and attempts another claim, so the loop terminates
// either when the lock is successfully claimed or when another process has
// legitimately replaced it.
const claimMaxRetries = 8

// claimStaleLock performs the atomic claim of a lock file that was stale at
// the time of the caller's prior check. It writes a PID-embedded claim file,
// re-checks staleness, then renames the claim onto the lock path. If the
// rename fails because another acquirer won the race, the claim file is
// removed and the error is propagated so the caller surfaces ErrLocked.
func claimStaleLock(lockPath string, data []byte) error {
	dir := filepath.Dir(lockPath)
	for i := 0; i < claimMaxRetries; i++ {
		// Unique claim file name per attempt. crypto/rand keeps it
		// unpredictable across processes so two claimants cannot collide.
		claimPath, err := uniqueClaimPath(dir)
		if err != nil {
			return fmt.Errorf("generate lock claim name: %w", err)
		}
		if err := createLockFile(claimPath, data); err != nil {
			return fmt.Errorf("create lock claim: %w", err)
		}

		// Re-check the lock file: if it has been replaced by another
		// acquirer since our stale check, abandon this claim.
		existing, err := readWorkspaceLock(lockPath)
		if err != nil {
			os.Remove(claimPath)
			return err
		}
		if !isLockStale(existing) {
			os.Remove(claimPath)
			return fmt.Errorf("workspace is locked by another operation (PID %d): %w", existing.PID, ErrLocked)
		}

		// Atomically commit our claim. On POSIX rename(2) silently
		// overwrites the target, so two concurrent acquirers can both
		// see their Rename succeed — the loser's content is the one
		// that ends up on disk, but both believe they own the lock.
		// To close this TOCTOU we re-read the lock after rename and
		// verify our PID is the recorded owner. The loser detects the
		// mismatch and returns ErrLocked.
		//
		// On Windows, os.Rename fails when the target exists, so the
		// loser's Rename returns an error and falls through to retry.
		if err := os.Rename(claimPath, lockPath); err == nil {
			after, rerr := readWorkspaceLock(lockPath)
			if rerr != nil {
				// Lock file vanished between rename and re-read
				// (extremely unlikely). Treat as lost race.
				return fmt.Errorf("verify lock ownership after rename: %w: %w", rerr, ErrLocked)
			}
			if after.PID != os.Getpid() {
				return fmt.Errorf("lost lock race to PID %d: %w", after.PID, ErrLocked)
			}
			return nil
		}
		// Rename failed — either another acquirer replaced the lock
		// between our re-check and the rename, or the platform refuses
		// to overwrite. Clean up the claim and retry the whole loop.
		os.Remove(claimPath)
	}
	return fmt.Errorf("could not claim stale workspace lock after %d attempts: %w", claimMaxRetries, ErrLocked)
}

// uniqueClaimPath returns a unique path for a lock claim file in dir, of the
// form "workspace.lock.claim.<pid>.<rand>" where rand is 8 hex characters
// derived from crypto/rand.
func uniqueClaimPath(dir string) (string, error) {
	var buf [4]byte
	if _, err := cryptoRand.Read(buf[:]); err != nil {
		return "", err
	}
	name := fmt.Sprintf("workspace.lock.claim.%d.%x", os.Getpid(), buf[:])
	return filepath.Join(dir, name), nil
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
	RemoveLockIfOwned(lockPath, os.Getpid())
}

// removeLockIfOwned removes the workspace lock file only if it was created by
// the given PID. This prevents clobbering a lock that another command has
// acquired after the recorded owner stopped using it. It is shared by
// ReleaseWorkspaceLock (which passes the current PID) and the watch daemon's
// shutdown path (which passes the daemon's PID).
func RemoveLockIfOwned(lockPath string, pid int) {
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
// If the lock has been stolen by another process (PID mismatch),
// TouchWorkspaceLock returns ErrLockLost. The caller MUST abort the
// operation immediately — continuing to modify the store while another
// process holds the lock will cause corruption. A missing or unparseable
// lock file is treated as ErrLockLost as well, since the lock should not
// disappear while the operation holds it.
func TouchWorkspaceLock(workDir string) error {
	lockPath := filepath.Join(workDir, ".drift", "workspace.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace lock disappeared during operation: %w", ErrLockLost)
		}
		return fmt.Errorf("read workspace lock: %w", err)
	}
	var lock workspaceLockData
	if json.Unmarshal(data, &lock) != nil {
		return fmt.Errorf("workspace lock corrupted during operation: %w", ErrLockLost)
	}
	if lock.PID != os.Getpid() {
		return fmt.Errorf("workspace lock stolen by PID %d: %w", lock.PID, ErrLockLost)
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
	if lock.PID > 0 && !proc.ProcessExistsWithStartTime(lock.PID, lock.StartTime) {
		return true
	}
	return false
}
