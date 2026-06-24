package cli

import (
	"fmt"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <path> [<path>...]",
	Short: "Add file contents to the staging area",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Expand glob patterns and collect unique paths.
		paths, err := worktree.ExpandAddPaths(sharedDir, args)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return fmt.Errorf("no matching files found")
		}

		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}

		// Special case: "." means add all.
		var added int
		if len(paths) == 1 && paths[0] == "." {
			added, err = sharedRepo.WT.StageAll(&idx)
		} else {
			added, err = sharedRepo.WT.StagePaths(&idx, paths)
		}
		if err != nil {
			return err
		}

		if err := sharedStore.SaveIndex(&idx); err != nil {
			return fmt.Errorf("failed to save index: %w", err)
		}

		// For "add .", always print the count (even if 0) to match the
		// original behavior. For specific paths, only print if files were added.
		if len(paths) == 1 && paths[0] == "." {
			fmt.Printf("Added %d file(s)\n", added)
		} else if added > 0 {
			fmt.Printf("Added %d file(s)\n", added)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
