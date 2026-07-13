package chunker

import (
	"context"
	"io"

	"github.com/Alei-001/drift/internal/core"
)

// Chunker splits an io.Reader into content-addressed chunks, invoking fn
// for each chunk as it is produced. This streaming interface avoids
// accumulating all chunks in memory for large files. The context lets
// callers cancel a long-running chunking operation mid-stream. If fn
// returns a non-nil error, Chunk stops and returns that error.
type Chunker interface {
	Chunk(ctx context.Context, r io.Reader, fn func(*core.Chunk) error) error
}
