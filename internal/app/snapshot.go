package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

type ExportFormat string

const (
	ExportDir ExportFormat = "dir"
	ExportZip ExportFormat = "zip"
	ExportTar ExportFormat = "tar"
)

// RestoreResult contains the outcome of a restore operation.
type RestoreResult struct {
	Version  string
	Added    int
	Modified int
	Deleted  int
}

// Restore restores the working tree to a specific version.
// If filters is non-empty, only files matching the filters are restored.
// If force is false, the restore is rejected when the staging area or working tree has pending changes.
func (a *App) Restore(version string, filters []string, force bool) (*RestoreResult, error) {
	hasFilter := len(filters) > 0

	var oldIdx core.Index
	if err := a.store.LoadIndex(&oldIdx); err != nil {
		return nil, err
	}

	if !force {
		hasPending, err := a.hasPendingStagedChanges(&oldIdx, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to check pending staged changes: %w", err)
		}
		if hasPending {
			return nil, fmt.Errorf("staging area has pending changes (use --force to discard)")
		}
		currentCommit, err := a.currentCommit()
		if err != nil {
			return nil, fmt.Errorf("failed to load current commit: %w", err)
		}
		dirty, err := a.wt.HasModifications(currentCommit, &oldIdx, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to check worktree modifications: %w", err)
		}
		if dirty {
			return nil, fmt.Errorf("working tree has unstaged modifications (use --force to discard)")
		}
	}

	commit, err := a.ResolveCommit(version)
	if err != nil {
		return nil, err
	}

	targetTree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load target tree: %w", err)
	}

	reader := core.NewTreeReader(a.store)
	targetBlobs, err := reader.ListBlobs(targetTree, "")
	if err != nil {
		return nil, err
	}

	if hasFilter {
		normalized, err := worktree.NormalizePathFilters(a.dir, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to normalize filters: %w", err)
		}
		targetBlobs = worktree.FilterBlobs(targetBlobs, normalized)
		if len(targetBlobs) == 0 {
			return nil, fmt.Errorf("no matching files found in %s for given paths", version)
		}
	}

	targetPaths := make(map[string]bool)
	for _, b := range targetBlobs {
		targetPaths[b.Path] = true
	}

	prevBlobs := make(map[string]bool)
	currentBranch := a.CurrentBranch()
	// Best-effort: for added/modified/deleted statistics.
	if currentHash, err := a.store.GetRef(currentBranch); err == nil {
		if currentHash != commit.Hash {
			if currentCommit, err := a.findCommitByHash(currentHash); err == nil {
				if t, err := a.store.GetTree(currentCommit.TreeHash); err == nil {
					// Best-effort: for added/modified/deleted statistics.
					prevBlobsList, _ := reader.ListBlobs(t, "")
					for _, b := range prevBlobsList {
						prevBlobs[b.Path] = true
					}
				}
			}
		}
	}

	newIdx := &core.Index{}
	if hasFilter {
		for _, e := range oldIdx.Entries {
			if !worktree.PathMatchesAny(e.Path, filters) {
				newIdx.Add(e)
			}
		}
	}

	var deletedPaths []string

	for _, b := range targetBlobs {
		entry, err := a.wt.WriteBlob(b)
		if err != nil {
			return nil, err
		}
		newIdx.Add(entry)
	}

	var added, modified int
	for _, b := range targetBlobs {
		if prevBlobs[b.Path] {
			modified++
		} else {
			added++
		}
	}

	var deleted int
	for path := range prevBlobs {
		if targetPaths[path] {
			continue
		}
		if hasFilter && !worktree.PathMatchesAny(path, filters) {
			continue
		}
		if err := core.ValidateTreePath(path); err != nil {
			continue
		}
		fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		deleted++
		deletedPaths = append(deletedPaths, path)
	}

	for _, entry := range oldIdx.Entries {
		if targetPaths[entry.Path] {
			continue
		}
		if _, inPrev := prevBlobs[entry.Path]; inPrev {
			continue
		}
		if hasFilter && !worktree.PathMatchesAny(entry.Path, filters) {
			continue
		}
		if err := core.ValidateTreePath(entry.Path); err != nil {
			continue
		}
		fullPath := filepath.Join(a.dir, filepath.FromSlash(entry.Path))
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		deleted++
		deletedPaths = append(deletedPaths, entry.Path)
	}

	a.wt.CleanEmptyDirs(deletedPaths)

	if err := a.store.SaveIndex(newIdx); err != nil {
		return nil, fmt.Errorf("failed to update index: %w", err)
	}

	// Record operation for undo log. RefChanges is empty because restore
	// doesn't change any refs (HEAD, branch refs remain unchanged).
	// Undo for restore is a no-op since there are no refs to revert.
	if err := a.recordOperation(OpRestore, fmt.Sprintf("restore %s", version), []RefChange{}); err != nil {
		return nil, err
	}

	return &RestoreResult{
		Version:  version,
		Added:    added,
		Modified: modified,
		Deleted:  deleted,
	}, nil
}

