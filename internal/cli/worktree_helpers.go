package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
)

// writeBlobToWorktree writes a blob's content to the working tree at the given
// root directory. It handles symlinks, executable permissions, and path
// validation. Returns the IndexEntry reflecting the on-disk state.
//
// Regular files are streamed via GetBlobToWriter to avoid loading large blobs
// (PSD, video) into memory. Symlinks still use GetBlob since their target
// strings are small.
func writeBlobToWorktree(store *storage.Store, root string, blob core.BlobEntry) (core.IndexEntry, error) {
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return core.IndexEntry{}, fmt.Errorf("unsafe path %q: %w", blob.Path, err)
	}

	fullPath := filepath.Join(root, filepath.FromSlash(blob.Path))

	// Symlink: load full blob (small — just the target string).
	if blob.Mode == core.ModeSymlink {
		data, err := store.GetBlob(blob.Hash)
		if err != nil {
			return core.IndexEntry{}, err
		}
		if err := removeExistingPath(fullPath); err != nil {
			return core.IndexEntry{}, err
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return core.IndexEntry{}, err
		}
		target := string(data)
		// P0-#17: reject symlinks that escape the repository root, which
		// could be planted by a malicious repository to read/write arbitrary
		// files on the user's machine during restore/switch.
		if err := core.ValidateSymlinkTarget(root, blob.Path, target); err != nil {
			return core.IndexEntry{}, fmt.Errorf("unsafe symlink %q: %w", blob.Path, err)
		}
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

	// Regular/executable file: stream to disk without loading full blob.
	perm := os.FileMode(core.ToOSFileMode(blob.Mode))

	// Fast path: if the existing file already matches, skip the write.
	// Compare via size + hash instead of loading both into memory.
	if info, err := os.Stat(fullPath); err == nil {
		blobSize, sizeErr := store.GetBlobSize(blob.Hash)
		if sizeErr == nil && info.Size() == blobSize {
			// Size matches. For non-CRLF mode, verify hash to avoid
			// rewriting identical content. CRLF mode changes content,
			// so hash comparison doesn't apply — just rewrite.
			if !(runtime.GOOS == "windows" &&
				sharedConfig != nil && sharedConfig.Core.AutoCRLF == "true") {
				fileHash, hashErr := core.CalculateHashFromFile(fullPath)
				if hashErr == nil && fileHash == blob.Hash {
					_ = os.Chmod(fullPath, perm)
					info, _ := os.Stat(fullPath)
					return core.IndexEntry{
						Path:       blob.Path,
						Hash:       blob.Hash,
						ModifiedAt: info.ModTime(),
						Size:       info.Size(),
						Mode:       blob.Mode,
					}, nil
				}
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return core.IndexEntry{}, err
	}

	// Stream blob content to the file. GetBlobToWriter verifies the hash
	// during streaming, catching corruption without a separate pass.
	f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return core.IndexEntry{}, err
	}

	var writer io.Writer = f
	if runtime.GOOS == "windows" &&
		sharedConfig != nil && sharedConfig.Core.AutoCRLF == "true" {
		writer = core.NewCRLFWriter(f)
	}

	if err := store.GetBlobToWriter(blob.Hash, writer); err != nil {
		f.Close()
		return core.IndexEntry{}, err
	}
	if err := f.Close(); err != nil {
		return core.IndexEntry{}, err
	}
	// Force-set permissions for existing files.
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
				data, err := sharedStore.GetBlob(b.Hash)
				if err != nil {
					return false, fmt.Errorf("failed to read blob %s for %s: %w", b.Hash, b.Path, err)
				}
				if target != string(data) {
					return true, nil
				}
			}
			_ = info
			continue
		}

		// Size fast-path: skip hash computation if sizes differ.
		blobSize, sizeErr := sharedStore.GetBlobSize(b.Hash)
		if sizeErr == nil && info.Size() != blobSize {
			return true, nil
		}

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
