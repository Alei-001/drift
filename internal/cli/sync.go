package cli

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// NewSyncCmd creates the sync subcommand.
func NewSyncCmd(application *apppkg.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync <subcommand>",
		Short: "Synchronize with remote repositories",
	}

	// sync remote
	remoteCmd := &cobra.Command{
		Use:   "remote [<url>]",
		Short: "Configure remote repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			show, _ := cmd.Flags().GetBool("show")
			unset, _ := cmd.Flags().GetBool("unset")
			protocol, _ := cmd.Flags().GetString("protocol")

			// Show current remote
			if show {
				info, err := application.SyncRemoteShow()
				if err != nil {
					return err
				}
				fmt.Printf("Protocol: %s\n", info.Protocol)
				if info.Host != "" {
					fmt.Printf("Host: %s\n", info.Host)
				}
				if info.Port != 0 {
					fmt.Printf("Port: %d\n", info.Port)
				}
				if info.Path != "" {
					fmt.Printf("Path: %s\n", info.Path)
				}
				if info.Username != "" {
					fmt.Printf("Username: %s\n", info.Username)
				}
				if info.TLS {
					fmt.Println("TLS: enabled")
				}
				if info.Share != "" {
					fmt.Printf("Share: %s\n", info.Share)
				}
				return nil
			}

			// Unset remote
			if unset {
				if err := application.SyncRemoteUnset(); err != nil {
					return err
				}
				fmt.Println("Remote configuration removed")
				return nil
			}

			// Set remote with explicit protocol
			if protocol != "" {
				return setRemoteWithProtocol(application, protocol, args)
			}

			// Auto-detect from URL
			if len(args) > 0 {
				return setRemoteFromURL(application, args[0])
			}

			return cmd.Help()
		},
	}

	remoteCmd.Flags().Bool("show", false, "Show current remote configuration")
	remoteCmd.Flags().Bool("unset", false, "Remove remote configuration")
	remoteCmd.Flags().String("protocol", "", "Protocol (webdav, sftp, smb, local)")

	// sync enable
	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.SyncEnable(); err != nil {
				return err
			}
			fmt.Println("Sync enabled")
			return nil
		},
	}

	// sync disable
	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.SyncDisable(); err != nil {
				return err
			}
			fmt.Println("Sync disabled")
			return nil
		},
	}

	// sync now
	nowCmd := &cobra.Command{
		Use:   "now",
		Short: "Sync immediately",
		RunE: func(cmd *cobra.Command, args []string) error {
			stats, err := application.SyncNow()
			if err != nil {
				return err
			}
			fmt.Println("Sync completed")
			if stats.Pushed > 0 {
				fmt.Printf("  Pushed: %d file(s)\n", stats.Pushed)
			}
			if stats.Pulled > 0 {
				fmt.Printf("  Pulled: %d file(s)\n", stats.Pulled)
			}
			if stats.RemoteDeleted > 0 {
				fmt.Printf("  Deleted on remote: %d file(s)\n", stats.RemoteDeleted)
			}
			if stats.LocalDeleted > 0 {
				fmt.Printf("  Deleted locally: %d file(s)\n", stats.LocalDeleted)
			}
			if stats.Conflicts > 0 {
				fmt.Printf("  Conflicts: %d file(s)\n", stats.Conflicts)
			}
			return nil
		},
	}

	// sync status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := application.SyncStatus()
			if err != nil {
				return err
			}

			if status.Enabled {
				fmt.Println("Sync is enabled")
			} else {
				fmt.Println("Sync is disabled")
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

	cmd.AddCommand(remoteCmd, enableCmd, disableCmd, nowCmd, statusCmd)

	return cmd
}

// setRemoteWithProtocol sets remote with explicit protocol.
func setRemoteWithProtocol(application *apppkg.App, protocol string, args []string) error {
	opts := apppkg.SyncRemoteOptions{}

	switch protocol {
	case "webdav":
		if len(args) == 0 {
			return fmt.Errorf("URL required for webdav protocol")
		}
		u, err := url.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}
		opts.Host = u.Hostname()
		port := u.Port()
		if port != "" {
			p, err := strconv.Atoi(port)
			if err != nil {
				return fmt.Errorf("invalid port: %w", err)
			}
			opts.Port = p
		}
		opts.Path = u.Path
		opts.Username = u.User.Username()
		if p, ok := u.User.Password(); ok {
			opts.Password = p
		}
		opts.TLS = u.Scheme == "https"

	case "sftp":
		if len(args) == 0 {
			return fmt.Errorf("URL required for sftp protocol")
		}
		u, err := url.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}
		opts.Host = u.Hostname()
		port := u.Port()
		if port != "" {
			p, err := strconv.Atoi(port)
			if err != nil {
				return fmt.Errorf("invalid port: %w", err)
			}
			opts.Port = p
		}
		opts.Username = u.User.Username()
		if p, ok := u.User.Password(); ok {
			opts.Password = p
		}

	case "smb":
		if len(args) == 0 {
			return fmt.Errorf("path required for smb protocol")
		}
		opts.Path = args[0]
		if len(args) > 1 {
			opts.Share = args[1]
		}

	case "local":
		if len(args) == 0 {
			return fmt.Errorf("path required for local protocol")
		}
		opts.Path = args[0]

	default:
		return fmt.Errorf("unsupported protocol: %s", protocol)
	}

	// Prompt for username if not provided
	if opts.Username == "" && protocol != "local" {
		opts.Username = promptInput("Username")
	}

	// Prompt for password if not provided
	if opts.Password == "" && protocol != "local" {
		opts.Password = promptPassword("Password")
	}

	return application.SyncRemoteSet(protocol, opts)
}

// setRemoteFromURL auto-detects protocol from URL.
func setRemoteFromURL(application *apppkg.App, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	var protocol string
	switch u.Scheme {
	case "http", "https":
		protocol = "webdav"
	case "sftp":
		protocol = "sftp"
	default:
		// Assume local path
		return setRemoteWithProtocol(application, "local", []string{rawURL})
	}

	return setRemoteWithProtocol(application, protocol, []string{rawURL})
}

// promptInput prompts the user for input.
func promptInput(prompt string) string {
	fmt.Print(prompt + ": ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// promptPassword prompts the user for password with hidden input.
func promptPassword(prompt string) string {
	fmt.Print(prompt + ": ")
	reader := bufio.NewReader(os.Stdin)

	// Try to disable echo using golang.org/x/term.
	if oldState, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer term.Restore(int(os.Stdin.Fd()), oldState)
		bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return ""
		}
		return string(bytes)
	}

	// Fallback: read normally (non-TTY environment).
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}
