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
		commits, err := sharedStore.ListCommits()
		if err != nil {
			return fmt.Errorf("failed to list commits: %w", err)
		}

		if len(commits) == 0 {
			fmt.Println("No versions yet")
			return nil
		}

		sort.Slice(commits, func(i, j int) bool {
			return commits[i].Timestamp.After(commits[j].Timestamp)
		})

		fmt.Println("Version history:")
		fmt.Println()
		for _, c := range commits {
			fmt.Printf("  %s  [%s]  %s  %s\n", c.ID, c.Branch, c.Timestamp.Format("2006-01-02 15:04"), c.Message)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
