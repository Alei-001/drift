package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/repo"
	"github.com/spf13/cobra"
)

var (
	historyOneline   bool
	historyAll       bool
	historyCount     int
	historyPorcelain bool
)

var historyCmd = &cobra.Command{
	Use:   "history [branch] [--all]",
	Short: "Show commit history",
	Long: `Show commit history for the current or specified branch.

By default, shows the history of the current branch. Use --all to show
commits across all branches (deduplicated, sorted by time, newest first).
Use --oneline for a compact one-line-per-commit format.

Examples:
  drift history              # full history of current branch
  drift history feature      # history of feature branch
  drift history --all        # history across all branches
  drift history --oneline    # one line per commit
  drift history -n 5         # last 5 commits
  drift history --all --oneline  # compact view of all branches
  drift history --porcelain  # machine-readable output`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := ""
		if len(args) == 1 {
			branch = args[0]
		}

		if historyAll {
			return historyAllBranches()
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
		if historyCount > 0 && historyCount < len(commits) {
			commits = commits[:historyCount]
		}

		if historyPorcelain {
			printCommitsPorcelain(commits)
		} else {
			printCommits(commits, historyOneline)
		}
		return nil
	},
}

// historyAllBranches shows commits across all branches, deduplicated and
// sorted by timestamp (newest first). This replaces the former `list` command.
func historyAllBranches() error {
	if !sharedStore.IsInitialized() {
		return fmt.Errorf("not a drift project (run 'drift init')")
	}

	refs, err := sharedStore.ListRefs()
	if err != nil {
		return fmt.Errorf("failed to list refs: %w", err)
	}

	// Build a reverse map of commit hash → names for alias display.
	hashToNames := make(map[string][]string)
	for refName, hash := range refs {
		if strings.HasPrefix(refName, "names/") {
			label := strings.TrimPrefix(refName, "names/")
			hashToNames[hash] = append(hashToNames[hash], label)
		}
	}

	seen := make(map[string]bool)
	var all []struct {
		id      string
		branch  string
		ts      int64
		message string
		names   []string
	}

	// Collect and sort branch names for deterministic iteration order, so
	// that a commit reachable from multiple branches always gets the same
	// branch label across runs.
	branches := make([]string, 0, len(refs))
	for branchName := range refs {
		branches = append(branches, branchName)
	}
	sort.Strings(branches)
	for _, branchName := range branches {
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
				names   []string
			}{
				id:      c.ID,
				branch:  c.Branch,
				ts:      c.Timestamp.UnixMilli(),
				message: c.Message,
				names:   hashToNames[c.Hash],
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

	if historyCount > 0 && historyCount < len(all) {
		all = all[:historyCount]
	}

	if historyPorcelain {
		for _, c := range all {
			msg := c.message
			if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
				msg = msg[:idx]
			}
			names := strings.Join(c.names, ",")
			fmt.Printf("%s\t%s\t%s\t%d\t%s\n", c.id, c.branch, names, c.ts, msg)
		}
		return nil
	}

	if historyOneline {
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
			idPart := c.id
			if len(c.names) > 0 {
				idPart += " (" + strings.Join(c.names, ", ") + ")"
			}
			fmt.Printf("%s [%s] %s\n", idPart, c.branch, msg)
		}
		return nil
	}

	fmt.Println("Version history:")
	fmt.Println()
	for _, c := range all {
		msg := c.message
		if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
			msg = msg[:idx] + "..."
		}
		idPart := c.id
		if len(c.names) > 0 {
			idPart += " (" + strings.Join(c.names, ", ") + ")"
		}
		fmt.Printf("  %s  [%s]  %s\n", idPart, c.branch, msg)
	}
	return nil
}

// printCommits renders commits in either detailed or oneline format.
func printCommits(commits []*core.Commit, oneline bool) {
	// Build a reverse map of commit hash → names for alias display.
	hashToNames := make(map[string][]string)
	if refs, err := sharedStore.ListRefs(); err == nil {
		for refName, hash := range refs {
			if strings.HasPrefix(refName, "names/") {
				label := strings.TrimPrefix(refName, "names/")
				hashToNames[hash] = append(hashToNames[hash], label)
			}
		}
	}

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
			idPart := c.ID
			if names := hashToNames[c.Hash]; len(names) > 0 {
				idPart += " (" + strings.Join(names, ", ") + ")"
			}
			fmt.Printf("%s [%s] %s\n", idPart, c.Branch, msg)
		}
		return
	}

	for i, c := range commits {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("commit %s\n", c.Hash)
		fmt.Printf("Version: %s\n", c.ID)
		if names := hashToNames[c.Hash]; len(names) > 0 {
			fmt.Printf("Names:   %s\n", strings.Join(names, ", "))
		}
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

// printCommitsPorcelain outputs commits in a machine-readable format:
// <id>\t<branch>\t<names>\t<timestamp>\t<message-first-line>
func printCommitsPorcelain(commits []*core.Commit) {
	hashToNames := make(map[string][]string)
	if refs, err := sharedStore.ListRefs(); err == nil {
		for refName, hash := range refs {
			if strings.HasPrefix(refName, "names/") {
				label := strings.TrimPrefix(refName, "names/")
				hashToNames[hash] = append(hashToNames[hash], label)
			}
		}
	}

	for _, c := range commits {
		msg := c.Message
		if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
			msg = msg[:idx]
		}
		names := strings.Join(hashToNames[c.Hash], ",")
		fmt.Printf("%s\t%s\t%s\t%d\t%s\n", c.ID, c.Branch, names, c.Timestamp.UnixMilli(), msg)
	}
}

// undoCount is the number of operations to undo with `drift undo -n`.
// Default is 1 (the most recent operation).
var undoCount int

var undoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Undo the most recent operation(s)",
	Long: `Undo the most recent operation(s) recorded in the operation log.

By default, undoes the single most recent operation. Use -n to undo
multiple operations in sequence.

Examples:
  drift undo          # undo the most recent operation
  drift undo -n 3     # undo the last 3 operations`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if undoCount < 1 {
			return fmt.Errorf("-n must be a positive integer")
		}

		var totalRestored int
		var lastDesc string
		var lastOp repo.OpType
		for i := 0; i < undoCount; i++ {
			last, restored, err := sharedRepo.Undo()
			if err != nil {
				if i == 0 {
					// Nothing was undone at all.
					return err
				}
				// We undid some operations before hitting this error.
				fmt.Printf("\nStopped after %d operation(s): %s\n", i, err)
				break
			}
			totalRestored += restored
			lastDesc = last.Desc
			lastOp = last.Op
		}

		if undoCount == 1 {
			fmt.Printf("Undid: %s (%s)\n", lastDesc, lastOp)
		} else {
			fmt.Printf("Undid %d operation(s); last was: %s (%s)\n", undoCount, lastDesc, lastOp)
		}
		fmt.Printf("Restored %d ref(s) to previous state.\n", totalRestored)
		return nil
	},
}

func init() {
	historyCmd.Flags().BoolVar(&historyOneline, "oneline", false, "Show one line per commit")
	historyCmd.Flags().BoolVar(&historyAll, "all", false, "Show commits across all branches")
	historyCmd.Flags().IntVarP(&historyCount, "number", "n", 0, "Limit number of commits")
	historyCmd.Flags().BoolVar(&historyPorcelain, "porcelain", false, "Machine-readable output")
	undoCmd.Flags().IntVarP(&undoCount, "number", "n", 1, "Number of operations to undo")
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(undoCmd)
}
