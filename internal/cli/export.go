package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export <version>",
	Short: "Export a version or branch to a directory or archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		version := args[0]
		output, _ := cmd.Flags().GetString("output")
		format, _ := cmd.Flags().GetString("format")

		if output == "" {
			return fmt.Errorf("output path is required (use -o flag)")
		}

		commit, err := resolveCommit(sharedStore, version)
		if err != nil {
			return err
		}

		tree, err := sharedStore.GetTree(commit.TreeHash)
		if err != nil {
			return fmt.Errorf("failed to load tree: %w", err)
		}

		reader := core.NewTreeReader(sharedStore)
		blobs, err := reader.ListBlobs(tree, "")
		if err != nil {
			return fmt.Errorf("failed to list files: %w", err)
		}

		switch format {
		case "zip":
			return exportZip(sharedStore, blobs, output)
		case "tar", "tar.gz":
			return exportTar(sharedStore, blobs, output)
		case "dir", "":
			return exportDir(sharedStore, blobs, output, len(blobs))
		default:
			return fmt.Errorf("unsupported format: %s (use dir, zip, or tar)", format)
		}
	},
}

func init() {
	exportCmd.Flags().StringP("output", "o", "", "Output path")
	exportCmd.Flags().StringP("format", "f", "dir", "Export format: dir, zip, tar")
	rootCmd.AddCommand(exportCmd)
}

func exportDir(store *storage.Store, blobs []core.BlobEntry, output string, total int) error {
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("directory already exists: %s", output)
	}

	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}

	for i, blob := range blobs {
		if err := writeBlobToFile(store, blob, output); err != nil {
			return err
		}
		fmt.Printf("\rExporting: %d/%d", i+1, total)
	}
	fmt.Println()

	fmt.Printf("Exported %d file(s) to %s\n", len(blobs), output)
	return nil
}

func exportZip(store *storage.Store, blobs []core.BlobEntry, output string) error {
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
		if err := addBlobToZip(store, blob, w); err != nil {
			return err
		}
	}

	fmt.Printf("Exported %d file(s) to %s\n", len(blobs), output)
	return nil
}

func exportTar(store *storage.Store, blobs []core.BlobEntry, output string) error {
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
		if err := addBlobToTar(store, blob, tw); err != nil {
			return err
		}
	}

	fmt.Printf("Exported %d file(s) to %s\n", len(blobs), output)
	return nil
}

func writeBlobToFile(store *storage.Store, blob core.BlobEntry, outputDir string) error {
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return fmt.Errorf("unsafe export path %q: %w", blob.Path, err)
	}

	data, err := store.GetBlob(blob.Hash)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(outputDir, filepath.FromSlash(blob.Path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	perm := os.FileMode(core.ToOSFileMode(blob.Mode))
	if err := os.WriteFile(fullPath, data, perm); err != nil {
		return err
	}
	return os.Chmod(fullPath, perm)
}

func addBlobToZip(store *storage.Store, blob core.BlobEntry, w *zip.Writer) error {
	// P3-#18: validate path before using it as a zip entry name, preventing
	// path traversal in the extracted archive.
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return fmt.Errorf("unsafe export path %q: %w", blob.Path, err)
	}

	data, err := store.GetBlob(blob.Hash)
	if err != nil {
		return err
	}

	f, err := w.Create(blob.Path)
	if err != nil {
		return err
	}

	_, err = f.Write(data)
	return err
}

func addBlobToTar(store *storage.Store, blob core.BlobEntry, tw *tar.Writer) error {
	// P3-#18: validate path before using it as a tar entry name.
	if err := core.ValidateTreePath(blob.Path); err != nil {
		return fmt.Errorf("unsafe export path %q: %w", blob.Path, err)
	}

	data, err := store.GetBlob(blob.Hash)
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name: blob.Path,
		Mode: int64(blob.Mode),
		Size: int64(len(data)),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	_, err = tw.Write(data)
	return err
}
