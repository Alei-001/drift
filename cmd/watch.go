package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
)

var watchInterval int
var watchKeep int

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Auto-watch and save changes",
}

var watchOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Start background watching",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if watchInterval <= 0 {
			statusFailed("Watch", "--interval must be a positive number of seconds.", "")
			return ErrSilent
		}
		if watchKeep < 0 {
			statusFailed("Watch", "--keep must be zero or a positive number.", "")
			return ErrSilent
		}
		pid, err := porcelain.StartDaemon(ctx, cwd, watchInterval, watchKeep)
		if err != nil {
			statusFailed("Watch", err.Error(), "use 'drift watch off' to stop it first.")
			return ErrSilent
		}
		statusActive("Watching")
		fmt.Printf("Daemon started (PID %d). Auto-save every %ds.\n", pid, watchInterval)
		fmt.Printf("Keep last %d auto-saves (older ones auto-pruned).\n", watchKeep)
		fmt.Println("Use 'drift watch off' to stop, 'drift watch status' to check.")
		return nil
	},
}

var watchOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Stop background watching",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		autoSaves, pruned, err := porcelain.StopDaemon(ctx, cwd)
		if err != nil {
			statusFailed("Watch", err.Error(), "")
			return ErrSilent
		}
		statusOK("Stopped")
		fmt.Printf("Daemon stopped. %d auto-saves created.\n", autoSaves)
		if pruned > 0 {
			fmt.Printf("%d older auto-saves pruned during this session.\n", pruned)
		}
		return nil
	},
}

var watchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show watch daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		state, active, err := porcelain.DaemonStatus(ctx, cwd)
		if err != nil {
			return err
		}
		if !active || state == nil {
			fmt.Println(">>> Watch [inactive]")
			fmt.Println("No watch daemon running.")
			fmt.Println("Start with 'drift watch on'.")
			return nil
		}
		statusActive("Watching")
		elapsed := time.Since(time.Unix(state.StartTime, 0))
		fmt.Printf("Running since %s (%s ago).\n",
			time.Unix(state.StartTime, 0).Format("15:04"), formatDuration(elapsed))
		if state.MaxSaves == 0 {
			fmt.Printf("Auto-saves: %d (unlimited)\n", state.AutoSaves)
		} else {
			fmt.Printf("Auto-saves: %d (%d max)\n", state.AutoSaves, state.MaxSaves)
		}
		if state.LastSaveTime > 0 {
			fmt.Printf("Last save: %s %s\n",
				time.Unix(state.LastSaveTime, 0).Format("15:04"), state.LastSaveChanges)
		}
		return nil
	},
}

var watchDaemonCmd = &cobra.Command{
	Use:    "_watch_daemon",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Watch", cwd)
		if err != nil {
			return err
		}
		defer store.Close()
		porcelain.RunDaemonLoop(ctx, store, cwd, watchInterval, watchKeep, &cfg.Core)
		return nil
	},
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return fmt.Sprintf("%.1f h", d.Hours())
}

// watchDefaultKeep is the CLI default for --keep. It is intentionally larger
// than core.DefaultAutoSaveKeep (10) because the interactive `drift watch on`
// command is meant for active work sessions where more history is desirable.
const watchDefaultKeep = 50

func init() {
	watchOnCmd.Flags().IntVar(&watchInterval, "interval", core.DefaultAutoSaveInterval, "detection interval in seconds")
	watchOnCmd.Flags().IntVar(&watchKeep, "keep", watchDefaultKeep, "max auto-saves to keep")
	watchDaemonCmd.Flags().IntVar(&watchInterval, "interval", core.DefaultAutoSaveInterval, "detection interval in seconds")
	watchDaemonCmd.Flags().IntVar(&watchKeep, "keep", watchDefaultKeep, "max auto-saves to keep")
	watchCmd.AddCommand(watchOnCmd)
	watchCmd.AddCommand(watchOffCmd)
	watchCmd.AddCommand(watchStatusCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(watchDaemonCmd)
}
