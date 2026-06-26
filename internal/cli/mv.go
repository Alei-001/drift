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

			globalDryRun, _ := cmd.Flags().GetBool("dry-run")

			if globalDryRun {
				moved, err := application.Move(sources, dest, apppkg.MoveOptions{
					Force:  force,
					DryRun: true,
				})
				if err != nil {
					return err
				}
				fmt.Printf("Would move %d file(s):\n", len(moved))
				for _, p := range moved {
					fmt.Printf("  %s\n", p)
				}
				return nil
			}

			moved, err := application.Move(sources, dest, apppkg.MoveOptions{
				Force: force,
			})
			if err != nil {
				return err
			}

			for _, p := range moved {
				fmt.Printf("Moved: %s\n", p)
			}
			fmt.Printf("Moved %d file(s)\n", len(moved))
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force move (overwrite existing)")

	return cmd
}
