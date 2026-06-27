package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"os"
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
