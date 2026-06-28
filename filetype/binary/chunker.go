package binary

import (
	"bytes"
	"io"

	"github.com/your-org/drift/chunker"
	"github.com/your-org/drift/core"
)

const largeFileThreshold = 104857600 // 100MB

// Chunk reads all data from the reader and selects an appropriate chunking strategy.
func (e *BinaryEngine) Chunk(r io.Reader) ([]*core.Chunk, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	if len(data) > largeFileThreshold {
		return chunker.NewFixedChunker(65536).Chunk(bytes.NewReader(data))
	}

	return chunker.NewFastCDCChunker().Chunk(bytes.NewReader(data))
}
