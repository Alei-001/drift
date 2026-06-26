package app

import (
	"fmt"
	"os"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
)

// GCOptions controls garbage collection behavior.
type GCOptions struct {
	// DryRun reports what would be deleted without actually removing files.
	DryRun bool
	// Verbose prints per-object deletion messages to stdout.
	Verbose bool
}

// GCResult summarizes a completed garbage collection.
type GCResult struct {
	ObjectsRemoved int
	BytesFreed     int64
}

// defaultGCAuto is the loose object count above which auto-GC triggers.
const defaultGCAuto = 1000

// gcAutoThreshold returns the configured gc.auto threshold, or the default.
func (a *App) gcAutoThreshold() int {
	if a.config != nil {
		v := a.config.Core.GCAuto
		if v > 0 {
			return v
		}
	}
	return defaultGCAuto
}

// GC removes unreachable objects from the store. Reachability is determined
// by walking the commit DAG from every ref (branches, tags, HEAD) and from
// every reflog entry, so objects needed by undo remain intact.
//
// The store lock is held for the entire operation — no concurrent writes.
func (a *App) GC(opts GCOptions) (*GCResult, error) {
	// 1. Collect all ref targets.
	refs, err := a.store.ListRefs()
	if err != nil {
		return nil, fmt.Errorf("list refs: %w", err)
	}
	startHashes := make([]string, 0, len(refs)+1)
	for _, hash := range refs {
		if hash != "" {
			startHashes = append(startHashes, hash)
		}
	}
	// HEAD may be a symref; ListRefs already dereferences it, but the raw
	// HEAD value could be a branch name. Add it just in case.
	if headHash, err := a.store.GetRef("HEAD"); err == nil && headHash != "" {
		startHashes = append(startHashes, headHash)
	}

	// 2. Collect reflog-referenced commit hashes.
	ops, err := a.ReadOperations()
	if err != nil {
		return nil, fmt.Errorf("read operations: %w", err)
	}
	for _, op := range ops {
		for _, ch := range op.RefChanges {
			if ch.Before != "" {
				startHashes = append(startHashes, ch.Before)
			}
			if ch.After != "" {
				startHashes = append(startHashes, ch.After)
			}
		}
	}

	// 3. Walk the DAG from every starting point to build the reachable set.
	reachable := core.CollectReachable(a.store, startHashes)

	// 4. List all objects on disk.
	entries, err := a.store.ListObjectEntries()
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}

	// 5. Identify unreachable objects.
	var toDelete []storage.ObjectEntry
	for _, e := range entries {
		if _, ok := reachable[e.Hash]; !ok {
			toDelete = append(toDelete, e)
		}
	}

	if len(toDelete) == 0 {
		return &GCResult{}, nil
	}

	// 6. Delete (or dry-run report).
	if opts.DryRun {
		var totalSize int64
		for _, e := range toDelete {
			totalSize += e.Size
			if opts.Verbose {
				fmt.Printf("would remove %s  %s\n", e.Type, e.Hash[:8])
			}
		}
		return &GCResult{
			ObjectsRemoved: len(toDelete),
			BytesFreed:     totalSize,
		}, nil
	}

	var result *GCResult
	err = a.store.WithLock(func() error {
		var removed int
		var freed int64
		for _, e := range toDelete {
			// Re-check existence under lock to avoid TOCTOU.
			if a.store.HasObject(e.Hash) {
				if err := a.store.DeleteObject(e.Hash); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to delete %s %s: %v\n", e.Type, e.Hash[:8], err)
					continue
				}
				removed++
				freed += e.Size
				if opts.Verbose {
					fmt.Printf("removed %s  %s\n", e.Type, e.Hash[:8])
				}
			}
		}
		result = &GCResult{
			ObjectsRemoved: removed,
			BytesFreed:     freed,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ShouldAutoGC returns true when the number of loose objects exceeds the
// gc.auto threshold. Callers should invoke autoGC() if this returns true.
func (a *App) ShouldAutoGC() bool {
	threshold := a.gcAutoThreshold()
	if threshold <= 0 {
		return false // disabled
	}
	entries, err := a.store.ListObjectEntries()
	if err != nil {
		return false
	}
	return len(entries) >= threshold
}

// autoGC runs garbage collection in the background and logs warnings to
// stderr on failure. It is non-fatal — the operation that triggered it
// continues regardless.
func (a *App) autoGC() {
	result, err := a.GC(GCOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: auto gc failed: %v\n", err)
		return
	}
	if result.ObjectsRemoved > 0 {
		fmt.Fprintf(os.Stderr, "Auto GC: removed %d object(s) (%d bytes freed)\n",
			result.ObjectsRemoved, result.BytesFreed)
	}
}
