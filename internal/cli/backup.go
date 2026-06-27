package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

func NewBackupCmd(application *apppkg.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup <subcommand>",
		Short: "Manage remote backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("use 'drift backup on', 'drift backup off', 'drift backup now', 'drift backup status', or 'drift backup log'")
		},
	}

	onCmd := &cobra.Command{
		Use:   "on",
		Short: "Enable auto-backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.ConfigSet(apppkg.LocalScope, "sync.enabled", "true"); err != nil {
				return err
			}
			fmt.Println(colorGreen("Auto-backup enabled"))
			return nil
		},
	}

	offCmd := &cobra.Command{
		Use:   "off",
		Short: "Disable auto-backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.ConfigSet(apppkg.LocalScope, "sync.enabled", "false"); err != nil {
				return err
			}
			fmt.Println("Auto-backup disabled")
			return nil
		},
	}

	nowCmd := &cobra.Command{
		Use:   "now",
		Short: "Backup now",
		RunE: func(cmd *cobra.Command, args []string) error {
			stats, err := application.SyncNow()
			if err != nil {
				return err
			}
			if stats.Pushed == 0 && stats.Pulled == 0 {
				fmt.Println(colorGray("Already up to date."))
			} else {
				fmt.Println(colorGreen(fmt.Sprintf("Backup complete: %d pushed, %d pulled", stats.Pushed, stats.Pulled)))
			}
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show backup status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := application.SyncStatus()
			if err != nil {
				return err
			}
			if status.Enabled {
				fmt.Println(colorGreen("Auto-backup: ON"))
			} else {
				fmt.Println(colorYellow("Auto-backup: OFF"))
			}
			if status.RemoteName != "" {
				fmt.Printf("Remote: %s\n", status.RemoteName)
			}
			if status.LastSync != "" {
				fmt.Printf("Last backup: %s\n", status.LastSync)
			}
			return nil
		},
	}

	logCmd := &cobra.Command{
		Use:   "log",
		Short: "Show backup history",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(colorGray("No backup history yet"))
			return nil
		},
	}

	cmd.AddCommand(onCmd, offCmd, nowCmd, statusCmd, logCmd)
	return cmd
}
