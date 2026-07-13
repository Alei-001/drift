package porcelain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/filetype"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/storage/stream"
	"github.com/Alei-001/drift/internal/util/format"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/Alei-001/drift/internal/util/pathutil"
)

// DiffWorkspaceVsSnapshot computes a workspace-vs-snapshot diff: files
// added, modified, or deleted relative to the snapshot. The workspace is
// walked once; each file is classified by size (and by content hash on
// size match). It returns the classified file lists without printing; the
// caller renders the result.
func DiffWorkspaceVsSnapshot(ctx context.Context, workDir string, snapshot *core.Snapshot, cfg *core.CoreConfig) (FileDiffResult, error) {
	snapFiles := make(map[string]*core.FileEntry)
	for i := range snapshot.Files {
		snapFiles[snapshot.Files[i].Path] = &snapshot.Files[i]
	}

	var result FileDiffResult
	err := fsutil.Walk(workDir, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := pathutil.Rel(workDir, path)
		if err != nil {
			return fmt.Errorf("resolve relative path %s: %w", path, err)
		}
		if info.IsDir() {
			return nil
		}
		// Skip symlinks: they are not tracked by snapshots, so a
		// symlink would always appear as "added" in the diff. Skipping
		// keeps the diff focused on real file changes.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		snapEntry, exists := snapFiles[rel]
		if !exists {
			result.Added = append(result.Added, rel)
			return nil
		}

		if info.Size() != snapEntry.Size {
			result.Modified = append(result.Modified, rel)
		} else {
			workHash, hashErr := ComputeFileHash(path)
			if hashErr != nil || workHash != snapEntry.Hash {
				result.Modified = append(result.Modified, rel)
			}
		}
		delete(snapFiles, rel)
		return nil
	})
	if err != nil {
		return FileDiffResult{}, fmt.Errorf("walk workspace: %w", err)
	}

	for path := range snapFiles {
		result.Deleted = append(result.Deleted, path)
	}
	return result, nil
}

// DiffWorkspaceFileVsSnapshot computes a content-level diff for a single
// file: workspace vs snapshot. The workspace file is opened with os.Open
// and streamed through the engine, and snapshot content is streamed from
// chunks via a chunkReader, so the file bytes are never buffered whole in
// memory. It returns the diff content (and any hint) without printing;
// the caller renders the result.
func DiffWorkspaceFileVsSnapshot(ctx context.Context, store storage.Storer, workDir string, snapshot *core.Snapshot, filePath string) (ContentDiffResult, error) {
	// Normalize path and resolve absolute paths
	filePath, err := pathutil.RelToWorkDir(workDir, filePath)
	if err != nil {
		return ContentDiffResult{}, fmt.Errorf("cannot resolve path: %w", err)
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
		return ContentDiffResult{}, fmt.Errorf("stat %s: %w", fullPath, statErr)
	}

	if snapEntry == nil {
		// File not in snapshot: added in workspace
		if os.IsNotExist(statErr) {
			return ContentDiffResult{Stderr: fmt.Sprintf("  hint: '%s' not found in snapshot or workspace.\n", filePath)}, nil
		}
		return ContentDiffResult{
			Stdout:  fmt.Sprintf("  +  %s  (new file, %s)\n", filePath, format.Bytes(info.Size())),
			Kind:    "added",
			NewSize: info.Size(),
		}, nil
	}

	if os.IsNotExist(statErr) {
		// File was in snapshot but deleted from workspace
		return ContentDiffResult{
			Stdout:  fmt.Sprintf("  -  %s  (deleted, was %s)\n", filePath, format.Bytes(snapEntry.Size)),
			Kind:    "deleted",
			OldSize: snapEntry.Size,
		}, nil
	}

	// Both exist. If sizes match, short-circuit on a streaming BLAKE3 hash
	// comparison (the workspace file is hashed by streaming, not by reading
	// it whole into memory).
	if info.Size() == snapEntry.Size {
		workHash, err := stream.HashFileContent(fullPath)
		if err != nil {
			return ContentDiffResult{}, fmt.Errorf("hash %s: %w", fullPath, err)
		}
		snapHash, err := stream.HashChunkData(ctx, store, snapEntry.Chunks)
		if err != nil {
			return ContentDiffResult{}, fmt.Errorf("hash snapshot chunks for %s: %w", filePath, err)
		}
		if workHash == snapHash {
			return ContentDiffResult{Stdout: "  (no change)\n", Kind: "unchanged"}, nil
		}
	}

	// Content differs: open the workspace file and run a streaming engine diff.
	workFile, err := os.Open(fullPath)
	if err != nil {
		return ContentDiffResult{}, fmt.Errorf("open %s: %w", fullPath, err)
	}
	defer workFile.Close()

	header, workReader, err := stream.PeekHeader(workFile, core.HeaderPeekSize)
	if err != nil {
		return ContentDiffResult{}, fmt.Errorf("read header %s: %w", fullPath, err)
	}
	engine := filetype.DetectEngine(filePath, header)

	snapReader := stream.NewChunkReader(ctx, store, snapEntry.Chunks)
	oldLabel := snapshot.ShortID() + "/" + filePath
	newLabel := "workspace/" + filePath

	if engine != nil && engine.Name() == "text" {
		diff, diffErr := engine.Diff(ctx, oldLabel, snapReader, newLabel, workReader)
		if diffErr != nil {
			return ContentDiffResult{}, fmt.Errorf("diff %s: %w", filePath, diffErr)
		}
		return ContentDiffResult{Stdout: diff + "\n", Kind: "text", Diff: diff}, nil
	}
	// Binary file: emit metadata only (size change, optional dimensions).
	snapHeader, _, err := stream.PeekHeader(snapReader, core.HeaderPeekSize)
	if err != nil {
		return ContentDiffResult{}, fmt.Errorf("read snapshot header %s: %w", filePath, err)
	}
	out := fmt.Sprintf("  Size:       %s -> %s (%+s)\n",
		format.Bytes(snapEntry.Size), format.Bytes(info.Size()),
		format.Bytes(info.Size()-snapEntry.Size))
	oldDims := format.ImageDimensions(snapHeader)
	newDims := format.ImageDimensions(header)
	if oldDims != "" || newDims != "" {
		out += fmt.Sprintf("  Dimensions: %s -> %s\n", oldDims, newDims)
	}
	out += "\n  (binary file — metadata only)\n"
	return ContentDiffResult{
		Stdout:        out,
		Kind:          "binary",
		OldSize:       snapEntry.Size,
		NewSize:       info.Size(),
		OldDimensions: oldDims,
		NewDimensions: newDims,
	}, nil
}
