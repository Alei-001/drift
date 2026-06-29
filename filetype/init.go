package filetype

import (
	"github.com/your-org/drift/filetype/binary"
	"github.com/your-org/drift/filetype/image"
	"github.com/your-org/drift/filetype/text"
	"github.com/your-org/drift/filetype/video"
)

func init() {
	// Register engines in order: text → image → video → binary.
	// Binary is the fallback and will match anything.
	Register(text.NewEngine())
	Register(image.NewEngine())
	Register(video.NewEngine())
	Register(binary.NewEngine())
}
