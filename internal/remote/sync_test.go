package remote

import (
	"bytes"
	"context"
	"testing"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/backends/memory"
)

// makeTestSnapshot builds a snapshot with one file entry referencing one chunk.
// The snapshot is stored in the given store. Returns the snapshot and chunk hash.
func makeTestSnapshot(t *testing.T, store storage.Storer, msg string, prevID *core.SnapshotID) (core.SnapshotID, core.Hash) {
	t.Helper()
	ctx := context.Background()

	chunkData := []byte("hello world chunk data")
	chunkHash := core.Hash{}
	// Use a simple deterministic hash for testing.
	for i, b := range chunkData {
		chunkHash[i%32] ^= b
	}
	chunk := &core.Chunk{
		Hash:  chunkHash,
		Size:  uint32(len(chunkData)),
		Data:  chunkData,
		Flags: core.ChunkFlagNone,
	}
	if err := store.PutChunk(ctx, chunk); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	snap := &core.Snapshot{
		Message:   msg,
		Author:    "tester",
		Timestamp: 1700000000,
		PrevID:    prevID,
		Files: []core.FileEntry{
			{
				Path:   "test.txt",
				Mode:   0o644,
				Size:   int64(len(chunkData)),
				Chunks: []core.Hash{chunkHash},
				Hash:   chunkHash, // simplified for test
			},
		},
		TotalSize: int64(len(chunkData)),
	}

	// Assign snapshot ID = hash of message (deterministic for test).
	for i, b := range []byte(msg) {
		snap.ID.Hash[i%32] ^= b
	}
	if err := store.PutSnapshot(ctx, snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}
	return snap.ID, chunkHash
}

func setupBranchRef(t *testing.T, store storage.Storer, branch string, target core.Hash) {
	t.Helper()
	ctx := context.Background()
	ref := &core.Reference{
		Name:   "heads/" + branch,
		Type:   core.RefTypeBranch,
		Target: target,
	}
	if err := store.SetRef(ctx, "heads/"+branch, ref); err != nil {
		t.Fatalf("SetRef %s: %v", branch, err)
	}
	headRef := &core.Reference{
		Name:   "HEAD",
		SymRef: "heads/" + branch,
	}
	if err := store.SetRef(ctx, "HEAD", headRef); err != nil {
		t.Fatalf("SetRef HEAD: %v", err)
	}
}

// TestPush_PushesSnapshotAndChunk verifies push uploads snapshots, manifests,
// and chunks to the remote.
func TestPush_PushesSnapshotAndChunk(t *testing.T) {
	store := memory.NewMemoryStorage()
	defer store.Close()
	rfs := NewMockRemoteFS()

	snapID, chunkHash := makeTestSnapshot(t, store, "test message", nil)
	setupBranchRef(t, store, "main", snapID.Hash)

	stats, err := Push(context.Background(), store, rfs, "")
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if stats.SnapshotsUploaded != 1 {
		t.Errorf("expected 1 snapshot uploaded, got %d", stats.SnapshotsUploaded)
	}
	if stats.ManifestsUploaded != 1 {
		t.Errorf("expected 1 manifest uploaded, got %d", stats.ManifestsUploaded)
	}
	if stats.ChunksUploaded != 1 {
		t.Errorf("expected 1 chunk uploaded, got %d", stats.ChunksUploaded)
	}
	if stats.RefsUpdated != 1 {
		t.Errorf("expected 1 ref updated, got %d", stats.RefsUpdated)
	}

	// Verify snapshot exists on remote.
	snapPath := snapshotRemotePath(snapID)
	if _, err := rfs.Stat(snapPath); err != nil {
		t.Errorf("remote snapshot not found: %v", err)
	}
	// Verify manifest exists on remote.
	manifestPath := manifestRemotePath(snapID)
	if _, err := rfs.Stat(manifestPath); err != nil {
		t.Errorf("remote manifest not found: %v", err)
	}
	// Verify chunk exists on remote.
	chunkPath := chunkRemotePath(chunkHash)
	if _, err := rfs.Stat(chunkPath); err != nil {
		t.Errorf("remote chunk not found: %v", err)
	}
}

