// Package worktree encapsulates working-tree file operations: writing blobs
// to disk, staging changes, checking for modifications, and managing
// work-in-progress (WIP) state. It bridges the storage layer (content-addressed
// objects) and the filesystem (working directory).
package worktree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
)

// Worktree represents the working directory and provides operations for
// writing blobs to disk, staging changes, and checking for modifications.
type Worktree struct {
	Store    *storage.Store
	Root     string
	AutoCRLF string
}

// New creates a new Worktree.
func New(store *storage.Store, root, autoCRLF string) *Worktree {
	return &Worktree{Store: store, Root: root, AutoCRLF: autoCRLF}
}

// WriteBlob writes a blob's content to the working tree. It handles symlinks,
// executable permissions, and path validation. Returns the IndexEntry
// reflecting the on-disk state.
func (w *Worktree) WriteBlob(blob core.BlobEntry) (core.IndexEntry, error) {
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return core.IndexEntry{}, fmt.Errorf("unsafe path %q: %w", blob.Path, err)
	}

	fullPath := filepath.Join(w.Root, filepath.FromSlash(blob.Path))

	// Symlink: load full blob (small — just the target string).
	if blob.Mode == core.ModeSymlink {
		data, err := w.Store.GetBlob(blob.Hash)
		if err != nil {
			return core.IndexEntry{}, err
		}
		if err := RemovePath(fullPath); err != nil {
			return core.IndexEntry{}, err
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return core.IndexEntry{}, err
		}
		target := string(data)
		if err := core.ValidateSymlinkTarget(w.Root, blob.Path, target); err != nil {
			return core.IndexEntry{}, fmt.Errorf("unsafe symlink %q: %w", blob.Path, err)
		}
		if err := os.Symlink(target, fullPath); err != nil {
			return core.IndexEntry{}, fmt.Errorf("failed to create symlink %s: %w", blob.Path, err)
		}
		info, err := os.Lstat(fullPath)
		if err != nil {
			return core.IndexEntry{}, fmt.Errorf("failed to stat symlink %s: %w", blob.Path, err)
		}
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
	if info, err := os.Stat(fullPath); err == nil {
		blobSize, sizeErr := w.Store.GetBlobSize(blob.Hash)
		if sizeErr == nil && info.Size() == blobSize {
			if !(runtime.GOOS == "windows" && w.AutoCRLF == "true") {
				fileHash, hashErr := core.CalculateHashFromFile(fullPath)
				if hashErr == nil && fileHash == blob.Hash {
					_ = os.Chmod(fullPath, perm)
					info, err := os.Stat(fullPath)
					if err != nil {
						return core.IndexEntry{}, fmt.Errorf("failed to stat %s: %w", blob.Path, err)
					}
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

	f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return core.IndexEntry{}, err
	}

	var writer io.Writer = f
	if runtime.GOOS == "windows" && w.AutoCRLF == "true" {
		writer = core.NewCRLFWriter(f)
	}

	if err := w.Store.GetBlobToWriter(blob.Hash, writer); err != nil {
		f.Close()
		_ = os.Remove(fullPath)
		return core.IndexEntry{}, err
	}
	if err := f.Close(); err != nil {
		return core.IndexEntry{}, err
	}
	if err := os.Chmod(fullPath, perm); err != nil {
		return core.IndexEntry{}, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return core.IndexEntry{}, fmt.Errorf("failed to stat %s: %w", blob.Path, err)
	}
	return core.IndexEntry{
		Path:       blob.Path,
		Hash:       blob.Hash,
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       blob.Mode,
	}, nil
}

// RemovePath removes a file/symlink/dir at path if it exists.
func RemovePath(fullPath string) error {
	if _, err := os.Lstat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(fullPath)
}

// CleanEmptyDirs removes empty directories along the parent chains of the
// given deleted paths (using forward-slash paths relative to root).
func (w *Worktree) CleanEmptyDirs(deletedPaths []string) {
	seen := make(map[string]bool)
	for _, p := range deletedPaths {
		dir := filepath.Dir(p)
		for dir != "." && dir != string(filepath.Separator) && !seen[dir] {
			seen[dir] = true
			fullDir := filepath.Join(w.Root, filepath.FromSlash(dir))
			entries, err := os.ReadDir(fullDir)
			if err != nil || len(entries) > 0 {
				break
			}
			rel, _ := filepath.Rel(w.Root, fullDir)
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

// HasModifications checks whether any tracked file in the current branch's
// commit has unstaged modifications in the working tree.
// If filters is non-empty, only paths matching the filters are checked.
func (w *Worktree) HasModifications(commit *core.Commit, idx *core.Index, filters []string) (bool, error) {
	if commit == nil {
		return false, nil
	}

	tree, err := w.Store.GetTree(commit.TreeHash)
	if err != nil {
		return false, err
	}

	reader := core.NewTreeReader(w.Store)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return false, err
	}

	for _, b := range blobs {
		if !PathMatchesAny(b.Path, filters) {
			continue
		}
		if idx.Has(b.Path) {
			continue
		}
		fullPath := filepath.Join(w.Root, filepath.FromSlash(b.Path))
		info, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			continue
		}

		if b.Mode == core.ModeSymlink {
			target, err := os.Readlink(fullPath)
			if err != nil || target != "" {
				data, err := w.Store.GetBlob(b.Hash)
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

		blobSize, sizeErr := w.Store.GetBlobSize(b.Hash)
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

// LoadParentTreeHashes returns a map of path -> blob hash from the current
// branch's latest commit tree. Returns nil if no commit exists yet.
func (w *Worktree) LoadParentTreeHashes() (map[string]string, error) {
	branch, err := w.Store.GetRef("HEAD")
	if err != nil {
		if !errors.Is(err, storage.ErrObjectNotFound) {
			return nil, err
		}
		branch = "main"
	}
	commitHash, err := w.Store.GetRef(branch)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if commitHash == "" {
		return nil, nil
	}
	commit, err := w.Store.GetCommit(commitHash)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, nil
		}
		return nil, err
	}
	tree, err := w.Store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, err
	}
	reader := core.NewTreeReader(w.Store)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(blobs))
	for _, b := range blobs {
		m[b.Path] = b.Hash
	}
	return m, nil
}
