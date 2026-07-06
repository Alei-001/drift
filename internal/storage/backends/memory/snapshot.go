package memory

import (
	"context"
	"fmt"
	"sort"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
)

// GetSnapshot retrieves a snapshot.
func (ms *MemoryStorage) GetSnapshot(ctx context.Context, id core.SnapshotID) (*core.Snapshot, error) {
	v, ok := ms.snapshots[id.Hash.FullString()]
	if !ok {
		return nil, fmt.Errorf("get snapshot %s: %w", id.Hash.FullString(), storage.ErrNotFound)
	}
	return storage.CloneSnapshot(v), nil
}

// PutSnapshot stores a snapshot and caches its lightweight manifest.
func (ms *MemoryStorage) PutSnapshot(ctx context.Context, snapshot *core.Snapshot) error {
	key := snapshot.ID.Hash.FullString()
	ms.snapshots[key] = storage.CloneSnapshot(snapshot)
	ms.manifests[key] = cloneManifest(core.SnapshotToManifest(snapshot))
	return nil
}

// DeleteSnapshot removes a snapshot and its manifest. It is idempotent.
func (ms *MemoryStorage) DeleteSnapshot(ctx context.Context, id core.SnapshotID) error {
	key := id.Hash.FullString()
	delete(ms.snapshots, key)
	delete(ms.manifests, key)
	return nil
}

// ListSnapshots lists all snapshots via lightweight manifests, sorted by
// timestamp descending, with optional limit/offset pagination. Returns
// snapshot summaries without file lists — call GetSnapshot for full details.
func (ms *MemoryStorage) ListSnapshots(ctx context.Context, opts *storage.ListOptions) ([]*core.SnapshotSummary, error) {
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
