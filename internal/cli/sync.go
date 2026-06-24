package cli

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	syncProtocol    string
	syncHost        string
	syncPort        int
	syncPath        string
	syncUser        string
	syncPass        string
	syncTLS         bool
	syncInsecure    bool
	syncShare       string
	syncKeyPath     string
)

// syncCmd is the parent command for all sync operations.
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Manage remote synchronization",
	Long: `Synchronize projects to a remote storage location.

Supported protocols:
  local  - local filesystem (NAS mount, cloud-drive synced folder)
  webdav - WebDAV server (Nextcloud, ownCloud, Synology, 坚果云)
  ftp    - FTP/FTPS server
  sftp   - SFTP server (SSH file transfer)
  smb    - SMB/CIFS share (Windows share, NAS)

Setup:
  drift sync remote --protocol local --path /mnt/nas
  drift sync remote --protocol webdav --host cloud.example.com --path /dav --tls --user alice
  drift sync remote --protocol ftp --host nas.local --path /backups --user alice
  drift sync remote --protocol sftp --host nas.local --path /backups --user alice
  drift sync remote --protocol smb --host nas.local --share photos --user alice
  drift sync enable
  drift save -m "changes"              # save auto-syncs if enabled

Shorthand (auto-detects protocol):
  drift sync remote /mnt/nas                       # → local
  drift sync remote https://cloud.example.com/dav  # → webdav`,
}

// syncRemoteCmd manages the global remote configuration.
var syncRemoteCmd = &cobra.Command{
	Use:   "remote [path-or-url]",
	Short: "Set, show, or unset the global remote",
	Long: `Configure the remote storage backend.

With --protocol, uses the unified field set:
  drift sync remote --protocol webdav --host cloud.example.com --port 443 \
    --path /dav --tls --user alice --pass secret

Without --protocol, auto-detects from the positional argument:
  drift sync remote /mnt/nas                       # local path
  drift sync remote https://cloud.example.com/dav  # WebDAV URL

Other:
  drift sync remote --show    # show current remote
  drift sync remote --unset   # remove remote`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}

		if syncUnsetRemote {
			*gcfg = driftsync.GlobalConfig{}
			if err := driftsync.SaveGlobalConfig(gcfg); err != nil {
				return err
			}
			fmt.Println("Remote unset")
			return nil
		}

		if syncShowRemote {
			if gcfg.Protocol == "" {
				fmt.Println("No remote configured")
				return nil
			}
			fmt.Printf("Protocol: %s\n", gcfg.Protocol)
			if gcfg.Host != "" {
				fmt.Printf("Host:     %s:%d\n", gcfg.Host, gcfg.EffectivePort())
			}
			if gcfg.Path != "" {
				fmt.Printf("Path:     %s\n", gcfg.Path)
			}
			if gcfg.Share != "" {
				fmt.Printf("Share:    %s\n", gcfg.Share)
			}
			if gcfg.Username != "" {
				fmt.Printf("User:     %s\n", gcfg.Username)
			}
			if gcfg.TLS {
				fmt.Printf("TLS:      yes\n")
			}
			return nil
		}

		if syncProtocol != "" {
			// Explicit protocol mode: use unified fields.
			return setRemoteFromFlags(gcfg)
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		// Auto-detect mode from positional argument.
		target := args[0]
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return setRemoteFromWebDAVURL(gcfg, target)
		}
		return setRemoteFromLocalPath(gcfg, target)
	},
}

// setRemoteFromFlags configures the remote from the unified --protocol flags.
func setRemoteFromFlags(gcfg *driftsync.GlobalConfig) error {
	protocol := strings.ToLower(syncProtocol)
	switch protocol {
	case "local":
		if syncPath == "" {
			return fmt.Errorf("--path is required for local protocol")
		}
		abs, err := filepath.Abs(syncPath)
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
		*gcfg = driftsync.GlobalConfig{
			Protocol: "local",
			Path:     abs,
		}
		fmt.Printf("Remote set: local://%s\n", abs)

	case "webdav", "ftp", "sftp", "smb":
		if syncHost == "" {
			return fmt.Errorf("--host is required for %s protocol", protocol)
		}
		if protocol == "smb" && syncShare == "" {
			return fmt.Errorf("--share is required for smb protocol")
		}
		gcfg.Protocol = protocol
		gcfg.Host = syncHost
		if syncPort != 0 {
			gcfg.Port = syncPort
		} else {
			gcfg.Port = 0 // use default
		}
		gcfg.Path = syncPath
		gcfg.Username = syncUser
		gcfg.Password = syncPass
		gcfg.TLS = syncTLS
		gcfg.InsecureSkipVerify = syncInsecure
		gcfg.Share = syncShare
		gcfg.KeyPath = syncKeyPath

		// Prompt for credentials if not provided and protocol needs them.
		if protocol != "local" && gcfg.Username == "" {
			fmt.Print("Username: ")
			reader := bufio.NewReader(os.Stdin)
			u, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read username: %w", err)
			}
			gcfg.Username = strings.TrimSpace(u)
		}
		// Prompt for password if not provided.
		// For SFTP, skip the prompt only if a key path is configured (key-based auth).
		needPassword := gcfg.Password == "" && (protocol != "sftp" || gcfg.KeyPath == "")
		if needPassword {
			fmt.Print("Password: ")
			p, err := readPassword()
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			gcfg.Password = p
		}

		fmt.Printf("Remote set: %s\n", remoteDisplayString(gcfg, ""))
	default:
		return fmt.Errorf("unsupported protocol %q (valid: local, webdav, ftp, sftp, smb)", protocol)
	}

	return driftsync.SaveGlobalConfig(gcfg)
}

