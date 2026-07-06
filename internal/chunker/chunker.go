package chunker

import (
	"io"

	"github.com/your-org/drift/internal/core"
)

// Chunker splits an io.Reader into content-addressed chunks.
type Chunker interface {
	Chunk(r io.Reader) ([]*core.Chunk, error)
}
