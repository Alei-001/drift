package porcelain

import (
	"context"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
)

// gcHash builds a non-zero Hash from a single byte, so each snapshot/chunk
// has a distinct, easily-readable identity.
func gcHash(b byte) core.Hash {
	return core.Hash{b}
}

func TestCollectGarbage_AllReachable(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()
	snapHash := gcHash(0x01)
	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snapHash})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})
	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snapHash}, PrevID: nil})

	report, err := CollectGarbage(context.Background(), store, dir, false, 0)
	if err != nil {
		t.Fatalf("CollectGarbage failed: %v", err)
	}
	if report.SnapshotsRemoved != 0 {
		t.Errorf("expected 0 snapshots removed, got %d", report.SnapshotsRemoved)
	}
	if report.ChunksRemoved != 0 {
		t.Errorf("expected 0 chunks removed, got %d", report.ChunksRemoved)
	}
}

func TestCollectGarbage_ReclaimsUnreachable(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	snap1 := gcHash(0x01)
	snap2 := gcHash(0x02)
	snap3 := gcHash(0x03) // orphan
	chunk1 := gcHash(0x10)
	chunk2 := gcHash(0x11)
	chunk3 := gcHash(0x12)

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap2})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	store.PutSnapshot(context.Background(), &core.Snapshot{
		ID:     core.SnapshotID{Hash: snap1},
		PrevID: nil,
		Files:  []core.FileEntry{{Path: "a", Chunks: []core.Hash{chunk1}}},
	})
	store.PutSnapshot(context.Background(), &core.Snapshot{
		ID:     core.SnapshotID{Hash: snap2},
		PrevID: &core.SnapshotID{Hash: snap1},
		Files:  []core.FileEntry{{Path: "b", Chunks: []core.Hash{chunk2}}},
	})
	store.PutSnapshot(context.Background(), &core.Snapshot{
		ID:     core.SnapshotID{Hash: snap3},
		PrevID: nil,
		Files:  []core.FileEntry{{Path: "c", Chunks: []core.Hash{chunk3}}},
	})

	store.PutChunk(context.Background(), &core.Chunk{Hash: chunk1, Size: 100, Data: []byte("chunk1")})
	store.PutChunk(context.Background(), &core.Chunk{Hash: chunk2, Size: 200, Data: []byte("chunk2")})
	store.PutChunk(context.Background(), &core.Chunk{Hash: chunk3, Size: 300, Data: []byte("chunk3")})

	report, err := CollectGarbage(context.Background(), store, dir, false, 0)
	if err != nil {
		t.Fatalf("CollectGarbage failed: %v", err)
	}
	if report.SnapshotsRemoved != 1 {
		t.Errorf("expected 1 snapshot removed, got %d", report.SnapshotsRemoved)
	}
	if report.ChunksRemoved != 1 {
		t.Errorf("expected 1 chunk removed, got %d", report.ChunksRemoved)
	}
	if report.FreedBytes != 300 {
		t.Errorf("expected 300 freed bytes, got %d", report.FreedBytes)
	}

	// snap1 and snap2 preserved; snap3 gone.
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snap1}); err != nil {
		t.Errorf("snap1 should be preserved: %v", err)
	}
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snap2}); err != nil {
		t.Errorf("snap2 should be preserved: %v", err)
	}
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snap3}); err == nil {
		t.Error("snap3 should have been removed")
	}
	// chunk1 and chunk2 preserved; chunk3 gone.
	if ok, _ := store.HasChunk(context.Background(), chunk1); !ok {
		t.Error("chunk1 should be preserved")
	}
	if ok, _ := store.HasChunk(context.Background(), chunk2); !ok {
		t.Error("chunk2 should be preserved")
	}
	if ok, _ := store.HasChunk(context.Background(), chunk3); ok {
		t.Error("chunk3 should have been removed")
	}
}

func TestCollectGarbage_SharedChunkPreserved(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	snap1 := gcHash(0x01) // reachable via main
	snap3 := gcHash(0x03) // orphan
	chunkA := gcHash(0x20)

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap1})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	// Both snapshots reference the same chunkA.
	store.PutSnapshot(context.Background(), &core.Snapshot{
		ID:     core.SnapshotID{Hash: snap1},
		PrevID: nil,
		Files:  []core.FileEntry{{Path: "a", Chunks: []core.Hash{chunkA}}},
	})
	store.PutSnapshot(context.Background(), &core.Snapshot{
		ID:     core.SnapshotID{Hash: snap3},
		PrevID: nil,
		Files:  []core.FileEntry{{Path: "c", Chunks: []core.Hash{chunkA}}},
	})
	store.PutChunk(context.Background(), &core.Chunk{Hash: chunkA, Size: 150, Data: []byte("shared")})

	report, err := CollectGarbage(context.Background(), store, dir, false, 0)
	if err != nil {
		t.Fatalf("CollectGarbage failed: %v", err)
	}
	if report.SnapshotsRemoved != 1 {
		t.Errorf("expected 1 snapshot removed, got %d", report.SnapshotsRemoved)
	}
	if report.ChunksRemoved != 0 {
		t.Errorf("expected 0 chunks removed (chunkA still referenced), got %d", report.ChunksRemoved)
	}

	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snap3}); err == nil {
		t.Error("snap3 should have been removed")
	}
	if ok, _ := store.HasChunk(context.Background(), chunkA); !ok {
		t.Error("chunkA should be preserved because snap1 still references it")
	}
}

