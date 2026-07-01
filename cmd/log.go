package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
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
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
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
			branchMap, _ = buildBranchMap(ctx, store)
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

		// Sort newest-first: by timestamp descending. When timestamps are
		// equal (common for rapid successive saves), use the PrevID chain —
		// if snapshot A's PrevID points to snapshot B, then A is newer.
		sortSnapshotsNewestFirst(filtered)

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
		} else if prev.Size != f.Size || !slices.Equal(prev.Chunks, f.Chunks) {
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

// sortSnapshotsNewestFirst sorts snapshots newest-first. Primary sort key is
// timestamp (descending). When timestamps are equal (rapid successive saves),
// it uses the PrevID chain: if A.PrevID == B.ID, then A is newer than B.
// This is stable for unrelated snapshots.
func sortSnapshotsNewestFirst(snaps []*core.Snapshot) {
	// Build a set of IDs for quick lookup.
	idSet := make(map[core.SnapshotID]bool, len(snaps))
	for _, s := range snaps {
		idSet[s.ID] = true
	}
	// For each snapshot, determine if it is preceded by another snapshot
	// in the list (i.e., another snapshot's PrevID points to it). If so,
	// that other snapshot is newer.
	isNewer := make(map[core.SnapshotID]int, len(snaps)) // depth from root
	for _, s := range snaps {
		depth := 0
		current := s
		for current.PrevID != nil && idSet[*current.PrevID] {
			depth++
			// Find the predecessor snapshot in the list.
			var pred *core.Snapshot
			for _, p := range snaps {
				if p.ID == *current.PrevID {
					pred = p
					break
				}
			}
			if pred == nil {
				break
			}
			current = pred
		}
		isNewer[s.ID] = depth
	}
	sort.SliceStable(snaps, func(i, j int) bool {
		if snaps[i].Timestamp != snaps[j].Timestamp {
			return snaps[i].Timestamp > snaps[j].Timestamp
		}
		// Higher depth = further from root = newer.
		return isNewer[snaps[i].ID] > isNewer[snaps[j].ID]
	})
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

// buildBranchMap assigns each snapshot to exactly one branch: the one whose
// tip is the *nearest* descendant (fewest PrevID hops). This matches the
// mental model "this commit belongs to the branch it was made on" — a fork
// point stays attributed to its original branch, and each snapshot shows at
// most one branch name in the log.
//
// A snapshot unreachable from any branch tip (orphaned) gets no entry.
func buildBranchMap(ctx context.Context, store storage.Storer) (map[string][]string, error) {
	branches, _, err := porcelain.ListBranches(ctx, store)
	if err != nil {
		return nil, err
	}

	// For each branch, walk its PrevID chain and record the hop distance from
	// the tip for every snapshot. distance[branchName][hashStr] = hops.
	type branchWalk struct {
		name string
		dist map[string]int
	}
	var walks []branchWalk
	for _, b := range branches {
		if b.Target.IsZero() {
			continue
		}
		name := strings.TrimPrefix(b.Name, "heads/")
		bw := branchWalk{name: name, dist: make(map[string]int)}
		currHash := b.Target
		hops := 0
		for !currHash.IsZero() {
			hashStr := currHash.String()
			if _, seen := bw.dist[hashStr]; seen {
				break // cycle guard
			}
			bw.dist[hashStr] = hops
			snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: currHash})
			if err != nil {
				break
			}
			if snap.PrevID == nil {
				break
			}
			currHash = snap.PrevID.Hash
			hops++
		}
		walks = append(walks, bw)
	}

	// For each snapshot, pick the branch with the smallest hop count.
	// Ties (a snapshot equidistant from two tips) are broken by branch name
	// for determinism — this only happens at a true fork point where two
	// branches share the same tip distance, which is rare.
	bestDist := make(map[string]int)
	bestName := make(map[string]string)
	for _, bw := range walks {
		for hashStr, d := range bw.dist {
			cur, ok := bestDist[hashStr]
			if !ok || d < cur || (d == cur && bw.name < bestName[hashStr]) {
				bestDist[hashStr] = d
				bestName[hashStr] = bw.name
			}
		}
	}
	result := make(map[string][]string)
	for hashStr, name := range bestName {
		result[hashStr] = []string{name}
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
