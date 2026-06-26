package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewRmCmd creates the rm subcommand.
func NewRmCmd(application *apppkg.App) *cobra.Command {
	var (
		cached    bool
		recursive bool
		force     bool
	)

	cmd := &cobra.Command{
		Use:   "rm <path> [<path>...]",
		Short: "Remove files from the working tree and staging area",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Honor global --dry-run: only preview, don't delete.
			globalDryRun, _ := cmd.Flags().GetBool("dry-run")

			// First, get list of files that would be removed (dry run)
			toRemove, err := application.Remove(args, apppkg.RemoveOptions{
				Cached:    cached,
				Recursive: recursive,
				DryRun:    true,
			})
			if err != nil {
				return err
			}

			if len(toRemove) == 0 {
				fmt.Println("No files to remove")
				return nil
			}

			// Global --dry-run: print what would be removed and exit.
			if globalDryRun {
				fmt.Printf("Would remove %d file(s):\n", len(toRemove))
				for _, p := range toRemove {
					fmt.Printf("  %s\n", p)
				}
				return nil
			}

			// Confirm before deleting files from the working tree
			// --cached only modifies the index, so no confirmation is needed
			if !cached {
				if !confirmAction(force, fmt.Sprintf("Delete %d file(s)?", len(toRemove)), toRemove) {
					fmt.Println("Aborted")
					return nil
				}
			}

			// Actually remove the files
			removed, err := application.Remove(args, apppkg.RemoveOptions{
				Cached:    cached,
				Recursive: recursive,
				DryRun:    false,
			})
			if err != nil {
				return err
			}

			for _, p := range removed {
				fmt.Printf("Removed: %s\n", p)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&cached, "cached", false, "Only remove from staging area")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Remove directories recursively")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}
