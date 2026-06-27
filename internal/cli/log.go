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
			formatCommits(commits, tagsByHash, oneline, allBranches)
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
func formatCommits(commits []*core.Commit, tagsByHash map[string][]string, oneline, allBranches bool) {
	if oneline {
		const maxCol = 40
		const sep = "    "
		const colSep = ""

		cw := newColWidths(4) // version, branch(/msg), msg, tag

		cw.feed(0, "VERSION")
		if allBranches {
			cw.feed(1, "BRANCH")
			cw.feed(2, "MESSAGE")
		} else {
			cw.feed(1, "MESSAGE")
		}
		cw.feed(3, "TAG")

		for _, c := range commits {
			id := c.ID
			if len(id) > 8 {
				id = id[:8]
			}
			cw.feed(0, id)
			if allBranches {
				cw.feed(1, c.Branch)
				cw.feed(2, truncateByWidth(c.Message, maxCol))
			} else {
				cw.feed(1, truncateByWidth(c.Message, maxCol))
			}
			cw.feed(3, strings.Join(tagsByHash[c.Hash], ", "))
		}

		cw.capAll(maxCol)

		wVer := cw.v[0]
		wBch := cw.v[1]
		wMsg := cw.v[1]
		if allBranches {
			wMsg = cw.v[2]
		}
		wTag := cw.v[3]

		hdrs := []interface{}{
			colorCyan(fmt.Sprintf("%-*s", wVer, "VERSION")),
		}
		if allBranches {
			hdrs = append(hdrs,
				colorCyan(fmt.Sprintf("%-*s", wBch, "BRANCH")),
				colorCyan(fmt.Sprintf("%-*s", wMsg, "MESSAGE")))
		} else {
			hdrs = append(hdrs, colorCyan(fmt.Sprintf("%-*s", wMsg, "MESSAGE")))
		}
		hdrs = append(hdrs, colorCyan(fmt.Sprintf("%-*s", wTag, "TAG")))
		fmt.Print(joinRow(hdrs, sep, colSep), "\n")

		for _, c := range commits {
			id := c.ID
			if len(id) > 8 {
				id = id[:8]
			}
			tag := strings.Join(tagsByHash[c.Hash], ", ")
			msg := truncateByWidth(c.Message, wMsg)

			row := []interface{}{
				colorYellow(fmt.Sprintf("%-*s", wVer, id)),
			}
			if allBranches {
				row = append(row,
					colorCyan(fmt.Sprintf("%-*s", wBch, c.Branch)),
					fmt.Sprintf("%-*s", wMsg, msg))
			} else {
				row = append(row, fmt.Sprintf("%-*s", wMsg, msg))
			}
			row = append(row, colorGreen(fmt.Sprintf("%-*s", wTag, tag)))
			fmt.Print(joinRow(row, sep, colSep), "\n")
		}
		return
	}

	for _, c := range commits {
		tags := tagsByHash[c.Hash]
		fmt.Printf("commit %s\n", colorYellow(c.ID))
		if allBranches {
			fmt.Printf("Branch:  %s\n", colorCyan(c.Branch))
		}
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

// joinRow joins row values with sep between adjacent columns, and appends
// colSep after the last column. All values are already strings.
func joinRow(row []interface{}, sep, colSep string) string {
	var b strings.Builder
	for i, v := range row {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(v.(string))
	}
	b.WriteString(colSep)
	return b.String()
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
