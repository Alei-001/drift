package text

import (
	"github.com/your-org/drift/internal/chunker"
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
// defaults (4/8/16 KB — tuned for source code and structured text).
func (e *TextEngine) ChunkerFor(fileSize int64) chunker.Chunker {
	if fileSize < textWholeFileThreshold {
		return nil
	}
	return chunker.NewFastCDCChunkerWithParams(textChunkMinSize, textChunkAvgSize, textChunkMaxSize)
}
