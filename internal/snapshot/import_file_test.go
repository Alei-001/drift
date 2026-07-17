package snapshot

import (
	"github.com/Alei-001/drift/internal/errs"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/memory"
)

// importTestBranch sets up a store with a "feature" branch whose tip snapshot
// contains two files: a text file and a binary file. Returns the store and
// the chunk hashes so tests can verify content after import.
func importTestBranch(t *testing.T) (*store.StoreSet, []byte, []byte) {
	t.Helper()
	ms := memory.NewMemoryStorage()

	textData := []byte("imported text content")
	binData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0x00, 0x42}
	cText := gcChunk(textData, uint32(len(textData)))
	cBin := gcChunk(binData, uint32(len(binData)))
	ms.PutChunk(context.Background(), cText)
	ms.PutChunk(context.Background(), cBin)

	textHash := computeFileHashFromHashes([]core.Hash{cText.Hash})
	binHash := computeFileHashFromHashes([]core.Hash{cBin.Hash})

	files := []core.FileEntry{
		{
			Path:   "docs/readme.txt",
			Mode:   core.FileMode(0o644),
			Size:   int64(len(textData)),
			Chunks: []core.Hash{cText.Hash},
			Hash:   textHash,
		},
		{
			Path:   "assets/blob.bin",
			Mode:   core.FileMode(0o644),
			Size:   int64(len(binData)),
			Chunks: []core.Hash{cBin.Hash},
			Hash:   binHash,
		},
	}
	snap := &core.Snapshot{
		Timestamp: 100,
		PrevID:    nil,
		Files:     files,
	}
	snap.ID = computeSnapshotID(snap)
	ms.PutSnapshot(context.Background(), snap)

	ms.SetRef(context.Background(), "heads/feature", &core.Reference{
		Name:   "heads/feature",
		Type:   core.RefTypeBranch,
		Target: snap.ID.Hash,
	})

	return store.NewStoreSet(ms), textData, binData
}

// TestImportFileFromBranch_NormalImport reconstructs a file from the feature
// branch into the current workspace and verifies the content and index.
func TestImportFileFromBranch_NormalImport(t *testing.T) {
	store, textData, _ := importTestBranch(t)
	dir := t.TempDir()

	entry, err := ImportFileFromBranch(context.Background(), store, dir, "feature", "docs/readme.txt", nil)
	if err != nil {
		t.Fatalf("ImportFileFromBranch failed: %v", err)
	}
	if entry.Path != "docs/readme.txt" {
		t.Errorf("expected entry path 'docs/readme.txt', got %q", entry.Path)
	}

	// Verify the file was written with correct content.
	content, err := os.ReadFile(filepath.Join(dir, "docs", "readme.txt"))
	if err != nil {
		t.Fatalf("read imported file: %v", err)
	}
	if string(content) != string(textData) {
		t.Errorf("content = %q, want %q", string(content), string(textData))
	}

	// Verify the index was NOT updated: the imported file must appear as
	// a new (added) file in 'drift status' so the next 'drift save' can
	// capture it. This is the intended workflow documented in the command
	// help ("After importing, run 'drift save' to record the change").
	idx, err := store.Index.GetIndex(context.Background())
	if err == nil && len(idx.Entries) > 0 {
		for _, e := range idx.Entries {
			if e.Path == "docs/readme.txt" {
				t.Error("index should NOT contain 'docs/readme.txt' after import; the file must be detectable as 'added' by status/save")
			}
		}
	}
}

// TestImportFileFromBranch_BranchNotFound verifies that importing from a
// non-existent branch returns errs.ErrBranchNotFound.
func TestImportFileFromBranch_BranchNotFound(t *testing.T) {
	store, _, _ := importTestBranch(t)
	dir := t.TempDir()

	_, err := ImportFileFromBranch(context.Background(), store, dir, "nonexistent", "docs/readme.txt", nil)
	if err == nil {
		t.Fatal("expected error for non-existent branch, got nil")
	}
	if !errors.Is(err, errs.ErrBranchNotFound) {
		t.Errorf("expected errs.ErrBranchNotFound, got %v", err)
	}
}

