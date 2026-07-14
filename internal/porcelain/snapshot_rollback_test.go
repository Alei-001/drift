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

// failingSetIndexStore wraps a Storer and forces SetIndex to return an error.
// All other methods delegate to the embedded Storer so the store remains fully
// functional except for SetIndex. Used to exercise the CreateSnapshot failure
// path that fires when SetIndex fails after SetRef has already committed.
type failingSetIndexStore struct {
	storage.Storer
	setIndexErr error
}

func (s *failingSetIndexStore) SetIndex(ctx context.Context, index *core.Index) error {
	return s.setIndexErr
}

// failingSetRefStore wraps a Storer and forces SetRef to return an error.
// Used to exercise the CreateSnapshot failure path that fires when SetRef
// fails — the commit point. Under the git-style ordering (SetRef before
// SetIndex), a SetRef failure means the snapshot is unreachable and the
// branch ref is untouched.
type failingSetRefStore struct {
	storage.Storer
	setRefErr error
}

func (s *failingSetRefStore) SetRef(ctx context.Context, name string, ref *core.Reference) error {
	return s.setRefErr
}

// TestCreateSnapshot_SetRefFailureLeavesBranchUntouched verifies that when
// SetRef fails, the branch ref is left pointing at the previous snapshot.
// Under the git-style ordering (SetRef is the commit point, before
// SetIndex), a SetRef failure means the snapshot written by PutSnapshot is
// unreachable and will be collected by GC. No rollback is needed because
// SetIndex never ran.
func TestCreateSnapshot_SetRefFailureLeavesBranchUntouched(t *testing.T) {
	store := setupBranchStore()
	defer store.Close()
	ctx := context.Background()
	dir := t.TempDir()

	// First commit succeeds with the plain store.
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	snap1, err := CreateSnapshot(ctx, store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot: %v", err)
	}

	// Modify the file so the second commit detects a change.
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v2 modified"), 0644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	// Wrap the store so SetRef fails. All other ops delegate to memory.
	failingStore := &failingSetRefStore{
		Storer:    store,
		setRefErr: errors.New("simulated SetRef I/O failure"),
	}

	_, err = CreateSnapshot(ctx, failingStore, dir, "second commit", "test", nil)
	if err == nil {
		t.Fatal("expected CreateSnapshot to fail when SetRef fails, got nil")
	}
	if !strings.Contains(err.Error(), "update branch") {
		t.Errorf("expected error mentioning 'update branch', got: %v", err)
	}

	// The branch ref was never updated (SetRef is the commit point), so
	// it must still point to snap1. The snapshot written by PutSnapshot
	// is unreachable and will be collected by GC.
	branchRef, err := store.GetRef(ctx, "heads/main")
	if err != nil {
		t.Fatalf("GetRef heads/main after SetRef failure: %v", err)
	}
	if branchRef.Target != snap1.ID.Hash {
		t.Errorf("branch ref should still point to snap1, got %s, want %s",
			branchRef.Target.String(), snap1.ID.Hash.String())
	}
}

