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

	if err := addFile(store, root, path, fullPath, info, &idx); err != nil {
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

	var added int
	err := core.WalkWorkingDirWithIgnore(fullDir, root, func(path string, info os.FileInfo) error {
		relPath := filepath.ToSlash(filepath.Join(dirPath, path))
		fullPath := filepath.Join(root, filepath.FromSlash(relPath))
		if err := addFile(store, root, relPath, fullPath, info, &idx); err != nil {
			return err
		}
		added++
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

	var added int
	err := core.WalkWorkingDir(root, func(path string, info os.FileInfo) error {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := addFile(store, root, path, fullPath, info, &idx); err != nil {
			return err
		}
		added++
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

func addFile(store *storage.Store, root, relPath, fullPath string, info os.FileInfo, idx *core.Index) error {
	hash, err := store.PutBlobFromFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to store %s: %w", relPath, err)
	}

	entry := core.IndexEntry{
		Path:       relPath,
		Hash:       hash,
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       core.NormalizeMode(info.Mode()),
	}

	idx.Add(entry)

	fmt.Printf("Added: %s\n", relPath)
	return nil
}
