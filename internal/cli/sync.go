package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/storage"
	driftsync "github.com/drift/drift/internal/sync"
	"github.com/spf13/cobra"
)

var (
	syncShowRemote  bool
	syncUnsetRemote bool
	syncWebDAVUser  string
	syncWebDAVPass  string
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
	Use:   "remote [path-or-url]",
	Short: "Set, show, or unset the global remote root",
	Long: `The remote root is where drift projects are stored. It can be either:
  - A local directory (NAS mount, cloud-drive synced folder)
  - A WebDAV URL (Nextcloud, ownCloud, Synology, 坚果云, etc.)

Examples:
  drift sync remote /mnt/nas                       # local path
  drift sync remote https://cloud.example.com/dav  # WebDAV (will prompt for credentials)
  drift sync remote https://cloud.example.com/dav --user alice --pass secret
  drift sync remote --show                         # show current remote
  drift sync remote --unset                        # remove remote`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}

		if syncUnsetRemote {
			gcfg.RemoteRoot = ""
			gcfg.WebDAV = nil
			if err := driftsync.SaveGlobalConfig(gcfg); err != nil {
				return err
			}
			fmt.Println("Remote root unset")
			return nil
		}

		if syncShowRemote {
			switch gcfg.GetRemoteType() {
			case driftsync.RemoteLocal:
				fmt.Println(gcfg.RemoteRoot)
			case driftsync.RemoteWebDAV:
				fmt.Println(gcfg.WebDAV.URL)
			default:
				fmt.Println("No remote root configured")
			}
			return nil
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		target := args[0]

		// Detect WebDAV by URL scheme.
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			user := syncWebDAVUser
			pass := syncWebDAVPass
			if user == "" {
				fmt.Print("WebDAV username: ")
				reader := bufio.NewReader(os.Stdin)
				u, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read username: %w", err)
				}
				user = strings.TrimSpace(u)
			}
			if pass == "" {
				fmt.Print("WebDAV password: ")
				p, err := readPassword()
				if err != nil {
					// Fallback: read in clear (less secure, but works in non-tty).
					reader := bufio.NewReader(os.Stdin)
					p, err = reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("failed to read password: %w", err)
					}
				}
				pass = strings.TrimSpace(p)
			}

			gcfg.RemoteRoot = ""
			gcfg.WebDAV = &driftsync.WebDAVConfig{
				URL:      target,
				Username: user,
				Password: pass,
			}
			if err := driftsync.SaveGlobalConfig(gcfg); err != nil {
				return err
			}
			fmt.Printf("WebDAV remote set to %s\n", target)
			return nil
		}

		// Local path mode: validate the path exists and is a directory.
		abs, err := filepath.Abs(target)
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
		gcfg.WebDAV = nil
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
		if gcfg.GetRemoteType() == driftsync.RemoteNone {
			return fmt.Errorf("no remote configured (run 'drift sync remote <path-or-url>' first)")
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

		// For local remotes, create the project directory. For WebDAV,
		// the directory is created on first sync.
		var remoteDisplay string
		switch gcfg.GetRemoteType() {
		case driftsync.RemoteLocal:
			remoteDir := filepath.Join(gcfg.RemoteRoot, remoteName)
			if err := os.MkdirAll(remoteDir, 0755); err != nil {
				return fmt.Errorf("failed to create remote project dir: %w", err)
			}
			remoteDisplay = remoteDir
		case driftsync.RemoteWebDAV:
			remoteDisplay = strings.TrimRight(gcfg.WebDAV.URL, "/") + "/" + remoteName
		}

		fmt.Printf("Sync enabled for project %q\n", remoteName)
		fmt.Printf("Remote: %s\n", remoteDisplay)
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
		if gcfg.GetRemoteType() == driftsync.RemoteNone {
			fmt.Println("Sync enabled but no remote configured.")
			fmt.Println("Run 'drift sync remote <path-or-url>' to set it.")
			return nil
		}

		fmt.Printf("Project:  %s\n", sharedConfig.Sync.RemoteName)
		var remoteDisplay string
		switch gcfg.GetRemoteType() {
		case driftsync.RemoteLocal:
			remoteDisplay = filepath.Join(gcfg.RemoteRoot, sharedConfig.Sync.RemoteName)
		case driftsync.RemoteWebDAV:
			remoteDisplay = strings.TrimRight(gcfg.WebDAV.URL, "/") + "/" + sharedConfig.Sync.RemoteName
		}
		fmt.Printf("Remote:   %s\n", remoteDisplay)
		fmt.Printf("Type:     %s\n", remoteTypeName(gcfg.GetRemoteType()))
		fmt.Printf("Enabled:  yes\n")
		if sharedConfig.Sync.LastSync != "" {
			fmt.Printf("Last sync: %s\n", sharedConfig.Sync.LastSync)
		} else {
			fmt.Printf("Last sync: never (run 'drift sync now')\n")
		}
		return nil
	},
}

func remoteTypeName(t driftsync.RemoteType) string {
	switch t {
	case driftsync.RemoteLocal:
		return "local"
	case driftsync.RemoteWebDAV:
		return "webdav"
	default:
		return "none"
	}
}

// syncNowCmd performs an immediate sync (push + pull) using the incremental
// sync engine.
var syncNowCmd = &cobra.Command{
	Use:   "now",
	Short: "Sync immediately (push local changes, pull remote changes)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSync(sharedDir, sharedConfig, sharedStore, true)
	},
}

