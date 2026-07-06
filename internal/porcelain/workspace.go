package porcelain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
)

// writeFileFromChunks reconstructs a file at path by concatenating chunk data
// in order. It writes to a temporary file first, then renames atomically. On
// any error the temp file is removed and the original file is left untouched.
func writeFileFromChunks(ctx context.Context, store storage.Storer, path string, chunks []core.Hash, perm os.FileMode) (err error) {
	tmpPath := path + ".drifttmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	for _, h := range chunks {
		if err = ctx.Err(); err != nil {
			return err
		}
		chunk, err := store.GetChunk(ctx, h)
		if err != nil {
			return fmt.Errorf("get chunk %s: %w", h.String(), err)
		}
		if _, err := f.Write(chunk.Data); err != nil {
			return fmt.Errorf("write chunk data: %w", err)
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
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

// restoreFilesToWorkspace reconstructs all files from snap into workDir,
// removing any workspace files not present in the snapshot. On partial
// failure it skips the cleanup phase to avoid deleting workspace files the
// user may still need, and updates the index to reflect only successfully
// restored entries.
func restoreFilesToWorkspace(ctx context.Context, store storage.Storer, workDir, ignoreFile string, snap *core.Snapshot) error {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("resolve workDir: %w", err)
	}

	snapFiles := make(map[string]bool)
	failedSet := make(map[string]bool)
	var failures []string

	for _, entry := range snap.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
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
			if err := ctx.Err(); err != nil {
				return err
			}
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
		if err := ctx.Err(); err != nil {
			return err
		}
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
