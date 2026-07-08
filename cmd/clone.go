package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/Alei-001/drift/internal/porcelain"
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

		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
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
			statusFailed("Clone", err.Error(), "check the remote URL and network connectivity")
			return ErrSilent
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
	rootCmd.AddCommand(cloneCmd)
}
