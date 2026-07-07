package binary

import (
	"github.com/your-org/drift/internal/chunker"
)

// ChunkerFor delegates to the shared binary chunker strategy (FastCDC with
// size-tiered parameters). Explicitly declared here — rather than via an
// embedded chunker.DefaultSelector on the engine struct — so that the
// binary package mirrors the file layout of the text/image/video engines
// (engine.go + chunker.go + differ.go + preview.go).
func (e *BinaryEngine) ChunkerFor(fileSize int64) chunker.Chunker {
	return chunker.DefaultSelector{}.ChunkerFor(fileSize)
}
