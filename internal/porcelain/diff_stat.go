package porcelain

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
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/stream"
	"github.com/your-org/drift/internal/util/fsutil"
	"github.com/your-org/drift/internal/util/pathutil"
)

// FileStat holds per-file change statistics for --stat output. Path is the
// workspace-relative file path. Insertions and Deletions are the unified-diff
// line counts (zero for binary files). Binary is true for non-text files or
// read errors, where Ins/Del are not meaningful. OldSize and NewSize are the
// byte sizes before and after the change (whichever side exists).
type FileStat struct {
	Path       string
	Insertions int
	Deletions  int
	Binary     bool
	OldSize    int64
	NewSize    int64
}

// ComputeStatSnapshots computes per-file change statistics between two
// snapshots without printing. It is the structured counterpart to
// DiffSnapshots: where DiffSnapshots classifies files into added/modified/
// deleted buckets, ComputeStatSnapshots also reads chunk data for modified
// files and runs the text engine to count insertions/deletions. The caller
// renders the result.
func ComputeStatSnapshots(ctx context.Context, store storage.Storer, snap1, snap2 *core.Snapshot) ([]FileStat, error) {
	snap1Files := make(map[string]*core.FileEntry)
	for i := range snap1.Files {
		snap1Files[snap1.Files[i].Path] = &snap1.Files[i]
	}

	var stats []FileStat
	for i := range snap2.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e2 := &snap2.Files[i]
		e1, exists := snap1Files[e2.Path]
		if !exists {
			ins, del, bin := computeSnapFileStat(ctx, store, nil, e2)
			stats = append(stats, FileStat{Path: e2.Path, Insertions: ins, Deletions: del, Binary: bin, NewSize: e2.Size})
			continue
		}
		if e1.Size != e2.Size || !slices.Equal(e1.Chunks, e2.Chunks) {
			ins, del, bin := computeSnapFileStat(ctx, store, e1, e2)
			stats = append(stats, FileStat{Path: e2.Path, Insertions: ins, Deletions: del, Binary: bin, OldSize: e1.Size, NewSize: e2.Size})
		}
		delete(snap1Files, e2.Path)
	}
	for path, e1 := range snap1Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ins, del, bin := computeSnapFileStat(ctx, store, e1, nil)
		stats = append(stats, FileStat{Path: path, Insertions: ins, Deletions: del, Binary: bin, OldSize: e1.Size})
	}
	return stats, nil
}

// ComputeStatWorkspace computes per-file change statistics between the
// workspace and a snapshot without printing. It walks the workspace via
// fsutil.Walk so that the .drift/ directory and ignore-file patterns are
// honored, and compares same-size files by BLAKE3 hash so that content-only
// changes are not silently skipped. The caller renders the result.
func ComputeStatWorkspace(ctx context.Context, store storage.Storer, workDir string, cfg *core.CoreConfig, snap *core.Snapshot) ([]FileStat, error) {
	snapFiles := make(map[string]*core.FileEntry)
	for i := range snap.Files {
		snapFiles[snap.Files[i].Path] = &snap.Files[i]
	}

	var stats []FileStat
	walkErr := fsutil.Walk(workDir, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := pathutil.Rel(workDir, path)
		if relErr != nil {
			return nil
		}
		e1, exists := snapFiles[rel]
		if !exists {
			stats = append(stats, FileStat{Path: rel, Binary: true, NewSize: info.Size()})
		} else if info.Size() != e1.Size {
			ins, del, bin := computeWorkFileStat(ctx, store, path, e1)
			stats = append(stats, FileStat{Path: rel, Insertions: ins, Deletions: del, Binary: bin, OldSize: e1.Size, NewSize: info.Size()})
		} else {
			// Same size: compare BLAKE3 hash to catch content changes that
			// preserve size (e.g. "cp -p" preserves size and modtime).
			workHash, hashErr := ComputeFileHash(path)
			if hashErr != nil || workHash != e1.Hash {
				ins, del, bin := computeWorkFileStat(ctx, store, path, e1)
				stats = append(stats, FileStat{Path: rel, Insertions: ins, Deletions: del, Binary: bin, OldSize: e1.Size, NewSize: info.Size()})
			}
		}
		delete(snapFiles, rel)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk workspace: %w", walkErr)
	}
	for path, e1 := range snapFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stats = append(stats, FileStat{Path: path, Binary: true, OldSize: e1.Size})
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
	diff, err := engine.Diff(ctx, path, bytes.NewReader(c1), path, bytes.NewReader(c2))
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
	diff, err := engine.Diff(ctx, e1.Path, bytes.NewReader(c1), e1.Path, bytes.NewReader(c2))
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
