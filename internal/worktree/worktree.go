// Worktree represents the working directory and provides operations for
// writing blobs to disk, staging changes, checking for modifications, and
// managing work-in-progress (WIP) state. It bridges the storage layer
// (content-addressed objects) and the filesystem (working directory).
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

// errUntrackedFound is a sentinel error used internally by HasModifications
// to short-circuit the worktree walk when an untracked file is found.
var errUntrackedFound = errors.New("untracked file found")

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

// HasModifications checks whether the working tree has unstaged modifications
// (modified tracked files, deleted tracked files, or untracked files).
// If filters is non-empty, only paths matching the filters are checked.
func (w *Worktree) HasModifications(commit *core.Commit, idx *core.Index, filters []string) (bool, error) {
	// Build the set of tracked paths from both index and commit.
	tracked := make(map[string]string) // path → expected hash (index preferred)
	for _, e := range idx.Entries {
		if PathMatchesAny(e.Path, filters) {
			tracked[e.Path] = e.Hash
		}
	}

	if commit != nil {
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
			if _, inIdx := tracked[b.Path]; !inIdx {
				tracked[b.Path] = b.Hash
			}
		}
	}

	// Check each tracked file: does worktree match the expected hash?
	for path, expectedHash := range tracked {
		fullPath := filepath.Join(w.Root, filepath.FromSlash(path))
		info, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil // tracked file deleted
			}
			continue
		}

		// Symlink: compare target string.
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(fullPath)
			if err != nil {
				continue
			}
			data, err := w.Store.GetBlob(expectedHash)
			if err != nil {
				continue
			}
			if target != string(data) {
				return true, nil
			}
			continue
		}

		// Fast path: size comparison.
		blobSize, sizeErr := w.Store.GetBlobSize(expectedHash)
		if sizeErr == nil && info.Size() != blobSize {
			return true, nil
		}

		fileHash, err := core.CalculateHashFromFile(fullPath)
		if err != nil {
			continue
		}
		if fileHash != expectedHash {
			return true, nil
		}
	}

	// Check for untracked files (not in index, not in commit).
	walkErr := core.WalkWorkingDir(w.Root, func(path string, info os.FileInfo) error {
		if _, isTracked := tracked[path]; isTracked {
			return nil
		}
		if len(filters) > 0 && !PathMatchesAny(path, filters) {
			return nil
		}
		return errUntrackedFound
	})
	if walkErr == errUntrackedFound {
		return true, nil
	}
	if walkErr != nil {
		return false, walkErr
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

// CleanUntracked removes untracked files from the working tree.
// Returns the list of removed file paths.
// If dryRun is true, returns the list without deleting.
// If includeDirs is true, also removes empty directories after file deletion.
func (w *Worktree) CleanUntracked(includeDirs bool, dryRun bool) ([]string, error) {
	var idx core.Index
	if err := w.Store.LoadIndex(&idx); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	// Build a set of tracked paths (index + last commit).
	tracked := make(map[string]bool)
	for _, e := range idx.Entries {
		tracked[e.Path] = true
	}
	parentHashes, err := w.LoadParentTreeHashes()
	if err != nil {
		return nil, fmt.Errorf("failed to load parent tree hashes: %w", err)
	}
	for p := range parentHashes {
		tracked[p] = true
	}

	// Walk the working dir and find untracked files.
	var untracked []string
	err = core.WalkWorkingDirWithIgnore(w.Root, w.Root, func(path string, info os.FileInfo) error {
		if !tracked[path] {
			untracked = append(untracked, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if dryRun {
		return untracked, nil
	}

	// Delete untracked files.
	for _, p := range untracked {
		fullPath := filepath.Join(w.Root, filepath.FromSlash(p))
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove %s: %w", p, err)
		}
	}

	// Optionally remove empty directories left behind by the deletions.
	if includeDirs {
		w.CleanEmptyDirs(untracked)
	}

	return untracked, nil
}
