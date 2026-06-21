package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show version history",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		store := storage.NewStore(dir)
		if !store.IsInitialized() {
			return fmt.Errorf("not a drift project (run 'drift init')")
		}

		commits, err := store.ListCommits()
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
			fmt.Printf("  %s  %s  %s\n", c.ID, c.Timestamp.Format("2006-01-02 15:04"), c.Message)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
