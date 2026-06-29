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
	"github.com/your-org/drift/util/fsutil"
	"github.com/your-org/drift/util/pathutil"
	"github.com/zeebo/blake3"
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
		defer store.Close()

		if len(args) == 0 {
			snap := resolveHeadSnapshot(store)
			if snap == nil {
				statusFailed("Diff", "no snapshot to compare against.", "use 'drift save -m \"message\"' to create one first.")
				return fmt.Errorf("no HEAD snapshot")
			}
			diffWorkspaceVsSnapshot(store, cwd, snap)
		} else if len(args) == 1 {
			// Try as snapshot ID first
			snap1 := resolveSnapshot(store, args[0])
			if snap1 != nil {
				diffWorkspaceVsSnapshot(store, cwd, snap1)
				return nil
			}
			// Fall back: treat as file path, compare with HEAD
			headSnap := resolveHeadSnapshot(store)
			if headSnap == nil {
				statusFailed("Diff", "no snapshot to compare against.", "use 'drift save -m \"message\"' to create one first.")
				return fmt.Errorf("no HEAD snapshot")
			}
			return diffWorkspaceFileVsSnapshot(store, cwd, headSnap, args[0])
		} else if len(args) == 3 {
			snap1 := resolveSnapshot(store, args[0])
			if snap1 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
				return fmt.Errorf("snapshot not found: %s", args[0])
			}
			snap2 := resolveSnapshot(store, args[1])
			if snap2 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[1]), "use 'drift log' to list available snapshots.")
				return fmt.Errorf("snapshot not found: %s", args[1])
			}
			filePath := args[2]
			diffFileInSnapshots(store, cwd, snap1, snap2, filePath)
		} else {
			snap1 := resolveSnapshot(store, args[0])
			if snap1 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
				return fmt.Errorf("snapshot not found: %s", args[0])
			}
			snap2 := resolveSnapshot(store, args[1])
			if snap2 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[1]), "use 'drift log' to list available snapshots.")
				return fmt.Errorf("snapshot not found: %s", args[1])
			}
			diffSnapshots(store, snap1, snap2)
		}
		return nil
	},
}

// resolveHeadSnapshot returns the HEAD snapshot, or nil if none exists.
func resolveHeadSnapshot(store storage.Storer) *core.Snapshot {
	headRef, err := store.GetRef("HEAD")
	if err != nil {
		return nil
	}
	if headRef.Target.IsZero() {
		return nil
	}
	snap, err := store.GetSnapshot(core.SnapshotID{Hash: headRef.Target})
	if err != nil {
		return nil
	}
	return snap
}

func diffWorkspaceVsSnapshot(store storage.Storer, workDir string, snapshot *core.Snapshot) {
	snapFiles := make(map[string]*core.FileEntry)
	for i := range snapshot.Files {
		snapFiles[snapshot.Files[i].Path] = &snapshot.Files[i]
	}

	var added, modified []string
	var deleted []string

	_ = fsutil.Walk(workDir, func(path string, info os.FileInfo) error {
		rel, _ := pathutil.Rel(workDir, path)
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
		} else {
			// Same size: compare content hash
			workHash := blake3.Sum256(data)
			var workCoreHash core.Hash
			copy(workCoreHash[:], workHash[:])
			if workCoreHash != snapEntry.Hash {
				modified = append(modified, rel)
			}
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

// diffWorkspaceFileVsSnapshot shows content-level diff for a single file: workspace vs snapshot.
func diffWorkspaceFileVsSnapshot(store storage.Storer, workDir string, snapshot *core.Snapshot, filePath string) error {
	// Normalize path and resolve absolute paths
	filePath, err := pathutil.RelToWorkDir(workDir, filePath)
	if err != nil {
		return fmt.Errorf("cannot resolve path: %w", err)
	}

	// Find the file in the snapshot
	var snapEntry *core.FileEntry
	for i := range snapshot.Files {
		if snapshot.Files[i].Path == filePath {
			snapEntry = &snapshot.Files[i]
			break
		}
	}

	fmt.Printf(">>> Diff %s → workspace %s\n", snapshot.ShortID(), filePath)

	// Read workspace file
	fullPath := filepath.Join(workDir, filePath)
	workData, workErr := os.ReadFile(fullPath)

	if snapEntry == nil {
		// File not in snapshot: added in workspace
		if workErr != nil {
			fmt.Fprintf(os.Stderr, "  hint: '%s' not found in snapshot or workspace.\n", filePath)
			return nil
		}
		fmt.Printf("  +  %s  (new file, %s)\n", filePath, formatSize(int64(len(workData))))
		return nil
	}

	if workErr != nil {
		// File was in snapshot but deleted from workspace
		fmt.Printf("  -  %s  (deleted, was %s)\n", filePath, formatSize(snapEntry.Size))
		return nil
	}

	// Both exist: compare content
	var snapData []byte

	if int64(len(workData)) != snapEntry.Size {
		goto assemble
	}

	// Same size: BLAKE3 hash comparison (low memory, no chunk assembly)
	{
		workHash := blake3.Sum256(workData)
		snapHasher := blake3.New()
		for _, h := range snapEntry.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				return fmt.Errorf("read chunk %s for %s: %w", h.String(), filePath, err)
			}
			snapHasher.Write(chunk.Data)
		}
		var snapHash [32]byte
		snapHasher.Sum(snapHash[:0])

		if workHash == snapHash {
			fmt.Printf("  (no change)\n")
			return nil
		}
	}

assemble:
	// Assemble snapshot data for diff
	for _, h := range snapEntry.Chunks {
		chunk, err := store.GetChunk(h)
		if err != nil {
			return fmt.Errorf("read chunk %s for %s: %w", h.String(), filePath, err)
		}
		snapData = append(snapData, chunk.Data...)
	}

	header := workData
	if len(header) > 512 {
		header = header[:512]
	}
	engine := filetype.DetectEngine(filePath, header)
	if engine != nil && engine.Name() == "text" {
		diff, _ := engine.Diff(snapshot.ShortID()+"/"+filePath, snapData, "workspace/"+filePath, workData)
		fmt.Println()
		fmt.Println(diff)
	} else {
		fmt.Printf("  Size:       %s → %s (%+s)\n",
			formatSize(snapEntry.Size), formatSize(int64(len(workData))),
			formatSize(int64(len(workData))-snapEntry.Size))
		fmt.Println("\n  (binary file — metadata only)")
	}
	return nil
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

func diffFileInSnapshots(store storage.Storer, workDir string, snap1, snap2 *core.Snapshot, filePath string) {
	filePath, err := pathutil.RelToWorkDir(workDir, filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot resolve path: %v\n", err)
		return
	}

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
	if entry1 == nil && entry2 == nil {
		fmt.Fprintf(os.Stderr, "  warning: '%s' not found in either snapshot.\n", filePath)
		return
	}
	if entry1.Size != entry2.Size || !chunkHashesEqual(entry1.Chunks, entry2.Chunks) {
		var data1, data2 []byte
		for _, h := range entry1.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: cannot read chunk %s: %v\n", h.String(), err)
				continue
			}
			data1 = append(data1, chunk.Data...)
		}
		for _, h := range entry2.Chunks {
			chunk, err := store.GetChunk(h)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: cannot read chunk %s: %v\n", h.String(), err)
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
