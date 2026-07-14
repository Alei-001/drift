package porcelain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

// AutoSavePrefix is the message prefix that identifies auto-saved snapshots
// (created by 'drift watch'). Commands that filter or label auto-saves (e.g.
// 'log' hiding them by default, 'gc --keep-auto' preserving them) share this
// constant so the prefix has a single source of truth.
const AutoSavePrefix = "auto -"

// GCReport describes the outcome of a garbage collection pass.
type GCReport struct {
	SnapshotsRemoved int
	ChunksRemoved    int
	FreedBytes       int64
	AutoKept         int
	LoosePacked      int
	PacksRewritten   int
}

// collectRoots gathers the target hashes of all branch and tag references,
// plus the HEAD ref when it is detached (SymRef == "" with a non-zero
// Target). References with a zero target (e.g. a freshly initialized branch
// with no commits) are skipped.
//
// The detached-HEAD case is essential: when HEAD points directly at a
// snapshot (rather than symbolically at a branch), that snapshot is a root
// even if no branch or tag references it. Without this, GC would collect a
// snapshot the user is actively viewing, severing the only reference to it.
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
	// A detached HEAD (SymRef == "") with a non-zero target is itself a
	// root: the snapshot it points at may not be referenced by any branch
	// or tag, so without this it would be incorrectly collected.
	headRef, err := store.GetRef(ctx, "HEAD")
	if err == nil && headRef.SymRef == "" && !headRef.Target.IsZero() {
		roots = append(roots, headRef.Target)
	}
	return roots, nil
}

// bfsReachable performs a BFS from all branch, tag, and detached-HEAD
// references by following PrevID pointers backwards, returning the visited
// set (a hash is reachable iff it was enqueued). When withCache is true, the
// returned snapCache holds the full Snapshot objects fetched during BFS
// (keyed by hash), letting callers like CollectGarbage avoid a second round
// of GetSnapshot calls. When withCache is false, snapCache is nil and the
// BFS does not retain the full Snapshot objects, reducing memory for callers
// that only need the reachable set (e.g. pruneAutoSnapshots,
// CountUnreachableSnapshots).
//
// This is the single reachability primitive; the previous separate
// computeReachableSet and computeReachability functions were near-identical
// copies whose only difference was whether they populated the cache. Folding
// them into one function eliminates the risk of the two code paths diverging
// on what "reachable" means.
func bfsReachable(ctx context.Context, store storage.Storer, withCache bool) (visited map[core.Hash]bool, snapCache map[core.Hash]*core.Snapshot, err error) {
	roots, rerr := collectRoots(ctx, store)
	if rerr != nil {
		return nil, nil, rerr
	}

	visited = make(map[core.Hash]bool)
	if withCache {
		snapCache = make(map[core.Hash]*core.Snapshot)
	}
	queue := make([]core.Hash, 0, len(roots))

	for _, h := range roots {
		if !h.IsZero() && !visited[h] {
			visited[h] = true
			queue = append(queue, h)
		}
	}

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		h := queue[0]
		queue = queue[1:]

		snap, gerr := store.GetSnapshot(ctx, core.SnapshotID{Hash: h})
		if gerr != nil {
			// Snapshot referenced by a ref but missing from storage: skip
			// it but continue traversing the rest of the graph. Only
			// ErrNotFound is skippable; any other error (e.g. corruption
			// or an I/O failure) must propagate so the caller can decide.
			if errors.Is(gerr, storage.ErrNotFound) {
				continue
			}
			return nil, nil, fmt.Errorf("get snapshot %s: %w", h.FullString(), gerr)
		}
		if withCache {
			snapCache[h] = snap
		}

		if snap.PrevID != nil && !snap.PrevID.Hash.IsZero() && !visited[snap.PrevID.Hash] {
			visited[snap.PrevID.Hash] = true
			queue = append(queue, snap.PrevID.Hash)
		}
	}

	return visited, snapCache, nil
}

// computeReachableSet returns the set of snapshot hashes reachable from all
// branch, tag, and detached-HEAD references by following PrevID pointers
// backwards. It is a thin wrapper around bfsReachable for callers that do
// not need the snapshot cache.
func computeReachableSet(ctx context.Context, store storage.Storer) (map[core.Hash]bool, error) {
	visited, _, err := bfsReachable(ctx, store, false)
	return visited, err
}

// computeReachability returns the reachable set, the full list of stored
// snapshots, and a cache of the full Snapshot objects fetched during BFS
// (keyed by hash). The cache lets callers like CollectGarbage avoid a second
// round of GetSnapshot calls when collecting reachable chunks.
func computeReachability(ctx context.Context, store storage.Storer) (map[core.Hash]bool, []*core.SnapshotSummary, map[core.Hash]*core.Snapshot, error) {
	visited, snapCache, err := bfsReachable(ctx, store, true)
	if err != nil {
		return nil, nil, nil, err
	}
	allSnapshots, err := store.ListSnapshots(ctx, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list snapshots: %w", err)
	}
	return visited, allSnapshots, snapCache, nil
}

