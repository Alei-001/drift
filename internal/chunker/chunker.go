package chunker

import (
	"context"
	"io"

	"github.com/Alei-001/drift/internal/core"
)

// Chunker splits an io.Reader into content-addressed chunks. The context
// lets callers cancel a long-running chunking operation mid-stream.
type Chunker interface {
	Chunk(ctx context.Context, r io.Reader) ([]*core.Chunk, error)
}
