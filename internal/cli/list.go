package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list [branch]",
	Short: "Show version history with branch information",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !sharedStore.IsInitialized() {
			return fmt.Errorf("not a drift project (run 'drift init')")
		}

		type commitInfo struct {
			ID        string
			Branch    string
			Timestamp string
			Message   string
			ts        int64
		}
		var all []commitInfo
		seen := make(map[string]bool)

		addBranch := func(branchName string) error {
			commits, err := sharedStore.ListBranchCommits(branchName)
			if err != nil {
				return err
			}
			for _, c := range commits {
				if seen[c.Hash] {
					continue
				}
				seen[c.Hash] = true
				all = append(all, commitInfo{
					ID:        c.ID,
					Branch:    c.Branch,
					Timestamp: c.Timestamp.Format("2006-01-02 15:04"),
					Message:   c.Message,
					ts:        c.Timestamp.UnixMilli(),
				})
			}
			return nil
		}

		if len(args) == 1 {
			branchName := args[0]
			if _, err := sharedStore.GetRef(branchName); err != nil {
				return fmt.Errorf("branch not found: %s", branchName)
			}
			if err := addBranch(branchName); err != nil {
				return err
			}
		} else {
			refs, err := sharedStore.ListRefs()
			if err != nil {
				return fmt.Errorf("failed to list refs: %w", err)
			}
			for branchName := range refs {
				if branchName == "HEAD" {
					continue
				}
				_ = addBranch(branchName)
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
