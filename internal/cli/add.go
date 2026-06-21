package cli

import (
	"fmt"
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
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		store := storage.NewStore(dir)
		if !store.IsInitialized() {
			return fmt.Errorf("not a drift project (run 'drift init')")
		}

		target := args[0]
		if target == "." {
			return addAll(store, dir)
		}
		return addPath(store, dir, target)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
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
	return addFile(store, root, path, fullPath, info)
}

func addDirectory(store *storage.Store, root, dirPath string) error {
	fullDir := filepath.Join(root, filepath.FromSlash(dirPath))
	var added int

	err := core.WalkWorkingDir(fullDir, func(path string, info os.FileInfo) error {
		relPath := filepath.ToSlash(filepath.Join(dirPath, path))
		fullPath := filepath.Join(root, filepath.FromSlash(relPath))
		if err := addFile(store, root, relPath, fullPath, info); err != nil {
			return err
		}
		added++
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("Added %d file(s)\n", added)
	return nil
}

func addAll(store *storage.Store, root string) error {
	var added int

	err := core.WalkWorkingDir(root, func(path string, info os.FileInfo) error {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := addFile(store, root, path, fullPath, info); err != nil {
			return err
		}
		added++
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("Added %d file(s)\n", added)
	return nil
}

func addFile(store *storage.Store, root, relPath, fullPath string, info os.FileInfo) error {
	hash, err := store.PutBlobFromFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to store %s: %w", relPath, err)
	}

	var idx core.Index
	_ = store.LoadIndex(&idx)

	entry := core.IndexEntry{
		Path:       relPath,
		Hash:       hash,
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       uint32(info.Mode()),
	}

	idx.Add(entry)

	if err := store.SaveIndex(&idx); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	fmt.Printf("Added: %s\n", relPath)
	return nil
}
