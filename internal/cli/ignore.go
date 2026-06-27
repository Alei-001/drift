package cli

import (
	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewIgnoreCmd creates the ignore subcommand.
func NewIgnoreCmd(application *apppkg.App) *cobra.Command {
	return &cobra.Command{
		Use:   "ignore <pattern>",
		Short: "Add a pattern to .driftignore",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return application.AddIgnorePattern(args[0])
		},
	}
}
