package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/your-org/drift/internal/porcelain"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/stream"
	"github.com/your-org/drift/internal/util/format"
	"github.com/your-org/drift/internal/util/fsutil"
	"github.com/your-org/drift/internal/util/pathutil"
)

// fileStat holds per-file change statistics for --stat output.
type fileStat struct {
	path       string
	insertions int
	deletions  int
	binary     bool
	oldSize    int64
	newSize    int64
}

// diffStatSnapshots prints a --stat summary between two snapshots.
func diffStatSnapshots(ctx context.Context, store storage.Storer, snap1, snap2 *core.Snapshot) error {
	stats, err := computeStatSnapshots(ctx, store, snap1, snap2)
	if err != nil {
		return err
	}
	printStatOutput(stats)
	return nil
}

// computeStatSnapshots computes per-file change statistics between two
// snapshots without printing. Extracted from diffStatSnapshots so the JSON
// path can reuse the same computation.
func computeStatSnapshots(ctx context.Context, store storage.Storer, snap1, snap2 *core.Snapshot) ([]fileStat, error) {
	snap1Files := make(map[string]*core.FileEntry)
	for i := range snap1.Files {
		snap1Files[snap1.Files[i].Path] = &snap1.Files[i]
	}

	var stats []fileStat
	for i := range snap2.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e2 := &snap2.Files[i]
		e1, exists := snap1Files[e2.Path]
		if !exists {
			ins, del, bin := computeSnapFileStat(ctx, store, nil, e2)
			stats = append(stats, fileStat{path: e2.Path, insertions: ins, deletions: del, binary: bin, newSize: e2.Size})
			continue
		}
		if e1.Size != e2.Size || !slices.Equal(e1.Chunks, e2.Chunks) {
			ins, del, bin := computeSnapFileStat(ctx, store, e1, e2)
			stats = append(stats, fileStat{path: e2.Path, insertions: ins, deletions: del, binary: bin, oldSize: e1.Size, newSize: e2.Size})
		}
		delete(snap1Files, e2.Path)
	}
	for path, e1 := range snap1Files {
		ins, del, bin := computeSnapFileStat(ctx, store, e1, nil)
		stats = append(stats, fileStat{path: path, insertions: ins, deletions: del, binary: bin, oldSize: e1.Size})
	}
	return stats, nil
}

// diffStatWorkspace prints a --stat summary between workspace and snapshot.
// It walks the workspace via fsutil.Walk so that the .drift/ directory and
// ignore-file patterns are honored, and compares same-size files by BLAKE3
// hash so that content-only changes are not silently skipped.
func diffStatWorkspace(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap *core.Snapshot) error {
	stats, err := computeStatWorkspace(ctx, store, cwd, cfg, snap)
	if err != nil {
		return err
	}
	printStatOutput(stats)
	return nil
}

// computeStatWorkspace computes per-file change statistics between the
// workspace and a snapshot without printing. Extracted from
// diffStatWorkspace so the JSON path can reuse the same computation.
func computeStatWorkspace(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap *core.Snapshot) ([]fileStat, error) {
	snapFiles := make(map[string]*core.FileEntry)
	for i := range snap.Files {
		snapFiles[snap.Files[i].Path] = &snap.Files[i]
	}

	var stats []fileStat
	walkErr := fsutil.Walk(cwd, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := pathutil.Rel(cwd, path)
		if relErr != nil {
			return nil
		}
		e1, exists := snapFiles[rel]
		if !exists {
			stats = append(stats, fileStat{path: rel, insertions: 0, deletions: 0, binary: true, newSize: info.Size()})
		} else if info.Size() != e1.Size {
			ins, del, bin := computeWorkFileStat(ctx, store, path, e1)
			stats = append(stats, fileStat{path: rel, insertions: ins, deletions: del, binary: bin, oldSize: e1.Size, newSize: info.Size()})
		} else {
			// Same size: compare BLAKE3 hash to catch content changes that
			// preserve size (e.g. "cp -p" preserves size and modtime).
			workHash, hashErr := porcelain.ComputeFileHash(path, cfg)
			if hashErr != nil || workHash != e1.Hash {
				ins, del, bin := computeWorkFileStat(ctx, store, path, e1)
				stats = append(stats, fileStat{path: rel, insertions: ins, deletions: del, binary: bin, oldSize: e1.Size, newSize: info.Size()})
			}
		}
		delete(snapFiles, rel)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk workspace: %w", walkErr)
	}
	for path, e1 := range snapFiles {
		stats = append(stats, fileStat{path: path, insertions: 0, deletions: 0, binary: true, oldSize: e1.Size})
	}
	return stats, nil
}

