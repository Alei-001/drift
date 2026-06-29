package image

import (
	"github.com/your-org/drift/chunker"
)

// ChunkerFor returns the chunking strategy for an image file of the given
// size. Image files share the binary-class 3-tier strategy.
func (e *ImageEngine) ChunkerFor(fileSize int64) chunker.Chunker {
	return chunker.BinaryChunkerFor(fileSize)
}
