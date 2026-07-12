package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/spf13/cobra"
)

var watchInterval int
var watchKeep int

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Auto-watch and save changes",
	Long:  "Manage the background watch daemon that auto-saves workspace changes. Subcommands: on, off, status, pause, resume.",
}

var watchOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Start background watching",
	Long:  "Start a background daemon that watches the workspace for file changes and auto-saves snapshots at the configured interval. Only creates a snapshot when changes are detected.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
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
		// Pre-validate that cwd is an openable drift project BEFORE spawning
		// the daemon subprocess. Without this, StartDaemon would spawn a
		// child that fails during its own openProjectOrReport and exits
		// silently (no console on Windows), leaving a stale PID file and a
		// misleading "Daemon started" message. Validating here surfaces the
		// error synchronously in the parent where the user can see it.
		store, cfg, err := openProjectOrReport("Watch", "watch", cwd)
		if err != nil {
			return err
		}
		store.Close()
		_ = cfg // validated; the daemon reopens the project itself
		pid, err := porcelain.StartDaemon(ctx, cwd, watchInterval, watchKeep)
		if err != nil {
			statusFailed("Watch", err.Error(), "use 'drift watch off' to stop it first.")
			return ErrSilent
		}
		if !globalQuiet {
			statusActive("Watching")
			fmt.Printf("Daemon started (PID %d). Auto-save every %ds.\n", pid, watchInterval)
			fmt.Printf("Keep last %d auto-saves (older ones auto-pruned).\n", watchKeep)
			fmt.Println("Use 'drift watch off' to stop, 'drift watch status' to check.")
		}
		return nil
	},
}

var watchOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Stop background watching",
	Long:  "Stop the running watch daemon. Reports the number of auto-saves created and snapshots pruned during the session.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		autoSaves, pruned, err := porcelain.StopDaemon(ctx, cwd)
		if err != nil {
			statusFailed("Watch", err.Error(), "")
			return ErrSilent
		}
		if !globalQuiet {
			statusOK("Stopped")
			fmt.Printf("Daemon stopped. %d auto-saves created.\n", autoSaves)
			if pruned > 0 {
				fmt.Printf("%d older auto-saves pruned during this session.\n", pruned)
			}
		}
		return nil
	},
}

var watchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show watch daemon status",
	Long:  "Show whether the watch daemon is running, paused, or inactive. When active, displays uptime, auto-save count, and last save summary.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		state, active, err := porcelain.DaemonStatus(ctx, cwd)
		if err != nil {
			return err
		}
		// Quiet mode: success produces no output (exit code is authoritative).
		if globalQuiet {
			return nil
		}
		if !active || state == nil {
			fmt.Println(">>> Watch [inactive]")
			fmt.Println("No watch daemon running.")
			fmt.Println("Start with 'drift watch on'.")
			return nil
		}
		if state.Paused {
			fmt.Println(">>> Watch [paused]")
			fmt.Println("Daemon paused. Configuration retained.")
			fmt.Println("Use 'drift watch resume' to continue.")
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
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		// File logging is initialized by rootCmd.PersistentPreRunE (shared
		// with all commands). The daemon runs as a background process
		// without a terminal, so slog output goes to .drift/logs/drift.log.
		slog.Info("watch daemon starting", "cwd", cwd, "interval", watchInterval, "keep", watchKeep)

		store, cfg, err := openProjectOrReport("Watch", "watch", cwd)
		if err != nil {
			slog.Error("watch daemon open project failed", "error", err)
			// Init failed before RunDaemonLoop could install its cleanup
			// handlers. The parent already wrote watch.pid pointing at this
			// process; remove it so a subsequent `drift watch status` does
			// not see a stale PID. Best-effort: only remove if the file
			// contains OUR pid, to avoid clobbering a different daemon that
			// won the start race.
			porcelain.RemoveStalePidFile(cwd, os.Getpid())
			return err
		}
		defer store.Close()
		slog.Info("watch daemon project opened, entering loop")
		porcelain.RunDaemonLoop(ctx, store, cwd, watchInterval, watchKeep, &cfg.Core)
		slog.Info("watch daemon exited")
		return nil
	},
}

// watchPauseCmd pauses the running daemon without stopping it. The
// configuration (--interval/--keep) is retained so resume can pick up
// where pause left off.
var watchPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause the watch daemon",
	Long:  "Pause the running watch daemon without stopping it. The configuration (--interval/--keep) is retained so 'drift watch resume' can pick up where pause left off.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		if _, err := porcelain.PauseDaemon(ctx, cwd); err != nil {
			statusFailed("Watch", err.Error(), "use 'drift watch on' to start watching.")
			return ErrSilent
		}
		if !globalQuiet {
			fmt.Println(">>> Watch [paused]")
			fmt.Println("Daemon paused. Configuration retained.")
			fmt.Println("Use 'drift watch resume' to continue.")
		}
		return nil
	},
}

// watchResumeCmd resumes a paused daemon. The interval from the
// preserved state is echoed back so the user knows detection has resumed.
var watchResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume the watch daemon",
	Long:  "Resume a paused watch daemon. Detection resumes on the next tick using the retained --interval and --keep configuration.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		state, err := porcelain.ResumeDaemon(ctx, cwd)
		if err != nil {
			statusFailed("Watch", err.Error(), "use 'drift watch on' to start watching.")
			return ErrSilent
		}
		interval := state.Interval
		if interval <= 0 {
			interval = core.DefaultAutoSaveInterval
		}
		if !globalQuiet {
			statusActive("Watching")
			fmt.Printf("Daemon resumed. Auto-save every %ds.\n", interval)
		}
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
	watchCmd.AddCommand(watchPauseCmd)
	watchCmd.AddCommand(watchResumeCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(watchDaemonCmd)
}
