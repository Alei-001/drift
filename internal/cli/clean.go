package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewCleanCmd creates the clean subcommand.
func NewCleanCmd(application *apppkg.App) *cobra.Command {
	var (
		dryRun bool
		dirs   bool
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove untracked files from the working tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check the persistent --dry-run flag as well as the local one.
			if globalDryRun, _ := cmd.Flags().GetBool("dry-run"); globalDryRun {
				dryRun = true
			}

			// First, get list of files that would be cleaned (dry run)
			cleaned, err := application.Clean(apppkg.CleanOptions{
				Dirs:   dirs,
				DryRun: true,
			})
			if err != nil {
				return err
			}

			if len(cleaned) == 0 {
				fmt.Println(colorGray("Nothing to clean"))
				return nil
			}

			// Show what would be cleaned
			if dryRun {
				fmt.Println(colorYellow(fmt.Sprintf("Would clean %d file(s):", len(cleaned))))
				for _, f := range cleaned {
					fmt.Printf("  %s\n", f)
				}
				return nil
			}

			// Confirm before cleaning
			if !confirmAction(force, fmt.Sprintf("Clean %d file(s)?", len(cleaned)), cleaned) {
				fmt.Println(colorRed("Aborted"))
				return nil
			}

			// Actually clean the files
			cleaned, err = application.Clean(apppkg.CleanOptions{
				Dirs:   dirs,
				DryRun: false,
			})
			if err != nil {
				return err
			}

			fmt.Println(colorGreen(fmt.Sprintf("Cleaned %d file(s)", len(cleaned))))
			return nil
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Show what would be cleaned")
	cmd.Flags().BoolVarP(&dirs, "dirs", "d", false, "Also remove empty directories")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}
