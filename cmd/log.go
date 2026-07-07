package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
)

// Column widths for the default table view. Messages, branch names, and tags
// longer than these widths are truncated with "..." (width-1 runes plus the
// ellipsis).
const (
	msgColWidth    = 28
	branchColWidth = 16
	tagColWidth    = 12
	tagMaxLen      = 10
)

var logLimit int
var logDetail string
var logAll bool
var logBranch string

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show snapshot history",
	Long: `Browse snapshot history.

By default only the current branch's history is shown (including commits
inherited from parent branches). Auto-saves created by 'drift watch' are
hidden unless --all is given. Use --branch to walk another branch's chain,
--limit to cap the number of entries, and --detail to inspect the file
changes of a single snapshot.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Log", "log", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if logDetail != "" {
			return logDetailMode(ctx, store, logDetail)
		}

		// branchMap (snapshot hash -> branch names whose tip points at it) is
		// used to decorate the branch column. Only snapshots that ARE a branch
		// tip get a label — inherited commits remain blank so the user can
		// see where branches diverge (git --decorate semantics).
		branchMap, err := porcelain.ResolveBranchTips(ctx, store)
		if err != nil {
			return err
		}

		// tagMap (snapshot hash -> tag names whose ref Target points at it)
		// reflects the authoritative tag state. Tags added after the save via
		// 'drift tag add' only exist as refs (the snapshot is immutable), so
		// reading s.Tags alone would miss them. We merge both sources below.
		tagMap, err := porcelain.ResolveTagTips(ctx, store)
		if err != nil {
			return err
		}

		var snapshots []*core.SnapshotSummary
		var labelBranch string

		if logAll {
			// --all: list every snapshot in the store, regardless of branch.
			snapshots, err = store.ListSnapshots(ctx, &storage.ListOptions{})
			if err != nil {
				return err
			}
		} else {
			// Default or --branch: walk the PrevID chain from the branch tip.
			labelBranch = logBranch
			if labelBranch == "" {
				labelBranch = porcelain.ResolveCurrentBranchName(ctx, store)
			}
			startHash, err := resolveBranchTipHash(ctx, store, logBranch)
			if err != nil {
				return err
			}
			if startHash.IsZero() {
				return reportNoSnapshots(ctx, logBranch, logAll)
			}
			snapshots, err = porcelain.WalkSnapshotChain(ctx, store, startHash)
			if err != nil {
				return err
			}
		}

		// Filter auto-saves unless --all.
		var filtered []*core.SnapshotSummary
		for _, s := range snapshots {
			if !logAll && strings.HasPrefix(s.Message, porcelain.AutoSavePrefix) {
				continue
			}
			filtered = append(filtered, s)
		}

		// Sort newest-first: by timestamp descending. When timestamps are
		// equal (common for rapid successive saves), use the PrevID chain —
		// if snapshot A's PrevID points to snapshot B, then A is newer.
		porcelain.SortSnapshotSummariesNewestFirst(filtered)

		// Apply limit after filtering (both global and branch paths).
		if logLimit > 0 && len(filtered) > logLimit {
			filtered = filtered[:logLimit]
		}

		if len(filtered) == 0 {
			return reportNoSnapshots(ctx, logBranch, logAll)
		}

		if globalJSON {
			return logJSONMode(ctx, store, filtered, branchMap, tagMap)
		}

		// Quiet mode: success produces no output (exit code is authoritative).
		if globalQuiet {
			return nil
		}

		// Default table format.
		noun := "snapshots"
		if len(filtered) == 1 {
			noun = "snapshot"
		}
		label := fmt.Sprintf("History (%d %s", len(filtered), noun)
		if labelBranch != "" {
			label += fmt.Sprintf(" on '%s'", labelBranch)
		}
		if logAll {
			label += ", all branches"
		}
		if logAll && includesAutoSaves(filtered) {
			label += ", including auto-saves"
		}
		label += ")"
		fmt.Printf(">>> %s\n", label)
		for _, s := range filtered {
			if err := ctx.Err(); err != nil {
				return err
			}
			timeStr := time.Unix(s.Timestamp, 0).Format("2006-01-02 15:04")
			add, mod, del := porcelain.CountSnapshotChanges(ctx, store, s)
			changes := formatChangesCompact(add, mod, del)

			msg := s.Message
			if len([]rune(msg)) > msgColWidth {
				msg = string([]rune(msg)[:msgColWidth-1]) + "..."
			}

			// Branch column: only branch tips get labeled; others stay blank.
			branchCol := formatBranchColumn(branchMap[s.ID.Hash.String()])

			// Tag column: merge embedded snapshot tags with tag refs (which
			// may have been added after the save via 'drift tag add'). The
			// merged list is wrapped in brackets, e.g. "[v1.0]" or
			// "[v1.0,v2.0]"; overflow truncates to "[v1.0,+N]".
			tagCol := formatTagColumn(s.Tags, tagMap[s.ID.Hash.String()])

			suffix := ""
			if strings.HasPrefix(s.Message, porcelain.AutoSavePrefix) {
				suffix = "    · dimmed"
			}
			fmt.Printf("%s  %s  %-*s  %-*s  %-*s  %s%s\n",
				s.ShortID(), timeStr,
				branchColWidth, branchCol,
				msgColWidth, msg,
				tagColWidth, tagCol,
				changes, suffix)
		}
		return nil
	},
}

// resolveBranchTipHash returns the snapshot hash that the given branch points
// at. If branchName is empty, it resolves HEAD (following symref to the
// current branch). Returns a zero hash if no tip is set. Reports an error and
// returns ErrSilent if a named branch does not exist.
func resolveBranchTipHash(ctx context.Context, store storage.Storer, branchName string) (core.Hash, error) {
	if branchName == "" {
		headRef, err := store.GetRef(ctx, "HEAD")
		if err != nil {
			return core.Hash{}, nil
		}
		target := headRef.Target
		if headRef.SymRef != "" {
			bRef, err := store.GetRef(ctx, headRef.SymRef)
			if err != nil {
				return core.Hash{}, nil
			}
			target = bRef.Target
		}
		return target, nil
	}
	ref, err := store.GetRef(ctx, "heads/"+branchName)
	if err != nil {
		reportFailed("Log", "log", fmt.Sprintf("branch '%s' not found.", branchName), "use 'drift branch list' to list existing branches.")
		return core.Hash{}, ErrSilent
	}
	return ref.Target, nil
}

// reportNoSnapshots emits the "no snapshots" failure and returns ErrSilent.
// The hint adapts to the active mode (branch-filtered, all, or default).
func reportNoSnapshots(ctx context.Context, branchName string, all bool) error {
	hint := "use 'drift save -m \"message\"' to create your first snapshot."
	if branchName != "" {
		hint = fmt.Sprintf("branch '%s' has no snapshots yet.", branchName)
	} else if all {
		hint = "use 'drift save -m \"message\"' to create your first snapshot."
	}
	reportFailed("Log", "log", "no snapshots yet.", hint)
	return ErrSilent
}

// logDetailMode prints the file-change detail for a single snapshot. In JSON
// mode it emits an envelope; in quiet mode it produces no output; otherwise
// it prints the human-readable snapshot header, file list, and summary.
func logDetailMode(ctx context.Context, store storage.Storer, id string) error {
	snapshot := resolveSnapshot(ctx, store, id)
	if snapshot == nil {
		reportFailed("Log", "log", fmt.Sprintf("snapshot not found: %s.", id), "use 'drift log' to list available snapshots.")
		return ErrSilent
	}

	add, mod, del, err := porcelain.SnapshotFileDiff(ctx, store, snapshot)
	if err != nil {
		return err
	}

	if globalJSON {
		return logDetailJSONMode(ctx, store, snapshot, add, mod, del)
	}

	// Quiet mode: success produces no output.
	if globalQuiet {
		return nil
	}

	fmt.Printf(">>> Snapshot %s\n", snapshot.ShortID())
	timeStr := time.Unix(snapshot.Timestamp, 0).Format("2006-01-02 15:04")
	fmt.Printf("%s  %s\n", timeStr, snapshot.Message)
	printFileListWithLineCount(add, mod, del, store)
	total := len(add) + len(mod) + len(del)
	summaryLine(total, len(add), len(mod), len(del))
	return nil
}

// formatChangesCompact formats change counts as "+A ~M -D", omitting zero parts.
// Example: 2 added, 1 modified, 0 deleted -> "+2 ~1"
func formatChangesCompact(added, modified, deleted int) string {
	var parts []string
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("~%d", modified))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("-%d", deleted))
	}
	if len(parts) == 0 {
		return "+0"
	}
	return strings.Join(parts, " ")
}

func init() {
	logCmd.Flags().IntVarP(&logLimit, "limit", "l", 30, "limit number of entries")
	logCmd.Flags().StringVar(&logDetail, "detail", "", "show file change details for a snapshot (e.g. @id:12ab)")
	logCmd.Flags().StringVar(&logBranch, "branch", "", "show history of a specific branch (default: current branch)")
	logCmd.Flags().BoolVar(&logAll, "all", false, "show snapshots from all branches (including auto-saves)")
	rootCmd.AddCommand(logCmd)
}
