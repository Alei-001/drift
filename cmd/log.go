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
var logCompact bool
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
			snapshot := resolveSnapshot(store, logVerbose)
			if snapshot == nil {
				return fmt.Errorf("snapshot not found: %s", logVerbose)
			}

			timeStr := time.Unix(snapshot.Timestamp, 0).Format("2006-01-02 15:04:05")
			fmt.Printf("Snapshot %s  %s  %s\n", snapshot.ShortID(), timeStr, snapshot.Message)

			currentFiles := make(map[string]*core.FileEntry)
			for i := range snapshot.Files {
				currentFiles[snapshot.Files[i].Path] = &snapshot.Files[i]
			}

			var prevFiles map[string]*core.FileEntry
			if snapshot.PrevID != nil {
				prevSnap, err := store.GetSnapshot(*snapshot.PrevID)
				if err == nil {
					prevFiles = make(map[string]*core.FileEntry)
					for i := range prevSnap.Files {
						prevFiles[prevSnap.Files[i].Path] = &prevSnap.Files[i]
					}
				}
			}

			for path, entry := range currentFiles {
				if prevFiles != nil {
					if prevEntry, ok := prevFiles[path]; ok {
						if entry.Size != prevEntry.Size || !chunkHashesEqual(entry.Chunks, prevEntry.Chunks) {
							fmt.Printf("  %-40s M  %s\n", path, formatSize(entry.Size))
						}
					} else {
						fmt.Printf("  %-40s A  %s\n", path, formatSize(entry.Size))
					}
				} else {
					fmt.Printf("  %-40s    %s\n", path, formatSize(entry.Size))
				}
			}

			if prevFiles != nil {
				for path := range prevFiles {
					if _, ok := currentFiles[path]; !ok {
						fmt.Printf("  %-40s D\n", path)
					}
				}
			}
			return nil
		}

		opts := &storage.ListOptions{Limit: logLimit, Offset: 0}
		snapshots, err := store.ListSnapshots(opts)
		if err != nil {
			return err
		}

		if logCompact {
			for _, s := range snapshots {
				if !logAll && strings.HasPrefix(s.Message, "auto -") {
					continue
				}
				fmt.Printf("%s %s\n", s.ShortID(), s.Message)
			}
			return nil
		}

		if len(snapshots) == 0 {
			fmt.Println("No snapshots yet.")
			return nil
		}

		fmt.Printf("%-10s %-20s %-30s %s\n", "ID", "TIME", "MESSAGE", "CHANGES")
		fmt.Println(strings.Repeat("-", 80))
		for _, s := range snapshots {
			if !logAll && strings.HasPrefix(s.Message, "auto -") {
				continue
			}
			timeStr := time.Unix(s.Timestamp, 0).Format("2006-01-02 15:04:05")
			fmt.Printf("%-10s %-20s %-30s %d files\n", s.ShortID(), timeStr, s.Message, len(s.Files))
		}
		return nil
	},
}

func formatSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
}

func init() {
	logCmd.Flags().IntVarP(&logLimit, "limit", "l", 0, "limit number of entries")
	logCmd.Flags().StringVarP(&logVerbose, "verbose", "v", "", "show file details for a snapshot")
	logCmd.Flags().BoolVarP(&logCompact, "compact", "c", false, "compact output for scripting")
	logCmd.Flags().BoolVar(&logAll, "all", false, "include auto-saved snapshots")
	rootCmd.AddCommand(logCmd)
}
