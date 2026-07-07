package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/Alei-001/drift/internal/porcelain"
)

var statusShort bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show working tree status",
	Long:  "Show changes since the last save. Lists all added, modified, and deleted files in the working tree.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Status", "status", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		// Resolve the current branch name once; empty means detached HEAD.
		// Surface it in human and JSON output so users always know where they
		// stand without running 'drift branch list'.
		currentBranch := porcelain.ResolveCurrentBranchName(ctx, store)

		changes, err := porcelain.DetectChanges(ctx, store, cwd, &cfg.Core)
		if err != nil {
			reportFailed("Status", "status", err.Error(), "")
			return ErrSilent
		}

		if globalJSON {
			added := changes.Added
			if added == nil {
				added = []string{}
			}
			modified := changes.Modified
			if modified == nil {
				modified = []string{}
			}
			deleted := changes.Deleted
			if deleted == nil {
				deleted = []string{}
			}
			data := statusJSONData{
				Branch:   currentBranch,
				Added:    added,
				Modified: modified,
				Deleted:  deleted,
				Summary: statusJSONSummary{
					Total:    len(added) + len(modified) + len(deleted),
					Added:    len(added),
					Modified: len(modified),
					Deleted:  len(deleted),
				},
			}
			return outputJSON(JSONEnvelope{Command: "status", Status: "ok", Data: data})
		}

		// Quiet mode: success produces no output (exit code is authoritative).
		if globalQuiet {
			return nil
		}

		total := len(changes.Added) + len(changes.Modified) + len(changes.Deleted)

		if total == 0 {
			statusOK("Status")
			// --short is paths-only; skip the branch context line so
			// scripts don't have to filter prose.
			if !statusShort {
				printBranchLine(currentBranch)
			}
			fmt.Println("Nothing changed since last save.")
			return nil
		}

		if statusShort {
			fmt.Printf(">>> Status (%d %s)\n", total, pluralFile(total))
			for _, p := range changes.Added {
				fmt.Println(p)
			}
			for _, p := range changes.Modified {
				fmt.Println(p)
			}
			for _, p := range changes.Deleted {
				fmt.Println(p)
			}
		} else {
			header := fmt.Sprintf("Status (%d %s changed since last save)", total, pluralFile(total))
			fmt.Printf(">>> %s\n", header)
			printBranchLine(currentBranch)
			fmt.Println()
			for _, p := range changes.Added {
				fmt.Printf("  +  %s\n", p)
			}
			for _, p := range changes.Modified {
				fmt.Printf("  ~  %s\n", p)
			}
			for _, p := range changes.Deleted {
				fmt.Printf("  -  %s\n", p)
			}
			summaryLine(total, len(changes.Added), len(changes.Modified), len(changes.Deleted))
		}
		return nil
	},
}

// printBranchLine renders the current-branch context line shown in the human
// status output. A non-empty branch name yields "On branch: <name>"; a detached
// HEAD yields "HEAD detached". The line is omitted only when --short is used,
// because that format is paths-only for scripting.
func printBranchLine(branch string) {
	if branch == "" {
		fmt.Println("HEAD detached")
		return
	}
	fmt.Printf("On branch: %s\n", branch)
}

func init() {
	statusCmd.Flags().BoolVarP(&statusShort, "short", "s", false, "short format, paths only")
	rootCmd.AddCommand(statusCmd)
}

// statusJSONSummary is the per-category change tally for 'drift status --json'.
type statusJSONSummary struct {
	Total    int `json:"total"`
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

// statusJSONData is the data payload of the 'drift status --json' envelope.
// Branch is the current branch name (without "heads/" prefix), or empty if
// HEAD is detached.
type statusJSONData struct {
	Branch   string            `json:"branch"`
	Added    []string          `json:"added"`
	Modified []string          `json:"modified"`
	Deleted  []string          `json:"deleted"`
	Summary  statusJSONSummary `json:"summary"`
}
