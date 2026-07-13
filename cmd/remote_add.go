package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Alei-001/drift/internal/remote"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// remoteAddCmd adds a new remote, with interactive prompting for missing fields.
var remoteAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new remote (interactive if fields missing)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cwd, err := getCwd()
		if err != nil {
			return err
		}

		// Read flags.
		remoteType, _ := cmd.Flags().GetString("type")
		url, _ := cmd.Flags().GetString("url")
		user, _ := cmd.Flags().GetString("user")
		password, _ := cmd.Flags().GetString("password")
		noSavePassword, _ := cmd.Flags().GetBool("no-save-password")
		options, _ := cmd.Flags().GetStringArray("option")
		passwordStdin, _ := cmd.Flags().GetBool("password-stdin")

		// When --password-stdin is set, read the password from os.Stdin
		// (trimming trailing whitespace). This supports automation scripts
		// and pipes; --password-stdin takes precedence over --password.
		if passwordStdin {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read password from stdin: %w", err)
			}
			password = strings.TrimSpace(string(data))
		} else if password != "" {
			// --password is visible in process listings (ps, Task Manager,
			// /proc/<pid>/cmdline) to any local user. Warn so users opt
			// into --password-stdin or interactive prompting instead.
			fmt.Fprintln(os.Stderr, "warning: --password is visible in process listings; prefer --password-stdin for security")
		}

		// Parse --option key=value into Options map.
		optMap := make(map[string]string)
		for _, o := range options {
			parts := strings.SplitN(o, "=", 2)
			if len(parts) != 2 {
				reportFailed("Remote add", "remote add", fmt.Sprintf("invalid --option %q (expected key=value).", o), "use --option key=value, repeatable.")
				return ErrSilent
			}
			optMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}

		// Interactive mode: prompt for missing url or user.
		interactive := url == "" || user == ""
		if interactive {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				reportFailed("Remote add", "remote add", "interactive mode requires a terminal.", "provide --url and --user via flags.")
				return ErrSilent
			}
			reader := bufio.NewReader(os.Stdin)
			if remoteType == "" {
				remoteType = "webdav"
			}
			fmt.Printf("Protocol (webdav/smb) [%s]: ", remoteType)
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			if line != "" {
				remoteType = line
			}
			if url == "" {
				fmt.Print("URL: ")
				line, _ = reader.ReadString('\n')
				url = strings.TrimSpace(line)
				if url == "" {
					reportFailed("Remote add", "remote add", "URL is required.", "")
					return ErrSilent
				}
			}
			if user == "" {
				fmt.Print("Username: ")
				line, _ = reader.ReadString('\n')
				user = strings.TrimSpace(line)
				if user == "" {
					reportFailed("Remote add", "remote add", "username is required.", "")
					return ErrSilent
				}
			}
			if password == "" {
				fmt.Print("Password: ")
				passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println() // newline after password prompt
				if err != nil {
					reportFailed("Remote add", "remote add", fmt.Sprintf("read password: %s.", err), "")
					return ErrSilent
				}
				password = string(passBytes)
			}
			// For SMB, prompt for domain if not provided.
			if remoteType == "smb" && optMap["domain"] == "" {
				fmt.Print("Domain (optional, press Enter to skip): ")
				line, _ := reader.ReadString('\n')
				domain := strings.TrimSpace(line)
				if domain != "" {
					optMap["domain"] = domain
				}
			}
			// Ask whether to save password.
			if !noSavePassword {
				fmt.Print("Save password to user-level credentials.json? [Y/n]: ")
				line, _ := reader.ReadString('\n')
				answer := strings.TrimSpace(strings.ToLower(line))
				if answer != "n" && answer != "no" {
					noSavePassword = false
				} else {
					noSavePassword = true
				}
			}
		}

		if remoteType == "" {
			remoteType = "webdav"
		}

		cfg := remote.RemoteConfig{
			Name:    name,
			Type:    remoteType,
			URL:     url,
			User:    user,
			Options: optMap,
		}

		// Save password to user-level credentials.json FIRST, so that if
		// credential saving fails, the remote definition is not left
		// orphaned (without credentials, push/pull would fail).
		credentialsSaved := false
		if password != "" && !noSavePassword {
			host, err := remote.HostFromURL(url)
			if err != nil {
				reportFailed("Remote add", "remote add", "could not parse URL for credentials.", "check that --url is a valid remote URL.")
				return ErrSilent
			}
			cred, err := remote.LoadCredentials()
			if err != nil {
				reportFailed("Remote add", "remote add", "could not load existing credentials.", "")
				return ErrSilent
			}
			cred.AddOrUpdateCredential(remote.Credential{
				Remote:   name,
				Host:     host,
				User:     user,
				Password: password,
			})
			if err := remote.SaveCredentials(cred); err != nil {
				reportFailed("Remote add", "remote add", "could not save credentials.", "check that the user-level config directory is writable.")
				return ErrSilent
			}
			credentialsSaved = true
		}

		// Now save the remote definition to remotes.json.
		rf, err := loadRemotesOrReport(cwd)
		if err != nil {
			return err
		}
		if _, err := rf.FindRemote(name); err == nil {
			reportFailed("Remote add", "remote add", fmt.Sprintf("remote %q already exists", name), "use 'drift remote set-url' to update the URL of an existing remote.")
			return ErrSilent
		}
		rf.AddOrUpdateRemote(cfg)
		if err := saveRemotes(cwd, rf); err != nil {
			return err
		}

		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "remote add",
				Status:  "ok",
				Data: remoteAddData{
					Name:             cfg.Name,
					Type:             cfg.Type,
					URL:              cfg.URL,
					User:             cfg.User,
					CredentialsSaved: credentialsSaved,
				},
			})
		}

		if password != "" && !noSavePassword {
			fmt.Printf("Remote %q added (credentials saved to user-level config).\n", name)
		} else if password != "" && noSavePassword {
			fmt.Printf("Remote %q added (password NOT saved, will prompt on next push/pull).\n", name)
		} else {
			fmt.Printf("Remote %q added.\n", name)
		}

		// Offer to test connection (interactive mode only, never in JSON mode).
		if interactive && !globalJSON {
			fmt.Print("Test connection now? [Y/n]: ")
			reader := bufio.NewReader(os.Stdin)
			line, _ := reader.ReadString('\n')
			answer := strings.TrimSpace(strings.ToLower(line))
			if answer != "n" && answer != "no" {
				// Stash password for the test.
				if cfg.Options == nil {
					cfg.Options = make(map[string]string)
				}
				cfg.Options["_password"] = password
				rfs, err := remote.NewRemoteFS(cfg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					return ErrSilent
				}
				defer rfs.Close()
				if _, err := rfs.List(context.Background(), "."); err != nil {
					fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
					return ErrSilent
				}
				fmt.Println("✓ Connected.")
			}
		}
		return nil
	},
}

func init() {
	remoteAddCmd.Flags().String("type", "webdav", "protocol type (webdav|smb)")
	remoteAddCmd.Flags().String("url", "", "remote URL")
	remoteAddCmd.Flags().String("user", "", "username")
	remoteAddCmd.Flags().String("password", "", "password (saved to user-level credentials.json)")
	remoteAddCmd.Flags().Bool("password-stdin", false, "read password from standard input (for automation/scripts)")
	remoteAddCmd.Flags().Bool("no-save-password", false, "do not save password (prompt on each push/pull)")
	remoteAddCmd.Flags().StringArray("option", nil, "protocol-specific field (key=value, repeatable)")
}

// remoteAddData is the JSON payload for a successful drift remote add.
type remoteAddData struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	URL              string `json:"url"`
	User             string `json:"user"`
	CredentialsSaved bool   `json:"credentials_saved"`
}
