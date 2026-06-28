package binary

import (
	"io"

	"github.com/your-org/drift/chunker"
	"github.com/your-org/drift/core"
)

// Chunk streams data through FastCDC chunking directly without buffering.
func (e *BinaryEngine) Chunk(r io.Reader) ([]*core.Chunk, error) {
	return chunker.NewFastCDCChunker().Chunk(r)
}
