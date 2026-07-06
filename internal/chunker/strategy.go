package chunker

import "github.com/your-org/drift/internal/core"

// DefaultSelector is the default Chunker selector: it delegates to
// BinaryChunkerFor for all files.
type DefaultSelector struct{}

// ChunkerFor returns a Chunker appropriate for the given file size and
// configuration. It currently always delegates to BinaryChunkerFor.
func (DefaultSelector) ChunkerFor(fileSize int64, cfg *core.CoreConfig) Chunker {
	return BinaryChunkerFor(fileSize, cfg)
}

// binaryLargeThreshold and binaryHugeThreshold are the file-size cutoffs that
// select which chunking strategy to use. They are unexported because no
// external package needs to reference them — BinaryChunkerFor encapsulates
// the entire size-based dispatch.
const (
	binaryLargeThreshold = 50 * 1024 * 1024   // 50MB
	binaryHugeThreshold  = 500 * 1024 * 1024  // 500MB
)

// BinaryChunkerFor returns a Chunker tuned for binary file content of the
// given size. Files below binaryLargeThreshold use FastCDC with the
// configured sizes; files between binaryLargeThreshold and
// binaryHugeThreshold use FastCDC with sizes scaled 4×; files at or above
// binaryHugeThreshold use fixed chunking derived from the configured average
// size scaled 8×. A nil cfg is treated as the package defaults.
func BinaryChunkerFor(fileSize int64, cfg *core.CoreConfig) Chunker {
	// Default to the package-level constants (which alias core defaults).
	// A cfg with chunk sizes > 0 overrides them; 0 means "use engine default"
	// so partial JSON configs can opt into per-engine sizing.
	minSize, avgSize, maxSize := fastCDCDefaultMinSize, fastCDCDefaultAvgSize, fastCDCDefaultMaxSize
	if cfg != nil {
		if cfg.ChunkMinSize > 0 {
			minSize = cfg.ChunkMinSize
		}
		if cfg.ChunkAvgSize > 0 {
			avgSize = cfg.ChunkAvgSize
		}
		if cfg.ChunkMaxSize > 0 {
			maxSize = cfg.ChunkMaxSize
		}
	}

	switch {
	case fileSize < binaryLargeThreshold:
		return NewFastCDCChunkerWithParams(minSize, avgSize, maxSize)
	case fileSize < binaryHugeThreshold:
		// Scale all three proportionally to keep the user's min/avg/max
		// ratio while producing fewer, larger chunks for big files.
		return NewFastCDCChunkerWithParams(minSize*4, avgSize*4, maxSize*4)
	default:
		// For huge files, use fixed chunking with a size derived from
		// the user's avg, scaled up to reduce chunk count.
		return NewFixedChunker(avgSize * 8)
	}
}
