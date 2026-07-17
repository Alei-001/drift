package snapshot

import (
	"github.com/Alei-001/drift/internal/errs"
	"context"
	"fmt"
	"io"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/engine"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/stream"
	"github.com/Alei-001/drift/internal/util/format"
	"github.com/Alei-001/drift/internal/util/pathutil"
)



// FileViewResult holds the content and metadata of a file read from a snapshot.
type FileViewResult struct {
	Path       string
	Kind       string
	Size       int64
	ModTime    int64
	Dimensions string
	Content    []byte
}

// ReadSnapshotFile reads the full content of a file from a snapshot,
// reassembling its chunks and detecting the file type.
func ReadSnapshotFile(ctx context.Context, st *store.StoreSet, snapshot *core.Snapshot, workDir, filePath string) (FileViewResult, error) {
	normalizedPath, err := pathutil.RelToWorkDir(workDir, filePath)
	if err != nil {
		return FileViewResult{}, fmt.Errorf("%w: %w", errs.ErrInvalidPath, err)
	}

	var targetEntry *core.FileEntry
	for i := range snapshot.Files {
		if snapshot.Files[i].Path == normalizedPath {
			targetEntry = &snapshot.Files[i]
			break
		}
	}
	if targetEntry == nil {
		return FileViewResult{}, fmt.Errorf("%w: %s", errs.ErrFileNotFound, normalizedPath)
	}

	chunkR := stream.NewChunkReader(ctx, st.Chunks, targetEntry.Chunks)
	header, fullReader, err := stream.PeekHeader(chunkR, core.HeaderPeekSize)
	if err != nil {
		return FileViewResult{}, fmt.Errorf("read file chunks: %w", err)
	}
	engine := engine.DetectEngine(normalizedPath, header)

	content, err := io.ReadAll(fullReader)
	if err != nil {
		return FileViewResult{}, fmt.Errorf("read file content: %w", err)
	}

	result := FileViewResult{
		Path:    normalizedPath,
		Size:    targetEntry.Size,
		ModTime: targetEntry.ModTime,
		Content: content,
	}

	if engine == nil {
		result.Kind = "binary"
		return result, nil
	}

	result.Kind = engine.Name()
	if engine.Name() == "image" {
		result.Dimensions = format.ImageDimensions(header)
	}

	return result, nil
}
