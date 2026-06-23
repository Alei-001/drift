package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		create, _ := cmd.Flags().GetBool("create")

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
			if !create {
				return fmt.Errorf("branch not found: %s", branch)
			}
			// Create the branch from the current branch's commit.
			parentHash, _ := sharedStore.GetRef(currentBranch)
			if err := sharedStore.SaveRef(branch, parentHash); err != nil {
				return fmt.Errorf("failed to create branch: %w", err)
			}
			fmt.Printf("Created branch: %s\n", branch)
			commitHash = parentHash
		} else if create {
			return fmt.Errorf("branch %q already exists", branch)
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
	switchCmd.Flags().BoolP("create", "c", false, "Create the branch if it does not exist, then switch")
	rootCmd.AddCommand(switchCmd)
}

// findCommitByHash loads a commit by its hash directly from the commit store.
// Commit files are named <hash>.dcm, so this is O(1) — no need to scan all
// commits. Mirrors go-git's CommitObject lookup.
func findCommitByHash(store *storage.Store, hash string) (*core.Commit, error) {
	c, err := store.GetCommit(hash)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, fmt.Errorf("commit not found: %s", hash)
		}
		return nil, err
	}
	return c, nil
}

// resolveCommit resolves a version specifier to a commit. This is the single
// shared resolver used by diff, export, restore, and log. P1-#4: previously
// diff.go and export.go had separate, inconsistent implementations.
//
// Supported formats:
//   - branch name (e.g. "main") → latest commit on that branch
//   - version ID (e.g. "v1") → commit with that ID in current branch
//   - branch/version (e.g. "main/v1") → commit with that ID in that branch
//
// Ambiguous version IDs (same ID in multiple branches) return an error
// suggesting the branch/version form.
func resolveCommit(store *storage.Store, version string) (*core.Commit, error) {
	// branch/version format (e.g. "main/v1").
	if strings.Contains(version, "/") {
		parts := strings.SplitN(version, "/", 2)
		branchName := parts[0]
		versionID := parts[1]

		branchHash, err := store.GetRef(branchName)
		if err != nil || branchHash == "" {
			return nil, fmt.Errorf("branch not found: %s", branchName)
		}

		// Walk the branch's commit chain (O(depth), not O(all commits)).
		commits, err := store.ListBranchCommits(branchName)
		if err != nil {
			return nil, err
		}
		for _, c := range commits {
			if c.ID == versionID && c.Branch == branchName {
				return c, nil
			}
		}
		return nil, fmt.Errorf("version %s not found in branch %s", versionID, branchName)
	}

	// Try branch name first (latest commit on that branch).
	if hash, err := store.GetRef(version); err == nil && hash != "" {
		if commit, err := findCommitByHash(store, hash); err == nil && commit != nil {
			return commit, nil
		}
	} else if err != nil && !errors.Is(err, storage.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to resolve %q as branch: %w", version, err)
	}

	// Try version ID in current branch, then any branch.
	currentBranch, _ := store.GetRef("HEAD")
	if currentBranch == "" {
		currentBranch = "main"
	}

	// P1-#9: walk only the current branch's chain instead of all commits.
	if commits, err := store.ListBranchCommits(currentBranch); err == nil {
		for _, c := range commits {
			if c.ID == version {
				return c, nil
			}
		}
	}

	// Not in current branch — search other branches for ambiguity.
	refs, err := store.ListRefs()
	if err != nil {
		return nil, err
	}
	var found *core.Commit
	var foundBranch string
	for branchName := range refs {
		if branchName == "HEAD" || branchName == currentBranch {
			continue
		}
		commits, err := store.ListBranchCommits(branchName)
		if err != nil {
			continue
		}
		for _, c := range commits {
			if c.ID == version {
				if found != nil {
					return nil, fmt.Errorf("ambiguous version %s: exists in both %s and %s branches. Use branch/version format (e.g., %s/%s)",
						version, foundBranch, c.Branch, foundBranch, version)
				}
				found = c
				foundBranch = c.Branch
			}
		}
	}

	if found != nil {
		return found, nil
	}

	return nil, fmt.Errorf("version not found: %s", version)
}
