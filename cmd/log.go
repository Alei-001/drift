package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/porcelain"
	"github.com/your-org/drift/internal/storage"
)

var logLimit int
var logVerbose string
var logJSON bool
var logAll bool
var logBranch string

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show snapshot history",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Log", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if logVerbose != "" {
			return logVerboseMode(ctx, store, logVerbose)
		}

		var snapshots []*core.SnapshotSummary
		var branchMap map[string][]string

		if logBranch != "" {
			// Branch-filtered: list ALL snapshots, then keep only those whose
			// nearest-tip attribution is this branch. This shows the branch's
			// own commits, excluding commits inherited from parent branches
			// (which are attributed to the parent by the nearest-tip rule).
			if _, refErr := store.GetRef(ctx, "heads/"+logBranch); refErr != nil {
				statusFailed("Log", fmt.Sprintf("branch '%s' not found.", logBranch), "use 'drift branch' to list existing branches.")
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
			// Global: list all, build branch map for the column
			opts := &storage.ListOptions{Limit: logLimit, Offset: 0}
			snapshots, err = store.ListSnapshots(ctx, opts)
			if err != nil {
				return err
			}
			branchMap, err = porcelain.ResolveSnapshotBranches(ctx, store)
			if err != nil {
				return err
			}
		}

		// Filter auto-saves unless --all
		var filtered []*core.SnapshotSummary
		for _, s := range snapshots {
			if !logAll && strings.HasPrefix(s.Message, "auto -") {
				continue
			}
			filtered = append(filtered, s)
		}

		// Sort newest-first: by timestamp descending. When timestamps are
		// equal (common for rapid successive saves), use the PrevID chain —
		// if snapshot A's PrevID points to snapshot B, then A is newer.
		sortSnapshotSummariesNewestFirst(filtered)

		// Apply limit after filtering for branch walk
		if logBranch != "" && logLimit > 0 && len(filtered) > logLimit {
			filtered = filtered[:logLimit]
		}

		if len(filtered) == 0 {
			hint := "use 'drift save -m \"message\"' to create your first snapshot."
			if logBranch != "" {
				hint = fmt.Sprintf("branch '%s' has no snapshots yet.", logBranch)
			}
			statusFailed("Log", "no snapshots yet.", hint)
			return ErrSilent
		}

		if logJSON {
			return logJSONMode(ctx, store, filtered, branchMap)
		}

		// Default table format
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
			timeStr := time.Unix(s.Timestamp, 0).Format("2006-01-02 15:04")
			add, mod, del := countSnapshotChanges(ctx, store, s)
			changes := formatChangesCompact(add, mod, del)

			msg := s.Message
			if len([]rune(msg)) > 28 {
				msg = string([]rune(msg)[:27]) + "…"
			}

			// Branch column
			branchCol := ""
			if branches, ok := branchMap[s.ID.Hash.String()]; ok && len(branches) > 0 {
				branchCol = strings.Join(branches, ",")
			}
			if len([]rune(branchCol)) > 16 {
				branchCol = string([]rune(branchCol)[:15]) + "…"
			}

			tag := ""
			for _, t := range s.Tags {
				if t != "" {
					tag = t
					break
				}
			}
			if tag != "" {
				if len([]rune(tag)) > 10 {
					tag = string([]rune(tag)[:9]) + "…"
				}
				tag = "[" + tag + "]"
			}

			suffix := ""
			if strings.HasPrefix(s.Message, "auto -") {
				suffix = "    · dimmed"
			}
			fmt.Printf("%s  %s  %-16s  %-28s  %-12s  %s%s\n", s.ShortID(), timeStr, branchCol, msg, tag, changes, suffix)
		}
		return nil
	},
}

func logVerboseMode(ctx context.Context, store storage.Storer, id string) error {
	snapshot := resolveSnapshot(ctx, store, id)
	if snapshot == nil {
		statusFailed("Log", fmt.Sprintf("snapshot not found: %s.", id), "use 'drift log' to list available snapshots.")
		return ErrSilent
	}

	fmt.Printf(">>> Snapshot %s\n", snapshot.ShortID())
	timeStr := time.Unix(snapshot.Timestamp, 0).Format("2006-01-02 15:04")
	fmt.Printf("%s  %s\n", timeStr, snapshot.Message)

	add, mod, del, err := computeChanges(ctx, store, snapshot)
	if err != nil {
		return err
	}
	printFileListWithLineCount(add, mod, del, store)
	total := len(add) + len(mod) + len(del)
	summaryLine(total, len(add), len(mod), len(del))
	return nil
}