// setRemoteFromWebDAVURL configures a WebDAV remote from a URL (backward compat).
func setRemoteFromWebDAVURL(gcfg *driftsync.GlobalConfig, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	gcfg.Protocol = "webdav"
	gcfg.Host = u.Hostname()
	gcfg.Path = u.Path
	gcfg.TLS = u.Scheme == "https"
	if u.Port() != "" {
		gcfg.Port, _ = strconv.Atoi(u.Port())
	} else if u.Scheme == "https" {
		gcfg.Port = 443
	} else {
		gcfg.Port = 80
	}
	gcfg.Username = syncUser
	gcfg.Password = syncPass

	// Prompt for credentials if not provided.
	if gcfg.Username == "" {
		fmt.Print("WebDAV username: ")
		reader := bufio.NewReader(os.Stdin)
		u, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
		gcfg.Username = strings.TrimSpace(u)
	}
	if gcfg.Password == "" {
		fmt.Print("WebDAV password: ")
		p, err := readPassword()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		gcfg.Password = p
	}

	if err := driftsync.SaveGlobalConfig(gcfg); err != nil {
		return err
	}
	fmt.Printf("Remote set: %s\n", rawURL)
	return nil
}

// setRemoteFromLocalPath configures a local remote from a path (backward compat).
func setRemoteFromLocalPath(gcfg *driftsync.GlobalConfig, target string) error {
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

	*gcfg = driftsync.GlobalConfig{
		Protocol: "local",
		Path:     abs,
	}
	if err := driftsync.SaveGlobalConfig(gcfg); err != nil {
		return err
	}
	fmt.Printf("Remote set: %s\n", abs)
	return nil
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
			return fmt.Errorf("no remote configured (run 'drift sync remote --protocol <local|webdav|ftp|sftp|smb> ...' first)")
		}

		remoteName := filepath.Base(sharedDir)

		if sharedConfig.Sync.ProjectID == "" {
			sharedConfig.Sync.ProjectID = driftsync.NewProjectID()
		}
		sharedConfig.Sync.Enabled = true
		sharedConfig.Sync.RemoteName = remoteName

		if err := config.SaveConfig(sharedStore.DriftDir(), sharedConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		// For local remotes, create the project directory.
		if gcfg.GetRemoteType() == driftsync.RemoteLocal {
			remoteDir := filepath.Join(gcfg.Path, remoteName)
			if err := os.MkdirAll(remoteDir, 0755); err != nil {
				return fmt.Errorf("failed to create remote project dir: %w", err)
			}
		}

		fmt.Printf("Sync enabled for project %q\n", remoteName)
		fmt.Printf("Remote: %s\n", remoteDisplayString(gcfg, remoteName))
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
			fmt.Println("Run 'drift sync remote --protocol <local|webdav|ftp|sftp|smb> ...' to set it.")
			return nil
		}

		fmt.Printf("Project:  %s\n", sharedConfig.Sync.RemoteName)
		fmt.Printf("Remote:   %s\n", remoteDisplayString(gcfg, sharedConfig.Sync.RemoteName))
		fmt.Printf("Protocol: %s\n", gcfg.Protocol)
		fmt.Printf("Enabled:  yes\n")
		if sharedConfig.Sync.LastSync != "" {
			fmt.Printf("Last sync: %s\n", sharedConfig.Sync.LastSync)
		} else {
			fmt.Printf("Last sync: never (run 'drift sync now')\n")
		}
		return nil
	},
}

