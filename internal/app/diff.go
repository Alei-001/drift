package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

type DiffOptions struct {
	V1    string
	V2    string
	Paths []string
}

type DiffEntry struct {
	Path     string
	Status   string
	IsBinary bool
	OldSize  int64
	NewSize  int64
	Edits    []core.DiffEdit
}

type DiffResult struct {
	Entries []DiffEntry
}

// computeEdits runs Myers diff on old/new content and returns the edit script.
// Returns nil for binary content or empty data.
func computeEdits(oldData, newData []byte, isBinary bool) []core.DiffEdit {
	if isBinary || len(oldData) == 0 || len(newData) == 0 {
		return nil
	}
	oldLines := strings.Split(string(oldData), "\n")
	newLines := strings.Split(string(newData), "\n")
	return core.Myers(oldLines, newLines)
}

func (a *App) Diff(opts DiffOptions) (*DiffResult, error) {
	filePaths := opts.Paths
	if len(filePaths) > 0 {
		normalized, err := worktree.NormalizePathFilters(a.dir, filePaths)
		if err != nil {
			return nil, err
		}
		filePaths = normalized
	}

	reader := core.NewTreeReader(a.store)

	if opts.V1 == "" && opts.V2 == "" {
		return a.diffWorktree(reader, "", filePaths)
	} else if opts.V2 == "" {
		return a.diffWorktree(reader, opts.V1, filePaths)
	}
	return a.diffVersions(reader, opts.V1, opts.V2, filePaths)
}

func (a *App) diffWorktree(reader *core.TreeReader, version string, filePaths []string) (*DiffResult, error) {
	var targetBlobs []core.BlobEntry

	if version == "" {
		latest, err := a.currentCommit()
		if err != nil || latest == nil {
			return nil, fmt.Errorf("no versions to compare against")
		}
		tree, err := a.store.GetTree(latest.TreeHash)
		if err != nil {
			return nil, err
		}
		targetBlobs, err = reader.ListBlobs(tree, "")
		if err != nil {
			return nil, err
		}
	} else {
		commit, err := a.ResolveCommit(version)
		if err != nil {
			return nil, err
		}
		tree, err := a.store.GetTree(commit.TreeHash)
		if err != nil {
			return nil, err
		}
		targetBlobs, err = reader.ListBlobs(tree, "")
		if err != nil {
			return nil, err
		}
	}

	if len(filePaths) > 0 {
		targetBlobs = worktree.FilterBlobs(targetBlobs, filePaths)
	}

	entries, err := a.collectWorktreeDiffs(targetBlobs, filePaths)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return &DiffResult{}, nil
	}

	return &DiffResult{Entries: entries}, nil
}

func (a *App) diffVersions(reader *core.TreeReader, v1, v2 string, filePaths []string) (*DiffResult, error) {
	commit1, err := a.ResolveCommit(v1)
	if err != nil {
		return nil, err
	}
	commit2, err := a.ResolveCommit(v2)
	if err != nil {
		return nil, err
	}

	tree1, err := a.store.GetTree(commit1.TreeHash)
	if err != nil {
		return nil, err
	}
	tree2, err := a.store.GetTree(commit2.TreeHash)
	if err != nil {
		return nil, err
	}

	changes, err := reader.LazyDiffTrees(tree1, tree2)
	if err != nil {
		return nil, err
	}

	entries, err := a.collectVersionDiffsFromChanges(changes, filePaths)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return &DiffResult{}, nil
	}

	return &DiffResult{Entries: entries}, nil
}

