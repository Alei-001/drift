package porcelain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/fsutil"
	"github.com/your-org/drift/util/pathutil"
)

func RestoreSnapshot(ctx context.Context, store storage.Storer, workDir string, snapshotID core.SnapshotID, filePath string, noBackup bool, cfg *core.CoreConfig) (string, error) {
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return "", err
	}
	defer ReleaseWorkspaceLock(workDir)

	snap, err := store.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return "", fmt.Errorf("get snapshot: %w", err)
	}

	var backupID string
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
		if err := restoreFilesToWorkspace(ctx, store, workDir, cfg.IgnoreFile, snap); err != nil {
			return "", err
		}
	} else {
		var pathErr error
		filePath, pathErr = pathutil.RelToWorkDir(workDir, filePath)
		if pathErr != nil {
			return "", fmt.Errorf("cannot resolve path: %w", pathErr)
		}

		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			return "", fmt.Errorf("resolve workDir: %w", err)
		}

		var restoredEntry *core.FileEntry
		for _, entry := range snap.Files {
			if entry.Path != filePath {
				continue
			}

			fullPath := filepath.Join(absWorkDir, entry.Path)

			if fullPath != absWorkDir && !strings.HasPrefix(fullPath, absWorkDir+string(filepath.Separator)) {
				continue
			}

			if entry.Mode.IsDir() {
				if err := os.MkdirAll(fullPath, 0755); err != nil {
					return "", fmt.Errorf("create dir %s: %w", fullPath, err)
				}
				continue
			}

			parentDir := filepath.Dir(fullPath)
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return "", fmt.Errorf("create parent dir %s: %w", parentDir, err)
			}

			perm := os.FileMode(entry.Mode & 0o777)
			if perm == 0 {
				perm = 0644
			}
			if err := writeFileFromChunks(ctx, store, fullPath, entry.Chunks, perm); err != nil {
				return "", fmt.Errorf("write file %s: %w", fullPath, err)
			}

			if err := os.Chtimes(fullPath, time.Unix(0, entry.ModTime), time.Unix(0, entry.ModTime)); err != nil {
				return "", fmt.Errorf("set modtime %s: %w", fullPath, err)
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
					return "", fmt.Errorf("read index: %w", err)
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
				return "", fmt.Errorf("update index: %w", err)
			}
		}
	}

	return backupID, nil
}

func writeFileFromChunks(ctx context.Context, store storage.Storer, path string, chunks []core.Hash, perm os.FileMode) error {
	tmpPath := path + ".drifttmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	for _, h := range chunks {
		chunk, err := store.GetChunk(ctx, h)
		if err != nil {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("get chunk %s: %w", h.String(), err)
		}
		if _, err := f.Write(chunk.Data); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("write chunk data: %w", err)
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func restoreFilesToWorkspace(ctx context.Context, store storage.Storer, workDir, ignoreFile string, snap *core.Snapshot) error {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("resolve workDir: %w", err)
	}

	snapFiles := make(map[string]bool)
	failedSet := make(map[string]bool)
	var failures []string

	for _, entry := range snap.Files {
		fullPath := filepath.Join(absWorkDir, entry.Path)

		if fullPath != absWorkDir && !strings.HasPrefix(fullPath, absWorkDir+string(filepath.Separator)) {
			continue
		}

		if entry.Mode.IsDir() {
			if err := os.MkdirAll(fullPath, 0755); err != nil {
				failedSet[entry.Path] = true
				snapFiles[entry.Path] = true
				failures = append(failures, fmt.Sprintf("create dir %s: %v", fullPath, err))
				continue
			}
			snapFiles[entry.Path] = true
			continue
		}

		parentDir := filepath.Dir(fullPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			failedSet[entry.Path] = true
			snapFiles[entry.Path] = true
			failures = append(failures, fmt.Sprintf("create parent dir %s: %v", parentDir, err))
			continue
		}

		perm := os.FileMode(entry.Mode & 0o777)
		if perm == 0 {
			perm = 0644
		}
		if err := writeFileFromChunks(ctx, store, fullPath, entry.Chunks, perm); err != nil {
			failedSet[entry.Path] = true
			snapFiles[entry.Path] = true
			failures = append(failures, fmt.Sprintf("write file %s: %v", fullPath, err))
			continue
		}

		if err := os.Chtimes(fullPath, time.Unix(0, entry.ModTime), time.Unix(0, entry.ModTime)); err != nil {
			failedSet[entry.Path] = true
			snapFiles[entry.Path] = true
			failures = append(failures, fmt.Sprintf("set modtime %s: %v", fullPath, err))
			continue
		}

		snapFiles[entry.Path] = true
	}

	cleanErr := fsutil.Walk(workDir, ignoreFile, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if snapFiles[rel] {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove extra file %s: %w", path, err)
		}
		return nil
	})
	// Don't early-return on cleanup error; update the index first so
	// the workspace state is consistent, then report the cleanup failure.
	newIndex := &core.Index{}
	for _, entry := range snap.Files {
		if failedSet[entry.Path] {
			continue
		}
		newIndex.Entries = append(newIndex.Entries, core.IndexEntry{
			Path:    entry.Path,
			Hash:    entry.Hash,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			Chunks:  entry.Chunks,
		})
	}
	if err := store.SetIndex(ctx, newIndex); err != nil {
		return fmt.Errorf("update index: %w", err)
	}

	if cleanErr != nil {
		return fmt.Errorf("clean workspace: %w", cleanErr)
	}

	if len(failures) > 0 {
		return fmt.Errorf("restore failed for %d file(s): %s", len(failures), strings.Join(failures, "; "))
	}

	return nil
}
