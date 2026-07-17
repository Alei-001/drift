package remote

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/memory"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// recordingRemoteFS wraps a MockRemoteFS and records the order of Write
// calls so tests can verify upload ordering (e.g. chunks before snapshots).
type recordingRemoteFS struct {
	*MockRemoteFS
	mu       sync.Mutex
	writes   []string // paths in Write order
	writesMu sync.Mutex
}

func newRecordingRemoteFS() *recordingRemoteFS {
	return &recordingRemoteFS{MockRemoteFS: NewMockRemoteFS()}
}

func (r *recordingRemoteFS) Write(ctx context.Context, path string, reader io.Reader) error {
	// Record the path before delegating. Read the data so the reader is
	// consumed (MockRemoteFS.Write reads it).
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	r.writesMu.Lock()
	r.writes = append(r.writes, path)
	r.writesMu.Unlock()
	return r.MockRemoteFS.Write(ctx, path, strings.NewReader(string(data)))
}

func (r *recordingRemoteFS) Writes() []string {
	r.writesMu.Lock()
	defer r.writesMu.Unlock()
	cp := make([]string, len(r.writes))
	copy(cp, r.writes)
	return cp
}

// TestPush_ChunksBeforeSnapshots verifies that push uploads chunks before
// snapshots. This guarantees that when a snapshot becomes visible on the
// remote, every chunk it references is already there — a concurrent pull
// never sees a half-complete snapshot.
func TestPush_ChunksBeforeSnapshots(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer store.Close()
	rfs := newRecordingRemoteFS()

	snapID, chunkHash := makeTestSnapshot(t, store, "order test", nil)
	setupBranchRef(t, store, "main", snapID.Hash)

	if _, err := Push(context.Background(), store, rfs, "", SyncOptions{}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	writes := rfs.Writes()
	chunkPath := chunkRemotePath(chunkHash)
	snapPath := snapshotRemotePath(snapID)

	chunkIdx := -1
	snapIdx := -1
	for i, p := range writes {
		if p == chunkPath {
			chunkIdx = i
		}
		if p == snapPath {
			snapIdx = i
		}
	}

	if chunkIdx == -1 {
		t.Fatal("chunk was not uploaded")
	}
	if snapIdx == -1 {
		t.Fatal("snapshot was not uploaded")
	}
	if chunkIdx > snapIdx {
		t.Errorf("expected chunk (index %d) to be uploaded before snapshot (index %d), but it was not",
			chunkIdx, snapIdx)
	}
}

// TestPush_SnapshotReferencesMissingChunk verifies that push uploads all
// chunks referenced by snapshots, even when a chunk is shared across multiple
// snapshots (deduplication).
func TestPush_SnapshotReferencesMissingChunk(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer store.Close()
	rfs := newRecordingRemoteFS()

	// Create two snapshots sharing the same chunk (dedup).
	snapID1, chunkHash := makeTestSnapshot(t, store, "snap1", nil)
	setupBranchRef(t, store, "main", snapID1.Hash)

	// Second snapshot with the same chunk but different message.
	snap2 := &core.Snapshot{
		Message:   "snap2",
		Author:    "tester",
		Timestamp: 1700000001,
		PrevID:    &snapID1,
		Files: []core.FileEntry{
			{
				Path:   "test.txt",
				Mode:   0o644,
				Size:   24,
				Chunks: []core.Hash{chunkHash},
				Hash:   chunkHash,
			},
		},
		TotalSize: 24,
	}
	snap2Proto := core.SnapshotToProto(snap2, false)
	marshaled := mustMarshalProto(t, snap2Proto)
	snap2.ID = core.SnapshotID{Hash: core.Hash(mustBlake3(t, marshaled))}
	store.Snapshots.PutSnapshot(context.Background(), snap2)

	// Update main to point at snap2.
	setupBranchRef(t, store, "main", snap2.ID.Hash)

	stats, err := Push(context.Background(), store, rfs, "", SyncOptions{})
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// The shared chunk should be uploaded only once.
	if stats.ChunksUploaded != 1 {
		t.Errorf("expected 1 chunk uploaded (dedup), got %d", stats.ChunksUploaded)
	}
	// Both snapshots should be uploaded.
	if stats.SnapshotsUploaded != 2 {
		t.Errorf("expected 2 snapshots uploaded, got %d", stats.SnapshotsUploaded)
	}

	// Verify the chunk exists on remote.
	chunkPath := chunkRemotePath(chunkHash)
	if _, err := rfs.Stat(context.Background(), chunkPath); err != nil {
		t.Errorf("chunk should exist on remote: %v", err)
	}
}

// TestPush_EmptyStorePushesNothing verifies that pushing an empty store
// (no snapshots, no refs) produces zero-upload stats without error.
func TestPush_EmptyStorePushesNothing(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer store.Close()
	rfs := NewMockRemoteFS()

	stats, err := Push(context.Background(), store, rfs, "", SyncOptions{})
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if stats.SnapshotsUploaded != 0 || stats.ChunksUploaded != 0 || stats.RefsUpdated != 0 {
		t.Errorf("expected all-zero stats, got snap=%d chunk=%d refs=%d",
			stats.SnapshotsUploaded, stats.ChunksUploaded, stats.RefsUpdated)
	}
}

// TestPush_DryRunDoesNotUpload verifies that PushDryRun does not upload
// anything to the remote while still reporting what would be uploaded.
func TestPush_DryRunDoesNotUpload(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer store.Close()
	rfs := newRecordingRemoteFS()

	snapID, _ := makeTestSnapshot(t, store, "dry-run test", nil)
	setupBranchRef(t, store, "main", snapID.Hash)

	stats, err := PushDryRun(context.Background(), store, rfs, "", SyncOptions{})
	if err != nil {
		t.Fatalf("PushDryRun failed: %v", err)
	}
	if stats.SnapshotsUploaded != 1 {
		t.Errorf("expected 1 snapshot reported, got %d", stats.SnapshotsUploaded)
	}
	if stats.ChunksUploaded != 1 {
		t.Errorf("expected 1 chunk reported, got %d", stats.ChunksUploaded)
	}

	// Nothing should have been actually written.
	if len(rfs.Writes()) != 0 {
		t.Errorf("expected 0 writes during dry-run, got %d", len(rfs.Writes()))
	}
}

// --- helpers for proto/marshal in tests ---

func mustMarshalProto(t *testing.T, m proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal proto: %v", err)
	}
	return data
}

func mustBlake3(t *testing.T, data []byte) [32]byte {
	t.Helper()
	return blake3.Sum256(data)
}
