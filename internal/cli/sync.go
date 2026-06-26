package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewSyncCmd creates the sync subcommand.
func NewSyncCmd(application *apppkg.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync <subcommand>",
		Short: "Synchronize with remote repositories",
	}

	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.SyncEnable(); err != nil {
				return err
			}
			fmt.Println("Sync enabled")
			return nil
		},
	}

	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.SyncDisable(); err != nil {
				return err
			}
			fmt.Println("Sync disabled")
			return nil
		},
	}

	nowCmd := &cobra.Command{
		Use:   "now",
		Short: "Sync immediately",
		RunE: func(cmd *cobra.Command, args []string) error {
			stats, err := application.SyncNow()
			if err != nil {
				return err
			}
			fmt.Println("Sync completed")
			if stats.Pushed > 0 {
				fmt.Printf("  Pushed: %d file(s)\n", stats.Pushed)
			}
			if stats.Pulled > 0 {
				fmt.Printf("  Pulled: %d file(s)\n", stats.Pulled)
			}
			if stats.RemoteDeleted > 0 {
				fmt.Printf("  Deleted on remote: %d file(s)\n", stats.RemoteDeleted)
			}
			if stats.LocalDeleted > 0 {
				fmt.Printf("  Deleted locally: %d file(s)\n", stats.LocalDeleted)
			}
			if stats.Conflicts > 0 {
				fmt.Printf("  Conflicts: %d file(s)\n", stats.Conflicts)
			}
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := application.SyncStatus()
			if err != nil {
				return err
			}

			if status.Enabled {
				fmt.Println("Sync is enabled")
			} else {
				fmt.Println("Sync is disabled")
			}

			if status.RemoteName != "" {
				fmt.Printf("Remote: %s\n", status.RemoteName)
			}

			if status.LastSync != "" {
				fmt.Printf("Last sync: %s\n", status.LastSync)
			}

			return nil
		},
	}

	cmd.AddCommand(enableCmd, disableCmd, nowCmd, statusCmd)

	return cmd
}
