package porcelain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/Alei-001/drift/internal/util/pathutil"
)

// RestoreSnapshot restores files from snapshotID into workDir. When filePath
// is empty the entire snapshot is restored (workspace files absent from the
// snapshot are removed); otherwise only that single file is restored and the
// index is updated for it. When noBackup is false a backup snapshot of the
// current workspace is created first and its short ID is returned in backupID
// (empty when no backup was needed, e.g. ErrNothingToSave). cfg may be nil
// (core.DefaultConfig is used). The named return err is wrapped by a defer
// so that on failure the backup ID (if any) is appended for rollback guidance.
//
// Rollback strategy: restore is not transactional across multiple files.
// Per-file atomicity is provided by writeFileFromChunks (temp file + rename),
// so a write failure never leaves a half-written file. When a restore fails
// partway, restoreFilesToWorkspace skips the cleanup phase (deletion of
// non-snapshot files) and updates the index to reflect only successfully
// restored entries, so the workspace stays as consistent as possible. The
// backup snapshot (when enabled) captures the pre-restore workspace state so
// the user can manually roll back with `drift restore <backupID>`.
func RestoreSnapshot(ctx context.Context, store storage.Storer, workDir string, snapshotID core.SnapshotID, filePath string, noBackup bool, cfg *core.CoreConfig) (backupID string, err error) {
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}
	if err = AcquireWorkspaceLock(workDir); err != nil {
		return "", err
	}
	defer ReleaseWorkspaceLock(workDir)

	// When a backup snapshot was created and a later step fails, include
	// the backup snapshot ID in the returned error so users can roll back.
	defer func() {
		if err != nil && backupID != "" {
			err = fmt.Errorf("%w (backup snapshot %s created for rollback)", err, backupID)
		}
	}()

	snap, err := store.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return "", fmt.Errorf("get snapshot: %w", err)
	}

	if !noBackup {
		backupMsg := fmt.Sprintf("backup: restore to %s", snapshotID.Hash.String())
		backupSnap, backupErr := createSnapshotInLock(ctx, store, workDir, backupMsg, "drift", cfg, false)
		if backupErr != nil {
			if !errors.Is(backupErr, ErrNothingToSave) {
				return "", fmt.Errorf("create backup: %w", backupErr)
			}
		} else {
			backupID = backupSnap.ShortID()
		}
	}

	if filePath == "" {
		if err = restoreFilesToWorkspace(ctx, store, workDir, cfg.IgnoreFile, snap); err != nil {
			return backupID, fmt.Errorf("restore workspace: %w", err)
		}
		// Update HEAD (and the current branch, when attached) to point at
		// the restored snapshot. Without this the workspace would match
		// snapshotID while HEAD still references the pre-restore tip, so
		// the next save would link to the wrong parent and sever the
		// history chain. Mirrors architecture.md §5.2 step 3.
		headRef, headErr := store.GetRef(ctx, "HEAD")
		if headErr != nil {
			return backupID, fmt.Errorf("read HEAD for update: %w", headErr)
		}
		if headRef.SymRef != "" {
			// Save the previous branch ref so a HEAD-update failure can
			// roll back the branch update. Without this, a SetRef("HEAD")
			// failure would leave the branch pointing at the restored
			// snapshot while HEAD still references the pre-restore tip,
			// desynchronizing HEAD from its symbolic target.
			oldBranchRef, oldBranchErr := store.GetRef(ctx, headRef.SymRef)
			if oldBranchErr != nil {
				return backupID, fmt.Errorf("read branch %s for rollback: %w", headRef.SymRef, oldBranchErr)
			}
			branchRef := &core.Reference{
				Name:   headRef.SymRef,
				Type:   core.RefTypeBranch,
				Target: snapshotID.Hash,
			}
			if err = store.SetRef(ctx, headRef.SymRef, branchRef); err != nil {
				// Workspace and index have already been rewritten to
				// snapshotID, but the branch ref failed to update.
				// The backup snapshot (backupID) lets the user recover.
				return backupID, fmt.Errorf("update branch %s (workspace already restored to %s; backup is %s — re-run restore or restore the backup to recover): %w", headRef.SymRef, snapshotID.Hash.String(), backupID, err)
			}
			headRef.Target = snapshotID.Hash
			if err = store.SetRef(ctx, "HEAD", headRef); err != nil {
				// Roll back the branch ref to its pre-restore value so
				// HEAD and its symbolic target stay consistent.
				_ = store.SetRef(ctx, headRef.SymRef, oldBranchRef)
				return backupID, fmt.Errorf("update HEAD (rolled back branch %s): %w", headRef.SymRef, err)
			}
		} else {
			headRef.Target = snapshotID.Hash
			if err = store.SetRef(ctx, "HEAD", headRef); err != nil {
				return backupID, fmt.Errorf("update HEAD: %w", err)
			}
		}
	} else {
		var pathErr error
		filePath, pathErr = pathutil.RelToWorkDir(workDir, filePath)
		if pathErr != nil {
			return backupID, fmt.Errorf("cannot resolve path: %w", pathErr)
		}

		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			return backupID, fmt.Errorf("resolve workDir: %w", err)
		}

		var restoredEntry *core.FileEntry
		for _, entry := range snap.Files {
			if err = ctx.Err(); err != nil {
				return backupID, err
			}
			if entry.Path != filePath {
				continue
			}

			fullPath := filepath.Join(absWorkDir, entry.Path)

			if fullPath != absWorkDir && !strings.HasPrefix(fullPath, absWorkDir+string(filepath.Separator)) {
				continue
			}

			safePath, err := resolveSecurePath(absWorkDir, entry.Path)
			if err != nil {
				return backupID, fmt.Errorf("validate path %s: %w", entry.Path, err)
			}
			fullPath = safePath

			// Refuse to restore entries recorded as symlinks: the
			// snapshot schema cannot carry the symlink target, so
			// restoring such an entry as a regular file would silently
			// replace the user's symlink. Skip it instead so an explicit
			// 'restore <file>' returns an error rather than corrupting
			// the workspace.
			if entry.Mode.IsSymlink() {
				return backupID, fmt.Errorf("cannot restore %q: symlink entries are not restorable", entry.Path)
			}

			if entry.Mode.IsDir() {
				if err := os.MkdirAll(fullPath, fsutil.DefaultDirPerm); err != nil {
					return backupID, fmt.Errorf("create dir %s: %w", fullPath, err)
				}
				continue
			}

			parentDir := filepath.Dir(fullPath)
			if err := os.MkdirAll(parentDir, fsutil.DefaultDirPerm); err != nil {
				return backupID, fmt.Errorf("create parent dir %s: %w", parentDir, err)
			}

			perm := os.FileMode(entry.Mode & 0o777)
			// Mask group/other write bits (umask 0o022 semantics) to
			// prevent malicious snapshots from creating world-writable
			// files on restore.
			perm &^= 0o022
			if perm == 0 {
				perm = fsutil.DefaultFilePerm
			}
			if err := writeFileFromChunks(ctx, store, fullPath, entry.Chunks, perm); err != nil {
				return backupID, fmt.Errorf("write file %s: %w", fullPath, err)
			}

			if err := os.Chtimes(fullPath, time.Unix(0, entry.ModTime), time.Unix(0, entry.ModTime)); err != nil {
				return backupID, fmt.Errorf("set modtime %s: %w", fullPath, err)
			}

			entryCopy := entry
			restoredEntry = &entryCopy
		}

		if restoredEntry == nil {
			return backupID, fmt.Errorf("%w: %q in snapshot %s", ErrFileNotFound, filePath, snapshotID.Hash.String())
		}

		if restoredEntry != nil {
			existingIndex, err := store.GetIndex(ctx)
			if err != nil {
				if !errors.Is(err, storage.ErrNotFound) {
					return backupID, fmt.Errorf("read index: %w", err)
				}
				existingIndex = &core.Index{}
			}
			found := false
			for i := range existingIndex.Entries {
				if existingIndex.Entries[i].Path == filePath {
					existingIndex.Entries[i].Size = restoredEntry.Size
					existingIndex.Entries[i].ModTime = restoredEntry.ModTime
					existingIndex.Entries[i].Chunks = restoredEntry.Chunks
					existingIndex.Entries[i].Hash = restoredEntry.Hash
					found = true
					break
				}
			}
			if !found {
				existingIndex.Entries = append(existingIndex.Entries, core.IndexEntry{
					Path:    restoredEntry.Path,
					Hash:    restoredEntry.Hash,
					Size:    restoredEntry.Size,
					ModTime: restoredEntry.ModTime,
					Chunks:  restoredEntry.Chunks,
				})
			}
			if err := store.SetIndex(ctx, existingIndex); err != nil {
				return backupID, fmt.Errorf("update index: %w", err)
			}
		}
	}

	if filePath == "" {
		slog.Info("snapshot restored", "id", snapshotID.Hash.String(), "files", len(snap.Files), "backup", backupID)
	} else {
		slog.Info("file restored", "snapshot", snapshotID.Hash.String(), "file", filePath, "backup", backupID)
	}

	return backupID, nil
}

