package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/filetype"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/filesystem"
	"github.com/your-org/drift/util/fsutil"
)

var diffCmd = &cobra.Command{
	Use:   "diff [<id1>] [<id2>] [<file>]",
	Short: "Show changes between snapshots or workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		if len(args) == 0 {
			headRef, err := store.GetRef("HEAD")
			if err != nil {
				return err
			}
			headSnapshot, err := store.GetSnapshot(core.SnapshotID{Hash: headRef.Target})
			if err != nil {
				return err
			}
			diffWorkspaceVsSnapshot(store, cwd, headSnapshot)
		} else if len(args) == 1 {
			snap1 := resolveSnapshot(store, args[0])
			if snap1 == nil {
				return fmt.Errorf("snapshot not found: %s", args[0])
			}
			diffWorkspaceVsSnapshot(store, cwd, snap1)
		} else if len(args) == 3 {
			snap1 := resolveSnapshot(store, args[0])
			if snap1 == nil {
				return fmt.Errorf("snapshot not found: %s", args[0])
			}
			snap2 := resolveSnapshot(store, args[1])
			if snap2 == nil {
				return fmt.Errorf("snapshot not found: %s", args[1])
			}
			filePath := args[2]
			diffFileInSnapshots(store, snap1, snap2, filePath)
		} else {
			snap1 := resolveSnapshot(store, args[0])
			if snap1 == nil {
				return fmt.Errorf("snapshot not found: %s", args[0])
			}
			snap2 := resolveSnapshot(store, args[1])
			if snap2 == nil {
				return fmt.Errorf("snapshot not found: %s", args[1])
			}
			diffSnapshots(store, snap1, snap2)
		}
		return nil
	},
}

func diffWorkspaceVsSnapshot(store storage.Storer, workDir string, snapshot *core.Snapshot) {
	snapFiles := make(map[string]*core.FileEntry)
	for i := range snapshot.Files {
		snapFiles[snapshot.Files[i].Path] = &snapshot.Files[i]
	}

	var added, modified []string
	var deleted []string

	_ = fsutil.Walk(workDir, func(path string, info os.FileInfo) error {
		rel, _ := filepath.Rel(workDir, path)
		if info.IsDir() {
			return nil
		}

		snapEntry, exists := snapFiles[rel]
		if !exists {
			added = append(added, rel)
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read %s: %v\n", rel, err)
			return nil
		}

		if len(data) != int(snapEntry.Size) {
			modified = append(modified, rel)
		}
		delete(snapFiles, rel)
		return nil
	})

	for path := range snapFiles {
		deleted = append(deleted, path)
	}

	fmt.Printf(">>> Diff workspace → %s\n", snapshot.ShortID())
	fmt.Println()
	for _, p := range added {
		fmt.Printf("  +  %s\n", p)
	}
	for _, p := range modified {
		fmt.Printf("  ~  %s\n", p)
	}
	for _, p := range deleted {
		fmt.Printf("  -  %s\n", p)
	}
	total := len(added) + len(modified) + len(deleted)
	fmt.Printf("\n  %d files: +%d ~%d -%d\n", total, len(added), len(modified), len(deleted))
}

func diffSnapshots(store storage.Storer, snap1, snap2 *core.Snapshot) {
	snap1Files := make(map[string]*core.FileEntry)
	for i := range snap1.Files {
		snap1Files[snap1.Files[i].Path] = &snap1.Files[i]
	}

	var added, modified, deleted []string

	for i := range snap2.Files {
		entry2 := &snap2.Files[i]
		entry1, exists := snap1Files[entry2.Path]
		if !exists {
			added = append(added, entry2.Path)
			continue
		}

		if entry1.Size != entry2.Size || !chunkHashesEqual(entry1.Chunks, entry2.Chunks) {
			modified = append(modified, entry2.Path)
		}
		delete(snap1Files, entry2.Path)
	}

	for path := range snap1Files {
		deleted = append(deleted, path)
	}

	fmt.Printf(">>> Diff %s → %s\n", snap1.ShortID(), snap2.ShortID())
	fmt.Println()
	for _, p := range added {
		fmt.Printf("  +  %s\n", p)
	}
	for _, p := range modified {
		fmt.Printf("  ~  %s\n", p)
	}
	for _, p := range deleted {
		fmt.Printf("  -  %s\n", p)
	}
	total := len(added) + len(modified) + len(deleted)
	fmt.Printf("\n  %d files: +%d ~%d -%d\n", total, len(added), len(modified), len(deleted))
}

func diffFileInSnapshots(store storage.Storer, snap1, snap2 *core.Snapshot, filePath string) {
	var entry1, entry2 *core.FileEntry
	for i := range snap1.Files {
		if snap1.Files[i].Path == filePath {
			entry1 = &snap1.Files[i]
			break
		}
	}
	for i := range snap2.Files {
		if snap2.Files[i].Path == filePath {
			entry2 = &snap2.Files[i]
			break
		}
	}

	fmt.Printf(">>> Diff %s → %s %s\n", snap1.ShortID(), snap2.ShortID(), filePath)

	if entry1 == nil && entry2 != nil {
		fmt.Printf("  +  %s  (added)\n", filePath)
		return
	}
	if entry1 != nil && entry2 == nil {
		fmt.Printf("  -  %s  (deleted)\n", filePath)
		return
	}
	if entry1.Size != entry2.Size || !chunkHashesEqual(entry1.Chunks, entry2.Chunks) {
		var data1, data2 []byte
		for _, h := range entry1.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				continue
			}
			data1 = append(data1, chunk.Data...)
		}
		for _, h := range entry2.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				continue
			}
			data2 = append(data2, chunk.Data...)
		}

		header := data2
		if len(header) > 512 {
			header = header[:512]
		}
		engine := filetype.DetectEngine(filePath, header)
		if engine != nil && engine.Name() == "text" {
			diff, _ := engine.Diff(snap1.ShortID()+"/"+filePath, data1, snap2.ShortID()+"/"+filePath, data2)
			fmt.Println(diff)
		} else {
			fmt.Printf("  Size:       %s → %s (+%s)\n",
				formatSize(entry1.Size), formatSize(entry2.Size),
				formatSize(entry2.Size-entry1.Size))
			fmt.Println("\n  (binary file — metadata only)")
		}
	}
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
