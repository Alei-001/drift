package porcelain

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/stream"
	"github.com/your-org/drift/internal/util/format"
	"github.com/your-org/drift/internal/util/pathutil"
)

// DiffSnapshots prints a file-level diff between two snapshots: files added,
// modified, or deleted going from snap1 to snap2. The status line is emitted
// by the caller; this function prints only the file list and summary line.
func DiffSnapshots(store storage.Storer, snap1, snap2 *core.Snapshot) {
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
	fmt.Printf("\n  %s\n", formatDiffSummary(total, len(added), len(modified), len(deleted)))
}

// formatDiffSummary formats a change summary line "N files: +A ~M -D",
// omitting zero-count parts. It mirrors the cmd package's summaryLine but
// returns a string so porcelain helpers can print it inline.
func formatDiffSummary(total, added, mod, del int) string {
	var parts []string
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if mod > 0 {
		parts = append(parts, fmt.Sprintf("~%d", mod))
	}
	if del > 0 {
		parts = append(parts, fmt.Sprintf("-%d", del))
	}
	if len(parts) == 0 {
		parts = append(parts, "+0")
	}
	noun := "files"
	if total == 1 {
		noun = "file"
	}
	return fmt.Sprintf("%d %s: %s", total, noun, strings.Join(parts, " "))
}

// DiffFileInSnapshots shows content-level diff for a single file between two
// snapshots. Both versions are streamed from chunks; the engine is selected
// from the snap2 header. The status line is emitted by the caller; this
// function prints only the diff content.
func DiffFileInSnapshots(ctx context.Context, store storage.Storer, workDir string, snap1, snap2 *core.Snapshot, filePath string) {
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
		diff, diffErr := engine.Diff(ctx, snap1.ShortID()+"/"+filePath, fullReader1, snap2.ShortID()+"/"+filePath, fullReader2)
		if diffErr != nil {
			fmt.Fprintf(os.Stderr, "  warning: cannot diff %s: %v\n", filePath, diffErr)
			return
		}
		fmt.Println(diff)
	} else {
		sizeDiff := format.Bytes(entry2.Size - entry1.Size)
		sign := "+"
		if entry2.Size < entry1.Size {
			sign = ""
		}
		fmt.Printf("  Size:       %s -> %s (%s%s)\n",
			format.Bytes(entry1.Size), format.Bytes(entry2.Size), sign, sizeDiff)
		oldDims := format.ImageDimensions(header1)
		newDims := format.ImageDimensions(header2)
		if oldDims != "" || newDims != "" {
			fmt.Printf("  Dimensions: %s -> %s\n", oldDims, newDims)
		}
		fmt.Println("\n  (binary file — metadata only)")
	}
}
