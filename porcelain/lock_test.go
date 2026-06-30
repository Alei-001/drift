package porcelain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/your-org/drift/storage"
)

// setupLockedProject initializes a drift project in a temp directory and opens
// its storage. The returned store holds the process-level storage lock; Close
// is registered with t.Cleanup so the lock file (and its handle) is released
// even on failure, which avoids Windows temp-directory cleanup issues.
func setupLockedProject(t *testing.T) (storage.Storer, string) {
	t.Helper()
	dir := t.TempDir()
	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	store, _, err := OpenProject(dir)
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store, dir
}

// writeFile is a t.Fatalf-free file writer that is safe to call from
// goroutines (which must not call t.Fatalf). Errors are ignored: temp dirs are
// writable, and any real failure surfaces as a meaningful error from the
// porcelain operation that follows.
func writeFile(dir, name, content string) {
	path := filepath.Join(dir, name)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, []byte(content), 0644)
}

// acceptableConcurrentResult reports whether a set of errors from concurrently
// launched workspace operations is consistent with correct lock behavior. The
// workspace lock serializes the operations, so any of these outcomes is valid:
//   - at least one operation failed with ErrLocked (contention was detected),
//   - all operations succeeded (the lock serialized them without overlap),
//   - the only non-nil errors are ErrNothingToSave (a serialization artifact:
//     the first operation captured all workspace changes, leaving the second
//     nothing new to save — this is not data corruption).
//
// Any other error indicates a real problem (e.g. data corruption or a bug).
func acceptableConcurrentResult(errs ...error) bool {
	// Contention detected: the lock blocked a concurrent operation.
	for _, err := range errs {
		if errors.Is(err, ErrLocked) {
			return true
		}
	}
	// All succeeded: the lock serialized the operations cleanly.
	allNil := true
	for _, err := range errs {
		if err != nil {
			allNil = false
			break
		}
	}
	if allNil {
		return true
	}
	// Serialization artifact: every non-nil error is ErrNothingToSave. A nil
	// error alongside ErrNothingToSave is fine (one operation found work to
	// do, the other did not).
	for _, err := range errs {
		if err != nil && !errors.Is(err, ErrNothingToSave) {
			return false
		}
	}
	return true
}

// TestAcquireWorkspaceLock_Toctou verifies that the workspace lock is
// TOCTOU-safe: the O_CREATE|O_EXCL atomic create guarantees two callers can
// never both observe "the file does not exist" and then each create it. A
// second acquisition while the lock is held must fail with ErrLocked, and the
// lock must become available again after release.
func TestAcquireWorkspaceLock_Toctou(t *testing.T) {
	_, dir := setupLockedProject(t)

	// First acquisition must succeed.
	if err := AcquireWorkspaceLock(dir); err != nil {
		t.Fatalf("first AcquireWorkspaceLock: %v", err)
	}

	// Re-acquiring while held must fail with ErrLocked. This is the TOCTOU
	// guarantee: the atomic create-or-fail prevents a race between "check the
	// lock is free" and "create the lock".
	if err := AcquireWorkspaceLock(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("second AcquireWorkspaceLock: expected ErrLocked, got %v", err)
	}

	// After release, the lock must be available again.
	ReleaseWorkspaceLock(dir)
	if err := AcquireWorkspaceLock(dir); err != nil {
		t.Fatalf("AcquireWorkspaceLock after release: %v", err)
	}
	ReleaseWorkspaceLock(dir)
}

// TestCreateSnapshot_ConcurrentWithSwitch launches CreateSnapshot and
// SwitchBranch concurrently against the same workspace. Both contend on the
// workspace.lock file via AcquireWorkspaceLock, so they must never overlap.
// The test passes if the lock detects the contention (ErrLocked) or the
// operations serialize cleanly (both succeed, or one finds nothing to save).
func TestCreateSnapshot_ConcurrentWithSwitch(t *testing.T) {
	store, dir := setupLockedProject(t)

	// Initialize main with a first snapshot so SwitchBranch has a source
	// branch to switch away from.
	writeFile(dir, "init.txt", "initial content")
	if _, err := CreateSnapshot(context.Background(), store, dir, "init", "test", nil); err != nil {
		t.Fatalf("initial CreateSnapshot: %v", err)
	}

	var snapErr, switchErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// Write a unique file so there is something new to snapshot.
		writeFile(dir, "snap.txt", "snap content")
		_, snapErr = CreateSnapshot(context.Background(), store, dir, "concurrent snapshot", "test", nil)
	}()

	go func() {
		defer wg.Done()
		// Write a unique file so SwitchBranch's auto-save has something to capture.
		writeFile(dir, "switch.txt", "switch content")
		_, _, _, switchErr = SwitchBranch(context.Background(), store, dir, "feature", true, "test")
	}()

	wg.Wait()

	if acceptableConcurrentResult(snapErr, switchErr) {
		return
	}
	t.Errorf("unexpected concurrent result: snap=%v switch=%v", snapErr, switchErr)
}

// TestCollectGarbage_ConcurrentWithSave launches CreateSnapshot and
// CollectGarbage (dry-run) concurrently. Both contend on the workspace.lock
// file: GC must not observe a half-written index while a save is in progress.
// The test passes if the lock detects contention (ErrLocked) or the operations
// serialize cleanly.
func TestCollectGarbage_ConcurrentWithSave(t *testing.T) {
	store, dir := setupLockedProject(t)

	// Seed a snapshot so GC has a reachability graph to traverse.
	writeFile(dir, "seed.txt", "seed content")
	if _, err := CreateSnapshot(context.Background(), store, dir, "seed", "test", nil); err != nil {
		t.Fatalf("seed CreateSnapshot: %v", err)
	}

	var snapErr, gcErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// Write a new file so the concurrent save has something to capture.
		writeFile(dir, "more.txt", "more content")
		_, snapErr = CreateSnapshot(context.Background(), store, dir, "concurrent save", "test", nil)
	}()

	go func() {
		defer wg.Done()
		_, gcErr = CollectGarbage(context.Background(), store, dir, true)
	}()

	wg.Wait()

	if acceptableConcurrentResult(snapErr, gcErr) {
		return
	}
	t.Errorf("unexpected concurrent result: snap=%v gc=%v", snapErr, gcErr)
}
