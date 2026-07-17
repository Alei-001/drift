package remote

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/memory"
	"github.com/zeebo/blake3"
)

// failingChunkStorer wraps a ChunkStorer and forces ListChunks to return
// an error. All other methods delegate to the embedded ChunkStorer.
type failingChunkStorer struct {
	store.ChunkStorer
	listErr error
}

func (s *failingChunkStorer) ListChunks(ctx context.Context) ([]core.Hash, error) {
	return nil, s.listErr
}

// TestListLocalChunkHashes_ErrorPropagation verifies that a ListChunks failure
// is surfaced as an error from listLocalChunkHashes rather than being silently
// swallowed into an empty set. Silently swallowing would cause pull to
// re-download every chunk and could mask a real storage problem.
func TestListLocalChunkHashes_ErrorPropagation(t *testing.T) {
	ms := memory.NewMemoryStorage()
	ss := store.NewStoreSet(ms)
	ss.Chunks = &failingChunkStorer{ChunkStorer: ss.Chunks, listErr: errors.New("simulated ListChunks I/O failure")}
	defer ss.Close()

	_, err := listLocalChunkHashes(context.Background(), ss)
	if err == nil {
		t.Fatal("expected error from listLocalChunkHashes when ListChunks fails, got nil")
	}
	if !strings.Contains(err.Error(), "list local chunks") {
		t.Errorf("expected error wrapping 'list local chunks', got: %v", err)
	}
	if !strings.Contains(err.Error(), "simulated ListChunks I/O failure") {
		t.Errorf("expected underlying error preserved, got: %v", err)
	}
}

// TestListLocalChunkHashes_EmptyStoreReturnsEmptySet verifies the normal path:
// a store with no chunks returns an empty (non-nil) set without error.
func TestListLocalChunkHashes_EmptyStoreReturnsEmptySet(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer store.Close()

	result, err := listLocalChunkHashes(context.Background(), store)
	if err != nil {
		t.Fatalf("listLocalChunkHashes on empty store: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty set, got %d entries", len(result))
	}
}

// TestListLocalChunkHashes_ReturnsExistingChunks verifies that chunks stored
// in the local store are returned as a set.
func TestListLocalChunkHashes_ReturnsExistingChunks(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	defer store.Close()

	ctx := context.Background()
	chunkData := []byte("test chunk data")
	chunkHash := core.Hash(blake3.Sum256(chunkData))
	if err := store.Chunks.PutChunk(ctx, &core.Chunk{
		Hash: chunkHash,
		Size: uint32(len(chunkData)),
		Data: chunkData,
	}); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	result, err := listLocalChunkHashes(ctx, store)
	if err != nil {
		t.Fatalf("listLocalChunkHashes: %v", err)
	}
	if !result[chunkHash] {
		t.Errorf("expected chunk %x in set", chunkHash[:8])
	}
}

// TestPushRef_FastForwardCheckError verifies that when the isAncestor call
// inside pushRef fails (e.g. because the local snapshot chain is broken), the
// underlying error is surfaced — NOT collapsed into errRefDiverged. This
// regression guards P1-6: the user must distinguish a real divergence from a
// broken local chain.
func TestPushRef_FastForwardCheckError(t *testing.T) {
	// Source store: has a snapshot that will be pushed to the remote.
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID, _ := makeTestSnapshot(t, srcStore, "ancestor test", nil)

	// Push the snapshot to the mock remote so rfs.Stat passes.
	if err := pushSnapshot(context.Background(), srcStore, rfs, snapID); err != nil {
		t.Fatalf("pushSnapshot: %v", err)
	}

	// Write a ref file on the remote with a DIFFERENT target hash so the
	// fast-forward check is triggered.
	differentHash := core.Hash{0xAB, 0xCD, 0xEF}
	refBody := differentHash.FullString() + "\n"
	if err := rfs.Write(context.Background(), refRemotePath("heads/main"), strings.NewReader(refBody)); err != nil {
		t.Fatalf("write remote ref: %v", err)
	}

	// Local store: empty — does NOT have the snapshot for snapID.Hash.
	// isAncestor will call GetSnapshot and fail with ErrNotFound.
	localStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer localStore.Close()

	ref := &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: snapID.Hash,
	}
	_, err := pushRef(context.Background(), localStore, rfs, ref)
	if err == nil {
		t.Fatal("expected error from pushRef when isAncestor fails, got nil")
	}
	if errors.Is(err, errRefDiverged) {
		t.Errorf("expected underlying error, not errRefDiverged: %v", err)
	}
	if !strings.Contains(err.Error(), "fast-forward check") {
		t.Errorf("expected error containing 'fast-forward check', got: %v", err)
	}
}

// TestPushRef_NewRefWrites verifies the normal path: when no remote ref
// exists, pushRef writes it and returns updated=true.
func TestPushRef_NewRefWrites(t *testing.T) {
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID, _ := makeTestSnapshot(t, srcStore, "new ref test", nil)
	if err := pushSnapshot(context.Background(), srcStore, rfs, snapID); err != nil {
		t.Fatalf("pushSnapshot: %v", err)
	}

	ref := &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: snapID.Hash,
	}
	updated, err := pushRef(context.Background(), srcStore, rfs, ref)
	if err != nil {
		t.Fatalf("pushRef new ref: %v", err)
	}
	if !updated {
		t.Error("expected updated=true for new ref")
	}

	// Verify the ref was actually written.
	rc, err := rfs.Read(context.Background(), refRemotePath("heads/main"))
	if err != nil {
		t.Fatalf("read remote ref: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if strings.TrimSpace(string(data)) != snapID.Hash.FullString() {
		t.Errorf("remote ref = %s, want %s", strings.TrimSpace(string(data)), snapID.Hash.FullString())
	}
}

// TestPushRef_SameTargetSkips verifies that when the remote ref already has
// the same target, pushRef skips (returns updated=false, no error).
func TestPushRef_SameTargetSkips(t *testing.T) {
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	rfs := NewMockRemoteFS()

	snapID, _ := makeTestSnapshot(t, srcStore, "same target test", nil)
	if err := pushSnapshot(context.Background(), srcStore, rfs, snapID); err != nil {
		t.Fatalf("pushSnapshot: %v", err)
	}

	// Pre-write the ref with the same target.
	refBody := snapID.Hash.FullString() + "\n"
	if err := rfs.Write(context.Background(), refRemotePath("heads/main"), strings.NewReader(refBody)); err != nil {
		t.Fatalf("write remote ref: %v", err)
	}

	ref := &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: snapID.Hash,
	}
	updated, err := pushRef(context.Background(), srcStore, rfs, ref)
	if err != nil {
		t.Fatalf("pushRef same target: %v", err)
	}
	if updated {
		t.Error("expected updated=false for same target")
	}
}