// CollectGarbage removes snapshots and chunks that are no longer reachable
// from any branch or tag reference. When dryRun is true nothing is deleted;
// the report reflects what would be reclaimed. FreedBytes is computed in
// both modes (best-effort via GetChunk) and is an estimate when dryRun.
//
// keepAuto preserves the N most recent unreachable [auto] snapshots (those
// whose message starts with the auto-save prefix) from deletion, acting as
// a safety net against accidental data loss. Their chunks are also kept.
//
// GC does not touch workspace files, but it must not run concurrently with
// CreateSnapshot: a save in progress may be about to link a chunk that GC
// would otherwise delete as unreachable. Acquiring the workspace lock
// serializes GC against save/switch/restore, which are the only operations
// that add new chunks or snapshots.
func CollectGarbage(ctx context.Context, store storage.Storer, workDir string, dryRun bool, keepAuto int) (GCReport, error) {
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return GCReport{}, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)

	var report GCReport

	reachable, allSnapshots, snapCache, err := computeReachability(ctx, store)
	if err != nil {
		return report, err
	}

	// Partition snapshots into reachable and unreachable.
	var unreachable []*core.SnapshotSummary
	for _, snap := range allSnapshots {
		if !reachable[snap.ID.Hash] {
			unreachable = append(unreachable, snap)
		}
	}

	// keptAuto holds the hashes of unreachable [auto] snapshots that
	// --keep-auto preserves from deletion (most recent N by timestamp).
	keptAuto := selectKeptAutoSnapshots(unreachable, keepAuto)
	report.AutoKept = len(keptAuto)

	// --- Unreachable snapshots (except kept auto-saves) ---
	for _, snap := range unreachable {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if keptAuto[snap.ID.Hash] {
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
	// Read chunk lists from the snapshot cache populated during BFS when
	// possible, falling back to GetSnapshot only for snapshots that were
	// not visited during BFS (e.g. kept-auto snapshots that are
	// unreachable but preserved by --keep-auto).
	reachableChunks := make(map[core.Hash]bool)
	for _, snap := range allSnapshots {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if !reachable[snap.ID.Hash] && !keptAuto[snap.ID.Hash] {
			continue
		}
		var full *core.Snapshot
		if cached, ok := snapCache[snap.ID.Hash]; ok {
			full = cached
		} else {
			f, err := store.GetSnapshot(ctx, snap.ID)
			if err != nil {
				slog.Warn("failed to load snapshot for chunk reachability, skipping", "snapshot", snap.ID.Hash.FullString(), "error", err)
				continue
			}
			full = f
		}
		for _, f := range full.Files {
			for _, c := range f.Chunks {
				if !c.IsZero() {
					reachableChunks[c] = true
				}
			}
		}
	}

	idx, err := store.GetIndex(ctx)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return report, fmt.Errorf("get workspace index: %w", err)
		}
	} else {
		for _, e := range idx.Entries {
			for _, c := range e.Chunks {
				if !c.IsZero() {
					reachableChunks[c] = true
				}
			}
		}
	}

	if compactor, ok := store.(storage.ChunkCompactor); ok {
		cr, err := compactor.CompactChunks(ctx, reachableChunks, dryRun)
		if err != nil {
			return report, fmt.Errorf("compact chunks: %w", err)
		}
		report.ChunksRemoved = cr.LooseDeleted + cr.PackDeadRemoved
		report.FreedBytes = cr.FreedBytes
		report.LoosePacked = cr.LoosePacked
		report.PacksRewritten = cr.PacksRewritten
	} else {
		allChunks, err := store.ListChunks(ctx)
		if err != nil {
			return report, fmt.Errorf("list chunks: %w", err)
		}

		for _, ch := range allChunks {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			if reachableChunks[ch] {
				continue
			}
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
	}

	return report, nil
}

// selectKeptAutoSnapshots returns a set of snapshot hashes to preserve from
// deletion when --keep-auto is set. Among the unreachable snapshots, it
// selects those whose message starts with the auto-save prefix, sorts them
// newest-first by timestamp, and keeps at most keepAuto of them. When
// keepAuto <= 0 the result is always empty.
func selectKeptAutoSnapshots(unreachable []*core.SnapshotSummary, keepAuto int) map[core.Hash]bool {
	if keepAuto <= 0 || len(unreachable) == 0 {
		return nil
	}
	var autoSnaps []*core.SnapshotSummary
	for _, s := range unreachable {
		if strings.HasPrefix(s.Message, AutoSavePrefix) {
			autoSnaps = append(autoSnaps, s)
		}
	}
	if len(autoSnaps) == 0 {
		return nil
	}
	sort.Slice(autoSnaps, func(i, j int) bool {
		return autoSnaps[i].Timestamp > autoSnaps[j].Timestamp
	})
	kept := make(map[core.Hash]bool, keepAuto)
	for i := 0; i < keepAuto && i < len(autoSnaps); i++ {
		kept[autoSnaps[i].ID.Hash] = true
	}
	return kept
}

// CountUnreachableSnapshots returns the number of snapshots that are not
// reachable from any branch or tag reference.
//
// Like CollectGarbage, it acquires the workspace lock so it does not observe
// a half-applied save/switch/restore that would produce a misleading count.
func CountUnreachableSnapshots(ctx context.Context, store storage.Storer, workDir string) (int, error) {
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return 0, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)

	// CountUnreachableSnapshots only needs the reachable set and the full
	// snapshot list, not the snapshot cache, so it uses the lighter
	// computeReachableSet directly.
	reachable, err := computeReachableSet(ctx, store)
	if err != nil {
		return 0, err
	}
	allSnapshots, err := store.ListSnapshots(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("list snapshots: %w", err)
	}

	count := 0
	for _, snap := range allSnapshots {
		if !reachable[snap.ID.Hash] {
			count++
		}
	}
	return count, nil
}
