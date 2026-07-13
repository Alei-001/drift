package porcelain

import (
	"context"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
)

// integrityTestStore builds a store with one snapshot referencing two chunks.
// Returns the store, the snapshot ID, and the two chunk hashes.
func integrityTestStore(t *testing.T) (*memory.MemoryStorage, core.SnapshotID, core.Hash, core.Hash) {
	t.Helper()
	store := memory.NewMemoryStorage()

	c1 := gcChunk([]byte("chunk-one-data"), 14)
	c2 := gcChunk([]byte("chunk-two-data"), 14)
	store.PutChunk(context.Background(), c1)
	store.PutChunk(context.Background(), c2)

	fileAHash := computeFileHashFromHashes([]core.Hash{c1.Hash})
	fileBHash := computeFileHashFromHashes([]core.Hash{c2.Hash})

	files := []core.FileEntry{
		{
			Path:   "a.txt",
			Mode:   core.FileMode(0o644),
			Size:   14,
			Chunks: []core.Hash{c1.Hash},
			Hash:   fileAHash,
		},
		{
			Path:   "b.txt",
			Mode:   core.FileMode(0o644),
			Size:   14,
			Chunks: []core.Hash{c2.Hash},
			Hash:   fileBHash,
		},
	}
	snap := &core.Snapshot{
		Timestamp: 100,
		Files:     files,
	}
	snap.ID = computeSnapshotID(snap)
	store.PutSnapshot(context.Background(), snap)

	return store, snap.ID, c1.Hash, c2.Hash
}

// TestVerifyIntegrity_AllValid verifies that a repository with all chunks
// and snapshots intact reports no corruption.
func TestVerifyIntegrity_AllValid(t *testing.T) {
	store, _, _, _ := integrityTestStore(t)

	report, err := VerifyIntegrity(context.Background(), store, "", "", false)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if report.Corrupt != 0 {
		t.Errorf("expected 0 corrupt chunks, got %d", report.Corrupt)
	}
	if report.Missing != 0 {
		t.Errorf("expected 0 missing chunks, got %d", report.Missing)
	}
	if report.SnapshotCorrupt != 0 {
		t.Errorf("expected 0 corrupt snapshots, got %d", report.SnapshotCorrupt)
	}
	if report.FileHashMismatch != 0 {
		t.Errorf("expected 0 file hash mismatches, got %d", report.FileHashMismatch)
	}
	if report.TotalBlocks != 2 {
		t.Errorf("expected 2 total blocks, got %d", report.TotalBlocks)
	}
}

// TestVerifyIntegrity_MissingChunk verifies that a chunk referenced by a
// snapshot but absent from the store is reported as missing.
func TestVerifyIntegrity_MissingChunk(t *testing.T) {
	store, _, ch1, _ := integrityTestStore(t)

	// Delete one chunk so the snapshot references it but it's gone.
	if err := store.DeleteChunk(context.Background(), ch1); err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}

	report, err := VerifyIntegrity(context.Background(), store, "", "", false)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if report.Missing != 1 {
		t.Errorf("expected 1 missing chunk, got %d", report.Missing)
	}
	if report.Corrupt != 0 {
		t.Errorf("expected 0 corrupt, got %d", report.Corrupt)
	}
}

