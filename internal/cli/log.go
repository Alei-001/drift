package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

// NewLogCmd creates the log subcommand (commit history).
func NewLogCmd(application *apppkg.App) *cobra.Command {
	var (
		allBranches bool
		oneline     bool
		number      int
		porcelain   bool
	)

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show commit history",
		RunE: func(cmd *cobra.Command, args []string) error {
			commits, err := application.History(apppkg.HistoryOptions{
				All:   allBranches,
				Limit: number,
			})
			if err != nil {
				return err
			}

			tagsByHash := application.TagsByHash()

			if porcelain {
				formatCommitsPorcelain(commits, tagsByHash)
			} else {
				formatCommits(commits, tagsByHash, oneline)
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
func formatCommits(commits []*core.Commit, tagsByHash map[string][]string, oneline bool) {
	for _, c := range commits {
		tags := tagsByHash[c.Hash]
		var tagStr string
		if len(tags) > 0 {
			tagStr = fmt.Sprintf(" (%s)", tags[0])
		}

		id := c.ID
		if len(id) > 8 {
			id = id[:8]
		}

		if oneline {
			fmt.Printf("%s%s %s\n", id, tagStr, c.Message)
		} else {
			fmt.Printf("commit %s%s\n", c.ID, tagStr)
			if c.Author.Email != "" {
				fmt.Printf("Author: %s <%s>\n", c.Author.Name, c.Author.Email)
			} else {
				fmt.Printf("Author: %s\n", c.Author.Name)
			}
			fmt.Printf("Date: %s\n", c.Timestamp.Format("Mon Jan 2 15:04:05 2006 -0700"))
			fmt.Printf("Message: %s\n", c.Message)
		}
	}
}

// formatCommitsPorcelain displays commits in machine-readable format.
func formatCommitsPorcelain(commits []*core.Commit, tagsByHash map[string][]string) {
	for _, c := range commits {
		tags := tagsByHash[c.Hash]
		fmt.Printf("commit %s\n", c.ID)
		if c.Author.Email != "" {
			fmt.Printf("author %s <%s>\n", c.Author.Name, c.Author.Email)
		} else {
			fmt.Printf("author %s\n", c.Author.Name)
		}
		fmt.Printf("date %s\n", c.Timestamp.Format("Mon Jan 2 15:04:05 2006 -0700"))
		fmt.Printf("message %s\n", c.Message)
		for _, tag := range tags {
			fmt.Printf("tag %s\n", tag)
		}
		fmt.Println()
	}
}
