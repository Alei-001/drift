package chunker

import "github.com/your-org/drift/internal/core"

type DefaultSelector struct{}

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
