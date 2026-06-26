package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewRestoreCmd creates the restore subcommand.
func NewRestoreCmd(application *apppkg.App) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "restore <version> [<path>...]",
		Short: "Restore working tree to a specific version",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version := args[0]
			filters := args[1:]

			result, err := application.Restore(version, filters, force)
			if err != nil {
				return err
			}

			fmt.Printf("Restored %s\n", result.Version)
			if result.Added > 0 {
				fmt.Printf("  Added %d file(s)\n", result.Added)
			}
			if result.Modified > 0 {
				fmt.Printf("  Modified %d file(s)\n", result.Modified)
			}
			if result.Deleted > 0 {
				fmt.Printf("  Deleted %d file(s)\n", result.Deleted)
			}
			total := result.Added + result.Modified + result.Deleted
			if total > 0 {
				fmt.Printf("Restored %d file(s)\n", total)
			} else {
				fmt.Println("Nothing to restore")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Discard pending changes")

	return cmd
}
