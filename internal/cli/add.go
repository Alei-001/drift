package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <path> [<path>...]",
	Short: "Add file contents to the staging area",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Expand glob patterns and collect unique paths.
		paths, err := expandAddPaths(args)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return fmt.Errorf("no matching files found")
		}

		// Special case: "." means add all.
		if len(paths) == 1 && paths[0] == "." {
			return addAll(sharedStore, sharedDir)
		}

		return addPaths(sharedStore, sharedDir, paths)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}

// expandAddPaths expands glob patterns in the given arguments and returns
// a deduplicated list of repository-relative paths. Literal paths (without
// glob metacharacters) are passed through as-is if they exist.
func expandAddPaths(args []string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	for _, arg := range args {
		// "." is a special case — pass through.
		if arg == "." {
			if !seen["."] {
				seen["."] = true
				result = append(result, ".")
			}
			continue
		}

		// Check if the argument contains glob metacharacters.
		if strings.ContainsAny(arg, "*?[") {
			matches, err := filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid glob pattern %q: %w", arg, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("no matches for pattern: %s", arg)
			}
			for _, m := range matches {
				rel, err := filepath.Rel(".", m)
				if err != nil {
					rel = m
				}
				rel = filepath.ToSlash(rel)
				if !seen[rel] {
					seen[rel] = true
					result = append(result, rel)
				}
			}
		} else {
			// Literal path — verify it exists.
			if _, err := os.Lstat(arg); err != nil {
				return nil, fmt.Errorf("path not found: %s", arg)
			}
			rel := filepath.ToSlash(arg)
			if !seen[rel] {
				seen[rel] = true
				result = append(result, rel)
			}
		}
	}

	return result, nil
}

// loadParentTreeHashes returns a map of path → blob hash from the current
// branch's latest commit tree. Returns nil if no commit exists yet.
func loadParentTreeHashes(store *storage.Store) map[string]string {
	branch, _ := store.GetRef("HEAD")
	if branch == "" {
		branch = "main"
	}
	commitHash, err := store.GetRef(branch)
	if err != nil || commitHash == "" {
		return nil
	}
	commit, err := store.GetCommit(commitHash)
	if err != nil {
		return nil
	}
	tree, err := store.GetTree(commit.TreeHash)
	if err != nil {
		return nil
	}
	reader := core.NewTreeReader(store)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(blobs))
	for _, b := range blobs {
		m[b.Path] = b.Hash
	}
	return m
}

// addPaths processes multiple paths in a single staging operation, sharing
// the index and parent-tree-hash map across all paths for efficiency.
func addPaths(store *storage.Store, root string, paths []string) error {
	var idx core.Index
	if err := store.LoadIndex(&idx); err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	parentHashes := loadParentTreeHashes(store)
	var added int

	for _, p := range paths {
		fullPath := filepath.Join(root, filepath.FromSlash(p))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return fmt.Errorf("path not found: %s", p)
		}

		if info.IsDir() {
			// Walk directory and add files within.
			n, err := addDirectoryInto(store, root, p, &idx, parentHashes)
			if err != nil {
				return err
			}
			added += n
		} else {
			wasAdded, err := addFile(store, root, p, fullPath, info, &idx, parentHashes)
			if err != nil {
				return err
			}
			if wasAdded {
				added++
			}
		}
	}

	if err := store.SaveIndex(&idx); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	if added > 0 {
		fmt.Printf("Added %d file(s)\n", added)
	}
	return nil
}

// addDirectoryInto walks a directory and adds files into the provided index.
// Returns the count of newly added files.
func addDirectoryInto(store *storage.Store, root, dirPath string, idx *core.Index, parentHashes map[string]string) (int, error) {
	fullDir := filepath.Join(root, filepath.FromSlash(dirPath))
	var added int

	err := core.WalkWorkingDirWithIgnore(fullDir, root, func(path string, info os.FileInfo) error {
		relPath := filepath.ToSlash(filepath.Join(dirPath, path))
		fullPath := filepath.Join(root, filepath.FromSlash(relPath))
		wasAdded, err := addFile(store, root, relPath, fullPath, info, idx, parentHashes)
		if err != nil {
			return err
		}
		if wasAdded {
			added++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

func addAll(store *storage.Store, root string) error {
	var idx core.Index
	if err := store.LoadIndex(&idx); err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	parentHashes := loadParentTreeHashes(store)
	var added int
	err := core.WalkWorkingDir(root, func(path string, info os.FileInfo) error {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		wasAdded, err := addFile(store, root, path, fullPath, info, &idx, parentHashes)
		if err != nil {
			return err
		}
		if wasAdded {
			added++
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err := store.SaveIndex(&idx); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	fmt.Printf("Added %d file(s)\n", added)
	return nil
}

func addFile(store *storage.Store, root, relPath, fullPath string, info os.FileInfo, idx *core.Index, parentTreeHashes map[string]string) (bool, error) {
	mode, err := core.NormalizeModeForPath(info.Mode(), relPath)
	if err != nil {
		// Skip unsupported file types (sockets, pipes, devices) with a notice.
		fmt.Printf("Skipped (unsupported type): %s\n", relPath)
		return false, nil
	}

	var hash string
	if mode == core.ModeSymlink {
		// Issue 17: store symlink target string as blob content, not the
		// dereferenced target file content.
		target, err := os.Readlink(fullPath)
		if err != nil {
			return false, fmt.Errorf("failed to read symlink %s: %w", relPath, err)
		}
		// P0-#17: reject symlinks that escape the repository root, which
		// could be used to read/write arbitrary files on restore.
		if err := core.ValidateSymlinkTarget(root, relPath, target); err != nil {
			return false, fmt.Errorf("unsafe symlink %s: %w", relPath, err)
		}
		hash, err = store.PutBlob([]byte(target))
		if err != nil {
			return false, fmt.Errorf("failed to store symlink %s: %w", relPath, err)
		}
	} else {
		hash, err = putBlobForAdd(store, fullPath, sharedConfig.Core.AutoCRLF)
		if err != nil {
			return false, fmt.Errorf("failed to store %s: %w", relPath, err)
		}
	}

	// Skip if the file content hasn't changed since last staging.
	if existing, err := idx.Entry(relPath); err == nil && existing.Hash == hash {
		return false, nil
	}

	// Skip if the file content matches the last commit (index was cleared by save).
	if parentHash, ok := parentTreeHashes[relPath]; ok && parentHash == hash {
		return false, nil
	}

	entry := core.IndexEntry{
		Path:       relPath,
		Hash:       hash,
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       mode,
	}

	if err := idx.Add(entry); err != nil {
		return false, fmt.Errorf("failed to add %s: %w", relPath, err)
	}

	fmt.Printf("Added: %s\n", relPath)
	return true, nil
}

func putBlobForAdd(store *storage.Store, path, autoCRLF string) (string, error) {
	if autoCRLF == "" {
		return store.PutBlobFromFile(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// B9: use pooled byte slice for the 8KB head read.
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
		return store.PutBlobFromReader(r)
	}

	// B9: use pooled buffer for the LF-normalized output.
	buf := core.GetBuffer()
	defer core.PutBuffer(buf)
	buf.Reset()

	w := core.NewLFWriter(buf)
	if _, err := io.Copy(w, r); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return store.PutBlobFromReader(buf)
}
