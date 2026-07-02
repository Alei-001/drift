package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/filetype"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/stream"
	"github.com/your-org/drift/util/fsutil"
	"github.com/your-org/drift/util/pathutil"
)

var diffCmd = &cobra.Command{
	Use:   "diff [<id1>] [<id2>] [<file>]",
	Short: "Show changes between snapshots or workspace",
	Args:  cobra.MaximumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Diff", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if len(args) == 0 {
			snap := resolveHeadSnapshot(ctx, store)
			if snap == nil {
				statusFailed("Diff", "no snapshot to compare against.", "use 'drift save -m \"message\"' to create one first.")
				return ErrSilent
			}
			return diffWorkspaceVsSnapshot(store, cwd, snap, &cfg.Core)
		} else if len(args) == 1 {
			// Try as snapshot ID first
			snap1 := resolveSnapshot(ctx, store, args[0])
			if snap1 != nil {
				return diffWorkspaceVsSnapshot(store, cwd, snap1, &cfg.Core)
			}
			// Fall back: treat as file path, compare with HEAD
			headSnap := resolveHeadSnapshot(ctx, store)
			if headSnap == nil {
				statusFailed("Diff", "no snapshot to compare against.", "use 'drift save -m \"message\"' to create one first.")
				return ErrSilent
			}
			return diffWorkspaceFileVsSnapshot(ctx, store, cwd, headSnap, args[0])
		} else if len(args) == 3 {
			snap1 := resolveSnapshot(ctx, store, args[0])
			if snap1 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
				return ErrSilent
			}
			snap2 := resolveSnapshot(ctx, store, args[1])
			if snap2 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[1]), "use 'drift log' to list available snapshots.")
				return ErrSilent
			}
			filePath := args[2]
			diffFileInSnapshots(ctx, store, cwd, snap1, snap2, filePath)
		} else {
			snap1 := resolveSnapshot(ctx, store, args[0])
			if snap1 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
				return ErrSilent
			}
			snap2 := resolveSnapshot(ctx, store, args[1])
			if snap2 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[1]), "use 'drift log' to list available snapshots.")
				return ErrSilent
			}
			diffSnapshots(store, snap1, snap2)
		}
		return nil
	},
}

// resolveHeadSnapshot returns the HEAD snapshot, or nil if none exists.
func resolveHeadSnapshot(ctx context.Context, store storage.Storer) *core.Snapshot {
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return nil
	}
	if headRef.Target.IsZero() {
		return nil
	}
	snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: headRef.Target})
	if err != nil {
		return nil
	}
	return snap
}

func diffWorkspaceVsSnapshot(store storage.Storer, workDir string, snapshot *core.Snapshot, cfg *core.CoreConfig) error {
	snapFiles := make(map[string]*core.FileEntry)
	for i := range snapshot.Files {
		snapFiles[snapshot.Files[i].Path] = &snapshot.Files[i]
	}

	var added, modified []string
	var deleted []string

	err := fsutil.Walk(workDir, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		rel, err := pathutil.Rel(workDir, path)
		if err != nil {
			return fmt.Errorf("resolve relative path %s: %w", path, err)
		}
		if info.IsDir() {
			return nil
		}

		snapEntry, exists := snapFiles[rel]
		if !exists {
			added = append(added, rel)
			return nil
		}

		if info.Size() != snapEntry.Size {
			modified = append(modified, rel)
		} else {
			workHash, hashErr := porcelain.ComputeFileHash(path, cfg)
			if hashErr != nil || workHash != snapEntry.Hash {
				modified = append(modified, rel)
			}
		}
		delete(snapFiles, rel)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk workspace: %w", err)
	}

	for path := range snapFiles {
		deleted = append(deleted, path)
	}

	fmt.Printf(">>> Diff workspace → %s\n", snapshot.ShortID())
	total := len(added) + len(modified) + len(deleted)
	if total == 0 {
		fmt.Println()
		fmt.Println("  No changes.")
		return nil
	}
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
	summaryLine(total, len(added), len(modified), len(deleted))
	return nil
}

