package cli

import (
	"fmt"
	"strings"

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
		Use:   "log [<branch>]",
		Short: "Show commit history",
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := ""
			if len(args) > 0 {
				branch = args[0]
			}
			commits, err := application.History(apppkg.HistoryOptions{
				Branch: branch,
				All:    allBranches,
				Limit:  number,
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
	if oneline {
		const versionWidth = 8
		const sep = "    "

		msgWidth := 20
		for _, c := range commits {
			if len(c.Message) > msgWidth {
				msgWidth = len(c.Message)
			}
		}
		if msgWidth > 60 {
			msgWidth = 60
		}

		fmt.Printf("%s%s%s%s%s\n",
			colorCyan(fmt.Sprintf("%-*s", versionWidth, "VERSION")),
			sep,
			colorCyan(fmt.Sprintf("%-*s", msgWidth, "MESSAGE")),
			sep,
			colorCyan("TAG"))
		for _, c := range commits {
			id := c.ID
			if len(id) > 8 {
				id = id[:8]
			}
			tags := tagsByHash[c.Hash]
			tag := strings.Join(tags, ", ")
			msg := c.Message
			if len(msg) > msgWidth {
				msg = msg[:msgWidth-3] + "..."
			}
			fmt.Printf("%s%s%s%s%s\n",
				colorYellow(fmt.Sprintf("%-*s", versionWidth, id)),
				sep,
				fmt.Sprintf("%-*s", msgWidth, msg),
				sep,
				colorGreen(tag))
		}
		return
	}

	for _, c := range commits {
		tags := tagsByHash[c.Hash]
		fmt.Printf("commit %s\n", colorYellow(c.ID))
		if len(tags) > 0 {
			fmt.Printf("Tags:    %s\n", colorGreen(strings.Join(tags, ", ")))
		}
		if c.Author.Email != "" {
			fmt.Printf("Author:  %s <%s>\n", c.Author.Name, c.Author.Email)
		} else {
			fmt.Printf("Author:  %s\n", c.Author.Name)
		}
		fmt.Printf("Date:    %s\n", c.Timestamp.Format("Mon Jan 2 15:04:05 2006 -0700"))
		fmt.Printf("Message: %s\n", c.Message)
		fmt.Println()
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
