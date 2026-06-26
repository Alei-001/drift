package cli

import (
	"fmt"
	"os"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewUndoCmd creates the undo subcommand.
func NewUndoCmd(application *apppkg.App) *cobra.Command {
	var number int

	cmd := &cobra.Command{
		Use:   "undo",
		Short: "Undo recent operations",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := application.Undo(number)
			if err != nil {
				return err
			}

			undone := number - result.RemainingCount
			fmt.Printf("Undid %d operation(s)\n", undone)
			fmt.Printf("  %s: %s\n", result.Entry.Op, result.Entry.Desc)

			if result.Warning != "" {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", result.Warning)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&number, "number", "n", 1, "Number of operations to undo")

	return cmd
}
