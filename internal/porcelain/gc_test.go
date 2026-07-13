package porcelain

import (
	"context"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// computeSnapshotID computes the BLAKE3 hash of a snapshot's marshaled proto
// (with IdHash omitted), mirroring porcelain.CreateSnapshot. Needed because
// the memory backend's GetSnapshot verifies content integrity.
func computeSnapshotID(snap *core.Snapshot) core.SnapshotID {
	p := core.SnapshotToProto(snap, false)
	marshaled, _ := proto.Marshal(p)
	return core.SnapshotID{Hash: core.Hash(blake3.Sum256(marshaled))}
}

// gcPutSnapshot creates and stores a snapshot with a content-derived ID. The
// tag byte is used as the Timestamp to ensure distinct snapshots produce
// distinct IDs even when PrevID and Files are identical. Returns the
// snapshot's hash.
func gcPutSnapshot(store *memory.MemoryStorage, tag byte, prevID *core.SnapshotID, files []core.FileEntry) core.Hash {
	snap := &core.Snapshot{
		Timestamp: int64(tag),
		PrevID:    prevID,
		Files:     files,
	}
	snap.ID = computeSnapshotID(snap)
	store.PutSnapshot(context.Background(), snap)
	return snap.ID.Hash
}

// gcHash builds a non-zero Hash from a single byte, so each snapshot has a
// distinct, easily-readable identity. Use gcHash only for snapshot IDs,
// which are not subject to content-hash validation. For chunks stored via
// PutChunk, use gcChunk instead.
func gcHash(b byte) core.Hash {
	return core.Hash{b}
}

// gcChunk builds a Chunk whose Hash is the BLAKE3 of data, matching the
// hash validation enforced by PutChunk. size is the logical chunk size
// reported in GC's FreedBytes (independent of len(data) so tests can
// assert specific reclaimed-byte counts).
func gcChunk(data []byte, size uint32) *core.Chunk {
	var hash core.Hash
	sum := blake3.Sum256(data)
	copy(hash[:], sum[:])
	return &core.Chunk{Hash: hash, Size: size, Data: data}
}

func TestCollectGarbage_AllReachable(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()
	snapHash := gcPutSnapshot(store, 0x01, nil, nil)
	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snapHash})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

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

	c1 := gcChunk([]byte("chunk1"), 100)
	c2 := gcChunk([]byte("chunk2"), 200)
	c3 := gcChunk([]byte("chunk3"), 300)
	chunk1, chunk2, chunk3 := c1.Hash, c2.Hash, c3.Hash

	snap1 := gcPutSnapshot(store, 0x01, nil, []core.FileEntry{{Path: "a", Chunks: []core.Hash{chunk1}}})
	snap2 := gcPutSnapshot(store, 0x02, &core.SnapshotID{Hash: snap1}, []core.FileEntry{{Path: "b", Chunks: []core.Hash{chunk2}}})
	snap3 := gcPutSnapshot(store, 0x03, nil, []core.FileEntry{{Path: "c", Chunks: []core.Hash{chunk3}}}) // orphan

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap2})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	store.PutChunk(context.Background(), c1)
	store.PutChunk(context.Background(), c2)
	store.PutChunk(context.Background(), c3)

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

	cA := gcChunk([]byte("shared"), 150)
	chunkA := cA.Hash

	snap1 := gcPutSnapshot(store, 0x01, nil, []core.FileEntry{{Path: "a", Chunks: []core.Hash{chunkA}}}) // reachable via main
	snap3 := gcPutSnapshot(store, 0x03, nil, []core.FileEntry{{Path: "c", Chunks: []core.Hash{chunkA}}}) // orphan

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap1})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})
	store.PutChunk(context.Background(), cA)

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

	c1 := gcChunk([]byte("chunk1"), 100)
	c2 := gcChunk([]byte("chunk2"), 250)
	chunk1, chunk2 := c1.Hash, c2.Hash // chunk2 only referenced by orphan snap2

	snap1 := gcPutSnapshot(store, 0x01, nil, []core.FileEntry{{Path: "a", Chunks: []core.Hash{chunk1}}}) // reachable
	snap2 := gcPutSnapshot(store, 0x02, nil, []core.FileEntry{{Path: "b", Chunks: []core.Hash{chunk2}}}) // orphan

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap1})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	store.PutChunk(context.Background(), c1)
	store.PutChunk(context.Background(), c2)

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

	snap1 := gcPutSnapshot(store, 0x01, nil, nil)
	snap2 := gcPutSnapshot(store, 0x02, &core.SnapshotID{Hash: snap1}, nil)
	snap3 := gcPutSnapshot(store, 0x03, &core.SnapshotID{Hash: snap2}, nil)
	snap4 := gcPutSnapshot(store, 0x04, nil, nil) // orphan

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap3})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

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

	snap1 := gcPutSnapshot(store, 0x01, nil, nil) // reachable only via tag
	snap2 := gcPutSnapshot(store, 0x02, nil, nil) // reachable via main

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap2})
	store.SetRef(context.Background(), "tags/v1", &core.Reference{Name: "tags/v1", Type: core.RefTypeTag, Target: snap1})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

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

	snap1 := gcPutSnapshot(store, 0x01, nil, nil) // reachable
	gcPutSnapshot(store, 0x02, nil, nil)          // orphan
	gcPutSnapshot(store, 0x03, nil, nil)          // orphan

	store.SetRef(context.Background(), "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: snap1})
	store.SetRef(context.Background(), "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	count, err := CountUnreachableSnapshots(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("CountUnreachableSnapshots failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unreachable snapshots, got %d", count)
	}
}

// TestCollectGarbage_DetachedHeadPreserved is a regression test for a bug
// where GC would collect a snapshot that a detached HEAD points at, severing
// the only reference to it. collectRoots must include a detached HEAD's
// target (SymRef=="" with a non-zero Target) as a root, even when no branch
// or tag references it.
func TestCollectGarbage_DetachedHeadPreserved(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	// Build a chain A -> B. Neither A nor B is referenced by a branch or tag.
	snapA := gcPutSnapshot(store, 0x01, nil, nil)
	snapB := gcPutSnapshot(store, 0x02, &core.SnapshotID{Hash: snapA}, nil)

	// Detached HEAD: SymRef is empty, Target points directly at B.
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: snapB,
	})
	// No branch or tag references B — HEAD is the only reference.

	report, err := CollectGarbage(context.Background(), store, dir, false, 0)
	if err != nil {
		t.Fatalf("CollectGarbage failed: %v", err)
	}
	if report.SnapshotsRemoved != 0 {
		t.Errorf("expected 0 snapshots removed (detached HEAD preserves B), got %d", report.SnapshotsRemoved)
	}

	// B must still exist.
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snapB}); err != nil {
		t.Errorf("snapB should be preserved by detached HEAD: %v", err)
	}
	// A must also exist (reachable from B via PrevID).
	if _, err := store.GetSnapshot(context.Background(), core.SnapshotID{Hash: snapA}); err != nil {
		t.Errorf("snapA should be preserved (reachable from B): %v", err)
	}
}
