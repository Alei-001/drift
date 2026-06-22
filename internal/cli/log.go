package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	logOneline bool
	logCount   int
)

var logCmd = &cobra.Command{
	Use:   "log [branch]",
	Short: "Show commit history",
	Long: `Show commit history for the current or specified branch.

Examples:
  drift log              # full history of current branch
  drift log feature      # history of feature branch
  drift log --oneline    # one line per commit
  drift log -n 5         # last 5 commits`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := ""
		if len(args) == 1 {
			branch = args[0]
		}
		if branch == "" {
			branch, _ = sharedStore.GetRef("HEAD")
			if branch == "" {
				branch = "main"
			}
		}

		commits, err := sharedStore.ListBranchCommits(branch)
		if err != nil {
			return fmt.Errorf("failed to read branch history: %w", err)
		}

		if len(commits) == 0 {
			fmt.Printf("No commits on branch %s yet\n", branch)
			return nil
		}

		// ListBranchCommits returns newest-first already.
		if logCount > 0 && logCount < len(commits) {
			commits = commits[:logCount]
		}

		if logOneline {
			for _, c := range commits {
				msg := c.Message
				if msg == "" {
					msg = "(no message)"
				}
				// Truncate long messages in oneline mode.
				if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
					msg = msg[:idx]
				}
				if len(msg) > 60 {
					msg = msg[:57] + "..."
				}
				fmt.Printf("%s [%s] %s\n", c.ID, c.Branch, msg)
			}
			return nil
		}

		for i, c := range commits {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("commit %s\n", c.Hash)
			fmt.Printf("Version: %s\n", c.ID)
			fmt.Printf("Branch:  %s\n", c.Branch)
			fmt.Printf("Date:    %s\n", c.Timestamp.Format("2006-01-02 15:04:05"))
			if c.Author.Name != "" {
				fmt.Printf("Author:  %s <%s>\n", c.Author.Name, c.Author.Email)
			}
			if c.Message != "" {
				fmt.Printf("\n    %s\n", c.Message)
			}
		}
		return nil
	},
}

func init() {
	logCmd.Flags().BoolVar(&logOneline, "oneline", false, "Show one line per commit")
	logCmd.Flags().IntVarP(&logCount, "number", "n", 0, "Limit number of commits")
	rootCmd.AddCommand(logCmd)
}
