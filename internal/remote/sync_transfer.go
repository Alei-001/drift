package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/schollz/progressbar/v3"
)

// defaultConcurrency is the maximum number of concurrent chunk transfers
// during push and pull when no override is configured. Higher values improve
// throughput on high-latency links (WebDAV) but may overwhelm low-bandwidth
// connections.
const defaultConcurrency = 8

// concurrency is the worker count used by push/pull chunk transfers. It
// defaults to defaultConcurrency and can be overridden via SetConcurrency
// (typically from RemoteConfig.Options["concurrency"] in the porcelain
// layer). It is read once at the start of each transfer loop, so changing
// it during an in-flight push/pull has no effect on that operation.
// Access is atomic because SetConcurrency may be called from a different
// goroutine than the transfer loops that read it.
var concurrency atomic.Int32

func init() {
	concurrency.Store(int32(defaultConcurrency))
}

// SetConcurrency overrides the worker count for push/pull chunk transfers.
// It must be called before Push/Pull. A non-positive value falls back to the
// default (8). This is the hook by which RemoteConfig.Options["concurrency"]
// reaches the transfer loops without threading an extra parameter through
// every Push/Pull signature.
func SetConcurrency(n int) {
	if n > 0 {
		concurrency.Store(int32(n))
	} else {
		concurrency.Store(int32(defaultConcurrency))
	}
}

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

// Push uploads local objects (chunks, snapshots, manifests, refs) to the
// remote. Objects already present on the remote are skipped. Refs that
// diverge (same name, different target) cause an error for that ref — the
// user must pull first. HEAD and config are NOT synced (see design doc §6.1).
//
// Upload order is chunks → snapshots → manifests → refs. This guarantees
// that when a snapshot becomes visible on the remote, every chunk it
// references is already there (so a concurrent pull never sees a
// half-complete snapshot), and refs are updated last so a branch tip only
// ever points at a fully-uploaded object graph.
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

	// 1. Upload chunks first so snapshots always reference existing chunks.
	if err := pushChunksConcurrent(ctx, store, rfs, chunkHashes, stats); err != nil {
		return nil, err
	}

	// 2. Upload snapshots.
	for _, id := range snapHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapPath := snapshotRemotePath(id)
		if _, err := rfs.Stat(ctx, snapPath); err == nil {
			stats.SnapshotsSkipped++
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat remote snapshot %s: %w", id.Hash.String(), err)
		}
		if err := pushSnapshot(ctx, store, rfs, id); err != nil {
			return nil, fmt.Errorf("push snapshot %s: %w", id.Hash.String(), err)
		}
		stats.SnapshotsUploaded++
	}

	// 3. Upload manifests. Manifest existence is checked independently of
	// snapshot existence (P1-9): a previously-skipped snapshot may still
	// need its manifest uploaded if the manifest was missing or stale.
	for _, id := range snapHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		manifestPath := manifestRemotePath(id)
		if _, err := rfs.Stat(ctx, manifestPath); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat remote manifest %s: %w", id.Hash.String(), err)
		}
		if err := pushManifest(ctx, store, rfs, id); err != nil {
			return nil, fmt.Errorf("push manifest %s: %w", id.Hash.String(), err)
		}
		stats.ManifestsUploaded++
	}

	// 4. Update refs last so branch tips only point at fully-uploaded
	// objects (snapshot + chunks + manifest all present).
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