func (a *App) Export(version, output string, format ExportFormat, filters []string) error {
	commit, err := a.ResolveCommit(version)
	if err != nil {
		return err
	}

	tree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		return fmt.Errorf("failed to load tree: %w", err)
	}

	reader := core.NewTreeReader(a.store)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	// Apply path filters if provided.
	if len(filters) > 0 {
		normalized, err := worktree.NormalizePathFilters(a.dir, filters)
		if err != nil {
			return fmt.Errorf("failed to normalize filters: %w", err)
		}
		blobs = worktree.FilterBlobs(blobs, normalized)
	}

	switch format {
	case ExportZip:
		return a.exportZip(blobs, output)
	case ExportTar:
		return a.exportTar(blobs, output)
	case ExportDir, "":
		return a.exportDir(blobs, output)
	default:
		return fmt.Errorf("unsupported format: %s (use dir, zip, or tar)", format)
	}
}

func (a *App) exportDir(blobs []core.BlobEntry, output string) error {
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("directory already exists: %s", output)
	}

	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}

	for _, blob := range blobs {
		if err := a.writeBlobToFile(blob, output); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) exportZip(blobs []core.BlobEntry, output string) error {
	if !strings.HasSuffix(output, ".zip") {
		output += ".zip"
	}

	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("file already exists: %s", output)
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for _, blob := range blobs {
		if err := a.addBlobToZip(blob, w); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) exportTar(blobs []core.BlobEntry, output string) error {
	if !strings.HasSuffix(output, ".tar.gz") {
		output += ".tar.gz"
	}

	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("file already exists: %s", output)
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, blob := range blobs {
		if err := a.addBlobToTar(blob, tw); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) writeBlobToFile(blob core.BlobEntry, outputDir string) error {
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return fmt.Errorf("unsafe export path %q: %w", blob.Path, err)
	}

	fullPath := filepath.Join(outputDir, filepath.FromSlash(blob.Path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	perm := os.FileMode(core.ToOSFileMode(blob.Mode))
	f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if err := a.store.GetBlobToWriter(blob.Hash, f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(fullPath, perm)
}

func (a *App) addBlobToZip(blob core.BlobEntry, w *zip.Writer) error {
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return fmt.Errorf("unsafe export path %q: %w", blob.Path, err)
	}

	f, err := w.Create(blob.Path)
	if err != nil {
		return err
	}

	return a.store.GetBlobToWriter(blob.Hash, f)
}

func (a *App) addBlobToTar(blob core.BlobEntry, tw *tar.Writer) error {
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return fmt.Errorf("unsafe export path %q: %w", blob.Path, err)
	}

	size, err := a.store.GetBlobSize(blob.Hash)
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name: blob.Path,
		Mode: int64(blob.Mode),
		Size: size,
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	return a.store.GetBlobToWriter(blob.Hash, tw)
}
