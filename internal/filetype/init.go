package filetype

import (
	"github.com/Alei-001/drift/internal/filetype/binary"
	"github.com/Alei-001/drift/internal/filetype/image"
	"github.com/Alei-001/drift/internal/filetype/text"
	"github.com/Alei-001/drift/internal/filetype/video"
)

func init() {
	// Register engines in order: text → image → video → binary.
	// Binary is the fallback and will match anything.
	Register(text.NewEngine())
	Register(image.NewEngine())
	Register(video.NewEngine())
	Register(binary.NewEngine())
}
