package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
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
		ctx := context.Background()
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if logVerbose != "" {
			return logVerboseMode(ctx, store, logVerbose)
		}

		var snapshots []*core.Snapshot
		var branchMap map[string][]string

		if logBranch != "" {
			// Branch-filtered: walk PrevID chain from branch tip
			snapshots, err = walkBranchHistory(ctx, store, logBranch)
			if err != nil {
				return err
			}
			branchMap = make(map[string][]string)
			for _, s := range snapshots {
				branchMap[s.ID.Hash.String()] = []string{logBranch}
			}
		} else {
			// Global: list all, build branch map for the column
			opts := &storage.ListOptions{Limit: logLimit, Offset: 0}
			snapshots, err = store.ListSnapshots(ctx, opts)
			if err != nil {
				return err
			}
			branchMap, _ = buildBranchMap(ctx, store)
		}

		// Filter auto-saves unless --all
		var filtered []*core.Snapshot
		for _, s := range snapshots {
			if !logAll && strings.HasPrefix(s.Message, "auto -") {
				continue
			}
			filtered = append(filtered, s)
		}

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
			return nil
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
			changes := fmt.Sprintf("+%d ~%d", add, mod)
			if del > 0 {
				changes += fmt.Sprintf(" -%d", del)
			}

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
		return fmt.Errorf("snapshot not found: %s", id)
	}

	fmt.Printf(">>> Snapshot %s\n", snapshot.ShortID())
	timeStr := time.Unix(snapshot.Timestamp, 0).Format("2006-01-02 15:04")
	fmt.Printf("%s  %s\n", timeStr, snapshot.Message)

	add, mod, del := computeVerboseChanges(ctx, store, snapshot)
	printFileListWithLineCount(add, mod, del, store)
	total := len(add) + len(mod) + len(del)
	summaryLine(total, len(add), len(mod), len(del))
	return nil
}

func computeVerboseChanges(ctx context.Context, store storage.Storer, snapshot *core.Snapshot) (added, modified []core.FileEntry, deleted []string) {
	currFiles := make(map[string]core.FileEntry)
	for _, f := range snapshot.Files {
		currFiles[f.Path] = f
	}

	var prevFiles map[string]core.FileEntry
	if snapshot.PrevID != nil {
		prevSnap, err := store.GetSnapshot(ctx, *snapshot.PrevID)
		if err == nil {
			prevFiles = make(map[string]core.FileEntry)
			for _, f := range prevSnap.Files {
				prevFiles[f.Path] = f
			}
		}
	}

	for _, f := range snapshot.Files {
		if prevFiles == nil {
			added = append(added, f)
			continue
		}
		if prev, ok := prevFiles[f.Path]; !ok {
			added = append(added, f)
		} else if prev.Size != f.Size || !chunkHashesEqual(prev.Chunks, f.Chunks) {
			modified = append(modified, f)
		}
	}

	if prevFiles != nil {
		for p := range prevFiles {
			if _, ok := currFiles[p]; !ok {
				deleted = append(deleted, p)
			}
		}
	}
	return
}

func countSnapshotChanges(ctx context.Context, store storage.Storer, snapshot *core.Snapshot) (added, modified, deleted int) {
	a, m, d := computeVerboseChanges(ctx, store, snapshot)
	return len(a), len(m), len(d)
}

func logJSONMode(ctx context.Context, store storage.Storer, snapshots []*core.Snapshot, branchMap map[string][]string) error {
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

// walkBranchHistory walks the PrevID chain from a branch's tip snapshot and
// returns all reachable snapshots in newest-first order.
func walkBranchHistory(ctx context.Context, store storage.Storer, branchName string) ([]*core.Snapshot, error) {
	ref, err := store.GetRef(ctx, "heads/"+branchName)
	if err != nil {
		return nil, fmt.Errorf("branch '%s' not found", branchName)
	}
	if ref.Target.IsZero() {
		return nil, nil
	}

	var snapshots []*core.Snapshot
	current := &core.SnapshotID{Hash: ref.Target}
	for current != nil {
		snap, err := store.GetSnapshot(ctx, *current)
		if err != nil {
			break
		}
		snapshots = append(snapshots, snap)
		if snap.PrevID != nil {
			current = snap.PrevID
		} else {
			current = nil
		}
	}
	return snapshots, nil
}

// buildBranchMap builds a map from snapshot hash to the list of branch names
// whose history (PrevID chain) contains that snapshot.
func buildBranchMap(ctx context.Context, store storage.Storer) (map[string][]string, error) {
	branches, _, err := porcelain.ListBranches(ctx, store)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]string)
	for _, b := range branches {
		name := strings.TrimPrefix(b.Name, "heads/")
		if b.Target.IsZero() {
			continue
		}
		current := &core.SnapshotID{Hash: b.Target}
		for current != nil {
			snap, err := store.GetSnapshot(ctx, *current)
			if err != nil {
				break
			}
			hashStr := snap.ID.Hash.String()
			already := false
			for _, existing := range result[hashStr] {
				if existing == name {
					already = true
					break
				}
			}
			if !already {
				result[hashStr] = append(result[hashStr], name)
			}
			if snap.PrevID != nil {
				current = snap.PrevID
			} else {
				current = nil
			}
		}
	}
	return result, nil
}

func init() {
	logCmd.Flags().IntVarP(&logLimit, "limit", "l", 10, "limit number of entries")
	logCmd.Flags().StringVarP(&logVerbose, "verbose", "v", "", "show file details for a snapshot")
	logCmd.Flags().StringVarP(&logBranch, "branch", "b", "", "filter history by branch")
	logCmd.Flags().BoolVar(&logJSON, "json", false, "output in JSON format for scripting")
	logCmd.Flags().BoolVar(&logAll, "all", false, "include auto-saved snapshots")
	rootCmd.AddCommand(logCmd)
}
