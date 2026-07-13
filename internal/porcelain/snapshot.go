package porcelain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/Alei-001/drift/internal/util/pathutil"
	"github.com/schollz/progressbar/v3"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// CreateSnapshot scans workDir, chunks new or modified files, stores them,
// and writes a new snapshot on the current branch (HEAD's symbolic target,
// defaulting to heads/main). The workspace lock is acquired for the duration
// of the save. message must be non-empty; author defaults to "drift" when
// empty; cfg may be nil (core.DefaultConfig is used). Returns ErrNothingToSave
// when the workspace matches the index.
//
// Tags are NOT written into the snapshot. Tags live exclusively as
// tags/<name> refs (created by cmd/save.go via AddTag after the snapshot
// exists), which keeps them mutable — 'tag delete' and 'tag rename' actually
// take effect on the log view, instead of being frozen into the immutable
// snapshot. Old snapshots with embedded Tags fields are still readable;
// ResolveTagTips + mergeTags in the log layer merges both sources so
// historical data is preserved.
func CreateSnapshot(ctx context.Context, store storage.Storer, workDir string, message string, author string, cfg *core.CoreConfig) (*core.Snapshot, error) {
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)
	return createSnapshotInLock(ctx, store, workDir, message, author, cfg, true)
}

func createSnapshotInLock(ctx context.Context, store storage.Storer, workDir string, message string, author string, cfg *core.CoreConfig, showProgress bool) (*core.Snapshot, error) {
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if author == "" {
		author = "drift"
	}
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}

	var headHash core.Hash
	headRef, err := store.GetRef(ctx, "HEAD")
	if err == nil {
		headHash = headRef.Target
	} else if !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("read HEAD reference: %w", err)
	}

	oldIndex, err := store.GetIndex(ctx)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("read index: %w", err)
		}
		oldIndex = &core.Index{}
	}

	var workspaceFiles []workspaceFile
	err = fsutil.Walk(workDir, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Skip symlinks: they cannot be represented as a regular
		// FileEntry (the proto schema has no symlink-target field) and
		// following them via os.Open would chunk the target's content
		// while recording the symlink's mode, producing an inconsistent
		// entry that restore would write back as a regular file. Skipping
		// preserves the symlink in the workspace without tracking it.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		workspaceFiles = append(workspaceFiles, workspaceFile{path: path, info: info})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}

	oldIndexMap := make(map[string]*core.IndexEntry)
	for i := range oldIndex.Entries {
		oldIndexMap[oldIndex.Entries[i].Path] = &oldIndex.Entries[i]
	}

	var fileEntries []core.FileEntry
	var totalSize int64
	changed := false

	// Show a progress bar for user-initiated saves with many files. The
	// threshold is intentionally low (10) so even moderate projects get
	// feedback; the progressbar library is a no-op on non-terminal stderr.
	var bar *progressbar.ProgressBar
	if showProgress && len(workspaceFiles) >= 10 {
		bar = progressbar.Default(int64(len(workspaceFiles)), "saving")
	}

	// First pass: separate unchanged (fast path) from changed files.
	// Unchanged files are identified by matching ModTime + Size against
	// the old index; their entries are copied without re-chunking.
	//
	// Security note: this fast path is a performance optimization, NOT a
	// security guarantee. It trusts (mtime, size) as a proxy for "content
	// unchanged". An adversary who can forge mtime (e.g. touch -t) and
	// preserve size could trick CreateSnapshot into reusing stale chunks.
	// Drift assumes the workspace is not under active adversarial
	// tampering between snapshots; verified content integrity would
	// require re-reading every file, defeating the purpose of the index.
	// The merge phase below still re-chunks any file whose mtime or size
	// differs, and the resulting FileEntry.Hash is compared against the
	// old index hash to detect real content changes.
	entryMap := make(map[string]core.FileEntry, len(workspaceFiles))
	var orderedPaths []string
	var tasks []fileTask

	for _, f := range workspaceFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relPath, err := pathutil.Rel(workDir, f.path)
		if err != nil {
			return nil, fmt.Errorf("relative path for %s: %w", f.path, err)
		}
		orderedPaths = append(orderedPaths, relPath)

		if oldEntry, ok := oldIndexMap[relPath]; ok &&
			oldEntry.Size == f.info.Size() &&
			oldEntry.ModTime == f.info.ModTime().UnixNano() {
			entryMap[relPath] = core.FileEntry{
				Path:    relPath,
				Mode:    core.FileMode(f.info.Mode()),
				Size:    f.info.Size(),
				ModTime: f.info.ModTime().UnixNano(),
				Chunks:  oldEntry.Chunks,
				Hash:    oldEntry.Hash,
			}
			totalSize += f.info.Size()
			if bar != nil {
				bar.Add(1) //nolint:errcheck
			}
			continue
		}

		tasks = append(tasks, fileTask{wf: f, relPath: relPath})
	}

	// Concurrent processing of changed files using a worker pool sized
	// to runtime.NumCPU(). Each task opens its own file and stores chunks
	// to content-addressed paths, so no locking is needed.
	processedEntries, err := chunkFilesConcurrent(ctx, store, tasks, bar)
	if err != nil {
		return nil, err
	}

	// Merge results in workspace walk order (deterministic) and detect
	// actual changes by comparing content hashes.
	for _, relPath := range orderedPaths {
		entry, ok := entryMap[relPath]
		if !ok {
			entry = processedEntries[relPath]
			totalSize += entry.Size

			if oldEntry, ok := oldIndexMap[relPath]; ok {
				if oldEntry.Hash != entry.Hash {
					changed = true
				}
			} else {
				changed = true
			}
		}
		fileEntries = append(fileEntries, entry)
	}

	if bar != nil {
		bar.Finish() //nolint:errcheck
	}

	if !changed && len(oldIndexMap) != len(workspaceFiles) {
		changed = true
	}

	if !changed {
		return nil, ErrNothingToSave
	}

	var prevID *core.SnapshotID
	if !headHash.IsZero() {
		prevID = &core.SnapshotID{Hash: headHash}
	}

	snap := &core.Snapshot{
		PrevID:    prevID,
		Message:   message,
		Author:    author,
		Timestamp: time.Now().Unix(),
		Files:     fileEntries,
		TotalSize: totalSize,
	}

	snapProto := core.SnapshotToProto(snap, false)
	marshaled, err := proto.Marshal(snapProto)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	var hash [core.HashSize]byte = blake3.Sum256(marshaled)
	snap.ID = core.SnapshotID{Hash: core.Hash(hash)}

	if err := store.PutSnapshot(ctx, snap); err != nil {
		return nil, fmt.Errorf("save snapshot: %w", err)
	}

	symRef := "heads/main"
	if headRef != nil && headRef.SymRef != "" {
		symRef = headRef.SymRef
	}

	newIndex := &core.Index{
		UpdatedAt: time.Now().Unix(),
	}
	for _, entry := range fileEntries {
		newIndex.Entries = append(newIndex.Entries, core.IndexEntry{
			Path:    entry.Path,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			Chunks:  entry.Chunks,
			Hash:    entry.Hash,
		})
	}
	branchRef := &core.Reference{
		Name:   symRef,
		Type:   core.RefTypeBranch,
		Target: snap.ID.Hash,
	}
	// Save the previous branch ref so SetIndex failure can roll back the
	// SetRef below. Without this, a SetIndex error would leave the branch
	// pointing at the new snapshot while the index still reflects the old
	// workspace state, making the next save link to the wrong parent.
	oldBranchRef, oldBranchErr := store.GetRef(ctx, symRef)
	if oldBranchErr != nil && !errors.Is(oldBranchErr, storage.ErrNotFound) {
		return nil, fmt.Errorf("read previous branch ref: %w", oldBranchErr)
	}
	if err := store.SetRef(ctx, symRef, branchRef); err != nil {
		return nil, fmt.Errorf("update branch: %w", err)
	}

	if err := store.SetIndex(ctx, newIndex); err != nil {
		// Roll back the branch ref to its pre-save value. If the branch
		// did not exist before (oldBranchErr == ErrNotFound), delete the
		// ref we just created; otherwise restore the previous value.
		if errors.Is(oldBranchErr, storage.ErrNotFound) {
			_ = store.DeleteRef(ctx, symRef)
		} else {
			_ = store.SetRef(ctx, symRef, oldBranchRef)
		}
		return nil, fmt.Errorf("update index (rolled back branch ref): %w", err)
	}

	slog.Info("snapshot created", "id", snap.ShortID(), "branch", symRef, "files", len(snap.Files), "size", snap.TotalSize, "message", message)

	return snap, nil
}
