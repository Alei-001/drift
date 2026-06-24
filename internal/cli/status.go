package cli

import (
	"fmt"
	"sort"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var statusPorcelain bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current branch, version, and working tree status",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show current branch and version
		currentBranch, _ := sharedStore.GetRef("HEAD")
		if currentBranch == "" {
			currentBranch = "main"
		}

		commit, _ := sharedRepo.CurrentCommit()
		if commit != nil {
			fmt.Printf("On branch %s, version %s\n\n", currentBranch, commit.ID)
		} else {
			fmt.Printf("On branch %s, no commits yet\n\n", currentBranch)
		}

		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}

		var commitTree *core.Tree
		if commit != nil {
			if commit.TreeHash != "" {
				t, err := sharedStore.GetTree(commit.TreeHash)
				if err == nil {
					commitTree = t
				}
			}
		}

		status, err := core.ComputeStatus(commitTree, &idx, sharedDir, sharedStore)
		if err != nil {
			return fmt.Errorf("failed to compute status: %w", err)
		}

		if statusPorcelain {
			printStatusPorcelain(status)
		} else {
			printStatus(status)
		}
		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVar(&statusPorcelain, "porcelain", false, "Machine-readable output (XY path format)")
	rootCmd.AddCommand(statusCmd)
}

func printStatus(s core.Status) {
	if s.IsClean() {
		fmt.Println("Nothing to commit, working tree clean")
		return
	}

	// Collect and sort paths for deterministic output order.
	paths := make([]string, 0, len(s))
	for path := range s {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	// Issue 25: single pass to collect groups, avoiding repeated iteration.
	var staged, unstaged, untracked []string
	for _, path := range paths {
		fs := s[path]
		if fs.Staging != core.Unmodified && fs.Staging != core.Untracked {
			staged = append(staged, fmt.Sprintf("  %s %s", fs.Staging, path))
		}
		if fs.Worktree != core.Unmodified && fs.Worktree != core.Untracked {
			unstaged = append(unstaged, fmt.Sprintf("  %s %s", fs.Worktree, path))
		}
		if fs.Worktree == core.Untracked {
			untracked = append(untracked, "  "+path)
		}
	}

	if len(staged) > 0 {
		fmt.Println("Staged changes:")
		for _, line := range staged {
			fmt.Println(line)
		}
		fmt.Println()
	}

	if len(unstaged) > 0 {
		fmt.Println("Unstaged changes:")
		for _, line := range unstaged {
			fmt.Println(line)
		}
		fmt.Println()
	}

	if len(untracked) > 0 {
		fmt.Println("Untracked files:")
		for _, line := range untracked {
			fmt.Println(line)
		}
	}
}

// printStatusPorcelain outputs status in a machine-readable format similar
// to `git status --porcelain`: two status characters (staged + worktree)
// followed by the file path, one per line.
func printStatusPorcelain(s core.Status) {
	// Collect and sort paths for deterministic output.
	paths := make([]string, 0, len(s))
	for path := range s {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		fs := s[path]
		staged := statusCode(fs.Staging)
		worktree := statusCode(fs.Worktree)
		if staged == ' ' && worktree == ' ' {
			continue
		}
		fmt.Printf("%c%c %s\n", staged, worktree, path)
	}
}

// statusCode converts a StatusCode to a single character for porcelain output.
func statusCode(c core.StatusCode) byte {
	switch c {
	case core.Modified:
		return 'M'
	case core.Added:
		return 'A'
	case core.Deleted:
		return 'D'
	case core.Untracked:
		return '?'
	default:
		return ' '
	}
}
