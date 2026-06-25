package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewUnstageCmd creates the unstage subcommand.
func NewUnstageCmd(application *apppkg.App) *cobra.Command {
	return &cobra.Command{
		Use:   "unstage <path> [<path>...]",
		Short: "Remove file contents from the staging area",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			unstaged, notFound, err := application.Unstage(args)
			if err != nil {
				return err
			}

			for _, p := range unstaged {
				fmt.Printf("Unstaged: %s\n", p)
			}
			for _, p := range notFound {
				fmt.Printf("%s is not staged\n", p)
			}
			return nil
		},
	}
}
