package porcelain

import (
	"context"
	"fmt"
	"slices"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/filetype"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/storage/stream"
	"github.com/Alei-001/drift/internal/util/format"
	"github.com/Alei-001/drift/internal/util/pathutil"
)

// DiffSnapshots computes a file-level diff between two snapshots: files
// added, modified, or deleted going from snap1 to snap2. It returns the
// classified file lists without printing; the caller renders the result.
func DiffSnapshots(snap1, snap2 *core.Snapshot) FileDiffResult {
	snap1Files := make(map[string]*core.FileEntry)
	for i := range snap1.Files {
		snap1Files[snap1.Files[i].Path] = &snap1.Files[i]
	}

	var result FileDiffResult
	for i := range snap2.Files {
		entry2 := &snap2.Files[i]
		entry1, exists := snap1Files[entry2.Path]
		if !exists {
			result.Added = append(result.Added, entry2.Path)
			continue
		}

		if entry1.Size != entry2.Size || !slices.Equal(entry1.Chunks, entry2.Chunks) {
			result.Modified = append(result.Modified, entry2.Path)
		}
		delete(snap1Files, entry2.Path)
	}

	for path := range snap1Files {
		result.Deleted = append(result.Deleted, path)
	}
	return result
}

// DiffFileInSnapshots computes a content-level diff for a single file
// between two snapshots. Both versions are streamed from chunks; the engine
// is selected from the snap2 header. It returns the diff content (and any
// warning) without printing; the caller renders the result.
func DiffFileInSnapshots(ctx context.Context, store storage.Storer, workDir string, snap1, snap2 *core.Snapshot, filePath string) ContentDiffResult {
	filePath, err := pathutil.RelToWorkDir(workDir, filePath)
	if err != nil {
		return ContentDiffResult{Stderr: fmt.Sprintf("warning: cannot resolve path: %v\n", err)}
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

	if entry1 == nil && entry2 != nil {
		return ContentDiffResult{
			Stdout:  fmt.Sprintf("  +  %s  (added)\n", filePath),
			Kind:    "added",
			NewSize: entry2.Size,
		}
	}
	if entry1 != nil && entry2 == nil {
		return ContentDiffResult{
			Stdout:  fmt.Sprintf("  -  %s  (deleted)\n", filePath),
			Kind:    "deleted",
			OldSize: entry1.Size,
		}
	}
	if entry1 == nil && entry2 == nil {
		return ContentDiffResult{Stderr: fmt.Sprintf("  warning: '%s' not found in either snapshot.\n", filePath)}
	}
	if entry1.Size == entry2.Size && slices.Equal(entry1.Chunks, entry2.Chunks) {
		return ContentDiffResult{Stdout: "  (no change)\n", Kind: "unchanged"}
	}

	// Stream both snapshot versions from chunks; peek a header from each for
	// engine detection and dimension parsing without buffering the file.
	reader1 := stream.NewChunkReader(ctx, store, entry1.Chunks)
	reader2 := stream.NewChunkReader(ctx, store, entry2.Chunks)
	header1, fullReader1, err := stream.PeekHeader(reader1, core.HeaderPeekSize)
	if err != nil {
		return ContentDiffResult{Stderr: fmt.Sprintf("  warning: cannot read chunk for %s: %v\n", filePath, err)}
	}
	header2, fullReader2, err := stream.PeekHeader(reader2, core.HeaderPeekSize)
	if err != nil {
		return ContentDiffResult{Stderr: fmt.Sprintf("  warning: cannot read chunk for %s: %v\n", filePath, err)}
	}
	engine := filetype.DetectEngine(filePath, header2)
	if engine != nil && engine.Name() == "text" {
		diff, diffErr := engine.Diff(ctx, snap1.ShortID()+"/"+filePath, fullReader1, snap2.ShortID()+"/"+filePath, fullReader2)
		if diffErr != nil {
			return ContentDiffResult{Stderr: fmt.Sprintf("  warning: cannot diff %s: %v\n", filePath, diffErr)}
		}
		return ContentDiffResult{Stdout: diff + "\n", Kind: "text", Diff: diff}
	}
	// Binary file: emit metadata only (size change, optional dimensions).
	sizeDiff := format.Bytes(entry2.Size - entry1.Size)
	sign := "+"
	if entry2.Size < entry1.Size {
		sign = ""
	}
	out := fmt.Sprintf("  Size:       %s -> %s (%s%s)\n",
		format.Bytes(entry1.Size), format.Bytes(entry2.Size), sign, sizeDiff)
	oldDims := format.ImageDimensions(header1)
	newDims := format.ImageDimensions(header2)
	if oldDims != "" || newDims != "" {
		out += fmt.Sprintf("  Dimensions: %s -> %s\n", oldDims, newDims)
	}
	out += "\n  (binary file — metadata only)\n"
	return ContentDiffResult{
		Stdout:        out,
		Kind:          "binary",
		OldSize:       entry1.Size,
		NewSize:       entry2.Size,
		OldDimensions: oldDims,
		NewDimensions: newDims,
	}
}
