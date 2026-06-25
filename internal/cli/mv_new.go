package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewMvCmd creates the mv subcommand.
func NewMvCmd(application *apppkg.App) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "mv <source> [<source>...] <dest>",
		Short: "Move or rename files",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sources := args[:len(args)-1]
			dest := args[len(args)-1]

			if err := application.Move(sources, dest, apppkg.MoveOptions{
				Force: force,
			}); err != nil {
				return err
			}

			fmt.Printf("Moved %d file(s)\n", len(sources))
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force move (overwrite existing)")

	return cmd
}
