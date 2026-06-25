package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

	blobs1, err := reader.ListBlobs(tree1, "")
	if err != nil {
		return nil, err
	}
	blobs2, err := reader.ListBlobs(tree2, "")
	if err != nil {
		return nil, err
	}

	if len(filePaths) > 0 {
		blobs1 = worktree.FilterBlobs(blobs1, filePaths)
		blobs2 = worktree.FilterBlobs(blobs2, filePaths)
	}

	entries, err := a.collectVersionDiffs(blobs1, blobs2)
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
			entries = append(entries, DiffEntry{
				Path:     blob.Path,
				Status:   "modified",
				IsBinary: isBinary(blobData) || isBinary(workData),
				OldSize:  int64(len(blobData)),
				NewSize:  int64(len(workData)),
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
		if string(workData) == string(blobData) {
			continue
		}

		entries = append(entries, DiffEntry{
			Path:     blob.Path,
			Status:   "modified",
			IsBinary: isBinary(blobData) || isBinary(workData),
			OldSize:  int64(len(blobData)),
			NewSize:  int64(len(workData)),
		})
	}

	if len(filePaths) == 0 {
		var idx core.Index
		// Non-fatal: index may not exist in a fresh repo; empty index means all files are untracked, which is correct.
		_ = a.store.LoadIndex(&idx)

		err := core.WalkWorkingDir(a.dir, func(path string, info os.FileInfo) error {
			if trackedPaths[path] || idx.Has(path) {
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
		if err != nil {
			return entries, fmt.Errorf("failed to walk working dir: %w", err)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	return entries, nil
}

func (a *App) collectVersionDiffs(blobs1, blobs2 []core.BlobEntry) ([]DiffEntry, error) {
	var entries []DiffEntry

	map1 := make(map[string]core.BlobEntry)
	for _, b := range blobs1 {
		map1[b.Path] = b
	}
	map2 := make(map[string]core.BlobEntry)
	for _, b := range blobs2 {
		map2[b.Path] = b
	}

	for path, b1 := range map1 {
		if b2, exists := map2[path]; exists {
			if b1.Hash == b2.Hash {
				continue
			}
			data1, err := a.store.GetBlob(b1.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", b1.Hash, path, err)
			}
			data2, err := a.store.GetBlob(b2.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", b2.Hash, path, err)
			}
			entries = append(entries, DiffEntry{
				Path:     path,
				Status:   "modified",
				IsBinary: isBinary(data1) || isBinary(data2),
				OldSize:  int64(len(data1)),
				NewSize:  int64(len(data2)),
			})
		} else {
			data1, err := a.store.GetBlob(b1.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", b1.Hash, path, err)
			}
			entries = append(entries, DiffEntry{
				Path:     path,
				Status:   "deleted",
				IsBinary: isBinary(data1),
				OldSize:  int64(len(data1)),
				NewSize:  0,
			})
		}
	}

	for path, b2 := range map2 {
		if _, exists := map1[path]; !exists {
			data2, err := a.store.GetBlob(b2.Hash)
			if err != nil {
				return nil, fmt.Errorf("failed to read blob %s for %s: %w", b2.Hash, path, err)
			}
			entries = append(entries, DiffEntry{
				Path:     path,
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
