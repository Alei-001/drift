package porcelain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
)

// failingHeadSetRefStore wraps a Storer and forces SetRef to fail only for
// the "HEAD" ref. All other SetRef calls (branch refs, etc.) and all other
// methods delegate to the embedded Storer. Used to exercise the
// RestoreSnapshot rollback path that fires when SetRef("HEAD") fails after
// the branch ref has already been updated to the restore target.
type failingHeadSetRefStore struct {
	storage.Storer
}

func (s *failingHeadSetRefStore) SetRef(ctx context.Context, name string, ref *core.Reference) error {
	if name == "HEAD" {
		return errors.New("simulated HEAD SetRef failure")
	}
	return s.Storer.SetRef(ctx, name, ref)
}

// setupRestoreSnapshots creates a memory store via setupBranchStore and two
// snapshots so HEAD and the branch ref both point at snap2. Returns the
// store and both snapshot IDs.
func setupRestoreSnapshots(t *testing.T, dir string) (*memory.MemoryStorage, core.SnapshotID, core.SnapshotID) {
	t.Helper()
	store := setupBranchStore()
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v1"), 0644); err != nil {
		t.Fatalf("write file v1: %v", err)
	}
	snap1, err := CreateSnapshot(ctx, store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot: %v", err)
	}

	// Different-length content so CreateSnapshot detects the change.
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v2 modified"), 0644); err != nil {
		t.Fatalf("write file v2: %v", err)
	}
	snap2, err := CreateSnapshot(ctx, store, dir, "second commit", "test", nil)
	if err != nil {
		t.Fatalf("second CreateSnapshot: %v", err)
	}

	return store, snap1.ID, snap2.ID
}

// TestRestoreSnapshot_HeadUpdateFailureRollsBackBranch verifies that when
// SetRef("HEAD") fails during a full restore, the branch ref — which was
// already updated to the restore target — is rolled back to its pre-restore
// value. Without the rollback, the branch would point at the restore target
// while HEAD still references the pre-restore tip, desynchronizing HEAD from
// its symbolic target and breaking the history chain on the next save
// (architecture.md §5.2 step 3).
func TestRestoreSnapshot_HeadUpdateFailureRollsBackBranch(t *testing.T) {
	dir := t.TempDir()
	store, snap1, snap2 := setupRestoreSnapshots(t, dir)
	defer store.Close()
	ctx := context.Background()

	// Sanity: before restore, branch resolves to snap2.
	branchBefore, _ := store.GetRef(ctx, "heads/main")
	if branchBefore.Target != snap2.Hash {
		t.Fatalf("precondition: heads/main should be at snap2, got %s", branchBefore.Target.String())
	}

	failingStore := &failingHeadSetRefStore{Storer: store}

	// Full restore to snap1 with noBackup=true. SetRef("HEAD") will fail.
	_, err := RestoreSnapshot(ctx, failingStore, dir, snap1, "", true, nil)
	if err == nil {
		t.Fatal("expected RestoreSnapshot to fail when SetRef HEAD fails, got nil")
	}
	if !strings.Contains(err.Error(), "update HEAD") {
		t.Errorf("expected error mentioning 'update HEAD', got: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back branch") {
		t.Errorf("expected error mentioning 'rolled back branch', got: %v", err)
	}

	// The branch ref must have been rolled back to snap2 (pre-restore).
	branchAfter, err := store.GetRef(ctx, "heads/main")
	if err != nil {
		t.Fatalf("GetRef heads/main after rollback: %v", err)
	}
	if branchAfter.Target != snap2.Hash {
		t.Errorf("heads/main should be rolled back to snap2, got %s, want %s",
			branchAfter.Target.String(), snap2.Hash.String())
	}
	if branchAfter.Target == snap1.Hash {
		t.Error("heads/main should not point to snap1 (restore target) after rollback")
	}
}

// TestRestoreSnapshot_HeadUpdateFailureKeepsHEAD verifies that after the
// SetRef("HEAD") failure, HEAD still resolves to the pre-restore snapshot
// (snap2): the failed SetRef left HEAD's stored value untouched, and the
// branch rollback kept the symref chain consistent.
func TestRestoreSnapshot_HeadUpdateFailureKeepsHEAD(t *testing.T) {
	dir := t.TempDir()
	store, snap1, snap2 := setupRestoreSnapshots(t, dir)
	defer store.Close()
	ctx := context.Background()

	failingStore := &failingHeadSetRefStore{Storer: store}

	if _, err := RestoreSnapshot(ctx, failingStore, dir, snap1, "", true, nil); err == nil {
		t.Fatal("expected RestoreSnapshot to fail when SetRef HEAD fails, got nil")
	}

	// HEAD resolves through heads/main (rolled back to snap2) -> snap2.
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD after failed restore: %v", err)
	}
	if headRef.Target != snap2.Hash {
		t.Errorf("HEAD should resolve to snap2 (pre-restore) after rollback, got %s, want %s",
			headRef.Target.String(), snap2.Hash.String())
	}
}

// TestRestoreSnapshot_HeadUpdateFailureFilesRestored verifies that the
// workspace files were restored to the target snapshot content even though
// the ref update failed. This confirms the failure is specifically in the
// ref-update phase (after restoreFilesToWorkspace completed), not in the
// file-write phase — so the user gets a clear signal that files changed
// but the refs were rolled back.
func TestRestoreSnapshot_HeadUpdateFailureFilesRestored(t *testing.T) {
	dir := t.TempDir()
	store, snap1, _ := setupRestoreSnapshots(t, dir)
	defer store.Close()
	ctx := context.Background()

	failingStore := &failingHeadSetRefStore{Storer: store}

	if _, err := RestoreSnapshot(ctx, failingStore, dir, snap1, "", true, nil); err == nil {
		t.Fatal("expected RestoreSnapshot to fail when SetRef HEAD fails, got nil")
	}

	// The workspace file must reflect snap1's content (restoreFilesToWorkspace
	// ran before the ref-update phase that failed).
	content, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatalf("read file.txt after restore: %v", err)
	}
	if string(content) != "content v1" {
		t.Errorf("expected file restored to snap1 content 'content v1', got %q", string(content))
	}
}
