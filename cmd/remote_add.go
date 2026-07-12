package cmd

import (
	"bufio"
	"fmt"
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
		cwd, err := getCwd(cmd)
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

		// Parse --option key=value into Options map.
		optMap := make(map[string]string)
		for _, o := range options {
			parts := strings.SplitN(o, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --option %q (expected key=value)", o)
			}
			optMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}

		// Interactive mode: prompt for missing url or user.
		interactive := url == "" || user == ""
		if interactive {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("interactive mode requires a terminal; provide --url and --user via flags")
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
					return fmt.Errorf("URL is required")
				}
			}
			if user == "" {
				fmt.Print("Username: ")
				line, _ = reader.ReadString('\n')
				user = strings.TrimSpace(line)
				if user == "" {
					return fmt.Errorf("username is required")
				}
			}
			if password == "" {
				fmt.Print("Password: ")
				passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println() // newline after password prompt
				if err != nil {
					return fmt.Errorf("read password: %w", err)
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
		if password != "" && !noSavePassword {
			host, err := remote.HostFromURL(url)
			if err != nil {
				return fmt.Errorf("parse URL for credentials: %w", err)
			}
			cred, err := remote.LoadCredentials()
			if err != nil {
				return fmt.Errorf("load credentials: %w", err)
			}
			cred.AddOrUpdateCredential(remote.Credential{
				Remote:   name,
				Host:     host,
				User:     user,
				Password: password,
			})
			if err := remote.SaveCredentials(cred); err != nil {
				return fmt.Errorf("save credentials: %w", err)
			}
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

		if password != "" && !noSavePassword {
			fmt.Printf("Remote %q added (credentials saved to user-level config).\n", name)
		} else if password != "" && noSavePassword {
			fmt.Printf("Remote %q added (password NOT saved, will prompt on next push/pull).\n", name)
		} else {
			fmt.Printf("Remote %q added.\n", name)
		}

		// Offer to test connection.
		if interactive {
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
				if _, err := rfs.List("."); err != nil {
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
	remoteAddCmd.Flags().Bool("no-save-password", false, "do not save password (prompt on each push/pull)")
	remoteAddCmd.Flags().StringArray("option", nil, "protocol-specific field (key=value, repeatable)")
}
