package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/remote"
)

// remoteCmd is the parent command for remote management.
var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote storage backends (add, remove, list, set-url, test)",
	Long:  "Manage remote storage backends for push/pull. Subcommands: add, remove, list, set-url, test.",
}

// remoteListCmd lists all configured remotes.
var remoteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured remotes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		if len(rf.Remotes) == 0 {
			fmt.Println("(no remotes configured)")
			return nil
		}
		sort.Slice(rf.Remotes, func(i, j int) bool {
			return rf.Remotes[i].Name < rf.Remotes[j].Name
		})
		for _, r := range rf.Remotes {
			fmt.Printf("%s\t%s\t%s\n", r.Name, r.Type, r.URL)
		}
		return nil
	},
}

// remoteRemoveCmd deletes a remote from remotes.json. Credentials are NOT
// deleted (they may be shared with other repos).
var remoteRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a remote (credentials preserved in user-level config)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		if !rf.RemoveRemote(name) {
			return fmt.Errorf("remote %q not found", name)
		}
		if err := saveRemotes(cwd, rf); err != nil {
			return err
		}
		fmt.Printf("Remote %q removed. Credentials preserved in user-level config.\n", name)
		return nil
	},
}

// remoteSetURLCmd updates a remote's URL. Warns when host changes (credentials
// may need updating).
var remoteSetURLCmd = &cobra.Command{
	Use:   "set-url <name> <new-url>",
	Short: "Update a remote's URL",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, newURL := args[0], args[1]
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		cfg, err := rf.FindRemote(name)
		if err != nil {
			return fmt.Errorf("remote %q not found", name)
		}
		oldHost, _ := remote.HostFromURL(cfg.URL)
		newHost, _ := remote.HostFromURL(newURL)
		cfg.URL = newURL
		rf.AddOrUpdateRemote(cfg)
		if err := saveRemotes(cwd, rf); err != nil {
			return err
		}
		fmt.Printf("Remote %q URL updated to %s\n", name, newURL)
		if oldHost != newHost && oldHost != "" && newHost != "" {
			fmt.Fprintf(os.Stderr, "warning: host changed (%s → %s); password may need reconfiguring.\n", oldHost, newHost)
		}
		return nil
	},
}

// remoteTestCmd tests connectivity to a remote.
var remoteTestCmd = &cobra.Command{
	Use:   "test <name>",
	Short: "Test connectivity to a remote",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		cfg, err := resolveRemote(cwd, name)
		if err != nil {
			return err
		}
		rfs, err := remote.NewRemoteFS(cfg)
		if err != nil {
			return fmt.Errorf("create remote client: %w", err)
		}
		defer rfs.Close()
		// Test by listing the root directory.
		if _, err := rfs.List("."); err != nil {
			statusFailed("Remote test", err.Error(), "check URL, credentials, and network connectivity")
			return ErrSilent
		}
		statusOK("Remote %q reachable", name)
		return nil
	},
}

// --- helpers ---

// driftDir returns the .drift directory path under cwd.
func driftDir(cwd string) string {
	return filepath.Join(cwd, ".drift")
}

// loadRemotesOrReport loads remotes.json, reporting not-a-repo errors.
func loadRemotesOrReport(cwd string) (*remote.RemotesFile, error) {
	dir := driftDir(cwd)
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("not a drift repository: %w", err)
	}
	rf, err := remote.LoadRemotes(dir)
	if err != nil {
		return nil, fmt.Errorf("load remotes: %w", err)
	}
	return rf, nil
}

// saveRemotes writes remotes.json.
func saveRemotes(cwd string, rf *remote.RemotesFile) error {
	return remote.SaveRemotes(driftDir(cwd), rf)
}

// resolveRemote loads remotes.json + credentials.json and merges the password
// into the RemoteConfig for protocol construction.
func resolveRemote(cwd, name string) (remote.RemoteConfig, error) {
	rf, err := loadRemotesOrReport(cwd)
	if err != nil {
		return remote.RemoteConfig{}, err
	}
	cfg, err := rf.FindRemote(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return remote.RemoteConfig{}, fmt.Errorf("remote %q not found", name)
		}
		return remote.RemoteConfig{}, err
	}
	// Look up password in user-level credentials.json.
	host, err := remote.HostFromURL(cfg.URL)
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("parse remote URL: %w", err)
	}
	cred, err := remote.LoadCredentials()
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("load credentials: %w", err)
	}
	password, err := cred.FindCredential(host, cfg.User)
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("no credential for %s@%s: run 'drift remote add' to configure", cfg.User, host)
	}
	// Stash password in Options so the protocol factory can read it. This is
	// a transient value (never persisted to remotes.json), kept only for the
	// lifetime of this RemoteFS construction.
	if cfg.Options == nil {
		cfg.Options = make(map[string]string)
	}
	cfg.Options["_password"] = password
	return cfg, nil
}

func init() {
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteRemoveCmd)
	remoteCmd.AddCommand(remoteSetURLCmd)
	remoteCmd.AddCommand(remoteTestCmd)
	rootCmd.AddCommand(remoteCmd)
}
