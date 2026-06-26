package worktree

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
)

// StageAll stages all working-tree changes (equivalent to 'drift add .').
// Returns the list of newly staged paths and the list of skipped paths
// (unsupported file types). Output formatting is the caller's responsibility.
func (w *Worktree) StageAll(idx *core.Index) (added, skipped []string, err error) {
	parentHashes, err := w.LoadParentTreeHashes()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load parent tree hashes: %w", err)
	}
	err = core.WalkWorkingDir(w.Root, func(path string, info os.FileInfo) error {
		fullPath := filepath.Join(w.Root, filepath.FromSlash(path))
		a, sk, err := w.addFile(path, fullPath, info, idx, parentHashes)
		if err != nil {
			return err
		}
		added = append(added, a...)
		skipped = append(skipped, sk...)
		return nil
	})
	if err != nil {
		return nil, nil, err
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
		added = append(added, path)
	}

	return added, skipped, nil
}

// StagePaths stages specific paths. Returns the list of newly staged paths
// and skipped paths. Output formatting is the caller's responsibility.
func (w *Worktree) StagePaths(idx *core.Index, paths []string) (added, skipped []string, err error) {
	parentHashes, err := w.LoadParentTreeHashes()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load parent tree hashes: %w", err)
	}

	for _, p := range paths {
		fullPath := filepath.Join(w.Root, filepath.FromSlash(p))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return nil, nil, fmt.Errorf("path not found: %s", p)
		}

		if info.IsDir() {
			a, sk, err := w.addDirectoryInto(p, idx, parentHashes)
			if err != nil {
				return nil, nil, err
			}
			added = append(added, a...)
			skipped = append(skipped, sk...)
		} else {
			a, sk, err := w.addFile(p, fullPath, info, idx, parentHashes)
			if err != nil {
				return nil, nil, err
			}
			added = append(added, a...)
			skipped = append(skipped, sk...)
		}
	}
	return added, skipped, nil
}

// StageWorktreeChanges re-stages worktree modifications into the index,
// so they can be captured by WIP save. This is a simplified version of
// StageAll that only updates entries for files that changed.
func (w *Worktree) StageWorktreeChanges(idx *core.Index) error {
	parentHashes, err := w.LoadParentTreeHashes()
	if err != nil {
		return fmt.Errorf("failed to load parent tree hashes: %w", err)
	}

	return core.WalkWorkingDir(w.Root, func(path string, info os.FileInfo) error {
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
	})
}

// PutBlobForAdd stores a blob from a file, applying LF normalization when
// AutoCRLF is configured.
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

// addFile stages a single file/symlink. Returns the list of newly staged
// paths (at most one) and the list of skipped paths (at most one, for
// unsupported file types).
func (w *Worktree) addFile(relPath, fullPath string, info os.FileInfo, idx *core.Index, parentTreeHashes map[string]string) (added, skipped []string, err error) {
	mode, err := core.NormalizeModeForPath(info.Mode(), relPath)
	if err != nil {
		return nil, []string{relPath}, nil
	}

	var hash string
	if mode == core.ModeSymlink {
		target, err := os.Readlink(fullPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read symlink %s: %w", relPath, err)
		}
		if err := core.ValidateSymlinkTarget(w.Root, relPath, target); err != nil {
			return nil, nil, fmt.Errorf("unsafe symlink %s: %w", relPath, err)
		}
		hash, err = w.Store.PutBlob([]byte(target))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to store symlink %s: %w", relPath, err)
		}
	} else {
		hash, err = w.PutBlobForAdd(fullPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to store %s: %w", relPath, err)
		}
	}

	if existing, err := idx.Entry(relPath); err == nil && existing.Hash == hash {
		return nil, nil, nil
	}

	// The index is supposed to be a full snapshot of all tracked files.
	// When a working file matches its parent-commit hash but is missing from
	// the index (e.g. the index was partially rebuilt), still record it so
	// that StageAll/StagePaths converge on a full snapshot. This is not
	// reported as "Added" — the file is unchanged — to keep output noise-free.
	if parentHash, ok := parentTreeHashes[relPath]; ok && parentHash == hash {
		entry := core.IndexEntry{
			Path:       relPath,
			Hash:       hash,
			ModifiedAt: info.ModTime(),
			Size:       info.Size(),
			Mode:       mode,
		}
		if err := idx.Add(entry); err != nil {
			return nil, nil, fmt.Errorf("failed to add %s: %w", relPath, err)
		}
		return nil, nil, nil
	}

	entry := core.IndexEntry{
		Path:       relPath,
		Hash:       hash,
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       mode,
	}

	if err := idx.Add(entry); err != nil {
		return nil, nil, fmt.Errorf("failed to add %s: %w", relPath, err)
	}

	return []string{relPath}, nil, nil
}

// addDirectoryInto walks a directory and adds files into the provided index.
func (w *Worktree) addDirectoryInto(dirPath string, idx *core.Index, parentHashes map[string]string) (added, skipped []string, err error) {
	fullDir := filepath.Join(w.Root, filepath.FromSlash(dirPath))

	err = core.WalkWorkingDirWithIgnore(fullDir, w.Root, func(path string, info os.FileInfo) error {
		relPath := filepath.ToSlash(filepath.Join(dirPath, path))
		fullPath := filepath.Join(w.Root, filepath.FromSlash(relPath))
		a, sk, err := w.addFile(relPath, fullPath, info, idx, parentHashes)
		if err != nil {
			return err
		}
		added = append(added, a...)
		skipped = append(skipped, sk...)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return added, skipped, nil
}

// ExpandAddPaths expands glob patterns in the given arguments and returns
// a deduplicated list of repository-relative paths. rootDir is the repository
// root used to compute repository-relative paths (independent of process cwd).
func ExpandAddPaths(rootDir string, args []string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	for _, arg := range args {
		if arg == "." {
			if !seen["."] {
				seen["."] = true
				result = append(result, ".")
			}
			continue
		}

		if strings.ContainsAny(arg, "*?[") {
			matches, err := filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid glob pattern %q: %w", arg, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("no matches for pattern: %s", arg)
			}
			for _, m := range matches {
				absPath, err := filepath.Abs(m)
				if err != nil {
					return nil, fmt.Errorf("cannot resolve relative path %q: %w", m, err)
				}
				rel, err := filepath.Rel(rootDir, absPath)
				if err != nil {
					return nil, fmt.Errorf("cannot resolve relative path %q: %w", m, err)
				}
				rel = filepath.ToSlash(rel)
				if !seen[rel] {
					seen[rel] = true
					result = append(result, rel)
				}
			}
		} else {
			if _, err := os.Lstat(arg); err != nil {
				return nil, fmt.Errorf("path not found: %s", arg)
			}
			absPath, err := filepath.Abs(arg)
			if err != nil {
				return nil, fmt.Errorf("cannot resolve relative path %q: %w", arg, err)
			}
			rel, err := filepath.Rel(rootDir, absPath)
			if err != nil {
				return nil, fmt.Errorf("cannot resolve relative path %q: %w", arg, err)
			}
			rel = filepath.ToSlash(rel)
			if !seen[rel] {
				seen[rel] = true
				result = append(result, rel)
			}
		}
	}

	return result, nil
}
