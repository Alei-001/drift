package memory

import (
	"context"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/zeebo/blake3"
)

// makeChunk creates a Chunk whose Hash is the BLAKE3 of data, matching
// the hash validation enforced by PutChunk.
func makeChunk(data []byte) *core.Chunk {
	var hash core.Hash
	sum := blake3.Sum256(data)
	copy(hash[:], sum[:])
	return &core.Chunk{Hash: hash, Size: uint32(len(data)), Data: data}
}

func TestDeleteChunk_Idempotent(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	ch := makeChunk([]byte("x"))

	// Deleting a non-existent chunk should not error.
	if err := store.DeleteChunk(ctx, ch.Hash); err != nil {
		t.Errorf("DeleteChunk on missing chunk returned error: %v", err)
	}

	// Add the chunk and delete it twice.
	if err := store.PutChunk(ctx, ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}
	if err := store.DeleteChunk(ctx, ch.Hash); err != nil {
		t.Errorf("DeleteChunk returned error: %v", err)
	}
	if err := store.DeleteChunk(ctx, ch.Hash); err != nil {
		t.Errorf("second DeleteChunk returned error: %v", err)
	}

	if ok, _ := store.HasChunk(ctx, ch.Hash); ok {
		t.Error("expected HasChunk=false after delete")
	}
}

func TestListChunks_Empty(t *testing.T) {
	store := NewMemoryStorage()
	hashes, err := store.ListChunks(context.Background())
	if err != nil {
		t.Fatalf("ListChunks failed: %v", err)
	}
	if len(hashes) != 0 {
		t.Errorf("expected 0 hashes for empty store, got %d", len(hashes))
	}
}

func TestListChunks_Multiple(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	chunks := []*core.Chunk{
		makeChunk([]byte("chunk-a")),
		makeChunk([]byte("chunk-b")),
		makeChunk([]byte("chunk-c")),
	}
	for _, ch := range chunks {
		if err := store.PutChunk(ctx, ch); err != nil {
			t.Fatalf("PutChunk: %v", err)
		}
	}
	got, err := store.ListChunks(ctx)
	if err != nil {
		t.Fatalf("ListChunks failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 hashes, got %d", len(got))
	}

	// All stored hashes should be present in the result.
	gotMap := make(map[string]bool)
	for _, h := range got {
		gotMap[h.FullString()] = true
	}
	for _, ch := range chunks {
		if !gotMap[ch.Hash.FullString()] {
			t.Errorf("expected hash %s in ListChunks result", ch.Hash.FullString())
		}
	}
}

// TestPutChunk_Idempotent verifies that storing the same chunk twice does
// not error and the data is consistent. (With hash validation, two different
// data values cannot share the same hash, so "overwrite with different data"
// is no longer a valid scenario.)
func TestPutChunk_Idempotent(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	ch := makeChunk([]byte("idempotent data"))

	if err := store.PutChunk(ctx, ch); err != nil {
		t.Fatalf("first PutChunk: %v", err)
	}
	if err := store.PutChunk(ctx, ch); err != nil {
		t.Fatalf("second PutChunk: %v", err)
	}

	got, err := store.GetChunk(ctx, ch.Hash)
	if err != nil {
		t.Fatalf("GetChunk failed: %v", err)
	}
	if string(got.Data) != "idempotent data" {
		t.Errorf("Data: got %q, want %q", got.Data, "idempotent data")
	}
}

func TestHasChunk_AfterDelete(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	ch := makeChunk([]byte("x"))
	if err := store.PutChunk(ctx, ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	if ok, _ := store.HasChunk(ctx, ch.Hash); !ok {
		t.Error("expected HasChunk=true before delete")
	}
	store.DeleteChunk(ctx, ch.Hash)
	if ok, _ := store.HasChunk(ctx, ch.Hash); ok {
		t.Error("expected HasChunk=false after delete")
	}
}

func TestGetChunk_ReturnsClone(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	ch := makeChunk([]byte("original"))
	if err := store.PutChunk(ctx, ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	got1, _ := store.GetChunk(ctx, ch.Hash)
	got1.Data[0] = 'X'

	got2, _ := store.GetChunk(ctx, ch.Hash)
	if string(got2.Data) != "original" {
		t.Errorf("mutating first GetChunk result affected stored data: got %q", got2.Data)
	}
}

// TestPutChunk_HashMismatch verifies that PutChunk rejects a chunk whose
// Hash does not match the BLAKE3 of its Data.
func TestPutChunk_HashMismatch(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	ch := &core.Chunk{Hash: core.Hash{0x01}, Data: []byte("mismatch")}

	err := store.PutChunk(ctx, ch)
	if err == nil {
		t.Fatal("expected error for hash mismatch, got nil")
	}
}