// TestImportFileFromBranch_FileNotFound verifies that importing a file that
// does not exist in the branch's snapshot returns errs.ErrFileNotFound.
func TestImportFileFromBranch_FileNotFound(t *testing.T) {
	store, _, _ := importTestBranch(t)
	dir := t.TempDir()

	_, err := ImportFileFromBranch(context.Background(), store, dir, "feature", "nonexistent.txt", nil)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !errors.Is(err, errs.ErrFileNotFound) {
		t.Errorf("expected errs.ErrFileNotFound, got %v", err)
	}
}

// TestImportFileFromBranch_PathTraversalRejected verifies that a file path
// containing ".." is rejected by pathutil.RelToWorkDir before any file is
// written outside the workspace.
func TestImportFileFromBranch_PathTraversalRejected(t *testing.T) {
	store, _, _ := importTestBranch(t)
	dir := t.TempDir()

	_, err := ImportFileFromBranch(context.Background(), store, dir, "feature", "../../../etc/passwd", nil)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	// The error should come from pathutil.RelToWorkDir, wrapped as
	// "invalid file path: ...".
}

// TestImportFileFromBranch_BinaryFile verifies that binary files (with null
// bytes and high bytes) are reconstructed correctly.
func TestImportFileFromBranch_BinaryFile(t *testing.T) {
	store, _, binData := importTestBranch(t)
	dir := t.TempDir()

	entry, err := ImportFileFromBranch(context.Background(), store, dir, "feature", "assets/blob.bin", nil)
	if err != nil {
		t.Fatalf("ImportFileFromBranch binary failed: %v", err)
	}
	if entry.Path != "assets/blob.bin" {
		t.Errorf("expected entry path 'assets/blob.bin', got %q", entry.Path)
	}

	content, err := os.ReadFile(filepath.Join(dir, "assets", "blob.bin"))
	if err != nil {
		t.Fatalf("read imported binary: %v", err)
	}
	if string(content) != string(binData) {
		t.Errorf("binary content mismatch: got %v, want %v", content, binData)
	}
}

// TestImportFileFromBranch_BranchNoSnapshots verifies that importing from a
// branch with a zero target (freshly initialized, no commits) returns
// errs.ErrSnapshotNotFound.
func TestImportFileFromBranch_BranchNoSnapshots(t *testing.T) {
	store := memory.NewMemoryStorage()
	store.SetRef(context.Background(), "heads/empty", &core.Reference{
		Name:   "heads/empty",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})

	dir := t.TempDir()
	storeSet := store.NewStoreSet(store)
	_, err := ImportFileFromBranch(context.Background(), storeSet, dir, "empty", "file.txt", nil)
	if err == nil {
		t.Fatal("expected error for branch with no snapshots, got nil")
	}
	if !errors.Is(err, errs.ErrSnapshotNotFound) {
		t.Errorf("expected errs.ErrSnapshotNotFound, got %v", err)
	}
}

// TestImportFileFromBranch_PreservesExistingIndex verifies that import does
// not touch the index even when a pre-existing entry for the same path is
// present. The index must stay as-is so that 'drift status' detects the
// workspace file as modified (different size/hash) and 'drift save' can
// capture it.
func TestImportFileFromBranch_PreservesExistingIndex(t *testing.T) {
	store, _, _ := importTestBranch(t)
	dir := t.TempDir()

	// Pre-populate the index with a stale entry for docs/readme.txt.
	store.Index.SetIndex(context.Background(), &core.Index{
		Entries: []core.IndexEntry{
			{Path: "docs/readme.txt", Size: 1},
		},
	})

	_, err := ImportFileFromBranch(context.Background(), store, dir, "feature", "docs/readme.txt", nil)
	if err != nil {
		t.Fatalf("ImportFileFromBranch failed: %v", err)
	}

	// The index entry should remain unchanged (size 1, not the imported
	// file's real size) so status/save detect the mismatch.
	idx, err := store.Index.GetIndex(context.Background())
	if err != nil {
		t.Fatalf("GetIndex failed: %v", err)
	}
	for _, e := range idx.Entries {
		if e.Path == "docs/readme.txt" && e.Size != 1 {
			t.Errorf("index should be unchanged (size 1), got size %d", e.Size)
		}
	}
}
