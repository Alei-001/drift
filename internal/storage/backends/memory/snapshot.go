package memory

import (
	"context"
	"fmt"
	"sort"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// GetSnapshot retrieves a snapshot and verifies its integrity by recomputing
// the BLAKE3 hash of its marshaled proto (with IdHash omitted), mirroring the
// filesystem backend. A mismatch indicates corruption and returns
// storage.ErrCorrupted so callers can handle it uniformly.
func (ms *MemoryStorage) GetSnapshot(ctx context.Context, id core.SnapshotID) (*core.Snapshot, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	v, ok := ms.snapshots[id.Hash.FullString()]
	if !ok {
		return nil, fmt.Errorf("get snapshot %s: %w", id.Hash.FullString(), storage.ErrNotFound)
	}
	clone := storage.CloneSnapshot(v)
	// Verify integrity: recompute the content hash (BLAKE3 of the marshaled
	// proto with IdHash omitted) and compare to the requested ID. This
	// catches in-memory corruption or a snapshot stored under the wrong key.
	p := core.SnapshotToProto(clone, false)
	// Deterministic: true ensures map fields (Extra) are serialized in
	// sorted key order, matching the hash computed in CreateSnapshot.
	// Without this, snapshots with 2+ Extra entries would intermittently
	// fail integrity verification due to non-deterministic map iteration.
	recomputed, err := proto.MarshalOptions{Deterministic: true}.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("re-marshal snapshot %s: %w", id.Hash.FullString(), storage.ErrCorrupted)
	}
	if core.Hash(blake3.Sum256(recomputed)) != id.Hash {
		return nil, fmt.Errorf("snapshot %s integrity check failed: %w", id.Hash.FullString(), storage.ErrCorrupted)
	}
	return clone, nil
}

// PutSnapshot stores a snapshot and caches its lightweight manifest.
func (ms *MemoryStorage) PutSnapshot(ctx context.Context, snapshot *core.Snapshot) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	key := snapshot.ID.Hash.FullString()
	ms.snapshots[key] = storage.CloneSnapshot(snapshot)
	ms.manifests[key] = cloneManifest(core.SnapshotToManifest(snapshot))
	return nil
}

// DeleteSnapshot removes a snapshot and its manifest. It is idempotent.
func (ms *MemoryStorage) DeleteSnapshot(ctx context.Context, id core.SnapshotID) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	key := id.Hash.FullString()
	delete(ms.snapshots, key)
	delete(ms.manifests, key)
	return nil
}

// ListSnapshots lists all snapshots via lightweight manifests, sorted by
// timestamp descending, with optional limit/offset pagination. Returns
// snapshot summaries without file lists — call GetSnapshot for full details.
func (ms *MemoryStorage) ListSnapshots(ctx context.Context, opts *storage.ListOptions) ([]*core.SnapshotSummary, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	var summaries []*core.SnapshotSummary
	for _, m := range ms.manifests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		summaries = append(summaries, core.ManifestToSummary(m))
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Timestamp > summaries[j].Timestamp
	})

	return storage.ApplySummaryPagination(summaries, opts), nil
}
