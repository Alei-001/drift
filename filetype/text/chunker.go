package text

import (
	"github.com/your-org/drift/chunker"
	"github.com/your-org/drift/core"
)

const textWholeFileThreshold = 64 * 1024

func (e *TextEngine) ChunkerFor(fileSize int64, cfg *core.CoreConfig) chunker.Chunker {
	if fileSize < textWholeFileThreshold {
		return nil
	}
	minSize, avgSize, maxSize := 4096, 8192, 16384
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
	return chunker.NewFastCDCChunkerWithParams(minSize, avgSize, maxSize)
}