// remoteDisplayString returns a human-readable URL-like string for the remote.
func remoteDisplayString(gcfg *driftsync.GlobalConfig, remoteName string) string {
	port := gcfg.EffectivePort()
	switch gcfg.Protocol {
	case "local":
		if remoteName != "" {
			return filepath.Join(gcfg.Path, remoteName)
		}
		return gcfg.Path
	case "webdav":
		scheme := "http"
		if gcfg.TLS {
			scheme = "https"
		}
		base := fmt.Sprintf("%s://%s:%d", scheme, gcfg.Host, port)
		if gcfg.Path != "" {
			base += "/" + strings.TrimPrefix(gcfg.Path, "/")
		}
		if remoteName != "" {
			base += "/" + remoteName
		}
		return base
	case "ftp":
		scheme := "ftp"
		if gcfg.TLS {
			scheme = "ftps"
		}
		s := fmt.Sprintf("%s://%s:%d", scheme, gcfg.Host, port)
		if gcfg.Path != "" {
			s += "/" + strings.Trim(gcfg.Path, "/")
		}
		if remoteName != "" {
			s += "/" + remoteName
		}
		return s
	case "sftp":
		s := fmt.Sprintf("sftp://%s@%s:%d", gcfg.Username, gcfg.Host, port)
		if gcfg.Path != "" {
			s += "/" + strings.Trim(gcfg.Path, "/")
		}
		if remoteName != "" {
			s += "/" + remoteName
		}
		return s
	case "smb":
		s := fmt.Sprintf("smb://%s@%s/%s", gcfg.Username, gcfg.Host, gcfg.Share)
		if gcfg.Path != "" {
			s += "/" + strings.Trim(gcfg.Path, "/")
		}
		if remoteName != "" {
			s += "/" + remoteName
		}
		return s
	}
	return "(unknown)"
}

// syncNowCmd performs an immediate sync.
var syncNowCmd = &cobra.Command{
	Use:   "now",
	Short: "Sync immediately (push local changes, pull remote changes)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSync(sharedDir, sharedConfig, sharedStore, true)
	},
}

// runSync executes a sync operation.
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
		remoteDir := filepath.Join(gcfg.Path, remoteName)
		if err := os.MkdirAll(remoteDir, 0755); err != nil {
			return fmt.Errorf("failed to access remote: %w", err)
		}
	}

	if verbose {
		fmt.Printf("Syncing to %s...\n", remoteDisplayString(gcfg, remoteName))
	}

	// Create the transport (may connect to remote server).
	transport, err := driftsync.ProjectTransportForConfig(gcfg, remoteName)
	if err != nil {
		return fmt.Errorf("failed to connect to remote: %w", err)
	}
	defer transport.Close()

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
	if len(r.Conflicts) > 0 {
		fmt.Printf("  Warning: %d conflict(s) detected (local version used):\n", len(r.Conflicts))
		for _, p := range r.Conflicts {
			fmt.Printf("    %s\n", p)
		}
	}
}

// AutoSyncAfterSave is called after a successful 'drift save' to trigger
// background synchronization. Silent if no remote configured; warns on failure.
func AutoSyncAfterSave(localDir string, cfg *config.Config, store *storage.Store) {
	if !cfg.Sync.Enabled {
		return
	}
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil || gcfg.GetRemoteType() == driftsync.RemoteNone {
		return
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := runSync(localDir, cfg, store, false); err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		return
	}
	if lastErr != nil {
		fmt.Fprintf(os.Stderr, "\n⚠ Sync failed: %v (will retry next save)\n", lastErr)
	}
}

// readPassword reads a password from stdin. The password is echoed to the
// terminal (no echo suppression is implemented). For a more secure experience,
// use the --pass flag or pipe input non-interactively.
func readPassword() (string, error) {
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
	syncRemoteCmd.Flags().StringVar(&syncProtocol, "protocol", "", "Protocol: local, webdav, ftp, sftp, smb")
	syncRemoteCmd.Flags().StringVar(&syncHost, "host", "", "Remote server hostname or IP")
	syncRemoteCmd.Flags().IntVar(&syncPort, "port", 0, "Remote server port (0 = protocol default)")
	syncRemoteCmd.Flags().StringVar(&syncPath, "path", "", "Remote base path or local filesystem path")
	syncRemoteCmd.Flags().StringVar(&syncUser, "user", "", "Username for authentication")
	syncRemoteCmd.Flags().StringVar(&syncPass, "pass", "", "Password for authentication")
	syncRemoteCmd.Flags().BoolVar(&syncTLS, "tls", false, "Use TLS (FTPS, HTTPS)")
	syncRemoteCmd.Flags().BoolVar(&syncInsecure, "insecure", false, "Skip TLS certificate verification (self-signed certs)")
	syncRemoteCmd.Flags().StringVar(&syncShare, "share", "", "SMB share name")
	syncRemoteCmd.Flags().StringVar(&syncKeyPath, "key-path", "", "SFTP private key path")

	syncCmd.AddCommand(syncRemoteCmd)
	syncCmd.AddCommand(syncEnableCmd)
	syncCmd.AddCommand(syncDisableCmd)
	syncCmd.AddCommand(syncStatusCmd)
	syncCmd.AddCommand(syncNowCmd)
	rootCmd.AddCommand(syncCmd)
}
