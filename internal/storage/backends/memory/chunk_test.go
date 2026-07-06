package memory

import (
	"context"
	"testing"

	"github.com/your-org/drift/internal/core"
)

func TestDeleteChunk_Idempotent(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	hash := core.Hash{0x01}

	// Deleting a non-existent chunk should not error.
	if err := store.DeleteChunk(ctx, hash); err != nil {
		t.Errorf("DeleteChunk on missing chunk returned error: %v", err)
	}

	// Add the chunk and delete it twice.
	store.PutChunk(ctx, &core.Chunk{Hash: hash, Data: []byte("x")})
	if err := store.DeleteChunk(ctx, hash); err != nil {
		t.Errorf("DeleteChunk returned error: %v", err)
	}
	if err := store.DeleteChunk(ctx, hash); err != nil {
		t.Errorf("second DeleteChunk returned error: %v", err)
	}

	if ok, _ := store.HasChunk(ctx, hash); ok {
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
	hashes := []core.Hash{{0x01}, {0x02}, {0x03}}
	for _, h := range hashes {
		store.PutChunk(ctx, &core.Chunk{Hash: h, Data: []byte{0}})
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
	for _, h := range hashes {
		if !gotMap[h.FullString()] {
			t.Errorf("expected hash %s in ListChunks result", h.FullString())
		}
	}
}

func TestPutChunk_Overwrite(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	hash := core.Hash{0x01}

	store.PutChunk(ctx, &core.Chunk{Hash: hash, Data: []byte("original")})
	store.PutChunk(ctx, &core.Chunk{Hash: hash, Data: []byte("replaced")})

	got, err := store.GetChunk(ctx, hash)
	if err != nil {
		t.Fatalf("GetChunk failed: %v", err)
	}
	if string(got.Data) != "replaced" {
		t.Errorf("Data: got %q, want %q (overwrite)", got.Data, "replaced")
	}
}

func TestHasChunk_AfterDelete(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	hash := core.Hash{0x01}
	store.PutChunk(ctx, &core.Chunk{Hash: hash, Data: []byte("x")})

	if ok, _ := store.HasChunk(ctx, hash); !ok {
		t.Error("expected HasChunk=true before delete")
	}
	store.DeleteChunk(ctx, hash)
	if ok, _ := store.HasChunk(ctx, hash); ok {
		t.Error("expected HasChunk=false after delete")
	}
}

func TestGetChunk_ReturnsClone(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	hash := core.Hash{0x01}
	store.PutChunk(ctx, &core.Chunk{Hash: hash, Data: []byte("original")})

	got1, _ := store.GetChunk(ctx, hash)
	got1.Data[0] = 'X'

	got2, _ := store.GetChunk(ctx, hash)
	if string(got2.Data) != "original" {
		t.Errorf("mutating first GetChunk result affected stored data: got %q", got2.Data)
	}
}