func (a *App) collectWorktreeDiffs(targetBlobs []core.BlobEntry, filePaths []string) ([]DiffEntry, error) {
	var entries []DiffEntry

	// Load index for mtime fast-path: if a file's mtime matches the index entry,
	// and the hash matches, we can skip re-reading the file entirely.
	var idx core.Index
	_ = a.store.LoadIndex(&idx)

	trackedPaths := make(map[string]bool, len(targetBlobs))
	for _, blob := range targetBlobs {
		trackedPaths[blob.Path] = true
		fullPath := filepath.Join(a.dir, filepath.FromSlash(blob.Path))

		info, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				blobData, err := a.store.GetBlob(blob.Hash)
				if err != nil {
					return nil, fmt.Errorf("failed to read blob %s for %s: %w", blob.Hash, blob.Path, err)
				}
				entries = append(entries, DiffEntry{
					Path:     blob.Path,
					Status:   "deleted",
					IsBinary: isBinary(blobData),
					OldSize:  int64(len(blobData)),
					NewSize:  0,
				})
			}
			continue
		}

		// Mtime fast-path: if the index has this file with matching mtime,
		// modification time hasn't changed since we last staged it. Combined
		// with hash check, this lets us skip re-reading the file.
		if ie, idxErr := idx.Entry(blob.Path); idxErr == nil {
			if ie.ModifiedAt.Equal(info.ModTime()) && ie.Hash == blob.Hash {
				continue // unchanged
			}
		}

		// Symlink: compare target string, not file content.
		if blob.Mode == core.ModeSymlink || info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(fullPath)
			if err != nil {
				continue
			}
			blobData, err := a.store.GetBlob(blob.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", blob.Hash, blob.Path, err)
			}
			if target == string(blobData) {
				continue
			}
			entries = append(entries, DiffEntry{
				Path:     blob.Path,
				Status:   "modified",
				IsBinary: false,
				OldSize:  int64(len(blobData)),
				NewSize:  int64(len(target)),
			})
			continue
		}

		blobSize, sizeErr := a.store.GetBlobSize(blob.Hash)
		if sizeErr == nil && info.Size() != blobSize {
			workData, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			blobData, err := a.store.GetBlob(blob.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", blob.Hash, blob.Path, err)
			}
			isBin := isBinary(blobData) || isBinary(workData)
			entries = append(entries, DiffEntry{
				Path:     blob.Path,
				Status:   "modified",
				IsBinary: isBin,
				OldSize:  int64(len(blobData)),
				NewSize:  int64(len(workData)),
				Edits:    computeEdits(blobData, workData, isBin),
			})
			continue
		}

		same, err := fileHashMatchesBlob(fullPath, blob.Hash)
		if err != nil {
			continue
		}
		if same {
			continue
		}

		workData, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		blobData, err := a.store.GetBlob(blob.Hash)
		if err != nil {
			return nil, fmt.Errorf("failed to read blob %s for %s: %w", blob.Hash, blob.Path, err)
		}

		isBin := isBinary(blobData) || isBinary(workData)
		entries = append(entries, DiffEntry{
			Path:     blob.Path,
			Status:   "modified",
			IsBinary: isBin,
			OldSize:  int64(len(blobData)),
			NewSize:  int64(len(workData)),
			Edits:    computeEdits(blobData, workData, isBin),
		})
	}

	// Walk working dir for untracked (added) files. Always run, but apply
	// path filters when present so filtered diffs don't miss new files.

	walkErr := core.WalkWorkingDir(a.dir, func(path string, info os.FileInfo) error {
		if trackedPaths[path] || idx.Has(path) {
			return nil
		}
		if len(filePaths) > 0 && !worktree.PathMatchesAny(path, filePaths) {
			return nil
		}
		fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil
		}
		entries = append(entries, DiffEntry{
			Path:     path,
			Status:   "added",
			IsBinary: isBinary(data),
			OldSize:  0,
			NewSize:  int64(len(data)),
		})
		return nil
	})
	if walkErr != nil {
		return entries, fmt.Errorf("failed to walk working dir: %w", walkErr)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	return entries, nil
}

// collectVersionDiffsFromChanges converts DiffChange entries from LazyDiffTrees
// into DiffEntry results with full content diff. When filePaths is non-empty,
// only changes matching those paths are included.
func (a *App) collectVersionDiffsFromChanges(changes []core.DiffChange, filePaths []string) ([]DiffEntry, error) {
	var entries []DiffEntry

	for _, ch := range changes {
		if len(filePaths) > 0 && !worktree.PathMatchesAny(ch.Path, filePaths) {
			continue
		}

		switch {
		case ch.Old != nil && ch.New != nil:
			// Modified.
			if ch.Old.Hash == ch.New.Hash {
				continue
			}
			data1, err := a.store.GetBlob(ch.Old.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", ch.Old.Hash, ch.Path, err)
			}
			data2, err := a.store.GetBlob(ch.New.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", ch.New.Hash, ch.Path, err)
			}
			isBin := isBinary(data1) || isBinary(data2)
			entries = append(entries, DiffEntry{
				Path:     ch.Path,
				Status:   "modified",
				IsBinary: isBin,
				OldSize:  int64(len(data1)),
				NewSize:  int64(len(data2)),
				Edits:    computeEdits(data1, data2, isBin),
			})
		case ch.Old != nil:
			// Deleted.
			data1, err := a.store.GetBlob(ch.Old.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", ch.Old.Hash, ch.Path, err)
			}
			entries = append(entries, DiffEntry{
				Path:     ch.Path,
				Status:   "deleted",
				IsBinary: isBinary(data1),
				OldSize:  int64(len(data1)),
				NewSize:  0,
			})
		case ch.New != nil:
			// Added.
			data2, err := a.store.GetBlob(ch.New.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", ch.New.Hash, ch.Path, err)
			}
			entries = append(entries, DiffEntry{
				Path:     ch.Path,
				Status:   "added",
				IsBinary: isBinary(data2),
				OldSize:  0,
				NewSize:  int64(len(data2)),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	return entries, nil
}

func fileHashMatchesBlob(filePath string, blobHash string) (bool, error) {
	fileHash, err := core.CalculateHashFromFile(filePath)
	if err != nil {
		return false, err
	}
	return fileHash == blobHash, nil
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	limit := 8192
	if len(data) < limit {
		limit = len(data)
	}
	for _, b := range data[:limit] {
		if b == 0 {
			return true
		}
	}
	return false
}
