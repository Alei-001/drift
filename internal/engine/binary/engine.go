package binary

import (
	"io"

	"github.com/Alei-001/drift/internal/chunker"
	"github.com/Alei-001/drift/internal/core"
)

// BinaryEngine is the fallback engine for binary files.
type BinaryEngine struct{}

// NewEngine creates a new BinaryEngine.
func NewEngine() *BinaryEngine {
	return &BinaryEngine{}
}

// Name returns "binary".
func (e *BinaryEngine) Name() string {
	return "binary"
}

// DetectByMagic returns false; binary has no magic signature.
func (e *BinaryEngine) DetectByMagic(header []byte) bool {
	return false
}

// DetectByExtension returns false; binary matches no specific extension.
func (e *BinaryEngine) DetectByExtension(path string) bool {
	return false
}

// DetectByHeuristic returns true; binary is the final fallback engine and
// matches any file that no other engine claimed.
func (e *BinaryEngine) DetectByHeuristic(path string, header []byte) bool {
	return true
}

// ChunkerFor delegates to the shared binary chunker strategy.
func (e *BinaryEngine) ChunkerFor(fileSize int64) chunker.Chunker {
	return chunker.DefaultSelector{}.ChunkerFor(fileSize)
}

// Metadata returns the file metadata for binary files.
func (e *BinaryEngine) Metadata() *core.FileMetadata {
	return &core.FileMetadata{MIMEType: "application/octet-stream"}
}

// Preview returns a placeholder indicating binary content.
func (e *BinaryEngine) Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error) {
	return "[binary file]", nil
}
