package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewUnstageCmd creates the unstage subcommand.
func NewUnstageCmd(application *apppkg.App) *cobra.Command {
	return &cobra.Command{
		Use:   "unstage [<path>...]",
		Short: "Remove files from the staging area (no args clears all)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if err := application.ClearStaging(); err != nil {
					return err
				}
				fmt.Println("Staging area cleared")
				return nil
			}

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
