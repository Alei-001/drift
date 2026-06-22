package cli

import (
	"fmt"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the working tree status",
	RunE: func(cmd *cobra.Command, args []string) error {
		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}

		var commitTree *core.Tree
		if latest, _ := currentBranchCommit(sharedStore); latest != nil {
			if latest.TreeHash != "" {
				t, err := sharedStore.GetTree(latest.TreeHash)
				if err == nil {
					commitTree = t
				}
			}
		}

		status, err := core.ComputeStatus(commitTree, &idx, sharedDir, sharedStore)
		if err != nil {
			return fmt.Errorf("failed to compute status: %w", err)
		}

		printStatus(status)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func printStatus(s core.Status) {
	if s.IsClean() {
		fmt.Println("Nothing to commit, working tree clean")
		return
	}

	hasStaged := false
	hasUnstaged := false

	for path, fs := range s {
		if fs.Staging != core.Unmodified && fs.Staging != core.Untracked {
			hasStaged = true
			_ = path
		}
		if fs.Worktree != core.Unmodified && fs.Worktree != core.Untracked {
			hasUnstaged = true
		}
	}

	if hasStaged {
		fmt.Println("Staged changes:")
		for path, fs := range s {
			if fs.Staging != core.Unmodified && fs.Staging != core.Untracked {
				fmt.Printf("  %s %s\n", fs.Staging, path)
			}
		}
		fmt.Println()
	}

	if hasUnstaged {
		fmt.Println("Unstaged changes:")
		for path, fs := range s {
			if fs.Worktree != core.Unmodified && fs.Worktree != core.Untracked {
				fmt.Printf("  %s %s\n", fs.Worktree, path)
			}
		}
		fmt.Println()
	}

	hasUntracked := false
	for _, fs := range s {
		if fs.Worktree == core.Untracked {
			hasUntracked = true
			break
		}
	}
	if hasUntracked {
		fmt.Println("Untracked files:")
		for path, fs := range s {
			if fs.Worktree == core.Untracked {
				fmt.Printf("  %s\n", path)
			}
		}
	}
}
