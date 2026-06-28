package filetype

import (
	"io"

	"github.com/your-org/drift/core"
)

// Engine combines all file-type-specific capabilities.
type Engine interface {
	Detector
	Chunker
	Differ
	Previewer
}

// Detector checks if an engine can handle a file.
type Detector interface {
	Detect(path string, header []byte) bool
	Name() string
}

// Chunker creates chunks from file content.
type Chunker interface {
	Chunk(r io.Reader) ([]*core.Chunk, error)
}

// Differ produces diff between two file contents.
type Differ interface {
	Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error)
}

// Previewer generates a text preview of file content.
type Previewer interface {
	Preview(data []byte, maxLines int) string
}
