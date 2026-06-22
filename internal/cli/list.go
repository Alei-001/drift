package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show version history with branch information",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !sharedStore.IsInitialized() {
			return fmt.Errorf("not a drift project (run 'drift init')")
		}
		// P1-#9: iterate branch refs and walk each chain, instead of
		// scanning and deserializing every commit file on disk.
		refs, err := sharedStore.ListRefs()
		if err != nil {
			return fmt.Errorf("failed to list refs: %w", err)
		}

		type commitInfo struct {
			ID        string
			Branch    string
			Timestamp string
			Message   string
			ts        int64
		}
		var all []commitInfo

		for branchName := range refs {
			if branchName == "HEAD" {
				continue
			}
			commits, err := sharedStore.ListBranchCommits(branchName)
			if err != nil {
				continue
			}
			for _, c := range commits {
				all = append(all, commitInfo{
					ID:        c.ID,
					Branch:    c.Branch,
					Timestamp: c.Timestamp.Format("2006-01-02 15:04"),
					Message:   c.Message,
					ts:        c.Timestamp.UnixMilli(),
				})
			}
		}

		if len(all) == 0 {
			fmt.Println("No versions yet")
			return nil
		}

		sort.Slice(all, func(i, j int) bool {
			return all[i].ts > all[j].ts
		})

		fmt.Println("Version history:")
		fmt.Println()
		for _, c := range all {
			fmt.Printf("  %s  [%s]  %s  %s\n", c.ID, c.Branch, c.Timestamp, c.Message)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
