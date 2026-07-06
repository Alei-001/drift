package porcelain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/your-org/drift/internal/core"
)

// TestCountSnapshotDiff_BothNil verifies the degenerate case where both
// snapshots are nil: zero files differ.
func TestCountSnapshotDiff_BothNil(t *testing.T) {
	if got := countSnapshotDiff(nil, nil); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// TestCountSnapshotDiff_FromNil verifies that when "from" is nil, every file
// in "to" is counted as added.
func TestCountSnapshotDiff_FromNil(t *testing.T) {
	to := &core.Snapshot{Files: []core.FileEntry{
		{Path: "a"}, {Path: "b"}, {Path: "c"},
	}}
	if got := countSnapshotDiff(nil, to); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

// TestCountSnapshotDiff_ToNil verifies that when "to" is nil, every file in
// "from" is counted as deleted.
func TestCountSnapshotDiff_ToNil(t *testing.T) {
	from := &core.Snapshot{Files: []core.FileEntry{
		{Path: "a"}, {Path: "b"},
	}}
	if got := countSnapshotDiff(from, nil); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}

// TestCountSnapshotDiff_Identical verifies that two snapshots with the same
// paths and hashes produce zero diff.
func TestCountSnapshotDiff_Identical(t *testing.T) {
	hashA := core.Hash{0xAA}
	hashB := core.Hash{0xBB}
	from := &core.Snapshot{Files: []core.FileEntry{
		{Path: "a", Hash: hashA},
		{Path: "b", Hash: hashB},
	}}
	to := &core.Snapshot{Files: []core.FileEntry{
		{Path: "a", Hash: hashA},
		{Path: "b", Hash: hashB},
	}}
	if got := countSnapshotDiff(from, to); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// TestCountSnapshotDiff_AddedRemovedModified verifies the full mix: a file
// unchanged, one removed, one modified, one added.
func TestCountSnapshotDiff_AddedRemovedModified(t *testing.T) {
	hashA := core.Hash{0xAA}
	hashB := core.Hash{0xBB}
	hashC := core.Hash{0xCC}
	hashC2 := core.Hash{0xCC, 0x01}
	hashD := core.Hash{0xDD}
	from := &core.Snapshot{Files: []core.FileEntry{
		{Path: "a", Hash: hashA}, // unchanged
		{Path: "b", Hash: hashB}, // removed in 'to'
		{Path: "c", Hash: hashC}, // modified in 'to'
	}}
	to := &core.Snapshot{Files: []core.FileEntry{
		{Path: "a", Hash: hashA},  // unchanged
		{Path: "c", Hash: hashC2}, // modified
		{Path: "d", Hash: hashD},  // added
	}}
	// 1 removed (b) + 1 modified (c) + 1 added (d) = 3
	if got := countSnapshotDiff(from, to); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

// TestCountSnapshotDiff_PathOrderingInvariance verifies that the diff count is
// independent of the order files appear in either snapshot's Files slice.
func TestCountSnapshotDiff_PathOrderingInvariance(t *testing.T) {
	hashA := core.Hash{0xAA}
	hashB := core.Hash{0xBB}
	from := &core.Snapshot{Files: []core.FileEntry{
		{Path: "a", Hash: hashA},
		{Path: "b", Hash: hashB},
	}}
	to := &core.Snapshot{Files: []core.FileEntry{
		{Path: "b", Hash: hashB},
		{Path: "a", Hash: hashA},
	}}
	if got := countSnapshotDiff(from, to); got != 0 {
		t.Errorf("expected 0 for reordered identical files, got %d", got)
	}
}

// TestCountSnapshotDiff_EmptySnapshots verifies that two empty (but non-nil)
// snapshots produce zero diff.
func TestCountSnapshotDiff_EmptySnapshots(t *testing.T) {
	from := &core.Snapshot{}
	to := &core.Snapshot{}
	if got := countSnapshotDiff(from, to); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// TestUndoLastSave_Success verifies that undo moves HEAD back to the previous
// snapshot. After undo, the branch target equals the first snapshot's hash,
// and the undone snapshot is still retrievable (it becomes unreachable, not
// deleted).
func TestUndoLastSave_Success(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v1"), 0644)
	snap1, err := CreateSnapshot(context.Background(), store, dir, "first", "test", nil, nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot: %v", err)
	}

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v2 modified"), 0644)
	snap2, err := CreateSnapshot(context.Background(), store, dir, "second", "test", nil, nil)
	if err != nil {
		t.Fatalf("second CreateSnapshot: %v", err)
	}

	if err := UndoLastSave(context.Background(), store, dir, nil); err != nil {
		t.Fatalf("UndoLastSave: %v", err)
	}

	headRef, err := store.GetRef(context.Background(), "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD: %v", err)
	}
	if headRef.Target != snap1.ID.Hash {
		t.Errorf("expected HEAD at snap1 %s, got %s", snap1.ID.Hash.String(), headRef.Target.String())
	}

	mainRef, err := store.GetRef(context.Background(), "heads/main")
	if err != nil {
		t.Fatalf("GetRef heads/main: %v", err)
	}
	if mainRef.Target != snap1.ID.Hash {
		t.Errorf("expected main at snap1, got %s", mainRef.Target.String())
	}

	// The undone snapshot is not deleted (becomes unreachable for gc).
	if _, err := store.GetSnapshot(context.Background(), snap2.ID); err != nil {
		t.Errorf("undone snapshot should still exist: %v", err)
	}
}

// TestUndoLastSave_AtInitialSnapshot verifies that undo refuses when HEAD is
// at the initial snapshot (no PrevID to revert to).
func TestUndoLastSave_AtInitialSnapshot(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v1"), 0644)
	_, err := CreateSnapshot(context.Background(), store, dir, "first", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	err = UndoLastSave(context.Background(), store, dir, nil)
	if !errors.Is(err, ErrCannotUndo) {
		t.Fatalf("expected ErrCannotUndo, got %v", err)
	}
}

// TestUndoLastSave_NoSnapshots verifies that undo refuses when there are no
// snapshots at all (HEAD target is zero).
func TestUndoLastSave_NoSnapshots(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	err := UndoLastSave(context.Background(), store, dir, nil)
	if !errors.Is(err, ErrCannotUndo) {
		t.Fatalf("expected ErrCannotUndo, got %v", err)
	}
}

// TestUndoLastSave_UncommittedChanges verifies that undo refuses when the
// workspace has uncommitted changes after the last save.
func TestUndoLastSave_UncommittedChanges(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v1"), 0644)
	_, err := CreateSnapshot(context.Background(), store, dir, "first", "test", nil, nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot: %v", err)
	}

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v2 modified"), 0644)
	_, err = CreateSnapshot(context.Background(), store, dir, "second", "test", nil, nil)
	if err != nil {
		t.Fatalf("second CreateSnapshot: %v", err)
	}

	// Introduce an uncommitted change after the last save.
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v3 uncommitted"), 0644)

	err = UndoLastSave(context.Background(), store, dir, nil)
	if !errors.Is(err, ErrUncommittedChanges) {
		t.Fatalf("expected ErrUncommittedChanges, got %v", err)
	}
}

// TestUndoLastSave_UpdatesIndex verifies that after undo, the index is
// rebuilt to match the previous snapshot, so subsequent DetectChanges
// correctly reports workspace differences relative to the new HEAD.
func TestUndoLastSave_UpdatesIndex(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v1"), 0644)
	snap1, err := CreateSnapshot(context.Background(), store, dir, "first", "test", nil, nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot: %v", err)
	}

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("v2 modified"), 0644)
	_, err = CreateSnapshot(context.Background(), store, dir, "second", "test", nil, nil)
	if err != nil {
		t.Fatalf("second CreateSnapshot: %v", err)
	}

	if err := UndoLastSave(context.Background(), store, dir, nil); err != nil {
		t.Fatalf("UndoLastSave: %v", err)
	}

	// After undo, the index matches snap1. The workspace still has snap2's
	// file content, so DetectChanges should report file1.txt as modified.
	summary, err := DetectChanges(context.Background(), store, dir, nil)
	if err != nil {
		t.Fatalf("DetectChanges: %v", err)
	}
	if len(summary.Modified) != 1 || summary.Modified[0] != "file1.txt" {
		t.Errorf("expected file1.txt modified, got %v", summary.Modified)
	}
	if len(summary.Added) != 0 || len(summary.Deleted) != 0 {
		t.Errorf("expected no added/deleted, got added=%v deleted=%v", summary.Added, summary.Deleted)
	}

	// HEAD should be at snap1.
	headRef, err := store.GetRef(context.Background(), "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD: %v", err)
	}
	if headRef.Target != snap1.ID.Hash {
		t.Errorf("expected HEAD at snap1, got %s", headRef.Target.String())
	}
}
