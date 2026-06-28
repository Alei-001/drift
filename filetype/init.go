package filetype

import (
	"github.com/your-org/drift/filetype/binary"
	"github.com/your-org/drift/filetype/text"
)

func init() {
	// Register text first so it's checked first in the Detect loop.
	// Binary is the fallback and will match anything.
	Register(text.NewEngine())
	Register(binary.NewEngine())
}
