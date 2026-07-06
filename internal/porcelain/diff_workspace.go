package porcelain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/stream"
	"github.com/your-org/drift/internal/util/format"
	"github.com/your-org/drift/internal/util/fsutil"
	"github.com/your-org/drift/internal/util/pathutil"
)

// Note: the functions in this file mix business logic with console output
// (fmt.Printf). This is an interim state: the logic was moved here from
// cmd/diff.go to enforce the "cmd has no business logic" rule, but the
// output formatting has not yet been separated from the diff computation.
// A future refactor should return structured diff results and let cmd
// handle all rendering.

// DiffWorkspaceVsSnapshot prints a workspace-vs-snapshot diff: files added,
// modified, or deleted relative to the snapshot. The workspace is walked
// once; each file is classified by size (and by content hash on size match).
// The status line is emitted by the caller; this function prints only the
// file list and summary line.
func DiffWorkspaceVsSnapshot(store storage.Storer, workDir string, snapshot *core.Snapshot, cfg *core.CoreConfig) error {
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
			workHash, hashErr := ComputeFileHash(path, cfg)
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
	fmt.Printf("\n  %s\n", formatDiffSummary(total, len(added), len(modified), len(deleted)))
	return nil
}

// DiffWorkspaceFileVsSnapshot shows content-level diff for a single file:
// workspace vs snapshot. The workspace file is opened with os.Open and
// streamed through the engine, and snapshot content is streamed from chunks
// via a chunkReader, so the file bytes are never buffered whole in memory.
// The status line is emitted by the caller; this function prints only the
// diff content.
func DiffWorkspaceFileVsSnapshot(ctx context.Context, store storage.Storer, workDir string, snapshot *core.Snapshot, filePath string) error {
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
		fmt.Printf("  +  %s  (new file, %s)\n", filePath, format.Bytes(info.Size()))
		return nil
	}

	if os.IsNotExist(statErr) {
		// File was in snapshot but deleted from workspace
		fmt.Printf("  -  %s  (deleted, was %s)\n", filePath, format.Bytes(snapEntry.Size))
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
		fmt.Println(diff)
	} else {
		fmt.Printf("  Size:       %s -> %s (%+s)\n",
			format.Bytes(snapEntry.Size), format.Bytes(info.Size()),
			format.Bytes(info.Size()-snapEntry.Size))
		snapHeader, _, err := stream.PeekHeader(snapReader, core.HeaderPeekSize)
		if err != nil {
			return fmt.Errorf("read snapshot header %s: %w", filePath, err)
		}
		oldDims := format.ImageDimensions(snapHeader)
		newDims := format.ImageDimensions(header)
		if oldDims != "" || newDims != "" {
			fmt.Printf("  Dimensions: %s -> %s\n", oldDims, newDims)
		}
		fmt.Println("\n  (binary file — metadata only)")
	}
	return nil
}
