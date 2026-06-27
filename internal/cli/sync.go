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

	pushCmd := &cobra.Command{
		Use:   "push [<branch>]",
		Short: "Push objects to the remote",
		RunE: func(cb *cobra.Command, args []string) error {
			branch := ""
			if len(args) > 0 {
				branch = args[0]
			}
			stats, err := application.Push(branch)
			if err != nil {
				return err
			}
			fmt.Println(colorGreen(fmt.Sprintf("Push complete: %d new object(s) pushed to %s", stats.Pushed, stats.Branch)))
			return nil
		},
	}

	pullCmd := &cobra.Command{
		Use:   "pull [<branch>]",
		Short: "Pull objects from the remote",
		RunE: func(cb *cobra.Command, args []string) error {
			branch := ""
			if len(args) > 0 {
				branch = args[0]
			}
			stats, err := application.Pull(branch)
			if err != nil {
				return err
			}
			if stats.Pulled == 0 {
				fmt.Println(colorGray("Already up to date."))
			} else {
				fmt.Println(colorGreen(fmt.Sprintf("Pull complete: %d new object(s) pulled from %s", stats.Pulled, stats.Branch)))
			}
			return nil
		},
	}

	nowCmd := &cobra.Command{
		Use:   "now",
		Short: "Push then pull",
		RunE: func(cb *cobra.Command, args []string) error {
			stats, err := application.SyncNow()
			if err != nil {
				return err
			}
			if stats.Pushed == 0 && stats.Pulled == 0 {
				fmt.Println(colorGray("Already up to date."))
			} else {
				fmt.Println(colorGreen(fmt.Sprintf("Sync complete: %d pushed, %d pulled", stats.Pushed, stats.Pulled)))
			}
			return nil
		},
	}

	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable sync",
		RunE: func(cb *cobra.Command, args []string) error {
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
		RunE: func(cb *cobra.Command, args []string) error {
			if err := application.SyncDisable(); err != nil {
				return err
			}
			fmt.Println("Sync disabled")
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync status",
		RunE: func(cb *cobra.Command, args []string) error {
			status, err := application.SyncStatus()
			if err != nil {
				return err
			}

			if status.Enabled {
				fmt.Println(colorGreen("Sync is enabled"))
			} else {
				fmt.Println(colorYellow("Sync is disabled"))
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

	cmd.AddCommand(pushCmd, pullCmd, nowCmd, enableCmd, disableCmd, statusCmd)

	return cmd
}
