package cli

import (
	"fmt"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var unstageCmd = &cobra.Command{
	Use:   "unstage [path]",
	Short: "Unstage staged changes",
	Long: `Unstage staged changes.

Without arguments, clears the entire staging area.
With a path argument, removes only that file from the staging area.

The working tree files are never modified — only the index is updated.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Single-file unstage: remove just that entry from the index.
		if len(args) == 1 {
			path := args[0]
			if err := core.ValidateTreePath(path); err != nil {
				return fmt.Errorf("invalid path: %w", err)
			}

			var idx core.Index
			if err := sharedStore.LoadIndex(&idx); err != nil {
				return fmt.Errorf("failed to load index: %w", err)
			}

			if !idx.Has(path) {
				fmt.Printf("%s is not staged\n", path)
				return nil
			}

			idx.Remove(path)
			if err := sharedStore.SaveIndex(&idx); err != nil {
				return fmt.Errorf("failed to unstage: %w", err)
			}

			fmt.Printf("Unstaged: %s\n", path)
			return nil
		}

		// Clear-all unstage (original behavior).
		idx := &core.Index{}
		if err := sharedStore.SaveIndex(idx); err != nil {
			return fmt.Errorf("failed to unstage: %w", err)
		}

		fmt.Println("Staging area cleared")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(unstageCmd)
}