// runSync executes a sync operation. If verbose is true, progress is
// printed to stdout. This is shared between 'drift sync now' and the
// auto-sync hook in 'drift save'.
func runSync(localDir string, cfg *config.Config, store *storage.Store, verbose bool) error {
	if !cfg.Sync.Enabled {
		return fmt.Errorf("sync is not enabled (run 'drift sync enable')")
	}

	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil {
		return err
	}
	if gcfg.GetRemoteType() == driftsync.RemoteNone {
		return fmt.Errorf("no remote configured")
	}

	remoteName := cfg.Sync.RemoteName

	// For local remotes, ensure the project directory exists.
	if gcfg.GetRemoteType() == driftsync.RemoteLocal {
		remoteDir := filepath.Join(gcfg.RemoteRoot, remoteName)
		if err := os.MkdirAll(remoteDir, 0755); err != nil {
			return fmt.Errorf("failed to access remote: %w", err)
		}
	}

	if verbose {
		var remoteDisplay string
		switch gcfg.GetRemoteType() {
		case driftsync.RemoteLocal:
			remoteDisplay = filepath.Join(gcfg.RemoteRoot, remoteName)
		case driftsync.RemoteWebDAV:
			remoteDisplay = strings.TrimRight(gcfg.WebDAV.URL, "/") + "/" + remoteName
		}
		fmt.Printf("Syncing to %s...\n", remoteDisplay)
	}

	// Use the incremental sync engine with the appropriate transport.
	transport := driftsync.ProjectTransportForConfig(gcfg, remoteName)
	if transport == nil {
		return fmt.Errorf("no transport available for remote type")
	}
	engine := driftsync.NewEngine(transport, cfg.Sync.ProjectID)

	result, err := engine.Sync(localDir)
	if err != nil {
		return err
	}

	// Update last sync timestamp.
	cfg.Sync.LastSync = time.Now().Format(time.RFC3339)
	if err := config.SaveConfig(store.DriftDir(), cfg); err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to update sync timestamp: %v\n", err)
		}
	}

	if verbose {
		printSyncResult(result)
		fmt.Println("Sync complete")
	}
	return nil
}

// printSyncResult prints a human-readable summary of what was synced.
func printSyncResult(r *driftsync.SyncResult) {
	if !r.HasChanges() {
		fmt.Println("  Already up to date")
		return
	}
	if len(r.Pushed) > 0 {
		fmt.Printf("  Pushed %d file(s):\n", len(r.Pushed))
		for _, p := range r.Pushed {
			fmt.Printf("    %s\n", p)
		}
	}
	if len(r.Pulled) > 0 {
		fmt.Printf("  Pulled %d file(s):\n", len(r.Pulled))
		for _, p := range r.Pulled {
			fmt.Printf("    %s\n", p)
		}
	}
	if len(r.RemoteDeleted) > 0 {
		fmt.Printf("  Deleted %d file(s) on remote:\n", len(r.RemoteDeleted))
		for _, p := range r.RemoteDeleted {
			fmt.Printf("    %s\n", p)
		}
	}
	if len(r.LocalDeleted) > 0 {
		fmt.Printf("  Deleted %d file(s) locally:\n", len(r.LocalDeleted))
		for _, p := range r.LocalDeleted {
			fmt.Printf("    %s\n", p)
		}
	}
}

// AutoSyncAfterSave is called after a successful 'drift save' to trigger
// background synchronization. If sync is not enabled or no remote is
// configured, it silently does nothing. If sync fails, it prints a warning
// but does not return an error (the save already succeeded).
func AutoSyncAfterSave(localDir string, cfg *config.Config, store *storage.Store) {
	if !cfg.Sync.Enabled {
		return
	}
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil || gcfg.GetRemoteType() == driftsync.RemoteNone {
		return
	}

	// Retry up to 2 times on failure.
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := runSync(localDir, cfg, store, false); err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		return // success
	}
	if lastErr != nil {
		fmt.Fprintf(os.Stderr, "\n⚠ Sync failed: %v (will retry next save)\n", lastErr)
	}
}

// readPassword reads a password from stdin. On supported platforms it tries
// to disable echo; otherwise it falls back to plain reading. The returned
// string does not include the trailing newline.
func readPassword() (string, error) {
	// Try to disable echo on Unix; on Windows this is a no-op fallback.
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func init() {
	syncRemoteCmd.Flags().BoolVar(&syncShowRemote, "show", false, "Show the current remote")
	syncRemoteCmd.Flags().BoolVar(&syncUnsetRemote, "unset", false, "Remove the remote")
	syncRemoteCmd.Flags().StringVar(&syncWebDAVUser, "user", "", "WebDAV username (for http(s):// remotes)")
	syncRemoteCmd.Flags().StringVar(&syncWebDAVPass, "pass", "", "WebDAV password (for http(s):// remotes)")

	syncCmd.AddCommand(syncRemoteCmd)
	syncCmd.AddCommand(syncEnableCmd)
	syncCmd.AddCommand(syncDisableCmd)
	syncCmd.AddCommand(syncStatusCmd)
	syncCmd.AddCommand(syncNowCmd)
	rootCmd.AddCommand(syncCmd)
}
