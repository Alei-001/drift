package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewAddCmd creates the add subcommand.
func NewAddCmd(application *apppkg.App) *cobra.Command {
	return &cobra.Command{
		Use:   "add <path> [<path>...]",
		Short: "Add file contents to the staging area",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			added, err := application.Add(args)
			if err != nil {
				return err
			}

			// For "add .", always print the count (even if 0) to match the
			// original behavior. For specific paths, only print if files were added.
			if len(args) == 1 && args[0] == "." {
				fmt.Printf("Added %d file(s)\n", added)
			} else if added > 0 {
				fmt.Printf("Added %d file(s)\n", added)
			}
			return nil
		},
	}
}
