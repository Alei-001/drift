package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewBackCmd creates the back subcommand.
func NewBackCmd(application *apppkg.App) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "back [<version>] [<path>...]",
		Short: "Restore working tree to a specific version",
		RunE: func(cmd *cobra.Command, args []string) error {
			version := ""
			var filters []string
			if len(args) > 0 {
				version = args[0]
				filters = args[1:]
			} else {
				version = application.CurrentBranch()
			}

			if !force {
				status, err := application.Status()
				if err != nil {
					return err
				}
				if !status.IsClean() {
					return fmt.Errorf("You have unsaved changes. Save them first with 'drift save' or use --force to discard.")
				}
			}

			result, err := application.Restore(version, filters, force)
			if err != nil {
				return err
			}

			total := result.Added + result.Modified + result.Deleted
			if total == 0 {
				fmt.Println(colorGray("Nothing to restore"))
				return nil
			}
			if result.Added > 0 {
				fmt.Println(colorGreen(fmt.Sprintf("  Added %d file(s)", result.Added)))
			}
			if result.Modified > 0 {
				fmt.Println(colorYellow(fmt.Sprintf("  Modified %d file(s)", result.Modified)))
			}
			if result.Deleted > 0 {
				fmt.Println(colorRed(fmt.Sprintf("  Deleted %d file(s)", result.Deleted)))
			}
			fmt.Println(colorGreen(fmt.Sprintf("Restored %d file(s) from %s", total, result.Version)))

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Discard pending changes")

	return cmd
}
