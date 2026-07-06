package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
)

func TestPutSnapshot_ClonesInput(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	snap := &core.Snapshot{
		ID:      core.SnapshotID{Hash: core.Hash{0x01}},
		Message: "original",
		Files:   []core.FileEntry{{Path: "a.txt"}},
		Tags:    []string{"v1"},
	}
	store.PutSnapshot(ctx, snap)

	// Mutate the input snapshot; stored value should be unaffected.
	snap.Message = "mutated"
	snap.Files[0].Path = "mutated.txt"
	snap.Tags[0] = "mutated"

	got, _ := store.GetSnapshot(ctx, snap.ID)
	if got.Message != "original" {
		t.Errorf("Message: got %q, want %q", got.Message, "original")
	}
	if got.Files[0].Path != "a.txt" {
		t.Errorf("Files[0].Path: got %q, want %q", got.Files[0].Path, "a.txt")
	}
	if got.Tags[0] != "v1" {
		t.Errorf("Tags[0]: got %q, want %q", got.Tags[0], "v1")
	}
}

func TestGetSnapshot_ClonesState(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	id := core.SnapshotID{Hash: core.Hash{0x01}}
	store.PutSnapshot(ctx, &core.Snapshot{
		ID:      id,
		Message: "original",
		Files:   []core.FileEntry{{Path: "a.txt"}},
	})

	got, _ := store.GetSnapshot(ctx, id)
	got.Message = "mutated"
	got.Files[0].Path = "mutated.txt"

	again, _ := store.GetSnapshot(ctx, id)
	if again.Message != "original" {
		t.Errorf("mutating returned snapshot affected stored state: Message=%q", again.Message)
	}
	if again.Files[0].Path != "a.txt" {
		t.Errorf("mutating returned snapshot affected stored state: Path=%q", again.Files[0].Path)
	}
}

func TestPutSnapshot_Overwrite(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	id := core.SnapshotID{Hash: core.Hash{0x01}}
	store.PutSnapshot(ctx, &core.Snapshot{ID: id, Message: "first"})
	store.PutSnapshot(ctx, &core.Snapshot{ID: id, Message: "second"})

	got, _ := store.GetSnapshot(ctx, id)
	if got.Message != "second" {
		t.Errorf("Message after overwrite: got %q, want %q", got.Message, "second")
	}
}

func TestDeleteSnapshot_Idempotent(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	id := core.SnapshotID{Hash: core.Hash{0x01}}

	// Deleting a non-existent snapshot should not error.
	if err := store.DeleteSnapshot(ctx, id); err != nil {
		t.Errorf("DeleteSnapshot on missing snapshot returned error: %v", err)
	}

	store.PutSnapshot(ctx, &core.Snapshot{ID: id})
	if err := store.DeleteSnapshot(ctx, id); err != nil {
		t.Errorf("DeleteSnapshot returned error: %v", err)
	}
	// Deleting again should still not error.
	if err := store.DeleteSnapshot(ctx, id); err != nil {
		t.Errorf("second DeleteSnapshot returned error: %v", err)
	}
}

func TestListSnapshots_Pagination(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		store.PutSnapshot(ctx, &core.Snapshot{
			ID:        core.SnapshotID{Hash: core.Hash{byte(i)}},
			Message:   "snap",
			Timestamp: int64(i),
		})
	}

	// Limit only.
	page, _ := store.ListSnapshots(ctx, &storage.ListOptions{Limit: 3})
	if len(page) != 3 {
		t.Errorf("Limit=3: got %d, want 3", len(page))
	}

	// Offset only.
	tail, _ := store.ListSnapshots(ctx, &storage.ListOptions{Offset: 8})
	if len(tail) != 2 {
		t.Errorf("Offset=8: got %d, want 2", len(tail))
	}

	// Offset beyond length returns nil.
	empty, _ := store.ListSnapshots(ctx, &storage.ListOptions{Offset: 100})
	if empty != nil {
		t.Errorf("Offset=100: got %v, want nil", empty)
	}

	// Limit + Offset.
	mid, _ := store.ListSnapshots(ctx, &storage.ListOptions{Limit: 3, Offset: 3})
	if len(mid) != 3 {
		t.Errorf("Limit=3 Offset=3: got %d, want 3", len(mid))
	}
}

func TestListSnapshots_SortedByTimestampDesc(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	// Insert out of order.
	for _, ts := range []int64{5, 1, 9, 3, 7} {
		store.PutSnapshot(ctx, &core.Snapshot{
			ID:        core.SnapshotID{Hash: core.Hash{byte(ts)}},
			Timestamp: ts,
		})
	}
	snaps, _ := store.ListSnapshots(ctx, nil)
	if len(snaps) != 5 {
		t.Fatalf("expected 5 snapshots, got %d", len(snaps))
	}
	for i := 1; i < len(snaps); i++ {
		if snaps[i].Timestamp > snaps[i-1].Timestamp {
			t.Errorf("not sorted desc: [%d]=%d > [%d]=%d", i-1, snaps[i-1].Timestamp, i, snaps[i].Timestamp)
		}
	}
}

func TestListSnapshots_Empty(t *testing.T) {
	store := NewMemoryStorage()
	snaps, err := store.ListSnapshots(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if snaps != nil {
		t.Errorf("expected nil for empty store, got %d snapshots", len(snaps))
	}
}

func TestGetSnapshot_WithPrevID(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	prevID := &core.SnapshotID{Hash: core.Hash{0x99}}
	id := core.SnapshotID{Hash: core.Hash{0x01}}
	store.PutSnapshot(ctx, &core.Snapshot{
		ID:        id,
		PrevID:    prevID,
		Message:   "second",
		Timestamp: 2,
	})

	got, err := store.GetSnapshot(ctx, id)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if got.PrevID == nil || got.PrevID.Hash != prevID.Hash {
		t.Errorf("PrevID: got %v, want %v", got.PrevID, prevID)
	}
}

func TestGetSnapshot_NotFound_WrappedError(t *testing.T) {
	store := NewMemoryStorage()
	_, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: core.Hash{0x99}})
	if err == nil {
		t.Fatal("expected error for missing snapshot, got nil")
	}
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
