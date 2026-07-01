package binary

import (
	"github.com/your-org/drift/chunker"
	"github.com/your-org/drift/core"
)

func (e *BinaryEngine) ChunkerFor(fileSize int64, cfg *core.CoreConfig) chunker.Chunker {
	return chunker.BinaryChunkerFor(fileSize, cfg)
}
