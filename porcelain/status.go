package porcelain

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/fsutil"
	"github.com/your-org/drift/util/pathutil"
)

// ChangeSummary summarizes workspace changes since last save.
type ChangeSummary struct {
	Added    []string
	Modified []string
	Deleted  []string
}

// DetectChanges compares the workspace against the stored index and returns changes.
func DetectChanges(store storage.Storer, workDir string) (*ChangeSummary, error) {
	ctx := context.Background()
	index, err := store.GetIndex(ctx)
	if err != nil {
		return nil, err
	}

	workspaceFiles := make(map[string]os.FileInfo)
	err = fsutil.Walk(workDir, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rel, _ := pathutil.Rel(workDir, path)
		workspaceFiles[rel] = info
		return nil
	})
	if err != nil {
		return nil, err
	}

	summary := &ChangeSummary{}
	printed := make(map[string]bool)

	for _, entry := range index.Entries {
		if info, ok := workspaceFiles[entry.Path]; ok {
			if info.Size() != entry.Size || info.ModTime().Unix() != entry.ModTime {
				summary.Modified = append(summary.Modified, entry.Path)
			} else {
				fileHash, hashErr := ComputeFileHash(filepath.Join(workDir, entry.Path))
				if hashErr != nil || fileHash != entry.Hash {
					summary.Modified = append(summary.Modified, entry.Path)
				}
			}
			printed[entry.Path] = true
		} else {
			summary.Deleted = append(summary.Deleted, entry.Path)
			printed[entry.Path] = true
		}
	}

	for path := range workspaceFiles {
		if !printed[path] {
			summary.Added = append(summary.Added, path)
		}
	}

	sort.Strings(summary.Added)
	sort.Strings(summary.Modified)
	sort.Strings(summary.Deleted)

	return summary, nil
}
