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
			fmt.Println(colorGreen(fmt.Sprintf("Undid %d operation(s)", undone)))
			fmt.Printf("  %s: %s\n", colorYellow(string(result.Entry.Op)), result.Entry.Desc)

			if result.Warning != "" {
				fmt.Fprintf(os.Stderr, "%s: %s\n", colorYellow("Warning"), result.Warning)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&number, "number", "n", 1, "Number of operations to undo")

	return cmd
}
