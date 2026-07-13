package porcelain

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/filetype"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/storage/stream"
	"github.com/Alei-001/drift/internal/util/format"
	"github.com/Alei-001/drift/internal/util/pathutil"
)

var ErrInvalidPath = errors.New("invalid file path")

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
func ReadSnapshotFile(ctx context.Context, store storage.Storer, snapshot *core.Snapshot, workDir, filePath string) (FileViewResult, error) {
	normalizedPath, err := pathutil.RelToWorkDir(workDir, filePath)
	if err != nil {
		return FileViewResult{}, fmt.Errorf("%w: %w", ErrInvalidPath, err)
	}

	var targetEntry *core.FileEntry
	for i := range snapshot.Files {
		if snapshot.Files[i].Path == normalizedPath {
			targetEntry = &snapshot.Files[i]
			break
		}
	}
	if targetEntry == nil {
		return FileViewResult{}, fmt.Errorf("%w: %s", ErrFileNotFound, normalizedPath)
	}

	chunkR := stream.NewChunkReader(ctx, store, targetEntry.Chunks)
	header, fullReader, err := stream.PeekHeader(chunkR, core.HeaderPeekSize)
	if err != nil {
		return FileViewResult{}, fmt.Errorf("read file chunks: %w", err)
	}
	engine := filetype.DetectEngine(normalizedPath, header)

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