// computeSnapFileStat computes insertions/deletions for a file between two
// snapshot entries. Returns isBinary=true for non-text files or read errors.
func computeSnapFileStat(ctx context.Context, store storage.Storer, e1, e2 *core.FileEntry) (ins, del int, isBinary bool) {
	var c1, c2 []byte
	if e1 != nil {
		data, err := readAllChunks(ctx, store, e1.Chunks)
		if err != nil {
			return 0, 0, true
		}
		c1 = data
	}
	if e2 != nil {
		data, err := readAllChunks(ctx, store, e2.Chunks)
		if err != nil {
			return 0, 0, true
		}
		c2 = data
	}
	header := c2
	if header == nil {
		header = c1
	}
	if len(header) > core.HeaderPeekSize {
		header = header[:core.HeaderPeekSize]
	}
	path := ""
	if e2 != nil {
		path = e2.Path
	} else {
		path = e1.Path
	}
	engine := filetype.DetectEngine(path, header)
	if engine == nil || engine.Name() != "text" {
		return 0, 0, true
	}
	diff, err := engine.Diff(path, bytes.NewReader(c1), path, bytes.NewReader(c2))
	if err != nil {
		return 0, 0, true
	}
	ins, del = countDiffLines(diff)
	return ins, del, false
}

// computeWorkFileStat computes insertions/deletions for a workspace file vs
// a snapshot entry.
func computeWorkFileStat(ctx context.Context, store storage.Storer, workPath string, e1 *core.FileEntry) (ins, del int, isBinary bool) {
	c1, err := readAllChunks(ctx, store, e1.Chunks)
	if err != nil {
		return 0, 0, true
	}
	c2, err := os.ReadFile(workPath)
	if err != nil {
		return 0, 0, true
	}
	header := c2
	if len(header) > core.HeaderPeekSize {
		header = header[:core.HeaderPeekSize]
	}
	engine := filetype.DetectEngine(e1.Path, header)
	if engine == nil || engine.Name() != "text" {
		return 0, 0, true
	}
	diff, err := engine.Diff(e1.Path, bytes.NewReader(c1), e1.Path, bytes.NewReader(c2))
	if err != nil {
		return 0, 0, true
	}
	ins, del = countDiffLines(diff)
	return ins, del, false
}

// readAllChunks reads all chunk data for the given hashes into a single
// byte slice. The returned error must be checked by callers; treating a
// nil result as empty content would produce false "whole file added/deleted"
// diffs.
func readAllChunks(ctx context.Context, store storage.Storer, hashes []core.Hash) ([]byte, error) {
	return io.ReadAll(stream.NewChunkReader(ctx, store, hashes))
}

// countDiffLines counts + and - lines in a unified diff string, skipping
// the +++ and --- headers.
func countDiffLines(diff string) (ins, del int) {
	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			ins++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			del++
		}
	}
	return ins, del
}

// maxStatBars caps the visual length of the +/- bar in --stat output.
const maxStatBars = 10

// statBar renders a compact visual bar of insertions (+) and deletions (-),
// scaled to at most maxStatBars characters.
func statBar(ins, del int) string {
	total := ins + del
	if total == 0 {
		return ""
	}
	bars := maxStatBars
	if total < bars {
		bars = total
	}
	plus := bars * ins / total
	return strings.Repeat("+", plus) + strings.Repeat("-", bars-plus)
}

// printStatOutput prints the --stat file list and summary line.
func printStatOutput(stats []fileStat) {
	if len(stats) == 0 {
		fmt.Println()
		fmt.Println("  No changes.")
		return
	}
	fmt.Println()
	pathWidth := 0
	for _, s := range stats {
		if len(s.path) > pathWidth {
			pathWidth = len(s.path)
		}
	}
	totalIns, totalDel := 0, 0
	for _, s := range stats {
		if s.binary {
			fmt.Printf("  %-*s | Bin %s -> %s\n", pathWidth, s.path,
				format.Bytes(s.oldSize), format.Bytes(s.newSize))
			continue
		}
		total := s.insertions + s.deletions
		fmt.Printf("  %-*s | %d %s\n", pathWidth, s.path, total, statBar(s.insertions, s.deletions))
		totalIns += s.insertions
		totalDel += s.deletions
	}
	fmt.Printf("\n  %d %s changed, %d insertions(+), %d deletions(-)\n",
		len(stats), pluralFile(len(stats)), totalIns, totalDel)
}
