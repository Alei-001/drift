package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/drift/drift/internal/config"
	driftsync "github.com/drift/drift/internal/sync"
	"github.com/spf13/cobra"
)

var (
	syncShowRemote  bool
	syncUnsetRemote bool
)

// syncCmd is the parent command for all sync operations.
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Manage remote synchronization",
	Long: `Synchronize projects to a remote storage location (NAS mount, cloud-drive
synced folder, etc.).

Setup:
  drift sync remote /mnt/nas           # set remote root (global)
  drift sync enable                    # enable sync for current project
  drift save -m "changes"              # save auto-syncs if enabled

Other commands:
  drift sync status                    # show sync state
  drift sync now                       # sync immediately
  drift sync disable                   # disable sync for current project`,
}

// syncRemoteCmd manages the global remote root path.
var syncRemoteCmd = &cobra.Command{
	Use:   "remote [path]",
	Short: "Set, show, or unset the global remote root path",
	Long: `The remote root is a directory (e.g. a NAS mount or cloud-drive folder) where
drift projects are stored. It is configured once and shared by all projects.

Examples:
  drift sync remote /mnt/nas           # set remote root
  drift sync remote --show             # show current remote root
  drift sync remote --unset            # remove remote root`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}

		if syncUnsetRemote {
			gcfg.RemoteRoot = ""
			if err := driftsync.SaveGlobalConfig(gcfg); err != nil {
				return err
			}
			fmt.Println("Remote root unset")
			return nil
		}

		if syncShowRemote {
			if gcfg.RemoteRoot == "" {
				fmt.Println("No remote root configured")
			} else {
				fmt.Println(gcfg.RemoteRoot)
			}
			return nil
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		// Set mode: validate the path exists and is a directory.
		path := args[0]
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("cannot access %q: %w", abs, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", abs)
		}

		gcfg.RemoteRoot = abs
		if err := driftsync.SaveGlobalConfig(gcfg); err != nil {
			return err
		}
		fmt.Printf("Remote root set to %s\n", abs)
		return nil
	},
}

// syncEnableCmd enables sync for the current project.
var syncEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable sync for the current project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}
		if gcfg.RemoteRoot == "" {
			return fmt.Errorf("no remote root configured (run 'drift sync remote <path>' first)")
		}

		// Use the current directory name as the remote project name.
		remoteName := filepath.Base(sharedDir)

		// Ensure the project has an ID.
		if sharedConfig.Sync.ProjectID == "" {
			sharedConfig.Sync.ProjectID = driftsync.NewProjectID()
		}
		sharedConfig.Sync.Enabled = true
		sharedConfig.Sync.RemoteName = remoteName

		if err := config.SaveConfig(sharedStore.DriftDir(), sharedConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		// Create the remote project directory if it doesn't exist.
		remoteDir := filepath.Join(gcfg.RemoteRoot, remoteName)
		if err := os.MkdirAll(remoteDir, 0755); err != nil {
			return fmt.Errorf("failed to create remote project dir: %w", err)
		}

		fmt.Printf("Sync enabled for project %q\n", remoteName)
		fmt.Printf("Remote: %s\n", remoteDir)
		fmt.Println("\nRun 'drift sync now' to perform the initial sync.")
		return nil
	},
}

// syncDisableCmd disables sync for the current project.
var syncDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable sync for the current project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !sharedConfig.Sync.Enabled {
			fmt.Println("Sync is not enabled for this project")
			return nil
		}
		sharedConfig.Sync.Enabled = false
		if err := config.SaveConfig(sharedStore.DriftDir(), sharedConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("Sync disabled")
		return nil
	},
}

// syncStatusCmd shows the current sync state.
var syncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status for the current project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !sharedConfig.Sync.Enabled {
			fmt.Println("Sync is not enabled for this project")
			fmt.Println("Run 'drift sync enable' to enable.")
			return nil
		}

		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}
		if gcfg.RemoteRoot == "" {
			fmt.Println("Sync enabled but no remote root configured.")
			fmt.Println("Run 'drift sync remote <path>' to set it.")
			return nil
		}

		fmt.Printf("Project:  %s\n", sharedConfig.Sync.RemoteName)
		fmt.Printf("Remote:   %s\n", filepath.Join(gcfg.RemoteRoot, sharedConfig.Sync.RemoteName))
		fmt.Printf("Enabled:  yes\n")
		if sharedConfig.Sync.LastSync != "" {
			fmt.Printf("Last sync: %s\n", sharedConfig.Sync.LastSync)
		} else {
			fmt.Printf("Last sync: never (run 'drift sync now')\n")
		}
		return nil
	},
}

// syncNowCmd performs an immediate sync (push + pull).
var syncNowCmd = &cobra.Command{
	Use:   "now",
	Short: "Sync immediately (push local changes, pull remote changes)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !sharedConfig.Sync.Enabled {
			return fmt.Errorf("sync is not enabled (run 'drift sync enable')")
		}

		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}
		if gcfg.RemoteRoot == "" {
			return fmt.Errorf("no remote root configured")
		}

		remoteName := sharedConfig.Sync.RemoteName
		remoteDir := filepath.Join(gcfg.RemoteRoot, remoteName)

		// Ensure remote project dir exists.
		if err := os.MkdirAll(remoteDir, 0755); err != nil {
			return fmt.Errorf("failed to access remote: %w", err)
		}

		// Phase 1: simple full-directory mirror. Copy local → remote, then
		// remote → local, skipping identical files. This is a baseline; a
		// proper incremental sync (hash-based, with deletion tracking) will
		// come in a later step.
		fmt.Printf("Syncing to %s...\n", remoteDir)

		if err := mirrorDir(sharedDir, remoteDir); err != nil {
			return fmt.Errorf("push failed: %w", err)
		}
		if err := mirrorDir(remoteDir, sharedDir); err != nil {
			return fmt.Errorf("pull failed: %w", err)
		}

		// Update last sync timestamp.
		sharedConfig.Sync.LastSync = time.Now().Format(time.RFC3339)
		if err := config.SaveConfig(sharedStore.DriftDir(), sharedConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update sync timestamp: %v\n", err)
		}

		fmt.Println("Sync complete")
		return nil
	},
}

// mirrorDir copies all files from src into dst that are missing or newer in
// src. It does not delete files in dst that are absent from src (that requires
// deletion tracking, planned for a later phase). The .drift/lock file is
// always skipped to avoid cross-machine lock contention.
func mirrorDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip the lock file — it is machine-local.
		if rel == filepath.Join(".drift", "lock") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		// Skip symlinks for safety.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Skip if target exists and is not older than source.
		if tInfo, err := os.Stat(target); err == nil {
			if !tInfo.ModTime().Before(info.ModTime()) && tInfo.Size() == info.Size() {
				return nil
			}
		}

		return copyFileForSync(path, target, info.Mode())
	})
}

// copyFileForSync copies a single file, creating parent dirs as needed.
func copyFileForSync(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func init() {
	syncRemoteCmd.Flags().BoolVar(&syncShowRemote, "show", false, "Show the current remote root")
	syncRemoteCmd.Flags().BoolVar(&syncUnsetRemote, "unset", false, "Remove the remote root")

	syncCmd.AddCommand(syncRemoteCmd)
	syncCmd.AddCommand(syncEnableCmd)
	syncCmd.AddCommand(syncDisableCmd)
	syncCmd.AddCommand(syncStatusCmd)
	syncCmd.AddCommand(syncNowCmd)
	rootCmd.AddCommand(syncCmd)
}
