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
			result, err := application.Add(args)
			if err != nil {
				return err
			}

			for _, p := range result.Skipped {
				fmt.Printf("Skipped (unsupported type): %s\n", colorYellow(p))
			}
			for _, p := range result.Added {
				fmt.Printf("Added: %s\n", colorGreen(p))
			}

			fmt.Println(colorGreen(fmt.Sprintf("Added %d file(s)", len(result.Added))))
			return nil
		},
	}
}
