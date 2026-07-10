package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/schollz/progressbar/v3"
)

// concurrency is the maximum number of concurrent chunk transfers
// during push and pull. Higher values improve throughput on high-latency
// links (WebDAV) but may overwhelm low-bandwidth connections.
const concurrency = 8

// SyncStats reports the outcome of a push or pull operation.
type SyncStats struct {
	SnapshotsUploaded int
	SnapshotsSkipped  int
	ManifestsUploaded int
	ChunksUploaded    int
	ChunksSkipped     int
	RefsUpdated       int
	RefsDiverged      int // pull only: refs saved as <name>.remote
	IndexRebuilt      bool
	BranchTipChanged  string // branch name whose tip advanced ("" if none)
}

// LsRemote lists all refs on a remote without downloading objects.
func LsRemote(ctx context.Context, rfs RemoteFS) ([]*core.Reference, error) {
	return listRemoteRefs(ctx, rfs)
}

// Push uploads local objects (snapshots, manifests, chunks, refs) to the
// remote. Objects already present on the remote are skipped. Refs that
// diverge (same name, different target) cause an error for that ref — the
// user must pull first. HEAD and config are NOT synced (see design doc §6.1).
//
// If branch is non-empty, only that branch's snapshot chain and its chunks
// are synced, plus the branch ref itself.
func Push(ctx context.Context, store storage.Storer, rfs RemoteFS, branch string) (*SyncStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stats := &SyncStats{}

	snapHashes, chunkHashes, refs, err := collectPushScope(ctx, store, branch)
	if err != nil {
		return nil, fmt.Errorf("collect push scope: %w", err)
	}

	// Upload snapshots + manifests.
	for _, id := range snapHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapPath := snapshotRemotePath(id)
		if _, err := rfs.Stat(snapPath); err == nil {
			stats.SnapshotsSkipped++
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat remote snapshot %s: %w", id.Hash.String(), err)
		}
		if err := pushSnapshot(ctx, store, rfs, id); err != nil {
			return nil, fmt.Errorf("push snapshot %s: %w", id.Hash.String(), err)
		}
		stats.SnapshotsUploaded++
		// Manifest is derived from the snapshot, uploaded alongside.
		if err := pushManifest(ctx, store, rfs, id); err != nil {
			return nil, fmt.Errorf("push manifest %s: %w", id.Hash.String(), err)
		}
		stats.ManifestsUploaded++
	}

	// Upload chunks concurrently with progress reporting.
	if err := pushChunksConcurrent(ctx, store, rfs, chunkHashes, stats); err != nil {
		return nil, err
	}

	// Upload refs (only those whose target snapshot exists on the remote).
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		updated, err := pushRef(ctx, store, rfs, ref)
		if err != nil {
			if errors.Is(err, errRefDiverged) {
				return nil, fmt.Errorf("ref %q diverged: pull first: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("push ref %q: %w", ref.Name, err)
		}
		if updated {
			stats.RefsUpdated++
		}
	}

	return stats, nil
}

// Pull downloads remote objects to local. Objects already present locally
// are skipped. Diverged refs (same name, different target) are saved as
// <name>.remote locally. HEAD and config are NOT synced. After pulling, if
// the current branch tip changed, the local index is rebuilt.
//
// If branch is non-empty, only that branch's snapshot chain and its chunks
// are synced, plus the branch ref itself.
func Pull(ctx context.Context, store storage.Storer, rfs RemoteFS, branch string) (*SyncStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stats := &SyncStats{}

	// Determine old tip (before pull) to detect tip change.
	oldTip, _ := currentBranchTip(ctx, store)

	snapHashes, chunkHashes, refs, err := collectPullScope(ctx, rfs, branch)
	if err != nil {
		return nil, fmt.Errorf("collect pull scope: %w", err)
	}

	// Download snapshots.
	for _, id := range snapHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Check existence via GetSnapshot + ErrNotFound (no HasSnapshot in the interface).
		if _, err := store.GetSnapshot(ctx, id); err == nil {
			stats.SnapshotsSkipped++
			continue
		} else if !errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("check local snapshot %s: %w", id.Hash.String(), err)
		}
		if err := pullSnapshot(ctx, store, rfs, id); err != nil {
			return nil, fmt.Errorf("pull snapshot %s: %w", id.Hash.String(), err)
		}
		stats.SnapshotsUploaded++
	}

	// Download chunks concurrently with progress reporting.
	if err := pullChunksConcurrent(ctx, store, rfs, chunkHashes, stats); err != nil {
		return nil, err
	}

	// Merge refs (append-only, never overwrite).
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		updated, diverged, err := pullRef(ctx, store, rfs, ref)
		if err != nil {
			return nil, fmt.Errorf("pull ref %q: %w", ref.Name, err)
		}
		if updated {
			stats.RefsUpdated++
		}
		if diverged {
			stats.RefsDiverged++
		}
	}

	// Rebuild index if current branch tip changed.
	newTip, err := currentBranchTip(ctx, store)
	if err == nil && newTip.Hash != oldTip.Hash && !newTip.Hash.IsZero() {
		if err := rebuildIndex(ctx, store, newTip); err != nil {
			return nil, fmt.Errorf("rebuild index: %w", err)
		}
		stats.IndexRebuilt = true
		stats.BranchTipChanged = currentBranchName(ctx, store)
	}

	return stats, nil
}

