package cli

import (
	"fmt"
	"os"
	"path/filepath"

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
Use --force to discard staged changes and unstaged modifications.`,
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
			if dirty, err := hasWorktreeModifications(); err == nil && dirty {
				return fmt.Errorf("working tree has unstaged modifications (use --force to discard)")
			}
		}

		commit, err := resolveCommit(sharedStore, version)
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
		var deletedPaths []string

		// P1-#2: write target blobs to worktree. added/modified counts are
		// computed below by comparing against prevBlobs, not during the write
		// loop (the old in-loop logic was dead code — os.Lstat after write
		// always succeeds, so "added" was always 0).
		for _, b := range targetBlobs {
			entry, err := writeBlobToWorktree(sharedStore, sharedDir, b)
			if err != nil {
				return err
			}
			newIdx.Add(entry)
		}

		// Compute added/modified by comparing target against prevBlobs.
		var added, modified int
		for _, b := range targetBlobs {
			if prevBlobs[b.Path] {
				modified++
			} else {
				added++
			}
		}

		var deleted int
		for path := range prevBlobs {
			if !targetPaths[path] {
				if err := core.ValidateTreePath(path); err != nil {
					continue
				}
				fullPath := filepath.Join(sharedDir, filepath.FromSlash(path))
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				deleted++
				deletedPaths = append(deletedPaths, path)
			}
		}

		for _, entry := range oldIdx.Entries {
			if !targetPaths[entry.Path] {
				if _, inPrev := prevBlobs[entry.Path]; !inPrev {
					if err := core.ValidateTreePath(entry.Path); err != nil {
						continue
					}
					fullPath := filepath.Join(sharedDir, filepath.FromSlash(entry.Path))
					if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
						return err
					}
					deleted++
					deletedPaths = append(deletedPaths, entry.Path)
				}
			}
		}

		cleanEmptyDirsAffected(sharedDir, deletedPaths)

		if err := sharedStore.SaveIndex(newIdx); err != nil {
			return fmt.Errorf("failed to update index: %w", err)
		}

		fmt.Printf("Restored to %s: %d added, %d modified, %d deleted\n", version, added, modified, deleted)
		return nil
	},
}

func init() {
	restoreCmd.Flags().Bool("force", false, "Discard staged changes and unstaged modifications, then force restore")
	rootCmd.AddCommand(restoreCmd)
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