// TestPush_SkipsExistingObjects verifies push skips objects already on the remote.
func TestPush_SkipsExistingObjects(t *testing.T) {
	store := memory.NewMemoryStorage()
	defer store.Close()
	rfs := NewMockRemoteFS()

	snapID, chunkHash := makeTestSnapshot(t, store, "skip test", nil)
	setupBranchRef(t, store, "main", snapID.Hash)

	// First push.
	_, err := Push(context.Background(), store, rfs, "")
	if err != nil {
		t.Fatalf("first Push: %v", err)
	}
	// Second push should skip everything.
	stats, err := Push(context.Background(), store, rfs, "")
	if err != nil {
		t.Fatalf("second Push: %v", err)
	}
	if stats.SnapshotsSkipped != 1 {
		t.Errorf("expected 1 snapshot skipped, got %d", stats.SnapshotsSkipped)
	}
	if stats.ChunksSkipped != 1 {
		t.Errorf("expected 1 chunk skipped, got %d", stats.ChunksSkipped)
	}
	if stats.SnapshotsUploaded != 0 || stats.ChunksUploaded != 0 {
		t.Errorf("expected 0 uploads on second push, got snap=%d chunk=%d",
			stats.SnapshotsUploaded, stats.ChunksUploaded)
	}
	// suppress unused var warning
	_ = chunkHash
}

