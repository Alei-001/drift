package snapshot

import (
	"archive/zip"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
)

// ExportResult reports the outcome of a snapshot export operation.
type ExportResult struct {
	FileCount int
	TotalSize int64
}

// ExportSnapshot reconstructs all files from the given snapshot and writes
// them to a zip archive at outputPath. Files are streamed chunk-by-chunk
// into the zip writer, so peak memory is bounded by the largest chunk
// rather than the largest file. Directory entries are preserved.
//
// The output path's parent directory is created if it does not exist.
// If the output file already exists it is overwritten.
func ExportSnapshot(ctx context.Context, st *store.StoreSet, snapID core.SnapshotID, outputPath string) (*ExportResult, error) {
	snap, err := st.Snapshots.GetSnapshot(ctx, snapID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	fileCount := 0
	for _, entry := range snap.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Sanitize the zip entry path to prevent zip-slip: a malicious
		// snapshot with entry.Path = "../../etc/cron.d/evil" would embed
		// a path-traversal entry in the zip, which naive extractors
		// (including some server-side pipelines) may follow to write
		// outside the extraction directory. Strip leading "../", "./",
		// and "/" so the entry is always relative to the zip root.
		zipPath := filepath.ToSlash(filepath.Clean(entry.Path))
		zipPath = strings.TrimPrefix(zipPath, "../")
		zipPath = strings.TrimPrefix(zipPath, "./")
		zipPath = strings.TrimPrefix(zipPath, "/")
		if zipPath == "" || strings.HasPrefix(zipPath, "../") {
			slog.Warn("skipping zip entry with escaping path", "path", entry.Path)
			continue
		}

		if entry.Mode.IsDir() {
			if !strings.HasSuffix(zipPath, "/") {
				zipPath += "/"
			}
			fh := &zip.FileHeader{
				Name:   zipPath,
				Method: zip.Store,
			}
			fh.SetMode(os.FileMode(entry.Mode))
			if _, err := zw.CreateHeader(fh); err != nil {
				return nil, fmt.Errorf("create zip dir entry %s: %w", entry.Path, err)
			}
			continue
		}

		fh := &zip.FileHeader{
			Name:   zipPath,
			Method: zip.Deflate,
		}
		perm := os.FileMode(entry.Mode & 0o777)
		// Mask group/other write bits (umask 0o022 semantics) for
		// consistency with restore/import.
		perm &^= 0o022
		if perm == 0 {
			perm = 0644
		}
		fh.SetMode(perm)

		w, err := zw.CreateHeader(fh)
		if err != nil {
			return nil, fmt.Errorf("create zip entry %s: %w", entry.Path, err)
		}

		for _, h := range entry.Chunks {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			chunk, err := st.Chunks.GetChunk(ctx, h)
			if err != nil {
				return nil, fmt.Errorf("get chunk %s for %s: %w", h.String(), entry.Path, err)
			}
			if _, err := w.Write(chunk.Data); err != nil {
				return nil, fmt.Errorf("write chunk data for %s: %w", entry.Path, err)
			}
		}

		fileCount++
	}

	slog.Info("snapshot exported", "id", snap.ShortID(), "files", fileCount, "size", snap.TotalSize, "output", outputPath)

	return &ExportResult{
		FileCount: fileCount,
		TotalSize: snap.TotalSize,
	}, nil
}