// diffWorkspaceFileVsSnapshot shows content-level diff for a single file: workspace vs snapshot.
// The workspace file is opened with os.Open and streamed through the engine,
// and snapshot content is streamed from chunks via a chunkReader, so the file
// bytes are never buffered whole in memory.
func diffWorkspaceFileVsSnapshot(ctx context.Context, store storage.Storer, workDir string, snapshot *core.Snapshot, filePath string) error {
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

	fullPath := filepath.Join(workDir, filePath)
	info, statErr := os.Stat(fullPath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("stat %s: %w", fullPath, statErr)
	}

	if snapEntry == nil {
		// File not in snapshot: added in workspace
		if os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "  hint: '%s' not found in snapshot or workspace.\n", filePath)
			return nil
		}
		fmt.Printf("  +  %s  (new file, %s)\n", filePath, formatSize(info.Size()))
		return nil
	}

	if os.IsNotExist(statErr) {
		// File was in snapshot but deleted from workspace
		fmt.Printf("  -  %s  (deleted, was %s)\n", filePath, formatSize(snapEntry.Size))
		return nil
	}

	// Both exist. If sizes match, short-circuit on a streaming BLAKE3 hash
	// comparison (the workspace file is hashed by streaming, not by reading
	// it whole into memory).
	if info.Size() == snapEntry.Size {
		workHash, err := stream.HashFileContent(fullPath)
		if err != nil {
			return fmt.Errorf("hash %s: %w", fullPath, err)
		}
		snapHash, err := stream.HashChunkData(ctx, store, snapEntry.Chunks)
		if err != nil {
			return fmt.Errorf("hash snapshot chunks for %s: %w", filePath, err)
		}
		if workHash == snapHash {
			fmt.Printf("  (no change)\n")
			return nil
		}
	}

	// Content differs: open the workspace file and run a streaming engine diff.
	workFile, err := os.Open(fullPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", fullPath, err)
	}
	defer workFile.Close()

		header, workReader, err := stream.PeekHeader(workFile, core.HeaderPeekSize)
	if err != nil {
		return fmt.Errorf("read header %s: %w", fullPath, err)
	}
	engine := filetype.DetectEngine(filePath, header)

	snapReader := stream.NewChunkReader(ctx, store, snapEntry.Chunks)
	oldLabel := snapshot.ShortID() + "/" + filePath
	newLabel := "workspace/" + filePath

	if engine != nil && engine.Name() == "text" {
		diff, diffErr := engine.Diff(oldLabel, snapReader, newLabel, workReader)
		if diffErr != nil {
			return fmt.Errorf("diff %s: %w", filePath, diffErr)
		}
		fmt.Println()
		fmt.Println(diff)
	} else {
		fmt.Printf("  Size:       %s → %s (%+s)\n",
			formatSize(snapEntry.Size), formatSize(info.Size()),
			formatSize(info.Size()-snapEntry.Size))
		snapHeader, _, err := stream.PeekHeader(snapReader, core.HeaderPeekSize)
		if err != nil {
			return fmt.Errorf("read snapshot header %s: %w", filePath, err)
		}
		oldDims := imageDimensions(snapHeader)
		newDims := imageDimensions(header)
		if oldDims != "" || newDims != "" {
			fmt.Printf("  Dimensions: %s → %s\n", oldDims, newDims)
		}
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

		if entry1.Size != entry2.Size || !slices.Equal(entry1.Chunks, entry2.Chunks) {
			modified = append(modified, entry2.Path)
		}
		delete(snap1Files, entry2.Path)
	}

	for path := range snap1Files {
		deleted = append(deleted, path)
	}

	fmt.Printf(">>> Diff %s → %s\n", snap1.ShortID(), snap2.ShortID())
	total := len(added) + len(modified) + len(deleted)
	if total == 0 {
		fmt.Println()
		fmt.Println("  No changes.")
		return
	}
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
	summaryLine(total, len(added), len(modified), len(deleted))
}

func diffFileInSnapshots(ctx context.Context, store storage.Storer, workDir string, snap1, snap2 *core.Snapshot, filePath string) {
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
	if entry1.Size == entry2.Size && slices.Equal(entry1.Chunks, entry2.Chunks) {
		fmt.Println("  (no change)")
		return
	}

	// Stream both snapshot versions from chunks; peek a header from each for
	// engine detection and dimension parsing without buffering the file.
	reader1 := stream.NewChunkReader(ctx, store, entry1.Chunks)
	reader2 := stream.NewChunkReader(ctx, store, entry2.Chunks)
	header1, fullReader1, err := stream.PeekHeader(reader1, core.HeaderPeekSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: cannot read chunk for %s: %v\n", filePath, err)
		return
	}
	header2, fullReader2, err := stream.PeekHeader(reader2, core.HeaderPeekSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: cannot read chunk for %s: %v\n", filePath, err)
		return
	}
	engine := filetype.DetectEngine(filePath, header2)
	if engine != nil && engine.Name() == "text" {
		diff, diffErr := engine.Diff(snap1.ShortID()+"/"+filePath, fullReader1, snap2.ShortID()+"/"+filePath, fullReader2)
		if diffErr != nil {
			fmt.Fprintf(os.Stderr, "  warning: cannot diff %s: %v\n", filePath, diffErr)
			return
		}
		fmt.Println(diff)
	} else {
		sizeDiff := formatSize(entry2.Size - entry1.Size)
		sign := "+"
		if entry2.Size < entry1.Size {
			sign = ""
		}
		fmt.Printf("  Size:       %s → %s (%s%s)\n",
			formatSize(entry1.Size), formatSize(entry2.Size), sign, sizeDiff)
		oldDims := imageDimensions(header1)
		newDims := imageDimensions(header2)
		if oldDims != "" || newDims != "" {
			fmt.Printf("  Dimensions: %s → %s\n", oldDims, newDims)
		}
		fmt.Println("\n  (binary file — metadata only)")
	}
}

func init() {
	rootCmd.AddCommand(diffCmd)
}


