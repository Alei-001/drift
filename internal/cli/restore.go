package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <version>",
	Short: "Restore working tree to a specific version",
	Long: `Restore the working tree to the state of a given version.
Version can be a version ID (e.g., v1) or branch name (e.g., main).
Files that differ from the target version will be overwritten.
Branch reference is NOT changed - only working tree is updated.
Untracked files are preserved.
Use --force to discard staged changes.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		version := args[0]
		force, _ := cmd.Flags().GetBool("force")

		var oldIdx core.Index
		if err := sharedStore.LoadIndex(&oldIdx); err != nil {
			return err
		}

		if !force {
			if hasPendingChanges, err := hasPendingStagedChanges(&oldIdx); err == nil && hasPendingChanges {
				return fmt.Errorf("staging area has pending changes (use --force to discard)")
			}
		}

		commit, err := findCommitByPrefix(sharedStore, version)
		if err != nil {
			return err
		}

		targetTree, err := sharedStore.GetTree(commit.TreeHash)
		if err != nil {
			return fmt.Errorf("failed to load target tree: %w", err)
		}

		reader := core.NewTreeReader(sharedStore)
		targetBlobs, err := reader.ListBlobs(targetTree, "")
		if err != nil {
			return err
		}

		targetPaths := make(map[string]bool)
		for _, b := range targetBlobs {
			targetPaths[b.Path] = true
		}

		prevBlobs := make(map[string]bool)
		currentBranch, _ := sharedStore.GetRef("HEAD")
		if currentBranch == "" {
			currentBranch = "main"
		}
		if currentHash, err := sharedStore.GetRef(currentBranch); err == nil {
			if currentHash != commit.Hash {
				if currentCommit, err := findCommitByHash(sharedStore, currentHash); err == nil {
					if t, err := sharedStore.GetTree(currentCommit.TreeHash); err == nil {
						prevBlobsList, _ := reader.ListBlobs(t, "")
						for _, b := range prevBlobsList {
							prevBlobs[b.Path] = true
						}
					}
				}
			}
		}

		newIdx := &core.Index{}
		var added, modified, deleted int

		for _, b := range targetBlobs {
			fullPath := filepath.Join(sharedDir, filepath.FromSlash(b.Path))
			data, err := sharedStore.GetBlob(b.Hash)
			if err != nil {
				return err
			}

			existing, err := os.ReadFile(fullPath)
			if err == nil && bytes.Equal(existing, data) {
				info, _ := os.Stat(fullPath)
				newIdx.Add(core.IndexEntry{
					Path:       b.Path,
					Hash:       b.Hash,
					ModifiedAt: info.ModTime(),
					Size:       info.Size(),
					Mode:       b.Mode,
				})
				continue
			}

			if err != nil {
				added++
			} else {
				modified++
			}

			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(fullPath, data, os.FileMode(core.ToOSFileMode(b.Mode))); err != nil {
				return err
			}

			info, _ := os.Stat(fullPath)
			newIdx.Add(core.IndexEntry{
				Path:       b.Path,
				Hash:       b.Hash,
				ModifiedAt: info.ModTime(),
				Size:       info.Size(),
				Mode:       b.Mode,
			})
		}

		for path := range prevBlobs {
			if !targetPaths[path] {
				fullPath := filepath.Join(sharedDir, filepath.FromSlash(path))
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				deleted++
			}
		}

		for _, entry := range oldIdx.Entries {
			if !targetPaths[entry.Path] {
				if _, inPrev := prevBlobs[entry.Path]; !inPrev {
					fullPath := filepath.Join(sharedDir, filepath.FromSlash(entry.Path))
					if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
						return err
					}
					deleted++
				}
			}
		}

		cleanEmptyDirs(sharedDir, newIdx)

		if err := sharedStore.SaveIndex(newIdx); err != nil {
			return fmt.Errorf("failed to update index: %w", err)
		}

		fmt.Printf("Restored to %s: %d added, %d modified, %d deleted\n", version, added, modified, deleted)
		return nil
	},
}

func init() {
	restoreCmd.Flags().Bool("force", false, "Discard staged changes and force restore")
	rootCmd.AddCommand(restoreCmd)
}

func cleanEmptyDirs(root string, idx *core.Index) {
	tracked := make(map[string]bool)
	for _, e := range idx.Entries {
		dir := filepath.Dir(e.Path)
		for dir != "." {
			tracked[dir] = true
			dir = filepath.Dir(dir)
		}
	}

	var dirs []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." || rel == ".drift" || len(rel) > 0 && rel[0] == '.' {
			return nil
		}
		dirs = append(dirs, rel)
		return nil
	})

	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	for _, d := range dirs {
		if !tracked[filepath.ToSlash(d)] {
			os.Remove(filepath.Join(root, d))
		}
	}
}

func hasPendingStagedChanges(idx *core.Index) (bool, error) {
	if len(idx.Entries) == 0 {
		return false, nil
	}

	commit, err := currentBranchCommit(sharedStore)
	if err != nil {
		return false, err
	}

	if commit == nil {
		return len(idx.Entries) > 0, nil
	}

	tree, err := sharedStore.GetTree(commit.TreeHash)
	if err != nil {
		return false, err
	}

	reader := core.NewTreeReader(sharedStore)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return false, err
	}

	commitFiles := make(map[string]string)
	for _, b := range blobs {
		commitFiles[b.Path] = b.Hash
	}

	for _, entry := range idx.Entries {
		commitHash, exists := commitFiles[entry.Path]
		if !exists || commitHash != entry.Hash {
			return true, nil
		}
	}

	for path := range commitFiles {
		if _, err := idx.Entry(path); err != nil {
			return true, nil
		}
	}

	return false, nil
}