// PushDryRun collects the push scope and returns stats without actually
// uploading anything. The remote is only read (for existence checks).
func PushDryRun(ctx context.Context, store storage.Storer, rfs RemoteFS, branch string) (*SyncStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stats := &SyncStats{}

	snapHashes, chunkHashes, refs, err := collectPushScope(ctx, store, branch)
	if err != nil {
		return nil, fmt.Errorf("collect push scope: %w", err)
	}

	for _, id := range snapHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapPath := snapshotRemotePath(id)
		if _, err := rfs.Stat(snapPath); err == nil {
			stats.SnapshotsSkipped++
		} else if errors.Is(err, os.ErrNotExist) {
			stats.SnapshotsUploaded++
			stats.ManifestsUploaded++
		} else {
			return nil, fmt.Errorf("stat remote snapshot %s: %w", id.Hash.String(), err)
		}
	}

	for _, ch := range chunkHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chPath := chunkRemotePath(ch)
		if _, err := rfs.Stat(chPath); err == nil {
			stats.ChunksSkipped++
		} else if errors.Is(err, os.ErrNotExist) {
			stats.ChunksUploaded++
		} else {
			return nil, fmt.Errorf("stat remote chunk %s: %w", ch.String(), err)
		}
	}

	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapPath := snapshotRemotePath(core.SnapshotID{Hash: ref.Target})
		if _, err := rfs.Stat(snapPath); err == nil {
			stats.RefsUpdated++
		}
	}

	return stats, nil
}

// PullDryRun collects the pull scope and returns stats without actually
// downloading anything. The remote is only read (for listing).
func PullDryRun(ctx context.Context, store storage.Storer, rfs RemoteFS, branch string) (*SyncStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stats := &SyncStats{}

	snapHashes, chunkHashes, refs, err := collectPullScope(ctx, rfs, branch)
	if err != nil {
		return nil, fmt.Errorf("collect pull scope: %w", err)
	}

	for _, id := range snapHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := store.GetSnapshot(ctx, id); err == nil {
			stats.SnapshotsSkipped++
		} else if errors.Is(err, storage.ErrNotFound) {
			stats.SnapshotsUploaded++
		}
	}

	for _, ch := range chunkHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		has, _ := store.HasChunk(ctx, ch)
		if has {
			stats.ChunksSkipped++
		} else {
			stats.ChunksUploaded++
		}
	}

	stats.RefsUpdated = len(refs)

	return stats, nil
}

// pushChunksConcurrent uploads chunkHashes to rfs with bounded concurrency.
func pushChunksConcurrent(ctx context.Context, store storage.Storer, rfs RemoteFS, chunkHashes []core.Hash, stats *SyncStats) error {
	chunkTotal := len(chunkHashes)
	if chunkTotal == 0 {
		return nil
	}
	bar := progressbar.Default(int64(chunkTotal), "chunks push")
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, ch := range chunkHashes {
		if err := ctx.Err(); err != nil {
			firstErr = err
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(h core.Hash) {
			defer wg.Done()
			defer func() { <-sem }()
			chPath := chunkRemotePath(h)
			if _, statErr := rfs.Stat(chPath); statErr == nil {
				mu.Lock()
				stats.ChunksSkipped++
				mu.Unlock()
				bar.Add(1) //nolint:errcheck
				return
			}
			if err := pushChunk(ctx, store, rfs, h); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("push chunk %s: %w", h.String(), err)
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			stats.ChunksUploaded++
			mu.Unlock()
			bar.Add(1) //nolint:errcheck
		}(ch)
	}
	wg.Wait()
	bar.Finish() //nolint:errcheck
	return firstErr
}

// pullChunksConcurrent downloads chunkHashes from rfs with bounded concurrency.
func pullChunksConcurrent(ctx context.Context, store storage.Storer, rfs RemoteFS, chunkHashes []core.Hash, stats *SyncStats) error {
	chunkTotal := len(chunkHashes)
	if chunkTotal == 0 {
		return nil
	}
	bar := progressbar.Default(int64(chunkTotal), "chunks pull")
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, ch := range chunkHashes {
		if err := ctx.Err(); err != nil {
			firstErr = err
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(h core.Hash) {
			defer wg.Done()
			defer func() { <-sem }()
			has, hasErr := store.HasChunk(ctx, h)
			if hasErr != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("check local chunk %s: %w", h.String(), hasErr)
				}
				mu.Unlock()
				return
			}
			if has {
				mu.Lock()
				stats.ChunksSkipped++
				mu.Unlock()
				bar.Add(1) //nolint:errcheck
				return
			}
			if err := pullChunk(ctx, store, rfs, h); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("pull chunk %s: %w", h.String(), err)
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			stats.ChunksUploaded++
			mu.Unlock()
			bar.Add(1) //nolint:errcheck
		}(ch)
	}
	wg.Wait()
	bar.Finish() //nolint:errcheck
	return firstErr
}
