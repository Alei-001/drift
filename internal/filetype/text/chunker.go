package text

import (
	"github.com/your-org/drift/internal/chunker"
	"github.com/your-org/drift/internal/core"
)

// Whole-file chunking threshold: text files smaller than this are stored as a
// single chunk.
const textWholeFileThreshold = 64 * 1024

// Default FastCDC chunk-size parameters for text files larger than
// textWholeFileThreshold. Sizes are tuned for source code and structured text.
const (
	textChunkMinSize = 4 * 1024
	textChunkAvgSize = 8 * 1024
	textChunkMaxSize = 16 * 1024
)

// ChunkerFor selects the chunking strategy for a text file. Files smaller than
// textWholeFileThreshold return nil, signalling the caller to store the whole
// file as a single chunk. Larger files use FastCDC with the text-tiered
// defaults, overridden by cfg when non-zero values are provided.
func (e *TextEngine) ChunkerFor(fileSize int64, cfg *core.CoreConfig) chunker.Chunker {
	if fileSize < textWholeFileThreshold {
		return nil
	}
	minSize, avgSize, maxSize := textChunkMinSize, textChunkAvgSize, textChunkMaxSize
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
