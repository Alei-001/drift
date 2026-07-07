package filesystem

import (
	"context"
	"fmt"
	"testing"

	"github.com/Alei-001/drift/internal/core"
)

// TestGetIndex_LargeIndex verifies that GetIndex can read a large staging
// index (many entries with chunk hashes) via the streaming os.Open +
// io.ReadAll path without panic, and that data round-trips correctly.
func TestGetIndex_LargeIndex(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	const numEntries = 10_000
	entries := make([]core.IndexEntry, numEntries)
	for i := range entries {
		chunks := make([]core.Hash, 4)
		for j := range chunks {
			chunks[j] = core.Hash{byte(i), byte(i >> 8), byte(j), 0x03}
		}
		entries[i] = core.IndexEntry{
			Path:    fmt.Sprintf("path/to/file_%05d.bin", i),
			Hash:    core.Hash{byte(i), byte(i >> 8), byte(i >> 16), 0x04},
			Size:    int64(i * 100),
			ModTime: int64(i),
			Chunks:  chunks,
		}
	}
	idx := &core.Index{Entries: entries, UpdatedAt: 99}

	ctx := context.Background()
	if err := store.SetIndex(ctx, idx); err != nil {
		t.Fatalf("SetIndex: %v", err)
	}

	got, err := store.GetIndex(ctx)
	if err != nil {
		t.Fatalf("GetIndex large: %v", err)
	}
	if got.UpdatedAt != 99 {
		t.Errorf("UpdatedAt: got %d, want 99", got.UpdatedAt)
	}
	if len(got.Entries) != numEntries {
		t.Fatalf("Entries length: got %d, want %d", len(got.Entries), numEntries)
	}
	// Spot-check first and last entries.
	first, last := got.Entries[0], got.Entries[numEntries-1]
	if first.Path != entries[0].Path {
		t.Errorf("first path: got %q, want %q", first.Path, entries[0].Path)
	}
	if first.Hash != entries[0].Hash {
		t.Errorf("first hash mismatch")
	}
	if len(first.Chunks) != 4 {
		t.Errorf("first chunks: got %d, want 4", len(first.Chunks))
	}
	if first.Chunks[0] != entries[0].Chunks[0] {
		t.Errorf("first chunk[0] mismatch")
	}
	if last.Path != entries[numEntries-1].Path {
		t.Errorf("last path: got %q, want %q", last.Path, entries[numEntries-1].Path)
	}
	if last.Hash != entries[numEntries-1].Hash {
		t.Errorf("last hash mismatch")
	}
}

// TestGetIndex_MissingReturnsEmpty verifies that GetIndex returns an empty
// Index (not an error) when the index file does not exist.
func TestGetIndex_MissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	got, err := store.GetIndex(context.Background())
	if err != nil {
		t.Fatalf("GetIndex on missing file: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty Index")
	}
	if len(got.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(got.Entries))
	}
}
