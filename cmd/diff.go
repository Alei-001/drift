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
			diffSnapshots(store, cwd, snap1, snap2)
		}
		return nil
	},
}

func diffWorkspaceVsSnapshot(store storage.Storer, workDir string, snapshot *core.Snapshot) {
	snapFiles := make(map[string]*core.FileEntry)
	for i := range snapshot.Files {
		snapFiles[snapshot.Files[i].Path] = &snapshot.Files[i]
	}

	_ = fsutil.Walk(workDir, func(path string, info os.FileInfo) error {
		rel, _ := filepath.Rel(workDir, path)
		if info.IsDir() {
			return nil
		}

		snapEntry, exists := snapFiles[rel]
		if !exists {
			fmt.Printf("A %s\n", rel)
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read %s: %v\n", rel, err)
			return nil
		}

		if len(data) != int(snapEntry.Size) {
			fmt.Printf("M %s\n", rel)
			showFileDiff(store, rel, snapEntry, data)
		}
		delete(snapFiles, rel)
		return nil
	})

	for path := range snapFiles {
		fmt.Printf("D %s\n", path)
	}
}

func showFileDiff(store storage.Storer, path string, entry *core.FileEntry, newData []byte) {
	var oldData []byte
	for _, hash := range entry.Chunks {
		chunk, err := store.GetChunk(hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot get chunk %s: %v\n", hash.String(), err)
			continue
		}
		oldData = append(oldData, chunk.Data...)
	}

	header := newData
	if len(header) > 512 {
		header = header[:512]
	}
	engine := filetype.DetectEngine(path, header)
	if engine != nil && engine.Name() == "text" {
		diff, _ := engine.Diff("a/"+path, oldData, "b/"+path, newData)
		fmt.Println(diff)
	} else {
		fmt.Printf("  (binary file changed, %d -> %d bytes)\n", len(oldData), len(newData))
	}
}

func diffSnapshots(store storage.Storer, workDir string, snap1, snap2 *core.Snapshot) {
	_ = workDir // unused, kept for signature compatibility

	snap1Files := make(map[string]*core.FileEntry)
	for i := range snap1.Files {
		snap1Files[snap1.Files[i].Path] = &snap1.Files[i]
	}

	for i := range snap2.Files {
		entry2 := &snap2.Files[i]
		entry1, exists := snap1Files[entry2.Path]
		if !exists {
			fmt.Printf("A %s\n", entry2.Path)
			continue
		}

		// Compare hashes: if chunk list differs, file changed
		if entry1.Size != entry2.Size || !chunkHashesEqual(entry1.Chunks, entry2.Chunks) {
			fmt.Printf("M %s\n", entry2.Path)

			// Assemble both versions and show diff
			var data1, data2 []byte
			for _, h := range entry1.Chunks {
				chunk, err := store.GetChunk(h)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: cannot get chunk %s: %v\n", h.String(), err)
					continue
				}
				data1 = append(data1, chunk.Data...)
			}
			for _, h := range entry2.Chunks {
				chunk, err := store.GetChunk(h)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: cannot get chunk %s: %v\n", h.String(), err)
					continue
				}
				data2 = append(data2, chunk.Data...)
			}

			header := data2
			if len(header) > 512 {
				header = header[:512]
			}
			engine := filetype.DetectEngine(entry2.Path, header)
			if engine != nil && engine.Name() == "text" {
				diff, _ := engine.Diff("a/"+entry2.Path, data1, "b/"+entry2.Path, data2)
				fmt.Println(diff)
			} else {
				fmt.Printf("  (binary file changed, %d -> %d bytes)\n", len(data1), len(data2))
			}
		}
		delete(snap1Files, entry2.Path)
	}

	for path := range snap1Files {
		fmt.Printf("D %s\n", path)
	}
}

func chunkHashesEqual(a, b []core.Hash) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

	if entry1 == nil && entry2 != nil {
		fmt.Printf("A %s\n", filePath)
		return
	}
	if entry1 != nil && entry2 == nil {
		fmt.Printf("D %s\n", filePath)
		return
	}
	if entry1.Size != entry2.Size || !chunkHashesEqual(entry1.Chunks, entry2.Chunks) {
		var data1, data2 []byte
		for _, h := range entry1.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot get chunk %s: %v\n", h.String(), err)
				continue
			}
			data1 = append(data1, chunk.Data...)
		}
		for _, h := range entry2.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot get chunk %s: %v\n", h.String(), err)
				continue
			}
			data2 = append(data2, chunk.Data...)
		}
		showFileDiff(store, filePath, entry1, data2)
	}
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
