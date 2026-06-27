// Worktree represents the working directory and provides operations for
// writing blobs to disk, detecting changes, checking for modifications, and
// managing work-in-progress (WIP) state. It bridges the storage layer
// (content-addressed objects) and the filesystem (working directory).
package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
)

// errUntrackedFound is a sentinel error used internally by HasModifications
// to short-circuit the worktree walk when an untracked file is found.
var errUntrackedFound = errors.New("untracked file found")

// Worktree represents the working directory and provides operations for
// writing blobs to disk, detecting changes, and checking for modifications.
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
	sort.Slice(deletedPaths, func(i, j int) bool {
		return strings.Count(deletedPaths[i], "/") > strings.Count(deletedPaths[j], "/")
	})
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
			delete(seen, filepath.Dir(dir))
			dir = filepath.Dir(dir)
		}
	}
}

// HasModifications checks whether the working tree has unsaved modifications
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

		// Fast path: size comparison. Skip when autocrlf may cause
		// size mismatch between LF blob and CRLF worktree file.
		if !(runtime.GOOS == "windows" && w.AutoCRLF == "true") {
			blobSize, sizeErr := w.Store.GetBlobSize(expectedHash)
			if sizeErr == nil && info.Size() != blobSize {
				return true, nil
			}
		}

		var fileHash string
		var hashErr error
		if runtime.GOOS == "windows" && w.AutoCRLF == "true" {
			fileHash, hashErr = core.CalculateHashFromFileLF(fullPath)
		} else {
			fileHash, hashErr = core.CalculateHashFromFile(fullPath)
		}
		if hashErr != nil {
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

// LoadParentTreeBlobs returns the blob entries of the current branch's latest
// commit tree (path, hash, mode). Returns nil if no commit exists yet.
func (w *Worktree) LoadParentTreeBlobs() ([]core.BlobEntry, error) {
	branch, err := w.Store.GetRef("HEAD")
	if err != nil {
		if !errors.Is(err, storage.ErrObjectNotFound) {
			return nil, err
		}
		branch = "main"
	}
	if branch == "" {
		return nil, nil
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
	return reader.ListBlobs(tree, "")
}

// LoadParentTreeHashes returns a map of path -> blob hash from the current
// branch's latest commit tree. Returns nil if no commit exists yet.
func (w *Worktree) LoadParentTreeHashes() (map[string]string, error) {
	blobs, err := w.LoadParentTreeBlobs()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(blobs))
	for _, b := range blobs {
		m[b.Path] = b.Hash
	}
	return m, nil
}

// BuildIndexFromCommit builds a full-snapshot index from the current branch's
// latest commit tree. Each entry is populated with the committed blob's hash
// and mode, plus the on-disk mtime/size when the working-tree file is present
// (so the status fast-path stays effective). When the working file is absent
// (the file was deleted in the worktree), mtime/size are zero — status will
// correctly report Worktree=Deleted via the size/hash fallback. Returns an
// empty index when there is no commit yet (fresh repository).
func (w *Worktree) BuildIndexFromCommit() (*core.Index, error) {
	blobs, err := w.LoadParentTreeBlobs()
	if err != nil {
		return nil, err
	}
	idx := &core.Index{}
	for _, b := range blobs {
		entry := core.IndexEntry{
			Path: b.Path,
			Hash: b.Hash,
			Mode: b.Mode,
		}
		fullPath := filepath.Join(w.Root, filepath.FromSlash(b.Path))
		if info, err := os.Lstat(fullPath); err == nil {
			entry.ModifiedAt = info.ModTime()
			entry.Size = info.Size()
		}
		if err := idx.Add(entry); err != nil {
			return nil, fmt.Errorf("failed to add %s to index: %w", b.Path, err)
		}
	}
	return idx, nil
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

// BuildChangedIndex scans the working directory and builds an index reflecting
// all current file state, comparing against the parent tree. It auto-detects
// modified, added (respecting .driftignore), and deleted files.
// Returns the complete index and the list of paths that changed.
func (w *Worktree) BuildChangedIndex(parentTree *core.Tree) (*core.Index, []string, error) {
	idx, err := w.BuildIndexFromCommit()
	if err != nil {
		return nil, nil, err
	}
	if idx == nil {
		idx = &core.Index{}
	}

	currentFiles := make(map[string]string)

	err = core.WalkWorkingDirWithIgnore(w.Root, w.Root, func(path string, info os.FileInfo) error {
		fullPath := filepath.Join(w.Root, filepath.FromSlash(path))
		mode := info.Mode()

		if !mode.IsRegular() && mode&os.ModeSymlink == 0 {
			return nil
		}

		if mode&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(fullPath)
			if readErr != nil {
				return nil
			}
			currentFiles[path] = core.CalculateHash([]byte(target))
			return nil
		}

		fileHash, hashErr := core.CalculateHashFromFile(fullPath)
		if hashErr != nil {
			return nil
		}
		currentFiles[path] = fileHash
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	var changed []string
	var deleted []string

	for _, entry := range idx.Entries {
		newHash, onDisk := currentFiles[entry.Path]
		if !onDisk {
			changed = append(changed, entry.Path)
			deleted = append(deleted, entry.Path)
			continue
		}
		if entry.Hash != newHash {
			changed = append(changed, entry.Path)
			entry.Hash = newHash
			fullPath := filepath.Join(w.Root, filepath.FromSlash(entry.Path))
			if info, statErr := os.Lstat(fullPath); statErr == nil {
				entry.ModifiedAt = info.ModTime()
				entry.Size = info.Size()
				if info.Mode()&os.ModeSymlink != 0 {
					entry.Mode = core.ModeSymlink
				} else if info.Mode()&0111 != 0 {
					entry.Mode = core.ModeExecutable
				} else {
					entry.Mode = core.ModeRegular
				}
			}
			if addErr := idx.Add(entry); addErr != nil {
				return nil, nil, fmt.Errorf("failed to update %s in index: %w", entry.Path, addErr)
			}
		}
	}

	for _, path := range deleted {
		idx.Remove(path)
	}

	for path, hash := range currentFiles {
		if idx.Has(path) {
			continue
		}
		changed = append(changed, path)
		fullPath := filepath.Join(w.Root, filepath.FromSlash(path))
		info, statErr := os.Lstat(fullPath)
		mode := uint32(core.ModeRegular)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				mode = core.ModeSymlink
			} else if info.Mode()&0111 != 0 {
				mode = core.ModeExecutable
			}
		}
		entry := core.IndexEntry{
			Path: path,
			Hash: hash,
			Mode: mode,
		}
		if statErr == nil {
			entry.ModifiedAt = info.ModTime()
			entry.Size = info.Size()
		}
		if addErr := idx.Add(entry); addErr != nil {
			return nil, nil, fmt.Errorf("failed to add %s to index: %w", path, addErr)
		}
	}

	sort.Strings(changed)
	return idx, changed, nil
}

func (w *Worktree) StageWorktreeChanges(idx *core.Index) error {
	parentHashes, err := w.LoadParentTreeHashes()
	if err != nil {
		return fmt.Errorf("failed to load parent tree hashes: %w", err)
	}

	if err := core.WalkWorkingDir(w.Root, func(path string, info os.FileInfo) error {
		fullPath := filepath.Join(w.Root, filepath.FromSlash(path))

		mode, err := core.NormalizeModeForPath(info.Mode(), path)
		if err != nil {
			return fmt.Errorf("failed to normalize mode for %s: %w", path, err)
		}

		var hash string
		if mode == core.ModeSymlink {
			target, err := os.Readlink(fullPath)
			if err != nil {
				return fmt.Errorf("failed to read symlink %s: %w", path, err)
			}
			if err := core.ValidateSymlinkTarget(w.Root, path, target); err != nil {
				return fmt.Errorf("unsafe symlink %s: %w", path, err)
			}
			hash, err = w.Store.PutBlob([]byte(target))
			if err != nil {
				return fmt.Errorf("failed to store symlink %s: %w", path, err)
			}
		} else {
			hash, err = w.PutBlobForAdd(fullPath)
			if err != nil {
				return fmt.Errorf("failed to store %s: %w", path, err)
			}
		}

		if parentHash, ok := parentHashes[path]; ok && parentHash == hash {
			return nil
		}

		entry := core.IndexEntry{
			Path:       path,
			Hash:       hash,
			ModifiedAt: info.ModTime(),
			Size:       info.Size(),
			Mode:       mode,
		}
		return idx.Add(entry)
	}); err != nil {
		return err
	}

	var deleted []string
	for _, entry := range idx.Entries {
		fullPath := filepath.Join(w.Root, filepath.FromSlash(entry.Path))
		if _, err := os.Lstat(fullPath); os.IsNotExist(err) {
			deleted = append(deleted, entry.Path)
		}
	}
	for _, path := range deleted {
		idx.Remove(path)
	}

	return nil
}

func (w *Worktree) PutBlobForAdd(path string) (string, error) {
	if w.AutoCRLF == "" {
		return w.Store.PutBlobFromFile(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	headBuf := core.GetByteSlice()
	if cap(*headBuf) < 8192 {
		*headBuf = make([]byte, 8192)
	}
	head := (*headBuf)[:8192]
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		core.PutByteSlice(headBuf)
		return "", err
	}
	head = head[:n]
	defer core.PutByteSlice(headBuf)

	r := io.MultiReader(bytes.NewReader(head), f)

	if bytes.Contains(head, []byte{0}) {
		return w.Store.PutBlobFromReader(r)
	}

	buf := core.GetBuffer()
	defer core.PutBuffer(buf)
	buf.Reset()

	writer := core.NewLFWriter(buf)
	if _, err := io.Copy(writer, r); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return w.Store.PutBlobFromReader(buf)
}

func (w *Worktree) StoreBlob(fullPath string) (hash string, mode uint32, err error) {
	info, err := os.Lstat(fullPath)
	if err != nil {
		return "", 0, err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(fullPath)
		if err != nil {
			return "", 0, err
		}
		hash, blbErr := w.Store.PutBlob([]byte(target))
		if blbErr != nil {
			return "", 0, blbErr
		}
		return hash, core.ModeSymlink, nil
	}

	if !info.Mode().IsRegular() {
		mode, modeErr := core.NormalizeModeForPath(info.Mode(), "")
		if modeErr != nil {
			return "", 0, modeErr
		}
		return "", mode, fmt.Errorf("unsupported file type")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", 0, err
	}

	isText := !bytes.Contains(data, []byte{0})
	if isText {
		data = bytes.ReplaceAll(data, []byte{'\r', '\n'}, []byte{'\n'})
	}

	hash, blbErr := w.Store.PutBlob(data)
	if blbErr != nil {
		return "", 0, blbErr
	}

	finalMode := uint32(core.ModeRegular)
	if info.Mode()&0111 != 0 {
		finalMode = core.ModeExecutable
	}

	return hash, finalMode, nil
}