// TestCreateSnapshot_SetIndexFailureCommitsSnapshot verifies that when
// SetIndex fails, the branch ref has already been updated to the new
// snapshot (SetRef runs first as the commit point). The snapshot is
// durable; the index is stale but will be rebuilt on the next save. The
// returned error includes the snapshot ID so the user knows the commit
// succeeded, and the returned snapshot is non-nil.
func TestCreateSnapshot_SetIndexFailureCommitsSnapshot(t *testing.T) {
	store := setupBranchStore()
	defer store.Close()
	ctx := context.Background()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	snap1, err := CreateSnapshot(ctx, store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v2 modified"), 0644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	failingStore := &failingSetIndexStore{
		Storer:      store,
		setIndexErr: errors.New("simulated SetIndex I/O failure"),
	}

	snap2, err := CreateSnapshot(ctx, failingStore, dir, "second commit", "test", nil)
	if err == nil {
		t.Fatal("expected CreateSnapshot to fail when SetIndex fails, got nil")
	}
	if !strings.Contains(err.Error(), "committed but index update failed") {
		t.Errorf("expected error mentioning 'committed but index update failed', got: %v", err)
	}
	if snap2 == nil {
		t.Fatal("expected non-nil snapshot even when SetIndex fails (commit already succeeded)")
	}
	if !strings.Contains(err.Error(), snap2.ShortID()) {
		t.Errorf("error should contain snapshot ID %q, got: %v", snap2.ShortID(), err)
	}

	// The branch ref was already updated to snap2 (SetRef is the commit
	// point, running before SetIndex). History is correct.
	branchRef, err := store.GetRef(ctx, "heads/main")
	if err != nil {
		t.Fatalf("GetRef heads/main after SetIndex failure: %v", err)
	}
	if branchRef.Target != snap2.ID.Hash {
		t.Errorf("branch ref should point to snap2 (commit succeeded), got %s, want %s",
			branchRef.Target.String(), snap2.ID.Hash.String())
	}

	// snap1 is still reachable as the parent of snap2.
	if snap2.PrevID == nil || snap2.PrevID.Hash != snap1.ID.Hash {
		t.Errorf("snap2 should have snap1 as parent, got prev=%v", snap2.PrevID)
	}
}

// TestCreateSnapshot_SetIndexFailureKeepsHEADAtNewSnapshot verifies that
// after a SetIndex failure, HEAD resolves through the symref chain to the
// new snapshot (snap2), not the previous one. Because SetRef already
// updated the branch, HEAD follows — the history chain is intact.
func TestCreateSnapshot_SetIndexFailureKeepsHEADAtNewSnapshot(t *testing.T) {
	store := setupBranchStore()
	defer store.Close()
	ctx := context.Background()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := CreateSnapshot(ctx, store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v2 modified"), 0644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	failingStore := &failingSetIndexStore{
		Storer:      store,
		setIndexErr: errors.New("simulated SetIndex I/O failure"),
	}
	snap2, err := CreateSnapshot(ctx, failingStore, dir, "second commit", "test", nil)
	if err == nil {
		t.Fatal("expected CreateSnapshot to fail when SetIndex fails, got nil")
	}

	// HEAD resolves through the symref to heads/main, which was already
	// updated to snap2. So HEAD must resolve to snap2.
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD after SetIndex failure: %v", err)
	}
	if headRef.Target != snap2.ID.Hash {
		t.Errorf("HEAD should resolve to snap2 (commit succeeded), got %s, want %s",
			headRef.Target.String(), snap2.ID.Hash.String())
	}
}

// TestCreateSnapshot_SetRefFailureOnFirstCommitDoesNotCreateBranch verifies
// that when SetRef fails on the first commit (no prior branch ref), the
// branch ref is never created. The snapshot is unreachable and GC will
// reclaim it.
func TestCreateSnapshot_SetRefFailureOnFirstCommitDoesNotCreateBranch(t *testing.T) {
	store := memory.NewMemoryStorage()
	defer store.Close()
	ctx := context.Background()

	// HEAD symref -> heads/main, but heads/main does NOT exist yet.
	store.SetRef(ctx, "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	store.SetIndex(ctx, &core.Index{
		Entries: []core.IndexEntry{},
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("first content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	failingStore := &failingSetRefStore{
		Storer:    store,
		setRefErr: errors.New("simulated SetRef I/O failure"),
	}

	_, err := CreateSnapshot(ctx, failingStore, dir, "first commit", "test", nil)
	if err == nil {
		t.Fatal("expected CreateSnapshot to fail when SetRef fails, got nil")
	}
	if !strings.Contains(err.Error(), "update branch") {
		t.Errorf("expected error mentioning 'update branch', got: %v", err)
	}

	// heads/main was never created because SetRef is the commit point
	// and it failed.
	_, err = store.GetRef(ctx, "heads/main")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected heads/main to not exist, got err=%v", err)
	}
}