func countSnapshotChanges(ctx context.Context, store storage.Storer, summary *core.SnapshotSummary) (added, modified, deleted int) {
	snapshot, err := store.GetSnapshot(ctx, summary.ID)
	if err != nil {
		slog.Warn("load snapshot for changes", "snapshot", summary.ShortID(), "error", err)
		return 0, 0, 0
	}
	a, m, d, err := computeChanges(ctx, store, snapshot)
	if err != nil {
		slog.Warn("compute snapshot changes failed", "snapshot", snapshot.ShortID(), "error", err)
		return 0, 0, 0
	}
	return len(a), len(m), len(d)
}

// formatChangesCompact formats change counts as "+A ~M -D", omitting zero parts.
// Example: 2 added, 1 modified, 0 deleted → "+2 ~1"
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

// sortSnapshotSummariesNewestFirst sorts snapshot summaries newest-first.
// Primary sort key is timestamp (descending). When timestamps are equal (rapid
// successive saves), it uses the PrevID chain: if A.PrevID == B.ID then A is
// newer than B. This is stable for unrelated summaries.
func sortSnapshotSummariesNewestFirst(snaps []*core.SnapshotSummary) {
	summaryByID := make(map[core.SnapshotID]*core.SnapshotSummary, len(snaps))
	for _, s := range snaps {
		summaryByID[s.ID] = s
	}

	depth := make(map[core.SnapshotID]int, len(snaps))
	for _, start := range snaps {
		if _, ok := depth[start.ID]; ok {
			continue
		}
		var chain []*core.SnapshotSummary
		cur := start
		for cur != nil {
			if d, ok := depth[cur.ID]; ok {
				for i := len(chain) - 1; i >= 0; i-- {
					d++
					depth[chain[i].ID] = d
				}
				chain = nil
				break
			}
			chain = append(chain, cur)
			if cur.PrevID != nil {
				cur = summaryByID[*cur.PrevID]
			} else {
				cur = nil
			}
		}
		for i := len(chain) - 1; i >= 0; i-- {
			depth[chain[i].ID] = len(chain) - 1 - i
		}
	}

	sort.SliceStable(snaps, func(i, j int) bool {
		if snaps[i].Timestamp != snaps[j].Timestamp {
			return snaps[i].Timestamp > snaps[j].Timestamp
		}
		return depth[snaps[i].ID] > depth[snaps[j].ID]
	})
}

func logJSONMode(ctx context.Context, store storage.Storer, snapshots []*core.SnapshotSummary, branchMap map[string][]string) error {
	type jsonEntry struct {
		ID      string   `json:"id"`
		Time    string   `json:"time"`
		Message string   `json:"message"`
		Branch  []string `json:"branch,omitempty"`
		Tag     *string  `json:"tag"`
		Changes string   `json:"changes"`
	}

	var entries []jsonEntry
	for _, s := range snapshots {
		add, mod, del := countSnapshotChanges(ctx, store, s)
		changes := fmt.Sprintf("+%d ~%d -%d", add, mod, del)
		entry := jsonEntry{
			ID:      s.ShortID(),
			Time:    time.Unix(s.Timestamp, 0).Format("2006-01-02T15:04:05"),
			Message: s.Message,
			Changes: changes,
		}
		if branches, ok := branchMap[s.ID.Hash.String()]; ok && len(branches) > 0 {
			entry.Branch = branches
		}
		if len(s.Tags) > 0 && s.Tags[0] != "" {
			tag := s.Tags[0]
			entry.Tag = &tag
		}
		entries = append(entries, entry)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf(">>> History (%d)\n", len(snapshots))
	os.Stdout.Write(data)
	fmt.Println()
	return nil
}

func init() {
	logCmd.Flags().IntVarP(&logLimit, "limit", "l", 10, "limit number of entries")
	logCmd.Flags().StringVarP(&logVerbose, "verbose", "v", "", "show file details for a snapshot")
	logCmd.Flags().StringVarP(&logBranch, "branch", "b", "", "filter history by branch")
	logCmd.Flags().BoolVar(&logJSON, "json", false, "output in JSON format for scripting")
	logCmd.Flags().BoolVar(&logAll, "all", false, "include auto-saved snapshots")
	rootCmd.AddCommand(logCmd)
}
