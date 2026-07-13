package porcelain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
)

// failingSetIndexStore wraps a Storer and forces SetIndex to return an error.
// All other methods delegate to the embedded Storer so the store remains fully
// functional except for SetIndex. Used to exercise the CreateSnapshot rollback
// path that fires when SetIndex fails after the branch ref has been updated.
type failingSetIndexStore struct {
	storage.Storer
	setIndexErr error
}

func (s *failingSetIndexStore) SetIndex(ctx context.Context, index *core.Index) error {
	return s.setIndexErr
}

// TestCreateSnapshot_SetIndexFailureRollsBackBranch verifies that when SetIndex
// fails on a second commit, the branch ref is rolled back to its pre-save value
// (the first snapshot). Without the rollback, the branch would point at the new
// snapshot while the index still reflects the old workspace state, causing the
// next save to link to the wrong parent.
func TestCreateSnapshot_SetIndexFailureRollsBackBranch(t *testing.T) {
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

	// Modify the file so the second commit detects a change (different size
	// so the mtime+size fast path does not short-circuit).
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content v2 modified"), 0644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	// Wrap the store so SetIndex fails. All other ops delegate to memory.
	failingStore := &failingSetIndexStore{
		Storer:      store,
		setIndexErr: errors.New("simulated SetIndex I/O failure"),
	}

	_, err = CreateSnapshot(ctx, failingStore, dir, "second commit", "test", nil)
	if err == nil {
		t.Fatal("expected CreateSnapshot to fail when SetIndex fails, got nil")
	}
	if !strings.Contains(err.Error(), "rolled back branch ref") {
		t.Errorf("expected error mentioning 'rolled back branch ref', got: %v", err)
	}
	if !strings.Contains(err.Error(), "simulated SetIndex I/O failure") {
		t.Errorf("expected underlying error preserved, got: %v", err)
	}

	// The branch ref must have been rolled back to snap1.
	branchRef, err := store.GetRef(ctx, "heads/main")
	if err != nil {
		t.Fatalf("GetRef heads/main after rollback: %v", err)
	}
	if branchRef.Target != snap1.ID.Hash {
		t.Errorf("branch ref should point to snap1 after rollback, got %s, want %s",
			branchRef.Target.String(), snap1.ID.Hash.String())
	}
}

// TestCreateSnapshot_SetIndexFailureDeletesNewBranch verifies the other
// rollback branch: when the branch ref did NOT exist before the save (first
// commit on a fresh symref target) and SetIndex fails, the newly-created
// branch ref is deleted so the store returns to its pre-save state.
func TestCreateSnapshot_SetIndexFailureDeletesNewBranch(t *testing.T) {
	store := memory.NewMemoryStorage()
	defer store.Close()
	ctx := context.Background()

	// HEAD symref -> heads/main, but heads/main does NOT exist yet. An
	// empty index is set so CreateSnapshot reads it without relying on the
	// ErrNotFound fallback.
	store.SetRef(ctx, "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	store.SetIndex(ctx, &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("first content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	failingStore := &failingSetIndexStore{
		Storer:      store,
		setIndexErr: errors.New("simulated SetIndex I/O failure"),
	}

	_, err := CreateSnapshot(ctx, failingStore, dir, "first commit", "test", nil)
	if err == nil {
		t.Fatal("expected CreateSnapshot to fail when SetIndex fails, got nil")
	}
	if !strings.Contains(err.Error(), "rolled back branch ref") {
		t.Errorf("expected error mentioning 'rolled back branch ref', got: %v", err)
	}

	// heads/main was created by SetRef then deleted by the rollback, so it
	// must no longer exist.
	_, err = store.GetRef(ctx, "heads/main")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected heads/main to be deleted after rollback, got err=%v", err)
	}
}

// TestCreateSnapshot_SetIndexFailureKeepsHEADConsistent verifies that after
// the SetIndex rollback, GetRef("HEAD") still resolves through the symref
// chain to the pre-failure snapshot, so HEAD and the branch ref stay
// consistent (no dangling symref, no HEAD pointing at an orphaned snapshot).
func TestCreateSnapshot_SetIndexFailureKeepsHEADConsistent(t *testing.T) {
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
	if _, err := CreateSnapshot(ctx, failingStore, dir, "second commit", "test", nil); err == nil {
		t.Fatal("expected CreateSnapshot to fail when SetIndex fails, got nil")
	}

	// HEAD resolves through the symref to heads/main, which was rolled back
	// to snap1. So HEAD must still resolve to snap1.
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD after rollback: %v", err)
	}
	if headRef.Target != snap1.ID.Hash {
		t.Errorf("HEAD should resolve to snap1 after rollback, got %s, want %s",
			headRef.Target.String(), snap1.ID.Hash.String())
	}
}
