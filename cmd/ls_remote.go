package cmd

import (
	"github.com/Alei-001/drift/internal/sync"
	"fmt"

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
		cwd, err := getCwd()
		if err != nil {
			return err
		}

		refs, err := sync.LsRemote(ctx, cwd, remoteName)
		if err != nil {
			reportFailed("Ls-remote", "ls-remote", "could not list remote refs.", "check remote configuration and network connectivity", err)
			return silentWrap(err)
		}

		if globalJSON {
			refList := make([]lsRemoteRef, 0, len(refs))
			for _, r := range refs {
				refList = append(refList, lsRemoteRef{
					Name:   r.Name,
					Target: r.Target.FullString(),
				})
			}
			return outputJSON(JSONEnvelope{
				Command: "ls-remote",
				Status:  "ok",
				Data:    lsRemoteData{Refs: refList},
			})
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

// lsRemoteData is the JSON payload for a successful drift ls-remote.
type lsRemoteData struct {
	Refs []lsRemoteRef `json:"refs"`
}

// lsRemoteRef is a single reference entry in the ls-remote JSON output.
type lsRemoteRef struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}