func TestCollectGarbage_DryRunNoDelete(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	snap1 := gcHash(0x01) // reachable
	snap2 := gcHash(0x02) // orphan
	chunk1 := gcHash(0x10)
	chunk2 := gcHash(0x11) // only referenced by orphan snap2

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap1})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	store.PutSnapshot(context.Background(), &core.Snapshot{
		ID:     core.SnapshotID{Hash: snap1},
		PrevID: nil,
		Files:  []core.FileEntry{{Path: "a", Chunks: []core.Hash{chunk1}}},
	})
	store.PutSnapshot(context.Background(), &core.Snapshot{
		ID:     core.SnapshotID{Hash: snap2},
		PrevID: nil,
		Files:  []core.FileEntry{{Path: "b", Chunks: []core.Hash{chunk2}}},
	})
	store.PutChunk(context.Background(), &core.Chunk{Hash: chunk1, Size: 100, Data: []byte("chunk1")})
	store.PutChunk(context.Background(), &core.Chunk{Hash: chunk2, Size: 250, Data: []byte("chunk2")})

	report, err := CollectGarbage(context.Background(), store, dir, true, 0)
	if err != nil {
		t.Fatalf("CollectGarbage dry-run failed: %v", err)
	}
	if report.SnapshotsRemoved != 1 {
		t.Errorf("expected 1 snapshot reported, got %d", report.SnapshotsRemoved)
	}
	if report.ChunksRemoved != 1 {
		t.Errorf("expected 1 chunk reported, got %d", report.ChunksRemoved)
	}
	if report.FreedBytes != 250 {
		t.Errorf("expected 250 freed bytes (estimate), got %d", report.FreedBytes)
	}

	// Nothing should actually be deleted.
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snap2}); err != nil {
		t.Errorf("snap2 should still exist after dry-run: %v", err)
	}
	if ok, _ := store.HasChunk(context.Background(), chunk2); !ok {
		t.Error("chunk2 should still exist after dry-run")
	}
}

func TestCollectGarbage_PrevIDChain(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	snap1 := gcHash(0x01)
	snap2 := gcHash(0x02)
	snap3 := gcHash(0x03)
	snap4 := gcHash(0x04) // orphan

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap3})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap1}, PrevID: nil})
	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap2}, PrevID: &core.SnapshotID{Hash: snap1}})
	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap3}, PrevID: &core.SnapshotID{Hash: snap2}})
	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap4}, PrevID: nil})

	report, err := CollectGarbage(context.Background(), store, dir, false, 0)
	if err != nil {
		t.Fatalf("CollectGarbage failed: %v", err)
	}
	if report.SnapshotsRemoved != 1 {
		t.Errorf("expected 1 snapshot removed, got %d", report.SnapshotsRemoved)
	}

	for _, h := range []core.Hash{snap1, snap2, snap3} {
		if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: h}); err != nil {
			t.Errorf("snapshot %x should be preserved: %v", h, err)
		}
	}
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snap4}); err == nil {
		t.Error("snap4 should have been removed")
	}
}

func TestCollectGarbage_TagKeepsSnapshot(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	snap1 := gcHash(0x01) // reachable only via tag
	snap2 := gcHash(0x02) // reachable via main

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap2})
	store.SetRef(context.Background(), "tags/v1", &core.Reference{Name: "tags/v1", Type: core.RefTypeTag, Target: snap1})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap1}, PrevID: nil})
	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap2}, PrevID: nil})

	report, err := CollectGarbage(context.Background(), store, dir, false, 0)
	if err != nil {
		t.Fatalf("CollectGarbage failed: %v", err)
	}
	if report.SnapshotsRemoved != 0 {
		t.Errorf("expected 0 snapshots removed (snap1 kept by tag), got %d", report.SnapshotsRemoved)
	}

	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snap1}); err != nil {
		t.Errorf("snap1 should be preserved by tags/v1: %v", err)
	}
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snap2}); err != nil {
		t.Errorf("snap2 should be preserved by heads/main: %v", err)
	}
}

func TestCollectGarbage_ZeroTargetSkipped(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	// main with zero target (freshly initialized, no commits) plus an orphan.
	snapOrphan := gcHash(0x0f)

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: core.Hash{}})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})
	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snapOrphan}, PrevID: nil})

	report, err := CollectGarbage(context.Background(), store, dir, false, 0)
	if err != nil {
		t.Fatalf("CollectGarbage failed: %v", err)
	}
	if report.SnapshotsRemoved != 1 {
		t.Errorf("expected 1 snapshot removed (orphan), got %d", report.SnapshotsRemoved)
	}
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snapOrphan}); err == nil {
		t.Error("orphan snapshot should have been removed")
	}
}

func TestCountUnreachableSnapshots(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	snap1 := gcHash(0x01) // reachable
	snap2 := gcHash(0x02) // orphan
	snap3 := gcHash(0x03) // orphan

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap1})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap1}, PrevID: nil})
	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap2}, PrevID: nil})
	store.PutSnapshot(context.Background(), &core.Snapshot{ID: core.SnapshotID{Hash: snap3}, PrevID: nil})

	count, err := CountUnreachableSnapshots(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("CountUnreachableSnapshots failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unreachable snapshots, got %d", count)
	}
}