// ComputeRestoreChanges compares the current workspace files against the
// target snapshot and returns what would change if the snapshot were restored:
// files in the snapshot but not in the workspace (added), files in both but
// with different content (modified), and files in the workspace but not in
// the snapshot (deleted). Modification is detected by comparing content
// hashes (BLAKE3) rather than modtime, since tools like "cp -p" preserve
// modtime while changing content.
func ComputeRestoreChanges(ctx context.Context, workDir string, cfg *core.CoreConfig, snapshot *core.Snapshot) (added []core.FileEntry, modified []core.FileEntry, deleted []string, err error) {
	type fileInfo struct {
		size int64
		path string
	}
	workspaceFiles := make(map[string]fileInfo)
	walkErr := fsutil.Walk(workDir, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Skip symlinks: snapshots never track them (see
		// createSnapshotInLock), so they must not be reported as
		// "deleted" by ComputeRestoreChanges — otherwise the caller
		// might delete a user's symlink.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := pathutil.Rel(workDir, path)
		if relErr != nil {
			return nil
		}
		workspaceFiles[rel] = fileInfo{size: info.Size(), path: path}
		return nil
	})
	if walkErr != nil {
		return nil, nil, nil, walkErr
	}

	snapFiles := make(map[string]bool)
	for _, f := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		snapFiles[f.Path] = true
		if ws, ok := workspaceFiles[f.Path]; !ok {
			added = append(added, f)
		} else if ws.size != f.Size {
			modified = append(modified, f)
		} else {
			// Same size: compare content hash to detect changes that
			// preserve size (e.g. "cp -p" preserves modtime too).
			workHash, hashErr := ComputeFileHash(ws.path)
			if hashErr != nil || workHash != f.Hash {
				modified = append(modified, f)
			}
		}
	}

	for path := range workspaceFiles {
		if !snapFiles[path] {
			deleted = append(deleted, path)
		}
	}

	sort.Slice(added, func(i, j int) bool { return added[i].Path < added[j].Path })
	sort.Slice(modified, func(i, j int) bool { return modified[i].Path < modified[j].Path })
	sort.Strings(deleted)

	return added, modified, deleted, nil
}
