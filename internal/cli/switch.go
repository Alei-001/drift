package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch <branch>",
	Short: "Switch to another branch",
	Long: `Switch to another branch and restore the working tree.
The staging area must be empty (or use --force to discard).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := args[0]
		force, _ := cmd.Flags().GetBool("force")

		// Issue 27: no-op if already on this branch.
		currentBranch, _ := sharedStore.GetRef("HEAD")
		if currentBranch == "" {
			currentBranch = "main"
		}
		if branch == currentBranch {
			fmt.Printf("Already on branch: %s\n", branch)
			return nil
		}

		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return err
		}

		if !force {
			if hasPendingChanges, err := hasPendingStagedChanges(&idx); err == nil && hasPendingChanges {
				return fmt.Errorf("staging area has pending changes (use --force to discard)")
			}
			if dirty, err := hasWorktreeModifications(); err == nil && dirty {
				return fmt.Errorf("working tree has unstaged modifications (use --force to discard)")
			}
		}

		commitHash, err := sharedStore.GetRef(branch)
		if err != nil {
			return fmt.Errorf("branch not found: %s", branch)
		}

		reader := core.NewTreeReader(sharedStore)

		currentBlobs := make(map[string]bool)
		if currentCommit, _ := currentBranchCommit(sharedStore); currentCommit != nil {
			if t, err := sharedStore.GetTree(currentCommit.TreeHash); err == nil {
				if blobs, err := reader.ListBlobs(t, ""); err == nil {
					for _, b := range blobs {
						currentBlobs[b.Path] = true
					}
				}
			}
		}

		if err := sharedStore.SaveRef("HEAD", branch); err != nil {
			return fmt.Errorf("failed to update HEAD: %w", err)
		}

		// Issue 2: handle empty branch — clear old branch's files from worktree.
		if commitHash == "" {
			var deletedPaths []string
			for path := range currentBlobs {
				if err := core.ValidateTreePath(path); err != nil {
					continue
				}
				fullPath := filepath.Join(sharedDir, filepath.FromSlash(path))
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				deletedPaths = append(deletedPaths, path)
			}
			cleanEmptyDirsAffected(sharedDir, deletedPaths)
			newIdx := &core.Index{}
			if err := sharedStore.SaveIndex(newIdx); err != nil {
				return fmt.Errorf("failed to update index: %w", err)
			}
			fmt.Printf("Switched to branch: %s\n", branch)
			return nil
		}

		commit, err := findCommitByHash(sharedStore, commitHash)
		if err != nil {
			return fmt.Errorf("failed to load commit: %w", err)
		}

		targetTree, err := sharedStore.GetTree(commit.TreeHash)
		if err != nil {
			return fmt.Errorf("failed to load tree: %w", err)
		}

		targetBlobs, err := reader.ListBlobs(targetTree, "")
		if err != nil {
			return err
		}

		targetPaths := make(map[string]bool)
		for _, b := range targetBlobs {
			targetPaths[b.Path] = true
		}

		newIdx := &core.Index{}
		for _, b := range targetBlobs {
			entry, err := writeBlobToWorktree(sharedStore, sharedDir, b)
			if err != nil {
				return err
			}
			newIdx.Add(entry)
		}

		var deletedPaths []string
		for path := range currentBlobs {
			if !targetPaths[path] {
				if err := core.ValidateTreePath(path); err != nil {
					continue
				}
				fullPath := filepath.Join(sharedDir, filepath.FromSlash(path))
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				deletedPaths = append(deletedPaths, path)
			}
		}

		cleanEmptyDirsAffected(sharedDir, deletedPaths)

		if err := sharedStore.SaveIndex(newIdx); err != nil {
			return fmt.Errorf("failed to update index: %w", err)
		}

		fmt.Printf("Switched to branch: %s\n", branch)
		return nil
	},
}

func init() {
	switchCmd.Flags().Bool("force", false, "Discard staged changes and unstaged modifications, then force switch")
	rootCmd.AddCommand(switchCmd)
}

func findCommitByHash(store *storage.Store, hash string) (*core.Commit, error) {
	commits, err := store.ListCommits()
	if err != nil {
		return nil, err
	}

	for _, c := range commits {
		if c.Hash == hash {
			return c, nil
		}
	}

	return nil, fmt.Errorf("commit not found: %s", hash)
}
