package porcelain

import (
	"context"
	"errors"
	"fmt"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
)

// GCReport describes the outcome of a garbage collection pass.
type GCReport struct {
	SnapshotsRemoved int
	ChunksRemoved    int
	FreedBytes       int64
}

// collectRoots gathers the target hashes of all branch and tag references.
// References with a zero target (e.g. a freshly initialized branch with no
// commits) are skipped.
func collectRoots(ctx context.Context, store storage.Storer) ([]core.Hash, error) {
	var roots []core.Hash
	for _, prefix := range []string{"heads/", "tags/"} {
		refs, err := store.ListRefs(ctx, prefix)
		if err != nil {
			return nil, fmt.Errorf("list refs %q: %w", prefix, err)
		}
		for _, ref := range refs {
			if ref.Target.IsZero() {
				continue
			}
			roots = append(roots, ref.Target)
		}
	}
	return roots, nil
}

// computeReachability performs a BFS from all branch and tag references to
// determine which snapshots are reachable. It returns the visited set (which
// doubles as the reachability set — a hash is reachable iff it was enqueued)
// and the full list of stored snapshots.
func computeReachability(ctx context.Context, store storage.Storer) (map[core.Hash]bool, []*core.Snapshot, error) {
	roots, err := collectRoots(ctx, store)
	if err != nil {
		return nil, nil, err
	}

	visited := make(map[core.Hash]bool)
	queue := make([]core.Hash, 0, len(roots))

	for _, h := range roots {
		if !h.IsZero() && !visited[h] {
			visited[h] = true
			queue = append(queue, h)
		}
	}

	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]

		snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: h})
		if err != nil {
			// Snapshot referenced by a ref but missing from storage: skip
			// it but continue traversing the rest of the graph.
			continue
		}

		if snap.PrevID != nil && !snap.PrevID.Hash.IsZero() && !visited[snap.PrevID.Hash] {
			visited[snap.PrevID.Hash] = true
			queue = append(queue, snap.PrevID.Hash)
		}
	}

	allSnapshots, err := store.ListSnapshots(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list snapshots: %w", err)
	}

	return visited, allSnapshots, nil
}

// CollectGarbage removes snapshots and chunks that are no longer reachable
// from any branch or tag reference. When dryRun is true nothing is deleted;
// the report reflects what would be reclaimed. FreedBytes is computed in
// both modes (best-effort via GetChunk) and is an estimate when dryRun.
//
// GC does not touch workspace files, but it must not run concurrently with
// CreateSnapshot: a save in progress may be about to link a chunk that GC
// would otherwise delete as unreachable. Acquiring the workspace lock
// serializes GC against save/switch/restore, which are the only operations
// that add new chunks or snapshots.
func CollectGarbage(ctx context.Context, store storage.Storer, workDir string, dryRun bool) (GCReport, error) {
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return GCReport{}, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)

	var report GCReport

	reachable, allSnapshots, err := computeReachability(ctx, store)
	if err != nil {
		return report, err
	}

	// --- Unreachable snapshots ---
	for _, snap := range allSnapshots {
		if reachable[snap.ID.Hash] {
			continue
		}
		report.SnapshotsRemoved++
		if !dryRun {
			if err := store.DeleteSnapshot(ctx, snap.ID); err != nil {
				return report, fmt.Errorf("delete snapshot %s: %w", snap.ID.Hash.FullString(), err)
			}
		}
	}

	// --- Unreferenced chunks ---
	// Collect chunk hashes referenced by reachable snapshots.
	reachableChunks := make(map[core.Hash]bool)
	for _, snap := range allSnapshots {
		if !reachable[snap.ID.Hash] {
			continue
		}
		for _, f := range snap.Files {
			for _, c := range f.Chunks {
				if !c.IsZero() {
					reachableChunks[c] = true
				}
			}
		}
	}

	// Include chunks referenced by the workspace index. The index may
	// reference chunks from snapshots that are otherwise unreachable
	// (e.g. after a partial restore or branch switch), and deleting them
	// would corrupt the index.
	idx, err := store.GetIndex(ctx)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return report, fmt.Errorf("get workspace index: %w", err)
		}
		// No index exists yet; nothing to mark as reachable.
	} else {
		for _, e := range idx.Entries {
			for _, c := range e.Chunks {
				if !c.IsZero() {
					reachableChunks[c] = true
				}
			}
		}
	}

	allChunks, err := store.ListChunks(ctx)
	if err != nil {
		return report, fmt.Errorf("list chunks: %w", err)
	}

	for _, ch := range allChunks {
		if reachableChunks[ch] {
			continue
		}
		// Accumulate freed bytes in both modes. If GetChunk fails, skip the
		// size contribution without aborting the pass.
		if chunk, gerr := store.GetChunk(ctx, ch); gerr == nil {
			report.FreedBytes += int64(chunk.Size)
		}
		report.ChunksRemoved++
		if !dryRun {
			if err := store.DeleteChunk(ctx, ch); err != nil {
				return report, fmt.Errorf("delete chunk %s: %w", ch.FullString(), err)
			}
		}
	}

	return report, nil
}

// CountUnreachableSnapshots returns the number of snapshots that are not
// reachable from any branch or tag reference.
func CountUnreachableSnapshots(ctx context.Context, store storage.Storer) (int, error) {
	reachable, allSnapshots, err := computeReachability(ctx, store)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, snap := range allSnapshots {
		if !reachable[snap.ID.Hash] {
			count++
		}
	}
	return count, nil
}