// TestPull_DownloadsObjects verifies pull downloads snapshots and chunks.
func TestPull_DownloadsObjects(t *testing.T) {
	// Source store: has snapshot + chunk.
	srcStore := memory.NewMemoryStorage()
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID, chunkHash := makeTestSnapshot(t, srcStore, "pull test", nil)
	setupBranchRef(t, srcStore, "main", snapID.Hash)

	// Push from source to remote.
	if _, err := Push(context.Background(), srcStore, rfs, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Destination store: empty.
	dstStore := memory.NewMemoryStorage()
	defer dstStore.Close()

	stats, err := Pull(context.Background(), dstStore, rfs, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if stats.SnapshotsUploaded != 1 {
		t.Errorf("expected 1 snapshot downloaded, got %d", stats.SnapshotsUploaded)
	}
	if stats.ChunksUploaded != 1 {
		t.Errorf("expected 1 chunk downloaded, got %d", stats.ChunksUploaded)
	}

	// Verify snapshot exists in destination.
	snap, err := dstStore.GetSnapshot(context.Background(), snapID)
	if err != nil {
		t.Errorf("GetSnapshot on dst: %v", err)
	}
	if snap.Message != "pull test" {
		t.Errorf("snap.Message = %q, want %q", snap.Message, "pull test")
	}
	// Verify chunk exists in destination.
	chunk, err := dstStore.GetChunk(context.Background(), chunkHash)
	if err != nil {
		t.Errorf("GetChunk on dst: %v", err)
	}
	if !bytes.Equal(chunk.Data, []byte("hello world chunk data")) {
		t.Errorf("chunk data mismatch")
	}
}

// TestPull_SkipsExistingObjects verifies pull skips objects already local.
func TestPull_SkipsExistingObjects(t *testing.T) {
	srcStore := memory.NewMemoryStorage()
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID, _ := makeTestSnapshot(t, srcStore, "skip pull", nil)
	setupBranchRef(t, srcStore, "main", snapID.Hash)

	if _, err := Push(context.Background(), srcStore, rfs, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Pre-populate destination with the same snapshot.
	dstStore := memory.NewMemoryStorage()
	defer dstStore.Close()
	makeTestSnapshot(t, dstStore, "skip pull", nil)
	setupBranchRef(t, dstStore, "main", snapID.Hash)

	stats, err := Pull(context.Background(), dstStore, rfs, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if stats.SnapshotsSkipped != 1 {
		t.Errorf("expected 1 snapshot skipped, got %d", stats.SnapshotsSkipped)
	}
}

// TestPush_PushRefDiverged verifies push fails when a remote ref points to a
// different target than the local ref.
func TestPush_PushRefDiverged(t *testing.T) {
	srcStore := memory.NewMemoryStorage()
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID1, _ := makeTestSnapshot(t, srcStore, "first", nil)
	setupBranchRef(t, srcStore, "main", snapID1.Hash)

	// First push succeeds.
	if _, err := Push(context.Background(), srcStore, rfs, ""); err != nil {
		t.Fatalf("first Push: %v", err)
	}

	// Now create a second snapshot and update local ref.
	snapID2, _ := makeTestSnapshot(t, srcStore, "second", &snapID1)
	setupBranchRef(t, srcStore, "main", snapID2.Hash)

	// Manually set remote ref to a different target (simulating divergence).
	// We'll push snapID2 first (so the snapshot exists on remote), then
	// manually change the remote ref to snapID1 to simulate divergence.
	if err := pushSnapshot(context.Background(), srcStore, rfs, snapID2); err != nil {
		t.Fatalf("pushSnapshot: %v", err)
	}
	// Write the OLD ref target to remote (simulating another client pushed snapID1).
	refPath := refRemotePath("heads/main")
	if err := rfs.Write(refPath, bytes.NewReader([]byte(snapID1.Hash.FullString()+"\n"))); err != nil {
		t.Fatalf("write diverged ref: %v", err)
	}

	// Now push should fail with ref diverged.
	_, err := Push(context.Background(), srcStore, rfs, "")
	if err == nil {
		t.Fatal("expected Push to fail with ref diverged, got nil")
	}
}

// TestPull_RefDivergedSavedAsRemote verifies pull saves diverged refs as <name>.remote.
func TestPull_RefDivergedSavedAsRemote(t *testing.T) {
	srcStore := memory.NewMemoryStorage()
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID1, _ := makeTestSnapshot(t, srcStore, "local version", nil)
	snapID2, _ := makeTestSnapshot(t, srcStore, "remote version", nil)
	setupBranchRef(t, srcStore, "main", snapID2.Hash)

	// Push the remote version.
	if _, err := Push(context.Background(), srcStore, rfs, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Destination has a different ref target (diverged).
	dstStore := memory.NewMemoryStorage()
	defer dstStore.Close()
	makeTestSnapshot(t, dstStore, "local version", nil)
	setupBranchRef(t, dstStore, "main", snapID1.Hash)

	stats, err := Pull(context.Background(), dstStore, rfs, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if stats.RefsDiverged != 1 {
		t.Errorf("expected 1 ref diverged, got %d", stats.RefsDiverged)
	}

	// Verify the .remote ref was saved.
	remoteRef, err := dstStore.GetRef(context.Background(), "heads/main.remote")
	if err != nil {
		t.Errorf("expected heads/main.remote ref, got error: %v", err)
	}
	if remoteRef.Target != snapID2.Hash {
		t.Errorf("remote ref target = %x, want %x", remoteRef.Target, snapID2.Hash)
	}
}

// TestPush_BranchScoped verifies --branch flag limits push scope.
func TestPush_BranchScoped(t *testing.T) {
	store := memory.NewMemoryStorage()
	defer store.Close()
	rfs := NewMockRemoteFS()

	// Create two branches with different snapshots.
	snapID1, _ := makeTestSnapshot(t, store, "branch-a", nil)
	setupBranchRef(t, store, "main", snapID1.Hash)

	snapID2, _ := makeTestSnapshot(t, store, "branch-b", nil)
	refB := &core.Reference{Name: "heads/feature", Type: core.RefTypeBranch, Target: snapID2.Hash}
	if err := store.SetRef(context.Background(), "heads/feature", refB); err != nil {
		t.Fatalf("SetRef feature: %v", err)
	}

	// Push only the feature branch.
	stats, err := Push(context.Background(), store, rfs, "feature")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if stats.SnapshotsUploaded != 1 {
		t.Errorf("expected 1 snapshot (feature only), got %d", stats.SnapshotsUploaded)
	}
	// main's snapshot should NOT be on remote.
	snap1Path := snapshotRemotePath(snapID1)
	if _, err := rfs.Stat(snap1Path); err == nil {
		t.Error("main snapshot should not be on remote (branch-scoped push)")
	}
	// feature's snapshot SHOULD be on remote.
	snap2Path := snapshotRemotePath(snapID2)
	if _, err := rfs.Stat(snap2Path); err != nil {
		t.Error("feature snapshot should be on remote")
	}
}