// partialPullErr wraps a pull error with a summary of objects already
// downloaded, so the user knows the operation can be safely retried.
// In content-addressable storage, partially-downloaded objects are valid
// (just incomplete); retrying Pull will skip them and fetch the rest.
func partialPullErr(err error, stats *SyncStats) error {
	return fmt.Errorf("pull failed (already downloaded %d snapshots, %d chunks; safe to retry): %w",
		stats.SnapshotsUploaded, stats.ChunksUploaded, err)
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
			return stats, partialPullErr(err, stats)
		}
		// Existence check: GetSnapshot + ErrNotFound. The Storer interface
		// has no HasSnapshot (P2-b optimization opportunity): adding a
		// lightweight HasSnapshot would avoid deserializing the snapshot
		// body here. For the memory backend GetSnapshot is a cheap map
		// lookup; for the filesystem backend it is a file read. Since the
		// common pull case is "snapshot missing locally", GetSnapshot
		// returns ErrNotFound quickly after a stat, so the cost is one
		// extra stat per snapshot — acceptable until profiling shows
		// otherwise.
		if _, err := store.GetSnapshot(ctx, id); err == nil {
			stats.SnapshotsSkipped++
			continue
		} else if !errors.Is(err, storage.ErrNotFound) {
			return stats, partialPullErr(fmt.Errorf("check local snapshot %s: %w", id.Hash.String(), err), stats)
		}
		if err := pullSnapshot(ctx, store, rfs, id); err != nil {
			return stats, partialPullErr(fmt.Errorf("pull snapshot %s: %w", id.Hash.String(), err), stats)
		}
		stats.SnapshotsUploaded++
	}

	// Download chunks concurrently with progress reporting.
	if err := pullChunksConcurrent(ctx, store, rfs, chunkHashes, stats); err != nil {
		return stats, partialPullErr(err, stats)
	}

	// Merge refs (append-only, never overwrite).
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return stats, partialPullErr(err, stats)
		}
		updated, diverged, err := pullRef(ctx, store, rfs, ref)
		if err != nil {
			return stats, partialPullErr(fmt.Errorf("pull ref %q: %w", ref.Name, err), stats)
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
			return stats, partialPullErr(fmt.Errorf("rebuild index: %w", err), stats)
		}
		stats.IndexRebuilt = true
		stats.BranchTipChanged = currentBranchName(ctx, store)
	}

	return stats, nil
}

// refPushStatus classifies what a real push would do for a single ref. It is
// used by PushDryRun so the dry-run stats reflect the same updated / skipped
// / diverged distinction that pushRef computes, rather than optimistically
// counting every ref whose target snapshot exists.
type refPushStatus int

const (
	refSkipNoTarget refPushStatus = iota // target snapshot not on remote
	refSkipSame                          // remote ref already at target
	refUpdate                            // new ref or fast-forward
	refDiverge                           // not fast-forwardable
)

// classifyRefPush runs the read-only portion of pushRef (stat target, read
// existing remote ref, fast-forward check) without writing anything. It
// mirrors pushRef's logic so dry-run stats are accurate.
func classifyRefPush(ctx context.Context, store storage.Storer, rfs RemoteFS, ref *core.Reference) (refPushStatus, error) {
	snapPath := snapshotRemotePath(core.SnapshotID{Hash: ref.Target})
	if _, err := rfs.Stat(ctx, snapPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return refSkipNoTarget, nil
		}
		return 0, fmt.Errorf("stat remote snapshot for ref: %w", err)
	}
	refPath := refRemotePath(ref.Name)
	existing, err := rfs.Read(ctx, refPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return refUpdate, nil // new ref
		}
		return 0, fmt.Errorf("read existing remote ref: %w", err)
	}
	defer existing.Close()
	data, err := io.ReadAll(existing)
	if err != nil {
		return 0, fmt.Errorf("read existing remote ref: %w", err)
	}
	existingHashStr := strings.TrimSpace(string(data))
	if existingHashStr == ref.Target.FullString() {
		return refSkipSame, nil
	}
	existingHash, parseErr := parseHashHex(existingHashStr)
	if parseErr != nil {
		return refDiverge, nil
	}
	ok, ancErr := isAncestor(ctx, store, ref.Target, existingHash)
	if ancErr != nil {
		return 0, fmt.Errorf("fast-forward check against %s: %w", existingHash.FullString(), ancErr)
	}
	if ok {
		return refUpdate, nil
	}
	return refDiverge, nil
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
		if _, err := rfs.Stat(ctx, snapPath); err == nil {
			stats.SnapshotsSkipped++
		} else if errors.Is(err, os.ErrNotExist) {
			stats.SnapshotsUploaded++
			stats.ManifestsUploaded++
		} else {
			return nil, fmt.Errorf("stat remote snapshot %s: %w", id.Hash.String(), err)
		}
	}

	// Batch check: list remote chunk directories once instead of per-chunk Stat.
	remoteChunkSet, err := listRemoteChunkHashes(ctx, rfs, chunkHashes)
	if err != nil {
		return nil, fmt.Errorf("list remote chunks: %w", err)
	}
	for _, ch := range chunkHashes {
		if remoteChunkSet[ch] {
			stats.ChunksSkipped++
		} else {
			stats.ChunksUploaded++
		}
	}

	// Ref stats: run the same read-only classification pushRef uses so the
	// dry-run distinguishes updated / skipped / diverged (P2-g), rather than
	// optimistically counting every ref whose target snapshot exists.
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		status, err := classifyRefPush(ctx, store, rfs, ref)
		if err != nil {
			return nil, fmt.Errorf("classify ref %q: %w", ref.Name, err)
		}
		if status == refUpdate {
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

	// Batch check: list local chunks once instead of per-chunk HasChunk.
	localChunkSet, err := listLocalChunkHashes(ctx, store)
	if err != nil {
		return nil, fmt.Errorf("list local chunks: %w", err)
	}
	for _, ch := range chunkHashes {
		if localChunkSet[ch] {
			stats.ChunksSkipped++
		} else {
			stats.ChunksUploaded++
		}
	}

	stats.RefsUpdated = len(refs)

	return stats, nil
}

