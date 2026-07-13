package filetype

import (
	"github.com/Alei-001/drift/internal/filetype/binary"
	"github.com/Alei-001/drift/internal/filetype/image"
	"github.com/Alei-001/drift/internal/filetype/text"
	"github.com/Alei-001/drift/internal/filetype/video"
)

func init() {
	// Engine registration order matters for heuristic detection: text
	// must be registered before binary, because binary's
	// DetectByHeuristic always returns true and would short-circuit text
	// detection. The order is: text → image → video → binary.
	// Binary is also set as the explicit fallback so Detect never
	// returns nil, even if a future change removes binary from the
	// heuristic layer.
	binEngine := binary.NewEngine()
	Register(text.NewEngine())
	Register(image.NewEngine())
	Register(video.NewEngine())
	Register(binEngine)
	SetFallback(binEngine)
}
