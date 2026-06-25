package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewCloneCmd creates the clone subcommand.
func NewCloneCmd(application *apppkg.App) *cobra.Command {
	return &cobra.Command{
		Use:   "clone <remote> [directory]",
		Short: "Clone a remote repository",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			remoteName := args[0]
			destDir := "."
			if len(args) > 1 {
				destDir = args[1]
			}

			if err := application.Clone(remoteName, destDir); err != nil {
				return err
			}

			fmt.Printf("Cloned %s to %s\n", remoteName, destDir)
			fmt.Println("\nNext steps:")
			fmt.Println("  cd " + destDir)
			fmt.Println("  drift log --all")
			return nil
		},
	}
}
