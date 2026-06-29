package video

import (
	"github.com/your-org/drift/chunker"
)

// ChunkerFor returns the chunking strategy for a video file of the given
// size. Video files share the binary-class 3-tier strategy.
func (e *VideoEngine) ChunkerFor(fileSize int64) chunker.Chunker {
	return chunker.BinaryChunkerFor(fileSize)
}
