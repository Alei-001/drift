package memory

import (
	"context"
	"testing"

	"github.com/your-org/drift/internal/core"
)

func TestSetIndex_Overwrite(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()

	store.SetIndex(ctx, &core.Index{
		Entries:   []core.IndexEntry{{Path: "a.txt"}},
		UpdatedAt: 1,
	})
	store.SetIndex(ctx, &core.Index{
		Entries:   []core.IndexEntry{{Path: "b.txt"}},
		UpdatedAt: 2,
	})

	got, err := store.GetIndex(ctx)
	if err != nil {
		t.Fatalf("GetIndex failed: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Entries len: got %d, want 1", len(got.Entries))
	}
	if got.Entries[0].Path != "b.txt" {
		t.Errorf("Entries[0].Path: got %q, want %q", got.Entries[0].Path, "b.txt")
	}
	if got.UpdatedAt != 2 {
		t.Errorf("UpdatedAt: got %d, want 2", got.UpdatedAt)
	}
}

func TestSetIndex_ClonesInput(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	idx := &core.Index{
		Entries:   []core.IndexEntry{{Path: "a.txt", Chunks: []core.Hash{{0x11}}}},
		UpdatedAt: 1,
	}
	store.SetIndex(ctx, idx)

	// Mutate the input; stored index should be unaffected.
	idx.Entries[0].Path = "mutated"
	idx.Entries[0].Chunks[0] = core.Hash{0xff}

	got, _ := store.GetIndex(ctx)
	if got.Entries[0].Path != "a.txt" {
		t.Errorf("mutating input affected stored Path: got %q", got.Entries[0].Path)
	}
	if got.Entries[0].Chunks[0] != (core.Hash{0x11}) {
		t.Errorf("mutating input affected stored Chunks: got %v", got.Entries[0].Chunks[0])
	}
}

func TestGetIndex_ClonesState(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetIndex(ctx, &core.Index{
		Entries:   []core.IndexEntry{{Path: "a.txt"}},
		UpdatedAt: 1,
	})

	got, _ := store.GetIndex(ctx)
	got.Entries[0].Path = "mutated"

	again, _ := store.GetIndex(ctx)
	if again.Entries[0].Path != "a.txt" {
		t.Errorf("mutating returned index affected stored state: got %q", again.Entries[0].Path)
	}
}

func TestSetIndex_NilEntries(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetIndex(ctx, &core.Index{UpdatedAt: 1})

	got, _ := store.GetIndex(ctx)
	if got.Entries != nil {
		t.Errorf("Entries: got %v, want nil", got.Entries)
	}
	if got.UpdatedAt != 1 {
		t.Errorf("UpdatedAt: got %d, want 1", got.UpdatedAt)
	}
}

func TestSetIndex_PreservesChunks(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	chunks := []core.Hash{{0x11}, {0x22}, {0x33}}
	store.SetIndex(ctx, &core.Index{
		Entries: []core.IndexEntry{
			{Path: "a.txt", Chunks: chunks},
		},
		UpdatedAt: 1,
	})

	got, _ := store.GetIndex(ctx)
	if len(got.Entries[0].Chunks) != 3 {
		t.Fatalf("Chunks len: got %d, want 3", len(got.Entries[0].Chunks))
	}
	for i, h := range chunks {
		if got.Entries[0].Chunks[i] != h {
			t.Errorf("Chunks[%d]: got %v, want %v", i, got.Entries[0].Chunks[i], h)
		}
	}
}

func TestClose_NoOp(t *testing.T) {
	store := NewMemoryStorage()
	if err := store.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
	// Close should be idempotent.
	if err := store.Close(); err != nil {
		t.Errorf("second Close returned error: %v", err)
	}
}