// pushChunksConcurrent uploads chunkHashes to rfs with bounded concurrency.
// Remote chunk existence is checked in batch (one List per prefix directory)
// before uploading, replacing N per-chunk Stat calls with at most 256 List
// calls (typically far fewer). On the first error the derived context is
// cancelled so no new workers are launched and in-flight workers exit early.
func pushChunksConcurrent(ctx context.Context, store storage.Storer, rfs RemoteFS, chunkHashes []core.Hash, stats *SyncStats) error {
	chunkTotal := len(chunkHashes)
	if chunkTotal == 0 {
		return nil
	}

	// Batch check: list remote chunk directories to build existence set.
	remoteChunkSet, err := listRemoteChunkHashes(ctx, rfs, chunkHashes)
	if err != nil {
		return fmt.Errorf("list remote chunks: %w", err)
	}

	// Partition into skip / upload lists.
	var toUpload []core.Hash
	for _, ch := range chunkHashes {
		if remoteChunkSet[ch] {
			stats.ChunksSkipped++
		} else {
			toUpload = append(toUpload, ch)
		}
	}
	if len(toUpload) == 0 {
		return nil
	}

	// Derive a cancellable context so the first error stops launching new
	// workers and signals in-flight workers to abandon their work.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bar := progressbar.Default(int64(len(toUpload)), "chunks push")
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, int(concurrency.Load()))
	var wg sync.WaitGroup
	for _, ch := range toUpload {
		if err := ctx.Err(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(h core.Hash) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := pushChunk(ctx, store, rfs, h); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("push chunk %s: %w", h.String(), err)
					cancel() // stop launching new workers
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
// Local chunk existence is checked in batch (one ListChunks call) before
// downloading, replacing N per-chunk HasChunk calls with a single call. On
// the first error the derived context is cancelled so no new workers are
// launched and in-flight workers exit early.
func pullChunksConcurrent(ctx context.Context, store storage.Storer, rfs RemoteFS, chunkHashes []core.Hash, stats *SyncStats) error {
	chunkTotal := len(chunkHashes)
	if chunkTotal == 0 {
		return nil
	}

	// Batch check: list local chunks once instead of per-chunk HasChunk.
	localChunkSet, err := listLocalChunkHashes(ctx, store)
	if err != nil {
		return fmt.Errorf("list local chunks: %w", err)
	}

	// Partition into skip / download lists.
	var toDownload []core.Hash
	for _, ch := range chunkHashes {
		if localChunkSet[ch] {
			stats.ChunksSkipped++
		} else {
			toDownload = append(toDownload, ch)
		}
	}
	if len(toDownload) == 0 {
		return nil
	}

	// Derive a cancellable context so the first error stops launching new
	// workers and signals in-flight workers to abandon their work.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bar := progressbar.Default(int64(len(toDownload)), "chunks pull")
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, int(concurrency.Load()))
	var wg sync.WaitGroup
	for _, ch := range toDownload {
		if err := ctx.Err(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(h core.Hash) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := pullChunk(ctx, store, rfs, h); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("pull chunk %s: %w", h.String(), err)
					cancel() // stop launching new workers
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
