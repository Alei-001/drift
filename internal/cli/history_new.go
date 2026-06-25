package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

// NewHistoryCmd creates the history subcommand.
func NewHistoryCmd(application *apppkg.App) *cobra.Command {
	var (
		allBranches bool
		oneline     bool
		number      int
		porcelain   bool
	)

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show commit history",
		RunE: func(cmd *cobra.Command, args []string) error {
			commits, err := application.History(apppkg.HistoryOptions{
				All:   allBranches,
				Limit: number,
			})
			if err != nil {
				return err
			}

			namesByHash := application.NamesByHash()

			if porcelain {
				formatCommitsPorcelain(commits, namesByHash)
			} else {
				formatCommits(commits, namesByHash, oneline)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&allBranches, "all", false, "Show all branches")
	cmd.Flags().BoolVar(&oneline, "oneline", false, "Show one line per commit")
	cmd.Flags().IntVarP(&number, "number", "n", 0, "Limit number of commits (0 = all)")
	cmd.Flags().BoolVar(&porcelain, "porcelain", false, "Machine-readable output")

	return cmd
}

// formatCommits displays commits in human-readable format.
func formatCommits(commits []*core.Commit, namesByHash map[string][]string, oneline bool) {
	for _, c := range commits {
		names := namesByHash[c.Hash]
		var nameStr string
		if len(names) > 0 {
			nameStr = fmt.Sprintf(" (%s)", names[0])
		}

		if oneline {
			fmt.Printf("%s%s %s\n", c.ID[:8], nameStr, c.Message)
		} else {
			fmt.Printf("commit %s%s\n", c.ID, nameStr)
			fmt.Printf("Author: %s\n", c.Author.Name)
			fmt.Printf("Date: %s\n\n", c.Timestamp.Format("Mon Jan 2 15:04:05 2006 -0700"))
			fmt.Printf("    %s\n\n", c.Message)
		}
	}
}

// formatCommitsPorcelain displays commits in machine-readable format.
func formatCommitsPorcelain(commits []*core.Commit, namesByHash map[string][]string) {
	for _, c := range commits {
		names := namesByHash[c.Hash]
		fmt.Printf("commit %s\n", c.ID)
		fmt.Printf("author %s\n", c.Author.Name)
		fmt.Printf("date %s\n", c.Timestamp.Format("Mon Jan 2 15:04:05 2006 -0700"))
		fmt.Printf("message %s\n", c.Message)
		for _, name := range names {
			fmt.Printf("name %s\n", name)
		}
		fmt.Println()
	}
}
