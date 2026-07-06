package porcelain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
	"github.com/your-org/drift/internal/util/pathutil"
)

// RestoreSnapshot restores files from snapshotID into workDir. When filePath
// is empty the entire snapshot is restored (workspace files absent from the
// snapshot are removed); otherwise only that single file is restored and the
// index is updated for it. When noBackup is false a backup snapshot of the
// current workspace is created first and its short ID is returned in backupID
// (empty when no backup was needed, e.g. ErrNothingToSave). cfg may be nil
// (core.DefaultConfig is used). The named return err is wrapped by a defer
// so that on failure the backup ID (if any) is appended for rollback guidance.
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
		backupSnap, backupErr := createSnapshotInLock(ctx, store, workDir, backupMsg, "drift", nil, cfg)
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
			branchRef := &core.Reference{
				Name:   headRef.SymRef,
				Type:   core.RefTypeBranch,
				Target: snapshotID.Hash,
			}
			if err = store.SetRef(ctx, headRef.SymRef, branchRef); err != nil {
				return backupID, fmt.Errorf("update branch %s: %w", headRef.SymRef, err)
			}
			headRef.Target = snapshotID.Hash
			if err = store.SetRef(ctx, "HEAD", headRef); err != nil {
				return backupID, fmt.Errorf("update HEAD: %w", err)
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
			return backupID, fmt.Errorf("file %q not found in snapshot %s", filePath, snapshotID.Hash.String())
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

	return backupID, nil
}
