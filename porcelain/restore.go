package porcelain

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
)

// RestoreSnapshot restores workspace to a snapshot.
// If filePath is non-empty, only restore that specific file.
// If noBackup is false, a backup snapshot is created before restoring.
func RestoreSnapshot(store storage.Storer, workDir string, snapshotID core.SnapshotID, filePath string, noBackup bool) error {
	// Get target snapshot
	snap, err := store.GetSnapshot(snapshotID)
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}

	// Create backup snapshot if requested
	if !noBackup {
		backupMsg := fmt.Sprintf("backup: restore to %s", snapshotID.Hash.String())
		_, backupErr := CreateSnapshot(store, workDir, backupMsg, "drift")
		if backupErr != nil && backupErr.Error() != "nothing to save" {
			return fmt.Errorf("create backup: %w", backupErr)
		}
	}

	// Restore files
	for _, entry := range snap.Files {
		if filePath != "" && entry.Path != filePath {
			continue
		}

		fullPath := filepath.Join(workDir, entry.Path)

		if entry.Mode.IsDir() {
			if err := os.MkdirAll(fullPath, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", fullPath, err)
			}
			continue
		}

		// Assemble file content from chunks
		var assembled []byte
		for _, h := range entry.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				return fmt.Errorf("get chunk %s for %s: %w", h.String(), entry.Path, err)
			}
			assembled = append(assembled, chunk.Data...)
		}

		// Ensure parent directory exists
		parentDir := filepath.Dir(fullPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("create parent dir %s: %w", parentDir, err)
		}

		if err := os.WriteFile(fullPath, assembled, 0644); err != nil {
			return fmt.Errorf("write file %s: %w", fullPath, err)
		}

		// Restore original modification time
		if err := os.Chtimes(fullPath, time.Unix(entry.ModTime, 0), time.Unix(entry.ModTime, 0)); err != nil {
			return fmt.Errorf("set modtime %s: %w", fullPath, err)
		}
	}

	// Update HEAD reference
	headRef := &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: snapshotID.Hash,
	}
	if err := store.SetRef("HEAD", headRef); err != nil {
		return fmt.Errorf("update HEAD: %w", err)
	}

	// Rebuild index from restored files
	newIndex := &core.Index{}
	for _, entry := range snap.Files {
		if filePath != "" && entry.Path != filePath {
			continue
		}
		newIndex.Entries = append(newIndex.Entries, core.IndexEntry{
			Path:    entry.Path,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			Chunks:  entry.Chunks,
		})
	}
	if err := store.SetIndex(newIndex); err != nil {
		return fmt.Errorf("update index: %w", err)
	}

	return nil
}
