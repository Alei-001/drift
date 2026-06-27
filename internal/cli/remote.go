package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

func NewRemoteCmd(application *apppkg.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote <subcommand>",
		Short: "Configure remote backup target",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("use 'drift remote setup', 'drift remote show', or 'drift remote remove'")
		},
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive remote configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return application.RemoteSetup()
		},
	}

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show current remote configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return application.RemoteShow()
		},
	}

	removeCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove remote configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return application.RemoteRemove()
		},
	}

	cmd.AddCommand(setupCmd, showCmd, removeCmd)
	return cmd
}
