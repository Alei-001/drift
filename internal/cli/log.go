package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var (
	logOneline bool
	logAll     bool
	logCount   int
)

var logCmd = &cobra.Command{
	Use:   "log [branch] [--all]",
	Short: "Show commit history",
	Long: `Show commit history for the current or specified branch.

By default, shows the history of the current branch. Use --all to show
commits across all branches (deduplicated, sorted by time, newest first).
Use --oneline for a compact one-line-per-commit format.

Examples:
  drift log              # full history of current branch
  drift log feature      # history of feature branch
  drift log --all        # history across all branches
  drift log --oneline    # one line per commit
  drift log -n 5         # last 5 commits
  drift log --all --oneline  # compact view of all branches`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := ""
		if len(args) == 1 {
			branch = args[0]
		}

		if logAll {
			return logAllBranches()
		}

		userSpecifiedBranch := branch != ""

		if branch == "" {
			branch, _ = sharedStore.GetRef("HEAD")
			if branch == "" {
				branch = "main"
			}
		}

		// Validate explicitly-specified branch exists. The default branch
		// (from HEAD) may not have a ref yet on a fresh project with no
		// commits, so we don't validate it — ListBranchCommits will simply
		// return empty in that case.
		if userSpecifiedBranch {
			if _, err := sharedStore.GetRef(branch); err != nil {
				return fmt.Errorf("branch not found: %s", branch)
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

		printCommits(commits, logOneline)
		return nil
	},
}

// logAllBranches shows commits across all branches, deduplicated and
// sorted by timestamp (newest first). This replaces the former `list` command.
func logAllBranches() error {
	if !sharedStore.IsInitialized() {
		return fmt.Errorf("not a drift project (run 'drift init')")
	}

	refs, err := sharedStore.ListRefs()
	if err != nil {
		return fmt.Errorf("failed to list refs: %w", err)
	}

	seen := make(map[string]bool)
	var all []struct {
		id      string
		branch  string
		ts      int64
		message string
	}

	for branchName := range refs {
		if branchName == "HEAD" || strings.HasPrefix(branchName, "names/") {
			continue
		}
		commits, err := sharedStore.ListBranchCommits(branchName)
		if err != nil {
			continue
		}
		for _, c := range commits {
			if seen[c.Hash] {
				continue
			}
			seen[c.Hash] = true
			all = append(all, struct {
				id      string
				branch  string
				ts      int64
				message string
			}{
				id:      c.ID,
				branch:  c.Branch,
				ts:      c.Timestamp.UnixMilli(),
				message: c.Message,
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

	if logCount > 0 && logCount < len(all) {
		all = all[:logCount]
	}

	if logOneline {
		for _, c := range all {
			msg := c.message
			if msg == "" {
				msg = "(no message)"
			}
			if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
				msg = msg[:idx]
			}
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Printf("%s [%s] %s\n", c.id, c.branch, msg)
		}
		return nil
	}

	fmt.Println("Version history:")
	fmt.Println()
	for _, c := range all {
		fmt.Printf("  %s  [%s]  %s\n", c.id, c.branch, c.message)
	}
	return nil
}

// printCommits renders commits in either detailed or oneline format.
func printCommits(commits []*core.Commit, oneline bool) {
	if oneline {
		for _, c := range commits {
			msg := c.Message
			if msg == "" {
				msg = "(no message)"
			}
			if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
				msg = msg[:idx]
			}
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Printf("%s [%s] %s\n", c.ID, c.Branch, msg)
		}
		return
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
}

func init() {
	logCmd.Flags().BoolVar(&logOneline, "oneline", false, "Show one line per commit")
	logCmd.Flags().BoolVar(&logAll, "all", false, "Show commits across all branches")
	logCmd.Flags().IntVarP(&logCount, "number", "n", 0, "Limit number of commits")
	rootCmd.AddCommand(logCmd)
}
