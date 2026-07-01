package chunker

import "github.com/your-org/drift/core"

const (
	BinaryLargeThreshold = 50 * 1024 * 1024
	BinaryHugeThreshold  = 500 * 1024 * 1024
)

func BinaryChunkerFor(fileSize int64, cfg *core.CoreConfig) Chunker {
	minSize, avgSize, maxSize := 128*1024, 256*1024, 512*1024
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
	case fileSize < BinaryLargeThreshold:
		return NewFastCDCChunkerWithParams(minSize, avgSize, maxSize)
	case fileSize < BinaryHugeThreshold:
		// Scale all three proportionally to keep the user's min/avg/max
		// ratio while producing fewer, larger chunks for big files.
		return NewFastCDCChunkerWithParams(minSize*4, avgSize*4, maxSize*4)
	default:
		// For huge files, use fixed chunking with a size derived from
		// the user's avg, scaled up to reduce chunk count.
		return NewFixedChunker(avgSize * 8)
	}
}
