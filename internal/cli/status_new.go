package cli

import (
	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates the status subcommand.
func NewStatusCmd(application *apppkg.App) *cobra.Command {
	var porcelain bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show working tree status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := application.Status()
			if err != nil {
				return err
			}

			if porcelain {
				printStatusPorcelain(*status)
			} else {
				printStatus(*status)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&porcelain, "porcelain", false, "Machine-readable output")

	return cmd
}
