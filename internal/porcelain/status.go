package porcelain

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
	"github.com/your-org/drift/internal/util/pathutil"
)

// ChangeSummary summarizes workspace changes since last save.
type ChangeSummary struct {
	Added    []string
	Modified []string
	Deleted  []string
}

// DetectChanges compares the workspace against the stored index and returns changes.
//
// It acquires the workspace lock so that the workspace scan and the index it
// is compared against cannot be mutated mid-comparison by a concurrent save,
// switch, or restore (which would otherwise produce a tear: half the files
// from the old state, half from the new).
func DetectChanges(ctx context.Context, store storage.Storer, workDir string, cfg *core.CoreConfig) (*ChangeSummary, error) {
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)

	index, err := store.GetIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	workspaceFiles := make(map[string]os.FileInfo)
	err = fsutil.Walk(workDir, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rel, err := pathutil.Rel(workDir, path)
		if err != nil {
			return err
		}
		workspaceFiles[rel] = info
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}

	summary := &ChangeSummary{}
	printed := make(map[string]bool)

	for _, entry := range index.Entries {
		if info, ok := workspaceFiles[entry.Path]; ok {
			if info.Size() != entry.Size || info.ModTime().UnixNano() != entry.ModTime {
				summary.Modified = append(summary.Modified, entry.Path)
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
