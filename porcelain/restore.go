package porcelain

import (
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

// RestoreSnapshot restores workspace to a snapshot.
// If filePath is non-empty, only restore that specific file.
// If noBackup is false, a backup snapshot is created before restoring.
func RestoreSnapshot(store storage.Storer, workDir string, snapshotID core.SnapshotID, filePath string, noBackup bool) (string, error) {
	// Get target snapshot
	snap, err := store.GetSnapshot(snapshotID)
	if err != nil {
		return "", fmt.Errorf("get snapshot: %w", err)
	}

	// Create backup snapshot if requested
	var backupID string
	if !noBackup {
		backupMsg := fmt.Sprintf("backup: restore to %s", snapshotID.Hash.String())
		backupSnap, backupErr := CreateSnapshot(store, workDir, backupMsg, "drift", nil)
		if backupErr != nil {
			if backupErr.Error() != "nothing to save" {
				return "", fmt.Errorf("create backup: %w", backupErr)
			}
		} else {
			backupID = backupSnap.ShortID()
		}
	}

	// Normalize path and resolve absolute paths
	var pathErr error
	filePath, pathErr = pathutil.RelToWorkDir(workDir, filePath)
	if pathErr != nil {
		return "", fmt.Errorf("cannot resolve path: %w", pathErr)
	}

	// Track restored entry for single-file index update
	var restoredEntry *core.FileEntry
	// Track snapshot file paths for full-restore cleanup
	snapFiles := make(map[string]bool)

	// Resolve absolute workDir for path traversal protection
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve workDir: %w", err)
	}

	// Restore files
	for _, entry := range snap.Files {
		if filePath != "" && entry.Path != filePath {
			continue
		}

		fullPath := filepath.Join(absWorkDir, entry.Path)

		// Verify the resolved path stays within workDir (defense in depth against path traversal)
		if fullPath != absWorkDir && !strings.HasPrefix(fullPath, absWorkDir+string(filepath.Separator)) {
			continue
		}

		if entry.Mode.IsDir() {
			if err := os.MkdirAll(fullPath, 0755); err != nil {
				return "", fmt.Errorf("create dir %s: %w", fullPath, err)
			}
			snapFiles[entry.Path] = true
			continue
		}

		// Assemble file content from chunks
		var assembled []byte
		for _, h := range entry.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				return "", fmt.Errorf("get chunk %s for %s: %w", h.String(), entry.Path, err)
			}
			assembled = append(assembled, chunk.Data...)
		}

		// Ensure parent directory exists
		parentDir := filepath.Dir(fullPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return "", fmt.Errorf("create parent dir %s: %w", parentDir, err)
		}

		// Restore original file permissions
		perm := os.FileMode(entry.Mode & 0o777)
		if perm == 0 {
			perm = 0644
		}
		if err := os.WriteFile(fullPath, assembled, perm); err != nil {
			return "", fmt.Errorf("write file %s: %w", fullPath, err)
		}

		// Restore original modification time
		if err := os.Chtimes(fullPath, time.Unix(entry.ModTime, 0), time.Unix(entry.ModTime, 0)); err != nil {
			return "", fmt.Errorf("set modtime %s: %w", fullPath, err)
		}

		snapFiles[entry.Path] = true

		// Track the restored entry for single-file index update
		if filePath != "" {
			entryCopy := entry
			restoredEntry = &entryCopy
		}
	}

	if filePath == "" {
		// Full restore: remove files in workspace not in the snapshot
		cleanErr := fsutil.Walk(workDir, func(path string, info os.FileInfo) error {
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
		if cleanErr != nil {
			return "", fmt.Errorf("clean workspace: %w", cleanErr)
		}

		// Update HEAD reference
		headRef := &core.Reference{
			Name:   "HEAD",
			Type:   core.RefTypeHead,
			Target: snapshotID.Hash,
		}
		if err := store.SetRef("HEAD", headRef); err != nil {
			return "", fmt.Errorf("update HEAD: %w", err)
		}

		// Rebuild index from restored files
		newIndex := &core.Index{}
		for _, entry := range snap.Files {
			newIndex.Entries = append(newIndex.Entries, core.IndexEntry{
				Path:    entry.Path,
				Hash:    entry.Hash,
				Size:    entry.Size,
				ModTime: entry.ModTime,
				Chunks:  entry.Chunks,
			})
		}
		if err := store.SetIndex(newIndex); err != nil {
			return "", fmt.Errorf("update index: %w", err)
		}
	} else {
		// Single-file restore: do not move HEAD, only update the restored entry
		if restoredEntry != nil {
			existingIndex, err := store.GetIndex()
			if err != nil {
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
			if err := store.SetIndex(existingIndex); err != nil {
				return "", fmt.Errorf("update index: %w", err)
			}
		}
	}

	return backupID, nil
}
