package text

import (
	"io"

	"github.com/your-org/drift/chunker"
	"github.com/your-org/drift/core"
)

// Chunk uses FastCDC chunking for text files.
func (e *TextEngine) Chunk(r io.Reader) ([]*core.Chunk, error) {
	return chunker.NewFastCDCChunker().Chunk(r)
}
