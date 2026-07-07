package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/porcelain"
	"github.com/your-org/drift/internal/storage"
)

// autoSavePrefix is the message prefix that identifies auto-saved snapshots
// (created by 'drift watch'). The log command hides these by default unless
// --all is given. Mirrors the unexported porcelain.autoSavePrefix.
const autoSavePrefix = "auto -"

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

By default only manually created snapshots are shown; auto-saves created by
'drift watch' are hidden unless --all is given. Use --branch to filter by
branch, --limit to cap the number of entries, and --detail to inspect the
file changes of a single snapshot.`,
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

		var snapshots []*core.SnapshotSummary
		var branchMap map[string][]string

		if logBranch != "" {
			// Branch-filtered: list ALL snapshots, then keep only those whose
			// nearest-tip attribution is this branch. This shows the branch's
			// own commits, excluding commits inherited from parent branches
			// (which are attributed to the parent by the nearest-tip rule).
			if _, refErr := store.GetRef(ctx, "heads/"+logBranch); refErr != nil {
				reportFailed("Log", "log", fmt.Sprintf("branch '%s' not found.", logBranch), "use 'drift branch' to list existing branches.")
				return ErrSilent
			}
			allSnaps, err := store.ListSnapshots(ctx, &storage.ListOptions{})
			if err != nil {
				return err
			}
			branchMap, err = porcelain.ResolveSnapshotBranches(ctx, store)
			if err != nil {
				return err
			}
			for _, s := range allSnaps {
				if names, ok := branchMap[s.ID.Hash.String()]; ok && len(names) > 0 && names[0] == logBranch {
					snapshots = append(snapshots, s)
				}
			}
		} else {
			// Global: list ALL snapshots, then filter auto-saves and apply the
			// limit afterwards. Applying the limit at the storage layer would
			// surface only the N most recent entries — which may be dominated
			// by auto-saves — hiding older manual snapshots from the user.
			snapshots, err = store.ListSnapshots(ctx, &storage.ListOptions{})
			if err != nil {
				return err
			}
			branchMap, err = porcelain.ResolveSnapshotBranches(ctx, store)
			if err != nil {
				return err
			}
		}

		// Filter auto-saves unless --all.
		var filtered []*core.SnapshotSummary
		for _, s := range snapshots {
			if !logAll && strings.HasPrefix(s.Message, autoSavePrefix) {
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
			hint := "use 'drift save -m \"message\"' to create your first snapshot."
			if logBranch != "" {
				hint = fmt.Sprintf("branch '%s' has no snapshots yet.", logBranch)
			}
			reportFailed("Log", "log", "no snapshots yet.", hint)
			return ErrSilent
		}

		if globalJSON {
			return logJSONMode(ctx, store, filtered, branchMap)
		}

		// Quiet mode: success produces no output (exit code is authoritative).
		if globalQuiet {
			return nil
		}

		// Default table format.
		label := fmt.Sprintf("History (%d snapshots", len(filtered))
		if logBranch != "" {
			label += fmt.Sprintf(" on '%s'", logBranch)
		}
		if logAll {
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

			// Branch column.
			branchCol := ""
			if branches, ok := branchMap[s.ID.Hash.String()]; ok && len(branches) > 0 {
				branchCol = strings.Join(branches, ",")
			}
			if len([]rune(branchCol)) > branchColWidth {
				branchCol = string([]rune(branchCol)[:branchColWidth-1]) + "..."
			}

			tag := ""
			for _, t := range s.Tags {
				if t != "" {
					tag = t
					break
				}
			}
			if tag != "" {
				if len([]rune(tag)) > tagMaxLen {
					tag = string([]rune(tag)[:tagMaxLen-1]) + "..."
				}
				tag = "[" + tag + "]"
			}

			suffix := ""
			if strings.HasPrefix(s.Message, autoSavePrefix) {
				suffix = "    · dimmed"
			}
			fmt.Printf("%s  %s  %-*s  %-*s  %-*s  %s%s\n",
				s.ShortID(), timeStr,
				branchColWidth, branchCol,
				msgColWidth, msg,
				tagColWidth, tag,
				changes, suffix)
		}
		return nil
	},
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
	logCmd.Flags().StringVar(&logBranch, "branch", "", "filter history by branch")
	logCmd.Flags().BoolVar(&logAll, "all", false, "include auto-saved snapshots")
	rootCmd.AddCommand(logCmd)
}
