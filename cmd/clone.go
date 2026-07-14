package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/spf13/cobra"
)

// cloneCmd downloads a remote drift repository into a new directory.
var cloneCmd = &cobra.Command{
	Use:   "clone <url> [<directory>]",
	Short: "Clone a remote drift repository",
	Long: `Clone a remote drift repository into a new directory.

This is equivalent to: drift init <dir> && drift remote add origin <url> && drift pull origin --all

If no directory is specified, the last component of the URL path is used.
The current branch after clone is the remote's HEAD branch.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]
		dir := ""
		if len(args) > 1 {
			dir = args[1]
		}
		remoteType, _ := cmd.Flags().GetString("type")
		user, _ := cmd.Flags().GetString("user")
		password, _ := cmd.Flags().GetString("password")
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

		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}

		result, err := porcelain.CloneRemote(ctx, porcelain.CloneOptions{
			TargetDir:  dir,
			WorkDir:    cwd,
			RemoteURL:  url,
			RemoteType: remoteType,
			User:       user,
			Password:   password,
		})
		if err != nil {
			reportFailed("Clone", "clone", "clone failed.", "check the remote URL and network connectivity", err)
			return silentWrap(err)
		}

		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "clone",
				Status:  "ok",
				Data: cloneData{
					Dir:              result.Dir,
					RemoteURL:        url,
					Branch:           result.Branch,
					Snapshots:        result.Snapshots,
					Branches:         result.Branches,
					Tags:             result.Tags,
					CredentialsSaved: result.CredentialsSaved,
				},
			})
		}

		statusOK("Cloned into '%s'", result.Dir)
		fmt.Printf("  snapshots:  %d\n", result.Snapshots)
		fmt.Printf("  branches:   %d\n", result.Branches)
		fmt.Printf("  tags:       %d\n", result.Tags)
		fmt.Printf("  branch:     %s\n", result.Branch)
		if !result.CredentialsSaved {
			fmt.Fprintln(os.Stderr, "  warning: password was not saved; provide credentials on next push/pull.")
		}
		return nil
	},
}

func init() {
	cloneCmd.Flags().String("type", "webdav", "protocol type (webdav|smb)")
	cloneCmd.Flags().String("user", "", "remote username")
	cloneCmd.Flags().String("password", "", "remote password")
	cloneCmd.Flags().Bool("password-stdin", false, "read password from standard input (for automation/scripts)")
	rootCmd.AddCommand(cloneCmd)
}

// cloneData is the JSON payload for a successful drift clone.
type cloneData struct {
	Dir              string `json:"dir"`
	RemoteURL        string `json:"remote_url"`
	Branch           string `json:"branch"`
	Snapshots        int    `json:"snapshots"`
	Branches         int    `json:"branches"`
	Tags             int    `json:"tags"`
	CredentialsSaved bool   `json:"credentials_saved"`
}
