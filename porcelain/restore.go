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
				if err := os.MkdirAll(fullPath, 0755); err != nil {
					return backupID, fmt.Errorf("create dir %s: %w", fullPath, err)
				}
				continue
			}

			parentDir := filepath.Dir(fullPath)
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return backupID, fmt.Errorf("create parent dir %s: %w", parentDir, err)
			}

			perm := os.FileMode(entry.Mode & 0o777)
			if perm == 0 {
				perm = 0644
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

// resolveSecurePath validates that writing to relPath inside absWorkDir
// cannot escape the workspace via symlink traversal. It resolves symlinks
// on the target path (if it exists) or its nearest existing ancestor
// (when the target is a new file) and verifies the resolved location
// stays within absWorkDir. It returns the absolute path safe for writing,
// or an error if the resolved path escapes absWorkDir.
func resolveSecurePath(absWorkDir, relPath string) (string, error) {
	// Resolve absWorkDir itself so symlink comparisons are consistent
	// even when the workspace path contains symlinks (e.g. /var ->
	// /private/var on macOS).
	workDirResolved, err := filepath.EvalSymlinks(absWorkDir)
	if err != nil {
		return "", fmt.Errorf("eval symlinks for workDir %s: %w", absWorkDir, err)
	}
	fullPath := filepath.Join(workDirResolved, relPath)

	// Walk up from fullPath to the nearest existing ancestor. Resolving
	// that ancestor catches cases where a parent component is a symlink
	// pointing outside the workspace (e.g. evil -> /tmp, restoring
	// evil/sub/file.txt).
	target := fullPath
	for {
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			if err := checkWithin(workDirResolved, resolved); err != nil {
				return "", err
			}
			break
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("eval symlinks %s: %w", target, err)
		}
		if target == workDirResolved {
			break
		}
		parent := filepath.Dir(target)
		if parent == target {
			// Reached filesystem root without crossing workDirResolved.
			break
		}
		target = parent
	}

	return fullPath, nil
}

// checkWithin verifies that target stays inside baseDir after cleaning.
func checkWithin(baseDir, target string) error {
	rel, err := filepath.Rel(baseDir, target)
	if err != nil {
		return fmt.Errorf("compute rel from %s to %s: %w", baseDir, target, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("resolved path %s escapes workspace root %s", target, baseDir)
	}
	return nil
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

		safePath, err := resolveSecurePath(absWorkDir, entry.Path)
		if err != nil {
			failedSet[entry.Path] = true
			snapFiles[entry.Path] = true
			failures = append(failures, fmt.Sprintf("validate path %s: %v", entry.Path, err))
			continue
		}
		fullPath = safePath

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

	// On partial failure, skip the cleanup phase (which deletes files
	// not present in the snapshot). Running cleanup when some files
	// failed to restore would delete workspace files the user may still
	// need, causing data loss and leaving the workspace inconsistent.
	// The index update below still records only successfully restored
	// entries so callers know which files were actually written.
	var cleanErr error
	if len(failures) == 0 {
		cleanErr = fsutil.Walk(workDir, ignoreFile, func(path string, info os.FileInfo) error {
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
	}
	// Don't early-return on cleanup error; update the index first so
	// the workspace state is consistent, then report the cleanup failure.
	newIndex := &core.Index{UpdatedAt: time.Now().Unix()}
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
