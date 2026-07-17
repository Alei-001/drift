package remote

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/memory"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// makeTestSnapshot builds a snapshot with one file entry referencing one chunk.
// The snapshot is stored in the given store. Returns the snapshot and chunk hash.
func makeTestSnapshot(t *testing.T, st *store.StoreSet, msg string, prevID *core.SnapshotID) (core.SnapshotID, core.Hash) {
	t.Helper()
	ctx := context.Background()

	chunkData := []byte("hello world chunk data")
	chunkHash := core.Hash(blake3.Sum256(chunkData))
	chunk := &core.Chunk{
		Hash:  chunkHash,
		Size:  uint32(len(chunkData)),
		Data:  chunkData,
		Flags: core.ChunkFlagNone,
	}
	if err := st.Chunks.PutChunk(ctx, chunk); err != nil {
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

	// Compute the real snapshot ID = BLAKE3(proto without IdHash),
	// matching porcelain.CreateSnapshot's ID computation so that
	// pullSnapshot's integrity check passes.
	snapProto := core.SnapshotToProto(snap, false)
	marshaled, err := proto.Marshal(snapProto)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	snap.ID = core.SnapshotID{Hash: core.Hash(blake3.Sum256(marshaled))}
	if err := st.Snapshots.PutSnapshot(ctx, snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}
	return snap.ID, chunkHash
}

func setupBranchRef(t *testing.T, st *store.StoreSet, branch string, target core.Hash) {
	t.Helper()
	ctx := context.Background()
	ref := &core.Reference{
		Name:   "heads/" + branch,
		Type:   core.RefTypeBranch,
		Target: target,
	}
	if err := st.Refs.SetRef(ctx, "heads/"+branch, ref); err != nil {
		t.Fatalf("SetRef %s: %v", branch, err)
	}
	headRef := &core.Reference{
		Name:   "HEAD",
		SymRef: "heads/" + branch,
	}
	if err := st.Refs.SetRef(ctx, "HEAD", headRef); err != nil {
		t.Fatalf("SetRef HEAD: %v", err)
	}
}

// TestPush_PushesSnapshotAndChunk verifies push uploads snapshots, manifests,
// and chunks to the remote.
func TestPush_PushesSnapshotAndChunk(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer st.Close()
	rfs := NewMockRemoteFS()

	snapID, chunkHash := makeTestSnapshot(t, st, "test message", nil)
	setupBranchRef(t, st, "main", snapID.Hash)

	stats, err := Push(context.Background(), st, rfs, "", SyncOptions{})
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
	if _, err := rfs.Stat(context.Background(), snapPath); err != nil {
		t.Errorf("remote snapshot not found: %v", err)
	}
	// Verify manifest exists on remote.
	manifestPath := manifestRemotePath(snapID)
	if _, err := rfs.Stat(context.Background(), manifestPath); err != nil {
		t.Errorf("remote manifest not found: %v", err)
	}
	// Verify chunk exists on remote.
	chunkPath := chunkRemotePath(chunkHash)
	if _, err := rfs.Stat(context.Background(), chunkPath); err != nil {
		t.Errorf("remote chunk not found: %v", err)
	}
}

// TestPush_SkipsExistingObjects verifies push skips objects already on the remote.
func TestPush_SkipsExistingObjects(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer st.Close()
	rfs := NewMockRemoteFS()

	snapID, chunkHash := makeTestSnapshot(t, st, "skip test", nil)
	setupBranchRef(t, st, "main", snapID.Hash)

	// First push.
	_, err := Push(context.Background(), st, rfs, "", SyncOptions{})
	if err != nil {
		t.Fatalf("first Push: %v", err)
	}
	// Second push should skip everything.
	stats, err := Push(context.Background(), st, rfs, "", SyncOptions{})
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
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID, chunkHash := makeTestSnapshot(t, srcStore, "pull test", nil)
	setupBranchRef(t, srcStore, "main", snapID.Hash)

	// Push from source to remote.
	if _, err := Push(context.Background(), srcStore, rfs, "", SyncOptions{}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Destination store: empty.
	dstStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer dstStore.Close()

	stats, err := Pull(context.Background(), dstStore, rfs, "", SyncOptions{})
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
	snap, err := dstStore.Snapshots.GetSnapshot(context.Background(), snapID)
	if err != nil {
		t.Errorf("GetSnapshot on dst: %v", err)
	}
	if snap.Message != "pull test" {
		t.Errorf("snap.Message = %q, want %q", snap.Message, "pull test")
	}
	// Verify chunk exists in destination.
	chunk, err := dstStore.Chunks.GetChunk(context.Background(), chunkHash)
	if err != nil {
		t.Errorf("GetChunk on dst: %v", err)
	}
	if !bytes.Equal(chunk.Data, []byte("hello world chunk data")) {
		t.Errorf("chunk data mismatch")
	}
}

// TestPull_SkipsExistingObjects verifies pull skips objects already local.
func TestPull_SkipsExistingObjects(t *testing.T) {
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID, _ := makeTestSnapshot(t, srcStore, "skip pull", nil)
	setupBranchRef(t, srcStore, "main", snapID.Hash)

	if _, err := Push(context.Background(), srcStore, rfs, "", SyncOptions{}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Pre-populate destination with the same snapshot.
	dstStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer dstStore.Close()
	makeTestSnapshot(t, dstStore, "skip pull", nil)
	setupBranchRef(t, dstStore, "main", snapID.Hash)

	stats, err := Pull(context.Background(), dstStore, rfs, "", SyncOptions{})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if stats.SnapshotsSkipped != 1 {
		t.Errorf("expected 1 snapshot skipped, got %d", stats.SnapshotsSkipped)
	}
}

// TestPush_PushRefDiverged verifies push fails when a remote ref points to a
// target that is NOT an ancestor of the local target (true divergence).
func TestPush_PushRefDiverged(t *testing.T) {
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	// Create a base snapshot, then two divergent children.
	baseSnapID, _ := makeTestSnapshot(t, srcStore, "base", nil)
	snapID1, _ := makeTestSnapshot(t, srcStore, "first branch", &baseSnapID)
	snapID2, _ := makeTestSnapshot(t, srcStore, "second branch", &baseSnapID)

	// Push base + snapID1, set remote main = snapID1.
	setupBranchRef(t, srcStore, "main", snapID1.Hash)
	if _, err := Push(context.Background(), srcStore, rfs, "", SyncOptions{}); err != nil {
		t.Fatalf("first Push: %v", err)
	}

	// Push snapID2's snapshot object to remote (so it exists there), but
	// keep remote ref at snapID1.
	if err := pushSnapshot(context.Background(), srcStore, rfs, snapID2); err != nil {
		t.Fatalf("pushSnapshot: %v", err)
	}

	// Now switch local main to snapID2 (divergent from remote's snapID1:
	// neither is an ancestor of the other).
	setupBranchRef(t, srcStore, "main", snapID2.Hash)

	// Push should fail — snapID2 and snapID1 are siblings, not fast-forward.
	_, err := Push(context.Background(), srcStore, rfs, "", SyncOptions{})
	if err == nil {
		t.Fatal("expected Push to fail with ref diverged, got nil")
	}
}

// TestPush_FastForward verifies push succeeds when the remote ref target is an
// ancestor of the local target (local is simply ahead).
func TestPush_FastForward(t *testing.T) {
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	// Create base, push it.
	baseSnapID, _ := makeTestSnapshot(t, srcStore, "base", nil)
	setupBranchRef(t, srcStore, "main", baseSnapID.Hash)
	if _, err := Push(context.Background(), srcStore, rfs, "", SyncOptions{}); err != nil {
		t.Fatalf("first Push: %v", err)
	}

	// Create a child snapshot and update local main.
	childSnapID, _ := makeTestSnapshot(t, srcStore, "child", &baseSnapID)
	setupBranchRef(t, srcStore, "main", childSnapID.Hash)

	// Push should fast-forward the remote ref (base is ancestor of child).
	stats, err := Push(context.Background(), srcStore, rfs, "", SyncOptions{})
	if err != nil {
		t.Fatalf("Push fast-forward failed: %v", err)
	}
	if stats.RefsUpdated != 1 {
		t.Errorf("expected 1 ref updated (fast-forward), got %d", stats.RefsUpdated)
	}

	// Verify remote ref now points to the child.
	rc, err := rfs.Read(context.Background(), refRemotePath("heads/main"))
	if err != nil {
		t.Fatalf("read remote ref: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if strings.TrimSpace(string(data)) != childSnapID.Hash.FullString() {
		t.Errorf("remote ref = %s, want %s (fast-forward)", strings.TrimSpace(string(data)), childSnapID.Hash.FullString())
	}
}

// TestPull_FastForward verifies pull fast-forwards the local ref when the
// remote is ahead (local target is an ancestor of remote target).
func TestPull_FastForward(t *testing.T) {
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	// Source: base → child (child is ahead).
	baseSnapID, _ := makeTestSnapshot(t, srcStore, "base", nil)
	childSnapID, _ := makeTestSnapshot(t, srcStore, "child", &baseSnapID)
	setupBranchRef(t, srcStore, "main", childSnapID.Hash)

	// Push to remote.
	if _, err := Push(context.Background(), srcStore, rfs, "", SyncOptions{}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Destination: has base, main points to base (behind remote).
	dstStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer dstStore.Close()
	makeTestSnapshot(t, dstStore, "base", nil)
	setupBranchRef(t, dstStore, "main", baseSnapID.Hash)

	stats, err := Pull(context.Background(), dstStore, rfs, "", SyncOptions{})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if stats.RefsUpdated != 1 {
		t.Errorf("expected 1 ref updated (fast-forward), got %d", stats.RefsUpdated)
	}
	if stats.RefsDiverged != 0 {
		t.Errorf("expected 0 diverged, got %d", stats.RefsDiverged)
	}

	// Verify local ref now points to child.
	localRef, err := dstStore.Refs.GetRef(context.Background(), "heads/main")
	if err != nil {
		t.Fatalf("GetRef: %v", err)
	}
	if localRef.Target != childSnapID.Hash {
		t.Errorf("local ref = %s, want %s (fast-forward)", localRef.Target.FullString(), childSnapID.Hash.FullString())
	}
}

// TestPull_RefDivergedSavedAsRemote verifies pull saves diverged refs as <name>.remote.
func TestPull_RefDivergedSavedAsRemote(t *testing.T) {
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID1, _ := makeTestSnapshot(t, srcStore, "local version", nil)
	snapID2, _ := makeTestSnapshot(t, srcStore, "remote version", nil)
	setupBranchRef(t, srcStore, "main", snapID2.Hash)

	// Push the remote version.
	if _, err := Push(context.Background(), srcStore, rfs, "", SyncOptions{}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Destination has a different ref target (diverged).
	dstStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer dstStore.Close()
	makeTestSnapshot(t, dstStore, "local version", nil)
	setupBranchRef(t, dstStore, "main", snapID1.Hash)

	stats, err := Pull(context.Background(), dstStore, rfs, "", SyncOptions{})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if stats.RefsDiverged != 1 {
		t.Errorf("expected 1 ref diverged, got %d", stats.RefsDiverged)
	}

	// Verify the .remote ref was saved.
	remoteRef, err := dstStore.Refs.GetRef(context.Background(), "heads/main.remote")
	if err != nil {
		t.Errorf("expected heads/main.remote ref, got error: %v", err)
	}
	if remoteRef.Target != snapID2.Hash {
		t.Errorf("remote ref target = %x, want %x", remoteRef.Target, snapID2.Hash)
	}
}

// TestPush_BranchScoped verifies --branch flag limits push scope.
func TestPush_BranchScoped(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer st.Close()
	rfs := NewMockRemoteFS()

	// Create two branches with different snapshots.
	snapID1, _ := makeTestSnapshot(t, st, "branch-a", nil)
	setupBranchRef(t, st, "main", snapID1.Hash)

	snapID2, _ := makeTestSnapshot(t, st, "branch-b", nil)
	refB := &core.Reference{Name: "heads/feature", Type: core.RefTypeBranch, Target: snapID2.Hash}
	if err := st.Refs.SetRef(context.Background(), "heads/feature", refB); err != nil {
		t.Fatalf("SetRef feature: %v", err)
	}

	// Push only the feature branch.
	stats, err := Push(context.Background(), st, rfs, "feature", SyncOptions{})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if stats.SnapshotsUploaded != 1 {
		t.Errorf("expected 1 snapshot (feature only), got %d", stats.SnapshotsUploaded)
	}
	// main's snapshot should NOT be on remote.
	snap1Path := snapshotRemotePath(snapID1)
	if _, err := rfs.Stat(context.Background(), snap1Path); err == nil {
		t.Error("main snapshot should not be on remote (branch-scoped push)")
	}
	// feature's snapshot SHOULD be on remote.
	snap2Path := snapshotRemotePath(snapID2)
	if _, err := rfs.Stat(context.Background(), snap2Path); err != nil {
		t.Error("feature snapshot should be on remote")
	}
}
