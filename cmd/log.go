package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/filesystem"
)

var logLimit int
var logVerbose string
var logJSON bool
var logAll bool

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show snapshot history",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		if logVerbose != "" {
			return logVerboseMode(store, logVerbose)
		}

		opts := &storage.ListOptions{Limit: logLimit, Offset: 0}
		snapshots, err := store.ListSnapshots(opts)
		if err != nil {
			return err
		}

		// Filter auto-saves unless --all
		var filtered []*core.Snapshot
		for _, s := range snapshots {
			if !logAll && strings.HasPrefix(s.Message, "auto -") {
				continue
			}
			filtered = append(filtered, s)
		}

		if len(filtered) == 0 {
			statusFailed("Log", "no snapshots yet.", "use 'drift save -m \"message\"' to create your first snapshot.")
			return fmt.Errorf("no snapshots")
		}

		if logJSON {
			return logJSONMode(store, filtered)
		}

		// Default table format
		label := fmt.Sprintf("History (%d snapshots)", len(filtered))
		if logAll {
			label = fmt.Sprintf("History (%d snapshots, including auto-saves)", len(filtered))
		}
		fmt.Printf(">>> %s\n", label)
		for _, s := range filtered {
			timeStr := time.Unix(s.Timestamp, 0).Format("2006-01-02 15:04")
			add, mod, del := countSnapshotChanges(store, s)
			changes := fmt.Sprintf("+%d ~%d", add, mod)
			if del > 0 {
				changes += fmt.Sprintf(" -%d", del)
			}

			msg := s.Message
			if len(msg) > 28 {
				msg = msg[:27] + "…"
			}

			tag := ""
			for _, t := range s.Tags {
				if t != "" {
					tag = t
					break
				}
			}
			if tag != "" {
				if len(tag) > 10 {
					tag = tag[:9] + "…"
				}
				tag = "[" + tag + "]"
			}

			suffix := ""
			if strings.HasPrefix(s.Message, "auto -") {
				suffix = "    · dimmed"
			}
			fmt.Printf("%s  %s  %-28s  %-12s  %s%s\n", s.ShortID(), timeStr, msg, tag, changes, suffix)
		}
		return nil
	},
}

func logVerboseMode(store storage.Storer, id string) error {
	snapshot := resolveSnapshot(store, id)
	if snapshot == nil {
		return fmt.Errorf("snapshot not found: %s", id)
	}

	fmt.Printf(">>> Snapshot %s\n", snapshot.ShortID())
	timeStr := time.Unix(snapshot.Timestamp, 0).Format("2006-01-02 15:04")
	fmt.Printf("%s  %s\n", timeStr, snapshot.Message)

	add, mod, del := computeVerboseChanges(store, snapshot)
	printFileListWithLineCount(add, mod, del, store)
	total := len(add) + len(mod) + len(del)
	summaryLine(total, len(add), len(mod), len(del))
	return nil
}

func computeVerboseChanges(store storage.Storer, snapshot *core.Snapshot) (added, modified []core.FileEntry, deleted []string) {
	currFiles := make(map[string]core.FileEntry)
	for _, f := range snapshot.Files {
		currFiles[f.Path] = f
	}

	var prevFiles map[string]core.FileEntry
	if snapshot.PrevID != nil {
		prevSnap, err := store.GetSnapshot(*snapshot.PrevID)
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

func countSnapshotChanges(store storage.Storer, snapshot *core.Snapshot) (added, modified, deleted int) {
	a, m, d := computeVerboseChanges(store, snapshot)
	return len(a), len(m), len(d)
}

func logJSONMode(store storage.Storer, snapshots []*core.Snapshot) error {
	fmt.Printf(">>> History (%d)\n", len(snapshots))
	fmt.Println("[")
	for i, s := range snapshots {
		add, mod, del := countSnapshotChanges(store, s)
		tag := "null"
		if len(s.Tags) > 0 && s.Tags[0] != "" {
			tag = `"` + s.Tags[0] + `"`
		}
		changes := fmt.Sprintf("+%d ~%d -%d", add, mod, del)
		comma := ","
		if i == len(snapshots)-1 {
			comma = ""
		}
		fmt.Printf(`  {"id":"%s","time":"%s","message":"%s","tag":%s,"changes":"%s"}%s`,
			s.ShortID(),
			time.Unix(s.Timestamp, 0).Format("2006-01-02T15:04:05"),
			s.Message,
			tag,
			changes,
			comma)
		fmt.Println()
	}
	fmt.Println("]")
	return nil
}

func init() {
	logCmd.Flags().IntVarP(&logLimit, "limit", "l", 0, "limit number of entries")
	logCmd.Flags().StringVarP(&logVerbose, "verbose", "v", "", "show file details for a snapshot")
	logCmd.Flags().BoolVar(&logJSON, "json", false, "output in JSON format for scripting")
	logCmd.Flags().BoolVar(&logAll, "all", false, "include auto-saved snapshots")
	rootCmd.AddCommand(logCmd)
}
