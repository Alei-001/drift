package cmd

import (
	"fmt"

	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/spf13/cobra"
)

// lsRemoteCmd lists references on a remote without downloading data.
var lsRemoteCmd = &cobra.Command{
	Use:   "ls-remote <remote>",
	Short: "List remote references (branches and tags)",
	Long:  "List all branches and tags on a remote without downloading any objects.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		remoteName := args[0]

		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}

		refs, err := porcelain.LsRemote(ctx, cwd, remoteName)
		if err != nil {
			statusFailed("Ls-remote", err.Error(), "check remote configuration and network connectivity")
			return ErrSilent
		}

		if len(refs) == 0 {
			fmt.Println("(no refs on remote)")
			return nil
		}

		for _, r := range refs {
			fmt.Printf("%s\t%s\n", r.Target.FullString(), r.Name)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lsRemoteCmd)
}
