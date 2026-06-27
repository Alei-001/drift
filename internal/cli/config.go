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

// NewConfigCmd creates the config subcommand.
func NewConfigCmd(application *apppkg.App) *cobra.Command {
	var (
		global bool
		unset  string
	)

	cmd := &cobra.Command{
		Use:   "config [<key> [<value>]]",
		Short: "Get or set configuration options",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := apppkg.LocalScope
			if global {
				scope = apppkg.GlobalScope
			}

			// Unset config
			if unset != "" {
				if err := application.ConfigUnset(scope, unset); err != nil {
					return err
				}
				fmt.Printf("Unset %s\n", colorYellow(unset))
				return nil
			}

			// Get config (single arg, not "list")
			if len(args) == 1 && args[0] != "list" {
				value, err := application.ConfigGet(scope, args[0])
				if err != nil {
					return err
				}
				fmt.Println(value)
				return nil
			}

			// Set config
			if len(args) >= 2 {
				key := args[0]
				value := args[1]
				if err := application.ConfigSet(scope, key, value); err != nil {
					return err
				}
				fmt.Printf("Set %s = %s\n", colorYellow(key), value)
				return nil
			}

			// List all config: drift config, drift config list, drift config --global
			entries, err := application.ConfigList(scope)
			if err != nil {
				return err
			}

			formatConfigList(entries)
			return nil
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "Use global config")
	cmd.Flags().StringVar(&unset, "unset", "", "Unset a config option")

	remoteCmd := &cobra.Command{
		Use:   "remote [<url>]",
		Short: "Configure remote repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			show, _ := cmd.Flags().GetBool("show")
			unsetRemote, _ := cmd.Flags().GetBool("unset")
			protocol, _ := cmd.Flags().GetString("protocol")

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

			if unsetRemote {
				if err := application.SyncRemoteUnset(); err != nil {
					return err
				}
				fmt.Println(colorGreen("Remote configuration removed"))
				return nil
			}

			if protocol != "" {
				return setRemoteWithProtocol(application, protocol, args)
			}

			if len(args) > 0 {
				return setRemoteFromURL(application, args[0])
			}

			return cmd.Help()
		},
	}

	remoteCmd.Flags().Bool("show", false, "Show current remote configuration")
	remoteCmd.Flags().Bool("unset", false, "Remove remote configuration")
	remoteCmd.Flags().String("protocol", "", "Protocol (webdav, sftp, smb, local)")

	cmd.AddCommand(remoteCmd)

	return cmd
}

func formatConfigList(entries []apppkg.ConfigEntry) {
	sectionOrder := []string{"core", "sync", "user", "remote"}
	bySection := make(map[string][]apppkg.ConfigEntry)
	for _, e := range entries {
		sec, _, _ := strings.Cut(e.Key, ".")
		bySection[sec] = append(bySection[sec], e)
	}

	for _, sec := range sectionOrder {
		group, ok := bySection[sec]
		if !ok {
			continue
		}
		keyWidth := 0
		for _, e := range group {
			_, name, _ := strings.Cut(e.Key, ".")
			if len(name) > keyWidth {
				keyWidth = len(name)
			}
		}
		fmt.Printf("[%s]\n", colorCyan(sec))
		for _, e := range group {
			_, name, _ := strings.Cut(e.Key, ".")
			fmt.Printf("  %-*s = %s\n", keyWidth, colorYellow(name), e.Value)
		}
	}
}

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

	default:
		return fmt.Errorf("unsupported protocol: %s", protocol)
	}

	if opts.Username == "" {
		opts.Username = promptInput("Username")
	}

	if opts.Password == "" {
		opts.Password = promptPassword("Password")
	}

	return application.SyncRemoteSet(protocol, opts)
}

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
		return fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}

	return setRemoteWithProtocol(application, protocol, []string{rawURL})
}

func promptInput(prompt string) string {
	fmt.Print(prompt + ": ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func promptPassword(prompt string) string {
	fmt.Print(prompt + ": ")
	reader := bufio.NewReader(os.Stdin)

	if oldState, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer term.Restore(int(os.Stdin.Fd()), oldState)
		bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return ""
		}
		return string(bytes)
	}

	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}
