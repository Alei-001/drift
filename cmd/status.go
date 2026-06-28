package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage/filesystem"
	"github.com/your-org/drift/util/fsutil"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show working tree status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		index, err := store.GetIndex()
		if err != nil {
			return err
		}

		workspaceFiles := make(map[string]os.FileInfo)
		_ = fsutil.Walk(cwd, func(path string, info os.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(cwd, path)
			rel = filepath.ToSlash(rel)
			workspaceFiles[rel] = info
			return nil
		})

		type change struct {
			status string
			path   string
		}
		var changes []change

		printed := make(map[string]bool)
		for _, entry := range index.Entries {
			if info, ok := workspaceFiles[entry.Path]; ok {
				if info.Size() != entry.Size || info.ModTime().Unix() != entry.ModTime {
					changes = append(changes, change{"M", entry.Path})
				}
				printed[entry.Path] = true
			} else {
				changes = append(changes, change{"D", entry.Path})
				printed[entry.Path] = true
			}
		}

		for path := range workspaceFiles {
			if !printed[path] {
				changes = append(changes, change{"A", path})
			}
		}

		if len(changes) == 0 {
			fmt.Println("Nothing to save, working tree clean.")
			return nil
		}

		headRef, err := store.GetRef("HEAD")
		if err != nil || headRef.Target.IsZero() {
			fmt.Printf("%d files changed:\n", len(changes))
		} else {
			headSnapshot, err := store.GetSnapshot(core.SnapshotID{Hash: headRef.Target})
			if err != nil {
				fmt.Printf("%d files changed:\n", len(changes))
			} else {
				since := time.Since(time.Unix(headSnapshot.Timestamp, 0))
				fmt.Printf("Changes since last save (%s ago), %d files changed:\n", formatDuration(since), len(changes))
			}
		}

		for _, c := range changes {
			fmt.Printf("%s %s\n", c.status, c.path)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d second(s)", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%d minute(s)", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%d hour(s)", int(d.Hours()))
	}
	return fmt.Sprintf("%d day(s)", int(d.Hours()/24))
}
