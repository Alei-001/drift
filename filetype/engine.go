package filetype

import (
	"github.com/your-org/drift/chunker"
	"github.com/your-org/drift/core"
)

type Engine interface {
	Detector
	ChunkerSelector
	Differ
	Previewer
}

type Detector interface {
	Name() string
	DetectByMagic(header []byte) bool
	DetectByExtension(path string) bool
	DetectByHeuristic(path string, header []byte) bool
}

type ChunkerSelector interface {
	ChunkerFor(fileSize int64, cfg *core.CoreConfig) chunker.Chunker
}

type Differ interface {
	Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error)
}

type Previewer interface {
	Preview(data []byte, maxLines int) string
}
