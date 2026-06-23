package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <version> [<path>...]",
	Short: "Restore working tree to a specific version",
	Long: `Restore the working tree to the state of a given version.
Version can be a version ID (e.g., v1) or branch name (e.g., main).
Files that differ from the target version will be overwritten.
Branch reference is NOT changed - only working tree is updated.
Untracked files are preserved.
Use --force to discard staged changes and unstaged modifications.

If one or more paths are given, only files matching those paths are
restored; all other files are left untouched. This is useful for
reverting a single file or directory without affecting the rest of
the working tree.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		version := args[0]
		force, _ := cmd.Flags().GetBool("force")

		filters, err := normalizePathFilters(args[1:])
		if err != nil {
			return err
		}
		hasFilter := len(filters) > 0

		var oldIdx core.Index
		if err := sharedStore.LoadIndex(&oldIdx); err != nil {
			return err
		}

		if !force {
			if hasPendingChanges, err := hasPendingStagedChanges(&oldIdx, filters); err == nil && hasPendingChanges {
				return fmt.Errorf("staging area has pending changes (use --force to discard)")
			}
			if dirty, err := hasWorktreeModifications(filters); err == nil && dirty {
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

		// Apply path filter if specified.
		if hasFilter {
			targetBlobs = filterBlobs(targetBlobs, filters)
			if len(targetBlobs) == 0 {
				return fmt.Errorf("no matching files found in %s for given paths", version)
			}
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

		// Build new index. For partial restore, preserve entries outside
		// the filter; for full restore, start from scratch.
		newIdx := &core.Index{}
		if hasFilter {
			for _, e := range oldIdx.Entries {
				if !pathMatchesAny(e.Path, filters) {
					newIdx.Add(e)
				}
			}
		}

		var deletedPaths []string

		// Write target blobs to worktree.
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

		// Delete files that exist in the current branch but not in the
		// target version. For partial restore, only delete files matching
		// the filter.
		var deleted int
		for path := range prevBlobs {
			if targetPaths[path] {
				continue
			}
			if hasFilter && !pathMatchesAny(path, filters) {
				continue
			}
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

		// Also delete staged-new files (in index but not in current branch
		// or target) that match the filter.
		for _, entry := range oldIdx.Entries {
			if targetPaths[entry.Path] {
				continue
			}
			if _, inPrev := prevBlobs[entry.Path]; inPrev {
				continue
			}
			if hasFilter && !pathMatchesAny(entry.Path, filters) {
				continue
			}
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

func hasPendingStagedChanges(idx *core.Index, filters []string) (bool, error) {
	if len(idx.Entries) == 0 {
		return false, nil
	}

	commit, err := currentBranchCommit(sharedStore)
	if err != nil {
		return false, err
	}

	if commit == nil {
		for _, entry := range idx.Entries {
			if pathMatchesAny(entry.Path, filters) {
				return true, nil
			}
		}
		return false, nil
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
		if !pathMatchesAny(entry.Path, filters) {
			continue
		}
		commitHash, exists := commitFiles[entry.Path]
		if !exists || commitHash != entry.Hash {
			return true, nil
		}
	}

	for path := range commitFiles {
		if !pathMatchesAny(path, filters) {
			continue
		}
		if _, err := idx.Entry(path); err != nil {
			return true, nil
		}
	}

	return false, nil
}
