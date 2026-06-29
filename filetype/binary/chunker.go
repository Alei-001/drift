package binary

import (
	"github.com/your-org/drift/chunker"
)

// ChunkerFor returns the chunking strategy for a binary file of the given
// size. Binary files use the shared binary-class 3-tier strategy.
func (e *BinaryEngine) ChunkerFor(fileSize int64) chunker.Chunker {
	return chunker.BinaryChunkerFor(fileSize)
}
