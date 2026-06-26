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
	var normalized []string
	if hasFilter {
		var err error
		normalized, err = worktree.NormalizePathFilters(a.dir, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to normalize filters: %w", err)
		}
	}

	var oldIdx core.Index
	if err := a.store.LoadIndex(&oldIdx); err != nil {
		return nil, err
	}

	if !force {
		hasPending, err := a.hasPendingStagedChanges(&oldIdx, normalized)
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
		dirty, err := a.wt.HasModifications(currentCommit, &oldIdx, normalized)
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
		for i, filter := range normalized {
			matched := false
			for _, b := range targetBlobs {
				if worktree.PathMatchesAny(b.Path, []string{filter}) {
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("'%s' did not match any files in version %s", filters[i], version)
			}
		}
		targetBlobs = worktree.FilterBlobs(targetBlobs, normalized)
	}

	targetPaths := make(map[string]bool)
	for _, b := range targetBlobs {
		targetPaths[b.Path] = true
	}

	prevBlobs := make(map[string]string)
	currentBranch := a.CurrentBranch()
	// prevBlobs maps the current branch's latest commit tree (path → hash).
	// Used for deletion of files not in the restore target and for
	// added/modified/deleted statistics. Empty when restoring to the
	// current commit (deletion falls back to oldIdx entries).
	if currentHash, err := a.store.GetRef(currentBranch); err == nil {
		if currentHash != commit.Hash && currentHash != "" {
			if currentCommit, err := a.findCommitByHash(currentHash); err == nil {
				if t, err := a.store.GetTree(currentCommit.TreeHash); err == nil {
					prevBlobsList, err := reader.ListBlobs(t, "")
					if err != nil {
						return nil, fmt.Errorf("failed to list blobs from current branch tree: %w", err)
					}
					for _, b := range prevBlobsList {
						prevBlobs[b.Path] = b.Hash
					}
				}
			}
		}
	}

	newIdx := &core.Index{}
	if hasFilter {
		for _, e := range oldIdx.Entries {
			if !worktree.PathMatchesAny(e.Path, normalized) {
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
		prevHash, inPrev := prevBlobs[b.Path]
		if !inPrev {
			added++
		} else if prevHash != b.Hash {
			modified++
		}
		// If hash matches, the file is unchanged — don't count it.
	}

	var deleted int
	for path := range prevBlobs {
		if targetPaths[path] {
			continue
		}
		if hasFilter && !worktree.PathMatchesAny(path, normalized) {
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
		if hasFilter && !worktree.PathMatchesAny(entry.Path, normalized) {
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

	// Record operation for undo log. Save the old index snapshot so undo
	// can restore the pre-restore index state.
	oldIdxSnapshot := make([]core.IndexEntry, len(oldIdx.Entries))
	copy(oldIdxSnapshot, oldIdx.Entries)
	if err := a.recordOperationWithIndex(OpRestore, fmt.Sprintf("restore %s", version), []RefChange{}, oldIdxSnapshot); err != nil {
		return nil, err
	}

	return &RestoreResult{
		Version:  version,
		Added:    added,
		Modified: modified,
		Deleted:  deleted,
	}, nil
}

func (a *App) Export(version, output string, format ExportFormat, filters []string) (string, error) {
	commit, err := a.ResolveCommit(version)
	if err != nil {
		return "", err
	}

	tree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		return "", fmt.Errorf("failed to load tree: %w", err)
	}

	reader := core.NewTreeReader(a.store)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return "", fmt.Errorf("failed to list files: %w", err)
	}

	// Apply path filters if provided.
	if len(filters) > 0 {
		normalized, err := worktree.NormalizePathFilters(a.dir, filters)
		if err != nil {
			return "", fmt.Errorf("failed to normalize filters: %w", err)
		}
		blobs = worktree.FilterBlobs(blobs, normalized)
	}

	actualOutput := output
	switch format {
	case ExportZip:
		if !strings.HasSuffix(output, ".zip") {
			actualOutput = output + ".zip"
		}
		if err := a.exportZip(blobs, actualOutput); err != nil {
			return "", err
		}
	case ExportTar:
		if !strings.HasSuffix(output, ".tar.gz") {
			actualOutput = output + ".tar.gz"
		}
		if err := a.exportTar(blobs, actualOutput); err != nil {
			return "", err
		}
	case ExportDir, "":
		if err := a.exportDir(blobs, output); err != nil {
			return "", err
		}
		actualOutput = output
	default:
		return "", fmt.Errorf("unsupported format: %s (use dir, zip, or tar)", format)
	}

	return actualOutput, nil
}

func (a *App) exportDir(blobs []core.BlobEntry, output string) error {
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("directory already exists: %s", output)
	}

	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}

	// Reuse a Worktree rooted at the output directory so symlink handling,
	// fast-path checks, and permission restoration are shared with restore.
	exportWt := worktree.New(a.store, output, a.wt.AutoCRLF)
	for _, blob := range blobs {
		if _, err := exportWt.WriteBlob(blob); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) exportZip(blobs []core.BlobEntry, output string) error {
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("file already exists: %s", output)
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}

	w := zip.NewWriter(f)
	writeErr := a.writeBlobsToZip(blobs, w)
	closeErr := w.Close()
	f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return fmt.Errorf("failed to finalize zip: %w", closeErr)
	}
	return nil
}

func (a *App) writeBlobsToZip(blobs []core.BlobEntry, w *zip.Writer) error {
	for _, blob := range blobs {
		if err := a.addBlobToZip(blob, w); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) exportTar(blobs []core.BlobEntry, output string) error {
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("file already exists: %s", output)
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	writeErr := a.writeBlobsToTar(blobs, tw)
	tarCloseErr := tw.Close()
	gzipCloseErr := gw.Close()
	f.Close()
	if writeErr != nil {
		return writeErr
	}
	if tarCloseErr != nil {
		return fmt.Errorf("failed to finalize tar: %w", tarCloseErr)
	}
	if gzipCloseErr != nil {
		return fmt.Errorf("failed to finalize gzip: %w", gzipCloseErr)
	}
	return nil
}

func (a *App) writeBlobsToTar(blobs []core.BlobEntry, tw *tar.Writer) error {
	for _, blob := range blobs {
		if err := a.addBlobToTar(blob, tw); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) addBlobToZip(blob core.BlobEntry, w *zip.Writer) error {
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return fmt.Errorf("unsafe export path %q: %w", blob.Path, err)
	}

	fh := &zip.FileHeader{
		Name:   blob.Path,
		Method: zip.Deflate,
	}
	fh.SetMode(os.FileMode(core.ToOSFileMode(blob.Mode)))

	f, err := w.CreateHeader(fh)
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
