package chunker

import (
	"io"

	"github.com/your-org/drift/core"
)

type Chunker interface {
	Chunk(r io.Reader) ([]*core.Chunk, error)
}
