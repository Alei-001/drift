package cli

import (
	"fmt"
	"sort"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates the status subcommand.
func NewStatusCmd(application *apppkg.App) *cobra.Command {
	var porcelain bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show working tree status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := application.Status()
			if err != nil {
				return err
			}

			if porcelain {
				printStatusPorcelain(*status)
			} else {
				printStatus(*status)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&porcelain, "porcelain", false, "Machine-readable output")

	return cmd
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

func statusCode(c core.StatusCode) byte {
	return byte(c)
}
