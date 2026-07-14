package porcelain

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

// countSnapshotDiff returns the number of files that differ (added, removed,
// or content-changed) between two snapshots. Either snapshot may be nil.
func countSnapshotDiff(from, to *core.Snapshot) int {
	if from == nil && to == nil {
		return 0
	}
	if from == nil {
		return len(to.Files)
	}
	if to == nil {
		return len(from.Files)
	}
	fromFiles := make(map[string]core.FileEntry)
	for _, f := range from.Files {
		fromFiles[f.Path] = f
	}
	count := 0
	seen := make(map[string]bool)
	for _, f := range to.Files {
		seen[f.Path] = true
		if prev, ok := fromFiles[f.Path]; !ok {
			count++
		} else if prev.Hash != f.Hash {
			count++
		}
	}
	for p := range fromFiles {
		if !seen[p] {
			count++
		}
	}
	return count
}

// ResolveHeadSnapshot returns the HEAD snapshot, or nil if none exists.
//
// When HEAD is a symbolic reference to a branch, the branch's target snapshot
// is returned. Storer.GetRef is contractually required to fully resolve
// symrefs: a GetRef("HEAD") call on a symref HEAD must return a Reference
// whose Target is the final snapshot hash (not the intermediate branch ref).
// This matches the documented contract of storage.Storer.GetRef and the
// filesystem backend's implementation in
// internal/storage/backends/filesystem/ref.go. Callers therefore never need
// a second GetRef to chase symrefs. resolveHead (in resolve.go) relies on
// the same contract; the two functions intentionally use a single GetRef.
func ResolveHeadSnapshot(ctx context.Context, store storage.Storer) *core.Snapshot {
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return nil
	}
	if headRef.Target.IsZero() {
		return nil
	}
	snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: headRef.Target})
	if err != nil {
		return nil
	}
	return snap
}

// SnapshotFileDiff diffs the given snapshot against its predecessor, returning
// the added, modified, and deleted file sets. When the snapshot has no
// predecessor (initial snapshot), every file is treated as added.
//
// Modification is detected by comparing file Hash (BLAKE3), consistent with
// countSnapshotDiff. Hash changes iff size or chunk list changes, so this is
// equivalent to comparing (Size, Chunks) but simpler.
func SnapshotFileDiff(ctx context.Context, store storage.Storer, snapshot *core.Snapshot) (added []core.FileEntry, modified []core.FileEntry, deleted []string, err error) {
	currFiles := make(map[string]core.FileEntry)
	for _, f := range snapshot.Files {
		currFiles[f.Path] = f
	}

	// Get previous snapshot files.
	var prevFiles map[string]core.FileEntry
	if snapshot.PrevID != nil {
		prevSnap, err := store.GetSnapshot(ctx, *snapshot.PrevID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read previous snapshot: %w", err)
		}
		prevFiles = make(map[string]core.FileEntry)
		for _, f := range prevSnap.Files {
			prevFiles[f.Path] = f
		}
	}

	// Find added and modified.
	for _, f := range snapshot.Files {
		if prevFiles == nil {
			added = append(added, f)
			continue
		}
		if prev, ok := prevFiles[f.Path]; !ok {
			added = append(added, f)
		} else if prev.Hash != f.Hash {
			modified = append(modified, f)
		}
	}

	// Find deleted.
	if prevFiles != nil {
		for p := range prevFiles {
			if _, ok := currFiles[p]; !ok {
				deleted = append(deleted, p)
			}
		}
	}

	return added, modified, deleted, nil
}

// CountSnapshotChanges loads the snapshot referenced by summary and returns
// the added/modified/deleted file counts relative to its parent. Errors are
// logged and zero counts are returned so that a single failure does not abort
// the whole log listing.
func CountSnapshotChanges(ctx context.Context, store storage.Storer, summary *core.SnapshotSummary) (added, modified, deleted int) {
	snapshot, err := store.GetSnapshot(ctx, summary.ID)
	if err != nil {
		slog.Warn("load snapshot for changes", "snapshot", summary.ShortID(), "error", err)
		return 0, 0, 0
	}
	a, m, d, err := SnapshotFileDiff(ctx, store, snapshot)
	if err != nil {
		slog.Warn("compute snapshot changes failed", "snapshot", snapshot.ShortID(), "error", err)
		return 0, 0, 0
	}
	return len(a), len(m), len(d)
}

// UndoLastSave reverts the last save operation by moving HEAD back to the
// previous snapshot. The undone snapshot becomes unreachable (will be
// collected by gc). It refuses if there are uncommitted workspace changes.
//
// If HEAD is a symbolic reference to a branch, the branch's target is moved
// back. If HEAD is detached, HEAD's own target is moved back. The workspace
// files are not touched; the index is rebuilt from the previous snapshot so
// that subsequent status/save operations reflect the new HEAD.
//
// Workspace sync: the on-disk files still reflect the undone snapshot. To
// make the workspace match the new HEAD, run `drift restore <prevID>` after
// a successful undo. Without this, `drift status` will report the workspace
// as "dirty" (the files match the old HEAD, not the new one).
func UndoLastSave(ctx context.Context, store storage.Storer, workDir string, cfg *core.CoreConfig) error {
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)

	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}

	currentHash := headRef.Target
	if currentHash.IsZero() {
		return ErrCannotUndo
	}

	currentSnap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: currentHash})
	if err != nil {
		return fmt.Errorf("get current snapshot: %w", err)
	}

	if currentSnap.PrevID == nil || currentSnap.PrevID.Hash.IsZero() {
		return ErrCannotUndo
	}

	summary, err := detectChangesNoLock(ctx, store, workDir, cfg)
	if err != nil {
		return fmt.Errorf("detect changes: %w", err)
	}
	if len(summary.Added) > 0 || len(summary.Modified) > 0 || len(summary.Deleted) > 0 {
		return ErrUncommittedChanges
	}

	prevHash := currentSnap.PrevID.Hash
	if headRef.SymRef != "" {
		branchRef, err := store.GetRef(ctx, headRef.SymRef)
		if err != nil {
			return fmt.Errorf("read branch ref: %w", err)
		}
		branchRef.Target = prevHash
		if err := store.SetRef(ctx, headRef.SymRef, branchRef); err != nil {
			return fmt.Errorf("update branch ref: %w", err)
		}
	} else {
		headRef.Target = prevHash
		if err := store.SetRef(ctx, "HEAD", headRef); err != nil {
			return fmt.Errorf("update HEAD: %w", err)
		}
	}

	if err := RebuildIndexFromSnapshot(ctx, store, core.SnapshotID{Hash: prevHash}); err != nil {
		// HEAD has already been moved to prevHash (the commit point).
		// The index is stale (still reflects the undone snapshot), but
		// the next save will re-chunk every file and rebuild it. Tell
		// the user explicitly so they know the undo succeeded but the
		// index needs a follow-up save to converge.
		return fmt.Errorf("undo committed (HEAD moved to %s) but index rebuild failed; run 'drift save' to rebuild the index: %w", prevHash.String(), err)
	}

	return nil
}
