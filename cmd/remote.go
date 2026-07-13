package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Alei-001/drift/internal/remote"
	"github.com/spf13/cobra"
)

// remoteCmd is the parent command for remote management.
var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote storage backends (add, list, remove, rename, set-url, show, test)",
	Long:  "Manage remote storage backends for push/pull. Subcommands: add, list, remove, rename, set-url, show, test.",
}

// remoteListCmd lists all configured remotes.
var remoteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured remotes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		sort.Slice(rf.Remotes, func(i, j int) bool {
			return rf.Remotes[i].Name < rf.Remotes[j].Name
		})
		if globalJSON {
			entries := make([]remoteListEntry, 0, len(rf.Remotes))
			for _, r := range rf.Remotes {
				entries = append(entries, remoteListEntry{
					Name: r.Name,
					Type: r.Type,
					URL:  r.URL,
					User: r.User,
				})
			}
			return outputJSON(JSONEnvelope{
				Command: "remote list",
				Status:  "ok",
				Data:    remoteListData{Remotes: entries},
			})
		}
		if len(rf.Remotes) == 0 {
			fmt.Println("(no remotes configured)")
			return nil
		}
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
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		if !rf.RemoveRemote(name) {
			reportFailed("Remote remove", "remote remove", fmt.Sprintf("remote %q not found", name), "use 'drift remote list' to see configured remotes.")
			return ErrSilent
		}
		if err := saveRemotes(cwd, rf); err != nil {
			return err
		}
		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "remote remove",
				Status:  "ok",
				Data:    remoteRemoveData{Name: name},
			})
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
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		cfg, err := rf.FindRemote(name)
		if err != nil {
			reportFailed("Remote set-url", "remote set-url", fmt.Sprintf("remote %q not found", name), "use 'drift remote list' to see configured remotes.")
			return ErrSilent
		}
		oldHost, _ := remote.HostFromURL(cfg.URL)
		newHost, _ := remote.HostFromURL(newURL)
		cfg.URL = newURL
		rf.AddOrUpdateRemote(cfg)
		if err := saveRemotes(cwd, rf); err != nil {
			return err
		}
		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "remote set-url",
				Status:  "ok",
				Data:    remoteSetURLData{Name: name, URL: newURL},
			})
		}
		fmt.Printf("Remote %q URL updated to %s\n", name, newURL)
		if oldHost != newHost && oldHost != "" && newHost != "" {
			fmt.Fprintf(os.Stderr, "warning: host changed (%s -> %s); password may need reconfiguring.\n", oldHost, newHost)
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
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		cfg, err := resolveRemote(cwd, name)
		if err != nil {
			reportFailed("Remote test", "remote test", "could not resolve remote.", "use 'drift remote list' to see configured remotes, or 'drift remote add' to configure one.")
			return ErrSilent
		}
		rfs, err := remote.NewRemoteFS(cfg)
		if err != nil {
			reportFailed("Remote test", "remote test", "could not create remote client.", "check the remote URL and protocol type.")
			return ErrSilent
		}
		defer rfs.Close()
		if _, err := rfs.List(context.Background(), "."); err != nil {
			reportFailed("Remote test", "remote test", "remote is not reachable.", "check URL, credentials, and network connectivity")
			return ErrSilent
		}
		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "remote test",
				Status:  "ok",
				Data:    remoteTestData{Name: name, Reachable: true},
			})
		}
		statusOK("Remote %q reachable", name)
		return nil
	},
}

// remoteRenameCmd renames a remote.
var remoteRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a remote",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldName, newName := args[0], args[1]
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		cfg, err := rf.FindRemote(oldName)
		if err != nil {
			reportFailed("Remote rename", "remote rename", fmt.Sprintf("remote %q not found", oldName), "use 'drift remote list' to see configured remotes.")
			return ErrSilent
		}
		if _, err := rf.FindRemote(newName); err == nil {
			reportFailed("Remote rename", "remote rename", fmt.Sprintf("remote %q already exists", newName), "use a different name or remove the existing remote first.")
			return ErrSilent
		}
		cfg.Name = newName
		rf.RemoveRemote(oldName)
		rf.AddOrUpdateRemote(cfg)
		if err := saveRemotes(cwd, rf); err != nil {
			return err
		}
		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "remote rename",
				Status:  "ok",
				Data:    remoteRenameData{OldName: oldName, NewName: newName},
			})
		}
		fmt.Printf("Remote %q renamed to %q\n", oldName, newName)
		return nil
	},
}

// remoteShowCmd displays the configuration of a single remote.
var remoteShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a remote's configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		cfg, err := rf.FindRemote(name)
		if err != nil {
			reportFailed("Remote show", "remote show", fmt.Sprintf("remote %q not found", name), "use 'drift remote list' to see configured remotes.")
			return ErrSilent
		}
		opts := make(map[string]string)
		for k, v := range cfg.Options {
			if k == "_password" {
				continue
			}
			opts[k] = v
		}
		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "remote show",
				Status:  "ok",
				Data: remoteShowData{
					Name:    cfg.Name,
					Type:    cfg.Type,
					URL:     cfg.URL,
					User:    cfg.User,
					Options: opts,
				},
			})
		}
		fmt.Printf("  name:     %s\n", cfg.Name)
		fmt.Printf("  type:     %s\n", cfg.Type)
		fmt.Printf("  url:      %s\n", cfg.URL)
		fmt.Printf("  user:     %s\n", cfg.User)
		for k, v := range opts {
			fmt.Printf("  option:   %s=%s\n", k, v)
		}
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
	// Look up password in user-level credentials.json by remote name.
	cred, err := remote.LoadCredentials()
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("load credentials: %w", err)
	}
	password, err := cred.FindCredential(name)
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("no credential for remote %q: run 'drift remote add' to configure", name)
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
	remoteCmd.AddCommand(remoteRenameCmd)
	remoteCmd.AddCommand(remoteShowCmd)
	rootCmd.AddCommand(remoteCmd)
}

// --- JSON data types for remote subcommands ---

// remoteListData is the JSON payload for a successful drift remote list.
type remoteListData struct {
	Remotes []remoteListEntry `json:"remotes"`
}

// remoteListEntry is a single remote in the remote list JSON output.
type remoteListEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url"`
	User string `json:"user"`
}

// remoteRemoveData is the JSON payload for a successful drift remote remove.
type remoteRemoveData struct {
	Name string `json:"name"`
}

// remoteSetURLData is the JSON payload for a successful drift remote set-url.
type remoteSetURLData struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// remoteTestData is the JSON payload for a successful drift remote test.
type remoteTestData struct {
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
}

// remoteRenameData is the JSON payload for a successful drift remote rename.
type remoteRenameData struct {
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
}

// remoteShowData is the JSON payload for a successful drift remote show.
type remoteShowData struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	User    string            `json:"user"`
	Options map[string]string `json:"options"`
}