// TestVerifyIntegrity_FileHashMismatch verifies that a snapshot entry whose
// Hash field does not match computeFileHashFromHashes(entry.Chunks) is
// reported as a file hash mismatch (P1-5 check).
func TestVerifyIntegrity_FileHashMismatch(t *testing.T) {
	store := memory.NewMemoryStorage()

	c1 := gcChunk([]byte("good-data"), 9)
	store.PutChunk(context.Background(), c1)

	// Deliberately set a WRONG file hash so it does not match
	// computeFileHashFromHashes([c1.Hash]).
	wrongHash := gcHash(0x99)

	files := []core.FileEntry{
		{
			Path:   "a.txt",
			Mode:   core.FileMode(0o644),
			Size:   9,
			Chunks: []core.Hash{c1.Hash},
			Hash:   wrongHash, // wrong: does not match blake3(concat(c1.Hash))
		},
	}
	snap := &core.Snapshot{
		Timestamp: 50,
		Files:     files,
	}
	snap.ID = computeSnapshotID(snap)
	store.PutSnapshot(context.Background(), snap)

	report, err := VerifyIntegrity(context.Background(), store, "", "", false)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if report.FileHashMismatch != 1 {
		t.Errorf("expected 1 file hash mismatch, got %d", report.FileHashMismatch)
	}
	// The chunk itself is fine, so no corrupt/missing.
	if report.Corrupt != 0 || report.Missing != 0 {
		t.Errorf("expected 0 corrupt and 0 missing, got corrupt=%d missing=%d",
			report.Corrupt, report.Missing)
	}
}

// TestVerifyIntegrity_FilterGlob verifies that the filter parameter limits
// verification to files matching the glob pattern.
func TestVerifyIntegrity_FilterGlob(t *testing.T) {
	store, _, _, _ := integrityTestStore(t)

	// Only check "a.txt" — the filter should exclude b.txt's chunk.
	report, err := VerifyIntegrity(context.Background(), store, "", "a.txt", false)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	// Only 1 unique chunk (from a.txt) should be checked.
	if report.TotalBlocks != 1 {
		t.Errorf("expected 1 total block (filter=a.txt), got %d", report.TotalBlocks)
	}
	if report.Corrupt != 0 || report.Missing != 0 {
		t.Errorf("expected 0 corrupt/missing, got corrupt=%d missing=%d",
			report.Corrupt, report.Missing)
	}
}

// TestVerifyIntegrity_VerboseOutput verifies that verbose mode populates the
// VerboseRefs slice with per-chunk reference context.
func TestVerifyIntegrity_VerboseOutput(t *testing.T) {
	store, _, _, _ := integrityTestStore(t)

	report, err := VerifyIntegrity(context.Background(), store, "", "", true)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	// Two files × one chunk each = 2 verbose refs.
	if len(report.VerboseRefs) != 2 {
		t.Fatalf("expected 2 verbose refs, got %d", len(report.VerboseRefs))
	}
	for _, r := range report.VerboseRefs {
		if r.Status != ChunkOK {
			t.Errorf("expected ChunkOK for %s, got %s", r.FilePath, r.Status)
		}
		if r.SnapID == "" {
			t.Error("expected non-empty SnapID in verbose ref")
		}
		if r.FilePath == "" {
			t.Error("expected non-empty FilePath in verbose ref")
		}
	}
}

// TestVerifyIntegrity_EmptyStore verifies that an empty store produces a
// zero-block report with no errors.
func TestVerifyIntegrity_EmptyStore(t *testing.T) {
	store := memory.NewMemoryStorage()

	report, err := VerifyIntegrity(context.Background(), store, "", "", false)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if report.TotalBlocks != 0 {
		t.Errorf("expected 0 blocks, got %d", report.TotalBlocks)
	}
	if report.Corrupt != 0 || report.Missing != 0 {
		t.Errorf("expected 0 corrupt/missing, got corrupt=%d missing=%d",
			report.Corrupt, report.Missing)
	}
}

// TestVerifyIntegrity_FilterNoMatch verifies that a filter matching no files
// produces a zero-block report with no errors.
func TestVerifyIntegrity_FilterNoMatch(t *testing.T) {
	store, _, _, _ := integrityTestStore(t)

	// No file matches "nonexistent*" — the report should have 0 blocks.
	report, err := VerifyIntegrity(context.Background(), store, "", "nonexistent*", false)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if report.TotalBlocks != 0 {
		t.Errorf("expected 0 blocks for non-matching filter, got %d", report.TotalBlocks)
	}
	if report.Corrupt != 0 || report.Missing != 0 {
		t.Errorf("expected 0 corrupt/missing, got corrupt=%d missing=%d",
			report.Corrupt, report.Missing)
	}
}
