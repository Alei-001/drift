package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add file contents to the staging area",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		if target == "." {
			return addAll(sharedStore, sharedDir)
		}
		return addPath(sharedStore, sharedDir, target)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
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

func addPath(store *storage.Store, root, path string) error {
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	info, err := os.Lstat(fullPath)
	if err != nil {
		return fmt.Errorf("path not found: %s", path)
	}

	if info.IsDir() {
		return addDirectory(store, root, path)
	}

	var idx core.Index
	if err := store.LoadIndex(&idx); err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	parentHashes := loadParentTreeHashes(store)
	if _, err := addFile(store, root, path, fullPath, info, &idx, parentHashes); err != nil {
		return err
	}

	if err := store.SaveIndex(&idx); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	return nil
}

func addDirectory(store *storage.Store, root, dirPath string) error {
	fullDir := filepath.Join(root, filepath.FromSlash(dirPath))

	var idx core.Index
	if err := store.LoadIndex(&idx); err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	parentHashes := loadParentTreeHashes(store)
	var added int
	err := core.WalkWorkingDirWithIgnore(fullDir, root, func(path string, info os.FileInfo) error {
		relPath := filepath.ToSlash(filepath.Join(dirPath, path))
		fullPath := filepath.Join(root, filepath.FromSlash(relPath))
		wasAdded, err := addFile(store, root, relPath, fullPath, info, &idx, parentHashes)
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
	mode, err := core.NormalizeMode(info.Mode())
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
