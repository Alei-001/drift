package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
)

// writeBlobToWorktree writes a blob's content to the working tree at the given
// root directory. It handles symlinks, executable permissions, and path
// validation. Returns the IndexEntry reflecting the on-disk state.
func writeBlobToWorktree(store *storage.Store, root string, blob core.BlobEntry) (core.IndexEntry, error) {
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return core.IndexEntry{}, fmt.Errorf("unsafe path %q: %w", blob.Path, err)
	}

	fullPath := filepath.Join(root, filepath.FromSlash(blob.Path))

	data, err := store.GetBlob(blob.Hash)
	if err != nil {
		return core.IndexEntry{}, err
	}

	// Symlink: remove existing entry and create a symlink to the stored target.
	if blob.Mode == core.ModeSymlink {
		if err := removeExistingPath(fullPath); err != nil {
			return core.IndexEntry{}, err
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return core.IndexEntry{}, err
		}
		target := string(data)
		if err := os.Symlink(target, fullPath); err != nil {
			return core.IndexEntry{}, fmt.Errorf("failed to create symlink %s: %w", blob.Path, err)
		}
		info, _ := os.Lstat(fullPath)
		return core.IndexEntry{
			Path:       blob.Path,
			Hash:       blob.Hash,
			ModifiedAt: info.ModTime(),
			Size:       info.Size(),
			Mode:       blob.Mode,
		}, nil
	}

	// Regular/executable file.
	existing, err := os.ReadFile(fullPath)
	if err == nil && bytes.Equal(existing, data) {
		// Content matches; ensure permissions are correct.
		_ = os.Chmod(fullPath, os.FileMode(core.ToOSFileMode(blob.Mode)))
		info, _ := os.Stat(fullPath)
		return core.IndexEntry{
			Path:       blob.Path,
			Hash:       blob.Hash,
			ModifiedAt: info.ModTime(),
			Size:       info.Size(),
			Mode:       blob.Mode,
		}, nil
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return core.IndexEntry{}, err
	}

	perm := os.FileMode(core.ToOSFileMode(blob.Mode))
	if err := os.WriteFile(fullPath, data, perm); err != nil {
		return core.IndexEntry{}, err
	}
	// WriteFile only applies perm on creation; force-set for existing files.
	if err := os.Chmod(fullPath, perm); err != nil {
		return core.IndexEntry{}, err
	}

	info, _ := os.Stat(fullPath)
	return core.IndexEntry{
		Path:       blob.Path,
		Hash:       blob.Hash,
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       blob.Mode,
	}, nil
}

// removeExistingPath removes a file/symlink/dir at path if it exists.
func removeExistingPath(fullPath string) error {
	if _, err := os.Lstat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(fullPath)
}

// hasWorktreeModifications checks whether any tracked file in the current
// branch's commit has unstaged modifications in the working tree.
// Used by restore/switch to prevent silent overwrites (Issue 11).
func hasWorktreeModifications() (bool, error) {
	commit, err := currentBranchCommit(sharedStore)
	if err != nil {
		return false, err
	}
	if commit == nil {
		return false, nil
	}

	tree, err := sharedStore.GetTree(commit.TreeHash)
	if err != nil {
		return false, nil
	}

	reader := core.NewTreeReader(sharedStore)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return false, err
	}

	var idx core.Index
	if err := sharedStore.LoadIndex(&idx); err != nil {
		return false, err
	}

	for _, b := range blobs {
		// If file is staged, its worktree status is checked elsewhere.
		if idx.Has(b.Path) {
			continue
		}
		fullPath := filepath.Join(sharedDir, filepath.FromSlash(b.Path))
		info, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil // tracked file deleted in worktree
			}
			continue
		}

		// Symlink: compare link target.
		if b.Mode == core.ModeSymlink {
			target, err := os.Readlink(fullPath)
			if err != nil || target != "" {
				// Readlink error or mismatched target means modification.
				data, _ := sharedStore.GetBlob(b.Hash)
				if target != string(data) {
					return true, nil
				}
			}
			_ = info
			continue
		}

		// Quick mtime+size check before hashing.
		fileHash, err := core.CalculateHashFromFile(fullPath)
		if err != nil {
			continue
		}
		if fileHash != b.Hash {
			return true, nil
		}
	}

	return false, nil
}

// cleanEmptyDirsAffected removes empty directories along the parent chains of
// the given deleted paths. This avoids a full filepath.Walk over the worktree
// (Issue 19).
func cleanEmptyDirsAffected(root string, deletedPaths []string) {
	seen := make(map[string]bool)
	// Process deepest first so parents are checked after children.
	for _, p := range deletedPaths {
		dir := filepath.Dir(p)
		for dir != "." && dir != string(filepath.Separator) && !seen[dir] {
			seen[dir] = true
			fullDir := filepath.Join(root, filepath.FromSlash(dir))
			entries, err := os.ReadDir(fullDir)
			if err != nil || len(entries) > 0 {
				break
			}
			// Don't remove the root or .drift.
			rel, _ := filepath.Rel(root, fullDir)
			if rel == "." || rel == ".drift" {
				break
			}
			if err := os.Remove(fullDir); err != nil {
				break
			}
			dir = filepath.Dir(dir)
		}
	}
}
